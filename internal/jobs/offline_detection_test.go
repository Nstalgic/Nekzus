package jobs

import (
	"sync"
	"testing"
	"time"

	"github.com/nstalgic/nekzus/internal/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockMetrics for testing
type MockMetrics struct {
	mu             sync.Mutex
	DevicesOnline  float64
	DevicesOffline float64
}

func (m *MockMetrics) SetDevicesOnline(count float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.DevicesOnline = count
}

func (m *MockMetrics) SetDevicesOffline(count float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.DevicesOffline = count
}

func (m *MockMetrics) GetDevicesOnline() float64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.DevicesOnline
}

func (m *MockMetrics) GetDevicesOffline() float64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.DevicesOffline
}

// Test 4.1: Job creation
func TestNewOfflineDetectionJob(t *testing.T) {
	// Arrange
	store := &storage.Store{}
	metrics := &MockMetrics{}
	threshold := 5 * time.Minute
	interval := 30 * time.Second

	// Act
	job := NewOfflineDetectionJob(store, metrics, threshold, interval)

	// Assert
	assert.NotNil(t, job)
	assert.Equal(t, store, job.storage)
	assert.Equal(t, metrics, job.metrics)
	assert.Equal(t, threshold, job.offlineThreshold)
	assert.Equal(t, interval, job.interval)
}

// Test 4.2: Empty database - all metrics zero
func TestOfflineDetectionJob_EmptyDatabase(t *testing.T) {
	// Arrange
	store, cleanup := setupTestStore(t)
	defer cleanup()

	metrics := &MockMetrics{}
	threshold := 5 * time.Minute
	job := NewOfflineDetectionJob(store, metrics, threshold, 30*time.Second)

	// Act
	err := job.Run()

	// Assert
	assert.NoError(t, err)
	assert.Equal(t, float64(0), metrics.GetDevicesOnline())
	assert.Equal(t, float64(0), metrics.GetDevicesOffline())
}

// Test 4.3: All devices online
func TestOfflineDetectionJob_AllDevicesOnline(t *testing.T) {
	// Arrange
	store, cleanup := setupTestStore(t)
	defer cleanup()

	// Create 3 devices with recent last_seen
	now := time.Now()
	createDeviceWithLastSeen(t, store, "device_1", now.Add(-1*time.Minute))
	createDeviceWithLastSeen(t, store, "device_2", now.Add(-2*time.Minute))
	createDeviceWithLastSeen(t, store, "device_3", now.Add(-3*time.Minute))

	metrics := &MockMetrics{}
	threshold := 5 * time.Minute // All devices are within threshold
	job := NewOfflineDetectionJob(store, metrics, threshold, 30*time.Second)

	// Act
	err := job.Run()

	// Assert
	assert.NoError(t, err)
	assert.Equal(t, float64(3), metrics.GetDevicesOnline())
	assert.Equal(t, float64(0), metrics.GetDevicesOffline())
}

// Test 4.4: All devices offline
func TestOfflineDetectionJob_AllDevicesOffline(t *testing.T) {
	// Arrange
	store, cleanup := setupTestStore(t)
	defer cleanup()

	// Create 3 devices with old last_seen
	now := time.Now()
	createDeviceWithLastSeen(t, store, "device_1", now.Add(-10*time.Minute))
	createDeviceWithLastSeen(t, store, "device_2", now.Add(-15*time.Minute))
	createDeviceWithLastSeen(t, store, "device_3", now.Add(-20*time.Minute))

	metrics := &MockMetrics{}
	threshold := 5 * time.Minute // All devices exceed threshold
	job := NewOfflineDetectionJob(store, metrics, threshold, 30*time.Second)

	// Act
	err := job.Run()

	// Assert
	assert.NoError(t, err)
	assert.Equal(t, float64(0), metrics.GetDevicesOnline())
	assert.Equal(t, float64(3), metrics.GetDevicesOffline())
}

// Test 4.5: Mixed online/offline devices
func TestOfflineDetectionJob_MixedStatus(t *testing.T) {
	// Arrange
	store, cleanup := setupTestStore(t)
	defer cleanup()

	// Create 5 devices: 3 online, 2 offline
	now := time.Now()
	createDeviceWithLastSeen(t, store, "device_1", now.Add(-1*time.Minute))  // online
	createDeviceWithLastSeen(t, store, "device_2", now.Add(-10*time.Minute)) // offline
	createDeviceWithLastSeen(t, store, "device_3", now.Add(-3*time.Minute))  // online
	createDeviceWithLastSeen(t, store, "device_4", now.Add(-15*time.Minute)) // offline
	createDeviceWithLastSeen(t, store, "device_5", now.Add(-2*time.Minute))  // online

	metrics := &MockMetrics{}
	threshold := 5 * time.Minute
	job := NewOfflineDetectionJob(store, metrics, threshold, 30*time.Second)

	// Act
	err := job.Run()

	// Assert
	assert.NoError(t, err)
	assert.Equal(t, float64(3), metrics.GetDevicesOnline())
	assert.Equal(t, float64(2), metrics.GetDevicesOffline())
}

// Test 4.6: Threshold boundary condition
func TestOfflineDetectionJob_ThresholdBoundary(t *testing.T) {
	// Arrange
	store, cleanup := setupTestStore(t)
	defer cleanup()

	threshold := 5 * time.Minute
	now := time.Now()

	// Device well within threshold (clearly online)
	createDeviceWithLastSeen(t, store, "device_1", now.Add(-threshold+10*time.Second))
	// Device well past threshold (clearly offline)
	createDeviceWithLastSeen(t, store, "device_2", now.Add(-threshold-10*time.Second))
	// Device just before threshold (should be online)
	createDeviceWithLastSeen(t, store, "device_3", now.Add(-threshold+1*time.Minute))

	metrics := &MockMetrics{}
	job := NewOfflineDetectionJob(store, metrics, threshold, 30*time.Second)

	// Act
	err := job.Run()

	// Assert
	assert.NoError(t, err)
	// Devices within threshold should be online, one past should be offline
	assert.Equal(t, float64(2), metrics.GetDevicesOnline())  // device_1 and device_3
	assert.Equal(t, float64(1), metrics.GetDevicesOffline()) // device_2
}

// Test 4.7: Start and stop job
func TestOfflineDetectionJob_StartStop(t *testing.T) {
	// Arrange
	store, cleanup := setupTestStore(t)
	defer cleanup()

	metrics := &MockMetrics{}
	threshold := 5 * time.Minute
	interval := 100 * time.Millisecond // Short interval for testing
	job := NewOfflineDetectionJob(store, metrics, threshold, interval)

	// Act - Start job
	err := job.Start()
	require.NoError(t, err)

	// Give it time to run at least once
	time.Sleep(150 * time.Millisecond)

	// Stop job
	err = job.Stop()
	require.NoError(t, err)

	// Assert - Job should have run and stopped cleanly
	// No specific assertions, just verify no panics
}

// Test 4.8: Cannot start job twice
func TestOfflineDetectionJob_CannotStartTwice(t *testing.T) {
	// Arrange
	store, cleanup := setupTestStore(t)
	defer cleanup()

	metrics := &MockMetrics{}
	job := NewOfflineDetectionJob(store, metrics, 5*time.Minute, 1*time.Second)

	// Act
	err1 := job.Start()
	err2 := job.Start()

	// Cleanup
	job.Stop()

	// Assert
	assert.NoError(t, err1)
	assert.Error(t, err2, "Should not be able to start job twice")
	assert.Contains(t, err2.Error(), "already running")
}

// Test 4.9: Stop job that is not running
func TestOfflineDetectionJob_StopNotRunning(t *testing.T) {
	// Arrange
	store, cleanup := setupTestStore(t)
	defer cleanup()

	metrics := &MockMetrics{}
	job := NewOfflineDetectionJob(store, metrics, 5*time.Minute, 1*time.Second)

	// Act
	err := job.Stop()

	// Assert
	assert.Error(t, err, "Should error when stopping job that isn't running")
	assert.Contains(t, err.Error(), "not running")
}

// Test 4.10: Job runs periodically
func TestOfflineDetectionJob_RunsPeriodically(t *testing.T) {
	// Arrange
	store, cleanup := setupTestStore(t)
	defer cleanup()

	// Create device
	now := time.Now()
	createDeviceWithLastSeen(t, store, "device_1", now.Add(-1*time.Minute))

	metrics := &MockMetrics{}
	threshold := 5 * time.Minute
	interval := 50 * time.Millisecond // Very short interval
	job := NewOfflineDetectionJob(store, metrics, threshold, interval)

	// Act - Start job
	err := job.Start()
	require.NoError(t, err)
	defer job.Stop()

	// Wait for multiple runs
	time.Sleep(150 * time.Millisecond) // Should run 2-3 times

	// Assert - Metrics should be updated (proves job ran)
	assert.Equal(t, float64(1), metrics.GetDevicesOnline())
	assert.Equal(t, float64(0), metrics.GetDevicesOffline())
}

// Test 4.11: Concurrent safety
func TestOfflineDetectionJob_ConcurrentSafety(t *testing.T) {
	// Arrange
	store, cleanup := setupTestStore(t)
	defer cleanup()

	metrics := &MockMetrics{}
	threshold := 5 * time.Minute
	interval := 10 * time.Millisecond
	job := NewOfflineDetectionJob(store, metrics, threshold, interval)

	// Act - Start job and let it run while we read metrics
	err := job.Start()
	require.NoError(t, err)
	defer job.Stop()

	// Concurrently read metrics while job is updating them
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				_ = metrics.GetDevicesOnline()
				_ = metrics.GetDevicesOffline()
				time.Sleep(5 * time.Millisecond)
			}
		}()
	}

	wg.Wait()

	// Assert - No race conditions (test with -race flag)
	// If there are race conditions, -race flag will catch them
}

// Helper: setup test store
func setupTestStore(t *testing.T) (*storage.Store, func()) {
	t.Helper()
	store, err := storage.NewStore(storage.Config{
		DatabasePath: ":memory:",
	})
	require.NoError(t, err)
	return store, func() { store.Close() }
}

// Helper: create device with specific last_seen time
func createDeviceWithLastSeen(t *testing.T, store *storage.Store, deviceID string, lastSeen time.Time) {
	t.Helper()

	// Create device first
	err := store.SaveDevice(deviceID, "Test Device", "ios", "17.0", []string{"routes:read"})
	require.NoError(t, err)

	// Update last_seen directly in database
	_, err = store.DB().Exec(`UPDATE devices SET last_seen = ? WHERE device_id = ?`, lastSeen, deviceID)
	require.NoError(t, err)
}
