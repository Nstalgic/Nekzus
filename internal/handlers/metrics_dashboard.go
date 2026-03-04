package handlers

import (
	"net/http"

	apperrors "github.com/nstalgic/nekzus/internal/errors"
	"github.com/nstalgic/nekzus/internal/httputil"
	"github.com/nstalgic/nekzus/internal/metrics"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

// MetricsDashboardHandler handles metrics dashboard API endpoints
type MetricsDashboardHandler struct {
	metrics *metrics.Metrics
}

// NewMetricsDashboardHandler creates a new metrics dashboard handler
func NewMetricsDashboardHandler(m *metrics.Metrics) *MetricsDashboardHandler {
	return &MetricsDashboardHandler{
		metrics: m,
	}
}

// MetricsDashboardResponse is the response structure for the metrics dashboard
type MetricsDashboardResponse struct {
	HTTP      *HTTPMetrics      `json:"http"`
	Proxy     *ProxyMetrics     `json:"proxy"`
	WebSocket *WebSocketMetrics `json:"websocket"`
	Auth      *AuthMetrics      `json:"auth"`
	Discovery *DiscoveryMetrics `json:"discovery"`
	Health    *HealthMetrics    `json:"health"`
	System    *SystemMetrics    `json:"system"`
}

// HTTPMetrics contains HTTP-related metrics
type HTTPMetrics struct {
	TotalRequests    int64            `json:"totalRequests"`
	RequestsPerSec   float64          `json:"requestsPerSec"`
	ErrorRate        float64          `json:"errorRate"`
	AvgLatencyMs     float64          `json:"avgLatencyMs"`
	P50LatencyMs     float64          `json:"p50LatencyMs"`
	P95LatencyMs     float64          `json:"p95LatencyMs"`
	P99LatencyMs     float64          `json:"p99LatencyMs"`
	InFlightRequests int64            `json:"inFlightRequests"`
	ByStatus         map[string]int64 `json:"byStatus"`
	ByMethod         map[string]int64 `json:"byMethod"`
	TopPaths         []PathMetric     `json:"topPaths"`
}

// PathMetric represents metrics for a specific path
type PathMetric struct {
	Path     string  `json:"path"`
	Count    int64   `json:"count"`
	AvgMs    float64 `json:"avgMs"`
	ErrorPct float64 `json:"errorPct"`
}

// ProxyMetrics contains proxy-related metrics
type ProxyMetrics struct {
	TotalRequests  int64            `json:"totalRequests"`
	ActiveSessions int64            `json:"activeSessions"`
	BytesIn        int64            `json:"bytesIn"`
	BytesOut       int64            `json:"bytesOut"`
	UpstreamErrors int64            `json:"upstreamErrors"`
	ByApp          []AppProxyMetric `json:"byApp"`
}

// AppProxyMetric represents proxy metrics for a specific app
type AppProxyMetric struct {
	AppID    string  `json:"appId"`
	Requests int64   `json:"requests"`
	Errors   int64   `json:"errors"`
	AvgMs    float64 `json:"avgMs"`
	BytesIn  int64   `json:"bytesIn"`
	BytesOut int64   `json:"bytesOut"`
}

// WebSocketMetrics contains WebSocket-related metrics
type WebSocketMetrics struct {
	ActiveConnections int64                `json:"activeConnections"`
	TotalConnections  int64                `json:"totalConnections"`
	AvgDurationSec    float64              `json:"avgDurationSec"`
	TotalMessages     int64                `json:"totalMessages"`
	TotalBytesIn      int64                `json:"totalBytesIn"`
	TotalBytesOut     int64                `json:"totalBytesOut"`
	ByApp             []AppWebSocketMetric `json:"byApp"`
}

// AppWebSocketMetric represents WebSocket metrics for a specific app
type AppWebSocketMetric struct {
	AppID       string `json:"appId"`
	Connections int64  `json:"connections"`
	MessagesIn  int64  `json:"messagesIn"`
	MessagesOut int64  `json:"messagesOut"`
}

// AuthMetrics contains authentication-related metrics
type AuthMetrics struct {
	PairingSuccess     int64   `json:"pairingSuccess"`
	PairingFailure     int64   `json:"pairingFailure"`
	PairingSuccessRate float64 `json:"pairingSuccessRate"`
	JWTValidations     int64   `json:"jwtValidations"`
	JWTFailures        int64   `json:"jwtFailures"`
	JWTSuccessRate     float64 `json:"jwtSuccessRate"`
	TokenRefreshes     int64   `json:"tokenRefreshes"`
	LocalAuthBypasses  int64   `json:"localAuthBypasses"`
	ActiveBootstrap    int64   `json:"activeBootstrap"`
}

// DiscoveryMetrics contains discovery-related metrics
type DiscoveryMetrics struct {
	TotalScans       int64          `json:"totalScans"`
	TotalProposals   int64          `json:"totalProposals"`
	PendingProposals int64          `json:"pendingProposals"`
	ActiveWorkers    int64          `json:"activeWorkers"`
	BySource         []SourceMetric `json:"bySource"`
}

// SourceMetric represents discovery metrics for a specific source
type SourceMetric struct {
	Source    string  `json:"source"`
	Scans     int64   `json:"scans"`
	Proposals int64   `json:"proposals"`
	AvgScanMs float64 `json:"avgScanMs"`
}

// HealthMetrics contains health check metrics
type HealthMetrics struct {
	UptimePercent   float64           `json:"uptimePercent"`
	TotalChecks     int64             `json:"totalChecks"`
	HealthyChecks   int64             `json:"healthyChecks"`
	ComponentStatus []ComponentHealth `json:"componentStatus"`
	ServiceStatus   []ServiceHealth   `json:"serviceStatus"`
}

// ComponentHealth represents health status for a component
type ComponentHealth struct {
	Component string  `json:"component"`
	Status    int     `json:"status"` // 0=unknown, 1=healthy, 2=degraded, 3=unhealthy
	CheckMs   float64 `json:"checkMs"`
}

// ServiceHealth represents health status for a service
type ServiceHealth struct {
	AppID   string  `json:"appId"`
	Status  int     `json:"status"` // 0=unknown, 1=healthy, 2=unhealthy
	CheckMs float64 `json:"checkMs"`
}

// SystemMetrics contains system-level metrics
type SystemMetrics struct {
	UptimeSeconds     float64 `json:"uptimeSeconds"`
	StartTime         int64   `json:"startTime"`
	ConfigReloads     int64   `json:"configReloads"`
	LastReloadStatus  int     `json:"lastReloadStatus"` // 0=unknown, 1=success, 2=error
	CertificatesTotal int64   `json:"certificatesTotal"`
	NotificationQueue int64   `json:"notificationQueue"`
}

// HandleMetricsDashboard returns aggregated metrics for the dashboard
// GET /api/v1/metrics/dashboard
func (h *MetricsDashboardHandler) HandleMetricsDashboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		apperrors.WriteJSON(w, apperrors.New("METHOD_NOT_ALLOWED", "Method not allowed", http.StatusMethodNotAllowed))
		return
	}

	if h.metrics == nil {
		apperrors.WriteJSON(w, apperrors.New("METRICS_UNAVAILABLE", "Metrics not available", http.StatusServiceUnavailable))
		return
	}

	response := h.collectMetrics()

	if err := httputil.WriteJSON(w, http.StatusOK, response); err != nil {
		statslog.Error("failed to encode metrics dashboard response", "error", err)
	}
}

// collectMetrics gathers all metrics into the response structure
func (h *MetricsDashboardHandler) collectMetrics() *MetricsDashboardResponse {
	return &MetricsDashboardResponse{
		HTTP:      h.collectHTTPMetrics(),
		Proxy:     h.collectProxyMetrics(),
		WebSocket: h.collectWebSocketMetrics(),
		Auth:      h.collectAuthMetrics(),
		Discovery: h.collectDiscoveryMetrics(),
		Health:    h.collectHealthMetrics(),
		System:    h.collectSystemMetrics(),
	}
}

// collectHTTPMetrics collects HTTP-related metrics
func (h *MetricsDashboardHandler) collectHTTPMetrics() *HTTPMetrics {
	metrics := &HTTPMetrics{
		ByStatus: make(map[string]int64),
		ByMethod: make(map[string]int64),
		TopPaths: make([]PathMetric, 0),
	}

	// Collect request totals
	total, _ := h.metrics.GetHTTPRequestsTotal()
	metrics.TotalRequests = int64(total)

	// Get average latency
	metrics.AvgLatencyMs = h.metrics.GetAverageLatency()

	// Get in-flight requests
	metrics.InFlightRequests = int64(getGaugeValue(h.metrics.HTTPRequestsInFlight))

	// Collect by status and method from counter vec
	h.collectCounterVecByLabel(h.metrics.HTTPRequestsTotal, func(labels map[string]string, value float64) {
		if status, ok := labels["status"]; ok {
			metrics.ByStatus[status] += int64(value)
		}
		if method, ok := labels["method"]; ok {
			metrics.ByMethod[method] += int64(value)
		}
	})

	// Calculate error rate (5xx responses)
	var errorCount int64
	for status, count := range metrics.ByStatus {
		if len(status) > 0 && status[0] == '5' {
			errorCount += count
		}
	}
	if metrics.TotalRequests > 0 {
		metrics.ErrorRate = float64(errorCount) / float64(metrics.TotalRequests) * 100
	}

	// Calculate latency percentiles from histogram
	metrics.P50LatencyMs, metrics.P95LatencyMs, metrics.P99LatencyMs = h.getLatencyPercentiles()

	return metrics
}

// collectProxyMetrics collects proxy-related metrics
func (h *MetricsDashboardHandler) collectProxyMetrics() *ProxyMetrics {
	metrics := &ProxyMetrics{
		ByApp: make([]AppProxyMetric, 0),
	}

	metrics.ActiveSessions = int64(getGaugeValue(h.metrics.ProxyActiveSessions))

	// Collect by app
	appMetrics := make(map[string]*AppProxyMetric)

	h.collectCounterVecByLabel(h.metrics.ProxyRequestsTotal, func(labels map[string]string, value float64) {
		appID := labels["app_id"]
		if appID == "" {
			return
		}

		if _, ok := appMetrics[appID]; !ok {
			appMetrics[appID] = &AppProxyMetric{AppID: appID}
		}

		status := labels["status"]
		appMetrics[appID].Requests += int64(value)
		metrics.TotalRequests += int64(value)

		if len(status) > 0 && status[0] == '5' {
			appMetrics[appID].Errors += int64(value)
			metrics.UpstreamErrors += int64(value)
		}
	})

	// Collect bytes transferred
	h.collectCounterVecByLabel(h.metrics.ProxyBytesTransferred, func(labels map[string]string, value float64) {
		appID := labels["app_id"]
		direction := labels["direction"]

		if appID != "" {
			if _, ok := appMetrics[appID]; !ok {
				appMetrics[appID] = &AppProxyMetric{AppID: appID}
			}

			if direction == "in" {
				appMetrics[appID].BytesIn += int64(value)
				metrics.BytesIn += int64(value)
			} else if direction == "out" {
				appMetrics[appID].BytesOut += int64(value)
				metrics.BytesOut += int64(value)
			}
		}
	})

	// Convert map to slice
	for _, app := range appMetrics {
		metrics.ByApp = append(metrics.ByApp, *app)
	}

	return metrics
}

// collectWebSocketMetrics collects WebSocket-related metrics
func (h *MetricsDashboardHandler) collectWebSocketMetrics() *WebSocketMetrics {
	metrics := &WebSocketMetrics{
		ByApp: make([]AppWebSocketMetric, 0),
	}

	metrics.ActiveConnections = int64(getGaugeValue(h.metrics.WebSocketConnectionsActive))

	// Collect connection totals
	h.collectCounterVecByLabel(h.metrics.WebSocketConnectionsTotal, func(labels map[string]string, value float64) {
		metrics.TotalConnections += int64(value)
	})

	// Collect messages
	appMetrics := make(map[string]*AppWebSocketMetric)

	h.collectCounterVecByLabel(h.metrics.WebSocketMessagesTotal, func(labels map[string]string, value float64) {
		appID := labels["app_id"]
		direction := labels["direction"]

		if appID != "" {
			if _, ok := appMetrics[appID]; !ok {
				appMetrics[appID] = &AppWebSocketMetric{AppID: appID}
			}

			if direction == "in" {
				appMetrics[appID].MessagesIn += int64(value)
			} else if direction == "out" {
				appMetrics[appID].MessagesOut += int64(value)
			}
			metrics.TotalMessages += int64(value)
		}
	})

	// Collect bytes
	h.collectCounterVecByLabel(h.metrics.WebSocketBytesTransferred, func(labels map[string]string, value float64) {
		direction := labels["direction"]
		if direction == "in" {
			metrics.TotalBytesIn += int64(value)
		} else if direction == "out" {
			metrics.TotalBytesOut += int64(value)
		}
	})

	// Convert map to slice
	for _, app := range appMetrics {
		metrics.ByApp = append(metrics.ByApp, *app)
	}

	return metrics
}

// collectAuthMetrics collects authentication-related metrics
func (h *MetricsDashboardHandler) collectAuthMetrics() *AuthMetrics {
	metrics := &AuthMetrics{}

	// Pairing metrics
	h.collectCounterVecByLabel(h.metrics.AuthPairingTotal, func(labels map[string]string, value float64) {
		status := labels["status"]
		if status == "success" {
			metrics.PairingSuccess += int64(value)
		} else if status == "failure" || status == "error" {
			metrics.PairingFailure += int64(value)
		}
	})

	totalPairing := metrics.PairingSuccess + metrics.PairingFailure
	if totalPairing > 0 {
		metrics.PairingSuccessRate = float64(metrics.PairingSuccess) / float64(totalPairing) * 100
	}

	// JWT validation metrics
	h.collectCounterVecByLabel(h.metrics.AuthJWTValidationsTotal, func(labels map[string]string, value float64) {
		status := labels["status"]
		if status == "valid" || status == "success" {
			metrics.JWTValidations += int64(value)
		} else {
			metrics.JWTFailures += int64(value)
		}
	})

	totalJWT := metrics.JWTValidations + metrics.JWTFailures
	if totalJWT > 0 {
		metrics.JWTSuccessRate = float64(metrics.JWTValidations) / float64(totalJWT) * 100
	}

	// Token refresh metrics
	h.collectCounterVecByLabel(h.metrics.AuthRefreshTotal, func(labels map[string]string, value float64) {
		metrics.TokenRefreshes += int64(value)
	})

	// Local auth bypasses
	h.collectCounterVecByLabel(h.metrics.AuthLocalAuthTotal, func(labels map[string]string, value float64) {
		metrics.LocalAuthBypasses += int64(value)
	})

	// Active bootstrap tokens
	metrics.ActiveBootstrap = int64(getGaugeValue(h.metrics.AuthBootstrapTokensActive))

	return metrics
}

// collectDiscoveryMetrics collects discovery-related metrics
func (h *MetricsDashboardHandler) collectDiscoveryMetrics() *DiscoveryMetrics {
	metrics := &DiscoveryMetrics{
		BySource: make([]SourceMetric, 0),
	}

	metrics.PendingProposals = int64(getGaugeValue(h.metrics.DiscoveryProposalsPending))
	metrics.ActiveWorkers = int64(getGaugeValue(h.metrics.DiscoveryWorkersActive))

	// Collect by source
	sourceMetrics := make(map[string]*SourceMetric)

	h.collectCounterVecByLabel(h.metrics.DiscoveryScansTotal, func(labels map[string]string, value float64) {
		source := labels["source"]
		if source == "" {
			return
		}

		if _, ok := sourceMetrics[source]; !ok {
			sourceMetrics[source] = &SourceMetric{Source: source}
		}
		sourceMetrics[source].Scans += int64(value)
		metrics.TotalScans += int64(value)
	})

	h.collectCounterVecByLabel(h.metrics.DiscoveryProposalsTotal, func(labels map[string]string, value float64) {
		source := labels["source"]
		if source == "" {
			return
		}

		if _, ok := sourceMetrics[source]; !ok {
			sourceMetrics[source] = &SourceMetric{Source: source}
		}
		sourceMetrics[source].Proposals += int64(value)
		metrics.TotalProposals += int64(value)
	})

	// Convert map to slice
	for _, src := range sourceMetrics {
		metrics.BySource = append(metrics.BySource, *src)
	}

	return metrics
}

// collectHealthMetrics collects health check metrics
func (h *MetricsDashboardHandler) collectHealthMetrics() *HealthMetrics {
	metrics := &HealthMetrics{
		ComponentStatus: make([]ComponentHealth, 0),
		ServiceStatus:   make([]ServiceHealth, 0),
	}

	metrics.UptimePercent = h.metrics.GetUptimePercent()

	// Collect component health
	componentStatus := make(map[string]int)
	h.collectGaugeVecByLabel(h.metrics.HealthCheckStatus, func(labels map[string]string, value float64) {
		component := labels["component"]
		if component != "" {
			componentStatus[component] = int(value)
		}
	})

	for component, status := range componentStatus {
		metrics.ComponentStatus = append(metrics.ComponentStatus, ComponentHealth{
			Component: component,
			Status:    status,
		})
	}

	// Collect service health
	serviceStatus := make(map[string]int)
	h.collectGaugeVecByLabel(h.metrics.ServiceHealthStatus, func(labels map[string]string, value float64) {
		appID := labels["app_id"]
		if appID != "" {
			serviceStatus[appID] = int(value)
		}
	})

	for appID, status := range serviceStatus {
		metrics.ServiceStatus = append(metrics.ServiceStatus, ServiceHealth{
			AppID:  appID,
			Status: status,
		})
	}

	// Collect health check counts
	h.collectCounterVecByLabel(h.metrics.HealthChecksTotal, func(labels map[string]string, value float64) {
		status := labels["status"]
		metrics.TotalChecks += int64(value)
		if status == "healthy" {
			metrics.HealthyChecks += int64(value)
		}
	})

	return metrics
}

// collectSystemMetrics collects system-level metrics
func (h *MetricsDashboardHandler) collectSystemMetrics() *SystemMetrics {
	metrics := &SystemMetrics{}

	metrics.UptimeSeconds = getGaugeValue(h.metrics.UptimeSeconds)
	metrics.StartTime = int64(getGaugeValue(h.metrics.StartTime))
	metrics.LastReloadStatus = int(getGaugeValue(h.metrics.ConfigLastReloadStatus))
	metrics.CertificatesTotal = int64(getGaugeValue(h.metrics.CertificatesTotal))
	metrics.NotificationQueue = int64(getGaugeValue(h.metrics.NotificationQueueDepth))

	// Count config reloads
	h.collectCounterVecByLabel(h.metrics.ConfigReloadsTotal, func(labels map[string]string, value float64) {
		metrics.ConfigReloads += int64(value)
	})

	return metrics
}

// Helper functions

// getGaugeValue safely gets the current value of a Gauge
func getGaugeValue(g prometheus.Gauge) float64 {
	if g == nil {
		return 0
	}

	ch := make(chan prometheus.Metric, 1)
	g.Collect(ch)
	close(ch)

	metric := <-ch
	if metric == nil {
		return 0
	}

	var m dto.Metric
	if err := metric.Write(&m); err != nil {
		return 0
	}

	if m.Gauge != nil && m.Gauge.Value != nil {
		return *m.Gauge.Value
	}
	return 0
}

// collectCounterVecByLabel iterates over a CounterVec and calls the callback for each label set
func (h *MetricsDashboardHandler) collectCounterVecByLabel(cv *prometheus.CounterVec, callback func(labels map[string]string, value float64)) {
	if cv == nil {
		return
	}

	ch := make(chan prometheus.Metric, 100)
	go func() {
		cv.Collect(ch)
		close(ch)
	}()

	for metric := range ch {
		var m dto.Metric
		if err := metric.Write(&m); err != nil {
			continue
		}

		labels := make(map[string]string)
		for _, lp := range m.Label {
			if lp.Name != nil && lp.Value != nil {
				labels[*lp.Name] = *lp.Value
			}
		}

		var value float64
		if m.Counter != nil && m.Counter.Value != nil {
			value = *m.Counter.Value
		}

		callback(labels, value)
	}
}

// collectGaugeVecByLabel iterates over a GaugeVec and calls the callback for each label set
func (h *MetricsDashboardHandler) collectGaugeVecByLabel(gv *prometheus.GaugeVec, callback func(labels map[string]string, value float64)) {
	if gv == nil {
		return
	}

	ch := make(chan prometheus.Metric, 100)
	go func() {
		gv.Collect(ch)
		close(ch)
	}()

	for metric := range ch {
		var m dto.Metric
		if err := metric.Write(&m); err != nil {
			continue
		}

		labels := make(map[string]string)
		for _, lp := range m.Label {
			if lp.Name != nil && lp.Value != nil {
				labels[*lp.Name] = *lp.Value
			}
		}

		var value float64
		if m.Gauge != nil && m.Gauge.Value != nil {
			value = *m.Gauge.Value
		}

		callback(labels, value)
	}
}

// getLatencyPercentiles calculates P50, P95, P99 from the histogram
func (h *MetricsDashboardHandler) getLatencyPercentiles() (p50, p95, p99 float64) {
	metricFamilies, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		return 0, 0, 0
	}

	for _, mf := range metricFamilies {
		if mf.GetName() != "nekzus_http_request_duration_seconds" &&
			mf.GetName() != "test_http_request_duration_seconds" {
			continue
		}

		for _, metric := range mf.GetMetric() {
			hist := metric.GetHistogram()
			if hist == nil {
				continue
			}

			totalCount := hist.GetSampleCount()
			if totalCount == 0 {
				continue
			}

			// Calculate percentiles from cumulative buckets
			p50Target := float64(totalCount) * 0.5
			p95Target := float64(totalCount) * 0.95
			p99Target := float64(totalCount) * 0.99

			buckets := hist.GetBucket()
			for _, bucket := range buckets {
				cumCount := float64(bucket.GetCumulativeCount())
				upperBound := bucket.GetUpperBound() * 1000 // Convert to ms

				if p50 == 0 && cumCount >= p50Target {
					p50 = upperBound
				}
				if p95 == 0 && cumCount >= p95Target {
					p95 = upperBound
				}
				if p99 == 0 && cumCount >= p99Target {
					p99 = upperBound
				}
			}
		}
	}

	return p50, p95, p99
}
