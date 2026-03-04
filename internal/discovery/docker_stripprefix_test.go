package discovery

import (
	"testing"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/network"
	apptypes "github.com/nstalgic/nekzus/internal/types"
)

// TestDockerStripPrefixLabel verifies that the nekzus.route.strip_prefix and
// nekzus.route.rewrite_html labels are correctly read from container labels
// and set in the SuggestedRoute.
func TestDockerStripPrefixLabel(t *testing.T) {
	store := &mockProposalStore{}
	bus := &mockEventBus{}
	dm := NewDiscoveryManager(store, bus, nil)
	worker, err := NewDockerDiscoveryWorker(dm, "/var/run/docker.sock", 30*time.Second)
	if err != nil {
		t.Skipf("Skipping test - Docker not available: %v", err)
	}
	defer worker.Stop()

	tests := []struct {
		name            string
		container       *types.Container
		wantStripPrefix bool
		wantRewriteHTML bool
	}{
		{
			name: "strip_prefix explicitly set to true",
			container: &types.Container{
				ID:    "abc123",
				Names: []string{"/test-app-with-strip"},
				Image: "busybox:latest",
				State: "running",
				Ports: []types.Port{
					{PrivatePort: 8080, Type: "tcp"},
				},
				NetworkSettings: &types.SummaryNetworkSettings{
					Networks: map[string]*network.EndpointSettings{
						"bridge": {IPAddress: "172.17.0.2"},
					},
				},
				Labels: map[string]string{
					"nekzus.enable":             "true",
					"nekzus.app.id":             "test-app",
					"nekzus.app.name":           "Test App",
					"nekzus.route.path":         "/apps/test/",
					"nekzus.route.strip_prefix": "true",
					"nekzus.route.rewrite_html": "true",
				},
			},
			wantStripPrefix: true,
			wantRewriteHTML: true,
		},
		{
			name: "strip_prefix explicitly set to false",
			container: &types.Container{
				ID:    "def456",
				Names: []string{"/test-app-no-strip"},
				Image: "busybox:latest",
				State: "running",
				Ports: []types.Port{
					{PrivatePort: 8080, Type: "tcp"},
				},
				NetworkSettings: &types.SummaryNetworkSettings{
					Networks: map[string]*network.EndpointSettings{
						"bridge": {IPAddress: "172.17.0.3"},
					},
				},
				Labels: map[string]string{
					"nekzus.enable":             "true",
					"nekzus.app.id":             "test-app2",
					"nekzus.app.name":           "Test App 2",
					"nekzus.route.path":         "/apps/test2/",
					"nekzus.route.strip_prefix": "false",
				},
			},
			wantStripPrefix: false,
			wantRewriteHTML: false,
		},
		{
			name: "strip_prefix not set (defaults to false)",
			container: &types.Container{
				ID:    "ghi789",
				Names: []string{"/test-app-default"},
				Image: "busybox:latest",
				State: "running",
				Ports: []types.Port{
					{PrivatePort: 8080, Type: "tcp"},
				},
				NetworkSettings: &types.SummaryNetworkSettings{
					Networks: map[string]*network.EndpointSettings{
						"bridge": {IPAddress: "172.17.0.4"},
					},
				},
				Labels: map[string]string{
					"nekzus.enable":     "true",
					"nekzus.app.id":     "test-app3",
					"nekzus.app.name":   "Test App 3",
					"nekzus.route.path": "/apps/test3/",
					// strip_prefix label not set
				},
			},
			wantStripPrefix: false,
			wantRewriteHTML: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear proposals before test
			store.proposals = []*apptypes.Proposal{}

			// Process the container (this will submit proposals)
			worker.processContainer(*tt.container)

			// Give a moment for async processing
			time.Sleep(10 * time.Millisecond)

			// Get proposals
			proposals := store.GetProposals()

			// Check that a proposal was created
			if len(proposals) == 0 {
				t.Fatal("Expected proposal to be created, but got none")
			}

			// Get the first proposal
			proposal := proposals[0]

			if proposal == nil {
				t.Fatal("Proposal is nil")
			}

			// Verify the StripPrefix field
			if proposal.SuggestedRoute.StripPrefix != tt.wantStripPrefix {
				t.Errorf("Expected StripPrefix=%v, got %v",
					tt.wantStripPrefix, proposal.SuggestedRoute.StripPrefix)
			}

			// Verify the RewriteHTML field
			if proposal.SuggestedRoute.RewriteHTML != tt.wantRewriteHTML {
				t.Errorf("Expected RewriteHTML=%v, got %v",
					tt.wantRewriteHTML, proposal.SuggestedRoute.RewriteHTML)
			}

			// Additional verification: check that other route fields are set correctly
			if proposal.SuggestedRoute.AppID != tt.container.Labels["nekzus.app.id"] {
				t.Errorf("Expected AppID=%s, got %s",
					tt.container.Labels["nekzus.app.id"], proposal.SuggestedRoute.AppID)
			}

			if proposal.SuggestedRoute.PathBase != tt.container.Labels["nekzus.route.path"] {
				t.Errorf("Expected PathBase=%s, got %s",
					tt.container.Labels["nekzus.route.path"], proposal.SuggestedRoute.PathBase)
			}
		})
	}
}
