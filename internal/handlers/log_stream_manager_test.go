package handlers

import (
	"sync"
	"testing"
	"time"
)

func TestLogStreamManager_StartStream_Success(t *testing.T) {
	mgr := NewLogStreamManager()

	ctx, cancel := mgr.StartStream("device-1", "container-abc")
	defer cancel()

	if ctx == nil {
		t.Fatal("Expected context to be non-nil")
	}

	// Verify stream is tracked
	stream := mgr.GetActiveStream("device-1")
	if stream == nil {
		t.Fatal("Expected active stream to exist")
	}
	if stream.ContainerID != "container-abc" {
		t.Errorf("Expected containerID 'container-abc', got %s", stream.ContainerID)
	}
	if stream.DeviceID != "device-1" {
		t.Errorf("Expected deviceID 'device-1', got %s", stream.DeviceID)
	}
}

func TestLogStreamManager_StartStream_ReplacesExisting(t *testing.T) {
	mgr := NewLogStreamManager()

	// Start first stream
	ctx1, _ := mgr.StartStream("device-1", "container-1")

	// Capture when first context is cancelled
	ctx1Done := make(chan struct{})
	go func() {
		<-ctx1.Done()
		close(ctx1Done)
	}()

	// Start second stream for same device
	ctx2, cancel2 := mgr.StartStream("device-1", "container-2")
	defer cancel2()

	// First context should be cancelled
	select {
	case <-ctx1Done:
		// Expected
	case <-time.After(100 * time.Millisecond):
		t.Error("Expected first context to be cancelled")
	}

	// Second context should still be active
	select {
	case <-ctx2.Done():
		t.Error("Second context should not be cancelled")
	default:
		// Expected
	}

	// Only second stream should be tracked
	stream := mgr.GetActiveStream("device-1")
	if stream == nil {
		t.Fatal("Expected active stream to exist")
	}
	if stream.ContainerID != "container-2" {
		t.Errorf("Expected containerID 'container-2', got %s", stream.ContainerID)
	}
}

func TestLogStreamManager_StopStream_Success(t *testing.T) {
	mgr := NewLogStreamManager()

	ctx, _ := mgr.StartStream("device-1", "container-abc")

	// Capture when context is cancelled
	ctxDone := make(chan struct{})
	go func() {
		<-ctx.Done()
		close(ctxDone)
	}()

	// Stop the stream
	stopped := mgr.StopStream("device-1")
	if !stopped {
		t.Error("Expected StopStream to return true")
	}

	// Context should be cancelled
	select {
	case <-ctxDone:
		// Expected
	case <-time.After(100 * time.Millisecond):
		t.Error("Expected context to be cancelled")
	}

	// Stream should be removed
	stream := mgr.GetActiveStream("device-1")
	if stream != nil {
		t.Error("Expected no active stream after stop")
	}
}

func TestLogStreamManager_StopStream_NoActiveStream(t *testing.T) {
	mgr := NewLogStreamManager()

	// Stop non-existent stream
	stopped := mgr.StopStream("device-1")
	if stopped {
		t.Error("Expected StopStream to return false for non-existent stream")
	}
}

func TestLogStreamManager_GetActiveStream_NoStream(t *testing.T) {
	mgr := NewLogStreamManager()

	stream := mgr.GetActiveStream("device-1")
	if stream != nil {
		t.Error("Expected nil for non-existent device")
	}
}

func TestLogStreamManager_RemoveStream(t *testing.T) {
	mgr := NewLogStreamManager()

	_, cancel := mgr.StartStream("device-1", "container-abc")
	defer cancel()

	// Verify stream exists
	if mgr.GetActiveStream("device-1") == nil {
		t.Fatal("Expected stream to exist")
	}

	// Remove stream
	mgr.RemoveStream("device-1")

	// Stream should be gone
	if mgr.GetActiveStream("device-1") != nil {
		t.Error("Expected stream to be removed")
	}
}

func TestLogStreamManager_StopAllForDevice(t *testing.T) {
	mgr := NewLogStreamManager()

	ctx, _ := mgr.StartStream("device-1", "container-abc")

	// Capture when context is cancelled
	ctxDone := make(chan struct{})
	go func() {
		<-ctx.Done()
		close(ctxDone)
	}()

	// Stop all for device
	mgr.StopAllForDevice("device-1")

	// Context should be cancelled
	select {
	case <-ctxDone:
		// Expected
	case <-time.After(100 * time.Millisecond):
		t.Error("Expected context to be cancelled")
	}

	// Stream should be removed
	if mgr.GetActiveStream("device-1") != nil {
		t.Error("Expected no active stream")
	}
}

func TestLogStreamManager_ConcurrentAccess(t *testing.T) {
	mgr := NewLogStreamManager()

	var wg sync.WaitGroup
	devices := []string{"device-1", "device-2", "device-3", "device-4", "device-5"}

	// Start streams concurrently
	for _, deviceID := range devices {
		wg.Add(1)
		go func(d string) {
			defer wg.Done()
			_, cancel := mgr.StartStream(d, "container-"+d)
			defer cancel()

			// Simulate some work
			time.Sleep(10 * time.Millisecond)

			// Access stream
			_ = mgr.GetActiveStream(d)
		}(deviceID)
	}

	// Stop streams concurrently
	for _, deviceID := range devices {
		wg.Add(1)
		go func(d string) {
			defer wg.Done()
			time.Sleep(5 * time.Millisecond)
			mgr.StopStream(d)
		}(deviceID)
	}

	wg.Wait()
	// Test should not panic or deadlock
}

func TestLogStreamManager_MultipleDevices(t *testing.T) {
	mgr := NewLogStreamManager()

	// Start streams for multiple devices
	_, cancel1 := mgr.StartStream("device-1", "container-1")
	defer cancel1()
	_, cancel2 := mgr.StartStream("device-2", "container-2")
	defer cancel2()

	// Both should have active streams
	stream1 := mgr.GetActiveStream("device-1")
	stream2 := mgr.GetActiveStream("device-2")

	if stream1 == nil || stream2 == nil {
		t.Fatal("Both devices should have active streams")
	}

	if stream1.ContainerID != "container-1" {
		t.Errorf("Expected container-1, got %s", stream1.ContainerID)
	}
	if stream2.ContainerID != "container-2" {
		t.Errorf("Expected container-2, got %s", stream2.ContainerID)
	}

	// Stop one device
	mgr.StopStream("device-1")

	// Only device-2 should have active stream
	if mgr.GetActiveStream("device-1") != nil {
		t.Error("device-1 should not have active stream")
	}
	if mgr.GetActiveStream("device-2") == nil {
		t.Error("device-2 should still have active stream")
	}
}

func TestLogStreamManager_StreamStartedAt(t *testing.T) {
	mgr := NewLogStreamManager()

	before := time.Now()
	_, cancel := mgr.StartStream("device-1", "container-abc")
	defer cancel()
	after := time.Now()

	stream := mgr.GetActiveStream("device-1")
	if stream == nil {
		t.Fatal("Expected stream to exist")
	}

	if stream.StartedAt.Before(before) || stream.StartedAt.After(after) {
		t.Errorf("StartedAt %v should be between %v and %v", stream.StartedAt, before, after)
	}
}
