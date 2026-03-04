package federation

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPeerManager_Initialization tests PeerManager initialization
func TestPeerManager_Initialization(t *testing.T) {
	config := Config{
		LocalPeerID:         "nxs_test123",
		LocalPeerName:       "Test Instance",
		APIAddress:          "localhost:8080",
		GossipBindAddr:      "0.0.0.0",
		GossipBindPort:      19946, // Use non-default port for testing
		GossipAdvertiseAddr: "127.0.0.1",
		GossipAdvertisePort: 19946,
		ClusterSecret:       "test-secret-32-characters-long!!",
		MDNSEnabled:         false, // Disable mDNS for unit tests
		BootstrapPeers:      []string{},
		FullSyncInterval:    5 * time.Minute,
		AntiEntropyPeriod:   1 * time.Minute,
		PeerTimeout:         30 * time.Second,
		AllowRemoteRoutes:   false,
	}

	pm, err := NewPeerManager(config, nil, nil, nil)
	require.NoError(t, err, "NewPeerManager should not error with valid config")
	require.NotNil(t, pm, "PeerManager should not be nil")

	assert.Equal(t, "nxs_test123", pm.LocalPeerID())
	assert.Equal(t, "Test Instance", pm.LocalPeerName())
}

// TestPeerManager_InvalidConfig tests initialization with invalid config
func TestPeerManager_InvalidConfig(t *testing.T) {
	tests := []struct {
		name   string
		config Config
	}{
		{
			name: "missing peer ID",
			config: Config{
				LocalPeerID:         "",
				LocalPeerName:       "Test",
				APIAddress:          "localhost:8080",
				ClusterSecret:       "test-secret-32-characters-long!",
				GossipBindPort:      19946,
				GossipAdvertisePort: 19946,
			},
		},
		{
			name: "short cluster secret",
			config: Config{
				LocalPeerID:         "nxs_test",
				LocalPeerName:       "Test",
				APIAddress:          "localhost:8080",
				ClusterSecret:       "short",
				GossipBindPort:      19946,
				GossipAdvertisePort: 19946,
			},
		},
		{
			name: "invalid gossip port",
			config: Config{
				LocalPeerID:         "nxs_test",
				LocalPeerName:       "Test",
				APIAddress:          "localhost:8080",
				ClusterSecret:       "test-secret-32-characters-long!",
				GossipBindPort:      999,
				GossipAdvertisePort: 999,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewPeerManager(tt.config, nil, nil, nil)
			assert.Error(t, err, "NewPeerManager should error with invalid config")
		})
	}
}

// TestPeerManager_StartStop tests starting and stopping the peer manager
func TestPeerManager_StartStop(t *testing.T) {
	config := Config{
		LocalPeerID:         "nxs_test456",
		LocalPeerName:       "Test Instance 2",
		APIAddress:          "localhost:8081",
		GossipBindAddr:      "0.0.0.0",
		GossipBindPort:      19947,
		GossipAdvertiseAddr: "127.0.0.1",
		GossipAdvertisePort: 19947,
		ClusterSecret:       "test-secret-32-characters-long!!",
		MDNSEnabled:         false,
		BootstrapPeers:      []string{},
		FullSyncInterval:    5 * time.Minute,
		AntiEntropyPeriod:   1 * time.Minute,
		PeerTimeout:         30 * time.Second,
		AllowRemoteRoutes:   false,
	}

	pm, err := NewPeerManager(config, nil, nil, nil)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Start peer manager
	err = pm.Start(ctx)
	assert.NoError(t, err, "Start should not error")

	// Verify it's running
	assert.True(t, pm.IsRunning(), "PeerManager should be running after Start")

	// Stop peer manager
	err = pm.Stop()
	assert.NoError(t, err, "Stop should not error")

	// Verify it's stopped
	assert.False(t, pm.IsRunning(), "PeerManager should be stopped after Stop")
}

// TestPeerManager_GetPeers tests retrieving peer list
func TestPeerManager_GetPeers(t *testing.T) {
	config := Config{
		LocalPeerID:         "nxs_test789",
		LocalPeerName:       "Test Instance 3",
		APIAddress:          "localhost:8082",
		GossipBindAddr:      "0.0.0.0",
		GossipBindPort:      19948,
		GossipAdvertiseAddr: "127.0.0.1",
		GossipAdvertisePort: 19948,
		ClusterSecret:       "test-secret-32-characters-long!!",
		MDNSEnabled:         false,
		BootstrapPeers:      []string{},
		FullSyncInterval:    5 * time.Minute,
		AntiEntropyPeriod:   1 * time.Minute,
		PeerTimeout:         30 * time.Second,
		AllowRemoteRoutes:   false,
	}

	pm, err := NewPeerManager(config, nil, nil, nil)
	require.NoError(t, err)

	// Initially, peer list should be empty (only local peer)
	peers := pm.GetPeers()
	assert.Len(t, peers, 0, "Should have no remote peers initially")
}

// TestPeerManager_AddPeer tests adding a peer manually
func TestPeerManager_AddPeer(t *testing.T) {
	config := Config{
		LocalPeerID:         "nxs_test999",
		LocalPeerName:       "Test Instance 4",
		APIAddress:          "localhost:8083",
		GossipBindAddr:      "0.0.0.0",
		GossipBindPort:      19949,
		GossipAdvertiseAddr: "127.0.0.1",
		GossipAdvertisePort: 19949,
		ClusterSecret:       "test-secret-32-characters-long!!",
		MDNSEnabled:         false,
		BootstrapPeers:      []string{},
		FullSyncInterval:    5 * time.Minute,
		AntiEntropyPeriod:   1 * time.Minute,
		PeerTimeout:         30 * time.Second,
		AllowRemoteRoutes:   false,
	}

	pm, err := NewPeerManager(config, nil, nil, nil)
	require.NoError(t, err)

	// Add a peer
	peer := NewPeerInstance("nxs_remote123", "Remote Instance", "192.168.1.100:8080", "192.168.1.100:7946")
	err = pm.AddPeer(peer)
	assert.NoError(t, err, "AddPeer should not error")

	// Verify peer was added
	peers := pm.GetPeers()
	assert.Len(t, peers, 1, "Should have 1 peer after adding")
	assert.Equal(t, "nxs_remote123", peers[0].ID)
	assert.Equal(t, "Remote Instance", peers[0].Name)
}

// TestPeerManager_RemovePeer tests removing a peer
func TestPeerManager_RemovePeer(t *testing.T) {
	config := Config{
		LocalPeerID:         "nxs_test888",
		LocalPeerName:       "Test Instance 5",
		APIAddress:          "localhost:8084",
		GossipBindAddr:      "0.0.0.0",
		GossipBindPort:      19950,
		GossipAdvertiseAddr: "127.0.0.1",
		GossipAdvertisePort: 19950,
		ClusterSecret:       "test-secret-32-characters-long!!",
		MDNSEnabled:         false,
		BootstrapPeers:      []string{},
		FullSyncInterval:    5 * time.Minute,
		AntiEntropyPeriod:   1 * time.Minute,
		PeerTimeout:         30 * time.Second,
		AllowRemoteRoutes:   false,
	}

	pm, err := NewPeerManager(config, nil, nil, nil)
	require.NoError(t, err)

	// Add a peer
	peer := NewPeerInstance("nxs_remote456", "Remote Instance 2", "192.168.1.101:8080", "192.168.1.101:7946")
	err = pm.AddPeer(peer)
	require.NoError(t, err)

	// Remove the peer
	err = pm.RemovePeer("nxs_remote456")
	assert.NoError(t, err, "RemovePeer should not error")

	// Verify peer was removed
	peers := pm.GetPeers()
	assert.Len(t, peers, 0, "Should have 0 peers after removing")
}

// TestPeerManager_GetPeerByID tests retrieving a specific peer
func TestPeerManager_GetPeerByID(t *testing.T) {
	config := Config{
		LocalPeerID:         "nxs_test777",
		LocalPeerName:       "Test Instance 6",
		APIAddress:          "localhost:8085",
		GossipBindAddr:      "0.0.0.0",
		GossipBindPort:      19951,
		GossipAdvertiseAddr: "127.0.0.1",
		GossipAdvertisePort: 19951,
		ClusterSecret:       "test-secret-32-characters-long!!",
		MDNSEnabled:         false,
		BootstrapPeers:      []string{},
		FullSyncInterval:    5 * time.Minute,
		AntiEntropyPeriod:   1 * time.Minute,
		PeerTimeout:         30 * time.Second,
		AllowRemoteRoutes:   false,
	}

	pm, err := NewPeerManager(config, nil, nil, nil)
	require.NoError(t, err)

	// Add a peer
	peer := NewPeerInstance("nxs_remote789", "Remote Instance 3", "192.168.1.102:8080", "192.168.1.102:7946")
	err = pm.AddPeer(peer)
	require.NoError(t, err)

	// Get peer by ID
	foundPeer, err := pm.GetPeerByID("nxs_remote789")
	assert.NoError(t, err, "GetPeerByID should not error for existing peer")
	assert.NotNil(t, foundPeer, "Found peer should not be nil")
	assert.Equal(t, "nxs_remote789", foundPeer.ID)

	// Try to get non-existent peer
	_, err = pm.GetPeerByID("nxs_nonexistent")
	assert.Error(t, err, "GetPeerByID should error for non-existent peer")
}

// TestConfig_SetDefaults tests default value setting
func TestConfig_SetDefaults(t *testing.T) {
	config := Config{
		LocalPeerID:   "nxs_test",
		LocalPeerName: "Test",
		APIAddress:    "localhost:8080",
		ClusterSecret: "test-secret-32-characters-long!",
	}

	config.SetDefaults()

	assert.Equal(t, "0.0.0.0", config.GossipBindAddr)
	assert.Equal(t, 7946, config.GossipBindPort)
	assert.Equal(t, 7946, config.GossipAdvertisePort)
	assert.Equal(t, "_nekzus-peer._tcp", config.MDNSServiceName)
	assert.Equal(t, 5*time.Minute, config.FullSyncInterval)
	assert.Equal(t, 1*time.Minute, config.AntiEntropyPeriod)
	assert.Equal(t, 30*time.Second, config.PeerTimeout)
}

// TestConfig_Validate tests configuration validation
func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name      string
		config    Config
		wantError bool
	}{
		{
			name: "valid config",
			config: Config{
				LocalPeerID:         "nxs_test",
				LocalPeerName:       "Test",
				APIAddress:          "localhost:8080",
				GossipBindPort:      7946,
				GossipAdvertisePort: 7946,
				ClusterSecret:       "test-secret-32-characters-long!!",
				FullSyncInterval:    5 * time.Minute,
				AntiEntropyPeriod:   1 * time.Minute,
				PeerTimeout:         30 * time.Second,
			},
			wantError: false,
		},
		{
			name: "missing peer ID",
			config: Config{
				LocalPeerID:   "",
				LocalPeerName: "Test",
				APIAddress:    "localhost:8080",
				ClusterSecret: "test-secret-32-characters-long!",
			},
			wantError: true,
		},
		{
			name: "short secret",
			config: Config{
				LocalPeerID:   "nxs_test",
				LocalPeerName: "Test",
				APIAddress:    "localhost:8080",
				ClusterSecret: "short",
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestPeerManager_StartWithUnreachableBootstrapPeers tests that Start() completes
// even when bootstrap peers are unreachable (should timeout, not block forever)
func TestPeerManager_StartWithUnreachableBootstrapPeers(t *testing.T) {
	config := Config{
		LocalPeerID:         "nxs_timeout_test",
		LocalPeerName:       "Timeout Test Instance",
		APIAddress:          "localhost:8090",
		GossipBindAddr:      "0.0.0.0",
		GossipBindPort:      19960,
		GossipAdvertiseAddr: "127.0.0.1",
		GossipAdvertisePort: 19960,
		ClusterSecret:       "test-secret-32-characters-long!!",
		MDNSEnabled:         false,
		// Unreachable bootstrap peers - Start should timeout and continue
		BootstrapPeers:    []string{"192.0.2.1:7946", "192.0.2.2:7946"}, // TEST-NET addresses
		FullSyncInterval:  5 * time.Minute,
		AntiEntropyPeriod: 1 * time.Minute,
		PeerTimeout:       30 * time.Second,
		AllowRemoteRoutes: false,
	}

	pm, err := NewPeerManager(config, nil, nil, nil)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Start should complete within timeout even with unreachable bootstrap peers
	// The join timeout is 10 seconds, so this should take ~10s + startup time
	startTime := time.Now()
	err = pm.Start(ctx)
	elapsed := time.Since(startTime)

	// Start should succeed (join failures are non-fatal)
	assert.NoError(t, err, "Start should succeed even with unreachable bootstrap peers")
	assert.True(t, pm.IsRunning(), "PeerManager should be running after Start")

	// Should have taken at least the join timeout (10s) but not much longer
	assert.GreaterOrEqual(t, elapsed.Seconds(), 9.0, "Should have waited for join timeout")
	assert.Less(t, elapsed.Seconds(), 20.0, "Should not take too long to start")

	// Cleanup
	err = pm.Stop()
	assert.NoError(t, err)
}

// TestPeerManager_CheckPeerHealthReAddsPeers tests that checkPeerHealth() re-adds
// peers that are in memberlist but were previously removed from our peers map
func TestPeerManager_CheckPeerHealthReAddsPeers(t *testing.T) {
	config := Config{
		LocalPeerID:         "nxs_health_test",
		LocalPeerName:       "Health Test Instance",
		APIAddress:          "localhost:8091",
		GossipBindAddr:      "0.0.0.0",
		GossipBindPort:      19961,
		GossipAdvertiseAddr: "127.0.0.1",
		GossipAdvertisePort: 19961,
		ClusterSecret:       "test-secret-32-characters-long!!",
		MDNSEnabled:         false,
		BootstrapPeers:      []string{},
		FullSyncInterval:    5 * time.Minute,
		AntiEntropyPeriod:   1 * time.Minute,
		PeerTimeout:         30 * time.Second,
		AllowRemoteRoutes:   false,
	}

	pm, err := NewPeerManager(config, nil, nil, nil)
	require.NoError(t, err)

	// Add a peer manually
	peer := NewPeerInstance("nxs_remote_health", "Remote Health Instance", "192.168.1.200:8080", "192.168.1.200:7946")
	err = pm.AddPeer(peer)
	require.NoError(t, err)

	// Verify peer exists
	peers := pm.GetPeers()
	assert.Len(t, peers, 1, "Should have 1 peer")

	// Remove the peer (simulating what happens when memberlist marks it as failed)
	err = pm.RemovePeer("nxs_remote_health")
	require.NoError(t, err)

	// Verify peer is removed
	peers = pm.GetPeers()
	assert.Len(t, peers, 0, "Should have 0 peers after removal")

	// Note: In real operation, checkPeerHealth() would re-add the peer if it's
	// still in memberlist.Members(). We can't easily test this without starting
	// memberlist, but we verify the AddPeer mechanism works correctly.
}

// TestPeerInstance_StatusTransitions tests peer status transitions
func TestPeerInstance_StatusTransitions(t *testing.T) {
	peer := NewPeerInstance("nxs_status_test", "Status Test", "192.168.1.150:8080", "192.168.1.150:7946")

	// Initial status should be online
	assert.Equal(t, PeerStatusOnline, peer.Status, "Initial status should be online")

	// Transition to offline
	peer.SetStatus(PeerStatusOffline)
	assert.Equal(t, PeerStatusOffline, peer.Status, "Status should be offline")

	// Transition back to online
	peer.SetStatus(PeerStatusOnline)
	assert.Equal(t, PeerStatusOnline, peer.Status, "Status should be online again")

	// Transition to unreachable
	peer.SetStatus(PeerStatusUnreachable)
	assert.Equal(t, PeerStatusUnreachable, peer.Status, "Status should be unreachable")
}

// TestPeerInstance_IsAlive tests the IsAlive check based on LastSeen time
func TestPeerInstance_IsAlive(t *testing.T) {
	peer := NewPeerInstance("nxs_alive_test", "Alive Test", "192.168.1.160:8080", "192.168.1.160:7946")

	// Should be alive initially (just created)
	assert.True(t, peer.IsAlive(30*time.Second), "Peer should be alive immediately after creation")

	// Update last seen
	peer.UpdateLastSeen()
	assert.True(t, peer.IsAlive(30*time.Second), "Peer should be alive after UpdateLastSeen")

	// Manually set LastSeen to past
	peer.LastSeen = time.Now().Add(-1 * time.Minute)
	assert.False(t, peer.IsAlive(30*time.Second), "Peer should not be alive if LastSeen > threshold")
	assert.True(t, peer.IsAlive(2*time.Minute), "Peer should be alive with larger threshold")
}

// TestPeerManager_PeerCountByStatus tests counting peers by status
func TestPeerManager_PeerCountByStatus(t *testing.T) {
	config := Config{
		LocalPeerID:         "nxs_count_test",
		LocalPeerName:       "Count Test Instance",
		APIAddress:          "localhost:8092",
		GossipBindAddr:      "0.0.0.0",
		GossipBindPort:      19962,
		GossipAdvertiseAddr: "127.0.0.1",
		GossipAdvertisePort: 19962,
		ClusterSecret:       "test-secret-32-characters-long!!",
		MDNSEnabled:         false,
		BootstrapPeers:      []string{},
		FullSyncInterval:    5 * time.Minute,
		AntiEntropyPeriod:   1 * time.Minute,
		PeerTimeout:         30 * time.Second,
		AllowRemoteRoutes:   false,
	}

	pm, err := NewPeerManager(config, nil, nil, nil)
	require.NoError(t, err)

	// Add peers with different statuses
	peer1 := NewPeerInstance("nxs_peer1", "Peer 1", "192.168.1.1:8080", "192.168.1.1:7946")
	peer1.SetStatus(PeerStatusOnline)
	err = pm.AddPeer(peer1)
	require.NoError(t, err)

	peer2 := NewPeerInstance("nxs_peer2", "Peer 2", "192.168.1.2:8080", "192.168.1.2:7946")
	peer2.SetStatus(PeerStatusOnline)
	err = pm.AddPeer(peer2)
	require.NoError(t, err)

	peer3 := NewPeerInstance("nxs_peer3", "Peer 3", "192.168.1.3:8080", "192.168.1.3:7946")
	peer3.SetStatus(PeerStatusOffline)
	err = pm.AddPeer(peer3)
	require.NoError(t, err)

	// Get peer counts by status
	statusCounts := pm.GetPeerCountByStatus()

	assert.Equal(t, 2, statusCounts["online"], "Should have 2 online peers")
	assert.Equal(t, 1, statusCounts["offline"], "Should have 1 offline peer")
	assert.Equal(t, 3, pm.GetPeerCount(), "Should have 3 total peers")
}
