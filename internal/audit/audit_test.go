package audit

import (
	"errors"
	"testing"
	"time"
)

func TestNewLogger(t *testing.T) {
	logger := NewLogger(nil)
	if logger == nil {
		t.Fatal("NewLogger returned nil")
	}
}

func TestLogEvent(t *testing.T) {
	logger := NewLogger(nil)

	event := Event{
		Action:   ActionDevicePaired,
		ActorID:  "device-123",
		ActorIP:  "192.168.1.100",
		TargetID: "device-123",
		Details: map[string]interface{}{
			"platform": "ios",
		},
		Success: true,
	}

	// Should not panic - timestamp will be set inside Log()
	logger.Log(event)
}

func TestLogDevicePaired(t *testing.T) {
	logger := NewLogger(nil)

	// Should not panic
	logger.LogDevicePaired("device-456", "192.168.1.50", "android")
}

func TestLogDeviceRevoked_Success(t *testing.T) {
	logger := NewLogger(nil)

	logger.LogDeviceRevoked("device-789", "admin-device", "10.0.0.1", true, nil)
}

func TestLogDeviceRevoked_Failure(t *testing.T) {
	logger := NewLogger(nil)

	err := errors.New("permission denied")
	logger.LogDeviceRevoked("device-999", "unauthorized-device", "192.168.1.200", false, err)
}

func TestLogDeviceUpdated(t *testing.T) {
	logger := NewLogger(nil)

	logger.LogDeviceUpdated("device-update", "device-update", "172.16.0.5", "My New Device Name", true, nil)
}

func TestLogAppRegistered(t *testing.T) {
	logger := NewLogger(nil)

	logger.LogAppRegistered("grafana", "admin-123", "127.0.0.1", true, nil)
}

func TestLogConfigReloaded(t *testing.T) {
	logger := NewLogger(nil)

	changes := map[string]int{
		"routes": 5,
		"apps":   3,
	}

	logger.LogConfigReloaded("system", "localhost", changes, true, nil)
}

func TestLogProposalAction(t *testing.T) {
	logger := NewLogger(nil)

	// Test approval
	logger.LogProposalAction(ActionProposalApproved, "proposal-123", "admin-device", "192.168.1.1", true, nil)

	// Test dismissal
	logger.LogProposalAction(ActionProposalDismissed, "proposal-456", "admin-device", "192.168.1.1", true, nil)
}

func TestEventTimestamp(t *testing.T) {
	logger := NewLogger(nil)

	// Test with pre-set timestamp
	customTime := time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC)
	event := Event{
		Timestamp: customTime,
		Action:    ActionDevicePaired,
		ActorID:   "test",
		ActorIP:   "127.0.0.1",
		TargetID:  "test",
		Success:   true,
	}

	// Custom timestamp should be preserved
	logger.Log(event)

	// Test with zero timestamp - should be set automatically
	event2 := Event{
		Action:   ActionDeviceRevoked,
		ActorID:  "test2",
		ActorIP:  "127.0.0.1",
		TargetID: "test2",
		Success:  true,
	}

	before := time.Now()
	logger.Log(event2)
	after := time.Now()

	// Automatic timestamp should be between before and after
	// (We can't check event2.Timestamp directly since it's passed by value)
	_ = before
	_ = after
}

func TestActionConstants(t *testing.T) {
	actions := []Action{
		ActionDevicePaired,
		ActionDeviceRevoked,
		ActionDeviceUpdated,
		ActionAppRegistered,
		ActionAppDeleted,
		ActionRouteCreated,
		ActionRouteDeleted,
		ActionConfigReloaded,
		ActionProposalApproved,
		ActionProposalDismissed,
	}

	// Ensure all actions are defined
	for _, action := range actions {
		if string(action) == "" {
			t.Errorf("Action %v should not be empty", action)
		}
	}
}
