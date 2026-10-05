package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	apperrors "github.com/nstalgic/nekzus/internal/errors"
	"github.com/nstalgic/nekzus/internal/httputil"
	"github.com/nstalgic/nekzus/internal/types"
	wsmanager "github.com/nstalgic/nekzus/internal/websocket"
)

// WebhookActivityPayload represents the payload for creating an activity event via webhook
type WebhookActivityPayload struct {
	Message   string   `json:"message"`             // Required: The activity message
	Icon      string   `json:"icon,omitempty"`      // Optional: Icon name (e.g., "Bell", "AlertTriangle")
	IconClass string   `json:"iconClass,omitempty"` // Optional: Icon class (e.g., "success", "warning", "danger")
	Details   string   `json:"details,omitempty"`   // Optional: Additional details
	DeviceIDs []string `json:"deviceIds,omitempty"` // Optional: Target specific devices (empty = broadcast to all)
}

// WebhookNotifyPayload represents the payload for arbitrary notifications via webhook
type WebhookNotifyPayload struct {
	DeviceIDs []string               `json:"deviceIds,omitempty"` // Optional: Target specific devices
	Type      string                 `json:"type,omitempty"`      // Optional: Custom type
	Data      map[string]interface{} `json:"data,omitempty"`      // Arbitrary JSON data
}

// handleWebhookActivity handles POST requests to create activity events
// Endpoint: POST /api/v1/webhooks/activity
func (app *Application) handleWebhookActivity(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()
	endpoint := "activity"

	if r.Method != http.MethodPost {
		apperrors.WriteJSON(w, apperrors.New("METHOD_NOT_ALLOWED", "Method not allowed", http.StatusMethodNotAllowed))
		app.metrics.WebhookRequestsTotal.WithLabelValues(endpoint, "405").Inc()
		return
	}

	// Parse request body
	var payload WebhookActivityPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		apperrors.WriteJSON(w, apperrors.Wrap(err, "INVALID_REQUEST", "Invalid JSON payload", http.StatusBadRequest))
		app.metrics.WebhookRequestsTotal.WithLabelValues(endpoint, "400").Inc()
		app.metrics.WebhookRequestDuration.WithLabelValues(endpoint).Observe(time.Since(startTime).Seconds())
		return
	}

	// Validate required fields
	if payload.Message == "" {
		apperrors.WriteJSON(w, apperrors.New("INVALID_REQUEST", "Missing required field: message", http.StatusBadRequest))
		app.metrics.WebhookRequestsTotal.WithLabelValues(endpoint, "400").Inc()
		app.metrics.WebhookRequestDuration.WithLabelValues(endpoint).Observe(time.Since(startTime).Seconds())
		return
	}

	// Apply defaults
	if payload.Icon == "" {
		payload.Icon = "Bell" // Default icon
	}

	// Create activity event
	event := types.ActivityEvent{
		ID:        fmt.Sprintf("webhook-%d", time.Now().UnixNano()),
		Type:      "webhook.activity",
		Icon:      payload.Icon,
		IconClass: payload.IconClass,
		Message:   payload.Message,
		Details:   payload.Details,
		Timestamp: time.Now().UnixMilli(),
	}

	// Add to activity tracker (persists to DB if available)
	if app.managers.Activity != nil {
		if err := app.managers.Activity.Add(event); err != nil {
			log.Warn("failed to add webhook activity", "error", err)
			apperrors.WriteJSON(w, apperrors.Wrap(err, "ACTIVITY_ADD_FAILED", "Failed to add activity", http.StatusInternalServerError))
			app.metrics.WebhookRequestsTotal.WithLabelValues(endpoint, "500").Inc()
			app.metrics.WebhookRequestDuration.WithLabelValues(endpoint).Observe(time.Since(startTime).Seconds())
			return
		}
	}

	// Broadcast to WebSocket clients
	wsMessage := types.WebSocketMessage{
		Type:      types.WSMsgTypeWebhook,
		Data:      event,
		Timestamp: time.Now(),
	}

	// If deviceIds specified, use targeted delivery through the notification queue
	if len(payload.DeviceIDs) > 0 {
		app.deliverTargetedWebhook(wsMessage, payload.DeviceIDs, "webhook.activity", event)
	} else {
		// No targets: every paired device plus non-device clients (web UI)
		app.deliverBroadcastWebhook(wsMessage, "webhook.activity", event)
	}

	// Record metrics
	app.metrics.WebhookRequestsTotal.WithLabelValues(endpoint, "200").Inc()
	app.metrics.WebhookRequestDuration.WithLabelValues(endpoint).Observe(time.Since(startTime).Seconds())

	// Return success response
	if err := httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"eventId": event.ID,
	}); err != nil {
		log.Error("failed to encode json response", "error", err)
	}
}

// handleWebhookNotify handles POST requests to send arbitrary notifications
// Endpoint: POST /api/v1/webhooks/notify
func (app *Application) handleWebhookNotify(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()
	endpoint := "notify"

	if r.Method != http.MethodPost {
		apperrors.WriteJSON(w, apperrors.New("METHOD_NOT_ALLOWED", "Method not allowed", http.StatusMethodNotAllowed))
		app.metrics.WebhookRequestsTotal.WithLabelValues(endpoint, "405").Inc()
		return
	}

	// Parse request body as generic JSON
	var payload WebhookNotifyPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		apperrors.WriteJSON(w, apperrors.Wrap(err, "INVALID_REQUEST", "Invalid JSON payload", http.StatusBadRequest))
		app.metrics.WebhookRequestsTotal.WithLabelValues(endpoint, "400").Inc()
		app.metrics.WebhookRequestDuration.WithLabelValues(endpoint).Observe(time.Since(startTime).Seconds())
		return
	}

	// Create WebSocket message with the arbitrary payload
	wsMessage := types.WebSocketMessage{
		Type:      types.WSMsgTypeWebhook,
		Data:      payload,
		Timestamp: time.Now(),
	}

	// If deviceIds specified, use targeted delivery through the notification queue
	if len(payload.DeviceIDs) > 0 {
		app.deliverTargetedWebhook(wsMessage, payload.DeviceIDs, "webhook.notify", payload)
	} else {
		// No targets: every paired device plus non-device clients (web UI)
		app.deliverBroadcastWebhook(wsMessage, "webhook.notify", payload)
	}

	// Record metrics
	app.metrics.WebhookRequestsTotal.WithLabelValues(endpoint, "200").Inc()
	app.metrics.WebhookRequestDuration.WithLabelValues(endpoint).Observe(time.Since(startTime).Seconds())

	// Return success response
	if err := httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"sent":    true,
	}); err != nil {
		log.Error("failed to encode json response", "error", err)
	}
}

// deliverTargetedWebhook delivers a webhook to specific devices.
//
// When the notification queue is available, every target device gets a queued,
// ACK-tracked copy and no live send. The queue delivers immediately to online
// devices and replays on reconnect, so a live send that is silently lost (stale
// socket, suspended app, full send buffer) is still retried. A live copy would
// only produce a duplicate on the device.
//
// Without a queue (notifications disabled), fall back to a live send to online
// targets plus a stored row for offline ones.
func (app *Application) deliverTargetedWebhook(wsMessage types.WebSocketMessage, deviceIDs []string, queueType string, payload interface{}) {
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		log.Warn("failed to marshal webhook payload for queue", "type", queueType, "error", err)
	}

	if app.notificationQueue != nil && payloadJSON != nil {
		for _, deviceID := range deviceIDs {
			// Enqueue returns an error when the buffer is full but the row was
			// still persisted, so this is a warning, not a lost notification.
			if err := app.notificationQueue.Enqueue(deviceID, queueType, payloadJSON, 30*24*time.Hour, 5); err != nil {
				log.Warn("webhook enqueue reported an error", "type", queueType, "device_id", deviceID, "error", err)
			}
		}
		return
	}

	// Track which devices are online
	onlineDevices := make(map[string]bool)
	app.managers.WebSocket.BroadcastFiltered(wsMessage, func(client *wsmanager.Client) bool {
		deviceID := client.GetDeviceID()
		for _, targetID := range deviceIDs {
			if deviceID == targetID {
				onlineDevices[deviceID] = true
				return true
			}
		}
		return false
	})

	// Queue notifications for offline devices (only if storage is available)
	if app.storage != nil && payloadJSON != nil {
		for _, deviceID := range deviceIDs {
			if !onlineDevices[deviceID] {
				_, queueErr := app.storage.EnqueueNotification(
					deviceID,
					queueType,
					payloadJSON,
					30*24*time.Hour, // 30 day TTL
					5,               // max retries
				)
				if queueErr != nil {
					log.Warn("failed to queue webhook for offline device",
						"type", queueType, "device_id", deviceID, "error", queueErr)
				} else {
					log.Info("queued webhook for offline device", "type", queueType, "device_id", deviceID)
				}
			}
		}
	}
}

// deliverBroadcastWebhook delivers an untargeted webhook to everyone.
//
// Paired devices each get a queued, ACK-tracked copy (see deliverTargetedWebhook),
// so offline devices receive it on reconnect and lost live sends are retried.
// Connections that are not paired devices, such as the web UI ("admin" or
// "anonymous"), have no queue rows and keep receiving the live broadcast.
//
// If the queue or device list is unavailable, fall back to a live broadcast to
// all connected clients.
func (app *Application) deliverBroadcastWebhook(wsMessage types.WebSocketMessage, queueType string, payload interface{}) {
	if app.notificationQueue == nil || app.storage == nil {
		app.managers.WebSocket.Broadcast(wsMessage)
		return
	}

	devices, err := app.storage.ListDevices()
	if err != nil {
		log.Warn("failed to list devices for webhook, broadcasting live", "type", queueType, "error", err)
		app.managers.WebSocket.Broadcast(wsMessage)
		return
	}

	paired := make(map[string]bool, len(devices))
	deviceIDs := make([]string, 0, len(devices))
	for _, d := range devices {
		paired[d.ID] = true
		deviceIDs = append(deviceIDs, d.ID)
	}

	// Live copy only for connections that won't get a queued one
	app.managers.WebSocket.BroadcastFiltered(wsMessage, func(client *wsmanager.Client) bool {
		return !paired[client.GetDeviceID()]
	})

	if len(deviceIDs) > 0 {
		app.deliverTargetedWebhook(wsMessage, deviceIDs, queueType, payload)
	}
}
