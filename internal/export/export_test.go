package export

import (
	"archive/zip"
	"bytes"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/go-connections/nat"
	"gopkg.in/yaml.v3"
)

// TestMapEnvironment tests environment variable mapping with sanitization
func TestMapEnvironment(t *testing.T) {
	tests := []struct {
		name             string
		env              []string
		sanitizeSecrets  bool
		expectedValues   map[string]string
		expectedWarnings []string
	}{
		{
			name:            "simple environment variables",
			env:             []string{"DB_HOST=localhost", "DB_PORT=5432"},
			sanitizeSecrets: false,
			expectedValues: map[string]string{
				"DB_HOST": "localhost",
				"DB_PORT": "5432",
			},
			expectedWarnings: nil,
		},
		{
			name:            "sanitize password",
			env:             []string{"DB_HOST=localhost", "DB_PASSWORD=secret123"},
			sanitizeSecrets: true,
			expectedValues: map[string]string{
				"DB_HOST":     "localhost",
				"DB_PASSWORD": "${DB_PASSWORD:?Required}",
			},
			expectedWarnings: []string{"DB_PASSWORD"},
		},
		{
			name:            "sanitize multiple sensitive vars",
			env:             []string{"API_KEY=abc123", "SECRET_TOKEN=xyz", "APP_NAME=myapp"},
			sanitizeSecrets: true,
			expectedValues: map[string]string{
				"API_KEY":      "${API_KEY:?Required}",
				"SECRET_TOKEN": "${SECRET_TOKEN:?Required}",
				"APP_NAME":     "myapp",
			},
			expectedWarnings: []string{"API_KEY", "SECRET_TOKEN"},
		},
		{
			name:            "no sanitization when disabled",
			env:             []string{"DB_PASSWORD=secret123"},
			sanitizeSecrets: false,
			expectedValues: map[string]string{
				"DB_PASSWORD": "secret123",
			},
			expectedWarnings: nil,
		},
		{
			name:            "empty environment",
			env:             []string{},
			sanitizeSecrets: true,
			expectedValues:  map[string]string{},
		},
		{
			name:            "malformed env var ignored",
			env:             []string{"VALID=value", "INVALID_NO_EQUALS"},
			sanitizeSecrets: false,
			expectedValues: map[string]string{
				"VALID": "value",
			},
		},
		{
			name:            "env var with equals in value",
			env:             []string{"CONNECTION_STRING=host=localhost;port=5432"},
			sanitizeSecrets: false,
			expectedValues: map[string]string{
				"CONNECTION_STRING": "host=localhost;port=5432",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, warnings := MapEnvironment(tt.env, tt.sanitizeSecrets)

			// Check values
			for key, expected := range tt.expectedValues {
				if got, ok := result[key]; !ok {
					t.Errorf("expected key %s not found", key)
				} else if got != expected {
					t.Errorf("key %s: expected %q, got %q", key, expected, got)
				}
			}

			// Check no extra keys
			if len(result) != len(tt.expectedValues) {
				t.Errorf("expected %d keys, got %d", len(tt.expectedValues), len(result))
			}

			// Check warnings contain expected sensitive vars
			for _, expectedWarning := range tt.expectedWarnings {
				found := false
				for _, w := range warnings {
					if strings.Contains(w, expectedWarning) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected warning containing %q not found in %v", expectedWarning, warnings)
				}
			}
		})
	}
}

// TestMapVolumes tests volume mapping from Docker mounts
func TestMapVolumes(t *testing.T) {
	tests := []struct {
		name           string
		mounts         []container.MountPoint
		expectedTypes  map[string]string // destination -> type
		expectedSource map[string]string // destination -> source
		expectedRO     map[string]bool   // destination -> read-only
	}{
		{
			name: "named volume",
			mounts: []container.MountPoint{
				{
					Type:        mount.TypeVolume,
					Name:        "pgdata",
					Destination: "/var/lib/postgresql/data",
					RW:          true,
				},
			},
			expectedTypes:  map[string]string{"/var/lib/postgresql/data": "volume"},
			expectedSource: map[string]string{"/var/lib/postgresql/data": "pgdata"},
			expectedRO:     map[string]bool{"/var/lib/postgresql/data": false},
		},
		{
			name: "bind mount",
			mounts: []container.MountPoint{
				{
					Type:        mount.TypeBind,
					Source:      "/host/config",
					Destination: "/app/config",
					RW:          false,
				},
			},
			expectedTypes:  map[string]string{"/app/config": "bind"},
			expectedSource: map[string]string{"/app/config": "/host/config"},
			expectedRO:     map[string]bool{"/app/config": true},
		},
		{
			name: "tmpfs mount",
			mounts: []container.MountPoint{
				{
					Type:        mount.TypeTmpfs,
					Destination: "/tmp",
					RW:          true,
				},
			},
			expectedTypes:  map[string]string{"/tmp": "tmpfs"},
			expectedSource: map[string]string{"/tmp": ""},
			expectedRO:     map[string]bool{"/tmp": false},
		},
		{
			name: "mixed volumes",
			mounts: []container.MountPoint{
				{
					Type:        mount.TypeVolume,
					Name:        "data",
					Destination: "/data",
					RW:          true,
				},
				{
					Type:        mount.TypeBind,
					Source:      "/etc/config",
					Destination: "/config",
					RW:          false,
				},
			},
			expectedTypes: map[string]string{
				"/data":   "volume",
				"/config": "bind",
			},
			expectedSource: map[string]string{
				"/data":   "data",
				"/config": "/etc/config",
			},
			expectedRO: map[string]bool{
				"/data":   false,
				"/config": true,
			},
		},
		{
			name:           "empty mounts",
			mounts:         []container.MountPoint{},
			expectedTypes:  map[string]string{},
			expectedSource: map[string]string{},
			expectedRO:     map[string]bool{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MapVolumes(tt.mounts)

			if len(result) != len(tt.expectedTypes) {
				t.Errorf("expected %d volumes, got %d", len(tt.expectedTypes), len(result))
			}

			for _, vol := range result {
				expectedType, ok := tt.expectedTypes[vol.Target]
				if !ok {
					t.Errorf("unexpected volume target: %s", vol.Target)
					continue
				}
				if vol.Type != expectedType {
					t.Errorf("volume %s: expected type %s, got %s", vol.Target, expectedType, vol.Type)
				}
				if vol.Source != tt.expectedSource[vol.Target] {
					t.Errorf("volume %s: expected source %s, got %s", vol.Target, tt.expectedSource[vol.Target], vol.Source)
				}
				if vol.ReadOnly != tt.expectedRO[vol.Target] {
					t.Errorf("volume %s: expected read-only %v, got %v", vol.Target, tt.expectedRO[vol.Target], vol.ReadOnly)
				}
			}
		})
	}
}

// TestMapPorts tests port mapping from Docker port bindings
func TestMapPorts(t *testing.T) {
	tests := []struct {
		name          string
		portBindings  nat.PortMap
		exposedPorts  nat.PortSet
		expectedPorts []string // Expected in format "host:container" or "container"
	}{
		{
			name: "simple port mapping",
			portBindings: nat.PortMap{
				"80/tcp": []nat.PortBinding{{HostIP: "0.0.0.0", HostPort: "8080"}},
			},
			exposedPorts:  nat.PortSet{"80/tcp": struct{}{}},
			expectedPorts: []string{"8080:80"},
		},
		{
			name: "multiple port mappings",
			portBindings: nat.PortMap{
				"80/tcp":  []nat.PortBinding{{HostIP: "0.0.0.0", HostPort: "8080"}},
				"443/tcp": []nat.PortBinding{{HostIP: "0.0.0.0", HostPort: "8443"}},
			},
			exposedPorts: nat.PortSet{
				"80/tcp":  struct{}{},
				"443/tcp": struct{}{},
			},
			expectedPorts: []string{"8080:80", "8443:443"},
		},
		{
			name: "udp port",
			portBindings: nat.PortMap{
				"53/udp": []nat.PortBinding{{HostIP: "0.0.0.0", HostPort: "5353"}},
			},
			exposedPorts:  nat.PortSet{"53/udp": struct{}{}},
			expectedPorts: []string{"5353:53/udp"},
		},
		{
			name: "specific host IP",
			portBindings: nat.PortMap{
				"80/tcp": []nat.PortBinding{{HostIP: "127.0.0.1", HostPort: "8080"}},
			},
			exposedPorts:  nat.PortSet{"80/tcp": struct{}{}},
			expectedPorts: []string{"127.0.0.1:8080:80"},
		},
		{
			name:          "exposed port without binding",
			portBindings:  nat.PortMap{},
			exposedPorts:  nat.PortSet{"3000/tcp": struct{}{}},
			expectedPorts: []string{}, // Expose-only ports handled separately
		},
		{
			name:          "empty ports",
			portBindings:  nat.PortMap{},
			exposedPorts:  nat.PortSet{},
			expectedPorts: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MapPorts(tt.portBindings)

			if len(result) != len(tt.expectedPorts) {
				t.Errorf("expected %d ports, got %d: %v", len(tt.expectedPorts), len(result), result)
				return
			}

			// Check each expected port is present
			for _, expected := range tt.expectedPorts {
				found := false
				for _, got := range result {
					if got == expected {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected port %s not found in %v", expected, result)
				}
			}
		})
	}
}

// TestMapRestartPolicy tests restart policy mapping
func TestMapRestartPolicy(t *testing.T) {
	tests := []struct {
		name     string
		policy   container.RestartPolicy
		expected string
	}{
		{
			name:     "always",
			policy:   container.RestartPolicy{Name: "always"},
			expected: "always",
		},
		{
			name:     "unless-stopped",
			policy:   container.RestartPolicy{Name: "unless-stopped"},
			expected: "unless-stopped",
		},
		{
			name:     "on-failure without max",
			policy:   container.RestartPolicy{Name: "on-failure"},
			expected: "on-failure",
		},
		{
			name:     "on-failure with max",
			policy:   container.RestartPolicy{Name: "on-failure", MaximumRetryCount: 5},
			expected: "on-failure:5",
		},
		{
			name:     "no restart",
			policy:   container.RestartPolicy{Name: "no"},
			expected: "no",
		},
		{
			name:     "empty policy",
			policy:   container.RestartPolicy{},
			expected: "no",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MapRestartPolicy(tt.policy)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

// TestMapNetworks tests network mapping
func TestMapNetworks(t *testing.T) {
	tests := []struct {
		name             string
		networks         map[string]*network.EndpointSettings
		expectedNetworks []string
		expectedAliases  map[string][]string
	}{
		{
			name: "single user network",
			networks: map[string]*network.EndpointSettings{
				"mynetwork": {Aliases: []string{"myservice"}},
			},
			expectedNetworks: []string{"mynetwork"},
			expectedAliases:  map[string][]string{"mynetwork": {"myservice"}},
		},
		{
			name: "skip default bridge",
			networks: map[string]*network.EndpointSettings{
				"bridge":    {},
				"mynetwork": {Aliases: []string{"app"}},
			},
			expectedNetworks: []string{"mynetwork"},
			expectedAliases:  map[string][]string{"mynetwork": {"app"}},
		},
		{
			name: "multiple networks",
			networks: map[string]*network.EndpointSettings{
				"frontend": {Aliases: []string{"web"}},
				"backend":  {Aliases: []string{"api"}},
			},
			expectedNetworks: []string{"frontend", "backend"},
			expectedAliases: map[string][]string{
				"frontend": {"web"},
				"backend":  {"api"},
			},
		},
		{
			name:             "empty networks",
			networks:         map[string]*network.EndpointSettings{},
			expectedNetworks: []string{},
			expectedAliases:  map[string][]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MapNetworks(tt.networks)

			if len(result) != len(tt.expectedNetworks) {
				t.Errorf("expected %d networks, got %d", len(tt.expectedNetworks), len(result))
			}

			for _, expectedNet := range tt.expectedNetworks {
				found := false
				for netName, netConfig := range result {
					if netName == expectedNet {
						found = true
						// Check aliases
						expectedAliases := tt.expectedAliases[expectedNet]
						if netConfig != nil && len(expectedAliases) > 0 {
							if len(netConfig.Aliases) != len(expectedAliases) {
								t.Errorf("network %s: expected %d aliases, got %d",
									netName, len(expectedAliases), len(netConfig.Aliases))
							}
						}
						break
					}
				}
				if !found {
					t.Errorf("expected network %s not found", expectedNet)
				}
			}
		})
	}
}

// TestMapHealthCheck tests health check mapping
func TestMapHealthCheck(t *testing.T) {
	tests := []struct {
		name        string
		healthCheck *container.HealthConfig
		expectNil   bool
		expectedCmd []string
	}{
		{
			name:      "nil health check",
			expectNil: true,
		},
		{
			name: "CMD health check",
			healthCheck: &container.HealthConfig{
				Test:     []string{"CMD", "curl", "-f", "http://localhost/health"},
				Interval: 30 * time.Second,
				Timeout:  10 * time.Second,
				Retries:  3,
			},
			expectNil:   false,
			expectedCmd: []string{"CMD", "curl", "-f", "http://localhost/health"},
		},
		{
			name: "CMD-SHELL health check",
			healthCheck: &container.HealthConfig{
				Test:     []string{"CMD-SHELL", "curl -f http://localhost/health || exit 1"},
				Interval: 30 * time.Second,
				Timeout:  10 * time.Second,
				Retries:  3,
			},
			expectNil:   false,
			expectedCmd: []string{"CMD-SHELL", "curl -f http://localhost/health || exit 1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MapHealthCheck(tt.healthCheck)

			if tt.expectNil {
				if result != nil {
					t.Errorf("expected nil, got %+v", result)
				}
				return
			}

			if result == nil {
				t.Fatal("expected non-nil result")
			}

			if len(result.Test) != len(tt.expectedCmd) {
				t.Errorf("expected %d test commands, got %d", len(tt.expectedCmd), len(result.Test))
			}

			for i, cmd := range tt.expectedCmd {
				if i < len(result.Test) && result.Test[i] != cmd {
					t.Errorf("test[%d]: expected %q, got %q", i, cmd, result.Test[i])
				}
			}
		})
	}
}

// TestIsSensitiveKey tests detection of sensitive environment variable names
func TestIsSensitiveKey(t *testing.T) {
	tests := []struct {
		key       string
		sensitive bool
	}{
		{"DB_PASSWORD", true},
		{"POSTGRES_PASSWORD", true},
		{"API_KEY", true},
		{"SECRET_TOKEN", true},
		{"AWS_SECRET_ACCESS_KEY", true},
		{"PRIVATE_KEY", true},
		{"AUTH_TOKEN", true},
		{"CREDENTIAL_FILE", true},
		{"APP_NAME", false},
		{"DB_HOST", false},
		{"PORT", false},
		{"LOG_LEVEL", false},
		{"TZ", false},
		{"password", true}, // Case insensitive
		{"Password", true},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			result := IsSensitiveKey(tt.key)
			if result != tt.sensitive {
				t.Errorf("key %q: expected sensitive=%v, got %v", tt.key, tt.sensitive, result)
			}
		})
	}
}

// TestSanitizeContainerName tests container name sanitization
func TestSanitizeContainerName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "remove leading slash",
			input:    "/nginx",
			expected: "nginx",
		},
		{
			name:     "no leading slash",
			input:    "nginx",
			expected: "nginx",
		},
		{
			name:     "multiple slashes",
			input:    "//nginx",
			expected: "/nginx", // Only removes one leading slash
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeContainerName(tt.input)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

// TestExportToCompose tests the full export to Compose YAML
func TestExportToCompose(t *testing.T) {
	// Create a mock container inspection response
	containerJSON := &container.InspectResponse{
		ContainerJSONBase: &container.ContainerJSONBase{
			Name: "/test-nginx",
			HostConfig: &container.HostConfig{
				PortBindings: nat.PortMap{
					"80/tcp": []nat.PortBinding{{HostIP: "0.0.0.0", HostPort: "8080"}},
				},
				RestartPolicy: container.RestartPolicy{Name: "unless-stopped"},
			},
		},
		Config: &container.Config{
			Image: "nginx:latest",
			Env:   []string{"NGINX_HOST=localhost", "NGINX_PORT=80"},
			Labels: map[string]string{
				"maintainer": "test",
			},
		},
		Mounts: []container.MountPoint{
			{
				Type:        mount.TypeVolume,
				Name:        "nginx-data",
				Destination: "/usr/share/nginx/html",
				RW:          true,
			},
		},
		NetworkSettings: &container.NetworkSettings{
			Networks: map[string]*network.EndpointSettings{
				"webnet": {Aliases: []string{"nginx"}},
			},
		},
	}

	options := ExportOptions{
		SanitizeSecrets: false,
		IncludeVolumes:  true,
		IncludeNetworks: true,
	}

	result, err := ExportToCompose(containerJSON, options)
	if err != nil {
		t.Fatalf("ExportToCompose failed: %v", err)
	}

	// Parse the YAML to verify structure
	var compose map[string]interface{}
	if err := yaml.Unmarshal([]byte(result.Content), &compose); err != nil {
		t.Fatalf("failed to parse generated YAML: %v", err)
	}

	// Check services exist
	services, ok := compose["services"].(map[string]interface{})
	if !ok {
		t.Fatal("services section not found or invalid")
	}

	// Check our service exists
	service, ok := services["test-nginx"].(map[string]interface{})
	if !ok {
		t.Fatal("test-nginx service not found")
	}

	// Check image
	if service["image"] != "nginx:latest" {
		t.Errorf("expected image nginx:latest, got %v", service["image"])
	}

	// Check container_name
	if service["container_name"] != "test-nginx" {
		t.Errorf("expected container_name test-nginx, got %v", service["container_name"])
	}

	// Check restart policy
	if service["restart"] != "unless-stopped" {
		t.Errorf("expected restart unless-stopped, got %v", service["restart"])
	}

	// Check filename
	if result.Filename != "test-nginx-compose.yml" {
		t.Errorf("expected filename test-nginx-compose.yml, got %s", result.Filename)
	}
}

// TestExportToComposeWithSensitiveData tests export with sensitive data sanitization
func TestExportToComposeWithSensitiveData(t *testing.T) {
	containerJSON := &container.InspectResponse{
		ContainerJSONBase: &container.ContainerJSONBase{
			Name: "/postgres",
			HostConfig: &container.HostConfig{
				RestartPolicy: container.RestartPolicy{Name: "always"},
			},
		},
		Config: &container.Config{
			Image: "postgres:15",
			Env:   []string{"POSTGRES_USER=admin", "POSTGRES_PASSWORD=supersecret"},
		},
		Mounts:          []container.MountPoint{},
		NetworkSettings: &container.NetworkSettings{Networks: map[string]*network.EndpointSettings{}},
	}

	options := ExportOptions{
		SanitizeSecrets: true,
		IncludeVolumes:  true,
		IncludeNetworks: true,
	}

	result, err := ExportToCompose(containerJSON, options)
	if err != nil {
		t.Fatalf("ExportToCompose failed: %v", err)
	}

	// Verify password is sanitized
	if strings.Contains(result.Content, "supersecret") {
		t.Error("sensitive password was not sanitized from output")
	}

	// Verify placeholder is present
	if !strings.Contains(result.Content, "${POSTGRES_PASSWORD:?Required}") {
		t.Error("expected password placeholder not found")
	}

	// Verify warning was generated
	foundWarning := false
	for _, w := range result.Warnings {
		if strings.Contains(w, "POSTGRES_PASSWORD") {
			foundWarning = true
			break
		}
	}
	if !foundWarning {
		t.Error("expected warning about POSTGRES_PASSWORD not found")
	}
}

// TestExportOptionsValidation tests export options validation
func TestExportOptionsValidation(t *testing.T) {
	tests := []struct {
		name    string
		options ExportOptions
		wantErr bool
	}{
		{
			name: "valid options",
			options: ExportOptions{
				SanitizeSecrets: true,
				IncludeVolumes:  true,
				IncludeNetworks: true,
			},
			wantErr: false,
		},
		{
			name:    "default options",
			options: ExportOptions{},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.options.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestCollectTopLevelVolumes tests extraction of top-level volumes from service volumes
func TestCollectTopLevelVolumes(t *testing.T) {
	volumes := []VolumeConfig{
		{Type: "volume", Source: "pgdata", Target: "/var/lib/postgresql/data"},
		{Type: "bind", Source: "/host/config", Target: "/config"},
		{Type: "volume", Source: "logs", Target: "/var/log"},
		{Type: "tmpfs", Source: "", Target: "/tmp"},
	}

	result := CollectTopLevelVolumes(volumes)

	// Should only include named volumes (type "volume")
	if len(result) != 2 {
		t.Errorf("expected 2 top-level volumes, got %d", len(result))
	}

	if _, ok := result["pgdata"]; !ok {
		t.Error("expected pgdata volume not found")
	}
	if _, ok := result["logs"]; !ok {
		t.Error("expected logs volume not found")
	}
}

// TestCollectTopLevelNetworks tests extraction of top-level networks
func TestCollectTopLevelNetworks(t *testing.T) {
	networks := map[string]*NetworkConfig{
		"frontend": {Aliases: []string{"web"}},
		"backend":  {Aliases: []string{"api"}},
	}

	result := CollectTopLevelNetworks(networks)

	if len(result) != 2 {
		t.Errorf("expected 2 top-level networks, got %d", len(result))
	}

	if _, ok := result["frontend"]; !ok {
		t.Error("expected frontend network not found")
	}
	if _, ok := result["backend"]; !ok {
		t.Error("expected backend network not found")
	}
}

// --- Batch Export Tests ---

// TestBatchExportToCompose tests exporting multiple containers to a single compose file
func TestBatchExportToCompose(t *testing.T) {
	// Create mock container inspection responses for a media server stack
	containers := []*container.InspectResponse{
		{
			ContainerJSONBase: &container.ContainerJSONBase{
				Name: "/plex",
				HostConfig: &container.HostConfig{
					PortBindings: nat.PortMap{
						"32400/tcp": []nat.PortBinding{{HostIP: "0.0.0.0", HostPort: "32400"}},
					},
					RestartPolicy: container.RestartPolicy{Name: "unless-stopped"},
				},
			},
			Config: &container.Config{
				Image: "plexinc/pms-docker:latest",
				Env:   []string{"PLEX_UID=1000", "PLEX_GID=1000", "TZ=America/New_York"},
			},
			Mounts: []container.MountPoint{
				{Type: mount.TypeVolume, Name: "plex-config", Destination: "/config", RW: true},
				{Type: mount.TypeBind, Source: "/mnt/media", Destination: "/data", RW: true},
			},
			NetworkSettings: &container.NetworkSettings{
				Networks: map[string]*network.EndpointSettings{
					"media-network": {Aliases: []string{"plex"}},
				},
			},
		},
		{
			ContainerJSONBase: &container.ContainerJSONBase{
				Name: "/sonarr",
				HostConfig: &container.HostConfig{
					PortBindings: nat.PortMap{
						"8989/tcp": []nat.PortBinding{{HostIP: "0.0.0.0", HostPort: "8989"}},
					},
					RestartPolicy: container.RestartPolicy{Name: "unless-stopped"},
				},
			},
			Config: &container.Config{
				Image: "linuxserver/sonarr:latest",
				Env:   []string{"PUID=1000", "PGID=1000", "TZ=America/New_York"},
			},
			Mounts: []container.MountPoint{
				{Type: mount.TypeVolume, Name: "sonarr-config", Destination: "/config", RW: true},
				{Type: mount.TypeBind, Source: "/mnt/media", Destination: "/data", RW: true},
			},
			NetworkSettings: &container.NetworkSettings{
				Networks: map[string]*network.EndpointSettings{
					"media-network": {Aliases: []string{"sonarr"}},
				},
			},
		},
		{
			ContainerJSONBase: &container.ContainerJSONBase{
				Name: "/radarr",
				HostConfig: &container.HostConfig{
					PortBindings: nat.PortMap{
						"7878/tcp": []nat.PortBinding{{HostIP: "0.0.0.0", HostPort: "7878"}},
					},
					RestartPolicy: container.RestartPolicy{Name: "unless-stopped"},
				},
			},
			Config: &container.Config{
				Image: "linuxserver/radarr:latest",
				Env:   []string{"PUID=1000", "PGID=1000", "TZ=America/New_York"},
			},
			Mounts: []container.MountPoint{
				{Type: mount.TypeVolume, Name: "radarr-config", Destination: "/config", RW: true},
				{Type: mount.TypeBind, Source: "/mnt/media", Destination: "/data", RW: true},
			},
			NetworkSettings: &container.NetworkSettings{
				Networks: map[string]*network.EndpointSettings{
					"media-network": {Aliases: []string{"radarr"}},
				},
			},
		},
	}

	options := ExportOptions{
		SanitizeSecrets: false,
		IncludeVolumes:  true,
		IncludeNetworks: true,
	}

	result, err := BatchExportToCompose(containers, options, "media-stack")
	if err != nil {
		t.Fatalf("BatchExportToCompose failed: %v", err)
	}

	// Parse the YAML to verify structure
	var compose ComposeFile
	if err := yaml.Unmarshal([]byte(result.Content), &compose); err != nil {
		t.Fatalf("failed to parse generated YAML: %v", err)
	}

	// Check all 3 services exist
	if len(compose.Services) != 3 {
		t.Errorf("expected 3 services, got %d", len(compose.Services))
	}

	// Check each service exists
	for _, name := range []string{"plex", "sonarr", "radarr"} {
		if _, ok := compose.Services[name]; !ok {
			t.Errorf("service %s not found", name)
		}
	}

	// Check filename
	if result.Filename != "media-stack-compose.yml" {
		t.Errorf("expected filename media-stack-compose.yml, got %s", result.Filename)
	}
}

// TestBatchExportNetworkDeduplication tests that shared networks are deduplicated
func TestBatchExportNetworkDeduplication(t *testing.T) {
	// Two containers sharing the same network
	containers := []*container.InspectResponse{
		{
			ContainerJSONBase: &container.ContainerJSONBase{
				Name:       "/app1",
				HostConfig: &container.HostConfig{RestartPolicy: container.RestartPolicy{Name: "no"}},
			},
			Config: &container.Config{Image: "nginx:latest"},
			Mounts: []container.MountPoint{},
			NetworkSettings: &container.NetworkSettings{
				Networks: map[string]*network.EndpointSettings{
					"shared-network": {Aliases: []string{"app1"}},
					"app1-only":      {Aliases: []string{"app1-internal"}},
				},
			},
		},
		{
			ContainerJSONBase: &container.ContainerJSONBase{
				Name:       "/app2",
				HostConfig: &container.HostConfig{RestartPolicy: container.RestartPolicy{Name: "no"}},
			},
			Config: &container.Config{Image: "nginx:latest"},
			Mounts: []container.MountPoint{},
			NetworkSettings: &container.NetworkSettings{
				Networks: map[string]*network.EndpointSettings{
					"shared-network": {Aliases: []string{"app2"}},
					"app2-only":      {Aliases: []string{"app2-internal"}},
				},
			},
		},
	}

	options := ExportOptions{
		SanitizeSecrets: false,
		IncludeVolumes:  true,
		IncludeNetworks: true,
	}

	result, err := BatchExportToCompose(containers, options, "test-stack")
	if err != nil {
		t.Fatalf("BatchExportToCompose failed: %v", err)
	}

	var compose ComposeFile
	if err := yaml.Unmarshal([]byte(result.Content), &compose); err != nil {
		t.Fatalf("failed to parse generated YAML: %v", err)
	}

	// Should have 3 unique networks: shared-network, app1-only, app2-only
	if len(compose.Networks) != 3 {
		t.Errorf("expected 3 deduplicated networks, got %d: %v", len(compose.Networks), compose.Networks)
	}

	// Verify each network exists exactly once
	expectedNetworks := []string{"shared-network", "app1-only", "app2-only"}
	for _, netName := range expectedNetworks {
		if _, ok := compose.Networks[netName]; !ok {
			t.Errorf("expected network %s not found in top-level networks", netName)
		}
	}
}

// TestBatchExportVolumeDeduplication tests that shared volumes are deduplicated
func TestBatchExportVolumeDeduplication(t *testing.T) {
	// Two containers sharing the same named volume
	containers := []*container.InspectResponse{
		{
			ContainerJSONBase: &container.ContainerJSONBase{
				Name:       "/app1",
				HostConfig: &container.HostConfig{RestartPolicy: container.RestartPolicy{Name: "no"}},
			},
			Config: &container.Config{Image: "nginx:latest"},
			Mounts: []container.MountPoint{
				{Type: mount.TypeVolume, Name: "shared-data", Destination: "/data", RW: true},
				{Type: mount.TypeVolume, Name: "app1-config", Destination: "/config", RW: true},
			},
			NetworkSettings: &container.NetworkSettings{Networks: map[string]*network.EndpointSettings{}},
		},
		{
			ContainerJSONBase: &container.ContainerJSONBase{
				Name:       "/app2",
				HostConfig: &container.HostConfig{RestartPolicy: container.RestartPolicy{Name: "no"}},
			},
			Config: &container.Config{Image: "nginx:latest"},
			Mounts: []container.MountPoint{
				{Type: mount.TypeVolume, Name: "shared-data", Destination: "/data", RW: true},
				{Type: mount.TypeVolume, Name: "app2-config", Destination: "/config", RW: true},
			},
			NetworkSettings: &container.NetworkSettings{Networks: map[string]*network.EndpointSettings{}},
		},
	}

	options := ExportOptions{
		SanitizeSecrets: false,
		IncludeVolumes:  true,
		IncludeNetworks: true,
	}

	result, err := BatchExportToCompose(containers, options, "test-stack")
	if err != nil {
		t.Fatalf("BatchExportToCompose failed: %v", err)
	}

	var compose ComposeFile
	if err := yaml.Unmarshal([]byte(result.Content), &compose); err != nil {
		t.Fatalf("failed to parse generated YAML: %v", err)
	}

	// Should have 3 unique volumes: shared-data, app1-config, app2-config
	if len(compose.Volumes) != 3 {
		t.Errorf("expected 3 deduplicated volumes, got %d: %v", len(compose.Volumes), compose.Volumes)
	}

	// Verify each volume exists exactly once
	expectedVolumes := []string{"shared-data", "app1-config", "app2-config"}
	for _, volName := range expectedVolumes {
		if _, ok := compose.Volumes[volName]; !ok {
			t.Errorf("expected volume %s not found in top-level volumes", volName)
		}
	}
}

// TestBatchExportWithSensitiveData tests batch export with sensitive data sanitization
func TestBatchExportWithSensitiveData(t *testing.T) {
	containers := []*container.InspectResponse{
		{
			ContainerJSONBase: &container.ContainerJSONBase{
				Name:       "/postgres",
				HostConfig: &container.HostConfig{RestartPolicy: container.RestartPolicy{Name: "always"}},
			},
			Config: &container.Config{
				Image: "postgres:15",
				Env:   []string{"POSTGRES_USER=admin", "POSTGRES_PASSWORD=dbsecret123"},
			},
			Mounts:          []container.MountPoint{},
			NetworkSettings: &container.NetworkSettings{Networks: map[string]*network.EndpointSettings{}},
		},
		{
			ContainerJSONBase: &container.ContainerJSONBase{
				Name:       "/redis",
				HostConfig: &container.HostConfig{RestartPolicy: container.RestartPolicy{Name: "always"}},
			},
			Config: &container.Config{
				Image: "redis:7",
				Env:   []string{"REDIS_PASSWORD=redissecret456"},
			},
			Mounts:          []container.MountPoint{},
			NetworkSettings: &container.NetworkSettings{Networks: map[string]*network.EndpointSettings{}},
		},
	}

	options := ExportOptions{
		SanitizeSecrets: true,
		IncludeVolumes:  true,
		IncludeNetworks: true,
	}

	result, err := BatchExportToCompose(containers, options, "db-stack")
	if err != nil {
		t.Fatalf("BatchExportToCompose failed: %v", err)
	}

	// Verify passwords are sanitized
	if strings.Contains(result.Content, "dbsecret123") {
		t.Error("postgres password was not sanitized")
	}
	if strings.Contains(result.Content, "redissecret456") {
		t.Error("redis password was not sanitized")
	}

	// Verify placeholders are present
	if !strings.Contains(result.Content, "${POSTGRES_PASSWORD:?Required}") {
		t.Error("expected POSTGRES_PASSWORD placeholder not found")
	}
	if !strings.Contains(result.Content, "${REDIS_PASSWORD:?Required}") {
		t.Error("expected REDIS_PASSWORD placeholder not found")
	}

	// Verify warnings were generated for both
	foundPostgres := false
	foundRedis := false
	for _, w := range result.Warnings {
		if strings.Contains(w, "POSTGRES_PASSWORD") {
			foundPostgres = true
		}
		if strings.Contains(w, "REDIS_PASSWORD") {
			foundRedis = true
		}
	}
	if !foundPostgres {
		t.Error("expected warning about POSTGRES_PASSWORD not found")
	}
	if !foundRedis {
		t.Error("expected warning about REDIS_PASSWORD not found")
	}
}

// TestBatchExportEmptyList tests batch export with empty container list
func TestBatchExportEmptyList(t *testing.T) {
	containers := []*container.InspectResponse{}
	options := ExportOptions{
		SanitizeSecrets: false,
		IncludeVolumes:  true,
		IncludeNetworks: true,
	}

	_, err := BatchExportToCompose(containers, options, "empty-stack")
	if err == nil {
		t.Error("expected error for empty container list, got nil")
	}
}

// TestBatchExportSingleContainer tests batch export with single container (should work like single export)
func TestBatchExportSingleContainer(t *testing.T) {
	containers := []*container.InspectResponse{
		{
			ContainerJSONBase: &container.ContainerJSONBase{
				Name:       "/nginx",
				HostConfig: &container.HostConfig{RestartPolicy: container.RestartPolicy{Name: "always"}},
			},
			Config: &container.Config{Image: "nginx:latest"},
			Mounts: []container.MountPoint{
				{Type: mount.TypeVolume, Name: "nginx-data", Destination: "/usr/share/nginx/html", RW: true},
			},
			NetworkSettings: &container.NetworkSettings{Networks: map[string]*network.EndpointSettings{}},
		},
	}

	options := ExportOptions{
		SanitizeSecrets: false,
		IncludeVolumes:  true,
		IncludeNetworks: true,
	}

	result, err := BatchExportToCompose(containers, options, "single-stack")
	if err != nil {
		t.Fatalf("BatchExportToCompose failed: %v", err)
	}

	var compose ComposeFile
	if err := yaml.Unmarshal([]byte(result.Content), &compose); err != nil {
		t.Fatalf("failed to parse generated YAML: %v", err)
	}

	if len(compose.Services) != 1 {
		t.Errorf("expected 1 service, got %d", len(compose.Services))
	}

	if _, ok := compose.Services["nginx"]; !ok {
		t.Error("nginx service not found")
	}
}

// TestBatchExportContainerNameCollision tests handling of duplicate container names
func TestBatchExportContainerNameCollision(t *testing.T) {
	// Two containers with the same name (shouldn't happen in practice, but test the handling)
	containers := []*container.InspectResponse{
		{
			ContainerJSONBase: &container.ContainerJSONBase{
				Name:       "/app",
				HostConfig: &container.HostConfig{RestartPolicy: container.RestartPolicy{Name: "no"}},
			},
			Config:          &container.Config{Image: "nginx:latest"},
			Mounts:          []container.MountPoint{},
			NetworkSettings: &container.NetworkSettings{Networks: map[string]*network.EndpointSettings{}},
		},
		{
			ContainerJSONBase: &container.ContainerJSONBase{
				Name:       "/app",
				HostConfig: &container.HostConfig{RestartPolicy: container.RestartPolicy{Name: "no"}},
			},
			Config:          &container.Config{Image: "redis:latest"},
			Mounts:          []container.MountPoint{},
			NetworkSettings: &container.NetworkSettings{Networks: map[string]*network.EndpointSettings{}},
		},
	}

	options := ExportOptions{
		SanitizeSecrets: false,
		IncludeVolumes:  true,
		IncludeNetworks: true,
	}

	result, err := BatchExportToCompose(containers, options, "collision-stack")
	if err != nil {
		t.Fatalf("BatchExportToCompose failed: %v", err)
	}

	var compose ComposeFile
	if err := yaml.Unmarshal([]byte(result.Content), &compose); err != nil {
		t.Fatalf("failed to parse generated YAML: %v", err)
	}

	// Should have 2 services with unique names (second one should be renamed)
	if len(compose.Services) != 2 {
		t.Errorf("expected 2 services (with collision handling), got %d", len(compose.Services))
	}
}

// --- Env File Generation Tests ---

// TestGenerateEnvFile tests generation of .env file from sensitive variables
func TestGenerateEnvFile(t *testing.T) {
	sensitiveVars := map[string]string{
		"DB_PASSWORD":     "supersecret123",
		"API_KEY":         "key-abc-123",
		"POSTGRES_PASSWD": "dbpass",
	}

	result := GenerateEnvFile(sensitiveVars)

	// Check header comment
	if !strings.Contains(result, "# Environment variables for Docker Compose") {
		t.Error("expected header comment not found")
	}

	// Check each variable is present with CHANGE_ME placeholder
	for key := range sensitiveVars {
		expectedLine := key + "=CHANGE_ME"
		if !strings.Contains(result, expectedLine) {
			t.Errorf("expected line %q not found in output", expectedLine)
		}
	}

	// Check warning comment
	if !strings.Contains(result, "IMPORTANT") {
		t.Error("expected warning comment not found")
	}
}

// TestGenerateEnvFileEmpty tests generation with no sensitive variables
func TestGenerateEnvFileEmpty(t *testing.T) {
	sensitiveVars := map[string]string{}

	result := GenerateEnvFile(sensitiveVars)

	// Should return empty string when no sensitive vars
	if result != "" {
		t.Errorf("expected empty string for no sensitive vars, got %q", result)
	}
}

// TestCollectSensitiveVars tests extraction of sensitive variables from environment
func TestCollectSensitiveVars(t *testing.T) {
	env := []string{
		"DB_HOST=localhost",
		"DB_PORT=5432",
		"DB_PASSWORD=secret123",
		"API_KEY=abc123",
		"TZ=America/New_York",
		"SECRET_TOKEN=token456",
	}

	result := CollectSensitiveVars(env)

	// Should only have 3 sensitive vars
	if len(result) != 3 {
		t.Errorf("expected 3 sensitive vars, got %d: %v", len(result), result)
	}

	// Check expected vars are present with original values
	expected := map[string]string{
		"DB_PASSWORD":  "secret123",
		"API_KEY":      "abc123",
		"SECRET_TOKEN": "token456",
	}

	for key, val := range expected {
		if result[key] != val {
			t.Errorf("expected %s=%s, got %s=%s", key, val, key, result[key])
		}
	}

	// Check non-sensitive vars are NOT present
	if _, ok := result["DB_HOST"]; ok {
		t.Error("DB_HOST should not be in sensitive vars")
	}
}

// TestExportToComposeWithEnvFile tests single container export with env file generation
func TestExportToComposeWithEnvFile(t *testing.T) {
	containerJSON := &container.InspectResponse{
		ContainerJSONBase: &container.ContainerJSONBase{
			Name: "/postgres",
			HostConfig: &container.HostConfig{
				RestartPolicy: container.RestartPolicy{Name: "always"},
			},
		},
		Config: &container.Config{
			Image: "postgres:15",
			Env:   []string{"POSTGRES_USER=admin", "POSTGRES_PASSWORD=supersecret", "POSTGRES_DB=myapp"},
		},
		Mounts:          []container.MountPoint{},
		NetworkSettings: &container.NetworkSettings{Networks: map[string]*network.EndpointSettings{}},
	}

	options := ExportOptions{
		SanitizeSecrets: true,
		IncludeVolumes:  true,
		IncludeNetworks: true,
	}

	result, err := ExportToComposeWithEnv(containerJSON, options)
	if err != nil {
		t.Fatalf("ExportToComposeWithEnv failed: %v", err)
	}

	// Compose content should have variable reference
	if !strings.Contains(result.Content, "${POSTGRES_PASSWORD}") {
		t.Error("compose should reference POSTGRES_PASSWORD as variable")
	}

	// Should NOT contain the actual secret
	if strings.Contains(result.Content, "supersecret") {
		t.Error("compose should not contain actual secret value")
	}

	// Env file should be generated
	if result.EnvContent == "" {
		t.Error("expected env file content to be generated")
	}

	// Env file should have CHANGE_ME placeholder
	if !strings.Contains(result.EnvContent, "POSTGRES_PASSWORD=CHANGE_ME") {
		t.Error("env file should have CHANGE_ME placeholder for password")
	}

	// Env filename should be set
	if result.EnvFilename != ".env.example" {
		t.Errorf("expected env filename .env.example, got %s", result.EnvFilename)
	}
}

// TestBatchExportWithEnvFile tests batch export with env file generation
func TestBatchExportWithEnvFile(t *testing.T) {
	containers := []*container.InspectResponse{
		{
			ContainerJSONBase: &container.ContainerJSONBase{
				Name:       "/postgres",
				HostConfig: &container.HostConfig{RestartPolicy: container.RestartPolicy{Name: "always"}},
			},
			Config: &container.Config{
				Image: "postgres:15",
				Env:   []string{"POSTGRES_USER=admin", "POSTGRES_PASSWORD=dbsecret123"},
			},
			Mounts:          []container.MountPoint{},
			NetworkSettings: &container.NetworkSettings{Networks: map[string]*network.EndpointSettings{}},
		},
		{
			ContainerJSONBase: &container.ContainerJSONBase{
				Name:       "/redis",
				HostConfig: &container.HostConfig{RestartPolicy: container.RestartPolicy{Name: "always"}},
			},
			Config: &container.Config{
				Image: "redis:7",
				Env:   []string{"REDIS_PASSWORD=redissecret456"},
			},
			Mounts:          []container.MountPoint{},
			NetworkSettings: &container.NetworkSettings{Networks: map[string]*network.EndpointSettings{}},
		},
	}

	options := ExportOptions{
		SanitizeSecrets: true,
		IncludeVolumes:  true,
		IncludeNetworks: true,
	}

	result, err := BatchExportToComposeWithEnv(containers, options, "db-stack")
	if err != nil {
		t.Fatalf("BatchExportToComposeWithEnv failed: %v", err)
	}

	// Verify passwords are NOT in compose
	if strings.Contains(result.Content, "dbsecret123") {
		t.Error("postgres password should not be in compose content")
	}
	if strings.Contains(result.Content, "redissecret456") {
		t.Error("redis password should not be in compose content")
	}

	// Verify variable references are in compose
	if !strings.Contains(result.Content, "${POSTGRES_PASSWORD}") {
		t.Error("compose should reference POSTGRES_PASSWORD as variable")
	}
	if !strings.Contains(result.Content, "${REDIS_PASSWORD}") {
		t.Error("compose should reference REDIS_PASSWORD as variable")
	}

	// Verify env file has both passwords
	if !strings.Contains(result.EnvContent, "POSTGRES_PASSWORD=CHANGE_ME") {
		t.Error("env file should have POSTGRES_PASSWORD placeholder")
	}
	if !strings.Contains(result.EnvContent, "REDIS_PASSWORD=CHANGE_ME") {
		t.Error("env file should have REDIS_PASSWORD placeholder")
	}
}

// TestExportWithEnvFileNoSecrets tests export when there are no sensitive variables
func TestExportWithEnvFileNoSecrets(t *testing.T) {
	containerJSON := &container.InspectResponse{
		ContainerJSONBase: &container.ContainerJSONBase{
			Name: "/nginx",
			HostConfig: &container.HostConfig{
				RestartPolicy: container.RestartPolicy{Name: "always"},
			},
		},
		Config: &container.Config{
			Image: "nginx:latest",
			Env:   []string{"NGINX_HOST=localhost", "NGINX_PORT=80"},
		},
		Mounts:          []container.MountPoint{},
		NetworkSettings: &container.NetworkSettings{Networks: map[string]*network.EndpointSettings{}},
	}

	options := ExportOptions{
		SanitizeSecrets: true,
		IncludeVolumes:  true,
		IncludeNetworks: true,
	}

	result, err := ExportToComposeWithEnv(containerJSON, options)
	if err != nil {
		t.Fatalf("ExportToComposeWithEnv failed: %v", err)
	}

	// No sensitive vars, so no env file
	if result.EnvContent != "" {
		t.Errorf("expected no env content for non-sensitive vars, got %q", result.EnvContent)
	}

	// Env filename should be empty
	if result.EnvFilename != "" {
		t.Errorf("expected no env filename, got %s", result.EnvFilename)
	}
}

// --- Zip Bundle Tests ---

// TestCreateZipBundle tests creating a ZIP with compose and env files
func TestCreateZipBundle(t *testing.T) {
	result := &ExportResult{
		Format:      "compose",
		Content:     "services:\n  app:\n    image: nginx\n",
		Filename:    "test-compose.yml",
		EnvContent:  "DB_PASSWORD=CHANGE_ME\n",
		EnvFilename: ".env.example",
	}

	zipData, zipFilename, err := CreateZipBundle(result, "test-stack")
	if err != nil {
		t.Fatalf("CreateZipBundle failed: %v", err)
	}

	// Check filename
	if zipFilename != "test-stack.zip" {
		t.Errorf("expected filename 'test-stack.zip', got %s", zipFilename)
	}

	// Check ZIP is not empty
	if len(zipData) == 0 {
		t.Error("expected non-empty ZIP data")
	}

	// Verify ZIP contents
	zipReader, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		t.Fatalf("failed to read ZIP: %v", err)
	}

	// Should have 2 files
	if len(zipReader.File) != 2 {
		t.Errorf("expected 2 files in ZIP, got %d", len(zipReader.File))
	}

	// Check file names
	foundCompose := false
	foundEnv := false
	for _, f := range zipReader.File {
		if f.Name == "test-compose.yml" {
			foundCompose = true
		}
		if f.Name == ".env.example" {
			foundEnv = true
		}
	}

	if !foundCompose {
		t.Error("compose file not found in ZIP")
	}
	if !foundEnv {
		t.Error("env file not found in ZIP")
	}
}

// TestCreateZipBundleNoEnvFile tests ZIP creation without env file
func TestCreateZipBundleNoEnvFile(t *testing.T) {
	result := &ExportResult{
		Format:   "compose",
		Content:  "services:\n  app:\n    image: nginx\n",
		Filename: "nginx-compose.yml",
		// No env file
	}

	zipData, zipFilename, err := CreateZipBundle(result, "nginx-stack")
	if err != nil {
		t.Fatalf("CreateZipBundle failed: %v", err)
	}

	// Check filename
	if zipFilename != "nginx-stack.zip" {
		t.Errorf("expected filename 'nginx-stack.zip', got %s", zipFilename)
	}

	// Verify ZIP has only 1 file
	zipReader, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		t.Fatalf("failed to read ZIP: %v", err)
	}

	if len(zipReader.File) != 1 {
		t.Errorf("expected 1 file in ZIP, got %d", len(zipReader.File))
	}

	if zipReader.File[0].Name != "nginx-compose.yml" {
		t.Errorf("expected compose file, got %s", zipReader.File[0].Name)
	}
}

// TestCreateZipBundleVerifyContents tests that ZIP file contents are correct
func TestCreateZipBundleVerifyContents(t *testing.T) {
	composeContent := "services:\n  db:\n    image: postgres:15\n    environment:\n      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD}\n"
	envContent := "# Environment variables\nPOSTGRES_PASSWORD=CHANGE_ME\n"

	result := &ExportResult{
		Format:      "compose",
		Content:     composeContent,
		Filename:    "db-compose.yml",
		EnvContent:  envContent,
		EnvFilename: ".env.example",
	}

	zipData, _, err := CreateZipBundle(result, "db-stack")
	if err != nil {
		t.Fatalf("CreateZipBundle failed: %v", err)
	}

	zipReader, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		t.Fatalf("failed to read ZIP: %v", err)
	}

	for _, f := range zipReader.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("failed to open file %s: %v", f.Name, err)
		}

		content, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatalf("failed to read file %s: %v", f.Name, err)
		}

		switch f.Name {
		case "db-compose.yml":
			if string(content) != composeContent {
				t.Errorf("compose content mismatch:\nexpected: %q\ngot: %q", composeContent, string(content))
			}
		case ".env.example":
			if string(content) != envContent {
				t.Errorf("env content mismatch:\nexpected: %q\ngot: %q", envContent, string(content))
			}
		default:
			t.Errorf("unexpected file in ZIP: %s", f.Name)
		}
	}
}
