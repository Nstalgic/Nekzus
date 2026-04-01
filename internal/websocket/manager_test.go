package websocket

import (
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/nstalgic/nekzus/internal/metrics"
	"github.com/nstalgic/nekzus/internal/storage"
	"github.com/nstalgic/nekzus/internal/types"
)

func TestWebSocketManager_NewManager(t *testing.T) {
	m := metrics.New("test_ws_manager")
	store, err := storage.NewStore(storage.Config{DatabasePath: ":memory:"})
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	wsMgr := NewManager(m, store)
	if wsMgr == nil {
		t.Fatal("Expected non-nil Manager")
	}
	if wsMgr.metrics != m {
		t.Error("Manager metrics not set correctly")
	}
	if wsMgr.storage != store {
		t.Error("Manager storage not set correctly")
	}
}

func TestWebSocketManager_Subscribe(t *testing.T) {
	m := metrics.New("test_ws_subscribe")
	wsMgr := NewManager(m, nil)

	client := &Client{
		deviceID: "device-123",
		conn:     nil, // Would be a real websocket.Conn in practice
	}

	wsMgr.Subscribe(client)

	// Verify client was added
	wsMgr.mu.RLock()
	_, exists := wsMgr.clients[client]
	wsMgr.mu.RUnlock()

	if !exists {
		t.Error("Expected client to be subscribed")
	}
}

func TestWebSocketManager_Unsubscribe(t *testing.T) {
	m := metrics.New("test_ws_unsubscribe")
	wsMgr := NewManager(m, nil)

	client := &Client{
		deviceID: "device-456",
		conn:     nil,
	}

	// Subscribe then unsubscribe
	wsMgr.Subscribe(client)
	wsMgr.Unsubscribe(client)

	// Verify client was removed
	wsMgr.mu.RLock()
	_, exists := wsMgr.clients[client]
	wsMgr.mu.RUnlock()

	if exists {
		t.Error("Expected client to be unsubscribed")
	}
}

func TestWebSocketManager_Broadcast(t *testing.T) {
	m := metrics.New("test_ws_broadcast")
	wsMgr := NewManager(m, nil)

	// Create mock clients with channels to receive messages
	numClients := 3
	receivedMsgs := make([]chan types.WebSocketMessage, numClients)

	for i := 0; i < numClients; i++ {
		receivedMsgs[i] = make(chan types.WebSocketMessage, 10)

		client := &Client{
			deviceID: string(rune('A' + i)),
			conn:     nil,
			sendChan: receivedMsgs[i],
		}

		wsMgr.Subscribe(client)
	}

	// Drain device_status messages from Subscribe broadcasts
	for i := 0; i < numClients; i++ {
		drainStatusMessages(receivedMsgs[i])
	}

	// Broadcast a message
	testMsg := types.WebSocketMessage{
		Type:      types.WSMsgTypeConfigReload,
		Data:      map[string]string{"test": "data"},
		Timestamp: time.Now(),
	}

	wsMgr.Broadcast(testMsg)

	// Verify all clients received the message
	timeout := time.After(1 * time.Second)
	for i := 0; i < numClients; i++ {
		select {
		case msg := <-receivedMsgs[i]:
			if msg.Type != testMsg.Type {
				t.Errorf("Client %d received wrong message type: got %s, want %s", i, msg.Type, testMsg.Type)
			}
		case <-timeout:
			t.Errorf("Client %d did not receive message within timeout", i)
		}
	}
}

func TestWebSocketManager_BroadcastFiltered(t *testing.T) {
	m := metrics.New("test_ws_broadcast_filtered")
	wsMgr := NewManager(m, nil)

	// Create clients
	client1Chan := make(chan types.WebSocketMessage, 10)
	client1 := &Client{
		deviceID: "device-filter-1",
		conn:     nil,
		sendChan: client1Chan,
	}

	client2Chan := make(chan types.WebSocketMessage, 10)
	client2 := &Client{
		deviceID: "device-filter-2",
		conn:     nil,
		sendChan: client2Chan,
	}

	wsMgr.Subscribe(client1)
	wsMgr.Subscribe(client2)

	// Drain device_status messages from Subscribe broadcasts
	drainStatusMessages(client1Chan)
	drainStatusMessages(client2Chan)

	// Broadcast with filter that only allows client1
	testMsg := types.WebSocketMessage{
		Type: types.WSMsgTypeDevicePaired,
		Data: "test",
	}

	filter := func(client *Client) bool {
		return client.deviceID == "device-filter-1"
	}

	wsMgr.BroadcastFiltered(testMsg, filter)

	// Verify only client1 received the message
	select {
	case <-client1Chan:
		// Expected
	case <-time.After(500 * time.Millisecond):
		t.Error("Client 1 should have received the message")
	}

	select {
	case <-client2Chan:
		t.Error("Client 2 should not have received the message")
	case <-time.After(100 * time.Millisecond):
		// Expected - timeout means no message received
	}
}

func TestWebSocketManager_ActiveConnections(t *testing.T) {
	m := metrics.New("test_ws_active")
	wsMgr := NewManager(m, nil)

	// Initially should be 0
	if count := wsMgr.ActiveConnections(); count != 0 {
		t.Errorf("Expected 0 active connections, got %d", count)
	}

	// Add some clients
	for i := 0; i < 5; i++ {
		client := &Client{
			deviceID: string(rune('A' + i)),
			conn:     nil,
		}
		wsMgr.Subscribe(client)
	}

	if count := wsMgr.ActiveConnections(); count != 5 {
		t.Errorf("Expected 5 active connections, got %d", count)
	}
}

func TestWebSocketManager_ConcurrentAccess(t *testing.T) {
	m := metrics.New("test_ws_concurrent")
	wsMgr := NewManager(m, nil)

	// Test concurrent subscribe/unsubscribe/broadcast operations
	var wg sync.WaitGroup
	numGoroutines := 10

	// Concurrent subscribes
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			client := &Client{
				deviceID: string(rune('A' + id)),
				conn:     nil,
				sendChan: make(chan types.WebSocketMessage, 10),
			}
			wsMgr.Subscribe(client)
		}(i)
	}

	// Concurrent broadcasts
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			msg := types.WebSocketMessage{
				Type: types.WSMsgTypeConfigReload,
				Data: id,
			}
			wsMgr.Broadcast(msg)
		}(i)
	}

	wg.Wait()

	// Verify we have the expected number of clients
	if count := wsMgr.ActiveConnections(); count != numGoroutines {
		t.Errorf("Expected %d active connections after concurrent access, got %d", numGoroutines, count)
	}
}

func TestWebSocketManager_BroadcastToClosedChannel(t *testing.T) {
	m := metrics.New("test_ws_closed_channel")
	wsMgr := NewManager(m, nil)

	// Create a client with a channel that will be closed
	clientChan := make(chan types.WebSocketMessage, 1)
	client := &Client{
		deviceID: "device-closed",
		conn:     nil,
		sendChan: clientChan,
	}

	wsMgr.Subscribe(client)

	// Close the channel to simulate a disconnected client
	close(clientChan)

	// Broadcasting should not panic
	testMsg := types.WebSocketMessage{
		Type: types.WSMsgTypeConfigReload,
		Data: "test",
	}

	// This should not panic even though the channel is closed
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Broadcast panicked when sending to closed channel: %v", r)
		}
	}()

	wsMgr.Broadcast(testMsg)
}

func TestWebSocketManager_MessageSerialization(t *testing.T) {
	m := metrics.New("test_ws_serialization")
	wsMgr := NewManager(m, nil)

	// Create a client
	clientChan := make(chan types.WebSocketMessage, 10)
	client := &Client{
		deviceID: "device-serialize",
		conn:     nil,
		sendChan: clientChan,
	}

	wsMgr.Subscribe(client)

	// Drain device_status message from Subscribe
	drainStatusMessages(clientChan)

	// Test various data types
	tests := []struct {
		name string
		msg  types.WebSocketMessage
	}{
		{
			name: "string data",
			msg: types.WebSocketMessage{
				Type: types.WSMsgTypeConfigReload,
				Data: "simple string",
			},
		},
		{
			name: "map data",
			msg: types.WebSocketMessage{
				Type: types.WSMsgTypeDiscovery,
				Data: map[string]interface{}{
					"key":   "value",
					"count": 42,
				},
			},
		},
		{
			name: "struct data",
			msg: types.WebSocketMessage{
				Type: types.WSMsgTypeDevicePaired,
				Data: struct {
					DeviceID string
					Platform string
				}{
					DeviceID: "test-device",
					Platform: "ios",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wsMgr.Broadcast(tt.msg)

			select {
			case received := <-clientChan:
				if received.Type != tt.msg.Type {
					t.Errorf("Message type mismatch: got %s, want %s", received.Type, tt.msg.Type)
				}

				// Verify data can be JSON marshaled/unmarshaled
				jsonData, err := json.Marshal(received.Data)
				if err != nil {
					t.Errorf("Failed to marshal message data: %v", err)
				}

				// Verify we can unmarshal it back
				var decoded interface{}
				if err := json.Unmarshal(jsonData, &decoded); err != nil {
					t.Errorf("Failed to unmarshal message data: %v", err)
				}
			case <-time.After(500 * time.Millisecond):
				t.Error("Did not receive message within timeout")
			}
		})
	}
}

func TestWebSocketManager_MetricsRecording(t *testing.T) {
	m := metrics.New("test_ws_metrics")
	wsMgr := NewManager(m, nil)

	// Subscribe a client (should increment active connections metric)
	client := &Client{
		deviceID: "device-metrics",
		conn:     nil,
		sendChan: make(chan types.WebSocketMessage, 10),
	}

	wsMgr.Subscribe(client)

	// Broadcast a message (should increment messages sent metric)
	testMsg := types.WebSocketMessage{
		Type: types.WSMsgTypeConfigReload,
		Data: "metrics test",
	}

	wsMgr.Broadcast(testMsg)

	// Unsubscribe (should decrement active connections metric)
	wsMgr.Unsubscribe(client)

	// Note: We're not asserting specific metric values here because that would
	// require exposing internal metrics state. In a real scenario, you might want to
	// use a mock metrics recorder or check Prometheus metrics endpoint.
}

// mockWebSocketConn is a mock implementation of websocket.Conn for testing
type mockWebSocketConn struct {
	*websocket.Conn
	written [][]byte
	mu      sync.Mutex
}

func (m *mockWebSocketConn) WriteJSON(v interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	m.written = append(m.written, data)
	return nil
}

func (m *mockWebSocketConn) Close() error {
	return nil
}

func (m *mockWebSocketConn) getWritten() [][]byte {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([][]byte{}, m.written...)
}

// TestDisconnectDevice tests that DisconnectDevice properly closes WebSocket connections
func TestDisconnectDevice(t *testing.T) {
	// Create WebSocket manager
	wm := NewManager(nil, nil)

	// Create mock clients
	client1 := &Client{
		deviceID: "device-123",
		conn:     nil, // Mock connection (nil for test)
		sendChan: make(chan types.WebSocketMessage, 10),
	}

	client2 := &Client{
		deviceID: "device-123", // Same device, different connection
		conn:     nil,
		sendChan: make(chan types.WebSocketMessage, 10),
	}

	client3 := &Client{
		deviceID: "device-456", // Different device
		conn:     nil,
		sendChan: make(chan types.WebSocketMessage, 10),
	}

	// Subscribe all clients
	wm.Subscribe(client1)
	wm.Subscribe(client2)
	wm.Subscribe(client3)

	// Verify all clients are connected
	if wm.ActiveConnections() != 3 {
		t.Errorf("Expected 3 active connections, got %d", wm.ActiveConnections())
	}

	// Disconnect device-123
	disconnected := wm.DisconnectDevice("device-123")

	// Verify correct number disconnected
	if disconnected != 2 {
		t.Errorf("Expected 2 connections disconnected, got %d", disconnected)
	}

	// Verify remaining connections
	if wm.ActiveConnections() != 1 {
		t.Errorf("Expected 1 remaining connection, got %d", wm.ActiveConnections())
	}

	// Verify channels are closed (drain any buffered messages first)
	if !isDrainedAndClosed(client1.sendChan) {
		t.Error("Expected client1 sendChan to be closed")
	}

	if !isDrainedAndClosed(client2.sendChan) {
		t.Error("Expected client2 sendChan to be closed")
	}

	if isChannelClosed(client3.sendChan) {
		t.Error("Expected client3 sendChan to remain open")
	}
}

// TestDisconnectDevice_NoMatchingDevices tests disconnecting a device that doesn't exist
func TestDisconnectDevice_NoMatchingDevices(t *testing.T) {
	wm := NewManager(nil, nil)

	client := &Client{
		deviceID: "device-123",
		conn:     nil,
		sendChan: make(chan types.WebSocketMessage, 10),
	}

	wm.Subscribe(client)

	// Try to disconnect non-existent device
	disconnected := wm.DisconnectDevice("device-999")

	if disconnected != 0 {
		t.Errorf("Expected 0 connections disconnected, got %d", disconnected)
	}

	// Verify client still connected
	if wm.ActiveConnections() != 1 {
		t.Errorf("Expected 1 active connection, got %d", wm.ActiveConnections())
	}
}

// TestDisconnectDevice_EmptyManager tests disconnecting from empty manager
func TestDisconnectDevice_EmptyManager(t *testing.T) {
	wm := NewManager(nil, nil)

	disconnected := wm.DisconnectDevice("device-123")

	if disconnected != 0 {
		t.Errorf("Expected 0 connections disconnected, got %d", disconnected)
	}
}

// TestDisconnectDevice_WithMetrics tests that metrics are properly updated when disconnecting
func TestDisconnectDevice_WithMetrics(t *testing.T) {
	m := metrics.New("test_ws_disconnect_metrics")
	wm := NewManager(m, nil)

	client := &Client{
		deviceID: "device-metrics-test",
		conn:     nil,
		sendChan: make(chan types.WebSocketMessage, 10),
	}

	wm.Subscribe(client)

	// Verify 1 connection
	if wm.ActiveConnections() != 1 {
		t.Errorf("Expected 1 active connection, got %d", wm.ActiveConnections())
	}

	// Disconnect device
	disconnected := wm.DisconnectDevice("device-metrics-test")

	if disconnected != 1 {
		t.Errorf("Expected 1 connection disconnected, got %d", disconnected)
	}

	// Verify metrics updated (connection count should be 0)
	if wm.ActiveConnections() != 0 {
		t.Errorf("Expected 0 active connections after disconnect, got %d", wm.ActiveConnections())
	}
}

// TestDisconnectDevice_ConcurrentDisconnects tests that concurrent disconnects don't cause race conditions
func TestDisconnectDevice_ConcurrentDisconnects(t *testing.T) {
	wm := NewManager(nil, nil)

	// Create multiple clients with the same device ID
	numClients := 10
	deviceID := "device-concurrent"

	for i := 0; i < numClients; i++ {
		client := &Client{
			deviceID: deviceID,
			conn:     nil,
			sendChan: make(chan types.WebSocketMessage, 10),
		}
		wm.Subscribe(client)
	}

	// Verify all clients connected
	if wm.ActiveConnections() != numClients {
		t.Errorf("Expected %d active connections, got %d", numClients, wm.ActiveConnections())
	}

	// Disconnect concurrently from multiple goroutines
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			wm.DisconnectDevice(deviceID)
		}()
	}

	wg.Wait()

	// All clients should be disconnected
	if wm.ActiveConnections() != 0 {
		t.Errorf("Expected 0 active connections after concurrent disconnects, got %d", wm.ActiveConnections())
	}
}

// isChannelClosed checks if a channel is closed by attempting a non-blocking receive
// drainStatusMessages drains any device_status messages from the channel
// that are generated by Subscribe/Unsubscribe broadcasts
func drainStatusMessages(ch chan types.WebSocketMessage) {
	for {
		select {
		case msg := <-ch:
			if msg.Type != types.WSMsgTypeDeviceStatus {
				// Put it back - this is not a status message
				ch <- msg
				return
			}
		default:
			return
		}
	}
}

func isChannelClosed(ch chan types.WebSocketMessage) bool {
	select {
	case _, ok := <-ch:
		return !ok
	default:
		return false
	}
}

// isDrainedAndClosed drains any buffered messages then checks if the channel is closed
func isDrainedAndClosed(ch chan types.WebSocketMessage) bool {
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				return true
			}
			// Drained a buffered message, keep going
		default:
			return false
		}
	}
}

// TestBroadcastFiltered_PoolOptimization tests that sync.Pool optimization works correctly
func TestBroadcastFiltered_PoolOptimization(t *testing.T) {
	wm := NewManager(nil, nil)

	// Create test clients
	numClients := 100
	clients := make([]*Client, numClients)
	for i := 0; i < numClients; i++ {
		client := &Client{
			deviceID: "device-" + string(rune('0'+i%10)),
			sendChan: make(chan types.WebSocketMessage, 10),
		}
		clients[i] = client
		wm.Subscribe(client)
	}

	// Drain device_status messages from Subscribe broadcasts
	for _, client := range clients {
		drainStatusMessages(client.sendChan)
	}

	// Test message
	msg := types.WebSocketMessage{
		Type: "test_pool",
		Data: map[string]string{"foo": "bar"},
	}

	// Broadcast without filter
	wm.BroadcastFiltered(msg, nil)

	// Verify all clients received the message
	receivedCount := 0
	for _, client := range clients {
		select {
		case received := <-client.sendChan:
			if received.Type != msg.Type {
				t.Errorf("Expected message type %s, got %s", msg.Type, received.Type)
			}
			receivedCount++
		case <-time.After(100 * time.Millisecond):
			t.Errorf("Timeout waiting for message on client %s", client.deviceID)
		}
	}

	if receivedCount != numClients {
		t.Errorf("Expected %d clients to receive message, got %d", numClients, receivedCount)
	}

	// Cleanup
	for _, client := range clients {
		wm.Unsubscribe(client)
	}
}

// TestBroadcastFiltered_WithFilter_Pool tests that filtering works correctly with pool
func TestBroadcastFiltered_WithFilter_Pool(t *testing.T) {
	wm := NewManager(nil, nil)

	// Create test clients with different device IDs
	clients := []*Client{
		{deviceID: "device-1", sendChan: make(chan types.WebSocketMessage, 10)},
		{deviceID: "device-2", sendChan: make(chan types.WebSocketMessage, 10)},
		{deviceID: "device-3", sendChan: make(chan types.WebSocketMessage, 10)},
	}

	for _, client := range clients {
		wm.Subscribe(client)
	}

	// Drain device_status messages from Subscribe broadcasts
	for _, client := range clients {
		drainStatusMessages(client.sendChan)
	}

	msg := types.WebSocketMessage{
		Type: "filtered_pool",
		Data: map[string]string{"test": "data"},
	}

	// Filter: only send to device-1 and device-3
	filter := func(c *Client) bool {
		return c.deviceID == "device-1" || c.deviceID == "device-3"
	}

	wm.BroadcastFiltered(msg, filter)

	// Verify device-1 and device-3 received, device-2 did not
	for _, client := range clients {
		select {
		case received := <-client.sendChan:
			if client.deviceID == "device-2" {
				t.Errorf("device-2 should not have received message")
			}
			if received.Type != msg.Type {
				t.Errorf("Expected message type %s, got %s", msg.Type, received.Type)
			}
		case <-time.After(100 * time.Millisecond):
			if client.deviceID != "device-2" {
				t.Errorf("Timeout waiting for message on client %s", client.deviceID)
			}
		}
	}

	// Cleanup
	for _, client := range clients {
		wm.Unsubscribe(client)
	}
}

// TestBroadcastConcurrency_Pool tests concurrent broadcasts don't cause race conditions
func TestBroadcastConcurrency_Pool(t *testing.T) {
	wm := NewManager(nil, nil)

	// Create clients
	numClients := 50
	clients := make([]*Client, numClients)
	for i := 0; i < numClients; i++ {
		client := &Client{
			deviceID: "device-" + string(rune('0'+i%10)),
			sendChan: make(chan types.WebSocketMessage, 100),
		}
		clients[i] = client
		wm.Subscribe(client)
	}

	// Concurrent broadcasts
	numBroadcasts := 20
	var wg sync.WaitGroup
	wg.Add(numBroadcasts)

	for i := 0; i < numBroadcasts; i++ {
		go func(iteration int) {
			defer wg.Done()
			msg := types.WebSocketMessage{
				Type: "concurrent_pool",
				Data: map[string]int{"iteration": iteration},
			}
			wm.BroadcastFiltered(msg, nil)
		}(i)
	}

	wg.Wait()

	// Verify each client received all broadcasts
	for _, client := range clients {
		count := 0
		timeout := time.After(1 * time.Second)
	drainLoop:
		for {
			select {
			case <-client.sendChan:
				count++
				if count >= numBroadcasts {
					break drainLoop
				}
			case <-timeout:
				break drainLoop
			}
		}

		if count != numBroadcasts {
			t.Errorf("Client %s received %d messages, expected %d", client.deviceID, count, numBroadcasts)
		}
	}

	// Cleanup
	for _, client := range clients {
		wm.Unsubscribe(client)
	}
}

// BenchmarkBroadcast_Pool benchmarks broadcast with sync.Pool optimization
func BenchmarkBroadcast_Pool(b *testing.B) {
	wm := NewManager(nil, nil)

	// Create clients
	numClients := 100
	clients := make([]*Client, numClients)
	for i := 0; i < numClients; i++ {
		client := &Client{
			deviceID: "device-" + string(rune('0'+i%10)),
			sendChan: make(chan types.WebSocketMessage, 100),
		}
		clients[i] = client
		wm.Subscribe(client)
	}

	// Drain messages in background
	done := make(chan struct{})
	for _, client := range clients {
		go func(c *Client) {
			for {
				select {
				case <-c.sendChan:
					// Drain
				case <-done:
					return
				}
			}
		}(client)
	}

	msg := types.WebSocketMessage{
		Type: "benchmark_pool",
		Data: map[string]string{"test": "data"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		wm.BroadcastFiltered(msg, nil)
	}
	b.StopTimer()

	close(done)

	// Cleanup
	for _, client := range clients {
		wm.Unsubscribe(client)
	}
}

// BenchmarkBroadcastFiltered_Pool benchmarks filtered broadcast with pool
func BenchmarkBroadcastFiltered_Pool(b *testing.B) {
	wm := NewManager(nil, nil)

	// Create clients
	numClients := 100
	clients := make([]*Client, numClients)
	for i := 0; i < numClients; i++ {
		client := &Client{
			deviceID: "device-" + string(rune('0'+i%10)),
			sendChan: make(chan types.WebSocketMessage, 100),
		}
		clients[i] = client
		wm.Subscribe(client)
	}

	// Drain messages in background
	done := make(chan struct{})
	for _, client := range clients {
		go func(c *Client) {
			for {
				select {
				case <-c.sendChan:
					// Drain
				case <-done:
					return
				}
			}
		}(client)
	}

	msg := types.WebSocketMessage{
		Type: "benchmark_filtered_pool",
		Data: map[string]string{"test": "data"},
	}

	// Filter: only send to even-numbered devices
	filter := func(c *Client) bool {
		return c.deviceID[len(c.deviceID)-1]%2 == 0
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		wm.BroadcastFiltered(msg, filter)
	}
	b.StopTimer()

	close(done)

	// Cleanup
	for _, client := range clients {
		wm.Unsubscribe(client)
	}
}

// ============================================
// Tests for MQTT-style topic subscriptions
// ============================================

func TestClient_SubscribeToTopics(t *testing.T) {
	client := NewClient("device-1", nil, make(chan types.WebSocketMessage, 10))

	// Subscribe to topics
	err := client.SubscribeToTopics([]string{"health_change", "container.#"}, SubscriptionOptions{QoS: 1})
	if err != nil {
		t.Errorf("Failed to subscribe: %v", err)
	}

	// Verify subscriptions
	subs := client.GetSubscriptions()
	if len(subs) != 2 {
		t.Errorf("Expected 2 subscriptions, got %d", len(subs))
	}

	if opts, ok := subs["health_change"]; !ok || opts.QoS != 1 {
		t.Error("health_change subscription not found or wrong QoS")
	}

	if opts, ok := subs["container.#"]; !ok || opts.QoS != 1 {
		t.Error("container.# subscription not found or wrong QoS")
	}
}

func TestClient_SubscribeToTopics_InvalidPattern(t *testing.T) {
	client := NewClient("device-1", nil, make(chan types.WebSocketMessage, 10))

	// Try to subscribe with invalid pattern
	err := client.SubscribeToTopics([]string{"container.#.invalid"}, SubscriptionOptions{QoS: 1})
	if err == nil {
		t.Error("Expected error for invalid pattern")
	}
}

func TestClient_UnsubscribeFromTopics(t *testing.T) {
	client := NewClient("device-1", nil, make(chan types.WebSocketMessage, 10))

	// Subscribe to topics
	_ = client.SubscribeToTopics([]string{"health_change", "discovery"}, SubscriptionOptions{QoS: 0})

	// Unsubscribe from one
	client.UnsubscribeFromTopics([]string{"health_change"})

	// Verify
	subs := client.GetSubscriptions()
	if len(subs) != 1 {
		t.Errorf("Expected 1 subscription after unsubscribe, got %d", len(subs))
	}

	if _, ok := subs["discovery"]; !ok {
		t.Error("discovery subscription should still exist")
	}
}

func TestClient_IsSubscribedTo(t *testing.T) {
	client := NewClient("device-1", nil, make(chan types.WebSocketMessage, 10))

	// No subscriptions - should match everything (backward compatible)
	if !client.IsSubscribedTo("health_change") {
		t.Error("Client with no subscriptions should match all topics")
	}

	// Add subscriptions
	_ = client.SubscribeToTopics([]string{"health_change", "container.#"}, SubscriptionOptions{QoS: 0})

	// Test matching
	tests := []struct {
		topic    string
		expected bool
	}{
		{"health_change", true},
		{"container.logs", true},
		{"container.logs.data", true},
		{"discovery", false},
		{"config_reload", false},
	}

	for _, tt := range tests {
		result := client.IsSubscribedTo(tt.topic)
		if result != tt.expected {
			t.Errorf("IsSubscribedTo(%q) = %v, want %v", tt.topic, result, tt.expected)
		}
	}
}

func TestClient_GetSubscriptionQoS(t *testing.T) {
	client := NewClient("device-1", nil, make(chan types.WebSocketMessage, 10))

	// Add subscriptions with different QoS
	_ = client.SubscribeToTopics([]string{"health_change"}, SubscriptionOptions{QoS: 0})
	_ = client.SubscribeToTopics([]string{"container.#"}, SubscriptionOptions{QoS: 1})

	// Test QoS retrieval
	if qos := client.GetSubscriptionQoS("health_change"); qos != 0 {
		t.Errorf("Expected QoS 0 for health_change, got %d", qos)
	}

	if qos := client.GetSubscriptionQoS("container.logs"); qos != 1 {
		t.Errorf("Expected QoS 1 for container.logs, got %d", qos)
	}

	// Non-matching topic should return 0
	if qos := client.GetSubscriptionQoS("discovery"); qos != 0 {
		t.Errorf("Expected QoS 0 for non-matching topic, got %d", qos)
	}
}

func TestClient_LastWill(t *testing.T) {
	client := NewClient("device-1", nil, make(chan types.WebSocketMessage, 10))

	// Initially no last will
	if lw := client.GetLastWill(); lw != nil {
		t.Error("Expected nil last will initially")
	}

	// Set last will
	lw := &LastWill{
		Topic: "device_status",
		Message: types.WebSocketMessage{
			Type: "device_offline",
			Data: map[string]string{"deviceId": "device-1"},
		},
		QoS: 1,
	}
	client.SetLastWill(lw)

	// Retrieve
	retrieved := client.GetLastWill()
	if retrieved == nil {
		t.Fatal("Expected last will to be set")
	}
	if retrieved.Topic != "device_status" {
		t.Errorf("Expected topic device_status, got %s", retrieved.Topic)
	}

	// Clear
	client.ClearLastWill()
	if client.GetLastWill() != nil {
		t.Error("Expected nil last will after clear")
	}
}

func TestManager_SubscribeClientToTopics(t *testing.T) {
	wm := NewManager(nil, nil)

	// Create and subscribe a client
	client := NewClient("device-1", nil, make(chan types.WebSocketMessage, 10))
	wm.Subscribe(client)

	// Subscribe to topics via manager
	err := wm.SubscribeClientToTopics("device-1", []string{"health_change", "discovery"}, SubscriptionOptions{QoS: 1})
	if err != nil {
		t.Errorf("Failed to subscribe: %v", err)
	}

	// Verify
	subs, _ := wm.GetClientSubscriptions("device-1")
	if len(subs) != 2 {
		t.Errorf("Expected 2 subscriptions, got %d", len(subs))
	}
}

func TestManager_SubscribeClientToTopics_NotConnected(t *testing.T) {
	wm := NewManager(nil, nil)

	err := wm.SubscribeClientToTopics("non-existent", []string{"health_change"}, SubscriptionOptions{QoS: 0})
	if err == nil {
		t.Error("Expected error for non-connected device")
	}
}

func TestManager_UnsubscribeClientFromTopics(t *testing.T) {
	wm := NewManager(nil, nil)

	client := NewClient("device-1", nil, make(chan types.WebSocketMessage, 10))
	wm.Subscribe(client)

	_ = wm.SubscribeClientToTopics("device-1", []string{"health_change", "discovery"}, SubscriptionOptions{QoS: 0})
	_ = wm.UnsubscribeClientFromTopics("device-1", []string{"health_change"})

	subs, _ := wm.GetClientSubscriptions("device-1")
	if len(subs) != 1 {
		t.Errorf("Expected 1 subscription after unsubscribe, got %d", len(subs))
	}
}

func TestManager_PublishToTopic(t *testing.T) {
	wm := NewManager(nil, nil)

	// Create clients with different subscriptions
	client1 := NewClient("device-1", nil, make(chan types.WebSocketMessage, 10))
	client2 := NewClient("device-2", nil, make(chan types.WebSocketMessage, 10))

	wm.Subscribe(client1)
	wm.Subscribe(client2)

	// Drain device_status messages from Subscribe broadcasts
	drainStatusMessages(client1.sendChan)
	drainStatusMessages(client2.sendChan)

	// Subscribe client1 to health_change only
	_ = client1.SubscribeToTopics([]string{"health_change"}, SubscriptionOptions{QoS: 0})

	// Subscribe client2 to discovery only
	_ = client2.SubscribeToTopics([]string{"discovery"}, SubscriptionOptions{QoS: 0})

	// Publish health_change
	msg := types.WebSocketMessage{
		Type: types.WSMsgTypeHealthChange,
		Data: map[string]string{"status": "healthy"},
	}
	wm.PublishToTopic("health_change", msg)

	// Client1 should receive it
	select {
	case received := <-client1.sendChan:
		if received.Type != types.WSMsgTypeHealthChange {
			t.Errorf("Expected health_change, got %s", received.Type)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Client1 should have received health_change")
	}

	// Client2 should not receive it
	select {
	case <-client2.sendChan:
		t.Error("Client2 should not have received health_change")
	case <-time.After(50 * time.Millisecond):
		// Expected
	}
}

func TestManager_RetainedMessages(t *testing.T) {
	wm := NewManager(nil, nil)

	// Set retained message
	msg := types.WebSocketMessage{
		Type:   types.WSMsgTypeHealthChange,
		Data:   map[string]string{"status": "healthy"},
		Retain: true,
	}
	wm.SetRetainedMessage("health_change", msg)

	// Retrieve
	rm := wm.GetRetainedMessage("health_change")
	if rm == nil {
		t.Fatal("Expected retained message")
	}
	if rm.Topic != "health_change" {
		t.Errorf("Expected topic health_change, got %s", rm.Topic)
	}

	// Count
	if count := wm.GetRetainedMessageCount(); count != 1 {
		t.Errorf("Expected 1 retained message, got %d", count)
	}

	// Clear
	wm.ClearRetainedMessage("health_change")
	if wm.GetRetainedMessage("health_change") != nil {
		t.Error("Expected nil after clear")
	}
}

func TestManager_RetainedMessages_Expiry(t *testing.T) {
	wm := NewManager(nil, nil)

	// Set retained message with expiry in the past
	msg := types.WebSocketMessage{
		Type:      types.WSMsgTypeHealthChange,
		Data:      map[string]string{"status": "healthy"},
		ExpiresAt: time.Now().Add(-1 * time.Hour),
	}
	wm.SetRetainedMessage("expired", msg)

	// Should not return expired message
	if rm := wm.GetRetainedMessage("expired"); rm != nil {
		t.Error("Should not return expired message")
	}

	// Set non-expired
	msg2 := types.WebSocketMessage{
		Type:      types.WSMsgTypeDiscovery,
		ExpiresAt: time.Now().Add(1 * time.Hour),
	}
	wm.SetRetainedMessage("valid", msg2)

	// Clean expired
	cleaned := wm.CleanExpiredRetainedMessages()
	if cleaned != 1 {
		t.Errorf("Expected 1 cleaned, got %d", cleaned)
	}

	// Valid should still exist
	if wm.GetRetainedMessage("valid") == nil {
		t.Error("Valid message should still exist")
	}
}

func TestManager_SendRetainedOnSubscribe(t *testing.T) {
	wm := NewManager(nil, nil)

	// Set retained message first
	msg := types.WebSocketMessage{
		Type: types.WSMsgTypeHealthChange,
		Data: map[string]string{"status": "healthy"},
	}
	wm.SetRetainedMessage("health_change", msg)

	// Now subscribe a client
	client := NewClient("device-1", nil, make(chan types.WebSocketMessage, 10))
	wm.Subscribe(client)

	// Drain device_status message from Subscribe
	select {
	case msg := <-client.sendChan:
		if msg.Type != types.WSMsgTypeDeviceStatus {
			t.Errorf("Expected device_status from Subscribe, got %s", msg.Type)
		}
	case <-time.After(100 * time.Millisecond):
	}

	// Subscribe to topic - should receive retained message
	_ = wm.SubscribeClientToTopics("device-1", []string{"health_change"}, SubscriptionOptions{QoS: 0})

	// Should receive retained message
	select {
	case received := <-client.sendChan:
		if received.Type != types.WSMsgTypeHealthChange {
			t.Errorf("Expected health_change, got %s", received.Type)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Should have received retained message")
	}
}

func TestManager_UnsubscribeUnexpected_PublishesLastWill(t *testing.T) {
	wm := NewManager(nil, nil)

	// Create client with last will
	client := NewClient("device-1", nil, make(chan types.WebSocketMessage, 10))
	lw := &LastWill{
		Topic: "device_status",
		Message: types.WebSocketMessage{
			Type: "device_offline",
			Data: map[string]string{"deviceId": "device-1"},
		},
		QoS: 0,
	}
	client.SetLastWill(lw)

	// Create another client to receive the LWT
	receiver := NewClient("receiver", nil, make(chan types.WebSocketMessage, 10))

	wm.Subscribe(client)
	wm.Subscribe(receiver)

	// Drain device_status messages from Subscribe broadcasts
	drainStatusMessages(receiver.sendChan)

	// Unexpected disconnect
	wm.UnsubscribeUnexpected(client)

	// Drain device_status (offline) message from Unsubscribe
	drainStatusMessages(receiver.sendChan)

	// Receiver should get the LWT
	select {
	case received := <-receiver.sendChan:
		if received.Type != "device_offline" {
			t.Errorf("Expected device_offline, got %s", received.Type)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Should have received last will message")
	}
}

func TestManager_ClientsByDevice(t *testing.T) {
	wm := NewManager(nil, nil)

	// Create multiple clients for same device
	client1 := NewClient("device-1", nil, make(chan types.WebSocketMessage, 10))
	client2 := NewClient("device-1", nil, make(chan types.WebSocketMessage, 10))
	client3 := NewClient("device-2", nil, make(chan types.WebSocketMessage, 10))

	wm.Subscribe(client1)
	wm.Subscribe(client2)
	wm.Subscribe(client3)

	// Verify clientsByDevice
	wm.mu.RLock()
	device1Clients := wm.clientsByDevice["device-1"]
	device2Clients := wm.clientsByDevice["device-2"]
	wm.mu.RUnlock()

	if len(device1Clients) != 2 {
		t.Errorf("Expected 2 clients for device-1, got %d", len(device1Clients))
	}
	if len(device2Clients) != 1 {
		t.Errorf("Expected 1 client for device-2, got %d", len(device2Clients))
	}

	// Unsubscribe one
	wm.Unsubscribe(client1)

	wm.mu.RLock()
	device1Clients = wm.clientsByDevice["device-1"]
	wm.mu.RUnlock()

	if len(device1Clients) != 1 {
		t.Errorf("Expected 1 client for device-1 after unsubscribe, got %d", len(device1Clients))
	}
}
