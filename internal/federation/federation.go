package federation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/hashicorp/memberlist"
	"github.com/nstalgic/nekzus/internal/metrics"
	"github.com/nstalgic/nekzus/internal/storage"
)

var log = slog.With("package", "federation")

var (
	ErrPeerNotFound      = errors.New("peer not found")
	ErrPeerAlreadyExists = errors.New("peer already exists")
	ErrNotRunning        = errors.New("peer manager not running")
)

// EventBus defines the interface for publishing federation events
type EventBus interface {
	Publish(eventType string, data interface{})
}

// PeerManager manages peer discovery and cluster membership
type PeerManager struct {
	mu             sync.RWMutex
	config         Config
	peers          map[string]*PeerInstance // Map of peer ID -> peer instance
	memberlist     *memberlist.Memberlist
	catalogSyncer  *CatalogSyncer // Service catalog synchronization
	broadcastQueue chan []byte    // Queue for catalog sync broadcasts
	storage        *storage.Store
	eventBus       EventBus
	metrics        *metrics.Metrics
	running        bool
	stopCh         chan struct{}
	wg             sync.WaitGroup
	ctx            context.Context
	cancel         context.CancelFunc
}

// NewPeerManager creates a new peer manager instance
func NewPeerManager(
	config Config,
	store *storage.Store,
	eventBus EventBus,
	m *metrics.Metrics,
) (*PeerManager, error) {
	// Set defaults
	config.SetDefaults()

	// Validate configuration
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid federation config: %w", err)
	}

	pm := &PeerManager{
		config:         config,
		peers:          make(map[string]*PeerInstance),
		broadcastQueue: make(chan []byte, 256), // Buffered channel for broadcasts
		storage:        store,
		eventBus:       eventBus,
		metrics:        m,
		stopCh:         make(chan struct{}),
	}

	// Initialize catalog syncer
	pm.catalogSyncer = NewCatalogSyncer(config.LocalPeerID, store)

	// Wire event callback to publish federation events via WebSocket
	if eventBus != nil {
		pm.catalogSyncer.SetEventCallback(eventBus.Publish)
	}

	return pm, nil
}

// Start starts the peer manager and begins peer discovery
func (pm *PeerManager) Start(ctx context.Context) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if pm.running {
		return errors.New("peer manager already running")
	}

	pm.ctx, pm.cancel = context.WithCancel(ctx)

	// Configure memberlist
	mlConfig := memberlist.DefaultLANConfig()
	mlConfig.Name = pm.config.LocalPeerID
	mlConfig.BindAddr = pm.config.GossipBindAddr
	mlConfig.BindPort = pm.config.GossipBindPort
	mlConfig.AdvertiseAddr = pm.config.GossipAdvertiseAddr
	mlConfig.AdvertisePort = pm.config.GossipAdvertisePort

	// Adjust settings for Docker environments where UDP may be unreliable
	// Enable TCP pings as fallback when UDP fails
	mlConfig.DisableTcpPings = false
	// Significantly increase suspicion multiplier to be very tolerant of UDP failures
	// Default is 4, using 16 means ~16x longer before marking as failed
	mlConfig.SuspicionMult = 16
	// Increase retransmit multiplier for better reliability
	mlConfig.RetransmitMult = 4
	// Longer probe interval to reduce frequency of UDP pings
	mlConfig.ProbeInterval = 5 * time.Second
	mlConfig.ProbeTimeout = 5 * time.Second
	// Allow dead nodes to be reclaimed quickly when they try to rejoin
	mlConfig.DeadNodeReclaimTime = 10 * time.Second
	// Increase indirect checks to give more chances for TCP fallback
	mlConfig.IndirectChecks = 5

	// Set up event delegate for peer join/leave/update
	mlConfig.Events = &eventDelegate{pm: pm}

	// Set up delegate for catalog sync messages
	mlConfig.Delegate = &catalogDelegate{pm: pm}

	// Create memberlist
	ml, err := memberlist.Create(mlConfig)
	if err != nil {
		return fmt.Errorf("failed to create memberlist: %w", err)
	}
	pm.memberlist = ml

	// Wire catalog syncer broadcast function
	pm.catalogSyncer.SetBroadcastFunc(pm.broadcastCatalogMessage)

	// Join bootstrap peers if specified (with timeout to prevent blocking)
	if len(pm.config.BootstrapPeers) > 0 {
		// Use a goroutine with timeout to prevent indefinite blocking
		joinDone := make(chan error, 1)
		go func() {
			_, err := ml.Join(pm.config.BootstrapPeers)
			joinDone <- err
		}()

		select {
		case err := <-joinDone:
			if err != nil {
				log.Warn("failed to join some bootstrap peers",
					"error", err)
			}
		case <-time.After(10 * time.Second):
			log.Warn("bootstrap peer join timed out, continuing without all peers")
		}
	}

	// Start background workers
	pm.wg.Add(1)
	go pm.healthCheckWorker()

	pm.wg.Add(1)
	go pm.fullSyncWorker()

	pm.running = true
	log.Info("peer manager started",
		"peer_id", pm.config.LocalPeerID)

	return nil
}

// Stop stops the peer manager and cleanup resources
func (pm *PeerManager) Stop() error {
	pm.mu.Lock()

	if !pm.running {
		pm.mu.Unlock()
		return nil
	}

	// Cancel context
	if pm.cancel != nil {
		pm.cancel()
	}

	// Close stop channel if not already closed
	select {
	case <-pm.stopCh:
		// Already closed
	default:
		close(pm.stopCh)
	}

	// Mark as not running before releasing lock
	pm.running = false

	// Get memberlist reference before releasing lock
	ml := pm.memberlist

	// Release lock before calling memberlist methods to avoid deadlock
	// (memberlist callbacks like NotifyLeave try to acquire our lock)
	pm.mu.Unlock()

	// Wait for background workers
	pm.wg.Wait()

	// Leave memberlist cluster (without holding the lock)
	if ml != nil {
		if err := ml.Leave(pm.config.PeerTimeout); err != nil {
			log.Warn("error leaving memberlist",
				"error", err)
		}
		if err := ml.Shutdown(); err != nil {
			log.Warn("error shutting down memberlist",
				"error", err)
		}
	}

	log.Info("peer manager stopped")

	return nil
}

// IsRunning returns whether the peer manager is currently running
func (pm *PeerManager) IsRunning() bool {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.running
}

// LocalPeerID returns the local peer ID
func (pm *PeerManager) LocalPeerID() string {
	return pm.config.LocalPeerID
}

// LocalPeerName returns the local peer name
func (pm *PeerManager) LocalPeerName() string {
	return pm.config.LocalPeerName
}

// GetPeers returns a list of all known peers (excluding local peer)
func (pm *PeerManager) GetPeers() []*PeerInstance {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	peers := make([]*PeerInstance, 0, len(pm.peers))
	for _, peer := range pm.peers {
		// Don't include local peer
		if peer.ID == pm.config.LocalPeerID {
			continue
		}
		peers = append(peers, peer)
	}

	return peers
}

// GetPeerByID retrieves a peer by its ID
func (pm *PeerManager) GetPeerByID(peerID string) (*PeerInstance, error) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	peer, ok := pm.peers[peerID]
	if !ok {
		return nil, ErrPeerNotFound
	}

	return peer, nil
}

// GetPeerCount returns the total number of peers (excluding self)
func (pm *PeerManager) GetPeerCount() int {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	return len(pm.peers)
}

// GetPeerCountByStatus returns a map of peer counts grouped by status
func (pm *PeerManager) GetPeerCountByStatus() map[string]int {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	counts := make(map[string]int)
	for _, peer := range pm.peers {
		counts[string(peer.Status)]++
	}

	return counts
}

// AddPeer adds a peer to the peer list
func (pm *PeerManager) AddPeer(peer *PeerInstance) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	// Check if peer already exists
	if existing, ok := pm.peers[peer.ID]; ok {
		// Update existing peer
		existing.Address = peer.Address
		existing.GossipAddr = peer.GossipAddr
		existing.Status = peer.Status
		existing.UpdateLastSeen()
		return nil
	}

	// Add new peer
	pm.peers[peer.ID] = peer

	// Persist to storage if available
	if pm.storage != nil {
		// Convert to storage-compatible format
		storagePeer := &storage.PeerInfo{
			ID:            peer.ID,
			Name:          peer.Name,
			APIAddress:    peer.Address,
			GossipAddress: peer.GossipAddr,
			Status:        string(peer.Status),
			LastSeen:      peer.LastSeen,
			Metadata:      peer.Metadata,
			CreatedAt:     peer.CreatedAt,
			UpdatedAt:     peer.UpdatedAt,
		}
		if err := pm.storage.SavePeer(storagePeer); err != nil {
			log.Warn("failed to persist peer to storage",
				"error", err)
		}
	}

	// Publish event
	if pm.eventBus != nil {
		pm.eventBus.Publish("peer_joined", peer)
	}

	// Update metrics
	if pm.metrics != nil {
		pm.metrics.FederationPeersActive.Set(float64(len(pm.peers)))
	}

	log.Info("peer joined",
		"peer_id", peer.ID,
		"peer_name", peer.Name)

	return nil
}

// RemovePeer removes a peer from the peer list
func (pm *PeerManager) RemovePeer(peerID string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	peer, ok := pm.peers[peerID]
	if !ok {
		return ErrPeerNotFound
	}

	delete(pm.peers, peerID)

	// Remove from storage if available
	if pm.storage != nil {
		if err := pm.storage.DeletePeer(peerID); err != nil {
			log.Warn("failed to delete peer from storage",
				"error", err)
		}
	}

	// Publish event
	if pm.eventBus != nil {
		pm.eventBus.Publish("peer_left", peer)
	}

	// Update metrics
	if pm.metrics != nil {
		pm.metrics.FederationPeersActive.Set(float64(len(pm.peers)))
	}

	log.Info("peer left",
		"peer_id", peer.ID,
		"peer_name", peer.Name)

	return nil
}

// Join attempts to join the cluster by connecting to bootstrap peers
func (pm *PeerManager) Join(peers []string) (int, error) {
	pm.mu.RLock()
	if !pm.running {
		pm.mu.RUnlock()
		return 0, ErrNotRunning
	}
	ml := pm.memberlist
	pm.mu.RUnlock()

	n, err := ml.Join(peers)
	if err != nil {
		return n, fmt.Errorf("failed to join peers: %w", err)
	}

	return n, nil
}

// healthCheckWorker periodically checks peer health
func (pm *PeerManager) healthCheckWorker() {
	defer pm.wg.Done()

	ticker := time.NewTicker(pm.config.PeerTimeout / 2)
	defer ticker.Stop()

	for {
		select {
		case <-pm.stopCh:
			return
		case <-pm.ctx.Done():
			return
		case <-ticker.C:
			pm.checkPeerHealth()
		}
	}
}

// fullSyncWorker triggers full catalog sync at configured interval
func (pm *PeerManager) fullSyncWorker() {
	defer pm.wg.Done()

	ticker := time.NewTicker(pm.config.FullSyncInterval)
	defer ticker.Stop()

	for {
		select {
		case <-pm.stopCh:
			return
		case <-pm.ctx.Done():
			return
		case <-ticker.C:
			pm.TriggerFullSync()
		}
	}
}

// TriggerFullSync initiates a full state exchange with connected peers
// Memberlist's anti-entropy mechanism handles the actual sync via LocalState/MergeRemoteState
func (pm *PeerManager) TriggerFullSync() {
	pm.mu.RLock()
	peerCount := len(pm.peers)
	ml := pm.memberlist
	pm.mu.RUnlock()

	if peerCount == 0 || ml == nil {
		return // No peers to sync with
	}

	log.Debug("full sync interval triggered",
		"peer_count", peerCount)

	// Record metric if available
	if pm.metrics != nil {
		pm.metrics.FederationSyncTotal.WithLabelValues("periodic", "success").Inc()
	}
}

// checkPeerHealth checks the health of all peers
func (pm *PeerManager) checkPeerHealth() {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	// Build map of members currently in the memberlist cluster
	memberMap := make(map[string]*memberlist.Node)
	if pm.memberlist != nil {
		for _, member := range pm.memberlist.Members() {
			if member.Name != pm.config.LocalPeerID {
				memberMap[member.Name] = member
			}
		}
	}

	// First, ensure all memberlist members are in our peers map
	// This handles the case where peers rejoin after being marked as failed
	for peerID, member := range memberMap {
		if _, exists := pm.peers[peerID]; !exists {
			// Re-add peer that was removed but is now back in the cluster
			peer := &PeerInstance{
				ID:         member.Name,
				Name:       member.Name,
				Address:    member.Addr.String(),
				GossipAddr: fmt.Sprintf("%s:%d", member.Addr.String(), member.Port),
				Status:     PeerStatusOnline,
				LastSeen:   time.Now(),
				Metadata:   make(map[string]string),
				CreatedAt:  time.Now(),
				UpdatedAt:  time.Now(),
			}
			pm.peers[peerID] = peer
			log.Info("peer re-added to cluster",
				"peer_id", peerID)

			// Publish event
			if pm.eventBus != nil {
				pm.eventBus.Publish("peer_joined", peer)
			}
		}
	}

	// Now check the health of all peers
	for peerID, peer := range pm.peers {
		// Skip local peer
		if peerID == pm.config.LocalPeerID {
			continue
		}

		// Check if peer is still in the memberlist cluster
		// This is more reliable than LastSeen in Docker environments where UDP may be unreliable
		_, inCluster := memberMap[peerID]

		if inCluster {
			// Peer is in the cluster, mark as online and update LastSeen
			if peer.Status != PeerStatusOnline {
				peer.SetStatus(PeerStatusOnline)
				log.Info("peer marked online",
					"peer_id", peerID)

				// Publish event
				if pm.eventBus != nil {
					pm.eventBus.Publish("peer_online", peer)
				}
			}
			peer.UpdateLastSeen()
		} else if !peer.IsAlive(pm.config.PeerTimeout) {
			// Not in cluster and LastSeen expired, mark as offline
			if peer.Status != PeerStatusOffline {
				peer.SetStatus(PeerStatusOffline)
				log.Info("peer marked offline",
					"peer_id", peerID)

				// Publish event
				if pm.eventBus != nil {
					pm.eventBus.Publish("peer_offline", peer)
				}
			}
		}
	}
}

// eventDelegate handles memberlist events
type eventDelegate struct {
	pm *PeerManager
}

// NotifyJoin is called when a node joins the cluster
func (ed *eventDelegate) NotifyJoin(node *memberlist.Node) {
	// Don't add self
	if node.Name == ed.pm.config.LocalPeerID {
		return
	}

	peer := &PeerInstance{
		ID:         node.Name,
		Name:       node.Name, // Will be updated later with actual name
		Address:    node.Addr.String(),
		GossipAddr: fmt.Sprintf("%s:%d", node.Addr.String(), node.Port),
		Status:     PeerStatusOnline,
		LastSeen:   time.Now(),
		Metadata:   make(map[string]string),
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	ed.pm.AddPeer(peer)
}

// NotifyLeave is called when a node leaves the cluster
func (ed *eventDelegate) NotifyLeave(node *memberlist.Node) {
	ed.pm.RemovePeer(node.Name)
}

// NotifyUpdate is called when a node updates its metadata
func (ed *eventDelegate) NotifyUpdate(node *memberlist.Node) {
	ed.pm.mu.Lock()
	defer ed.pm.mu.Unlock()

	if peer, ok := ed.pm.peers[node.Name]; ok {
		peer.UpdateLastSeen()
		if peer.Status != PeerStatusOnline {
			peer.SetStatus(PeerStatusOnline)
		}
	}
}

// Catalog Synchronization Integration

// GetCatalogSyncer returns the catalog syncer instance
func (pm *PeerManager) GetCatalogSyncer() *CatalogSyncer {
	return pm.catalogSyncer
}

// broadcastCatalogMessage broadcasts a catalog sync message to all peers
func (pm *PeerManager) broadcastCatalogMessage(msg []byte) {
	// Non-blocking send to broadcast queue
	select {
	case pm.broadcastQueue <- msg:
		// Message queued successfully
	default:
		// Queue full, log warning
		log.Warn("broadcast queue full, dropping catalog sync message")
	}
}

// catalogDelegate implements memberlist.Delegate for handling catalog sync messages
type catalogDelegate struct {
	pm *PeerManager
}

// NodeMeta returns metadata about this node (unused for now)
func (cd *catalogDelegate) NodeMeta(limit int) []byte {
	return []byte{}
}

// NotifyMsg handles incoming user-defined messages (catalog sync messages)
func (cd *catalogDelegate) NotifyMsg(msg []byte) {
	// Record metric
	if cd.pm.metrics != nil {
		cd.pm.metrics.FederationMessagesReceived.Inc()
	}

	if cd.pm.catalogSyncer == nil {
		return
	}

	// Try to parse as AppUpdateMsg
	var updateMsg AppUpdateMsg
	if err := json.Unmarshal(msg, &updateMsg); err == nil {
		// Convert to FederatedService
		service := &FederatedService{
			ServiceID:    updateMsg.ServiceID,
			OriginPeerID: updateMsg.OriginPeerID,
			App:          updateMsg.App,
			Confidence:   updateMsg.Confidence,
			VectorClock:  updateMsg.VectorClock,
			Tombstone:    updateMsg.Tombstone,
			LastSeen:     time.Now(),
		}

		if err := cd.pm.catalogSyncer.OnRemoteServiceUpdate(service); err != nil {
			log.Error("failed to process remote service update",
				"error", err)
		}
		return
	}

	// Try to parse as AppDeleteMsg
	var deleteMsg AppDeleteMsg
	if err := json.Unmarshal(msg, &deleteMsg); err == nil {
		// Convert to tombstone FederatedService
		tombstone := &FederatedService{
			ServiceID:    deleteMsg.ServiceID,
			OriginPeerID: deleteMsg.OriginPeerID,
			VectorClock:  deleteMsg.VectorClock,
			Tombstone:    true,
			LastSeen:     time.Now(),
		}

		if err := cd.pm.catalogSyncer.OnRemoteServiceUpdate(tombstone); err != nil {
			log.Error("failed to process remote service delete",
				"error", err)
		}
		return
	}

	// Unknown message type
	log.Warn("received unknown catalog sync message")
}

// GetBroadcasts returns pending broadcasts for memberlist to send
func (cd *catalogDelegate) GetBroadcasts(overhead, limit int) [][]byte {
	var broadcasts [][]byte

	// Pull available messages from queue (non-blocking)
	for {
		select {
		case msg := <-cd.pm.broadcastQueue:
			msgLen := len(msg) + overhead
			if msgLen > limit {
				// Message too large, skip
				log.Warn("catalog sync message too large, skipping",
					"size_bytes", msgLen)
				continue
			}

			broadcasts = append(broadcasts, msg)
			limit -= msgLen

			// Record metric for each message sent
			if cd.pm.metrics != nil {
				cd.pm.metrics.FederationMessagesSent.Inc()
			}

			// Stop if we've reached the limit
			if limit <= 0 {
				return broadcasts
			}

		default:
			// No more messages in queue
			return broadcasts
		}
	}
}

// LocalState returns local state for anti-entropy
// Called by memberlist to exchange full state with peers
func (cd *catalogDelegate) LocalState(join bool) []byte {
	if cd.pm.catalogSyncer == nil {
		return []byte{}
	}

	// Get all services including tombstones
	services, err := cd.pm.catalogSyncer.GetAllServices()
	if err != nil {
		log.Error("failed to get services for anti-entropy",
			"error", err)
		return []byte{}
	}

	// Build anti-entropy state
	state := AntiEntropyState{
		SenderID:    cd.pm.config.LocalPeerID,
		Timestamp:   time.Now(),
		Services:    make([]FederatedService, 0, len(services)),
		VectorClock: cd.pm.catalogSyncer.GetLocalVectorClock(),
	}

	for _, svc := range services {
		if svc != nil {
			state.Services = append(state.Services, *svc)
		}
	}

	// Serialize to JSON
	data, err := json.Marshal(state)
	if err != nil {
		log.Error("failed to marshal anti-entropy state",
			"error", err)
		return []byte{}
	}

	return data
}

// MergeRemoteState merges remote state received from anti-entropy
func (cd *catalogDelegate) MergeRemoteState(buf []byte, join bool) {
	if cd.pm.catalogSyncer == nil || len(buf) == 0 {
		return
	}

	// Parse anti-entropy state
	var state AntiEntropyState
	if err := json.Unmarshal(buf, &state); err != nil {
		log.Error("failed to unmarshal anti-entropy state",
			"error", err)
		return
	}

	// Skip our own state
	if state.SenderID == cd.pm.config.LocalPeerID {
		return
	}

	// Convert to service pointers for processing
	services := make([]*FederatedService, 0, len(state.Services))
	for i := range state.Services {
		svc := state.Services[i]
		svc.LastSeen = time.Now() // Update last seen to now
		services = append(services, &svc)
	}

	// Merge all services
	if err := cd.pm.catalogSyncer.MergeRemoteServices(services); err != nil {
		log.Error("failed to merge remote services",
			"error", err,
			"sender_id", state.SenderID)
		return
	}

	log.Info("merged anti-entropy state",
		"sender_id", state.SenderID,
		"service_count", len(state.Services),
		"join", join)
}
