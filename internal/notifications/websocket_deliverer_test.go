package notifications

import (
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"
)

// MockWebSocketSender implements WebSocketSender for testing
type MockWebSocketSender struct {
	connected    map[string]bool
	channelFull  map[string]bool
	sentMessages []sentMessage
}

type sentMessage struct {
	deviceID string
	msgType  string
	payload  json.RawMessage
	notifID  string
}

func NewMockWebSocketSender() *MockWebSocketSender {
	return &MockWebSocketSender{
		connected:    make(map[string]bool),
		channelFull:  make(map[string]bool),
		sentMessages: make([]sentMessage, 0),
	}
}

func (m *MockWebSocketSender) HasDeviceConnection(deviceID string) bool {
	return m.connected[deviceID]
}

func (m *MockWebSocketSender) SendToDevice(deviceID string, msgType string, payload json.RawMessage) error {
	if !m.connected[deviceID] {
		return fmt.Errorf("device %s is not connected via WebSocket", deviceID)
	}
	if m.channelFull[deviceID] {
		return fmt.Errorf("send channel full for device %s", deviceID)
	}
	m.sentMessages = append(m.sentMessages, sentMessage{
		deviceID: deviceID,
		msgType:  msgType,
		payload:  payload,
	})
	return nil
}

func (m *MockWebSocketSender) SendNotification(deviceID string, notifID string, msgType string, payload json.RawMessage) error {
	if !m.connected[deviceID] {
		return fmt.Errorf("device %s is not connected via WebSocket", deviceID)
	}
	if m.channelFull[deviceID] {
		return fmt.Errorf("send channel full for device %s", deviceID)
	}
	m.sentMessages = append(m.sentMessages, sentMessage{
		deviceID: deviceID,
		msgType:  msgType,
		payload:  payload,
		notifID:  notifID,
	})
	return nil
}

func (m *MockWebSocketSender) SetConnected(deviceID string, connected bool) {
	m.connected[deviceID] = connected
}

func (m *MockWebSocketSender) SetChannelFull(deviceID string, full bool) {
	m.channelFull[deviceID] = full
}

func (m *MockWebSocketSender) GetSentMessages() []sentMessage {
	return m.sentMessages
}

// BroadcastFiltered is kept for interface compatibility but not used
func (m *MockWebSocketSender) BroadcastFiltered(msg interface{}, filter func(client WebSocketClient) bool) {
	// Not used in new implementation
}

func TestWebSocketDeliverer_DeliverySuccess(t *testing.T) {
	sender := NewMockWebSocketSender()
	sender.SetConnected("device-1", true)

	deliverer := NewWebSocketDeliverer(sender)

	payload := json.RawMessage(`{"event":"test"}`)
	err := deliverer.DeliverNotification("device-1", "health_change", payload, 0)

	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}

	sent := sender.GetSentMessages()
	if len(sent) != 1 {
		t.Fatalf("expected 1 sent message, got %d", len(sent))
	}

	if sent[0].deviceID != "device-1" {
		t.Errorf("expected deviceID=device-1, got %s", sent[0].deviceID)
	}
	if sent[0].msgType != "health_change" {
		t.Errorf("expected msgType=health_change, got %s", sent[0].msgType)
	}
}

func TestWebSocketDeliverer_DeviceOffline(t *testing.T) {
	sender := NewMockWebSocketSender()
	sender.SetConnected("device-1", false)

	deliverer := NewWebSocketDeliverer(sender)

	payload := json.RawMessage(`{"event":"test"}`)
	err := deliverer.DeliverNotification("device-1", "health_change", payload, 0)

	if err == nil {
		t.Error("expected error for offline device, got nil")
	}

	sent := sender.GetSentMessages()
	if len(sent) != 0 {
		t.Errorf("expected no sent messages for offline device, got %d", len(sent))
	}
}

func TestWebSocketDeliverer_ChannelFull_ReturnsError(t *testing.T) {
	sender := NewMockWebSocketSender()
	sender.SetConnected("device-1", true)
	sender.SetChannelFull("device-1", true)

	deliverer := NewWebSocketDeliverer(sender)

	payload := json.RawMessage(`{"event":"test"}`)
	err := deliverer.DeliverNotification("device-1", "health_change", payload, 0)

	// This is the key test: when channel is full, we should get an error
	// so that the notification queue can persist and retry
	if err == nil {
		t.Error("expected error when send channel is full, got nil")
	}

	sent := sender.GetSentMessages()
	if len(sent) != 0 {
		t.Errorf("expected no sent messages when channel full, got %d", len(sent))
	}
}

func TestWebSocketDeliverer_MultipleDeliveries(t *testing.T) {
	sender := NewMockWebSocketSender()
	sender.SetConnected("device-1", true)
	sender.SetConnected("device-2", true)

	deliverer := NewWebSocketDeliverer(sender)

	// Deliver to device-1
	payload1 := json.RawMessage(`{"msg":"one"}`)
	err := deliverer.DeliverNotification("device-1", "type-a", payload1, 0)
	if err != nil {
		t.Errorf("delivery to device-1 failed: %v", err)
	}

	// Deliver to device-2
	payload2 := json.RawMessage(`{"msg":"two"}`)
	err = deliverer.DeliverNotification("device-2", "type-b", payload2, 0)
	if err != nil {
		t.Errorf("delivery to device-2 failed: %v", err)
	}

	sent := sender.GetSentMessages()
	if len(sent) != 2 {
		t.Fatalf("expected 2 sent messages, got %d", len(sent))
	}
}

func TestWebSocketDeliverer_WithACKTracker_RegistersNotification(t *testing.T) {
	sender := NewMockWebSocketSender()
	sender.SetConnected("device-1", true)

	ackTracker := NewACKTracker(ACKTrackerConfig{
		ACKTimeout:    5 * time.Second,
		CheckInterval: 100 * time.Millisecond,
	})
	defer ackTracker.Stop()

	deliverer := NewWebSocketDelivererWithACK(sender, ackTracker)

	payload := json.RawMessage(`{"event":"test"}`)
	err := deliverer.DeliverNotification("device-1", "health_change", payload, 0)

	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}

	// Verify notification was registered with ACK tracker
	if ackTracker.GetPendingCount() != 1 {
		t.Errorf("expected 1 pending notification, got %d", ackTracker.GetPendingCount())
	}

	// Verify message was sent with notification ID
	sent := sender.GetSentMessages()
	if len(sent) != 1 {
		t.Fatalf("expected 1 sent message, got %d", len(sent))
	}

	if sent[0].notifID == "" {
		t.Error("expected notification ID to be set")
	}
}

func TestWebSocketDeliverer_WithACKTracker_CancelsOnSendFailure(t *testing.T) {
	sender := NewMockWebSocketSender()
	sender.SetConnected("device-1", true)
	sender.SetChannelFull("device-1", true)

	ackTracker := NewACKTracker(ACKTrackerConfig{
		ACKTimeout:    5 * time.Second,
		CheckInterval: 100 * time.Millisecond,
	})
	defer ackTracker.Stop()

	deliverer := NewWebSocketDelivererWithACK(sender, ackTracker)

	payload := json.RawMessage(`{"event":"test"}`)
	err := deliverer.DeliverNotification("device-1", "health_change", payload, 0)

	if err == nil {
		t.Error("expected error when channel is full")
	}

	// Verify notification was cancelled from ACK tracker
	if ackTracker.GetPendingCount() != 0 {
		t.Errorf("expected 0 pending notifications after failure, got %d", ackTracker.GetPendingCount())
	}
}

func TestWebSocketDeliverer_WithACKTracker_ACKRemovesPending(t *testing.T) {
	sender := NewMockWebSocketSender()
	sender.SetConnected("device-1", true)

	ackTracker := NewACKTracker(ACKTrackerConfig{
		ACKTimeout:    5 * time.Second,
		CheckInterval: 100 * time.Millisecond,
	})
	defer ackTracker.Stop()

	deliverer := NewWebSocketDelivererWithACK(sender, ackTracker)

	payload := json.RawMessage(`{"event":"test"}`)
	err := deliverer.DeliverNotification("device-1", "health_change", payload, 0)
	if err != nil {
		t.Fatalf("delivery failed: %v", err)
	}

	// Get the notification ID from sent message
	sent := sender.GetSentMessages()
	notifID := sent[0].notifID

	// Verify pending
	if ackTracker.GetPendingCount() != 1 {
		t.Errorf("expected 1 pending, got %d", ackTracker.GetPendingCount())
	}

	// Simulate client ACK
	ackTracker.ACK(notifID)

	// Verify no longer pending
	if ackTracker.GetPendingCount() != 0 {
		t.Errorf("expected 0 pending after ACK, got %d", ackTracker.GetPendingCount())
	}
}

func TestWebSocketDeliverer_WithACKTracker_TimeoutTriggersCallback(t *testing.T) {
	sender := NewMockWebSocketSender()
	sender.SetConnected("device-1", true)

	var mu sync.Mutex
	var timeoutCalled bool
	var timeoutDeviceID string

	ackTracker := NewACKTracker(ACKTrackerConfig{
		ACKTimeout:    200 * time.Millisecond,
		CheckInterval: 50 * time.Millisecond,
		OnTimeout: func(notifID, deviceID, msgType string, payload json.RawMessage) {
			mu.Lock()
			timeoutCalled = true
			timeoutDeviceID = deviceID
			mu.Unlock()
		},
	})
	defer ackTracker.Stop()

	deliverer := NewWebSocketDelivererWithACK(sender, ackTracker)

	payload := json.RawMessage(`{"event":"test"}`)
	err := deliverer.DeliverNotification("device-1", "health_change", payload, 0)
	if err != nil {
		t.Fatalf("delivery failed: %v", err)
	}

	// Wait for timeout
	time.Sleep(400 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if !timeoutCalled {
		t.Error("expected timeout callback to be called")
	}

	if timeoutDeviceID != "device-1" {
		t.Errorf("expected device-1, got %s", timeoutDeviceID)
	}
}
