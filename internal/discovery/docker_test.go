package discovery

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/network"
)

func TestDockerDiscoveryWorkerName(t *testing.T) {
	store := &mockProposalStore{}
	bus := &mockEventBus{}
	dm := NewDiscoveryManager(store, bus, nil)

	worker, err := NewDockerDiscoveryWorker(dm, "/var/run/docker.sock", 30*time.Second)
	if err != nil {
		t.Skipf("Skipping test - Docker not available: %v", err)
	}
	defer worker.Stop()

	if worker.Name() != "docker" {
		t.Errorf("Expected worker name 'docker', got %s", worker.Name())
	}
}

func TestDockerDiscoveryWorkerCreation(t *testing.T) {
	store := &mockProposalStore{}
	bus := &mockEventBus{}
	dm := NewDiscoveryManager(store, bus, nil)

	socketPath := "/var/run/docker.sock"
	interval := 30 * time.Second

	worker, err := NewDockerDiscoveryWorker(dm, socketPath, interval)
	if err != nil {
		t.Skipf("Skipping test - Docker not available: %v", err)
	}
	defer worker.Stop()

	if worker == nil {
		t.Fatal("Expected non-nil worker")
	}
	if worker.dm != dm {
		t.Error("Discovery manager not set correctly")
	}
	if worker.pollInterval != interval {
		t.Errorf("Expected poll interval %v, got %v", interval, worker.pollInterval)
	}
}

func TestDockerDiscoveryWorkerCreationFailure(t *testing.T) {
	store := &mockProposalStore{}
	bus := &mockEventBus{}
	dm := NewDiscoveryManager(store, bus, nil)

	// Try to create worker with invalid socket path
	worker, err := NewDockerDiscoveryWorker(dm, "invalid://socket", 30*time.Second)

	// We expect this to succeed but potentially fail when actually used
	// The Docker client creation doesn't validate the socket path
	if err == nil && worker != nil {
		worker.Stop()
	}
}

func TestDockerDiscoveryWorkerStop(t *testing.T) {
	store := &mockProposalStore{}
	bus := &mockEventBus{}
	dm := NewDiscoveryManager(store, bus, nil)

	worker, err := NewDockerDiscoveryWorker(dm, "/var/run/docker.sock", 30*time.Second)
	if err != nil {
		t.Skipf("Skipping test - Docker not available: %v", err)
	}

	err = worker.Stop()
	if err != nil {
		t.Errorf("Expected no error on stop, got %v", err)
	}
}

func TestDockerGetLabel(t *testing.T) {
	store := &mockProposalStore{}
	bus := &mockEventBus{}
	dm := NewDiscoveryManager(store, bus, nil)
	worker, err := NewDockerDiscoveryWorker(dm, "/var/run/docker.sock", 30*time.Second)
	if err != nil {
		t.Skipf("Skipping test - Docker not available: %v", err)
	}
	defer worker.Stop()

	container := &types.Container{
		Labels: map[string]string{
			"nekzus.app.name": "Test App",
			"nekzus.app.id":   "testapp",
		},
	}

	// Test existing label
	name := worker.getLabel(container, "nekzus.app.name", "default")
	if name != "Test App" {
		t.Errorf("Expected 'Test App', got %s", name)
	}

	// Test non-existing label with default
	icon := worker.getLabel(container, "nekzus.app.icon", "default.png")
	if icon != "default.png" {
		t.Errorf("Expected default 'default.png', got %s", icon)
	}

	// Test empty string label value
	container.Labels["nekzus.app.icon"] = ""
	icon = worker.getLabel(container, "nekzus.app.icon", "default.png")
	if icon != "default.png" {
		t.Errorf("Expected default for empty label 'default.png', got %s", icon)
	}
}

func TestDockerGetTags(t *testing.T) {
	store := &mockProposalStore{}
	bus := &mockEventBus{}
	dm := NewDiscoveryManager(store, bus, nil)
	worker, err := NewDockerDiscoveryWorker(dm, "/var/run/docker.sock", 30*time.Second)
	if err != nil {
		t.Skipf("Skipping test - Docker not available: %v", err)
	}
	defer worker.Stop()

	tests := []struct {
		name      string
		container *types.Container
		wantTags  []string
	}{
		{
			name: "explicit tags from labels",
			container: &types.Container{
				Labels: map[string]string{
					"nekzus.app.tags": "docker,web,app",
				},
			},
			wantTags: []string{"docker", "web", "app"},
		},
		{
			name: "auto-generated tags for nginx",
			container: &types.Container{
				Image:  "nginx:alpine",
				Labels: map[string]string{},
				Ports: []types.Port{
					{PrivatePort: 80, Type: "tcp"},
				},
			},
			wantTags: []string{"docker", "web", "http"},
		},
		{
			name: "auto-generated tags for postgres",
			container: &types.Container{
				Image:  "postgres:14",
				Labels: map[string]string{},
				Ports: []types.Port{
					{PrivatePort: 5432, Type: "tcp"},
				},
			},
			wantTags: []string{"docker", "database", "postgres"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tags := worker.getTags(tt.container)
			if len(tags) != len(tt.wantTags) {
				t.Errorf("Expected %d tags, got %d: %v", len(tt.wantTags), len(tags), tags)
			}
			for i, want := range tt.wantTags {
				if i >= len(tags) || tags[i] != want {
					t.Errorf("Tag mismatch at index %d: expected %s, got %v", i, want, tags)
				}
			}
		})
	}
}

func TestDockerGetScopes(t *testing.T) {
	store := &mockProposalStore{}
	bus := &mockEventBus{}
	dm := NewDiscoveryManager(store, bus, nil)
	worker, err := NewDockerDiscoveryWorker(dm, "/var/run/docker.sock", 30*time.Second)
	if err != nil {
		t.Skipf("Skipping test - Docker not available: %v", err)
	}
	defer worker.Stop()

	tests := []struct {
		name       string
		container  *types.Container
		wantScopes []string
	}{
		{
			name: "explicit scopes from labels",
			container: &types.Container{
				Labels: map[string]string{
					"nekzus.route.scopes": "read:app,write:app",
				},
			},
			wantScopes: []string{"read:app", "write:app"},
		},
		{
			name: "default scope from app ID",
			container: &types.Container{
				Labels: map[string]string{
					"nekzus.app.id": "myapp",
				},
			},
			wantScopes: []string{"access:myapp"},
		},
		{
			name: "no scopes when no labels",
			container: &types.Container{
				Labels: map[string]string{},
			},
			wantScopes: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scopes := worker.getScopes(tt.container)
			if len(scopes) != len(tt.wantScopes) {
				t.Errorf("Expected %d scopes, got %d: %v", len(tt.wantScopes), len(scopes), scopes)
			}
			for i, want := range tt.wantScopes {
				if i >= len(scopes) || scopes[i] != want {
					t.Errorf("Scope mismatch at index %d: expected %s, got %v", i, want, scopes)
				}
			}
		})
	}
}

func TestDockerCalculateConfidence(t *testing.T) {
	store := &mockProposalStore{}
	bus := &mockEventBus{}
	dm := NewDiscoveryManager(store, bus, nil)
	worker, err := NewDockerDiscoveryWorker(dm, "/var/run/docker.sock", 30*time.Second)
	if err != nil {
		t.Skipf("Skipping test - Docker not available: %v", err)
	}
	defer worker.Stop()

	tests := []struct {
		name      string
		container *types.Container
		wantConf  float64
	}{
		{
			name: "explicit enable label",
			container: &types.Container{
				Labels: map[string]string{
					"nekzus.enable": "true",
				},
			},
			wantConf: 0.95,
		},
		{
			name: "app ID label only",
			container: &types.Container{
				Labels: map[string]string{
					"nekzus.app.id": "myapp",
				},
			},
			wantConf: 0.85,
		},
		{
			name: "well-known image (nginx)",
			container: &types.Container{
				Image:  "nginx:alpine",
				Labels: map[string]string{},
			},
			wantConf: 0.7, // 0.5 base + 0.2 well-known
		},
		{
			name: "well-known image with common port",
			container: &types.Container{
				Image:  "grafana/grafana:latest",
				Labels: map[string]string{},
				Ports: []types.Port{
					{PrivatePort: 3000, Type: "tcp"},
				},
			},
			wantConf: 0.8, // 0.5 base + 0.2 well-known + 0.1 common port
		},
		{
			name: "unknown container",
			container: &types.Container{
				Image:  "busybox:latest",
				Labels: map[string]string{},
			},
			wantConf: 0.5, // base confidence only
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conf := worker.calculateConfidence(tt.container)
			// Use tolerance for floating point comparison
			tolerance := 0.001
			if conf < tt.wantConf-tolerance || conf > tt.wantConf+tolerance {
				t.Errorf("Expected confidence %f, got %f (diff: %f)", tt.wantConf, conf, conf-tt.wantConf)
			}
		})
	}
}

func TestDockerIsSystemContainer(t *testing.T) {
	store := &mockProposalStore{}
	bus := &mockEventBus{}
	dm := NewDiscoveryManager(store, bus, nil)
	worker, err := NewDockerDiscoveryWorker(dm, "/var/run/docker.sock", 30*time.Second)
	if err != nil {
		t.Skipf("Skipping test - Docker not available: %v", err)
	}
	defer worker.Stop()

	tests := []struct {
		name       string
		container  *types.Container
		wantSystem bool
	}{
		{
			name: "test container should not be skipped",
			container: &types.Container{
				Names:  []string{"/test-webapp"},
				Labels: map[string]string{"nekzus.test": "true"},
			},
			wantSystem: false,
		},
		{
			name: "nexus container should be skipped",
			container: &types.Container{
				Names:  []string{"/nekzus-test"},
				Labels: map[string]string{},
			},
			wantSystem: true,
		},
		{
			name: "caddy container should be skipped",
			container: &types.Container{
				Names:  []string{"/nekzus-caddy"},
				Labels: map[string]string{},
			},
			wantSystem: true,
		},
		{
			name: "traefik image should be skipped",
			container: &types.Container{
				Image:  "traefik:v2.10",
				Names:  []string{"/traefik"},
				Labels: map[string]string{},
			},
			wantSystem: true,
		},
		{
			name: "normal app should not be skipped",
			container: &types.Container{
				Image:  "nginx:alpine",
				Names:  []string{"/my-webapp"},
				Labels: map[string]string{},
			},
			wantSystem: false,
		},
		{
			name: "explicitly skipped container",
			container: &types.Container{
				Image: "redis:alpine",
				Names: []string{"/my-redis"},
				Labels: map[string]string{
					"nekzus.skip": "true",
				},
			},
			wantSystem: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isSystem := worker.isSystemContainer(tt.container)
			if isSystem != tt.wantSystem {
				t.Errorf("Expected isSystemContainer=%v, got %v", tt.wantSystem, isSystem)
			}
		})
	}
}

func TestDockerGetSecurityNotes(t *testing.T) {
	store := &mockProposalStore{}
	bus := &mockEventBus{}
	dm := NewDiscoveryManager(store, bus, nil)
	worker, err := NewDockerDiscoveryWorker(dm, "/var/run/docker.sock", 30*time.Second)
	if err != nil {
		t.Skipf("Skipping test - Docker not available: %v", err)
	}
	defer worker.Stop()

	tests := []struct {
		name      string
		scheme    string
		host      string
		wantNotes []string
	}{
		{
			name:   "http with private IP",
			scheme: "http",
			host:   "192.168.1.100",
			wantNotes: []string{
				"Discovered via Docker API",
				"JWT required",
				"Upstream uses HTTP (unencrypted)",
				"Private network address",
			},
		},
		{
			name:   "https with public host",
			scheme: "https",
			host:   "example.com",
			wantNotes: []string{
				"Discovered via Docker API",
				"JWT required",
			},
		},
		{
			name:   "http with localhost",
			scheme: "http",
			host:   "127.0.0.1",
			wantNotes: []string{
				"Discovered via Docker API",
				"JWT required",
				"Upstream uses HTTP (unencrypted)",
				"Private network address",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			notes := worker.getSecurityNotes(tt.scheme, tt.host)
			if len(notes) != len(tt.wantNotes) {
				t.Errorf("Expected %d notes, got %d: %v", len(tt.wantNotes), len(notes), notes)
			}
			for i, want := range tt.wantNotes {
				if i >= len(notes) || notes[i] != want {
					t.Errorf("Note mismatch at index %d: expected %s, got %v", i, want, notes)
				}
			}
		})
	}
}

func TestDockerGetServiceName(t *testing.T) {
	store := &mockProposalStore{}
	bus := &mockEventBus{}
	dm := NewDiscoveryManager(store, bus, nil)
	worker, err := NewDockerDiscoveryWorker(dm, "/var/run/docker.sock", 30*time.Second)
	if err != nil {
		t.Skipf("Skipping test - Docker not available: %v", err)
	}
	defer worker.Stop()

	tests := []struct {
		name      string
		container *types.Container
		wantName  string
	}{
		{
			name: "from label",
			container: &types.Container{
				Names: []string{"/container123"},
				Image: "busybox:latest",
				Labels: map[string]string{
					"nekzus.app.name": "My App",
				},
			},
			wantName: "My App",
		},
		{
			name: "from container name",
			container: &types.Container{
				Names:  []string{"/my-webapp"},
				Image:  "nginx:alpine",
				Labels: map[string]string{},
			},
			wantName: "my-webapp",
		},
		{
			name: "from image name",
			container: &types.Container{
				Names:  []string{},
				Image:  "grafana/grafana:latest",
				Labels: map[string]string{},
			},
			wantName: "grafana",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name := worker.getServiceName(tt.container)
			if name != tt.wantName {
				t.Errorf("Expected service name %s, got %s", tt.wantName, name)
			}
		})
	}
}

func TestDockerGetContainerName(t *testing.T) {
	store := &mockProposalStore{}
	bus := &mockEventBus{}
	dm := NewDiscoveryManager(store, bus, nil)
	worker, err := NewDockerDiscoveryWorker(dm, "/var/run/docker.sock", 30*time.Second)
	if err != nil {
		t.Skipf("Skipping test - Docker not available: %v", err)
	}
	defer worker.Stop()

	tests := []struct {
		name      string
		container *types.Container
		wantName  string
	}{
		{
			name: "container with name",
			container: &types.Container{
				Names: []string{"/test-webapp1"},
			},
			wantName: "test-webapp1",
		},
		{
			name: "container without name",
			container: &types.Container{
				Names: []string{},
			},
			wantName: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name := worker.getContainerName(tt.container)
			if name != tt.wantName {
				t.Errorf("Expected container name %s, got %s", tt.wantName, name)
			}
		})
	}
}

func TestDockerGuessScheme(t *testing.T) {
	store := &mockProposalStore{}
	bus := &mockEventBus{}
	dm := NewDiscoveryManager(store, bus, nil)
	worker, err := NewDockerDiscoveryWorker(dm, "/var/run/docker.sock", 30*time.Second)
	if err != nil {
		t.Skipf("Skipping test - Docker not available: %v", err)
	}
	defer worker.Stop()

	tests := []struct {
		port       int
		wantScheme string
	}{
		{80, "http"},
		{8080, "http"},
		{3000, "http"},
		{443, "https"},
		{8443, "https"},
	}

	for _, tt := range tests {
		scheme := worker.guessScheme(tt.port)
		if scheme != tt.wantScheme {
			t.Errorf("guessScheme(%d) = %s, want %s", tt.port, scheme, tt.wantScheme)
		}
	}
}

func TestDockerWorkerStartStop(t *testing.T) {
	store := &mockProposalStore{}
	bus := &mockEventBus{}
	dm := NewDiscoveryManager(store, bus, nil)

	worker, err := NewDockerDiscoveryWorker(dm, "/var/run/docker.sock", 100*time.Millisecond)
	if err != nil {
		t.Skipf("Skipping test - Docker not available: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Start worker in goroutine
	errCh := make(chan error, 1)
	go func() {
		errCh <- worker.Start(ctx)
	}()

	// Let it run briefly
	time.Sleep(50 * time.Millisecond)

	// Stop worker
	cancel()
	worker.Stop()

	// Wait for completion
	select {
	case err := <-errCh:
		if err != nil && err != context.Canceled {
			t.Errorf("Unexpected error: %v", err)
		}
	case <-time.After(1 * time.Second):
		t.Error("Worker did not stop in time")
	}
}

func TestDockerGetContainerIPs(t *testing.T) {
	store := &mockProposalStore{}
	bus := &mockEventBus{}
	dm := NewDiscoveryManager(store, bus, nil)
	worker, err := NewDockerDiscoveryWorker(dm, "/var/run/docker.sock", 30*time.Second)
	if err != nil {
		t.Skipf("Skipping test - Docker not available: %v", err)
	}
	defer worker.Stop()

	tests := []struct {
		name      string
		container *types.Container
		wantIPs   map[string]string
	}{
		{
			name: "single network",
			container: &types.Container{
				NetworkSettings: &types.SummaryNetworkSettings{
					Networks: map[string]*network.EndpointSettings{
						"bridge": {IPAddress: "172.17.0.2"},
					},
				},
			},
			wantIPs: map[string]string{"bridge": "172.17.0.2"},
		},
		{
			name: "multiple networks",
			container: &types.Container{
				NetworkSettings: &types.SummaryNetworkSettings{
					Networks: map[string]*network.EndpointSettings{
						"bridge":      {IPAddress: "172.17.0.2"},
						"app-network": {IPAddress: "172.20.0.5"},
						"web-tier":    {IPAddress: "172.21.0.3"},
					},
				},
			},
			wantIPs: map[string]string{
				"bridge":      "172.17.0.2",
				"app-network": "172.20.0.5",
				"web-tier":    "172.21.0.3",
			},
		},
		{
			name: "network with empty IP",
			container: &types.Container{
				NetworkSettings: &types.SummaryNetworkSettings{
					Networks: map[string]*network.EndpointSettings{
						"bridge":   {IPAddress: "172.17.0.2"},
						"none":     {IPAddress: ""},
						"web-tier": {IPAddress: "172.21.0.3"},
					},
				},
			},
			wantIPs: map[string]string{
				"bridge":   "172.17.0.2",
				"web-tier": "172.21.0.3",
			},
		},
		{
			name: "no networks",
			container: &types.Container{
				NetworkSettings: &types.SummaryNetworkSettings{
					Networks: map[string]*network.EndpointSettings{},
				},
			},
			wantIPs: map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ips := worker.getContainerIPs(tt.container)
			if len(ips) != len(tt.wantIPs) {
				t.Errorf("Expected %d IPs, got %d: %v", len(tt.wantIPs), len(ips), ips)
			}
			for network, wantIP := range tt.wantIPs {
				if gotIP, ok := ips[network]; !ok || gotIP != wantIP {
					t.Errorf("Network %s: expected IP %s, got %s", network, wantIP, gotIP)
				}
			}
		})
	}
}

func TestDockerSelfExclusion(t *testing.T) {
	store := &mockProposalStore{}
	bus := &mockEventBus{}
	dm := NewDiscoveryManager(store, bus, nil)
	worker, err := NewDockerDiscoveryWorker(dm, "/var/run/docker.sock", 30*time.Second)
	if err != nil {
		t.Skipf("Skipping test - Docker not available: %v", err)
	}
	defer worker.Stop()

	tests := []struct {
		name       string
		container  *types.Container
		selfID     string
		selfName   string
		selfHost   string
		shouldSkip bool
	}{
		{
			name: "container matches self container ID",
			container: &types.Container{
				ID:     "abc123def456",
				Names:  []string{"/my-app"},
				Labels: map[string]string{},
			},
			selfID:     "abc123def456",
			selfName:   "",
			selfHost:   "",
			shouldSkip: true,
		},
		{
			name: "container matches self container name",
			container: &types.Container{
				ID:     "different-id",
				Names:  []string{"/nekzus-1"},
				Labels: map[string]string{},
			},
			selfID:     "",
			selfName:   "nekzus-1",
			selfHost:   "",
			shouldSkip: true,
		},
		{
			name: "container matches self hostname",
			container: &types.Container{
				ID:     "different-id",
				Names:  []string{"/my-hostname"},
				Labels: map[string]string{},
			},
			selfID:     "",
			selfName:   "",
			selfHost:   "my-hostname",
			shouldSkip: true,
		},
		{
			name: "container does not match self",
			container: &types.Container{
				ID:     "different-id",
				Names:  []string{"/some-app"},
				Labels: map[string]string{},
			},
			selfID:     "abc123",
			selfName:   "nekzus-1",
			selfHost:   "my-hostname",
			shouldSkip: false,
		},
		{
			name: "nekzus container skipped by pattern (fallback)",
			container: &types.Container{
				ID:     "different-id",
				Names:  []string{"/nekzus-test"},
				Labels: map[string]string{},
			},
			selfID:     "",
			selfName:   "",
			selfHost:   "",
			shouldSkip: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set self-identification on worker
			worker.selfContainerID = tt.selfID
			worker.selfContainerName = tt.selfName
			worker.selfHostname = tt.selfHost

			// Check if container should be skipped
			shouldSkip := worker.isSelfContainer(tt.container)
			if shouldSkip != tt.shouldSkip {
				t.Errorf("Expected shouldSkip=%v, got %v", tt.shouldSkip, shouldSkip)
			}
		})
	}
}

func TestIsKnownNonHTTPPort(t *testing.T) {
	tests := []struct {
		port        int
		wantNonHTTP bool
	}{
		// Non-HTTP infrastructure ports (should return true)
		{22, true},    // SSH
		{21, true},    // FTP
		{23, true},    // Telnet
		{25, true},    // SMTP
		{53, true},    // DNS
		{110, true},   // POP3
		{143, true},   // IMAP
		{389, true},   // LDAP
		{636, true},   // LDAPS
		{3306, true},  // MySQL
		{5432, true},  // PostgreSQL
		{6379, true},  // Redis
		{27017, true}, // MongoDB
		{11211, true}, // Memcached
		{5672, true},  // RabbitMQ AMQP
		{1883, true},  // MQTT

		// HTTP ports (should return false - not known non-HTTP)
		{80, false},
		{443, false},
		{8080, false},
		{8443, false},
		{3000, false}, // Grafana, Node.js
		{5230, false}, // Memos
		{8384, false}, // Syncthing
		{9000, false}, // SonarQube
		{9090, false}, // Prometheus

		// Unknown ports - should return false (might be HTTP)
		{12345, false},
		{54321, false},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("port_%d", tt.port), func(t *testing.T) {
			got := isKnownNonHTTPPort(tt.port)
			if got != tt.wantNonHTTP {
				t.Errorf("isKnownNonHTTPPort(%d) = %v, want %v", tt.port, got, tt.wantNonHTTP)
			}
		})
	}
}

func TestDockerPortFiltering(t *testing.T) {
	// Create a minimal worker without Docker connection for unit testing
	worker := &DockerDiscoveryWorker{}

	tests := []struct {
		name          string
		ports         []types.Port
		labels        map[string]string
		probeHost     string // Empty means no probing possible
		wantPortCount int
		wantPorts     []int
	}{
		{
			name: "Primary port label - only specified port",
			ports: []types.Port{
				{PrivatePort: 22, PublicPort: 2222, Type: "tcp"},
				{PrivatePort: 3000, PublicPort: 3000, Type: "tcp"},
				{PrivatePort: 8080, PublicPort: 8080, Type: "tcp"},
			},
			labels: map[string]string{
				"nekzus.primary_port": "8080",
			},
			probeHost:     "",
			wantPortCount: 1,
			wantPorts:     []int{8080},
		},
		{
			name: "All ports label - discover everything",
			ports: []types.Port{
				{PrivatePort: 22, PublicPort: 2222, Type: "tcp"},
				{PrivatePort: 3000, PublicPort: 3000, Type: "tcp"},
			},
			labels: map[string]string{
				"nekzus.discover.all_ports": "true",
			},
			probeHost:     "",
			wantPortCount: 2,
			wantPorts:     []int{2222, 3000},
		},
		{
			name: "UDP port skipped even with all_ports",
			ports: []types.Port{
				{PrivatePort: 53, PublicPort: 53, Type: "udp"},   // DNS UDP - skip
				{PrivatePort: 80, PublicPort: 80, Type: "tcp"},   // HTTP - discover
				{PrivatePort: 443, PublicPort: 443, Type: "tcp"}, // HTTPS - discover
			},
			labels: map[string]string{
				"nekzus.discover.all_ports": "true",
			},
			probeHost:     "",
			wantPortCount: 2,
			wantPorts:     []int{80, 443},
		},
		{
			name: "Known non-HTTP ports skipped without probing",
			ports: []types.Port{
				{PrivatePort: 22, PublicPort: 2222, Type: "tcp"},   // SSH - skip
				{PrivatePort: 3306, PublicPort: 3306, Type: "tcp"}, // MySQL - skip
				{PrivatePort: 5432, PublicPort: 5432, Type: "tcp"}, // PostgreSQL - skip
				{PrivatePort: 6379, PublicPort: 6379, Type: "tcp"}, // Redis - skip
			},
			labels:        map[string]string{},
			probeHost:     "",
			wantPortCount: 0, // All are known non-HTTP
			wantPorts:     []int{},
		},
		{
			name: "No host to probe - unknown ports skipped",
			ports: []types.Port{
				{PrivatePort: 22, PublicPort: 2222, Type: "tcp"},   // SSH - known non-HTTP, skip
				{PrivatePort: 3000, PublicPort: 3000, Type: "tcp"}, // Unknown - skip (can't probe)
				{PrivatePort: 8080, PublicPort: 8080, Type: "tcp"}, // Unknown - skip (can't probe)
			},
			labels:        map[string]string{},
			probeHost:     "",
			wantPortCount: 0, // No host = can't probe unknown ports
			wantPorts:     []int{},
		},
		{
			name: "Primary port overrides known non-HTTP",
			ports: []types.Port{
				{PrivatePort: 22, PublicPort: 22, Type: "tcp"},
				{PrivatePort: 3000, PublicPort: 3000, Type: "tcp"},
			},
			labels: map[string]string{
				"nekzus.primary_port": "22", // Force SSH port
			},
			probeHost:     "",
			wantPortCount: 1,
			wantPorts:     []int{22},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filteredPorts := worker.filterPorts(tt.ports, tt.labels, tt.probeHost)

			if len(filteredPorts) != tt.wantPortCount {
				t.Errorf("Expected %d ports, got %d: %v", tt.wantPortCount, len(filteredPorts), filteredPorts)
			}

			// Check specific ports
			for _, wantPort := range tt.wantPorts {
				found := false
				for _, port := range filteredPorts {
					actualPort := int(port.PublicPort)
					if actualPort == 0 {
						actualPort = int(port.PrivatePort)
					}
					if actualPort == wantPort {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected port %d to be in filtered list, but it wasn't", wantPort)
				}
			}
		})
	}
}

func TestDockerFilterNetworks(t *testing.T) {
	store := &mockProposalStore{}
	bus := &mockEventBus{}
	dm := NewDiscoveryManager(store, bus, nil)

	tests := []struct {
		name            string
		networks        map[string]string
		includeNetworks []string
		excludeNetworks []string
		networkMode     string
		wantNetworks    map[string]string
	}{
		{
			name: "no filtering - all mode",
			networks: map[string]string{
				"bridge":      "172.17.0.2",
				"app-network": "172.20.0.5",
				"web-tier":    "172.21.0.3",
			},
			includeNetworks: nil,
			excludeNetworks: nil,
			networkMode:     "all",
			wantNetworks: map[string]string{
				"bridge":      "172.17.0.2",
				"app-network": "172.20.0.5",
				"web-tier":    "172.21.0.3",
			},
		},
		{
			name: "include specific networks",
			networks: map[string]string{
				"bridge":      "172.17.0.2",
				"app-network": "172.20.0.5",
				"web-tier":    "172.21.0.3",
			},
			includeNetworks: []string{"app-network", "web-tier"},
			excludeNetworks: nil,
			networkMode:     "all",
			wantNetworks: map[string]string{
				"app-network": "172.20.0.5",
				"web-tier":    "172.21.0.3",
			},
		},
		{
			name: "exclude specific networks",
			networks: map[string]string{
				"bridge":      "172.17.0.2",
				"app-network": "172.20.0.5",
				"web-tier":    "172.21.0.3",
			},
			includeNetworks: nil,
			excludeNetworks: []string{"bridge"},
			networkMode:     "all",
			wantNetworks: map[string]string{
				"app-network": "172.20.0.5",
				"web-tier":    "172.21.0.3",
			},
		},
		{
			name: "first mode - returns first network",
			networks: map[string]string{
				"bridge":      "172.17.0.2",
				"app-network": "172.20.0.5",
			},
			includeNetworks: nil,
			excludeNetworks: nil,
			networkMode:     "first",
			wantNetworks:    map[string]string{}, // Will have 1 network, exact one depends on iteration
		},
		{
			name: "preferred mode with priority",
			networks: map[string]string{
				"bridge":      "172.17.0.2",
				"app-network": "172.20.0.5",
				"web-tier":    "172.21.0.3",
			},
			includeNetworks: []string{"web-tier", "app-network"},
			excludeNetworks: nil,
			networkMode:     "preferred",
			wantNetworks: map[string]string{
				"web-tier": "172.21.0.3",
			},
		},
		{
			name: "include and exclude - include takes precedence",
			networks: map[string]string{
				"bridge":      "172.17.0.2",
				"app-network": "172.20.0.5",
				"web-tier":    "172.21.0.3",
			},
			includeNetworks: []string{"app-network", "web-tier"},
			excludeNetworks: []string{"web-tier"},
			networkMode:     "all",
			wantNetworks: map[string]string{
				"app-network": "172.20.0.5",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			worker, err := NewDockerDiscoveryWorker(dm, "/var/run/docker.sock", 30*time.Second)
			if err != nil {
				t.Skipf("Skipping test - Docker not available: %v", err)
			}
			defer worker.Stop()

			// Set network configuration
			worker.includeNetworks = tt.includeNetworks
			worker.excludeNetworks = tt.excludeNetworks
			worker.networkMode = tt.networkMode

			filtered := worker.filterNetworks(tt.networks)

			// For "first" mode, just check we got exactly 1 network
			if tt.networkMode == "first" {
				if len(filtered) != 1 {
					t.Errorf("Expected 1 network in 'first' mode, got %d", len(filtered))
				}
				return
			}

			if len(filtered) != len(tt.wantNetworks) {
				t.Errorf("Expected %d networks, got %d: %v", len(tt.wantNetworks), len(filtered), filtered)
			}
			for network, wantIP := range tt.wantNetworks {
				if gotIP, ok := filtered[network]; !ok || gotIP != wantIP {
					t.Errorf("Network %s: expected IP %s, got %s", network, wantIP, gotIP)
				}
			}
		})
	}
}
