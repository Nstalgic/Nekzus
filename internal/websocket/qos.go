package websocket

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/nstalgic/nekzus/internal/types"
)

// QoSTrackerConfig configures the QoS tracker behavior.
type QoSTrackerConfig struct {
	ACKTimeout    time.Duration // How long to wait for ACK
	CheckInterval time.Duration // How often to check for timeouts
	MaxRetries    int           // Maximum retry attempts
	OnTimeout     func(messageID, deviceID, topic string)
	OnRetry       func(messageID, deviceID, topic string, msg types.WebSocketMessage, attempt int)
}

// QoSTracker tracks messages awaiting acknowledgment (QoS 1).
type QoSTracker struct {
	pending map[string]*PendingAck
	mu      sync.RWMutex
	config  QoSTrackerConfig
	ctx     context.Context
	cancel  context.CancelFunc
}

// NewQoSTracker creates a new QoS tracker.
func NewQoSTracker(config QoSTrackerConfig) *QoSTracker {
	if config.ACKTimeout == 0 {
		config.ACKTimeout = 30 * time.Second
	}
	if config.CheckInterval == 0 {
		config.CheckInterval = 5 * time.Second
	}
	if config.MaxRetries == 0 {
		config.MaxRetries = 3
	}

	ctx, cancel := context.WithCancel(context.Background())
	tracker := &QoSTracker{
		pending: make(map[string]*PendingAck),
		config:  config,
		ctx:     ctx,
		cancel:  cancel,
	}

	go tracker.checkLoop()

	return tracker
}

// RegisterQoS1 registers a message for QoS 1 tracking.
// Returns the assigned message ID.
func (t *QoSTracker) RegisterQoS1(deviceID, topic string, msg types.WebSocketMessage) string {
	messageID := uuid.New().String()
	now := time.Now()

	pending := &PendingAck{
		MessageID:  messageID,
		DeviceID:   deviceID,
		Topic:      topic,
		Message:    msg,
		QoS:        QoSAtLeastOnce,
		Status:     StatusPending,
		RetryCount: 0,
		SentAt:     now,
		ExpiresAt:  now.Add(t.config.ACKTimeout),
	}

	t.mu.Lock()
	t.pending[messageID] = pending
	t.mu.Unlock()

	return messageID
}

// ACK acknowledges a message, removing it from pending.
// Returns true if the message was found and acknowledged.
func (t *QoSTracker) ACK(messageID string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	if _, exists := t.pending[messageID]; exists {
		delete(t.pending, messageID)
		return true
	}
	return false
}

// Cancel cancels tracking for a specific message.
func (t *QoSTracker) Cancel(messageID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.pending, messageID)
}

// CancelForDevice cancels all pending messages for a device.
// Returns the number of messages cancelled.
func (t *QoSTracker) CancelForDevice(deviceID string) int {
	t.mu.Lock()
	defer t.mu.Unlock()

	count := 0
	for id, pending := range t.pending {
		if pending.DeviceID == deviceID {
			delete(t.pending, id)
			count++
		}
	}
	return count
}

// PendingCount returns the number of pending messages.
func (t *QoSTracker) PendingCount() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.pending)
}

// GetPending returns a copy of a pending message by ID.
func (t *QoSTracker) GetPending(messageID string) *PendingAck {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if pending, exists := t.pending[messageID]; exists {
		// Return a copy
		copy := *pending
		return &copy
	}
	return nil
}

// Stop stops the tracker and its background goroutine.
func (t *QoSTracker) Stop() {
	t.cancel()
}

// checkLoop periodically checks for expired messages.
func (t *QoSTracker) checkLoop() {
	ticker := time.NewTicker(t.config.CheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-t.ctx.Done():
			return
		case <-ticker.C:
			t.checkTimeouts()
		}
	}
}

// checkTimeouts processes expired messages.
func (t *QoSTracker) checkTimeouts() {
	now := time.Now()
	var expired []*PendingAck

	t.mu.Lock()
	for id, pending := range t.pending {
		if now.After(pending.ExpiresAt) {
			if pending.RetryCount >= t.config.MaxRetries {
				// Max retries reached, timeout
				expired = append(expired, pending)
				delete(t.pending, id)
			} else {
				// Retry
				pending.RetryCount++
				pending.ExpiresAt = now.Add(t.config.ACKTimeout)
				pending.Status = StatusSent

				if t.config.OnRetry != nil {
					// Call retry callback outside lock
					go t.config.OnRetry(pending.MessageID, pending.DeviceID, pending.Topic, pending.Message, pending.RetryCount)
				}
			}
		}
	}
	t.mu.Unlock()

	// Call timeout callbacks outside lock
	if t.config.OnTimeout != nil {
		for _, pending := range expired {
			t.config.OnTimeout(pending.MessageID, pending.DeviceID, pending.Topic)
		}
	}
}

// QoS2TrackerConfig configures the QoS 2 deduplication tracker.
type QoS2TrackerConfig struct {
	DedupWindow   time.Duration // How long to keep message IDs for dedup
	CheckInterval time.Duration // How often to clean expired entries
}

// qos2Entry tracks a QoS 2 message for deduplication.
type qos2Entry struct {
	DeviceID  string
	ExpiresAt time.Time
}

// QoS2Tracker tracks message IDs for exactly-once delivery (QoS 2).
type QoS2Tracker struct {
	processed map[string]*qos2Entry // messageID -> entry
	mu        sync.RWMutex
	config    QoS2TrackerConfig
	ctx       context.Context
	cancel    context.CancelFunc
}

// NewQoS2Tracker creates a new QoS 2 deduplication tracker.
func NewQoS2Tracker(config QoS2TrackerConfig) *QoS2Tracker {
	if config.DedupWindow == 0 {
		config.DedupWindow = 5 * time.Minute
	}
	if config.CheckInterval == 0 {
		config.CheckInterval = 1 * time.Minute
	}

	ctx, cancel := context.WithCancel(context.Background())
	tracker := &QoS2Tracker{
		processed: make(map[string]*qos2Entry),
		config:    config,
		ctx:       ctx,
		cancel:    cancel,
	}

	go tracker.cleanLoop()

	return tracker
}

// Register registers a message ID for deduplication.
// Returns true if this is a new message, false if it's a duplicate.
func (t *QoS2Tracker) Register(messageID, deviceID string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	key := messageID + ":" + deviceID

	// Check if already processed
	if entry, exists := t.processed[key]; exists {
		// Check if expired
		if time.Now().After(entry.ExpiresAt) {
			// Expired, allow re-registration
			delete(t.processed, key)
		} else {
			return false // Duplicate
		}
	}

	// Register new entry
	t.processed[key] = &qos2Entry{
		DeviceID:  deviceID,
		ExpiresAt: time.Now().Add(t.config.DedupWindow),
	}

	return true
}

// IsProcessed checks if a message has been processed.
func (t *QoS2Tracker) IsProcessed(messageID, deviceID string) bool {
	t.mu.RLock()
	defer t.mu.RUnlock()

	key := messageID + ":" + deviceID
	entry, exists := t.processed[key]
	if !exists {
		return false
	}

	// Check if expired
	return !time.Now().After(entry.ExpiresAt)
}

// Complete marks a QoS 2 message as fully completed.
// The message ID remains in the dedup table until it expires.
func (t *QoS2Tracker) Complete(messageID, deviceID string) {
	// For QoS 2, completion doesn't remove from dedup table
	// The entry will expire naturally
}

// Remove removes a message from the dedup table.
func (t *QoS2Tracker) Remove(messageID, deviceID string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	key := messageID + ":" + deviceID
	delete(t.processed, key)
}

// Stop stops the tracker.
func (t *QoS2Tracker) Stop() {
	t.cancel()
}

// cleanLoop periodically cleans expired entries.
func (t *QoS2Tracker) cleanLoop() {
	ticker := time.NewTicker(t.config.CheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-t.ctx.Done():
			return
		case <-ticker.C:
			t.cleanExpired()
		}
	}
}

// cleanExpired removes expired entries.
func (t *QoS2Tracker) cleanExpired() {
	now := time.Now()

	t.mu.Lock()
	defer t.mu.Unlock()

	for key, entry := range t.processed {
		if now.After(entry.ExpiresAt) {
			delete(t.processed, key)
		}
	}
}

// ProcessedCount returns the number of tracked message IDs.
func (t *QoS2Tracker) ProcessedCount() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.processed)
}
