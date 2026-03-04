package types

import (
	"encoding/json"
	"strconv"
	"strings"
	"sync"
	"time"

	composetypes "github.com/compose-spec/compose-go/v2/types"
)

// ServerConfig holds the complete application configuration.
type ServerConfig struct {
	Server struct {
		Addr             string `yaml:"addr" json:"addr"`
		HTTPRedirectAddr string `yaml:"http_redirect_addr" json:"http_redirect_addr"`
		BaseURL          string `yaml:"base_url" json:"base_url"`
		TLSCert          string `yaml:"tls_cert" json:"tls_cert"`
		TLSKey           string `yaml:"tls_key" json:"tls_key"`
	} `yaml:"server" json:"server"`
	Auth struct {
		Issuer        string   `yaml:"issuer" json:"issuer"`
		Audience      string   `yaml:"audience" json:"audience"`
		HS256Secret   string   `yaml:"hs256_secret" json:"hs256_secret"`
		DefaultScopes []string `yaml:"default_scopes,omitempty" json:"default_scopes,omitempty"` // Scopes applied to routes without explicit scopes
	} `yaml:"auth" json:"auth"`
	Bootstrap struct {
		Tokens []string `yaml:"tokens" json:"tokens"`
	} `yaml:"bootstrap" json:"bootstrap"`
	Storage struct {
		DatabasePath string `yaml:"database_path" json:"database_path"`
	} `yaml:"storage" json:"storage"`
	Notifications NotificationsConfig `yaml:"notifications" json:"notifications"`
	Discovery     struct {
		Enabled    bool             `yaml:"enabled" json:"enabled"`
		Docker     DockerConfig     `yaml:"docker" json:"docker"`
		MDNS       MDNSConfig       `yaml:"mdns" json:"mdns"`
		Kubernetes KubernetesConfig `yaml:"kubernetes" json:"kubernetes"`
	} `yaml:"discovery" json:"discovery"`
	HealthChecks HealthChecksConfig `yaml:"health_checks" json:"health_checks"`
	Metrics      MetricsConfig      `yaml:"metrics" json:"metrics"`
	Backup       BackupConfig       `yaml:"backup" json:"backup"`
	Toolbox      ToolboxConfig      `yaml:"toolbox" json:"toolbox"`
	Scripts      ScriptsConfig      `yaml:"scripts" json:"scripts"`
	Federation   FederationConfig   `yaml:"federation" json:"federation"`
	Runtimes     RuntimesConfig     `yaml:"runtimes" json:"runtimes"`
	System       SystemConfig       `yaml:"system" json:"system"`
	Routes       []Route            `yaml:"routes" json:"routes"`
	Apps         []App              `yaml:"apps" json:"apps"`
}

// SystemConfig holds system metrics configuration.
type SystemConfig struct {
	// HostRootPath is the path to mounted host root filesystem (e.g., "/mnt/host").
	// When set, system metrics (CPU, RAM, Disk) are read from the host via this mount.
	// When empty (default), metrics reflect the container's own resource usage.
	HostRootPath string `yaml:"host_root_path" json:"host_root_path"`
}

// DockerConfig holds Docker discovery settings.
type DockerConfig struct {
	Enabled         bool     `yaml:"enabled" json:"enabled"`
	SocketPath      string   `yaml:"socket_path" json:"socket_path"`
	PollInterval    string   `yaml:"poll_interval" json:"poll_interval"`       // e.g., "30s"
	Networks        []string `yaml:"networks" json:"networks"`                 // Specific networks to scan
	NetworkMode     string   `yaml:"network_mode" json:"network_mode"`         // "all", "first", "preferred"
	ExcludeNetworks []string `yaml:"exclude_networks" json:"exclude_networks"` // Networks to ignore
}

// MDNSConfig holds mDNS discovery settings.
type MDNSConfig struct {
	Enabled      bool     `yaml:"enabled" json:"enabled"`
	Services     []string `yaml:"services" json:"services"`
	ScanInterval string   `yaml:"scan_interval" json:"scan_interval"` // e.g., "60s"
}

// KubernetesConfig holds Kubernetes discovery settings.
type KubernetesConfig struct {
	Enabled      bool     `yaml:"enabled" json:"enabled"`
	Kubeconfig   string   `yaml:"kubeconfig" json:"kubeconfig"`       // Path to kubeconfig, empty for in-cluster
	Namespaces   []string `yaml:"namespaces" json:"namespaces"`       // Namespaces to watch, empty for all
	PollInterval string   `yaml:"poll_interval" json:"poll_interval"` // e.g., "30s"
}

// RuntimesConfig holds container runtime settings.
type RuntimesConfig struct {
	Primary    string                  `yaml:"primary" json:"primary"`       // Primary runtime: "docker" or "kubernetes"
	Docker     DockerRuntimeConfig     `yaml:"docker" json:"docker"`         // Docker runtime settings
	Kubernetes KubernetesRuntimeConfig `yaml:"kubernetes" json:"kubernetes"` // Kubernetes runtime settings
}

// DockerRuntimeConfig holds Docker runtime settings.
type DockerRuntimeConfig struct {
	Enabled    bool   `yaml:"enabled" json:"enabled"`
	SocketPath string `yaml:"socket_path" json:"socket_path"` // Docker socket path, empty for default
}

// KubernetesRuntimeConfig holds Kubernetes runtime settings.
type KubernetesRuntimeConfig struct {
	Enabled         bool     `yaml:"enabled" json:"enabled"`
	Kubeconfig      string   `yaml:"kubeconfig" json:"kubeconfig"`               // Path to kubeconfig, empty for in-cluster
	Context         string   `yaml:"context" json:"context"`                     // Kubernetes context to use
	Namespaces      []string `yaml:"namespaces" json:"namespaces"`               // Namespaces to manage, empty for all
	MetricsServer   bool     `yaml:"metrics_server" json:"metrics_server"`       // Whether to attempt using metrics server
	MetricsCacheTTL string   `yaml:"metrics_cache_ttl" json:"metrics_cache_ttl"` // Cache TTL for pod metrics (e.g., "30s")
}

// DeviceToken represents an authenticated device session.
type DeviceToken struct {
	Token     string    `json:"token"`
	DeviceID  string    `json:"deviceId"`
	ExpiresAt time.Time `json:"expiresAt"`
	Scopes    []string  `json:"scopes"`
}

// DiscoveryMetadata contains source-specific metadata from service discovery.
// This allows clients to access underlying identifiers (e.g., Docker container ID)
// for operations specific to that discovery type.
type DiscoveryMetadata struct {
	Source      string `json:"source"`                // Discovery source: "docker", "kubernetes", "mdns", "static"
	ContainerID string `json:"containerId,omitempty"` // Docker container ID (short, 12 chars)
	PodName     string `json:"podName,omitempty"`     // Kubernetes pod name
	ServiceName string `json:"serviceName,omitempty"` // mDNS service name
}

// App represents a discoverable application in the catalog.
type App struct {
	ID              string             `yaml:"id" json:"id"`
	Name            string             `yaml:"name" json:"name"`
	Icon            string             `yaml:"icon,omitempty" json:"icon,omitempty"`
	Tags            []string           `yaml:"tags,omitempty" json:"tags,omitempty"`
	Endpoints       map[string]string  `yaml:"endpoints,omitempty" json:"endpoints,omitempty"`
	ProxyPath       string             `yaml:"-" json:"proxyPath,omitempty"`       // Proxy path for this app (e.g., "/apps/api/")
	URL             string             `yaml:"-" json:"url,omitempty"`             // Full URL with protocol (e.g., "https://nexus.local:8443/apps/api/")
	FaviconURL      string             `yaml:"-" json:"faviconUrl,omitempty"`      // Favicon endpoint (e.g., "/api/v1/apps/grafana/favicon")
	HealthStatus    string             `yaml:"-" json:"healthStatus,omitempty"`    // healthy, unhealthy, unknown
	LastHealthCheck *time.Time         `yaml:"-" json:"lastHealthCheck,omitempty"` // Pointer to omit if nil
	DiscoveryMeta   *DiscoveryMetadata `yaml:"-" json:"discoveryMeta,omitempty"`   // Discovery source metadata (runtime-populated)
}

// Route defines a reverse proxy route mapping.
type Route struct {
	RouteID              string   `yaml:"id" json:"routeId"`
	AppID                string   `yaml:"app_id" json:"appId"`
	PathBase             string   `yaml:"path_base" json:"pathBase"`
	To                   string   `yaml:"to" json:"to"`
	ContainerID          string   `yaml:"container_id,omitempty" json:"containerId,omitempty"` // Explicit link to Docker/K8s container
	Scopes               []string `yaml:"scopes,omitempty" json:"scopes,omitempty"`
	PublicAccess         bool     `yaml:"public_access,omitempty" json:"publicAccess,omitempty"` // If true, bypasses default scopes (explicit opt-out)
	Websocket            bool     `yaml:"websockets,omitempty" json:"websockets,omitempty"`
	StripPrefix          bool     `yaml:"strip_prefix,omitempty" json:"stripPrefix,omitempty"`                    // Strip path prefix before proxying
	StripResponseCookies bool     `yaml:"strip_response_cookies,omitempty" json:"stripResponseCookies,omitempty"` // Remove Set-Cookie headers from upstream responses
	RewriteCookiePaths   bool     `yaml:"rewrite_cookie_paths,omitempty" json:"rewriteCookiePaths,omitempty"`     // Rewrite cookie paths when strip_prefix is enabled
	RewriteHTML          bool     `yaml:"rewrite_html,omitempty" json:"rewriteHTML,omitempty"`                    // Rewrite HTML responses to fix absolute asset paths for SPAs
	PersistCookies       bool     `yaml:"persist_cookies,omitempty" json:"persistCookies,omitempty"`              // Persist cookies for mobile webview session continuity

	// Health check configuration (route-level override)
	HealthCheckPath     string `yaml:"health_check_path,omitempty" json:"healthCheckPath,omitempty"`         // Path to health endpoint (e.g., "/health")
	HealthCheckTimeout  string `yaml:"health_check_timeout,omitempty" json:"healthCheckTimeout,omitempty"`   // Timeout duration (e.g., "5s")
	HealthCheckInterval string `yaml:"health_check_interval,omitempty" json:"healthCheckInterval,omitempty"` // Check interval (e.g., "30s")
	ExpectedStatusCodes []int  `yaml:"expected_status_codes,omitempty" json:"expectedStatusCodes,omitempty"` // Valid status codes (default: 200-299)
	SkipHealthCheck     bool   `yaml:"skip_health_check,omitempty" json:"skipHealthCheck,omitempty"`         // Skip health checks (for origin-validating apps)

	Status string `yaml:"-" json:"status,omitempty"` // Runtime status: ACTIVE, UNHEALTHY, UNKNOWN (not persisted)
}

// GetEffectiveScopes returns the scopes that should be enforced for this route.
// Priority: route.Scopes > defaultScopes (if not PublicAccess)
// If PublicAccess is true, returns empty slice (no scope requirement).
// If route has explicit Scopes, returns those.
// Otherwise returns defaultScopes from config.
func (r *Route) GetEffectiveScopes(defaultScopes []string) []string {
	// PublicAccess explicitly disables scope requirements
	if r.PublicAccess {
		return nil
	}

	// Route-specific scopes take precedence
	if len(r.Scopes) > 0 {
		return r.Scopes
	}

	// Fall back to default scopes from config
	return defaultScopes
}

// RouteHealthInfo provides computed health check information for API responses.
type RouteHealthInfo struct {
	ProbeURL          string     `json:"probeUrl"`               // Full URL being probed (e.g., "http://192.168.0.23:9117/health")
	Status            string     `json:"status"`                 // Current health status: healthy, unhealthy, unknown
	LastCheck         *time.Time `json:"lastCheck,omitempty"`    // Time of last health check
	EffectivePath     string     `json:"effectivePath"`          // Path being used (route-level or global fallback)
	EffectiveTimeout  string     `json:"effectiveTimeout"`       // Timeout being used
	EffectiveInterval string     `json:"effectiveInterval"`      // Interval being used
	EffectiveCodes    []int      `json:"effectiveCodes"`         // Status codes considered healthy
	ConfigSource      string     `json:"configSource,omitempty"` // Where config came from: "route", "per-service", "global"
	LastError         string     `json:"lastError,omitempty"`    // Last error message if unhealthy
}

// Proposal represents a discovered service awaiting approval.
// PortOption represents an available port for a discovered service.
type PortOption struct {
	Port   int    `json:"port"`
	Scheme string `json:"scheme"` // http or https
}

type Proposal struct {
	ID             string       `json:"id"`
	Source         string       `json:"source"`
	DetectedScheme string       `json:"detectedScheme"`
	DetectedHost   string       `json:"detectedHost"`
	DetectedPort   int          `json:"detectedPort"`
	AvailablePorts []PortOption `json:"availablePorts,omitempty"` // All valid ports user can choose from
	Confidence     float64      `json:"confidence"`
	SuggestedApp   App          `json:"suggestedApp"`
	SuggestedRoute Route        `json:"suggestedRoute"`
	Tags           []string     `json:"tags,omitempty"`
	LastSeen       string       `json:"lastSeen"`
	SecurityNotes  []string     `json:"securityNotes,omitempty"`
}

// ProposalStore defines the interface for managing discovery proposals.
type ProposalStore interface {
	GetProposalsMutex() *sync.RWMutex
	GetProposals() *[]Proposal
}

// ActivityEvent represents a system activity event for the activity feed.
type ActivityEvent struct {
	ID        string `json:"id"`
	Type      string `json:"type"`              // e.g., "device.paired", "config.reload"
	Icon      string `json:"icon"`              // Icon name (e.g., "Smartphone", "RefreshCw")
	IconClass string `json:"iconClass"`         // CSS class (e.g., "success", "warning")
	Message   string `json:"message"`           // Display message
	Details   string `json:"details,omitempty"` // Optional details
	Timestamp int64  `json:"timestamp"`         // Unix timestamp in milliseconds
}

// HealthChecksConfig holds service health check configuration.
type HealthChecksConfig struct {
	Enabled            bool                          `yaml:"enabled" json:"enabled"`
	Interval           string                        `yaml:"interval" json:"interval"`                       // e.g., "30s"
	Timeout            string                        `yaml:"timeout" json:"timeout"`                         // e.g., "5s"
	UnhealthyThreshold int                           `yaml:"unhealthy_threshold" json:"unhealthy_threshold"` // Consecutive failures before unhealthy
	Path               string                        `yaml:"path" json:"path"`                               // Default health check path
	PerService         map[string]ServiceHealthCheck `yaml:"per_service" json:"per_service"`                 // Per-service overrides
}

// ServiceHealthCheck holds per-service health check configuration.
type ServiceHealthCheck struct {
	Path     string `yaml:"path" json:"path"`         // Health check path (e.g., "/api/health")
	Interval string `yaml:"interval" json:"interval"` // Check interval (e.g., "15s")
	Timeout  string `yaml:"timeout" json:"timeout"`   // Request timeout (e.g., "3s")
}

// MetricsConfig holds Prometheus metrics endpoint configuration.
type MetricsConfig struct {
	Enabled bool   `yaml:"enabled" json:"enabled"` // Enable/disable the metrics HTTP endpoint
	Path    string `yaml:"path" json:"path"`       // Metrics endpoint path (default: /metrics)
}

// BackupConfig holds backup and disaster recovery configuration.
type BackupConfig struct {
	Enabled   bool   `yaml:"enabled" json:"enabled"`     // Enable/disable automatic backups
	Directory string `yaml:"directory" json:"directory"` // Directory to store backups
	Schedule  string `yaml:"schedule" json:"schedule"`   // Backup interval (e.g., "24h", "1h")
	Retention int    `yaml:"retention" json:"retention"` // Number of backups to keep
}

// WebSocketMessage represents a message sent over WebSocket connections.
type WebSocketMessage struct {
	Type           string      `json:"type"`                     // Message type (discovery, config_reload, device_paired, etc.)
	Topic          string      `json:"topic,omitempty"`          // Topic for pub/sub routing
	NotificationID string      `json:"notificationId,omitempty"` // Unique ID for ACK-based delivery
	MessageID      string      `json:"messageId,omitempty"`      // Unique ID for QoS tracking
	QoS            int         `json:"qos,omitempty"`            // Quality of Service: 0=fire-forget, 1=at-least-once, 2=exactly-once
	Data           interface{} `json:"data"`                     // Message payload
	Timestamp      time.Time   `json:"timestamp,omitempty"`      // Message timestamp
	ExpiresAt      time.Time   `json:"expiresAt,omitempty"`      // Message expiry time (TTL)
	Retain         bool        `json:"retain,omitempty"`         // Retain message for new subscribers
}

// WebSocket message type constants
const (
	WSMsgTypeDiscovery         = "discovery"
	WSMsgTypeConfigReload      = "config_reload"
	WSMsgTypeConfigWarning     = "config_warning"
	WSMsgTypeDevicePaired      = "device_paired"
	WSMsgTypeDeviceRevoked     = "device_revoked"
	WSMsgTypeHealthChange      = "health_change"
	WSMsgTypePortExposure      = "port_exposure_warning"
	WSMsgTypeWebhook           = "webhook"
	WSMsgTypeHello             = "hello"
	WSMsgTypePing              = "ping"
	WSMsgTypePong              = "pong"
	WSMsgTypeAuth              = "auth"
	WSMsgTypeAuthSuccess       = "auth_success"
	WSMsgTypeAuthFailed        = "auth_failed"
	WSMsgTypePeerJoined        = "peer_joined"        // Federation: New peer joined cluster
	WSMsgTypePeerLeft          = "peer_left"          // Federation: Peer left cluster
	WSMsgTypePeerOffline       = "peer_offline"       // Federation: Peer marked offline
	WSMsgTypeFederationSync    = "federation_sync"    // Federation: Service catalog sync event
	WSMsgTypeRouteAdded        = "route_added"        // Discovery: Route added via proposal approval
	WSMsgTypeRouteRemoved      = "route_removed"      // Discovery: Route removed
	WSMsgTypeProposalApproved  = "proposal_approved"  // Discovery: Proposal was approved
	WSMsgTypeProposalDismissed = "proposal_dismissed" // Discovery: Proposal was dismissed
	WSMsgTypeRepairRequired    = "repair_required"    // TLS: Device must re-pair due to TLS upgrade

	// Notification ACK message types
	WSMsgTypeNotification    = "notification"     // Server: Notification requiring ACK
	WSMsgTypeNotificationACK = "notification_ack" // Client: Acknowledgment of notification receipt

	// Container log streaming message types
	WSMsgTypeContainerLogsSubscribe   = "container.logs.subscribe"   // Client: Subscribe to log streaming
	WSMsgTypeContainerLogsUnsubscribe = "container.logs.unsubscribe" // Client: Unsubscribe from log streaming
	WSMsgTypeContainerLogs            = "container.logs"             // Server: Log data (stream: stdout/stderr)
	WSMsgTypeContainerLogsStarted     = "container.logs.started"     // Server: Log stream started confirmation
	WSMsgTypeContainerLogsEnded       = "container.logs.ended"       // Server: Log stream ended (reason: stopped/container_stopped/error)
	WSMsgTypeContainerLogsError       = "container.logs.error"       // Server: Log stream error

	// Legacy container log types (deprecated, use new types above)
	WSMsgTypeContainerLogsStart = "container.logs.start" // Deprecated: use WSMsgTypeContainerLogsSubscribe
	WSMsgTypeContainerLogsStop  = "container.logs.stop"  // Deprecated: use WSMsgTypeContainerLogsUnsubscribe
	WSMsgTypeContainerLogsData  = "container.logs.data"  // Deprecated: use WSMsgTypeContainerLogs

	// Script execution message types
	WSMsgTypeExecutionStarted   = "execution_started"   // Script execution began
	WSMsgTypeExecutionCompleted = "execution_completed" // Script completed successfully
	WSMsgTypeExecutionFailed    = "execution_failed"    // Script failed or timed out

	// MQTT-style subscription message types
	WSMsgTypeSubscribe   = "subscribe"   // Client: Subscribe to topics
	WSMsgTypeUnsubscribe = "unsubscribe" // Client: Unsubscribe from topics
	WSMsgTypeSubAck      = "suback"      // Server: Subscription acknowledgment
	WSMsgTypeUnsubAck    = "unsuback"    // Server: Unsubscription acknowledgment

	// MQTT-style QoS message types
	WSMsgTypeAck     = "ack"     // Client: QoS 1 acknowledgment
	WSMsgTypePubRec  = "pubrec"  // Server: QoS 2 step 2 - publish received
	WSMsgTypePubRel  = "pubrel"  // Client: QoS 2 step 3 - publish release
	WSMsgTypePubComp = "pubcomp" // Server: QoS 2 step 4 - publish complete

	// MQTT-style Last Will and Testament
	WSMsgTypeSetLastWill = "set_last_will" // Client: Set last will message
	WSMsgTypeLWTAck      = "lwtack"        // Server: Last will acknowledgment
)

// APIKey represents an API key stored in the database.
type APIKey struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`                 // User-friendly name for the key
	KeyHash    string     `json:"-"`                    // SHA256 hash of the key (never returned)
	Prefix     string     `json:"prefix"`               // First 8 chars for identification
	Scopes     []string   `json:"scopes"`               // Permission scopes
	ExpiresAt  *time.Time `json:"expiresAt,omitempty"`  // Optional expiration time
	LastUsedAt *time.Time `json:"lastUsedAt,omitempty"` // Last time key was used
	CreatedAt  time.Time  `json:"createdAt"`
	CreatedBy  string     `json:"createdBy,omitempty"` // Device/user that created the key
	RevokedAt  *time.Time `json:"revokedAt,omitempty"` // If revoked, when it was revoked
}

// APIKeyRequest represents a request to create a new API key.
type APIKeyRequest struct {
	Name      string     `json:"name"`                // User-friendly name (required)
	Scopes    []string   `json:"scopes"`              // Permission scopes (required)
	ExpiresAt *time.Time `json:"expiresAt,omitempty"` // Optional expiration
}

// APIKeyResponse represents an API key response (includes plaintext only on creation).
type APIKeyResponse struct {
	APIKey
	Key string `json:"key,omitempty"` // Plaintext key, only returned on creation
}

// NotificationsConfig holds WebSocket notification configuration.
type NotificationsConfig struct {
	Enabled          bool                   `yaml:"enabled" json:"enabled"`
	ACKTimeout       string                 `yaml:"ack_timeout" json:"ack_timeout"` // How long to wait for client ACK (e.g., "30s")
	Queue            QueueConfig            `yaml:"queue" json:"queue"`
	OfflineDetection OfflineDetectionConfig `yaml:"offline_detection" json:"offline_detection"`
}

// QueueConfig holds notification queue configuration.
type QueueConfig struct {
	WorkerCount   int    `yaml:"worker_count" json:"worker_count"`     // Number of worker goroutines
	BufferSize    int    `yaml:"buffer_size" json:"buffer_size"`       // Channel buffer size
	RetryInterval string `yaml:"retry_interval" json:"retry_interval"` // Retry interval (e.g., "30s")
	MaxRetries    int    `yaml:"max_retries" json:"max_retries"`       // Maximum retry attempts
}

// OfflineDetectionConfig holds offline device detection configuration.
type OfflineDetectionConfig struct {
	Enabled          bool   `yaml:"enabled" json:"enabled"`
	CheckInterval    string `yaml:"check_interval" json:"check_interval"`       // How often to check for offline devices
	OfflineThreshold string `yaml:"offline_threshold" json:"offline_threshold"` // Consider offline after this duration
}

// ScriptsConfig holds script execution configuration.
type ScriptsConfig struct {
	Enabled        bool   `yaml:"enabled" json:"enabled"`                   // Enable/disable script execution feature
	Directory      string `yaml:"directory" json:"directory"`               // Directory containing user scripts (container path)
	HostDirectory  string `yaml:"host_directory" json:"host_directory"`     // Host path for bind mounts (required for container execution)
	DefaultTimeout int    `yaml:"default_timeout" json:"default_timeout"`   // Default script timeout in seconds (default: 300)
	MaxOutputBytes int    `yaml:"max_output_bytes" json:"max_output_bytes"` // Maximum output size in bytes (default: 10MB)
}

// ToolboxConfig holds toolbox and one-click deployment configuration.
type ToolboxConfig struct {
	Enabled     bool   `yaml:"enabled" json:"enabled"`             // Enable/disable toolbox feature
	CatalogDir  string `yaml:"catalog_dir" json:"catalog_dir"`     // Directory containing Compose files (NEW)
	CatalogPath string `yaml:"catalog_path" json:"catalog_path"`   // DEPRECATED: Path to service catalog YAML file
	DataDir     string `yaml:"data_dir" json:"data_dir"`           // Base directory for service data volumes (container path)
	HostDataDir string `yaml:"host_data_dir" json:"host_data_dir"` // Host path for bind mounts (required for Docker-in-Docker)
	AutoRoute   bool   `yaml:"auto_route" json:"auto_route"`       // Automatically create routes for deployed services
	AutoStart   bool   `yaml:"auto_start" json:"auto_start"`       // Automatically start containers after deployment
}

// FederationConfig holds P2P federation settings for multi-instance deployments.
type FederationConfig struct {
	Enabled           bool       `yaml:"enabled" json:"enabled"`                         // Enable/disable federation
	ClusterSecret     string     `yaml:"cluster_secret" json:"cluster_secret"`           // Shared secret for peer authentication (32+ chars)
	GossipPort        int        `yaml:"gossip_port" json:"gossip_port"`                 // Port for gossip protocol (default: 7946)
	MDNSEnabled       bool       `yaml:"mdns_enabled" json:"mdns_enabled"`               // Enable mDNS peer discovery
	BootstrapPeers    []string   `yaml:"bootstrap_peers" json:"bootstrap_peers"`         // List of peer addresses to join (e.g., "192.168.1.100:7946")
	Sync              SyncConfig `yaml:"sync" json:"sync"`                               // Synchronization settings
	AllowRemoteRoutes bool       `yaml:"allow_remote_routes" json:"allow_remote_routes"` // Allow proxying to federated services (security risk)
}

// SyncConfig holds federation sync settings.
type SyncConfig struct {
	FullSyncInterval  string `yaml:"full_sync_interval" json:"full_sync_interval"`   // Interval for full catalog sync (e.g., "300s")
	AntiEntropyPeriod string `yaml:"anti_entropy_period" json:"anti_entropy_period"` // Interval for anti-entropy repair (e.g., "60s")
}

// Toolbox label constants for Docker Compose metadata
const (
	// Toolbox metadata labels
	ToolboxLabelName        = "nekzus.toolbox.name"
	ToolboxLabelIcon        = "nekzus.toolbox.icon"
	ToolboxLabelCategory    = "nekzus.toolbox.category"
	ToolboxLabelTags        = "nekzus.toolbox.tags"
	ToolboxLabelDescription = "nekzus.toolbox.description"
	ToolboxLabelDocs        = "nekzus.toolbox.documentation"
	ToolboxLabelImageURL    = "nekzus.toolbox.image_url"
	ToolboxLabelRepoURL     = "nekzus.toolbox.repository_url"

	// Discovery labels (for auto-routing)
	DiscoveryLabelEnable      = "nekzus.enable"
	DiscoveryLabelAppID       = "nekzus.app.id"
	DiscoveryLabelAppName     = "nekzus.app.name"
	DiscoveryLabelRoutePath   = "nekzus.route.path"
	DiscoveryLabelStripPrefix = "nekzus.route.strip_prefix"

	// Discovery port filtering labels
	DiscoveryLabelPrimaryPort = "nekzus.primary_port"       // Discover only this specific port
	DiscoveryLabelAllPorts    = "nekzus.discover.all_ports" // Discover all TCP ports (bypass HTTP filter)

	// Health check labels
	DiscoveryLabelSkipHealthCheck = "nekzus.health.skip" // Skip health checks for this service
)

// ServiceTemplate defines a deployable service in the toolbox catalog.
type ServiceTemplate struct {
	ID            string                `json:"id" yaml:"id"`                                   // Unique service identifier
	Name          string                `json:"name" yaml:"name"`                               // Display name
	Description   string                `json:"description" yaml:"description"`                 // Short description
	Icon          string                `json:"icon,omitempty" yaml:"icon"`                     // Icon (emoji or URL)
	Category      string                `json:"category" yaml:"category"`                       // Category (media, monitoring, etc.)
	Tags          []string              `json:"tags,omitempty" yaml:"tags"`                     // Search tags
	ImageURL      string                `json:"image_url,omitempty" yaml:"image_url"`           // Link to Docker Hub or registry page
	RepositoryURL string                `json:"repository_url,omitempty" yaml:"repository_url"` // Link to source code repository
	DockerConfig  DockerContainerConfig `json:"docker_config,omitempty" yaml:"docker_config"`   // DEPRECATED: Docker container configuration
	EnvVars       []EnvironmentVariable `json:"env_vars" yaml:"env_vars"`                       // User-configurable environment variables
	DefaultRoute  *ServiceRouteTemplate `json:"default_route,omitempty" yaml:"default_route"`   // Optional default route configuration
	Resources     *ResourceRequirements `json:"resources,omitempty" yaml:"resources"`           // Resource requirements
	SecurityNotes []string              `json:"security_notes,omitempty" yaml:"security_notes"` // Security warnings
	Documentation string                `json:"documentation,omitempty" yaml:"documentation"`   // External documentation URL

	// Compose-based deployment
	ComposeProject  *composetypes.Project `json:"-" yaml:"-"`                           // Parsed Compose project
	ComposeFilePath string                `json:"compose_file_path,omitempty" yaml:"-"` // Path to docker-compose.yml
}

// MarshalJSON implements custom JSON marshaling for ServiceTemplate.
// It includes computed fields (image, default_port) extracted from ComposeProject.
func (st *ServiceTemplate) MarshalJSON() ([]byte, error) {
	// Create alias type to avoid infinite recursion
	type Alias ServiceTemplate

	// Build the JSON object manually
	obj := map[string]interface{}{
		"id":          st.ID,
		"name":        st.Name,
		"description": st.Description,
		"category":    st.Category,
	}

	// Add optional string fields
	if st.Icon != "" {
		obj["icon"] = st.Icon
	}
	if st.ImageURL != "" {
		obj["image_url"] = st.ImageURL
	}
	if st.RepositoryURL != "" {
		obj["repository_url"] = st.RepositoryURL
	}
	if st.Documentation != "" {
		obj["documentation"] = st.Documentation
	}
	if st.ComposeFilePath != "" {
		obj["compose_file_path"] = st.ComposeFilePath
	}

	// Add optional slice fields
	if len(st.Tags) > 0 {
		obj["tags"] = st.Tags
	}
	if len(st.EnvVars) > 0 {
		obj["env_vars"] = st.EnvVars
	}
	if len(st.SecurityNotes) > 0 {
		obj["security_notes"] = st.SecurityNotes
	}

	// Add optional struct fields
	if st.DefaultRoute != nil {
		obj["default_route"] = st.DefaultRoute
	}
	if st.Resources != nil {
		obj["resources"] = st.Resources
	}

	// Add DockerConfig if present (legacy support)
	if st.DockerConfig.Image != "" {
		obj["docker_config"] = st.DockerConfig
	}

	// Extract computed fields from ComposeProject
	if st.ComposeProject != nil && len(st.ComposeProject.Services) > 0 {
		// Find the primary service (first service with toolbox labels)
		for _, svc := range st.ComposeProject.Services {
			if _, hasLabel := svc.Labels[ToolboxLabelName]; hasLabel || len(st.ComposeProject.Services) == 1 {
				// Extract image
				if svc.Image != "" {
					obj["image"] = svc.Image
				}

				// Extract default host port from env vars (more reliable than compose parsing)
				// Look for a PORT env var and use its default value
				for _, envVar := range st.EnvVars {
					upperName := strings.ToUpper(envVar.Name)
					if (strings.HasSuffix(upperName, "_PORT") || upperName == "PORT") && envVar.Default != "" {
						if port, err := strconv.Atoi(envVar.Default); err == nil && port > 0 {
							obj["default_port"] = port
							break
						}
					}
				}
				break
			}
		}
	}

	return json.Marshal(obj)
}

// DockerContainerConfig defines Docker container configuration for deployment.
type DockerContainerConfig struct {
	Image         string                `json:"image" yaml:"image"`                             // Docker image (e.g., "grafana/grafana:latest")
	Ports         []PortMapping         `json:"ports,omitempty" yaml:"ports"`                   // Port mappings
	Volumes       []VolumeMapping       `json:"volumes,omitempty" yaml:"volumes"`               // Volume mounts
	Environment   map[string]string     `json:"environment,omitempty" yaml:"environment"`       // Default environment variables
	Networks      []string              `json:"networks,omitempty" yaml:"networks"`             // Network names to connect to
	RestartPolicy string                `json:"restart_policy,omitempty" yaml:"restart_policy"` // Restart policy (e.g., "unless-stopped")
	HealthCheck   *ContainerHealthCheck `json:"health_check,omitempty" yaml:"health_check"`     // Container health check
}

// PortMapping defines a container port mapping.
type PortMapping struct {
	Container   int    `json:"container" yaml:"container"`               // Container port
	HostDefault int    `json:"host_default" yaml:"host_default"`         // Default host port
	Protocol    string `json:"protocol,omitempty" yaml:"protocol"`       // tcp or udp (default: tcp)
	Description string `json:"description,omitempty" yaml:"description"` // Port description
}

// VolumeMapping defines a container volume mount.
type VolumeMapping struct {
	Name        string `json:"name" yaml:"name"`                         // Volume name (used in data directory path)
	MountPath   string `json:"mount_path" yaml:"mount_path"`             // Container mount path
	Description string `json:"description,omitempty" yaml:"description"` // Volume description
}

// ContainerHealthCheck defines a container health check configuration.
type ContainerHealthCheck struct {
	Endpoint       string `json:"endpoint,omitempty" yaml:"endpoint"`               // HTTP endpoint to check
	ExpectedStatus int    `json:"expected_status,omitempty" yaml:"expected_status"` // Expected HTTP status code
	Interval       string `json:"interval,omitempty" yaml:"interval"`               // Check interval (e.g., "30s")
	Timeout        string `json:"timeout,omitempty" yaml:"timeout"`                 // Request timeout (e.g., "5s")
	Retries        int    `json:"retries,omitempty" yaml:"retries"`                 // Number of retries before unhealthy
}

// EnvironmentVariable defines a user-configurable environment variable.
type EnvironmentVariable struct {
	Name        string   `json:"name" yaml:"name"`                         // Environment variable name
	Label       string   `json:"label" yaml:"label"`                       // User-friendly label
	Description string   `json:"description,omitempty" yaml:"description"` // Help text
	Required    bool     `json:"required" yaml:"required"`                 // Is this variable required?
	Default     string   `json:"default,omitempty" yaml:"default"`         // Default value
	Type        string   `json:"type" yaml:"type"`                         // text, password, number, boolean, select
	Options     []string `json:"options,omitempty" yaml:"options"`         // Options for select type
	Validation  string   `json:"validation,omitempty" yaml:"validation"`   // Regex pattern for validation
	Placeholder string   `json:"placeholder,omitempty" yaml:"placeholder"` // Placeholder text
}

// ServiceRouteTemplate defines the default route configuration for a deployed service.
type ServiceRouteTemplate struct {
	PathBase    string   `json:"path_base" yaml:"path_base"`       // Route path base (e.g., "/apps/grafana/")
	Scopes      []string `json:"scopes,omitempty" yaml:"scopes"`   // Required scopes for access
	Websocket   bool     `json:"websocket" yaml:"websocket"`       // Enable WebSocket proxying
	StripPrefix bool     `json:"strip_prefix" yaml:"strip_prefix"` // Strip path prefix before proxying
}

// ResourceRequirements defines minimum resource requirements for a service.
type ResourceRequirements struct {
	MinCPU  string `json:"min_cpu,omitempty" yaml:"min_cpu"`   // Minimum CPU (e.g., "2")
	MinRAM  string `json:"min_ram,omitempty" yaml:"min_ram"`   // Minimum RAM (e.g., "2GB")
	MinDisk string `json:"min_disk,omitempty" yaml:"min_disk"` // Minimum disk space (e.g., "10GB")
}

// ToolboxDeployment represents a deployed service from the toolbox.
type ToolboxDeployment struct {
	ID                string            `json:"id"`                       // Unique deployment ID
	ServiceTemplateID string            `json:"service_template_id"`      // Service template ID
	ServiceName       string            `json:"service_name"`             // User-provided service name
	Status            string            `json:"status"`                   // pending, deploying, deployed, failed, stopped
	ContainerID       string            `json:"container_id,omitempty"`   // DEPRECATED: Docker container ID (for single-container deployments)
	ContainerName     string            `json:"container_name,omitempty"` // DEPRECATED: Docker container name
	ProjectName       string            `json:"project_name,omitempty"`   // Compose project name
	NetworkNames      []string          `json:"network_names,omitempty"`  // Created/joined networks
	VolumeNames       []string          `json:"volume_names,omitempty"`   // Created volumes
	EnvVars           map[string]string `json:"env_vars,omitempty"`       // User-provided environment variables
	CustomImage       string            `json:"custom_image,omitempty"`   // Custom Docker image override
	CustomPort        int               `json:"custom_port,omitempty"`    // Custom host port override
	RouteID           string            `json:"route_id,omitempty"`       // Associated route ID (if auto-created)
	ErrorMessage      string            `json:"error_message,omitempty"`  // Error message if deployment failed
	DeployedAt        *time.Time        `json:"deployed_at,omitempty"`    // Deployment timestamp
	DeployedBy        string            `json:"deployed_by,omitempty"`    // Device ID that deployed this service
	CreatedAt         time.Time         `json:"created_at"`               // Creation timestamp
	UpdatedAt         time.Time         `json:"updated_at"`               // Last update timestamp
}

// Deployment status constants
const (
	DeploymentStatusPending   = "pending"
	DeploymentStatusDeploying = "deploying"
	DeploymentStatusDeployed  = "deployed"
	DeploymentStatusFailed    = "failed"
	DeploymentStatusStopped   = "stopped"
)

// DeploymentRequest represents a request to deploy a service from the toolbox.
type DeploymentRequest struct {
	ServiceID   string            `json:"service_id"`             // Service template ID to deploy
	ServiceName string            `json:"service_name"`           // Custom name for the deployment
	EnvVars     map[string]string `json:"env_vars"`               // User-provided environment variables
	AutoStart   bool              `json:"auto_start"`             // Start container after deployment
	CustomImage string            `json:"custom_image,omitempty"` // Override the default Docker image
	CustomPort  int               `json:"custom_port,omitempty"`  // Override the default host port
}
