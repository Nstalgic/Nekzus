package config

import (
	"os"
	"strings"
	"testing"

	"github.com/nstalgic/nekzus/internal/types"
)

func TestValidate(t *testing.T) {
	tests := []struct {
		name        string
		cfg         types.ServerConfig
		wantErr     bool
		errContains string
	}{
		{
			name: "valid config",
			cfg: types.ServerConfig{
				Server: struct {
					Addr             string `yaml:"addr" json:"addr"`
					HTTPRedirectAddr string `yaml:"http_redirect_addr" json:"http_redirect_addr"`
					BaseURL          string `yaml:"base_url" json:"base_url"`
					TLSCert          string `yaml:"tls_cert" json:"tls_cert"`
					TLSKey           string `yaml:"tls_key" json:"tls_key"`
					RoutingMode      string `yaml:"routing_mode" json:"routing_mode"`
					BaseDomain       string `yaml:"base_domain" json:"base_domain"`
				}{
					Addr: ":8443",
				},
				Auth: struct {
					Issuer        string   `yaml:"issuer" json:"issuer"`
					Audience      string   `yaml:"audience" json:"audience"`
					HS256Secret   string   `yaml:"hs256_secret" json:"hs256_secret"`
					DefaultScopes []string `yaml:"default_scopes,omitempty" json:"default_scopes,omitempty"`
				}{
					HS256Secret: strings.Repeat("a", 32),
					Issuer:      "test",
					Audience:    "test",
				},
			},
			wantErr: false,
		},
		{
			// Empty JWT secret is now allowed at validation time - it will be
			// auto-generated during application startup if not provided
			name: "empty JWT secret (allowed - will be auto-generated)",
			cfg: types.ServerConfig{
				Server: struct {
					Addr             string `yaml:"addr" json:"addr"`
					HTTPRedirectAddr string `yaml:"http_redirect_addr" json:"http_redirect_addr"`
					BaseURL          string `yaml:"base_url" json:"base_url"`
					TLSCert          string `yaml:"tls_cert" json:"tls_cert"`
					TLSKey           string `yaml:"tls_key" json:"tls_key"`
					RoutingMode      string `yaml:"routing_mode" json:"routing_mode"`
					BaseDomain       string `yaml:"base_domain" json:"base_domain"`
				}{
					Addr: ":8443",
				},
				Auth: struct {
					Issuer        string   `yaml:"issuer" json:"issuer"`
					Audience      string   `yaml:"audience" json:"audience"`
					HS256Secret   string   `yaml:"hs256_secret" json:"hs256_secret"`
					DefaultScopes []string `yaml:"default_scopes,omitempty" json:"default_scopes,omitempty"`
				}{
					HS256Secret: "",
				},
			},
			wantErr: false,
		},
		{
			name: "JWT secret too short",
			cfg: types.ServerConfig{
				Auth: struct {
					Issuer        string   `yaml:"issuer" json:"issuer"`
					Audience      string   `yaml:"audience" json:"audience"`
					HS256Secret   string   `yaml:"hs256_secret" json:"hs256_secret"`
					DefaultScopes []string `yaml:"default_scopes,omitempty" json:"default_scopes,omitempty"`
				}{
					HS256Secret: "short",
				},
			},
			wantErr:     true,
			errContains: "at least 32 characters",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Validate(tt.cfg)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				} else if !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("error %q should contain %q", err.Error(), tt.errContains)
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestApplyEnvOverrides(t *testing.T) {
	// Set test environment variables
	os.Setenv("NEKZUS_ADDR", ":9999")
	os.Setenv("NEKZUS_JWT_SECRET", "env-secret-12345678901234567890")
	defer func() {
		os.Unsetenv("NEKZUS_ADDR")
		os.Unsetenv("NEKZUS_JWT_SECRET")
	}()

	cfg := types.ServerConfig{}
	cfg.Server.Addr = ":8443"
	cfg.Auth.HS256Secret = "original"

	ApplyEnvOverrides(&cfg)

	if cfg.Server.Addr != ":9999" {
		t.Errorf("Addr = %q, want %q", cfg.Server.Addr, ":9999")
	}

	if cfg.Auth.HS256Secret != "env-secret-12345678901234567890" {
		t.Errorf("HS256Secret = %q, want %q", cfg.Auth.HS256Secret, "env-secret-12345678901234567890")
	}
}

func TestSetDefaults(t *testing.T) {
	cfg := &types.ServerConfig{}

	SetDefaults(cfg)

	if cfg.Server.Addr != ":8443" {
		t.Errorf("Addr = %q, want %q", cfg.Server.Addr, ":8443")
	}

	if cfg.Auth.Issuer != "nekzus" {
		t.Errorf("Issuer = %q, want %q", cfg.Auth.Issuer, "nekzus")
	}

	if cfg.Auth.Audience != "nekzus-mobile" {
		t.Errorf("Audience = %q, want %q", cfg.Auth.Audience, "nekzus-mobile")
	}
}

func TestValidateDurations(t *testing.T) {
	tests := []struct {
		name        string
		cfg         types.ServerConfig
		wantErr     bool
		errContains string
	}{
		{
			name: "invalid docker poll interval",
			cfg: types.ServerConfig{
				Auth: struct {
					Issuer        string   `yaml:"issuer" json:"issuer"`
					Audience      string   `yaml:"audience" json:"audience"`
					HS256Secret   string   `yaml:"hs256_secret" json:"hs256_secret"`
					DefaultScopes []string `yaml:"default_scopes,omitempty" json:"default_scopes,omitempty"`
				}{
					HS256Secret: strings.Repeat("a", 32),
				},
				Discovery: struct {
					Enabled    bool                   `yaml:"enabled" json:"enabled"`
					Docker     types.DockerConfig     `yaml:"docker" json:"docker"`
					MDNS       types.MDNSConfig       `yaml:"mdns" json:"mdns"`
					Kubernetes types.KubernetesConfig `yaml:"kubernetes" json:"kubernetes"`
				}{
					Enabled: true,
					Docker: types.DockerConfig{
						Enabled:      true,
						PollInterval: "invalid",
					},
				},
			},
			wantErr:     true,
			errContains: "invalid duration",
		},
		{
			name: "invalid health check interval",
			cfg: types.ServerConfig{
				Auth: struct {
					Issuer        string   `yaml:"issuer" json:"issuer"`
					Audience      string   `yaml:"audience" json:"audience"`
					HS256Secret   string   `yaml:"hs256_secret" json:"hs256_secret"`
					DefaultScopes []string `yaml:"default_scopes,omitempty" json:"default_scopes,omitempty"`
				}{
					HS256Secret: strings.Repeat("a", 32),
				},
				HealthChecks: types.HealthChecksConfig{
					Enabled:  true,
					Interval: "not-a-duration",
				},
			},
			wantErr:     true,
			errContains: "invalid duration",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Validate(tt.cfg)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				} else if !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("error %q should contain %q", err.Error(), tt.errContains)
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestValidateServerAddress(t *testing.T) {
	tests := []struct {
		name        string
		addr        string
		wantErr     bool
		errContains string
	}{
		{
			name:    "valid port only",
			addr:    ":8443",
			wantErr: false,
		},
		{
			name:    "valid with host",
			addr:    "localhost:8443",
			wantErr: false,
		},
		{
			name:        "invalid port - too high",
			addr:        ":99999",
			wantErr:     true,
			errContains: "invalid port",
		},
		{
			name:        "invalid port - negative",
			addr:        ":-1",
			wantErr:     true,
			errContains: "invalid port",
		},
		{
			name:        "invalid port - not a number",
			addr:        ":abc",
			wantErr:     true,
			errContains: "invalid port",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := types.ServerConfig{
				Server: struct {
					Addr             string `yaml:"addr" json:"addr"`
					HTTPRedirectAddr string `yaml:"http_redirect_addr" json:"http_redirect_addr"`
					BaseURL          string `yaml:"base_url" json:"base_url"`
					TLSCert          string `yaml:"tls_cert" json:"tls_cert"`
					TLSKey           string `yaml:"tls_key" json:"tls_key"`
					RoutingMode      string `yaml:"routing_mode" json:"routing_mode"`
					BaseDomain       string `yaml:"base_domain" json:"base_domain"`
				}{
					Addr: tt.addr,
				},
				Auth: struct {
					Issuer        string   `yaml:"issuer" json:"issuer"`
					Audience      string   `yaml:"audience" json:"audience"`
					HS256Secret   string   `yaml:"hs256_secret" json:"hs256_secret"`
					DefaultScopes []string `yaml:"default_scopes,omitempty" json:"default_scopes,omitempty"`
				}{
					HS256Secret: strings.Repeat("a", 32),
				},
			}

			err := Validate(cfg)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				} else if !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("error %q should contain %q", err.Error(), tt.errContains)
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestValidateMetricsConfig(t *testing.T) {
	tests := []struct {
		name        string
		cfg         types.ServerConfig
		wantErr     bool
		errContains string
	}{
		{
			name: "valid metrics config - enabled",
			cfg: types.ServerConfig{
				Auth: struct {
					Issuer        string   `yaml:"issuer" json:"issuer"`
					Audience      string   `yaml:"audience" json:"audience"`
					HS256Secret   string   `yaml:"hs256_secret" json:"hs256_secret"`
					DefaultScopes []string `yaml:"default_scopes,omitempty" json:"default_scopes,omitempty"`
				}{
					HS256Secret: strings.Repeat("a", 32),
				},
				Metrics: types.MetricsConfig{
					Enabled: true,
					Path:    "/metrics",
				},
			},
			wantErr: false,
		},
		{
			name: "valid metrics config - disabled",
			cfg: types.ServerConfig{
				Auth: struct {
					Issuer        string   `yaml:"issuer" json:"issuer"`
					Audience      string   `yaml:"audience" json:"audience"`
					HS256Secret   string   `yaml:"hs256_secret" json:"hs256_secret"`
					DefaultScopes []string `yaml:"default_scopes,omitempty" json:"default_scopes,omitempty"`
				}{
					HS256Secret: strings.Repeat("a", 32),
				},
				Metrics: types.MetricsConfig{
					Enabled: false,
					Path:    "/metrics",
				},
			},
			wantErr: false,
		},
		{
			name: "invalid metrics path - no leading slash",
			cfg: types.ServerConfig{
				Auth: struct {
					Issuer        string   `yaml:"issuer" json:"issuer"`
					Audience      string   `yaml:"audience" json:"audience"`
					HS256Secret   string   `yaml:"hs256_secret" json:"hs256_secret"`
					DefaultScopes []string `yaml:"default_scopes,omitempty" json:"default_scopes,omitempty"`
				}{
					HS256Secret: strings.Repeat("a", 32),
				},
				Metrics: types.MetricsConfig{
					Enabled: true,
					Path:    "metrics",
				},
			},
			wantErr:     true,
			errContains: "must start with /",
		},
		{
			name: "invalid metrics path - conflicts with API path",
			cfg: types.ServerConfig{
				Auth: struct {
					Issuer        string   `yaml:"issuer" json:"issuer"`
					Audience      string   `yaml:"audience" json:"audience"`
					HS256Secret   string   `yaml:"hs256_secret" json:"hs256_secret"`
					DefaultScopes []string `yaml:"default_scopes,omitempty" json:"default_scopes,omitempty"`
				}{
					HS256Secret: strings.Repeat("a", 32),
				},
				Metrics: types.MetricsConfig{
					Enabled: true,
					Path:    "/api/v1/metrics",
				},
			},
			wantErr:     true,
			errContains: "cannot conflict with API paths",
		},
		{
			name: "invalid metrics path - conflicts with healthz",
			cfg: types.ServerConfig{
				Auth: struct {
					Issuer        string   `yaml:"issuer" json:"issuer"`
					Audience      string   `yaml:"audience" json:"audience"`
					HS256Secret   string   `yaml:"hs256_secret" json:"hs256_secret"`
					DefaultScopes []string `yaml:"default_scopes,omitempty" json:"default_scopes,omitempty"`
				}{
					HS256Secret: strings.Repeat("a", 32),
				},
				Metrics: types.MetricsConfig{
					Enabled: true,
					Path:    "/api/v1/healthz",
				},
			},
			wantErr:     true,
			errContains: "cannot conflict with API paths",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Validate(tt.cfg)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				} else if !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("error %q should contain %q", err.Error(), tt.errContains)
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestSetDefaultsMetrics(t *testing.T) {
	cfg := &types.ServerConfig{}

	SetDefaults(cfg)

	// Test that metrics are enabled by default
	if !cfg.Metrics.Enabled {
		t.Errorf("Metrics.Enabled = %v, want true", cfg.Metrics.Enabled)
	}

	// Test that default path is set
	if cfg.Metrics.Path != "/metrics" {
		t.Errorf("Metrics.Path = %q, want %q", cfg.Metrics.Path, "/metrics")
	}
}

func TestSetDefaultsMetricsPreservesExisting(t *testing.T) {
	cfg := &types.ServerConfig{
		Metrics: types.MetricsConfig{
			Enabled: false,
			Path:    "/custom-metrics",
		},
	}

	SetDefaults(cfg)

	// Test that existing values are preserved
	if cfg.Metrics.Enabled {
		t.Errorf("Metrics.Enabled = %v, want false (should preserve existing)", cfg.Metrics.Enabled)
	}

	if cfg.Metrics.Path != "/custom-metrics" {
		t.Errorf("Metrics.Path = %q, want %q (should preserve existing)", cfg.Metrics.Path, "/custom-metrics")
	}
}

func TestValidateWeakSecrets(t *testing.T) {
	// Save original env and restore after test
	originalEnv := os.Getenv("ENVIRONMENT")
	defer func() {
		if originalEnv != "" {
			os.Setenv("ENVIRONMENT", originalEnv)
		} else {
			os.Unsetenv("ENVIRONMENT")
		}
	}()

	tests := []struct {
		name        string
		secret      string
		environment string
		wantErr     bool
		errContains string
	}{
		{
			name:        "weak secret in production",
			secret:      strings.Repeat("a", 32) + "-test-secret",
			environment: "production",
			wantErr:     true,
			errContains: "weak pattern",
		},
		{
			name:        "weak secret with 'dev' pattern in production",
			secret:      "development-secret-1234567890123",
			environment: "production",
			wantErr:     true,
			errContains: "weak pattern 'dev'",
		},
		{
			name:        "weak secret with 'change-me' in production",
			secret:      "change-me-please-12345678901234",
			environment: "production",
			wantErr:     true,
			errContains: "weak pattern 'change-me'",
		},
		{
			name:        "weak secret allowed in development",
			secret:      "test-secret-12345678901234567890123",
			environment: "development",
			wantErr:     false,
		},
		{
			name:        "weak secret allowed in dev",
			secret:      "test-secret-12345678901234567890123",
			environment: "dev",
			wantErr:     false,
		},
		{
			name:        "weak secret allowed in test",
			secret:      "test-secret-12345678901234567890123",
			environment: "test",
			wantErr:     false,
		},
		{
			name:        "weak secret allowed when env not set",
			secret:      "test-secret-12345678901234567890123",
			environment: "",
			wantErr:     false,
		},
		{
			// Bootstrap tokens are now optional, so a strong secret should pass validation
			name:        "strong secret in production",
			secret:      "XpK9#mN!vB2$qR7&wL4^zT6*jH8@fG3%",
			environment: "production",
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.environment != "" {
				os.Setenv("ENVIRONMENT", tt.environment)
			} else {
				os.Unsetenv("ENVIRONMENT")
			}

			cfg := types.ServerConfig{
				Auth: struct {
					Issuer        string   `yaml:"issuer" json:"issuer"`
					Audience      string   `yaml:"audience" json:"audience"`
					HS256Secret   string   `yaml:"hs256_secret" json:"hs256_secret"`
					DefaultScopes []string `yaml:"default_scopes,omitempty" json:"default_scopes,omitempty"`
				}{
					HS256Secret: tt.secret,
				},
			}

			err := Validate(cfg)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				} else if !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("error %q should contain %q", err.Error(), tt.errContains)
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestValidateBootstrapTokens(t *testing.T) {
	// Save original env and restore after test
	originalEnv := os.Getenv("ENVIRONMENT")
	originalToken := os.Getenv("NEKZUS_BOOTSTRAP_TOKEN")
	defer func() {
		if originalEnv != "" {
			os.Setenv("ENVIRONMENT", originalEnv)
		} else {
			os.Unsetenv("ENVIRONMENT")
		}
		if originalToken != "" {
			os.Setenv("NEKZUS_BOOTSTRAP_TOKEN", originalToken)
		} else {
			os.Unsetenv("NEKZUS_BOOTSTRAP_TOKEN")
		}
	}()

	tests := []struct {
		name        string
		tokens      []string
		envToken    string
		environment string
		wantErr     bool
		errContains string
	}{
		{
			// Bootstrap tokens are now optional - QR code flow generates short-lived tokens
			name:        "no tokens in production (allowed - uses QR flow)",
			tokens:      []string{},
			envToken:    "",
			environment: "production",
			wantErr:     false,
		},
		{
			name:        "tokens in config - production",
			tokens:      []string{"token123"},
			envToken:    "",
			environment: "production",
			wantErr:     false,
		},
		{
			name:        "token in env - production",
			tokens:      []string{},
			envToken:    "env-token-123",
			environment: "production",
			wantErr:     false,
		},
		{
			name:        "no tokens in development",
			tokens:      []string{},
			envToken:    "",
			environment: "development",
			wantErr:     false,
		},
		{
			name:        "no tokens in dev",
			tokens:      []string{},
			envToken:    "",
			environment: "dev",
			wantErr:     false,
		},
		{
			name:        "no tokens when env not set",
			tokens:      []string{},
			envToken:    "",
			environment: "",
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.environment != "" {
				os.Setenv("ENVIRONMENT", tt.environment)
			} else {
				os.Unsetenv("ENVIRONMENT")
			}

			if tt.envToken != "" {
				os.Setenv("NEKZUS_BOOTSTRAP_TOKEN", tt.envToken)
			} else {
				os.Unsetenv("NEKZUS_BOOTSTRAP_TOKEN")
			}

			cfg := types.ServerConfig{
				Auth: struct {
					Issuer        string   `yaml:"issuer" json:"issuer"`
					Audience      string   `yaml:"audience" json:"audience"`
					HS256Secret   string   `yaml:"hs256_secret" json:"hs256_secret"`
					DefaultScopes []string `yaml:"default_scopes,omitempty" json:"default_scopes,omitempty"`
				}{
					HS256Secret: strings.Repeat("a", 32),
				},
			}
			cfg.Bootstrap.Tokens = tt.tokens

			err := Validate(cfg)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				} else if !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("error %q should contain %q", err.Error(), tt.errContains)
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestValidateTLSConfig(t *testing.T) {
	tests := []struct {
		name        string
		tlsCert     string
		tlsKey      string
		wantErr     bool
		errContains string
	}{
		{
			name:    "both cert and key provided",
			tlsCert: "/path/to/cert.pem",
			tlsKey:  "/path/to/key.pem",
			wantErr: false,
		},
		{
			name:    "neither cert nor key",
			tlsCert: "",
			tlsKey:  "",
			wantErr: false,
		},
		{
			name:        "cert without key",
			tlsCert:     "/path/to/cert.pem",
			tlsKey:      "",
			wantErr:     true,
			errContains: "server.tls_key is required when server.tls_cert is set",
		},
		{
			name:        "key without cert",
			tlsCert:     "",
			tlsKey:      "/path/to/key.pem",
			wantErr:     true,
			errContains: "server.tls_cert is required when server.tls_key is set",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := types.ServerConfig{
				Server: struct {
					Addr             string `yaml:"addr" json:"addr"`
					HTTPRedirectAddr string `yaml:"http_redirect_addr" json:"http_redirect_addr"`
					BaseURL          string `yaml:"base_url" json:"base_url"`
					TLSCert          string `yaml:"tls_cert" json:"tls_cert"`
					TLSKey           string `yaml:"tls_key" json:"tls_key"`
					RoutingMode      string `yaml:"routing_mode" json:"routing_mode"`
					BaseDomain       string `yaml:"base_domain" json:"base_domain"`
				}{
					TLSCert: tt.tlsCert,
					TLSKey:  tt.tlsKey,
				},
				Auth: struct {
					Issuer        string   `yaml:"issuer" json:"issuer"`
					Audience      string   `yaml:"audience" json:"audience"`
					HS256Secret   string   `yaml:"hs256_secret" json:"hs256_secret"`
					DefaultScopes []string `yaml:"default_scopes,omitempty" json:"default_scopes,omitempty"`
				}{
					HS256Secret: strings.Repeat("a", 32),
				},
			}

			err := Validate(cfg)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				} else if !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("error %q should contain %q", err.Error(), tt.errContains)
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestValidateDockerNetworkMode(t *testing.T) {
	tests := []struct {
		name        string
		networkMode string
		networks    []string
		wantErr     bool
		errContains string
	}{
		{
			name:        "all mode",
			networkMode: "all",
			networks:    nil,
			wantErr:     false,
		},
		{
			name:        "first mode",
			networkMode: "first",
			networks:    nil,
			wantErr:     false,
		},
		{
			name:        "preferred mode with networks",
			networkMode: "preferred",
			networks:    []string{"bridge", "host"},
			wantErr:     false,
		},
		{
			name:        "preferred mode without networks",
			networkMode: "preferred",
			networks:    []string{},
			wantErr:     true,
			errContains: "preferred' requires discovery.docker.networks",
		},
		{
			name:        "invalid mode",
			networkMode: "invalid",
			networks:    nil,
			wantErr:     true,
			errContains: "invalid discovery.docker.network_mode",
		},
		{
			name:        "empty mode defaults to all",
			networkMode: "",
			networks:    nil,
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := types.ServerConfig{
				Auth: struct {
					Issuer        string   `yaml:"issuer" json:"issuer"`
					Audience      string   `yaml:"audience" json:"audience"`
					HS256Secret   string   `yaml:"hs256_secret" json:"hs256_secret"`
					DefaultScopes []string `yaml:"default_scopes,omitempty" json:"default_scopes,omitempty"`
				}{
					HS256Secret: strings.Repeat("a", 32),
				},
				Discovery: struct {
					Enabled    bool                   `yaml:"enabled" json:"enabled"`
					Docker     types.DockerConfig     `yaml:"docker" json:"docker"`
					MDNS       types.MDNSConfig       `yaml:"mdns" json:"mdns"`
					Kubernetes types.KubernetesConfig `yaml:"kubernetes" json:"kubernetes"`
				}{
					Enabled: true,
					Docker: types.DockerConfig{
						Enabled:     true,
						NetworkMode: tt.networkMode,
						Networks:    tt.networks,
					},
				},
			}

			err := Validate(cfg)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				} else if !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("error %q should contain %q", err.Error(), tt.errContains)
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestValidateRoutes(t *testing.T) {
	tests := []struct {
		name        string
		routes      []types.Route
		wantErr     bool
		errContains string
	}{
		{
			name: "valid routes",
			routes: []types.Route{
				{RouteID: "grafana", PathBase: "/grafana", To: "http://localhost:3000"},
			},
			wantErr: false,
		},
		{
			name: "missing route ID",
			routes: []types.Route{
				{RouteID: "", PathBase: "/grafana", To: "http://localhost:3000"},
			},
			wantErr:     true,
			errContains: "routes[0].id is required",
		},
		{
			name: "missing path_base",
			routes: []types.Route{
				{RouteID: "grafana", PathBase: "", To: "http://localhost:3000"},
			},
			wantErr:     true,
			errContains: "routes[0].path_base is required",
		},
		{
			name: "missing to",
			routes: []types.Route{
				{RouteID: "grafana", PathBase: "/grafana", To: ""},
			},
			wantErr:     true,
			errContains: "routes[0].to is required",
		},
		{
			name: "path_base without leading slash",
			routes: []types.Route{
				{RouteID: "grafana", PathBase: "grafana", To: "http://localhost:3000"},
			},
			wantErr:     true,
			errContains: "routes[0].path_base must start with /",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := types.ServerConfig{
				Auth: struct {
					Issuer        string   `yaml:"issuer" json:"issuer"`
					Audience      string   `yaml:"audience" json:"audience"`
					HS256Secret   string   `yaml:"hs256_secret" json:"hs256_secret"`
					DefaultScopes []string `yaml:"default_scopes,omitempty" json:"default_scopes,omitempty"`
				}{
					HS256Secret: strings.Repeat("a", 32),
				},
				Routes: tt.routes,
			}

			err := Validate(cfg)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				} else if !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("error %q should contain %q", err.Error(), tt.errContains)
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestValidateApps(t *testing.T) {
	tests := []struct {
		name        string
		apps        []types.App
		wantErr     bool
		errContains string
	}{
		{
			name: "valid apps",
			apps: []types.App{
				{ID: "grafana", Name: "Grafana"},
			},
			wantErr: false,
		},
		{
			name: "missing app ID",
			apps: []types.App{
				{ID: "", Name: "Grafana"},
			},
			wantErr:     true,
			errContains: "apps[0].id is required",
		},
		{
			name: "missing app name",
			apps: []types.App{
				{ID: "grafana", Name: ""},
			},
			wantErr:     true,
			errContains: "apps[0].name is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := types.ServerConfig{
				Auth: struct {
					Issuer        string   `yaml:"issuer" json:"issuer"`
					Audience      string   `yaml:"audience" json:"audience"`
					HS256Secret   string   `yaml:"hs256_secret" json:"hs256_secret"`
					DefaultScopes []string `yaml:"default_scopes,omitempty" json:"default_scopes,omitempty"`
				}{
					HS256Secret: strings.Repeat("a", 32),
				},
				Apps: tt.apps,
			}

			err := Validate(cfg)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				} else if !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("error %q should contain %q", err.Error(), tt.errContains)
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestApplyEnvOverridesComplete(t *testing.T) {
	// Set all test environment variables
	envVars := map[string]string{
		"NEKZUS_ADDR":                  ":9999",
		"NEKZUS_TLS_CERT":              "/env/cert.pem",
		"NEKZUS_TLS_KEY":               "/env/key.pem",
		"NEKZUS_JWT_SECRET":            "env-secret-12345678901234567890",
		"NEKZUS_BOOTSTRAP_TOKEN":       "env-bootstrap-token",
		"NEKZUS_DATABASE_PATH":         "/env/data/nexus.db",
		"NEKZUS_TOOLBOX_HOST_DATA_DIR": "/env/toolbox",
	}

	for k, v := range envVars {
		os.Setenv(k, v)
	}
	defer func() {
		for k := range envVars {
			os.Unsetenv(k)
		}
	}()

	cfg := types.ServerConfig{}
	cfg.Server.Addr = ":8443"
	cfg.Server.TLSCert = "/original/cert.pem"
	cfg.Server.TLSKey = "/original/key.pem"
	cfg.Auth.HS256Secret = "original"
	cfg.Bootstrap.Tokens = []string{"original-token"}
	cfg.Storage.DatabasePath = "/original/nexus.db"
	cfg.Toolbox.HostDataDir = "/original/toolbox"

	ApplyEnvOverrides(&cfg)

	// Verify all overrides were applied
	if cfg.Server.Addr != ":9999" {
		t.Errorf("Addr = %q, want %q", cfg.Server.Addr, ":9999")
	}
	if cfg.Server.TLSCert != "/env/cert.pem" {
		t.Errorf("TLSCert = %q, want %q", cfg.Server.TLSCert, "/env/cert.pem")
	}
	if cfg.Server.TLSKey != "/env/key.pem" {
		t.Errorf("TLSKey = %q, want %q", cfg.Server.TLSKey, "/env/key.pem")
	}
	if cfg.Auth.HS256Secret != "env-secret-12345678901234567890" {
		t.Errorf("HS256Secret = %q, want %q", cfg.Auth.HS256Secret, "env-secret-12345678901234567890")
	}
	if len(cfg.Bootstrap.Tokens) != 2 || cfg.Bootstrap.Tokens[1] != "env-bootstrap-token" {
		t.Errorf("Bootstrap tokens = %v, want to include env-bootstrap-token", cfg.Bootstrap.Tokens)
	}
	if cfg.Storage.DatabasePath != "/env/data/nexus.db" {
		t.Errorf("DatabasePath = %q, want %q", cfg.Storage.DatabasePath, "/env/data/nexus.db")
	}
	if cfg.Toolbox.HostDataDir != "/env/toolbox" {
		t.Errorf("HostDataDir = %q, want %q", cfg.Toolbox.HostDataDir, "/env/toolbox")
	}
}

func TestValidateRuntimesConfig(t *testing.T) {
	tests := []struct {
		name        string
		cfg         types.RuntimesConfig
		wantErr     bool
		errContains string
		wantPrimary string
	}{
		{
			name:        "empty config defaults to docker",
			cfg:         types.RuntimesConfig{},
			wantErr:     false,
			wantPrimary: "docker",
		},
		{
			name: "docker enabled sets docker as primary",
			cfg: types.RuntimesConfig{
				Docker: types.DockerRuntimeConfig{
					Enabled: true,
				},
			},
			wantErr:     false,
			wantPrimary: "docker",
		},
		{
			name: "kubernetes enabled sets kubernetes as primary",
			cfg: types.RuntimesConfig{
				Kubernetes: types.KubernetesRuntimeConfig{
					Enabled: true,
				},
			},
			wantErr:     false,
			wantPrimary: "kubernetes",
		},
		{
			name: "explicit primary is respected",
			cfg: types.RuntimesConfig{
				Primary: "kubernetes",
				Docker: types.DockerRuntimeConfig{
					Enabled: true,
				},
				Kubernetes: types.KubernetesRuntimeConfig{
					Enabled: true,
				},
			},
			wantErr:     false,
			wantPrimary: "kubernetes",
		},
		{
			name: "invalid primary returns error",
			cfg: types.RuntimesConfig{
				Primary: "invalid",
			},
			wantErr:     true,
			errContains: "invalid value",
		},
		{
			name: "docker socket path defaults",
			cfg: types.RuntimesConfig{
				Docker: types.DockerRuntimeConfig{
					Enabled: true,
				},
			},
			wantErr:     false,
			wantPrimary: "docker",
		},
		{
			name: "kubernetes metrics cache TTL validates",
			cfg: types.RuntimesConfig{
				Kubernetes: types.KubernetesRuntimeConfig{
					Enabled:         true,
					MetricsCacheTTL: "invalid",
				},
			},
			wantErr:     true,
			errContains: "metrics_cache_ttl",
		},
		{
			name: "valid kubernetes config",
			cfg: types.RuntimesConfig{
				Kubernetes: types.KubernetesRuntimeConfig{
					Enabled:         true,
					Kubeconfig:      "/path/to/kubeconfig",
					Context:         "my-context",
					Namespaces:      []string{"default", "production"},
					MetricsServer:   true,
					MetricsCacheTTL: "60s",
				},
			},
			wantErr:     false,
			wantPrimary: "kubernetes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateRuntimesConfig(&tt.cfg)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got nil")
				} else if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("error = %q, want to contain %q", err.Error(), tt.errContains)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if tt.wantPrimary != "" && tt.cfg.Primary != tt.wantPrimary {
					t.Errorf("Primary = %q, want %q", tt.cfg.Primary, tt.wantPrimary)
				}
			}
		})
	}
}
