package websocket

import (
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/nstalgic/nekzus/internal/discovery"
	"github.com/nstalgic/nekzus/internal/metrics"
	"github.com/nstalgic/nekzus/internal/storage"
	"github.com/nstalgic/nekzus/internal/types"
)

var log = slog.With("package", "websocket")

// Client represents a connected WebSocket client
type Client struct {
	deviceID      string
	conn          *websocket.Conn
	sendChan      chan types.WebSocketMessage
	subscriptions map[string]SubscriptionOptions // topic pattern -> options
	lastWill      *LastWill                       // message to publish on unexpected disconnect
	subMu         sync.RWMutex                    // protects subscriptions and lastWill
	mu            sync.Mutex
	chanClosed    bool // Guard flag to prevent double-close of sendChan
	closeChanMu   sync.Mutex
}

// NewClient creates a new WebSocket client
func NewClient(deviceID string, conn *websocket.Conn, sendChan chan types.WebSocketMessage) *Client {
	return &Client{
		deviceID:      deviceID,
		conn:          conn,
		sendChan:      sendChan,
		subscriptions: make(map[string]SubscriptionOptions),
	}
}

// SubscribeToTopics adds topic subscriptions for this client.
// If no subscriptions are set, client receives all messages (backward compatible).
func (c *Client) SubscribeToTopics(patterns []string, opts SubscriptionOptions) error {
	c.subMu.Lock()
	defer c.subMu.Unlock()

	// Check if adding these patterns would exceed the limit
	newCount := 0
	for _, pattern := range patterns {
		if _, exists := c.subscriptions[pattern]; !exists {
			newCount++
		}
	}
	if len(c.subscriptions)+newCount > MaxSubscriptionsPerClient {
		return fmt.Errorf("subscription limit exceeded (max %d)", MaxSubscriptionsPerClient)
	}

	for _, pattern := range patterns {
		if err := ValidatePattern(pattern); err != nil {
			return fmt.Errorf("invalid pattern %q: %w", pattern, err)
		}
		c.subscriptions[pattern] = opts
	}
	return nil
}

// UnsubscribeFromTopics removes topic subscriptions for this client.
func (c *Client) UnsubscribeFromTopics(patterns []string) {
	c.subMu.Lock()
	defer c.subMu.Unlock()

	for _, pattern := range patterns {
		delete(c.subscriptions, pattern)
	}
}

// GetSubscriptions returns a copy of the client's subscriptions.
func (c *Client) GetSubscriptions() map[string]SubscriptionOptions {
	c.subMu.RLock()
	defer c.subMu.RUnlock()

	result := make(map[string]SubscriptionOptions, len(c.subscriptions))
	for k, v := range c.subscriptions {
		result[k] = v
	}
	return result
}

// HasSubscriptions returns true if client has explicit subscriptions.
func (c *Client) HasSubscriptions() bool {
	c.subMu.RLock()
	defer c.subMu.RUnlock()
	return len(c.subscriptions) > 0
}

// IsSubscribedTo checks if client is subscribed to receive messages for a topic.
// Returns true if client has no subscriptions (receives all) or has a matching pattern.
func (c *Client) IsSubscribedTo(topic string) bool {
	c.subMu.RLock()
	defer c.subMu.RUnlock()

	// No subscriptions means receive all messages (backward compatible)
	if len(c.subscriptions) == 0 {
		return true
	}

	// Check if any subscription pattern matches
	for pattern := range c.subscriptions {
		if MatchTopic(pattern, topic) {
			return true
		}
	}
	return false
}

// GetSubscriptionQoS returns the QoS level for a topic.
// Returns 0 if no matching subscription found.
func (c *Client) GetSubscriptionQoS(topic string) int {
	c.subMu.RLock()
	defer c.subMu.RUnlock()

	for pattern, opts := range c.subscriptions {
		if MatchTopic(pattern, topic) {
			return opts.QoS
		}
	}
	return QoSAtMostOnce
}

// SetLastWill sets the last will message for this client.
func (c *Client) SetLastWill(lw *LastWill) {
	c.subMu.Lock()
	defer c.subMu.Unlock()
	c.lastWill = lw
}

// GetLastWill returns the last will message for this client.
func (c *Client) GetLastWill() *LastWill {
	c.subMu.RLock()
	defer c.subMu.RUnlock()
	return c.lastWill
}

// ClearLastWill removes the last will message.
func (c *Client) ClearLastWill() {
	c.subMu.Lock()
	defer c.subMu.Unlock()
	c.lastWill = nil
}

// GetDeviceID returns the device ID for this client
func (c *Client) GetDeviceID() string {
	return c.deviceID
}

// GetSendChan returns the send channel for this client (for message writing)
func (c *Client) GetSendChan() chan types.WebSocketMessage {
	return c.sendChan
}

// closeSendChanSafe safely closes the sendChan, preventing double-close panics
func (c *Client) closeSendChanSafe() {
	c.closeChanMu.Lock()
	defer c.closeChanMu.Unlock()

	if !c.chanClosed && c.sendChan != nil {
		close(c.sendChan)
		c.chanClosed = true
	}
}

// clientSlicePool is a sync.Pool for reusing client slice buffers during broadcast
// This reduces GC pressure by reusing allocations instead of creating new slices on every broadcast
var clientSlicePool = sync.Pool{
	New: func() interface{} {
		// Pre-allocate with capacity for typical number of clients
		return make([]*Client, 0, 100)
	},
}

// Manager manages WebSocket connections and message broadcasting
type Manager struct {
	clients           map[*Client]struct{}
	clientsByDevice   map[string][]*Client   // deviceID -> clients (for quick lookup)
	retainedMessages  map[string]*RetainedMessage // topic -> retained message
	mu                sync.RWMutex
	retainedMu        sync.RWMutex
	metrics           *metrics.Metrics
	storage           *storage.Store
	pingInterval      time.Duration         // Configurable ping interval for testing
	pongWait          time.Duration         // Configurable pong wait timeout
	authTimeout       time.Duration         // Configurable auth timeout for testing
	onDeviceConnectCb func(deviceID string) // Callback when device connects
}

// NewManager creates a new WebSocket manager
func NewManager(m *metrics.Metrics, store *storage.Store) *Manager {
	return &Manager{
		clients:          make(map[*Client]struct{}),
		clientsByDevice:  make(map[string][]*Client),
		retainedMessages: make(map[string]*RetainedMessage),
		metrics:          m,
		storage:          store,
		pingInterval:     30 * time.Second, // Default ping interval
		pongWait:         60 * time.Second, // Default pong wait timeout
		authTimeout:      15 * time.Second, // Default auth timeout
	}
}

// Subscribe adds a client to the WebSocket manager
func (wm *Manager) Subscribe(client *Client) {
	wm.mu.Lock()
	deviceID := client.deviceID
	callback := wm.onDeviceConnectCb
	wm.clients[client] = struct{}{}
	wm.clientsByDevice[deviceID] = append(wm.clientsByDevice[deviceID], client)
	clientCount := len(wm.clients)
	wm.mu.Unlock()

	// Record metrics
	if wm.metrics != nil {
		wm.metrics.WebSocketConnectionsActive.Inc()
		wm.metrics.WebSocketConnectionsTotal.WithLabelValues("subscribe", "success").Inc()
	}

	log.Info("websocket client subscribed",
		"device_id", deviceID,
		"total_clients", clientCount)

	// Trigger device connect callback (for notification retries)
	if callback != nil {
		go callback(deviceID)
	}
}

// SetOnDeviceConnect sets the callback to be called when a device connects
func (wm *Manager) SetOnDeviceConnect(callback func(deviceID string)) {
	wm.mu.Lock()
	defer wm.mu.Unlock()
	wm.onDeviceConnectCb = callback
}

// Unsubscribe removes a client from the WebSocket manager
func (wm *Manager) Unsubscribe(client *Client) {
	wm.unsubscribeClient(client, false)
}

// UnsubscribeUnexpected removes a client and publishes its last will message
func (wm *Manager) UnsubscribeUnexpected(client *Client) {
	wm.unsubscribeClient(client, true)
}

// unsubscribeClient handles client removal with optional LWT publishing
func (wm *Manager) unsubscribeClient(client *Client, publishLastWill bool) {
	// Get last will before locking manager (to avoid nested locks)
	var lastWill *LastWill
	if publishLastWill {
		lastWill = client.GetLastWill()
	}

	wm.mu.Lock()
	if _, exists := wm.clients[client]; exists {
		delete(wm.clients, client)

		// Remove from clientsByDevice
		deviceID := client.deviceID
		clients := wm.clientsByDevice[deviceID]
		for i, c := range clients {
			if c == client {
				wm.clientsByDevice[deviceID] = append(clients[:i], clients[i+1:]...)
				break
			}
		}
		if len(wm.clientsByDevice[deviceID]) == 0 {
			delete(wm.clientsByDevice, deviceID)
		}

		// Close the send channel safely to prevent double-close
		client.closeSendChanSafe()

		// Record metrics
		if wm.metrics != nil {
			wm.metrics.WebSocketConnectionsActive.Dec()
		}

		log.Info("websocket client unsubscribed",
			"device_id", client.deviceID,
			"remaining_clients", len(wm.clients),
			"unexpected", publishLastWill)
	}
	wm.mu.Unlock()

	// Publish last will after releasing lock
	if lastWill != nil {
		wm.publishLastWillMessage(lastWill)
	}
}

// publishLastWillMessage publishes a last will message
func (wm *Manager) publishLastWillMessage(lw *LastWill) {
	msg := lw.Message
	msg.Topic = lw.Topic
	msg.QoS = lw.QoS
	msg.Timestamp = time.Now()

	log.Info("publishing last will message",
		"topic", lw.Topic,
		"qos", lw.QoS)

	wm.PublishToTopic(lw.Topic, msg)
}

// Broadcast sends a message to all connected clients
func (wm *Manager) Broadcast(msg types.WebSocketMessage) {
	wm.BroadcastFiltered(msg, nil)
}

// BroadcastFiltered sends a message to clients that pass the filter function
// If filter is nil, broadcasts to all clients
// Uses sync.Pool to reuse slice allocations and reduce GC pressure
func (wm *Manager) BroadcastFiltered(msg types.WebSocketMessage, filter func(*Client) bool) {
	// Get a slice from the pool
	clients := clientSlicePool.Get().([]*Client)
	clients = clients[:0] // Reset length while preserving capacity

	// Copy client list while holding lock (minimize lock time)
	wm.mu.RLock()
	totalClients := len(wm.clients)
	for client := range wm.clients {
		// Apply filter if provided
		if filter != nil && !filter(client) {
			continue
		}
		clients = append(clients, client)
	}
	wm.mu.RUnlock()

	// Broadcast to copied list (lock not held)
	sentCount := 0
	droppedCount := 0
	for _, client := range clients {
		// Non-blocking send to avoid blocking on slow clients
		// Recover from panics if channel is closed during broadcast
		func() {
			defer func() {
				if r := recover(); r != nil {
					droppedCount++
					log.Warn("failed to send message to client (channel closed)",
						"device_id", client.deviceID,
						"panic", r)
				}
			}()

			if client.sendChan != nil {
				select {
				case client.sendChan <- msg:
					sentCount++
				default:
					// Channel full - drop message
					droppedCount++
					log.Warn("failed to send message to client (channel full)",
						"device_id", client.deviceID)
				}
			}
		}()
	}

	// Record metrics
	if wm.metrics != nil {
		wm.metrics.WebSocketMessagesTotal.WithLabelValues("broadcast", "sent").Add(float64(sentCount))
		if droppedCount > 0 {
			wm.metrics.WebSocketMessagesTotal.WithLabelValues("broadcast", "dropped").Add(float64(droppedCount))
		}
	}

	// Log at Info level if any messages were dropped or if debugging delivery issues
	if droppedCount > 0 {
		log.Warn("broadcast message had drops",
			"type", msg.Type,
			"sent_count", sentCount,
			"total_clients", totalClients,
			"dropped", droppedCount)
	} else {
		log.Info("broadcast message",
			"type", msg.Type,
			"sent_count", sentCount,
			"total_clients", totalClients)
	}

	// Return the slice to the pool for reuse
	clientSlicePool.Put(clients)
}

// ActiveConnections returns the number of active WebSocket connections
func (wm *Manager) ActiveConnections() int {
	wm.mu.RLock()
	defer wm.mu.RUnlock()
	return len(wm.clients)
}

// GetConnectedDevices returns a list of all connected device IDs with connection count
func (wm *Manager) GetConnectedDevices() map[string]int {
	wm.mu.RLock()
	defer wm.mu.RUnlock()

	devices := make(map[string]int)
	for client := range wm.clients {
		devices[client.deviceID]++
	}
	return devices
}

// HasDeviceConnection checks if a device has an active WebSocket connection
func (wm *Manager) HasDeviceConnection(deviceID string) bool {
	wm.mu.RLock()
	defer wm.mu.RUnlock()

	// Log all connected device IDs for debugging
	var connectedIDs []string
	for client := range wm.clients {
		connectedIDs = append(connectedIDs, client.deviceID)
		if client.deviceID == deviceID {
			log.Info("HasDeviceConnection found match", "device_id", deviceID, "all_connected", connectedIDs)
			return true
		}
	}
	log.Info("HasDeviceConnection no match", "device_id", deviceID, "all_connected", connectedIDs)
	return false
}

// PublishDiscoveryEvent publishes a discovery event to all connected clients
func (wm *Manager) PublishDiscoveryEvent(eventData interface{}) {
	// Extract proposal directly from discovery.Event
	// Mobile app expects Data to be the Proposal object directly
	var data interface{}
	if evt, ok := eventData.(discovery.Event); ok {
		data = evt.GetData()
	} else {
		data = eventData
	}

	msg := types.WebSocketMessage{
		Type: types.WSMsgTypeDiscovery,
		Data: data,
	}
	wm.Broadcast(msg)
}

// PublishConfigReload publishes a config reload event to all connected clients
func (wm *Manager) PublishConfigReload() {
	msg := types.WebSocketMessage{
		Type: types.WSMsgTypeConfigReload,
		Data: map[string]string{
			"message": "Configuration reloaded",
		},
	}
	wm.Broadcast(msg)
}

// PublishConfigWarning publishes a config warning event to all connected clients
func (wm *Manager) PublishConfigWarning(warning string) {
	msg := types.WebSocketMessage{
		Type: types.WSMsgTypeConfigWarning,
		Data: map[string]string{
			"warning": warning,
		},
	}
	wm.Broadcast(msg)
}

// PublishDevicePaired publishes a device paired event to all connected clients
func (wm *Manager) PublishDevicePaired(deviceID, deviceName, platform string) {
	msg := types.WebSocketMessage{
		Type: types.WSMsgTypeDevicePaired,
		Data: map[string]string{
			"deviceId":   deviceID,
			"deviceName": deviceName,
			"platform":   platform,
		},
	}
	wm.Broadcast(msg)
}

// PublishDeviceRevoked publishes a device revoked event to all connected clients
func (wm *Manager) PublishDeviceRevoked(deviceID string) {
	msg := types.WebSocketMessage{
		Type: types.WSMsgTypeDeviceRevoked,
		Data: map[string]string{
			"deviceId": deviceID,
		},
	}
	wm.Broadcast(msg)
}

// PublishHealthChange publishes a health status change event to all connected clients
func (wm *Manager) PublishHealthChange(appID, appName, proxyPath, status, message string) {
	msg := types.WebSocketMessage{
		Type: types.WSMsgTypeHealthChange,
		Data: map[string]interface{}{
			"appId":     appID,
			"appName":   appName,
			"proxyPath": proxyPath,
			"status":    status,
			"message":   message,
			"timestamp": time.Now().Unix(),
		},
	}
	wm.Broadcast(msg)
}

// PublishPortExposureWarning publishes a port exposure security warning to all connected clients
func (wm *Manager) PublishPortExposureWarning(appID, appName, riskLevel, summary string, bindings []map[string]interface{}, recommendations []string) {
	msg := types.WebSocketMessage{
		Type: types.WSMsgTypePortExposure,
		Data: map[string]interface{}{
			"appId":           appID,
			"appName":         appName,
			"riskLevel":       riskLevel,
			"summary":         summary,
			"bindings":        bindings,
			"recommendations": recommendations,
			"timestamp":       time.Now().Unix(),
		},
		Timestamp: time.Now(),
	}
	wm.Broadcast(msg)
}

// PublishRouteAdded publishes a route added event to all connected clients.
// This is called when a discovery proposal is approved and a new route is created.
func (wm *Manager) PublishRouteAdded(route types.Route) {
	msg := types.WebSocketMessage{
		Type: types.WSMsgTypeRouteAdded,
		Data: map[string]interface{}{
			"route":     route,
			"timestamp": time.Now().Unix(),
		},
		Timestamp: time.Now(),
	}
	wm.Broadcast(msg)
}

// PublishRouteRemoved publishes a route removed event to all connected clients.
// This is called when a route is deleted from the system.
func (wm *Manager) PublishRouteRemoved(routeID string) {
	msg := types.WebSocketMessage{
		Type: types.WSMsgTypeRouteRemoved,
		Data: map[string]interface{}{
			"routeId":   routeID,
			"timestamp": time.Now().Unix(),
		},
		Timestamp: time.Now(),
	}
	wm.Broadcast(msg)
}

// PublishProposalApproved publishes a proposal approved event to all connected clients.
// This notifies the UI that a discovery proposal has been approved.
func (wm *Manager) PublishProposalApproved(proposalID string) {
	msg := types.WebSocketMessage{
		Type: types.WSMsgTypeProposalApproved,
		Data: map[string]interface{}{
			"proposalId": proposalID,
			"timestamp":  time.Now().Unix(),
		},
		Timestamp: time.Now(),
	}
	wm.Broadcast(msg)
}

// PublishProposalDismissed publishes a proposal dismissed event to all connected clients.
// This notifies the UI that a discovery proposal has been dismissed.
func (wm *Manager) PublishProposalDismissed(proposalID string) {
	msg := types.WebSocketMessage{
		Type: types.WSMsgTypeProposalDismissed,
		Data: map[string]interface{}{
			"proposalId": proposalID,
			"timestamp":  time.Now().Unix(),
		},
		Timestamp: time.Now(),
	}
	wm.Broadcast(msg)
}

// PublishRepairRequired notifies all connected devices that they need to re-pair
// due to a TLS upgrade. This is sent when TLS is enabled after devices were paired
// without certificate pinning.
func (wm *Manager) PublishRepairRequired(reason, newBaseURL string) {
	msg := types.WebSocketMessage{
		Type: types.WSMsgTypeRepairRequired,
		Data: map[string]interface{}{
			"reason":     reason,
			"newBaseUrl": newBaseURL,
			"message":    "Server TLS configuration has changed. Please re-pair your device to establish a secure connection with certificate pinning.",
			"timestamp":  time.Now().Unix(),
		},
		Timestamp: time.Now(),
	}
	wm.Broadcast(msg)
	log.Info("published repair_required notification to all devices",
		"reason", reason,
		"client_count", wm.ActiveConnections())
}

// DisconnectDevice forcefully disconnects all WebSocket connections for a specific device
func (wm *Manager) DisconnectDevice(deviceID string) int {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	disconnectedCount := 0

	// Find all clients with matching deviceID
	for client := range wm.clients {
		if client.deviceID == deviceID {
			// Close the WebSocket connection
			if client.conn != nil {
				client.mu.Lock()
				client.conn.Close()
				client.mu.Unlock()
			}

			// Close send channel safely to prevent double-close
			client.closeSendChanSafe()

			// Remove from clients map
			delete(wm.clients, client)
			disconnectedCount++

			// Record metrics
			if wm.metrics != nil {
				wm.metrics.WebSocketConnectionsActive.Dec()
			}

			log.Info("disconnected websocket client for revoked device",
				"device_id", deviceID)
		}
	}

	if disconnectedCount > 0 {
		log.Info("disconnected websocket connections for device",
			"count", disconnectedCount,
			"device_id", deviceID)
	}

	return disconnectedCount
}

// SendToDevice sends a message to a specific device via WebSocket
func (wm *Manager) SendToDevice(deviceID string, message interface{}) error {
	wm.mu.RLock()
	defer wm.mu.RUnlock()

	// Find client(s) for this device
	var sent bool
	for client := range wm.clients {
		if client.deviceID == deviceID {
			// Create WebSocket message
			var msg types.WebSocketMessage
			switch v := message.(type) {
			case types.WebSocketMessage:
				msg = v
			case map[string]interface{}:
				msg = types.WebSocketMessage{
					Type: "notification",
					Data: v,
				}
			default:
				msg = types.WebSocketMessage{
					Type: "notification",
					Data: message,
				}
			}

			// Try to send message
			select {
			case client.sendChan <- msg:
				sent = true
			default:
				// Send channel is full, message will be dropped
				log.Warn("send channel full for device, message dropped",
					"device_id", deviceID)
			}
		}
	}

	if !sent {
		return fmt.Errorf("device %s is not connected via WebSocket", deviceID)
	}

	return nil
}

// IsDeviceOnline checks if a device has an active WebSocket connection
func (wm *Manager) IsDeviceOnline(deviceID string) bool {
	return wm.HasDeviceConnection(deviceID)
}

// GetAuthTimeout returns the auth timeout duration for testing
func (wm *Manager) GetAuthTimeout() time.Duration {
	return wm.authTimeout
}

// GetPingInterval returns the ping interval duration for testing
func (wm *Manager) GetPingInterval() time.Duration {
	return wm.pingInterval
}

// GetPongWait returns the pong wait duration for testing
func (wm *Manager) GetPongWait() time.Duration {
	return wm.pongWait
}

// SetAuthTimeout sets the auth timeout duration for testing
func (wm *Manager) SetAuthTimeout(d time.Duration) {
	wm.authTimeout = d
}

// SetPingInterval sets the ping interval duration for testing
func (wm *Manager) SetPingInterval(d time.Duration) {
	wm.pingInterval = d
}

// SetPongWait sets the pong wait duration for testing
func (wm *Manager) SetPongWait(d time.Duration) {
	wm.pongWait = d
}

// Publish implements the EventBus interface for federation events
// This allows federation.PeerManager to publish WebSocket events
func (wm *Manager) Publish(eventType string, data interface{}) {
	msg := types.WebSocketMessage{
		Type:      eventType,
		Data:      data,
		Timestamp: time.Now(),
	}
	wm.Broadcast(msg)
}

// PublishToTopic publishes a message to clients subscribed to a topic.
// The topic is derived from msg.Topic if set, otherwise from msg.Type.
func (wm *Manager) PublishToTopic(topic string, msg types.WebSocketMessage) {
	if topic == "" {
		topic = msg.Type
	}
	msg.Topic = topic

	// Handle retained messages
	if msg.Retain {
		wm.SetRetainedMessage(topic, msg)
	}

	// Check message expiry
	if !msg.ExpiresAt.IsZero() && time.Now().After(msg.ExpiresAt) {
		log.Debug("message expired, not publishing", "topic", topic)
		return
	}

	// Broadcast to subscribed clients
	wm.BroadcastFiltered(msg, func(client *Client) bool {
		return client.IsSubscribedTo(topic)
	})
}

// SubscribeClientToTopics subscribes a device to topics.
// This is called when handling a subscribe message from a client.
func (wm *Manager) SubscribeClientToTopics(deviceID string, patterns []string, opts SubscriptionOptions) error {
	wm.mu.RLock()
	clients := wm.clientsByDevice[deviceID]
	wm.mu.RUnlock()

	if len(clients) == 0 {
		return fmt.Errorf("device %s is not connected", deviceID)
	}

	for _, client := range clients {
		if err := client.SubscribeToTopics(patterns, opts); err != nil {
			return err
		}
	}

	// Send retained messages for matching topics
	wm.sendRetainedMessages(clients, patterns)

	log.Info("device subscribed to topics",
		"device_id", deviceID,
		"topics", patterns,
		"qos", opts.QoS)

	return nil
}

// UnsubscribeClientFromTopics unsubscribes a device from topics.
func (wm *Manager) UnsubscribeClientFromTopics(deviceID string, patterns []string) error {
	wm.mu.RLock()
	clients := wm.clientsByDevice[deviceID]
	wm.mu.RUnlock()

	if len(clients) == 0 {
		return fmt.Errorf("device %s is not connected", deviceID)
	}

	for _, client := range clients {
		client.UnsubscribeFromTopics(patterns)
	}

	log.Info("device unsubscribed from topics",
		"device_id", deviceID,
		"topics", patterns)

	return nil
}

// SetClientLastWill sets the last will message for a device.
func (wm *Manager) SetClientLastWill(deviceID string, lw *LastWill) error {
	wm.mu.RLock()
	clients := wm.clientsByDevice[deviceID]
	wm.mu.RUnlock()

	if len(clients) == 0 {
		return fmt.Errorf("device %s is not connected", deviceID)
	}

	for _, client := range clients {
		client.SetLastWill(lw)
	}

	log.Info("device set last will",
		"device_id", deviceID,
		"topic", lw.Topic,
		"qos", lw.QoS)

	return nil
}

// ClearClientLastWill removes the last will message for a device.
func (wm *Manager) ClearClientLastWill(deviceID string) error {
	wm.mu.RLock()
	clients := wm.clientsByDevice[deviceID]
	wm.mu.RUnlock()

	if len(clients) == 0 {
		return fmt.Errorf("device %s is not connected", deviceID)
	}

	for _, client := range clients {
		client.ClearLastWill()
	}

	log.Info("device cleared last will", "device_id", deviceID)

	return nil
}

// GetClientSubscriptions returns the subscriptions for a device.
func (wm *Manager) GetClientSubscriptions(deviceID string) (map[string]SubscriptionOptions, error) {
	wm.mu.RLock()
	clients := wm.clientsByDevice[deviceID]
	wm.mu.RUnlock()

	if len(clients) == 0 {
		return nil, fmt.Errorf("device %s is not connected", deviceID)
	}

	// Return first client's subscriptions (they should all be the same)
	return clients[0].GetSubscriptions(), nil
}

// SetRetainedMessage stores a retained message for a topic.
func (wm *Manager) SetRetainedMessage(topic string, msg types.WebSocketMessage) {
	wm.retainedMu.Lock()
	defer wm.retainedMu.Unlock()

	now := time.Now()
	wm.retainedMessages[topic] = &RetainedMessage{
		Topic:     topic,
		Message:   msg,
		ExpiresAt: msg.ExpiresAt,
		CreatedAt: now,
		UpdatedAt: now,
	}

	log.Debug("retained message stored", "topic", topic)
}

// GetRetainedMessage retrieves a retained message for a topic.
func (wm *Manager) GetRetainedMessage(topic string) *RetainedMessage {
	wm.retainedMu.RLock()
	defer wm.retainedMu.RUnlock()

	rm := wm.retainedMessages[topic]
	if rm != nil && rm.IsExpired() {
		return nil
	}
	return rm
}

// ClearRetainedMessage removes a retained message for a topic.
func (wm *Manager) ClearRetainedMessage(topic string) {
	wm.retainedMu.Lock()
	defer wm.retainedMu.Unlock()
	delete(wm.retainedMessages, topic)
}

// CleanExpiredRetainedMessages removes expired retained messages.
func (wm *Manager) CleanExpiredRetainedMessages() int {
	wm.retainedMu.Lock()
	defer wm.retainedMu.Unlock()

	count := 0
	for topic, rm := range wm.retainedMessages {
		if rm.IsExpired() {
			delete(wm.retainedMessages, topic)
			count++
		}
	}

	if count > 0 {
		log.Debug("cleaned expired retained messages", "count", count)
	}

	return count
}

// sendRetainedMessages sends matching retained messages to clients.
func (wm *Manager) sendRetainedMessages(clients []*Client, patterns []string) {
	wm.retainedMu.RLock()
	defer wm.retainedMu.RUnlock()

	for topic, rm := range wm.retainedMessages {
		if rm.IsExpired() {
			continue
		}

		// Check if any pattern matches this topic
		if !MatchesAnyTopic(patterns, topic) {
			continue
		}

		// Send to all clients
		for _, client := range clients {
			select {
			case client.sendChan <- rm.Message:
				log.Debug("sent retained message",
					"topic", topic,
					"device_id", client.deviceID)
			default:
				log.Warn("failed to send retained message (channel full)",
					"topic", topic,
					"device_id", client.deviceID)
			}
		}
	}
}

// GetRetainedMessageCount returns the number of retained messages.
func (wm *Manager) GetRetainedMessageCount() int {
	wm.retainedMu.RLock()
	defer wm.retainedMu.RUnlock()
	return len(wm.retainedMessages)
}
