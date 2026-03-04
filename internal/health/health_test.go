package health

import (
	"context"
	"sync"
	"testing"
	"time"
)

// mockChecker is a mock health checker for testing
type mockChecker struct {
	name   string
	status Status
	delay  time.Duration
}

func (m *mockChecker) Name() string {
	return m.name
}

func (m *mockChecker) Check(ctx context.Context) ComponentHealth {
	if m.delay > 0 {
		time.Sleep(m.delay)
	}
	return ComponentHealth{
		Status:      m.status,
		Message:     "mock check",
		LastChecked: time.Now(),
	}
}

// mockMetrics is a mock metrics recorder for testing
type mockMetrics struct {
	mu      sync.Mutex
	records []struct {
		component string
		status    string
		duration  time.Duration
	}
}

func (m *mockMetrics) RecordHealthCheck(component, status string, duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.records = append(m.records, struct {
		component string
		status    string
		duration  time.Duration
	}{component, status, duration})
}

func TestNewManager(t *testing.T) {
	metrics := &mockMetrics{}
	manager := NewManager("1.0.0", time.Now(), metrics)

	if manager == nil {
		t.Fatal("manager should not be nil")
	}

	if manager.version != "1.0.0" {
		t.Errorf("expected version 1.0.0, got %s", manager.version)
	}

	if manager.cacheTTL != 5*time.Second {
		t.Errorf("expected cache TTL 5s, got %s", manager.cacheTTL)
	}
}

func TestRegisterChecker(t *testing.T) {
	metrics := &mockMetrics{}
	manager := NewManager("1.0.0", time.Now(), metrics)

	checker := &mockChecker{name: "test", status: StatusHealthy}
	manager.RegisterChecker(checker)

	components := manager.ListComponents()
	if len(components) != 1 {
		t.Errorf("expected 1 component, got %d", len(components))
	}

	if components[0] != "test" {
		t.Errorf("expected component name 'test', got %s", components[0])
	}
}

func TestCheckHealthy(t *testing.T) {
	metrics := &mockMetrics{}
	manager := NewManager("1.0.0", time.Now(), metrics)

	manager.RegisterChecker(&mockChecker{name: "comp1", status: StatusHealthy})
	manager.RegisterChecker(&mockChecker{name: "comp2", status: StatusHealthy})

	ctx := context.Background()
	health := manager.Check(ctx)

	if health.Status != StatusHealthy {
		t.Errorf("expected overall status healthy, got %s", health.Status)
	}

	if len(health.Components) != 2 {
		t.Errorf("expected 2 components, got %d", len(health.Components))
	}

	// Verify metrics were recorded
	if len(metrics.records) != 2 {
		t.Errorf("expected 2 metric records, got %d", len(metrics.records))
	}
}

func TestCheckDegraded(t *testing.T) {
	metrics := &mockMetrics{}
	manager := NewManager("1.0.0", time.Now(), metrics)

	manager.RegisterChecker(&mockChecker{name: "comp1", status: StatusHealthy})
	manager.RegisterChecker(&mockChecker{name: "comp2", status: StatusDegraded})

	ctx := context.Background()
	health := manager.Check(ctx)

	if health.Status != StatusDegraded {
		t.Errorf("expected overall status degraded, got %s", health.Status)
	}
}

func TestCheckUnhealthy(t *testing.T) {
	metrics := &mockMetrics{}
	manager := NewManager("1.0.0", time.Now(), metrics)

	manager.RegisterChecker(&mockChecker{name: "comp1", status: StatusHealthy})
	manager.RegisterChecker(&mockChecker{name: "comp2", status: StatusUnhealthy})

	ctx := context.Background()
	health := manager.Check(ctx)

	if health.Status != StatusUnhealthy {
		t.Errorf("expected overall status unhealthy, got %s", health.Status)
	}
}

func TestCheckCaching(t *testing.T) {
	t.Parallel()

	metrics := &mockMetrics{}
	manager := NewManager("1.0.0", time.Now(), metrics)
	manager.SetCacheTTL(50 * time.Millisecond) // Reduced from 1s for faster testing

	checker := &mockChecker{name: "test", status: StatusHealthy, delay: 10 * time.Millisecond}
	manager.RegisterChecker(checker)

	ctx := context.Background()

	// First check - should perform actual check
	start := time.Now()
	manager.Check(ctx)
	firstDuration := time.Since(start)

	// Second check - should use cache
	start = time.Now()
	manager.Check(ctx)
	secondDuration := time.Since(start)

	// Second check should be much faster (cached)
	if secondDuration >= firstDuration {
		t.Errorf("second check should be faster (cached), first=%s, second=%s", firstDuration, secondDuration)
	}

	// Wait for cache to expire (reduced from 1100ms to 60ms)
	time.Sleep(60 * time.Millisecond)

	// Third check - cache expired, should perform actual check
	start = time.Now()
	manager.Check(ctx)
	thirdDuration := time.Since(start)

	// Third check should take time again
	if thirdDuration < 5*time.Millisecond {
		t.Errorf("third check should take time (cache expired), got %s", thirdDuration)
	}
}

func TestIsHealthy(t *testing.T) {
	metrics := &mockMetrics{}
	manager := NewManager("1.0.0", time.Now(), metrics)

	testCases := []struct {
		name            string
		componentStatus Status
		expectedHealthy bool
	}{
		{"healthy", StatusHealthy, true},
		{"degraded", StatusDegraded, true},
		{"unhealthy", StatusUnhealthy, false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			manager.ClearCache()
			manager = NewManager("1.0.0", time.Now(), metrics)
			manager.RegisterChecker(&mockChecker{name: "test", status: tc.componentStatus})

			ctx := context.Background()
			healthy := manager.IsHealthy(ctx)

			if healthy != tc.expectedHealthy {
				t.Errorf("expected IsHealthy=%v for status %s, got %v", tc.expectedHealthy, tc.componentStatus, healthy)
			}
		})
	}
}

func TestIsReady(t *testing.T) {
	metrics := &mockMetrics{}
	manager := NewManager("1.0.0", time.Now(), metrics)

	manager.RegisterChecker(&mockChecker{name: "test", status: StatusHealthy})

	ctx := context.Background()
	if !manager.IsReady(ctx) {
		t.Error("expected ready when healthy")
	}

	manager.ClearCache()
	manager = NewManager("1.0.0", time.Now(), metrics)
	manager.RegisterChecker(&mockChecker{name: "test", status: StatusUnhealthy})

	if manager.IsReady(ctx) {
		t.Error("expected not ready when unhealthy")
	}
}

func TestSimpleLivenessCheck(t *testing.T) {
	metrics := &mockMetrics{}
	manager := NewManager("1.0.0", time.Now(), metrics)

	// Liveness should always return true if the process is running
	if !manager.SimpleLivenessCheck() {
		t.Error("liveness check should always return true")
	}
}

func TestGetComponentHealth(t *testing.T) {
	metrics := &mockMetrics{}
	manager := NewManager("1.0.0", time.Now(), metrics)

	manager.RegisterChecker(&mockChecker{name: "test", status: StatusHealthy})

	ctx := context.Background()
	health, err := manager.GetComponentHealth(ctx, "test")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if health.Status != StatusHealthy {
		t.Errorf("expected healthy status, got %s", health.Status)
	}

	// Test non-existent component
	_, err = manager.GetComponentHealth(ctx, "nonexistent")
	if err == nil {
		t.Error("expected error for non-existent component")
	}
}

func TestClearCache(t *testing.T) {
	metrics := &mockMetrics{}
	manager := NewManager("1.0.0", time.Now(), metrics)

	checker := &mockChecker{name: "test", status: StatusHealthy}
	manager.RegisterChecker(checker)

	ctx := context.Background()

	// Perform check to populate cache
	manager.Check(ctx)

	// Clear cache
	manager.ClearCache()

	// Next check should not use cache
	// We can verify this by checking that metrics were recorded again
	initialRecords := len(metrics.records)
	manager.Check(ctx)
	if len(metrics.records) != initialRecords+1 {
		t.Error("cache should have been cleared")
	}
}

func TestConcurrentChecks(t *testing.T) {
	metrics := &mockMetrics{}
	manager := NewManager("1.0.0", time.Now(), metrics)

	// Register multiple checkers
	for i := 0; i < 10; i++ {
		manager.RegisterChecker(&mockChecker{
			name:   string(rune('a' + i)),
			status: StatusHealthy,
			delay:  10 * time.Millisecond,
		})
	}

	ctx := context.Background()

	// Perform concurrent checks
	done := make(chan bool, 5)
	for i := 0; i < 5; i++ {
		go func() {
			manager.Check(ctx)
			done <- true
		}()
	}

	// Wait for all to complete
	for i := 0; i < 5; i++ {
		<-done
	}

	// Should not panic and should have results
	health := manager.Check(ctx)
	if len(health.Components) != 10 {
		t.Errorf("expected 10 components, got %d", len(health.Components))
	}
}

func TestCheckTimeout(t *testing.T) {
	metrics := &mockMetrics{}
	manager := NewManager("1.0.0", time.Now(), metrics)

	// Register a slow checker
	manager.RegisterChecker(&mockChecker{
		name:   "slow",
		status: StatusHealthy,
		delay:  500 * time.Millisecond,
	})

	// Create context with short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Check should complete even with timeout
	// The individual checker will be interrupted by context
	health := manager.Check(ctx)

	// Should still get a result
	if health.Components == nil {
		t.Error("expected components even with timeout")
	}
}

func TestCalculateOverallStatus(t *testing.T) {
	metrics := &mockMetrics{}
	manager := NewManager("1.0.0", time.Now(), metrics)

	testCases := []struct {
		name           string
		components     map[string]ComponentHealth
		expectedStatus Status
	}{
		{
			name:           "empty",
			components:     map[string]ComponentHealth{},
			expectedStatus: StatusUnknown,
		},
		{
			name: "all healthy",
			components: map[string]ComponentHealth{
				"c1": {Status: StatusHealthy},
				"c2": {Status: StatusHealthy},
			},
			expectedStatus: StatusHealthy,
		},
		{
			name: "one degraded",
			components: map[string]ComponentHealth{
				"c1": {Status: StatusHealthy},
				"c2": {Status: StatusDegraded},
			},
			expectedStatus: StatusDegraded,
		},
		{
			name: "one unhealthy",
			components: map[string]ComponentHealth{
				"c1": {Status: StatusHealthy},
				"c2": {Status: StatusUnhealthy},
			},
			expectedStatus: StatusUnhealthy,
		},
		{
			name: "unhealthy takes precedence over degraded",
			components: map[string]ComponentHealth{
				"c1": {Status: StatusDegraded},
				"c2": {Status: StatusUnhealthy},
			},
			expectedStatus: StatusUnhealthy,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			status := manager.calculateOverallStatus(tc.components)
			if status != tc.expectedStatus {
				t.Errorf("expected %s, got %s", tc.expectedStatus, status)
			}
		})
	}
}

func TestSetCacheTTL(t *testing.T) {
	metrics := &mockMetrics{}
	manager := NewManager("1.0.0", time.Now(), metrics)

	manager.SetCacheTTL(10 * time.Second)

	if manager.cacheTTL != 10*time.Second {
		t.Errorf("expected cache TTL 10s, got %s", manager.cacheTTL)
	}
}

func TestListComponents(t *testing.T) {
	metrics := &mockMetrics{}
	manager := NewManager("1.0.0", time.Now(), metrics)

	manager.RegisterChecker(&mockChecker{name: "comp1", status: StatusHealthy})
	manager.RegisterChecker(&mockChecker{name: "comp2", status: StatusHealthy})
	manager.RegisterChecker(&mockChecker{name: "comp3", status: StatusHealthy})

	components := manager.ListComponents()

	if len(components) != 3 {
		t.Errorf("expected 3 components, got %d", len(components))
	}

	// Verify all components are present
	componentMap := make(map[string]bool)
	for _, name := range components {
		componentMap[name] = true
	}

	for _, expected := range []string{"comp1", "comp2", "comp3"} {
		if !componentMap[expected] {
			t.Errorf("expected component %s not found", expected)
		}
	}
}
