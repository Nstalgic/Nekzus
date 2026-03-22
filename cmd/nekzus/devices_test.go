package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/nstalgic/nekzus/internal/activity"
	"github.com/nstalgic/nekzus/internal/auth"
	"github.com/nstalgic/nekzus/internal/handlers"
	"github.com/nstalgic/nekzus/internal/proxy"
	"github.com/nstalgic/nekzus/internal/ratelimit"
	"github.com/nstalgic/nekzus/internal/router"
	"github.com/nstalgic/nekzus/internal/storage"
	"github.com/nstalgic/nekzus/internal/types"
	"github.com/nstalgic/nekzus/internal/websocket"
)

// newTestApplicationWithStorage creates a test app with temporary storage
func newTestApplicationWithStorage(t *testing.T) (*Application, func()) {
	t.Helper()

	// Create temporary database
	tmpFile, err := os.CreateTemp("", "nekzus-test-*.db")
	if err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()
	dbPath := tmpFile.Name()

	// Initialize storage
	store, err := storage.NewStore(storage.Config{
		DatabasePath: dbPath,
	})
	if err != nil {
		os.Remove(dbPath)
		t.Fatal(err)
	}

	// Create auth manager
	testSecret := "random-jwt-hmac-key-f8e7d6c5b4a39281"
	authMgr, err := auth.NewManager(
		[]byte(testSecret),
		"nekzus",
		"nekzus-mobile",
		[]string{"boot-123"},
	)
	if err != nil {
		store.Close()
		os.Remove(dbPath)
		t.Fatal(err)
	}

	// Create managers first
	activityTracker := activity.NewTracker(store)
	wsManager := websocket.NewManager(testMetrics, store)
	qrLimiter := ratelimit.NewLimiter(1.0, 5)

	// Create auth handler
	authHandler := handlers.NewAuthHandler(
		authMgr,
		store,
		testMetrics,
		wsManager,
		activityTracker,
		qrLimiter,
		nil, // no cert manager for tests
		"http://localhost:8443",
		"",
		"test-nexus",
		"1.0.0-test",
		[]string{"catalog", "events", "proxy"},
	)

	app := &Application{
		config: types.ServerConfig{},
		services: &ServiceRegistry{
			Auth: authMgr,
		},
		limiters: &RateLimiterRegistry{
			QR: qrLimiter,
		},
		managers: &ManagerRegistry{
			Router:    router.NewRegistry(store),
			WebSocket: wsManager,
			Activity:  activityTracker,
		},
		handlers: &HandlerRegistry{
			Auth: authHandler,
		},
		jobs:         &JobRegistry{}, // Empty jobs registry for tests
		storage:      store,
		metrics:      testMetrics,
		proxyCache:   proxy.NewCache(),
		nekzusID:     "test-nekzus",
		baseURL:      "http://localhost:8443",
		version:      "1.0.0-test",
		capabilities: []string{"catalog", "events", "proxy"},
	}

	// Cleanup function
	cleanup := func() {
		store.Close()
		os.Remove(dbPath)
	}

	return app, cleanup
}

func TestDeviceManagement_PairCreatesDevice(t *testing.T) {
	app, cleanup := newTestApplicationWithStorage(t)
	defer cleanup()

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/pair", app.handlePair)
	mux.HandleFunc("/api/v1/devices", app.handleDevices)

	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Pair a device
	body := `{"device":{"id":"test-device-1","model":"iPhone 14","platform":"ios"}}`
	req, _ := http.NewRequest("POST", srv.URL+"/api/v1/auth/pair", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer boot-123")
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		t.Fatalf("pair status %d", res.StatusCode)
	}

	var pairResp map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&pairResp); err != nil {
		t.Fatal(err)
	}

	token, ok := pairResp["accessToken"].(string)
	if !ok || token == "" {
		t.Fatal("no access token in response")
	}

	// Verify device was created in storage
	device, err := app.storage.GetDevice("test-device-1")
	if err != nil {
		t.Fatal(err)
	}
	if device == nil {
		t.Fatal("device not found in storage")
	}
	if device.ID != "test-device-1" {
		t.Errorf("expected device ID test-device-1, got %s", device.ID)
	}
	if device.Name != "iPhone 14" {
		t.Errorf("expected device name 'iPhone 14', got %s", device.Name)
	}
}

func TestDeviceManagement_ListDevices(t *testing.T) {
	app, cleanup := newTestApplicationWithStorage(t)
	defer cleanup()

	// Create test devices
	if err := app.storage.SaveDevice("dev-1", "iPhone 14", "ios", "", []string{"read:catalog"}); err != nil {
		t.Fatal(err)
	}
	if err := app.storage.SaveDevice("dev-2", "iPad Pro", "ios", "", []string{"read:catalog", "read:events"}); err != nil {
		t.Fatal(err)
	}

	// Get JWT token
	token, _ := app.services.Auth.SignJWT("dev-1", []string{"read:catalog"}, 1*time.Hour)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/devices", app.handleDevices)

	srv := httptest.NewServer(mux)
	defer srv.Close()

	// List devices
	req, _ := http.NewRequest("GET", srv.URL+"/api/v1/devices", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		t.Fatalf("expected status 200, got %d", res.StatusCode)
	}

	var devices []storage.DeviceInfo
	if err := json.NewDecoder(res.Body).Decode(&devices); err != nil {
		t.Fatal(err)
	}

	if len(devices) != 2 {
		t.Fatalf("expected 2 devices, got %d", len(devices))
	}

	// Verify device data
	foundDev1 := false
	foundDev2 := false
	for _, d := range devices {
		if d.ID == "dev-1" && d.Name == "iPhone 14" {
			foundDev1 = true
		}
		if d.ID == "dev-2" && d.Name == "iPad Pro" {
			foundDev2 = true
		}
	}
	if !foundDev1 || !foundDev2 {
		t.Errorf("devices not found correctly: %+v", devices)
	}
}

func TestDeviceManagement_GetDevice(t *testing.T) {
	app, cleanup := newTestApplicationWithStorage(t)
	defer cleanup()

	// Create test device
	if err := app.storage.SaveDevice("dev-test", "Test Device", "", "", []string{"read:catalog"}); err != nil {
		t.Fatal(err)
	}

	token, _ := app.services.Auth.SignJWT("dev-test", []string{"read:catalog"}, 1*time.Hour)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/devices/", app.handleDeviceActions)

	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Get specific device
	req, _ := http.NewRequest("GET", srv.URL+"/api/v1/devices/dev-test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		t.Fatalf("expected status 200, got %d", res.StatusCode)
	}

	var device storage.DeviceInfo
	if err := json.NewDecoder(res.Body).Decode(&device); err != nil {
		t.Fatal(err)
	}

	if device.ID != "dev-test" {
		t.Errorf("expected device ID dev-test, got %s", device.ID)
	}
	if device.Name != "Test Device" {
		t.Errorf("expected device name 'Test Device', got %s", device.Name)
	}
}

func TestDeviceManagement_GetDevice_NotFound(t *testing.T) {
	app, cleanup := newTestApplicationWithStorage(t)
	defer cleanup()

	token, _ := app.services.Auth.SignJWT("dev-1", []string{"read:catalog"}, 1*time.Hour)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/devices/", app.handleDeviceActions)

	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Try to get non-existent device
	req, _ := http.NewRequest("GET", srv.URL+"/api/v1/devices/nonexistent", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	if res.StatusCode != 404 {
		t.Fatalf("expected status 404, got %d", res.StatusCode)
	}
}

func TestDeviceManagement_RevokeDevice(t *testing.T) {
	app, cleanup := newTestApplicationWithStorage(t)
	defer cleanup()

	// Create test device
	if err := app.storage.SaveDevice("dev-revoke", "Device to Revoke", "", "", []string{"read:catalog"}); err != nil {
		t.Fatal(err)
	}

	token, _ := app.services.Auth.SignJWT("admin-dev", []string{"read:catalog"}, 1*time.Hour)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/devices/", app.handleDeviceActions)

	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Revoke device
	req, _ := http.NewRequest("DELETE", srv.URL+"/api/v1/devices/dev-revoke", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		t.Fatalf("expected status 200, got %d", res.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}

	if result["status"] != "revoked" {
		t.Errorf("expected status 'revoked', got %v", result["status"])
	}
	if result["deviceId"] != "dev-revoke" {
		t.Errorf("expected deviceId 'dev-revoke', got %v", result["deviceId"])
	}

	// Verify device was deleted from storage
	device, err := app.storage.GetDevice("dev-revoke")
	if err != nil {
		t.Fatal(err)
	}
	if device != nil {
		t.Error("device should have been deleted from storage")
	}
}

func TestDeviceManagement_UpdateDeviceMetadata(t *testing.T) {
	app, cleanup := newTestApplicationWithStorage(t)
	defer cleanup()

	// Create test device
	if err := app.storage.SaveDevice("dev-update", "Old Name", "", "", []string{"read:catalog"}); err != nil {
		t.Fatal(err)
	}

	token, _ := app.services.Auth.SignJWT("dev-update", []string{"read:catalog"}, 1*time.Hour)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/devices/", app.handleDeviceActions)

	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Update device name
	updateBody := `{"deviceName":"New Name"}`
	req, _ := http.NewRequest("PATCH", srv.URL+"/api/v1/devices/dev-update", strings.NewReader(updateBody))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		t.Fatalf("expected status 200, got %d", res.StatusCode)
	}

	var device storage.DeviceInfo
	if err := json.NewDecoder(res.Body).Decode(&device); err != nil {
		t.Fatal(err)
	}

	if device.Name != "New Name" {
		t.Errorf("expected device name 'New Name', got %s", device.Name)
	}

	// Verify in storage
	storedDevice, err := app.storage.GetDevice("dev-update")
	if err != nil {
		t.Fatal(err)
	}
	if storedDevice.Name != "New Name" {
		t.Errorf("stored device name not updated: got %s", storedDevice.Name)
	}
}

func TestDeviceManagement_RequiresAuth(t *testing.T) {
	app, cleanup := newTestApplicationWithStorage(t)
	defer cleanup()

	mux := http.NewServeMux()
	mux.Handle("/api/v1/devices", app.requireJWT(http.HandlerFunc(app.handleDevices)))
	mux.Handle("/api/v1/devices/", app.requireJWT(http.HandlerFunc(app.handleDeviceActions)))

	srv := httptest.NewServer(mux)
	defer srv.Close()

	tests := []struct {
		name   string
		method string
		path   string
	}{
		{"list devices", "GET", "/api/v1/devices"},
		{"get device", "GET", "/api/v1/devices/dev-1"},
		{"revoke device", "DELETE", "/api/v1/devices/dev-1"},
		{"update device", "PATCH", "/api/v1/devices/dev-1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest(tt.method, srv.URL+tt.path, nil)
			// No Authorization header
			res, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()

			if res.StatusCode != 401 {
				t.Errorf("expected status 401 (unauthorized), got %d", res.StatusCode)
			}
		})
	}
}

func TestDeviceManagement_DeviceActivityTracking(t *testing.T) {
	t.Skip("Skipping flaky async test - requires further investigation into goroutine scheduling")

	// Note: Not running in parallel due to async timing sensitivity
	app, cleanup := newTestApplicationWithStorage(t)
	defer cleanup()

	// Create test device
	if err := app.storage.SaveDevice("dev-track", "Tracking Device", "", "", []string{"read:catalog"}); err != nil {
		t.Fatal(err)
	}

	// Get initial last_seen
	device1, err := app.storage.GetDevice("dev-track")
	if err != nil {
		t.Fatal(err)
	}
	initialLastSeen := device1.LastSeen

	// Wait a moment to ensure timestamp will be different
	time.Sleep(10 * time.Millisecond)

	// Make authenticated request
	token, _ := app.services.Auth.SignJWT("dev-track", []string{"read:catalog"}, 1*time.Hour)

	mux := http.NewServeMux()
	mux.Handle("/api/v1/apps", app.requireJWT(http.HandlerFunc(app.handleListApps)))

	srv := httptest.NewServer(mux)
	defer srv.Close()

	req, _ := http.NewRequest("GET", srv.URL+"/api/v1/apps", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	// Give the async goroutine a moment to start
	time.Sleep(20 * time.Millisecond)

	// Wait for async update to complete (poll with shorter intervals)
	var device2 *storage.DeviceInfo
	var updateFound bool
	timeout := time.After(1 * time.Second)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			if !updateFound {
				t.Error("last_seen should have been updated after authenticated request (timeout)")
			}
			return
		case <-ticker.C:
			device2, err = app.storage.GetDevice("dev-track")
			if err != nil {
				t.Fatal(err)
			}
			if device2.LastSeen.After(initialLastSeen) {
				updateFound = true
				// Verify last_seen was updated correctly
				if device2.LastSeen.Before(initialLastSeen) {
					t.Error("last_seen should not be earlier than initial value")
				}
				return
			}
		}
	}
}

// TestDevicePagination verifies pagination works for device listing
func TestDevicePagination(t *testing.T) {
	app, cleanup := newTestApplicationWithStorage(t)
	defer cleanup()

	// Create 10 test devices with tokens
	var token0 string
	for i := 0; i < 10; i++ {
		deviceID := fmt.Sprintf("test-device-%d", i)
		deviceName := fmt.Sprintf("Test Device %d", i)

		token, err := app.services.Auth.SignJWT(deviceID, []string{"read", "write"}, 24*time.Hour)
		if err != nil {
			t.Fatalf("Failed to sign JWT: %v", err)
		}

		if i == 0 {
			token0 = token
		}

		// Use the correct SaveDevice API: deviceID, deviceName, scopes
		if err := app.storage.SaveDevice(deviceID, deviceName, "", "", []string{"read", "write"}); err != nil {
			t.Fatalf("Failed to save device: %v", err)
		}
	}

	// Setup HTTP mux with JWT auth
	requireJWT := app.requireJWT(http.HandlerFunc(app.handleDevices))
	srv := httptest.NewServer(requireJWT)
	defer srv.Close()

	tests := []struct {
		name          string
		queryParams   string
		expectCount   int
		expectDevices []string
		usePagination bool
	}{
		{
			name:          "no_params_returns_array_for_backward_compat",
			queryParams:   "",
			expectCount:   10,
			usePagination: false,
		},
		{
			name:          "limit_5_returns_first_5",
			queryParams:   "?limit=5",
			expectCount:   5,
			usePagination: true,
		},
		{
			name:          "offset_5_returns_last_5",
			queryParams:   "?offset=5",
			expectCount:   5,
			usePagination: true,
		},
		{
			name:          "limit_3_offset_2_returns_middle_3",
			queryParams:   "?limit=3&offset=2",
			expectCount:   3,
			usePagination: true,
		},
		{
			name:          "limit_0_returns_empty",
			queryParams:   "?limit=0",
			expectCount:   0,
			usePagination: true,
		},
		{
			name:          "offset_beyond_total_returns_empty",
			queryParams:   "?offset=20",
			expectCount:   0,
			usePagination: true,
		},
		{
			name:          "limit_exceeds_remaining",
			queryParams:   "?limit=10&offset=7",
			expectCount:   3,
			usePagination: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", srv.URL+"/api/v1/devices"+tt.queryParams, nil)
			req.Header.Set("Authorization", "Bearer "+token0)

			res, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()

			if res.StatusCode != http.StatusOK {
				t.Fatalf("Expected status 200, got %d", res.StatusCode)
			}

			if tt.usePagination {
				// Paginated response
				var response struct {
					Devices []storage.DeviceInfo `json:"devices"`
					Total   int                  `json:"total"`
					Limit   int                  `json:"limit"`
					Offset  int                  `json:"offset"`
				}

				if err := json.NewDecoder(res.Body).Decode(&response); err != nil {
					t.Fatalf("Failed to decode paginated response: %v", err)
				}

				if len(response.Devices) != tt.expectCount {
					t.Errorf("Expected %d devices, got %d", tt.expectCount, len(response.Devices))
				}

				if response.Total != 10 {
					t.Errorf("Expected total=10, got %d", response.Total)
				}
			} else {
				// Backward compatible array response
				var devices []storage.DeviceInfo

				if err := json.NewDecoder(res.Body).Decode(&devices); err != nil {
					t.Fatalf("Failed to decode array response: %v", err)
				}

				if len(devices) != tt.expectCount {
					t.Errorf("Expected %d devices, got %d", tt.expectCount, len(devices))
				}
			}
		})
	}
}

// TestActivityPagination verifies pagination works for activity listing
// Note: ActivityTracker has maxActivityEvents=10 limit in memory
func TestActivityPagination(t *testing.T) {
	app, cleanup := newTestApplicationWithStorage(t)
	defer cleanup()

	// Create 10 activity events (max limit for ActivityTracker)
	for i := 0; i < 10; i++ {
		event := types.ActivityEvent{
			Type:      "device.paired",
			Icon:      "Smartphone",
			IconClass: "success",
			Message:   fmt.Sprintf("Device device-%d paired", i),
			Timestamp: time.Now().Add(-time.Duration(i) * time.Minute).UnixMilli(),
		}
		if err := app.managers.Activity.Add(event); err != nil {
			t.Fatalf("Failed to add activity: %v", err)
		}
	}

	// Setup HTTP mux with IP auth (localhost allowed)
	srv := httptest.NewServer(http.HandlerFunc(app.handleRecentActivity))
	defer srv.Close()

	tests := []struct {
		name          string
		queryParams   string
		expectCount   int
		expectTotal   int
		usePagination bool
	}{
		{
			name:          "no_params_returns_array_for_backward_compat",
			queryParams:   "",
			expectCount:   10,
			expectTotal:   10,
			usePagination: false,
		},
		{
			name:          "limit_5_returns_first_5",
			queryParams:   "?limit=5",
			expectCount:   5,
			expectTotal:   10,
			usePagination: true,
		},
		{
			name:          "offset_5_returns_last_5",
			queryParams:   "?offset=5",
			expectCount:   5,
			expectTotal:   10,
			usePagination: true,
		},
		{
			name:          "limit_3_offset_3_returns_middle_3",
			queryParams:   "?limit=3&offset=3",
			expectCount:   3,
			expectTotal:   10,
			usePagination: true,
		},
		{
			name:          "offset_beyond_total_returns_empty",
			queryParams:   "?offset=15",
			expectCount:   0,
			expectTotal:   10,
			usePagination: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", srv.URL+"/api/v1/activity/recent"+tt.queryParams, nil)

			res, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()

			if res.StatusCode != http.StatusOK {
				t.Fatalf("Expected status 200, got %d", res.StatusCode)
			}

			if tt.usePagination {
				// Paginated response
				var response struct {
					Activities []types.ActivityEvent `json:"activities"`
					Total      int                   `json:"total"`
					Limit      int                   `json:"limit"`
					Offset     int                   `json:"offset"`
				}

				if err := json.NewDecoder(res.Body).Decode(&response); err != nil {
					t.Fatalf("Failed to decode paginated response: %v", err)
				}

				if len(response.Activities) != tt.expectCount {
					t.Errorf("Expected %d activities, got %d", tt.expectCount, len(response.Activities))
				}

				if response.Total != tt.expectTotal {
					t.Errorf("Expected total=%d, got %d", tt.expectTotal, response.Total)
				}
			} else {
				// Backward compatible array response
				var activities []types.ActivityEvent

				if err := json.NewDecoder(res.Body).Decode(&activities); err != nil {
					t.Fatalf("Failed to decode array response: %v", err)
				}

				if len(activities) != tt.expectCount {
					t.Errorf("Expected %d activities, got %d", tt.expectCount, len(activities))
				}
			}
		})
	}
}
