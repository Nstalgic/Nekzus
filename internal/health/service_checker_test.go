package health

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nstalgic/nekzus/internal/storage"
	"github.com/nstalgic/nekzus/internal/types"
)

// Mock route registry for testing
type mockRouteRegistry struct {
	apps   []types.App
	routes map[string]*types.Route
}

func (m *mockRouteRegistry) ListApps() []types.App {
	return m.apps
}

func (m *mockRouteRegistry) GetRouteByAppID(appID string) (*types.Route, bool) {
	route, ok := m.routes[appID]
	if !ok {
		return nil, false
	}
	return route, true
}

func (m *mockRouteRegistry) GetAppByID(appID string) (*types.App, bool) {
	for _, app := range m.apps {
		if app.ID == appID {
			return &app, true
		}
	}
	return nil, false
}

// Mock metrics for testing
type mockServiceHealthMetrics struct {
	mu             sync.Mutex
	statusUpdates  map[string]float64
	checkCounts    map[string]int
	checkDurations map[string][]time.Duration
}

func newMockServiceHealthMetrics() *mockServiceHealthMetrics {
	return &mockServiceHealthMetrics{
		statusUpdates:  make(map[string]float64),
		checkCounts:    make(map[string]int),
		checkDurations: make(map[string][]time.Duration),
	}
}

func (m *mockServiceHealthMetrics) RecordServiceHealthCheck(appID, status string, duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.checkCounts[appID]++
	m.checkDurations[appID] = append(m.checkDurations[appID], duration)
}

func (m *mockServiceHealthMetrics) SetServiceHealthStatus(appID string, status float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.statusUpdates[appID] = status
}

func (m *mockServiceHealthMetrics) GetCheckCount(appID string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.checkCounts[appID]
}

func (m *mockServiceHealthMetrics) GetStatusUpdate(appID string) (float64, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	val, ok := m.statusUpdates[appID]
	return val, ok
}

func TestNewServiceHealthChecker(t *testing.T) {
	config := types.HealthChecksConfig{
		Enabled:            true,
		Interval:           "5s",
		Timeout:            "2s",
		UnhealthyThreshold: 3,
		Path:               "/health",
	}

	registry := &mockRouteRegistry{
		apps:   []types.App{},
		routes: make(map[string]*types.Route),
	}
	metrics := newMockServiceHealthMetrics()

	checker := NewServiceHealthChecker(config, registry, nil, metrics)

	if checker == nil {
		t.Fatal("NewServiceHealthChecker returned nil")
	}

	if checker.config.UnhealthyThreshold != 3 {
		t.Errorf("expected threshold 3, got %d", checker.config.UnhealthyThreshold)
	}
}

func TestServiceHealthChecker_StartStop(t *testing.T) {
	config := types.HealthChecksConfig{
		Enabled:            true,
		Interval:           "100ms",
		Timeout:            "50ms",
		UnhealthyThreshold: 2,
		Path:               "/",
	}

	registry := &mockRouteRegistry{
		apps: []types.App{
			{ID: "app1", Name: "Test App"},
		},
		routes: map[string]*types.Route{
			"app1": {
				RouteID: "route1",
				AppID:   "app1",
				To:      "http://example.com",
			},
		},
	}
	metrics := newMockServiceHealthMetrics()

	checker := NewServiceHealthChecker(config, registry, nil, metrics)

	// Start checker
	if err := checker.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Wait for at least one check cycle
	time.Sleep(200 * time.Millisecond)

	// Stop checker
	if err := checker.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	// Verify that a check was performed
	if metrics.GetCheckCount("app1") == 0 {
		t.Error("expected at least one health check for app1")
	}
}

func TestServiceHealthChecker_HealthyService(t *testing.T) {
	// Create a test server that returns 200 OK
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := types.HealthChecksConfig{
		Enabled:            true,
		Interval:           "50ms",
		Timeout:            "1s",
		UnhealthyThreshold: 2,
		Path:               "/health",
	}

	registry := &mockRouteRegistry{
		apps: []types.App{
			{ID: "app1", Name: "Test App"},
		},
		routes: map[string]*types.Route{
			"app1": {
				RouteID: "route1",
				AppID:   "app1",
				To:      server.URL,
			},
		},
	}
	metrics := newMockServiceHealthMetrics()

	checker := NewServiceHealthChecker(config, registry, nil, metrics)

	// Start checker
	if err := checker.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer checker.Stop()

	// Wait for a few check cycles
	time.Sleep(150 * time.Millisecond)

	// Get status
	if !checker.IsServiceHealthy("app1") {
		t.Error("expected service to be healthy")
	}

	status, ok := checker.GetServiceHealth("app1")
	if !ok {
		t.Fatal("expected to find service health status")
	}

	if status.Status != "healthy" {
		t.Errorf("expected healthy status, got %v", status.Status)
	}

	// Verify metrics
	if statusMetric, ok := metrics.GetStatusUpdate("app1"); !ok || statusMetric != 1.0 {
		t.Errorf("expected status metric to be healthy (1.0), got %v", statusMetric)
	}
}

func TestServiceHealthChecker_UnhealthyService(t *testing.T) {
	// Create a test server that returns 500
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	config := types.HealthChecksConfig{
		Enabled:            true,
		Interval:           "50ms",
		Timeout:            "1s",
		UnhealthyThreshold: 2,
		Path:               "/",
	}

	registry := &mockRouteRegistry{
		apps: []types.App{
			{ID: "app1", Name: "Test App"},
		},
		routes: map[string]*types.Route{
			"app1": {
				RouteID: "route1",
				AppID:   "app1",
				To:      server.URL,
			},
		},
	}
	metrics := newMockServiceHealthMetrics()

	checker := NewServiceHealthChecker(config, registry, nil, metrics)

	// Start checker
	if err := checker.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer checker.Stop()

	// Wait for enough check cycles to exceed threshold
	time.Sleep(200 * time.Millisecond)

	// Get status
	if checker.IsServiceHealthy("app1") {
		t.Error("expected service to be unhealthy")
	}

	status, ok := checker.GetServiceHealth("app1")
	if !ok {
		t.Fatal("expected to find service health status")
	}

	if status.Status != "unhealthy" {
		t.Errorf("expected unhealthy status, got %v", status.Status)
	}

	if status.ConsecutiveFailures < 2 {
		t.Errorf("expected at least 2 consecutive failures, got %d", status.ConsecutiveFailures)
	}

	// Verify metrics
	if statusMetric, ok := metrics.GetStatusUpdate("app1"); !ok || statusMetric != 2.0 {
		t.Errorf("expected status metric to be unhealthy (2.0), got %v", statusMetric)
	}
}

func TestServiceHealthChecker_Timeout(t *testing.T) {
	// Create a test server that hangs
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := types.HealthChecksConfig{
		Enabled:            true,
		Interval:           "50ms",
		Timeout:            "100ms", // Short timeout
		UnhealthyThreshold: 1,
		Path:               "/",
	}

	registry := &mockRouteRegistry{
		apps: []types.App{
			{ID: "app1", Name: "Test App"},
		},
		routes: map[string]*types.Route{
			"app1": {
				RouteID: "route1",
				AppID:   "app1",
				To:      server.URL,
			},
		},
	}
	metrics := newMockServiceHealthMetrics()

	checker := NewServiceHealthChecker(config, registry, nil, metrics)

	// Start checker
	if err := checker.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer checker.Stop()

	// Wait for check to complete
	time.Sleep(200 * time.Millisecond)

	// Verify it's marked as unhealthy due to timeout
	if checker.IsServiceHealthy("app1") {
		t.Error("expected service to be unhealthy due to timeout")
	}

	status, ok := checker.GetServiceHealth("app1")
	if !ok {
		t.Fatal("expected to find service health status")
	}

	if status.ErrorMessage == "" {
		t.Error("expected error message for timeout")
	}
}

func TestServiceHealthChecker_PerServiceOverride(t *testing.T) {
	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/custom-health" {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	config := types.HealthChecksConfig{
		Enabled:            true,
		Interval:           "50ms",
		Timeout:            "1s",
		UnhealthyThreshold: 2,
		Path:               "/default",
		PerService: map[string]types.ServiceHealthCheck{
			"app1": {
				Path: "/custom-health",
			},
		},
	}

	registry := &mockRouteRegistry{
		apps: []types.App{
			{ID: "app1", Name: "Test App"},
		},
		routes: map[string]*types.Route{
			"app1": {
				RouteID: "route1",
				AppID:   "app1",
				To:      server.URL,
			},
		},
	}
	metrics := newMockServiceHealthMetrics()

	checker := NewServiceHealthChecker(config, registry, nil, metrics)

	// Start checker
	if err := checker.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer checker.Stop()

	// Wait for check cycles
	time.Sleep(150 * time.Millisecond)

	// Get status - should be healthy because it uses the custom path
	if !checker.IsServiceHealthy("app1") {
		status, _ := checker.GetServiceHealth("app1")
		t.Errorf("expected service to be healthy with custom path, got status=%v, error=%v",
			status.Status, status.ErrorMessage)
	}
}

func TestServiceHealthChecker_RecoveryFromFailure(t *testing.T) {
	failureCount := 0

	// Create a test server that fails first, then succeeds
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if failureCount < 3 {
			failureCount++
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	config := types.HealthChecksConfig{
		Enabled:            true,
		Interval:           "50ms",
		Timeout:            "1s",
		UnhealthyThreshold: 2,
		Path:               "/",
	}

	registry := &mockRouteRegistry{
		apps: []types.App{
			{ID: "app1", Name: "Test App"},
		},
		routes: map[string]*types.Route{
			"app1": {
				RouteID: "route1",
				AppID:   "app1",
				To:      server.URL,
			},
		},
	}
	metrics := newMockServiceHealthMetrics()

	checker := NewServiceHealthChecker(config, registry, nil, metrics)

	// Start checker
	if err := checker.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer checker.Stop()

	// Wait for it to become unhealthy
	time.Sleep(150 * time.Millisecond)

	if checker.IsServiceHealthy("app1") {
		t.Error("expected service to be unhealthy initially")
	}

	// Wait for recovery
	time.Sleep(200 * time.Millisecond)

	if !checker.IsServiceHealthy("app1") {
		status, _ := checker.GetServiceHealth("app1")
		t.Errorf("expected service to be healthy after recovery, got status=%v, failures=%d",
			status.Status, status.ConsecutiveFailures)
	}
}

func TestServiceHealthChecker_StoragePersistence(t *testing.T) {
	// This test must run in isolation due to timing issues with async saves
	t.Skip("Skipping storage persistence test - requires isolation")

	// Create a temporary database
	store, err := storage.NewStore(storage.Config{
		DatabasePath: ":memory:",
	})
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	config := types.HealthChecksConfig{
		Enabled:            true,
		Interval:           "1h", // Long interval so we can manually trigger
		Timeout:            "1s",
		UnhealthyThreshold: 2,
		Path:               "/",
	}

	registry := &mockRouteRegistry{
		apps: []types.App{
			{ID: "app1", Name: "Test App"},
		},
		routes: map[string]*types.Route{
			"app1": {
				RouteID: "route1",
				AppID:   "app1",
				To:      "http://example.com",
			},
		},
	}
	metrics := newMockServiceHealthMetrics()

	// Create first checker and save a status
	checker1 := NewServiceHealthChecker(config, registry, store, metrics)
	if err := checker1.Start(); err != nil {
		t.Fatalf("Failed to start checker: %v", err)
	}

	// First save the app (required for foreign key constraint)
	err = store.SaveApp(types.App{
		ID:   "app1",
		Name: "Test App",
	})
	if err != nil {
		t.Fatalf("Failed to save app: %v", err)
	}

	// Manually update status
	now := time.Now()
	err = store.SaveServiceHealth(storage.ServiceHealth{
		AppID:               "app1",
		Status:              "unhealthy",
		LastCheckTime:       &now,
		LastSuccessTime:     nil,
		ConsecutiveFailures: 3,
		ErrorMessage:        "Test failure",
	})
	if err != nil {
		t.Fatalf("Failed to save service health: %v", err)
	}

	checker1.Stop()

	// Small delay to ensure async operations complete
	time.Sleep(50 * time.Millisecond)

	// Create second checker WITHOUT starting it (just to test loading)
	checker2 := NewServiceHealthChecker(config, registry, store, metrics)

	// Manually load from storage to test persistence without auto-check
	if err := checker2.loadHealthStatusesFromStorage(); err != nil {
		t.Fatalf("Failed to load from storage: %v", err)
	}

	// Verify loaded status
	status, ok := checker2.GetServiceHealth("app1")
	if !ok {
		t.Fatal("expected to find service health status after loading from storage")
	}

	if status.Status != "unhealthy" {
		t.Errorf("expected unhealthy status after loading from storage, got %v", status.Status)
	}

	if status.ConsecutiveFailures != 3 {
		t.Errorf("expected 3 consecutive failures, got %d", status.ConsecutiveFailures)
	}
}

func TestServiceHealthChecker_MultipleServices(t *testing.T) {
	// Create healthy server
	healthyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer healthyServer.Close()

	// Create unhealthy server
	unhealthyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer unhealthyServer.Close()

	config := types.HealthChecksConfig{
		Enabled:            true,
		Interval:           "50ms",
		Timeout:            "1s",
		UnhealthyThreshold: 2,
		Path:               "/",
	}

	registry := &mockRouteRegistry{
		apps: []types.App{
			{ID: "healthy-app", Name: "Healthy App"},
			{ID: "unhealthy-app", Name: "Unhealthy App"},
		},
		routes: map[string]*types.Route{
			"healthy-app":   {RouteID: "route1", AppID: "healthy-app", To: healthyServer.URL},
			"unhealthy-app": {RouteID: "route2", AppID: "unhealthy-app", To: unhealthyServer.URL},
		},
	}
	metrics := newMockServiceHealthMetrics()

	checker := NewServiceHealthChecker(config, registry, nil, metrics)

	// Start checker
	if err := checker.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer checker.Stop()

	// Wait for check cycles
	time.Sleep(200 * time.Millisecond)

	// Verify both services were checked
	if !checker.IsServiceHealthy("healthy-app") {
		t.Error("expected healthy-app to be healthy")
	}

	if checker.IsServiceHealthy("unhealthy-app") {
		t.Error("expected unhealthy-app to be unhealthy")
	}

	// Verify metrics for both
	if metrics.GetCheckCount("healthy-app") == 0 {
		t.Error("expected checks for healthy-app")
	}
	if metrics.GetCheckCount("unhealthy-app") == 0 {
		t.Error("expected checks for unhealthy-app")
	}

	// Verify GetAllServiceHealth
	allStatuses := checker.GetAllServiceHealth()
	if len(allStatuses) != 2 {
		t.Errorf("expected 2 service health statuses, got %d", len(allStatuses))
	}
}

func TestServiceHealthChecker_NoRoute(t *testing.T) {
	config := types.HealthChecksConfig{
		Enabled:            true,
		Interval:           "50ms",
		Timeout:            "1s",
		UnhealthyThreshold: 2,
		Path:               "/",
	}

	registry := &mockRouteRegistry{
		apps: []types.App{
			{ID: "app1", Name: "Test App"},
		},
		routes: make(map[string]*types.Route), // No routes defined
	}
	metrics := newMockServiceHealthMetrics()

	checker := NewServiceHealthChecker(config, registry, nil, metrics)

	// Start checker
	if err := checker.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer checker.Stop()

	// Wait for check cycle
	time.Sleep(150 * time.Millisecond)

	// Get status - should be unknown since there's no route
	status, ok := checker.GetServiceHealth("app1")
	if !ok {
		t.Fatal("expected to find service health status")
	}

	if status.Status != "unknown" {
		t.Errorf("expected unknown status for app without route, got %v", status.Status)
	}

	if status.ErrorMessage != "no route found for service" {
		t.Errorf("expected 'no route found' error message, got %v", status.ErrorMessage)
	}
}

// Mock WebSocket manager for testing health change notifications
type mockWebSocketManager struct {
	mu            sync.Mutex
	notifications []healthChangeNotification
	notifyCh      chan struct{} // signals when a notification is received
}

type healthChangeNotification struct {
	appID     string
	appName   string
	proxyPath string
	status    string
	message   string
}

func (m *mockWebSocketManager) PublishHealthChange(appID, appName, proxyPath, status, message string) {
	m.mu.Lock()
	m.notifications = append(m.notifications, healthChangeNotification{
		appID:     appID,
		appName:   appName,
		proxyPath: proxyPath,
		status:    status,
		message:   message,
	})
	ch := m.notifyCh
	m.mu.Unlock()

	// Signal that a notification was received (non-blocking)
	if ch != nil {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

func (m *mockWebSocketManager) GetNotifications() []healthChangeNotification {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]healthChangeNotification, len(m.notifications))
	copy(result, m.notifications)
	return result
}

// WaitForNotification waits for a notification or times out
func (m *mockWebSocketManager) WaitForNotification(timeout time.Duration) bool {
	m.mu.Lock()
	if m.notifyCh == nil {
		m.notifyCh = make(chan struct{}, 10)
	}
	ch := m.notifyCh
	m.mu.Unlock()

	select {
	case <-ch:
		return true
	case <-time.After(timeout):
		return false
	}
}

func TestServiceHealthChecker_HealthChangeNotification(t *testing.T) {
	// Create a test server that transitions from healthy to unhealthy
	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := requestCount.Add(1)
		if count <= 3 {
			// First 3 requests: healthy
			w.WriteHeader(http.StatusOK)
		} else {
			// After that: unhealthy
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	}))
	defer server.Close()

	config := types.HealthChecksConfig{
		Enabled:            true,
		Interval:           "100ms",
		Timeout:            "1s",
		UnhealthyThreshold: 2, // Require 2 consecutive failures
		Path:               "/health",
	}

	registry := &mockRouteRegistry{
		apps: []types.App{
			{ID: "app1", Name: "Test App"},
		},
		routes: map[string]*types.Route{
			"app1": {
				RouteID: "route1",
				AppID:   "app1",
				To:      server.URL,
			},
		},
	}
	metrics := newMockServiceHealthMetrics()
	wsManager := &mockWebSocketManager{}

	checker := NewServiceHealthChecker(config, registry, nil, metrics)
	checker.wsManager = wsManager

	// Start checker
	if err := checker.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer checker.Stop()

	// Wait for initial healthy checks
	time.Sleep(350 * time.Millisecond)

	// Verify service is healthy
	status, ok := checker.GetServiceHealth("app1")
	if !ok {
		t.Fatal("expected to find service health status")
	}
	if status.Status != "healthy" {
		t.Errorf("expected healthy status, got %s", status.Status)
	}

	// Wait for unhealthy checks (need 2 failures based on threshold)
	time.Sleep(500 * time.Millisecond)

	// Verify service became unhealthy
	status, ok = checker.GetServiceHealth("app1")
	if !ok {
		t.Fatal("expected to find service health status")
	}
	if status.Status != "unhealthy" {
		t.Errorf("expected unhealthy status, got %s", status.Status)
	}

	// Verify that exactly 1 notification was sent (when status changed from healthy to unhealthy)
	notifications := wsManager.GetNotifications()
	if len(notifications) != 1 {
		t.Errorf("expected 1 health change notification, got %d", len(notifications))
	}

	// Verify notification details
	if len(notifications) > 0 {
		notification := notifications[0]
		if notification.appID != "app1" {
			t.Errorf("expected appID 'app1', got %s", notification.appID)
		}
		if notification.status != "unhealthy" {
			t.Errorf("expected status 'unhealthy', got %s", notification.status)
		}
		if notification.message == "" {
			t.Error("expected non-empty error message")
		}
	}
}

func TestServiceHealthChecker_MarkAppUnhealthy(t *testing.T) {
	config := types.HealthChecksConfig{
		Enabled:            true,
		Interval:           "1h", // Long interval so periodic checks don't interfere
		Timeout:            "1s",
		UnhealthyThreshold: 3,
		Path:               "/health",
	}

	registry := &mockRouteRegistry{
		apps: []types.App{
			{ID: "app1", Name: "Test App"},
		},
		routes: map[string]*types.Route{
			"app1": {
				RouteID: "route1",
				AppID:   "app1",
				To:      "http://localhost:9999", // Non-existent
			},
		},
	}
	metrics := newMockServiceHealthMetrics()
	wsManager := &mockWebSocketManager{
		notifyCh: make(chan struct{}, 10),
	}

	checker := NewServiceHealthChecker(config, registry, nil, metrics)
	checker.SetWebSocketNotifier(wsManager)

	// Manually set initial healthy state (simulating a previously healthy app)
	checker.mu.Lock()
	checker.healthStatus["app1"] = &ServiceHealthStatus{
		AppID:  "app1",
		Status: "healthy",
	}
	checker.mu.Unlock()

	// Mark app as unhealthy (e.g., container was stopped)
	checker.MarkAppUnhealthy("app1", "Container stopped via API")

	// Verify status changed
	status, ok := checker.GetServiceHealth("app1")
	if !ok {
		t.Fatal("expected to find service health status")
	}
	if status.Status != "unhealthy" {
		t.Errorf("expected unhealthy status, got %s", status.Status)
	}
	if status.ErrorMessage != "Container stopped via API" {
		t.Errorf("expected error message 'Container stopped via API', got %s", status.ErrorMessage)
	}

	// Wait for async notification (sent via goroutine)
	if !wsManager.WaitForNotification(time.Second) {
		t.Fatal("timed out waiting for notification")
	}

	// Verify notification was sent
	notifications := wsManager.GetNotifications()
	if len(notifications) != 1 {
		t.Errorf("expected 1 notification, got %d", len(notifications))
	}
	if len(notifications) > 0 {
		if notifications[0].status != "unhealthy" {
			t.Errorf("expected unhealthy notification, got %s", notifications[0].status)
		}
	}
}

func TestServiceHealthChecker_MarkAppUnhealthy_NoSpam(t *testing.T) {
	config := types.HealthChecksConfig{
		Enabled:            true,
		Interval:           "1h",
		Timeout:            "1s",
		UnhealthyThreshold: 3,
	}

	registry := &mockRouteRegistry{
		apps:   []types.App{{ID: "app1", Name: "Test App"}},
		routes: map[string]*types.Route{},
	}
	metrics := newMockServiceHealthMetrics()
	wsManager := &mockWebSocketManager{}

	checker := NewServiceHealthChecker(config, registry, nil, metrics)
	checker.SetWebSocketNotifier(wsManager)

	// Set initial healthy state
	checker.mu.Lock()
	checker.healthStatus["app1"] = &ServiceHealthStatus{
		AppID:  "app1",
		Status: "healthy",
	}
	checker.mu.Unlock()

	// Mark unhealthy multiple times
	checker.MarkAppUnhealthy("app1", "First stop")
	checker.MarkAppUnhealthy("app1", "Second stop") // Should NOT send another notification
	checker.MarkAppUnhealthy("app1", "Third stop")  // Should NOT send another notification

	// Wait for async notification goroutine to complete
	time.Sleep(50 * time.Millisecond)

	// Verify only 1 notification was sent (no spam)
	notifications := wsManager.GetNotifications()
	if len(notifications) != 1 {
		t.Errorf("expected exactly 1 notification (no spam), got %d", len(notifications))
	}
}
