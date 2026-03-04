package main

import (
	"net/http"

	apperrors "github.com/nstalgic/nekzus/internal/errors"
	"github.com/nstalgic/nekzus/internal/handlers"
)

// handleDevices handles device list operations
func (app *Application) handleDevices(w http.ResponseWriter, r *http.Request) {
	// Authentication is handled by IP-based middleware
	// Local requests bypass JWT, external requests are validated before reaching here

	if app.storage == nil {
		apperrors.WriteJSON(w, apperrors.New("STORAGE_UNAVAILABLE", "Storage service is not available", http.StatusServiceUnavailable))
		return
	}

	handler := handlers.NewDeviceHandler(app.storage)
	handler.SetWebSocketManager(app.managers.WebSocket)
	handler.SetActivityTracker(app.managers.Activity)
	handler.HandleListDevices(w, r)
}

// handleAdminDevices handles device listing for admin UI (no JWT required)
func (app *Application) handleAdminDevices(w http.ResponseWriter, r *http.Request) {
	if app.storage == nil {
		apperrors.WriteJSON(w, apperrors.New("STORAGE_UNAVAILABLE", "Storage service is not available", http.StatusServiceUnavailable))
		return
	}

	handler := handlers.NewDeviceHandler(app.storage)
	handler.SetWebSocketManager(app.managers.WebSocket)
	handler.SetActivityTracker(app.managers.Activity)
	handler.HandleListDevices(w, r)
}

// handleDeviceActions handles device-specific operations (get, revoke, update)
func (app *Application) handleDeviceActions(w http.ResponseWriter, r *http.Request) {
	app.handleDeviceActionsWithPrefix(w, r, "/api/v1/devices/")
}

// handleAdminDeviceActions handles device-specific operations for admin UI (no JWT required)
func (app *Application) handleAdminDeviceActions(w http.ResponseWriter, r *http.Request) {
	app.handleDeviceActionsWithPrefix(w, r, "/api/v1/admin/devices/")
}

// handleDeviceActionsWithPrefix handles device-specific operations with a given path prefix
func (app *Application) handleDeviceActionsWithPrefix(w http.ResponseWriter, r *http.Request, pathPrefix string) {
	// Authentication is handled by IP-based middleware
	// Local requests bypass JWT, external requests are validated before reaching here

	if app.storage == nil {
		apperrors.WriteJSON(w, apperrors.New("STORAGE_UNAVAILABLE", "Storage service is not available", http.StatusServiceUnavailable))
		return
	}

	handler := handlers.NewDeviceHandler(app.storage)
	handler.SetWebSocketManager(app.managers.WebSocket)
	handler.SetActivityTracker(app.managers.Activity)

	switch r.Method {
	case http.MethodGet:
		handler.HandleGetDevice(w, r)
	case http.MethodDelete:
		// Revoke the device (handler now handles disconnect and events)
		handler.HandleRevokeDevice(w, r)
	case http.MethodPatch:
		handler.HandleUpdateDeviceMetadata(w, r)
	default:
		apperrors.WriteJSON(w, apperrors.New("METHOD_NOT_ALLOWED", "Method not allowed", http.StatusMethodNotAllowed))
	}
}
