package router

import (
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/nstalgic/nekzus/internal/storage"
	"github.com/nstalgic/nekzus/internal/types"
)

var log = slog.With("package", "router")

// Registry manages routes and apps with thread-safe access and persistence
type Registry struct {
	mu                     sync.RWMutex
	routes                 map[string]types.Route               // In-memory cache
	apps                   map[string]types.App                 // In-memory cache
	radixIndex             *RadixTree                           // Radix tree for fast path lookup
	subdomainIndex         map[string]string                    // subdomain -> routeID
	storage                *storage.Store                       // Persistent storage
	onAppAddedCallback     func(*types.App)                     // Federation: Called when app is added
	onAppRemovedCallback   func(string)                         // Federation: Called when app is removed
	onRouteRemovedCallback func(types.Route)                    // Proxy cache eviction: Called when route is removed
	onRouteUpdatedCallback func(oldRoute, newRoute types.Route) // Proxy cache eviction: Called when route target changes
}

// NewRegistry creates a new route registry with storage backend
func NewRegistry(store *storage.Store) *Registry {
	registry := &Registry{
		routes:         make(map[string]types.Route),
		apps:           make(map[string]types.App),
		radixIndex:     NewRadixTree(),
		subdomainIndex: make(map[string]string),
		storage:        store,
	}

	// Load existing data from storage
	if store != nil {
		if err := registry.loadFromStorage(); err != nil {
			log.Warn("Failed to load data from storage", "error", err)
		}
	}

	return registry
}

// loadFromStorage loads routes and apps from the database into memory
func (r *Registry) loadFromStorage() error {
	// Load apps
	apps, err := r.storage.ListApps()
	if err != nil {
		return err
	}
	for _, app := range apps {
		r.apps[app.ID] = app
	}
	log.Info("Loaded apps from storage", "count", len(apps))

	// Load routes
	routes, err := r.storage.ListRoutes()
	if err != nil {
		return err
	}
	for _, route := range routes {
		r.routes[route.RouteID] = route
		r.radixIndex.Insert(route)
		if route.Subdomain != "" && (route.RoutingMode == "subdomain" || route.RoutingMode == "both") {
			r.subdomainIndex[route.Subdomain] = route.RouteID
		}
	}
	log.Info("Loaded routes from storage", "count", len(routes))

	return nil
}

// UpsertRoute adds or updates a route (both memory and storage)
func (r *Registry) UpsertRoute(rt types.Route) error {
	r.mu.Lock()

	// Normalize: If strip_prefix is enabled, ensure trailing slash for predictable prefix matching
	if rt.StripPrefix && !strings.HasSuffix(rt.PathBase, "/") {
		log.Info("Route has strip_prefix=true but no trailing slash, adding trailing slash", "routeID", rt.RouteID)
		rt.PathBase = rt.PathBase + "/"
	}

	// Default: If strip_prefix is enabled, enable rewrite_html for proper path rewriting
	if rt.StripPrefix && !rt.RewriteHTML {
		rt.RewriteHTML = true
	}

	// Check if route target changed (for proxy cache eviction)
	oldRoute, exists := r.routes[rt.RouteID]
	targetChanged := exists && oldRoute.To != rt.To

	// Update memory
	r.routes[rt.RouteID] = rt
	r.radixIndex.Insert(rt)

	// Maintain subdomain index
	if rt.Subdomain != "" && (rt.RoutingMode == "subdomain" || rt.RoutingMode == "both") {
		// Check uniqueness: another route must not own this subdomain
		if existingRouteID, taken := r.subdomainIndex[rt.Subdomain]; taken && existingRouteID != rt.RouteID {
			// Rollback
			if exists {
				r.routes[rt.RouteID] = oldRoute
				r.radixIndex.Insert(oldRoute)
			} else {
				delete(r.routes, rt.RouteID)
				r.radixIndex.Delete(rt.RouteID)
			}
			r.mu.Unlock()
			return fmt.Errorf("subdomain %q is already used by route %q", rt.Subdomain, existingRouteID)
		}
		r.subdomainIndex[rt.Subdomain] = rt.RouteID
	}

	// Clean up old subdomain if it changed
	if exists && oldRoute.Subdomain != "" && oldRoute.Subdomain != rt.Subdomain {
		if r.subdomainIndex[oldRoute.Subdomain] == rt.RouteID {
			delete(r.subdomainIndex, oldRoute.Subdomain)
		}
	}
	// Also clean up if mode changed away from subdomain
	if exists && oldRoute.Subdomain != "" && rt.RoutingMode != "subdomain" && rt.RoutingMode != "both" && rt.Subdomain == "" {
		if r.subdomainIndex[oldRoute.Subdomain] == rt.RouteID {
			delete(r.subdomainIndex, oldRoute.Subdomain)
		}
	}

	callback := r.onRouteUpdatedCallback
	r.mu.Unlock()

	// Persist to storage
	if r.storage != nil {
		if err := r.storage.SaveRoute(rt); err != nil {
			// Rollback memory change on storage failure
			r.mu.Lock()
			delete(r.routes, rt.RouteID)
			r.radixIndex.Delete(rt.RouteID)
			// Rollback subdomain index
			if rt.Subdomain != "" {
				delete(r.subdomainIndex, rt.Subdomain)
			}
			if exists {
				r.routes[rt.RouteID] = oldRoute
				r.radixIndex.Insert(oldRoute)
				if oldRoute.Subdomain != "" && (oldRoute.RoutingMode == "subdomain" || oldRoute.RoutingMode == "both") {
					r.subdomainIndex[oldRoute.Subdomain] = rt.RouteID
				}
			}
			r.mu.Unlock()
			return err
		}
	}

	// Notify proxy cache eviction if target changed
	if targetChanged && callback != nil {
		callback(oldRoute, rt)
	}

	return nil
}

// UpsertRouteWithValidation adds or updates a route with path collision detection
// Use this for API-created routes to prevent silent conflicts
func (r *Registry) UpsertRouteWithValidation(rt types.Route) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Normalize: If strip_prefix is enabled, ensure trailing slash
	if rt.StripPrefix && !strings.HasSuffix(rt.PathBase, "/") {
		log.Info("route has strip_prefix=true but no trailing slash, adding trailing slash for consistent prefix matching",
			"route_id", rt.RouteID)
		rt.PathBase = rt.PathBase + "/"
	}

	// Default: If strip_prefix is enabled, enable rewrite_html for proper path rewriting
	if rt.StripPrefix && !rt.RewriteHTML {
		rt.RewriteHTML = true
	}

	// Use validation to detect path collisions
	if err := r.radixIndex.InsertWithValidation(rt); err != nil {
		return err
	}

	// Check old route for subdomain cleanup
	oldRoute, exists := r.routes[rt.RouteID]

	// Update memory
	r.routes[rt.RouteID] = rt

	// Maintain subdomain index
	if rt.Subdomain != "" && (rt.RoutingMode == "subdomain" || rt.RoutingMode == "both") {
		// Check uniqueness: another route must not own this subdomain
		if existingRouteID, taken := r.subdomainIndex[rt.Subdomain]; taken && existingRouteID != rt.RouteID {
			// Rollback
			if exists {
				r.routes[rt.RouteID] = oldRoute
				r.radixIndex.Insert(oldRoute)
			} else {
				delete(r.routes, rt.RouteID)
				r.radixIndex.Delete(rt.RouteID)
			}
			return fmt.Errorf("subdomain %q is already used by route %q", rt.Subdomain, existingRouteID)
		}
		r.subdomainIndex[rt.Subdomain] = rt.RouteID
	}

	// Clean up old subdomain if it changed
	if exists && oldRoute.Subdomain != "" && oldRoute.Subdomain != rt.Subdomain {
		if r.subdomainIndex[oldRoute.Subdomain] == rt.RouteID {
			delete(r.subdomainIndex, oldRoute.Subdomain)
		}
	}
	// Also clean up if mode changed away from subdomain
	if exists && oldRoute.Subdomain != "" && rt.RoutingMode != "subdomain" && rt.RoutingMode != "both" && rt.Subdomain == "" {
		if r.subdomainIndex[oldRoute.Subdomain] == rt.RouteID {
			delete(r.subdomainIndex, oldRoute.Subdomain)
		}
	}

	// Persist to storage
	if r.storage != nil {
		if err := r.storage.SaveRoute(rt); err != nil {
			// Rollback memory changes on storage failure
			delete(r.routes, rt.RouteID)
			r.radixIndex.Delete(rt.RouteID)
			// Rollback subdomain index
			if rt.Subdomain != "" {
				delete(r.subdomainIndex, rt.Subdomain)
			}
			if exists {
				r.routes[rt.RouteID] = oldRoute
				r.radixIndex.Insert(oldRoute)
				if oldRoute.Subdomain != "" && (oldRoute.RoutingMode == "subdomain" || oldRoute.RoutingMode == "both") {
					r.subdomainIndex[oldRoute.Subdomain] = rt.RouteID
				}
			}
			return err
		}
	}

	return nil
}

// RemoveRoute deletes a route by ID (both memory and storage)
// Also removes the associated app if no other routes reference it
func (r *Registry) RemoveRoute(id string) error {
	r.mu.Lock()

	// Capture route before deletion for callback and app cleanup
	route, exists := r.routes[id]
	appID := ""
	if exists {
		appID = route.AppID
	}

	// Remove from subdomain index
	if exists && route.Subdomain != "" {
		if r.subdomainIndex[route.Subdomain] == id {
			delete(r.subdomainIndex, route.Subdomain)
		}
	}

	// Remove from memory
	delete(r.routes, id)
	r.radixIndex.Delete(id)

	// Check if any other routes still reference this app
	appHasOtherRoutes := false
	if appID != "" {
		for _, rt := range r.routes {
			if rt.AppID == appID {
				appHasOtherRoutes = true
				break
			}
		}
	}

	callback := r.onRouteRemovedCallback
	r.mu.Unlock()

	// Remove from storage
	if r.storage != nil {
		if err := r.storage.DeleteRoute(id); err != nil {
			return err
		}
	}

	// Notify proxy cache eviction
	if exists && callback != nil {
		callback(route)
	}

	// Remove orphaned app if no other routes reference it
	if appID != "" && !appHasOtherRoutes {
		if err := r.RemoveApp(appID); err != nil {
			log.Warn("failed to remove orphaned app", "app_id", appID, "error", err)
		}
	}

	return nil
}

// GetRouteByPath finds the best matching route for a given path (longest prefix)
func (r *Registry) GetRouteByPath(path string) (types.Route, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Use radix tree for O(m) lookup where m is path length
	// (vs O(n) linear search where n is number of routes)
	return r.radixIndex.Search(path)
}

// GetRouteBySubdomain looks up a route by its subdomain.
func (r *Registry) GetRouteBySubdomain(subdomain string) (types.Route, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	routeID, ok := r.subdomainIndex[subdomain]
	if !ok {
		return types.Route{}, false
	}
	route, ok := r.routes[routeID]
	return route, ok
}

// UpsertApp adds or updates an app in the catalog (both memory and storage)
func (r *Registry) UpsertApp(a types.App) error {
	r.mu.Lock()

	// Check if app is new or updated
	_, exists := r.apps[a.ID]

	// Update memory
	r.apps[a.ID] = a

	r.mu.Unlock()

	// Persist to storage
	if r.storage != nil {
		if err := r.storage.SaveApp(a); err != nil {
			// Rollback memory change on storage failure
			r.mu.Lock()
			delete(r.apps, a.ID)
			r.mu.Unlock()
			return err
		}
	}

	// Notify federation if this is a new app or update
	if r.onAppAddedCallback != nil {
		appCopy := a // Create a copy to pass to callback
		r.onAppAddedCallback(&appCopy)
	}

	if !exists {
		log.Info("app registered",
			"name", a.Name,
			"app_id", a.ID)
	}

	return nil
}

// RemoveApp removes an app from the catalog (both memory and storage)
func (r *Registry) RemoveApp(appID string) error {
	r.mu.Lock()

	// Check if app exists
	_, exists := r.apps[appID]
	if !exists {
		r.mu.Unlock()
		return nil // App doesn't exist, nothing to do
	}

	// Remove from memory
	delete(r.apps, appID)

	r.mu.Unlock()

	// Remove from storage
	if r.storage != nil {
		if err := r.storage.DeleteApp(appID); err != nil {
			return err
		}
	}

	// Notify federation
	if r.onAppRemovedCallback != nil {
		r.onAppRemovedCallback(appID)
	}

	log.Info("app unregistered",
		"app_id", appID)

	return nil
}

// SetFederationCallbacks sets callbacks for federation integration
func (r *Registry) SetFederationCallbacks(onAdded func(*types.App), onRemoved func(string)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.onAppAddedCallback = onAdded
	r.onAppRemovedCallback = onRemoved
}

// SetProxyCacheCallbacks sets callbacks for proxy cache eviction
func (r *Registry) SetProxyCacheCallbacks(onRouteRemoved func(types.Route), onRouteUpdated func(oldRoute, newRoute types.Route)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.onRouteRemovedCallback = onRouteRemoved
	r.onRouteUpdatedCallback = onRouteUpdated
}

// GetRouteByAppID finds a route for a given app ID
func (r *Registry) GetRouteByAppID(appID string) (*types.Route, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, rt := range r.routes {
		if rt.AppID == appID {
			route := rt
			return &route, true
		}
	}

	return nil, false
}

// GetAppByID finds an app by its ID
func (r *Registry) GetAppByID(appID string) (*types.App, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if app, ok := r.apps[appID]; ok {
		return &app, true
	}
	return nil, false
}

// ListApps returns all registered apps
func (r *Registry) ListApps() []types.App {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]types.App, 0, len(r.apps))
	for _, a := range r.apps {
		out = append(out, a)
	}
	return out
}

// ListRoutes returns all registered routes
func (r *Registry) ListRoutes() []types.Route {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]types.Route, 0, len(r.routes))
	for _, rt := range r.routes {
		out = append(out, rt)
	}
	return out
}

// ReplaceRoutes replaces all routes with new ones (for hot reload)
// Build new data structures outside lock, then atomic swap
func (r *Registry) ReplaceRoutes(newRoutes []types.Route) error {
	// Build ALL new state outside lock (including storage prep)
	newRouteMap := make(map[string]types.Route)
	newRadixIndex := NewRadixTree()
	newRouteIDs := make(map[string]bool)

	for i, rt := range newRoutes {
		// Normalize: If strip_prefix is enabled, ensure trailing slash
		if rt.StripPrefix && !strings.HasSuffix(rt.PathBase, "/") {
			rt.PathBase = rt.PathBase + "/"
		}
		// Default: If strip_prefix is enabled, enable rewrite_html
		if rt.StripPrefix && !rt.RewriteHTML {
			rt.RewriteHTML = true
		}
		newRoutes[i] = rt
		newRouteMap[rt.RouteID] = rt
		newRadixIndex.Insert(rt)
		newRouteIDs[rt.RouteID] = true
	}

	// Determine routes to delete (with read lock only)
	var routesToDelete []string
	r.mu.RLock()
	for routeID := range r.routes {
		if !newRouteIDs[routeID] {
			routesToDelete = append(routesToDelete, routeID)
		}
	}
	r.mu.RUnlock()

	// Perform all storage operations OUTSIDE lock
	if r.storage != nil {
		// Delete old routes
		for _, routeID := range routesToDelete {
			if err := r.storage.DeleteRoute(routeID); err != nil {
				log.Warn("failed to delete route from storage",
					"route_id", routeID,
					"error", err)
			}
		}

		// Save new routes
		for _, rt := range newRoutes {
			if err := r.storage.SaveRoute(rt); err != nil {
				log.Warn("failed to persist route",
					"route_id", rt.RouteID,
					"error", err)
			}
		}
	}

	// Only hold write lock for atomic swap
	r.mu.Lock()
	r.routes = newRouteMap
	r.radixIndex = newRadixIndex
	r.mu.Unlock()

	log.Info("config reload: replaced routes",
		"count", len(newRoutes))
	return nil
}

// ReplaceApps replaces all apps with new ones (for hot reload)
func (r *Registry) ReplaceApps(newApps []types.App) error {
	// Build new app map outside lock
	newAppMap := make(map[string]types.App)
	newAppIDs := make(map[string]bool)
	for _, app := range newApps {
		newAppMap[app.ID] = app
		newAppIDs[app.ID] = true
	}

	// Determine apps to delete (with read lock only)
	var appsToDelete []string
	r.mu.RLock()
	for appID := range r.apps {
		if !newAppIDs[appID] {
			appsToDelete = append(appsToDelete, appID)
		}
	}
	r.mu.RUnlock()

	// Perform all storage operations OUTSIDE lock
	if r.storage != nil {
		// Delete old apps
		for _, appID := range appsToDelete {
			if err := r.storage.DeleteApp(appID); err != nil {
				log.Warn("failed to delete app from storage",
					"app_id", appID,
					"error", err)
			}
		}

		// Save new apps
		for _, app := range newApps {
			if err := r.storage.SaveApp(app); err != nil {
				log.Warn("failed to persist app",
					"app_id", app.ID,
					"error", err)
			}
		}
	}

	// Only hold write lock for atomic swap
	r.mu.Lock()
	r.apps = newAppMap
	r.mu.Unlock()

	log.Info("config reload: replaced apps",
		"count", len(newApps))
	return nil
}
