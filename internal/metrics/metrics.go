package metrics

import (
	"runtime"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	dto "github.com/prometheus/client_model/go"
)

// Metrics holds all Prometheus metrics for Nekzus
type Metrics struct {
	// HTTP Metrics
	HTTPRequestsTotal     *prometheus.CounterVec
	HTTPRequestDuration   *prometheus.HistogramVec
	HTTPRequestsInFlight  prometheus.Gauge
	HTTPResponseSizeBytes *prometheus.HistogramVec
	HTTPRequestSizeBytes  *prometheus.HistogramVec

	// Authentication Metrics
	AuthPairingTotal          *prometheus.CounterVec
	AuthPairingDuration       prometheus.Histogram
	AuthRefreshTotal          *prometheus.CounterVec
	AuthJWTValidationsTotal   *prometheus.CounterVec
	AuthLocalAuthTotal        *prometheus.CounterVec
	AuthBootstrapTokensActive prometheus.Gauge

	// Device Management Metrics
	DevicesTotal          prometheus.Gauge
	DevicesOnline         prometheus.Gauge
	DevicesOffline        prometheus.Gauge
	DeviceOperationsTotal *prometheus.CounterVec
	DeviceLastSeenAge     *prometheus.GaugeVec

	// Proxy Metrics
	ProxyRequestsTotal    *prometheus.CounterVec
	ProxyRequestDuration  *prometheus.HistogramVec
	ProxyUpstreamErrors   *prometheus.CounterVec
	ProxyActiveSessions   prometheus.Gauge
	ProxyBytesTransferred *prometheus.CounterVec

	// WebSocket Metrics
	WebSocketConnectionsActive  prometheus.Gauge
	WebSocketConnectionsTotal   *prometheus.CounterVec
	WebSocketConnectionDuration prometheus.Histogram
	WebSocketMessagesTotal      *prometheus.CounterVec
	WebSocketBytesTransferred   *prometheus.CounterVec

	// Webhook Metrics
	WebhookRequestsTotal   *prometheus.CounterVec
	WebhookRequestDuration *prometheus.HistogramVec

	// Discovery Metrics
	DiscoveryProposalsTotal   *prometheus.CounterVec
	DiscoveryScansTotal       *prometheus.CounterVec
	DiscoveryScanDuration     *prometheus.HistogramVec
	DiscoveryProposalsPending prometheus.Gauge
	DiscoveryWorkersActive    prometheus.Gauge

	// Storage Metrics
	StorageOperationsTotal   *prometheus.CounterVec
	StorageOperationDuration *prometheus.HistogramVec
	StorageAppsTotal         prometheus.Gauge
	StorageRoutesTotal       prometheus.Gauge

	// SSE/Events Metrics
	SSEConnectionsActive  prometheus.Gauge
	SSEEventsPublished    *prometheus.CounterVec
	SSEConnectionDuration prometheus.Histogram

	// System Metrics
	BuildInfo     *prometheus.GaugeVec
	StartTime     prometheus.Gauge
	UptimeSeconds prometheus.Gauge

	// Health Check Metrics
	HealthCheckStatus   *prometheus.GaugeVec
	HealthCheckDuration *prometheus.HistogramVec
	HealthChecksTotal   *prometheus.CounterVec

	// Service Health Metrics
	ServiceHealthStatus        *prometheus.GaugeVec
	ServiceHealthChecksTotal   *prometheus.CounterVec
	ServiceHealthCheckDuration *prometheus.HistogramVec

	// Config Reload Metrics
	ConfigReloadsTotal     *prometheus.CounterVec
	ConfigReloadDuration   prometheus.Histogram
	ConfigLastReloadTime   prometheus.Gauge
	ConfigLastReloadStatus prometheus.Gauge

	// Certificate Metrics
	CertificateExpirySeconds    *prometheus.GaugeVec
	CertificateGenerateTotal    *prometheus.CounterVec
	CertificateGenerateErrors   *prometheus.CounterVec
	CertificateGenerateDuration *prometheus.HistogramVec
	CertificateRenewTotal       *prometheus.CounterVec
	CertificateServeTotal       prometheus.Counter
	CertificateMissTotal        prometheus.Counter
	CertificatesTotal           prometheus.Gauge

	// Notification Queue Metrics
	NotificationQueueDepth        prometheus.Gauge
	NotificationsEnqueuedTotal    prometheus.Counter
	NotificationsDeliveredTotal   *prometheus.CounterVec
	NotificationsFailedTotal      *prometheus.CounterVec
	NotificationsExpiredTotal     prometheus.Counter
	NotificationsRetriedTotal     prometheus.Counter
	NotificationStorageFallsTotal prometheus.Counter
	NotificationDeliveryDuration  *prometheus.HistogramVec
	NotificationQueueWorkers      prometheus.Gauge

	// Federation Metrics
	FederationPeersActive      prometheus.Gauge
	FederationServicesTotal    *prometheus.GaugeVec
	FederationSyncTotal        *prometheus.CounterVec
	FederationSyncErrors       *prometheus.CounterVec
	FederationSyncDuration     prometheus.Histogram
	FederationMessagesSent     prometheus.Counter
	FederationMessagesReceived prometheus.Counter
}

// New creates and registers all Prometheus metrics
func New(namespace string) *Metrics {
	if namespace == "" {
		namespace = "nekzus"
	}

	m := &Metrics{
		// HTTP Metrics
		HTTPRequestsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "http_requests_total",
				Help:      "Total number of HTTP requests processed",
			},
			[]string{"method", "path", "status"},
		),
		HTTPRequestDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "http_request_duration_seconds",
				Help:      "HTTP request latency in seconds",
				Buckets:   []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
			},
			[]string{"method", "path"},
		),
		HTTPRequestsInFlight: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "http_requests_in_flight",
				Help:      "Current number of HTTP requests being processed",
			},
		),
		HTTPResponseSizeBytes: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "http_response_size_bytes",
				Help:      "HTTP response size in bytes",
				Buckets:   prometheus.ExponentialBuckets(100, 10, 8),
			},
			[]string{"method", "path"},
		),
		HTTPRequestSizeBytes: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "http_request_size_bytes",
				Help:      "HTTP request size in bytes",
				Buckets:   prometheus.ExponentialBuckets(100, 10, 8),
			},
			[]string{"method", "path"},
		),

		// Authentication Metrics
		AuthPairingTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "auth_pairing_total",
				Help:      "Total number of device pairing attempts",
			},
			[]string{"status", "platform"},
		),
		AuthPairingDuration: promauto.NewHistogram(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "auth_pairing_duration_seconds",
				Help:      "Device pairing operation duration",
				Buckets:   prometheus.DefBuckets,
			},
		),
		AuthRefreshTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "auth_refresh_total",
				Help:      "Total number of token refresh attempts",
			},
			[]string{"status"},
		),
		AuthJWTValidationsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "auth_jwt_validations_total",
				Help:      "Total number of JWT validation attempts",
			},
			[]string{"status"},
		),
		AuthLocalAuthTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "auth_local_auth_total",
				Help:      "Total number of local (localhost/LAN) authentication bypasses",
			},
			[]string{"status"},
		),
		AuthBootstrapTokensActive: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "auth_bootstrap_tokens_active",
				Help:      "Number of active bootstrap tokens",
			},
		),

		// Device Management Metrics
		DevicesTotal: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "devices_total",
				Help:      "Total number of paired devices",
			},
		),
		DevicesOnline: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "devices_online",
				Help:      "Number of devices currently online",
			},
		),
		DevicesOffline: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "devices_offline",
				Help:      "Number of devices currently offline",
			},
		),
		DeviceOperationsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "device_operations_total",
				Help:      "Total number of device operations",
			},
			[]string{"operation", "status"},
		),
		DeviceLastSeenAge: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "device_last_seen_age_seconds",
				Help:      "Time since device last seen in seconds",
			},
			[]string{"device_id"},
		),

		// Proxy Metrics
		ProxyRequestsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "proxy_requests_total",
				Help:      "Total number of proxy requests",
			},
			[]string{"app_id", "status"},
		),
		ProxyRequestDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "proxy_request_duration_seconds",
				Help:      "Proxy request duration in seconds",
				Buckets:   []float64{.01, .05, .1, .25, .5, 1, 2.5, 5, 10, 30},
			},
			[]string{"app_id"},
		),
		ProxyUpstreamErrors: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "proxy_upstream_errors_total",
				Help:      "Total number of upstream errors",
			},
			[]string{"app_id", "error_type"},
		),
		ProxyActiveSessions: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "proxy_active_sessions",
				Help:      "Current number of active proxy sessions",
			},
		),
		ProxyBytesTransferred: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "proxy_bytes_transferred_total",
				Help:      "Total bytes transferred through proxy",
			},
			[]string{"app_id", "direction"},
		),

		// WebSocket Metrics
		WebSocketConnectionsActive: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "websocket_connections_active",
				Help:      "Current number of active WebSocket connections",
			},
		),
		WebSocketConnectionsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "websocket_connections_total",
				Help:      "Total number of WebSocket connections",
			},
			[]string{"app_id", "status"},
		),
		WebSocketConnectionDuration: promauto.NewHistogram(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "websocket_connection_duration_seconds",
				Help:      "WebSocket connection duration in seconds",
				Buckets:   []float64{1, 10, 30, 60, 300, 600, 1800, 3600, 7200},
			},
		),
		WebSocketMessagesTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "websocket_messages_total",
				Help:      "Total number of WebSocket messages",
			},
			[]string{"app_id", "direction"},
		),
		WebSocketBytesTransferred: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "websocket_bytes_transferred_total",
				Help:      "Total bytes transferred over WebSocket",
			},
			[]string{"app_id", "direction"},
		),

		// Webhook Metrics
		WebhookRequestsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "webhook_requests_total",
				Help:      "Total number of webhook requests",
			},
			[]string{"endpoint", "status"},
		),
		WebhookRequestDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "webhook_request_duration_seconds",
				Help:      "Webhook request duration in seconds",
				Buckets:   prometheus.DefBuckets,
			},
			[]string{"endpoint"},
		),

		// Discovery Metrics
		DiscoveryProposalsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "discovery_proposals_total",
				Help:      "Total number of discovery proposals",
			},
			[]string{"source", "status"},
		),
		DiscoveryScansTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "discovery_scans_total",
				Help:      "Total number of discovery scans",
			},
			[]string{"source", "status"},
		),
		DiscoveryScanDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "discovery_scan_duration_seconds",
				Help:      "Discovery scan duration in seconds",
				Buckets:   []float64{.5, 1, 2, 5, 10, 30, 60},
			},
			[]string{"source"},
		),
		DiscoveryProposalsPending: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "discovery_proposals_pending",
				Help:      "Number of pending discovery proposals",
			},
		),
		DiscoveryWorkersActive: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "discovery_workers_active",
				Help:      "Number of active discovery workers",
			},
		),

		// Storage Metrics
		StorageOperationsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "storage_operations_total",
				Help:      "Total number of storage operations",
			},
			[]string{"operation", "entity", "status"},
		),
		StorageOperationDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "storage_operation_duration_seconds",
				Help:      "Storage operation duration in seconds",
				Buckets:   []float64{.0001, .0005, .001, .005, .01, .05, .1, .5},
			},
			[]string{"operation", "entity"},
		),
		StorageAppsTotal: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "storage_apps_total",
				Help:      "Total number of apps in storage",
			},
		),
		StorageRoutesTotal: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "storage_routes_total",
				Help:      "Total number of routes in storage",
			},
		),

		// SSE/Events Metrics
		SSEConnectionsActive: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "sse_connections_active",
				Help:      "Current number of active WebSocket connections (metric name kept for compatibility)",
			},
		),
		SSEEventsPublished: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "sse_events_published_total",
				Help:      "Total number of WebSocket events published (metric name kept for compatibility)",
			},
			[]string{"event_type"},
		),
		SSEConnectionDuration: promauto.NewHistogram(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "sse_connection_duration_seconds",
				Help:      "WebSocket connection duration in seconds (metric name kept for compatibility)",
				Buckets:   []float64{1, 10, 30, 60, 300, 600, 1800, 3600},
			},
		),

		// System Metrics
		BuildInfo: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "build_info",
				Help:      "Build information",
			},
			[]string{"version", "go_version"},
		),
		StartTime: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "start_time_seconds",
				Help:      "Unix timestamp of when the server started",
			},
		),
		UptimeSeconds: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "uptime_seconds",
				Help:      "Number of seconds the server has been running",
			},
		),

		// Health Check Metrics
		HealthCheckStatus: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "health_check_status",
				Help:      "Health check status (0=unknown, 1=healthy, 2=degraded, 3=unhealthy)",
			},
			[]string{"component"},
		),
		HealthCheckDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "health_check_duration_seconds",
				Help:      "Health check duration in seconds",
				Buckets:   []float64{.001, .005, .01, .05, .1, .5, 1, 2},
			},
			[]string{"component"},
		),
		HealthChecksTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "health_checks_total",
				Help:      "Total number of health checks performed",
			},
			[]string{"component", "status"},
		),

		// Service Health Metrics
		ServiceHealthStatus: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "service_health_status",
				Help:      "Service health status (0=unknown, 1=healthy, 2=unhealthy)",
			},
			[]string{"app_id"},
		),
		ServiceHealthChecksTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "service_health_checks_total",
				Help:      "Total number of service health checks performed",
			},
			[]string{"app_id", "status"},
		),
		ServiceHealthCheckDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "service_health_check_duration_seconds",
				Help:      "Service health check duration in seconds",
				Buckets:   []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1, 2},
			},
			[]string{"app_id"},
		),

		// Config Reload Metrics
		ConfigReloadsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "config_reloads_total",
				Help:      "Total number of configuration reload attempts",
			},
			[]string{"status"}, // success, error_load, error_validation, error_handler
		),
		ConfigReloadDuration: promauto.NewHistogram(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "config_reload_duration_seconds",
				Help:      "Configuration reload duration in seconds",
				Buckets:   []float64{.001, .005, .01, .05, .1, .5, 1, 2, 5},
			},
		),
		ConfigLastReloadTime: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "config_last_reload_time_seconds",
				Help:      "Unix timestamp of last successful config reload",
			},
		),
		ConfigLastReloadStatus: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "config_last_reload_status",
				Help:      "Status of last config reload (0=unknown, 1=success, 2=error)",
			},
		),

		// Certificate Metrics
		CertificateExpirySeconds: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "certificate_expiry_seconds",
				Help:      "Seconds until certificate expiry",
			},
			[]string{"domain", "issuer"},
		),
		CertificateGenerateTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "certificate_generate_total",
				Help:      "Total number of certificate generation attempts",
			},
			[]string{"provider", "status"}, // status: success, error
		),
		CertificateGenerateErrors: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "certificate_generate_errors_total",
				Help:      "Total number of certificate generation errors",
			},
			[]string{"provider", "error_type"},
		),
		CertificateGenerateDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "certificate_generate_duration_seconds",
				Help:      "Certificate generation duration in seconds",
				Buckets:   []float64{.01, .05, .1, .25, .5, 1, 2.5, 5, 10},
			},
			[]string{"provider"},
		),
		CertificateRenewTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "certificate_renew_total",
				Help:      "Total number of certificate renewal attempts",
			},
			[]string{"provider", "status"}, // status: success, error
		),
		CertificateServeTotal: promauto.NewCounter(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "certificate_serve_total",
				Help:      "Total number of certificates served via SNI",
			},
		),
		CertificateMissTotal: promauto.NewCounter(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "certificate_miss_total",
				Help:      "Total number of certificate misses (no cert for domain)",
			},
		),
		CertificatesTotal: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "certificates_total",
				Help:      "Total number of certificates currently managed",
			},
		),

		// Notification Queue Metrics
		NotificationQueueDepth: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "notification_queue_depth",
				Help:      "Current number of notifications in the queue",
			},
		),
		NotificationsEnqueuedTotal: promauto.NewCounter(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "notifications_enqueued_total",
				Help:      "Total number of notifications enqueued",
			},
		),
		NotificationsDeliveredTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "notifications_delivered_total",
				Help:      "Total number of notifications successfully delivered",
			},
			[]string{"type"}, // notification type
		),
		NotificationsFailedTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "notifications_failed_total",
				Help:      "Total number of notifications that failed delivery",
			},
			[]string{"type", "reason"}, // type and failure reason
		),
		NotificationsExpiredTotal: promauto.NewCounter(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "notifications_expired_total",
				Help:      "Total number of notifications that expired before delivery",
			},
		),
		NotificationsRetriedTotal: promauto.NewCounter(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "notifications_retried_total",
				Help:      "Total number of notification retry attempts",
			},
		),
		NotificationStorageFallsTotal: promauto.NewCounter(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "notification_storage_falls_total",
				Help:      "Total number of times notifications fell back to storage due to queue overflow",
			},
		),
		NotificationDeliveryDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "notification_delivery_duration_seconds",
				Help:      "Notification delivery duration in seconds",
				Buckets:   []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1},
			},
			[]string{"type", "status"}, // type and delivery status
		),
		NotificationQueueWorkers: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "notification_queue_workers",
				Help:      "Number of active notification queue workers",
			},
		),

		// Federation Metrics
		FederationPeersActive: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "federation_peers_active",
				Help:      "Number of active federation peers",
			},
		),
		FederationServicesTotal: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "federation_services_total",
				Help:      "Total number of federated services",
			},
			[]string{"origin"}, // "local" or "remote"
		),
		FederationSyncTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "federation_sync_total",
				Help:      "Total number of federation sync operations",
			},
			[]string{"type"}, // "full", "delta", "anti_entropy"
		),
		FederationSyncErrors: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "federation_sync_errors_total",
				Help:      "Total number of federation sync errors",
			},
			[]string{"type", "reason"}, // sync type and error reason
		),
		FederationSyncDuration: promauto.NewHistogram(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "federation_sync_duration_seconds",
				Help:      "Federation sync duration in seconds",
				Buckets:   []float64{.01, .05, .1, .25, .5, 1, 2.5, 5, 10},
			},
		),
		FederationMessagesSent: promauto.NewCounter(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "federation_messages_sent_total",
				Help:      "Total number of federation messages sent",
			},
		),
		FederationMessagesReceived: promauto.NewCounter(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "federation_messages_received_total",
				Help:      "Total number of federation messages received",
			},
		),
	}

	// Set initial values
	m.StartTime.Set(float64(time.Now().Unix()))
	m.BuildInfo.WithLabelValues("unknown", runtime.Version()).Set(1)
	m.ConfigLastReloadStatus.Set(0) // Unknown initially

	return m
}

// RecordHTTPRequest records HTTP request metrics
func (m *Metrics) RecordHTTPRequest(method, path, status string, duration time.Duration, requestSize, responseSize int64) {
	m.HTTPRequestsTotal.WithLabelValues(method, path, status).Inc()
	m.HTTPRequestDuration.WithLabelValues(method, path).Observe(duration.Seconds())
	if requestSize > 0 {
		m.HTTPRequestSizeBytes.WithLabelValues(method, path).Observe(float64(requestSize))
	}
	if responseSize > 0 {
		m.HTTPResponseSizeBytes.WithLabelValues(method, path).Observe(float64(responseSize))
	}
}

// RecordProxyRequest records proxy request metrics
func (m *Metrics) RecordProxyRequest(appID, status string, duration time.Duration) {
	m.ProxyRequestsTotal.WithLabelValues(appID, status).Inc()
	m.ProxyRequestDuration.WithLabelValues(appID).Observe(duration.Seconds())
}

// RecordProxyError records proxy error metrics
func (m *Metrics) RecordProxyError(appID, errorType string) {
	m.ProxyUpstreamErrors.WithLabelValues(appID, errorType).Inc()
}

// RecordProxyBytes records bytes transferred through proxy
func (m *Metrics) RecordProxyBytes(appID, direction string, bytes int64) {
	m.ProxyBytesTransferred.WithLabelValues(appID, direction).Add(float64(bytes))
}

// RecordAuthPairing records device pairing metrics
func (m *Metrics) RecordAuthPairing(status, platform string, duration time.Duration) {
	m.AuthPairingTotal.WithLabelValues(status, platform).Inc()
	m.AuthPairingDuration.Observe(duration.Seconds())
}

// RecordAuthRefresh records token refresh metrics
func (m *Metrics) RecordAuthRefresh(status string) {
	m.AuthRefreshTotal.WithLabelValues(status).Inc()
}

// RecordJWTValidation records JWT validation metrics
func (m *Metrics) RecordJWTValidation(status string) {
	m.AuthJWTValidationsTotal.WithLabelValues(status).Inc()
}

// RecordLocalAuth records local authentication bypass metrics
func (m *Metrics) RecordLocalAuth(status string) {
	m.AuthLocalAuthTotal.WithLabelValues(status).Inc()
}

// RecordDeviceOperation records device management operation metrics
func (m *Metrics) RecordDeviceOperation(operation, status string) {
	m.DeviceOperationsTotal.WithLabelValues(operation, status).Inc()
}

// UpdateDeviceLastSeen updates device last seen age metric
func (m *Metrics) UpdateDeviceLastSeen(deviceID string, lastSeen time.Time) {
	age := time.Since(lastSeen).Seconds()
	m.DeviceLastSeenAge.WithLabelValues(deviceID).Set(age)
}

// RecordDiscoveryProposal records discovery proposal metrics
func (m *Metrics) RecordDiscoveryProposal(source, status string) {
	m.DiscoveryProposalsTotal.WithLabelValues(source, status).Inc()
}

// RecordDiscoveryScan records discovery scan metrics
func (m *Metrics) RecordDiscoveryScan(source, status string, duration time.Duration) {
	m.DiscoveryScansTotal.WithLabelValues(source, status).Inc()
	m.DiscoveryScanDuration.WithLabelValues(source).Observe(duration.Seconds())
}

// RecordStorageOperation records storage operation metrics
func (m *Metrics) RecordStorageOperation(operation, entity, status string, duration time.Duration) {
	m.StorageOperationsTotal.WithLabelValues(operation, entity, status).Inc()
	m.StorageOperationDuration.WithLabelValues(operation, entity).Observe(duration.Seconds())
}

// RecordSSEEvent records SSE event publication
func (m *Metrics) RecordSSEEvent(eventType string) {
	m.SSEEventsPublished.WithLabelValues(eventType).Inc()
}

// UpdateUptime updates the uptime metric
func (m *Metrics) UpdateUptime(startTime time.Time) {
	m.UptimeSeconds.Set(time.Since(startTime).Seconds())
}

// SetBuildInfo sets build information
func (m *Metrics) SetBuildInfo(version, goVersion string) {
	m.BuildInfo.Reset()
	m.BuildInfo.WithLabelValues(version, goVersion).Set(1)
}

// RecordHealthCheck records health check metrics
func (m *Metrics) RecordHealthCheck(component, status string, duration time.Duration) {
	// Map status to numeric value for gauge
	// 0=unknown, 1=healthy, 2=degraded, 3=unhealthy
	var statusValue float64
	switch status {
	case "healthy":
		statusValue = 1
	case "degraded":
		statusValue = 2
	case "unhealthy":
		statusValue = 3
	default:
		statusValue = 0
	}

	m.HealthCheckStatus.WithLabelValues(component).Set(statusValue)
	m.HealthCheckDuration.WithLabelValues(component).Observe(duration.Seconds())
	m.HealthChecksTotal.WithLabelValues(component, status).Inc()
}

// RecordServiceHealthCheck records service health check metrics
func (m *Metrics) RecordServiceHealthCheck(appID, status string, duration time.Duration) {
	m.ServiceHealthChecksTotal.WithLabelValues(appID, status).Inc()
	m.ServiceHealthCheckDuration.WithLabelValues(appID).Observe(duration.Seconds())
}

// SetServiceHealthStatus sets the service health status gauge
func (m *Metrics) SetServiceHealthStatus(appID string, status float64) {
	m.ServiceHealthStatus.WithLabelValues(appID).Set(status)
}

// RecordConfigReload records config reload metrics
func (m *Metrics) RecordConfigReload(status string, duration time.Duration) {
	m.ConfigReloadDuration.Observe(duration.Seconds())

	// Update status gauge (1=success, 2=error)
	if status == "success" {
		m.ConfigLastReloadStatus.Set(1)
		m.ConfigLastReloadTime.Set(float64(time.Now().Unix()))
	} else {
		m.ConfigLastReloadStatus.Set(2)
	}
}

// IncrementConfigReloadTotal increments config reload counter
func (m *Metrics) IncrementConfigReloadTotal(status string) {
	m.ConfigReloadsTotal.WithLabelValues(status).Inc()
}

// RecordCertificateGeneration records certificate generation metrics
func (m *Metrics) RecordCertificateGeneration(provider, status string, duration time.Duration) {
	m.CertificateGenerateTotal.WithLabelValues(provider, status).Inc()
	m.CertificateGenerateDuration.WithLabelValues(provider).Observe(duration.Seconds())
}

// RecordCertificateGenerationError records certificate generation errors
func (m *Metrics) RecordCertificateGenerationError(provider, errorType string) {
	m.CertificateGenerateErrors.WithLabelValues(provider, errorType).Inc()
	m.CertificateGenerateTotal.WithLabelValues(provider, "error").Inc()
}

// RecordCertificateRenewal records certificate renewal attempts
func (m *Metrics) RecordCertificateRenewal(provider, status string) {
	m.CertificateRenewTotal.WithLabelValues(provider, status).Inc()
}

// SetCertificateExpiry sets the expiry time for a certificate
func (m *Metrics) SetCertificateExpiry(domain, issuer string, secondsUntilExpiry float64) {
	m.CertificateExpirySeconds.WithLabelValues(domain, issuer).Set(secondsUntilExpiry)
}

// IncrementCertificateServe increments the certificate serve counter
func (m *Metrics) IncrementCertificateServe() {
	m.CertificateServeTotal.Inc()
}

// IncrementCertificateMiss increments the certificate miss counter
func (m *Metrics) IncrementCertificateMiss() {
	m.CertificateMissTotal.Inc()
}

// SetCertificatesTotal sets the total number of certificates
func (m *Metrics) SetCertificatesTotal(count int) {
	m.CertificatesTotal.Set(float64(count))
}

// SetDevicesOnline sets the number of devices currently online
func (m *Metrics) SetDevicesOnline(count float64) {
	m.DevicesOnline.Set(count)
}

// SetDevicesOffline sets the number of devices currently offline
func (m *Metrics) SetDevicesOffline(count float64) {
	m.DevicesOffline.Set(count)
}

// GetHTTPRequestsTotal returns the total count of all HTTP requests across all labels
// This sums all counters in the HTTPRequestsTotal metric
func (m *Metrics) GetHTTPRequestsTotal() (float64, error) {
	// Use Prometheus DTO to collect metric values
	metricChan := make(chan prometheus.Metric, 100)
	go func() {
		m.HTTPRequestsTotal.Collect(metricChan)
		close(metricChan)
	}()

	var total float64
	for metric := range metricChan {
		var dtoMetric dto.Metric
		if err := metric.Write(&dtoMetric); err != nil {
			return 0, err
		}
		if dtoMetric.Counter != nil && dtoMetric.Counter.Value != nil {
			total += *dtoMetric.Counter.Value
		}
	}

	return total, nil
}

// GetAverageLatency calculates the average HTTP request latency in milliseconds
// Returns 0 if no requests have been recorded
func (m *Metrics) GetAverageLatency() float64 {
	// Use prometheus Gather to get current metric values
	metricFamilies, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		return 0
	}

	// Find the HTTP request duration histogram
	for _, mf := range metricFamilies {
		if mf.GetName() == "nekzus_http_request_duration_seconds" {
			var totalSum float64
			var totalCount uint64

			// Sum all histogram observations across all label combinations
			for _, metric := range mf.GetMetric() {
				if h := metric.GetHistogram(); h != nil {
					totalSum += h.GetSampleSum()
					totalCount += h.GetSampleCount()
				}
			}

			if totalCount == 0 {
				return 0
			}

			// Return average in milliseconds
			avgSeconds := totalSum / float64(totalCount)
			return avgSeconds * 1000.0
		}
	}

	return 0
}

// GetUptimePercent calculates the overall service health uptime percentage
// Based on health check success rate (service_health_checks_total metric)
// Returns 100.0 if no health checks have been recorded
func (m *Metrics) GetUptimePercent() float64 {
	metricFamilies, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		return 100.0
	}

	var totalChecks float64
	var healthyChecks float64

	// Find the service health checks counter
	for _, mf := range metricFamilies {
		if mf.GetName() == "nekzus_service_health_checks_total" {
			// Sum all health check counters across all apps
			for _, metric := range mf.GetMetric() {
				// Get the status label (healthy vs unhealthy)
				var status string
				for _, label := range metric.GetLabel() {
					if label.GetName() == "status" {
						status = label.GetValue()
						break
					}
				}

				count := metric.GetCounter().GetValue()
				totalChecks += count

				if status == "healthy" {
					healthyChecks += count
				}
			}
		}
	}

	if totalChecks == 0 {
		// No health checks recorded yet, return 100%
		return 100.0
	}

	return (healthyChecks / totalChecks) * 100.0
}
