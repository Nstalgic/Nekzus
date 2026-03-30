package notifications

import (
	"encoding/json"
	"time"
)

// Notification status constants
const (
	StatusPending   = "pending"
	StatusDelivered = "delivered"
	StatusFailed    = "failed"
	StatusExpired   = "expired"
	StatusDismissed = "dismissed"
)

// Notification represents an in-memory notification
type Notification struct {
	ID         int64
	DeviceID   string
	Type       string
	Payload    json.RawMessage
	TTL        time.Duration
	MaxRetries int
	CreatedAt  time.Time
}

// StoredNotification represents a notification persisted in storage
type StoredNotification struct {
	ID            int64
	DeviceID      string
	Type          string
	Payload       json.RawMessage
	Status        string
	RetryCount    int
	MaxRetries    int
	CreatedAt     time.Time
	ExpiresAt     time.Time
	DeliveredAt   time.Time
	LastAttemptAt time.Time
	ErrorMessage  string
}

// QueueConfig holds configuration for the notification queue
type QueueConfig struct {
	WorkerCount   int           // Number of worker goroutines
	BufferSize    int           // Channel buffer size
	RetryInterval time.Duration // Interval for retry processor
}

// QueueMetrics holds metrics for the notification queue
type QueueMetrics struct {
	Enqueued     int64 // Total notifications enqueued
	Delivered    int64 // Total notifications delivered
	Failed       int64 // Total notifications failed (max retries)
	Expired      int64 // Total notifications expired
	Retried      int64 // Total retry attempts
	QueueDepth   int   // Current queue depth (channel length)
	WorkerCount  int   // Number of active workers
	StorageFalls int64 // Times fell back to storage (overflow)
}

// Storage defines the interface for notification persistence
type Storage interface {
	EnqueueNotification(deviceID, msgType string, payload json.RawMessage, ttl time.Duration, maxRetries int) (int64, error)
	GetPendingNotifications(deviceID string) ([]*StoredNotification, error)
	GetNotificationByID(id int64) (*StoredNotification, error)
	GetAllRetryableNotifications() ([]*StoredNotification, error)
	MarkNotificationDelivered(id int64) error
	MarkNotificationExpired(id int64) error
	UpdateNotificationRetry(id int64, errorMsg string) error
	ResetNotificationForRetry(id int64) error
}

// Deliverer defines the interface for notification delivery
type Deliverer interface {
	// DeliverNotification delivers a notification to a device
	// storageID is the database ID used for ACK tracking (0 for in-memory only notifications)
	DeliverNotification(deviceID string, msgType string, payload json.RawMessage, storageID int64) error
}

// ConnectivityChecker defines the interface for checking device connectivity
type ConnectivityChecker interface {
	IsDeviceConnected(deviceID string) bool
}
