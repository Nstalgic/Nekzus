package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nstalgic/nekzus/internal/middleware"
)

// setupDiscoveryTestRouter creates an HTTP mux with discovery endpoints and strict JWT auth
func setupDiscoveryTestRouter(app *Application) *http.ServeMux {
	mux := http.NewServeMux()

	// Create strict JWT auth middleware
	strictJWT := middleware.NewStrictJWTAuth(app.services.Auth, app.storage, app.metrics)

	// Discovery endpoints with strict JWT auth
	mux.Handle("/api/v1/discovery/proposals", strictJWT(http.HandlerFunc(app.handleListProposals)))
	mux.Handle("/api/v1/discovery/proposals/", strictJWT(http.HandlerFunc(app.handleProposalActions)))
	mux.Handle("/api/v1/discovery/rediscover", strictJWT(http.HandlerFunc(app.handleRediscover)))

	return mux
}

// TestDiscoverySecurityRevokedDevice tests that revoked devices cannot access discovery endpoints
// This is a critical security test for the vulnerability where revoked devices could still
// access discovery endpoints from local IPs
func TestDiscoverySecurityRevokedDevice(t *testing.T) {
	app, cleanup := newTestApplicationWithStorage(t)
	defer cleanup()

	mux := setupDiscoveryTestRouter(app)

	// Step 1: Pair a mobile device
	deviceID := "mobile-security-test-001"
	token, err := app.services.Auth.SignJWT(deviceID, []string{"read:*", "write:*"}, 24*time.Hour)
	if err != nil {
		t.Fatalf("Failed to sign JWT: %v", err)
	}

	// Create device in storage
	if err := app.storage.SaveDevice(deviceID, "Security Test Device", "ios", "17.0", []string{"read:*", "write:*"}); err != nil {
		t.Fatalf("Failed to create device: %v", err)
	}

	// Step 2: Verify device can access discovery endpoints with valid token
	req := httptest.NewRequest("GET", "/api/v1/discovery/proposals", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()

	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Valid device should access discovery endpoint, got status %d", rr.Code)
	}

	// Step 3: Revoke the device
	if err := app.storage.DeleteDevice(deviceID); err != nil {
		t.Fatalf("Failed to revoke device: %v", err)
	}

	// Step 4: Attempt to access discovery endpoint with the same valid JWT token
	// This should now FAIL because device has been revoked
	req = httptest.NewRequest("GET", "/api/v1/discovery/proposals", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr = httptest.NewRecorder()

	mux.ServeHTTP(rr, req)

	// CRITICAL SECURITY CHECK: Revoked device MUST be rejected
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("Revoked device should be rejected, got status %d (expected 401)", rr.Code)
		t.Errorf("Response body: %s", rr.Body.String())
	}

	body := rr.Body.String()
	if body != "device access revoked\n" {
		t.Errorf("Expected 'device access revoked' error, got: %q", body)
	}
}

// TestDiscoverySecurityLocalIPNoBypass tests that local IP addresses do NOT bypass authentication
// This tests the fix for the vulnerability where any device on the local network could access
// discovery endpoints without authentication
func TestDiscoverySecurityLocalIPNoBypass(t *testing.T) {
	app, cleanup := newTestApplicationWithStorage(t)
	defer cleanup()

	mux := setupDiscoveryTestRouter(app)

	localIPs := []string{
		"127.0.0.1:12345",
		"192.168.1.100:8080",
		"10.0.0.50:3000",
		"172.16.0.1:5000",
	}

	for _, remoteAddr := range localIPs {
		t.Run(fmt.Sprintf("LocalIP_%s", remoteAddr), func(t *testing.T) {
			// Attempt to access discovery endpoint without token from local IP
			req := httptest.NewRequest("GET", "/api/v1/discovery/proposals", nil)
			req.RemoteAddr = remoteAddr
			// No Authorization header

			rr := httptest.NewRecorder()
			mux.ServeHTTP(rr, req)

			// CRITICAL SECURITY CHECK: Local IP without token MUST be rejected
			if rr.Code != http.StatusUnauthorized {
				t.Errorf("Local IP %s without token should be rejected, got status %d (expected 401)", remoteAddr, rr.Code)
				t.Errorf("Response body: %s", rr.Body.String())
			}

			body := rr.Body.String()
			if body != "missing bearer token\n" {
				t.Errorf("Expected 'missing bearer token' error, got: %q", body)
			}
		})
	}
}

// TestDiscoverySecurityApproveWithRevokedDevice tests that revoked devices cannot approve proposals
func TestDiscoverySecurityApproveWithRevokedDevice(t *testing.T) {
	app, cleanup := newTestApplicationWithStorage(t)
	defer cleanup()

	mux := setupDiscoveryTestRouter(app)

	// Create a device and then revoke it
	deviceID := "revoked-approver-001"
	token, err := app.services.Auth.SignJWT(deviceID, []string{"read:*", "write:*"}, 24*time.Hour)
	if err != nil {
		t.Fatalf("Failed to sign JWT: %v", err)
	}

	if err := app.storage.SaveDevice(deviceID, "Revoked Approver", "android", "14.0", []string{"read:*", "write:*"}); err != nil {
		t.Fatalf("Failed to create device: %v", err)
	}

	// Revoke the device
	if err := app.storage.DeleteDevice(deviceID); err != nil {
		t.Fatalf("Failed to revoke device: %v", err)
	}

	// Attempt to approve a discovery proposal with revoked device's token
	proposalID := "test-proposal-123"
	req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/discovery/proposals/%s/approve", proposalID), nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()

	mux.ServeHTTP(rr, req)

	// CRITICAL SECURITY CHECK: Revoked device MUST NOT be able to approve proposals
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("Revoked device should not approve proposals, got status %d (expected 401)", rr.Code)
		t.Errorf("Response body: %s", rr.Body.String())
	}
}

// TestDiscoverySecurityDismissWithRevokedDevice tests that revoked devices cannot dismiss proposals
func TestDiscoverySecurityDismissWithRevokedDevice(t *testing.T) {
	app, cleanup := newTestApplicationWithStorage(t)
	defer cleanup()

	mux := setupDiscoveryTestRouter(app)

	// Create a device and then revoke it
	deviceID := "revoked-dismisser-001"
	token, err := app.services.Auth.SignJWT(deviceID, []string{"read:*", "write:*"}, 24*time.Hour)
	if err != nil {
		t.Fatalf("Failed to sign JWT: %v", err)
	}

	if err := app.storage.SaveDevice(deviceID, "Revoked Dismisser", "ios", "17.0", []string{"read:*", "write:*"}); err != nil {
		t.Fatalf("Failed to create device: %v", err)
	}

	// Revoke the device
	if err := app.storage.DeleteDevice(deviceID); err != nil {
		t.Fatalf("Failed to revoke device: %v", err)
	}

	// Attempt to dismiss a discovery proposal with revoked device's token
	proposalID := "test-proposal-456"
	req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/discovery/proposals/%s/dismiss", proposalID), nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()

	mux.ServeHTTP(rr, req)

	// CRITICAL SECURITY CHECK: Revoked device MUST NOT be able to dismiss proposals
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("Revoked device should not dismiss proposals, got status %d (expected 401)", rr.Code)
		t.Errorf("Response body: %s", rr.Body.String())
	}
}

// TestDiscoverySecurityRediscoverWithRevokedDevice tests that revoked devices cannot trigger rediscovery
func TestDiscoverySecurityRediscoverWithRevokedDevice(t *testing.T) {
	app, cleanup := newTestApplicationWithStorage(t)
	defer cleanup()

	mux := setupDiscoveryTestRouter(app)

	// Create a device and then revoke it
	deviceID := "revoked-rediscoverer-001"
	token, err := app.services.Auth.SignJWT(deviceID, []string{"read:*", "write:*"}, 24*time.Hour)
	if err != nil {
		t.Fatalf("Failed to sign JWT: %v", err)
	}

	if err := app.storage.SaveDevice(deviceID, "Revoked Rediscoverer", "android", "14.0", []string{"read:*", "write:*"}); err != nil {
		t.Fatalf("Failed to create device: %v", err)
	}

	// Revoke the device
	if err := app.storage.DeleteDevice(deviceID); err != nil {
		t.Fatalf("Failed to revoke device: %v", err)
	}

	// Attempt to trigger rediscovery with revoked device's token
	req := httptest.NewRequest("POST", "/api/v1/discovery/rediscover", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()

	mux.ServeHTTP(rr, req)

	// CRITICAL SECURITY CHECK: Revoked device MUST NOT be able to trigger rediscovery
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("Revoked device should not trigger rediscovery, got status %d (expected 401)", rr.Code)
		t.Errorf("Response body: %s", rr.Body.String())
	}
}

// TestDiscoverySecurityExpiredToken tests that expired tokens are rejected
func TestDiscoverySecurityExpiredToken(t *testing.T) {
	app, cleanup := newTestApplicationWithStorage(t)
	defer cleanup()

	mux := setupDiscoveryTestRouter(app)

	// Create a device with an expired token
	deviceID := "expired-token-device"
	expiredToken, err := app.services.Auth.SignJWT(deviceID, []string{"read:*"}, -1*time.Hour) // Expired 1 hour ago
	if err != nil {
		t.Fatalf("Failed to sign JWT: %v", err)
	}

	if err := app.storage.SaveDevice(deviceID, "Expired Token Device", "ios", "17.0", []string{"read:*"}); err != nil {
		t.Fatalf("Failed to create device: %v", err)
	}

	// Attempt to access discovery endpoint with expired token
	req := httptest.NewRequest("GET", "/api/v1/discovery/proposals", nil)
	req.Header.Set("Authorization", "Bearer "+expiredToken)
	rr := httptest.NewRecorder()

	mux.ServeHTTP(rr, req)

	// CRITICAL SECURITY CHECK: Expired token MUST be rejected
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("Expired token should be rejected, got status %d (expected 401)", rr.Code)
	}
}

// TestDiscoverySecurityValidDeviceAccess tests that valid devices CAN still access discovery
// This is a positive test case to ensure we didn't break legitimate access
func TestDiscoverySecurityValidDeviceAccess(t *testing.T) {
	app, cleanup := newTestApplicationWithStorage(t)
	defer cleanup()

	mux := setupDiscoveryTestRouter(app)

	// Create a valid device
	deviceID := "valid-access-device"
	token, err := app.services.Auth.SignJWT(deviceID, []string{"read:*", "write:*"}, 24*time.Hour)
	if err != nil {
		t.Fatalf("Failed to sign JWT: %v", err)
	}

	if err := app.storage.SaveDevice(deviceID, "Valid Access Device", "ios", "17.0", []string{"read:*", "write:*"}); err != nil {
		t.Fatalf("Failed to create device: %v", err)
	}

	// Access discovery endpoint with valid token
	req := httptest.NewRequest("GET", "/api/v1/discovery/proposals", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()

	mux.ServeHTTP(rr, req)

	// Valid device should be allowed access
	if rr.Code != http.StatusOK {
		t.Errorf("Valid device should access discovery endpoint, got status %d (expected 200)", rr.Code)
		t.Errorf("Response body: %s", rr.Body.String())
	}

	// Verify response is valid JSON array (discovery proposals)
	var proposals []interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &proposals); err != nil {
		t.Errorf("Response should be valid JSON array: %v", err)
	}
}
