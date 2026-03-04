package discovery

import (
	"testing"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/network"
)

// Self-Container Detection Tests
// These tests verify that self-container detection only matches on container ID
// and container name, NOT hostname (to avoid false positives).

// TestIsSelfContainer_ByContainerID tests that self-container detection works by container ID.
func TestIsSelfContainer_ByContainerID(t *testing.T) {
	worker := &DockerDiscoveryWorker{
		selfContainerID:   "abc123def456",
		selfContainerName: "nekzus",
		selfHostname:      "my-hostname",
	}

	tests := []struct {
		name        string
		containerID string
		want        bool
	}{
		{
			name:        "exact full ID match",
			containerID: "abc123def456",
			want:        true,
		},
		{
			name:        "self ID is prefix of container ID",
			containerID: "abc123def456789xyz",
			want:        true,
		},
		{
			name:        "container ID is prefix of self ID",
			containerID: "abc123def",
			want:        true,
		},
		{
			name:        "short ID comparison (12 chars)",
			containerID: "abc123def456other",
			want:        true,
		},
		{
			name:        "different container ID",
			containerID: "xyz789abc012",
			want:        false,
		},
		{
			name:        "empty container ID",
			containerID: "",
			want:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			container := &types.Container{
				ID:    tt.containerID,
				Names: []string{"/other-container"},
				NetworkSettings: &types.SummaryNetworkSettings{
					Networks: map[string]*network.EndpointSettings{
						"bridge": {IPAddress: "172.17.0.2"},
					},
				},
			}

			got := worker.isSelfContainer(container)
			if got != tt.want {
				t.Errorf("isSelfContainer() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestIsSelfContainer_ByContainerName tests that self-container detection works by container name.
func TestIsSelfContainer_ByContainerName(t *testing.T) {
	worker := &DockerDiscoveryWorker{
		selfContainerID:   "abc123def456",
		selfContainerName: "nekzus",
		selfHostname:      "my-hostname",
	}

	tests := []struct {
		name           string
		containerNames []string
		want           bool
	}{
		{
			name:           "exact name match (with leading slash)",
			containerNames: []string{"/nekzus"},
			want:           true,
		},
		{
			name:           "exact name match (without leading slash)",
			containerNames: []string{"nekzus"},
			want:           true,
		},
		{
			name:           "different container name",
			containerNames: []string{"/grafana"},
			want:           false,
		},
		{
			name:           "partial name match should NOT match (not a substring)",
			containerNames: []string{"/nekzus-test"},
			want:           false,
		},
		{
			name:           "multiple names, one matches",
			containerNames: []string{"/alias", "/nekzus"},
			want:           true,
		},
		{
			name:           "empty names",
			containerNames: []string{},
			want:           false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			container := &types.Container{
				ID:    "differentid123456",
				Names: tt.containerNames,
				NetworkSettings: &types.SummaryNetworkSettings{
					Networks: map[string]*network.EndpointSettings{
						"bridge": {IPAddress: "172.17.0.2"},
					},
				},
			}

			got := worker.isSelfContainer(container)
			if got != tt.want {
				t.Errorf("isSelfContainer() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestIsSelfContainer_NoHostnameMatching tests that hostname matching is REMOVED
// to avoid false positives when container names match the hostname.
func TestIsSelfContainer_NoHostnameMatching(t *testing.T) {
	worker := &DockerDiscoveryWorker{
		selfContainerID:   "abc123def456",
		selfContainerName: "nekzus",
		selfHostname:      "my-hostname",
	}

	tests := []struct {
		name           string
		containerID    string
		containerNames []string
		want           bool
		description    string
	}{
		{
			name:           "container name equals hostname - should NOT match",
			containerID:    "differentid123456",
			containerNames: []string{"/my-hostname"},
			want:           false,
			description:    "Container with name matching hostname should not be considered self (hostname matching removed)",
		},
		{
			name:           "container with hostname-like name - should NOT match",
			containerID:    "differentid123456",
			containerNames: []string{"/my-hostname-app"},
			want:           false,
			description:    "Container with hostname prefix should not match",
		},
		{
			name:           "real self-container by ID should still match",
			containerID:    "abc123def456",
			containerNames: []string{"/grafana"},
			want:           true,
			description:    "Container with matching ID should still be detected as self",
		},
		{
			name:           "real self-container by name should still match",
			containerID:    "differentid123456",
			containerNames: []string{"/nekzus"},
			want:           true,
			description:    "Container with matching name should still be detected as self",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			container := &types.Container{
				ID:    tt.containerID,
				Names: tt.containerNames,
				NetworkSettings: &types.SummaryNetworkSettings{
					Networks: map[string]*network.EndpointSettings{
						"bridge": {IPAddress: "172.17.0.2"},
					},
				},
			}

			got := worker.isSelfContainer(container)
			if got != tt.want {
				t.Errorf("isSelfContainer() = %v, want %v\n%s", got, tt.want, tt.description)
			}
		})
	}
}

// TestIsSelfContainer_EmptySelfIdentity tests behavior when self-identity is not set.
func TestIsSelfContainer_EmptySelfIdentity(t *testing.T) {
	worker := &DockerDiscoveryWorker{
		selfContainerID:   "",
		selfContainerName: "",
		selfHostname:      "",
	}

	container := &types.Container{
		ID:    "anycontainerid",
		Names: []string{"/some-container"},
		NetworkSettings: &types.SummaryNetworkSettings{
			Networks: map[string]*network.EndpointSettings{
				"bridge": {IPAddress: "172.17.0.2"},
			},
		},
	}

	got := worker.isSelfContainer(container)
	if got {
		t.Errorf("isSelfContainer() = true, want false when self-identity is empty")
	}
}
