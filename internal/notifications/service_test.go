package notifications

import (
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/nstalgic/nekzus/internal/types"
)

// mockQueue implements a minimal queue interface for testing
type mockQueue struct {
	mu            sync.Mutex
	notifications []enqueueCall
}

type enqueueCall struct {
	DeviceID   string
	MsgType    string
	Payload    json.RawMessage
	TTL        time.Duration
	MaxRetries int
}

func (m *mockQueue) Enqueue(deviceID string, msgType string, payload json.RawMessage, ttl time.Duration, maxRetries int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.notifications = append(m.notifications, enqueueCall{
		DeviceID:   deviceID,
		MsgType:    msgType,
		Payload:    payload,
		TTL:        ttl,
		MaxRetries: maxRetries,
	})
	return nil
}

func (m *mockQueue) getNotifications() []enqueueCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.notifications
}

// mockDeviceStorage implements device listing for testing
type mockDeviceStorage struct {
	devices []DeviceInfo
}

func (m *mockDeviceStorage) ListDevices() ([]DeviceInfo, error) {
	return m.devices, nil
}

func TestSend_EnqueuesNotification(t *testing.T) {
	queue := &mockQueue{}
	storage := &mockDeviceStorage{}
	config := DefaultServiceConfig()

	service := NewService(queue, storage, config)

	data := map[string]string{"message": "test notification"}
	err := service.Send("device-123", types.WSMsgTypeHealthChange, data)
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	notifications := queue.getNotifications()
	if len(notifications) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(notifications))
	}

	notif := notifications[0]
	if notif.DeviceID != "device-123" {
		t.Errorf("DeviceID = %q, want %q", notif.DeviceID, "device-123")
	}
	if notif.MsgType != types.WSMsgTypeHealthChange {
		t.Errorf("MsgType = %q, want %q", notif.MsgType, types.WSMsgTypeHealthChange)
	}
}

func TestSend_UsesCorrectTTLForType(t *testing.T) {
	tests := []struct {
		name        string
		msgType     string
		wantTTL     time.Duration
		wantRetries int
	}{
		{
			name:        "repair_required uses 30 days TTL",
			msgType:     types.WSMsgTypeRepairRequired,
			wantTTL:     30 * 24 * time.Hour,
			wantRetries: 20,
		},
		{
			name:        "device_revoked uses 30 days TTL",
			msgType:     types.WSMsgTypeDeviceRevoked,
			wantTTL:     30 * 24 * time.Hour,
			wantRetries: 10,
		},
		{
			name:        "health_change uses 7 days TTL",
			msgType:     types.WSMsgTypeHealthChange,
			wantTTL:     7 * 24 * time.Hour,
			wantRetries: 5,
		},
		{
			name:        "unknown type uses default TTL",
			msgType:     "unknown_type",
			wantTTL:     30 * 24 * time.Hour,
			wantRetries: 10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			queue := &mockQueue{}
			storage := &mockDeviceStorage{}
			config := DefaultServiceConfig()

			service := NewService(queue, storage, config)

			data := map[string]string{"test": "data"}
			err := service.Send("device-123", tt.msgType, data)
			if err != nil {
				t.Fatalf("Send() error = %v", err)
			}

			notifications := queue.getNotifications()
			if len(notifications) != 1 {
				t.Fatalf("expected 1 notification, got %d", len(notifications))
			}

			notif := notifications[0]
			if notif.TTL != tt.wantTTL {
				t.Errorf("TTL = %v, want %v", notif.TTL, tt.wantTTL)
			}
			if notif.MaxRetries != tt.wantRetries {
				t.Errorf("MaxRetries = %d, want %d", notif.MaxRetries, tt.wantRetries)
			}
		})
	}
}

func TestSendToAll_EnqueuesForEachDevice(t *testing.T) {
	queue := &mockQueue{}
	deviceStorage := &mockDeviceStorage{
		devices: []DeviceInfo{
			{ID: "device-1", Name: "Device 1"},
			{ID: "device-2", Name: "Device 2"},
			{ID: "device-3", Name: "Device 3"},
		},
	}
	config := DefaultServiceConfig()

	service := NewService(queue, deviceStorage, config)

	data := map[string]string{"reason": "tls_enabled"}
	err := service.SendToAll(types.WSMsgTypeRepairRequired, data)
	if err != nil {
		t.Fatalf("SendToAll() error = %v", err)
	}

	notifications := queue.getNotifications()
	if len(notifications) != 3 {
		t.Fatalf("expected 3 notifications, got %d", len(notifications))
	}

	// Verify each device got a notification
	deviceIDs := make(map[string]bool)
	for _, notif := range notifications {
		deviceIDs[notif.DeviceID] = true
		if notif.MsgType != types.WSMsgTypeRepairRequired {
			t.Errorf("MsgType = %q, want %q", notif.MsgType, types.WSMsgTypeRepairRequired)
		}
	}

	for _, expectedID := range []string{"device-1", "device-2", "device-3"} {
		if !deviceIDs[expectedID] {
			t.Errorf("device %q did not receive notification", expectedID)
		}
	}
}

func TestSendToAll_EmptyDeviceList(t *testing.T) {
	queue := &mockQueue{}
	deviceStorage := &mockDeviceStorage{
		devices: []DeviceInfo{},
	}
	config := DefaultServiceConfig()

	service := NewService(queue, deviceStorage, config)

	data := map[string]string{"test": "data"}
	err := service.SendToAll(types.WSMsgTypeRepairRequired, data)
	if err != nil {
		t.Fatalf("SendToAll() error = %v", err)
	}

	notifications := queue.getNotifications()
	if len(notifications) != 0 {
		t.Errorf("expected 0 notifications for empty device list, got %d", len(notifications))
	}
}

func TestSend_PayloadIsSerialized(t *testing.T) {
	queue := &mockQueue{}
	deviceStorage := &mockDeviceStorage{}
	config := DefaultServiceConfig()

	service := NewService(queue, deviceStorage, config)

	data := map[string]interface{}{
		"reason":     "tls_enabled",
		"newBaseUrl": "https://nexus.local:8443",
		"timestamp":  1234567890,
	}

	err := service.Send("device-123", types.WSMsgTypeRepairRequired, data)
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	notifications := queue.getNotifications()
	if len(notifications) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(notifications))
	}

	// Verify payload is valid JSON
	var payload map[string]interface{}
	if err := json.Unmarshal(notifications[0].Payload, &payload); err != nil {
		t.Fatalf("failed to unmarshal payload: %v", err)
	}

	if payload["reason"] != "tls_enabled" {
		t.Errorf("payload.reason = %v, want %v", payload["reason"], "tls_enabled")
	}
}
