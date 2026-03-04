package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/nstalgic/nekzus/internal/activity"
	"github.com/nstalgic/nekzus/internal/notifications"
	"github.com/nstalgic/nekzus/internal/storage"
	"github.com/nstalgic/nekzus/internal/types"
	wsmanager "github.com/nstalgic/nekzus/internal/websocket"
)

// types import is used for types.WSMsgTypeWebhook and types.ActivityEvent

// Helper function to connect and authenticate a WebSocket client
func connectAndAuthenticateClient(t *testing.T, wsURL, deviceID string, app *Application) *websocket.Conn {
	t.Helper()

	// Register device in storage if storage is available
	// This is required because WebSocket auth checks device existence in storage
	if app.storage != nil {
		if err := app.storage.SaveDevice(deviceID, deviceID, "test", "1.0", []string{"read:events"}); err != nil {
			t.Fatalf("Failed to register device %s in storage: %v", deviceID, err)
		}
	}

	// Generate JWT for this device
	token, err := app.services.Auth.SignJWT(deviceID, []string{"read:events"}, 3600*time.Second)
	if err != nil {
		t.Fatalf("Failed to generate JWT for %s: %v", deviceID, err)
	}

	// Connect to WebSocket
	dialer := websocket.Dialer{HandshakeTimeout: 5 * time.Second}
	conn, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect %s: %v", deviceID, err)
	}

	// Authenticate
	authenticateWebSocket(t, conn, token)

	return conn
}

// Helper function to send activity webhook
func sendWebhookActivity(t *testing.T, serverURL string, payload WebhookActivityPayload) *http.Response {
	t.Helper()

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Failed to marshal payload: %v", err)
	}

	resp, err := http.Post(serverURL+"/api/v1/webhooks/activity", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Failed to send webhook: %v", err)
	}

	return resp
}

// Helper function to send notify webhook
func sendWebhookNotify(t *testing.T, serverURL string, payload WebhookNotifyPayload) *http.Response {
	t.Helper()

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Failed to marshal payload: %v", err)
	}

	resp, err := http.Post(serverURL+"/api/v1/webhooks/notify", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Failed to send webhook: %v", err)
	}

	return resp
}

// Helper function to expect a WebSocket message
func expectWebSocketMessage(t *testing.T, conn *websocket.Conn, expectedType string, timeout time.Duration) types.WebSocketMessage {
	t.Helper()

	var msg types.WebSocketMessage
	conn.SetReadDeadline(time.Now().Add(timeout))
	err := conn.ReadJSON(&msg)
	if err != nil {
		t.Fatalf("Failed to read WebSocket message: %v", err)
	}

	if msg.Type != expectedType {
		t.Errorf("Expected message type %s, got %s", expectedType, msg.Type)
	}

	return msg
}

// Helper function to expect NO WebSocket message (for filtering tests)
func expectNoWebSocketMessage(t *testing.T, conn *websocket.Conn, timeout time.Duration) {
	t.Helper()

	var msg types.WebSocketMessage
	conn.SetReadDeadline(time.Now().Add(timeout))
	err := conn.ReadJSON(&msg)

	// Clear the deadline for future reads
	conn.SetReadDeadline(time.Time{})

	if err == nil {
		t.Errorf("Expected no message, but received: %+v", msg)
	}
	// Timeout error is expected, so we don't fail on it
}

// TestWebhookActivityNotification_Broadcast tests that activity webhooks
// without deviceIds broadcast to all connected clients
func TestWebhookActivityNotification_Broadcast(t *testing.T) {
	// Setup test application with storage
	tmpFile, err := os.CreateTemp("", "nekzus-webhook-broadcast-*.db")
	if err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()
	dbPath := tmpFile.Name()
	defer os.Remove(dbPath)

	store, err := storage.NewStore(storage.Config{
		DatabasePath: dbPath,
	})
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	// Create auth manager for testing
	testApp := newTestApplication(t)

	app := &Application{
		storage: store,
		metrics: testMetrics,
		services: &ServiceRegistry{
			Auth: testApp.services.Auth,
		},
		managers: &ManagerRegistry{
			WebSocket: wsmanager.NewManager(testMetrics, store),
			Activity:  activity.NewTracker(store),
		},
		jobs: &JobRegistry{}, // Empty jobs registry for tests
	}

	// Create test server
	mux := http.NewServeMux()
	mux.Handle("/api/v1/ws", http.HandlerFunc(app.handleWebSocket))
	mux.Handle("/api/v1/webhooks/activity", http.HandlerFunc(app.handleWebhookActivity))

	server := httptest.NewServer(mux)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/v1/ws"

	// Connect 3 WebSocket clients
	client1 := connectAndAuthenticateClient(t, wsURL, "device-broadcast-1", app)
	defer client1.Close()

	client2 := connectAndAuthenticateClient(t, wsURL, "device-broadcast-2", app)
	defer client2.Close()

	client3 := connectAndAuthenticateClient(t, wsURL, "device-broadcast-3", app)
	defer client3.Close()

	// Verify all clients are connected
	if count := app.managers.WebSocket.ActiveConnections(); count != 3 {
		t.Fatalf("Expected 3 active connections, got %d", count)
	}

	// Send activity webhook with NO deviceIds (broadcast to all)
	payload := WebhookActivityPayload{
		Message:   "Broadcast test notification",
		Icon:      "Radio",
		IconClass: "success",
		Details:   "This should reach all clients",
	}

	resp := sendWebhookActivity(t, server.URL, payload)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	// All 3 clients should receive the webhook message
	msg1 := expectWebSocketMessage(t, client1, types.WSMsgTypeWebhook, 2*time.Second)
	msg2 := expectWebSocketMessage(t, client2, types.WSMsgTypeWebhook, 2*time.Second)
	msg3 := expectWebSocketMessage(t, client3, types.WSMsgTypeWebhook, 2*time.Second)

	// Verify message data contains ActivityEvent
	for i, msg := range []types.WebSocketMessage{msg1, msg2, msg3} {
		// Convert Data to ActivityEvent
		dataBytes, _ := json.Marshal(msg.Data)
		var event types.ActivityEvent
		if err := json.Unmarshal(dataBytes, &event); err != nil {
			t.Errorf("Client %d: Failed to unmarshal ActivityEvent: %v", i+1, err)
			continue
		}

		if event.Message != payload.Message {
			t.Errorf("Client %d: Expected message '%s', got '%s'", i+1, payload.Message, event.Message)
		}
		if event.Icon != payload.Icon {
			t.Errorf("Client %d: Expected icon '%s', got '%s'", i+1, payload.Icon, event.Icon)
		}
		if event.IconClass != payload.IconClass {
			t.Errorf("Client %d: Expected iconClass '%s', got '%s'", i+1, payload.IconClass, event.IconClass)
		}
		if event.Type != "webhook.activity" {
			t.Errorf("Client %d: Expected type 'webhook.activity', got '%s'", i+1, event.Type)
		}
	}
}

// TestWebhookActivityNotification_TargetedDelivery tests that activity webhooks
// with deviceIds only reach the specified devices
func TestWebhookActivityNotification_TargetedDelivery(t *testing.T) {
	// Setup test application
	tmpFile, err := os.CreateTemp("", "nekzus-webhook-targeted-*.db")
	if err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()
	dbPath := tmpFile.Name()
	defer os.Remove(dbPath)

	store, err := storage.NewStore(storage.Config{
		DatabasePath: dbPath,
	})
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	// Create auth manager for testing
	testApp := newTestApplication(t)

	app := &Application{
		storage: store,
		metrics: testMetrics,
		services: &ServiceRegistry{
			Auth: testApp.services.Auth,
		},
		managers: &ManagerRegistry{
			WebSocket: wsmanager.NewManager(testMetrics, store),
			Activity:  activity.NewTracker(store),
		},
		jobs: &JobRegistry{}, // Empty jobs registry for tests
	}

	// Create test server
	mux := http.NewServeMux()
	mux.Handle("/api/v1/ws", http.HandlerFunc(app.handleWebSocket))
	mux.Handle("/api/v1/webhooks/activity", http.HandlerFunc(app.handleWebhookActivity))

	server := httptest.NewServer(mux)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/v1/ws"

	// Connect 3 WebSocket clients with specific device IDs
	client1 := connectAndAuthenticateClient(t, wsURL, "device-target-1", app)
	defer client1.Close()

	client2 := connectAndAuthenticateClient(t, wsURL, "device-target-2", app)
	defer client2.Close()

	client3 := connectAndAuthenticateClient(t, wsURL, "device-target-3", app)
	defer client3.Close()

	// Send activity webhook targeting only device-target-1 and device-target-3
	payload := WebhookActivityPayload{
		Message:   "Targeted notification",
		Icon:      "Target",
		IconClass: "warning",
		Details:   "Only for devices 1 and 3",
		DeviceIDs: []string{"device-target-1", "device-target-3"},
	}

	resp := sendWebhookActivity(t, server.URL, payload)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	// Client 1 and 3 should receive the message
	msg1 := expectWebSocketMessage(t, client1, types.WSMsgTypeWebhook, 2*time.Second)
	msg3 := expectWebSocketMessage(t, client3, types.WSMsgTypeWebhook, 2*time.Second)

	// Verify message content
	for i, msg := range []types.WebSocketMessage{msg1, msg3} {
		dataBytes, _ := json.Marshal(msg.Data)
		var event types.ActivityEvent
		if err := json.Unmarshal(dataBytes, &event); err != nil {
			t.Errorf("Targeted client %d: Failed to unmarshal: %v", i, err)
			continue
		}

		if event.Message != payload.Message {
			t.Errorf("Targeted client: Expected message '%s', got '%s'", payload.Message, event.Message)
		}
	}

	// Client 2 should NOT receive any message (timeout expected)
	expectNoWebSocketMessage(t, client2, 500*time.Millisecond)
}

// TestWebhookNotifyNotification_Broadcast tests that notify webhooks
// broadcast arbitrary data to all clients
func TestWebhookNotifyNotification_Broadcast(t *testing.T) {
	// Setup test application
	testApp := newTestApplication(t)

	app := &Application{
		storage: nil, // Notify doesn't use storage
		metrics: testMetrics,
		services: &ServiceRegistry{
			Auth: testApp.services.Auth,
		},
		managers: &ManagerRegistry{
			WebSocket: wsmanager.NewManager(testMetrics, nil),
			Activity:  activity.NewTracker(nil), // No storage for notify tests
		},
		jobs: &JobRegistry{}, // Empty jobs registry for tests
	}

	// Create test server
	mux := http.NewServeMux()
	mux.Handle("/api/v1/ws", http.HandlerFunc(app.handleWebSocket))
	mux.Handle("/api/v1/webhooks/notify", http.HandlerFunc(app.handleWebhookNotify))

	server := httptest.NewServer(mux)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/v1/ws"

	// Connect 2 WebSocket clients
	client1 := connectAndAuthenticateClient(t, wsURL, "device-notify-1", app)
	defer client1.Close()

	client2 := connectAndAuthenticateClient(t, wsURL, "device-notify-2", app)
	defer client2.Close()

	// Send notify webhook with custom payload
	payload := WebhookNotifyPayload{
		Type: "custom_alert",
		Data: map[string]interface{}{
			"alertType": "cpu_usage",
			"threshold": 90,
			"current":   95,
			"message":   "CPU usage critical",
		},
	}

	resp := sendWebhookNotify(t, server.URL, payload)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	// Both clients should receive the notification
	msg1 := expectWebSocketMessage(t, client1, types.WSMsgTypeWebhook, 2*time.Second)
	msg2 := expectWebSocketMessage(t, client2, types.WSMsgTypeWebhook, 2*time.Second)

	// Verify payload data
	for i, msg := range []types.WebSocketMessage{msg1, msg2} {
		dataBytes, _ := json.Marshal(msg.Data)
		var received WebhookNotifyPayload
		if err := json.Unmarshal(dataBytes, &received); err != nil {
			t.Errorf("Client %d: Failed to unmarshal notify payload: %v", i+1, err)
			continue
		}

		if received.Type != payload.Type {
			t.Errorf("Client %d: Expected type '%s', got '%s'", i+1, payload.Type, received.Type)
		}

		// Verify custom data fields
		if alertType, ok := received.Data["alertType"].(string); !ok || alertType != "cpu_usage" {
			t.Errorf("Client %d: Invalid alertType in data", i+1)
		}
		if current, ok := received.Data["current"].(float64); !ok || current != 95 {
			t.Errorf("Client %d: Invalid current value in data", i+1)
		}
	}
}

// TestWebhookNotifyNotification_TargetedDelivery tests that notify webhooks
// can target specific devices
func TestWebhookNotifyNotification_TargetedDelivery(t *testing.T) {
	// Setup test application
	testApp := newTestApplication(t)

	app := &Application{
		storage: nil,
		metrics: testMetrics,
		services: &ServiceRegistry{
			Auth: testApp.services.Auth,
		},
		managers: &ManagerRegistry{
			WebSocket: wsmanager.NewManager(testMetrics, nil),
			Activity:  activity.NewTracker(nil), // No storage for notify tests
		},
		jobs: &JobRegistry{}, // Empty jobs registry for tests
	}

	// Create test server
	mux := http.NewServeMux()
	mux.Handle("/api/v1/ws", http.HandlerFunc(app.handleWebSocket))
	mux.Handle("/api/v1/webhooks/notify", http.HandlerFunc(app.handleWebhookNotify))

	server := httptest.NewServer(mux)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/v1/ws"

	// Connect 3 clients
	client1 := connectAndAuthenticateClient(t, wsURL, "device-notify-target-1", app)
	defer client1.Close()

	client2 := connectAndAuthenticateClient(t, wsURL, "device-notify-target-2", app)
	defer client2.Close()

	client3 := connectAndAuthenticateClient(t, wsURL, "device-notify-target-3", app)
	defer client3.Close()

	// Send targeted notify webhook (only to device 2)
	payload := WebhookNotifyPayload{
		DeviceIDs: []string{"device-notify-target-2"},
		Type:      "targeted_update",
		Data: map[string]interface{}{
			"action":  "update_config",
			"version": "2.0.1",
		},
	}

	resp := sendWebhookNotify(t, server.URL, payload)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	// Only client 2 should receive the message
	msg2 := expectWebSocketMessage(t, client2, types.WSMsgTypeWebhook, 2*time.Second)

	// Verify content
	dataBytes, _ := json.Marshal(msg2.Data)
	var received WebhookNotifyPayload
	if err := json.Unmarshal(dataBytes, &received); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if received.Type != payload.Type {
		t.Errorf("Expected type '%s', got '%s'", payload.Type, received.Type)
	}

	// Clients 1 and 3 should NOT receive any message
	expectNoWebSocketMessage(t, client1, 500*time.Millisecond)
	expectNoWebSocketMessage(t, client3, 500*time.Millisecond)
}

// TestWebhookNotification_MultipleMessages tests sending multiple webhooks
// and verifying clients receive all messages in order
func TestWebhookNotification_MultipleMessages(t *testing.T) {
	// Setup test application with storage for activity webhooks
	tmpFile, err := os.CreateTemp("", "nekzus-webhook-multi-*.db")
	if err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()
	dbPath := tmpFile.Name()
	defer os.Remove(dbPath)

	store, err := storage.NewStore(storage.Config{
		DatabasePath: dbPath,
	})
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	// Create auth manager for testing
	testApp := newTestApplication(t)

	app := &Application{
		storage: store,
		metrics: testMetrics,
		services: &ServiceRegistry{
			Auth: testApp.services.Auth,
		},
		managers: &ManagerRegistry{
			WebSocket: wsmanager.NewManager(testMetrics, store),
			Activity:  activity.NewTracker(store),
		},
		jobs: &JobRegistry{}, // Empty jobs registry for tests
	}

	// Create test server with both webhook endpoints
	mux := http.NewServeMux()
	mux.Handle("/api/v1/ws", http.HandlerFunc(app.handleWebSocket))
	mux.Handle("/api/v1/webhooks/activity", http.HandlerFunc(app.handleWebhookActivity))
	mux.Handle("/api/v1/webhooks/notify", http.HandlerFunc(app.handleWebhookNotify))

	server := httptest.NewServer(mux)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/v1/ws"

	// Connect 2 clients
	client1 := connectAndAuthenticateClient(t, wsURL, "device-multi-1", app)
	defer client1.Close()

	client2 := connectAndAuthenticateClient(t, wsURL, "device-multi-2", app)
	defer client2.Close()

	// Send multiple webhooks in sequence
	webhooks := []struct {
		name     string
		sendFunc func()
	}{
		{
			name: "activity-1",
			sendFunc: func() {
				payload := WebhookActivityPayload{
					Message: "First activity",
					Icon:    "Bell",
				}
				resp := sendWebhookActivity(t, server.URL, payload)
				resp.Body.Close()
			},
		},
		{
			name: "notify-1",
			sendFunc: func() {
				payload := WebhookNotifyPayload{
					Type: "update",
					Data: map[string]interface{}{"index": 1},
				}
				resp := sendWebhookNotify(t, server.URL, payload)
				resp.Body.Close()
			},
		},
		{
			name: "activity-2",
			sendFunc: func() {
				payload := WebhookActivityPayload{
					Message: "Second activity",
					Icon:    "CheckCircle",
				}
				resp := sendWebhookActivity(t, server.URL, payload)
				resp.Body.Close()
			},
		},
	}

	// Send all webhooks
	for _, wh := range webhooks {
		wh.sendFunc()
		// Small delay to ensure order (optional, but helps with timing)
		time.Sleep(50 * time.Millisecond)
	}

	// Both clients should receive all 3 messages
	messagesReceived := 0
	for i := 0; i < 3; i++ {
		msg1 := expectWebSocketMessage(t, client1, types.WSMsgTypeWebhook, 2*time.Second)
		msg2 := expectWebSocketMessage(t, client2, types.WSMsgTypeWebhook, 2*time.Second)

		if msg1.Type == types.WSMsgTypeWebhook {
			messagesReceived++
		}
		if msg2.Type == types.WSMsgTypeWebhook {
			messagesReceived++
		}
	}

	// Each client should have received 3 messages (6 total)
	if messagesReceived != 6 {
		t.Errorf("Expected 6 total messages received, got %d", messagesReceived)
	}
}

// TestWebhookNotification_ClientReconnect tests that new clients
// don't receive old webhook messages (webhooks are real-time only)
func TestWebhookNotification_ClientReconnect(t *testing.T) {
	// Setup test application
	tmpFile, err := os.CreateTemp("", "nekzus-webhook-reconnect-*.db")
	if err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()
	dbPath := tmpFile.Name()
	defer os.Remove(dbPath)

	store, err := storage.NewStore(storage.Config{
		DatabasePath: dbPath,
	})
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	// Create auth manager for testing
	testApp := newTestApplication(t)

	app := &Application{
		storage: store,
		metrics: testMetrics,
		services: &ServiceRegistry{
			Auth: testApp.services.Auth,
		},
		managers: &ManagerRegistry{
			WebSocket: wsmanager.NewManager(testMetrics, store),
			Activity:  activity.NewTracker(store),
		},
		jobs: &JobRegistry{}, // Empty jobs registry for tests
	}

	// Create test server
	mux := http.NewServeMux()
	mux.Handle("/api/v1/ws", http.HandlerFunc(app.handleWebSocket))
	mux.Handle("/api/v1/webhooks/activity", http.HandlerFunc(app.handleWebhookActivity))

	server := httptest.NewServer(mux)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/v1/ws"

	// Connect first client
	client1 := connectAndAuthenticateClient(t, wsURL, "device-reconnect-1", app)

	// Send webhook while client1 is connected
	payload := WebhookActivityPayload{
		Message: "Message before reconnect",
		Icon:    "Bell",
	}
	resp := sendWebhookActivity(t, server.URL, payload)
	resp.Body.Close()

	// Client1 should receive it
	expectWebSocketMessage(t, client1, types.WSMsgTypeWebhook, 2*time.Second)

	// Disconnect client1
	client1.Close()
	time.Sleep(100 * time.Millisecond)

	// Reconnect as new client
	client2 := connectAndAuthenticateClient(t, wsURL, "device-reconnect-2", app)
	defer client2.Close()

	// Send new webhook (client2 should receive this one, but not the old one)
	payload2 := WebhookActivityPayload{
		Message: "Message after reconnect",
		Icon:    "Refresh",
	}
	resp2 := sendWebhookActivity(t, server.URL, payload2)
	resp2.Body.Close()

	// Client2 should receive the new message
	msg := expectWebSocketMessage(t, client2, types.WSMsgTypeWebhook, 2*time.Second)

	dataBytes, _ := json.Marshal(msg.Data)
	var event types.ActivityEvent
	if err := json.Unmarshal(dataBytes, &event); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if event.Message != payload2.Message {
		t.Errorf("Expected message '%s', got '%s'", payload2.Message, event.Message)
	}
}

// TestWebhookNotification_ActivityPersistence verifies that activity webhooks
// are persisted to storage (unlike notify webhooks)
func TestWebhookNotification_ActivityPersistence(t *testing.T) {
	// Setup test application with storage
	tmpFile, err := os.CreateTemp("", "nekzus-webhook-persist-*.db")
	if err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()
	dbPath := tmpFile.Name()
	defer os.Remove(dbPath)

	store, err := storage.NewStore(storage.Config{
		DatabasePath: dbPath,
	})
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	// Create auth manager for testing
	testApp := newTestApplication(t)

	app := &Application{
		storage: store,
		metrics: testMetrics,
		services: &ServiceRegistry{
			Auth: testApp.services.Auth,
		},
		managers: &ManagerRegistry{
			WebSocket: wsmanager.NewManager(testMetrics, store),
			Activity:  activity.NewTracker(store),
		},
		jobs: &JobRegistry{}, // Empty jobs registry for tests
	}

	// Create test server
	mux := http.NewServeMux()
	mux.Handle("/api/v1/ws", http.HandlerFunc(app.handleWebSocket))
	mux.Handle("/api/v1/webhooks/activity", http.HandlerFunc(app.handleWebhookActivity))

	server := httptest.NewServer(mux)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/v1/ws"

	// Connect client
	client := connectAndAuthenticateClient(t, wsURL, "device-persist-1", app)
	defer client.Close()

	// Send activity webhook
	payload := WebhookActivityPayload{
		Message:   "Persisted activity",
		Icon:      "Database",
		IconClass: "info",
		Details:   "This should be stored",
	}

	resp := sendWebhookActivity(t, server.URL, payload)
	defer resp.Body.Close()

	// Client should receive via WebSocket
	expectWebSocketMessage(t, client, types.WSMsgTypeWebhook, 2*time.Second)

	// Verify activity was persisted
	activities := app.managers.Activity.Get()
	if len(activities) == 0 {
		t.Fatal("Expected at least 1 activity in tracker")
	}

	// Find our webhook activity
	found := false
	for _, activity := range activities {
		if activity.Message == payload.Message && activity.Type == "webhook.activity" {
			found = true
			if activity.Icon != payload.Icon {
				t.Errorf("Expected icon '%s', got '%s'", payload.Icon, activity.Icon)
			}
			if activity.IconClass != payload.IconClass {
				t.Errorf("Expected iconClass '%s', got '%s'", payload.IconClass, activity.IconClass)
			}
			if activity.Details != payload.Details {
				t.Errorf("Expected details '%s', got '%s'", payload.Details, activity.Details)
			}
			break
		}
	}

	if !found {
		t.Error("Activity webhook was not persisted to activity tracker")
	}
}

// TestWebhookNotification_OfflineDeviceQueuing verifies that webhooks sent to
// offline devices are queued and stored for later delivery
func TestWebhookNotification_OfflineDeviceQueuing(t *testing.T) {
	// Setup test application with storage
	tmpFile, err := os.CreateTemp("", "nekzus-webhook-queue-*.db")
	if err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()
	dbPath := tmpFile.Name()
	defer os.Remove(dbPath)

	store, err := storage.NewStore(storage.Config{
		DatabasePath: dbPath,
	})
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	// Register an offline device
	offlineDeviceID := "device-offline-test"
	if err := store.SaveDevice(offlineDeviceID, "Offline Device", "test", "1.0", []string{"read:events"}); err != nil {
		t.Fatalf("Failed to register device: %v", err)
	}

	// Create auth manager for testing
	testApp := newTestApplication(t)

	app := &Application{
		storage: store,
		metrics: testMetrics,
		services: &ServiceRegistry{
			Auth: testApp.services.Auth,
		},
		managers: &ManagerRegistry{
			WebSocket: wsmanager.NewManager(testMetrics, store),
			Activity:  activity.NewTracker(store),
		},
		jobs: &JobRegistry{},
	}

	// Create test server - note: NO WebSocket client connected for the target device
	mux := http.NewServeMux()
	mux.Handle("/api/v1/webhooks/notify", http.HandlerFunc(app.handleWebhookNotify))

	server := httptest.NewServer(mux)
	defer server.Close()

	// Send notify webhook targeting the offline device
	payload := WebhookNotifyPayload{
		DeviceIDs: []string{offlineDeviceID},
		Type:      "test_notification",
		Data: map[string]interface{}{
			"message": "This should be queued for offline device",
		},
	}

	resp := sendWebhookNotify(t, server.URL, payload)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	// Give a moment for async processing
	time.Sleep(100 * time.Millisecond)

	// Check that the notification was queued in storage
	result, err := store.ListNotifications(storage.NotificationListFilter{
		DeviceID: offlineDeviceID,
	}, 24*time.Hour) // Use a reasonable stale threshold
	if err != nil {
		t.Fatalf("Failed to list notifications: %v", err)
	}

	if len(result.Notifications) == 0 {
		t.Error("Expected notification to be queued for offline device, but found none")
	} else {
		// Verify the queued notification
		notif := result.Notifications[0]
		if notif.DeviceID != offlineDeviceID {
			t.Errorf("Expected device ID '%s', got '%s'", offlineDeviceID, notif.DeviceID)
		}
		if notif.Type != "webhook.notify" {
			t.Errorf("Expected type 'webhook.notify', got '%s'", notif.Type)
		}
		if notif.Status != "pending" {
			t.Errorf("Expected status 'pending', got '%s'", notif.Status)
		}
	}
}

// TestWebhookNotification_QueueDrainOnReconnect verifies that queued notifications
// are delivered when a device reconnects via WebSocket
func TestWebhookNotification_QueueDrainOnReconnect(t *testing.T) {
	// Setup test application with storage
	tmpFile, err := os.CreateTemp("", "nekzus-webhook-drain-*.db")
	if err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()
	dbPath := tmpFile.Name()
	defer os.Remove(dbPath)

	store, err := storage.NewStore(storage.Config{
		DatabasePath: dbPath,
	})
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	// Register a device (initially offline)
	deviceID := "device-drain-test"
	if err := store.SaveDevice(deviceID, "Drain Test Device", "test", "1.0", []string{"read:events"}); err != nil {
		t.Fatalf("Failed to register device: %v", err)
	}

	// Create auth manager for testing
	testApp := newTestApplication(t)

	// Create WebSocket manager
	wsManager := wsmanager.NewManager(testMetrics, store)

	app := &Application{
		storage: store,
		metrics: testMetrics,
		services: &ServiceRegistry{
			Auth: testApp.services.Auth,
		},
		managers: &ManagerRegistry{
			WebSocket: wsManager,
			Activity:  activity.NewTracker(store),
		},
		jobs: &JobRegistry{},
	}

	// Create test server
	mux := http.NewServeMux()
	mux.Handle("/api/v1/ws", http.HandlerFunc(app.handleWebSocket))
	mux.Handle("/api/v1/webhooks/notify", http.HandlerFunc(app.handleWebhookNotify))

	server := httptest.NewServer(mux)
	defer server.Close()

	// Queue a notification for the offline device (simulating webhook while offline)
	notifPayload, _ := json.Marshal(map[string]interface{}{
		"type": "test_notification",
		"data": map[string]interface{}{
			"message": "This was queued while offline",
		},
	})

	notifID, err := store.EnqueueNotification(
		deviceID,
		"webhook.notify",
		notifPayload,
		30*24*time.Hour, // 30 day TTL
		5,               // max retries
	)
	if err != nil {
		t.Fatalf("Failed to enqueue notification: %v", err)
	}

	// Verify notification is pending
	result, err := store.ListNotifications(storage.NotificationListFilter{
		DeviceID: deviceID,
	}, 24*time.Hour)
	if err != nil {
		t.Fatalf("Failed to list notifications: %v", err)
	}

	if len(result.Notifications) == 0 {
		t.Fatal("Expected notification to be queued")
	}
	if result.Notifications[0].Status != "pending" {
		t.Errorf("Expected status 'pending', got '%s'", result.Notifications[0].Status)
	}

	// Set up device connect callback to retry notifications
	wsManager.SetOnDeviceConnect(func(connectedDeviceID string) {
		// Get pending notifications and deliver them
		pending, err := store.GetPendingNotifications(connectedDeviceID)
		if err != nil {
			t.Logf("Failed to get pending notifications: %v", err)
			return
		}

		t.Logf("Found %d pending notifications for device %s", len(pending), connectedDeviceID)

		for _, notif := range pending {
			// Attempt delivery via WebSocket
			err := wsManager.SendToDevice(notif.DeviceID, types.WebSocketMessage{
				Type:      types.WSMsgTypeNotification,
				Data:      notif.Payload,
				Timestamp: time.Now(),
			})
			if err == nil {
				// Mark as delivered
				if markErr := store.MarkNotificationDelivered(notif.ID); markErr != nil {
					t.Logf("Failed to mark delivered: %v", markErr)
				} else {
					t.Logf("Notification %d delivered and marked", notif.ID)
				}
			} else {
				t.Logf("Failed to deliver notification %d: %v", notif.ID, err)
			}
		}
	})

	// Connect the device via WebSocket
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/v1/ws"
	client := connectAndAuthenticateClient(t, wsURL, deviceID, app)
	defer client.Close()

	// Wait for the callback to process
	time.Sleep(500 * time.Millisecond)

	// Expect the notification to be received
	client.SetReadDeadline(time.Now().Add(2 * time.Second))
	var msg types.WebSocketMessage
	if err := client.ReadJSON(&msg); err != nil {
		t.Fatalf("Failed to receive notification: %v", err)
	}

	if msg.Type != types.WSMsgTypeNotification {
		t.Errorf("Expected message type '%s', got '%s'", types.WSMsgTypeNotification, msg.Type)
	}

	// Verify notification is now marked as delivered
	result, err = store.ListNotifications(storage.NotificationListFilter{
		DeviceID: deviceID,
	}, 24*time.Hour)
	if err != nil {
		t.Fatalf("Failed to list notifications: %v", err)
	}

	if len(result.Notifications) == 0 {
		t.Fatal("Notification disappeared from storage")
	}

	// Find the notification we created
	var found bool
	for _, notif := range result.Notifications {
		if notif.ID == notifID {
			found = true
			if notif.Status != "delivered" {
				t.Errorf("Expected status 'delivered', got '%s'", notif.Status)
			}
			break
		}
	}
	if !found {
		t.Error("Could not find the queued notification in results")
	}
}

// TestWebhookNotification_QueueDrainWithRealQueue tests queue draining using the
// actual notification Queue and WebSocketDeliverer (as used in production)
func TestWebhookNotification_QueueDrainWithRealQueue(t *testing.T) {
	// Setup test application with storage
	tmpFile, err := os.CreateTemp("", "nekzus-webhook-realqueue-*.db")
	if err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()
	dbPath := tmpFile.Name()
	defer os.Remove(dbPath)

	store, err := storage.NewStore(storage.Config{
		DatabasePath: dbPath,
	})
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	// Register a device (initially offline)
	deviceID := "device-realqueue-test"
	if err := store.SaveDevice(deviceID, "Real Queue Test Device", "test", "1.0", []string{"read:events"}); err != nil {
		t.Fatalf("Failed to register device: %v", err)
	}

	// Create auth manager for testing
	testApp := newTestApplication(t)

	// Create WebSocket manager
	wsManager := wsmanager.NewManager(testMetrics, store)

	// Create WebSocket adapter for notifications
	wsAdapter := wsmanager.NewManagerAdapter(wsManager)

	// Create ACK tracker with OnACK callback to mark notifications delivered
	ackTracker := notifications.NewACKTracker(notifications.ACKTrackerConfig{
		ACKTimeout:    30 * time.Second,
		CheckInterval: 100 * time.Millisecond,
		OnACK: func(storageID int64) {
			if storageID > 0 {
				if err := store.MarkNotificationDelivered(storageID); err != nil {
					t.Logf("Failed to mark notification delivered: %v", err)
				} else {
					t.Logf("Notification %d marked delivered on ACK", storageID)
				}
			}
		},
	})
	defer ackTracker.Stop()

	// Create notification deliverer with ACK tracking (as used in production)
	deliverer := notifications.NewWebSocketDelivererWithACK(wsAdapter, ackTracker)

	// Create notification queue
	queueConfig := notifications.QueueConfig{
		WorkerCount: 2,
		BufferSize:  100,
	}
	notifQueue := notifications.NewQueue(queueConfig, store, deliverer)

	// Set connectivity checker
	notifQueue.SetConnectivityChecker(wsAdapter)

	// Start the queue
	ctx := context.Background()
	if err := notifQueue.Start(ctx); err != nil {
		t.Fatalf("Failed to start notification queue: %v", err)
	}
	defer notifQueue.Stop()

	app := &Application{
		storage:           store,
		metrics:           testMetrics,
		notificationQueue: notifQueue,
		wsDeliverer:       deliverer, // Set so handleNotificationACK can use ACK tracker
		services: &ServiceRegistry{
			Auth: testApp.services.Auth,
		},
		managers: &ManagerRegistry{
			WebSocket: wsManager,
			Activity:  activity.NewTracker(store),
		},
		jobs: &JobRegistry{},
	}

	// Set up device connect callback (like in real main.go)
	wsManager.SetOnDeviceConnect(func(connectedDeviceID string) {
		t.Logf("Device connect callback triggered for: %s", connectedDeviceID)
		if err := notifQueue.RetryDevice(connectedDeviceID); err != nil {
			t.Logf("RetryDevice error: %v", err)
		} else {
			t.Logf("RetryDevice completed for: %s", connectedDeviceID)
		}
	})

	// Create test server
	mux := http.NewServeMux()
	mux.Handle("/api/v1/ws", http.HandlerFunc(app.handleWebSocket))
	mux.Handle("/api/v1/webhooks/notify", http.HandlerFunc(app.handleWebhookNotify))

	server := httptest.NewServer(mux)
	defer server.Close()

	// Queue a notification for the offline device
	notifPayload, _ := json.Marshal(map[string]interface{}{
		"type": "test_notification",
		"data": map[string]interface{}{
			"message": "This was queued while offline (real queue)",
		},
	})

	notifID, err := store.EnqueueNotification(
		deviceID,
		"webhook.notify",
		notifPayload,
		30*24*time.Hour,
		5,
	)
	if err != nil {
		t.Fatalf("Failed to enqueue notification: %v", err)
	}
	t.Logf("Notification %d queued for device %s", notifID, deviceID)

	// Verify notification is pending
	result, err := store.ListNotifications(storage.NotificationListFilter{
		DeviceID: deviceID,
	}, 24*time.Hour)
	if err != nil {
		t.Fatalf("Failed to list notifications: %v", err)
	}

	if len(result.Notifications) == 0 {
		t.Fatal("Expected notification to be queued")
	}
	if result.Notifications[0].Status != "pending" {
		t.Errorf("Expected status 'pending', got '%s'", result.Notifications[0].Status)
	}

	// Connect the device via WebSocket
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/v1/ws"
	client := connectAndAuthenticateClient(t, wsURL, deviceID, app)
	defer client.Close()

	// Wait for the callback to process
	time.Sleep(500 * time.Millisecond)

	// Expect the notification to be received
	client.SetReadDeadline(time.Now().Add(2 * time.Second))
	var msg types.WebSocketMessage
	if err := client.ReadJSON(&msg); err != nil {
		t.Fatalf("Failed to receive notification: %v", err)
	}

	t.Logf("Received message type: %s, notificationId: %s", msg.Type, msg.NotificationID)

	// Send ACK for the notification (like mobile client does)
	if msg.NotificationID != "" {
		ackMsg := types.WebSocketMessage{
			Type:           types.WSMsgTypeNotificationACK,
			NotificationID: msg.NotificationID,
		}
		if err := client.WriteJSON(ackMsg); err != nil {
			t.Fatalf("Failed to send notification ACK: %v", err)
		}
		t.Logf("Sent ACK for notification: %s", msg.NotificationID)
	}

	// Give time for ACK to be processed and notification marked delivered
	time.Sleep(200 * time.Millisecond)
	result, err = store.ListNotifications(storage.NotificationListFilter{
		DeviceID: deviceID,
	}, 24*time.Hour)
	if err != nil {
		t.Fatalf("Failed to list notifications: %v", err)
	}

	// Find the notification we created
	var found bool
	for _, notif := range result.Notifications {
		if notif.ID == notifID {
			found = true
			t.Logf("Notification %d status: %s", notifID, notif.Status)
			if notif.Status != "delivered" {
				t.Errorf("Expected status 'delivered', got '%s'", notif.Status)
			}
			break
		}
	}
	if !found {
		t.Error("Could not find the queued notification in results")
	}
}
