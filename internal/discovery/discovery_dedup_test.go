package discovery

import (
	"sync"
	"testing"
	"time"

	"github.com/nstalgic/nekzus/internal/types"
)

// Debouncer Deduplication Tests

// TestDebouncer_ShouldProcess_FirstProposal tests that the first proposal is always processed.
func TestDebouncer_ShouldProcess_FirstProposal(t *testing.T) {
	d := newDebouncer(100 * time.Millisecond)

	// First proposal should always be processed
	if !d.ShouldProcess("proposal-1") {
		t.Error("First proposal should always be processed")
	}

	// Different key should also be processed
	if !d.ShouldProcess("proposal-2") {
		t.Error("First proposal for different key should be processed")
	}
}

// TestDebouncer_ShouldProcess_ImmediateDuplicate tests that immediate duplicates are rejected.
func TestDebouncer_ShouldProcess_ImmediateDuplicate(t *testing.T) {
	d := newDebouncer(100 * time.Millisecond)

	// First call should process
	if !d.ShouldProcess("duplicate-key") {
		t.Fatal("First call should be processed")
	}

	// Immediate second call should be blocked
	if d.ShouldProcess("duplicate-key") {
		t.Error("Immediate duplicate should be rejected")
	}

	// Third immediate call should also be blocked
	if d.ShouldProcess("duplicate-key") {
		t.Error("Third immediate call should also be rejected")
	}
}

// TestDebouncer_ShouldProcess_AfterInterval tests that proposals after interval are allowed.
func TestDebouncer_ShouldProcess_AfterInterval(t *testing.T) {
	interval := 50 * time.Millisecond
	d := newDebouncer(interval)

	// First call
	if !d.ShouldProcess("key") {
		t.Fatal("First call should be processed")
	}

	// Should be blocked before interval
	if d.ShouldProcess("key") {
		t.Error("Should be blocked before interval expires")
	}

	// Wait for interval to expire
	time.Sleep(interval + 10*time.Millisecond)

	// Should be allowed after interval
	if !d.ShouldProcess("key") {
		t.Error("Should be allowed after interval expires")
	}
}

// TestDebouncer_ShouldProcess_IndependentKeys tests that different keys are independent.
func TestDebouncer_ShouldProcess_IndependentKeys(t *testing.T) {
	d := newDebouncer(100 * time.Millisecond)

	// Process multiple different keys
	keys := []string{"key-a", "key-b", "key-c", "key-d"}
	for _, key := range keys {
		if !d.ShouldProcess(key) {
			t.Errorf("First call for key %q should be processed", key)
		}
	}

	// All keys should be blocked for duplicates
	for _, key := range keys {
		if d.ShouldProcess(key) {
			t.Errorf("Immediate duplicate for key %q should be rejected", key)
		}
	}
}

// TestDebouncer_ShouldProcess_ConcurrentSafe tests thread safety of debouncer.
func TestDebouncer_ShouldProcess_ConcurrentSafe(t *testing.T) {
	d := newDebouncer(10 * time.Millisecond)

	var wg sync.WaitGroup
	results := make([]bool, 100)

	// Launch 100 goroutines all trying to process the same key
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			results[idx] = d.ShouldProcess("concurrent-key")
		}(i)
	}

	wg.Wait()

	// Count how many got through
	processedCount := 0
	for _, processed := range results {
		if processed {
			processedCount++
		}
	}

	// Only one should have been processed
	if processedCount != 1 {
		t.Errorf("Expected exactly 1 to be processed, got %d", processedCount)
	}
}

// TestDebouncer_ShouldProcess_UpdatesTimestamp tests that processing updates the timestamp.
func TestDebouncer_ShouldProcess_UpdatesTimestamp(t *testing.T) {
	interval := 30 * time.Millisecond
	d := newDebouncer(interval)

	// First call at T0
	if !d.ShouldProcess("key") {
		t.Fatal("First call should be processed")
	}

	// Wait half the interval
	time.Sleep(interval / 2)

	// Should be blocked (not enough time passed)
	if d.ShouldProcess("key") {
		t.Error("Should be blocked before interval")
	}

	// Wait for more than full interval from T0
	time.Sleep(interval)

	// Now should be allowed
	if !d.ShouldProcess("key") {
		t.Error("Should be allowed after full interval")
	}

	// Immediately after, should be blocked again
	if d.ShouldProcess("key") {
		t.Error("Should be blocked immediately after processing")
	}
}

// TestDebouncer_ShouldProcess_EmptyKey tests behavior with empty key.
func TestDebouncer_ShouldProcess_EmptyKey(t *testing.T) {
	d := newDebouncer(100 * time.Millisecond)

	// Empty key should work like any other key
	if !d.ShouldProcess("") {
		t.Error("Empty key should be processable on first call")
	}

	if d.ShouldProcess("") {
		t.Error("Immediate duplicate of empty key should be rejected")
	}
}

// Discovery Events Publication Tests

// mockWSManager is a mock WebSocketManager for testing route events.
type mockWSManager struct {
	mu                     sync.Mutex
	routeAddedCalls        []types.Route
	routeRemovedCalls      []string
	proposalApprovedCalls  []string
	proposalDismissedCalls []string
}

func (m *mockWSManager) PublishRouteAdded(route types.Route) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.routeAddedCalls = append(m.routeAddedCalls, route)
}

func (m *mockWSManager) PublishRouteRemoved(routeID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.routeRemovedCalls = append(m.routeRemovedCalls, routeID)
}

func (m *mockWSManager) PublishProposalApproved(proposalID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.proposalApprovedCalls = append(m.proposalApprovedCalls, proposalID)
}

func (m *mockWSManager) PublishProposalDismissed(proposalID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.proposalDismissedCalls = append(m.proposalDismissedCalls, proposalID)
}

func (m *mockWSManager) GetRouteAddedCalls() []types.Route {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]types.Route, len(m.routeAddedCalls))
	copy(result, m.routeAddedCalls)
	return result
}

func (m *mockWSManager) GetRouteRemovedCalls() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]string, len(m.routeRemovedCalls))
	copy(result, m.routeRemovedCalls)
	return result
}

func (m *mockWSManager) GetProposalApprovedCalls() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]string, len(m.proposalApprovedCalls))
	copy(result, m.proposalApprovedCalls)
	return result
}

func (m *mockWSManager) GetProposalDismissedCalls() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]string, len(m.proposalDismissedCalls))
	copy(result, m.proposalDismissedCalls)
	return result
}

// TestDiscoveryManager_PublishRouteAddedOnApproval tests that route added events are published.
func TestDiscoveryManager_PublishRouteAddedOnApproval(t *testing.T) {
	store := &mockProposalStore{}
	bus := &mockEventBus{}
	dm := NewDiscoveryManager(store, bus, nil)

	// Set up the WebSocket manager
	wsManager := &mockWSManager{}
	dm.SetWSManager(wsManager)

	err := dm.Start()
	if err != nil {
		t.Fatalf("Failed to start discovery: %v", err)
	}
	defer dm.Stop()

	// Submit a proposal
	proposal := &types.Proposal{
		ID:             "route-add-test",
		Source:         "test",
		DetectedScheme: "http",
		DetectedHost:   "localhost",
		DetectedPort:   8080,
		SuggestedRoute: types.Route{
			RouteID:  "test-route",
			AppID:    "test-app",
			PathBase: "/apps/test/",
			To:       "http://localhost:8080",
		},
	}

	dm.SubmitProposal(proposal)
	time.Sleep(50 * time.Millisecond)

	// Approve the proposal (this should trigger PublishRouteAdded)
	dm.ApproveProposal("route-add-test")
	time.Sleep(50 * time.Millisecond)

	// Verify PublishRouteAdded was called
	calls := wsManager.GetRouteAddedCalls()
	if len(calls) != 1 {
		t.Fatalf("Expected 1 PublishRouteAdded call, got %d", len(calls))
	}

	if calls[0].RouteID != "test-route" {
		t.Errorf("Expected route ID 'test-route', got %q", calls[0].RouteID)
	}
}

// TestDiscoveryManager_PublishRouteRemovedOnDismissal tests that route removed events are published when relevant.
func TestDiscoveryManager_PublishRouteRemovedOnDismissal(t *testing.T) {
	store := &mockProposalStore{}
	bus := &mockEventBus{}
	dm := NewDiscoveryManager(store, bus, nil)

	// Set up the WebSocket manager
	wsManager := &mockWSManager{}
	dm.SetWSManager(wsManager)

	err := dm.Start()
	if err != nil {
		t.Fatalf("Failed to start discovery: %v", err)
	}
	defer dm.Stop()

	// Submit a proposal
	proposal := &types.Proposal{
		ID:     "route-remove-test",
		Source: "test",
		SuggestedRoute: types.Route{
			RouteID: "remove-route",
		},
	}

	dm.SubmitProposal(proposal)
	time.Sleep(50 * time.Millisecond)

	// Dismiss the proposal (should trigger PublishProposalDismissed)
	dm.DismissProposal("route-remove-test")
	time.Sleep(50 * time.Millisecond)

	// Verify PublishProposalDismissed was called
	calls := wsManager.GetProposalDismissedCalls()
	if len(calls) != 1 {
		t.Fatalf("Expected 1 PublishProposalDismissed call, got %d", len(calls))
	}

	if calls[0] != "route-remove-test" {
		t.Errorf("Expected proposal ID 'route-remove-test', got %q", calls[0])
	}
}

// TestDiscoveryManager_NoWSManager_NoPanic tests that operations work when wsManager is nil.
func TestDiscoveryManager_NoWSManager_NoPanic(t *testing.T) {
	store := &mockProposalStore{}
	bus := &mockEventBus{}
	dm := NewDiscoveryManager(store, bus, nil)

	// Don't set wsManager - it should be nil

	err := dm.Start()
	if err != nil {
		t.Fatalf("Failed to start discovery: %v", err)
	}
	defer dm.Stop()

	// Submit a proposal
	proposal := &types.Proposal{
		ID:     "no-panic-test",
		Source: "test",
	}

	dm.SubmitProposal(proposal)
	time.Sleep(50 * time.Millisecond)

	// These should not panic when wsManager is nil
	dm.ApproveProposal("no-panic-test")
	dm.DismissProposal("no-panic-test")

	// If we get here without panic, test passes
}

// TestDiscoveryManager_PublishProposalApproved tests that proposal approved events are published.
func TestDiscoveryManager_PublishProposalApproved(t *testing.T) {
	store := &mockProposalStore{}
	bus := &mockEventBus{}
	dm := NewDiscoveryManager(store, bus, nil)

	// Set up the WebSocket manager
	wsManager := &mockWSManager{}
	dm.SetWSManager(wsManager)

	err := dm.Start()
	if err != nil {
		t.Fatalf("Failed to start discovery: %v", err)
	}
	defer dm.Stop()

	// Submit a proposal
	proposal := &types.Proposal{
		ID:     "approved-test",
		Source: "test",
		SuggestedRoute: types.Route{
			RouteID: "approved-route",
		},
	}

	dm.SubmitProposal(proposal)
	time.Sleep(50 * time.Millisecond)

	// Approve the proposal
	dm.ApproveProposal("approved-test")
	time.Sleep(50 * time.Millisecond)

	// Verify PublishProposalApproved was called
	calls := wsManager.GetProposalApprovedCalls()
	if len(calls) != 1 {
		t.Fatalf("Expected 1 PublishProposalApproved call, got %d", len(calls))
	}

	if calls[0] != "approved-test" {
		t.Errorf("Expected proposal ID 'approved-test', got %q", calls[0])
	}
}

// TestDiscoveryManager_WSManagerConcurrency tests thread safety of WebSocket publishing.
func TestDiscoveryManager_WSManagerConcurrency(t *testing.T) {
	store := &mockProposalStore{}
	bus := &mockEventBus{}
	dm := NewDiscoveryManager(store, bus, nil)

	// Set up the WebSocket manager
	wsManager := &mockWSManager{}
	dm.SetWSManager(wsManager)

	err := dm.Start()
	if err != nil {
		t.Fatalf("Failed to start discovery: %v", err)
	}
	defer dm.Stop()

	// Submit multiple proposals concurrently
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			proposal := &types.Proposal{
				ID:     string(rune('a' + n)),
				Source: "concurrent",
				SuggestedRoute: types.Route{
					RouteID: string(rune('a' + n)),
				},
			}
			dm.SubmitProposal(proposal)
		}(i)
	}

	wg.Wait()
	time.Sleep(100 * time.Millisecond)

	// Approve all proposals concurrently
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			dm.ApproveProposal(string(rune('a' + n)))
		}(i)
	}

	wg.Wait()
	time.Sleep(100 * time.Millisecond)

	// All should have been approved
	calls := wsManager.GetProposalApprovedCalls()
	if len(calls) != 10 {
		t.Errorf("Expected 10 PublishProposalApproved calls, got %d", len(calls))
	}
}
