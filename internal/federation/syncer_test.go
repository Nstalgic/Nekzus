package federation

import (
	"sync"
	"testing"
	"time"

	"github.com/nstalgic/nekzus/internal/types"
)

// TestCatalogSyncer_NewCatalogSyncer tests creating a new syncer
func TestCatalogSyncer_NewCatalogSyncer(t *testing.T) {
	syncer := NewCatalogSyncer("peer1", nil)

	if syncer == nil {
		t.Fatal("NewCatalogSyncer() returned nil")
	}

	if syncer.localPeerID != "peer1" {
		t.Errorf("Expected localPeerID = 'peer1', got '%s'", syncer.localPeerID)
	}

	if syncer.vectorClock == nil {
		t.Error("Vector clock should be initialized")
	}

	if len(syncer.vectorClock) != 0 {
		t.Error("Vector clock should start empty")
	}
}

// TestCatalogSyncer_SetBroadcastFunc tests setting broadcast callback
func TestCatalogSyncer_SetBroadcastFunc(t *testing.T) {
	syncer := NewCatalogSyncer("peer1", nil)

	called := false
	syncer.SetBroadcastFunc(func(msg []byte) {
		called = true
	})

	// Trigger a broadcast
	app := &types.App{
		ID:   "test-app",
		Name: "Test App",
	}

	syncer.OnLocalServiceAdded(app)

	if !called {
		t.Error("Broadcast function should have been called")
	}
}

// TestCatalogSyncer_OnLocalServiceAdded tests adding local services
func TestCatalogSyncer_OnLocalServiceAdded(t *testing.T) {
	syncer := NewCatalogSyncer("peer1", nil)

	var broadcastedMsg []byte
	syncer.SetBroadcastFunc(func(msg []byte) {
		broadcastedMsg = msg
	})

	app := &types.App{
		ID:   "app1",
		Name: "App 1",
		Icon: "🌐",
	}

	err := syncer.OnLocalServiceAdded(app)
	if err != nil {
		t.Fatalf("OnLocalServiceAdded() failed: %v", err)
	}

	// Check vector clock was incremented
	if syncer.vectorClock["peer1"] != 1 {
		t.Errorf("Expected clock[peer1] = 1, got %d", syncer.vectorClock["peer1"])
	}

	// Check broadcast was sent
	if broadcastedMsg == nil {
		t.Error("Expected broadcast message to be sent")
	}

	// Second service should increment clock
	app2 := &types.App{ID: "app2", Name: "App 2"}
	syncer.OnLocalServiceAdded(app2)

	if syncer.vectorClock["peer1"] != 2 {
		t.Errorf("Expected clock[peer1] = 2, got %d", syncer.vectorClock["peer1"])
	}
}

// TestCatalogSyncer_OnLocalServiceUpdated tests updating local services
func TestCatalogSyncer_OnLocalServiceUpdated(t *testing.T) {
	syncer := NewCatalogSyncer("peer1", nil)

	var broadcastCount int
	syncer.SetBroadcastFunc(func(msg []byte) {
		broadcastCount++
	})

	app := &types.App{ID: "app1", Name: "App 1"}

	// Add service
	syncer.OnLocalServiceAdded(app)
	if broadcastCount != 1 {
		t.Errorf("Expected 1 broadcast, got %d", broadcastCount)
	}

	// Update service
	app.Name = "App 1 Updated"
	syncer.OnLocalServiceUpdated(app)

	if broadcastCount != 2 {
		t.Errorf("Expected 2 broadcasts, got %d", broadcastCount)
	}

	if syncer.vectorClock["peer1"] != 2 {
		t.Errorf("Expected clock[peer1] = 2, got %d", syncer.vectorClock["peer1"])
	}
}

// TestCatalogSyncer_OnLocalServiceDeleted tests deleting local services
func TestCatalogSyncer_OnLocalServiceDeleted(t *testing.T) {
	syncer := NewCatalogSyncer("peer1", nil)

	var broadcastCount int
	syncer.SetBroadcastFunc(func(msg []byte) {
		broadcastCount++
	})

	err := syncer.OnLocalServiceDeleted("app1")
	if err != nil {
		t.Fatalf("OnLocalServiceDeleted() failed: %v", err)
	}

	if broadcastCount != 1 {
		t.Errorf("Expected 1 broadcast, got %d", broadcastCount)
	}

	if syncer.vectorClock["peer1"] != 1 {
		t.Errorf("Expected clock[peer1] = 1, got %d", syncer.vectorClock["peer1"])
	}
}

// TestCatalogSyncer_OnRemoteServiceUpdate tests receiving remote updates
func TestCatalogSyncer_OnRemoteServiceUpdate(t *testing.T) {
	syncer := NewCatalogSyncer("peer1", nil)

	remoteService := &FederatedService{
		ServiceID:    "remote-app1",
		OriginPeerID: "peer2",
		App: &types.App{
			ID:   "remote-app1",
			Name: "Remote App 1",
		},
		VectorClock: VectorClock{"peer2": 5},
		Tombstone:   false,
		LastSeen:    time.Now(),
	}

	err := syncer.OnRemoteServiceUpdate(remoteService)
	if err != nil {
		t.Fatalf("OnRemoteServiceUpdate() failed: %v", err)
	}

	// Check vector clock was merged
	if syncer.vectorClock["peer2"] != 5 {
		t.Errorf("Expected clock[peer2] = 5, got %d", syncer.vectorClock["peer2"])
	}
}

// TestCatalogSyncer_ConflictResolution tests conflict handling on remote updates
func TestCatalogSyncer_ConflictResolution(t *testing.T) {
	tests := []struct {
		name            string
		localPeerID     string
		existingService *FederatedService
		remoteService   *FederatedService
		expectedMerge   bool // Should remote be merged?
	}{
		{
			name:        "remote wins - newer vector clock",
			localPeerID: "peer1",
			existingService: &FederatedService{
				ServiceID:    "app1",
				OriginPeerID: "peer2",
				VectorClock:  VectorClock{"peer2": 3},
				LastSeen:     time.Now().Add(-1 * time.Hour),
			},
			remoteService: &FederatedService{
				ServiceID:    "app1",
				OriginPeerID: "peer2",
				VectorClock:  VectorClock{"peer2": 5},
				LastSeen:     time.Now(),
			},
			expectedMerge: true,
		},
		{
			name:        "local wins - newer vector clock",
			localPeerID: "peer1",
			existingService: &FederatedService{
				ServiceID:    "app1",
				OriginPeerID: "peer2",
				VectorClock:  VectorClock{"peer2": 7},
				LastSeen:     time.Now(),
			},
			remoteService: &FederatedService{
				ServiceID:    "app1",
				OriginPeerID: "peer2",
				VectorClock:  VectorClock{"peer2": 5},
				LastSeen:     time.Now().Add(-1 * time.Hour),
			},
			expectedMerge: false,
		},
		{
			name:            "new service - should merge",
			localPeerID:     "peer1",
			existingService: nil,
			remoteService: &FederatedService{
				ServiceID:    "app1",
				OriginPeerID: "peer2",
				VectorClock:  VectorClock{"peer2": 1},
				LastSeen:     time.Now(),
			},
			expectedMerge: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			syncer := NewCatalogSyncer(tt.localPeerID, nil)

			// Store initial state if provided
			// Note: Without storage, we can't test full integration
			// This tests the logic flow

			err := syncer.OnRemoteServiceUpdate(tt.remoteService)
			if err != nil {
				t.Fatalf("OnRemoteServiceUpdate() failed: %v", err)
			}

			// Verify vector clock merge happened if expected
			if tt.expectedMerge {
				for peer, count := range tt.remoteService.VectorClock {
					if syncer.vectorClock[peer] != count {
						t.Errorf("Expected clock[%s] = %d, got %d", peer, count, syncer.vectorClock[peer])
					}
				}
			}
		})
	}
}

// TestCatalogSyncer_GetFederatedCatalog tests retrieving catalog
func TestCatalogSyncer_GetFederatedCatalog(t *testing.T) {
	syncer := NewCatalogSyncer("peer1", nil)

	// Without storage, should return empty list
	catalog, err := syncer.GetFederatedCatalog()
	if err != nil {
		t.Fatalf("GetFederatedCatalog() failed: %v", err)
	}

	if catalog == nil {
		t.Error("Catalog should not be nil")
	}

	if len(catalog) != 0 {
		t.Errorf("Expected empty catalog, got %d items", len(catalog))
	}
}

// TestCatalogSyncer_NoBroadcastFunc tests operations without broadcast callback
func TestCatalogSyncer_NoBroadcastFunc(t *testing.T) {
	syncer := NewCatalogSyncer("peer1", nil)

	app := &types.App{ID: "app1", Name: "App 1"}

	// Should not panic without broadcast function
	err := syncer.OnLocalServiceAdded(app)
	if err != nil {
		t.Fatalf("OnLocalServiceAdded() should not fail without broadcast func: %v", err)
	}

	err = syncer.OnLocalServiceUpdated(app)
	if err != nil {
		t.Fatalf("OnLocalServiceUpdated() should not fail without broadcast func: %v", err)
	}

	err = syncer.OnLocalServiceDeleted("app1")
	if err != nil {
		t.Fatalf("OnLocalServiceDeleted() should not fail without broadcast func: %v", err)
	}
}

// TestCatalogSyncer_ThreadSafety tests concurrent access
func TestCatalogSyncer_ThreadSafety(t *testing.T) {
	syncer := NewCatalogSyncer("peer1", nil)

	syncer.SetBroadcastFunc(func(msg []byte) {
		// No-op
	})

	// Run multiple concurrent operations
	done := make(chan bool)

	// Add services
	go func() {
		for i := 0; i < 10; i++ {
			app := &types.App{ID: "app1", Name: "App 1"}
			syncer.OnLocalServiceAdded(app)
		}
		done <- true
	}()

	// Update services
	go func() {
		for i := 0; i < 10; i++ {
			app := &types.App{ID: "app2", Name: "App 2"}
			syncer.OnLocalServiceUpdated(app)
		}
		done <- true
	}()

	// Remote updates
	go func() {
		for i := 0; i < 10; i++ {
			remote := &FederatedService{
				ServiceID:    "remote-app",
				OriginPeerID: "peer2",
				App:          &types.App{ID: "remote-app", Name: "Remote"},
				VectorClock:  VectorClock{"peer2": uint64(i + 1)},
				LastSeen:     time.Now(),
			}
			syncer.OnRemoteServiceUpdate(remote)
		}
		done <- true
	}()

	// Get catalog
	go func() {
		for i := 0; i < 10; i++ {
			syncer.GetFederatedCatalog()
		}
		done <- true
	}()

	// Wait for all goroutines
	for i := 0; i < 4; i++ {
		<-done
	}

	// Should not panic - test passes if we get here
}

// TestCatalogSyncer_GetLocalVectorClock tests getting a copy of the vector clock
func TestCatalogSyncer_GetLocalVectorClock(t *testing.T) {
	syncer := NewCatalogSyncer("peer1", nil)
	syncer.SetBroadcastFunc(func(msg []byte) {})

	// Add some services to increment the clock
	syncer.OnLocalServiceAdded(&types.App{ID: "app1", Name: "App 1"})
	syncer.OnLocalServiceAdded(&types.App{ID: "app2", Name: "App 2"})

	// Get vector clock
	clock := syncer.GetLocalVectorClock()

	if clock == nil {
		t.Fatal("GetLocalVectorClock() returned nil")
	}

	if clock["peer1"] != 2 {
		t.Errorf("Expected clock[peer1] = 2, got %d", clock["peer1"])
	}

	// Verify it's a copy (modifying returned clock shouldn't affect internal)
	clock["peer1"] = 100
	internalClock := syncer.GetLocalVectorClock()
	if internalClock["peer1"] == 100 {
		t.Error("GetLocalVectorClock() should return a copy, not the original")
	}
}

// TestCatalogSyncer_GetAllServices tests getting all services including tombstones
func TestCatalogSyncer_GetAllServices(t *testing.T) {
	syncer := NewCatalogSyncer("peer1", nil)

	// Without storage, should return empty list
	services, err := syncer.GetAllServices()
	if err != nil {
		t.Fatalf("GetAllServices() failed: %v", err)
	}

	if services == nil {
		t.Error("GetAllServices() should return empty slice, not nil")
	}

	if len(services) != 0 {
		t.Errorf("Expected empty slice, got %d items", len(services))
	}
}

// TestCatalogSyncer_MergeRemoteServices tests bulk merging of remote services
func TestCatalogSyncer_MergeRemoteServices(t *testing.T) {
	syncer := NewCatalogSyncer("peer1", nil)

	// Merge multiple remote services
	remoteServices := []*FederatedService{
		{
			ServiceID:    "app1",
			OriginPeerID: "peer2",
			App:          &types.App{ID: "app1", Name: "App 1"},
			VectorClock:  VectorClock{"peer2": 5},
			LastSeen:     time.Now(),
		},
		{
			ServiceID:    "app2",
			OriginPeerID: "peer3",
			App:          &types.App{ID: "app2", Name: "App 2"},
			VectorClock:  VectorClock{"peer3": 3},
			LastSeen:     time.Now(),
		},
	}

	err := syncer.MergeRemoteServices(remoteServices)
	if err != nil {
		t.Fatalf("MergeRemoteServices() failed: %v", err)
	}

	// Vector clocks should be merged
	clock := syncer.GetLocalVectorClock()
	if clock["peer2"] != 5 {
		t.Errorf("Expected clock[peer2] = 5, got %d", clock["peer2"])
	}
	if clock["peer3"] != 3 {
		t.Errorf("Expected clock[peer3] = 3, got %d", clock["peer3"])
	}
}

// TestCatalogSyncer_MergeRemoteServices_Empty tests merging empty service list
func TestCatalogSyncer_MergeRemoteServices_Empty(t *testing.T) {
	syncer := NewCatalogSyncer("peer1", nil)

	err := syncer.MergeRemoteServices([]*FederatedService{})
	if err != nil {
		t.Fatalf("MergeRemoteServices() with empty list should not fail: %v", err)
	}

	err = syncer.MergeRemoteServices(nil)
	if err != nil {
		t.Fatalf("MergeRemoteServices() with nil should not fail: %v", err)
	}
}

// TestCatalogSyncer_SetEventCallback tests setting event callback
func TestCatalogSyncer_SetEventCallback(t *testing.T) {
	syncer := NewCatalogSyncer("peer1", nil)
	syncer.SetBroadcastFunc(func(msg []byte) {})

	var receivedEvents []string
	syncer.SetEventCallback(func(eventType string, data interface{}) {
		receivedEvents = append(receivedEvents, eventType)
	})

	// Add a remote service - should trigger event
	remoteService := &FederatedService{
		ServiceID:    "remote-app1",
		OriginPeerID: "peer2",
		App:          &types.App{ID: "remote-app1", Name: "Remote App 1"},
		VectorClock:  VectorClock{"peer2": 5},
		LastSeen:     time.Now(),
	}

	err := syncer.OnRemoteServiceUpdate(remoteService)
	if err != nil {
		t.Fatalf("OnRemoteServiceUpdate() failed: %v", err)
	}

	// Should have received a service_added event
	if len(receivedEvents) != 1 {
		t.Errorf("Expected 1 event, got %d", len(receivedEvents))
	}
	if len(receivedEvents) > 0 && receivedEvents[0] != "federation_service_added" {
		t.Errorf("Expected federation_service_added event, got %s", receivedEvents[0])
	}
}

// TestCatalogSyncer_EventCallback_OnMergeRemoteServices tests events during bulk merge
func TestCatalogSyncer_EventCallback_OnMergeRemoteServices(t *testing.T) {
	syncer := NewCatalogSyncer("peer1", nil)

	var receivedEvents []string
	syncer.SetEventCallback(func(eventType string, data interface{}) {
		receivedEvents = append(receivedEvents, eventType)
	})

	// Merge multiple remote services
	remoteServices := []*FederatedService{
		{
			ServiceID:    "app1",
			OriginPeerID: "peer2",
			App:          &types.App{ID: "app1", Name: "App 1"},
			VectorClock:  VectorClock{"peer2": 5},
			LastSeen:     time.Now(),
		},
		{
			ServiceID:    "app2",
			OriginPeerID: "peer3",
			App:          &types.App{ID: "app2", Name: "App 2"},
			VectorClock:  VectorClock{"peer3": 3},
			LastSeen:     time.Now(),
		},
	}

	err := syncer.MergeRemoteServices(remoteServices)
	if err != nil {
		t.Fatalf("MergeRemoteServices() failed: %v", err)
	}

	// Should have received events for each service
	if len(receivedEvents) != 2 {
		t.Errorf("Expected 2 events, got %d: %v", len(receivedEvents), receivedEvents)
	}
}

// TestCatalogSyncer_MergeRemoteServices_SkipsSelf tests that services from self are skipped
func TestCatalogSyncer_MergeRemoteServices_SkipsSelf(t *testing.T) {
	syncer := NewCatalogSyncer("peer1", nil)

	initialClock := syncer.GetLocalVectorClock()

	// Try to merge a service that originated from ourselves
	selfServices := []*FederatedService{
		{
			ServiceID:    "app1",
			OriginPeerID: "peer1", // Same as local peer
			App:          &types.App{ID: "app1", Name: "App 1"},
			VectorClock:  VectorClock{"peer1": 100},
			LastSeen:     time.Now(),
		},
	}

	err := syncer.MergeRemoteServices(selfServices)
	if err != nil {
		t.Fatalf("MergeRemoteServices() failed: %v", err)
	}

	// Vector clock should NOT have been merged (we skip our own services)
	currentClock := syncer.GetLocalVectorClock()
	if currentClock["peer1"] != initialClock["peer1"] {
		t.Errorf("Should not merge services originating from self")
	}
}

// mockEventBus implements EventBus for testing (thread-safe)
type mockEventBus struct {
	mu     sync.Mutex
	events []struct {
		eventType string
		data      interface{}
	}
}

func (m *mockEventBus) Publish(eventType string, data interface{}) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, struct {
		eventType string
		data      interface{}
	}{eventType, data})
}

func (m *mockEventBus) EventCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.events)
}

func (m *mockEventBus) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = nil
}

// TestPeerManager_EventCallbackWiring tests that NewPeerManager wires the event callback
func TestPeerManager_EventCallbackWiring(t *testing.T) {
	eventBus := &mockEventBus{}

	config := Config{
		LocalPeerID:         "nxs_test_events",
		LocalPeerName:       "Test Instance",
		APIAddress:          "localhost:8080",
		GossipBindAddr:      "0.0.0.0",
		GossipBindPort:      19960,
		GossipAdvertiseAddr: "127.0.0.1",
		GossipAdvertisePort: 19960,
		ClusterSecret:       "test-secret-32-characters-long!!",
		MDNSEnabled:         false,
		BootstrapPeers:      []string{},
		FullSyncInterval:    1 * time.Minute,
		AntiEntropyPeriod:   1 * time.Minute,
		PeerTimeout:         30 * time.Second,
	}

	pm, err := NewPeerManager(config, nil, eventBus, nil)
	if err != nil {
		t.Fatalf("NewPeerManager() failed: %v", err)
	}

	// Trigger an event through the syncer
	remoteService := &FederatedService{
		ServiceID:    "remote-app1",
		OriginPeerID: "peer2",
		App:          &types.App{ID: "remote-app1", Name: "Remote App 1"},
		VectorClock:  VectorClock{"peer2": 5},
		LastSeen:     time.Now(),
	}

	err = pm.catalogSyncer.OnRemoteServiceUpdate(remoteService)
	if err != nil {
		t.Fatalf("OnRemoteServiceUpdate() failed: %v", err)
	}

	// Verify event was published to the EventBus
	if len(eventBus.events) != 1 {
		t.Errorf("Expected 1 event, got %d", len(eventBus.events))
	}

	if len(eventBus.events) > 0 && eventBus.events[0].eventType != "federation_service_added" {
		t.Errorf("Expected federation_service_added event, got %s", eventBus.events[0].eventType)
	}
}

// TestPeerManager_EventCallbackWiring_NilEventBus tests that nil EventBus doesn't cause issues
func TestPeerManager_EventCallbackWiring_NilEventBus(t *testing.T) {
	config := Config{
		LocalPeerID:         "nxs_test_nil_events",
		LocalPeerName:       "Test Instance",
		APIAddress:          "localhost:8080",
		GossipBindAddr:      "0.0.0.0",
		GossipBindPort:      19961,
		GossipAdvertiseAddr: "127.0.0.1",
		GossipAdvertisePort: 19961,
		ClusterSecret:       "test-secret-32-characters-long!!",
		MDNSEnabled:         false,
		BootstrapPeers:      []string{},
		FullSyncInterval:    1 * time.Minute,
		AntiEntropyPeriod:   1 * time.Minute,
		PeerTimeout:         30 * time.Second,
	}

	pm, err := NewPeerManager(config, nil, nil, nil)
	if err != nil {
		t.Fatalf("NewPeerManager() failed: %v", err)
	}

	// Trigger an event through the syncer - should not panic
	remoteService := &FederatedService{
		ServiceID:    "remote-app1",
		OriginPeerID: "peer2",
		App:          &types.App{ID: "remote-app1", Name: "Remote App 1"},
		VectorClock:  VectorClock{"peer2": 5},
		LastSeen:     time.Now(),
	}

	err = pm.catalogSyncer.OnRemoteServiceUpdate(remoteService)
	if err != nil {
		t.Fatalf("OnRemoteServiceUpdate() should not fail with nil EventBus: %v", err)
	}
}
