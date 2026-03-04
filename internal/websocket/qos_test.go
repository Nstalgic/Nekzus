package websocket

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/nstalgic/nekzus/internal/types"
)

func TestQoSTracker_NewTracker(t *testing.T) {
	tracker := NewQoSTracker(QoSTrackerConfig{
		ACKTimeout:    5 * time.Second,
		CheckInterval: 1 * time.Second,
		MaxRetries:    3,
	})

	if tracker == nil {
		t.Fatal("Expected non-nil tracker")
	}

	tracker.Stop()
}

func TestQoSTracker_RegisterQoS1(t *testing.T) {
	tracker := NewQoSTracker(QoSTrackerConfig{
		ACKTimeout:    5 * time.Second,
		CheckInterval: 1 * time.Second,
		MaxRetries:    3,
	})
	defer tracker.Stop()

	msg := types.WebSocketMessage{
		Type: types.WSMsgTypeHealthChange,
		Data: map[string]string{"status": "healthy"},
	}

	messageID := tracker.RegisterQoS1("device-1", "health_change", msg)

	if messageID == "" {
		t.Error("Expected non-empty message ID")
	}

	// Verify pending count
	if count := tracker.PendingCount(); count != 1 {
		t.Errorf("Expected 1 pending, got %d", count)
	}
}

func TestQoSTracker_ACK(t *testing.T) {
	tracker := NewQoSTracker(QoSTrackerConfig{
		ACKTimeout:    5 * time.Second,
		CheckInterval: 1 * time.Second,
		MaxRetries:    3,
	})
	defer tracker.Stop()

	msg := types.WebSocketMessage{
		Type: types.WSMsgTypeHealthChange,
		Data: map[string]string{"status": "healthy"},
	}

	messageID := tracker.RegisterQoS1("device-1", "health_change", msg)

	// ACK the message
	acked := tracker.ACK(messageID)
	if !acked {
		t.Error("Expected ACK to return true")
	}

	// Verify pending count
	if count := tracker.PendingCount(); count != 0 {
		t.Errorf("Expected 0 pending after ACK, got %d", count)
	}

	// ACK again should return false
	if tracker.ACK(messageID) {
		t.Error("Second ACK should return false")
	}
}

func TestQoSTracker_ACK_NonExistent(t *testing.T) {
	tracker := NewQoSTracker(QoSTrackerConfig{
		ACKTimeout:    5 * time.Second,
		CheckInterval: 1 * time.Second,
		MaxRetries:    3,
	})
	defer tracker.Stop()

	if tracker.ACK("non-existent-id") {
		t.Error("ACK of non-existent message should return false")
	}
}

func TestQoSTracker_Timeout(t *testing.T) {
	timeoutCalled := make(chan string, 1)

	tracker := NewQoSTracker(QoSTrackerConfig{
		ACKTimeout:    50 * time.Millisecond,
		CheckInterval: 10 * time.Millisecond,
		MaxRetries:    1,
		OnTimeout: func(messageID, deviceID, topic string) {
			timeoutCalled <- messageID
		},
	})
	defer tracker.Stop()

	msg := types.WebSocketMessage{
		Type: types.WSMsgTypeHealthChange,
		Data: map[string]string{"status": "healthy"},
	}

	messageID := tracker.RegisterQoS1("device-1", "health_change", msg)

	// Wait for timeout
	select {
	case timedOutID := <-timeoutCalled:
		if timedOutID != messageID {
			t.Errorf("Expected timeout for %s, got %s", messageID, timedOutID)
		}
	case <-time.After(200 * time.Millisecond):
		t.Error("Timeout callback not called")
	}
}

func TestQoSTracker_Retry(t *testing.T) {
	retryCalled := make(chan string, 5)
	var retryCount int32

	tracker := NewQoSTracker(QoSTrackerConfig{
		ACKTimeout:    30 * time.Millisecond,
		CheckInterval: 10 * time.Millisecond,
		MaxRetries:    3,
		OnRetry: func(messageID, deviceID, topic string, msg types.WebSocketMessage, attempt int) {
			atomic.AddInt32(&retryCount, 1)
			retryCalled <- messageID
		},
	})
	defer tracker.Stop()

	msg := types.WebSocketMessage{
		Type: types.WSMsgTypeHealthChange,
		Data: map[string]string{"status": "healthy"},
	}

	tracker.RegisterQoS1("device-1", "health_change", msg)

	// Wait for retries
	time.Sleep(150 * time.Millisecond)

	// Should have retried up to MaxRetries times
	if atomic.LoadInt32(&retryCount) < 1 {
		t.Error("Expected at least one retry")
	}
}

func TestQoSTracker_Cancel(t *testing.T) {
	tracker := NewQoSTracker(QoSTrackerConfig{
		ACKTimeout:    5 * time.Second,
		CheckInterval: 1 * time.Second,
		MaxRetries:    3,
	})
	defer tracker.Stop()

	msg := types.WebSocketMessage{
		Type: types.WSMsgTypeHealthChange,
	}

	messageID := tracker.RegisterQoS1("device-1", "health_change", msg)

	// Cancel
	tracker.Cancel(messageID)

	// Should no longer be pending
	if count := tracker.PendingCount(); count != 0 {
		t.Errorf("Expected 0 pending after cancel, got %d", count)
	}
}

func TestQoSTracker_CancelForDevice(t *testing.T) {
	tracker := NewQoSTracker(QoSTrackerConfig{
		ACKTimeout:    5 * time.Second,
		CheckInterval: 1 * time.Second,
		MaxRetries:    3,
	})
	defer tracker.Stop()

	msg := types.WebSocketMessage{Type: types.WSMsgTypeHealthChange}

	// Register messages for two devices
	tracker.RegisterQoS1("device-1", "topic1", msg)
	tracker.RegisterQoS1("device-1", "topic2", msg)
	tracker.RegisterQoS1("device-2", "topic1", msg)

	if count := tracker.PendingCount(); count != 3 {
		t.Errorf("Expected 3 pending, got %d", count)
	}

	// Cancel for device-1
	cancelled := tracker.CancelForDevice("device-1")
	if cancelled != 2 {
		t.Errorf("Expected 2 cancelled, got %d", cancelled)
	}

	// Only device-2 message should remain
	if count := tracker.PendingCount(); count != 1 {
		t.Errorf("Expected 1 pending after cancel, got %d", count)
	}
}

func TestQoS2Tracker_Register(t *testing.T) {
	tracker := NewQoS2Tracker(QoS2TrackerConfig{
		DedupWindow: 1 * time.Minute,
	})
	defer tracker.Stop()

	messageID := "msg-123"
	deviceID := "device-1"

	// First registration should succeed
	if !tracker.Register(messageID, deviceID) {
		t.Error("First registration should succeed")
	}

	// Duplicate registration should fail
	if tracker.Register(messageID, deviceID) {
		t.Error("Duplicate registration should fail")
	}
}

func TestQoS2Tracker_IsProcessed(t *testing.T) {
	tracker := NewQoS2Tracker(QoS2TrackerConfig{
		DedupWindow: 1 * time.Minute,
	})
	defer tracker.Stop()

	messageID := "msg-123"
	deviceID := "device-1"

	// Should not be processed initially
	if tracker.IsProcessed(messageID, deviceID) {
		t.Error("Message should not be processed initially")
	}

	// Register it
	tracker.Register(messageID, deviceID)

	// Should now be processed
	if !tracker.IsProcessed(messageID, deviceID) {
		t.Error("Message should be processed after registration")
	}
}

func TestQoS2Tracker_Expiry(t *testing.T) {
	tracker := NewQoS2Tracker(QoS2TrackerConfig{
		DedupWindow:   50 * time.Millisecond,
		CheckInterval: 10 * time.Millisecond,
	})
	defer tracker.Stop()

	messageID := "msg-123"
	deviceID := "device-1"

	tracker.Register(messageID, deviceID)

	// Wait for expiry
	time.Sleep(100 * time.Millisecond)

	// Should no longer be processed (expired)
	if tracker.IsProcessed(messageID, deviceID) {
		t.Error("Message should have expired")
	}

	// Should be able to register again
	if !tracker.Register(messageID, deviceID) {
		t.Error("Should be able to re-register after expiry")
	}
}

func TestQoS2Tracker_Complete(t *testing.T) {
	tracker := NewQoS2Tracker(QoS2TrackerConfig{
		DedupWindow: 1 * time.Minute,
	})
	defer tracker.Stop()

	messageID := "msg-123"
	deviceID := "device-1"

	tracker.Register(messageID, deviceID)

	// Complete the message
	tracker.Complete(messageID, deviceID)

	// Should still be in dedup table
	if !tracker.IsProcessed(messageID, deviceID) {
		t.Error("Message should still be marked as processed after complete")
	}
}

func BenchmarkQoSTracker_RegisterACK(b *testing.B) {
	tracker := NewQoSTracker(QoSTrackerConfig{
		ACKTimeout:    5 * time.Second,
		CheckInterval: 1 * time.Second,
		MaxRetries:    3,
	})
	defer tracker.Stop()

	msg := types.WebSocketMessage{Type: types.WSMsgTypeHealthChange}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		messageID := tracker.RegisterQoS1("device-1", "topic", msg)
		tracker.ACK(messageID)
	}
}

func BenchmarkQoS2Tracker_RegisterComplete(b *testing.B) {
	tracker := NewQoS2Tracker(QoS2TrackerConfig{
		DedupWindow: 1 * time.Minute,
	})
	defer tracker.Stop()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		messageID := "msg-" + string(rune(i))
		tracker.Register(messageID, "device-1")
		tracker.Complete(messageID, "device-1")
	}
}
