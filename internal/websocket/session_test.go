package websocket

import (
	"testing"
	"time"

	"github.com/nstalgic/nekzus/internal/types"
)

func TestInMemorySessionStore_SaveAndGet(t *testing.T) {
	store := NewInMemorySessionStore()

	session := &Session{
		DeviceID: "device-1",
		Subscriptions: map[string]SubscriptionOptions{
			"health_change": {QoS: 1},
			"container.#":   {QoS: 0},
		},
		LastSeen: time.Now(),
	}

	err := store.SaveSession(session)
	if err != nil {
		t.Fatalf("Failed to save session: %v", err)
	}

	retrieved, err := store.GetSession("device-1")
	if err != nil {
		t.Fatalf("Failed to get session: %v", err)
	}

	if retrieved == nil {
		t.Fatal("Expected session to be retrieved")
	}

	if len(retrieved.Subscriptions) != 2 {
		t.Errorf("Expected 2 subscriptions, got %d", len(retrieved.Subscriptions))
	}
}

func TestInMemorySessionStore_GetNonExistent(t *testing.T) {
	store := NewInMemorySessionStore()

	retrieved, err := store.GetSession("non-existent")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if retrieved != nil {
		t.Error("Expected nil for non-existent session")
	}
}

func TestInMemorySessionStore_Delete(t *testing.T) {
	store := NewInMemorySessionStore()

	session := &Session{DeviceID: "device-1"}
	_ = store.SaveSession(session)

	err := store.DeleteSession("device-1")
	if err != nil {
		t.Fatalf("Failed to delete session: %v", err)
	}

	retrieved, _ := store.GetSession("device-1")
	if retrieved != nil {
		t.Error("Session should be deleted")
	}
}

func TestInMemorySessionStore_UpdateLastSeen(t *testing.T) {
	store := NewInMemorySessionStore()

	oldTime := time.Now().Add(-1 * time.Hour)
	session := &Session{
		DeviceID: "device-1",
		LastSeen: oldTime,
	}
	_ = store.SaveSession(session)

	err := store.UpdateLastSeen("device-1")
	if err != nil {
		t.Fatalf("Failed to update last seen: %v", err)
	}

	retrieved, _ := store.GetSession("device-1")
	if retrieved.LastSeen.Before(oldTime.Add(30 * time.Minute)) {
		t.Error("Last seen should be updated to recent time")
	}
}

func TestInMemorySessionStore_CleanExpired(t *testing.T) {
	store := NewInMemorySessionStore()

	// Add old session
	oldSession := &Session{
		DeviceID: "old-device",
		LastSeen: time.Now().Add(-2 * time.Hour),
	}
	_ = store.SaveSession(oldSession)

	// Add recent session
	newSession := &Session{
		DeviceID: "new-device",
		LastSeen: time.Now(),
	}
	_ = store.SaveSession(newSession)

	// Clean sessions older than 1 hour
	cleaned, err := store.CleanExpiredSessions(1 * time.Hour)
	if err != nil {
		t.Fatalf("Failed to clean sessions: %v", err)
	}

	if cleaned != 1 {
		t.Errorf("Expected 1 cleaned, got %d", cleaned)
	}

	// Old should be gone
	old, _ := store.GetSession("old-device")
	if old != nil {
		t.Error("Old session should be cleaned")
	}

	// New should remain
	new, _ := store.GetSession("new-device")
	if new == nil {
		t.Error("New session should remain")
	}
}

func TestSessionFromClient(t *testing.T) {
	client := NewClient("device-1", nil, make(chan types.WebSocketMessage, 10))

	// Add subscriptions
	_ = client.SubscribeToTopics([]string{"health_change", "discovery"}, SubscriptionOptions{QoS: 1})

	// Add last will
	client.SetLastWill(&LastWill{
		Topic: "device_status",
		Message: types.WebSocketMessage{
			Type: "device_offline",
			Data: map[string]string{"deviceId": "device-1"},
		},
		QoS: 0,
	})

	session := SessionFromClient(client)

	if session.DeviceID != "device-1" {
		t.Errorf("Expected device ID device-1, got %s", session.DeviceID)
	}

	if len(session.Subscriptions) != 2 {
		t.Errorf("Expected 2 subscriptions, got %d", len(session.Subscriptions))
	}

	if session.LastWill == nil {
		t.Error("Expected last will to be set")
	}
}

func TestRestoreClientFromSession(t *testing.T) {
	client := NewClient("device-1", nil, make(chan types.WebSocketMessage, 10))

	session := &Session{
		DeviceID: "device-1",
		Subscriptions: map[string]SubscriptionOptions{
			"health_change": {QoS: 1},
			"container.#":   {QoS: 0},
		},
		LastWill: &LastWill{
			Topic: "device_status",
			QoS:   1,
		},
	}

	RestoreClientFromSession(client, session)

	// Verify subscriptions restored
	subs := client.GetSubscriptions()
	if len(subs) != 2 {
		t.Errorf("Expected 2 subscriptions, got %d", len(subs))
	}

	// Verify last will restored
	lw := client.GetLastWill()
	if lw == nil {
		t.Error("Expected last will to be restored")
	}
	if lw.Topic != "device_status" {
		t.Errorf("Expected topic device_status, got %s", lw.Topic)
	}
}

func TestRestoreClientFromSession_Nil(t *testing.T) {
	client := NewClient("device-1", nil, make(chan types.WebSocketMessage, 10))

	// Should not panic with nil session
	RestoreClientFromSession(client, nil)

	if client.HasSubscriptions() {
		t.Error("Client should have no subscriptions")
	}
}

func TestSessionManager(t *testing.T) {
	store := NewInMemorySessionStore()
	manager := NewSessionManager(store, 24*time.Hour)

	client := NewClient("device-1", nil, make(chan types.WebSocketMessage, 10))
	_ = client.SubscribeToTopics([]string{"health_change"}, SubscriptionOptions{QoS: 1})

	// Save session
	err := manager.SaveClientSession(client)
	if err != nil {
		t.Fatalf("Failed to save session: %v", err)
	}

	// Create new client and restore
	newClient := NewClient("device-1", nil, make(chan types.WebSocketMessage, 10))
	err = manager.RestoreClientSession(newClient)
	if err != nil {
		t.Fatalf("Failed to restore session: %v", err)
	}

	// Verify subscriptions restored
	subs := newClient.GetSubscriptions()
	if len(subs) != 1 {
		t.Errorf("Expected 1 subscription, got %d", len(subs))
	}
}

func TestMarshalUnmarshalSession(t *testing.T) {
	session := &Session{
		DeviceID: "device-1",
		Subscriptions: map[string]SubscriptionOptions{
			"health_change": {QoS: 1},
		},
		LastWill: &LastWill{
			Topic: "device_status",
			QoS:   0,
		},
		CreatedAt: time.Now(),
		LastSeen:  time.Now(),
	}

	data, err := MarshalSession(session)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	restored, err := UnmarshalSession(data)
	if err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if restored.DeviceID != session.DeviceID {
		t.Error("Device ID mismatch")
	}

	if len(restored.Subscriptions) != len(session.Subscriptions) {
		t.Error("Subscriptions mismatch")
	}
}
