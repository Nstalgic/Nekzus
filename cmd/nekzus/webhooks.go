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

	// If deviceIds specified, use targeted delivery with queueing for offline devices
	if len(payload.DeviceIDs) > 0 {
		// Marshal event for queue storage
		eventJSON, err := json.Marshal(event)
		if err != nil {
			log.Warn("failed to marshal activity event for queue", "error", err)
		}

		// Track which devices are online
		onlineDevices := make(map[string]bool)
		app.managers.WebSocket.BroadcastFiltered(wsMessage, func(client *wsmanager.Client) bool {
			deviceID := client.GetDeviceID()
			for _, targetID := range payload.DeviceIDs {
				if deviceID == targetID {
					onlineDevices[deviceID] = true
					return true
				}
			}
			return false
		})

		// Queue notifications for offline devices (only if storage is available)
		if app.storage != nil && eventJSON != nil {
			for _, deviceID := range payload.DeviceIDs {
				if !onlineDevices[deviceID] {
					// Device is offline - queue the notification
					_, queueErr := app.storage.EnqueueNotification(
						deviceID,
						"webhook.activity",
						eventJSON,
						30*24*time.Hour, // 30 day TTL
						5,               // max retries
					)
					if queueErr != nil {
						log.Warn("failed to queue activity notification for offline device",
							"device_id", deviceID, "error", queueErr)
					} else {
						log.Info("queued activity notification for offline device", "device_id", deviceID)
					}
				}
			}
		}
	} else {
		// Broadcast to all connected clients
		app.managers.WebSocket.Broadcast(wsMessage)
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

	// If deviceIds specified, use targeted delivery with queueing for offline devices
	if len(payload.DeviceIDs) > 0 {
		// Marshal payload for queue storage
		payloadJSON, err := json.Marshal(payload)
		if err != nil {
			log.Warn("failed to marshal webhook payload for queue", "error", err)
		}

		// Track which devices are online
		onlineDevices := make(map[string]bool)
		app.managers.WebSocket.BroadcastFiltered(wsMessage, func(client *wsmanager.Client) bool {
			deviceID := client.GetDeviceID()
			for _, targetID := range payload.DeviceIDs {
				if deviceID == targetID {
					onlineDevices[deviceID] = true
					return true
				}
			}
			return false
		})

		// Queue notifications for offline devices (only if storage is available)
		if app.storage != nil && payloadJSON != nil {
			for _, deviceID := range payload.DeviceIDs {
				if !onlineDevices[deviceID] {
					// Device is offline - queue the notification
					_, queueErr := app.storage.EnqueueNotification(
						deviceID,
						"webhook.notify",
						payloadJSON,
						30*24*time.Hour, // 30 day TTL
						5,               // max retries
					)
					if queueErr != nil {
						log.Warn("failed to queue notification for offline device",
							"device_id", deviceID, "error", queueErr)
					} else {
						log.Info("queued notification for offline device", "device_id", deviceID)
					}
				}
			}
		}
	} else {
		// Broadcast to all connected clients
		app.managers.WebSocket.Broadcast(wsMessage)
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
