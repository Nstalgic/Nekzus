package discovery

import (
	"testing"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/network"
)

// Label Validation Tests
// These tests verify that container labels are properly validated
// before processing to avoid invalid app IDs and route paths.

// TestIsValidAppID tests the app ID validation function.
func TestIsValidAppID(t *testing.T) {
	tests := []struct {
		name  string
		appID string
		want  bool
	}{
		// Valid app IDs
		{
			name:  "simple lowercase",
			appID: "grafana",
			want:  true,
		},
		{
			name:  "with dashes",
			appID: "uptime-kuma",
			want:  true,
		},
		{
			name:  "with underscores",
			appID: "my_app",
			want:  true,
		},
		{
			name:  "with numbers",
			appID: "app123",
			want:  true,
		},
		{
			name:  "mixed alphanumeric",
			appID: "my-app_v2",
			want:  true,
		},
		{
			name:  "uppercase letters",
			appID: "MyApp",
			want:  true,
		},
		{
			name:  "single character",
			appID: "a",
			want:  true,
		},
		{
			name:  "64 characters (max)",
			appID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			want:  true,
		},
		// Invalid app IDs
		{
			name:  "empty string",
			appID: "",
			want:  false,
		},
		{
			name:  "with spaces",
			appID: "my app",
			want:  false,
		},
		{
			name:  "with special characters (dots)",
			appID: "my.app",
			want:  false,
		},
		{
			name:  "with special characters (slash)",
			appID: "my/app",
			want:  false,
		},
		{
			name:  "with special characters (colon)",
			appID: "my:app",
			want:  false,
		},
		{
			name:  "with special characters (at)",
			appID: "my@app",
			want:  false,
		},
		{
			name:  "too long (65 characters)",
			appID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			want:  false,
		},
		{
			name:  "with percent sign",
			appID: "app%test",
			want:  false,
		},
		{
			name:  "with hash",
			appID: "app#1",
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isValidAppID(tt.appID)
			if got != tt.want {
				t.Errorf("isValidAppID(%q) = %v, want %v", tt.appID, got, tt.want)
			}
		})
	}
}

// TestValidateLabels_AppID tests validation of app.id label.
func TestValidateLabels_AppID(t *testing.T) {
	store := &mockProposalStore{}
	bus := &mockEventBus{}
	dm := NewDiscoveryManager(store, bus, nil)

	worker := &DockerDiscoveryWorker{
		dm:            dm,
		knownServices: make(map[string]bool),
		networkMode:   "all",
	}

	tests := []struct {
		name      string
		appID     string
		wantError bool
	}{
		{
			name:      "valid app ID",
			appID:     "grafana",
			wantError: false,
		},
		{
			name:      "valid app ID with dashes",
			appID:     "uptime-kuma",
			wantError: false,
		},
		{
			name:      "invalid app ID with spaces",
			appID:     "my app",
			wantError: true,
		},
		{
			name:      "invalid app ID with special chars",
			appID:     "my.app/v1",
			wantError: true,
		},
		{
			name:      "empty app ID (valid - uses default)",
			appID:     "",
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			container := &types.Container{
				ID:    "test123456789",
				Names: []string{"/test-container"},
				Labels: map[string]string{
					"nekzus.app.id": tt.appID,
				},
				NetworkSettings: &types.SummaryNetworkSettings{
					Networks: map[string]*network.EndpointSettings{
						"bridge": {IPAddress: "172.17.0.2"},
					},
				},
			}

			err := worker.validateLabels(container)
			if tt.wantError && err == nil {
				t.Errorf("validateLabels() expected error for app.id=%q, got nil", tt.appID)
			}
			if !tt.wantError && err != nil {
				t.Errorf("validateLabels() unexpected error for app.id=%q: %v", tt.appID, err)
			}
		})
	}
}

// TestValidateLabels_RoutePath tests validation of route.path label.
func TestValidateLabels_RoutePath(t *testing.T) {
	store := &mockProposalStore{}
	bus := &mockEventBus{}
	dm := NewDiscoveryManager(store, bus, nil)

	worker := &DockerDiscoveryWorker{
		dm:            dm,
		knownServices: make(map[string]bool),
		networkMode:   "all",
	}

	tests := []struct {
		name      string
		routePath string
		wantError bool
	}{
		{
			name:      "valid path starting with /",
			routePath: "/apps/grafana/",
			wantError: false,
		},
		{
			name:      "valid simple path",
			routePath: "/grafana",
			wantError: false,
		},
		{
			name:      "valid root path",
			routePath: "/",
			wantError: false,
		},
		{
			name:      "empty path (valid - uses default)",
			routePath: "",
			wantError: false,
		},
		{
			name:      "invalid path without leading slash",
			routePath: "apps/grafana/",
			wantError: true,
		},
		{
			name:      "invalid relative path",
			routePath: "grafana",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			container := &types.Container{
				ID:    "test123456789",
				Names: []string{"/test-container"},
				Labels: map[string]string{
					"nekzus.route.path": tt.routePath,
				},
				NetworkSettings: &types.SummaryNetworkSettings{
					Networks: map[string]*network.EndpointSettings{
						"bridge": {IPAddress: "172.17.0.2"},
					},
				},
			}

			err := worker.validateLabels(container)
			if tt.wantError && err == nil {
				t.Errorf("validateLabels() expected error for route.path=%q, got nil", tt.routePath)
			}
			if !tt.wantError && err != nil {
				t.Errorf("validateLabels() unexpected error for route.path=%q: %v", tt.routePath, err)
			}
		})
	}
}

// TestValidateLabels_CombinedValidation tests validation of multiple labels together.
func TestValidateLabels_CombinedValidation(t *testing.T) {
	store := &mockProposalStore{}
	bus := &mockEventBus{}
	dm := NewDiscoveryManager(store, bus, nil)

	worker := &DockerDiscoveryWorker{
		dm:            dm,
		knownServices: make(map[string]bool),
		networkMode:   "all",
	}

	tests := []struct {
		name      string
		labels    map[string]string
		wantError bool
	}{
		{
			name: "all valid labels",
			labels: map[string]string{
				"nekzus.app.id":     "grafana",
				"nekzus.route.path": "/apps/grafana/",
			},
			wantError: false,
		},
		{
			name: "invalid app ID with valid path",
			labels: map[string]string{
				"nekzus.app.id":     "my app",
				"nekzus.route.path": "/apps/grafana/",
			},
			wantError: true,
		},
		{
			name: "valid app ID with invalid path",
			labels: map[string]string{
				"nekzus.app.id":     "grafana",
				"nekzus.route.path": "apps/grafana/",
			},
			wantError: true,
		},
		{
			name:      "no labels (valid - defaults used)",
			labels:    map[string]string{},
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			container := &types.Container{
				ID:     "test123456789",
				Names:  []string{"/test-container"},
				Labels: tt.labels,
				NetworkSettings: &types.SummaryNetworkSettings{
					Networks: map[string]*network.EndpointSettings{
						"bridge": {IPAddress: "172.17.0.2"},
					},
				},
			}

			err := worker.validateLabels(container)
			if tt.wantError && err == nil {
				t.Errorf("validateLabels() expected error, got nil")
			}
			if !tt.wantError && err != nil {
				t.Errorf("validateLabels() unexpected error: %v", err)
			}
		})
	}
}
