package storage

import (
	"os"
	"testing"
	"time"

	"github.com/nstalgic/nekzus/internal/crypto"
)

func setupTestStoreWithCookies(t *testing.T) (*Store, []byte, func()) {
	t.Helper()

	// Create temp database
	tmpFile, err := os.CreateTemp("", "proxy_cookies_test_*.db")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	tmpFile.Close()

	store, err := NewStore(Config{DatabasePath: tmpFile.Name()})
	if err != nil {
		os.Remove(tmpFile.Name())
		t.Fatalf("failed to create store: %v", err)
	}

	// Create a test device first (required for FK constraint)
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

	cleanup := func() {
		store.Close()
		os.Remove(tmpFile.Name())
	}

	return store, key, cleanup
}

func TestSaveProxyCookie(t *testing.T) {
	store, key, cleanup := setupTestStoreWithCookies(t)
	defer cleanup()

	cookie := ProxyCookie{
		DeviceID:     "test-device",
		AppID:        "grafana",
		CookieName:   "grafana_session",
		CookieValue:  "abc123xyz",
		CookiePath:   "/",
		CookieDomain: "grafana.local",
		Secure:       true,
		HttpOnly:     true,
		SameSite:     "Strict",
	}

	err := store.SaveProxyCookie(cookie, key)
	if err != nil {
		t.Fatalf("SaveProxyCookie() error = %v", err)
	}

	// Retrieve and verify
	cookies, err := store.GetProxyCookies("test-device", "grafana", key)
	if err != nil {
		t.Fatalf("GetProxyCookies() error = %v", err)
	}

	if len(cookies) != 1 {
		t.Fatalf("GetProxyCookies() returned %d cookies, want 1", len(cookies))
	}

	got := cookies[0]
	if got.CookieValue != cookie.CookieValue {
		t.Errorf("CookieValue = %v, want %v", got.CookieValue, cookie.CookieValue)
	}
	if got.CookieName != cookie.CookieName {
		t.Errorf("CookieName = %v, want %v", got.CookieName, cookie.CookieName)
	}
	if got.Secure != cookie.Secure {
		t.Errorf("Secure = %v, want %v", got.Secure, cookie.Secure)
	}
}

func TestSaveProxyCookie_Update(t *testing.T) {
	store, key, cleanup := setupTestStoreWithCookies(t)
	defer cleanup()

	cookie := ProxyCookie{
		DeviceID:    "test-device",
		AppID:       "grafana",
		CookieName:  "session",
		CookieValue: "original",
		CookiePath:  "/",
	}

	// Save original
	err := store.SaveProxyCookie(cookie, key)
	if err != nil {
		t.Fatalf("SaveProxyCookie() error = %v", err)
	}

	// Update with new value
	cookie.CookieValue = "updated"
	err = store.SaveProxyCookie(cookie, key)
	if err != nil {
		t.Fatalf("SaveProxyCookie() update error = %v", err)
	}

	// Verify update
	cookies, _ := store.GetProxyCookies("test-device", "grafana", key)
	if len(cookies) != 1 {
		t.Fatalf("Expected 1 cookie after update, got %d", len(cookies))
	}
	if cookies[0].CookieValue != "updated" {
		t.Errorf("CookieValue = %v, want updated", cookies[0].CookieValue)
	}
}

func TestGetProxyCookies_FilterExpired(t *testing.T) {
	store, key, cleanup := setupTestStoreWithCookies(t)
	defer cleanup()

	// Valid cookie (expires in future)
	validCookie := ProxyCookie{
		DeviceID:    "test-device",
		AppID:       "grafana",
		CookieName:  "valid",
		CookieValue: "valid_value",
		CookiePath:  "/",
		ExpiresAt:   ptrTime(time.Now().Add(time.Hour)),
	}

	// Expired cookie
	expiredCookie := ProxyCookie{
		DeviceID:    "test-device",
		AppID:       "grafana",
		CookieName:  "expired",
		CookieValue: "expired_value",
		CookiePath:  "/",
		ExpiresAt:   ptrTime(time.Now().Add(-time.Hour)),
	}

	// Session cookie (no expiry)
	sessionCookie := ProxyCookie{
		DeviceID:    "test-device",
		AppID:       "grafana",
		CookieName:  "session",
		CookieValue: "session_value",
		CookiePath:  "/",
		ExpiresAt:   nil,
	}

	store.SaveProxyCookie(validCookie, key)
	store.SaveProxyCookie(expiredCookie, key)
	store.SaveProxyCookie(sessionCookie, key)

	cookies, err := store.GetProxyCookies("test-device", "grafana", key)
	if err != nil {
		t.Fatalf("GetProxyCookies() error = %v", err)
	}

	// Should only return valid and session cookies (not expired)
	if len(cookies) != 2 {
		t.Errorf("GetProxyCookies() returned %d cookies, want 2 (valid + session)", len(cookies))
	}

	names := make(map[string]bool)
	for _, c := range cookies {
		names[c.CookieName] = true
	}

	if !names["valid"] || !names["session"] {
		t.Errorf("Expected valid and session cookies, got %v", names)
	}
	if names["expired"] {
		t.Error("Expired cookie should have been filtered out")
	}
}

func TestGetProxyCookies_DeviceIsolation(t *testing.T) {
	store, key, cleanup := setupTestStoreWithCookies(t)
	defer cleanup()

	// Create second device
	store.SaveDevice("device-2", "Device 2", "android", "14.0", []string{"read:*"})

	// Save cookie for each device
	store.SaveProxyCookie(ProxyCookie{
		DeviceID:    "test-device",
		AppID:       "grafana",
		CookieName:  "session",
		CookieValue: "device1_session",
		CookiePath:  "/",
	}, key)

	store.SaveProxyCookie(ProxyCookie{
		DeviceID:    "device-2",
		AppID:       "grafana",
		CookieName:  "session",
		CookieValue: "device2_session",
		CookiePath:  "/",
	}, key)

	// Verify isolation
	cookies1, _ := store.GetProxyCookies("test-device", "grafana", key)
	cookies2, _ := store.GetProxyCookies("device-2", "grafana", key)

	if len(cookies1) != 1 || cookies1[0].CookieValue != "device1_session" {
		t.Errorf("Device 1 cookies incorrect: %v", cookies1)
	}
	if len(cookies2) != 1 || cookies2[0].CookieValue != "device2_session" {
		t.Errorf("Device 2 cookies incorrect: %v", cookies2)
	}
}

func TestDeleteProxyCookiesForApp(t *testing.T) {
	store, key, cleanup := setupTestStoreWithCookies(t)
	defer cleanup()

	// Save cookies for different apps
	store.SaveProxyCookie(ProxyCookie{
		DeviceID: "test-device", AppID: "grafana", CookieName: "s1", CookieValue: "v1", CookiePath: "/",
	}, key)
	store.SaveProxyCookie(ProxyCookie{
		DeviceID: "test-device", AppID: "grafana", CookieName: "s2", CookieValue: "v2", CookiePath: "/",
	}, key)
	store.SaveProxyCookie(ProxyCookie{
		DeviceID: "test-device", AppID: "pihole", CookieName: "s1", CookieValue: "v3", CookiePath: "/",
	}, key)

	// Delete grafana cookies
	err := store.DeleteProxyCookiesForApp("test-device", "grafana")
	if err != nil {
		t.Fatalf("DeleteProxyCookiesForApp() error = %v", err)
	}

	// Verify grafana cookies deleted
	grafanaCookies, _ := store.GetProxyCookies("test-device", "grafana", key)
	if len(grafanaCookies) != 0 {
		t.Errorf("Expected 0 grafana cookies, got %d", len(grafanaCookies))
	}

	// Verify pihole cookies still exist
	piholeCookies, _ := store.GetProxyCookies("test-device", "pihole", key)
	if len(piholeCookies) != 1 {
		t.Errorf("Expected 1 pihole cookie, got %d", len(piholeCookies))
	}
}

func TestDeleteProxyCookiesForDevice(t *testing.T) {
	store, key, cleanup := setupTestStoreWithCookies(t)
	defer cleanup()

	// Create second device
	store.SaveDevice("device-2", "Device 2", "android", "14.0", []string{"read:*"})

	// Save cookies for both devices
	store.SaveProxyCookie(ProxyCookie{
		DeviceID: "test-device", AppID: "grafana", CookieName: "s1", CookieValue: "v1", CookiePath: "/",
	}, key)
	store.SaveProxyCookie(ProxyCookie{
		DeviceID: "device-2", AppID: "grafana", CookieName: "s1", CookieValue: "v2", CookiePath: "/",
	}, key)

	// Delete all cookies for test-device
	err := store.DeleteProxyCookiesForDevice("test-device")
	if err != nil {
		t.Fatalf("DeleteProxyCookiesForDevice() error = %v", err)
	}

	// Verify test-device cookies deleted
	cookies1, _ := store.GetProxyCookies("test-device", "grafana", key)
	if len(cookies1) != 0 {
		t.Errorf("Expected 0 cookies for test-device, got %d", len(cookies1))
	}

	// Verify device-2 cookies still exist
	cookies2, _ := store.GetProxyCookies("device-2", "grafana", key)
	if len(cookies2) != 1 {
		t.Errorf("Expected 1 cookie for device-2, got %d", len(cookies2))
	}
}

func TestCleanupExpiredProxyCookies(t *testing.T) {
	store, key, cleanup := setupTestStoreWithCookies(t)
	defer cleanup()

	// Save expired cookie
	store.SaveProxyCookie(ProxyCookie{
		DeviceID:    "test-device",
		AppID:       "grafana",
		CookieName:  "expired",
		CookieValue: "v1",
		CookiePath:  "/",
		ExpiresAt:   ptrTime(time.Now().Add(-time.Hour)),
	}, key)

	// Save valid cookie
	store.SaveProxyCookie(ProxyCookie{
		DeviceID:    "test-device",
		AppID:       "grafana",
		CookieName:  "valid",
		CookieValue: "v2",
		CookiePath:  "/",
		ExpiresAt:   ptrTime(time.Now().Add(time.Hour)),
	}, key)

	// Run cleanup
	deleted, err := store.CleanupExpiredProxyCookies()
	if err != nil {
		t.Fatalf("CleanupExpiredProxyCookies() error = %v", err)
	}

	if deleted != 1 {
		t.Errorf("CleanupExpiredProxyCookies() deleted %d, want 1", deleted)
	}
}

func ptrTime(t time.Time) *time.Time {
	return &t
}
