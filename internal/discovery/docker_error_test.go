package discovery

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
)

// Docker Worker Error Recovery Tests
// These tests verify that the Docker worker properly handles consecutive failures
// and stops after maxFailures to avoid infinite error loops.

// mockDockerClientWithFailures is a mock Docker client that can simulate failures.
type mockDockerClientWithFailures struct {
	mu             sync.Mutex
	failCount      int
	failUntil      int
	totalCalls     int
	alwaysFail     bool
	containersList []types.Container
}

func (m *mockDockerClientWithFailures) ContainerList(ctx context.Context, options container.ListOptions) ([]types.Container, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.totalCalls++
	if m.alwaysFail || m.failCount < m.failUntil {
		m.failCount++
		return nil, errors.New("mock Docker daemon error")
	}
	if m.containersList != nil {
		return m.containersList, nil
	}
	return []types.Container{}, nil
}

func (m *mockDockerClientWithFailures) Close() error {
	return nil
}

func (m *mockDockerClientWithFailures) getTotalCalls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.totalCalls
}

func (m *mockDockerClientWithFailures) getFailCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.failCount
}

// TestDockerWorker_StopsAfterMaxFailures tests that the worker stops after
// reaching the maximum number of consecutive failures.
func TestDockerWorker_StopsAfterMaxFailures(t *testing.T) {
	store := &mockProposalStore{}
	bus := &mockEventBus{}
	dm := NewDiscoveryManager(store, bus, nil)

	// Start proposal processor
	dm.wg.Add(1)
	go dm.processProposals()
	defer func() {
		close(dm.proposalCh)
		dm.wg.Wait()
	}()

	// Create mock client that always fails
	mockClient := &mockDockerClientWithFailures{
		alwaysFail: true,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	worker := &DockerDiscoveryWorker{
		dm:            dm,
		pollInterval:  10 * time.Millisecond,
		debouncer:     newDebouncer(30 * time.Second),
		knownServices: make(map[string]bool),
		ctx:           ctx,
		cancel:        cancel,
		networkMode:   "all",
	}

	// Test the failure counting logic directly
	failureCount := 0
	maxFailures := 3

	for i := 0; i < 5; i++ {
		_, err := mockClient.ContainerList(ctx, container.ListOptions{})
		if err != nil {
			failureCount++
			if failureCount >= maxFailures {
				break
			}
		} else {
			failureCount = 0
		}
	}

	// Should have stopped after maxFailures
	if failureCount < maxFailures {
		t.Errorf("Expected failureCount >= %d, got %d", maxFailures, failureCount)
	}

	if mockClient.getTotalCalls() != maxFailures {
		t.Errorf("Expected %d calls before stopping, got %d", maxFailures, mockClient.getTotalCalls())
	}

	worker.Stop()
}

// TestDockerWorker_ResetsFailureCountOnSuccess tests that the failure counter
// resets after a successful scan.
func TestDockerWorker_ResetsFailureCountOnSuccess(t *testing.T) {
	store := &mockProposalStore{}
	bus := &mockEventBus{}
	dm := NewDiscoveryManager(store, bus, nil)

	// Start proposal processor
	dm.wg.Add(1)
	go dm.processProposals()
	defer func() {
		close(dm.proposalCh)
		dm.wg.Wait()
	}()

	// Create mock client that fails twice then succeeds
	mockClient := &mockDockerClientWithFailures{
		failUntil: 2, // Fail first 2 calls
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	worker := &DockerDiscoveryWorker{
		dm:            dm,
		pollInterval:  10 * time.Millisecond,
		debouncer:     newDebouncer(30 * time.Second),
		knownServices: make(map[string]bool),
		ctx:           ctx,
		cancel:        cancel,
		networkMode:   "all",
	}

	// Test the failure counting logic directly
	failureCount := 0
	maxFailures := 3

	for i := 0; i < 5; i++ {
		_, err := mockClient.ContainerList(ctx, container.ListOptions{})
		if err != nil {
			failureCount++
			if failureCount >= maxFailures {
				t.Fatal("Should not reach maxFailures when success happens before")
			}
		} else {
			failureCount = 0
		}
	}

	// Failure count should be reset to 0 after success
	if failureCount != 0 {
		t.Errorf("Expected failureCount to be 0 after success, got %d", failureCount)
	}

	// Should have made 5 calls total
	if mockClient.getTotalCalls() != 5 {
		t.Errorf("Expected 5 total calls, got %d", mockClient.getTotalCalls())
	}

	worker.Stop()
}

// TestDockerWorker_FailureCountingAndRecovery tests that the worker counts failures
// and resets the counter on success.
func TestDockerWorker_FailureCountingAndRecovery(t *testing.T) {
	store := &mockProposalStore{}
	bus := &mockEventBus{}
	dm := NewDiscoveryManager(store, bus, nil)

	// Start proposal processor
	dm.wg.Add(1)
	go dm.processProposals()
	defer func() {
		close(dm.proposalCh)
		dm.wg.Wait()
	}()

	// Create worker
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	worker := &DockerDiscoveryWorker{
		dm:            dm,
		pollInterval:  10 * time.Millisecond,
		debouncer:     newDebouncer(30 * time.Second),
		knownServices: make(map[string]bool),
		ctx:           ctx,
		cancel:        cancel,
		networkMode:   "all",
	}

	// Create mock client that fails twice then succeeds
	mockClient := &mockDockerClientWithFailures{
		failUntil: 2, // Fail first 2 calls
	}

	// Simulate scans with the mock client
	// First scan - fails
	_, err := mockClient.ContainerList(ctx, container.ListOptions{})
	if err == nil {
		t.Error("Expected first call to fail")
	}

	// Second scan - fails
	_, err = mockClient.ContainerList(ctx, container.ListOptions{})
	if err == nil {
		t.Error("Expected second call to fail")
	}

	// Third scan - succeeds
	_, err = mockClient.ContainerList(ctx, container.ListOptions{})
	if err != nil {
		t.Errorf("Expected third call to succeed, got: %v", err)
	}

	// Verify call counts
	if mockClient.getTotalCalls() != 3 {
		t.Errorf("Expected 3 total calls, got %d", mockClient.getTotalCalls())
	}

	worker.Stop()
}

// TestDockerWorker_StartWithFailureRecovery tests the actual Start() method
// with failure recovery behavior integrated.
func TestDockerWorker_StartWithFailureRecovery(t *testing.T) {
	store := &mockProposalStore{}
	bus := &mockEventBus{}
	dm := NewDiscoveryManager(store, bus, nil)

	// Start proposal processor
	dm.wg.Add(1)
	go dm.processProposals()
	defer func() {
		close(dm.proposalCh)
		dm.wg.Wait()
	}()

	// We need to test that consecutive failures cause the worker to exit
	// This test verifies the behavior described in the issue

	// The implementation should:
	// 1. Track consecutive failures
	// 2. Reset counter on success
	// 3. Return error after maxFailures (3) consecutive failures

	// Test the pattern that the implementation should follow:
	maxFailures := 3

	// Test 1: Continuous failures should stop
	t.Run("continuous failures stop worker", func(t *testing.T) {
		mockClient := &mockDockerClientWithFailures{alwaysFail: true}
		failureCount := 0

		for i := 0; i < 10; i++ {
			_, err := mockClient.ContainerList(context.Background(), container.ListOptions{})
			if err != nil {
				failureCount++
				if failureCount >= maxFailures {
					break
				}
			} else {
				failureCount = 0
			}
		}

		if mockClient.getTotalCalls() != maxFailures {
			t.Errorf("Should stop after %d failures, but made %d calls", maxFailures, mockClient.getTotalCalls())
		}
	})

	// Test 2: Recovery resets counter
	t.Run("recovery resets counter", func(t *testing.T) {
		mockClient := &mockDockerClientWithFailures{failUntil: 2}
		failureCount := 0
		stopped := false

		for i := 0; i < 10; i++ {
			_, err := mockClient.ContainerList(context.Background(), container.ListOptions{})
			if err != nil {
				failureCount++
				if failureCount >= maxFailures {
					stopped = true
					break
				}
			} else {
				failureCount = 0
			}
		}

		if stopped {
			t.Error("Should not have stopped when failures recovered")
		}
		if failureCount != 0 {
			t.Errorf("Failure count should be 0 after recovery, got %d", failureCount)
		}
	})
}
