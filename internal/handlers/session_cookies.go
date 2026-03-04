package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	apperrors "github.com/nstalgic/nekzus/internal/errors"
	"github.com/nstalgic/nekzus/internal/middleware"
	"github.com/nstalgic/nekzus/internal/storage"
)

var sessionCookiesLog = slog.With("package", "handlers", "handler", "session_cookies")

// SessionCookieStore defines the storage interface for session cookie operations.
type SessionCookieStore interface {
	ListProxyCookieSummaries(deviceID string) ([]storage.ProxyCookie, error)
	DeleteProxyCookiesForApp(deviceID, appID string) error
	DeleteProxyCookiesForDevice(deviceID string) error
}

// SessionCookiesHandler handles session cookie management API endpoints.
type SessionCookiesHandler struct {
	store SessionCookieStore
}

// NewSessionCookiesHandler creates a new session cookies handler.
func NewSessionCookiesHandler(store SessionCookieStore) *SessionCookiesHandler {
	return &SessionCookiesHandler{
		store: store,
	}
}

// AppSession represents a summary of stored cookies for an app.
type AppSession struct {
	AppID       string     `json:"appId"`
	CookieCount int        `json:"cookieCount"`
	LastUpdated time.Time  `json:"lastUpdated"`
	ExpiresAt   *time.Time `json:"expiresAt,omitempty"` // Earliest expiry if any
}

// SessionCookiesResponse is the response for listing session cookies.
type SessionCookiesResponse struct {
	Sessions []AppSession `json:"sessions"`
}

// List returns a summary of stored session cookies for the current device.
// GET /api/session-cookies
func (h *SessionCookiesHandler) List(w http.ResponseWriter, r *http.Request) {
	deviceID := middleware.GetDeviceIDFromContext(r.Context())
	if deviceID == "" {
		apperrors.WriteJSON(w, apperrors.ErrUnauthorized)
		return
	}

	cookies, err := h.store.ListProxyCookieSummaries(deviceID)
	if err != nil {
		sessionCookiesLog.Error("failed to list session cookies",
			"device_id", deviceID,
			"error", err)
		apperrors.WriteJSON(w, apperrors.ErrInternalServer)
		return
	}

	// Group cookies by app
	sessions := groupCookiesByApp(cookies)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(SessionCookiesResponse{Sessions: sessions}); err != nil {
		sessionCookiesLog.Error("failed to encode response", "error", err)
	}
}

// DeleteApp clears all stored session cookies for a specific app.
// DELETE /api/session-cookies/{appId}
func (h *SessionCookiesHandler) DeleteApp(w http.ResponseWriter, r *http.Request, appID string) {
	deviceID := middleware.GetDeviceIDFromContext(r.Context())
	if deviceID == "" {
		apperrors.WriteJSON(w, apperrors.ErrUnauthorized)
		return
	}

	if appID == "" {
		apperrors.WriteJSON(w, apperrors.New("INVALID_REQUEST", "App ID is required", http.StatusBadRequest))
		return
	}

	if err := h.store.DeleteProxyCookiesForApp(deviceID, appID); err != nil {
		sessionCookiesLog.Error("failed to delete session cookies for app",
			"device_id", deviceID,
			"app_id", appID,
			"error", err)
		apperrors.WriteJSON(w, apperrors.ErrInternalServer)
		return
	}

	sessionCookiesLog.Info("session cookies deleted for app",
		"device_id", deviceID,
		"app_id", appID)

	w.WriteHeader(http.StatusNoContent)
}

// DeleteAll clears all stored session cookies for the current device.
// DELETE /api/session-cookies
func (h *SessionCookiesHandler) DeleteAll(w http.ResponseWriter, r *http.Request) {
	deviceID := middleware.GetDeviceIDFromContext(r.Context())
	if deviceID == "" {
		apperrors.WriteJSON(w, apperrors.ErrUnauthorized)
		return
	}

	if err := h.store.DeleteProxyCookiesForDevice(deviceID); err != nil {
		sessionCookiesLog.Error("failed to delete all session cookies",
			"device_id", deviceID,
			"error", err)
		apperrors.WriteJSON(w, apperrors.ErrInternalServer)
		return
	}

	sessionCookiesLog.Info("all session cookies deleted for device",
		"device_id", deviceID)

	w.WriteHeader(http.StatusNoContent)
}

// groupCookiesByApp groups cookies by app ID and returns session summaries.
func groupCookiesByApp(cookies []storage.ProxyCookie) []AppSession {
	if len(cookies) == 0 {
		return []AppSession{}
	}

	// Group by app ID
	appMap := make(map[string]*AppSession)
	for _, c := range cookies {
		session, exists := appMap[c.AppID]
		if !exists {
			session = &AppSession{
				AppID:       c.AppID,
				CookieCount: 0,
				LastUpdated: c.UpdatedAt,
			}
			appMap[c.AppID] = session
		}

		session.CookieCount++

		// Track the most recent update
		if c.UpdatedAt.After(session.LastUpdated) {
			session.LastUpdated = c.UpdatedAt
		}

		// Track the earliest expiry (if any)
		if c.ExpiresAt != nil {
			if session.ExpiresAt == nil || c.ExpiresAt.Before(*session.ExpiresAt) {
				session.ExpiresAt = c.ExpiresAt
			}
		}
	}

	// Convert map to slice
	sessions := make([]AppSession, 0, len(appMap))
	for _, session := range appMap {
		sessions = append(sessions, *session)
	}

	return sessions
}
