package scripts

import (
	"encoding/json"
	"log/slog"
	"strings"
	"time"

	"github.com/nstalgic/nekzus/internal/types"
)

var notifierLog = slog.With("package", "scripts", "component", "notifier")

// WebSocketSender interface for sending WebSocket messages.
type WebSocketSender interface {
	SendToDevice(deviceID string, msgType string, payload json.RawMessage) error
	Broadcast(msgType string, payload json.RawMessage)
}

// NotificationQueue interface for queueing notifications for offline devices.
type NotificationQueue interface {
	Enqueue(deviceID string, msgType string, payload json.RawMessage, ttl time.Duration, maxRetries int) error
}

// NotifierConfig holds configuration for the WebSocketNotifier.
type NotifierConfig struct {
	TTL              time.Duration // TTL for queued notifications
	MaxRetries       int           // Max retries for queued notifications
	MaxOutputInNotif int           // Max output bytes to include in notification (0 = no limit)
}

// WebSocketNotifier sends execution notifications via WebSocket with queue fallback.
type WebSocketNotifier struct {
	ws     WebSocketSender
	queue  NotificationQueue
	config NotifierConfig
}

// NewWebSocketNotifier creates a new WebSocket notifier.
func NewWebSocketNotifier(ws WebSocketSender, queue NotificationQueue, config NotifierConfig) *WebSocketNotifier {
	if config.TTL == 0 {
		config.TTL = 24 * time.Hour
	}
	if config.MaxRetries == 0 {
		config.MaxRetries = 3
	}
	if config.MaxOutputInNotif == 0 {
		config.MaxOutputInNotif = 10 * 1024 // 10KB default
	}

	return &WebSocketNotifier{
		ws:     ws,
		queue:  queue,
		config: config,
	}
}

// NotifyExecutionStarted sends a notification when execution begins.
func (n *WebSocketNotifier) NotifyExecutionStarted(deviceID, executionID, scriptID, scriptName string) {
	if deviceID == "" || deviceID == "scheduler" {
		return
	}

	payload := map[string]interface{}{
		"executionId": executionID,
		"scriptId":    scriptID,
		"scriptName":  scriptName,
		"status":      "running",
		"timestamp":   time.Now().Unix(),
	}

	n.send(deviceID, types.WSMsgTypeExecutionStarted, payload)
}

// NotifyExecutionCompleted sends a notification when execution completes successfully.
func (n *WebSocketNotifier) NotifyExecutionCompleted(deviceID string, execution *Execution) {
	if deviceID == "" || deviceID == "scheduler" {
		return
	}

	output := execution.Output
	if n.config.MaxOutputInNotif > 0 && len(output) > n.config.MaxOutputInNotif {
		output = output[:n.config.MaxOutputInNotif] + "\n... [output truncated]"
	}

	var duration string
	if execution.StartedAt != nil && execution.CompletedAt != nil {
		duration = execution.CompletedAt.Sub(*execution.StartedAt).String()
	}

	payload := map[string]interface{}{
		"executionId": execution.ID,
		"scriptId":    execution.ScriptID,
		"status":      string(execution.Status),
		"output":      output,
		"duration":    duration,
		"timestamp":   time.Now().Unix(),
	}

	if execution.ExitCode != nil {
		payload["exitCode"] = *execution.ExitCode
	}

	n.send(deviceID, types.WSMsgTypeExecutionCompleted, payload)
}

// NotifyExecutionFailed sends a notification when execution fails.
func (n *WebSocketNotifier) NotifyExecutionFailed(deviceID string, execution *Execution, errorMsg string) {
	if deviceID == "" || deviceID == "scheduler" {
		return
	}

	output := execution.Output
	if n.config.MaxOutputInNotif > 0 && len(output) > n.config.MaxOutputInNotif {
		output = output[:n.config.MaxOutputInNotif] + "\n... [output truncated]"
	}

	var duration string
	if execution.StartedAt != nil && execution.CompletedAt != nil {
		duration = execution.CompletedAt.Sub(*execution.StartedAt).String()
	}

	payload := map[string]interface{}{
		"executionId":  execution.ID,
		"scriptId":     execution.ScriptID,
		"status":       string(execution.Status),
		"output":       output,
		"errorMessage": errorMsg,
		"duration":     duration,
		"timestamp":    time.Now().Unix(),
	}

	if execution.ExitCode != nil {
		payload["exitCode"] = *execution.ExitCode
	}

	n.send(deviceID, types.WSMsgTypeExecutionFailed, payload)
}

// send attempts to send via WebSocket, falling back to queue if offline.
func (n *WebSocketNotifier) send(deviceID, msgType string, payload map[string]interface{}) {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		notifierLog.Error("failed to marshal notification payload",
			"device_id", deviceID,
			"msg_type", msgType,
			"error", err)
		return
	}

	// For web-triggered executions, broadcast to all connected clients
	if strings.HasPrefix(deviceID, "web:") {
		if n.ws != nil {
			n.ws.Broadcast(msgType, payloadBytes)
			notifierLog.Debug("notification broadcast to all clients",
				"triggered_by", deviceID,
				"msg_type", msgType)
		}
		return
	}

	// Try WebSocket first for device-specific notifications
	if n.ws != nil {
		err = n.ws.SendToDevice(deviceID, msgType, payloadBytes)
		if err == nil {
			notifierLog.Debug("notification sent via WebSocket",
				"device_id", deviceID,
				"msg_type", msgType)
			return
		}

		notifierLog.Debug("WebSocket send failed, will queue",
			"device_id", deviceID,
			"msg_type", msgType,
			"error", err)
	}

	// Fall back to queue for offline devices
	if n.queue != nil {
		if err := n.queue.Enqueue(deviceID, msgType, payloadBytes, n.config.TTL, n.config.MaxRetries); err != nil {
			notifierLog.Error("failed to queue notification",
				"device_id", deviceID,
				"msg_type", msgType,
				"error", err)
		} else {
			notifierLog.Debug("notification queued for later delivery",
				"device_id", deviceID,
				"msg_type", msgType)
		}
	}
}
