package discovery

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/nstalgic/nekzus/internal/types"
)

var log = slog.With("package", "discovery")

// Discovery Interfaces

// Event represents a system event.
type Event interface {
	GetType() string
	GetData() interface{}
}

// EventBus handles publishing events to subscribers.
type EventBus interface {
	Publish(evt Event)
}

// simpleEvent is a basic implementation of Event.
type simpleEvent struct {
	Type string
	Data interface{}
}

func (e simpleEvent) GetType() string      { return e.Type }
func (e simpleEvent) GetData() interface{} { return e.Data }

// ProposalStore manages discovery proposals.
type ProposalStore interface {
	AddProposal(p *types.Proposal)
}

// WSNotifier defines the WebSocket notification interface for discovery events.
type WSNotifier interface {
	// PublishRouteAdded notifies clients when a route is added via discovery.
	PublishRouteAdded(route types.Route)
	// PublishRouteRemoved notifies clients when a route is removed.
	PublishRouteRemoved(routeID string)
	// PublishProposalApproved notifies clients when a proposal is approved.
	PublishProposalApproved(proposalID string)
	// PublishProposalDismissed notifies clients when a proposal is dismissed.
	PublishProposalDismissed(proposalID string)
}

// RouteChecker checks if an app already has a configured route.
type RouteChecker interface {
	// HasRouteForApp returns true if the app already has a route configured.
	HasRouteForApp(appID string) bool

	// ReconcileRouteTarget is called when discovery re-sees an app that
	// already has a route. Implementations should update the existing route's
	// target address if the proposal indicates the container moved to a new
	// host (e.g., after a Docker or NAS restart). A no-op is expected when
	// the container cannot be positively matched to the existing route.
	ReconcileRouteTarget(p *types.Proposal)
}

// DiscoveryWorker represents a service discovery implementation.
type DiscoveryWorker interface {
	// Name returns the worker's identifier.
	Name() string
	// Start begins the discovery process.
	Start(ctx context.Context) error
	// Stop gracefully shuts down the worker.
	Stop() error
}

// DiscoveryManager coordinates multiple discovery workers and manages proposals.
type DiscoveryManager struct {
	mu                 sync.RWMutex
	workers            []DiscoveryWorker
	proposals          map[string]*types.Proposal // keyed by ID for deduplication
	dismissedProposals map[string]bool            // tracks dismissed proposal IDs
	proposalStore      ProposalStore
	eventBus           EventBus
	wsManager          WSNotifier   // WebSocket manager for real-time notifications
	routeChecker       RouteChecker // checks if app already has a route
	ctx                context.Context
	cancel             context.CancelFunc
	wg                 sync.WaitGroup
	proposalCh         chan *types.Proposal
	proposalChClosed   bool         // flag to track if channel is closed
	proposalChMu       sync.RWMutex // protects proposalChClosed flag
}

// NewDiscoveryManager creates a new discovery manager.
// The routeChecker parameter is optional and can be nil.
func NewDiscoveryManager(store ProposalStore, bus EventBus, routeChecker RouteChecker) *DiscoveryManager {
	ctx, cancel := context.WithCancel(context.Background())
	return &DiscoveryManager{
		workers:            []DiscoveryWorker{},
		proposals:          make(map[string]*types.Proposal),
		dismissedProposals: make(map[string]bool),
		proposalStore:      store,
		eventBus:           bus,
		routeChecker:       routeChecker,
		ctx:                ctx,
		cancel:             cancel,
		proposalCh:         make(chan *types.Proposal, 100),
		proposalChClosed:   false,
	}
}

// SetWSManager sets the WebSocket manager for real-time notifications.
func (dm *DiscoveryManager) SetWSManager(wsManager WSNotifier) {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	dm.wsManager = wsManager
}

// RegisterWorker adds a discovery worker to the manager.
func (dm *DiscoveryManager) RegisterWorker(w DiscoveryWorker) {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	dm.workers = append(dm.workers, w)
}

// Start initializes all registered workers and begins discovery.
func (dm *DiscoveryManager) Start() error {
	dm.mu.RLock()
	workers := make([]DiscoveryWorker, len(dm.workers))
	copy(workers, dm.workers)
	dm.mu.RUnlock()

	// Start proposal processor
	dm.wg.Add(1)
	go dm.processProposals()

	// Start all workers
	for _, w := range workers {
		worker := w
		dm.wg.Add(1)
		go func() {
			defer dm.wg.Done()
			log.Info("starting worker", "worker", worker.Name())
			if err := worker.Start(dm.ctx); err != nil {
				log.Error("worker failed", "worker", worker.Name(), "error", err)
				// Stop the worker to clean up resources
				if stopErr := worker.Stop(); stopErr != nil {
					log.Error("error stopping failed worker", "worker", worker.Name(), "error", stopErr)
				}
				// Remove failed worker from the workers list
				dm.removeWorker(worker.Name())
			}
		}()
	}

	log.Info("started workers", "count", len(workers))
	return nil
}

// Stop gracefully shuts down all workers.
func (dm *DiscoveryManager) Stop() error {
	log.Info("stopping all workers")

	// Cancel context first to signal all goroutines to stop
	dm.cancel()

	// Stop all workers
	dm.mu.RLock()
	for _, w := range dm.workers {
		if err := w.Stop(); err != nil {
			log.Error("error stopping worker", "worker", w.Name(), "error", err)
		}
	}
	dm.mu.RUnlock()

	// Close proposal channel safely
	dm.proposalChMu.Lock()
	if !dm.proposalChClosed {
		dm.proposalChClosed = true
		close(dm.proposalCh)
	}
	dm.proposalChMu.Unlock()

	// Wait for all goroutines with timeout
	done := make(chan struct{})
	go func() {
		dm.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// All goroutines finished
	case <-time.After(5 * time.Second):
		log.Warn("timeout waiting for workers to stop")
	}

	log.Info("stopped")
	return nil
}

// SubmitProposal adds a new discovery proposal.
// Safe to call concurrently and after Stop() has been called.
func (dm *DiscoveryManager) SubmitProposal(p *types.Proposal) {
	// Check context first (fast path)
	select {
	case <-dm.ctx.Done():
		log.Debug("dropping proposal - shutting down", "proposal_id", p.ID)
		return
	default:
	}

	// Hold RLock during the entire send operation to prevent close during send
	dm.proposalChMu.RLock()
	defer dm.proposalChMu.RUnlock()

	// Check if channel is closed
	if dm.proposalChClosed {
		log.Debug("dropping proposal - channel closed", "proposal_id", p.ID)
		return
	}

	// Try to send with non-blocking select
	select {
	case dm.proposalCh <- p:
		// Successfully sent
	case <-dm.ctx.Done():
		log.Debug("dropping proposal - shutting down", "proposal_id", p.ID)
	default:
		log.Warn("proposal channel full, dropping proposal", "proposal_id", p.ID)
	}
}

// processProposals handles incoming proposals and deduplicates them.
func (dm *DiscoveryManager) processProposals() {
	defer dm.wg.Done()

	for p := range dm.proposalCh {
		dm.mu.Lock()

		// Check if proposal was dismissed - skip if so
		if dm.dismissedProposals[p.ID] {
			dm.mu.Unlock()
			log.Debug("ignoring dismissed proposal", "proposal_id", p.ID)
			continue
		}

		// Check if app already has a route configured - skip if so
		if dm.routeChecker != nil && p.SuggestedApp.ID != "" {
			if dm.routeChecker.HasRouteForApp(p.SuggestedApp.ID) {
				dm.mu.Unlock()
				// Give the route checker a chance to refresh the existing
				// route's target when the container's address has changed.
				dm.routeChecker.ReconcileRouteTarget(p)
				log.Debug("ignoring proposal - app already has route", "proposal_id", p.ID, "app_id", p.SuggestedApp.ID)
				continue
			}
		}

		// Check if proposal already exists
		if existing, exists := dm.proposals[p.ID]; exists {
			// Update confidence if higher
			if p.Confidence > existing.Confidence {
				existing.Confidence = p.Confidence
				existing.SecurityNotes = p.SecurityNotes
			}
			dm.mu.Unlock()
			continue
		}

		// Add new proposal
		dm.proposals[p.ID] = p
		dm.mu.Unlock()

		// Update proposal store
		dm.proposalStore.AddProposal(p)

		// Publish discovery event
		dm.eventBus.Publish(simpleEvent{
			Type: "discovery.proposal",
			Data: p,
		})

		log.Info("new proposal", "proposal_id", p.ID, "source", p.Source, "confidence", p.Confidence)
	}
}

// GetProposals returns all current proposals.
func (dm *DiscoveryManager) GetProposals() []*types.Proposal {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	proposals := make([]*types.Proposal, 0, len(dm.proposals))
	for _, p := range dm.proposals {
		proposals = append(proposals, p)
	}
	return proposals
}

// RemoveProposal removes a proposal by ID (used when approved).
func (dm *DiscoveryManager) RemoveProposal(id string) {
	dm.mu.Lock()
	delete(dm.proposals, id)
	dm.mu.Unlock()
}

// ApproveProposal approves a discovery proposal, removes it from the pending list,
// and publishes WebSocket events to notify connected clients.
func (dm *DiscoveryManager) ApproveProposal(id string) {
	dm.mu.Lock()
	proposal, exists := dm.proposals[id]
	if exists {
		delete(dm.proposals, id)
	}
	wsManager := dm.wsManager
	dm.mu.Unlock()

	if !exists {
		log.Warn("cannot approve non-existent proposal", "proposal_id", id)
		return
	}

	log.Info("approved proposal", "proposal_id", id)

	// Publish WebSocket events if wsManager is available
	if wsManager != nil {
		// Notify about the route being added
		wsManager.PublishRouteAdded(proposal.SuggestedRoute)
		// Notify that the proposal was approved
		wsManager.PublishProposalApproved(id)
	}
}

// DismissProposal removes a proposal and marks it as dismissed to prevent it from reappearing.
func (dm *DiscoveryManager) DismissProposal(id string) {
	dm.mu.Lock()
	// Remove from active proposals
	delete(dm.proposals, id)

	// Mark as dismissed to prevent re-discovery
	dm.dismissedProposals[id] = true
	wsManager := dm.wsManager
	dm.mu.Unlock()

	log.Info("dismissed proposal", "proposal_id", id)

	// Publish WebSocket event if wsManager is available
	if wsManager != nil {
		wsManager.PublishProposalDismissed(id)
	}
}

// GetDismissedProposals returns all dismissed proposal IDs.
func (dm *DiscoveryManager) GetDismissedProposals() []string {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	dismissed := make([]string, 0, len(dm.dismissedProposals))
	for id := range dm.dismissedProposals {
		dismissed = append(dismissed, id)
	}
	return dismissed
}

// Rediscover clears all dismissed and active proposals to allow fresh discovery.
// Returns the number of dismissed proposals and active proposals that were cleared.
func (dm *DiscoveryManager) Rediscover() (dismissedCount, activeCount int) {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	dismissedCount = len(dm.dismissedProposals)
	activeCount = len(dm.proposals)

	// Clear dismissed proposals to allow them to be rediscovered
	dm.dismissedProposals = make(map[string]bool)

	// Clear active proposals to force fresh discovery
	dm.proposals = make(map[string]*types.Proposal)

	log.Info("rediscovery triggered", "dismissed_count", dismissedCount, "active_count", activeCount)

	return dismissedCount, activeCount
}

// removeWorker removes a worker from the workers list by name.
func (dm *DiscoveryManager) removeWorker(name string) {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	for i, w := range dm.workers {
		if w.Name() == name {
			// Remove worker from slice
			dm.workers = append(dm.workers[:i], dm.workers[i+1:]...)
			log.Info("removed failed worker from pool", "worker", name, "active_workers", len(dm.workers))
			return
		}
	}
}

// ActiveWorkerCount returns the number of registered discovery workers.
func (dm *DiscoveryManager) ActiveWorkerCount() int {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	return len(dm.workers)
}

// PublishWarning publishes a configuration warning event
func (dm *DiscoveryManager) PublishWarning(workerName, warning, details string) {
	dm.eventBus.Publish(simpleEvent{
		Type: "config.warning",
		Data: map[string]interface{}{
			"component": workerName + " Discovery",
			"warning":   warning,
			"details":   details,
		},
	})
}

// Helper Functions

// generateProposalID creates a unique ID for a discovered service.
func generateProposalID(source, scheme, host string, port int) string {
	return fmt.Sprintf("proposal_%s_%s_%s_%d", source, scheme, sanitize(host), port)
}

// sanitize replaces non-alphanumeric characters with underscores.
// Optimized with fast path: if no sanitization needed, return original string without allocation.
func sanitize(s string) string {
	if s == "" {
		return s
	}

	// Fast path: check if sanitization is needed
	// If all characters are alphanumeric, return original string (no allocation)
	needsSanitization := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')) {
			needsSanitization = true
			break
		}
	}

	if !needsSanitization {
		return s // Fast path: no allocation needed
	}

	// Slow path: sanitization needed, preallocate exact size
	result := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
			result[i] = c
		} else {
			result[i] = '_'
		}
	}
	return string(result)
}

// debouncer helps rate-limit discovery proposals.
type debouncer struct {
	mu           sync.Mutex
	seen         map[string]time.Time
	interval     time.Duration
	lastCleanup  time.Time
	cleanupEvery time.Duration
	maxSize      int // Maximum number of entries before forced cleanup
}

// debouncerMaxSize is the maximum number of entries in the debouncer before forced cleanup
const debouncerMaxSize = 5000

func newDebouncer(interval time.Duration) *debouncer {
	return &debouncer{
		seen:         make(map[string]time.Time),
		interval:     interval,
		lastCleanup:  time.Now(),
		cleanupEvery: interval * 2, // Cleanup every 2 intervals (reduced from 10)
		maxSize:      debouncerMaxSize,
	}
}

// ShouldProcess returns true if enough time has passed since last processing.
// Also performs periodic cleanup of stale entries to prevent memory growth.
func (d *debouncer) ShouldProcess(key string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now()

	// Force cleanup if over max size
	if len(d.seen) >= d.maxSize {
		d.aggressiveCleanup(now)
	} else if now.Sub(d.lastCleanup) > d.cleanupEvery {
		// Periodic cleanup to prevent unbounded memory growth
		d.cleanup(now)
		d.lastCleanup = now
	}

	last, exists := d.seen[key]
	if !exists || now.Sub(last) > d.interval {
		d.seen[key] = now
		return true
	}
	return false
}

// cleanup removes stale entries older than interval * 2
// Must be called with lock held.
func (d *debouncer) cleanup(now time.Time) {
	threshold := now.Add(-d.interval * 2)
	for k, v := range d.seen {
		if v.Before(threshold) {
			delete(d.seen, k)
		}
	}
}

// aggressiveCleanup removes entries more aggressively when over max size.
// Must be called with lock held.
func (d *debouncer) aggressiveCleanup(now time.Time) {
	// First pass: remove entries older than interval
	threshold := now.Add(-d.interval)
	for k, v := range d.seen {
		if v.Before(threshold) {
			delete(d.seen, k)
		}
	}

	// If still over limit, remove oldest entries
	if len(d.seen) >= d.maxSize {
		// Reset to empty - this is a safety measure
		d.seen = make(map[string]time.Time)
		log.Warn("debouncer reset due to size overflow")
	}

	d.lastCleanup = now
}
