package storage

import (
	"os"
	"testing"
	"time"

	"github.com/nstalgic/nekzus/internal/types"
)

// setupTestStore creates a temporary database for testing
func setupTestStore(t *testing.T) (*Store, func()) {
	t.Helper()

	// Create temp database
	dbPath := t.TempDir() + "/test.db"

	store, err := NewStore(Config{
		DatabasePath: dbPath,
	})
	if err != nil {
		t.Fatalf("Failed to create test store: %v", err)
	}

	cleanup := func() {
		store.Close()
		os.Remove(dbPath)
	}

	return store, cleanup
}

func TestNewStore(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	if store == nil {
		t.Fatal("Expected store to be created")
	}
}

func TestConnectionPoolConfiguration(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	stats := store.db.Stats()

	// Test that connection pool is configured properly
	// MaxOpenConns should be 10 (increased from 3)
	if stats.MaxOpenConnections != 10 {
		t.Errorf("Expected MaxOpenConnections to be 10, got %d", stats.MaxOpenConnections)
	}

	// MaxIdleConns is internal but we can verify it's set by checking idle behavior
	// This will be validated indirectly through connection reuse
}

func TestConnectionPoolMetrics(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	// Perform some operations to use connections
	for i := 0; i < 5; i++ {
		_, err := store.db.Exec("SELECT 1")
		if err != nil {
			t.Fatalf("Query failed: %v", err)
		}
	}

	stats := store.db.Stats()

	// Verify connections are being tracked
	if stats.OpenConnections < 1 {
		t.Error("Expected at least 1 open connection")
	}

	// Verify connection reuse (should not open 5 new connections for 5 queries)
	if stats.OpenConnections > 5 {
		t.Errorf("Too many open connections: %d (connection pooling not working)", stats.OpenConnections)
	}
}

// Apps Tests

func TestSaveAndGetApp(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	app := types.App{
		ID:   "test-app",
		Name: "Test Application",
		Icon: "https://example.com/icon.png",
		Tags: []string{"web", "productivity"},
		Endpoints: map[string]string{
			"lan": "http://192.168.1.100:3000",
			"vpn": "http://10.0.0.5:3000",
		},
	}

	// Save app
	err := store.SaveApp(app)
	if err != nil {
		t.Fatalf("Failed to save app: %v", err)
	}

	// Retrieve app
	retrieved, err := store.GetApp("test-app")
	if err != nil {
		t.Fatalf("Failed to get app: %v", err)
	}

	if retrieved == nil {
		t.Fatal("Expected app to be retrieved")
	}

	if retrieved.ID != app.ID {
		t.Errorf("Expected ID %s, got %s", app.ID, retrieved.ID)
	}
	if retrieved.Name != app.Name {
		t.Errorf("Expected Name %s, got %s", app.Name, retrieved.Name)
	}
	if retrieved.Icon != app.Icon {
		t.Errorf("Expected Icon %s, got %s", app.Icon, retrieved.Icon)
	}
	if len(retrieved.Tags) != len(app.Tags) {
		t.Errorf("Expected %d tags, got %d", len(app.Tags), len(retrieved.Tags))
	}
	if len(retrieved.Endpoints) != len(app.Endpoints) {
		t.Errorf("Expected %d endpoints, got %d", len(app.Endpoints), len(retrieved.Endpoints))
	}
}

func TestUpdateApp(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	app := types.App{
		ID:   "test-app",
		Name: "Original Name",
	}

	// Save initial app
	err := store.SaveApp(app)
	if err != nil {
		t.Fatalf("Failed to save app: %v", err)
	}

	// Update app
	app.Name = "Updated Name"
	app.Icon = "https://example.com/new-icon.png"

	err = store.SaveApp(app)
	if err != nil {
		t.Fatalf("Failed to update app: %v", err)
	}

	// Retrieve updated app
	retrieved, err := store.GetApp("test-app")
	if err != nil {
		t.Fatalf("Failed to get app: %v", err)
	}

	if retrieved.Name != "Updated Name" {
		t.Errorf("Expected name to be updated to 'Updated Name', got %s", retrieved.Name)
	}
	if retrieved.Icon != "https://example.com/new-icon.png" {
		t.Errorf("Expected icon to be updated")
	}
}

func TestListApps(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	apps := []types.App{
		{ID: "app1", Name: "App 1"},
		{ID: "app2", Name: "App 2"},
		{ID: "app3", Name: "App 3"},
	}

	// Save multiple apps
	for _, app := range apps {
		if err := store.SaveApp(app); err != nil {
			t.Fatalf("Failed to save app: %v", err)
		}
	}

	// List apps
	retrieved, err := store.ListApps()
	if err != nil {
		t.Fatalf("Failed to list apps: %v", err)
	}

	if len(retrieved) != 3 {
		t.Errorf("Expected 3 apps, got %d", len(retrieved))
	}
}

func TestDeleteApp(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	app := types.App{ID: "test-app", Name: "Test App"}

	// Save app
	if err := store.SaveApp(app); err != nil {
		t.Fatalf("Failed to save app: %v", err)
	}

	// Delete app
	if err := store.DeleteApp("test-app"); err != nil {
		t.Fatalf("Failed to delete app: %v", err)
	}

	// Verify deletion
	retrieved, err := store.GetApp("test-app")
	if err != nil {
		t.Fatalf("Failed to get app: %v", err)
	}

	if retrieved != nil {
		t.Error("Expected app to be deleted")
	}
}

// Routes Tests

func TestSaveAndGetRoute(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	// Save app first (foreign key constraint)
	app := types.App{ID: "test-app", Name: "Test App"}
	if err := store.SaveApp(app); err != nil {
		t.Fatalf("Failed to save app: %v", err)
	}

	route := types.Route{
		RouteID:   "route-1",
		AppID:     "test-app",
		PathBase:  "/apps/test/",
		To:        "http://localhost:3000",
		Scopes:    []string{"read:test", "write:test"},
		Websocket: true,
	}

	// Save route
	err := store.SaveRoute(route)
	if err != nil {
		t.Fatalf("Failed to save route: %v", err)
	}

	// Retrieve route
	retrieved, err := store.GetRoute("route-1")
	if err != nil {
		t.Fatalf("Failed to get route: %v", err)
	}

	if retrieved == nil {
		t.Fatal("Expected route to be retrieved")
	}

	if retrieved.RouteID != route.RouteID {
		t.Errorf("Expected RouteID %s, got %s", route.RouteID, retrieved.RouteID)
	}
	if retrieved.PathBase != route.PathBase {
		t.Errorf("Expected PathBase %s, got %s", route.PathBase, retrieved.PathBase)
	}
	if retrieved.To != route.To {
		t.Errorf("Expected To %s, got %s", route.To, retrieved.To)
	}
	if !retrieved.Websocket {
		t.Error("Expected Websocket to be true")
	}
	if len(retrieved.Scopes) != len(route.Scopes) {
		t.Errorf("Expected %d scopes, got %d", len(route.Scopes), len(retrieved.Scopes))
	}
}

func TestListRoutes(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	// Save app first
	app := types.App{ID: "test-app", Name: "Test App"}
	if err := store.SaveApp(app); err != nil {
		t.Fatalf("Failed to save app: %v", err)
	}

	routes := []types.Route{
		{RouteID: "route-1", AppID: "test-app", PathBase: "/apps/test1/", To: "http://localhost:3001"},
		{RouteID: "route-2", AppID: "test-app", PathBase: "/apps/test2/", To: "http://localhost:3002"},
	}

	for _, route := range routes {
		if err := store.SaveRoute(route); err != nil {
			t.Fatalf("Failed to save route: %v", err)
		}
	}

	// List routes
	retrieved, err := store.ListRoutes()
	if err != nil {
		t.Fatalf("Failed to list routes: %v", err)
	}

	if len(retrieved) != 2 {
		t.Errorf("Expected 2 routes, got %d", len(retrieved))
	}
}

func TestDeleteRoute(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	// Save app and route
	app := types.App{ID: "test-app", Name: "Test App"}
	if err := store.SaveApp(app); err != nil {
		t.Fatalf("Failed to save app: %v", err)
	}

	route := types.Route{RouteID: "route-1", AppID: "test-app", PathBase: "/test/", To: "http://localhost:3000"}
	if err := store.SaveRoute(route); err != nil {
		t.Fatalf("Failed to save route: %v", err)
	}

	// Delete route
	if err := store.DeleteRoute("route-1"); err != nil {
		t.Fatalf("Failed to delete route: %v", err)
	}

	// Verify deletion
	retrieved, err := store.GetRoute("route-1")
	if err != nil {
		t.Fatalf("Failed to get route: %v", err)
	}

	if retrieved != nil {
		t.Error("Expected route to be deleted")
	}
}

func TestCascadeDeleteAppWithRoutes(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	// Save app
	app := types.App{ID: "test-app", Name: "Test App"}
	if err := store.SaveApp(app); err != nil {
		t.Fatalf("Failed to save app: %v", err)
	}

	// Save route
	route := types.Route{RouteID: "route-1", AppID: "test-app", PathBase: "/test/", To: "http://localhost:3000"}
	if err := store.SaveRoute(route); err != nil {
		t.Fatalf("Failed to save route: %v", err)
	}

	// Delete app (should cascade to routes)
	if err := store.DeleteApp("test-app"); err != nil {
		t.Fatalf("Failed to delete app: %v", err)
	}

	// Verify route was also deleted
	retrievedRoute, err := store.GetRoute("route-1")
	if err != nil {
		t.Fatalf("Failed to get route: %v", err)
	}

	if retrievedRoute != nil {
		t.Error("Expected route to be cascade deleted")
	}
}

// Proposals Tests

func TestSaveAndGetProposal(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	proposal := types.Proposal{
		ID:             "proposal-1",
		Source:         "docker",
		DetectedScheme: "http",
		DetectedHost:   "192.168.1.100",
		DetectedPort:   3000,
		Confidence:     0.95,
		SuggestedApp: types.App{
			ID:   "suggested-app",
			Name: "Suggested App",
		},
		SuggestedRoute: types.Route{
			RouteID:  "suggested-route",
			AppID:    "suggested-app",
			PathBase: "/apps/suggested/",
			To:       "http://192.168.1.100:3000",
		},
		Tags:          []string{"auto-discovered"},
		SecurityNotes: []string{"http-only"},
	}

	// Save proposal
	err := store.SaveProposal(proposal)
	if err != nil {
		t.Fatalf("Failed to save proposal: %v", err)
	}

	// Retrieve proposal
	retrieved, err := store.GetProposal("proposal-1")
	if err != nil {
		t.Fatalf("Failed to get proposal: %v", err)
	}

	if retrieved == nil {
		t.Fatal("Expected proposal to be retrieved")
	}

	if retrieved.ID != proposal.ID {
		t.Errorf("Expected ID %s, got %s", proposal.ID, retrieved.ID)
	}
	if retrieved.Source != proposal.Source {
		t.Errorf("Expected Source %s, got %s", proposal.Source, retrieved.Source)
	}
	if retrieved.Confidence != proposal.Confidence {
		t.Errorf("Expected Confidence %.2f, got %.2f", proposal.Confidence, retrieved.Confidence)
	}
	if retrieved.SuggestedApp.ID != proposal.SuggestedApp.ID {
		t.Errorf("Expected SuggestedApp ID %s, got %s", proposal.SuggestedApp.ID, retrieved.SuggestedApp.ID)
	}
}

func TestListProposals(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	proposals := []types.Proposal{
		{ID: "p1", Source: "docker", DetectedScheme: "http", DetectedHost: "host1", DetectedPort: 3001, Confidence: 0.8,
			SuggestedApp:   types.App{ID: "app1", Name: "App 1"},
			SuggestedRoute: types.Route{RouteID: "r1", AppID: "app1", PathBase: "/1/", To: "http://host1:3001"}},
		{ID: "p2", Source: "mdns", DetectedScheme: "https", DetectedHost: "host2", DetectedPort: 3002, Confidence: 0.9,
			SuggestedApp:   types.App{ID: "app2", Name: "App 2"},
			SuggestedRoute: types.Route{RouteID: "r2", AppID: "app2", PathBase: "/2/", To: "https://host2:3002"}},
	}

	for _, proposal := range proposals {
		if err := store.SaveProposal(proposal); err != nil {
			t.Fatalf("Failed to save proposal: %v", err)
		}
	}

	// List proposals
	retrieved, err := store.ListProposals()
	if err != nil {
		t.Fatalf("Failed to list proposals: %v", err)
	}

	if len(retrieved) != 2 {
		t.Errorf("Expected 2 proposals, got %d", len(retrieved))
	}
}

func TestDeleteProposal(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	proposal := types.Proposal{
		ID: "proposal-1", Source: "docker", DetectedScheme: "http",
		DetectedHost: "host1", DetectedPort: 3000, Confidence: 0.8,
		SuggestedApp:   types.App{ID: "app1", Name: "App 1"},
		SuggestedRoute: types.Route{RouteID: "r1", AppID: "app1", PathBase: "/1/", To: "http://host1:3000"},
	}

	if err := store.SaveProposal(proposal); err != nil {
		t.Fatalf("Failed to save proposal: %v", err)
	}

	// Delete proposal
	if err := store.DeleteProposal("proposal-1"); err != nil {
		t.Fatalf("Failed to delete proposal: %v", err)
	}

	// Verify deletion
	retrieved, err := store.GetProposal("proposal-1")
	if err != nil {
		t.Fatalf("Failed to get proposal: %v", err)
	}

	if retrieved != nil {
		t.Error("Expected proposal to be deleted")
	}
}

func TestCleanupStaleProposals(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	// Save a proposal
	proposal := types.Proposal{
		ID: "proposal-1", Source: "docker", DetectedScheme: "http",
		DetectedHost: "host1", DetectedPort: 3000, Confidence: 0.8,
		SuggestedApp:   types.App{ID: "app1", Name: "App 1"},
		SuggestedRoute: types.Route{RouteID: "r1", AppID: "app1", PathBase: "/1/", To: "http://host1:3000"},
	}

	if err := store.SaveProposal(proposal); err != nil {
		t.Fatalf("Failed to save proposal: %v", err)
	}

	// Manually update last_seen to be old
	_, err := store.db.Exec("UPDATE proposals SET last_seen = datetime('now', '-2 hours') WHERE id = ?", "proposal-1")
	if err != nil {
		t.Fatalf("Failed to update proposal timestamp: %v", err)
	}

	// Cleanup proposals older than 1 hour
	if err := store.CleanupStaleProposals(1 * time.Hour); err != nil {
		t.Fatalf("Failed to cleanup proposals: %v", err)
	}

	// Verify proposal was deleted
	retrieved, err := store.GetProposal("proposal-1")
	if err != nil {
		t.Fatalf("Failed to get proposal: %v", err)
	}

	if retrieved != nil {
		t.Error("Expected stale proposal to be cleaned up")
	}
}

// Devices Tests

func TestSaveAndGetDevice(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	// Save device
	err := store.SaveDevice("device-1", "My Phone", "", "", []string{"read:*", "write:apps"})
	if err != nil {
		t.Fatalf("Failed to save device: %v", err)
	}

	// Retrieve device
	device, err := store.GetDevice("device-1")
	if err != nil {
		t.Fatalf("Failed to get device: %v", err)
	}

	if device == nil {
		t.Fatal("Expected device to be retrieved")
	}

	if device.ID != "device-1" {
		t.Errorf("Expected ID device-1, got %s", device.ID)
	}
	if device.Name != "My Phone" {
		t.Errorf("Expected Name 'My Phone', got %s", device.Name)
	}
	if len(device.Scopes) != 2 {
		t.Errorf("Expected 2 scopes, got %d", len(device.Scopes))
	}
}

func TestListDevices(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	// Save multiple devices
	devices := []struct {
		id     string
		name   string
		scopes []string
	}{
		{"device-1", "Phone 1", []string{"read:*"}},
		{"device-2", "Phone 2", []string{"write:*"}},
		{"device-3", "Tablet", []string{"read:apps"}},
	}

	for _, d := range devices {
		if err := store.SaveDevice(d.id, d.name, "", "", d.scopes); err != nil {
			t.Fatalf("Failed to save device: %v", err)
		}
	}

	// List devices
	retrieved, err := store.ListDevices()
	if err != nil {
		t.Fatalf("Failed to list devices: %v", err)
	}

	if len(retrieved) != 3 {
		t.Errorf("Expected 3 devices, got %d", len(retrieved))
	}
}

func TestDeleteDevice(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	// Save device
	if err := store.SaveDevice("device-1", "Phone", "", "", []string{"read:*"}); err != nil {
		t.Fatalf("Failed to save device: %v", err)
	}

	// Delete device
	if err := store.DeleteDevice("device-1"); err != nil {
		t.Fatalf("Failed to delete device: %v", err)
	}

	// Verify deletion
	device, err := store.GetDevice("device-1")
	if err != nil {
		t.Fatalf("Failed to get device: %v", err)
	}

	if device != nil {
		t.Error("Expected device to be deleted")
	}
}

func TestUpdateDeviceLastSeen(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	// Save device
	if err := store.SaveDevice("device-1", "Phone", "", "", []string{"read:*"}); err != nil {
		t.Fatalf("Failed to save device: %v", err)
	}

	// Get initial last_seen
	device1, err := store.GetDevice("device-1")
	if err != nil {
		t.Fatalf("Failed to get device: %v", err)
	}

	// Wait for at least 1 second (SQLite datetime resolution)
	time.Sleep(1100 * time.Millisecond)

	if err := store.UpdateDeviceLastSeen("device-1"); err != nil {
		t.Fatalf("Failed to update device last seen: %v", err)
	}

	// Get updated last_seen
	device2, err := store.GetDevice("device-1")
	if err != nil {
		t.Fatalf("Failed to get device: %v", err)
	}

	if !device2.LastSeen.After(device1.LastSeen) {
		t.Errorf("Expected LastSeen to be updated: before=%v, after=%v", device1.LastSeen, device2.LastSeen)
	}
}

// Edge Cases and Error Handling

func TestGetNonExistent(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	// Try to get non-existent app
	app, err := store.GetApp("non-existent")
	if err != nil {
		t.Fatalf("Expected no error for non-existent app, got: %v", err)
	}
	if app != nil {
		t.Error("Expected nil for non-existent app")
	}

	// Try to get non-existent route
	route, err := store.GetRoute("non-existent")
	if err != nil {
		t.Fatalf("Expected no error for non-existent route, got: %v", err)
	}
	if route != nil {
		t.Error("Expected nil for non-existent route")
	}

	// Try to get non-existent proposal
	proposal, err := store.GetProposal("non-existent")
	if err != nil {
		t.Fatalf("Expected no error for non-existent proposal, got: %v", err)
	}
	if proposal != nil {
		t.Error("Expected nil for non-existent proposal")
	}

	// Try to get non-existent device
	device, err := store.GetDevice("non-existent")
	if err != nil {
		t.Fatalf("Expected no error for non-existent device, got: %v", err)
	}
	if device != nil {
		t.Error("Expected nil for non-existent device")
	}
}

func TestUpdateProposal(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	proposal := types.Proposal{
		ID: "proposal-1", Source: "docker", DetectedScheme: "http",
		DetectedHost: "host1", DetectedPort: 3000, Confidence: 0.8,
		SuggestedApp:   types.App{ID: "app1", Name: "App 1"},
		SuggestedRoute: types.Route{RouteID: "r1", AppID: "app1", PathBase: "/1/", To: "http://host1:3000"},
	}

	// Save initial proposal
	if err := store.SaveProposal(proposal); err != nil {
		t.Fatalf("Failed to save proposal: %v", err)
	}

	// Update proposal confidence
	proposal.Confidence = 0.95
	proposal.Tags = []string{"updated"}

	if err := store.SaveProposal(proposal); err != nil {
		t.Fatalf("Failed to update proposal: %v", err)
	}

	// Retrieve updated proposal
	retrieved, err := store.GetProposal("proposal-1")
	if err != nil {
		t.Fatalf("Failed to get proposal: %v", err)
	}

	if retrieved.Confidence != 0.95 {
		t.Errorf("Expected confidence 0.95, got %.2f", retrieved.Confidence)
	}
	if len(retrieved.Tags) != 1 || retrieved.Tags[0] != "updated" {
		t.Error("Expected tags to be updated")
	}
}

// Activity Events Tests

func TestAddActivity(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	event := types.ActivityEvent{
		ID:        "test-1",
		Type:      "device.paired",
		Icon:      "Smartphone",
		IconClass: "success",
		Message:   "Device paired: Test Device",
		Timestamp: time.Now().UnixMilli(),
	}

	err := store.AddActivity(event)
	if err != nil {
		t.Fatalf("Failed to add activity: %v", err)
	}

	// Verify event was added
	events, err := store.GetRecentActivity()
	if err != nil {
		t.Fatalf("Failed to get recent activity: %v", err)
	}

	if len(events) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(events))
	}

	if events[0].ID != "test-1" {
		t.Errorf("Expected ID 'test-1', got '%s'", events[0].ID)
	}
	if events[0].Type != "device.paired" {
		t.Errorf("Expected type 'device.paired', got '%s'", events[0].Type)
	}
	if events[0].Message != "Device paired: Test Device" {
		t.Errorf("Expected message 'Device paired: Test Device', got '%s'", events[0].Message)
	}
}

func TestAddActivity_WithDetails(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	event := types.ActivityEvent{
		ID:        "warning-1",
		Type:      "config.warning",
		Icon:      "AlertTriangle",
		IconClass: "warning",
		Message:   "Configuration warning",
		Details:   "Missing TLS certificate",
		Timestamp: time.Now().UnixMilli(),
	}

	err := store.AddActivity(event)
	if err != nil {
		t.Fatalf("Failed to add activity with details: %v", err)
	}

	events, err := store.GetRecentActivity()
	if err != nil {
		t.Fatalf("Failed to get recent activity: %v", err)
	}

	if len(events) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(events))
	}

	if events[0].Details != "Missing TLS certificate" {
		t.Errorf("Expected details 'Missing TLS certificate', got '%s'", events[0].Details)
	}
}

func TestGetRecentActivity_Limit10(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	// Add 15 events
	for i := 0; i < 15; i++ {
		event := types.ActivityEvent{
			ID:        string(rune('a' + i)),
			Type:      "test.event",
			Icon:      "Activity",
			Message:   "Test event",
			Timestamp: time.Now().UnixMilli() + int64(i*1000),
		}
		if err := store.AddActivity(event); err != nil {
			t.Fatalf("Failed to add activity: %v", err)
		}
	}

	// Should only return last 10
	events, err := store.GetRecentActivity()
	if err != nil {
		t.Fatalf("Failed to get recent activity: %v", err)
	}

	if len(events) != 10 {
		t.Fatalf("Expected 10 events (limit), got %d", len(events))
	}

	// Should be in newest-first order
	// Last added was 'o' (14th), first in result should be 'o'
	if events[0].ID != "o" {
		t.Errorf("Expected newest event 'o', got '%s'", events[0].ID)
	}

	// 10th event should be 'f' (5th added)
	if events[9].ID != "f" {
		t.Errorf("Expected 10th event 'f', got '%s'", events[9].ID)
	}
}

func TestGetRecentActivity_Order(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	// Add events with explicit timestamps
	baseTime := time.Now().UnixMilli()
	for i := 0; i < 5; i++ {
		event := types.ActivityEvent{
			ID:        string(rune('a' + i)),
			Type:      "test.event",
			Icon:      "Activity",
			Message:   "Test event",
			Timestamp: baseTime + int64(i*1000),
		}
		if err := store.AddActivity(event); err != nil {
			t.Fatalf("Failed to add activity: %v", err)
		}
	}

	events, err := store.GetRecentActivity()
	if err != nil {
		t.Fatalf("Failed to get recent activity: %v", err)
	}

	// Should be in newest-first order
	expectedOrder := []string{"e", "d", "c", "b", "a"}
	for i, expected := range expectedOrder {
		if events[i].ID != expected {
			t.Errorf("Position %d: expected '%s', got '%s'", i, expected, events[i].ID)
		}
	}
}

func TestGetRecentActivity_Empty(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	events, err := store.GetRecentActivity()
	if err != nil {
		t.Fatalf("Failed to get recent activity: %v", err)
	}

	if len(events) != 0 {
		t.Errorf("Expected 0 events, got %d", len(events))
	}

	// Should return non-nil slice
	if events == nil {
		t.Error("GetRecentActivity() should return non-nil slice")
	}
}

func TestPruneOldActivity(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	// Add 15 events
	for i := 0; i < 15; i++ {
		event := types.ActivityEvent{
			ID:        string(rune('a' + i)),
			Type:      "test.event",
			Icon:      "Activity",
			Message:   "Test event",
			Timestamp: time.Now().UnixMilli() + int64(i*1000),
		}
		if err := store.AddActivity(event); err != nil {
			t.Fatalf("Failed to add activity: %v", err)
		}
	}

	// Manually prune (normally called automatically by AddActivity)
	if err := store.PruneOldActivity(); err != nil {
		t.Fatalf("Failed to prune old activity: %v", err)
	}

	// Verify only 10 remain
	var count int
	err := store.db.QueryRow("SELECT COUNT(*) FROM activity_events").Scan(&count)
	if err != nil {
		t.Fatalf("Failed to count activity events: %v", err)
	}

	if count != 10 {
		t.Errorf("Expected 10 events after pruning, got %d", count)
	}
}

func TestAddActivity_AutoPrune(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	// Add 12 events (should auto-prune to 10)
	for i := 0; i < 12; i++ {
		event := types.ActivityEvent{
			ID:        string(rune('a' + i)),
			Type:      "test.event",
			Icon:      "Activity",
			Message:   "Test event",
			Timestamp: time.Now().UnixMilli() + int64(i*1000),
		}
		if err := store.AddActivity(event); err != nil {
			t.Fatalf("Failed to add activity: %v", err)
		}
	}

	// Verify exactly 10 remain in database
	var count int
	err := store.db.QueryRow("SELECT COUNT(*) FROM activity_events").Scan(&count)
	if err != nil {
		t.Fatalf("Failed to count activity events: %v", err)
	}

	if count != 10 {
		t.Errorf("Expected 10 events after auto-prune, got %d", count)
	}

	// Verify correct events remain (newest 10)
	events, err := store.GetRecentActivity()
	if err != nil {
		t.Fatalf("Failed to get recent activity: %v", err)
	}

	if events[0].ID != "l" { // 11th event (index 11)
		t.Errorf("Expected newest event 'l', got '%s'", events[0].ID)
	}
	if events[9].ID != "c" { // 2nd event (index 2)
		t.Errorf("Expected oldest remaining event 'c', got '%s'", events[9].ID)
	}
}

func TestActivityEvent_AllEventTypes(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	// Test all expected event types
	eventTypes := []struct {
		eventType string
		icon      string
		iconClass string
		message   string
	}{
		{"device.paired", "Smartphone", "success", "Device paired: Test Phone"},
		{"device.revoked", "XCircle", "", "Device revoked: Test Phone"},
		{"app.registered", "CheckCircle2", "success", "Registered: Test App"},
		{"discovery.proposal", "Radio", "", "New service discovered: Test Service"},
		{"discovery.proposal.dismissed", "X", "", "Proposal dismissed: test-id"},
		{"config.reload", "RefreshCw", "", "Configuration reloaded"},
		{"config.warning", "AlertTriangle", "warning", "Config warning: Test"},
	}

	for _, et := range eventTypes {
		event := types.ActivityEvent{
			ID:        et.eventType + "-test",
			Type:      et.eventType,
			Icon:      et.icon,
			IconClass: et.iconClass,
			Message:   et.message,
			Timestamp: time.Now().UnixMilli(),
		}

		if err := store.AddActivity(event); err != nil {
			t.Errorf("Failed to add event type '%s': %v", et.eventType, err)
		}
	}

	// Verify all events were added
	events, err := store.GetRecentActivity()
	if err != nil {
		t.Fatalf("Failed to get recent activity: %v", err)
	}

	if len(events) != len(eventTypes) {
		t.Errorf("Expected %d events, got %d", len(eventTypes), len(events))
	}
}

// DiscoveryMetadata Tests

func TestSaveAndGetApp_WithDiscoveryMeta(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	app := types.App{
		ID:   "docker-app",
		Name: "Docker Application",
		Icon: "https://example.com/icon.png",
		Tags: []string{"docker", "web"},
		Endpoints: map[string]string{
			"lan": "http://192.168.1.100:8080",
		},
		DiscoveryMeta: &types.DiscoveryMetadata{
			Source:      "docker",
			ContainerID: "abc123def456",
		},
	}

	// Save app with DiscoveryMeta
	err := store.SaveApp(app)
	if err != nil {
		t.Fatalf("Failed to save app: %v", err)
	}

	// Retrieve app
	retrieved, err := store.GetApp("docker-app")
	if err != nil {
		t.Fatalf("Failed to get app: %v", err)
	}

	if retrieved == nil {
		t.Fatal("Expected app to be retrieved")
	}

	if retrieved.DiscoveryMeta == nil {
		t.Fatal("Expected DiscoveryMeta to be present")
	}

	if retrieved.DiscoveryMeta.Source != "docker" {
		t.Errorf("Expected Source 'docker', got '%s'", retrieved.DiscoveryMeta.Source)
	}

	if retrieved.DiscoveryMeta.ContainerID != "abc123def456" {
		t.Errorf("Expected ContainerID 'abc123def456', got '%s'", retrieved.DiscoveryMeta.ContainerID)
	}
}

func TestSaveAndGetApp_WithoutDiscoveryMeta(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	app := types.App{
		ID:   "config-app",
		Name: "Config Application",
		Tags: []string{"manual"},
	}

	// Save app without DiscoveryMeta
	err := store.SaveApp(app)
	if err != nil {
		t.Fatalf("Failed to save app: %v", err)
	}

	// Retrieve app
	retrieved, err := store.GetApp("config-app")
	if err != nil {
		t.Fatalf("Failed to get app: %v", err)
	}

	if retrieved == nil {
		t.Fatal("Expected app to be retrieved")
	}

	if retrieved.DiscoveryMeta != nil {
		t.Errorf("Expected DiscoveryMeta to be nil, got %+v", retrieved.DiscoveryMeta)
	}
}

func TestListApps_WithDiscoveryMeta(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	apps := []types.App{
		{
			ID:   "docker-app",
			Name: "Docker App",
			DiscoveryMeta: &types.DiscoveryMetadata{
				Source:      "docker",
				ContainerID: "container123",
			},
		},
		{
			ID:   "k8s-app",
			Name: "Kubernetes App",
			DiscoveryMeta: &types.DiscoveryMetadata{
				Source:  "kubernetes",
				PodName: "pod-abc-123",
			},
		},
		{
			ID:   "manual-app",
			Name: "Manual App",
			// No DiscoveryMeta
		},
	}

	// Save apps
	for _, app := range apps {
		if err := store.SaveApp(app); err != nil {
			t.Fatalf("Failed to save app: %v", err)
		}
	}

	// List apps
	retrieved, err := store.ListApps()
	if err != nil {
		t.Fatalf("Failed to list apps: %v", err)
	}

	if len(retrieved) != 3 {
		t.Fatalf("Expected 3 apps, got %d", len(retrieved))
	}

	// Find and verify each app
	appMap := make(map[string]types.App)
	for _, app := range retrieved {
		appMap[app.ID] = app
	}

	// Verify docker-app
	dockerApp := appMap["docker-app"]
	if dockerApp.DiscoveryMeta == nil {
		t.Error("Expected docker-app to have DiscoveryMeta")
	} else {
		if dockerApp.DiscoveryMeta.Source != "docker" {
			t.Errorf("Expected docker-app Source 'docker', got '%s'", dockerApp.DiscoveryMeta.Source)
		}
		if dockerApp.DiscoveryMeta.ContainerID != "container123" {
			t.Errorf("Expected docker-app ContainerID 'container123', got '%s'", dockerApp.DiscoveryMeta.ContainerID)
		}
	}

	// Verify k8s-app
	k8sApp := appMap["k8s-app"]
	if k8sApp.DiscoveryMeta == nil {
		t.Error("Expected k8s-app to have DiscoveryMeta")
	} else {
		if k8sApp.DiscoveryMeta.Source != "kubernetes" {
			t.Errorf("Expected k8s-app Source 'kubernetes', got '%s'", k8sApp.DiscoveryMeta.Source)
		}
		if k8sApp.DiscoveryMeta.PodName != "pod-abc-123" {
			t.Errorf("Expected k8s-app PodName 'pod-abc-123', got '%s'", k8sApp.DiscoveryMeta.PodName)
		}
	}

	// Verify manual-app
	manualApp := appMap["manual-app"]
	if manualApp.DiscoveryMeta != nil {
		t.Errorf("Expected manual-app to have nil DiscoveryMeta, got %+v", manualApp.DiscoveryMeta)
	}
}

func TestUpdateApp_DiscoveryMeta(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	// Start with app without DiscoveryMeta
	app := types.App{
		ID:   "update-test",
		Name: "Update Test App",
	}

	if err := store.SaveApp(app); err != nil {
		t.Fatalf("Failed to save app: %v", err)
	}

	// Verify no DiscoveryMeta initially
	retrieved, err := store.GetApp("update-test")
	if err != nil {
		t.Fatalf("Failed to get app: %v", err)
	}
	if retrieved.DiscoveryMeta != nil {
		t.Error("Expected no DiscoveryMeta initially")
	}

	// Update to add DiscoveryMeta
	app.DiscoveryMeta = &types.DiscoveryMetadata{
		Source:      "docker",
		ContainerID: "newcontainer",
	}

	if err := store.SaveApp(app); err != nil {
		t.Fatalf("Failed to update app: %v", err)
	}

	// Verify DiscoveryMeta was added
	retrieved, err = store.GetApp("update-test")
	if err != nil {
		t.Fatalf("Failed to get updated app: %v", err)
	}
	if retrieved.DiscoveryMeta == nil {
		t.Fatal("Expected DiscoveryMeta after update")
	}
	if retrieved.DiscoveryMeta.ContainerID != "newcontainer" {
		t.Errorf("Expected ContainerID 'newcontainer', got '%s'", retrieved.DiscoveryMeta.ContainerID)
	}

	// Update to change container ID (simulating container restart)
	app.DiscoveryMeta.ContainerID = "differentcontainer"
	if err := store.SaveApp(app); err != nil {
		t.Fatalf("Failed to update app again: %v", err)
	}

	retrieved, err = store.GetApp("update-test")
	if err != nil {
		t.Fatalf("Failed to get re-updated app: %v", err)
	}
	if retrieved.DiscoveryMeta.ContainerID != "differentcontainer" {
		t.Errorf("Expected ContainerID 'differentcontainer', got '%s'", retrieved.DiscoveryMeta.ContainerID)
	}
}

func TestDiscoveryMeta_AllFields(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	// Test with all possible fields populated
	app := types.App{
		ID:   "full-meta-app",
		Name: "Full Metadata App",
		DiscoveryMeta: &types.DiscoveryMetadata{
			Source:      "docker",
			ContainerID: "abc123def456",
			PodName:     "test-pod",
			ServiceName: "test-service",
		},
	}

	if err := store.SaveApp(app); err != nil {
		t.Fatalf("Failed to save app: %v", err)
	}

	retrieved, err := store.GetApp("full-meta-app")
	if err != nil {
		t.Fatalf("Failed to get app: %v", err)
	}

	if retrieved.DiscoveryMeta == nil {
		t.Fatal("Expected DiscoveryMeta to be present")
	}

	meta := retrieved.DiscoveryMeta
	if meta.Source != "docker" {
		t.Errorf("Expected Source 'docker', got '%s'", meta.Source)
	}
	if meta.ContainerID != "abc123def456" {
		t.Errorf("Expected ContainerID 'abc123def456', got '%s'", meta.ContainerID)
	}
	if meta.PodName != "test-pod" {
		t.Errorf("Expected PodName 'test-pod', got '%s'", meta.PodName)
	}
	if meta.ServiceName != "test-service" {
		t.Errorf("Expected ServiceName 'test-service', got '%s'", meta.ServiceName)
	}
}
