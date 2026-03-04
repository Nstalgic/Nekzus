package handlers

import (
	"context"
	"sync"
	"time"
)

// LogStream represents an active log streaming session
type LogStream struct {
	ContainerID string
	DeviceID    string
	Cancel      context.CancelFunc
	StartedAt   time.Time
}

// LogStreamManager manages active log streams per device.
// Enforces single stream per device constraint.
type LogStreamManager struct {
	mu      sync.RWMutex
	streams map[string]*LogStream // deviceID -> active stream
}

// NewLogStreamManager creates a new log stream manager
func NewLogStreamManager() *LogStreamManager {
	return &LogStreamManager{
		streams: make(map[string]*LogStream),
	}
}

// StartStream starts a new log stream for a device.
// If device already has an active stream, it is stopped first.
// Returns a context that will be cancelled when the stream should stop.
func (m *LogStreamManager) StartStream(deviceID, containerID string) (context.Context, context.CancelFunc) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Stop existing stream if any
	if existing, ok := m.streams[deviceID]; ok {
		existing.Cancel()
	}

	ctx, cancel := context.WithCancel(context.Background())
	m.streams[deviceID] = &LogStream{
		ContainerID: containerID,
		DeviceID:    deviceID,
		Cancel:      cancel,
		StartedAt:   time.Now(),
	}

	return ctx, cancel
}

// StopStream stops the active log stream for a device.
// Returns true if a stream was stopped, false if no stream existed.
func (m *LogStreamManager) StopStream(deviceID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	if stream, ok := m.streams[deviceID]; ok {
		stream.Cancel()
		delete(m.streams, deviceID)
		return true
	}
	return false
}

// RemoveStream removes a stream from tracking without cancelling.
// This is called when the streaming goroutine ends naturally.
func (m *LogStreamManager) RemoveStream(deviceID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.streams, deviceID)
}

// GetActiveStream returns the active stream for a device (nil if none)
func (m *LogStreamManager) GetActiveStream(deviceID string) *LogStream {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.streams[deviceID]
}

// StopAllForDevice stops all streams for a device.
// This is called on device disconnect for cleanup.
func (m *LogStreamManager) StopAllForDevice(deviceID string) {
	m.StopStream(deviceID)
}
