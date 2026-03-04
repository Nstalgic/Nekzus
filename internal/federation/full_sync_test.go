package federation

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPeerManager_FullSyncWorker_StartStop tests that the worker starts and stops
func TestPeerManager_FullSyncWorker_StartStop(t *testing.T) {
	config := Config{
		LocalPeerID:         "nxs_test_fullsync",
		LocalPeerName:       "Test Instance",
		APIAddress:          "localhost:8080",
		GossipBindAddr:      "0.0.0.0",
		GossipBindPort:      19950,
		GossipAdvertiseAddr: "127.0.0.1",
		GossipAdvertisePort: 19950,
		ClusterSecret:       "test-secret-32-characters-long!!",
		MDNSEnabled:         false,
		BootstrapPeers:      []string{},
		FullSyncInterval:    1 * time.Minute, // Minimum allowed
		AntiEntropyPeriod:   1 * time.Minute,
		PeerTimeout:         30 * time.Second,
	}

	pm, err := NewPeerManager(config, nil, nil, nil)
	require.NoError(t, err)

	// Start the peer manager
	ctx := context.Background()
	err = pm.Start(ctx)
	require.NoError(t, err)

	// Let it run briefly
	time.Sleep(50 * time.Millisecond)

	// Stop should work without panic
	err = pm.Stop()
	assert.NoError(t, err)
}

// TestPeerManager_FullSyncWorker_Cancellation tests context cancellation
func TestPeerManager_FullSyncWorker_Cancellation(t *testing.T) {
	config := Config{
		LocalPeerID:         "nxs_test_cancel",
		LocalPeerName:       "Test Instance",
		APIAddress:          "localhost:8080",
		GossipBindAddr:      "0.0.0.0",
		GossipBindPort:      19951,
		GossipAdvertiseAddr: "127.0.0.1",
		GossipAdvertisePort: 19951,
		ClusterSecret:       "test-secret-32-characters-long!!",
		MDNSEnabled:         false,
		BootstrapPeers:      []string{},
		FullSyncInterval:    1 * time.Minute, // Minimum allowed
		AntiEntropyPeriod:   1 * time.Minute,
		PeerTimeout:         30 * time.Second,
	}

	pm, err := NewPeerManager(config, nil, nil, nil)
	require.NoError(t, err)

	// Use cancellable context
	ctx, cancel := context.WithCancel(context.Background())

	err = pm.Start(ctx)
	require.NoError(t, err)

	// Cancel the context
	cancel()

	// Give time for cancellation to propagate
	time.Sleep(50 * time.Millisecond)

	// Stop should still work
	err = pm.Stop()
	assert.NoError(t, err)
}

// TestPeerManager_TriggerFullSync tests the manual sync trigger
func TestPeerManager_TriggerFullSync(t *testing.T) {
	config := Config{
		LocalPeerID:         "nxs_test_trigger",
		LocalPeerName:       "Test Instance",
		APIAddress:          "localhost:8080",
		GossipBindAddr:      "0.0.0.0",
		GossipBindPort:      19952,
		GossipAdvertiseAddr: "127.0.0.1",
		GossipAdvertisePort: 19952,
		ClusterSecret:       "test-secret-32-characters-long!!",
		MDNSEnabled:         false,
		BootstrapPeers:      []string{},
		FullSyncInterval:    5 * time.Minute,
		AntiEntropyPeriod:   1 * time.Minute,
		PeerTimeout:         30 * time.Second,
	}

	pm, err := NewPeerManager(config, nil, nil, nil)
	require.NoError(t, err)

	ctx := context.Background()
	err = pm.Start(ctx)
	require.NoError(t, err)

	// Manual trigger should not panic (even with no peers)
	pm.TriggerFullSync()

	err = pm.Stop()
	assert.NoError(t, err)
}
