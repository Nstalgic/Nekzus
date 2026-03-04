package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/nstalgic/nekzus/internal/storage"
	"github.com/nstalgic/nekzus/internal/types"
)

// mockWebSocketManager is a test mock for WebSocketManager
type mockWebSocketManager struct {
	disconnectCalled bool
	disconnectedID   string
	publishCalled    bool
	publishedID      string
	disconnectCount  int
}

func (m *mockWebSocketManager) DisconnectDevice(deviceID string) int {
	m.disconnectCalled = true
	m.disconnectedID = deviceID
	m.disconnectCount = 1
	return m.disconnectCount
}

func (m *mockWebSocketManager) PublishDeviceRevoked(deviceID string) {
	m.publishCalled = true
	m.publishedID = deviceID
}

// mockActivityTracker is a test mock for ActivityTracker
type mockActivityTracker struct {
	addCalled    bool
	lastEvent    types.ActivityEvent
	addCallCount int
}

func (m *mockActivityTracker) Add(event types.ActivityEvent) error {
	m.addCalled = true
	m.lastEvent = event
	m.addCallCount++
	return nil
}

func TestDeviceHandler_RevokeDevice_DisconnectsWebSocket(t *testing.T) {
	// Create temporary database
	tmpFile, err := os.CreateTemp("", "nekzus-test-*.db")
	if err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()
	dbPath := tmpFile.Name()
	defer os.Remove(dbPath)

	// Initialize storage
	store, err := storage.NewStore(storage.Config{
		DatabasePath: dbPath,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// Create test device
	if err := store.SaveDevice("test-device", "Test Device", "ios", "16.0", []string{"read:catalog"}); err != nil {
		t.Fatal(err)
	}

	// Create handler with mocks
	mockWS := &mockWebSocketManager{}
	mockActivity := &mockActivityTracker{}

	handler := NewDeviceHandler(store)
	handler.SetWebSocketManager(mockWS)
	handler.SetActivityTracker(mockActivity)

	// Create test request
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/devices/test-device", nil)
	w := httptest.NewRecorder()

	// Execute
	handler.HandleRevokeDevice(w, req)

	// Verify HTTP response
	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	// Verify device was deleted from storage
	device, err := store.GetDevice("test-device")
	if err != nil {
		t.Fatal(err)
	}
	if device != nil {
		t.Error("device should have been deleted from storage")
	}

	// Verify WebSocket disconnect was called
	if !mockWS.disconnectCalled {
		t.Error("DisconnectDevice should have been called")
	}
	if mockWS.disconnectedID != "test-device" {
		t.Errorf("expected disconnect for 'test-device', got '%s'", mockWS.disconnectedID)
	}

	// Verify WebSocket event was published
	if !mockWS.publishCalled {
		t.Error("PublishDeviceRevoked should have been called")
	}
	if mockWS.publishedID != "test-device" {
		t.Errorf("expected publish for 'test-device', got '%s'", mockWS.publishedID)
	}

	// Verify activity was logged
	if !mockActivity.addCalled {
		t.Error("Activity.Add should have been called")
	}
	if mockActivity.lastEvent.Type != "device_revoked" {
		t.Errorf("expected activity type 'device_revoked', got '%s'", mockActivity.lastEvent.Type)
	}
	if mockActivity.lastEvent.Icon != "XCircle" {
		t.Errorf("expected activity icon 'XCircle', got '%s'", mockActivity.lastEvent.Icon)
	}
}

func TestDeviceHandler_RevokeDevice_WorksWithoutManagers(t *testing.T) {
	// Create temporary database
	tmpFile, err := os.CreateTemp("", "nekzus-test-*.db")
	if err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()
	dbPath := tmpFile.Name()
	defer os.Remove(dbPath)

	// Initialize storage
	store, err := storage.NewStore(storage.Config{
		DatabasePath: dbPath,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// Create test device
	if err := store.SaveDevice("test-device-2", "Test Device 2", "android", "13", []string{"read:catalog"}); err != nil {
		t.Fatal(err)
	}

	// Create handler WITHOUT managers (testing graceful degradation)
	handler := NewDeviceHandler(store)
	// Don't set WebSocket or Activity managers

	// Create test request
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/devices/test-device-2", nil)
	w := httptest.NewRecorder()

	// Execute (should not panic)
	handler.HandleRevokeDevice(w, req)

	// Verify HTTP response
	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	// Verify device was deleted from storage
	device, err := store.GetDevice("test-device-2")
	if err != nil {
		t.Fatal(err)
	}
	if device != nil {
		t.Error("device should have been deleted from storage")
	}
}

func TestDeviceHandler_RevokeDevice_NotFound(t *testing.T) {
	// Create temporary database
	tmpFile, err := os.CreateTemp("", "nekzus-test-*.db")
	if err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()
	dbPath := tmpFile.Name()
	defer os.Remove(dbPath)

	// Initialize storage
	store, err := storage.NewStore(storage.Config{
		DatabasePath: dbPath,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// Create handler
	handler := NewDeviceHandler(store)

	// Create test request for non-existent device
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/devices/nonexistent", nil)
	w := httptest.NewRecorder()

	// Execute
	handler.HandleRevokeDevice(w, req)

	// Verify HTTP response
	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", w.Code)
	}
}

func TestDeviceHandler_RevokeDevice_AdminPath(t *testing.T) {
	// This test verifies that device revocation works via the admin path
	// which is used by the web dashboard: /api/v1/admin/devices/{deviceId}

	// Create temporary database
	tmpFile, err := os.CreateTemp("", "nekzus-test-*.db")
	if err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()
	dbPath := tmpFile.Name()
	defer os.Remove(dbPath)

	// Initialize storage
	store, err := storage.NewStore(storage.Config{
		DatabasePath: dbPath,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// Create test device
	if err := store.SaveDevice("admin-test-device", "Admin Test Device", "ios", "16.0", []string{"read:catalog"}); err != nil {
		t.Fatal(err)
	}

	// Create handler with mocks
	mockWS := &mockWebSocketManager{}
	mockActivity := &mockActivityTracker{}

	handler := NewDeviceHandler(store)
	handler.SetWebSocketManager(mockWS)
	handler.SetActivityTracker(mockActivity)

	// Create test request using ADMIN path (this is what the web UI uses)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/devices/admin-test-device", nil)
	w := httptest.NewRecorder()

	// Execute
	handler.HandleRevokeDevice(w, req)

	// Verify HTTP response - should be 200 OK, not 400 "Device ID required"
	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify device was deleted from storage
	device, err := store.GetDevice("admin-test-device")
	if err != nil {
		t.Fatal(err)
	}
	if device != nil {
		t.Error("device should have been deleted from storage")
	}

	// Verify WebSocket disconnect was called
	if !mockWS.disconnectCalled {
		t.Error("DisconnectDevice should have been called")
	}
	if mockWS.disconnectedID != "admin-test-device" {
		t.Errorf("expected disconnect for 'admin-test-device', got '%s'", mockWS.disconnectedID)
	}
}

func TestExtractDeviceIDFromPath(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{
			name:     "standard path",
			path:     "/api/v1/devices/device-123",
			expected: "device-123",
		},
		{
			name:     "admin path",
			path:     "/api/v1/admin/devices/device-456",
			expected: "device-456",
		},
		{
			name:     "standard path with trailing slash",
			path:     "/api/v1/devices/device-789/",
			expected: "device-789",
		},
		{
			name:     "admin path with trailing slash",
			path:     "/api/v1/admin/devices/device-abc/",
			expected: "device-abc",
		},
		{
			name:     "path traversal attempt",
			path:     "/api/v1/devices/../../../etc/passwd",
			expected: "",
		},
		{
			name:     "empty device ID",
			path:     "/api/v1/devices/",
			expected: "",
		},
		{
			name:     "unrelated path",
			path:     "/api/v1/routes/some-route",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractDeviceIDFromPath(tt.path)
			if result != tt.expected {
				t.Errorf("extractDeviceIDFromPath(%q) = %q, want %q", tt.path, result, tt.expected)
			}
		})
	}
}

func TestDeviceHandler_ActivityEventTimestamp(t *testing.T) {
	// Create temporary database
	tmpFile, err := os.CreateTemp("", "nekzus-test-*.db")
	if err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()
	dbPath := tmpFile.Name()
	defer os.Remove(dbPath)

	// Initialize storage
	store, err := storage.NewStore(storage.Config{
		DatabasePath: dbPath,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// Create test device
	if err := store.SaveDevice("test-timestamp", "Test Timestamp", "ios", "16.0", []string{"read:catalog"}); err != nil {
		t.Fatal(err)
	}

	// Create handler with mock activity tracker
	mockActivity := &mockActivityTracker{}
	handler := NewDeviceHandler(store)
	handler.SetActivityTracker(mockActivity)

	// Record time before revocation
	beforeRevoke := time.Now().UnixMilli()

	// Create test request
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/devices/test-timestamp", nil)
	w := httptest.NewRecorder()

	// Execute
	handler.HandleRevokeDevice(w, req)

	// Record time after revocation
	afterRevoke := time.Now().UnixMilli()

	// Verify activity timestamp is within expected range
	if mockActivity.lastEvent.Timestamp < beforeRevoke {
		t.Errorf("activity timestamp too early: %d < %d", mockActivity.lastEvent.Timestamp, beforeRevoke)
	}
	if mockActivity.lastEvent.Timestamp > afterRevoke {
		t.Errorf("activity timestamp too late: %d > %d", mockActivity.lastEvent.Timestamp, afterRevoke)
	}
}

// TestDeviceHandler_UpdatePins tests the pin rotation endpoint
// POST /api/v1/devices/{deviceId}/pins
func TestDeviceHandler_UpdatePins(t *testing.T) {
	// Create temporary database
	tmpFile, err := os.CreateTemp("", "nekzus-test-*.db")
	if err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()
	dbPath := tmpFile.Name()
	defer os.Remove(dbPath)

	// Initialize storage
	store, err := storage.NewStore(storage.Config{
		DatabasePath: dbPath,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// Create test device
	if err := store.SaveDevice("test-pins-device", "Test Pins Device", "ios", "16.0", []string{"read:catalog"}); err != nil {
		t.Fatal(err)
	}

	// Create handler
	handler := NewDeviceHandler(store)

	// Create request with new pin
	reqBody := `{"newPin": "sha256/NewCertificateHash123456789abcdef=="}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/devices/test-pins-device/pins", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	// Execute
	handler.HandleUpdatePins(w, req)

	// Verify HTTP response
	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	// Parse response
	var response map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// Verify response structure
	success, ok := response["success"].(bool)
	if !ok || !success {
		t.Error("Expected success: true in response")
	}

	activePins, ok := response["activePins"].([]interface{})
	if !ok {
		t.Fatal("Expected activePins array in response")
	}

	// Should have the new pin
	found := false
	for _, pin := range activePins {
		if pin == "sha256/NewCertificateHash123456789abcdef==" {
			found = true
			break
		}
	}
	if !found {
		t.Error("New pin not found in activePins")
	}
}

// TestDeviceHandler_UpdatePins_DeviceNotFound tests pin update for non-existent device
func TestDeviceHandler_UpdatePins_DeviceNotFound(t *testing.T) {
	// Create temporary database
	tmpFile, err := os.CreateTemp("", "nekzus-test-*.db")
	if err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()
	dbPath := tmpFile.Name()
	defer os.Remove(dbPath)

	// Initialize storage
	store, err := storage.NewStore(storage.Config{
		DatabasePath: dbPath,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	handler := NewDeviceHandler(store)

	reqBody := `{"newPin": "sha256/SomePin=="}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/devices/nonexistent/pins", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.HandleUpdatePins(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", w.Code)
	}
}

// TestDeviceHandler_UpdatePins_InvalidPin tests pin update with invalid pin format
func TestDeviceHandler_UpdatePins_InvalidPin(t *testing.T) {
	// Create temporary database
	tmpFile, err := os.CreateTemp("", "nekzus-test-*.db")
	if err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()
	dbPath := tmpFile.Name()
	defer os.Remove(dbPath)

	// Initialize storage
	store, err := storage.NewStore(storage.Config{
		DatabasePath: dbPath,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// Create test device
	if err := store.SaveDevice("test-pins-device", "Test Pins Device", "ios", "16.0", []string{"read:catalog"}); err != nil {
		t.Fatal(err)
	}

	handler := NewDeviceHandler(store)

	// Invalid pin (doesn't start with sha256/)
	reqBody := `{"newPin": "invalid-pin-format"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/devices/test-pins-device/pins", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.HandleUpdatePins(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}
