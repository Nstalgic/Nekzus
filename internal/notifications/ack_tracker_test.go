package notifications

import (
	"encoding/json"
	"sync"
	"testing"
	"time"
)

func TestACKTracker_RegisterAndACK(t *testing.T) {
	tracker := NewACKTracker(ACKTrackerConfig{
		ACKTimeout:    1 * time.Second,
		CheckInterval: 100 * time.Millisecond,
	})
	defer tracker.Stop()

	deviceID := "device-1"
	msgType := "health_change"
	payload := json.RawMessage(`{"status":"healthy"}`)

	// Register a pending notification (storageID=0 for tests without storage)
	notifID := tracker.Register(deviceID, msgType, payload, 0)

	if notifID == "" {
		t.Fatal("expected non-empty notification ID")
	}

	// Verify it's pending
	if !tracker.IsPending(notifID) {
		t.Error("notification should be pending")
	}

	// ACK the notification
	tracker.ACK(notifID)

	// Verify it's no longer pending
	if tracker.IsPending(notifID) {
		t.Error("notification should not be pending after ACK")
	}
}

func TestACKTracker_TimeoutCallback(t *testing.T) {
	var timeoutCalled bool
	var timeoutNotifID string
	var mu sync.Mutex

	tracker := NewACKTracker(ACKTrackerConfig{
		ACKTimeout:    200 * time.Millisecond,
		CheckInterval: 50 * time.Millisecond,
		OnTimeout: func(notifID, deviceID, msgType string, payload json.RawMessage) {
			mu.Lock()
			timeoutCalled = true
			timeoutNotifID = notifID
			mu.Unlock()
		},
	})
	defer tracker.Stop()

	deviceID := "device-1"
	msgType := "health_change"
	payload := json.RawMessage(`{"status":"unhealthy"}`)

	// Register but don't ACK
	notifID := tracker.Register(deviceID, msgType, payload, 0)

	// Wait for timeout
	time.Sleep(400 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if !timeoutCalled {
		t.Error("timeout callback should have been called")
	}

	if timeoutNotifID != notifID {
		t.Errorf("expected timeout for %s, got %s", notifID, timeoutNotifID)
	}
}

func TestACKTracker_NoTimeoutAfterACK(t *testing.T) {
	var timeoutCalled bool
	var mu sync.Mutex

	tracker := NewACKTracker(ACKTrackerConfig{
		ACKTimeout:    200 * time.Millisecond,
		CheckInterval: 50 * time.Millisecond,
		OnTimeout: func(notifID, deviceID, msgType string, payload json.RawMessage) {
			mu.Lock()
			timeoutCalled = true
			mu.Unlock()
		},
	})
	defer tracker.Stop()

	deviceID := "device-1"
	msgType := "config_reload"
	payload := json.RawMessage(`{}`)

	// Register and immediately ACK
	notifID := tracker.Register(deviceID, msgType, payload, 0)
	tracker.ACK(notifID)

	// Wait past timeout
	time.Sleep(400 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if timeoutCalled {
		t.Error("timeout callback should NOT have been called after ACK")
	}
}

func TestACKTracker_MultipleNotifications(t *testing.T) {
	tracker := NewACKTracker(ACKTrackerConfig{
		ACKTimeout:    1 * time.Second,
		CheckInterval: 100 * time.Millisecond,
	})
	defer tracker.Stop()

	// Register multiple notifications
	ids := make([]string, 5)
	for i := 0; i < 5; i++ {
		payload := json.RawMessage(`{"idx":` + string(rune('0'+i)) + `}`)
		ids[i] = tracker.Register("device-1", "test", payload, 0)
	}

	// All should be pending
	for _, id := range ids {
		if !tracker.IsPending(id) {
			t.Errorf("notification %s should be pending", id)
		}
	}

	// ACK only some
	tracker.ACK(ids[0])
	tracker.ACK(ids[2])
	tracker.ACK(ids[4])

	// Check states
	if tracker.IsPending(ids[0]) {
		t.Error("ids[0] should not be pending after ACK")
	}
	if !tracker.IsPending(ids[1]) {
		t.Error("ids[1] should still be pending")
	}
	if tracker.IsPending(ids[2]) {
		t.Error("ids[2] should not be pending after ACK")
	}
	if !tracker.IsPending(ids[3]) {
		t.Error("ids[3] should still be pending")
	}
	if tracker.IsPending(ids[4]) {
		t.Error("ids[4] should not be pending after ACK")
	}
}

func TestACKTracker_ConcurrentAccess(t *testing.T) {
	tracker := NewACKTracker(ACKTrackerConfig{
		ACKTimeout:    5 * time.Second,
		CheckInterval: 100 * time.Millisecond,
	})
	defer tracker.Stop()

	var wg sync.WaitGroup
	numGoroutines := 50
	ids := make(chan string, numGoroutines)

	// Concurrent registrations
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			payload := json.RawMessage(`{"idx":` + string(rune('0'+idx%10)) + `}`)
			id := tracker.Register("device-"+string(rune('0'+idx%5)), "test", payload, 0)
			ids <- id
		}(i)
	}

	wg.Wait()
	close(ids)

	// Collect all IDs
	allIDs := make([]string, 0, numGoroutines)
	for id := range ids {
		allIDs = append(allIDs, id)
	}

	if len(allIDs) != numGoroutines {
		t.Errorf("expected %d IDs, got %d", numGoroutines, len(allIDs))
	}

	// Concurrent ACKs
	var wg2 sync.WaitGroup
	for _, id := range allIDs {
		wg2.Add(1)
		go func(notifID string) {
			defer wg2.Done()
			tracker.ACK(notifID)
		}(id)
	}
	wg2.Wait()

	// All should be acknowledged
	for _, id := range allIDs {
		if tracker.IsPending(id) {
			t.Errorf("notification %s should not be pending after ACK", id)
		}
	}
}

func TestACKTracker_GetPendingCount(t *testing.T) {
	tracker := NewACKTracker(ACKTrackerConfig{
		ACKTimeout:    5 * time.Second,
		CheckInterval: 100 * time.Millisecond,
	})
	defer tracker.Stop()

	if tracker.GetPendingCount() != 0 {
		t.Errorf("expected 0 pending, got %d", tracker.GetPendingCount())
	}

	// Add 3 notifications
	id1 := tracker.Register("device-1", "test", nil, 0)
	tracker.Register("device-1", "test", nil, 0)
	tracker.Register("device-1", "test", nil, 0)

	if tracker.GetPendingCount() != 3 {
		t.Errorf("expected 3 pending, got %d", tracker.GetPendingCount())
	}

	// ACK one
	tracker.ACK(id1)

	if tracker.GetPendingCount() != 2 {
		t.Errorf("expected 2 pending after ACK, got %d", tracker.GetPendingCount())
	}
}

func TestACKTracker_GetPendingForDevice(t *testing.T) {
	tracker := NewACKTracker(ACKTrackerConfig{
		ACKTimeout:    5 * time.Second,
		CheckInterval: 100 * time.Millisecond,
	})
	defer tracker.Stop()

	// Add notifications for different devices
	tracker.Register("device-1", "test", nil, 0)
	tracker.Register("device-1", "test", nil, 0)
	tracker.Register("device-2", "test", nil, 0)

	device1Pending := tracker.GetPendingForDevice("device-1")
	if len(device1Pending) != 2 {
		t.Errorf("expected 2 pending for device-1, got %d", len(device1Pending))
	}

	device2Pending := tracker.GetPendingForDevice("device-2")
	if len(device2Pending) != 1 {
		t.Errorf("expected 1 pending for device-2, got %d", len(device2Pending))
	}

	device3Pending := tracker.GetPendingForDevice("device-3")
	if len(device3Pending) != 0 {
		t.Errorf("expected 0 pending for device-3, got %d", len(device3Pending))
	}
}

func TestACKTracker_CancelPending(t *testing.T) {
	tracker := NewACKTracker(ACKTrackerConfig{
		ACKTimeout:    5 * time.Second,
		CheckInterval: 100 * time.Millisecond,
	})
	defer tracker.Stop()

	id := tracker.Register("device-1", "test", nil, 0)

	if !tracker.IsPending(id) {
		t.Error("should be pending after register")
	}

	// Cancel the pending notification
	tracker.Cancel(id)

	if tracker.IsPending(id) {
		t.Error("should not be pending after cancel")
	}
}
