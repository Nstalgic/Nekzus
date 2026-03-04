package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/nstalgic/nekzus/internal/types"
)

var log = slog.With("package", "config")

// Load reads and validates configuration from a file
func Load(path string) (types.ServerConfig, error) {
	var cfg types.ServerConfig

	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, fmt.Errorf("failed to read config file: %w", err)
	}

	// Parse based on file extension
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".yaml", ".yml":
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return cfg, fmt.Errorf("failed to parse YAML config: %w", err)
		}
	case ".json":
		if err := json.Unmarshal(data, &cfg); err != nil {
			return cfg, fmt.Errorf("failed to parse JSON config: %w", err)
		}
	default:
		// Try JSON as fallback
		if err := json.Unmarshal(data, &cfg); err != nil {
			return cfg, fmt.Errorf("unknown config format: %s", ext)
		}
	}

	// Validate configuration before env overrides
	if err := Validate(cfg); err != nil {
		return cfg, fmt.Errorf("config validation failed: %w", err)
	}

	// Apply environment variable overrides
	ApplyEnvOverrides(&cfg)

	// Re-validate after env overrides
	if err := Validate(cfg); err != nil {
		return cfg, fmt.Errorf("validation failed after env overrides: %w", err)
	}

	return cfg, nil
}

// ApplyEnvOverrides applies environment variable overrides to config
func ApplyEnvOverrides(cfg *types.ServerConfig) {
	if v := os.Getenv("NEKZUS_ADDR"); v != "" {
		cfg.Server.Addr = v
	}
	if v := os.Getenv("NEKZUS_TLS_CERT"); v != "" {
		cfg.Server.TLSCert = v
	}
	if v := os.Getenv("NEKZUS_TLS_KEY"); v != "" {
		cfg.Server.TLSKey = v
	}
	if v := os.Getenv("NEKZUS_JWT_SECRET"); v != "" {
		cfg.Auth.HS256Secret = v
	}
	if v := os.Getenv("NEKZUS_BOOTSTRAP_TOKEN"); v != "" {
		cfg.Bootstrap.Tokens = append(cfg.Bootstrap.Tokens, v)
	}
	if v := os.Getenv("NEKZUS_DATABASE_PATH"); v != "" {
		cfg.Storage.DatabasePath = v
	}
	if v := os.Getenv("NEKZUS_TOOLBOX_HOST_DATA_DIR"); v != "" {
		cfg.Toolbox.HostDataDir = v
	}
	if v := os.Getenv("NEKZUS_SCRIPTS_HOST_DIRECTORY"); v != "" {
		cfg.Scripts.HostDirectory = v
	}
	if v := os.Getenv("NEKZUS_HOST_ROOT_PATH"); v != "" {
		cfg.System.HostRootPath = v
	}

	// Discovery overrides
	if v := os.Getenv("NEKZUS_DISCOVERY_ENABLED"); v == "true" || v == "1" {
		cfg.Discovery.Enabled = true
	}
	if v := os.Getenv("NEKZUS_DISCOVERY_DOCKER_ENABLED"); v == "true" || v == "1" {
		cfg.Discovery.Docker.Enabled = true
	}

	// Toolbox overrides
	if v := os.Getenv("NEKZUS_TOOLBOX_ENABLED"); v == "true" || v == "1" {
		cfg.Toolbox.Enabled = true
	}

	// Scripts overrides
	if v := os.Getenv("NEKZUS_SCRIPTS_ENABLED"); v == "true" || v == "1" {
		cfg.Scripts.Enabled = true
	}

	// Notifications overrides
	if v := os.Getenv("NEKZUS_NOTIFICATIONS_ENABLED"); v == "true" || v == "1" {
		cfg.Notifications.Enabled = true
	}

	// Metrics overrides
	if v := os.Getenv("NEKZUS_METRICS_ENABLED"); v == "true" || v == "1" {
		cfg.Metrics.Enabled = true
	}
}

// Validate checks if the configuration is valid
func Validate(cfg types.ServerConfig) error {
	var errs []error

	// Validate server config
	if cfg.Server.Addr == "" {
		cfg.Server.Addr = ":8443" // Default
	}

	// Validate server address and port
	if err := validateServerAddress(cfg.Server.Addr); err != nil {
		errs = append(errs, fmt.Errorf("server.addr: %w", err))
	}

	// Validate auth config
	// Note: JWT secret is now optional at validation time - it can be auto-generated
	// during application startup if not provided. We only validate if one is given.
	if cfg.Auth.HS256Secret != "" {
		if len(cfg.Auth.HS256Secret) < 32 {
			errs = append(errs, errors.New("auth.hs256_secret must be at least 32 characters"))
		}

		// Check for weak secrets (only in non-dev environments)
		if !isDevEnvironment() {
			weakPatterns := []string{"dev", "test", "change-me", "example"}
			lowerSecret := strings.ToLower(cfg.Auth.HS256Secret)
			for _, pattern := range weakPatterns {
				if strings.Contains(lowerSecret, pattern) {
					errs = append(errs, fmt.Errorf("auth.hs256_secret contains weak pattern '%s' - use a strong secret in production", pattern))
				}
			}
		}
	}

	if cfg.Auth.Issuer == "" {
		cfg.Auth.Issuer = "nekzus"
	}
	if cfg.Auth.Audience == "" {
		cfg.Auth.Audience = "nekzus-mobile"
	}

	// Validate bootstrap tokens
	// Note: Bootstrap tokens are now optional. If not provided, users can still pair
	// devices using the web UI's QR code flow, which generates short-lived tokens.
	if len(cfg.Bootstrap.Tokens) == 0 && os.Getenv("NEKZUS_BOOTSTRAP_TOKEN") == "" {
		log.Info("no bootstrap tokens configured - device pairing will use QR code flow only")
	}

	// Validate TLS config (if TLS is being used)
	if cfg.Server.TLSCert != "" || cfg.Server.TLSKey != "" {
		if cfg.Server.TLSCert == "" {
			errs = append(errs, errors.New("server.tls_cert is required when server.tls_key is set"))
		}
		if cfg.Server.TLSKey == "" {
			errs = append(errs, errors.New("server.tls_key is required when server.tls_cert is set"))
		}

		// Note: We don't check if files exist here because they may be auto-generated on startup
		// The tlsutil.EnsureCertificates function will handle missing files by generating them
	}

	// Validate discovery config
	if cfg.Discovery.Enabled {
		if cfg.Discovery.Docker.Enabled {
			if cfg.Discovery.Docker.PollInterval == "" {
				cfg.Discovery.Docker.PollInterval = "30s"
			}
			// Validate duration
			if err := validateDuration(cfg.Discovery.Docker.PollInterval); err != nil {
				errs = append(errs, fmt.Errorf("discovery.docker.poll_interval: %w", err))
			}
			if cfg.Discovery.Docker.SocketPath == "" {
				cfg.Discovery.Docker.SocketPath = "unix:///var/run/docker.sock"
			}

			// Validate network mode
			if cfg.Discovery.Docker.NetworkMode != "" {
				validModes := []string{"all", "first", "preferred"}
				isValid := false
				for _, mode := range validModes {
					if cfg.Discovery.Docker.NetworkMode == mode {
						isValid = true
						break
					}
				}
				if !isValid {
					errs = append(errs, fmt.Errorf("invalid discovery.docker.network_mode: %s (must be 'all', 'first', or 'preferred')", cfg.Discovery.Docker.NetworkMode))
				}
			}

			// Validate preferred mode has networks list
			if cfg.Discovery.Docker.NetworkMode == "preferred" && len(cfg.Discovery.Docker.Networks) == 0 {
				errs = append(errs, errors.New("discovery.docker.network_mode 'preferred' requires discovery.docker.networks to be specified"))
			}
		}

		if cfg.Discovery.MDNS.Enabled {
			if cfg.Discovery.MDNS.ScanInterval == "" {
				cfg.Discovery.MDNS.ScanInterval = "60s"
			}
			// Validate duration
			if err := validateDuration(cfg.Discovery.MDNS.ScanInterval); err != nil {
				errs = append(errs, fmt.Errorf("discovery.mdns.scan_interval: %w", err))
			}
		}

		if cfg.Discovery.Kubernetes.Enabled {
			if cfg.Discovery.Kubernetes.PollInterval != "" {
				if err := validateDuration(cfg.Discovery.Kubernetes.PollInterval); err != nil {
					errs = append(errs, fmt.Errorf("discovery.kubernetes.poll_interval: %w", err))
				}
			}
		}

	}

	// Validate runtime configuration
	if err := validateRuntimesConfig(&cfg.Runtimes); err != nil {
		errs = append(errs, err)
	}

	// Validate health check configuration
	if cfg.HealthChecks.Enabled {
		if cfg.HealthChecks.Interval != "" {
			if err := validateDuration(cfg.HealthChecks.Interval); err != nil {
				errs = append(errs, fmt.Errorf("health_checks.interval: %w", err))
			}
		}
		if cfg.HealthChecks.Timeout != "" {
			if err := validateDuration(cfg.HealthChecks.Timeout); err != nil {
				errs = append(errs, fmt.Errorf("health_checks.timeout: %w", err))
			}
		}
	}

	// Validate metrics configuration
	if cfg.Metrics.Path != "" {
		if !strings.HasPrefix(cfg.Metrics.Path, "/") {
			errs = append(errs, errors.New("metrics.path must start with /"))
		}
		// Prevent conflicts with existing paths
		if cfg.Metrics.Path == "/api/v1/healthz" || strings.HasPrefix(cfg.Metrics.Path, "/api/") {
			errs = append(errs, errors.New("metrics.path cannot conflict with API paths"))
		}
	}

	// Validate routes
	for i, route := range cfg.Routes {
		if route.RouteID == "" {
			errs = append(errs, fmt.Errorf("routes[%d].id is required", i))
		}
		if route.PathBase == "" {
			errs = append(errs, fmt.Errorf("routes[%d].path_base is required", i))
		}
		if route.To == "" {
			errs = append(errs, fmt.Errorf("routes[%d].to is required", i))
		}
		if !strings.HasPrefix(route.PathBase, "/") {
			errs = append(errs, fmt.Errorf("routes[%d].path_base must start with /", i))
		}
	}

	// Validate apps
	for i, app := range cfg.Apps {
		if app.ID == "" {
			errs = append(errs, fmt.Errorf("apps[%d].id is required", i))
		}
		if app.Name == "" {
			errs = append(errs, fmt.Errorf("apps[%d].name is required", i))
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	return nil
}

// isDevEnvironment checks if running in development mode
func isDevEnvironment() bool {
	env := strings.ToLower(os.Getenv("ENVIRONMENT"))
	return env == "development" || env == "dev" || env == "test" || env == ""
}

// validateDuration validates that a string is a valid duration
func validateDuration(duration string) error {
	if duration == "" {
		return nil // Empty is okay, defaults will be applied
	}
	_, err := time.ParseDuration(duration)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", duration, err)
	}
	return nil
}

// validateRuntimesConfig validates the runtime configuration and applies defaults
func validateRuntimesConfig(cfg *types.RuntimesConfig) error {
	// Apply defaults if not set
	if cfg.Primary == "" {
		// Default to Docker if Docker is enabled or no runtime is specified
		if cfg.Docker.Enabled || (!cfg.Docker.Enabled && !cfg.Kubernetes.Enabled) {
			cfg.Primary = "docker"
		} else if cfg.Kubernetes.Enabled {
			cfg.Primary = "kubernetes"
		}
	}

	// Validate primary runtime
	validRuntimes := []string{"docker", "kubernetes"}
	isValid := false
	for _, rt := range validRuntimes {
		if cfg.Primary == rt {
			isValid = true
			break
		}
	}
	if cfg.Primary != "" && !isValid {
		return fmt.Errorf("runtimes.primary: invalid value %q (must be 'docker' or 'kubernetes')", cfg.Primary)
	}

	// Validate Docker runtime config
	if cfg.Docker.Enabled && cfg.Docker.SocketPath == "" {
		cfg.Docker.SocketPath = "unix:///var/run/docker.sock"
	}

	// Validate Kubernetes runtime config
	if cfg.Kubernetes.Enabled {
		if cfg.Kubernetes.MetricsCacheTTL != "" {
			if err := validateDuration(cfg.Kubernetes.MetricsCacheTTL); err != nil {
				return fmt.Errorf("runtimes.kubernetes.metrics_cache_ttl: %w", err)
			}
		} else {
			cfg.Kubernetes.MetricsCacheTTL = "30s" // Default cache TTL
		}
	}

	// Warn if primary runtime is not enabled
	if cfg.Primary == "docker" && !cfg.Docker.Enabled {
		log.Warn("runtimes.primary is 'docker' but runtimes.docker.enabled is false")
	}
	if cfg.Primary == "kubernetes" && !cfg.Kubernetes.Enabled {
		log.Warn("runtimes.primary is 'kubernetes' but runtimes.kubernetes.enabled is false")
	}

	return nil
}

// validateServerAddress validates a server address (host:port or :port)
func validateServerAddress(addr string) error {
	if addr == "" {
		return nil // Will use default
	}

	// Split host and port
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("invalid address format: %w", err)
	}

	// Validate port
	if portStr != "" {
		port, err := strconv.Atoi(portStr)
		if err != nil {
			return fmt.Errorf("invalid port %q: must be a number", portStr)
		}
		if port < 0 || port > 65535 {
			return fmt.Errorf("invalid port %d: must be between 0 and 65535", port)
		}
	}

	// Validate host if specified
	if host != "" {
		// Host can be an IP, hostname, or empty (bind to all)
		// Just check it's not obviously malformed
		if strings.Contains(host, " ") {
			return fmt.Errorf("invalid host %q: contains spaces", host)
		}
	}

	return nil
}

// SetDefaults sets default values for missing configuration
func SetDefaults(cfg *types.ServerConfig) {
	if cfg.Server.Addr == "" {
		cfg.Server.Addr = ":8443"
	}
	if cfg.Auth.Issuer == "" {
		cfg.Auth.Issuer = "nekzus"
	}
	if cfg.Auth.Audience == "" {
		cfg.Auth.Audience = "nekzus-mobile"
	}
	if cfg.Storage.DatabasePath == "" {
		cfg.Storage.DatabasePath = "./data/nexus.db"
	}
	if cfg.Discovery.Docker.PollInterval == "" {
		cfg.Discovery.Docker.PollInterval = "30s"
	}
	if cfg.Discovery.Docker.SocketPath == "" {
		cfg.Discovery.Docker.SocketPath = "unix:///var/run/docker.sock"
	}
	if cfg.Discovery.Docker.NetworkMode == "" {
		cfg.Discovery.Docker.NetworkMode = "all"
	}
	if cfg.Discovery.MDNS.ScanInterval == "" {
		cfg.Discovery.MDNS.ScanInterval = "60s"
	}

	// Set health check defaults
	// Health checks are always enabled with sensible defaults
	cfg.HealthChecks.Enabled = true
	if cfg.HealthChecks.Interval == "" {
		cfg.HealthChecks.Interval = "10s" // Check every 10 seconds
	}
	if cfg.HealthChecks.Timeout == "" {
		cfg.HealthChecks.Timeout = "5s" // 5 second request timeout
	}
	if cfg.HealthChecks.UnhealthyThreshold == 0 {
		cfg.HealthChecks.UnhealthyThreshold = 2 // 2 consecutive failures = 20s until unhealthy
	}
	if cfg.HealthChecks.Path == "" {
		cfg.HealthChecks.Path = "/" // Default to root path
	}

	// Set metrics defaults
	// Metrics endpoint is enabled by default for backward compatibility
	pathWasEmpty := cfg.Metrics.Path == ""
	if pathWasEmpty {
		cfg.Metrics.Path = "/metrics"
		// If metrics config wasn't provided, enable by default for backward compatibility
		cfg.Metrics.Enabled = true
	}
	// If path was already configured, respect the user's Enabled setting

	// Toolbox configuration defaults and deprecation handling
	if cfg.Toolbox.Enabled {
		// Check for deprecated catalog_path usage
		if cfg.Toolbox.CatalogDir == "" && cfg.Toolbox.CatalogPath != "" {
			log.Warn("toolbox.catalog_path is DEPRECATED and will be removed in a future release")
			log.Warn("Please migrate to toolbox.catalog_dir (Compose-based catalog)")
			log.Warn("See documentation for migration guide", "current_catalog_path", cfg.Toolbox.CatalogPath)
		}

		// Set default catalog directory if not specified
		if cfg.Toolbox.CatalogDir == "" && cfg.Toolbox.CatalogPath == "" {
			cfg.Toolbox.CatalogDir = "./toolbox"
		}

		// Set default data directory
		if cfg.Toolbox.DataDir == "" {
			cfg.Toolbox.DataDir = "./data/toolbox"
		}
	}
}
