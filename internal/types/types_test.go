package types

import (
	"encoding/json"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestServerConfigYAMLUnmarshal(t *testing.T) {
	yamlData := `
server:
  addr: ":8443"
  tls_cert: "/path/to/cert.pem"
  tls_key: "/path/to/key.pem"
auth:
  issuer: "test-issuer"
  audience: "test-audience"
  hs256_secret: "test-secret"
bootstrap:
  tokens:
    - "token1"
    - "token2"
discovery:
  enabled: true
  docker:
    enabled: true
    socket_path: "unix:///var/run/docker.sock"
    poll_interval: "30s"
  mdns:
    enabled: false
    services:
      - "_http._tcp"
    scan_interval: "60s"
routes:
  - id: "route1"
    app_id: "app1"
    path_base: "/apps/app1/"
    to: "http://localhost:3000"
    scopes:
      - "read:app1"
apps:
  - id: "app1"
    name: "Test App"
    icon: "icon.png"
    tags:
      - "test"
    endpoints:
      lan: "http://192.168.1.100:3000"
`

	var cfg ServerConfig
	err := yaml.Unmarshal([]byte(yamlData), &cfg)
	if err != nil {
		t.Fatalf("Failed to unmarshal YAML: %v", err)
	}

	// Verify server config
	if cfg.Server.Addr != ":8443" {
		t.Errorf("Expected addr :8443, got %s", cfg.Server.Addr)
	}
	if cfg.Server.TLSCert != "/path/to/cert.pem" {
		t.Errorf("Expected tls_cert /path/to/cert.pem, got %s", cfg.Server.TLSCert)
	}

	// Verify auth config
	if cfg.Auth.Issuer != "test-issuer" {
		t.Errorf("Expected issuer test-issuer, got %s", cfg.Auth.Issuer)
	}

	// Verify bootstrap tokens
	if len(cfg.Bootstrap.Tokens) != 2 {
		t.Errorf("Expected 2 bootstrap tokens, got %d", len(cfg.Bootstrap.Tokens))
	}

	// Verify discovery config
	if !cfg.Discovery.Enabled {
		t.Error("Expected discovery to be enabled")
	}
	if !cfg.Discovery.Docker.Enabled {
		t.Error("Expected docker discovery to be enabled")
	}
	if cfg.Discovery.MDNS.Enabled {
		t.Error("Expected mdns discovery to be disabled")
	}

	// Verify routes
	if len(cfg.Routes) != 1 {
		t.Fatalf("Expected 1 route, got %d", len(cfg.Routes))
	}
	if cfg.Routes[0].RouteID != "route1" {
		t.Errorf("Expected route id route1, got %s", cfg.Routes[0].RouteID)
	}

	// Verify apps
	if len(cfg.Apps) != 1 {
		t.Fatalf("Expected 1 app, got %d", len(cfg.Apps))
	}
	if cfg.Apps[0].ID != "app1" {
		t.Errorf("Expected app id app1, got %s", cfg.Apps[0].ID)
	}
}

func TestAppJSONMarshal(t *testing.T) {
	app := App{
		ID:   "test-app",
		Name: "Test Application",
		Icon: "test.png",
		Tags: []string{"tag1", "tag2"},
		Endpoints: map[string]string{
			"lan":  "http://192.168.1.100:8080",
			"mdns": "http://testapp.local:8080",
		},
	}

	data, err := json.Marshal(app)
	if err != nil {
		t.Fatalf("Failed to marshal app: %v", err)
	}

	var unmarshaled App
	err = json.Unmarshal(data, &unmarshaled)
	if err != nil {
		t.Fatalf("Failed to unmarshal app: %v", err)
	}

	if unmarshaled.ID != app.ID {
		t.Errorf("Expected ID %s, got %s", app.ID, unmarshaled.ID)
	}
	if unmarshaled.Name != app.Name {
		t.Errorf("Expected Name %s, got %s", app.Name, unmarshaled.Name)
	}
	if len(unmarshaled.Tags) != len(app.Tags) {
		t.Errorf("Expected %d tags, got %d", len(app.Tags), len(unmarshaled.Tags))
	}
	if len(unmarshaled.Endpoints) != len(app.Endpoints) {
		t.Errorf("Expected %d endpoints, got %d", len(app.Endpoints), len(unmarshaled.Endpoints))
	}
}

func TestRouteJSONMarshal(t *testing.T) {
	route := Route{
		RouteID:   "route-test",
		AppID:     "app-test",
		PathBase:  "/apps/test/",
		To:        "http://localhost:3000",
		Scopes:    []string{"read:test", "write:test"},
		Websocket: true,
	}

	data, err := json.Marshal(route)
	if err != nil {
		t.Fatalf("Failed to marshal route: %v", err)
	}

	var unmarshaled Route
	err = json.Unmarshal(data, &unmarshaled)
	if err != nil {
		t.Fatalf("Failed to unmarshal route: %v", err)
	}

	if unmarshaled.RouteID != route.RouteID {
		t.Errorf("Expected RouteID %s, got %s", route.RouteID, unmarshaled.RouteID)
	}
	if unmarshaled.To != route.To {
		t.Errorf("Expected To %s, got %s", route.To, unmarshaled.To)
	}
	if unmarshaled.Websocket != route.Websocket {
		t.Errorf("Expected Websocket %v, got %v", route.Websocket, unmarshaled.Websocket)
	}
}

func TestProposalJSONMarshal(t *testing.T) {
	proposal := Proposal{
		ID:             "proposal-123",
		Source:         "docker",
		DetectedScheme: "http",
		DetectedHost:   "192.168.1.100",
		DetectedPort:   8080,
		Confidence:     0.85,
		SuggestedApp: App{
			ID:   "detected-app",
			Name: "Detected App",
		},
		SuggestedRoute: Route{
			RouteID:  "route-detected",
			AppID:    "detected-app",
			PathBase: "/apps/detected/",
			To:       "http://192.168.1.100:8080",
		},
		Tags:          []string{"docker", "automated"},
		LastSeen:      "2024-10-08T12:00:00Z",
		SecurityNotes: []string{"HTTP only", "No authentication"},
	}

	data, err := json.Marshal(proposal)
	if err != nil {
		t.Fatalf("Failed to marshal proposal: %v", err)
	}

	var unmarshaled Proposal
	err = json.Unmarshal(data, &unmarshaled)
	if err != nil {
		t.Fatalf("Failed to unmarshal proposal: %v", err)
	}

	if unmarshaled.ID != proposal.ID {
		t.Errorf("Expected ID %s, got %s", proposal.ID, unmarshaled.ID)
	}
	if unmarshaled.Source != proposal.Source {
		t.Errorf("Expected Source %s, got %s", proposal.Source, unmarshaled.Source)
	}
	if unmarshaled.Confidence != proposal.Confidence {
		t.Errorf("Expected Confidence %f, got %f", proposal.Confidence, unmarshaled.Confidence)
	}
	if unmarshaled.DetectedPort != proposal.DetectedPort {
		t.Errorf("Expected DetectedPort %d, got %d", proposal.DetectedPort, unmarshaled.DetectedPort)
	}
	if len(unmarshaled.SecurityNotes) != len(proposal.SecurityNotes) {
		t.Errorf("Expected %d security notes, got %d", len(proposal.SecurityNotes), len(unmarshaled.SecurityNotes))
	}
}

func TestDockerConfigDefaults(t *testing.T) {
	cfg := DockerConfig{
		Enabled:      true,
		SocketPath:   "",
		PollInterval: "",
	}

	if !cfg.Enabled {
		t.Error("Expected docker discovery to be enabled")
	}
	// Empty socket path and poll interval should be handled by the application
	// This test just verifies the struct can be created with zero values
}

func TestMDNSConfigDefaults(t *testing.T) {
	cfg := MDNSConfig{
		Enabled:      true,
		Services:     []string{"_http._tcp", "_https._tcp"},
		ScanInterval: "60s",
	}

	if !cfg.Enabled {
		t.Error("Expected mdns discovery to be enabled")
	}
	if len(cfg.Services) != 2 {
		t.Errorf("Expected 2 services, got %d", len(cfg.Services))
	}
	if cfg.ScanInterval != "60s" {
		t.Errorf("Expected scan interval 60s, got %s", cfg.ScanInterval)
	}
}

func TestDeviceTokenValidation(t *testing.T) {
	// Test that DeviceToken can be properly marshaled/unmarshaled
	// This is important for JWT token storage
	token := DeviceToken{
		Token:     "test-token-abc123",
		DeviceID:  "device-ios-1",
		ExpiresAt: mustParseTime("2024-12-31T23:59:59Z"),
		Scopes:    []string{"read:catalog", "read:events"},
	}

	data, err := json.Marshal(token)
	if err != nil {
		t.Fatalf("Failed to marshal DeviceToken: %v", err)
	}

	var unmarshaled DeviceToken
	err = json.Unmarshal(data, &unmarshaled)
	if err != nil {
		t.Fatalf("Failed to unmarshal DeviceToken: %v", err)
	}

	if unmarshaled.Token != token.Token {
		t.Errorf("Expected Token %s, got %s", token.Token, unmarshaled.Token)
	}
	if unmarshaled.DeviceID != token.DeviceID {
		t.Errorf("Expected DeviceID %s, got %s", token.DeviceID, unmarshaled.DeviceID)
	}
	if len(unmarshaled.Scopes) != len(token.Scopes) {
		t.Errorf("Expected %d scopes, got %d", len(token.Scopes), len(unmarshaled.Scopes))
	}
}

// Helper function to parse time for tests
func mustParseTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

// --- Effective Scopes Tests ---

func TestRoute_GetEffectiveScopes(t *testing.T) {
	tests := []struct {
		name          string
		route         Route
		defaultScopes []string
		expected      []string
	}{
		{
			name: "route with explicit scopes ignores defaults",
			route: Route{
				RouteID: "r1",
				Scopes:  []string{"read:app", "write:app"},
			},
			defaultScopes: []string{"authenticated"},
			expected:      []string{"read:app", "write:app"},
		},
		{
			name: "route without scopes uses defaults",
			route: Route{
				RouteID: "r2",
				Scopes:  nil,
			},
			defaultScopes: []string{"authenticated"},
			expected:      []string{"authenticated"},
		},
		{
			name: "route with empty scopes uses defaults",
			route: Route{
				RouteID: "r3",
				Scopes:  []string{},
			},
			defaultScopes: []string{"authenticated"},
			expected:      []string{"authenticated"},
		},
		{
			name: "public access bypasses all scopes",
			route: Route{
				RouteID:      "r4",
				Scopes:       []string{"read:app"},
				PublicAccess: true,
			},
			defaultScopes: []string{"authenticated"},
			expected:      nil,
		},
		{
			name: "public access bypasses default scopes",
			route: Route{
				RouteID:      "r5",
				PublicAccess: true,
			},
			defaultScopes: []string{"authenticated"},
			expected:      nil,
		},
		{
			name: "no scopes and no defaults returns nil",
			route: Route{
				RouteID: "r6",
			},
			defaultScopes: nil,
			expected:      nil,
		},
		{
			name: "empty default scopes returns empty",
			route: Route{
				RouteID: "r7",
			},
			defaultScopes: []string{},
			expected:      []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.route.GetEffectiveScopes(tt.defaultScopes)

			// Compare nil/empty carefully
			if tt.expected == nil {
				if result != nil {
					t.Errorf("Expected nil, got %v", result)
				}
				return
			}

			if len(result) != len(tt.expected) {
				t.Errorf("Expected %d scopes %v, got %d scopes %v", len(tt.expected), tt.expected, len(result), result)
				return
			}

			for i, scope := range tt.expected {
				if result[i] != scope {
					t.Errorf("Expected scope[%d] = %s, got %s", i, scope, result[i])
				}
			}
		})
	}
}

func TestRoute_GetEffectiveScopes_PublicAccessYAML(t *testing.T) {
	yamlData := `
id: test-route
app_id: test-app
path_base: /apps/test/
to: http://localhost:3000
public_access: true
`
	var route Route
	if err := yaml.Unmarshal([]byte(yamlData), &route); err != nil {
		t.Fatalf("Failed to unmarshal route: %v", err)
	}

	if !route.PublicAccess {
		t.Error("Expected PublicAccess to be true from YAML")
	}

	scopes := route.GetEffectiveScopes([]string{"authenticated"})
	if scopes != nil {
		t.Errorf("Expected nil scopes for public access route, got %v", scopes)
	}
}

func TestServerConfig_DefaultScopes(t *testing.T) {
	yamlData := `
server:
  addr: ":8443"
auth:
  issuer: "test"
  default_scopes:
    - "authenticated"
    - "basic"
`
	var cfg ServerConfig
	if err := yaml.Unmarshal([]byte(yamlData), &cfg); err != nil {
		t.Fatalf("Failed to unmarshal config: %v", err)
	}

	if len(cfg.Auth.DefaultScopes) != 2 {
		t.Errorf("Expected 2 default scopes, got %d", len(cfg.Auth.DefaultScopes))
	}
	if cfg.Auth.DefaultScopes[0] != "authenticated" {
		t.Errorf("Expected first scope to be 'authenticated', got %s", cfg.Auth.DefaultScopes[0])
	}
}
