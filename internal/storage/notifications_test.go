package storage

import (
	"encoding/json"
	"testing"
	"time"
)

func TestEnqueueNotification(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	// Create a test device first
	err := store.SaveDevice("test-device", "Test Device", "", "", []string{"read", "write"})
	if err != nil {
		t.Fatalf("failed to create test device: %v", err)
	}

	payload := json.RawMessage(`{"key":"value"}`)
	ttl := 5 * time.Minute
	maxRetries := 3

	id, err := store.EnqueueNotification("test-device", "config_reload", payload, ttl, maxRetries)
	if err != nil {
		t.Fatalf("EnqueueNotification failed: %v", err)
	}

	if id == 0 {
		t.Error("expected non-zero notification ID")
	}
}

func TestGetPendingNotifications(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	// Create test device
	err := store.SaveDevice("device-1", "Device 1", "", "", []string{"read"})
	if err != nil {
		t.Fatalf("failed to create test device: %v", err)
	}

	// Enqueue multiple notifications
	payload1 := json.RawMessage(`{"msg":"one"}`)
	payload2 := json.RawMessage(`{"msg":"two"}`)

	_, err = store.EnqueueNotification("device-1", "event1", payload1, 5*time.Minute, 3)
	if err != nil {
		t.Fatalf("EnqueueNotification failed: %v", err)
	}

	_, err = store.EnqueueNotification("device-1", "event2", payload2, 5*time.Minute, 3)
	if err != nil {
		t.Fatalf("EnqueueNotification failed: %v", err)
	}

	// Get pending notifications
	pending, err := store.GetPendingNotifications("device-1")
	if err != nil {
		t.Fatalf("GetPendingNotifications failed: %v", err)
	}

	if len(pending) != 2 {
		t.Errorf("expected 2 pending notifications, got %d", len(pending))
	}

	// Verify notification details
	if pending[0].DeviceID != "device-1" {
		t.Errorf("expected device_id=device-1, got %s", pending[0].DeviceID)
	}

	if pending[0].Status != "pending" {
		t.Errorf("expected status=pending, got %s", pending[0].Status)
	}
}

func TestGetPendingNotifications_OnlyForDevice(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	// Create two devices
	store.SaveDevice("device-1", "Device 1", "", "", []string{"read"})
	store.SaveDevice("device-2", "Device 2", "", "", []string{"read"})

	// Enqueue for both devices
	payload := json.RawMessage(`{"msg":"test"}`)
	store.EnqueueNotification("device-1", "event", payload, 5*time.Minute, 3)
	store.EnqueueNotification("device-2", "event", payload, 5*time.Minute, 3)

	// Get pending for device-1
	pending, err := store.GetPendingNotifications("device-1")
	if err != nil {
		t.Fatalf("GetPendingNotifications failed: %v", err)
	}

	if len(pending) != 1 {
		t.Errorf("expected 1 pending notification for device-1, got %d", len(pending))
	}

	if pending[0].DeviceID != "device-1" {
		t.Errorf("expected device_id=device-1, got %s", pending[0].DeviceID)
	}
}

func TestGetPendingNotifications_ExcludesExpired(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	store.SaveDevice("device-1", "Device 1", "", "", []string{"read"})

	// Enqueue notification with very short TTL (already expired)
	payload := json.RawMessage(`{"msg":"expired"}`)
	shortTTL := -1 * time.Second // Negative means already expired

	_, err := store.EnqueueNotification("device-1", "event", payload, shortTTL, 3)
	if err != nil {
		t.Fatalf("EnqueueNotification failed: %v", err)
	}

	// Get pending - should exclude expired
	pending, err := store.GetPendingNotifications("device-1")
	if err != nil {
		t.Fatalf("GetPendingNotifications failed: %v", err)
	}

	if len(pending) != 0 {
		t.Errorf("expected 0 pending notifications (expired), got %d", len(pending))
	}
}

func TestMarkNotificationDelivered(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	store.SaveDevice("device-1", "Device 1", "", "", []string{"read"})

	payload := json.RawMessage(`{"msg":"test"}`)
	id, err := store.EnqueueNotification("device-1", "event", payload, 5*time.Minute, 3)
	if err != nil {
		t.Fatalf("EnqueueNotification failed: %v", err)
	}

	// Mark as delivered
	err = store.MarkNotificationDelivered(id)
	if err != nil {
		t.Fatalf("MarkNotificationDelivered failed: %v", err)
	}

	// Verify status changed
	pending, err := store.GetPendingNotifications("device-1")
	if err != nil {
		t.Fatalf("GetPendingNotifications failed: %v", err)
	}

	if len(pending) != 0 {
		t.Errorf("expected 0 pending after delivery, got %d", len(pending))
	}
}

func TestUpdateNotificationRetry(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	store.SaveDevice("device-1", "Device 1", "", "", []string{"read"})

	payload := json.RawMessage(`{"msg":"test"}`)
	id, err := store.EnqueueNotification("device-1", "event", payload, 5*time.Minute, 3)
	if err != nil {
		t.Fatalf("EnqueueNotification failed: %v", err)
	}

	// Update retry
	errorMsg := "device offline"
	err = store.UpdateNotificationRetry(id, errorMsg)
	if err != nil {
		t.Fatalf("UpdateNotificationRetry failed: %v", err)
	}

	// Verify retry count incremented
	pending, err := store.GetPendingNotifications("device-1")
	if err != nil {
		t.Fatalf("GetPendingNotifications failed: %v", err)
	}

	if len(pending) == 0 {
		t.Fatal("expected notification to still be pending")
	}

	if pending[0].RetryCount != 1 {
		t.Errorf("expected retry_count=1, got %d", pending[0].RetryCount)
	}

	if pending[0].ErrorMessage != errorMsg {
		t.Errorf("expected error_message=%s, got %s", errorMsg, pending[0].ErrorMessage)
	}
}

func TestUpdateNotificationRetry_MaxRetries(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	store.SaveDevice("device-1", "Device 1", "", "", []string{"read"})

	payload := json.RawMessage(`{"msg":"test"}`)
	maxRetries := 3
	id, err := store.EnqueueNotification("device-1", "event", payload, 5*time.Minute, maxRetries)
	if err != nil {
		t.Fatalf("EnqueueNotification failed: %v", err)
	}

	// Retry until max retries reached
	for i := 0; i < maxRetries; i++ {
		err = store.UpdateNotificationRetry(id, "offline")
		if err != nil {
			t.Fatalf("UpdateNotificationRetry failed: %v", err)
		}
	}

	// GetPendingNotifications returns both pending and failed (for reconnect drain)
	pending, err := store.GetPendingNotifications("device-1")
	if err != nil {
		t.Fatalf("GetPendingNotifications failed: %v", err)
	}

	if len(pending) != 1 {
		t.Errorf("expected 1 notification (failed, eligible for reconnect drain), got %d", len(pending))
	}
	if len(pending) > 0 && pending[0].Status != "failed" {
		t.Errorf("expected status 'failed', got '%s'", pending[0].Status)
	}

	// Verify it's marked as failed
	var status string
	query := `SELECT status FROM notifications WHERE id = ?`
	err = store.db.QueryRow(query, id).Scan(&status)
	if err != nil {
		t.Fatalf("failed to query status: %v", err)
	}

	if status != "failed" {
		t.Errorf("expected status=failed, got %s", status)
	}
}

func TestDeleteDevice_CascadesNotifications(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	store.SaveDevice("device-1", "Device 1", "", "", []string{"read"})

	payload := json.RawMessage(`{"msg":"test"}`)
	_, err := store.EnqueueNotification("device-1", "event", payload, 5*time.Minute, 3)
	if err != nil {
		t.Fatalf("EnqueueNotification failed: %v", err)
	}

	// Delete device
	err = store.DeleteDevice("device-1")
	if err != nil {
		t.Fatalf("DeleteDevice failed: %v", err)
	}

	// Verify notifications were deleted (cascade)
	pending, err := store.GetPendingNotifications("device-1")
	if err != nil {
		t.Fatalf("GetPendingNotifications failed: %v", err)
	}

	if len(pending) != 0 {
		t.Errorf("expected 0 notifications after device deletion, got %d", len(pending))
	}
}

func TestEnqueueNotification_PayloadJSON(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	store.SaveDevice("device-1", "Device 1", "", "", []string{"read"})

	// Test with complex JSON payload
	complexPayload := json.RawMessage(`{"config":{"server":"localhost","port":8080},"users":["alice","bob"]}`)

	id, err := store.EnqueueNotification("device-1", "config", complexPayload, 5*time.Minute, 3)
	if err != nil {
		t.Fatalf("EnqueueNotification failed: %v", err)
	}

	// Retrieve and verify payload
	pending, err := store.GetPendingNotifications("device-1")
	if err != nil {
		t.Fatalf("GetPendingNotifications failed: %v", err)
	}

	if len(pending) == 0 {
		t.Fatal("expected notification to be pending")
	}

	if string(pending[0].Payload) != string(complexPayload) {
		t.Errorf("payload mismatch:\nexpected: %s\ngot: %s", complexPayload, pending[0].Payload)
	}

	_ = id
}

func TestDeleteNotificationsForDevice(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	store.SaveDevice("revoke-device", "Revoke Device", "", "", []string{"read"})
	payload := json.RawMessage(`{}`)

	// Enqueue multiple notifications
	for i := 0; i < 5; i++ {
		_, err := store.EnqueueNotification("revoke-device", "test.type", payload, time.Hour, 3)
		if err != nil {
			t.Fatalf("EnqueueNotification failed: %v", err)
		}
	}

	// Verify 5 pending
	pending, _ := store.GetPendingNotifications("revoke-device")
	if len(pending) != 5 {
		t.Errorf("Expected 5 pending, got %d", len(pending))
	}

	// Delete notifications for device
	count, err := store.DeleteNotificationsForDevice("revoke-device")
	if err != nil {
		t.Fatalf("DeleteNotificationsForDevice failed: %v", err)
	}
	if count != 5 {
		t.Errorf("Expected 5 deleted, got %d", count)
	}

	// Verify 0 pending
	pending, _ = store.GetPendingNotifications("revoke-device")
	if len(pending) != 0 {
		t.Errorf("Expected 0 pending after delete, got %d", len(pending))
	}
}

func TestGetStaleNotifications(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	store.SaveDevice("stale-device", "Stale Device", "", "", []string{"read"})
	payload := json.RawMessage(`{}`)

	// Enqueue a notification
	_, err := store.EnqueueNotification("stale-device", "test.type", payload, 30*24*time.Hour, 3)
	if err != nil {
		t.Fatalf("EnqueueNotification failed: %v", err)
	}

	// With 1 second threshold from the future, everything is stale
	// (threshold = -1s means "created before 1 second from now")
	stale, err := store.GetStaleNotifications(-1 * time.Second)
	if err != nil {
		t.Fatalf("GetStaleNotifications failed: %v", err)
	}
	if len(stale) != 1 {
		t.Errorf("Expected 1 stale device, got %d", len(stale))
	}
	if len(stale) > 0 {
		if stale[0].DeviceID != "stale-device" {
			t.Errorf("Expected device stale-device, got %s", stale[0].DeviceID)
		}
		if stale[0].Count != 1 {
			t.Errorf("Expected count 1, got %d", stale[0].Count)
		}
	}

	// With 1 hour threshold, nothing is stale yet (just created)
	stale, err = store.GetStaleNotifications(time.Hour)
	if err != nil {
		t.Fatalf("GetStaleNotifications failed: %v", err)
	}
	if len(stale) != 0 {
		t.Errorf("Expected 0 stale with 1 hour threshold, got %d", len(stale))
	}
}

func TestGetNotificationQueueStats(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	store.SaveDevice("stats-device-1", "Stats Device 1", "", "", []string{"read"})
	store.SaveDevice("stats-device-2", "Stats Device 2", "", "", []string{"read"})
	store.SaveDevice("stats-device-3", "Stats Device 3", "", "", []string{"read"})
	payload := json.RawMessage(`{}`)

	// Enqueue some notifications
	id1, _ := store.EnqueueNotification("stats-device-1", "type1", payload, time.Hour, 3)
	id2, _ := store.EnqueueNotification("stats-device-2", "type2", payload, time.Hour, 3)
	store.EnqueueNotification("stats-device-3", "type3", payload, time.Hour, 3)

	// Mark one as delivered
	store.MarkNotificationDelivered(id1)

	// Mark one as failed via retry exhaustion
	for i := 0; i < 4; i++ {
		store.UpdateNotificationRetry(id2, "test error")
	}

	stats, err := store.GetNotificationQueueStats(time.Hour)
	if err != nil {
		t.Fatalf("GetNotificationQueueStats failed: %v", err)
	}

	if stats.TotalPending != 1 {
		t.Errorf("Expected 1 pending, got %d", stats.TotalPending)
	}
	if stats.TotalDelivered != 1 {
		t.Errorf("Expected 1 delivered, got %d", stats.TotalDelivered)
	}
	if stats.TotalFailed != 1 {
		t.Errorf("Expected 1 failed, got %d", stats.TotalFailed)
	}
}

func TestListNotifications(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	store.SaveDevice("list-device-a", "List Device A", "", "", []string{"read"})
	store.SaveDevice("list-device-b", "List Device B", "", "", []string{"read"})
	payload := json.RawMessage(`{}`)

	// Enqueue notifications for different devices and types
	store.EnqueueNotification("list-device-a", "type.alpha", payload, time.Hour, 3)
	store.EnqueueNotification("list-device-a", "type.beta", payload, time.Hour, 3)
	store.EnqueueNotification("list-device-b", "type.alpha", payload, time.Hour, 3)
	id, _ := store.EnqueueNotification("list-device-b", "type.beta", payload, time.Hour, 3)

	// Mark one as delivered
	store.MarkNotificationDelivered(id)

	// Test listing all
	result, err := store.ListNotifications(NotificationListFilter{}, time.Hour)
	if err != nil {
		t.Fatalf("ListNotifications failed: %v", err)
	}
	if result.Total != 4 {
		t.Errorf("Expected 4 total, got %d", result.Total)
	}

	// Test filtering by status
	result, err = store.ListNotifications(NotificationListFilter{Status: "pending"}, time.Hour)
	if err != nil {
		t.Fatalf("ListNotifications with status filter failed: %v", err)
	}
	if result.Total != 3 {
		t.Errorf("Expected 3 pending, got %d", result.Total)
	}

	// Test filtering by device
	result, err = store.ListNotifications(NotificationListFilter{DeviceID: "list-device-a"}, time.Hour)
	if err != nil {
		t.Fatalf("ListNotifications with device filter failed: %v", err)
	}
	if result.Total != 2 {
		t.Errorf("Expected 2 for list-device-a, got %d", result.Total)
	}

	// Test filtering by type
	result, err = store.ListNotifications(NotificationListFilter{Type: "type.alpha"}, time.Hour)
	if err != nil {
		t.Fatalf("ListNotifications with type filter failed: %v", err)
	}
	if result.Total != 2 {
		t.Errorf("Expected 2 for type.alpha, got %d", result.Total)
	}

	// Test pagination
	result, err = store.ListNotifications(NotificationListFilter{Limit: 2}, time.Hour)
	if err != nil {
		t.Fatalf("ListNotifications with limit failed: %v", err)
	}
	if len(result.Notifications) != 2 {
		t.Errorf("Expected 2 notifications with limit, got %d", len(result.Notifications))
	}
	if result.Total != 4 {
		t.Errorf("Total should still be 4, got %d", result.Total)
	}
}

func TestDismissNotification(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	store.SaveDevice("dismiss-device", "Dismiss Device", "", "", []string{"read"})
	payload := json.RawMessage(`{}`)
	id, _ := store.EnqueueNotification("dismiss-device", "test.type", payload, time.Hour, 3)

	// Dismiss the notification
	if err := store.DismissNotification(id); err != nil {
		t.Fatalf("DismissNotification failed: %v", err)
	}

	// Verify it's no longer pending
	pending, _ := store.GetPendingNotifications("dismiss-device")
	if len(pending) != 0 {
		t.Errorf("Expected 0 pending after dismiss, got %d", len(pending))
	}

	// Try to dismiss again - should fail
	if err := store.DismissNotification(id); err == nil {
		t.Error("Expected error when dismissing already dismissed notification")
	}
}

func TestDismissNotificationsForDevice(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	store.SaveDevice("bulk-dismiss-device", "Bulk Dismiss Device", "", "", []string{"read"})
	payload := json.RawMessage(`{}`)

	// Enqueue 3 notifications
	store.EnqueueNotification("bulk-dismiss-device", "type1", payload, time.Hour, 3)
	store.EnqueueNotification("bulk-dismiss-device", "type2", payload, time.Hour, 3)
	store.EnqueueNotification("bulk-dismiss-device", "type3", payload, time.Hour, 3)

	// Dismiss all for device
	count, err := store.DismissNotificationsForDevice("bulk-dismiss-device")
	if err != nil {
		t.Fatalf("DismissNotificationsForDevice failed: %v", err)
	}
	if count != 3 {
		t.Errorf("Expected 3 dismissed, got %d", count)
	}

	// Verify none pending
	pending, _ := store.GetPendingNotifications("bulk-dismiss-device")
	if len(pending) != 0 {
		t.Errorf("Expected 0 pending after dismiss, got %d", len(pending))
	}
}

func TestListNotifications_IsStaleFlag(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	store.SaveDevice("stale-flag-device", "Stale Flag Device", "", "", []string{"read"})
	payload := json.RawMessage(`{}`)
	store.EnqueueNotification("stale-flag-device", "test.type", payload, 30*24*time.Hour, 3)

	// With negative threshold (future), should be marked stale
	// Using -1s means thresholdTime = now + 1s, so all notifications created "before" that are stale
	result, err := store.ListNotifications(NotificationListFilter{DeviceID: "stale-flag-device"}, -1*time.Second)
	if err != nil {
		t.Fatalf("ListNotifications failed: %v", err)
	}
	if len(result.Notifications) != 1 {
		t.Fatalf("Expected 1 notification, got %d", len(result.Notifications))
	}
	if !result.Notifications[0].IsStale {
		t.Error("Expected notification to be marked as stale with negative threshold")
	}

	// With 1 hour threshold, should not be stale (just created)
	result, err = store.ListNotifications(NotificationListFilter{DeviceID: "stale-flag-device"}, time.Hour)
	if err != nil {
		t.Fatalf("ListNotifications failed: %v", err)
	}
	if result.Notifications[0].IsStale {
		t.Error("Expected notification to not be stale with 1 hour threshold")
	}
}
