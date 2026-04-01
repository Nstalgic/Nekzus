package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/nstalgic/nekzus/internal/types"
	wsmanager "github.com/nstalgic/nekzus/internal/websocket"
)

// Helper function to authenticate a WebSocket connection
func authenticateWebSocket(t *testing.T, conn *websocket.Conn, token string) {
	t.Helper()

	// Send auth message
	authMsg := types.WebSocketMessage{
		Type: types.WSMsgTypeAuth,
		Data: map[string]string{
			"token": token,
		},
	}

	err := conn.WriteJSON(authMsg)
	if err != nil {
		t.Fatalf("Failed to send auth message: %v", err)
	}

	// Read auth_success response
	var authResponse types.WebSocketMessage
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	err = conn.ReadJSON(&authResponse)
	if err != nil {
		t.Fatalf("Failed to read auth response: %v", err)
	}

	if authResponse.Type != types.WSMsgTypeAuthSuccess {
		t.Fatalf("Expected auth_success, got %s", authResponse.Type)
	}

	// Read hello message
	var helloMsg types.WebSocketMessage
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	err = conn.ReadJSON(&helloMsg)
	if err != nil {
		t.Fatalf("Failed to read hello message: %v", err)
	}

	if helloMsg.Type != types.WSMsgTypeHello {
		t.Fatalf("Expected hello message, got %s", helloMsg.Type)
	}
}

// readAppMessage reads the next non-device_status message from the WebSocket connection.
// device_status messages are automatically broadcast on connect/disconnect and are skipped.
func readAppMessage(t *testing.T, conn *websocket.Conn, timeout time.Duration) (types.WebSocketMessage, error) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		var msg types.WebSocketMessage
		conn.SetReadDeadline(deadline)
		if err := conn.ReadJSON(&msg); err != nil {
			return msg, err
		}
		if msg.Type == types.WSMsgTypeDeviceStatus {
			continue
		}
		return msg, nil
	}
}

func TestHandleWebSocket_LocalRequestNoAuth(t *testing.T) {
	app := newTestApplication(t)
	app.managers.WebSocket = wsmanager.NewManager(app.metrics, app.storage)

	// Generate valid JWT for authentication
	token, err := app.services.Auth.SignJWT("device-local-test", []string{"read:events"}, 3600*time.Second)
	if err != nil {
		t.Fatalf("Failed to generate JWT: %v", err)
	}

	// Create test server WITHOUT IP auth middleware
	mux := http.NewServeMux()
	mux.Handle("/api/v1/ws", http.HandlerFunc(app.handleWebSocket))

	server := httptest.NewServer(mux)
	defer server.Close()

	// Convert http:// to ws://
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/v1/ws"

	// Connect as local client
	dialer := websocket.Dialer{
		HandshakeTimeout: 5 * time.Second,
	}

	conn, resp, err := dialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect: %v (status: %v)", err, resp)
	}
	defer conn.Close()

	// Authenticate using helper function
	authenticateWebSocket(t, conn, token)
}

func TestHandleWebSocket_LocalRequestWithoutTokenAllowed(t *testing.T) {
	app := newTestApplication(t)
	app.managers.WebSocket = wsmanager.NewManager(app.metrics, app.storage)

	// Create test server WITHOUT auth middleware (auth is handled post-connection)
	mux := http.NewServeMux()
	mux.Handle("/api/v1/ws", http.HandlerFunc(app.handleWebSocket))

	server := httptest.NewServer(mux)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/v1/ws"

	// Try to connect without providing token (IP-based auth for localhost)
	dialer := websocket.Dialer{
		HandshakeTimeout: 5 * time.Second,
	}

	conn, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Connection should succeed, auth happens post-connection: %v", err)
	}
	defer conn.Close()

	// Send auth message without token (local IP-based auth)
	authMsg := types.WebSocketMessage{
		Type: types.WSMsgTypeAuth,
		Data: map[string]string{},
	}
	conn.WriteJSON(authMsg)

	// Should receive auth_success for local requests without token
	var response types.WebSocketMessage
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	err = conn.ReadJSON(&response)
	if err != nil {
		t.Fatalf("Failed to read response: %v", err)
	}

	if response.Type != types.WSMsgTypeAuthSuccess {
		t.Errorf("Expected auth_success for local request without token, got %s", response.Type)
	}

	// Verify device ID is "admin" for IP-based auth
	if dataMap, ok := response.Data.(map[string]interface{}); ok {
		if deviceID, ok := dataMap["deviceId"].(string); ok {
			if deviceID != "admin" {
				t.Errorf("Expected deviceId 'admin' for IP-based auth, got %s", deviceID)
			}
		}
	}
}

func TestHandleWebSocket_ExternalRequestWithValidJWT(t *testing.T) {
	app := newTestApplication(t)
	app.managers.WebSocket = wsmanager.NewManager(app.metrics, app.storage)

	// Generate valid JWT
	token, err := app.services.Auth.SignJWT("device-test-123", []string{"read:events"}, 3600*time.Second)
	if err != nil {
		t.Fatalf("Failed to generate JWT: %v", err)
	}

	// Create test server
	mux := http.NewServeMux()
	mux.Handle("/api/v1/ws", http.HandlerFunc(app.handleWebSocket))

	server := httptest.NewServer(mux)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/v1/ws"

	// Connect (no auth header needed, auth happens post-connection)
	dialer := websocket.Dialer{
		HandshakeTimeout: 5 * time.Second,
	}

	conn, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	// Authenticate using helper function
	authenticateWebSocket(t, conn, token)
}

func TestHandleWebSocket_ReceiveBroadcastMessages(t *testing.T) {
	app := newTestApplication(t)
	app.managers.WebSocket = wsmanager.NewManager(app.metrics, app.storage)

	// Generate valid JWT
	token, err := app.services.Auth.SignJWT("device-broadcast-test", []string{"read:events"}, 3600*time.Second)
	if err != nil {
		t.Fatalf("Failed to generate JWT: %v", err)
	}

	// Create test server
	mux := http.NewServeMux()
	mux.Handle("/api/v1/ws", http.HandlerFunc(app.handleWebSocket))

	server := httptest.NewServer(mux)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/v1/ws"

	// Connect client
	dialer := websocket.Dialer{HandshakeTimeout: 5 * time.Second}
	conn, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	// Authenticate
	authenticateWebSocket(t, conn, token)

	// Broadcast a test message
	testMessage := types.WebSocketMessage{
		Type:      types.WSMsgTypeConfigReload,
		Data:      map[string]string{"status": "reloaded"},
		Timestamp: time.Now(),
	}

	app.managers.WebSocket.Broadcast(testMessage)

	// Client should receive the broadcast (skip device_status from connect)
	receivedMsg, err := readAppMessage(t, conn, 2*time.Second)
	if err != nil {
		t.Fatalf("Failed to receive broadcast message: %v", err)
	}

	if receivedMsg.Type != types.WSMsgTypeConfigReload {
		t.Errorf("Expected config_reload message, got %s", receivedMsg.Type)
	}
}

func TestHandleWebSocket_PingPongHeartbeat(t *testing.T) {
	t.Parallel()

	app := newTestApplication(t)
	app.managers.WebSocket = wsmanager.NewManager(app.metrics, app.storage)

	// Override ping interval for faster testing
	app.managers.WebSocket.SetPingInterval(100 * time.Millisecond)
	app.managers.WebSocket.SetPongWait(200 * time.Millisecond)

	// Generate valid JWT
	token, err := app.services.Auth.SignJWT("device-ping-test", []string{"read:events"}, 3600*time.Second)
	if err != nil {
		t.Fatalf("Failed to generate JWT: %v", err)
	}

	// Create test server
	mux := http.NewServeMux()
	mux.Handle("/api/v1/ws", http.HandlerFunc(app.handleWebSocket))

	server := httptest.NewServer(mux)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/v1/ws"

	// Connect client
	dialer := websocket.Dialer{HandshakeTimeout: 5 * time.Second}
	conn, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	// Authenticate
	authenticateWebSocket(t, conn, token)

	// Set up ping handler (server sends pings, client responds with pongs automatically)
	pingReceived := make(chan bool, 1)
	conn.SetPingHandler(func(string) error {
		pingReceived <- true
		// Return nil to let gorilla/websocket send the pong response automatically
		return nil
	})

	// Start reading messages (required for ping handler to be called)
	go func() {
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				return
			}
		}
	}()

	// Wait for ping from server (should arrive within ~100ms)
	select {
	case <-pingReceived:
		// Success - ping received from server
	case <-time.After(1 * time.Second):
		t.Error("Did not receive ping within expected time")
	}
}

func TestHandleWebSocket_MultipleClients(t *testing.T) {
	app := newTestApplication(t)
	app.managers.WebSocket = wsmanager.NewManager(app.metrics, app.storage)

	// Create test server
	mux := http.NewServeMux()
	mux.Handle("/api/v1/ws", http.HandlerFunc(app.handleWebSocket))

	server := httptest.NewServer(mux)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/v1/ws"

	// Connect multiple clients
	numClients := 3
	clients := make([]*websocket.Conn, numClients)

	for i := 0; i < numClients; i++ {
		// Generate unique JWT for each client
		token, err := app.services.Auth.SignJWT(fmt.Sprintf("device-multi-%d", i), []string{"read:events"}, 3600*time.Second)
		if err != nil {
			t.Fatalf("Failed to generate JWT for client %d: %v", i, err)
		}

		dialer := websocket.Dialer{HandshakeTimeout: 5 * time.Second}
		conn, _, err := dialer.Dial(wsURL, nil)
		if err != nil {
			t.Fatalf("Failed to connect client %d: %v", i, err)
		}
		defer conn.Close()
		clients[i] = conn

		// Authenticate
		authenticateWebSocket(t, conn, token)
	}

	// Verify active connections
	if count := app.managers.WebSocket.ActiveConnections(); count != numClients {
		t.Errorf("Expected %d active connections, got %d", numClients, count)
	}

	// Broadcast a message
	testMessage := types.WebSocketMessage{
		Type: types.WSMsgTypeDevicePaired,
		Data: map[string]string{"deviceId": "new-device"},
	}
	app.managers.WebSocket.Broadcast(testMessage)

	// All clients should receive it (skip device_status from connect)
	for i, conn := range clients {
		msg, err := readAppMessage(t, conn, 2*time.Second)
		if err != nil {
			t.Errorf("Client %d failed to receive broadcast: %v", i, err)
			continue
		}

		if msg.Type != types.WSMsgTypeDevicePaired {
			t.Errorf("Client %d received wrong message type: %s", i, msg.Type)
		}
	}
}

func TestHandleWebSocket_ClientDisconnection(t *testing.T) {
	app := newTestApplication(t)
	app.managers.WebSocket = wsmanager.NewManager(app.metrics, app.storage)

	// Generate valid JWT
	token, err := app.services.Auth.SignJWT("device-disconnect-test", []string{"read:events"}, 3600*time.Second)
	if err != nil {
		t.Fatalf("Failed to generate JWT: %v", err)
	}

	// Create test server
	mux := http.NewServeMux()
	mux.Handle("/api/v1/ws", http.HandlerFunc(app.handleWebSocket))

	server := httptest.NewServer(mux)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/v1/ws"

	// Connect client
	dialer := websocket.Dialer{HandshakeTimeout: 5 * time.Second}
	conn, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}

	// Authenticate
	authenticateWebSocket(t, conn, token)

	// Verify client is connected
	if count := app.managers.WebSocket.ActiveConnections(); count != 1 {
		t.Errorf("Expected 1 active connection, got %d", count)
	}

	// Close connection
	conn.Close()

	// Give server time to detect disconnection
	time.Sleep(500 * time.Millisecond)

	// Verify client was removed
	if count := app.managers.WebSocket.ActiveConnections(); count != 0 {
		t.Errorf("Expected 0 active connections after disconnect, got %d", count)
	}
}

func TestHandleWebSocket_MessageSerialization(t *testing.T) {
	app := newTestApplication(t)
	app.managers.WebSocket = wsmanager.NewManager(app.metrics, app.storage)

	// Generate valid JWT
	token, err := app.services.Auth.SignJWT("device-serial-test", []string{"read:events"}, 3600*time.Second)
	if err != nil {
		t.Fatalf("Failed to generate JWT: %v", err)
	}

	// Create test server
	mux := http.NewServeMux()
	mux.Handle("/api/v1/ws", http.HandlerFunc(app.handleWebSocket))

	server := httptest.NewServer(mux)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/v1/ws"

	// Connect client
	dialer := websocket.Dialer{HandshakeTimeout: 5 * time.Second}
	conn, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	// Authenticate
	authenticateWebSocket(t, conn, token)

	// Test broadcasting various data types
	tests := []struct {
		name    string
		message types.WebSocketMessage
	}{
		{
			name: "string data",
			message: types.WebSocketMessage{
				Type: types.WSMsgTypeConfigReload,
				Data: "test string",
			},
		},
		{
			name: "complex object",
			message: types.WebSocketMessage{
				Type: types.WSMsgTypeDiscovery,
				Data: map[string]interface{}{
					"service": "grafana",
					"port":    3000,
					"tags":    []string{"monitoring", "dashboard"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Broadcast message
			app.managers.WebSocket.Broadcast(tt.message)

			// Receive and verify (skip device_status from connect)
			received, err := readAppMessage(t, conn, 2*time.Second)
			if err != nil {
				t.Fatalf("Failed to receive message: %v", err)
			}

			if received.Type != tt.message.Type {
				t.Errorf("Message type mismatch: got %s, want %s", received.Type, tt.message.Type)
			}

			// Verify data can be marshaled back
			dataBytes, err := json.Marshal(received.Data)
			if err != nil {
				t.Errorf("Failed to marshal received data: %v", err)
			}

			if len(dataBytes) == 0 {
				t.Error("Received empty data")
			}
		})
	}
}

// TestHandleWebSocket_PostConnectionAuth tests the new authentication flow
// where connections are accepted first, then auth message is expected
func TestHandleWebSocket_PostConnectionAuth_ValidToken(t *testing.T) {
	app := newTestApplication(t)
	app.managers.WebSocket = wsmanager.NewManager(app.metrics, app.storage)

	// Generate valid JWT
	token, err := app.services.Auth.SignJWT("device-test-auth", []string{"read:events"}, 3600*time.Second)
	if err != nil {
		t.Fatalf("Failed to generate JWT: %v", err)
	}

	// Create test server WITHOUT IP auth middleware
	mux := http.NewServeMux()
	mux.Handle("/api/v1/ws", http.HandlerFunc(app.handleWebSocket))

	server := httptest.NewServer(mux)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/v1/ws"

	// Connect without auth header (should be accepted)
	dialer := websocket.Dialer{HandshakeTimeout: 5 * time.Second}
	conn, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	// Send auth message with valid token
	authMsg := types.WebSocketMessage{
		Type: types.WSMsgTypeAuth,
		Data: map[string]string{
			"token": token,
		},
	}

	err = conn.WriteJSON(authMsg)
	if err != nil {
		t.Fatalf("Failed to send auth message: %v", err)
	}

	// Should receive auth_success response
	var response types.WebSocketMessage
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	err = conn.ReadJSON(&response)
	if err != nil {
		t.Fatalf("Failed to read auth response: %v", err)
	}

	if response.Type != types.WSMsgTypeAuthSuccess {
		t.Errorf("Expected auth_success message, got %s", response.Type)
	}

	// Should now receive hello message
	var helloMsg types.WebSocketMessage
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	err = conn.ReadJSON(&helloMsg)
	if err != nil {
		t.Fatalf("Failed to read hello message: %v", err)
	}

	if helloMsg.Type != types.WSMsgTypeHello {
		t.Errorf("Expected hello message after auth, got %s", helloMsg.Type)
	}
}

func TestHandleWebSocket_PostConnectionAuth_InvalidToken(t *testing.T) {
	app := newTestApplication(t)
	app.managers.WebSocket = wsmanager.NewManager(app.metrics, app.storage)

	// Create test server WITHOUT IP auth middleware
	mux := http.NewServeMux()
	mux.Handle("/api/v1/ws", http.HandlerFunc(app.handleWebSocket))

	server := httptest.NewServer(mux)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/v1/ws"

	// Connect without auth header
	dialer := websocket.Dialer{HandshakeTimeout: 5 * time.Second}
	conn, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	// Send auth message with invalid token
	authMsg := types.WebSocketMessage{
		Type: types.WSMsgTypeAuth,
		Data: map[string]string{
			"token": "invalid-token-12345",
		},
	}

	err = conn.WriteJSON(authMsg)
	if err != nil {
		t.Fatalf("Failed to send auth message: %v", err)
	}

	// Should receive auth_failed response
	var response types.WebSocketMessage
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	err = conn.ReadJSON(&response)
	if err != nil {
		t.Fatalf("Failed to read auth response: %v", err)
	}

	if response.Type != types.WSMsgTypeAuthFailed {
		t.Errorf("Expected auth_failed message, got %s", response.Type)
	}

	// Connection should close after failed auth
	_, _, err = conn.ReadMessage()
	if err == nil {
		t.Error("Expected connection to close after failed auth")
	}
}

func TestHandleWebSocket_PostConnectionAuth_NoAuthTimeout(t *testing.T) {
	t.Parallel()

	app := newTestApplication(t)
	app.managers.WebSocket = wsmanager.NewManager(app.metrics, app.storage)

	// Override auth timeout for faster testing
	app.managers.WebSocket.SetAuthTimeout(200 * time.Millisecond)

	// Create test server WITHOUT IP auth middleware
	mux := http.NewServeMux()
	mux.Handle("/api/v1/ws", http.HandlerFunc(app.handleWebSocket))

	server := httptest.NewServer(mux)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/v1/ws"

	// Connect without auth header
	dialer := websocket.Dialer{HandshakeTimeout: 5 * time.Second}
	conn, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	// Don't send auth message, just wait
	// Server should send auth_failed message and close connection after timeout (~200ms)
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))

	// Should receive auth_failed message
	var response types.WebSocketMessage
	err = conn.ReadJSON(&response)
	if err != nil {
		t.Fatalf("Failed to read timeout message: %v", err)
	}

	if response.Type != types.WSMsgTypeAuthFailed {
		t.Errorf("Expected auth_failed message on timeout, got %s", response.Type)
	}

	// Connection should close after auth_failed
	_, _, err = conn.ReadMessage()
	if err == nil {
		t.Error("Expected connection to close after timeout")
	}
}

func TestHandleWebSocket_PostConnectionAuth_BroadcastOnlyAfterAuth(t *testing.T) {
	app := newTestApplication(t)
	app.managers.WebSocket = wsmanager.NewManager(app.metrics, app.storage)

	// Generate valid JWT
	token, err := app.services.Auth.SignJWT("device-broadcast-test", []string{"read:events"}, 3600*time.Second)
	if err != nil {
		t.Fatalf("Failed to generate JWT: %v", err)
	}

	// Create test server WITHOUT IP auth middleware
	mux := http.NewServeMux()
	mux.Handle("/api/v1/ws", http.HandlerFunc(app.handleWebSocket))

	server := httptest.NewServer(mux)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/v1/ws"

	// Connect without auth
	dialer := websocket.Dialer{HandshakeTimeout: 5 * time.Second}
	conn, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	// Broadcast a message BEFORE authentication
	testMessage := types.WebSocketMessage{
		Type: types.WSMsgTypeConfigReload,
		Data: map[string]string{"status": "test-before-auth"},
	}
	app.managers.WebSocket.Broadcast(testMessage)

	// Wait a bit to ensure message would have been delivered if client was subscribed
	time.Sleep(100 * time.Millisecond)

	// Now authenticate
	authMsg := types.WebSocketMessage{
		Type: types.WSMsgTypeAuth,
		Data: map[string]string{
			"token": token,
		},
	}
	err = conn.WriteJSON(authMsg)
	if err != nil {
		t.Fatalf("Failed to send auth message: %v", err)
	}

	// Read auth_success
	var authResponse types.WebSocketMessage
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	conn.ReadJSON(&authResponse)

	// Read hello message
	var helloMsg types.WebSocketMessage
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	conn.ReadJSON(&helloMsg)

	// Now broadcast another message AFTER authentication
	testMessage2 := types.WebSocketMessage{
		Type: types.WSMsgTypeDevicePaired,
		Data: map[string]string{"deviceId": "test-after-auth"},
	}
	app.managers.WebSocket.Broadcast(testMessage2)

	// Should receive the message sent AFTER auth (skip device_status from connect)
	received, err := readAppMessage(t, conn, 2*time.Second)
	if err != nil {
		t.Fatalf("Failed to receive broadcast after auth: %v", err)
	}

	if received.Type != types.WSMsgTypeDevicePaired {
		t.Errorf("Expected device_paired message, got %s", received.Type)
	}
}

func TestHandleWebSocket_HealthChangeNotification(t *testing.T) {
	app := newTestApplication(t)
	app.managers.WebSocket = wsmanager.NewManager(app.metrics, app.storage)

	// Generate valid JWT
	token, err := app.services.Auth.SignJWT("device-health-test", []string{"read:events"}, 3600*time.Second)
	if err != nil {
		t.Fatalf("Failed to generate JWT: %v", err)
	}

	// Create test server
	mux := http.NewServeMux()
	mux.Handle("/api/v1/ws", http.HandlerFunc(app.handleWebSocket))

	server := httptest.NewServer(mux)
	defer server.Close()

	// Convert http:// to ws://
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/v1/ws"

	// Connect client
	dialer := websocket.Dialer{HandshakeTimeout: 5 * time.Second}
	conn, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	// Authenticate
	authenticateWebSocket(t, conn, token)

	// Publish a health change event
	app.managers.WebSocket.PublishHealthChange("test-app", "Test App", "/apps/test-app/", "unhealthy", "Connection timeout")

	// Client should receive the health change message (skip device_status from connect)
	msg, err := readAppMessage(t, conn, 2*time.Second)
	if err != nil {
		t.Fatalf("Failed to receive health change message: %v", err)
	}

	// Verify message type
	if msg.Type != types.WSMsgTypeHealthChange {
		t.Errorf("Expected message type %s, got %s", types.WSMsgTypeHealthChange, msg.Type)
	}

	// Verify message data
	data, ok := msg.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("Expected data to be map[string]interface{}, got %T", msg.Data)
	}

	if data["appId"] != "test-app" {
		t.Errorf("Expected appId 'test-app', got %v", data["appId"])
	}

	if data["status"] != "unhealthy" {
		t.Errorf("Expected status 'unhealthy', got %v", data["status"])
	}

	if data["message"] != "Connection timeout" {
		t.Errorf("Expected message 'Connection timeout', got %v", data["message"])
	}

	// Verify timestamp exists
	if _, ok := data["timestamp"]; !ok {
		t.Error("Expected timestamp field in health change message")
	}
}
