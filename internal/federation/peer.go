package federation

import (
	"time"
)

// PeerStatus represents the status of a peer instance
type PeerStatus string

const (
	PeerStatusOnline      PeerStatus = "online"
	PeerStatusOffline     PeerStatus = "offline"
	PeerStatusUnreachable PeerStatus = "unreachable"
)

// PeerInstance represents a single Nekzus instance in the federation
type PeerInstance struct {
	ID         string            `json:"id"`          // Unique instance ID (nxs_xxxxx)
	Name       string            `json:"name"`        // User-friendly name
	Address    string            `json:"address"`     // IP:Port for API
	GossipAddr string            `json:"gossip_addr"` // IP:Port for gossip protocol
	Status     PeerStatus        `json:"status"`      // online, offline, unreachable
	LastSeen   time.Time         `json:"last_seen"`   // Last time peer was seen alive
	Metadata   map[string]string `json:"metadata"`    // Capabilities, version, etc.
	CreatedAt  time.Time         `json:"created_at"`  // When this peer was first discovered
	UpdatedAt  time.Time         `json:"updated_at"`  // Last update time
}

// NewPeerInstance creates a new peer instance
func NewPeerInstance(id, name, address, gossipAddr string) *PeerInstance {
	now := time.Now()
	return &PeerInstance{
		ID:         id,
		Name:       name,
		Address:    address,
		GossipAddr: gossipAddr,
		Status:     PeerStatusOnline,
		LastSeen:   now,
		Metadata:   make(map[string]string),
		CreatedAt:  now,
		UpdatedAt:  now,
	}
}

// IsAlive checks if peer is considered alive based on last seen time
func (p *PeerInstance) IsAlive(threshold time.Duration) bool {
	return time.Since(p.LastSeen) < threshold
}

// UpdateLastSeen updates the last seen timestamp
func (p *PeerInstance) UpdateLastSeen() {
	p.LastSeen = time.Now()
	p.UpdatedAt = time.Now()
}

// SetStatus updates the peer status
func (p *PeerInstance) SetStatus(status PeerStatus) {
	p.Status = status
	p.UpdatedAt = time.Now()
}
