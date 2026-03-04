package proxy

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/nstalgic/nekzus/internal/crypto"
	"github.com/nstalgic/nekzus/internal/storage"
)

func setupTestSessionManager(t *testing.T) (*SessionCookieManager, func()) {
	t.Helper()

	// Create temp database
	tmpFile, err := os.CreateTemp("", "session_cookies_test_*.db")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	tmpFile.Close()

	store, err := storage.NewStore(storage.Config{DatabasePath: tmpFile.Name()})
	if err != nil {
		os.Remove(tmpFile.Name())
		t.Fatalf("failed to create store: %v", err)
	}

	// Create test device
	err = store.SaveDevice("test-device", "Test Device", "ios", "17.0", []string{"read:*"})
	if err != nil {
		store.Close()
		os.Remove(tmpFile.Name())
		t.Fatalf("failed to create test device: %v", err)
	}

	// Generate encryption key
	key, err := crypto.GenerateEncryptionKey()
	if err != nil {
		store.Close()
		os.Remove(tmpFile.Name())
		t.Fatalf("failed to generate key: %v", err)
	}

	encryptor, err := crypto.NewCookieEncryptor(key)
	if err != nil {
		store.Close()
		os.Remove(tmpFile.Name())
		t.Fatalf("failed to create encryptor: %v", err)
	}

	manager := NewSessionCookieManager(store, encryptor, key)

	cleanup := func() {
		store.Close()
		os.Remove(tmpFile.Name())
	}

	return manager, cleanup
}

func TestSessionCookieManager_CaptureAndInject(t *testing.T) {
	manager, cleanup := setupTestSessionManager(t)
	defer cleanup()

	// Simulate capturing cookies from a backend response
	cookies := []*http.Cookie{
		{
			Name:     "session_id",
			Value:    "abc123",
			Path:     "/",
			HttpOnly: true,
			Secure:   true,
		},
		{
			Name:  "user_pref",
			Value: "dark_mode",
			Path:  "/settings",
		},
	}

	err := manager.CaptureResponseCookies("test-device", "grafana", cookies)
	if err != nil {
		t.Fatalf("CaptureResponseCookies() error = %v", err)
	}

	// Create a new request and inject stored cookies
	req := httptest.NewRequest("GET", "http://example.com/", nil)
	err = manager.InjectRequestCookies("test-device", "grafana", req)
	if err != nil {
		t.Fatalf("InjectRequestCookies() error = %v", err)
	}

	// Verify cookies were injected
	injectedCookies := req.Cookies()
	if len(injectedCookies) != 2 {
		t.Fatalf("Expected 2 cookies, got %d", len(injectedCookies))
	}

	// Check specific cookies
	found := make(map[string]string)
	for _, c := range injectedCookies {
		found[c.Name] = c.Value
	}

	if found["session_id"] != "abc123" {
		t.Errorf("session_id = %v, want abc123", found["session_id"])
	}
	if found["user_pref"] != "dark_mode" {
		t.Errorf("user_pref = %v, want dark_mode", found["user_pref"])
	}
}

func TestSessionCookieManager_CaptureWithExpiry(t *testing.T) {
	manager, cleanup := setupTestSessionManager(t)
	defer cleanup()

	// Cookie that expires in the future
	futureExpiry := time.Now().Add(time.Hour)
	cookies := []*http.Cookie{
		{
			Name:    "valid",
			Value:   "future",
			Path:    "/",
			Expires: futureExpiry,
		},
	}

	err := manager.CaptureResponseCookies("test-device", "grafana", cookies)
	if err != nil {
		t.Fatalf("CaptureResponseCookies() error = %v", err)
	}

	// Should be injected
	req := httptest.NewRequest("GET", "http://example.com/", nil)
	manager.InjectRequestCookies("test-device", "grafana", req)

	if len(req.Cookies()) != 1 {
		t.Errorf("Expected 1 cookie, got %d", len(req.Cookies()))
	}
}

func TestSessionCookieManager_SkipExpiredCookies(t *testing.T) {
	manager, cleanup := setupTestSessionManager(t)
	defer cleanup()

	// Already expired cookie should not be captured
	expiredTime := time.Now().Add(-time.Hour)
	cookies := []*http.Cookie{
		{
			Name:    "expired",
			Value:   "past",
			Path:    "/",
			Expires: expiredTime,
		},
	}

	err := manager.CaptureResponseCookies("test-device", "grafana", cookies)
	if err != nil {
		t.Fatalf("CaptureResponseCookies() error = %v", err)
	}

	// Should not be injected
	req := httptest.NewRequest("GET", "http://example.com/", nil)
	manager.InjectRequestCookies("test-device", "grafana", req)

	if len(req.Cookies()) != 0 {
		t.Errorf("Expected 0 cookies (expired filtered), got %d", len(req.Cookies()))
	}
}

func TestSessionCookieManager_DeviceIsolation(t *testing.T) {
	manager, cleanup := setupTestSessionManager(t)
	defer cleanup()

	// Add second device
	manager.store.SaveDevice("device-2", "Device 2", "android", "14", []string{"read:*"})

	// Capture cookie for device 1
	manager.CaptureResponseCookies("test-device", "grafana", []*http.Cookie{
		{Name: "session", Value: "device1_session", Path: "/"},
	})

	// Capture cookie for device 2
	manager.CaptureResponseCookies("device-2", "grafana", []*http.Cookie{
		{Name: "session", Value: "device2_session", Path: "/"},
	})

	// Inject for device 1
	req1 := httptest.NewRequest("GET", "http://example.com/", nil)
	manager.InjectRequestCookies("test-device", "grafana", req1)

	// Inject for device 2
	req2 := httptest.NewRequest("GET", "http://example.com/", nil)
	manager.InjectRequestCookies("device-2", "grafana", req2)

	// Verify isolation
	cookies1 := req1.Cookies()
	cookies2 := req2.Cookies()

	if len(cookies1) != 1 || cookies1[0].Value != "device1_session" {
		t.Errorf("Device 1 got wrong cookies: %v", cookies1)
	}
	if len(cookies2) != 1 || cookies2[0].Value != "device2_session" {
		t.Errorf("Device 2 got wrong cookies: %v", cookies2)
	}
}

func TestSessionCookieManager_AppIsolation(t *testing.T) {
	manager, cleanup := setupTestSessionManager(t)
	defer cleanup()

	// Capture cookies for different apps
	manager.CaptureResponseCookies("test-device", "grafana", []*http.Cookie{
		{Name: "session", Value: "grafana_session", Path: "/"},
	})
	manager.CaptureResponseCookies("test-device", "pihole", []*http.Cookie{
		{Name: "session", Value: "pihole_session", Path: "/"},
	})

	// Inject for grafana
	reqGrafana := httptest.NewRequest("GET", "http://example.com/", nil)
	manager.InjectRequestCookies("test-device", "grafana", reqGrafana)

	// Inject for pihole
	reqPihole := httptest.NewRequest("GET", "http://example.com/", nil)
	manager.InjectRequestCookies("test-device", "pihole", reqPihole)

	grafanaCookies := reqGrafana.Cookies()
	piholeCookies := reqPihole.Cookies()

	if len(grafanaCookies) != 1 || grafanaCookies[0].Value != "grafana_session" {
		t.Errorf("Grafana got wrong cookies: %v", grafanaCookies)
	}
	if len(piholeCookies) != 1 || piholeCookies[0].Value != "pihole_session" {
		t.Errorf("Pihole got wrong cookies: %v", piholeCookies)
	}
}

func TestSessionCookieManager_ClearCookies(t *testing.T) {
	manager, cleanup := setupTestSessionManager(t)
	defer cleanup()

	// Capture some cookies
	manager.CaptureResponseCookies("test-device", "grafana", []*http.Cookie{
		{Name: "session", Value: "abc", Path: "/"},
	})

	// Clear cookies
	err := manager.ClearCookies("test-device", "grafana")
	if err != nil {
		t.Fatalf("ClearCookies() error = %v", err)
	}

	// Verify cleared
	req := httptest.NewRequest("GET", "http://example.com/", nil)
	manager.InjectRequestCookies("test-device", "grafana", req)

	if len(req.Cookies()) != 0 {
		t.Errorf("Expected 0 cookies after clear, got %d", len(req.Cookies()))
	}
}

func TestSessionCookieManager_UpdateExistingCookie(t *testing.T) {
	manager, cleanup := setupTestSessionManager(t)
	defer cleanup()

	// Capture initial cookie
	manager.CaptureResponseCookies("test-device", "grafana", []*http.Cookie{
		{Name: "session", Value: "original", Path: "/"},
	})

	// Update with new value
	manager.CaptureResponseCookies("test-device", "grafana", []*http.Cookie{
		{Name: "session", Value: "updated", Path: "/"},
	})

	// Verify updated
	req := httptest.NewRequest("GET", "http://example.com/", nil)
	manager.InjectRequestCookies("test-device", "grafana", req)

	cookies := req.Cookies()
	if len(cookies) != 1 {
		t.Fatalf("Expected 1 cookie, got %d", len(cookies))
	}
	if cookies[0].Value != "updated" {
		t.Errorf("Cookie value = %v, want updated", cookies[0].Value)
	}
}

func TestSessionCookieManager_EmptyDeviceID(t *testing.T) {
	manager, cleanup := setupTestSessionManager(t)
	defer cleanup()

	// Empty device ID should be handled gracefully
	err := manager.CaptureResponseCookies("", "grafana", []*http.Cookie{
		{Name: "session", Value: "abc", Path: "/"},
	})
	if err == nil {
		t.Error("Expected error for empty device ID")
	}

	req := httptest.NewRequest("GET", "http://example.com/", nil)
	err = manager.InjectRequestCookies("", "grafana", req)
	if err == nil {
		t.Error("Expected error for empty device ID on inject")
	}
}
