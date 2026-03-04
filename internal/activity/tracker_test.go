package activity

import (
	"testing"
	"time"

	"github.com/nstalgic/nekzus/internal/storage"
	"github.com/nstalgic/nekzus/internal/types"
)

// newTestStorage creates a temporary storage for testing
func newTestStorage(t *testing.T) *storage.Store {
	t.Helper()
	dbPath := t.TempDir() + "/test_activity.db"
	store, err := storage.NewStore(storage.Config{
		DatabasePath: dbPath,
	})
	if err != nil {
		t.Fatalf("Failed to create test storage: %v", err)
	}
	return store
}

// TestTracker_Add tests adding events to the activity tracker
func TestTracker_Add(t *testing.T) {
	tracker := NewTracker(nil) // No storage for this test

	event := types.ActivityEvent{
		ID:        "test-1",
		Type:      "device.paired",
		Icon:      "Smartphone",
		IconClass: "success",
		Message:   "Device paired: Test Device",
		Timestamp: time.Now().UnixMilli(),
	}

	err := tracker.Add(event)
	if err != nil {
		t.Fatalf("Add() failed: %v", err)
	}

	events := tracker.Get()
	if len(events) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(events))
	}

	if events[0].ID != "test-1" {
		t.Errorf("Expected ID 'test-1', got '%s'", events[0].ID)
	}
}

// TestTracker_Add_MaxLimit tests that tracker respects 10-item limit
func TestTracker_Add_MaxLimit(t *testing.T) {
	tracker := NewTracker(nil)

	// Add 15 events
	for i := 0; i < 15; i++ {
		event := types.ActivityEvent{
			ID:        string(rune('a' + i)),
			Type:      "test.event",
			Icon:      "Activity",
			Message:   "Test event",
			Timestamp: time.Now().UnixMilli(),
		}
		if err := tracker.Add(event); err != nil {
			t.Fatalf("Add() failed at iteration %d: %v", i, err)
		}
	}

	events := tracker.Get()
	if len(events) != 10 {
		t.Fatalf("Expected 10 events (max limit), got %d", len(events))
	}

	// Verify order: most recent first
	// Last added was 'o' (14th), first in result should be 'o'
	if events[0].ID != "o" {
		t.Errorf("Expected most recent event 'o', got '%s'", events[0].ID)
	}

	// Oldest should be 'f' (5th added, since we dropped 0-4)
	if events[9].ID != "f" {
		t.Errorf("Expected oldest event 'f', got '%s'", events[9].ID)
	}
}

// TestTracker_Add_Order tests that events are returned in newest-first order
func TestTracker_Add_Order(t *testing.T) {
	tracker := NewTracker(nil)

	// Add events with explicit timestamps
	baseTime := time.Now().UnixMilli()
	for i := 0; i < 5; i++ {
		event := types.ActivityEvent{
			ID:        string(rune('a' + i)),
			Type:      "test.event",
			Icon:      "Activity",
			Message:   "Test event",
			Timestamp: baseTime + int64(i*1000), // Each 1 second apart
		}
		if err := tracker.Add(event); err != nil {
			t.Fatalf("Add() failed: %v", err)
		}
	}

	events := tracker.Get()
	if len(events) != 5 {
		t.Fatalf("Expected 5 events, got %d", len(events))
	}

	// Should be in reverse order (newest first)
	expectedOrder := []string{"e", "d", "c", "b", "a"}
	for i, expected := range expectedOrder {
		if events[i].ID != expected {
			t.Errorf("Position %d: expected '%s', got '%s'", i, expected, events[i].ID)
		}
	}
}

// TestTracker_Get_Empty tests getting events from empty tracker
func TestTracker_Get_Empty(t *testing.T) {
	tracker := NewTracker(nil)

	events := tracker.Get()
	if len(events) != 0 {
		t.Errorf("Expected 0 events, got %d", len(events))
	}

	// Should return non-nil slice
	if events == nil {
		t.Error("Get() should return non-nil slice")
	}
}

// TestTracker_WithStorage tests integration with storage
func TestTracker_WithStorage(t *testing.T) {
	// Create temporary storage
	storage := newTestStorage(t)
	defer storage.Close()

	tracker := NewTracker(storage)

	event := types.ActivityEvent{
		ID:        "stored-1",
		Type:      "device.paired",
		Icon:      "Smartphone",
		IconClass: "success",
		Message:   "Device paired: Test Device",
		Timestamp: time.Now().UnixMilli(),
	}

	err := tracker.Add(event)
	if err != nil {
		t.Fatalf("Add() with storage failed: %v", err)
	}

	// Verify event is in memory
	events := tracker.Get()
	if len(events) != 1 {
		t.Fatalf("Expected 1 event in memory, got %d", len(events))
	}

	// Verify event is in storage
	storedEvents, err := storage.GetRecentActivity()
	if err != nil {
		t.Fatalf("GetRecentActivity() failed: %v", err)
	}

	if len(storedEvents) != 1 {
		t.Fatalf("Expected 1 event in storage, got %d", len(storedEvents))
	}

	if storedEvents[0].ID != "stored-1" {
		t.Errorf("Expected ID 'stored-1' in storage, got '%s'", storedEvents[0].ID)
	}
}

// TestTracker_LoadFromStorage tests loading events from storage on startup
func TestTracker_LoadFromStorage(t *testing.T) {
	// Create storage and add events directly
	storage := newTestStorage(t)
	defer storage.Close()

	// Pre-populate storage with 5 events
	for i := 0; i < 5; i++ {
		event := types.ActivityEvent{
			ID:        string(rune('a' + i)),
			Type:      "test.event",
			Icon:      "Activity",
			Message:   "Test event",
			Timestamp: time.Now().UnixMilli() + int64(i*1000),
		}
		if err := storage.AddActivity(event); err != nil {
			t.Fatalf("Failed to pre-populate storage: %v", err)
		}
	}

	// Create new tracker - should load from storage
	tracker := NewTracker(storage)

	events := tracker.Get()
	if len(events) != 5 {
		t.Fatalf("Expected 5 events loaded from storage, got %d", len(events))
	}

	// Should be in newest-first order
	if events[0].ID != "e" {
		t.Errorf("Expected newest event 'e', got '%s'", events[0].ID)
	}
}

// TestTracker_Concurrent tests concurrent access
func TestTracker_Concurrent(t *testing.T) {
	tracker := NewTracker(nil)

	// Launch 100 goroutines adding events
	done := make(chan bool)
	for i := 0; i < 100; i++ {
		go func(id int) {
			event := types.ActivityEvent{
				ID:        string(rune('a' + (id % 26))),
				Type:      "test.event",
				Icon:      "Activity",
				Message:   "Concurrent test",
				Timestamp: time.Now().UnixMilli(),
			}
			tracker.Add(event)
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 100; i++ {
		<-done
	}

	// Should have exactly 10 events (max limit)
	events := tracker.Get()
	if len(events) != 10 {
		t.Errorf("Expected 10 events after concurrent adds, got %d", len(events))
	}
}

// TestActivityEvent_JSONSerialization tests that ActivityEvent can be serialized
func TestActivityEvent_JSONSerialization(t *testing.T) {
	// This test will be used to verify the struct can be JSON marshaled for API responses
	// We'll implement the actual JSON test once the struct is defined
	// For now, just verify the concept
	event := types.ActivityEvent{
		ID:        "json-test",
		Type:      "device.paired",
		Icon:      "Smartphone",
		IconClass: "success",
		Message:   "JSON test",
		Details:   "Optional details",
		Timestamp: 1699564800000,
	}

	// Basic field checks
	if event.ID == "" {
		t.Error("ID should not be empty")
	}
	if event.Timestamp <= 0 {
		t.Error("Timestamp should be positive")
	}
}
