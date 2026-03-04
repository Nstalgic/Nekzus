package diagnostics

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRouteChecker(t *testing.T) {
	timeout := 5 * time.Second
	checker := NewRouteChecker(timeout)

	assert.NotNil(t, checker)
	assert.NotNil(t, checker.httpClient)
	assert.Equal(t, timeout, checker.timeout)
	assert.Equal(t, timeout, checker.httpClient.Timeout)
}

func TestCheckRoute_Success(t *testing.T) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))
	defer server.Close()

	checker := NewRouteChecker(5 * time.Second)
	result := checker.CheckRoute(context.Background(), "test-app", "/apps/test/", server.URL)

	assert.True(t, result.Reachable, "Route should be reachable")
	assert.Equal(t, "test-app", result.AppID)
	assert.Equal(t, "/apps/test/", result.RoutePath)
	assert.Equal(t, server.URL, result.UpstreamURL)
	assert.Equal(t, http.StatusOK, result.StatusCode)
	assert.Empty(t, result.Error)
	assert.Greater(t, result.Latency, time.Duration(0))
	assert.Greater(t, result.ResponseTime, time.Duration(0))
	assert.False(t, result.CheckedAt.IsZero())
}

func TestCheckRoute_Unreachable(t *testing.T) {
	checker := NewRouteChecker(1 * time.Second)

	// Use a non-routable IP address to simulate unreachable service
	result := checker.CheckRoute(context.Background(), "unreachable-app", "/apps/unreachable/", "http://192.0.2.1:9999")

	assert.False(t, result.Reachable, "Route should not be reachable")
	assert.NotEmpty(t, result.Error)
	assert.Equal(t, 0, result.StatusCode)
	assert.Greater(t, result.Latency, time.Duration(0))
}

func TestCheckRoute_UpstreamError(t *testing.T) {
	// Create test server that returns 500
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Internal Server Error"))
	}))
	defer server.Close()

	checker := NewRouteChecker(5 * time.Second)
	result := checker.CheckRoute(context.Background(), "error-app", "/apps/error/", server.URL)

	// Even with 500 error, the upstream is still "reachable"
	assert.True(t, result.Reachable, "Route should be reachable (upstream responding)")
	assert.Equal(t, http.StatusInternalServerError, result.StatusCode)
	assert.Contains(t, result.Error, "500")
}

func TestCheckRoute_SlowUpstream(t *testing.T) {
	// Create slow test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	checker := NewRouteChecker(5 * time.Second)
	result := checker.CheckRoute(context.Background(), "slow-app", "/apps/slow/", server.URL)

	assert.True(t, result.Reachable)
	assert.GreaterOrEqual(t, result.Latency, 100*time.Millisecond)
	assert.GreaterOrEqual(t, result.ResponseTime, 100*time.Millisecond)
}

func TestCheckRoute_Timeout(t *testing.T) {
	// Create server that hangs
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second) // Longer than client timeout
	}))
	defer server.Close()

	checker := NewRouteChecker(100 * time.Millisecond)
	result := checker.CheckRoute(context.Background(), "timeout-app", "/apps/timeout/", server.URL)

	assert.False(t, result.Reachable)
	assert.NotEmpty(t, result.Error)
	assert.Contains(t, result.Error, "exceeded")
}

func TestCheckRoute_ContextCancellation(t *testing.T) {
	// Create server that hangs
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second)
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	checker := NewRouteChecker(5 * time.Second)
	result := checker.CheckRoute(ctx, "cancelled-app", "/apps/cancelled/", server.URL)

	assert.False(t, result.Reachable)
	assert.NotEmpty(t, result.Error)
}

func TestCheckMultipleRoutes(t *testing.T) {
	// Create multiple test servers
	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server1.Close()

	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server2.Close()

	routes := []RouteInfo{
		{AppID: "app1", RoutePath: "/apps/app1/", UpstreamURL: server1.URL},
		{AppID: "app2", RoutePath: "/apps/app2/", UpstreamURL: server2.URL},
		{AppID: "app3", RoutePath: "/apps/app3/", UpstreamURL: "http://192.0.2.1:9999"}, // Unreachable
	}

	checker := NewRouteChecker(1 * time.Second)
	results := checker.CheckMultipleRoutes(context.Background(), routes)

	require.Len(t, results, 3)

	// Check that results match routes
	assert.Equal(t, "app1", results[0].AppID)
	assert.True(t, results[0].Reachable)

	assert.Equal(t, "app2", results[1].AppID)
	assert.True(t, results[1].Reachable)

	assert.Equal(t, "app3", results[2].AppID)
	assert.False(t, results[2].Reachable)
}

func TestGenerateReport(t *testing.T) {
	results := []*RouteCheckResult{
		{
			AppID:      "app1",
			Reachable:  true,
			Latency:    50 * time.Millisecond,
			StatusCode: 200,
		},
		{
			AppID:      "app2",
			Reachable:  true,
			Latency:    100 * time.Millisecond,
			StatusCode: 200,
		},
		{
			AppID:     "app3",
			Reachable: false,
			Latency:   1 * time.Second,
			Error:     "connection refused",
		},
	}

	report := GenerateReport(results)

	assert.Equal(t, 3, report.TotalRoutes)
	assert.Equal(t, 2, report.ReachableCount)
	assert.Equal(t, 1, report.UnreachableCount)
	assert.Equal(t, 50*time.Millisecond, report.MinLatency)
	assert.Equal(t, 1*time.Second, report.MaxLatency)

	// Average: (50 + 100 + 1000) / 3 = 383.33ms
	expectedAvg := (50*time.Millisecond + 100*time.Millisecond + 1*time.Second) / 3
	assert.Equal(t, expectedAvg, report.AverageLatency)

	assert.Len(t, report.Results, 3)
	assert.False(t, report.GeneratedAt.IsZero())
}

func TestGenerateReport_Empty(t *testing.T) {
	report := GenerateReport([]*RouteCheckResult{})

	assert.Equal(t, 0, report.TotalRoutes)
	assert.Equal(t, 0, report.ReachableCount)
	assert.Equal(t, 0, report.UnreachableCount)
	assert.Equal(t, time.Duration(0), report.MinLatency)
	assert.Equal(t, time.Duration(0), report.MaxLatency)
	assert.Equal(t, time.Duration(0), report.AverageLatency)
	assert.Empty(t, report.Results)
}

func TestCheckRoute_Redirect(t *testing.T) {
	// Create test server that redirects
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/redirected", http.StatusFound)
	}))
	defer server.Close()

	checker := NewRouteChecker(5 * time.Second)
	result := checker.CheckRoute(context.Background(), "redirect-app", "/apps/redirect/", server.URL)

	// Should not follow redirects, just report the redirect response
	assert.True(t, result.Reachable)
	assert.Equal(t, http.StatusFound, result.StatusCode)
}
