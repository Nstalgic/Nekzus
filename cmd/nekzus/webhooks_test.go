package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/nstalgic/nekzus/internal/activity"
	"github.com/nstalgic/nekzus/internal/storage"
	"github.com/nstalgic/nekzus/internal/websocket"
)

// TestWebhookActivity_Success tests successful activity webhook creation
func TestWebhookActivity_Success(t *testing.T) {
	// Setup test application with storage
	tmpFile, err := os.CreateTemp("", "nekzus-webhook-test-*.db")
	if err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()
	dbPath := tmpFile.Name()
	defer os.Remove(dbPath)

	store, err := storage.NewStore(storage.Config{
		DatabasePath: dbPath,
	})
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	app := &Application{
		storage: store,
		metrics: testMetrics,
		managers: &ManagerRegistry{
			WebSocket: websocket.NewManager(testMetrics, store),
			Activity:  activity.NewTracker(store),
		},
		jobs: &JobRegistry{}, // Empty jobs registry for tests
	}

	// Create test payload
	payload := map[string]interface{}{
		"message":   "Test webhook activity",
		"icon":      "Bell",
		"iconClass": "success",
		"details":   "This is a test",
	}
	body, _ := json.Marshal(payload)

	// Create request
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/activity", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	// Execute handler
	app.handleWebhookActivity(w, req)

	// Verify response
	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify activity was added
	activities := app.managers.Activity.Get()
	if len(activities) != 1 {
		t.Fatalf("Expected 1 activity, got %d", len(activities))
	}

	activity := activities[0]
	if activity.Message != "Test webhook activity" {
		t.Errorf("Expected message 'Test webhook activity', got '%s'", activity.Message)
	}
	if activity.Icon != "Bell" {
		t.Errorf("Expected icon 'Bell', got '%s'", activity.Icon)
	}
	if activity.IconClass != "success" {
		t.Errorf("Expected iconClass 'success', got '%s'", activity.IconClass)
	}
	if activity.Details != "This is a test" {
		t.Errorf("Expected details 'This is a test', got '%s'", activity.Details)
	}
}

// TestWebhookActivity_MinimalPayload tests webhook with only required fields
func TestWebhookActivity_MinimalPayload(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "nekzus-webhook-test-*.db")
	if err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()
	dbPath := tmpFile.Name()
	defer os.Remove(dbPath)

	store, err := storage.NewStore(storage.Config{
		DatabasePath: dbPath,
	})
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	app := &Application{
		storage: store,
		metrics: testMetrics,
		managers: &ManagerRegistry{
			WebSocket: websocket.NewManager(testMetrics, store),
			Activity:  activity.NewTracker(store),
		},
		jobs: &JobRegistry{}, // Empty jobs registry for tests
	}

	// Create minimal payload (only message)
	payload := map[string]interface{}{
		"message": "Minimal webhook",
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/activity", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	app.handleWebhookActivity(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify defaults were applied
	activities := app.managers.Activity.Get()
	if len(activities) != 1 {
		t.Fatalf("Expected 1 activity, got %d", len(activities))
	}

	activity := activities[0]
	if activity.Message != "Minimal webhook" {
		t.Errorf("Expected message 'Minimal webhook', got '%s'", activity.Message)
	}
	// Should have default icon
	if activity.Icon == "" {
		t.Error("Expected default icon to be set")
	}
}

// TestWebhookActivity_InvalidPayload tests error handling for invalid payloads
func TestWebhookActivity_InvalidPayload(t *testing.T) {
	app := &Application{
		metrics: testMetrics,
		managers: &ManagerRegistry{
			WebSocket: websocket.NewManager(testMetrics, nil),
			Activity:  activity.NewTracker(nil),
		},
		jobs: &JobRegistry{}, // Empty jobs registry for tests
	}

	tests := []struct {
		name    string
		payload string
		wantErr bool
	}{
		{
			name:    "Invalid JSON",
			payload: `{invalid json}`,
			wantErr: true,
		},
		{
			name:    "Missing message field",
			payload: `{"icon": "Bell"}`,
			wantErr: true,
		},
		{
			name:    "Empty message",
			payload: `{"message": ""}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/activity", bytes.NewReader([]byte(tt.payload)))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			app.handleWebhookActivity(w, req)

			if tt.wantErr && w.Code == http.StatusOK {
				t.Errorf("Expected error status, got 200")
			}
			if !tt.wantErr && w.Code != http.StatusOK {
				t.Errorf("Expected status 200, got %d", w.Code)
			}
		})
	}
}

// TestWebhookActivity_MethodNotAllowed tests that only POST is allowed
func TestWebhookActivity_MethodNotAllowed(t *testing.T) {
	app := &Application{
		metrics: testMetrics,
		managers: &ManagerRegistry{
			WebSocket: websocket.NewManager(testMetrics, nil),
			Activity:  activity.NewTracker(nil),
		},
		jobs: &JobRegistry{}, // Empty jobs registry for tests
	}

	methods := []string{http.MethodGet, http.MethodPut, http.MethodDelete, http.MethodPatch}

	for _, method := range methods {
		req := httptest.NewRequest(method, "/api/v1/webhooks/activity", nil)
		w := httptest.NewRecorder()

		app.handleWebhookActivity(w, req)

		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("Method %s: expected status 405, got %d", method, w.Code)
		}
	}
}

// TestWebhookNotify_Success tests successful arbitrary notification webhook
func TestWebhookNotify_Success(t *testing.T) {
	app := &Application{
		metrics: testMetrics,
		managers: &ManagerRegistry{
			WebSocket: websocket.NewManager(testMetrics, nil),
		},
		jobs: &JobRegistry{}, // Empty jobs registry for tests
	}

	// Create test payload with arbitrary JSON
	payload := map[string]interface{}{
		"type": "custom_alert",
		"data": map[string]interface{}{
			"alertType": "cpu",
			"value":     95,
			"timestamp": time.Now().Unix(),
		},
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/notify", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	app.handleWebhookNotify(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d: %s", w.Code, w.Body.String())
	}
}

// TestWebhookNotify_InvalidJSON tests error handling for invalid JSON
func TestWebhookNotify_InvalidJSON(t *testing.T) {
	app := &Application{
		metrics: testMetrics,
		managers: &ManagerRegistry{
			WebSocket: websocket.NewManager(testMetrics, nil),
		},
		jobs: &JobRegistry{}, // Empty jobs registry for tests
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/notify", bytes.NewReader([]byte(`{invalid}`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	app.handleWebhookNotify(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}
}

// TestWebhookActivity_DeviceFiltering tests targeting specific devices
func TestWebhookActivity_DeviceFiltering(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "nekzus-webhook-test-*.db")
	if err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()
	dbPath := tmpFile.Name()
	defer os.Remove(dbPath)

	store, err := storage.NewStore(storage.Config{
		DatabasePath: dbPath,
	})
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	app := &Application{
		storage: store,
		metrics: testMetrics,
		managers: &ManagerRegistry{
			WebSocket: websocket.NewManager(testMetrics, store),
			Activity:  activity.NewTracker(store),
		},
		jobs: &JobRegistry{}, // Empty jobs registry for tests
	}

	// Create payload targeting specific device IDs
	// Note: Devices don't need to exist for this test, we're just testing
	// that the handler accepts the deviceIds field and doesn't error
	payload := map[string]interface{}{
		"message":   "Targeted webhook",
		"deviceIds": []string{"device-1", "device-2"},
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/activity", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	app.handleWebhookActivity(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify activity was added
	activities := app.managers.Activity.Get()
	if len(activities) != 1 {
		t.Fatalf("Expected 1 activity, got %d", len(activities))
	}

	// Note: Actual WebSocket message filtering would be tested in integration test
}

// TestWebhookNotify_DeviceFiltering tests targeting specific devices for notifications
func TestWebhookNotify_DeviceFiltering(t *testing.T) {
	app := &Application{
		metrics: testMetrics,
		managers: &ManagerRegistry{
			WebSocket: websocket.NewManager(testMetrics, nil),
		},
		jobs: &JobRegistry{}, // Empty jobs registry for tests
	}

	payload := map[string]interface{}{
		"deviceIds": []string{"device-1", "device-2"},
		"type":      "custom_alert",
		"data":      map[string]string{"key": "value"},
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/notify", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	app.handleWebhookNotify(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

// TestWebhookActivity_Broadcast tests broadcasting to all devices (no deviceIds)
func TestWebhookActivity_Broadcast(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "nekzus-webhook-test-*.db")
	if err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()
	dbPath := tmpFile.Name()
	defer os.Remove(dbPath)

	store, err := storage.NewStore(storage.Config{
		DatabasePath: dbPath,
	})
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	app := &Application{
		storage: store,
		metrics: testMetrics,
		managers: &ManagerRegistry{
			WebSocket: websocket.NewManager(testMetrics, store),
			Activity:  activity.NewTracker(store),
		},
		jobs: &JobRegistry{}, // Empty jobs registry for tests
	}

	// Create payload with NO deviceIds (should broadcast to all)
	payload := map[string]interface{}{
		"message": "Broadcast webhook",
		"icon":    "Radio",
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/activity", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	app.handleWebhookActivity(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify activity was added
	activities := app.managers.Activity.Get()
	if len(activities) != 1 {
		t.Fatalf("Expected 1 activity, got %d", len(activities))
	}
}

// Note: WebSocket broadcast functionality is tested via integration tests
// since we can't easily mock the Broadcast method in this test setup.
