package discovery

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nstalgic/nekzus/internal/types"
)

// Race Condition Tests
// These tests verify safe shutdown and concurrent proposal handling

// TestDiscoveryManager_StopDuringProposals tests that Stop() doesn't panic
// when proposals are being submitted concurrently during shutdown.
func TestDiscoveryManager_StopDuringProposals(t *testing.T) {
	store := &mockProposalStore{}
	bus := &mockEventBus{}
	dm := NewDiscoveryManager(store, bus, nil)

	err := dm.Start()
	if err != nil {
		t.Fatalf("Failed to start discovery: %v", err)
	}

	// Start goroutines that submit proposals continuously
	var wg sync.WaitGroup
	stopSubmitting := make(chan struct{})
	panicOccurred := int32(0)

	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					atomic.StoreInt32(&panicOccurred, 1)
					t.Errorf("Panic occurred during proposal submission: %v", r)
				}
			}()

			for j := 0; ; j++ {
				select {
				case <-stopSubmitting:
					return
				default:
					proposal := &types.Proposal{
						ID:     fmt.Sprintf("proposal-%d-%d", id, j),
						Source: "race-test",
					}
					dm.SubmitProposal(proposal)
					time.Sleep(time.Microsecond)
				}
			}
		}(i)
	}

	// Let some proposals flow
	time.Sleep(10 * time.Millisecond)

	// Stop the discovery manager - this should not panic
	err = dm.Stop()
	if err != nil {
		t.Fatalf("Failed to stop discovery: %v", err)
	}

	// Signal goroutines to stop
	close(stopSubmitting)

	// Wait for all submission goroutines to finish
	wg.Wait()

	if atomic.LoadInt32(&panicOccurred) != 0 {
		t.Fatal("Panic occurred during concurrent shutdown")
	}
}

// TestDiscoveryManager_SubmitProposalAfterStop tests that submitting proposals
// after Stop() has been called doesn't cause a panic.
func TestDiscoveryManager_SubmitProposalAfterStop(t *testing.T) {
	store := &mockProposalStore{}
	bus := &mockEventBus{}
	dm := NewDiscoveryManager(store, bus, nil)

	err := dm.Start()
	if err != nil {
		t.Fatalf("Failed to start discovery: %v", err)
	}

	err = dm.Stop()
	if err != nil {
		t.Fatalf("Failed to stop discovery: %v", err)
	}

	// This should not panic
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("SubmitProposal panicked after Stop: %v", r)
		}
	}()

	proposal := &types.Proposal{
		ID:     "after-stop-proposal",
		Source: "test",
	}
	dm.SubmitProposal(proposal)
}

// TestDiscoveryManager_StopWithTimeout tests that Stop() completes within
// a reasonable timeout even when workers are slow to stop.
func TestDiscoveryManager_StopWithTimeout(t *testing.T) {
	store := &mockProposalStore{}
	bus := &mockEventBus{}
	dm := NewDiscoveryManager(store, bus, nil)

	// Create a slow worker that takes time to stop
	slowWorker := &slowMockWorker{
		name:      "slow-worker",
		startCh:   make(chan struct{}),
		stopDelay: 100 * time.Millisecond,
	}
	dm.RegisterWorker(slowWorker)

	err := dm.Start()
	if err != nil {
		t.Fatalf("Failed to start discovery: %v", err)
	}

	// Wait for worker to start
	select {
	case <-slowWorker.startCh:
	case <-time.After(time.Second):
		t.Fatal("Worker did not start in time")
	}

	// Stop should complete within reasonable time
	done := make(chan struct{})
	go func() {
		dm.Stop()
		close(done)
	}()

	select {
	case <-done:
		// Good, stopped in time
	case <-time.After(2 * time.Second):
		t.Fatal("Stop did not complete within timeout")
	}
}

// TestDiscoveryManager_ConcurrentProposalsDuringShutdown tests many concurrent
// proposals during the shutdown process.
func TestDiscoveryManager_ConcurrentProposalsDuringShutdown(t *testing.T) {
	store := &mockProposalStore{}
	bus := &mockEventBus{}
	dm := NewDiscoveryManager(store, bus, nil)

	err := dm.Start()
	if err != nil {
		t.Fatalf("Failed to start discovery: %v", err)
	}

	// Submit some initial proposals
	for i := 0; i < 50; i++ {
		proposal := &types.Proposal{
			ID:     fmt.Sprintf("initial-%d", i),
			Source: "test",
		}
		dm.SubmitProposal(proposal)
	}

	// Start concurrent submissions
	var wg sync.WaitGroup
	panicCount := int32(0)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					atomic.AddInt32(&panicCount, 1)
				}
			}()

			for j := 0; j < 100; j++ {
				proposal := &types.Proposal{
					ID:     fmt.Sprintf("concurrent-%d-%d", id, j),
					Source: "test",
				}
				dm.SubmitProposal(proposal)
			}
		}(i)
	}

	// Concurrently stop the manager
	time.Sleep(time.Millisecond)
	err = dm.Stop()
	if err != nil {
		t.Fatalf("Failed to stop discovery: %v", err)
	}

	wg.Wait()

	if panicCount > 0 {
		t.Fatalf("Got %d panics during concurrent shutdown", panicCount)
	}
}

// TestDiscoveryManager_ChannelFullScenario tests the behavior when the
// proposal channel is full.
func TestDiscoveryManager_ChannelFullScenario(t *testing.T) {
	store := &mockProposalStore{}
	bus := &mockEventBus{}

	// Create a new manager with a smaller buffer for testing
	ctx, cancel := context.WithCancel(context.Background())
	dm := &DiscoveryManager{
		workers:            []DiscoveryWorker{},
		proposals:          make(map[string]*types.Proposal),
		dismissedProposals: make(map[string]bool),
		proposalStore:      store,
		eventBus:           bus,
		proposalCh:         make(chan *types.Proposal, 5), // Small buffer
		ctx:                ctx,
		cancel:             cancel,
	}

	// Fill up the channel without processing
	for i := 0; i < 10; i++ {
		proposal := &types.Proposal{
			ID:     fmt.Sprintf("overflow-%d", i),
			Source: "test",
		}
		// This should not block - excess proposals should be dropped
		done := make(chan struct{})
		go func() {
			dm.SubmitProposal(proposal)
			close(done)
		}()

		select {
		case <-done:
			// Good, didn't block
		case <-time.After(100 * time.Millisecond):
			t.Fatalf("SubmitProposal blocked on proposal %d", i)
		}
	}

	cancel()
}

// slowMockWorker is a mock worker that takes time to stop
type slowMockWorker struct {
	name      string
	started   bool
	stopped   bool
	startCh   chan struct{}
	stopDelay time.Duration
}

func (w *slowMockWorker) Name() string {
	return w.name
}

func (w *slowMockWorker) Start(ctx context.Context) error {
	w.started = true
	close(w.startCh)
	<-ctx.Done()
	return nil
}

func (w *slowMockWorker) Stop() error {
	time.Sleep(w.stopDelay)
	w.stopped = true
	return nil
}

// Debouncer Memory Leak Tests

// TestDebouncer_CleanupInterval tests that cleanup happens at proper intervals.
func TestDebouncer_CleanupInterval(t *testing.T) {
	// Use a short interval for testing
	d := newDebouncer(10 * time.Millisecond)

	// Add many entries
	for i := 0; i < 100; i++ {
		d.ShouldProcess(fmt.Sprintf("key-%d", i))
	}

	d.mu.Lock()
	initialCount := len(d.seen)
	d.mu.Unlock()

	if initialCount != 100 {
		t.Fatalf("Expected 100 initial entries, got %d", initialCount)
	}

	// Wait for entries to become stale (2x interval)
	time.Sleep(25 * time.Millisecond)

	// Trigger cleanup by processing a new key
	d.ShouldProcess("trigger-cleanup")

	d.mu.Lock()
	finalCount := len(d.seen)
	d.mu.Unlock()

	// After cleanup, stale entries should be removed
	// Only the new "trigger-cleanup" entry should remain (or very few)
	if finalCount > 10 {
		t.Errorf("Expected cleanup to remove stale entries, but still have %d entries", finalCount)
	}
}

// TestDebouncer_MaxSizeEnforcement tests that the debouncer enforces max size.
func TestDebouncer_MaxSizeEnforcement(t *testing.T) {
	d := newDebouncer(time.Hour) // Long interval so entries don't expire

	// Add more than max allowed entries
	for i := 0; i < 6000; i++ {
		d.ShouldProcess(fmt.Sprintf("key-%d", i))
	}

	d.mu.Lock()
	count := len(d.seen)
	d.mu.Unlock()

	// Should be capped at max size (5000)
	if count > 5000 {
		t.Errorf("Debouncer should enforce max size of 5000, but has %d entries", count)
	}
}

// TestDebouncer_MemoryDoesNotGrowUnbounded tests memory doesn't grow without limit.
func TestDebouncer_MemoryDoesNotGrowUnbounded(t *testing.T) {
	d := newDebouncer(5 * time.Millisecond)

	// Simulate continuous operation
	for round := 0; round < 10; round++ {
		// Add many entries
		for i := 0; i < 500; i++ {
			d.ShouldProcess(fmt.Sprintf("round-%d-key-%d", round, i))
		}

		// Wait for cleanup interval
		time.Sleep(15 * time.Millisecond)

		// Trigger cleanup
		d.ShouldProcess(fmt.Sprintf("cleanup-trigger-%d", round))
	}

	d.mu.Lock()
	finalCount := len(d.seen)
	d.mu.Unlock()

	// After all rounds with cleanup, should not have accumulated all entries
	// Should only have recent entries (from last round or two)
	if finalCount > 1000 {
		t.Errorf("Memory grew unbounded, have %d entries", finalCount)
	}
}
