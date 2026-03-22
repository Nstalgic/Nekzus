package main

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/nstalgic/nekzus/internal/handlers"
	"github.com/nstalgic/nekzus/internal/httputil"
	"github.com/nstalgic/nekzus/internal/types"
	wsmanager "github.com/nstalgic/nekzus/internal/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// Allow all origins for now - auth is handled via JWT
		return true
	},
}

// handleWebSocket upgrades HTTP connection to WebSocket and manages client lifecycle
// with post-connection authentication flow
func (app *Application) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	log.Info("websocket connection attempt", "remote_addr", r.RemoteAddr)

	// Upgrade HTTP connection to WebSocket (no auth required yet)
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Error("failed to upgrade connection", "error", err)
		return
	}
	defer conn.Close()

	// Set timeout for receiving auth message (use configured timeout from manager)
	authTimeout := app.managers.WebSocket.GetAuthTimeout()
	conn.SetReadDeadline(time.Now().Add(authTimeout))

	// Wait for auth message from client
	var authMsg types.WebSocketMessage
	err = conn.ReadJSON(&authMsg)
	if err != nil {
		log.Warn("failed to read auth message", "error", err)
		// Send error and close
		conn.WriteJSON(types.WebSocketMessage{
			Type: types.WSMsgTypeAuthFailed,
			Data: map[string]string{
				"error": "timeout waiting for auth message",
			},
		})
		return
	}

	// Verify this is an auth message
	if authMsg.Type != types.WSMsgTypeAuth {
		log.Warn("expected auth message", "got", authMsg.Type)
		conn.WriteJSON(types.WebSocketMessage{
			Type: types.WSMsgTypeAuthFailed,
			Data: map[string]string{
				"error": "expected auth message",
			},
		})
		return
	}

	// Extract token from auth message
	var token string
	var deviceID string

	if dataMap, ok := authMsg.Data.(map[string]interface{}); ok {
		if tokenStr, ok := dataMap["token"].(string); ok {
			token = tokenStr
		}
	}

	// Check if request is from local network (localhost/private IP)
	isLocal := httputil.IsLocalRequest(r)

	// Allow empty token for local requests (IP-based auth)
	if token == "" {
		if !isLocal {
			log.Warn("no token provided from external ip", "remote_addr", r.RemoteAddr)
			conn.WriteJSON(types.WebSocketMessage{
				Type: types.WSMsgTypeAuthFailed,
				Data: map[string]string{
					"error": "token required for external connections",
				},
			})
			return
		}
		// Local request without token - allow as anonymous admin
		log.Info("local request without token, using ip-based auth", "remote_addr", r.RemoteAddr)
		deviceID = "admin"
	} else {
		// Validate JWT token
		_, claims, err := app.services.Auth.ParseJWT(token)
		if err != nil {
			log.Warn("invalid token", "error", err)
			conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			conn.WriteJSON(types.WebSocketMessage{
				Type: types.WSMsgTypeAuthFailed,
				Data: map[string]string{
					"error": "invalid token",
				},
			})
			return
		}

		// Extract device ID from claims
		if sub, ok := claims["sub"].(string); ok && sub != "" {
			deviceID = sub
		} else {
			deviceID = "anonymous"
		}
	}

	// Verify device exists in storage (check if device was revoked)
	// Skip check for admin (IP-based auth) and anonymous users
	if app.storage != nil && deviceID != "anonymous" && deviceID != "admin" {
		device, err := app.storage.GetDevice(deviceID)
		if err != nil {
			log.Error("error checking device in storage", "device_id", deviceID, "error", err)
			conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			conn.WriteJSON(types.WebSocketMessage{
				Type: types.WSMsgTypeAuthFailed,
				Data: map[string]string{
					"error": "authentication error",
				},
			})
			return
		}
		if device == nil {
			log.Warn("device not found in storage, may have been revoked", "device_id", deviceID)
			conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			conn.WriteJSON(types.WebSocketMessage{
				Type: types.WSMsgTypeAuthFailed,
				Data: map[string]string{
					"error": "device access revoked",
				},
			})
			return
		}
	}

	// Send auth success response
	authSuccessMsg := types.WebSocketMessage{
		Type: types.WSMsgTypeAuthSuccess,
		Data: map[string]interface{}{
			"deviceId": deviceID,
			"message":  "authenticated",
		},
		Timestamp: time.Now(),
	}

	conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	if err := conn.WriteJSON(authSuccessMsg); err != nil {
		log.Error("failed to send auth success message", "error", err)
		return
	}

	log.Info("client authenticated", "device_id", deviceID, "remote_addr", r.RemoteAddr)

	// Create client with buffered send channel
	client := wsmanager.NewClient(
		deviceID,
		conn,
		make(chan types.WebSocketMessage, 256),
	)

	// Subscribe client to manager (only after successful auth)
	app.managers.WebSocket.Subscribe(client)
	// Note: Unsubscribe is handled explicitly at the end based on disconnect type

	// Send hello message
	helloMsg := types.WebSocketMessage{
		Type: types.WSMsgTypeHello,
		Data: map[string]interface{}{
			"message":   "connected",
			"nekzusId":  app.nekzusID,
			"version":   app.version,
			"timestamp": time.Now().Unix(),
		},
		Timestamp: time.Now(),
	}

	if err := conn.WriteJSON(helloMsg); err != nil {
		log.Error("failed to send hello message", "error", err)
		return
	}

	// Set up ping/pong for keepalive (use configured intervals from manager)
	pingInterval := app.managers.WebSocket.GetPingInterval()
	pongWait := app.managers.WebSocket.GetPongWait()

	conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	// Channel to signal goroutine completion
	done := make(chan struct{})

	// Goroutine to write messages to client
	go func() {
		ticker := time.NewTicker(pingInterval)
		defer ticker.Stop()

		for {
			select {
			case msg, ok := <-client.GetSendChan():
				if !ok {
					// Channel closed, send close message
					conn.WriteMessage(websocket.CloseMessage, []byte{})
					return
				}

				// Set write deadline
				conn.SetWriteDeadline(time.Now().Add(10 * time.Second))

				// Send message
				if err := conn.WriteJSON(msg); err != nil {
					log.Error("failed to write message to client", "device_id", deviceID, "error", err)
					return
				}
				log.Info("wrote message to client", "device_id", deviceID, "type", msg.Type)

			case <-ticker.C:
				// Send ping
				conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
				if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
					log.Error("failed to send ping to client", "device_id", deviceID, "error", err)
					return
				}

			case <-done:
				return
			}
		}
	}()

	// Track if disconnect was clean
	unexpectedDisconnect := true

	// Read from client (mainly to detect disconnection and respond to control messages)
	for {
		messageType, message, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Error("unexpected close error from client", "device_id", deviceID, "error", err)
			} else {
				log.Info("client disconnected", "device_id", deviceID)
				unexpectedDisconnect = false
			}
			break
		}

		// Handle text messages (client-to-server commands)
		if messageType == websocket.TextMessage {
			var msg types.WebSocketMessage
			if err := json.Unmarshal(message, &msg); err != nil {
				log.Debug("failed to parse message from client", "device_id", deviceID, "error", err)
				continue
			}

			// Route message to appropriate handler
			app.handleWebSocketMessage(deviceID, msg)
		}
	}

	// Signal write goroutine to stop
	close(done)

	// Use appropriate unsubscribe based on disconnect type
	if unexpectedDisconnect {
		app.managers.WebSocket.UnsubscribeUnexpected(client)
	} else {
		app.managers.WebSocket.Unsubscribe(client)
	}

	// Clean up any active log streams for this device
	if app.handlers != nil && app.handlers.ContainerLogs != nil {
		app.handlers.ContainerLogs.GetStreamManager().StopAllForDevice(deviceID)
	}

	log.Info("client connection closed", "device_id", deviceID)
}

// handleWebSocketMessage routes incoming WebSocket messages to appropriate handlers
func (app *Application) handleWebSocketMessage(deviceID string, msg types.WebSocketMessage) {
	switch msg.Type {
	case types.WSMsgTypeSubscribe:
		app.handleTopicSubscribe(deviceID, msg)

	case types.WSMsgTypeUnsubscribe:
		app.handleTopicUnsubscribe(deviceID, msg)

	case types.WSMsgTypeSetLastWill:
		app.handleSetLastWill(deviceID, msg)

	case types.WSMsgTypeAck:
		app.handleQoSAck(deviceID, msg)

	case types.WSMsgTypeNotificationACK:
		// Handle notification acknowledgment from client
		app.handleNotificationACK(deviceID, msg)

	case types.WSMsgTypeContainerLogsSubscribe, types.WSMsgTypeContainerLogsStart:
		// Support both new (subscribe) and legacy (start) message types
		if app.handlers == nil || app.handlers.ContainerLogs == nil {
			log.Warn("container logs handler not available", "device_id", deviceID)
			return
		}

		// Parse the log start request from message data
		req, err := parseLogStartRequest(msg.Data)
		if err != nil {
			log.Warn("invalid log start request", "device_id", deviceID, "error", err)
			return
		}

		app.handlers.ContainerLogs.HandleStartStream(deviceID, req)

	case types.WSMsgTypeContainerLogsUnsubscribe, types.WSMsgTypeContainerLogsStop:
		// Support both new (unsubscribe) and legacy (stop) message types
		if app.handlers == nil || app.handlers.ContainerLogs == nil {
			return
		}

		// Parse the log stop request from message data
		req, err := parseLogStopRequest(msg.Data)
		if err != nil {
			log.Warn("invalid log stop request", "device_id", deviceID, "error", err)
			return
		}

		app.handlers.ContainerLogs.HandleStopStream(deviceID, req)

	default:
		log.Debug("unhandled message type", "device_id", deviceID, "type", msg.Type)
	}
}

// parseLogStartRequest extracts LogStartRequest from message data
func parseLogStartRequest(data interface{}) (handlers.LogStartRequest, error) {
	var req handlers.LogStartRequest

	// Convert data to JSON and back to struct
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return req, err
	}

	if err := json.Unmarshal(jsonBytes, &req); err != nil {
		return req, err
	}

	return req, nil
}

// parseLogStopRequest extracts LogStopRequest from message data
func parseLogStopRequest(data interface{}) (handlers.LogStopRequest, error) {
	var req handlers.LogStopRequest

	// Convert data to JSON and back to struct
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return req, err
	}

	if err := json.Unmarshal(jsonBytes, &req); err != nil {
		return req, err
	}

	return req, nil
}

// handleNotificationACK processes acknowledgment messages from clients
func (app *Application) handleNotificationACK(deviceID string, msg types.WebSocketMessage) {
	// Extract notification ID from message
	notifID := msg.NotificationID
	if notifID == "" {
		// Try to get it from the data field for backwards compatibility
		if dataMap, ok := msg.Data.(map[string]interface{}); ok {
			if id, ok := dataMap["notificationId"].(string); ok {
				notifID = id
			}
		}
	}

	if notifID == "" {
		log.Warn("notification ACK missing notificationId", "device_id", deviceID)
		return
	}

	// Get ACK tracker from the notification deliverer
	if app.wsDeliverer != nil && app.wsDeliverer.GetACKTracker() != nil {
		app.wsDeliverer.GetACKTracker().ACK(notifID)
		log.Debug("notification acknowledged",
			"notification_id", notifID,
			"device_id", deviceID)
	}
}

// handleTopicSubscribe handles topic subscription requests from clients
func (app *Application) handleTopicSubscribe(deviceID string, msg types.WebSocketMessage) {
	var req wsmanager.SubscribeRequest
	if err := parseMessageData(msg.Data, &req); err != nil {
		log.Warn("invalid subscribe request", "device_id", deviceID, "error", err)
		app.sendSubAck(deviceID, nil, false, "invalid request format")
		return
	}

	if len(req.Topics) == 0 {
		app.sendSubAck(deviceID, nil, false, "at least one topic is required")
		return
	}

	opts := wsmanager.SubscriptionOptions{QoS: req.QoS}
	if err := app.managers.WebSocket.SubscribeClientToTopics(deviceID, req.Topics, opts); err != nil {
		log.Warn("failed to subscribe to topics", "device_id", deviceID, "error", err)
		app.sendSubAck(deviceID, req.Topics, false, err.Error())
		return
	}

	app.sendSubAck(deviceID, req.Topics, true, "")
	log.Debug("client subscribed to topics", "device_id", deviceID, "topics", req.Topics, "qos", req.QoS)
}

// handleTopicUnsubscribe handles topic unsubscription requests from clients
func (app *Application) handleTopicUnsubscribe(deviceID string, msg types.WebSocketMessage) {
	var req wsmanager.UnsubscribeRequest
	if err := parseMessageData(msg.Data, &req); err != nil {
		log.Warn("invalid unsubscribe request", "device_id", deviceID, "error", err)
		return
	}

	if len(req.Topics) == 0 {
		return
	}

	if err := app.managers.WebSocket.UnsubscribeClientFromTopics(deviceID, req.Topics); err != nil {
		log.Warn("failed to unsubscribe from topics", "device_id", deviceID, "error", err)
		app.sendUnsubAck(deviceID, req.Topics, false)
		return
	}

	app.sendUnsubAck(deviceID, req.Topics, true)
	log.Debug("client unsubscribed from topics", "device_id", deviceID, "topics", req.Topics)
}

// handleSetLastWill handles last will message setup from clients
func (app *Application) handleSetLastWill(deviceID string, msg types.WebSocketMessage) {
	var req wsmanager.SetLastWillRequest
	if err := parseMessageData(msg.Data, &req); err != nil {
		log.Warn("invalid set_last_will request", "device_id", deviceID, "error", err)
		app.sendLWTAck(deviceID, false, false, "invalid request format")
		return
	}

	// Empty topic clears the last will
	if req.Topic == "" {
		if err := app.managers.WebSocket.ClearClientLastWill(deviceID); err != nil {
			log.Warn("failed to clear last will", "device_id", deviceID, "error", err)
			app.sendLWTAck(deviceID, false, false, err.Error())
			return
		}
		app.sendLWTAck(deviceID, true, true, "")
		log.Debug("client cleared last will", "device_id", deviceID)
		return
	}

	lw := &wsmanager.LastWill{
		Topic: req.Topic,
		Message: types.WebSocketMessage{
			Type: req.Topic, // Use topic as message type
			Data: req.Message,
		},
		QoS: req.QoS,
	}

	if err := app.managers.WebSocket.SetClientLastWill(deviceID, lw); err != nil {
		log.Warn("failed to set last will", "device_id", deviceID, "error", err)
		app.sendLWTAck(deviceID, false, false, err.Error())
		return
	}

	app.sendLWTAck(deviceID, true, false, "")
	log.Debug("client set last will", "device_id", deviceID, "topic", req.Topic, "qos", req.QoS)
}

// handleQoSAck handles QoS acknowledgment messages from clients
func (app *Application) handleQoSAck(deviceID string, msg types.WebSocketMessage) {
	messageID := msg.MessageID
	if messageID == "" {
		// Try to get from data
		if dataMap, ok := msg.Data.(map[string]interface{}); ok {
			if id, ok := dataMap["messageId"].(string); ok {
				messageID = id
			}
		}
	}

	if messageID == "" {
		log.Warn("QoS ACK missing messageId", "device_id", deviceID)
		return
	}

	// TODO: Implement QoS acknowledgment tracking
	log.Debug("QoS ACK received", "device_id", deviceID, "message_id", messageID)
}

// sendSubAck sends a subscription acknowledgment to a client
func (app *Application) sendSubAck(deviceID string, topics []string, success bool, errMsg string) {
	resp := wsmanager.SubAckResponse{
		Topics:  topics,
		Success: success,
		Error:   errMsg,
	}

	msg := types.WebSocketMessage{
		Type: types.WSMsgTypeSubAck,
		Data: resp,
	}

	if err := app.managers.WebSocket.SendToDevice(deviceID, msg); err != nil {
		log.Warn("failed to send suback", "device_id", deviceID, "error", err)
	}
}

// sendUnsubAck sends an unsubscription acknowledgment to a client
func (app *Application) sendUnsubAck(deviceID string, topics []string, success bool) {
	resp := wsmanager.UnsubAckResponse{
		Topics:  topics,
		Success: success,
	}

	msg := types.WebSocketMessage{
		Type: types.WSMsgTypeUnsubAck,
		Data: resp,
	}

	if err := app.managers.WebSocket.SendToDevice(deviceID, msg); err != nil {
		log.Warn("failed to send unsuback", "device_id", deviceID, "error", err)
	}
}

// sendLWTAck sends a last will acknowledgment to a client
func (app *Application) sendLWTAck(deviceID string, success, cleared bool, errMsg string) {
	resp := wsmanager.LWTAckResponse{
		Success: success,
		Cleared: cleared,
		Error:   errMsg,
	}

	msg := types.WebSocketMessage{
		Type: types.WSMsgTypeLWTAck,
		Data: resp,
	}

	if err := app.managers.WebSocket.SendToDevice(deviceID, msg); err != nil {
		log.Warn("failed to send lwtack", "device_id", deviceID, "error", err)
	}
}

// parseMessageData parses message data into a struct
func parseMessageData(data interface{}, dest interface{}) error {
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return json.Unmarshal(jsonBytes, dest)
}
