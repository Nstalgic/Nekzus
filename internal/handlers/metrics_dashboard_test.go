package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nstalgic/nekzus/internal/metrics"
)

func TestMetricsDashboardHandler_HandleMetricsDashboard(t *testing.T) {
	// Create real metrics instance
	m := metrics.New("test")

	// Create handler
	handler := NewMetricsDashboardHandler(m)

	t.Run("returns metrics summary on GET", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/metrics/dashboard", nil)
		rec := httptest.NewRecorder()

		handler.HandleMetricsDashboard(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
		}

		// Verify JSON response
		var response MetricsDashboardResponse
		if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		// Verify structure exists
		if response.HTTP == nil {
			t.Error("expected HTTP metrics to be present")
		}
		if response.Proxy == nil {
			t.Error("expected Proxy metrics to be present")
		}
		if response.WebSocket == nil {
			t.Error("expected WebSocket metrics to be present")
		}
		if response.Auth == nil {
			t.Error("expected Auth metrics to be present")
		}
		if response.Discovery == nil {
			t.Error("expected Discovery metrics to be present")
		}
		if response.Health == nil {
			t.Error("expected Health metrics to be present")
		}
		if response.System == nil {
			t.Error("expected System metrics to be present")
		}
	})

	t.Run("rejects non-GET methods", func(t *testing.T) {
		methods := []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch}

		for _, method := range methods {
			req := httptest.NewRequest(method, "/api/v1/metrics/dashboard", nil)
			rec := httptest.NewRecorder()

			handler.HandleMetricsDashboard(rec, req)

			if rec.Code != http.StatusMethodNotAllowed {
				t.Errorf("%s: expected status %d, got %d", method, http.StatusMethodNotAllowed, rec.Code)
			}
		}
	})
}

func TestMetricsDashboardHandler_RecordsMetrics(t *testing.T) {
	// Create real metrics instance
	m := metrics.New("test_records")

	// Record some test metrics
	m.HTTPRequestsTotal.WithLabelValues("GET", "/api/test", "200").Add(100)
	m.HTTPRequestsTotal.WithLabelValues("GET", "/api/test", "500").Add(5)
	m.HTTPRequestDuration.WithLabelValues("GET", "/api/test").Observe(0.05)
	m.HTTPRequestDuration.WithLabelValues("GET", "/api/test").Observe(0.1)
	m.HTTPRequestDuration.WithLabelValues("GET", "/api/test").Observe(0.2)

	m.ProxyRequestsTotal.WithLabelValues("grafana", "200").Add(50)
	m.ProxyRequestsTotal.WithLabelValues("grafana", "502").Add(2)
	m.WebSocketConnectionsActive.Set(5)

	m.AuthPairingTotal.WithLabelValues("success", "ios").Add(10)
	m.AuthPairingTotal.WithLabelValues("failure", "android").Add(2)
	m.AuthJWTValidationsTotal.WithLabelValues("valid").Add(100)
	m.AuthJWTValidationsTotal.WithLabelValues("invalid").Add(3)

	m.DiscoveryScansTotal.WithLabelValues("docker", "success").Add(20)
	m.DiscoveryProposalsPending.Set(3)

	// Create handler
	handler := NewMetricsDashboardHandler(m)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/metrics/dashboard", nil)
	rec := httptest.NewRecorder()

	handler.HandleMetricsDashboard(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var response MetricsDashboardResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Verify HTTP metrics
	if response.HTTP.TotalRequests < 100 {
		t.Errorf("expected at least 100 total requests, got %d", response.HTTP.TotalRequests)
	}

	// Verify WebSocket metrics
	if response.WebSocket.ActiveConnections != 5 {
		t.Errorf("expected 5 active WebSocket connections, got %d", response.WebSocket.ActiveConnections)
	}

	// Verify Discovery metrics
	if response.Discovery.PendingProposals != 3 {
		t.Errorf("expected 3 pending proposals, got %d", response.Discovery.PendingProposals)
	}
}

func TestMetricsDashboardHandler_NilMetrics(t *testing.T) {
	// Test handler with nil metrics (graceful degradation)
	handler := NewMetricsDashboardHandler(nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/metrics/dashboard", nil)
	rec := httptest.NewRecorder()

	handler.HandleMetricsDashboard(rec, req)

	// Should return 503 Service Unavailable when metrics are nil
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status %d, got %d", http.StatusServiceUnavailable, rec.Code)
	}
}
