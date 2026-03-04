package config

import (
	"os"
	"testing"

	"github.com/nstalgic/nekzus/internal/types"
)

// Test 1: Resolver creation
func TestNewResolver(t *testing.T) {
	// Act
	resolver := NewResolver()

	// Assert
	if resolver == nil {
		t.Fatal("Expected resolver to be created")
	}
	if resolver.envPrefix != "NEKZUS_" {
		t.Errorf("Expected envPrefix to be 'NEKZUS_', got %s", resolver.envPrefix)
	}
}

// Test 2: ResolveBaseURL - Priority 1: Environment variable
func TestResolveBaseURL_EnvVar(t *testing.T) {
	// Arrange
	os.Setenv("NEKZUS_BASE_URL", "https://env-url.example.com")
	defer os.Unsetenv("NEKZUS_BASE_URL")

	resolver := NewResolver()
	cfg := types.ServerConfig{
		Server: struct {
			Addr             string `yaml:"addr" json:"addr"`
			HTTPRedirectAddr string `yaml:"http_redirect_addr" json:"http_redirect_addr"`
			BaseURL          string `yaml:"base_url" json:"base_url"`
			TLSCert          string `yaml:"tls_cert" json:"tls_cert"`
			TLSKey           string `yaml:"tls_key" json:"tls_key"`
		}{
			BaseURL: "https://config-url.example.com",
		},
	}
	getLocalIP := func() string { return "192.168.1.100" }

	// Act
	result := resolver.ResolveBaseURL(cfg, getLocalIP)

	// Assert
	expected := "https://env-url.example.com"
	if result != expected {
		t.Errorf("Expected %s, got %s", expected, result)
	}
}

// Test 3: ResolveBaseURL - Priority 2: Config file
func TestResolveBaseURL_ConfigFile(t *testing.T) {
	// Arrange
	os.Unsetenv("NEKZUS_BASE_URL") // Ensure env var is not set

	resolver := NewResolver()
	cfg := types.ServerConfig{
		Server: struct {
			Addr             string `yaml:"addr" json:"addr"`
			HTTPRedirectAddr string `yaml:"http_redirect_addr" json:"http_redirect_addr"`
			BaseURL          string `yaml:"base_url" json:"base_url"`
			TLSCert          string `yaml:"tls_cert" json:"tls_cert"`
			TLSKey           string `yaml:"tls_key" json:"tls_key"`
		}{
			BaseURL: "https://config-url.example.com",
		},
	}
	getLocalIP := func() string { return "192.168.1.100" }

	// Act
	result := resolver.ResolveBaseURL(cfg, getLocalIP)

	// Assert
	expected := "https://config-url.example.com"
	if result != expected {
		t.Errorf("Expected %s, got %s", expected, result)
	}
}

// Test 4: ResolveBaseURL - Priority 3: Auto-detection with HTTP
func TestResolveBaseURL_AutoDetect_HTTP(t *testing.T) {
	// Arrange
	os.Unsetenv("NEKZUS_BASE_URL")

	resolver := NewResolver()
	cfg := types.ServerConfig{
		Server: struct {
			Addr             string `yaml:"addr" json:"addr"`
			HTTPRedirectAddr string `yaml:"http_redirect_addr" json:"http_redirect_addr"`
			BaseURL          string `yaml:"base_url" json:"base_url"`
			TLSCert          string `yaml:"tls_cert" json:"tls_cert"`
			TLSKey           string `yaml:"tls_key" json:"tls_key"`
		}{
			BaseURL: "", // No config URL
			Addr:    ":8080",
			TLSCert: "", // No TLS
			TLSKey:  "",
		},
	}
	getLocalIP := func() string { return "192.168.1.100" }

	// Act
	result := resolver.ResolveBaseURL(cfg, getLocalIP)

	// Assert
	expected := "http://192.168.1.100:8080"
	if result != expected {
		t.Errorf("Expected %s, got %s", expected, result)
	}
}

// Test 5: ResolveBaseURL - Priority 3: Auto-detection with HTTPS
func TestResolveBaseURL_AutoDetect_HTTPS(t *testing.T) {
	// Arrange
	os.Unsetenv("NEKZUS_BASE_URL")

	resolver := NewResolver()
	cfg := types.ServerConfig{
		Server: struct {
			Addr             string `yaml:"addr" json:"addr"`
			HTTPRedirectAddr string `yaml:"http_redirect_addr" json:"http_redirect_addr"`
			BaseURL          string `yaml:"base_url" json:"base_url"`
			TLSCert          string `yaml:"tls_cert" json:"tls_cert"`
			TLSKey           string `yaml:"tls_key" json:"tls_key"`
		}{
			BaseURL: "",
			Addr:    ":8443",
			TLSCert: "/path/to/cert.pem", // TLS enabled
			TLSKey:  "/path/to/key.pem",
		},
	}
	getLocalIP := func() string { return "192.168.1.50" }

	// Act
	result := resolver.ResolveBaseURL(cfg, getLocalIP)

	// Assert
	expected := "https://192.168.1.50:8443"
	if result != expected {
		t.Errorf("Expected %s, got %s", expected, result)
	}
}

// Test 6: WasAutoDetected - returns true when auto-detected
func TestWasAutoDetected_True(t *testing.T) {
	// Arrange
	os.Unsetenv("NEKZUS_BASE_URL")

	resolver := NewResolver()
	cfg := types.ServerConfig{
		Server: struct {
			Addr             string `yaml:"addr" json:"addr"`
			HTTPRedirectAddr string `yaml:"http_redirect_addr" json:"http_redirect_addr"`
			BaseURL          string `yaml:"base_url" json:"base_url"`
			TLSCert          string `yaml:"tls_cert" json:"tls_cert"`
			TLSKey           string `yaml:"tls_key" json:"tls_key"`
		}{
			BaseURL: "", // No config URL
		},
	}

	// Act
	result := resolver.WasAutoDetected(cfg)

	// Assert
	if !result {
		t.Error("Expected WasAutoDetected to return true")
	}
}

// Test 7: WasAutoDetected - returns false when from env var
func TestWasAutoDetected_False_EnvVar(t *testing.T) {
	// Arrange
	os.Setenv("NEKZUS_BASE_URL", "https://env-url.example.com")
	defer os.Unsetenv("NEKZUS_BASE_URL")

	resolver := NewResolver()
	cfg := types.ServerConfig{
		Server: struct {
			Addr             string `yaml:"addr" json:"addr"`
			HTTPRedirectAddr string `yaml:"http_redirect_addr" json:"http_redirect_addr"`
			BaseURL          string `yaml:"base_url" json:"base_url"`
			TLSCert          string `yaml:"tls_cert" json:"tls_cert"`
			TLSKey           string `yaml:"tls_key" json:"tls_key"`
		}{
			BaseURL: "",
		},
	}

	// Act
	result := resolver.WasAutoDetected(cfg)

	// Assert
	if result {
		t.Error("Expected WasAutoDetected to return false when env var is set")
	}
}

// Test 8: WasAutoDetected - returns false when from config file
func TestWasAutoDetected_False_ConfigFile(t *testing.T) {
	// Arrange
	os.Unsetenv("NEKZUS_BASE_URL")

	resolver := NewResolver()
	cfg := types.ServerConfig{
		Server: struct {
			Addr             string `yaml:"addr" json:"addr"`
			HTTPRedirectAddr string `yaml:"http_redirect_addr" json:"http_redirect_addr"`
			BaseURL          string `yaml:"base_url" json:"base_url"`
			TLSCert          string `yaml:"tls_cert" json:"tls_cert"`
			TLSKey           string `yaml:"tls_key" json:"tls_key"`
		}{
			BaseURL: "https://config-url.example.com",
		},
	}

	// Act
	result := resolver.WasAutoDetected(cfg)

	// Assert
	if result {
		t.Error("Expected WasAutoDetected to return false when config URL is set")
	}
}

// Test 9: Priority ordering - env var overrides config
func TestResolveBaseURL_PriorityOrdering(t *testing.T) {
	// Arrange
	os.Setenv("NEKZUS_BASE_URL", "https://priority-env.example.com")
	defer os.Unsetenv("NEKZUS_BASE_URL")

	resolver := NewResolver()
	cfg := types.ServerConfig{
		Server: struct {
			Addr             string `yaml:"addr" json:"addr"`
			HTTPRedirectAddr string `yaml:"http_redirect_addr" json:"http_redirect_addr"`
			BaseURL          string `yaml:"base_url" json:"base_url"`
			TLSCert          string `yaml:"tls_cert" json:"tls_cert"`
			TLSKey           string `yaml:"tls_key" json:"tls_key"`
		}{
			BaseURL: "https://priority-config.example.com",
			Addr:    ":8080",
		},
	}
	getLocalIP := func() string { return "192.168.1.200" }

	// Act
	result := resolver.ResolveBaseURL(cfg, getLocalIP)

	// Assert
	expected := "https://priority-env.example.com"
	if result != expected {
		t.Errorf("Expected env var to take priority, got %s", result)
	}
}
