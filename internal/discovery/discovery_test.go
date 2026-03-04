package discovery

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/nstalgic/nekzus/internal/types"
)

// Mock implementations for testing

type mockProposalStore struct {
	mu        sync.Mutex
	proposals []*types.Proposal
}

func (m *mockProposalStore) AddProposal(p *types.Proposal) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.proposals = append(m.proposals, p)
}

func (m *mockProposalStore) GetProposals() []*types.Proposal {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]*types.Proposal, len(m.proposals))
	copy(result, m.proposals)
	return result
}

type mockEventBus struct {
	mu     sync.Mutex
	events []Event
}

func (m *mockEventBus) Publish(evt Event) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, evt)
}

func (m *mockEventBus) GetEvents() []Event {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]Event, len(m.events))
	copy(result, m.events)
	return result
}

type mockWorker struct {
	name    string
	started bool
	stopped bool
	startCh chan struct{}
}

func (w *mockWorker) Name() string {
	return w.name
}

func (w *mockWorker) Start(ctx context.Context) error {
	w.started = true
	close(w.startCh)
	<-ctx.Done()
	return nil
}

func (w *mockWorker) Stop() error {
	w.stopped = true
	return nil
}

func newMockWorker(name string) *mockWorker {
	return &mockWorker{
		name:    name,
		startCh: make(chan struct{}),
	}
}

// Tests

func TestNewDiscoveryManager(t *testing.T) {
	store := &mockProposalStore{}
	bus := &mockEventBus{}

	dm := NewDiscoveryManager(store, bus, nil)
	if dm == nil {
		t.Fatal("Expected non-nil DiscoveryManager")
	}

	if dm.proposalStore != store {
		t.Error("ProposalStore not set correctly")
	}
	if dm.eventBus != bus {
		t.Error("EventBus not set correctly")
	}
	if dm.proposals == nil {
		t.Error("Proposals map should be initialized")
	}
}

func TestRegisterWorker(t *testing.T) {
	store := &mockProposalStore{}
	bus := &mockEventBus{}
	dm := NewDiscoveryManager(store, bus, nil)

	worker1 := newMockWorker("worker1")
	worker2 := newMockWorker("worker2")

	dm.RegisterWorker(worker1)
	dm.RegisterWorker(worker2)

	dm.mu.RLock()
	workerCount := len(dm.workers)
	dm.mu.RUnlock()

	if workerCount != 2 {
		t.Errorf("Expected 2 workers, got %d", workerCount)
	}
}

func TestDiscoveryManagerStartStop(t *testing.T) {
	store := &mockProposalStore{}
	bus := &mockEventBus{}
	dm := NewDiscoveryManager(store, bus, nil)

	worker := newMockWorker("test-worker")
	dm.RegisterWorker(worker)

	// Start discovery
	err := dm.Start()
	if err != nil {
		t.Fatalf("Failed to start discovery: %v", err)
	}

	// Wait for worker to start
	select {
	case <-worker.startCh:
		// Worker started
	case <-time.After(1 * time.Second):
		t.Fatal("Worker did not start in time")
	}

	if !worker.started {
		t.Error("Worker should be started")
	}

	// Stop discovery
	err = dm.Stop()
	if err != nil {
		t.Fatalf("Failed to stop discovery: %v", err)
	}

	if !worker.stopped {
		t.Error("Worker should be stopped")
	}
}

func TestSubmitProposal(t *testing.T) {
	store := &mockProposalStore{}
	bus := &mockEventBus{}
	dm := NewDiscoveryManager(store, bus, nil)

	// Start the proposal processor
	err := dm.Start()
	if err != nil {
		t.Fatalf("Failed to start discovery: %v", err)
	}
	defer dm.Stop()

	proposal := &types.Proposal{
		ID:             "test-proposal-1",
		Source:         "test",
		DetectedScheme: "http",
		DetectedHost:   "localhost",
		DetectedPort:   8080,
		Confidence:     0.9,
		SuggestedApp: types.App{
			ID:   "test-app",
			Name: "Test App",
		},
		SuggestedRoute: types.Route{
			RouteID:  "route-test",
			AppID:    "test-app",
			PathBase: "/apps/test/",
			To:       "http://localhost:8080",
		},
	}

	dm.SubmitProposal(proposal)

	// Wait for proposal to be processed
	time.Sleep(100 * time.Millisecond)

	// Check that proposal was added to store
	proposals := store.GetProposals()
	if len(proposals) != 1 {
		t.Fatalf("Expected 1 proposal in store, got %d", len(proposals))
	}
	if proposals[0].ID != "test-proposal-1" {
		t.Errorf("Expected proposal ID test-proposal-1, got %s", proposals[0].ID)
	}

	// Check that event was published
	events := bus.GetEvents()
	if len(events) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(events))
	}
	if events[0].GetType() != "discovery.proposal" {
		t.Errorf("Expected event type discovery.proposal, got %s", events[0].GetType())
	}
}

func TestProposalDeduplication(t *testing.T) {
	store := &mockProposalStore{}
	bus := &mockEventBus{}
	dm := NewDiscoveryManager(store, bus, nil)

	err := dm.Start()
	if err != nil {
		t.Fatalf("Failed to start discovery: %v", err)
	}
	defer dm.Stop()

	proposal1 := &types.Proposal{
		ID:         "dup-test",
		Source:     "test",
		Confidence: 0.5,
	}

	proposal2 := &types.Proposal{
		ID:         "dup-test", // Same ID
		Source:     "test",
		Confidence: 0.8, // Higher confidence
	}

	dm.SubmitProposal(proposal1)
	time.Sleep(50 * time.Millisecond)
	dm.SubmitProposal(proposal2)
	time.Sleep(50 * time.Millisecond)

	// Should only have one proposal in store (first one)
	proposals := store.GetProposals()
	if len(proposals) != 1 {
		t.Fatalf("Expected 1 proposal (deduplicated), got %d", len(proposals))
	}

	// Should only have one event (first proposal)
	events := bus.GetEvents()
	if len(events) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(events))
	}

	// Internal manager should have updated confidence
	internalProposals := dm.GetProposals()
	if len(internalProposals) != 1 {
		t.Fatalf("Expected 1 internal proposal, got %d", len(internalProposals))
	}
	if internalProposals[0].Confidence != 0.8 {
		t.Errorf("Expected confidence 0.8 (updated), got %f", internalProposals[0].Confidence)
	}
}

func TestGetProposals(t *testing.T) {
	store := &mockProposalStore{}
	bus := &mockEventBus{}
	dm := NewDiscoveryManager(store, bus, nil)

	err := dm.Start()
	if err != nil {
		t.Fatalf("Failed to start discovery: %v", err)
	}
	defer dm.Stop()

	// Submit multiple proposals
	for i := 1; i <= 3; i++ {
		proposal := &types.Proposal{
			ID:     string(rune('a' + i)),
			Source: "test",
		}
		dm.SubmitProposal(proposal)
	}

	time.Sleep(100 * time.Millisecond)

	proposals := dm.GetProposals()
	if len(proposals) != 3 {
		t.Errorf("Expected 3 proposals, got %d", len(proposals))
	}
}

func TestRemoveProposal(t *testing.T) {
	store := &mockProposalStore{}
	bus := &mockEventBus{}
	dm := NewDiscoveryManager(store, bus, nil)

	err := dm.Start()
	if err != nil {
		t.Fatalf("Failed to start discovery: %v", err)
	}
	defer dm.Stop()

	proposal := &types.Proposal{
		ID:     "remove-test",
		Source: "test",
	}

	dm.SubmitProposal(proposal)
	time.Sleep(50 * time.Millisecond)

	proposals := dm.GetProposals()
	if len(proposals) != 1 {
		t.Fatalf("Expected 1 proposal before removal, got %d", len(proposals))
	}

	dm.RemoveProposal("remove-test")

	proposals = dm.GetProposals()
	if len(proposals) != 0 {
		t.Errorf("Expected 0 proposals after removal, got %d", len(proposals))
	}
}

func TestGenerateProposalID(t *testing.T) {
	tests := []struct {
		source string
		scheme string
		host   string
		port   int
		want   string
	}{
		{"docker", "http", "container-abc", 8080, "proposal_docker_http_container_abc_8080"},
		{"mdns", "https", "service.local", 443, "proposal_mdns_https_service_local_443"},
		{"test", "http", "192.168.1.100", 3000, "proposal_test_http_192_168_1_100_3000"},
	}

	for _, tt := range tests {
		got := generateProposalID(tt.source, tt.scheme, tt.host, tt.port)
		if got != tt.want {
			t.Errorf("generateProposalID(%q, %q, %q, %d) = %q, want %q",
				tt.source, tt.scheme, tt.host, tt.port, got, tt.want)
		}
	}
}

func TestSanitize(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"hello-world", "hello_world"},
		{"test.service.local", "test_service_local"},
		{"192.168.1.100", "192_168_1_100"},
		{"app_name", "app_name"},
		{"CamelCase", "CamelCase"},
		{"with@special#chars!", "with_special_chars_"},
	}

	for _, tt := range tests {
		got := sanitize(tt.input)
		if got != tt.want {
			t.Errorf("sanitize(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestDebouncer(t *testing.T) {
	d := newDebouncer(100 * time.Millisecond)

	// First call should process
	if !d.ShouldProcess("key1") {
		t.Error("First call should be allowed to process")
	}

	// Immediate second call should be blocked
	if d.ShouldProcess("key1") {
		t.Error("Immediate second call should be blocked")
	}

	// Different key should process
	if !d.ShouldProcess("key2") {
		t.Error("Different key should be allowed to process")
	}

	// After interval, should process again
	time.Sleep(150 * time.Millisecond)
	if !d.ShouldProcess("key1") {
		t.Error("Call after interval should be allowed to process")
	}
}

func TestSimpleEvent(t *testing.T) {
	evt := simpleEvent{
		Type: "test.event",
		Data: map[string]string{"key": "value"},
	}

	if evt.GetType() != "test.event" {
		t.Errorf("Expected type test.event, got %s", evt.GetType())
	}

	data := evt.GetData()
	dataMap, ok := data.(map[string]string)
	if !ok {
		t.Fatal("Expected data to be map[string]string")
	}
	if dataMap["key"] != "value" {
		t.Errorf("Expected data key=value, got %v", dataMap)
	}
}

func TestConcurrentProposalSubmission(t *testing.T) {
	store := &mockProposalStore{}
	bus := &mockEventBus{}
	dm := NewDiscoveryManager(store, bus, nil)

	err := dm.Start()
	if err != nil {
		t.Fatalf("Failed to start discovery: %v", err)
	}
	defer dm.Stop()

	// Submit proposals concurrently
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			proposal := &types.Proposal{
				ID:     string(rune('a' + n)),
				Source: "concurrent",
			}
			dm.SubmitProposal(proposal)
		}(i)
	}

	wg.Wait()
	time.Sleep(200 * time.Millisecond)

	proposals := dm.GetProposals()
	if len(proposals) != 10 {
		t.Errorf("Expected 10 proposals, got %d", len(proposals))
	}
}

func TestDismissProposal(t *testing.T) {
	store := &mockProposalStore{}
	bus := &mockEventBus{}
	dm := NewDiscoveryManager(store, bus, nil)

	err := dm.Start()
	if err != nil {
		t.Fatalf("Failed to start discovery: %v", err)
	}
	defer dm.Stop()

	// Submit initial proposal
	proposal := &types.Proposal{
		ID:             "dismiss-test",
		Source:         "test",
		DetectedScheme: "http",
		DetectedHost:   "localhost",
		DetectedPort:   8080,
		Confidence:     0.9,
	}

	dm.SubmitProposal(proposal)
	time.Sleep(50 * time.Millisecond)

	// Verify proposal exists
	proposals := dm.GetProposals()
	if len(proposals) != 1 {
		t.Fatalf("Expected 1 proposal, got %d", len(proposals))
	}

	// Dismiss the proposal
	dm.DismissProposal("dismiss-test")

	// Verify proposal was removed
	proposals = dm.GetProposals()
	if len(proposals) != 0 {
		t.Errorf("Expected 0 proposals after dismissal, got %d", len(proposals))
	}

	// Submit same proposal again (simulating rediscovery)
	dm.SubmitProposal(proposal)
	time.Sleep(50 * time.Millisecond)

	// Verify proposal was NOT re-added (because it was dismissed)
	proposals = dm.GetProposals()
	if len(proposals) != 0 {
		t.Errorf("Expected 0 proposals after re-submission of dismissed proposal, got %d", len(proposals))
	}

	// Verify no new event was published for the dismissed proposal
	events := bus.GetEvents()
	if len(events) != 1 { // Only the first submission should have created an event
		t.Errorf("Expected 1 event (first submission only), got %d", len(events))
	}
}

func TestDismissedProposalsPersistAcrossScans(t *testing.T) {
	store := &mockProposalStore{}
	bus := &mockEventBus{}
	dm := NewDiscoveryManager(store, bus, nil)

	err := dm.Start()
	if err != nil {
		t.Fatalf("Failed to start discovery: %v", err)
	}
	defer dm.Stop()

	// Simulate multiple discovery scans with the same proposal ID
	proposal := &types.Proposal{
		ID:             "persistent-dismiss-test",
		Source:         "docker",
		DetectedScheme: "http",
		DetectedHost:   "test-container",
		DetectedPort:   3000,
		Confidence:     0.8,
	}

	// First scan - proposal is discovered
	dm.SubmitProposal(proposal)
	time.Sleep(50 * time.Millisecond)

	if len(dm.GetProposals()) != 1 {
		t.Fatalf("Expected 1 proposal after first scan, got %d", len(dm.GetProposals()))
	}

	// User dismisses the proposal
	dm.DismissProposal("persistent-dismiss-test")

	if len(dm.GetProposals()) != 0 {
		t.Fatalf("Expected 0 proposals after dismissal, got %d", len(dm.GetProposals()))
	}

	// Second scan - same container/service is discovered again (30 seconds later)
	dm.SubmitProposal(proposal)
	time.Sleep(50 * time.Millisecond)

	if len(dm.GetProposals()) != 0 {
		t.Errorf("Dismissed proposal should not reappear after subsequent scans, got %d proposals", len(dm.GetProposals()))
	}

	// Third scan - verify it stays dismissed
	dm.SubmitProposal(proposal)
	time.Sleep(50 * time.Millisecond)

	if len(dm.GetProposals()) != 0 {
		t.Errorf("Dismissed proposal should remain dismissed across multiple scans, got %d proposals", len(dm.GetProposals()))
	}
}

// Mock RouteChecker implementation for testing
type mockRouteChecker struct {
	mu         sync.Mutex
	routedApps map[string]bool
}

func newMockRouteChecker() *mockRouteChecker {
	return &mockRouteChecker{
		routedApps: make(map[string]bool),
	}
}

func (m *mockRouteChecker) HasRouteForApp(appID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.routedApps[appID]
}

func (m *mockRouteChecker) AddRoute(appID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.routedApps[appID] = true
}

func (m *mockRouteChecker) RemoveRoute(appID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.routedApps, appID)
}

// TestRouteCheckerFiltering tests that proposals for apps with existing routes are filtered out
func TestRouteCheckerFiltering(t *testing.T) {
	store := &mockProposalStore{}
	bus := &mockEventBus{}
	routeChecker := newMockRouteChecker()

	// Pre-configure some apps as having routes
	routeChecker.AddRoute("grafana")
	routeChecker.AddRoute("prometheus")

	dm := NewDiscoveryManager(store, bus, routeChecker)

	err := dm.Start()
	if err != nil {
		t.Fatalf("Failed to start discovery: %v", err)
	}
	defer dm.Stop()

	// Submit proposals for apps that already have routes
	proposalWithRoute := &types.Proposal{
		ID:             "filter-test-1",
		Source:         "docker",
		DetectedScheme: "http",
		DetectedHost:   "grafana",
		DetectedPort:   3000,
		Confidence:     0.9,
		SuggestedApp: types.App{
			ID:   "grafana",
			Name: "Grafana",
		},
	}

	dm.SubmitProposal(proposalWithRoute)
	time.Sleep(50 * time.Millisecond)

	// Proposal should be filtered out
	proposals := dm.GetProposals()
	if len(proposals) != 0 {
		t.Errorf("Expected 0 proposals (app already has route), got %d", len(proposals))
	}

	// Verify no event was published
	events := bus.GetEvents()
	if len(events) != 0 {
		t.Errorf("Expected 0 events (filtered proposal), got %d", len(events))
	}
}

// TestRouteCheckerAllowsNewApps tests that proposals for apps without routes are accepted
func TestRouteCheckerAllowsNewApps(t *testing.T) {
	store := &mockProposalStore{}
	bus := &mockEventBus{}
	routeChecker := newMockRouteChecker()

	// Pre-configure some apps as having routes
	routeChecker.AddRoute("grafana")

	dm := NewDiscoveryManager(store, bus, routeChecker)

	err := dm.Start()
	if err != nil {
		t.Fatalf("Failed to start discovery: %v", err)
	}
	defer dm.Stop()

	// Submit proposal for app without a route
	proposalNoRoute := &types.Proposal{
		ID:             "filter-test-2",
		Source:         "docker",
		DetectedScheme: "http",
		DetectedHost:   "uptime-kuma",
		DetectedPort:   3001,
		Confidence:     0.9,
		SuggestedApp: types.App{
			ID:   "uptime-kuma",
			Name: "Uptime Kuma",
		},
	}

	dm.SubmitProposal(proposalNoRoute)
	time.Sleep(50 * time.Millisecond)

	// Proposal should be accepted
	proposals := dm.GetProposals()
	if len(proposals) != 1 {
		t.Errorf("Expected 1 proposal (app has no route), got %d", len(proposals))
	}

	// Verify event was published
	events := bus.GetEvents()
	if len(events) != 1 {
		t.Errorf("Expected 1 event (accepted proposal), got %d", len(events))
	}
}

// TestRouteCheckerWithEmptyAppID tests that proposals with empty app ID bypass the route check
func TestRouteCheckerWithEmptyAppID(t *testing.T) {
	store := &mockProposalStore{}
	bus := &mockEventBus{}
	routeChecker := newMockRouteChecker()

	dm := NewDiscoveryManager(store, bus, routeChecker)

	err := dm.Start()
	if err != nil {
		t.Fatalf("Failed to start discovery: %v", err)
	}
	defer dm.Stop()

	// Submit proposal with empty app ID (e.g., unknown service)
	proposalNoAppID := &types.Proposal{
		ID:             "filter-test-3",
		Source:         "dns-sd",
		DetectedScheme: "http",
		DetectedHost:   "unknown-service.local",
		DetectedPort:   8080,
		Confidence:     0.5,
		SuggestedApp: types.App{
			ID:   "", // Empty app ID
			Name: "Unknown Service",
		},
	}

	dm.SubmitProposal(proposalNoAppID)
	time.Sleep(50 * time.Millisecond)

	// Proposal should be accepted (empty app ID bypasses route check)
	proposals := dm.GetProposals()
	if len(proposals) != 1 {
		t.Errorf("Expected 1 proposal (empty app ID bypasses check), got %d", len(proposals))
	}
}

// TestRouteCheckerNil tests that nil RouteChecker allows all proposals through
func TestRouteCheckerNil(t *testing.T) {
	store := &mockProposalStore{}
	bus := &mockEventBus{}

	// No route checker provided (nil)
	dm := NewDiscoveryManager(store, bus, nil)

	err := dm.Start()
	if err != nil {
		t.Fatalf("Failed to start discovery: %v", err)
	}
	defer dm.Stop()

	// Submit proposal
	proposal := &types.Proposal{
		ID:             "nil-checker-test",
		Source:         "docker",
		DetectedScheme: "http",
		DetectedHost:   "any-app",
		DetectedPort:   8080,
		Confidence:     0.9,
		SuggestedApp: types.App{
			ID:   "any-app",
			Name: "Any App",
		},
	}

	dm.SubmitProposal(proposal)
	time.Sleep(50 * time.Millisecond)

	// Proposal should be accepted (nil checker allows all)
	proposals := dm.GetProposals()
	if len(proposals) != 1 {
		t.Errorf("Expected 1 proposal (nil checker allows all), got %d", len(proposals))
	}
}

// TestRouteCheckerMixedProposals tests filtering a mix of apps with and without routes
func TestRouteCheckerMixedProposals(t *testing.T) {
	store := &mockProposalStore{}
	bus := &mockEventBus{}
	routeChecker := newMockRouteChecker()

	// Pre-configure some apps as having routes
	routeChecker.AddRoute("grafana")
	routeChecker.AddRoute("prometheus")

	dm := NewDiscoveryManager(store, bus, routeChecker)

	err := dm.Start()
	if err != nil {
		t.Fatalf("Failed to start discovery: %v", err)
	}
	defer dm.Stop()

	// Submit mix of proposals
	proposals := []*types.Proposal{
		{
			ID:           "mixed-1",
			Source:       "docker",
			SuggestedApp: types.App{ID: "grafana", Name: "Grafana"}, // Has route
		},
		{
			ID:           "mixed-2",
			Source:       "docker",
			SuggestedApp: types.App{ID: "uptime-kuma", Name: "Uptime Kuma"}, // No route
		},
		{
			ID:           "mixed-3",
			Source:       "docker",
			SuggestedApp: types.App{ID: "prometheus", Name: "Prometheus"}, // Has route
		},
		{
			ID:           "mixed-4",
			Source:       "docker",
			SuggestedApp: types.App{ID: "pihole", Name: "Pi-hole"}, // No route
		},
		{
			ID:           "mixed-5",
			Source:       "docker",
			SuggestedApp: types.App{ID: "", Name: "Unknown"}, // Empty ID - bypasses
		},
	}

	for _, p := range proposals {
		dm.SubmitProposal(p)
	}
	time.Sleep(100 * time.Millisecond)

	// Only 3 proposals should be accepted (uptime-kuma, pihole, unknown)
	accepted := dm.GetProposals()
	if len(accepted) != 3 {
		t.Errorf("Expected 3 proposals (2 filtered out), got %d", len(accepted))
	}

	// Verify the right proposals were accepted
	acceptedIDs := make(map[string]bool)
	for _, p := range accepted {
		acceptedIDs[p.ID] = true
	}

	if acceptedIDs["mixed-1"] {
		t.Error("grafana proposal should have been filtered")
	}
	if !acceptedIDs["mixed-2"] {
		t.Error("uptime-kuma proposal should have been accepted")
	}
	if acceptedIDs["mixed-3"] {
		t.Error("prometheus proposal should have been filtered")
	}
	if !acceptedIDs["mixed-4"] {
		t.Error("pihole proposal should have been accepted")
	}
	if !acceptedIDs["mixed-5"] {
		t.Error("unknown proposal (empty ID) should have been accepted")
	}
}

// Benchmark sanitize function to verify fast path optimization
func BenchmarkSanitize(b *testing.B) {
	tests := []struct {
		name  string
		input string
	}{
		{"fast path - no sanitization", "localhost"},
		{"fast path - alphanumeric", "service123"},
		{"slow path - with dots", "api.example.com"},
		{"slow path - mixed chars", "my-service@host:8080"},
	}

	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				sanitize(tt.input)
			}
		})
	}
}
