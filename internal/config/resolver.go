package config

import (
	"fmt"
	"os"

	"github.com/nstalgic/nekzus/internal/types"
)

// Resolver handles configuration value resolution with priority ordering
// Priority: Environment variables > Config file > Auto-detection/defaults
type Resolver struct {
	envPrefix string
}

// NewResolver creates a new configuration resolver
func NewResolver() *Resolver {
	return &Resolver{
		envPrefix: "NEKZUS_",
	}
}

// ResolveBaseURL resolves the base URL with priority ordering:
// 1. NEKZUS_BASE_URL environment variable
// 2. Configuration file (cfg.Server.BaseURL)
// 3. Auto-detection from local network IP
func (r *Resolver) ResolveBaseURL(cfg types.ServerConfig, getLocalIP func() string) string {
	// Priority 1: Environment variable
	if url := os.Getenv(r.envPrefix + "BASE_URL"); url != "" {
		return url
	}

	// Priority 2: Configuration file
	if cfg.Server.BaseURL != "" {
		return cfg.Server.BaseURL
	}

	// Priority 3: Auto-detection
	return r.autoDetectBaseURL(cfg, getLocalIP)
}

// autoDetectBaseURL constructs a base URL from the local network IP and server config
func (r *Resolver) autoDetectBaseURL(cfg types.ServerConfig, getLocalIP func() string) string {
	host := getLocalIP()
	protocol := "http"
	if cfg.Server.TLSCert != "" && cfg.Server.TLSKey != "" {
		protocol = "https"
	}
	return fmt.Sprintf("%s://%s%s", protocol, host, cfg.Server.Addr)
}

// WasAutoDetected returns true if the base URL was auto-detected (not from env or config)
func (r *Resolver) WasAutoDetected(cfg types.ServerConfig) bool {
	if os.Getenv(r.envPrefix+"BASE_URL") != "" {
		return false
	}
	if cfg.Server.BaseURL != "" {
		return false
	}
	return true
}
