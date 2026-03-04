package federation

import (
	"time"

	"github.com/nstalgic/nekzus/internal/types"
)

// FederatedService represents a service discovered from a peer with federation metadata
type FederatedService struct {
	ServiceID    string      `json:"serviceId"`    // Unique service ID (app ID)
	OriginPeerID string      `json:"originPeerId"` // Peer that owns this service
	App          *types.App  `json:"app"`          // Application metadata
	Confidence   float64     `json:"confidence"`   // Discovery confidence (0.0-1.0)
	LastSeen     time.Time   `json:"lastSeen"`     // Last time service was observed
	Tombstone    bool        `json:"tombstone"`    // If true, service was deleted
	VectorClock  VectorClock `json:"vectorClock"`  // Causality tracking for conflict resolution
}

// GossipMessageType represents the type of gossip message
type GossipMessageType string

const (
	// GossipAppUpdate indicates a service was added or updated
	GossipAppUpdate GossipMessageType = "app_update"

	// GossipAppDelete indicates a service was deleted (tombstone)
	GossipAppDelete GossipMessageType = "app_delete"

	// GossipFullSync is a full catalog sync request/response
	GossipFullSync GossipMessageType = "full_sync"

	// GossipAntiEntropy is an anti-entropy repair message
	GossipAntiEntropy GossipMessageType = "anti_entropy"
)

// GossipMessage represents a message sent between peers for service catalog sync
type GossipMessage struct {
	Type        GossipMessageType  `json:"type"`                  // Message type
	SenderID    string             `json:"senderId"`              // Sender peer ID
	Timestamp   time.Time          `json:"timestamp"`             // Message timestamp
	Services    []FederatedService `json:"services,omitempty"`    // Services payload
	VectorClock VectorClock        `json:"vectorClock,omitempty"` // Sender's vector clock
}

// AppUpdateMsg represents a single service update gossip message
type AppUpdateMsg struct {
	ServiceID    string      `json:"serviceId"`
	OriginPeerID string      `json:"originPeerId"`
	App          *types.App  `json:"app"`
	Confidence   float64     `json:"confidence"`
	VectorClock  VectorClock `json:"vectorClock"`
	Tombstone    bool        `json:"tombstone"`
}

// AppDeleteMsg represents a service deletion (tombstone) gossip message
type AppDeleteMsg struct {
	ServiceID    string      `json:"serviceId"`
	OriginPeerID string      `json:"originPeerId"`
	VectorClock  VectorClock `json:"vectorClock"`
}

// AntiEntropyState represents the full catalog state for anti-entropy sync
type AntiEntropyState struct {
	SenderID    string             `json:"senderId"`    // Peer sending this state
	Timestamp   time.Time          `json:"timestamp"`   // When state was captured
	Services    []FederatedService `json:"services"`    // All services (including tombstones)
	VectorClock VectorClock        `json:"vectorClock"` // Aggregate vector clock
}

// ConflictResolution represents the result of resolving a conflict
type ConflictResolution string

const (
	// ConflictKeepLocal means the local version is kept
	ConflictKeepLocal ConflictResolution = "keep_local"

	// ConflictKeepRemote means the remote version is kept
	ConflictKeepRemote ConflictResolution = "keep_remote"

	// ConflictMerge means both versions are merged
	ConflictMerge ConflictResolution = "merge"
)
