package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/nstalgic/nekzus/internal/middleware"
	"github.com/nstalgic/nekzus/internal/ratelimit"
	"github.com/nstalgic/nekzus/internal/types"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func TestMetricsEndpointEnabled(t *testing.T) {
	app := newTestApplication(t)
	app.limiters.Metrics = ratelimit.NewLimiter(1.0, 10) // Allow rate limiting

	// Create metrics handler with enabled state
	metricsEnabled := atomic.Bool{}
	metricsEnabled.Store(true)

	// Create the dynamic handler
	metricsHandler := createDynamicMetricsHandler(app.limiters.Metrics, &metricsEnabled)

	// Setup test server
	mux := http.NewServeMux()
	mux.Handle("/metrics", metricsHandler)

	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Test: metrics endpoint should return 200 with metrics data
	req, _ := http.NewRequest("GET", srv.URL+"/metrics", nil)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		t.Errorf("Expected status 200, got %d", res.StatusCode)
	}

	// Check Content-Type header for Prometheus metrics
	contentType := res.Header.Get("Content-Type")
	if contentType == "" {
		t.Error("Expected Content-Type header, got empty")
	}
}

func TestMetricsEndpointDisabled(t *testing.T) {
	app := newTestApplication(t)
	app.limiters.Metrics = ratelimit.NewLimiter(1.0, 10)

	// Create metrics handler with disabled state
	metricsEnabled := atomic.Bool{}
	metricsEnabled.Store(false)

	// Create the dynamic handler
	metricsHandler := createDynamicMetricsHandler(app.limiters.Metrics, &metricsEnabled)

	// Setup test server
	mux := http.NewServeMux()
	mux.Handle("/metrics", metricsHandler)

	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Test: metrics endpoint should return 404 when disabled
	req, _ := http.NewRequest("GET", srv.URL+"/metrics", nil)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	if res.StatusCode != 404 {
		t.Errorf("Expected status 404 when disabled, got %d", res.StatusCode)
	}
}

func TestMetricsEndpointToggle(t *testing.T) {
	app := newTestApplication(t)
	app.limiters.Metrics = ratelimit.NewLimiter(1.0, 10)

	// Create metrics handler with enabled state
	metricsEnabled := atomic.Bool{}
	metricsEnabled.Store(true)

	// Create the dynamic handler
	metricsHandler := createDynamicMetricsHandler(app.limiters.Metrics, &metricsEnabled)

	// Setup test server
	mux := http.NewServeMux()
	mux.Handle("/metrics", metricsHandler)

	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Test 1: Enabled - should return 200
	req, _ := http.NewRequest("GET", srv.URL+"/metrics", nil)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()

	if res.StatusCode != 200 {
		t.Errorf("Expected status 200 when enabled, got %d", res.StatusCode)
	}

	// Disable metrics
	metricsEnabled.Store(false)

	// Test 2: Disabled - should return 404
	req2, _ := http.NewRequest("GET", srv.URL+"/metrics", nil)
	res2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	res2.Body.Close()

	if res2.StatusCode != 404 {
		t.Errorf("Expected status 404 when disabled, got %d", res2.StatusCode)
	}

	// Re-enable metrics
	metricsEnabled.Store(true)

	// Test 3: Re-enabled - should return 200 again
	req3, _ := http.NewRequest("GET", srv.URL+"/metrics", nil)
	res3, err := http.DefaultClient.Do(req3)
	if err != nil {
		t.Fatal(err)
	}
	res3.Body.Close()

	if res3.StatusCode != 200 {
		t.Errorf("Expected status 200 when re-enabled, got %d", res3.StatusCode)
	}
}

func TestMetricsConfigHotReload(t *testing.T) {
	// Initialize metricsEnabled flag
	metricsEnabled := atomic.Bool{}
	metricsEnabled.Store(true)

	// Test initial config
	oldConfig := types.ServerConfig{
		Metrics: types.MetricsConfig{
			Enabled: true,
			Path:    "/metrics",
		},
	}

	// New config with metrics disabled
	newConfig := types.ServerConfig{
		Metrics: types.MetricsConfig{
			Enabled: false,
			Path:    "/metrics",
		},
	}

	// Test 1: Initial state - metrics enabled
	if !metricsEnabled.Load() {
		t.Error("Expected metrics to be initially enabled")
	}

	// Test 2: Disable metrics via config change
	handleMetricsConfigReload(&metricsEnabled, oldConfig.Metrics, newConfig.Metrics)

	if metricsEnabled.Load() {
		t.Error("Expected metrics to be disabled after reload")
	}

	// Test 3: Re-enable metrics
	reEnabledConfig := types.ServerConfig{
		Metrics: types.MetricsConfig{
			Enabled: true,
			Path:    "/metrics",
		},
	}

	handleMetricsConfigReload(&metricsEnabled, newConfig.Metrics, reEnabledConfig.Metrics)

	if !metricsEnabled.Load() {
		t.Error("Expected metrics to be re-enabled after second reload")
	}

	// Test 4: No change (enabled stays enabled)
	oldState := metricsEnabled.Load()
	handleMetricsConfigReload(&metricsEnabled, reEnabledConfig.Metrics, reEnabledConfig.Metrics)

	if metricsEnabled.Load() != oldState {
		t.Error("Expected metrics state to remain unchanged when config doesn't change")
	}
}

func TestMetricsPathCannotChange(t *testing.T) {
	// Test that path changes are rejected (require restart)
	oldConfig := types.MetricsConfig{
		Enabled: true,
		Path:    "/metrics",
	}

	newConfig := types.MetricsConfig{
		Enabled: true,
		Path:    "/custom-metrics",
	}

	// This should return an error because path changed
	err := validateMetricsConfigChange(oldConfig, newConfig)
	if err == nil {
		t.Error("Expected error when metrics path changes, got nil")
	}

	if err != nil && err.Error() != "metrics.path cannot be changed without restart" {
		t.Errorf("Expected specific error message, got: %v", err)
	}
}

func TestMetricsEnabledCanChange(t *testing.T) {
	// Test that enabled flag can change without restart
	oldConfig := types.MetricsConfig{
		Enabled: true,
		Path:    "/metrics",
	}

	newConfig := types.MetricsConfig{
		Enabled: false,
		Path:    "/metrics",
	}

	// This should NOT return an error because only enabled changed
	err := validateMetricsConfigChange(oldConfig, newConfig)
	if err != nil {
		t.Errorf("Expected no error when only metrics.enabled changes, got: %v", err)
	}
}

// Helper function to handle metrics config reload (this will be implemented in main.go)
func handleMetricsConfigReload(enabled *atomic.Bool, oldCfg, newCfg types.MetricsConfig) {
	if oldCfg.Enabled != newCfg.Enabled {
		enabled.Store(newCfg.Enabled)
	}
}

// Helper function to validate metrics config changes (this will be implemented in config package)
func validateMetricsConfigChange(oldCfg, newCfg types.MetricsConfig) error {
	if oldCfg.Path != newCfg.Path {
		return fmt.Errorf("metrics.path cannot be changed without restart")
	}
	return nil
}

// Helper function to create dynamic metrics handler (this will be implemented in main.go)
func createDynamicMetricsHandler(limiter *ratelimit.Limiter, enabled *atomic.Bool) http.Handler {
	// Wrap the actual metrics handler
	metricsHandler := middleware.RateLimit(limiter)(promhttp.Handler())

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if metrics are enabled
		if !enabled.Load() {
			http.NotFound(w, r)
			return
		}
		metricsHandler.ServeHTTP(w, r)
	})
}
