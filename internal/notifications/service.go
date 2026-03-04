package notifications

import (
	"encoding/json"
	"time"
)

// Enqueuer defines the interface for enqueueing notifications
type Enqueuer interface {
	Enqueue(deviceID string, msgType string, payload json.RawMessage, ttl time.Duration, maxRetries int) error
}

// DeviceInfo represents minimal device information needed for notifications
type DeviceInfo struct {
	ID   string
	Name string
}

// DeviceLister defines the interface for listing devices
type DeviceLister interface {
	ListDevices() ([]DeviceInfo, error)
}

// StorageDeviceLister is any storage that can list devices with ID field
type StorageDeviceLister interface {
	ListDevicesForNotifications() ([]DeviceInfo, error)
}

// DeviceListerAdapter adapts a storage.Store to the DeviceLister interface
type DeviceListerAdapter struct {
	listFunc func() ([]DeviceInfo, error)
}

// NewDeviceListerAdapter creates a new adapter from a list function
func NewDeviceListerAdapter(listFunc func() ([]DeviceInfo, error)) *DeviceListerAdapter {
	return &DeviceListerAdapter{listFunc: listFunc}
}

// ListDevices implements DeviceLister
func (a *DeviceListerAdapter) ListDevices() ([]DeviceInfo, error) {
	return a.listFunc()
}

// Service provides a unified interface for sending notifications.
// All notifications are queued first, then delivered by the queue workers.
// This ensures offline devices receive notifications when they reconnect.
type Service struct {
	queue   Enqueuer
	devices DeviceLister
	config  ServiceConfig
}

// NewService creates a new notification service
func NewService(queue Enqueuer, devices DeviceLister, config ServiceConfig) *Service {
	return &Service{
		queue:   queue,
		devices: devices,
		config:  config,
	}
}

// Send queues a notification for a single device.
// The notification will be delivered immediately if the device is online,
// or stored and retried when the device reconnects.
func (s *Service) Send(deviceID, msgType string, data interface{}) error {
	payload, err := json.Marshal(data)
	if err != nil {
		return err
	}

	typeConfig := s.config.GetTypeConfig(msgType)

	return s.queue.Enqueue(
		deviceID,
		msgType,
		json.RawMessage(payload),
		typeConfig.TTL,
		typeConfig.MaxRetries,
	)
}

// SendToAll queues a notification for all registered devices.
// Each device gets its own queued notification, ensuring offline devices
// receive the message when they reconnect.
func (s *Service) SendToAll(msgType string, data interface{}) error {
	devices, err := s.devices.ListDevices()
	if err != nil {
		log.Warn("failed to list devices for notification", "type", msgType, "error", err)
		return err
	}

	if len(devices) == 0 {
		log.Info("no devices to notify", "type", msgType)
		return nil
	}

	payload, err := json.Marshal(data)
	if err != nil {
		return err
	}

	typeConfig := s.config.GetTypeConfig(msgType)
	rawPayload := json.RawMessage(payload)

	var lastErr error
	successCount := 0

	for _, device := range devices {
		if err := s.queue.Enqueue(
			device.ID,
			msgType,
			rawPayload,
			typeConfig.TTL,
			typeConfig.MaxRetries,
		); err != nil {
			log.Warn("failed to enqueue notification",
				"device_id", device.ID,
				"type", msgType,
				"error", err,
			)
			lastErr = err
		} else {
			successCount++
		}
	}

	log.Info("queued notifications for all devices",
		"type", msgType,
		"total_devices", len(devices),
		"queued", successCount,
	)

	return lastErr
}
