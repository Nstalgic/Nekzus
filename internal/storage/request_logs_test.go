package storage

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper function to create a test device
func createTestDevice(t *testing.T, store *Store, deviceID string) {
	err := store.SaveDevice(deviceID, "Test Device", "ios", "17.0", []string{"routes:read"})
	require.NoError(t, err, "Failed to create test device")
}

// Test 1.1: Database Schema
func TestRequestLogsTableExists(t *testing.T) {
	// Arrange
	store, cleanup := setupTestStore(t)
	defer cleanup()

	// Act
	var exists int
	err := store.DB().QueryRow(`
		SELECT COUNT(*) FROM sqlite_master
		WHERE type='table' AND name='request_logs'
	`).Scan(&exists)

	// Assert
	assert.NoError(t, err)
	assert.Equal(t, 1, exists, "request_logs table should exist")
}

func TestRequestLogsIndexes(t *testing.T) {
	// Arrange
	store, cleanup := setupTestStore(t)
	defer cleanup()

	// Act
	rows, err := store.DB().Query(`
		SELECT name FROM sqlite_master
		WHERE type='index' AND tbl_name='request_logs'
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
	assert.Contains(t, indexes, "idx_request_logs_device_date")
	assert.Contains(t, indexes, "idx_request_logs_date")
}

// Test 1.2: IncrementRequestCount - First Request
func TestIncrementRequestCount_FirstRequest(t *testing.T) {
	// Arrange
	store, cleanup := setupTestStore(t)
	defer cleanup()

	deviceID := "device_123"
	createTestDevice(t, store, deviceID)

	latency := 50 * time.Millisecond
	bytes := int64(1024)
	isError := false

	// Act
	err := store.IncrementRequestCount(deviceID, latency, bytes, isError)

	// Assert
	assert.NoError(t, err)

	// Verify record created
	var count, errorCount int
	var bytesTransferred int64
	var avgLatency float64

	err = store.DB().QueryRow(`
		SELECT request_count, bytes_transferred, avg_latency_ms, error_count
		FROM request_logs
		WHERE device_id = ? AND date = ?
	`, deviceID, time.Now().Format("2006-01-02")).Scan(
		&count, &bytesTransferred, &avgLatency, &errorCount,
	)

	assert.NoError(t, err)
	assert.Equal(t, 1, count)
	assert.Equal(t, int64(1024), bytesTransferred)
	assert.InDelta(t, 50.0, avgLatency, 1.0) // Allow 1ms delta
	assert.Equal(t, 0, errorCount)
}

// Test 1.3: IncrementRequestCount - Multiple Requests
func TestIncrementRequestCount_MultipleRequests(t *testing.T) {
	// Arrange
	store, cleanup := setupTestStore(t)
	defer cleanup()

	deviceID := "device_123"
	createTestDevice(t, store, deviceID)

	// Act - Make 3 requests
	err1 := store.IncrementRequestCount(deviceID, 50*time.Millisecond, 1024, false)
	err2 := store.IncrementRequestCount(deviceID, 100*time.Millisecond, 2048, false)
	err3 := store.IncrementRequestCount(deviceID, 75*time.Millisecond, 512, true) // Error request

	// Assert
	assert.NoError(t, err1)
	assert.NoError(t, err2)
	assert.NoError(t, err3)

	// Verify aggregation
	var count, errorCount int
	var bytesTransferred int64
	var avgLatency float64

	err := store.DB().QueryRow(`
		SELECT request_count, bytes_transferred, avg_latency_ms, error_count
		FROM request_logs
		WHERE device_id = ? AND date = ?
	`, deviceID, time.Now().Format("2006-01-02")).Scan(
		&count, &bytesTransferred, &avgLatency, &errorCount,
	)

	assert.NoError(t, err)
	assert.Equal(t, 3, count)
	assert.Equal(t, int64(3584), bytesTransferred) // 1024 + 2048 + 512
	assert.InDelta(t, 75.0, avgLatency, 1.0)       // (50 + 100 + 75) / 3 = 75
	assert.Equal(t, 1, errorCount)
}

// Test 1.4: GetDeviceRequestsToday - No Requests
func TestGetDeviceRequestsToday_NoRequests(t *testing.T) {
	// Arrange
	store, cleanup := setupTestStore(t)
	defer cleanup()

	deviceID := "device_123"

	// Act
	count, err := store.GetDeviceRequestsToday(deviceID)

	// Assert
	assert.NoError(t, err)
	assert.Equal(t, 0, count)
}

// Test 1.5: GetDeviceRequestsToday - With Requests
func TestGetDeviceRequestsToday_WithRequests(t *testing.T) {
	// Arrange
	store, cleanup := setupTestStore(t)
	defer cleanup()

	deviceID := "device_123"
	createTestDevice(t, store, deviceID)

	// Make 5 requests
	for i := 0; i < 5; i++ {
		err := store.IncrementRequestCount(deviceID, 50*time.Millisecond, 1024, false)
		require.NoError(t, err)
	}

	// Act
	count, err := store.GetDeviceRequestsToday(deviceID)

	// Assert
	assert.NoError(t, err)
	assert.Equal(t, 5, count)
}

// Test 1.6: GetDeviceRequestsToday - Only Today's Requests
func TestGetDeviceRequestsToday_OnlyToday(t *testing.T) {
	// Arrange
	store, cleanup := setupTestStore(t)
	defer cleanup()

	deviceID := "device_123"
	createTestDevice(t, store, deviceID)

	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")

	// Insert yesterday's requests
	_, err := store.DB().Exec(`
		INSERT INTO request_logs (device_id, date, request_count, bytes_transferred, avg_latency_ms, error_count)
		VALUES (?, ?, 10, 10240, 50.0, 0)
	`, deviceID, yesterday)
	require.NoError(t, err)

	// Insert today's requests
	err = store.IncrementRequestCount(deviceID, 50*time.Millisecond, 1024, false)
	require.NoError(t, err)
	err = store.IncrementRequestCount(deviceID, 50*time.Millisecond, 1024, false)
	require.NoError(t, err)

	// Act
	count, err := store.GetDeviceRequestsToday(deviceID)

	// Assert
	assert.NoError(t, err)
	assert.Equal(t, 2, count, "Should only count today's requests")
}

// Test 1.7: ListDevices - Includes RequestsToday
func TestListDevices_IncludesRequestsToday(t *testing.T) {
	// Arrange
	store, cleanup := setupTestStore(t)
	defer cleanup()

	// Create device
	deviceID := "device_123"
	createTestDevice(t, store, deviceID)

	// Add requests
	for i := 0; i < 3; i++ {
		err := store.IncrementRequestCount(deviceID, 50*time.Millisecond, 1024, false)
		require.NoError(t, err)
	}

	// Act
	devices, err := store.ListDevices()

	// Assert
	assert.NoError(t, err)
	require.Len(t, devices, 1)
	assert.Equal(t, 3, devices[0].RequestsToday)
}

// Test 1.8: Concurrent Request Tracking
func TestIncrementRequestCount_Concurrent(t *testing.T) {
	// Arrange
	store, cleanup := setupTestStore(t)
	defer cleanup()

	deviceID := "device_123"
	createTestDevice(t, store, deviceID)

	numGoroutines := 100

	// Act - Concurrent increments
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			store.IncrementRequestCount(deviceID, 50*time.Millisecond, 1024, false)
		}()
	}

	wg.Wait()

	// Assert
	count, err := store.GetDeviceRequestsToday(deviceID)
	assert.NoError(t, err)
	assert.Equal(t, numGoroutines, count, "All concurrent requests should be counted")
}

// Test 1.9: Multiple Devices
func TestIncrementRequestCount_MultipleDevices(t *testing.T) {
	// Arrange
	store, cleanup := setupTestStore(t)
	defer cleanup()

	// Act - Track requests for 3 different devices
	for i := 1; i <= 3; i++ {
		deviceID := fmt.Sprintf("device_%d", i)
		createTestDevice(t, store, deviceID)
		for j := 0; j < i; j++ { // device_1 gets 1 request, device_2 gets 2, etc.
			err := store.IncrementRequestCount(deviceID, 50*time.Millisecond, 1024, false)
			require.NoError(t, err)
		}
	}

	// Assert
	count1, _ := store.GetDeviceRequestsToday("device_1")
	count2, _ := store.GetDeviceRequestsToday("device_2")
	count3, _ := store.GetDeviceRequestsToday("device_3")

	assert.Equal(t, 1, count1)
	assert.Equal(t, 2, count2)
	assert.Equal(t, 3, count3)
}

// Test 1.10: Average Latency Calculation
func TestIncrementRequestCount_AverageLatencyCalculation(t *testing.T) {
	// Arrange
	store, cleanup := setupTestStore(t)
	defer cleanup()

	deviceID := "device_123"
	createTestDevice(t, store, deviceID)

	// Act - Track requests with different latencies
	latencies := []time.Duration{
		10 * time.Millisecond,
		20 * time.Millisecond,
		30 * time.Millisecond,
		40 * time.Millisecond,
	}

	for _, latency := range latencies {
		err := store.IncrementRequestCount(deviceID, latency, 1024, false)
		require.NoError(t, err)
	}

	// Assert
	var avgLatency float64
	err := store.DB().QueryRow(`
		SELECT avg_latency_ms FROM request_logs
		WHERE device_id = ? AND date = ?
	`, deviceID, time.Now().Format("2006-01-02")).Scan(&avgLatency)

	assert.NoError(t, err)
	// Average should be (10 + 20 + 30 + 40) / 4 = 25
	assert.InDelta(t, 25.0, avgLatency, 1.0)
}

// Test 1.11: Error Count Tracking
func TestIncrementRequestCount_ErrorCounting(t *testing.T) {
	// Arrange
	store, cleanup := setupTestStore(t)
	defer cleanup()

	deviceID := "device_123"
	createTestDevice(t, store, deviceID)

	// Act - Track 5 successful and 3 error requests
	for i := 0; i < 5; i++ {
		err := store.IncrementRequestCount(deviceID, 50*time.Millisecond, 1024, false)
		require.NoError(t, err)
	}

	for i := 0; i < 3; i++ {
		err := store.IncrementRequestCount(deviceID, 50*time.Millisecond, 1024, true)
		require.NoError(t, err)
	}

	// Assert
	var errorCount int
	err := store.DB().QueryRow(`
		SELECT error_count FROM request_logs
		WHERE device_id = ? AND date = ?
	`, deviceID, time.Now().Format("2006-01-02")).Scan(&errorCount)

	assert.NoError(t, err)
	assert.Equal(t, 3, errorCount)
}

// Test 1.12: GetTotalRequestsToday - No Requests
func TestGetTotalRequestsToday_NoRequests(t *testing.T) {
	// Arrange
	store, cleanup := setupTestStore(t)
	defer cleanup()

	// Act
	total, err := store.GetTotalRequestsToday()

	// Assert
	assert.NoError(t, err)
	assert.Equal(t, 0, total, "Should return 0 when no requests today")
}

// Test 1.13: GetTotalRequestsToday - Single Device
func TestGetTotalRequestsToday_SingleDevice(t *testing.T) {
	// Arrange
	store, cleanup := setupTestStore(t)
	defer cleanup()

	deviceID := "device_123"
	createTestDevice(t, store, deviceID)

	// Add 5 requests
	for i := 0; i < 5; i++ {
		err := store.IncrementRequestCount(deviceID, 50*time.Millisecond, 1024, false)
		require.NoError(t, err)
	}

	// Act
	total, err := store.GetTotalRequestsToday()

	// Assert
	assert.NoError(t, err)
	assert.Equal(t, 5, total, "Should sum all requests for single device")
}

// Test 1.14: GetTotalRequestsToday - Multiple Devices
func TestGetTotalRequestsToday_MultipleDevices(t *testing.T) {
	// Arrange
	store, cleanup := setupTestStore(t)
	defer cleanup()

	// Create 3 devices with different request counts
	devices := []struct {
		id       string
		requests int
	}{
		{"device_1", 10},
		{"device_2", 25},
		{"device_3", 15},
	}

	for _, d := range devices {
		createTestDevice(t, store, d.id)
		for i := 0; i < d.requests; i++ {
			err := store.IncrementRequestCount(d.id, 50*time.Millisecond, 1024, false)
			require.NoError(t, err)
		}
	}

	// Act
	total, err := store.GetTotalRequestsToday()

	// Assert
	assert.NoError(t, err)
	assert.Equal(t, 50, total, "Should sum requests across all devices (10+25+15)")
}
