package storage

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test 3.1: Database Schema
func TestAuditLogsTableExists(t *testing.T) {
	// Arrange
	store, cleanup := setupTestStore(t)
	defer cleanup()

	// Act
	var exists int
	err := store.DB().QueryRow(`
		SELECT COUNT(*) FROM sqlite_master
		WHERE type='table' AND name='audit_logs'
	`).Scan(&exists)

	// Assert
	assert.NoError(t, err)
	assert.Equal(t, 1, exists, "audit_logs table should exist")
}

func TestAuditLogsIndexes(t *testing.T) {
	// Arrange
	store, cleanup := setupTestStore(t)
	defer cleanup()

	// Act
	rows, err := store.DB().Query(`
		SELECT name FROM sqlite_master
		WHERE type='index' AND tbl_name='audit_logs'
	`)
	require.NoError(t, err)
	defer rows.Close()

	indexes := []string{}
	for rows.Next() {
		var name string
		rows.Scan(&name)
		indexes = append(indexes, name)
	}

	// Assert
	assert.Contains(t, indexes, "idx_audit_logs_timestamp")
	assert.Contains(t, indexes, "idx_audit_logs_action")
	assert.Contains(t, indexes, "idx_audit_logs_actor")
}

// Test 3.2: LogAuditEvent - Insert event
func TestLogAuditEvent_Insert(t *testing.T) {
	// Arrange
	store, cleanup := setupTestStore(t)
	defer cleanup()

	event := AuditEvent{
		Timestamp: time.Now(),
		Action:    ActionDevicePaired,
		ActorID:   "device_123",
		ActorIP:   "192.168.1.100",
		TargetID:  "device_123",
		Details: map[string]interface{}{
			"platform": "ios",
		},
		Success: true,
	}

	// Act
	err := store.LogAuditEvent(event)

	// Assert
	assert.NoError(t, err)

	// Verify record created
	var count int
	err = store.DB().QueryRow(`SELECT COUNT(*) FROM audit_logs`).Scan(&count)
	assert.NoError(t, err)
	assert.Equal(t, 1, count)
}

// Test 3.3: LogAuditEvent - Multiple events
func TestLogAuditEvent_Multiple(t *testing.T) {
	// Arrange
	store, cleanup := setupTestStore(t)
	defer cleanup()

	events := []AuditEvent{
		{
			Timestamp: time.Now(),
			Action:    ActionDevicePaired,
			ActorID:   "device_1",
			ActorIP:   "192.168.1.1",
			TargetID:  "device_1",
			Success:   true,
		},
		{
			Timestamp: time.Now(),
			Action:    ActionDeviceRevoked,
			ActorID:   "admin",
			ActorIP:   "192.168.1.2",
			TargetID:  "device_2",
			Success:   true,
		},
		{
			Timestamp: time.Now(),
			Action:    ActionConfigReloaded,
			ActorID:   "system",
			ActorIP:   "localhost",
			TargetID:  "config",
			Success:   false,
			Error:     "validation failed",
		},
	}

	// Act
	for _, event := range events {
		err := store.LogAuditEvent(event)
		require.NoError(t, err)
	}

	// Assert
	var count int
	err := store.DB().QueryRow(`SELECT COUNT(*) FROM audit_logs`).Scan(&count)
	assert.NoError(t, err)
	assert.Equal(t, 3, count)
}

// Test 3.4: ListAuditLogs - Empty database
func TestListAuditLogs_Empty(t *testing.T) {
	// Arrange
	store, cleanup := setupTestStore(t)
	defer cleanup()

	// Act
	logs, err := store.ListAuditLogs(100, 0)

	// Assert
	assert.NoError(t, err)
	assert.Empty(t, logs)
}

// Test 3.5: ListAuditLogs - With events
func TestListAuditLogs_WithEvents(t *testing.T) {
	// Arrange
	store, cleanup := setupTestStore(t)
	defer cleanup()

	// Insert events
	for i := 0; i < 5; i++ {
		event := AuditEvent{
			Timestamp: time.Now().Add(time.Duration(i) * time.Second),
			Action:    ActionDevicePaired,
			ActorID:   "device_" + string(rune('1'+i)),
			ActorIP:   "192.168.1.1",
			TargetID:  "device_" + string(rune('1'+i)),
			Success:   true,
		}
		require.NoError(t, store.LogAuditEvent(event))
	}

	// Act
	logs, err := store.ListAuditLogs(100, 0)

	// Assert
	assert.NoError(t, err)
	assert.Len(t, logs, 5)
}

// Test 3.6: ListAuditLogs - Ordered by timestamp DESC
func TestListAuditLogs_OrderedByTimestamp(t *testing.T) {
	// Arrange
	store, cleanup := setupTestStore(t)
	defer cleanup()

	// Insert events with different timestamps
	baseTime := time.Now()
	events := []AuditEvent{
		{Timestamp: baseTime.Add(-3 * time.Hour), Action: ActionDevicePaired, ActorID: "old", ActorIP: "127.0.0.1", TargetID: "old", Success: true},
		{Timestamp: baseTime, Action: ActionDeviceRevoked, ActorID: "new", ActorIP: "127.0.0.1", TargetID: "new", Success: true},
		{Timestamp: baseTime.Add(-1 * time.Hour), Action: ActionConfigReloaded, ActorID: "mid", ActorIP: "127.0.0.1", TargetID: "mid", Success: true},
	}

	for _, event := range events {
		require.NoError(t, store.LogAuditEvent(event))
	}

	// Act
	logs, err := store.ListAuditLogs(100, 0)

	// Assert
	assert.NoError(t, err)
	require.Len(t, logs, 3)
	// Should be ordered newest first
	assert.Equal(t, "new", logs[0].ActorID)
	assert.Equal(t, "mid", logs[1].ActorID)
	assert.Equal(t, "old", logs[2].ActorID)
}

// Test 3.7: ListAuditLogs - Pagination limit
func TestListAuditLogs_PaginationLimit(t *testing.T) {
	// Arrange
	store, cleanup := setupTestStore(t)
	defer cleanup()

	// Insert 10 events
	for i := 0; i < 10; i++ {
		event := AuditEvent{
			Timestamp: time.Now().Add(time.Duration(i) * time.Second),
			Action:    ActionDevicePaired,
			ActorID:   "device",
			ActorIP:   "127.0.0.1",
			TargetID:  "device",
			Success:   true,
		}
		require.NoError(t, store.LogAuditEvent(event))
	}

	// Act
	logs, err := store.ListAuditLogs(5, 0)

	// Assert
	assert.NoError(t, err)
	assert.Len(t, logs, 5, "Should return only 5 logs when limit is 5")
}

// Test 3.8: ListAuditLogs - Pagination offset
func TestListAuditLogs_PaginationOffset(t *testing.T) {
	// Arrange
	store, cleanup := setupTestStore(t)
	defer cleanup()

	// Insert 10 events
	for i := 0; i < 10; i++ {
		event := AuditEvent{
			Timestamp: time.Now().Add(time.Duration(i) * time.Second),
			Action:    ActionDevicePaired,
			ActorID:   "device_" + string(rune('0'+i)),
			ActorIP:   "127.0.0.1",
			TargetID:  "device",
			Success:   true,
		}
		require.NoError(t, store.LogAuditEvent(event))
	}

	// Act - Get page 2 (skip first 5, get next 5)
	logs, err := store.ListAuditLogs(5, 5)

	// Assert
	assert.NoError(t, err)
	assert.Len(t, logs, 5, "Should return 5 logs for second page")
}

// Test 3.9: ListAuditLogs - Filter by action
func TestListAuditLogsByAction(t *testing.T) {
	// Arrange
	store, cleanup := setupTestStore(t)
	defer cleanup()

	// Insert different action types
	events := []AuditEvent{
		{Timestamp: time.Now(), Action: ActionDevicePaired, ActorID: "a1", ActorIP: "127.0.0.1", TargetID: "t1", Success: true},
		{Timestamp: time.Now(), Action: ActionDevicePaired, ActorID: "a2", ActorIP: "127.0.0.1", TargetID: "t2", Success: true},
		{Timestamp: time.Now(), Action: ActionDeviceRevoked, ActorID: "a3", ActorIP: "127.0.0.1", TargetID: "t3", Success: true},
		{Timestamp: time.Now(), Action: ActionConfigReloaded, ActorID: "a4", ActorIP: "127.0.0.1", TargetID: "t4", Success: true},
	}

	for _, event := range events {
		require.NoError(t, store.LogAuditEvent(event))
	}

	// Act
	logs, err := store.ListAuditLogsByAction(ActionDevicePaired, 100, 0)

	// Assert
	assert.NoError(t, err)
	assert.Len(t, logs, 2, "Should only return device.paired events")
	for _, log := range logs {
		assert.Equal(t, ActionDevicePaired, log.Action)
	}
}

// Test 3.10: ListAuditLogs - Filter by actor
func TestListAuditLogsByActor(t *testing.T) {
	// Arrange
	store, cleanup := setupTestStore(t)
	defer cleanup()

	// Insert events from different actors
	events := []AuditEvent{
		{Timestamp: time.Now(), Action: ActionDevicePaired, ActorID: "device_123", ActorIP: "127.0.0.1", TargetID: "t1", Success: true},
		{Timestamp: time.Now(), Action: ActionDeviceRevoked, ActorID: "device_123", ActorIP: "127.0.0.1", TargetID: "t2", Success: true},
		{Timestamp: time.Now(), Action: ActionConfigReloaded, ActorID: "admin", ActorIP: "127.0.0.1", TargetID: "t3", Success: true},
	}

	for _, event := range events {
		require.NoError(t, store.LogAuditEvent(event))
	}

	// Act
	logs, err := store.ListAuditLogsByActor("device_123", 100, 0)

	// Assert
	assert.NoError(t, err)
	assert.Len(t, logs, 2, "Should only return events from device_123")
	for _, log := range logs {
		assert.Equal(t, "device_123", log.ActorID)
	}
}

// Test 3.11: LogAuditEvent - Handles details JSON
func TestLogAuditEvent_JSONDetails(t *testing.T) {
	// Arrange
	store, cleanup := setupTestStore(t)
	defer cleanup()

	event := AuditEvent{
		Timestamp: time.Now(),
		Action:    ActionConfigReloaded,
		ActorID:   "system",
		ActorIP:   "localhost",
		TargetID:  "config",
		Details: map[string]interface{}{
			"routes": 5,
			"apps":   3,
			"nested": map[string]interface{}{
				"key": "value",
			},
		},
		Success: true,
	}

	// Act
	err := store.LogAuditEvent(event)
	require.NoError(t, err)

	// Retrieve and verify
	logs, err := store.ListAuditLogs(1, 0)

	// Assert
	assert.NoError(t, err)
	require.Len(t, logs, 1)
	assert.NotNil(t, logs[0].Details)
	assert.Equal(t, float64(5), logs[0].Details["routes"]) // JSON numbers become float64
	assert.Equal(t, float64(3), logs[0].Details["apps"])
}

// Test 3.12: LogAuditEvent - Handles error message
func TestLogAuditEvent_ErrorMessage(t *testing.T) {
	// Arrange
	store, cleanup := setupTestStore(t)
	defer cleanup()

	event := AuditEvent{
		Timestamp: time.Now(),
		Action:    ActionDeviceRevoked,
		ActorID:   "admin",
		ActorIP:   "192.168.1.1",
		TargetID:  "device_bad",
		Success:   false,
		Error:     "device not found",
	}

	// Act
	err := store.LogAuditEvent(event)
	require.NoError(t, err)

	// Retrieve and verify
	logs, err := store.ListAuditLogs(1, 0)

	// Assert
	assert.NoError(t, err)
	require.Len(t, logs, 1)
	assert.False(t, logs[0].Success)
	assert.Equal(t, "device not found", logs[0].Error)
}
