package notifications

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/google/uuid"
)

// PendingNotification represents a notification awaiting ACK
type PendingNotification struct {
	ID        string
	StorageID int64 // Database ID for marking delivered
	DeviceID  string
	Type      string
	Payload   json.RawMessage
	SentAt    time.Time
	ExpiresAt time.Time
}

// ACKTrackerConfig holds configuration for the ACK tracker
type ACKTrackerConfig struct {
	ACKTimeout    time.Duration                                                    // How long to wait for ACK before timeout
	CheckInterval time.Duration                                                    // How often to check for timeouts
	OnACK         func(storageID int64)                                            // Callback when notification is acknowledged
	OnTimeout     func(notifID, deviceID, msgType string, payload json.RawMessage) // Callback when notification times out
}

// ACKTracker tracks notifications awaiting acknowledgment from clients
type ACKTracker struct {
	config  ACKTrackerConfig
	pending map[string]*PendingNotification
	mu      sync.RWMutex
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

// NewACKTracker creates a new ACK tracker
func NewACKTracker(config ACKTrackerConfig) *ACKTracker {
	if config.ACKTimeout == 0 {
		config.ACKTimeout = 30 * time.Second
	}
	if config.CheckInterval == 0 {
		config.CheckInterval = 5 * time.Second
	}

	ctx, cancel := context.WithCancel(context.Background())

	tracker := &ACKTracker{
		config:  config,
		pending: make(map[string]*PendingNotification),
		ctx:     ctx,
		cancel:  cancel,
	}

	// Start timeout checker
	tracker.wg.Add(1)
	go tracker.timeoutChecker()

	return tracker
}

// Register adds a new notification to track and returns its unique ID
// storageID is the database ID used to mark the notification as delivered when ACK is received
func (t *ACKTracker) Register(deviceID, msgType string, payload json.RawMessage, storageID int64) string {
	notifID := uuid.New().String()
	now := time.Now()

	t.mu.Lock()
	t.pending[notifID] = &PendingNotification{
		ID:        notifID,
		StorageID: storageID,
		DeviceID:  deviceID,
		Type:      msgType,
		Payload:   payload,
		SentAt:    now,
		ExpiresAt: now.Add(t.config.ACKTimeout),
	}
	t.mu.Unlock()

	return notifID
}

// ACK marks a notification as acknowledged and triggers the OnACK callback
func (t *ACKTracker) ACK(notifID string) {
	t.mu.Lock()
	notif, exists := t.pending[notifID]
	var storageID int64
	if exists {
		storageID = notif.StorageID
		delete(t.pending, notifID)
	}
	t.mu.Unlock()

	// Call OnACK callback outside of lock if we have a storage ID
	if exists && storageID > 0 && t.config.OnACK != nil {
		t.config.OnACK(storageID)
	}
}

// Cancel removes a pending notification without triggering timeout callback
func (t *ACKTracker) Cancel(notifID string) {
	t.mu.Lock()
	delete(t.pending, notifID)
	t.mu.Unlock()
}

// IsPending checks if a notification is still awaiting ACK
func (t *ACKTracker) IsPending(notifID string) bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	_, exists := t.pending[notifID]
	return exists
}

// GetPendingCount returns the number of pending notifications
func (t *ACKTracker) GetPendingCount() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.pending)
}

// GetPendingForDevice returns all pending notification IDs for a device
func (t *ACKTracker) GetPendingForDevice(deviceID string) []string {
	t.mu.RLock()
	defer t.mu.RUnlock()

	var ids []string
	for id, notif := range t.pending {
		if notif.DeviceID == deviceID {
			ids = append(ids, id)
		}
	}
	return ids
}

// Stop stops the ACK tracker
func (t *ACKTracker) Stop() {
	t.cancel()
	t.wg.Wait()
}

// timeoutChecker periodically checks for timed out notifications
func (t *ACKTracker) timeoutChecker() {
	defer t.wg.Done()

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

// checkTimeouts finds and handles timed out notifications
func (t *ACKTracker) checkTimeouts() {
	now := time.Now()
	var timedOut []*PendingNotification

	t.mu.Lock()
	for id, notif := range t.pending {
		if now.After(notif.ExpiresAt) {
			timedOut = append(timedOut, notif)
			delete(t.pending, id)
		}
	}
	t.mu.Unlock()

	// Call timeout callback outside of lock
	if t.config.OnTimeout != nil {
		for _, notif := range timedOut {
			t.config.OnTimeout(notif.ID, notif.DeviceID, notif.Type, notif.Payload)
		}
	}
}
