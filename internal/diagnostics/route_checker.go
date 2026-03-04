package diagnostics

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

// RouteCheckResult represents the result of a route diagnostic check
type RouteCheckResult struct {
	AppID        string        `json:"app_id"`
	RoutePath    string        `json:"route_path"`
	UpstreamURL  string        `json:"upstream_url"`
	Reachable    bool          `json:"reachable"`
	Latency      time.Duration `json:"latency_ms"`
	StatusCode   int           `json:"status_code,omitempty"`
	Error        string        `json:"error,omitempty"`
	CheckedAt    time.Time     `json:"checked_at"`
	ResponseTime time.Duration `json:"response_time_ms,omitempty"`
}

// RouteChecker performs diagnostic checks on routes
type RouteChecker struct {
	httpClient *http.Client
	timeout    time.Duration
}

// NewRouteChecker creates a new route checker with configurable timeout
func NewRouteChecker(timeout time.Duration) *RouteChecker {
	return &RouteChecker{
		httpClient: &http.Client{
			Timeout: timeout,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				// Don't follow redirects - we just want to know if the upstream responds
				return http.ErrUseLastResponse
			},
		},
		timeout: timeout,
	}
}

// CheckRoute performs a health check on a specific route
func (rc *RouteChecker) CheckRoute(ctx context.Context, appID, routePath, upstreamURL string) *RouteCheckResult {
	result := &RouteCheckResult{
		AppID:       appID,
		RoutePath:   routePath,
		UpstreamURL: upstreamURL,
		CheckedAt:   time.Now(),
	}

	// Measure latency
	start := time.Now()

	// Create request with context
	req, err := http.NewRequestWithContext(ctx, "GET", upstreamURL, nil)
	if err != nil {
		result.Reachable = false
		result.Error = fmt.Sprintf("failed to create request: %v", err)
		result.Latency = time.Since(start)
		return result
	}

	// Perform request
	resp, err := rc.httpClient.Do(req)
	result.ResponseTime = time.Since(start)
	result.Latency = result.ResponseTime

	if err != nil {
		result.Reachable = false
		result.Error = fmt.Sprintf("request failed: %v", err)
		return result
	}
	defer resp.Body.Close()

	// Success
	result.Reachable = true
	result.StatusCode = resp.StatusCode

	// Note: We consider any response (even 4xx/5xx) as "reachable"
	// because it means the upstream is responding
	if resp.StatusCode >= 400 {
		result.Error = fmt.Sprintf("upstream returned %d", resp.StatusCode)
	}

	return result
}

// CheckMultipleRoutes checks multiple routes concurrently
func (rc *RouteChecker) CheckMultipleRoutes(ctx context.Context, routes []RouteInfo) []*RouteCheckResult {
	results := make([]*RouteCheckResult, len(routes))
	resultChan := make(chan struct {
		index  int
		result *RouteCheckResult
	}, len(routes))

	// Launch concurrent checks
	for i, route := range routes {
		go func(idx int, r RouteInfo) {
			result := rc.CheckRoute(ctx, r.AppID, r.RoutePath, r.UpstreamURL)
			resultChan <- struct {
				index  int
				result *RouteCheckResult
			}{idx, result}
		}(i, route)
	}

	// Collect results
	for i := 0; i < len(routes); i++ {
		res := <-resultChan
		results[res.index] = res.result
	}

	return results
}

// RouteInfo holds information about a route to check
type RouteInfo struct {
	AppID       string
	RoutePath   string
	UpstreamURL string
}

// DiagnosticReport contains summary statistics for multiple route checks
type DiagnosticReport struct {
	TotalRoutes      int                 `json:"total_routes"`
	ReachableCount   int                 `json:"reachable_count"`
	UnreachableCount int                 `json:"unreachable_count"`
	AverageLatency   time.Duration       `json:"average_latency_ms"`
	MaxLatency       time.Duration       `json:"max_latency_ms"`
	MinLatency       time.Duration       `json:"min_latency_ms"`
	Results          []*RouteCheckResult `json:"results"`
	GeneratedAt      time.Time           `json:"generated_at"`
}

// GenerateReport creates a diagnostic report from route check results
func GenerateReport(results []*RouteCheckResult) *DiagnosticReport {
	report := &DiagnosticReport{
		TotalRoutes: len(results),
		Results:     results,
		GeneratedAt: time.Now(),
	}

	if len(results) == 0 {
		return report
	}

	report.MinLatency = time.Duration(1<<63 - 1) // Max int64

	var totalLatency time.Duration

	for _, result := range results {
		if result.Reachable {
			report.ReachableCount++
		} else {
			report.UnreachableCount++
		}

		totalLatency += result.Latency

		if result.Latency > report.MaxLatency {
			report.MaxLatency = result.Latency
		}
		if result.Latency < report.MinLatency {
			report.MinLatency = result.Latency
		}
	}

	report.AverageLatency = totalLatency / time.Duration(len(results))

	// Reset min latency if no results
	if report.MinLatency == time.Duration(1<<63-1) {
		report.MinLatency = 0
	}

	return report
}
