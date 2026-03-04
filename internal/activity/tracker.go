package activity

import (
	"log/slog"
	"sync"

	"github.com/nstalgic/nekzus/internal/storage"
	"github.com/nstalgic/nekzus/internal/types"
)

var log = slog.With("package", "activity")

const maxActivityEvents = 10

// Tracker manages activity events with in-memory caching and database persistence.
type Tracker struct {
	mu      sync.RWMutex
	events  []types.ActivityEvent // In-memory circular buffer (max 10)
	storage *storage.Store        // Optional database persistence
}

// NewTracker creates a new activity tracker.
// If storage is provided, it loads recent events from the database.
func NewTracker(store *storage.Store) *Tracker {
	tracker := &Tracker{
		events:  make([]types.ActivityEvent, 0, maxActivityEvents),
		storage: store,
	}

	// Load recent events from storage if available
	if store != nil {
		events, err := store.GetRecentActivity()
		if err != nil {
			log.Warn("Failed to load recent activity from storage", "error", err)
		} else {
			tracker.events = events
		}
	}

	return tracker
}

// Add adds a new activity event to the tracker.
// Events are stored in memory (max 10) and persisted to database if available.
func (at *Tracker) Add(event types.ActivityEvent) error {
	at.mu.Lock()
	defer at.mu.Unlock()

	// Add to in-memory storage (prepend to keep newest first)
	at.events = append([]types.ActivityEvent{event}, at.events...)

	// Trim to max size
	if len(at.events) > maxActivityEvents {
		at.events = at.events[:maxActivityEvents]
	}

	// Persist to database if available
	if at.storage != nil {
		if err := at.storage.AddActivity(event); err != nil {
			log.Warn("Failed to persist activity event to storage", "error", err)
			// Don't return error - in-memory tracking still works
		}
	}

	return nil
}

// Get returns all activity events, newest first.
func (at *Tracker) Get() []types.ActivityEvent {
	at.mu.RLock()
	defer at.mu.RUnlock()

	// Return a copy to prevent external modification
	result := make([]types.ActivityEvent, len(at.events))
	copy(result, at.events)
	return result
}
