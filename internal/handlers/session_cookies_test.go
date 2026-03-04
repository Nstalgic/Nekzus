package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nstalgic/nekzus/internal/middleware"
	"github.com/nstalgic/nekzus/internal/storage"
)

// mockSessionCookieStore implements SessionCookieStore for testing
type mockSessionCookieStore struct {
	cookies       []storage.ProxyCookie
	listErr       error
	deleteAppErr  error
	deleteAllErr  error
	deletedAppID  string
	deletedDevice string
}

func (m *mockSessionCookieStore) ListProxyCookieSummaries(deviceID string) ([]storage.ProxyCookie, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	// Filter by device ID
	var result []storage.ProxyCookie
	for _, c := range m.cookies {
		if c.DeviceID == deviceID {
			result = append(result, c)
		}
	}
	return result, nil
}

func (m *mockSessionCookieStore) DeleteProxyCookiesForApp(deviceID, appID string) error {
	if m.deleteAppErr != nil {
		return m.deleteAppErr
	}
	m.deletedDevice = deviceID
	m.deletedAppID = appID
	return nil
}

func (m *mockSessionCookieStore) DeleteProxyCookiesForDevice(deviceID string) error {
	if m.deleteAllErr != nil {
		return m.deleteAllErr
	}
	m.deletedDevice = deviceID
	return nil
}

func TestSessionCookiesHandler_List(t *testing.T) {
	now := time.Now()
	expiry := now.Add(24 * time.Hour)

	store := &mockSessionCookieStore{
		cookies: []storage.ProxyCookie{
			{
				ID:         1,
				DeviceID:   "device-123",
				AppID:      "grafana",
				CookieName: "session_id",
				CookiePath: "/",
				ExpiresAt:  &expiry,
				Secure:     true,
				HttpOnly:   true,
				SameSite:   "Lax",
				CreatedAt:  now,
				UpdatedAt:  now,
			},
			{
				ID:         2,
				DeviceID:   "device-123",
				AppID:      "pihole",
				CookieName: "auth_token",
				CookiePath: "/admin",
				Secure:     false,
				HttpOnly:   false,
				SameSite:   "Strict",
				CreatedAt:  now,
				UpdatedAt:  now,
			},
		},
	}

	handler := NewSessionCookiesHandler(store)

	// Create request with device context
	req := httptest.NewRequest(http.MethodGet, "/api/session-cookies", nil)
	ctx := middleware.SetDeviceIDInContext(req.Context(), "device-123")
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	handler.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("List() status = %d, want %d", rr.Code, http.StatusOK)
	}

	var response SessionCookiesResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if len(response.Sessions) != 2 {
		t.Errorf("List() sessions = %d, want 2", len(response.Sessions))
	}

	// Verify first session
	if response.Sessions[0].AppID != "grafana" {
		t.Errorf("Sessions[0].AppID = %s, want grafana", response.Sessions[0].AppID)
	}
}

func TestSessionCookiesHandler_List_NoDeviceID(t *testing.T) {
	store := &mockSessionCookieStore{}
	handler := NewSessionCookiesHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/api/session-cookies", nil)
	rr := httptest.NewRecorder()
	handler.List(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("List() without device ID status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

func TestSessionCookiesHandler_List_EmptyResult(t *testing.T) {
	store := &mockSessionCookieStore{
		cookies: []storage.ProxyCookie{},
	}

	handler := NewSessionCookiesHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/api/session-cookies", nil)
	ctx := middleware.SetDeviceIDInContext(req.Context(), "device-123")
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	handler.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("List() status = %d, want %d", rr.Code, http.StatusOK)
	}

	var response SessionCookiesResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if len(response.Sessions) != 0 {
		t.Errorf("List() sessions = %d, want 0", len(response.Sessions))
	}
}

func TestSessionCookiesHandler_DeleteApp(t *testing.T) {
	store := &mockSessionCookieStore{}
	handler := NewSessionCookiesHandler(store)

	req := httptest.NewRequest(http.MethodDelete, "/api/session-cookies/grafana", nil)
	ctx := middleware.SetDeviceIDInContext(req.Context(), "device-123")
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	handler.DeleteApp(rr, req, "grafana")

	if rr.Code != http.StatusNoContent {
		t.Errorf("DeleteApp() status = %d, want %d", rr.Code, http.StatusNoContent)
	}

	if store.deletedDevice != "device-123" {
		t.Errorf("DeleteApp() deleted device = %s, want device-123", store.deletedDevice)
	}

	if store.deletedAppID != "grafana" {
		t.Errorf("DeleteApp() deleted app = %s, want grafana", store.deletedAppID)
	}
}

func TestSessionCookiesHandler_DeleteApp_NoDeviceID(t *testing.T) {
	store := &mockSessionCookieStore{}
	handler := NewSessionCookiesHandler(store)

	req := httptest.NewRequest(http.MethodDelete, "/api/session-cookies/grafana", nil)
	rr := httptest.NewRecorder()
	handler.DeleteApp(rr, req, "grafana")

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("DeleteApp() without device ID status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

func TestSessionCookiesHandler_DeleteAll(t *testing.T) {
	store := &mockSessionCookieStore{}
	handler := NewSessionCookiesHandler(store)

	req := httptest.NewRequest(http.MethodDelete, "/api/session-cookies", nil)
	ctx := middleware.SetDeviceIDInContext(req.Context(), "device-123")
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	handler.DeleteAll(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("DeleteAll() status = %d, want %d", rr.Code, http.StatusNoContent)
	}

	if store.deletedDevice != "device-123" {
		t.Errorf("DeleteAll() deleted device = %s, want device-123", store.deletedDevice)
	}
}

func TestSessionCookiesHandler_DeleteAll_NoDeviceID(t *testing.T) {
	store := &mockSessionCookieStore{}
	handler := NewSessionCookiesHandler(store)

	req := httptest.NewRequest(http.MethodDelete, "/api/session-cookies", nil)
	rr := httptest.NewRecorder()
	handler.DeleteAll(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("DeleteAll() without device ID status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

func TestSessionCookiesHandler_GroupByApp(t *testing.T) {
	now := time.Now()
	expiry := now.Add(24 * time.Hour)

	store := &mockSessionCookieStore{
		cookies: []storage.ProxyCookie{
			{DeviceID: "device-123", AppID: "grafana", CookieName: "session", ExpiresAt: &expiry, CreatedAt: now, UpdatedAt: now},
			{DeviceID: "device-123", AppID: "grafana", CookieName: "user_id", CreatedAt: now, UpdatedAt: now},
			{DeviceID: "device-123", AppID: "pihole", CookieName: "auth", CreatedAt: now, UpdatedAt: now},
		},
	}

	handler := NewSessionCookiesHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/api/session-cookies", nil)
	ctx := middleware.SetDeviceIDInContext(req.Context(), "device-123")
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	handler.List(rr, req)

	var response SessionCookiesResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	// Should have 2 app groups
	if len(response.Sessions) != 2 {
		t.Errorf("List() sessions = %d, want 2 (grouped by app)", len(response.Sessions))
	}

	// Find grafana session
	var grafanaSession *AppSession
	for i := range response.Sessions {
		if response.Sessions[i].AppID == "grafana" {
			grafanaSession = &response.Sessions[i]
			break
		}
	}

	if grafanaSession == nil {
		t.Fatal("Expected grafana session in response")
	}

	if grafanaSession.CookieCount != 2 {
		t.Errorf("grafana CookieCount = %d, want 2", grafanaSession.CookieCount)
	}
}
