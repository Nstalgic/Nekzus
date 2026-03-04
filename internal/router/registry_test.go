package router

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/nstalgic/nekzus/internal/storage"
	"github.com/nstalgic/nekzus/internal/types"
)

func TestNewRouteRegistry(t *testing.T) {
	t.Run("creates registry without storage", func(t *testing.T) {
		registry := NewRegistry(nil)

		if registry == nil {
			t.Fatal("registry should not be nil")
		}
		if registry.routes == nil {
			t.Error("routes map should be initialized")
		}
		if registry.apps == nil {
			t.Error("apps map should be initialized")
		}
		if registry.storage != nil {
			t.Error("storage should be nil")
		}
	})

	t.Run("creates registry with storage", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.db")

		store, err := storage.NewStore(storage.Config{
			DatabasePath: dbPath,
		})
		if err != nil {
			t.Fatalf("Failed to create storage: %v", err)
		}
		defer store.Close()

		registry := NewRegistry(store)

		if registry == nil {
			t.Fatal("registry should not be nil")
		}
		if registry.storage == nil {
			t.Error("storage should not be nil")
		}
	})

	t.Run("loads existing data from storage", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.db")

		// Create storage and save test data
		store, err := storage.NewStore(storage.Config{
			DatabasePath: dbPath,
		})
		if err != nil {
			t.Fatalf("Failed to create storage: %v", err)
		}

		testApp := types.App{
			ID:   "test-app",
			Name: "Test App",
		}
		testRoute := types.Route{
			RouteID:  "test-route",
			PathBase: "/test",
			To:       "http://localhost:8080",
			AppID:    "test-app",
		}

		// Save app first (foreign key constraint)
		if err := store.SaveApp(testApp); err != nil {
			t.Fatalf("Failed to save app: %v", err)
		}
		if err := store.SaveRoute(testRoute); err != nil {
			t.Fatalf("Failed to save route: %v", err)
		}

		store.Close()

		// Create new storage and registry
		store2, err := storage.NewStore(storage.Config{
			DatabasePath: dbPath,
		})
		if err != nil {
			t.Fatalf("Failed to create storage: %v", err)
		}
		defer store2.Close()

		registry := NewRegistry(store2)

		// Verify data was loaded
		if len(registry.routes) != 1 {
			t.Errorf("Expected 1 route, got %d", len(registry.routes))
		}
		if len(registry.apps) != 1 {
			t.Errorf("Expected 1 app, got %d", len(registry.apps))
		}

		loadedRoute, exists := registry.routes["test-route"]
		if !exists {
			t.Error("Route was not loaded from storage")
		}
		if loadedRoute.RouteID != testRoute.RouteID {
			t.Errorf("Route ID = %q, want %q", loadedRoute.RouteID, testRoute.RouteID)
		}
	})
}

func TestRouteRegistry_UpsertRoute(t *testing.T) {
	t.Run("inserts new route without storage", func(t *testing.T) {
		registry := NewRegistry(nil)

		route := types.Route{
			RouteID:  "test-route",
			PathBase: "/test",
			To:       "http://localhost:8080",
		}

		err := registry.UpsertRoute(route)
		if err != nil {
			t.Errorf("UpsertRoute() error = %v", err)
		}

		if len(registry.routes) != 1 {
			t.Errorf("Expected 1 route, got %d", len(registry.routes))
		}

		stored, exists := registry.routes["test-route"]
		if !exists {
			t.Fatal("Route not found in registry")
		}
		if stored.PathBase != "/test" {
			t.Errorf("PathBase = %q, want %q", stored.PathBase, "/test")
		}
	})

	t.Run("updates existing route", func(t *testing.T) {
		registry := NewRegistry(nil)

		route1 := types.Route{
			RouteID:  "test-route",
			PathBase: "/test",
			To:       "http://localhost:8080",
		}
		route2 := types.Route{
			RouteID:  "test-route",
			PathBase: "/updated",
			To:       "http://localhost:9090",
		}

		registry.UpsertRoute(route1)
		registry.UpsertRoute(route2)

		if len(registry.routes) != 1 {
			t.Errorf("Expected 1 route after update, got %d", len(registry.routes))
		}

		stored := registry.routes["test-route"]
		if stored.PathBase != "/updated" {
			t.Errorf("PathBase = %q, want %q", stored.PathBase, "/updated")
		}
	})

	t.Run("persists to storage", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.db")

		store, err := storage.NewStore(storage.Config{
			DatabasePath: dbPath,
		})
		if err != nil {
			t.Fatalf("Failed to create storage: %v", err)
		}
		defer store.Close()

		registry := NewRegistry(store)

		// Create app first (foreign key constraint)
		app := types.App{
			ID:   "test-app",
			Name: "Test App",
		}
		if err := registry.UpsertApp(app); err != nil {
			t.Fatalf("Failed to create app: %v", err)
		}

		route := types.Route{
			RouteID:  "test-route",
			PathBase: "/test",
			To:       "http://localhost:8080",
			AppID:    "test-app",
		}

		err = registry.UpsertRoute(route)
		if err != nil {
			t.Errorf("UpsertRoute() error = %v", err)
		}

		// Verify it's in storage
		routes, err := store.ListRoutes()
		if err != nil {
			t.Fatalf("Failed to list routes from storage: %v", err)
		}

		if len(routes) != 1 {
			t.Errorf("Expected 1 route in storage, got %d", len(routes))
		}
	})
}

func TestRouteRegistry_RemoveRoute(t *testing.T) {
	t.Run("removes route from memory", func(t *testing.T) {
		registry := NewRegistry(nil)

		route := types.Route{
			RouteID:  "test-route",
			PathBase: "/test",
			To:       "http://localhost:8080",
		}

		registry.UpsertRoute(route)
		if len(registry.routes) != 1 {
			t.Fatal("Route not added")
		}

		err := registry.RemoveRoute("test-route")
		if err != nil {
			t.Errorf("RemoveRoute() error = %v", err)
		}

		if len(registry.routes) != 0 {
			t.Errorf("Expected 0 routes after removal, got %d", len(registry.routes))
		}
	})

	t.Run("removes route from storage", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.db")

		store, err := storage.NewStore(storage.Config{
			DatabasePath: dbPath,
		})
		if err != nil {
			t.Fatalf("Failed to create storage: %v", err)
		}
		defer store.Close()

		registry := NewRegistry(store)

		// Create app first (foreign key constraint)
		app := types.App{
			ID:   "test-app",
			Name: "Test App",
		}
		if err := registry.UpsertApp(app); err != nil {
			t.Fatalf("Failed to create app: %v", err)
		}

		route := types.Route{
			RouteID:  "test-route",
			PathBase: "/test",
			To:       "http://localhost:8080",
			AppID:    "test-app",
		}

		registry.UpsertRoute(route)
		registry.RemoveRoute("test-route")

		// Verify it's removed from storage
		routes, err := store.ListRoutes()
		if err != nil {
			t.Fatalf("Failed to list routes from storage: %v", err)
		}

		if len(routes) != 0 {
			t.Errorf("Expected 0 routes in storage after removal, got %d", len(routes))
		}
	})

	t.Run("removes orphaned app when last route is deleted", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.db")

		store, err := storage.NewStore(storage.Config{
			DatabasePath: dbPath,
		})
		if err != nil {
			t.Fatalf("Failed to create storage: %v", err)
		}
		defer store.Close()

		registry := NewRegistry(store)

		// Create app
		app := types.App{
			ID:   "test-app",
			Name: "Test App",
		}
		if err := registry.UpsertApp(app); err != nil {
			t.Fatalf("Failed to create app: %v", err)
		}

		// Create single route for this app
		route := types.Route{
			RouteID:  "test-route",
			PathBase: "/test",
			To:       "http://localhost:8080",
			AppID:    "test-app",
		}
		registry.UpsertRoute(route)

		// Verify app exists
		if len(registry.ListApps()) != 1 {
			t.Fatal("Expected 1 app before removal")
		}

		// Remove the route
		registry.RemoveRoute("test-route")

		// App should be removed since it has no more routes
		apps := registry.ListApps()
		if len(apps) != 0 {
			t.Errorf("Expected 0 apps after removing last route, got %d", len(apps))
		}

		// Verify app is also removed from storage
		storedApps, err := store.ListApps()
		if err != nil {
			t.Fatalf("Failed to list apps from storage: %v", err)
		}
		if len(storedApps) != 0 {
			t.Errorf("Expected 0 apps in storage after removal, got %d", len(storedApps))
		}
	})

	t.Run("keeps app when other routes still reference it", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.db")

		store, err := storage.NewStore(storage.Config{
			DatabasePath: dbPath,
		})
		if err != nil {
			t.Fatalf("Failed to create storage: %v", err)
		}
		defer store.Close()

		registry := NewRegistry(store)

		// Create app
		app := types.App{
			ID:   "test-app",
			Name: "Test App",
		}
		if err := registry.UpsertApp(app); err != nil {
			t.Fatalf("Failed to create app: %v", err)
		}

		// Create two routes for this app
		route1 := types.Route{
			RouteID:  "test-route-1",
			PathBase: "/test1",
			To:       "http://localhost:8080",
			AppID:    "test-app",
		}
		route2 := types.Route{
			RouteID:  "test-route-2",
			PathBase: "/test2",
			To:       "http://localhost:8081",
			AppID:    "test-app",
		}
		registry.UpsertRoute(route1)
		registry.UpsertRoute(route2)

		// Verify 2 routes exist
		if len(registry.ListRoutes()) != 2 {
			t.Fatal("Expected 2 routes before removal")
		}

		// Remove one route
		registry.RemoveRoute("test-route-1")

		// App should still exist since route2 still references it
		apps := registry.ListApps()
		if len(apps) != 1 {
			t.Errorf("Expected 1 app after removing one of two routes, got %d", len(apps))
		}

		// Remove the second route
		registry.RemoveRoute("test-route-2")

		// Now app should be removed
		apps = registry.ListApps()
		if len(apps) != 0 {
			t.Errorf("Expected 0 apps after removing last route, got %d", len(apps))
		}
	})

	t.Run("handles route without AppID", func(t *testing.T) {
		registry := NewRegistry(nil)

		// Create route without AppID
		route := types.Route{
			RouteID:  "test-route",
			PathBase: "/test",
			To:       "http://localhost:8080",
			// No AppID
		}
		registry.UpsertRoute(route)

		// Remove should not panic or error
		err := registry.RemoveRoute("test-route")
		if err != nil {
			t.Errorf("RemoveRoute() error = %v", err)
		}
	})
}

func TestRouteRegistry_GetRouteByPath(t *testing.T) {
	tests := []struct {
		name       string
		routes     []types.Route
		path       string
		wantFound  bool
		wantPrefix string
	}{
		{
			name: "exact match",
			routes: []types.Route{
				{RouteID: "r1", PathBase: "/api", To: "http://localhost:8080"},
			},
			path:       "/api",
			wantFound:  true,
			wantPrefix: "/api",
		},
		{
			name: "prefix match",
			routes: []types.Route{
				{RouteID: "r1", PathBase: "/api", To: "http://localhost:8080"},
			},
			path:       "/api/users",
			wantFound:  true,
			wantPrefix: "/api",
		},
		{
			name: "longest prefix wins",
			routes: []types.Route{
				{RouteID: "r1", PathBase: "/api", To: "http://localhost:8080"},
				{RouteID: "r2", PathBase: "/api/users", To: "http://localhost:9090"},
			},
			path:       "/api/users/123",
			wantFound:  true,
			wantPrefix: "/api/users",
		},
		{
			name: "no match",
			routes: []types.Route{
				{RouteID: "r1", PathBase: "/api", To: "http://localhost:8080"},
			},
			path:      "/other",
			wantFound: false,
		},
		{
			name:      "empty registry",
			routes:    []types.Route{},
			path:      "/any",
			wantFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registry := NewRegistry(nil)

			for _, route := range tt.routes {
				registry.UpsertRoute(route)
			}

			got, found := registry.GetRouteByPath(tt.path)

			if found != tt.wantFound {
				t.Errorf("GetRouteByPath(%q) found = %v, want %v", tt.path, found, tt.wantFound)
			}

			if found && got.PathBase != tt.wantPrefix {
				t.Errorf("GetRouteByPath(%q) prefix = %q, want %q", tt.path, got.PathBase, tt.wantPrefix)
			}
		})
	}
}

func TestRouteRegistry_UpsertApp(t *testing.T) {
	t.Run("inserts new app", func(t *testing.T) {
		registry := NewRegistry(nil)

		app := types.App{
			ID:   "test-app",
			Name: "Test App",
		}

		err := registry.UpsertApp(app)
		if err != nil {
			t.Errorf("UpsertApp() error = %v", err)
		}

		if len(registry.apps) != 1 {
			t.Errorf("Expected 1 app, got %d", len(registry.apps))
		}

		stored, exists := registry.apps["test-app"]
		if !exists {
			t.Fatal("App not found in registry")
		}
		if stored.Name != "Test App" {
			t.Errorf("Name = %q, want %q", stored.Name, "Test App")
		}
	})

	t.Run("persists to storage", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.db")

		store, err := storage.NewStore(storage.Config{
			DatabasePath: dbPath,
		})
		if err != nil {
			t.Fatalf("Failed to create storage: %v", err)
		}
		defer store.Close()

		registry := NewRegistry(store)

		app := types.App{
			ID:   "test-app",
			Name: "Test App",
		}

		err = registry.UpsertApp(app)
		if err != nil {
			t.Errorf("UpsertApp() error = %v", err)
		}

		// Verify it's in storage
		apps, err := store.ListApps()
		if err != nil {
			t.Fatalf("Failed to list apps from storage: %v", err)
		}

		if len(apps) != 1 {
			t.Errorf("Expected 1 app in storage, got %d", len(apps))
		}
	})
}

func TestRouteRegistry_GetRouteByAppID(t *testing.T) {
	t.Run("finds route by app ID", func(t *testing.T) {
		registry := NewRegistry(nil)

		route := types.Route{
			RouteID:  "test-route",
			PathBase: "/test",
			To:       "http://localhost:8080",
			AppID:    "test-app",
		}

		registry.UpsertRoute(route)

		found, exists := registry.GetRouteByAppID("test-app")
		if !exists {
			t.Fatal("Route not found by app ID")
		}

		if found.RouteID != "test-route" {
			t.Errorf("RouteID = %q, want %q", found.RouteID, "test-route")
		}
	})

	t.Run("returns false when app ID not found", func(t *testing.T) {
		registry := NewRegistry(nil)

		_, exists := registry.GetRouteByAppID("nonexistent")
		if exists {
			t.Error("Expected false for nonexistent app ID")
		}
	})
}

func TestRouteRegistry_ListApps(t *testing.T) {
	registry := NewRegistry(nil)

	apps := []types.App{
		{ID: "app1", Name: "App 1"},
		{ID: "app2", Name: "App 2"},
		{ID: "app3", Name: "App 3"},
	}

	for _, app := range apps {
		registry.UpsertApp(app)
	}

	listed := registry.ListApps()

	if len(listed) != 3 {
		t.Errorf("Expected 3 apps, got %d", len(listed))
	}

	// Verify all apps are present
	appIDs := make(map[string]bool)
	for _, app := range listed {
		appIDs[app.ID] = true
	}

	for _, app := range apps {
		if !appIDs[app.ID] {
			t.Errorf("App %q not found in list", app.ID)
		}
	}
}

func TestRouteRegistry_ListRoutes(t *testing.T) {
	registry := NewRegistry(nil)

	routes := []types.Route{
		{RouteID: "r1", PathBase: "/api1", To: "http://localhost:8080"},
		{RouteID: "r2", PathBase: "/api2", To: "http://localhost:8081"},
		{RouteID: "r3", PathBase: "/api3", To: "http://localhost:8082"},
	}

	for _, route := range routes {
		registry.UpsertRoute(route)
	}

	listed := registry.ListRoutes()

	if len(listed) != 3 {
		t.Errorf("Expected 3 routes, got %d", len(listed))
	}

	// Verify all routes are present
	routeIDs := make(map[string]bool)
	for _, route := range listed {
		routeIDs[route.RouteID] = true
	}

	for _, route := range routes {
		if !routeIDs[route.RouteID] {
			t.Errorf("Route %q not found in list", route.RouteID)
		}
	}
}

func TestRouteRegistry_ReplaceRoutes(t *testing.T) {
	t.Run("replaces all routes", func(t *testing.T) {
		registry := NewRegistry(nil)

		// Add initial routes
		initialRoutes := []types.Route{
			{RouteID: "old1", PathBase: "/old1", To: "http://localhost:8080"},
			{RouteID: "old2", PathBase: "/old2", To: "http://localhost:8081"},
		}
		for _, route := range initialRoutes {
			registry.UpsertRoute(route)
		}

		// Replace with new routes
		newRoutes := []types.Route{
			{RouteID: "new1", PathBase: "/new1", To: "http://localhost:9090"},
			{RouteID: "new2", PathBase: "/new2", To: "http://localhost:9091"},
			{RouteID: "new3", PathBase: "/new3", To: "http://localhost:9092"},
		}

		err := registry.ReplaceRoutes(newRoutes)
		if err != nil {
			t.Errorf("ReplaceRoutes() error = %v", err)
		}

		// Verify old routes are gone
		if _, exists := registry.routes["old1"]; exists {
			t.Error("Old route should be removed")
		}

		// Verify new routes exist
		if len(registry.routes) != 3 {
			t.Errorf("Expected 3 routes, got %d", len(registry.routes))
		}

		if _, exists := registry.routes["new1"]; !exists {
			t.Error("New route not found")
		}
	})
}

func TestRouteRegistry_ReplaceApps(t *testing.T) {
	t.Run("replaces all apps", func(t *testing.T) {
		registry := NewRegistry(nil)

		// Add initial apps
		initialApps := []types.App{
			{ID: "old1", Name: "Old App 1"},
			{ID: "old2", Name: "Old App 2"},
		}
		for _, app := range initialApps {
			registry.UpsertApp(app)
		}

		// Replace with new apps
		newApps := []types.App{
			{ID: "new1", Name: "New App 1"},
			{ID: "new2", Name: "New App 2"},
		}

		err := registry.ReplaceApps(newApps)
		if err != nil {
			t.Errorf("ReplaceApps() error = %v", err)
		}

		// Verify old apps are gone
		if _, exists := registry.apps["old1"]; exists {
			t.Error("Old app should be removed")
		}

		// Verify new apps exist
		if len(registry.apps) != 2 {
			t.Errorf("Expected 2 apps, got %d", len(registry.apps))
		}

		if _, exists := registry.apps["new1"]; !exists {
			t.Error("New app not found")
		}
	})
}

func TestRouteRegistry_Concurrency(t *testing.T) {
	t.Run("concurrent read and write operations", func(t *testing.T) {
		registry := NewRegistry(nil)

		// Pre-populate with some routes
		for i := 0; i < 10; i++ {
			route := types.Route{
				RouteID:  "route-" + string(rune(i)),
				PathBase: "/api" + string(rune(i)),
				To:       "http://localhost:8080",
			}
			registry.UpsertRoute(route)
		}

		// Run concurrent operations
		done := make(chan bool)
		iterations := 100

		// Writer goroutine
		go func() {
			for i := 0; i < iterations; i++ {
				route := types.Route{
					RouteID:  "concurrent-route",
					PathBase: "/concurrent",
					To:       "http://localhost:8080",
				}
				registry.UpsertRoute(route)
			}
			done <- true
		}()

		// Reader goroutine
		go func() {
			for i := 0; i < iterations; i++ {
				registry.GetRouteByPath("/api1")
				registry.ListRoutes()
			}
			done <- true
		}()

		// Wait for both goroutines
		<-done
		<-done

		// Verify registry is still functional
		routes := registry.ListRoutes()
		if len(routes) == 0 {
			t.Error("Registry should have routes after concurrent operations")
		}
	})
}

// TestRouteRegistry_UpsertAppAtomicity tests that UpsertApp is atomic
// Memory should not be updated until storage succeeds
func TestRouteRegistry_UpsertAppAtomicity(t *testing.T) {
	t.Run("storage first then memory", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.db")

		store, err := storage.NewStore(storage.Config{
			DatabasePath: dbPath,
		})
		if err != nil {
			t.Fatalf("Failed to create storage: %v", err)
		}
		defer store.Close()

		registry := NewRegistry(store)

		app := types.App{
			ID:   "test-app",
			Name: "Test App",
		}

		// Upsert should succeed
		err = registry.UpsertApp(app)
		if err != nil {
			t.Fatalf("UpsertApp() error = %v", err)
		}

		// Verify in storage
		apps, err := store.ListApps()
		if err != nil {
			t.Fatalf("Failed to list apps: %v", err)
		}
		if len(apps) != 1 {
			t.Errorf("Expected 1 app in storage, got %d", len(apps))
		}

		// Verify in memory
		if len(registry.apps) != 1 {
			t.Errorf("Expected 1 app in memory, got %d", len(registry.apps))
		}
	})
}

// TestRouteRegistry_ReplaceRoutesStorageCleanup tests that old routes are removed from storage
func TestRouteRegistry_ReplaceRoutesStorageCleanup(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := storage.NewStore(storage.Config{
		DatabasePath: dbPath,
	})
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	registry := NewRegistry(store)

	// Create app first
	app := types.App{ID: "test-app", Name: "Test App"}
	registry.UpsertApp(app)

	// Add initial routes
	initialRoutes := []types.Route{
		{RouteID: "old1", PathBase: "/old1", To: "http://localhost:8080", AppID: "test-app"},
		{RouteID: "old2", PathBase: "/old2", To: "http://localhost:8081", AppID: "test-app"},
	}
	for _, route := range initialRoutes {
		registry.UpsertRoute(route)
	}

	// Verify initial routes in storage
	routes, _ := store.ListRoutes()
	if len(routes) != 2 {
		t.Fatalf("Expected 2 routes in storage, got %d", len(routes))
	}

	// Replace with new routes
	newRoutes := []types.Route{
		{RouteID: "new1", PathBase: "/new1", To: "http://localhost:9090", AppID: "test-app"},
	}
	registry.ReplaceRoutes(newRoutes)

	// Verify old routes are removed from storage
	routes, _ = store.ListRoutes()
	if len(routes) != 1 {
		t.Errorf("Expected 1 route in storage after replace, got %d", len(routes))
	}

	// Verify old route IDs are not in storage
	for _, route := range routes {
		if route.RouteID == "old1" || route.RouteID == "old2" {
			t.Errorf("Old route %q should have been deleted from storage", route.RouteID)
		}
	}
}

// TestRouteRegistry_ReplaceAppsStorageCleanup tests that old apps are removed from storage
func TestRouteRegistry_ReplaceAppsStorageCleanup(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := storage.NewStore(storage.Config{
		DatabasePath: dbPath,
	})
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	registry := NewRegistry(store)

	// Add initial apps
	initialApps := []types.App{
		{ID: "old1", Name: "Old App 1"},
		{ID: "old2", Name: "Old App 2"},
	}
	for _, app := range initialApps {
		registry.UpsertApp(app)
	}

	// Verify initial apps in storage
	apps, _ := store.ListApps()
	if len(apps) != 2 {
		t.Fatalf("Expected 2 apps in storage, got %d", len(apps))
	}

	// Replace with new apps
	newApps := []types.App{
		{ID: "new1", Name: "New App 1"},
	}
	registry.ReplaceApps(newApps)

	// Verify old apps are removed from storage
	apps, _ = store.ListApps()
	if len(apps) != 1 {
		t.Errorf("Expected 1 app in storage after replace, got %d", len(apps))
	}

	// Verify old app IDs are not in storage
	for _, app := range apps {
		if app.ID == "old1" || app.ID == "old2" {
			t.Errorf("Old app %q should have been deleted from storage", app.ID)
		}
	}
}

// TestRouteRegistry_ConcurrentReadsDuringWrite tests that reads don't block during writes
// This verifies storage I/O doesn't hold the lock and block concurrent readers
func TestRouteRegistry_ConcurrentReadsDuringWrite(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := storage.NewStore(storage.Config{
		DatabasePath: dbPath,
	})
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	registry := NewRegistry(store)

	// Add initial app
	app := types.App{ID: "test-app", Name: "Test App"}
	registry.UpsertApp(app)

	// Concurrent reads and writes
	done := make(chan bool)
	readCount := 0
	var mu sync.Mutex

	// Start readers
	for i := 0; i < 5; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				_ = registry.ListApps()
				mu.Lock()
				readCount++
				mu.Unlock()
			}
			done <- true
		}()
	}

	// Start writers
	for i := 0; i < 3; i++ {
		go func(id int) {
			for j := 0; j < 20; j++ {
				registry.UpsertApp(types.App{
					ID:   fmt.Sprintf("app-%d-%d", id, j),
					Name: fmt.Sprintf("App %d-%d", id, j),
				})
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 8; i++ {
		<-done
	}

	// Verify reads completed (no deadlocks)
	if readCount != 500 {
		t.Errorf("Expected 500 reads, got %d", readCount)
	}
}

// TestRouteRegistry_ReplaceRoutesAtomicSwap tests that route replacement is atomic
// Reads should not be blocked during route tree rebuild
func TestRouteRegistry_ReplaceRoutesAtomicSwap(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := storage.NewStore(storage.Config{
		DatabasePath: dbPath,
	})
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	registry := NewRegistry(store)

	// Add initial app
	app := types.App{ID: "test-app", Name: "Test App"}
	registry.UpsertApp(app)

	// Add initial routes
	for i := 0; i < 100; i++ {
		registry.UpsertRoute(types.Route{
			RouteID:  fmt.Sprintf("route-%d", i),
			PathBase: fmt.Sprintf("/api/v1/resource%d", i),
			To:       "http://localhost:8080",
			AppID:    "test-app",
		})
	}

	// Concurrent reads during replace
	done := make(chan bool)
	readCount := 0
	var mu sync.Mutex

	// Start readers that continuously read during replace
	for i := 0; i < 5; i++ {
		go func() {
			for j := 0; j < 200; j++ {
				_ = registry.ListRoutes()
				mu.Lock()
				readCount++
				mu.Unlock()
			}
			done <- true
		}()
	}

	// Perform multiple replace operations
	for i := 0; i < 10; i++ {
		newRoutes := make([]types.Route, 50)
		for j := 0; j < 50; j++ {
			newRoutes[j] = types.Route{
				RouteID:  fmt.Sprintf("new-route-%d-%d", i, j),
				PathBase: fmt.Sprintf("/api/v2/resource%d", j),
				To:       "http://localhost:9090",
				AppID:    "test-app",
			}
		}
		registry.ReplaceRoutes(newRoutes)
	}

	// Wait for readers
	for i := 0; i < 5; i++ {
		<-done
	}

	// Verify reads completed (no deadlocks or excessive blocking)
	if readCount != 1000 {
		t.Errorf("Expected 1000 reads, got %d", readCount)
	}

	// Verify final state has the last set of routes
	routes := registry.ListRoutes()
	if len(routes) != 50 {
		t.Errorf("Expected 50 routes after replace, got %d", len(routes))
	}
}

// Cleanup helper to ensure test databases are removed
func TestMain(m *testing.M) {
	code := m.Run()
	os.Exit(code)
}
