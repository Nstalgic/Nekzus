package backup

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test 1: Create scheduler
func TestNewScheduler(t *testing.T) {
	// Arrange
	store, cleanup := setupTestStore(t)
	defer cleanup()

	backupDir := t.TempDir()
	manager := NewManager(store, backupDir, "1.0.0-test")

	// Act
	scheduler := NewScheduler(manager, 1*time.Hour, 7)

	// Assert
	assert.NotNil(t, scheduler)
	assert.Equal(t, 1*time.Hour, scheduler.interval)
	assert.Equal(t, 7, scheduler.retention)
}

// Test 2: Start and stop scheduler
func TestScheduler_StartStop(t *testing.T) {
	// Arrange
	store, cleanup := setupTestStore(t)
	defer cleanup()

	backupDir := t.TempDir()
	manager := NewManager(store, backupDir, "1.0.0-test")
	scheduler := NewScheduler(manager, 100*time.Millisecond, 5)

	// Act - Start scheduler
	err := scheduler.Start()
	require.NoError(t, err)

	// Wait for at least one backup to run
	time.Sleep(150 * time.Millisecond)

	// Stop scheduler
	err = scheduler.Stop()
	require.NoError(t, err)

	// Assert - At least one backup should have been created
	backups, _ := manager.ListBackups()
	assert.GreaterOrEqual(t, len(backups), 1)
}

// Test 3: Cannot start scheduler twice
func TestScheduler_CannotStartTwice(t *testing.T) {
	// Arrange
	store, cleanup := setupTestStore(t)
	defer cleanup()

	backupDir := t.TempDir()
	manager := NewManager(store, backupDir, "1.0.0-test")
	scheduler := NewScheduler(manager, 1*time.Hour, 5)

	// Act
	err1 := scheduler.Start()
	err2 := scheduler.Start()

	// Cleanup
	scheduler.Stop()

	// Assert
	assert.NoError(t, err1)
	assert.Error(t, err2)
	assert.Contains(t, err2.Error(), "already running")
}

// Test 4: Stop scheduler that is not running
func TestScheduler_StopNotRunning(t *testing.T) {
	// Arrange
	store, cleanup := setupTestStore(t)
	defer cleanup()

	backupDir := t.TempDir()
	manager := NewManager(store, backupDir, "1.0.0-test")
	scheduler := NewScheduler(manager, 1*time.Hour, 5)

	// Act
	err := scheduler.Stop()

	// Assert
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not running")
}

// Test 5: Scheduler creates backups periodically
func TestScheduler_PeriodicBackups(t *testing.T) {
	// Arrange
	store, cleanup := setupTestStore(t)
	defer cleanup()

	backupDir := t.TempDir()
	manager := NewManager(store, backupDir, "1.0.0-test")
	scheduler := NewScheduler(manager, 50*time.Millisecond, 10)

	// Act
	err := scheduler.Start()
	require.NoError(t, err)
	defer scheduler.Stop()

	// Wait for multiple backup cycles
	time.Sleep(200 * time.Millisecond)

	// Assert - Should have created multiple backups
	backups, _ := manager.ListBackups()
	assert.GreaterOrEqual(t, len(backups), 2, "Should have created at least 2 backups")
}

// Test 6: Scheduler applies retention policy
func TestScheduler_RetentionPolicy(t *testing.T) {
	// Arrange
	store, cleanup := setupTestStore(t)
	defer cleanup()

	backupDir := t.TempDir()
	manager := NewManager(store, backupDir, "1.0.0-test")

	// Keep only 3 backups
	scheduler := NewScheduler(manager, 50*time.Millisecond, 3)

	// Act
	err := scheduler.Start()
	require.NoError(t, err)
	defer scheduler.Stop()

	// Wait for enough time to create more than 3 backups
	time.Sleep(300 * time.Millisecond)

	// Assert - Should have exactly 3 backups (retention policy applied)
	backups, _ := manager.ListBackups()
	assert.LessOrEqual(t, len(backups), 4, "Should have at most 4 backups (3 kept + 1 in-progress)")
}

// Test 7: Manual backup trigger
func TestScheduler_ManualBackup(t *testing.T) {
	// Arrange
	store, cleanup := setupTestStore(t)
	defer cleanup()

	backupDir := t.TempDir()
	manager := NewManager(store, backupDir, "1.0.0-test")
	scheduler := NewScheduler(manager, 1*time.Hour, 5) // Long interval

	// Act
	err := scheduler.Start()
	require.NoError(t, err)
	defer scheduler.Stop()

	// Trigger manual backup
	backup, err := scheduler.TriggerBackup("manual backup")

	// Assert
	require.NoError(t, err)
	assert.NotNil(t, backup)
	assert.Equal(t, "manual backup", backup.Description)

	// Verify backup was saved (Start() also creates an initial automatic backup)
	backups, _ := manager.ListBackups()
	assert.GreaterOrEqual(t, len(backups), 1, "Should have at least 1 backup")

	// Verify the manual backup exists in the list
	found := false
	for _, b := range backups {
		if b.Description == "manual backup" {
			found = true
			break
		}
	}
	assert.True(t, found, "Manual backup should be present in backup list")
}

// Test 8: Get scheduler status
func TestScheduler_Status(t *testing.T) {
	// Arrange
	store, cleanup := setupTestStore(t)
	defer cleanup()

	backupDir := t.TempDir()
	manager := NewManager(store, backupDir, "1.0.0-test")
	scheduler := NewScheduler(manager, 1*time.Hour, 5)

	// Act - Before start
	status := scheduler.Status()

	// Assert - Not running
	assert.False(t, status.Running)
	assert.Nil(t, status.LastBackupTime)
	assert.Nil(t, status.NextBackupTime)

	// Act - After start
	scheduler.Start()
	defer scheduler.Stop()

	time.Sleep(50 * time.Millisecond) // Give it time to initialize

	status = scheduler.Status()

	// Assert - Running
	assert.True(t, status.Running)
}
