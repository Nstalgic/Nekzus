package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/nstalgic/nekzus/internal/auth"
	"github.com/nstalgic/nekzus/internal/config"
	"github.com/nstalgic/nekzus/internal/metrics"
	"github.com/nstalgic/nekzus/internal/storage"
	"github.com/nstalgic/nekzus/internal/types"
	wsmanager "github.com/nstalgic/nekzus/internal/websocket"
)

// TestWebSocketRevokedDeviceCannotConnect tests that revoked devices cannot connect via WebSocket
func TestWebSocketRevokedDeviceCannotConnect(t *testing.T) {
	// Create test application
	cfg := types.ServerConfig{}
	config.SetDefaults(&cfg)
	cfg.Auth.HS256Secret = "a1b2c3d4e5f6g7h8i9j0k1l2m3n4o5p6q7r8s9t0u1v2w3x4y5z6"

	store, err := storage.NewStore(storage.Config{DatabasePath: ":memory:"})
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	authManager, err := auth.NewManager([]byte(cfg.Auth.HS256Secret), cfg.Auth.Issuer, cfg.Auth.Audience, cfg.Bootstrap.Tokens)
	if err != nil {
		t.Fatalf("Failed to create auth manager: %v", err)
	}

	m := metrics.New("test_websocket_revoke")
	wsManager := wsmanager.NewManager(m, store)

	app := &Application{
		config:  cfg,
		storage: store,
		services: &ServiceRegistry{
			Auth: authManager,
		},
		managers: &ManagerRegistry{
			WebSocket: wsManager,
		},
		jobs:    &JobRegistry{}, // Empty jobs registry for tests
		metrics: m,
		nexusID: "test-nexus",
		version: "1.0.0",
	}

	// Create a test device
	deviceID := "test-device-123"
	deviceName := "Test Device"
	scopes := []string{"read:catalog"}

	err = store.SaveDevice(deviceID, deviceName, "", "", scopes)
	if err != nil {
		t.Fatalf("Failed to save device: %v", err)
	}

	// Generate JWT for the device
	token, err := authManager.SignJWT(deviceID, []string{"read:catalog"}, 1*time.Hour)
	if err != nil {
		t.Fatalf("Failed to generate JWT: %v", err)
	}

	// Create test server
	server := httptest.NewServer(http.HandlerFunc(app.handleWebSocket))
	defer server.Close()

	// Convert http:// to ws://
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	// Test 1: Device should be able to connect BEFORE revocation
	t.Run("DeviceCanConnectBeforeRevocation", func(t *testing.T) {
		ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
		if err != nil {
			t.Fatalf("Failed to connect to WebSocket: %v", err)
		}
		defer ws.Close()

		// Send auth message
		authMsg := map[string]interface{}{
			"type": "auth",
			"data": map[string]string{
				"token": token,
			},
		}

		err = ws.WriteJSON(authMsg)
		if err != nil {
			t.Fatalf("Failed to send auth message: %v", err)
		}

		// Read auth response
		var response types.WebSocketMessage
		err = ws.ReadJSON(&response)
		if err != nil {
			t.Fatalf("Failed to read auth response: %v", err)
		}

		if response.Type != types.WSMsgTypeAuthSuccess {
			t.Errorf("Expected auth_success, got %s", response.Type)
		}
	})

	// Revoke the device
	err = store.DeleteDevice(deviceID)
	if err != nil {
		t.Fatalf("Failed to revoke device: %v", err)
	}

	// Test 2: Device should NOT be able to connect AFTER revocation
	t.Run("DeviceCannotConnectAfterRevocation", func(t *testing.T) {
		ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
		if err != nil {
			t.Fatalf("Failed to connect to WebSocket: %v", err)
		}
		defer ws.Close()

		// Send auth message with same (valid) JWT
		authMsg := map[string]interface{}{
			"type": "auth",
			"data": map[string]string{
				"token": token,
			},
		}

		err = ws.WriteJSON(authMsg)
		if err != nil {
			t.Fatalf("Failed to send auth message: %v", err)
		}

		// Read auth response
		var response types.WebSocketMessage
		err = ws.ReadJSON(&response)
		if err != nil {
			t.Fatalf("Failed to read auth response: %v", err)
		}

		// Should receive auth_failed
		if response.Type != types.WSMsgTypeAuthFailed {
			t.Errorf("Expected auth_failed for revoked device, got %s", response.Type)
		}

		// Check error message
		if dataMap, ok := response.Data.(map[string]interface{}); ok {
			if errMsg, ok := dataMap["error"].(string); ok {
				if errMsg != "device access revoked" {
					t.Errorf("Expected 'device access revoked' error, got '%s'", errMsg)
				}
			} else {
				t.Error("Auth failed message missing error field")
			}
		} else {
			t.Error("Auth failed message has invalid data format")
		}
	})
}

// TestWebSocketRevokeWhileConnected tests that connected devices are disconnected when revoked
func TestWebSocketRevokeWhileConnected(t *testing.T) {
	// Create test application
	cfg := types.ServerConfig{}
	config.SetDefaults(&cfg)
	cfg.Auth.HS256Secret = "a1b2c3d4e5f6g7h8i9j0k1l2m3n4o5p6q7r8s9t0u1v2w3x4y5z6"

	store, err := storage.NewStore(storage.Config{DatabasePath: ":memory:"})
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	authManager, err := auth.NewManager([]byte(cfg.Auth.HS256Secret), cfg.Auth.Issuer, cfg.Auth.Audience, cfg.Bootstrap.Tokens)
	if err != nil {
		t.Fatalf("Failed to create auth manager: %v", err)
	}

	m := metrics.New("test_websocket_revoke_connected")
	wsManager := wsmanager.NewManager(m, store)

	app := &Application{
		config:  cfg,
		storage: store,
		services: &ServiceRegistry{
			Auth: authManager,
		},
		managers: &ManagerRegistry{
			WebSocket: wsManager,
		},
		jobs:    &JobRegistry{}, // Empty jobs registry for tests
		metrics: m,
		nexusID: "test-nexus",
		version: "1.0.0",
	}

	// Create a test device
	deviceID := "test-device-456"
	deviceName := "Test Device"
	scopes := []string{"read:catalog"}

	err = store.SaveDevice(deviceID, deviceName, "", "", scopes)
	if err != nil {
		t.Fatalf("Failed to save device: %v", err)
	}

	// Generate JWT for the device
	token, err := authManager.SignJWT(deviceID, []string{"read:catalog"}, 1*time.Hour)
	if err != nil {
		t.Fatalf("Failed to generate JWT: %v", err)
	}

	// Create test server
	server := httptest.NewServer(http.HandlerFunc(app.handleWebSocket))
	defer server.Close()

	// Convert http:// to ws://
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	// Connect to WebSocket
	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect to WebSocket: %v", err)
	}
	defer ws.Close()

	// Send auth message
	authMsg := map[string]interface{}{
		"type": "auth",
		"data": map[string]string{
			"token": token,
		},
	}

	err = ws.WriteJSON(authMsg)
	if err != nil {
		t.Fatalf("Failed to send auth message: %v", err)
	}

	// Read auth response
	var authResponse types.WebSocketMessage
	err = ws.ReadJSON(&authResponse)
	if err != nil {
		t.Fatalf("Failed to read auth response: %v", err)
	}

	if authResponse.Type != types.WSMsgTypeAuthSuccess {
		t.Fatalf("Expected auth_success, got %s", authResponse.Type)
	}

	// Read hello message
	var helloResponse types.WebSocketMessage
	err = ws.ReadJSON(&helloResponse)
	if err != nil {
		t.Fatalf("Failed to read hello message: %v", err)
	}

	if helloResponse.Type != types.WSMsgTypeHello {
		t.Fatalf("Expected hello, got %s", helloResponse.Type)
	}

	// Verify device is connected
	if wsManager.ActiveConnections() != 1 {
		t.Errorf("Expected 1 active connection, got %d", wsManager.ActiveConnections())
	}

	// Revoke the device (disconnect should happen)
	disconnected := wsManager.DisconnectDevice(deviceID)
	if disconnected != 1 {
		t.Errorf("Expected 1 connection disconnected, got %d", disconnected)
	}

	// Verify connection was closed
	if wsManager.ActiveConnections() != 0 {
		t.Errorf("Expected 0 active connections after revocation, got %d", wsManager.ActiveConnections())
	}

	// Try to read from WebSocket - should fail with connection closed error
	var msg types.WebSocketMessage
	err = ws.ReadJSON(&msg)
	if err == nil {
		t.Error("Expected error reading from closed connection, got nil")
	}
}

// TestWebSocketAuthWithoutStorage tests WebSocket auth when storage is not available
func TestWebSocketAuthWithoutStorage(t *testing.T) {
	// Create test application WITHOUT storage
	cfg := types.ServerConfig{}
	config.SetDefaults(&cfg)
	cfg.Auth.HS256Secret = "a1b2c3d4e5f6g7h8i9j0k1l2m3n4o5p6q7r8s9t0u1v2w3x4y5z6"

	authManager, err := auth.NewManager([]byte(cfg.Auth.HS256Secret), cfg.Auth.Issuer, cfg.Auth.Audience, cfg.Bootstrap.Tokens)
	if err != nil {
		t.Fatalf("Failed to create auth manager: %v", err)
	}

	m := metrics.New("test_websocket_no_storage")
	wsManager := wsmanager.NewManager(m, nil)

	app := &Application{
		config:  cfg,
		storage: nil, // No storage
		services: &ServiceRegistry{
			Auth: authManager,
		},
		managers: &ManagerRegistry{
			WebSocket: wsManager,
		},
		jobs:    &JobRegistry{}, // Empty jobs registry for tests
		metrics: m,
		nexusID: "test-nexus",
		version: "1.0.0",
	}

	// Generate JWT
	deviceID := "test-device-789"
	token, err := authManager.SignJWT(deviceID, []string{"read:catalog"}, 1*time.Hour)
	if err != nil {
		t.Fatalf("Failed to generate JWT: %v", err)
	}

	// Create test server
	server := httptest.NewServer(http.HandlerFunc(app.handleWebSocket))
	defer server.Close()

	// Convert http:// to ws://
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	// Connect to WebSocket
	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect to WebSocket: %v", err)
	}
	defer ws.Close()

	// Send auth message
	authMsg := map[string]interface{}{
		"type": "auth",
		"data": map[string]string{
			"token": token,
		},
	}

	err = ws.WriteJSON(authMsg)
	if err != nil {
		t.Fatalf("Failed to send auth message: %v", err)
	}

	// Read auth response - should succeed even without storage
	var response types.WebSocketMessage
	err = ws.ReadJSON(&response)
	if err != nil {
		t.Fatalf("Failed to read auth response: %v", err)
	}

	if response.Type != types.WSMsgTypeAuthSuccess {
		t.Errorf("Expected auth_success when storage is unavailable, got %s", response.Type)
	}
}
