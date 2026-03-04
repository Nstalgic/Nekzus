package notifications

import (
	"encoding/json"
)

// WebSocketClient represents a minimal interface for WebSocket client filtering
type WebSocketClient interface {
	GetDeviceID() string
}

// WebSocketBroadcaster defines the interface for broadcasting WebSocket messages
type WebSocketBroadcaster interface {
	// HasDeviceConnection checks if a device has an active WebSocket connection
	HasDeviceConnection(deviceID string) bool

	// SendNotification sends a notification with ACK tracking to a specific device
	// Returns the notification ID and an error if the device is offline or the send channel is full
	SendNotification(deviceID string, notifID string, msgType string, payload json.RawMessage) error

	// SendToDevice sends a notification to a specific device (legacy, no ACK tracking)
	// Returns an error if the device is offline or the send channel is full
	SendToDevice(deviceID string, msgType string, payload json.RawMessage) error

	// BroadcastFiltered sends a message to clients matching the filter (deprecated for notifications)
	// The message should be compatible with types.WebSocketMessage
	BroadcastFiltered(msg interface{}, filter func(client WebSocketClient) bool)
}

// WebSocketDeliverer delivers notifications via WebSocket connections with ACK tracking
type WebSocketDeliverer struct {
	broadcaster WebSocketBroadcaster
	ackTracker  *ACKTracker
}

// NewWebSocketDeliverer creates a new WebSocket deliverer
func NewWebSocketDeliverer(broadcaster WebSocketBroadcaster) *WebSocketDeliverer {
	return &WebSocketDeliverer{
		broadcaster: broadcaster,
	}
}

// NewWebSocketDelivererWithACK creates a new WebSocket deliverer with ACK tracking
func NewWebSocketDelivererWithACK(broadcaster WebSocketBroadcaster, ackTracker *ACKTracker) *WebSocketDeliverer {
	return &WebSocketDeliverer{
		broadcaster: broadcaster,
		ackTracker:  ackTracker,
	}
}

// DeliverNotification delivers a notification to a device via WebSocket
// If ACK tracking is enabled, registers the notification for timeout handling
// storageID is passed to the ACK tracker for marking as delivered when ACK is received
func (d *WebSocketDeliverer) DeliverNotification(deviceID string, msgType string, payload json.RawMessage, storageID int64) error {
	log.Info("DeliverNotification called",
		"device_id", deviceID,
		"msg_type", msgType,
		"storage_id", storageID,
		"has_ack_tracker", d.ackTracker != nil)

	if d.ackTracker != nil {
		// Register with ACK tracker to get unique ID
		notifID := d.ackTracker.Register(deviceID, msgType, payload, storageID)
		log.Info("Registered with ACK tracker", "device_id", deviceID, "notif_id", notifID, "storage_id", storageID)

		// Send with notification ID for ACK tracking
		err := d.broadcaster.SendNotification(deviceID, notifID, msgType, payload)
		if err != nil {
			// Failed to send, cancel the pending ACK
			log.Error("SendNotification failed, cancelling ACK", "device_id", deviceID, "notif_id", notifID, "error", err)
			d.ackTracker.Cancel(notifID)
			return err
		}
		log.Info("DeliverNotification completed successfully", "device_id", deviceID, "notif_id", notifID)
		return nil
	}

	// Legacy path: no ACK tracking
	log.Info("Using legacy path (no ACK tracking)", "device_id", deviceID)
	return d.broadcaster.SendToDevice(deviceID, msgType, payload)
}

// GetACKTracker returns the ACK tracker (for handling incoming ACKs)
func (d *WebSocketDeliverer) GetACKTracker() *ACKTracker {
	return d.ackTracker
}
