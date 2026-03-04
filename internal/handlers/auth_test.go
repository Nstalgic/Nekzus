package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nstalgic/nekzus/internal/auth"
	"github.com/nstalgic/nekzus/internal/ratelimit"
)

// TestHandleVerifyToken_ValidToken tests verification with a valid token
func TestHandleVerifyToken_ValidToken(t *testing.T) {
	authMgr, err := auth.NewManager(
		[]byte("xk7f9q2m4n8b1v3c5z0w6y8u0i2o4p6a"),
		"test-issuer",
		"test-audience",
		[]string{},
	)
	if err != nil {
		t.Fatalf("Failed to create auth manager: %v", err)
	}
	defer authMgr.Stop()

	handler := NewAuthHandler(
		authMgr,
		nil, nil, nil, nil,
		ratelimit.NewLimiter(1.0, 5),
		nil,
		"https://test:8443",
		"",
		"test-nexus-id",
		"1.0.0",
		[]string{"catalog"},
	)

	// Sign a valid token
	deviceID := "test-device-123"
	token, err := authMgr.SignJWT(deviceID, []string{"read"}, time.Hour)
	if err != nil {
		t.Fatalf("Failed to sign JWT: %v", err)
	}

	// Create request with token
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/verify", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	handler.HandleVerifyToken(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var response map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// Check valid field
	valid, ok := response["valid"].(bool)
	if !ok || !valid {
		t.Error("Expected valid: true in response")
	}

	// Check deviceId field
	respDeviceID, ok := response["deviceId"].(string)
	if !ok || respDeviceID != deviceID {
		t.Errorf("Expected deviceId %s, got %v", deviceID, response["deviceId"])
	}

	// Check expiresAt field exists
	if _, ok := response["expiresAt"].(string); !ok {
		t.Error("Expected expiresAt field in response")
	}
}

// TestHandleVerifyToken_NoToken tests verification without a token
func TestHandleVerifyToken_NoToken(t *testing.T) {
	authMgr, _ := auth.NewManager(
		[]byte("xk7f9q2m4n8b1v3c5z0w6y8u0i2o4p6a"),
		"test-issuer",
		"test-audience",
		[]string{},
	)
	defer authMgr.Stop()

	handler := NewAuthHandler(
		authMgr,
		nil, nil, nil, nil,
		ratelimit.NewLimiter(1.0, 5),
		nil,
		"https://test:8443",
		"",
		"test-nexus-id",
		"1.0.0",
		[]string{"catalog"},
	)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/verify", nil)
	w := httptest.NewRecorder()

	handler.HandleVerifyToken(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401, got %d", w.Code)
	}

	var response map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// Check error response
	errorCode, ok := response["error"].(string)
	if !ok || errorCode != "AUTH_REQUIRED" {
		t.Errorf("Expected error AUTH_REQUIRED, got %v", response["error"])
	}
}

// TestHandleVerifyToken_InvalidToken tests verification with an invalid token
func TestHandleVerifyToken_InvalidToken(t *testing.T) {
	authMgr, _ := auth.NewManager(
		[]byte("xk7f9q2m4n8b1v3c5z0w6y8u0i2o4p6a"),
		"test-issuer",
		"test-audience",
		[]string{},
	)
	defer authMgr.Stop()

	handler := NewAuthHandler(
		authMgr,
		nil, nil, nil, nil,
		ratelimit.NewLimiter(1.0, 5),
		nil,
		"https://test:8443",
		"",
		"test-nexus-id",
		"1.0.0",
		[]string{"catalog"},
	)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/verify", nil)
	req.Header.Set("Authorization", "Bearer invalid-token-string")
	w := httptest.NewRecorder()

	handler.HandleVerifyToken(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401, got %d", w.Code)
	}
}

// TestHandleVerifyToken_RevokedToken tests verification with a revoked token
func TestHandleVerifyToken_RevokedToken(t *testing.T) {
	authMgr, _ := auth.NewManager(
		[]byte("xk7f9q2m4n8b1v3c5z0w6y8u0i2o4p6a"),
		"test-issuer",
		"test-audience",
		[]string{},
	)
	defer authMgr.Stop()

	handler := NewAuthHandler(
		authMgr,
		nil, nil, nil, nil,
		ratelimit.NewLimiter(1.0, 5),
		nil,
		"https://test:8443",
		"",
		"test-nexus-id",
		"1.0.0",
		[]string{"catalog"},
	)

	// Sign a token then revoke it
	token, _ := authMgr.SignJWT("test-device", []string{"read"}, time.Hour)
	authMgr.RevokeToken(token, time.Now().Add(time.Hour))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/verify", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	handler.HandleVerifyToken(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401, got %d", w.Code)
	}

	var response map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	errorCode, ok := response["error"].(string)
	if !ok || errorCode != "TOKEN_REVOKED" {
		t.Errorf("Expected error TOKEN_REVOKED, got %v", response["error"])
	}

	// Check numeric code
	code, ok := response["code"].(float64)
	if !ok || int(code) != 1003 {
		t.Errorf("Expected code 1003, got %v", response["code"])
	}
}

// TestHandleVerifyToken_RevokedDevice tests verification for a revoked device
func TestHandleVerifyToken_RevokedDevice(t *testing.T) {
	authMgr, _ := auth.NewManager(
		[]byte("xk7f9q2m4n8b1v3c5z0w6y8u0i2o4p6a"),
		"test-issuer",
		"test-audience",
		[]string{},
	)
	defer authMgr.Stop()

	handler := NewAuthHandler(
		authMgr,
		nil, nil, nil, nil,
		ratelimit.NewLimiter(1.0, 5),
		nil,
		"https://test:8443",
		"",
		"test-nexus-id",
		"1.0.0",
		[]string{"catalog"},
	)

	deviceID := "device-to-revoke"
	token, _ := authMgr.SignJWT(deviceID, []string{"read"}, time.Hour)
	authMgr.RevokeDevice(deviceID, time.Now().Add(time.Hour))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/verify", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	handler.HandleVerifyToken(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401, got %d", w.Code)
	}

	var response map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	errorCode, ok := response["error"].(string)
	if !ok || errorCode != "DEVICE_REVOKED" {
		t.Errorf("Expected error DEVICE_REVOKED, got %v", response["error"])
	}

	// Check numeric code (1004 for device revoked)
	code, ok := response["code"].(float64)
	if !ok || int(code) != 1004 {
		t.Errorf("Expected code 1004, got %v", response["code"])
	}
}

// TestHandleVerifyToken_MethodNotAllowed tests that only GET is allowed
func TestHandleVerifyToken_MethodNotAllowed(t *testing.T) {
	authMgr, _ := auth.NewManager(
		[]byte("xk7f9q2m4n8b1v3c5z0w6y8u0i2o4p6a"),
		"test-issuer",
		"test-audience",
		[]string{},
	)
	defer authMgr.Stop()

	handler := NewAuthHandler(
		authMgr,
		nil, nil, nil, nil,
		ratelimit.NewLimiter(1.0, 5),
		nil,
		"https://test:8443",
		"",
		"test-nexus-id",
		"1.0.0",
		[]string{"catalog"},
	)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/verify", nil)
	w := httptest.NewRecorder()

	handler.HandleVerifyToken(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405, got %d", w.Code)
	}
}
