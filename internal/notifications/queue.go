package notifications

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

var log = slog.With("package", "notifications")

// Queue manages notification delivery with channel-based workers
type Queue struct {
	config    QueueConfig
	storage   Storage
	deliverer Deliverer

	// Channel for in-memory queue
	notifChan chan *Notification

	// Worker management
	workerWg sync.WaitGroup
	ctx      context.Context
	cancel   context.CancelFunc

	// Connectivity checker for event-based retry
	connectivity ConnectivityChecker

	// Metrics
	metrics struct {
		enqueued     atomic.Int64
		delivered    atomic.Int64
		failed       atomic.Int64
		expired      atomic.Int64
		retried      atomic.Int64
		storageFalls atomic.Int64
	}
}

// NewQueue creates a new notification queue
func NewQueue(config QueueConfig, storage Storage, deliverer Deliverer) *Queue {
	return &Queue{
		config:    config,
		storage:   storage,
		deliverer: deliverer,
		notifChan: make(chan *Notification, config.BufferSize),
	}
}

// Start starts the queue workers
func (q *Queue) Start(ctx context.Context) error {
	if q.ctx != nil {
		return errors.New("queue already started")
	}

	q.ctx, q.cancel = context.WithCancel(ctx)

	// Start worker pool
	for i := 0; i < q.config.WorkerCount; i++ {
		q.workerWg.Add(1)
		go q.worker(i)
	}

	// Note: Background retry processor removed - retries are event-driven
	// via RetryDevice() on device connect and RetryNotification() from UI

	return nil
}

// SetConnectivityChecker sets the connectivity checker for device reachability
func (q *Queue) SetConnectivityChecker(checker ConnectivityChecker) {
	q.connectivity = checker
}

// Stop gracefully stops the queue
func (q *Queue) Stop() {
	if q.cancel != nil {
		q.cancel()
	}

	// Close notification channel to signal workers
	close(q.notifChan)

	// Wait for workers to finish processing current notifications
	q.workerWg.Wait()

	// Channel is already closed and drained by workers
	// Any notifications that failed delivery are already persisted to storage
}

// Enqueue adds a notification to the queue
func (q *Queue) Enqueue(deviceID string, msgType string, payload json.RawMessage, ttl time.Duration, maxRetries int) error {
	notif := &Notification{
		DeviceID:   deviceID,
		Type:       msgType,
		Payload:    payload,
		TTL:        ttl,
		MaxRetries: maxRetries,
		CreatedAt:  time.Now(),
	}

	q.metrics.enqueued.Add(1)

	// Try to enqueue to channel
	select {
	case q.notifChan <- notif:
		return nil
	default:
		// Channel full, fallback to storage
		q.metrics.storageFalls.Add(1)
		_, err := q.storage.EnqueueNotification(deviceID, msgType, payload, ttl, maxRetries)
		if err != nil {
			return err
		}
		return errors.New("queue buffer full, persisted to storage")
	}
}

// RetryDevice triggers retry of pending notifications for a device
func (q *Queue) RetryDevice(deviceID string) error {
	log.Info("RetryDevice called", "device_id", deviceID)

	// Load pending notifications from storage
	pending, err := q.storage.GetPendingNotifications(deviceID)
	if err != nil {
		log.Error("GetPendingNotifications failed", "device_id", deviceID, "error", err)
		return err
	}

	log.Info("GetPendingNotifications result", "device_id", deviceID, "pending_count", len(pending))

	// Try to enqueue each notification
	for _, notif := range pending {
		log.Info("Processing notification", "notif_id", notif.ID, "notif_device_id", notif.DeviceID, "type", notif.Type)

		// Check if expired
		if time.Now().After(notif.ExpiresAt) {
			log.Info("Notification expired", "notif_id", notif.ID, "expires_at", notif.ExpiresAt)
			q.metrics.expired.Add(1)
			continue
		}

		// Try to deliver - pass storage ID for ACK tracking
		// Notification will be marked delivered when client ACKs (via ACK tracker callback)
		err := q.deliverer.DeliverNotification(notif.DeviceID, notif.Type, notif.Payload, notif.ID)
		if err == nil {
			// WebSocket send successful - notification is now awaiting ACK
			// Do NOT mark as delivered here - that happens when client ACKs
			log.Info("Notification sent successfully (awaiting ACK)", "notif_id", notif.ID, "device_id", notif.DeviceID)
		} else {
			// Delivery failed, will be retried by retry processor
			log.Warn("Notification delivery failed", "notif_id", notif.ID, "device_id", notif.DeviceID, "error", err)
			q.metrics.retried.Add(1)
			if updateErr := q.storage.UpdateNotificationRetry(notif.ID, err.Error()); updateErr != nil {
				log.Warn("Failed to update notification retry", "error", updateErr)
			}
		}
	}

	return nil
}

// RetryNotification triggers retry for a single notification by ID
// Returns true if delivery succeeded, false if it failed
// Returns error if device is not connected (when connectivity checker is set)
func (q *Queue) RetryNotification(id int64) (bool, error) {
	log.Info("RetryNotification called", "notif_id", id)

	notif, err := q.storage.GetNotificationByID(id)
	if err != nil {
		log.Error("GetNotificationByID failed", "notif_id", id, "error", err)
		return false, err
	}
	if notif == nil {
		log.Warn("Notification not found", "notif_id", id)
		return false, errors.New("notification not found")
	}

	log.Info("Got notification", "notif_id", id, "device_id", notif.DeviceID, "type", notif.Type, "status", notif.Status)

	// Check if device is connected before attempting delivery
	if q.connectivity != nil {
		isConnected := q.connectivity.IsDeviceConnected(notif.DeviceID)
		log.Info("Connectivity check", "device_id", notif.DeviceID, "is_connected", isConnected)
		if !isConnected {
			return false, errors.New("device not connected")
		}
	} else {
		log.Warn("No connectivity checker set, skipping connectivity check")
	}

	// Check if expired
	if time.Now().After(notif.ExpiresAt) {
		log.Info("Notification expired", "notif_id", id, "expires_at", notif.ExpiresAt)
		q.metrics.expired.Add(1)
		return false, errors.New("notification expired")
	}

	// Reset for retry
	if resetErr := q.storage.ResetNotificationForRetry(id); resetErr != nil {
		log.Warn("Failed to reset notification for retry", "error", resetErr)
	}

	// Attempt delivery - pass storage ID for ACK tracking
	log.Info("Attempting delivery", "notif_id", id, "device_id", notif.DeviceID, "type", notif.Type)
	deliveryErr := q.deliverer.DeliverNotification(notif.DeviceID, notif.Type, notif.Payload, id)
	if deliveryErr == nil {
		// WebSocket send successful - notification is now awaiting ACK
		// Do NOT mark as delivered here - that happens when client ACKs
		log.Info("Notification sent successfully (awaiting ACK)", "notif_id", id, "device_id", notif.DeviceID)
		return true, nil
	}

	// Delivery failed, update retry count
	log.Warn("Delivery failed", "notif_id", id, "device_id", notif.DeviceID, "error", deliveryErr)
	q.metrics.retried.Add(1)
	if updateErr := q.storage.UpdateNotificationRetry(id, deliveryErr.Error()); updateErr != nil {
		log.Warn("Failed to update notification retry", "error", updateErr)
	}

	return false, nil
}

// GetMetrics returns current queue metrics
func (q *Queue) GetMetrics() QueueMetrics {
	return QueueMetrics{
		Enqueued:     q.metrics.enqueued.Load(),
		Delivered:    q.metrics.delivered.Load(),
		Failed:       q.metrics.failed.Load(),
		Expired:      q.metrics.expired.Load(),
		Retried:      q.metrics.retried.Load(),
		QueueDepth:   len(q.notifChan),
		WorkerCount:  q.config.WorkerCount,
		StorageFalls: q.metrics.storageFalls.Load(),
	}
}

// worker processes notifications from the channel
func (q *Queue) worker(id int) {
	defer q.workerWg.Done()

	for {
		select {
		case notif, ok := <-q.notifChan:
			if !ok {
				// Channel closed, exit
				return
			}
			q.processNotification(notif)
		case <-q.ctx.Done():
			// Context cancelled, but keep draining channel
			// This ensures we process all enqueued notifications
			for notif := range q.notifChan {
				q.processNotification(notif)
			}
			return
		}
	}
}

// processNotification attempts to deliver a notification
func (q *Queue) processNotification(notif *Notification) {
	// Check if expired
	if notif.TTL > 0 && time.Since(notif.CreatedAt) > notif.TTL {
		q.metrics.expired.Add(1)
		return
	}

	// First persist to storage to get an ID for ACK tracking
	storageID, storeErr := q.storage.EnqueueNotification(
		notif.DeviceID,
		notif.Type,
		notif.Payload,
		notif.TTL,
		notif.MaxRetries,
	)
	if storeErr != nil {
		log.Error("Failed to persist notification", "error", storeErr)
		q.metrics.failed.Add(1)
		return
	}

	// Attempt delivery with storage ID for ACK tracking
	err := q.deliverer.DeliverNotification(notif.DeviceID, notif.Type, notif.Payload, storageID)
	if err == nil {
		// WebSocket send successful - notification is now awaiting ACK
		// Do NOT mark as delivered here - that happens when client ACKs
		log.Info("Notification sent successfully (awaiting ACK)", "storage_id", storageID, "device_id", notif.DeviceID)
		return
	}

	// Delivery failed, update with first retry attempt
	log.Warn("Notification delivery failed", "storage_id", storageID, "device_id", notif.DeviceID, "error", err)
	updateErr := q.storage.UpdateNotificationRetry(storageID, err.Error())
	if updateErr != nil {
		log.Warn("Failed to update retry count", "error", updateErr)
	}

	q.metrics.retried.Add(1)
}
