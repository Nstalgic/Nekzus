package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nstalgic/nekzus/internal/health"
	"github.com/nstalgic/nekzus/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockRouteRegistry implements a minimal route registry for testing
type mockRouteRegistry struct {
	routes map[string]*types.Route
}

func (m *mockRouteRegistry) GetRouteByAppID(appID string) (*types.Route, bool) {
	route, ok := m.routes[appID]
	return route, ok
}

func (m *mockRouteRegistry) ListApps() []types.App {
	return nil
}

// mockServiceHealthChecker implements a minimal health checker for testing
type mockServiceHealthChecker struct {
	healthStatuses map[string]*health.ServiceHealthStatus
}

func (m *mockServiceHealthChecker) GetServiceHealth(appID string) (*health.ServiceHealthStatus, bool) {
	status, ok := m.healthStatuses[appID]
	return status, ok
}

func (m *mockServiceHealthChecker) IsServiceHealthy(appID string) bool {
	status, ok := m.healthStatuses[appID]
	if !ok {
		return true
	}
	return status.Status == "healthy"
}

func TestServiceHealthHandler_HandleServiceHealth(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name           string
		appID          string
		method         string
		routes         map[string]*types.Route
		healthStatuses map[string]*health.ServiceHealthStatus
		wantStatus     int
		wantHeader     string
	}{
		{
			name:   "GET healthy service",
			appID:  "grafana",
			method: http.MethodGet,
			routes: map[string]*types.Route{
				"grafana": {AppID: "grafana", PathBase: "/apps/grafana/", To: "http://localhost:3000"},
			},
			healthStatuses: map[string]*health.ServiceHealthStatus{
				"grafana": {
					AppID:               "grafana",
					Status:              "healthy",
					LastCheckTime:       now,
					ConsecutiveFailures: 0,
				},
			},
			wantStatus: http.StatusOK,
			wantHeader: "healthy",
		},
		{
			name:   "GET unhealthy service",
			appID:  "grafana",
			method: http.MethodGet,
			routes: map[string]*types.Route{
				"grafana": {AppID: "grafana", PathBase: "/apps/grafana/", To: "http://localhost:3000"},
			},
			healthStatuses: map[string]*health.ServiceHealthStatus{
				"grafana": {
					AppID:               "grafana",
					Status:              "unhealthy",
					LastCheckTime:       now,
					ConsecutiveFailures: 3,
					ErrorMessage:        "Connection refused",
				},
			},
			wantStatus: http.StatusServiceUnavailable,
			wantHeader: "unhealthy",
		},
		{
			name:   "HEAD healthy service",
			appID:  "grafana",
			method: http.MethodHead,
			routes: map[string]*types.Route{
				"grafana": {AppID: "grafana", PathBase: "/apps/grafana/", To: "http://localhost:3000"},
			},
			healthStatuses: map[string]*health.ServiceHealthStatus{
				"grafana": {
					AppID:               "grafana",
					Status:              "healthy",
					LastCheckTime:       now,
					ConsecutiveFailures: 0,
				},
			},
			wantStatus: http.StatusOK,
			wantHeader: "healthy",
		},
		{
			name:   "HEAD unhealthy service",
			appID:  "grafana",
			method: http.MethodHead,
			routes: map[string]*types.Route{
				"grafana": {AppID: "grafana", PathBase: "/apps/grafana/", To: "http://localhost:3000"},
			},
			healthStatuses: map[string]*health.ServiceHealthStatus{
				"grafana": {
					AppID:               "grafana",
					Status:              "unhealthy",
					LastCheckTime:       now,
					ConsecutiveFailures: 3,
				},
			},
			wantStatus: http.StatusServiceUnavailable,
			wantHeader: "unhealthy",
		},
		{
			name:           "service not found",
			appID:          "nonexistent",
			method:         http.MethodGet,
			routes:         map[string]*types.Route{},
			healthStatuses: map[string]*health.ServiceHealthStatus{},
			wantStatus:     http.StatusNotFound,
			wantHeader:     "",
		},
		{
			name:   "service exists but no health status - unknown",
			appID:  "newservice",
			method: http.MethodGet,
			routes: map[string]*types.Route{
				"newservice": {AppID: "newservice", PathBase: "/apps/newservice/", To: "http://localhost:8080"},
			},
			healthStatuses: map[string]*health.ServiceHealthStatus{},
			wantStatus:     http.StatusOK,
			wantHeader:     "unknown",
		},
		{
			name:   "unknown status returns 200",
			appID:  "grafana",
			method: http.MethodGet,
			routes: map[string]*types.Route{
				"grafana": {AppID: "grafana", PathBase: "/apps/grafana/", To: "http://localhost:3000"},
			},
			healthStatuses: map[string]*health.ServiceHealthStatus{
				"grafana": {
					AppID:               "grafana",
					Status:              "unknown",
					LastCheckTime:       now,
					ConsecutiveFailures: 0,
				},
			},
			wantStatus: http.StatusOK,
			wantHeader: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewServiceHealthHandler(
				&mockRouteRegistry{routes: tt.routes},
				&mockServiceHealthChecker{healthStatuses: tt.healthStatuses},
			)

			req := httptest.NewRequest(tt.method, "/api/v1/services/"+tt.appID+"/health", nil)
			rec := httptest.NewRecorder()

			handler.HandleServiceHealth(rec, req, tt.appID)

			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantHeader != "" {
				assert.Equal(t, tt.wantHeader, rec.Header().Get("X-Health-Status"))
			}

			// HEAD should have no body
			if tt.method == http.MethodHead {
				assert.Empty(t, rec.Body.String())
			}

			// GET should have JSON body for successful requests
			if tt.method == http.MethodGet && tt.wantStatus != http.StatusNotFound {
				assert.Contains(t, rec.Header().Get("Content-Type"), "application/json")
			}
		})
	}
}

func TestServiceHealthHandler_MethodNotAllowed(t *testing.T) {
	handler := NewServiceHealthHandler(
		&mockRouteRegistry{routes: map[string]*types.Route{
			"test": {AppID: "test"},
		}},
		&mockServiceHealthChecker{healthStatuses: map[string]*health.ServiceHealthStatus{}},
	)

	methods := []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch}

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/api/v1/services/test/health", nil)
			rec := httptest.NewRecorder()

			handler.HandleServiceHealth(rec, req, "test")

			assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
		})
	}
}

func TestServiceHealthHandler_ResponseHeaders(t *testing.T) {
	now := time.Now()

	handler := NewServiceHealthHandler(
		&mockRouteRegistry{routes: map[string]*types.Route{
			"grafana": {AppID: "grafana", PathBase: "/apps/grafana/", To: "http://localhost:3000"},
		}},
		&mockServiceHealthChecker{healthStatuses: map[string]*health.ServiceHealthStatus{
			"grafana": {
				AppID:               "grafana",
				Status:              "healthy",
				LastCheckTime:       now,
				ConsecutiveFailures: 0,
			},
		}},
	)

	req := httptest.NewRequest(http.MethodHead, "/api/v1/services/grafana/health", nil)
	rec := httptest.NewRecorder()

	handler.HandleServiceHealth(rec, req, "grafana")

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "healthy", rec.Header().Get("X-Health-Status"))
	assert.NotEmpty(t, rec.Header().Get("X-Last-Check"))
	assert.Equal(t, "0", rec.Header().Get("X-Consecutive-Failures"))
}

func TestServiceHealthHandler_CacheHeaders(t *testing.T) {
	now := time.Now()

	handler := NewServiceHealthHandler(
		&mockRouteRegistry{routes: map[string]*types.Route{
			"grafana": {AppID: "grafana"},
		}},
		&mockServiceHealthChecker{healthStatuses: map[string]*health.ServiceHealthStatus{
			"grafana": {
				AppID:         "grafana",
				Status:        "healthy",
				LastCheckTime: now,
			},
		}},
	)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/services/grafana/health", nil)
	rec := httptest.NewRecorder()

	handler.HandleServiceHealth(rec, req, "grafana")

	// Should have cache control headers for mobile clients
	assert.Equal(t, "no-cache", rec.Header().Get("Cache-Control"))
}
