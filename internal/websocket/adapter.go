package websocket

import (
	"encoding/json"
	"time"

	"github.com/nstalgic/nekzus/internal/notifications"
	"github.com/nstalgic/nekzus/internal/types"
)

// ManagerAdapter adapts Manager to notifications.WebSocketBroadcaster interface
type ManagerAdapter struct {
	manager *Manager
}

// NewManagerAdapter creates a new adapter
func NewManagerAdapter(manager *Manager) *ManagerAdapter {
	return &ManagerAdapter{
		manager: manager,
	}
}

// BroadcastFiltered sends a message to clients matching the filter
func (a *ManagerAdapter) BroadcastFiltered(msg interface{}, filter func(client notifications.WebSocketClient) bool) {
	// Convert message to WebSocketMessage
	var wsMsg types.WebSocketMessage

	// Handle different message types
	switch m := msg.(type) {
	case types.WebSocketMessage:
		wsMsg = m
	case map[string]interface{}:
		// Extract type and payload from map
		if msgType, ok := m["type"].(string); ok {
			wsMsg.Type = msgType
		}
		wsMsg.Data = m
	default:
		wsMsg.Type = "notification"
		wsMsg.Data = msg
	}

	// Adapt filter function
	var clientFilter func(*Client) bool
	if filter != nil {
		clientFilter = func(client *Client) bool {
			return filter(client)
		}
	}

	// Delegate to Manager
	a.manager.BroadcastFiltered(wsMsg, clientFilter)
}

// HasDeviceConnection checks if a device has an active WebSocket connection
func (a *ManagerAdapter) HasDeviceConnection(deviceID string) bool {
	return a.manager.HasDeviceConnection(deviceID)
}

// IsDeviceConnected implements notifications.ConnectivityChecker interface
func (a *ManagerAdapter) IsDeviceConnected(deviceID string) bool {
	return a.manager.HasDeviceConnection(deviceID)
}

// SendToDevice sends a notification to a specific device via WebSocket
// Returns an error if the device is offline or the send channel is full
func (a *ManagerAdapter) SendToDevice(deviceID string, msgType string, payload json.RawMessage) error {
	msg := types.WebSocketMessage{
		Type:      msgType,
		Data:      payload,
		Timestamp: time.Now(),
	}
	return a.manager.SendToDevice(deviceID, msg)
}

// Broadcast sends a message to all connected clients
func (a *ManagerAdapter) Broadcast(msgType string, payload json.RawMessage) {
	// Parse the payload to send it as structured data
	var data interface{}
	if err := json.Unmarshal(payload, &data); err != nil {
		data = payload
	}

	msg := types.WebSocketMessage{
		Type:      msgType,
		Data:      data,
		Timestamp: time.Now(),
	}

	a.manager.Broadcast(msg)
	log.Debug("broadcast message sent", "msg_type", msgType)
}

// SendNotification sends a notification with ACK tracking to a specific device
// The notification includes a unique ID that the client must acknowledge
// The message is sent in the same format as direct webhooks (unwrapped) for mobile compatibility
func (a *ManagerAdapter) SendNotification(deviceID string, notifID string, msgType string, payload json.RawMessage) error {
	// Parse the original payload to send it unwrapped
	var originalData interface{}
	if err := json.Unmarshal(payload, &originalData); err != nil {
		// If unmarshal fails, use raw payload
		originalData = payload
	}

	// Map stored msgType to WebSocket message type
	// e.g., "webhook.notify" -> "webhook", "webhook.activity" -> "webhook"
	wsType := msgType
	switch msgType {
	case "webhook.notify", "webhook.activity":
		wsType = types.WSMsgTypeWebhook
	}

	// Send in the same format as direct webhooks, but with notificationId for ACK tracking
	msg := types.WebSocketMessage{
		Type:           wsType,
		NotificationID: notifID,
		Data:           originalData,
		Timestamp:      time.Now(),
	}

	log.Info("SendNotification called",
		"device_id", deviceID,
		"notif_id", notifID,
		"msg_type", msgType,
		"ws_type", wsType,
		"payload_len", len(payload))

	err := a.manager.SendToDevice(deviceID, msg)
	if err != nil {
		log.Error("SendNotification failed", "device_id", deviceID, "notif_id", notifID, "error", err)
	} else {
		log.Info("SendNotification succeeded", "device_id", deviceID, "notif_id", notifID)
	}
	return err
}
