package main

import (
	"net/http"

	apperrors "github.com/nstalgic/nekzus/internal/errors"
	"github.com/nstalgic/nekzus/internal/handlers"
)

// handleAPIKeys handles listing and creating API keys
func (app *Application) handleAPIKeys(w http.ResponseWriter, r *http.Request) {
	if app.storage == nil {
		apperrors.WriteJSON(w, apperrors.New("STORAGE_UNAVAILABLE", "Storage service is not available", http.StatusServiceUnavailable))
		return
	}

	handler := handlers.NewAPIKeyHandler(app.storage)

	switch r.Method {
	case http.MethodGet:
		handler.HandleListAPIKeys(w, r)
	case http.MethodPost:
		handler.HandleCreateAPIKey(w, r)
	default:
		apperrors.WriteJSON(w, apperrors.New("METHOD_NOT_ALLOWED", "Method not allowed", http.StatusMethodNotAllowed))
	}
}

// handleAPIKeyActions handles operations on specific API keys
func (app *Application) handleAPIKeyActions(w http.ResponseWriter, r *http.Request) {
	if app.storage == nil {
		apperrors.WriteJSON(w, apperrors.New("STORAGE_UNAVAILABLE", "Storage service is not available", http.StatusServiceUnavailable))
		return
	}

	handler := handlers.NewAPIKeyHandler(app.storage)

	switch r.Method {
	case http.MethodGet:
		handler.HandleGetAPIKey(w, r)
	case http.MethodDelete:
		handler.HandleRevokeAPIKey(w, r)
	default:
		apperrors.WriteJSON(w, apperrors.New("METHOD_NOT_ALLOWED", "Method not allowed", http.StatusMethodNotAllowed))
	}
}
