package federation

import (
	"errors"
	"time"
)

// Config represents federation configuration
type Config struct {
	// LocalPeerID is the unique ID for this instance
	LocalPeerID string

	// LocalPeerName is the user-friendly name for this instance
	LocalPeerName string

	// APIAddress is the address where this instance's API is accessible
	APIAddress string

	// GossipBindAddr is the address to bind gossip protocol (default: 0.0.0.0)
	GossipBindAddr string

	// GossipBindPort is the port to bind gossip protocol (default: 7946)
	GossipBindPort int

	// GossipAdvertiseAddr is the address to advertise to other peers
	GossipAdvertiseAddr string

	// GossipAdvertisePort is the port to advertise to other peers
	GossipAdvertisePort int

	// ClusterSecret is the shared secret for authenticating peers
	ClusterSecret string

	// MDNSEnabled enables mDNS discovery of peers
	MDNSEnabled bool

	// MDNSServiceName is the mDNS service name (default: _nekzus-peer._tcp)
	MDNSServiceName string

	// BootstrapPeers is a list of known peer addresses to join
	BootstrapPeers []string

	// FullSyncInterval is how often to perform full sync with peers
	FullSyncInterval time.Duration

	// AntiEntropyPeriod is how often to run anti-entropy for consistency
	AntiEntropyPeriod time.Duration

	// PeerTimeout is how long before a peer is considered dead
	PeerTimeout time.Duration

	// AllowRemoteRoutes determines if routes from remote peers are exposed
	AllowRemoteRoutes bool
}

// Validate validates the federation configuration
func (c *Config) Validate() error {
	if c.LocalPeerID == "" {
		return errors.New("local_peer_id is required")
	}

	if c.LocalPeerName == "" {
		return errors.New("local_peer_name is required")
	}

	if c.APIAddress == "" {
		return errors.New("api_address is required")
	}

	if len(c.ClusterSecret) < 32 {
		return errors.New("cluster_secret must be at least 32 characters")
	}

	if c.GossipBindPort < 1024 || c.GossipBindPort > 65535 {
		return errors.New("gossip_bind_port must be between 1024 and 65535")
	}

	if c.GossipAdvertisePort < 1024 || c.GossipAdvertisePort > 65535 {
		return errors.New("gossip_advertise_port must be between 1024 and 65535")
	}

	if c.FullSyncInterval < time.Minute {
		return errors.New("full_sync_interval must be at least 1 minute")
	}

	if c.AntiEntropyPeriod < 30*time.Second {
		return errors.New("anti_entropy_period must be at least 30 seconds")
	}

	if c.PeerTimeout < 10*time.Second {
		return errors.New("peer_timeout must be at least 10 seconds")
	}

	return nil
}

// SetDefaults sets default values for optional configuration
func (c *Config) SetDefaults() {
	if c.GossipBindAddr == "" {
		c.GossipBindAddr = "0.0.0.0"
	}

	if c.GossipBindPort == 0 {
		c.GossipBindPort = 7946
	}

	if c.GossipAdvertisePort == 0 {
		c.GossipAdvertisePort = c.GossipBindPort
	}

	if c.MDNSServiceName == "" {
		c.MDNSServiceName = "_nekzus-peer._tcp"
	}

	if c.FullSyncInterval == 0 {
		c.FullSyncInterval = 5 * time.Minute
	}

	if c.AntiEntropyPeriod == 0 {
		c.AntiEntropyPeriod = 1 * time.Minute
	}

	if c.PeerTimeout == 0 {
		c.PeerTimeout = 30 * time.Second
	}
}
