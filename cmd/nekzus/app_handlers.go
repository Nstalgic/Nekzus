package main

import (
	"encoding/json"
	"net/http"
	"strings"

	apperrors "github.com/nstalgic/nekzus/internal/errors"
	"github.com/nstalgic/nekzus/internal/httputil"
	"github.com/nstalgic/nekzus/internal/types"
)

// handleListApps returns all registered applications with health status
func (app *Application) handleListApps(w http.ResponseWriter, r *http.Request) {
	// Get local apps
	apps := app.managers.Router.ListApps()

	// Merge with federated services if federation is enabled
	if app.managers.Peers != nil {
		catalogSyncer := app.managers.Peers.GetCatalogSyncer()
		if catalogSyncer != nil {
			federatedServices, err := catalogSyncer.GetFederatedCatalog()
			if err != nil {
				log.Warn("failed to get federated catalog", "error", err)
			} else {
				// Add federated services that aren't already in local list
				localAppIDs := make(map[string]bool)
				for _, app := range apps {
					localAppIDs[app.ID] = true
				}

				for _, fedService := range federatedServices {
					if !localAppIDs[fedService.ServiceID] && fedService.App != nil {
						// This is a remote service - add it to the list
						remoteApp := *fedService.App
						// Mark as federated service
						if remoteApp.Tags == nil {
							remoteApp.Tags = []string{}
						}
						remoteApp.Tags = append(remoteApp.Tags, "federated")
						apps = append(apps, remoteApp)
					}
				}
			}
		}
	}

	// Enrich apps with health status, proxy path, full URL, and favicon URL
	for i := range apps {
		// Add health status if health checker is available
		if app.jobs.ServiceHealth != nil {
			if healthStatus, ok := app.jobs.ServiceHealth.GetServiceHealth(apps[i].ID); ok {
				apps[i].HealthStatus = healthStatus.Status
				apps[i].LastHealthCheck = &healthStatus.LastCheckTime
			}
		}

		// Add proxy path and full URL from route
		if route, ok := app.managers.Router.GetRouteByAppID(apps[i].ID); ok {
			apps[i].ProxyPath = route.PathBase
			// Construct full URL with protocol from baseURL + proxyPath
			if app.baseURL != "" {
				apps[i].URL = strings.TrimSuffix(app.baseURL, "/") + route.PathBase
			}
		}

		// Add favicon URL (Nexus-served endpoint for the app's favicon)
		apps[i].FaviconURL = "/api/v1/apps/" + apps[i].ID + "/favicon"
	}

	if err := httputil.WriteJSON(w, http.StatusOK, apps); err != nil {
		log.Error("failed to encode json response", "error", err)
	}
}

// RouteWithHealthInfo wraps a route with computed health check information
type RouteWithHealthInfo struct {
	types.Route
	HealthInfo *types.RouteHealthInfo `json:"healthInfo,omitempty"`
}

// handleListRoutes returns all registered routes with health status
func (app *Application) handleListRoutes(w http.ResponseWriter, r *http.Request) {
	routes := app.managers.Router.ListRoutes()

	// Build response with health info
	response := make([]RouteWithHealthInfo, len(routes))

	for i := range routes {
		// Default to ACTIVE
		routes[i].Status = "ACTIVE"

		// Check if health checker is available and get health status
		if app.jobs.ServiceHealth != nil {
			if healthStatus, ok := app.jobs.ServiceHealth.GetServiceHealth(routes[i].AppID); ok {
				switch healthStatus.Status {
				case "healthy":
					routes[i].Status = "ACTIVE"
				case "unhealthy":
					routes[i].Status = "UNHEALTHY"
				default:
					routes[i].Status = "UNKNOWN"
				}
			}
		}

		response[i] = RouteWithHealthInfo{
			Route: routes[i],
		}

		// Add health info if health checker is available
		if app.jobs.ServiceHealth != nil {
			response[i].HealthInfo = app.jobs.ServiceHealth.GetRouteHealthInfo(routes[i].AppID, &routes[i])
		}
	}

	if err := httputil.WriteJSON(w, http.StatusOK, response); err != nil {
		log.Error("failed to encode json response", "error", err)
	}
}

// handleRouteActions handles individual route operations (GET, DELETE, PUT/PATCH)
func (app *Application) handleRouteActions(w http.ResponseWriter, r *http.Request) {
	// Extract route ID from URL path
	routeID := strings.TrimPrefix(r.URL.Path, "/api/v1/routes/")
	if routeID == "" {
		apperrors.WriteJSON(w, apperrors.ErrBadRequest)
		return
	}

	switch r.Method {
	case http.MethodDelete:
		// Delete the route
		if err := app.managers.Router.RemoveRoute(routeID); err != nil {
			apperrors.WriteJSON(w, apperrors.Wrap(err, "ROUTE_DELETE_FAILED", "Failed to delete route", http.StatusInternalServerError))
			return
		}

		// Publish WebSocket event for route deletion
		if app.managers.WebSocket != nil {
			app.managers.WebSocket.PublishConfigReload()
		}

		w.WriteHeader(http.StatusNoContent)

	case http.MethodPut, http.MethodPatch:
		// Update route
		var payload struct {
			types.Route
			Icon string `json:"icon,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			apperrors.WriteJSON(w, apperrors.Wrap(err, "INVALID_REQUEST_BODY", "Invalid request body", http.StatusBadRequest))
			return
		}

		// Ensure the route ID matches
		payload.Route.RouteID = routeID

		// Upsert the route with validation to detect path collisions
		if err := app.managers.Router.UpsertRouteWithValidation(payload.Route); err != nil {
			// Check if it's a path collision error
			if strings.Contains(err.Error(), "already used by route") {
				apperrors.WriteJSON(w, apperrors.New("ROUTE_CONFLICT", err.Error(), http.StatusConflict))
			} else {
				apperrors.WriteJSON(w, apperrors.Wrap(err, "ROUTE_UPDATE_FAILED", "Failed to update route", http.StatusInternalServerError))
			}
			return
		}

		// If icon is provided, update the app's icon
		if payload.Icon != "" && app.storage != nil {
			appID := payload.Route.AppID
			if appFromStorage, err := app.storage.GetApp(appID); err == nil {
				appFromStorage.Icon = payload.Icon
				if err := app.storage.SaveApp(*appFromStorage); err != nil {
					log.Warn("failed to update app icon", "error", err)
				}
			}
		}

		// Publish WebSocket event for route update
		if app.managers.WebSocket != nil {
			app.managers.WebSocket.PublishConfigReload()
		}

		if err := httputil.WriteJSON(w, http.StatusOK, payload.Route); err != nil {
			log.Error("failed to encode json response", "error", err)
		}

	default:
		apperrors.WriteJSON(w, apperrors.New("METHOD_NOT_ALLOWED", "Method not allowed", http.StatusMethodNotAllowed))
	}
}

// handleAdminInfo returns Nexus instance information
func (app *Application) handleAdminInfo(w http.ResponseWriter, r *http.Request) {
	info := map[string]interface{}{
		"version":      app.version,
		"nekzusId":     app.nekzusID,
		"capabilities": app.capabilities,
		"buildDate":    "2025-10-13",
	}
	if err := httputil.WriteJSON(w, http.StatusOK, info); err != nil {
		log.Error("failed to encode json response", "error", err)
	}
}

// handleWebSocketClients returns connected WebSocket clients for debugging
// GET /api/v1/admin/websocket/clients
func (app *Application) handleWebSocketClients(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	connectedDevices := app.managers.WebSocket.GetConnectedDevices()
	totalConnections := app.managers.WebSocket.ActiveConnections()

	response := map[string]interface{}{
		"totalConnections": totalConnections,
		"devices":          connectedDevices,
	}

	if err := httputil.WriteJSON(w, http.StatusOK, response); err != nil {
		log.Error("failed to encode json response", "error", err)
	}
}

// handleWebSocketDisconnect force disconnects WebSocket connections for a device
// DELETE /api/v1/admin/websocket/disconnect/{deviceId}
func (app *Application) handleWebSocketDisconnect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract device ID from path
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/admin/websocket/disconnect/")
	deviceID := strings.TrimSuffix(path, "/")

	if deviceID == "" {
		http.Error(w, "Device ID required", http.StatusBadRequest)
		return
	}

	disconnected := app.managers.WebSocket.DisconnectDevice(deviceID)

	log.Info("Force disconnected WebSocket connections",
		"device_id", deviceID,
		"connections_closed", disconnected)

	response := map[string]interface{}{
		"deviceId":          deviceID,
		"disconnectedCount": disconnected,
	}

	if err := httputil.WriteJSON(w, http.StatusOK, response); err != nil {
		log.Error("failed to encode json response", "error", err)
	}
}
