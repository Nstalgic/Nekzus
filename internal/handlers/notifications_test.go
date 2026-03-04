package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/nstalgic/nekzus/internal/storage"
)

func setupNotificationTestStore(t *testing.T) (*storage.Store, func()) {
	t.Helper()
	tmpFile, err := os.CreateTemp("", "nekzus-notif-test-*.db")
	if err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()
	dbPath := tmpFile.Name()

	store, err := storage.NewStore(storage.Config{
		DatabasePath: dbPath,
	})
	if err != nil {
		os.Remove(dbPath)
		t.Fatal(err)
	}

	return store, func() {
		store.Close()
		os.Remove(dbPath)
	}
}

func TestNotificationHandler_ListNotifications(t *testing.T) {
	store, cleanup := setupNotificationTestStore(t)
	defer cleanup()

	// Create test device and notification
	store.SaveDevice("test-device", "Test Device", "ios", "16.0", []string{"read"})
	payload := json.RawMessage(`{"key": "value"}`)
	store.EnqueueNotification("test-device", "device.revoked", payload, 30*24*time.Hour, 3)

	handler := NewNotificationHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/notifications", nil)
	w := httptest.NewRecorder()

	handler.HandleListNotifications(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var result storage.NotificationListResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if result.Total != 1 {
		t.Errorf("expected 1 notification, got %d", result.Total)
	}
	if len(result.Notifications) != 1 {
		t.Errorf("expected 1 notification in list, got %d", len(result.Notifications))
	}
	if result.Notifications[0].DeviceID != "test-device" {
		t.Errorf("expected device test-device, got %s", result.Notifications[0].DeviceID)
	}
}

func TestNotificationHandler_ListNotifications_WithFilters(t *testing.T) {
	store, cleanup := setupNotificationTestStore(t)
	defer cleanup()

	store.SaveDevice("device-a", "Device A", "ios", "16.0", []string{"read"})
	store.SaveDevice("device-b", "Device B", "android", "14", []string{"read"})

	payload := json.RawMessage(`{}`)
	store.EnqueueNotification("device-a", "type.one", payload, 30*24*time.Hour, 3)
	store.EnqueueNotification("device-b", "type.two", payload, 30*24*time.Hour, 3)

	handler := NewNotificationHandler(store)

	// Filter by device
	req := httptest.NewRequest(http.MethodGet, "/api/v1/notifications?device_id=device-a", nil)
	w := httptest.NewRecorder()

	handler.HandleListNotifications(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var result storage.NotificationListResult
	json.Unmarshal(w.Body.Bytes(), &result)

	if result.Total != 1 {
		t.Errorf("expected 1 notification for device-a, got %d", result.Total)
	}
}

func TestNotificationHandler_ListNotifications_MethodNotAllowed(t *testing.T) {
	store, cleanup := setupNotificationTestStore(t)
	defer cleanup()

	handler := NewNotificationHandler(store)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/notifications", nil)
	w := httptest.NewRecorder()

	handler.HandleListNotifications(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", w.Code)
	}
}

func TestNotificationHandler_GetStats(t *testing.T) {
	store, cleanup := setupNotificationTestStore(t)
	defer cleanup()

	store.SaveDevice("stats-device", "Stats Device", "ios", "16.0", []string{"read"})

	payload := json.RawMessage(`{}`)
	id1, _ := store.EnqueueNotification("stats-device", "type.one", payload, 30*24*time.Hour, 3)
	id2, _ := store.EnqueueNotification("stats-device", "type.two", payload, 30*24*time.Hour, 3)

	// Mark one as delivered
	store.MarkNotificationDelivered(id1)
	// Mark one as failed
	for i := 0; i < 4; i++ {
		store.UpdateNotificationRetry(id2, "test error")
	}

	handler := NewNotificationHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/notifications/stats", nil)
	w := httptest.NewRecorder()

	handler.HandleGetNotificationStats(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var stats storage.NotificationQueueStats
	if err := json.Unmarshal(w.Body.Bytes(), &stats); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if stats.TotalDelivered != 1 {
		t.Errorf("expected 1 delivered, got %d", stats.TotalDelivered)
	}
	if stats.TotalFailed != 1 {
		t.Errorf("expected 1 failed, got %d", stats.TotalFailed)
	}
}

func TestNotificationHandler_GetStaleNotifications(t *testing.T) {
	store, cleanup := setupNotificationTestStore(t)
	defer cleanup()

	store.SaveDevice("stale-device", "Stale Device", "ios", "16.0", []string{"read"})
	payload := json.RawMessage(`{}`)
	store.EnqueueNotification("stale-device", "test.type", payload, 30*24*time.Hour, 3)

	handler := NewNotificationHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/notifications/stale", nil)
	w := httptest.NewRecorder()

	handler.HandleGetStaleNotifications(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if _, ok := response["staleThresholdHours"]; !ok {
		t.Error("expected staleThresholdHours in response")
	}
	if _, ok := response["devices"]; !ok {
		t.Error("expected devices in response")
	}
}

func TestNotificationHandler_DismissNotification(t *testing.T) {
	store, cleanup := setupNotificationTestStore(t)
	defer cleanup()

	store.SaveDevice("dismiss-device", "Dismiss Device", "ios", "16.0", []string{"read"})
	payload := json.RawMessage(`{}`)
	id, _ := store.EnqueueNotification("dismiss-device", "test.type", payload, 30*24*time.Hour, 3)

	handler := NewNotificationHandler(store)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/notifications/"+itoa(id), nil)
	w := httptest.NewRecorder()

	handler.HandleDismissNotification(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var response map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &response)

	if response["status"] != "dismissed" {
		t.Errorf("expected status dismissed, got %v", response["status"])
	}

	// Verify notification was dismissed
	pending, _ := store.GetPendingNotifications("dismiss-device")
	if len(pending) != 0 {
		t.Errorf("expected 0 pending notifications, got %d", len(pending))
	}
}

func TestNotificationHandler_DismissNotification_NotFound(t *testing.T) {
	store, cleanup := setupNotificationTestStore(t)
	defer cleanup()

	handler := NewNotificationHandler(store)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/notifications/99999", nil)
	w := httptest.NewRecorder()

	handler.HandleDismissNotification(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestNotificationHandler_DismissNotification_InvalidID(t *testing.T) {
	store, cleanup := setupNotificationTestStore(t)
	defer cleanup()

	handler := NewNotificationHandler(store)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/notifications/invalid", nil)
	w := httptest.NewRecorder()

	handler.HandleDismissNotification(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestNotificationHandler_DismissDeviceNotifications(t *testing.T) {
	store, cleanup := setupNotificationTestStore(t)
	defer cleanup()

	store.SaveDevice("bulk-device", "Bulk Device", "ios", "16.0", []string{"read"})
	payload := json.RawMessage(`{}`)
	store.EnqueueNotification("bulk-device", "type.one", payload, 30*24*time.Hour, 3)
	store.EnqueueNotification("bulk-device", "type.two", payload, 30*24*time.Hour, 3)
	store.EnqueueNotification("bulk-device", "type.three", payload, 30*24*time.Hour, 3)

	handler := NewNotificationHandler(store)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/notifications/device/bulk-device", nil)
	w := httptest.NewRecorder()

	handler.HandleDismissDeviceNotifications(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var response map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &response)

	if response["status"] != "dismissed" {
		t.Errorf("expected status dismissed, got %v", response["status"])
	}
	if response["count"].(float64) != 3 {
		t.Errorf("expected count 3, got %v", response["count"])
	}

	// Verify all notifications dismissed
	pending, _ := store.GetPendingNotifications("bulk-device")
	if len(pending) != 0 {
		t.Errorf("expected 0 pending, got %d", len(pending))
	}
}

func TestNotificationHandler_DismissDeviceNotifications_EmptyID(t *testing.T) {
	store, cleanup := setupNotificationTestStore(t)
	defer cleanup()

	handler := NewNotificationHandler(store)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/notifications/device/", nil)
	w := httptest.NewRecorder()

	handler.HandleDismissDeviceNotifications(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestNotificationHandler_RetryNotification(t *testing.T) {
	store, cleanup := setupNotificationTestStore(t)
	defer cleanup()

	store.SaveDevice("retry-device", "Retry Device", "ios", "16.0", []string{"read"})
	payload := json.RawMessage(`{}`)
	id, _ := store.EnqueueNotification("retry-device", "test.type", payload, 30*24*time.Hour, 3)

	// Mark as failed
	for i := 0; i < 4; i++ {
		store.UpdateNotificationRetry(id, "test error")
	}

	handler := NewNotificationHandler(store)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/notifications/"+itoa(id)+"/retry", nil)
	w := httptest.NewRecorder()

	handler.HandleRetryNotification(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var response map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &response)

	if response["status"] != "queued" {
		t.Errorf("expected status queued, got %v", response["status"])
	}
}

func TestNotificationHandler_RetryNotification_NotFound(t *testing.T) {
	store, cleanup := setupNotificationTestStore(t)
	defer cleanup()

	handler := NewNotificationHandler(store)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/notifications/99999/retry", nil)
	w := httptest.NewRecorder()

	handler.HandleRetryNotification(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestNotificationHandler_BulkRetryNotifications(t *testing.T) {
	store, cleanup := setupNotificationTestStore(t)
	defer cleanup()

	store.SaveDevice("bulk-retry-device", "Bulk Retry Device", "ios", "16.0", []string{"read"})
	payload := json.RawMessage(`{}`)
	id1, _ := store.EnqueueNotification("bulk-retry-device", "type.one", payload, 30*24*time.Hour, 3)
	id2, _ := store.EnqueueNotification("bulk-retry-device", "type.two", payload, 30*24*time.Hour, 3)

	// Mark both as failed
	for i := 0; i < 4; i++ {
		store.UpdateNotificationRetry(id1, "test error")
		store.UpdateNotificationRetry(id2, "test error")
	}

	handler := NewNotificationHandler(store)

	body := strings.NewReader(`{"ids": [` + itoa(id1) + `, ` + itoa(id2) + `]}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/notifications/retry", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.HandleBulkRetryNotifications(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var response map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &response)

	if response["status"] != "queued" {
		t.Errorf("expected status queued, got %v", response["status"])
	}
	if response["count"].(float64) != 2 {
		t.Errorf("expected count 2, got %v", response["count"])
	}
}

func TestNotificationHandler_BulkRetryNotifications_EmptyIDs(t *testing.T) {
	store, cleanup := setupNotificationTestStore(t)
	defer cleanup()

	handler := NewNotificationHandler(store)

	body := strings.NewReader(`{"ids": []}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/notifications/retry", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.HandleBulkRetryNotifications(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestNotificationHandler_BulkRetryNotifications_InvalidBody(t *testing.T) {
	store, cleanup := setupNotificationTestStore(t)
	defer cleanup()

	handler := NewNotificationHandler(store)

	body := strings.NewReader(`invalid json`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/notifications/retry", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.HandleBulkRetryNotifications(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

// itoa converts int64 to string (helper for tests)
func itoa(n int64) string {
	return strconv.FormatInt(n, 10)
}
