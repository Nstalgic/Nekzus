package config

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nstalgic/nekzus/internal/types"
)

// mockMetrics implements WatcherMetrics for testing
type mockMetrics struct {
	mu           sync.Mutex
	reloadCalls  []reloadCall
	totalCalls   []totalCall
	recordCount  int
	successCount int
	errorCount   int
}

type reloadCall struct {
	status   string
	duration time.Duration
}

type totalCall struct {
	status string
}

func (m *mockMetrics) RecordConfigReload(status string, duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.reloadCalls = append(m.reloadCalls, reloadCall{status: status, duration: duration})
	m.recordCount++
	if status == "success" {
		m.successCount++
	} else {
		m.errorCount++
	}
}

func (m *mockMetrics) IncrementConfigReloadTotal(status string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.totalCalls = append(m.totalCalls, totalCall{status: status})
}

func (m *mockMetrics) getReloadCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.recordCount
}

func (m *mockMetrics) getSuccessCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.successCount
}

func (m *mockMetrics) getErrorCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.errorCount
}

// createTempConfigFile creates a temporary config file for testing
func createTempConfigFile(t *testing.T, content string) string {
	t.Helper()

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create temp config: %v", err)
	}

	return configPath
}

// baseConfigYAML returns a valid base configuration
func baseConfigYAML() string {
	return `
server:
  addr: ":8080"
  tls_cert: ""
  tls_key: ""

auth:
  hs256_secret: "test-secret-that-is-long-enough-for-validation-32chars"
  issuer: "nekzus"
  audience: "nekzus-clients"
  token_ttl: 43200

bootstrap:
  tokens: ["boot-token-1"]

storage:
  database_path: ":memory:"

apps:
  - id: "grafana"
    name: "Grafana"
    url: "http://grafana:3000"

routes:
  - id: "route-grafana"
    app_id: "grafana"
    path_base: "/grafana"
    to: "/"
`
}

// TestNewWatcher verifies watcher creation
func TestNewWatcher(t *testing.T) {
	configPath := createTempConfigFile(t, baseConfigYAML())

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	metrics := &mockMetrics{}
	watcher, err := NewWatcher(configPath, cfg, metrics)
	if err != nil {
		t.Fatalf("NewWatcher failed: %v", err)
	}

	if watcher == nil {
		t.Fatal("Expected watcher to be non-nil")
	}

	if watcher.configPath != configPath {
		t.Errorf("Expected configPath=%s, got %s", configPath, watcher.configPath)
	}

	if watcher.debouncePeriod != 500*time.Millisecond {
		t.Errorf("Expected debouncePeriod=500ms, got %v", watcher.debouncePeriod)
	}
}

// TestWatcherStartStop verifies starting and stopping the watcher
func TestWatcherStartStop(t *testing.T) {
	configPath := createTempConfigFile(t, baseConfigYAML())

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	metrics := &mockMetrics{}
	watcher, err := NewWatcher(configPath, cfg, metrics)
	if err != nil {
		t.Fatalf("NewWatcher failed: %v", err)
	}

	// Start watcher
	if err := watcher.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Give it a moment to start
	time.Sleep(100 * time.Millisecond)

	// Stop watcher
	if err := watcher.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
}

// TestWatcherReloadOnFileChange verifies reload when config file changes
func TestWatcherReloadOnFileChange(t *testing.T) {
	configPath := createTempConfigFile(t, baseConfigYAML())

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	metrics := &mockMetrics{}
	watcher, err := NewWatcher(configPath, cfg, metrics)
	if err != nil {
		t.Fatalf("NewWatcher failed: %v", err)
	}

	// Track handler calls
	var mu sync.Mutex
	handlerCalled := false
	var handlerOldConfig, handlerNewConfig types.ServerConfig

	watcher.RegisterReloadHandler(func(old, new types.ServerConfig) error {
		mu.Lock()
		defer mu.Unlock()
		handlerCalled = true
		handlerOldConfig = old
		handlerNewConfig = new
		return nil
	})

	if err := watcher.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer watcher.Stop()

	// Modify config file (change bootstrap token)
	newConfig := `
server:
  addr: ":8080"
  tls_cert: ""
  tls_key: ""

auth:
  hs256_secret: "test-secret-that-is-long-enough-for-validation-32chars"
  issuer: "nekzus"
  audience: "nekzus-clients"
  token_ttl: 43200

bootstrap:
  tokens: ["boot-token-2-changed"]

storage:
  database_path: ":memory:"

apps:
  - id: "prometheus"
    name: "Prometheus"
    url: "http://prometheus:9090"

routes:
  - id: "route-prometheus"
    app_id: "prometheus"
    path_base: "/prometheus"
    to: "/"
`

	if err := os.WriteFile(configPath, []byte(newConfig), 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	// Wait for reload (debounce + processing)
	time.Sleep(1 * time.Second)

	// Read values with lock
	mu.Lock()
	called := handlerCalled
	oldCfg := handlerOldConfig
	newCfg := handlerNewConfig
	mu.Unlock()

	if !called {
		t.Fatal("Expected reload handler to be called")
	}

	// Verify old config had grafana
	if len(oldCfg.Apps) == 0 || oldCfg.Apps[0].ID != "grafana" {
		t.Errorf("Expected old config to have grafana app")
	}

	// Verify new config has prometheus
	if len(newCfg.Apps) == 0 || newCfg.Apps[0].ID != "prometheus" {
		t.Errorf("Expected new config to have prometheus app")
	}

	// Verify metrics recorded
	if metrics.getSuccessCount() == 0 {
		t.Error("Expected success metric to be recorded")
	}
}

// TestWatcherDebounce verifies debouncing of rapid changes
func TestWatcherDebounce(t *testing.T) {
	configPath := createTempConfigFile(t, baseConfigYAML())

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	metrics := &mockMetrics{}
	watcher, err := NewWatcher(configPath, cfg, metrics)
	if err != nil {
		t.Fatalf("NewWatcher failed: %v", err)
	}

	// Reduce debounce period for faster testing
	watcher.debouncePeriod = 200 * time.Millisecond

	var handlerCallCount atomic.Int32
	watcher.RegisterReloadHandler(func(old, new types.ServerConfig) error {
		handlerCallCount.Add(1)
		return nil
	})

	if err := watcher.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer watcher.Stop()

	// Write multiple times rapidly
	for i := 0; i < 5; i++ {
		config := baseConfigYAML() + fmt.Sprintf("\n# Comment %d\n", i)
		if err := os.WriteFile(configPath, []byte(config), 0644); err != nil {
			t.Fatalf("Failed to write config: %v", err)
		}
		time.Sleep(50 * time.Millisecond)
	}

	// Wait for debounce + processing
	time.Sleep(500 * time.Millisecond)

	// Should only reload once due to debouncing
	count := handlerCallCount.Load()
	if count != 1 {
		t.Errorf("Expected handler to be called once, got %d calls", count)
	}
}

// TestWatcherValidationFailure verifies that non-reloadable changes are rejected
func TestWatcherValidationFailure(t *testing.T) {
	tests := []struct {
		name       string
		modifyFunc func(string) string
		wantError  string
	}{
		{
			name: "change_server_addr",
			modifyFunc: func(cfg string) string {
				return `
server:
  addr: ":9090"
  tls_cert: ""
  tls_key: ""
auth:
  hs256_secret: "test-secret-that-is-long-enough-for-validation-32chars"
  issuer: "nekzus"
  audience: "nekzus-clients"
storage:
  database_path: ":memory:"
`
			},
			wantError: "server.addr cannot be changed",
		},
		{
			name: "change_jwt_secret",
			modifyFunc: func(cfg string) string {
				return `
server:
  addr: ":8080"
  tls_cert: ""
  tls_key: ""
auth:
  hs256_secret: "different-secret-that-is-also-long-enough-validation"
  issuer: "nekzus"
  audience: "nekzus-clients"
storage:
  database_path: ":memory:"
`
			},
			wantError: "auth.hs256_secret cannot be changed",
		},
		{
			name: "change_database_path",
			modifyFunc: func(cfg string) string {
				return `
server:
  addr: ":8080"
  tls_cert: ""
  tls_key: ""
auth:
  hs256_secret: "test-secret-that-is-long-enough-for-validation-32chars"
  issuer: "nekzus"
  audience: "nekzus-clients"
storage:
  database_path: "/different/path/db.sqlite"
`
			},
			wantError: "storage.database_path cannot be changed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configPath := createTempConfigFile(t, baseConfigYAML())

			cfg, err := Load(configPath)
			if err != nil {
				t.Fatalf("Failed to load config: %v", err)
			}

			metrics := &mockMetrics{}
			watcher, err := NewWatcher(configPath, cfg, metrics)
			if err != nil {
				t.Fatalf("NewWatcher failed: %v", err)
			}

			if err := watcher.Start(); err != nil {
				t.Fatalf("Start failed: %v", err)
			}
			defer watcher.Stop()

			// Write invalid config
			newConfig := tt.modifyFunc(baseConfigYAML())
			if err := os.WriteFile(configPath, []byte(newConfig), 0644); err != nil {
				t.Fatalf("Failed to write config: %v", err)
			}

			// Wait for reload attempt
			time.Sleep(1 * time.Second)

			// Verify error metric was recorded
			if metrics.getErrorCount() == 0 {
				t.Error("Expected error metric to be recorded for validation failure")
			}
		})
	}
}

// TestWatcherHandlerFailure verifies error handling when a handler fails
func TestWatcherHandlerFailure(t *testing.T) {
	configPath := createTempConfigFile(t, baseConfigYAML())

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	metrics := &mockMetrics{}
	watcher, err := NewWatcher(configPath, cfg, metrics)
	if err != nil {
		t.Fatalf("NewWatcher failed: %v", err)
	}

	// Register a failing handler
	watcher.RegisterReloadHandler(func(old, new types.ServerConfig) error {
		return context.DeadlineExceeded
	})

	if err := watcher.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer watcher.Stop()

	// Modify config
	newConfig := baseConfigYAML() + "\n# Modified\n"
	if err := os.WriteFile(configPath, []byte(newConfig), 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	// Wait for reload attempt
	time.Sleep(1 * time.Second)

	// Verify error was recorded
	if metrics.getErrorCount() == 0 {
		t.Error("Expected error metric to be recorded for handler failure")
	}
}

// TestWatcherMultipleHandlers verifies multiple handlers execute in order
func TestWatcherMultipleHandlers(t *testing.T) {
	configPath := createTempConfigFile(t, baseConfigYAML())

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	metrics := &mockMetrics{}
	watcher, err := NewWatcher(configPath, cfg, metrics)
	if err != nil {
		t.Fatalf("NewWatcher failed: %v", err)
	}

	// Track handler execution order
	var execOrder []int
	var mu sync.Mutex

	watcher.RegisterReloadHandler(func(old, new types.ServerConfig) error {
		mu.Lock()
		execOrder = append(execOrder, 1)
		mu.Unlock()
		return nil
	})

	watcher.RegisterReloadHandler(func(old, new types.ServerConfig) error {
		mu.Lock()
		execOrder = append(execOrder, 2)
		mu.Unlock()
		return nil
	})

	watcher.RegisterReloadHandler(func(old, new types.ServerConfig) error {
		mu.Lock()
		execOrder = append(execOrder, 3)
		mu.Unlock()
		return nil
	})

	if err := watcher.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer watcher.Stop()

	// Modify config
	newConfig := baseConfigYAML() + "\n# Modified\n"
	if err := os.WriteFile(configPath, []byte(newConfig), 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	// Wait for reload
	time.Sleep(1 * time.Second)

	mu.Lock()
	defer mu.Unlock()

	if len(execOrder) != 3 {
		t.Fatalf("Expected 3 handlers to execute, got %d", len(execOrder))
	}

	// Verify execution order
	for i, expected := range []int{1, 2, 3} {
		if execOrder[i] != expected {
			t.Errorf("Handler %d: expected order %d, got %d", i, expected, execOrder[i])
		}
	}
}

// TestWatcherGetCurrentConfig verifies thread-safe config access
func TestWatcherGetCurrentConfig(t *testing.T) {
	configPath := createTempConfigFile(t, baseConfigYAML())

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	metrics := &mockMetrics{}
	watcher, err := NewWatcher(configPath, cfg, metrics)
	if err != nil {
		t.Fatalf("NewWatcher failed: %v", err)
	}

	// Get initial config
	currentCfg := watcher.GetCurrentConfig()
	if currentCfg.Server.Addr != ":8080" {
		t.Errorf("Expected addr=:8080, got %s", currentCfg.Server.Addr)
	}

	// Start watcher
	if err := watcher.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer watcher.Stop()

	// Concurrent reads while potentially reloading
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				cfg := watcher.GetCurrentConfig()
				if cfg.Server.Addr != ":8080" {
					t.Errorf("Unexpected addr: %s", cfg.Server.Addr)
				}
			}
		}()
	}

	wg.Wait()
}

// TestWatcherInvalidConfigFile verifies handling of invalid YAML
func TestWatcherInvalidConfigFile(t *testing.T) {
	configPath := createTempConfigFile(t, baseConfigYAML())

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	metrics := &mockMetrics{}
	watcher, err := NewWatcher(configPath, cfg, metrics)
	if err != nil {
		t.Fatalf("NewWatcher failed: %v", err)
	}

	if err := watcher.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer watcher.Stop()

	// Write invalid YAML
	invalidYAML := "invalid: [unclosed bracket"
	if err := os.WriteFile(configPath, []byte(invalidYAML), 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	// Wait for reload attempt
	time.Sleep(1 * time.Second)

	// Verify error was recorded
	if metrics.getErrorCount() == 0 {
		t.Error("Expected error metric to be recorded for invalid YAML")
	}

	// Verify old config is still in use
	currentCfg := watcher.GetCurrentConfig()
	if len(currentCfg.Apps) == 0 || currentCfg.Apps[0].ID != "grafana" {
		t.Error("Expected old config to remain after failed reload")
	}
}

// TestWatcherConcurrentReloads verifies thread safety
func TestWatcherConcurrentReloads(t *testing.T) {
	configPath := createTempConfigFile(t, baseConfigYAML())

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	metrics := &mockMetrics{}
	watcher, err := NewWatcher(configPath, cfg, metrics)
	if err != nil {
		t.Fatalf("NewWatcher failed: %v", err)
	}

	// Short debounce for faster testing
	watcher.debouncePeriod = 100 * time.Millisecond

	if err := watcher.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer watcher.Stop()

	// Trigger multiple concurrent file changes
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			config := baseConfigYAML() + fmt.Sprintf("\n# Change %d\n", n)
			if err := os.WriteFile(configPath, []byte(config), 0644); err != nil {
				t.Errorf("Failed to write config: %v", err)
			}
		}(i)
	}

	wg.Wait()

	// Wait for all reloads to complete
	time.Sleep(500 * time.Millisecond)

	// Should not crash or deadlock
	currentCfg := watcher.GetCurrentConfig()
	if currentCfg.Server.Addr != ":8080" {
		t.Errorf("Unexpected addr after concurrent reloads: %s", currentCfg.Server.Addr)
	}
}

// TestWatcherMetricsRecording verifies all metrics are properly recorded
func TestWatcherMetricsRecording(t *testing.T) {
	configPath := createTempConfigFile(t, baseConfigYAML())

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	metrics := &mockMetrics{}
	watcher, err := NewWatcher(configPath, cfg, metrics)
	if err != nil {
		t.Fatalf("NewWatcher failed: %v", err)
	}

	watcher.RegisterReloadHandler(func(old, new types.ServerConfig) error {
		return nil
	})

	if err := watcher.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer watcher.Stop()

	// Successful reload
	newConfig := baseConfigYAML() + "\n# Modified\n"
	if err := os.WriteFile(configPath, []byte(newConfig), 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	time.Sleep(1 * time.Second)

	// Verify success metrics
	if metrics.getSuccessCount() == 0 {
		t.Error("Expected success metrics to be recorded")
	}

	metrics.mu.Lock()
	if len(metrics.reloadCalls) == 0 {
		t.Error("Expected RecordConfigReload to be called")
	}
	if len(metrics.totalCalls) == 0 {
		t.Error("Expected IncrementConfigReloadTotal to be called")
	}
	lastCall := metrics.reloadCalls[len(metrics.reloadCalls)-1]
	if lastCall.status != "success" {
		t.Errorf("Expected status=success, got %s", lastCall.status)
	}
	if lastCall.duration == 0 {
		t.Error("Expected non-zero duration")
	}
	metrics.mu.Unlock()
}
