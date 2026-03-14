package scripts

import (
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/nstalgic/nekzus/internal/types"
)

// mockWebSocketSender implements the WebSocketSender interface for testing
type mockWebSocketSender struct {
	mu         sync.Mutex
	sentMsgs   []sentMessage
	onlineDevs map[string]bool
	sendError  error
}

type sentMessage struct {
	DeviceID string
	MsgType  string
	Payload  json.RawMessage
}

func newMockWebSocketSender() *mockWebSocketSender {
	return &mockWebSocketSender{
		sentMsgs:   make([]sentMessage, 0),
		onlineDevs: make(map[string]bool),
	}
}

func (m *mockWebSocketSender) SendToDevice(deviceID string, msgType string, payload json.RawMessage) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.sendError != nil {
		return m.sendError
	}

	// Check if device is online
	if !m.onlineDevs[deviceID] {
		return errors.New("device offline")
	}

	m.sentMsgs = append(m.sentMsgs, sentMessage{
		DeviceID: deviceID,
		MsgType:  msgType,
		Payload:  payload,
	})
	return nil
}

func (m *mockWebSocketSender) Broadcast(msgType string, payload json.RawMessage) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Record broadcast as a message to "broadcast" device
	m.sentMsgs = append(m.sentMsgs, sentMessage{
		DeviceID: "broadcast",
		MsgType:  msgType,
		Payload:  payload,
	})
}

func (m *mockWebSocketSender) setOnline(deviceID string, online bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onlineDevs[deviceID] = online
}

func (m *mockWebSocketSender) getSentMessages() []sentMessage {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]sentMessage, len(m.sentMsgs))
	copy(result, m.sentMsgs)
	return result
}

// mockNotificationQueue implements the NotificationQueue interface for testing
type mockNotificationQueue struct {
	mu       sync.Mutex
	enqueued []queuedNotification
	enqErr   error
}

type queuedNotification struct {
	DeviceID   string
	MsgType    string
	Payload    json.RawMessage
	TTL        time.Duration
	MaxRetries int
}

func newMockNotificationQueue() *mockNotificationQueue {
	return &mockNotificationQueue{
		enqueued: make([]queuedNotification, 0),
	}
}

func (m *mockNotificationQueue) Enqueue(deviceID string, msgType string, payload json.RawMessage, ttl time.Duration, maxRetries int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.enqErr != nil {
		return m.enqErr
	}

	m.enqueued = append(m.enqueued, queuedNotification{
		DeviceID:   deviceID,
		MsgType:    msgType,
		Payload:    payload,
		TTL:        ttl,
		MaxRetries: maxRetries,
	})
	return nil
}

func (m *mockNotificationQueue) getEnqueued() []queuedNotification {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]queuedNotification, len(m.enqueued))
	copy(result, m.enqueued)
	return result
}

// TestNotifier_SendsToOnlineDevice tests that notifications are sent via WebSocket
// when the device is online.
func TestNotifier_SendsToOnlineDevice(t *testing.T) {
	ws := newMockWebSocketSender()
	queue := newMockNotificationQueue()

	ws.setOnline("device-123", true)

	notifier := NewWebSocketNotifier(ws, queue, NotifierConfig{
		TTL:        24 * time.Hour,
		MaxRetries: 3,
	})

	execution := &Execution{
		ID:       "exec-123",
		ScriptID: "script-abc",
		Status:   ExecutionStatusCompleted,
		Output:   "Script output here",
		ExitCode: intPtr(0),
	}

	notifier.NotifyExecutionCompleted("device-123", execution)

	// Should have sent via WebSocket
	msgs := ws.getSentMessages()
	if len(msgs) != 1 {
		t.Errorf("expected 1 WebSocket message, got %d", len(msgs))
	}

	if len(msgs) > 0 {
		if msgs[0].DeviceID != "device-123" {
			t.Errorf("expected device-123, got %s", msgs[0].DeviceID)
		}
		if msgs[0].MsgType != types.WSMsgTypeExecutionCompleted {
			t.Errorf("expected %s, got %s", types.WSMsgTypeExecutionCompleted, msgs[0].MsgType)
		}
	}

	// Should NOT have queued (device was online)
	queued := queue.getEnqueued()
	if len(queued) != 0 {
		t.Errorf("expected 0 queued notifications, got %d", len(queued))
	}
}

// TestNotifier_QueuesForOfflineDevice tests that notifications are queued
// when the device is offline.
func TestNotifier_QueuesForOfflineDevice(t *testing.T) {
	ws := newMockWebSocketSender()
	queue := newMockNotificationQueue()

	// Device is offline
	ws.setOnline("device-offline", false)

	notifier := NewWebSocketNotifier(ws, queue, NotifierConfig{
		TTL:        24 * time.Hour,
		MaxRetries: 3,
	})

	execution := &Execution{
		ID:       "exec-456",
		ScriptID: "script-xyz",
		Status:   ExecutionStatusCompleted,
		Output:   "Output",
		ExitCode: intPtr(0),
	}

	notifier.NotifyExecutionCompleted("device-offline", execution)

	// Should have queued for later delivery
	queued := queue.getEnqueued()
	if len(queued) != 1 {
		t.Errorf("expected 1 queued notification, got %d", len(queued))
	}

	if len(queued) > 0 {
		if queued[0].DeviceID != "device-offline" {
			t.Errorf("expected device-offline, got %s", queued[0].DeviceID)
		}
		if queued[0].MsgType != types.WSMsgTypeExecutionCompleted {
			t.Errorf("expected %s, got %s", types.WSMsgTypeExecutionCompleted, queued[0].MsgType)
		}
		if queued[0].TTL != 24*time.Hour {
			t.Errorf("expected TTL 24h, got %v", queued[0].TTL)
		}
		if queued[0].MaxRetries != 3 {
			t.Errorf("expected maxRetries 3, got %d", queued[0].MaxRetries)
		}
	}
}

// TestNotifier_NotifyExecutionStarted tests the started notification.
func TestNotifier_NotifyExecutionStarted(t *testing.T) {
	ws := newMockWebSocketSender()
	queue := newMockNotificationQueue()

	ws.setOnline("device-start", true)

	notifier := NewWebSocketNotifier(ws, queue, NotifierConfig{
		TTL:        24 * time.Hour,
		MaxRetries: 3,
	})

	notifier.NotifyExecutionStarted("device-start", "exec-789", "script-abc", "My Script")

	msgs := ws.getSentMessages()
	if len(msgs) != 1 {
		t.Errorf("expected 1 message, got %d", len(msgs))
	}

	if len(msgs) > 0 {
		if msgs[0].MsgType != types.WSMsgTypeExecutionStarted {
			t.Errorf("expected %s, got %s", types.WSMsgTypeExecutionStarted, msgs[0].MsgType)
		}

		// Parse payload and verify content
		var payload map[string]interface{}
		if err := json.Unmarshal(msgs[0].Payload, &payload); err != nil {
			t.Fatalf("failed to parse payload: %v", err)
		}

		if payload["executionId"] != "exec-789" {
			t.Errorf("expected executionId exec-789, got %v", payload["executionId"])
		}
		if payload["scriptId"] != "script-abc" {
			t.Errorf("expected scriptId script-abc, got %v", payload["scriptId"])
		}
		if payload["scriptName"] != "My Script" {
			t.Errorf("expected scriptName 'My Script', got %v", payload["scriptName"])
		}
		if payload["status"] != "running" {
			t.Errorf("expected status running, got %v", payload["status"])
		}
	}
}

// TestNotifier_NotifyExecutionFailed tests the failed notification.
func TestNotifier_NotifyExecutionFailed(t *testing.T) {
	ws := newMockWebSocketSender()
	queue := newMockNotificationQueue()

	ws.setOnline("device-fail", true)

	notifier := NewWebSocketNotifier(ws, queue, NotifierConfig{
		TTL:        24 * time.Hour,
		MaxRetries: 3,
	})

	execution := &Execution{
		ID:           "exec-fail",
		ScriptID:     "script-xyz",
		Status:       ExecutionStatusFailed,
		Output:       "Error output",
		ExitCode:     intPtr(1),
		ErrorMessage: "Script failed",
	}

	notifier.NotifyExecutionFailed("device-fail", execution, "Script exited with code 1")

	msgs := ws.getSentMessages()
	if len(msgs) != 1 {
		t.Errorf("expected 1 message, got %d", len(msgs))
	}

	if len(msgs) > 0 {
		if msgs[0].MsgType != types.WSMsgTypeExecutionFailed {
			t.Errorf("expected %s, got %s", types.WSMsgTypeExecutionFailed, msgs[0].MsgType)
		}

		var payload map[string]interface{}
		if err := json.Unmarshal(msgs[0].Payload, &payload); err != nil {
			t.Fatalf("failed to parse payload: %v", err)
		}

		if payload["status"] != "failed" {
			t.Errorf("expected status failed, got %v", payload["status"])
		}
		if payload["errorMessage"] != "Script exited with code 1" {
			t.Errorf("expected errorMessage, got %v", payload["errorMessage"])
		}
	}
}

// TestNotifier_NotifyExecutionTimeout tests the timeout notification.
func TestNotifier_NotifyExecutionTimeout(t *testing.T) {
	ws := newMockWebSocketSender()
	queue := newMockNotificationQueue()

	ws.setOnline("device-timeout", true)

	notifier := NewWebSocketNotifier(ws, queue, NotifierConfig{
		TTL:        24 * time.Hour,
		MaxRetries: 3,
	})

	execution := &Execution{
		ID:       "exec-timeout",
		ScriptID: "script-slow",
		Status:   ExecutionStatusTimeout,
		Output:   "Partial output...",
	}

	notifier.NotifyExecutionFailed("device-timeout", execution, "Script timed out after 30s")

	msgs := ws.getSentMessages()
	if len(msgs) != 1 {
		t.Errorf("expected 1 message, got %d", len(msgs))
	}

	if len(msgs) > 0 {
		// Timeout is sent as failed notification
		if msgs[0].MsgType != types.WSMsgTypeExecutionFailed {
			t.Errorf("expected %s, got %s", types.WSMsgTypeExecutionFailed, msgs[0].MsgType)
		}
	}
}

// TestNotifier_PayloadTruncation tests that large output is truncated in notifications.
func TestNotifier_PayloadTruncation(t *testing.T) {
	ws := newMockWebSocketSender()
	queue := newMockNotificationQueue()

	ws.setOnline("device-large", true)

	notifier := NewWebSocketNotifier(ws, queue, NotifierConfig{
		TTL:              24 * time.Hour,
		MaxRetries:       3,
		MaxOutputInNotif: 1000, // Limit output to 1000 bytes in notification
	})

	// Create execution with large output
	largeOutput := make([]byte, 5000)
	for i := range largeOutput {
		largeOutput[i] = 'A'
	}

	execution := &Execution{
		ID:       "exec-large",
		ScriptID: "script-big",
		Status:   ExecutionStatusCompleted,
		Output:   string(largeOutput),
		ExitCode: intPtr(0),
	}

	notifier.NotifyExecutionCompleted("device-large", execution)

	msgs := ws.getSentMessages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(msgs[0].Payload, &payload); err != nil {
		t.Fatalf("failed to parse payload: %v", err)
	}

	output := payload["output"].(string)
	if len(output) > 1100 { // Allow some overhead for truncation message
		t.Errorf("expected output to be truncated, got %d bytes", len(output))
	}
}

// TestNotifier_NilQueue tests that notifier works without a queue (no offline fallback).
func TestNotifier_NilQueue(t *testing.T) {
	ws := newMockWebSocketSender()

	// Device is offline, but no queue configured
	ws.setOnline("device-no-queue", false)

	notifier := NewWebSocketNotifier(ws, nil, NotifierConfig{
		TTL:        24 * time.Hour,
		MaxRetries: 3,
	})

	execution := &Execution{
		ID:       "exec-no-queue",
		ScriptID: "script-xyz",
		Status:   ExecutionStatusCompleted,
		Output:   "Output",
		ExitCode: intPtr(0),
	}

	// Should not panic
	notifier.NotifyExecutionCompleted("device-no-queue", execution)

	// Should have attempted WebSocket (and failed silently)
	// No queue to check since it's nil
}

// TestNotifier_EmptyTriggeredBy tests handling when triggered by is empty.
func TestNotifier_EmptyTriggeredBy(t *testing.T) {
	ws := newMockWebSocketSender()
	queue := newMockNotificationQueue()

	notifier := NewWebSocketNotifier(ws, queue, NotifierConfig{
		TTL:        24 * time.Hour,
		MaxRetries: 3,
	})

	execution := &Execution{
		ID:       "exec-empty",
		ScriptID: "script-xyz",
		Status:   ExecutionStatusCompleted,
		Output:   "Output",
		ExitCode: intPtr(0),
	}

	// Empty device ID - should not send or queue
	notifier.NotifyExecutionCompleted("", execution)

	msgs := ws.getSentMessages()
	if len(msgs) != 0 {
		t.Errorf("expected 0 messages for empty device ID, got %d", len(msgs))
	}

	queued := queue.getEnqueued()
	if len(queued) != 0 {
		t.Errorf("expected 0 queued for empty device ID, got %d", len(queued))
	}
}

// TestNotifier_SchedulerTriggered tests notifications for scheduler-triggered executions.
func TestNotifier_SchedulerTriggered(t *testing.T) {
	ws := newMockWebSocketSender()
	queue := newMockNotificationQueue()

	notifier := NewWebSocketNotifier(ws, queue, NotifierConfig{
		TTL:        24 * time.Hour,
		MaxRetries: 3,
	})

	execution := &Execution{
		ID:       "exec-sched",
		ScriptID: "script-cron",
		Status:   ExecutionStatusCompleted,
		Output:   "Output",
		ExitCode: intPtr(0),
	}

	// Scheduler-triggered - typically broadcasts to all or skips notification
	notifier.NotifyExecutionCompleted("scheduler", execution)

	// Behavior depends on implementation - either broadcast or skip
	// For now, we expect no direct device notification for "scheduler"
	msgs := ws.getSentMessages()
	queued := queue.getEnqueued()

	// Both should be empty for scheduler (or implementation broadcasts)
	t.Logf("scheduler notification: %d WS messages, %d queued", len(msgs), len(queued))
}

// intPtr returns a pointer to an int
func intPtr(i int) *int {
	return &i
}
