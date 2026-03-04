// Package constants provides application-wide constant values for configuration,
// rate limiting, timeouts, and other magic numbers to improve code maintainability
// and reduce hardcoded values throughout the codebase.
package constants

// Rate Limiting Configuration
// These constants define rate limits for various API endpoints to prevent abuse
// and ensure fair resource allocation.
const (
	// QR Code generation is computationally expensive - strict limits prevent abuse
	QRLimitRate  = 1.0 // 1 request per second
	QRLimitBurst = 5   // Allow burst of 5 requests

	// Auth endpoints need protection but should allow reasonable retries
	AuthLimitRate  = 0.167 // 10 requests per minute (1/60*10 ≈ 0.167)
	AuthLimitBurst = 10    // Allow burst of 10 requests

	// Device management operations are frequent but not critical
	DeviceLimitRate  = 0.5 // 30 requests per minute (30/60 = 0.5)
	DeviceLimitBurst = 30  // Allow burst of 30 requests

	// Container operations can be more frequent as they're read-heavy
	ContainerLimitRate  = 1.0 // 60 requests per minute
	ContainerLimitBurst = 60  // Allow burst of 60 requests

	// Health checks are frequent monitoring operations
	HealthLimitRate  = 0.5 // 30 requests per minute
	HealthLimitBurst = 30  // Allow burst of 30 requests

	// Metrics endpoint is polled regularly by monitoring systems
	MetricsLimitRate  = 0.5 // 30 requests per minute
	MetricsLimitBurst = 30  // Allow burst of 30 requests

	// WebSocket connections need careful rate limiting during handshake
	WebSocketLimitRate  = 0.167 // 10 connections per minute
	WebSocketLimitBurst = 10    // Allow burst of 10 connections
)

// Health Check Configuration
const (
	// Memory threshold for health checks (in MB)
	// System is considered unhealthy if available memory drops below this value
	HealthCheckMemoryThresholdMB = 512

	// Disk threshold for health checks (as percentage)
	// System is considered unhealthy if disk usage exceeds this percentage
	HealthCheckDiskThresholdPercent = 90
)

// Request Body Size Limits
// These limits prevent excessive memory consumption from large request payloads
const (
	// Auth request body limit (1 MB)
	// Pairing requests include device info and shouldn't exceed this size
	MaxAuthRequestBodySize = 1 * 1024 * 1024

	// Toolbox deployment request body limit (2 MB)
	// Allows for larger compose files and configuration data
	MaxToolboxDeployBodySize = 2 * 1024 * 1024

	// Device management request body limit (512 KB)
	// Device updates are typically small
	MaxDeviceRequestBodySize = 512 * 1024

	// System configuration request body limit (1 MB)
	// System settings and config updates
	MaxSystemRequestBodySize = 1 * 1024 * 1024

	// Certificate import request body limit (5 MB)
	// Certificates and keys can be large, especially with full chains
	MaxCertificateRequestBodySize = 5 * 1024 * 1024
)

// Timeout Configuration (in seconds)
// These timeouts prevent operations from hanging indefinitely
const (
	// Toolbox deployment timeout (5 minutes)
	// Docker operations can be slow, especially on first pull
	DeploymentTimeoutSeconds = 300

	// Health check timeout (10 seconds)
	// Quick response time to detect unhealthy services
	HealthCheckTimeoutSeconds = 10

	// Graceful shutdown timeout (30 seconds)
	// Time allowed for in-flight requests to complete during shutdown
	GracefulShutdownTimeoutSeconds = 30

	// Discovery worker timeout (30 seconds)
	// Time allowed for service discovery operations
	DiscoveryTimeoutSeconds = 30

	// Database query timeout (5 seconds)
	// Prevent long-running queries from blocking
	DatabaseQueryTimeoutSeconds = 5
)

// WebSocket Configuration
const (
	// WebSocket buffer size (32 KB)
	// Optimal size for most message payloads
	WebSocketBufferSize = 32 * 1024

	// WebSocket ping interval (30 seconds)
	// Keep connections alive and detect disconnects
	WebSocketPingIntervalSeconds = 30

	// WebSocket write deadline (10 seconds)
	// Timeout for writing messages to slow clients
	WebSocketWriteDeadlineSeconds = 10
)

// Proxy Configuration
const (
	// Proxy cache size (maximum number of cached reverse proxies)
	ProxyCacheSize = 100

	// Proxy dial timeout (10 seconds)
	ProxyDialTimeoutSeconds = 10

	// Proxy idle connection timeout (90 seconds)
	ProxyIdleConnTimeoutSeconds = 90

	// Proxy TLS handshake timeout (10 seconds)
	ProxyTLSHandshakeTimeoutSeconds = 10
)

// Discovery Configuration
const (
	// Debouncer maximum size (maximum unique service keys to track)
	DebouncerMaxSize = 10000

	// Discovery interval (30 seconds)
	// How often to run discovery workers
	DiscoveryIntervalSeconds = 30

	// Service proposal TTL (5 minutes)
	// How long to wait before removing stale services
	ServiceProposalTTLSeconds = 300
)

// Backup Configuration
const (
	// Maximum backup retention count
	// Number of backup files to keep before pruning old ones
	MaxBackupRetentionCount = 10

	// Backup compression level (0-9, where 9 is maximum compression)
	BackupCompressionLevel = 6
)
