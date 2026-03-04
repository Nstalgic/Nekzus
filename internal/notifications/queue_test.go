package notifications

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	apperrors "github.com/nstalgic/nekzus/internal/errors"
)

// MockStorage implements notification storage for testing
type MockStorage struct {
	mu            sync.RWMutex
	notifications map[int64]*StoredNotification
	nextID        int64
	failEnqueue   bool
	failGet       bool
	failMark      bool
}

func NewMockStorage() *MockStorage {
	return &MockStorage{
		notifications: make(map[int64]*StoredNotification),
		nextID:        1,
	}
}

func (m *MockStorage) EnqueueNotification(deviceID, msgType string, payload json.RawMessage, ttl time.Duration, maxRetries int) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.failEnqueue {
		return 0, apperrors.New("STORAGE_ERROR", "mock enqueue error", 500)
	}

	id := m.nextID
	m.nextID++

	m.notifications[id] = &StoredNotification{
		ID:         id,
		DeviceID:   deviceID,
		Type:       msgType,
		Payload:    payload,
		Status:     StatusPending,
		RetryCount: 0,
		MaxRetries: maxRetries,
		CreatedAt:  time.Now(),
		ExpiresAt:  time.Now().Add(ttl),
	}

	return id, nil
}

func (m *MockStorage) GetPendingNotifications(deviceID string) ([]*StoredNotification, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.failGet {
		return nil, apperrors.New("STORAGE_ERROR", "mock get error", 500)
	}

	var result []*StoredNotification
	now := time.Now()
	for _, notif := range m.notifications {
		if notif.DeviceID == deviceID && notif.Status == StatusPending && notif.ExpiresAt.After(now) {
			result = append(result, notif)
		}
	}
	return result, nil
}

func (m *MockStorage) MarkNotificationDelivered(id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.failMark {
		return apperrors.New("STORAGE_ERROR", "mock mark error", 500)
	}

	if notif, ok := m.notifications[id]; ok {
		notif.Status = StatusDelivered
		notif.DeliveredAt = time.Now()
	}
	return nil
}

func (m *MockStorage) UpdateNotificationRetry(id int64, errorMsg string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if notif, ok := m.notifications[id]; ok {
		notif.RetryCount++
		notif.LastAttemptAt = time.Now()
		notif.ErrorMessage = errorMsg
		if notif.RetryCount >= notif.MaxRetries {
			notif.Status = StatusFailed
		}
	}
	return nil
}

func (m *MockStorage) GetNotification(id int64) *StoredNotification {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.notifications[id]
}

func (m *MockStorage) GetNotificationByID(id int64) (*StoredNotification, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.failGet {
		return nil, apperrors.New("STORAGE_ERROR", "mock get error", 500)
	}

	notif, ok := m.notifications[id]
	if !ok {
		// Match real storage behavior: return (nil, nil) when not found
		return nil, nil
	}
	return notif, nil
}

func (m *MockStorage) GetAllRetryableNotifications() ([]*StoredNotification, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.failGet {
		return nil, apperrors.New("STORAGE_ERROR", "mock get error", 500)
	}

	var result []*StoredNotification
	now := time.Now()
	for _, notif := range m.notifications {
		if notif.Status == StatusPending && notif.ExpiresAt.After(now) {
			result = append(result, notif)
		}
	}
	return result, nil
}

func (m *MockStorage) ResetNotificationForRetry(id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	notif, ok := m.notifications[id]
	if !ok {
		return apperrors.New("NOT_FOUND", "notification not found", 404)
	}

	notif.Status = StatusPending
	notif.RetryCount = 0
	notif.ErrorMessage = ""
	return nil
}

// MockDeliverer implements notification delivery for testing
type MockDeliverer struct {
	mu              sync.RWMutex
	delivered       []DeliveryAttempt
	shouldFail      map[string]bool // deviceID -> fail
	deliveryLatency time.Duration
}

type DeliveryAttempt struct {
	DeviceID  string
	Type      string
	Payload   json.RawMessage
	Timestamp time.Time
}

func NewMockDeliverer() *MockDeliverer {
	return &MockDeliverer{
		delivered:  make([]DeliveryAttempt, 0),
		shouldFail: make(map[string]bool),
	}
}

func (m *MockDeliverer) DeliverNotification(deviceID string, msgType string, payload json.RawMessage, storageID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.deliveryLatency > 0 {
		time.Sleep(m.deliveryLatency)
	}

	if m.shouldFail[deviceID] {
		return apperrors.New("DELIVERY_FAILED", "device offline", 503)
	}

	m.delivered = append(m.delivered, DeliveryAttempt{
		DeviceID:  deviceID,
		Type:      msgType,
		Payload:   payload,
		Timestamp: time.Now(),
	})

	return nil
}

func (m *MockDeliverer) SetDeviceOffline(deviceID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.shouldFail[deviceID] = true
}

func (m *MockDeliverer) SetDeviceOnline(deviceID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.shouldFail[deviceID] = false
}

func (m *MockDeliverer) GetDeliveryCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.delivered)
}

func (m *MockDeliverer) GetDeliveriesForDevice(deviceID string) []DeliveryAttempt {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []DeliveryAttempt
	for _, d := range m.delivered {
		if d.DeviceID == deviceID {
			result = append(result, d)
		}
	}
	return result
}

// MockConnectivityChecker implements ConnectivityChecker for testing
type MockConnectivityChecker struct {
	mu        sync.RWMutex
	connected map[string]bool
}

func NewMockConnectivityChecker() *MockConnectivityChecker {
	return &MockConnectivityChecker{
		connected: make(map[string]bool),
	}
}

func (m *MockConnectivityChecker) IsDeviceConnected(deviceID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.connected[deviceID]
}

func (m *MockConnectivityChecker) SetConnected(deviceID string, connected bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.connected[deviceID] = connected
}

func TestNewQueue(t *testing.T) {
	storage := NewMockStorage()
	deliverer := NewMockDeliverer()

	config := QueueConfig{
		WorkerCount:   5,
		BufferSize:    100,
		RetryInterval: 1 * time.Second,
	}

	queue := NewQueue(config, storage, deliverer)
	if queue == nil {
		t.Fatal("NewQueue returned nil")
	}

	if queue.config.WorkerCount != 5 {
		t.Errorf("expected WorkerCount=5, got %d", queue.config.WorkerCount)
	}

	if queue.config.BufferSize != 100 {
		t.Errorf("expected BufferSize=100, got %d", queue.config.BufferSize)
	}
}

func TestEnqueue_DeliveryToOnlineDevice(t *testing.T) {
	storage := NewMockStorage()
	deliverer := NewMockDeliverer()

	config := QueueConfig{
		WorkerCount:   2,
		BufferSize:    10,
		RetryInterval: 100 * time.Millisecond,
	}

	queue := NewQueue(config, storage, deliverer)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := queue.Start(ctx); err != nil {
		t.Fatalf("failed to start queue: %v", err)
	}
	defer queue.Stop()

	// Device is online by default in mock
	deviceID := "test-device-1"
	msgType := "config_reload"
	payload := json.RawMessage(`{"key":"value"}`)

	err := queue.Enqueue(deviceID, msgType, payload, 5*time.Minute, 3)
	if err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}

	// Wait for delivery
	time.Sleep(500 * time.Millisecond)

	deliveries := deliverer.GetDeliveriesForDevice(deviceID)
	if len(deliveries) != 1 {
		t.Errorf("expected 1 delivery, got %d", len(deliveries))
	}

	if len(deliveries) > 0 {
		if deliveries[0].Type != msgType {
			t.Errorf("expected type=%s, got %s", msgType, deliveries[0].Type)
		}
		if string(deliveries[0].Payload) != string(payload) {
			t.Errorf("expected payload=%s, got %s", payload, deliveries[0].Payload)
		}
	}
}

func TestEnqueue_PersistenceOnOfflineDevice(t *testing.T) {
	storage := NewMockStorage()
	deliverer := NewMockDeliverer()

	deviceID := "offline-device"
	deliverer.SetDeviceOffline(deviceID)

	// Use long retry interval to prevent background processor from exhausting retries
	// This test verifies initial persistence to storage, not background retry behavior
	config := QueueConfig{
		WorkerCount:   2,
		BufferSize:    10,
		RetryInterval: 5 * time.Second,
	}

	queue := NewQueue(config, storage, deliverer)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := queue.Start(ctx); err != nil {
		t.Fatalf("failed to start queue: %v", err)
	}
	defer queue.Stop()

	msgType := "health_change"
	payload := json.RawMessage(`{"status":"unhealthy"}`)

	err := queue.Enqueue(deviceID, msgType, payload, 5*time.Minute, 3)
	if err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}

	// Wait for delivery attempt
	time.Sleep(500 * time.Millisecond)

	// Check no delivery happened
	if deliverer.GetDeliveryCount() != 0 {
		t.Errorf("expected 0 deliveries, got %d", deliverer.GetDeliveryCount())
	}

	// Check notification persisted to storage
	pending, err := storage.GetPendingNotifications(deviceID)
	if err != nil {
		t.Fatalf("GetPendingNotifications failed: %v", err)
	}

	if len(pending) != 1 {
		t.Fatalf("expected 1 pending notification, got %d", len(pending))
	}

	if pending[0].RetryCount != 1 {
		t.Errorf("expected RetryCount=1, got %d", pending[0].RetryCount)
	}
}

func TestRetryOnConnect(t *testing.T) {
	storage := NewMockStorage()
	deliverer := NewMockDeliverer()

	deviceID := "reconnect-device"
	deliverer.SetDeviceOffline(deviceID)

	// Use long retry interval to prevent background processor from interfering
	// This test is specifically for explicit RetryDevice calls on device connect
	config := QueueConfig{
		WorkerCount:   2,
		BufferSize:    10,
		RetryInterval: 5 * time.Second,
	}

	queue := NewQueue(config, storage, deliverer)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := queue.Start(ctx); err != nil {
		t.Fatalf("failed to start queue: %v", err)
	}
	defer queue.Stop()

	// Enqueue while offline
	payload := json.RawMessage(`{"event":"test"}`)
	err := queue.Enqueue(deviceID, "event", payload, 5*time.Minute, 3)
	if err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}

	time.Sleep(300 * time.Millisecond)

	// Verify no delivery yet
	if deliverer.GetDeliveryCount() != 0 {
		t.Errorf("expected 0 deliveries while offline, got %d", deliverer.GetDeliveryCount())
	}

	// Device comes online
	deliverer.SetDeviceOnline(deviceID)

	// Trigger retry
	err = queue.RetryDevice(deviceID)
	if err != nil {
		t.Fatalf("RetryDevice failed: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	// Verify delivery happened
	deliveries := deliverer.GetDeliveriesForDevice(deviceID)
	if len(deliveries) != 1 {
		t.Errorf("expected 1 delivery after reconnect, got %d", len(deliveries))
	}
}

func TestMultipleWorkers_Concurrent(t *testing.T) {
	storage := NewMockStorage()
	deliverer := NewMockDeliverer()

	config := QueueConfig{
		WorkerCount:   5,
		BufferSize:    100,
		RetryInterval: 100 * time.Millisecond,
	}

	queue := NewQueue(config, storage, deliverer)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := queue.Start(ctx); err != nil {
		t.Fatalf("failed to start queue: %v", err)
	}
	defer queue.Stop()

	// Enqueue 50 notifications concurrently
	numNotifications := 50
	var wg sync.WaitGroup

	for i := 0; i < numNotifications; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			deviceID := "device-" + string(rune(idx%10))
			payload := json.RawMessage(`{"index":` + string(rune(idx)) + `}`)
			if err := queue.Enqueue(deviceID, "test", payload, 5*time.Minute, 3); err != nil {
				t.Errorf("Enqueue failed: %v", err)
			}
		}(i)
	}

	wg.Wait()
	time.Sleep(2 * time.Second)

	deliveryCount := deliverer.GetDeliveryCount()
	if deliveryCount != numNotifications {
		t.Errorf("expected %d deliveries, got %d", numNotifications, deliveryCount)
	}
}

func TestGracefulShutdown_PersistsUndelivered(t *testing.T) {
	storage := NewMockStorage()
	deliverer := NewMockDeliverer()

	// Make delivery slow so we can test shutdown
	deliverer.deliveryLatency = 100 * time.Millisecond

	config := QueueConfig{
		WorkerCount:   1,
		BufferSize:    10,
		RetryInterval: 1 * time.Second,
	}

	queue := NewQueue(config, storage, deliverer)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := queue.Start(ctx); err != nil {
		t.Fatalf("failed to start queue: %v", err)
	}

	// Enqueue multiple notifications
	for i := 0; i < 5; i++ {
		deviceID := "device-shutdown"
		payload := json.RawMessage(`{"index":` + string(rune(i)) + `}`)
		if err := queue.Enqueue(deviceID, "test", payload, 5*time.Minute, 3); err != nil {
			t.Fatalf("Enqueue failed: %v", err)
		}
	}

	// Shutdown quickly - some should be unprocessed
	time.Sleep(50 * time.Millisecond)
	queue.Stop()

	// With ACK-required flow, all notifications are persisted to storage first
	// and remain "pending" until client ACKs. So all 5 should be pending.
	pending, err := storage.GetPendingNotifications("device-shutdown")
	if err != nil {
		t.Fatalf("GetPendingNotifications failed: %v", err)
	}

	// All notifications should be in storage (persisted before send)
	// They remain pending until ACK is received
	if len(pending) != 5 {
		t.Errorf("expected 5 pending notifications (awaiting ACK), got %d", len(pending))
	}
}

func TestMaxRetries(t *testing.T) {
	storage := NewMockStorage()
	deliverer := NewMockDeliverer()

	deviceID := "retry-device"
	deliverer.SetDeviceOffline(deviceID)

	config := QueueConfig{
		WorkerCount:   1,
		BufferSize:    10,
		RetryInterval: 100 * time.Millisecond,
	}

	queue := NewQueue(config, storage, deliverer)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := queue.Start(ctx); err != nil {
		t.Fatalf("failed to start queue: %v", err)
	}
	defer queue.Stop()

	maxRetries := 3
	payload := json.RawMessage(`{"test":"data"}`)

	err := queue.Enqueue(deviceID, "test", payload, 5*time.Minute, maxRetries)
	if err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}

	// Wait for initial attempt + 3 retries
	time.Sleep(500 * time.Millisecond)

	// Manually trigger retries
	for i := 0; i < maxRetries; i++ {
		queue.RetryDevice(deviceID)
		time.Sleep(200 * time.Millisecond)
	}

	// Check notification is marked as failed
	pending, _ := storage.GetPendingNotifications(deviceID)
	if len(pending) > 0 {
		t.Errorf("notification should be marked as failed after max retries")
	}
}

func TestExpiredNotifications(t *testing.T) {
	storage := NewMockStorage()
	deliverer := NewMockDeliverer()

	deviceID := "expire-device"
	deliverer.SetDeviceOffline(deviceID)

	config := QueueConfig{
		WorkerCount:   1,
		BufferSize:    10,
		RetryInterval: 100 * time.Millisecond,
	}

	queue := NewQueue(config, storage, deliverer)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := queue.Start(ctx); err != nil {
		t.Fatalf("failed to start queue: %v", err)
	}
	defer queue.Stop()

	// Enqueue with very short TTL
	shortTTL := 200 * time.Millisecond
	payload := json.RawMessage(`{"test":"expire"}`)

	err := queue.Enqueue(deviceID, "test", payload, shortTTL, 3)
	if err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}

	// Wait for expiration
	time.Sleep(500 * time.Millisecond)

	// Bring device online
	deliverer.SetDeviceOnline(deviceID)
	queue.RetryDevice(deviceID)

	time.Sleep(200 * time.Millisecond)

	// Should not deliver expired notification
	if deliverer.GetDeliveryCount() != 0 {
		t.Errorf("expired notification should not be delivered")
	}
}

func TestQueueMetrics(t *testing.T) {
	storage := NewMockStorage()
	deliverer := NewMockDeliverer()

	config := QueueConfig{
		WorkerCount:   2,
		BufferSize:    10,
		RetryInterval: 100 * time.Millisecond,
	}

	queue := NewQueue(config, storage, deliverer)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := queue.Start(ctx); err != nil {
		t.Fatalf("failed to start queue: %v", err)
	}
	defer queue.Stop()

	// Enqueue some notifications
	for i := 0; i < 5; i++ {
		payload := json.RawMessage(`{"idx":` + string(rune(i)) + `}`)
		queue.Enqueue("device-1", "test", payload, 5*time.Minute, 3)
	}

	time.Sleep(500 * time.Millisecond)

	metrics := queue.GetMetrics()

	if metrics.Enqueued != 5 {
		t.Errorf("expected Enqueued=5, got %d", metrics.Enqueued)
	}

	// With ACK-required flow, notifications are not marked "delivered" until client ACKs
	// So delivered count should be 0 after initial send (notifications are awaiting ACK)
	if metrics.Delivered != 0 {
		t.Errorf("expected Delivered=0 (awaiting ACK), got %d", metrics.Delivered)
	}

	if metrics.Failed != 0 {
		t.Errorf("expected Failed=0, got %d", metrics.Failed)
	}
}

func TestBufferOverflow(t *testing.T) {
	storage := NewMockStorage()
	deliverer := NewMockDeliverer()

	// Small buffer to test overflow
	config := QueueConfig{
		WorkerCount:   1,
		BufferSize:    5,
		RetryInterval: 1 * time.Second,
	}

	deliverer.deliveryLatency = 100 * time.Millisecond // Slow delivery

	queue := NewQueue(config, storage, deliverer)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := queue.Start(ctx); err != nil {
		t.Fatalf("failed to start queue: %v", err)
	}
	defer queue.Stop()

	// Try to enqueue more than buffer size
	var overflowCount atomic.Int32

	for i := 0; i < 10; i++ {
		payload := json.RawMessage(`{"idx":` + string(rune(i)) + `}`)
		err := queue.Enqueue("device-1", "test", payload, 5*time.Minute, 3)
		if err != nil {
			// Should fallback to storage on overflow
			if err.Error() == "queue buffer full, persisted to storage" {
				overflowCount.Add(1)
			}
		}
	}

	// Some should have overflowed
	if overflowCount.Load() == 0 {
		t.Error("expected some notifications to overflow to storage")
	}

	// Wait for processing
	time.Sleep(3 * time.Second)

	// All should eventually be delivered
	if deliverer.GetDeliveryCount() < 5 {
		t.Errorf("expected at least 5 deliveries, got %d", deliverer.GetDeliveryCount())
	}
}

func TestRetryNotification_SingleNotification(t *testing.T) {
	storage := NewMockStorage()
	deliverer := NewMockDeliverer()

	deviceID := "retry-single-device"
	deliverer.SetDeviceOffline(deviceID)

	config := QueueConfig{
		WorkerCount:   2,
		BufferSize:    10,
		RetryInterval: 1 * time.Second,
	}

	queue := NewQueue(config, storage, deliverer)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := queue.Start(ctx); err != nil {
		t.Fatalf("failed to start queue: %v", err)
	}
	defer queue.Stop()

	// Enqueue while device is offline - will fail and persist to storage
	payload := json.RawMessage(`{"test":"retry"}`)
	err := queue.Enqueue(deviceID, "test.event", payload, 5*time.Minute, 3)
	if err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}

	// Wait for initial delivery attempt to fail
	time.Sleep(300 * time.Millisecond)

	// Verify no delivery
	if deliverer.GetDeliveryCount() != 0 {
		t.Errorf("expected 0 deliveries while offline, got %d", deliverer.GetDeliveryCount())
	}

	// Get the notification ID from storage
	pending, _ := storage.GetPendingNotifications(deviceID)
	if len(pending) != 1 {
		t.Fatalf("expected 1 pending notification, got %d", len(pending))
	}
	notifID := pending[0].ID

	// Device comes online
	deliverer.SetDeviceOnline(deviceID)

	// Retry single notification by ID
	delivered, err := queue.RetryNotification(notifID)
	if err != nil {
		t.Fatalf("RetryNotification failed: %v", err)
	}
	if !delivered {
		t.Error("expected notification to be delivered")
	}

	// Verify delivery happened (WebSocket send was successful)
	deliveries := deliverer.GetDeliveriesForDevice(deviceID)
	if len(deliveries) != 1 {
		t.Errorf("expected 1 delivery after retry, got %d", len(deliveries))
	}

	// With ACK-required flow, notification remains "pending" until client ACKs
	// The "delivered" return value means WebSocket send succeeded, not storage status
	notif := storage.GetNotification(notifID)
	if notif.Status != StatusPending {
		t.Errorf("expected status=pending (awaiting ACK), got %s", notif.Status)
	}
}

func TestRetryNotification_NotFound(t *testing.T) {
	storage := NewMockStorage()
	deliverer := NewMockDeliverer()

	config := QueueConfig{
		WorkerCount:   1,
		BufferSize:    10,
		RetryInterval: 1 * time.Second,
	}

	queue := NewQueue(config, storage, deliverer)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := queue.Start(ctx); err != nil {
		t.Fatalf("failed to start queue: %v", err)
	}
	defer queue.Stop()

	// Try to retry non-existent notification
	_, err := queue.RetryNotification(999999)
	if err == nil {
		t.Error("expected error for non-existent notification")
	}
}

func TestRetryNotification_DeviceOffline(t *testing.T) {
	storage := NewMockStorage()
	deliverer := NewMockDeliverer()

	deviceID := "offline-retry-device"
	deliverer.SetDeviceOffline(deviceID)

	config := QueueConfig{
		WorkerCount:   1,
		BufferSize:    10,
		RetryInterval: 1 * time.Second,
	}

	queue := NewQueue(config, storage, deliverer)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := queue.Start(ctx); err != nil {
		t.Fatalf("failed to start queue: %v", err)
	}
	defer queue.Stop()

	// Enqueue
	payload := json.RawMessage(`{"test":"offline"}`)
	queue.Enqueue(deviceID, "test.event", payload, 5*time.Minute, 3)

	time.Sleep(300 * time.Millisecond)

	pending, _ := storage.GetPendingNotifications(deviceID)
	if len(pending) != 1 {
		t.Fatalf("expected 1 pending notification, got %d", len(pending))
	}
	notifID := pending[0].ID

	// Try retry while still offline
	delivered, err := queue.RetryNotification(notifID)
	if err != nil {
		t.Fatalf("RetryNotification should not error on delivery failure: %v", err)
	}
	if delivered {
		t.Error("expected delivery to fail while offline")
	}

	// Notification should still be pending (with increased retry count)
	notif := storage.GetNotification(notifID)
	if notif.Status != StatusPending && notif.Status != StatusFailed {
		t.Errorf("expected status pending or failed, got %s", notif.Status)
	}
}

// Note: Background retry processor tests removed - retries are now event-driven
// via device connect (RetryDevice) and UI retry button (RetryNotification)

func TestRetryNotification_ChecksConnectivity(t *testing.T) {
	storage := NewMockStorage()
	deliverer := NewMockDeliverer()
	connectivity := NewMockConnectivityChecker()

	deviceID := "connectivity-check-device"

	config := QueueConfig{
		WorkerCount:   1,
		BufferSize:    10,
		RetryInterval: 5 * time.Second,
	}

	queue := NewQueue(config, storage, deliverer)
	queue.SetConnectivityChecker(connectivity)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := queue.Start(ctx); err != nil {
		t.Fatalf("failed to start queue: %v", err)
	}
	defer queue.Stop()

	// Manually add notification to storage (simulating failed delivery)
	payload := json.RawMessage(`{"test":"connectivity"}`)
	id, _ := storage.EnqueueNotification(deviceID, "test.event", payload, 5*time.Minute, 3)

	// Device is NOT connected
	connectivity.SetConnected(deviceID, false)

	// Retry should fail with "device not connected" error
	_, err := queue.RetryNotification(id)
	if err == nil {
		t.Error("expected error when device not connected")
	}
	if err != nil && err.Error() != "device not connected" {
		t.Errorf("expected 'device not connected' error, got: %v", err)
	}

	// Verify no delivery was attempted
	if deliverer.GetDeliveryCount() != 0 {
		t.Errorf("expected 0 deliveries when device offline, got %d", deliverer.GetDeliveryCount())
	}

	// Now connect the device
	connectivity.SetConnected(deviceID, true)

	// Retry should succeed
	delivered, err := queue.RetryNotification(id)
	if err != nil {
		t.Fatalf("unexpected error when device connected: %v", err)
	}
	if !delivered {
		t.Error("expected delivery to succeed when device connected")
	}

	// Verify delivery happened
	if deliverer.GetDeliveryCount() != 1 {
		t.Errorf("expected 1 delivery, got %d", deliverer.GetDeliveryCount())
	}
}

func TestRetryNotification_NoConnectivityChecker_AlwaysAttempts(t *testing.T) {
	storage := NewMockStorage()
	deliverer := NewMockDeliverer()

	deviceID := "no-checker-device"

	config := QueueConfig{
		WorkerCount:   1,
		BufferSize:    10,
		RetryInterval: 5 * time.Second,
	}

	queue := NewQueue(config, storage, deliverer)
	// NOT setting connectivity checker - should attempt delivery anyway

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := queue.Start(ctx); err != nil {
		t.Fatalf("failed to start queue: %v", err)
	}
	defer queue.Stop()

	// Add notification to storage
	payload := json.RawMessage(`{"test":"no-checker"}`)
	id, _ := storage.EnqueueNotification(deviceID, "test.event", payload, 5*time.Minute, 3)

	// Retry should attempt delivery (no connectivity check)
	delivered, err := queue.RetryNotification(id)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !delivered {
		t.Error("expected delivery when no connectivity checker set")
	}

	if deliverer.GetDeliveryCount() != 1 {
		t.Errorf("expected 1 delivery, got %d", deliverer.GetDeliveryCount())
	}
}
