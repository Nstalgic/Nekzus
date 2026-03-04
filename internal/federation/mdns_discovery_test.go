package federation

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMDNSPeerDiscovery_ServiceNameFormat tests the mDNS service name format
func TestMDNSPeerDiscovery_ServiceNameFormat(t *testing.T) {
	config := Config{
		LocalPeerID:         "nxs_test_mdns",
		LocalPeerName:       "Test Instance",
		APIAddress:          "localhost:8080",
		GossipBindAddr:      "0.0.0.0",
		GossipBindPort:      19990,
		GossipAdvertiseAddr: "127.0.0.1",
		GossipAdvertisePort: 19990,
		ClusterSecret:       "test-secret-32-characters-long!!",
		MDNSEnabled:         true,
	}
	config.SetDefaults()

	// Default mDNS service name should follow DNS-SD convention
	assert.Equal(t, "_nekzus-peer._tcp", config.MDNSServiceName)
}

// TestMDNSPeerDiscovery_TXTRecord tests the TXT record format for peer info
func TestMDNSPeerDiscovery_TXTRecord(t *testing.T) {
	config := Config{
		LocalPeerID:         "nxs_test_mdns",
		LocalPeerName:       "My Home Server",
		APIAddress:          "192.168.1.100:8080",
		GossipBindAddr:      "0.0.0.0",
		GossipBindPort:      19990,
		GossipAdvertiseAddr: "192.168.1.100",
		GossipAdvertisePort: 19990,
		ClusterSecret:       "test-secret-32-characters-long!!",
		MDNSEnabled:         true,
	}
	config.SetDefaults()

	// Build TXT record
	txtRecord := buildPeerTXTRecord(config)

	// Should contain peer ID
	assert.Contains(t, txtRecord, "peer_id=nxs_test_mdns")
	// Should contain peer name
	assert.Contains(t, txtRecord, "peer_name=My Home Server")
	// Should contain gossip port
	assert.Contains(t, txtRecord, "gossip_port=19990")
}

// TestMDNSPeerDiscovery_ParseTXTRecord tests parsing peer info from TXT record
func TestMDNSPeerDiscovery_ParseTXTRecord(t *testing.T) {
	txtFields := []string{
		"peer_id=nxs_remote_peer",
		"peer_name=Remote Server",
		"gossip_port=19991",
		"api_address=192.168.1.200:8080",
	}

	info := parsePeerTXTRecord(txtFields)

	assert.Equal(t, "nxs_remote_peer", info.PeerID)
	assert.Equal(t, "Remote Server", info.PeerName)
	assert.Equal(t, 19991, info.GossipPort)
	assert.Equal(t, "192.168.1.200:8080", info.APIAddress)
}

// TestMDNSPeerDiscovery_ParseTXTRecord_Empty tests handling empty TXT record
func TestMDNSPeerDiscovery_ParseTXTRecord_Empty(t *testing.T) {
	info := parsePeerTXTRecord([]string{})

	assert.Empty(t, info.PeerID)
	assert.Empty(t, info.PeerName)
	assert.Equal(t, 0, info.GossipPort)
}

// TestMDNSPeerDiscovery_ParseTXTRecord_Malformed tests handling malformed TXT records
func TestMDNSPeerDiscovery_ParseTXTRecord_Malformed(t *testing.T) {
	txtFields := []string{
		"peer_id=valid_id",
		"malformed_entry_no_equals",
		"empty_value=",
		"=no_key",
		"gossip_port=not_a_number",
	}

	info := parsePeerTXTRecord(txtFields)

	// Should still get valid entries
	assert.Equal(t, "valid_id", info.PeerID)
	// Port should be 0 for invalid value
	assert.Equal(t, 0, info.GossipPort)
}

// TestMDNSPeerDiscovery_BuildGossipAddress tests building gossip address from discovered peer
func TestMDNSPeerDiscovery_BuildGossipAddress(t *testing.T) {
	tests := []struct {
		name     string
		host     string
		port     int
		expected string
	}{
		{
			name:     "IPv4",
			host:     "192.168.1.100",
			port:     19990,
			expected: "192.168.1.100:19990",
		},
		{
			name:     "IPv6",
			host:     "fe80::1",
			port:     19990,
			expected: "[fe80::1]:19990",
		},
		{
			name:     "hostname",
			host:     "server.local",
			port:     19990,
			expected: "server.local:19990",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildGossipAddress(tt.host, tt.port)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestMDNSPeerDiscovery_ShouldConnectToPeer tests filtering self and already-connected peers
func TestMDNSPeerDiscovery_ShouldConnectToPeer(t *testing.T) {
	localPeerID := "nxs_local"

	tests := []struct {
		name       string
		remotePeer string
		expected   bool
	}{
		{
			name:       "different peer",
			remotePeer: "nxs_remote",
			expected:   true,
		},
		{
			name:       "same peer (self)",
			remotePeer: "nxs_local",
			expected:   false,
		},
		{
			name:       "empty peer ID",
			remotePeer: "",
			expected:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := shouldConnectToPeer(localPeerID, tt.remotePeer)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestMDNSPeerDiscovery_ConfigEnabled tests enabling mDNS discovery via config
func TestMDNSPeerDiscovery_ConfigEnabled(t *testing.T) {
	config := Config{
		LocalPeerID:         "nxs_test",
		LocalPeerName:       "Test",
		APIAddress:          "localhost:8080",
		GossipBindAddr:      "0.0.0.0",
		GossipBindPort:      19990,
		GossipAdvertiseAddr: "127.0.0.1",
		GossipAdvertisePort: 19990,
		ClusterSecret:       "test-secret-32-characters-long!!",
		MDNSEnabled:         true,
	}
	config.SetDefaults()

	require.True(t, config.MDNSEnabled)
	require.NotEmpty(t, config.MDNSServiceName)
}

// TestMDNSPeerDiscovery_ConfigDisabled tests disabling mDNS discovery via config
func TestMDNSPeerDiscovery_ConfigDisabled(t *testing.T) {
	config := Config{
		LocalPeerID:         "nxs_test",
		LocalPeerName:       "Test",
		APIAddress:          "localhost:8080",
		GossipBindAddr:      "0.0.0.0",
		GossipBindPort:      19990,
		GossipAdvertiseAddr: "127.0.0.1",
		GossipAdvertisePort: 19990,
		ClusterSecret:       "test-secret-32-characters-long!!",
		MDNSEnabled:         false,
	}
	config.SetDefaults()

	require.False(t, config.MDNSEnabled)
}
