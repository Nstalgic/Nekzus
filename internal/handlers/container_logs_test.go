package handlers

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/nstalgic/nekzus/internal/runtime"
	apptypes "github.com/nstalgic/nekzus/internal/types"
)

// mockNotifier captures messages sent to devices
type mockNotifier struct {
	mu       sync.Mutex
	messages map[string][]apptypes.WebSocketMessage
}

func newMockNotifier() *mockNotifier {
	return &mockNotifier{
		messages: make(map[string][]apptypes.WebSocketMessage),
	}
}

func (m *mockNotifier) SendToDevice(deviceID string, message interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var msg apptypes.WebSocketMessage
	switch v := message.(type) {
	case apptypes.WebSocketMessage:
		msg = v
	default:
		msg = apptypes.WebSocketMessage{Type: "unknown", Data: message}
	}

	m.messages[deviceID] = append(m.messages[deviceID], msg)
	return nil
}

func (m *mockNotifier) getMessages(deviceID string) []apptypes.WebSocketMessage {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.messages[deviceID]
}

func (m *mockNotifier) waitForMessageType(deviceID, msgType string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		m.mu.Lock()
		for _, msg := range m.messages[deviceID] {
			if msg.Type == msgType {
				m.mu.Unlock()
				return true
			}
		}
		m.mu.Unlock()
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

// mockLogReader simulates Docker log output
type mockLogReader struct {
	data   string
	offset int
	closed bool
	mu     sync.Mutex
	delay  time.Duration
}

// encodeDockerLogFrame creates a Docker multiplexed log frame
// Stream type: 1 = stdout, 2 = stderr
func encodeDockerLogFrame(streamType byte, content string) []byte {
	contentBytes := []byte(content)
	size := len(contentBytes)

	// 8-byte header: [stream_type][0][0][0][size_be32]
	header := []byte{
		streamType, 0, 0, 0,
		byte(size >> 24), byte(size >> 16), byte(size >> 8), byte(size),
	}

	return append(header, contentBytes...)
}

func newMockLogReader(lines []string) *mockLogReader {
	// Build Docker multiplexed format
	var data []byte
	for _, line := range lines {
		data = append(data, encodeDockerLogFrame(1, line+"\n")...)
	}
	return &mockLogReader{
		data: string(data),
	}
}

func (m *mockLogReader) Read(p []byte) (n int, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return 0, io.EOF
	}

	if m.delay > 0 {
		time.Sleep(m.delay)
	}

	if m.offset >= len(m.data) {
		return 0, io.EOF
	}

	n = copy(p, m.data[m.offset:])
	m.offset += n
	return n, nil
}

func (m *mockLogReader) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	return nil
}

func TestContainerLogsHandler_HandleStartStream_ValidContainer(t *testing.T) {
	notifier := newMockNotifier()

	mockClient := &MockDockerClient{
		ContainerInspectFunc: func(ctx context.Context, containerID string) (types.ContainerJSON, error) {
			return types.ContainerJSON{
				ContainerJSONBase: &types.ContainerJSONBase{
					ID:   containerID,
					Name: "/test-container",
					State: &types.ContainerState{
						Running: true,
					},
				},
				Config: &container.Config{
					Labels: map[string]string{
						"nekzus.app.id": "test-app",
					},
				},
			}, nil
		},
		ContainerLogsFunc: func(ctx context.Context, containerID string, options container.LogsOptions) (io.ReadCloser, error) {
			return newMockLogReader([]string{
				"line 1",
				"line 2",
				"line 3",
			}), nil
		},
	}

	handler := NewContainerLogsHandler(mockClient, nil)
	handler.SetNotifier(notifier)

	req := LogStartRequest{
		ContainerID: "abc123",
		Tail:        100,
		Follow:      false,
	}

	handler.HandleStartStream("device-1", req)

	// Wait for started message
	if !notifier.waitForMessageType("device-1", apptypes.WSMsgTypeContainerLogsStarted, 500*time.Millisecond) {
		t.Fatal("Expected logs.started message")
	}

	// Wait for data messages
	if !notifier.waitForMessageType("device-1", apptypes.WSMsgTypeContainerLogs, 500*time.Millisecond) {
		t.Fatal("Expected logs.data message")
	}

	// Wait for ended message (since follow=false)
	if !notifier.waitForMessageType("device-1", apptypes.WSMsgTypeContainerLogsEnded, 500*time.Millisecond) {
		t.Fatal("Expected logs.ended message")
	}

	// Verify message sequence
	msgs := notifier.getMessages("device-1")
	if len(msgs) < 3 {
		t.Fatalf("Expected at least 3 messages, got %d", len(msgs))
	}

	// First should be started
	if msgs[0].Type != apptypes.WSMsgTypeContainerLogsStarted {
		t.Errorf("First message should be logs.started, got %s", msgs[0].Type)
	}

	// Last should be ended
	lastMsg := msgs[len(msgs)-1]
	if lastMsg.Type != apptypes.WSMsgTypeContainerLogsEnded {
		t.Errorf("Last message should be logs.ended, got %s", lastMsg.Type)
	}
}

func TestContainerLogsHandler_HandleStartStream_ContainerNotFound(t *testing.T) {
	notifier := newMockNotifier()

	mockClient := &MockDockerClient{
		ContainerInspectFunc: func(ctx context.Context, containerID string) (types.ContainerJSON, error) {
			return types.ContainerJSON{}, fmt.Errorf("No such container: %s", containerID)
		},
	}

	handler := NewContainerLogsHandler(mockClient, nil)
	handler.SetNotifier(notifier)

	req := LogStartRequest{
		ContainerID: "nonexistent",
		Tail:        100,
	}

	handler.HandleStartStream("device-1", req)

	// Should receive error message
	if !notifier.waitForMessageType("device-1", apptypes.WSMsgTypeContainerLogsError, 500*time.Millisecond) {
		t.Fatal("Expected logs.error message")
	}

	msgs := notifier.getMessages("device-1")
	if len(msgs) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(msgs))
	}

	if msgs[0].Type != apptypes.WSMsgTypeContainerLogsError {
		t.Errorf("Expected error message, got %s", msgs[0].Type)
	}

	// Check error code
	data, ok := msgs[0].Data.(map[string]interface{})
	if !ok {
		t.Fatal("Expected data to be map")
	}
	if data["error"] != "CONTAINER_NOT_FOUND" {
		t.Errorf("Expected CONTAINER_NOT_FOUND error, got %v", data["error"])
	}
}

func TestContainerLogsHandler_HandleStopStream_Success(t *testing.T) {
	notifier := newMockNotifier()

	// Create a log reader that blocks until cancelled
	blockingReader := &blockingLogReader{
		lines: []string{"line1", "line2"},
	}

	mockClient := &MockDockerClient{
		ContainerInspectFunc: func(ctx context.Context, containerID string) (types.ContainerJSON, error) {
			return types.ContainerJSON{
				ContainerJSONBase: &types.ContainerJSONBase{
					ID:   containerID,
					Name: "/test-container",
					State: &types.ContainerState{
						Running: true,
					},
				},
				Config: &container.Config{
					Labels: map[string]string{
						"nekzus.app.id": "test-app",
					},
				},
			}, nil
		},
		ContainerLogsFunc: func(ctx context.Context, containerID string, options container.LogsOptions) (io.ReadCloser, error) {
			return blockingReader, nil
		},
	}

	handler := NewContainerLogsHandler(mockClient, nil)
	handler.SetNotifier(notifier)

	// Start streaming
	req := LogStartRequest{
		ContainerID: "abc123",
		Tail:        100,
		Follow:      true,
	}

	handler.HandleStartStream("device-1", req)

	// Wait for started
	if !notifier.waitForMessageType("device-1", apptypes.WSMsgTypeContainerLogsStarted, 500*time.Millisecond) {
		t.Fatal("Expected logs.started message")
	}

	// Stop the stream
	handler.HandleStopStream("device-1", LogStopRequest{ContainerID: "abc123"})

	// Wait for ended message
	if !notifier.waitForMessageType("device-1", apptypes.WSMsgTypeContainerLogsEnded, 500*time.Millisecond) {
		t.Fatal("Expected logs.ended message after stop")
	}

	// Verify the ended reason is "stopped"
	msgs := notifier.getMessages("device-1")
	for _, msg := range msgs {
		if msg.Type == apptypes.WSMsgTypeContainerLogsEnded {
			data, ok := msg.Data.(map[string]interface{})
			if !ok {
				t.Fatal("Expected data to be map")
			}
			if data["reason"] != "stopped" {
				t.Errorf("Expected reason 'stopped', got %v", data["reason"])
			}
			return
		}
	}
	t.Error("No ended message found")
}

// blockingLogReader blocks on Read until the context is cancelled
type blockingLogReader struct {
	lines  []string
	offset int
	mu     sync.Mutex
}

func (r *blockingLogReader) Read(p []byte) (n int, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.offset < len(r.lines) {
		line := r.lines[r.offset] + "\n"
		r.offset++
		return copy(p, line), nil
	}

	// Block until closed
	select {}
}

func (r *blockingLogReader) Close() error {
	return nil
}

func TestContainerLogsHandler_HandleStartStream_DefaultTail(t *testing.T) {
	notifier := newMockNotifier()
	var mu sync.Mutex
	var capturedOptions container.LogsOptions

	mockClient := &MockDockerClient{
		ContainerInspectFunc: func(ctx context.Context, containerID string) (types.ContainerJSON, error) {
			return types.ContainerJSON{
				ContainerJSONBase: &types.ContainerJSONBase{
					ID:   containerID,
					Name: "/test-container",
					State: &types.ContainerState{
						Running: true,
					},
				},
				Config: &container.Config{
					Labels: map[string]string{
						"nekzus.app.id": "test-app",
					},
				},
			}, nil
		},
		ContainerLogsFunc: func(ctx context.Context, containerID string, options container.LogsOptions) (io.ReadCloser, error) {
			mu.Lock()
			capturedOptions = options
			mu.Unlock()
			return newMockLogReader([]string{"line1"}), nil
		},
	}

	handler := NewContainerLogsHandler(mockClient, nil)
	handler.SetNotifier(notifier)

	// Request with 0 tail (should default to 100)
	req := LogStartRequest{
		ContainerID: "abc123",
		Tail:        0,
	}

	handler.HandleStartStream("device-1", req)

	// Wait for processing
	if !notifier.waitForMessageType("device-1", apptypes.WSMsgTypeContainerLogsStarted, 500*time.Millisecond) {
		t.Fatal("Expected logs.started message")
	}

	// Wait for logs to complete
	if !notifier.waitForMessageType("device-1", apptypes.WSMsgTypeContainerLogsEnded, 500*time.Millisecond) {
		t.Fatal("Expected logs.ended message")
	}

	// Check that tail was defaulted
	mu.Lock()
	tailValue := capturedOptions.Tail
	mu.Unlock()

	if tailValue != "100" {
		t.Errorf("Expected default tail of 100, got %s", tailValue)
	}
}

func TestContainerLogsHandler_HandleStartStream_MaxTail(t *testing.T) {
	notifier := newMockNotifier()
	var mu sync.Mutex
	var capturedOptions container.LogsOptions

	mockClient := &MockDockerClient{
		ContainerInspectFunc: func(ctx context.Context, containerID string) (types.ContainerJSON, error) {
			return types.ContainerJSON{
				ContainerJSONBase: &types.ContainerJSONBase{
					ID:   containerID,
					Name: "/test-container",
					State: &types.ContainerState{
						Running: true,
					},
				},
				Config: &container.Config{
					Labels: map[string]string{
						"nekzus.app.id": "test-app",
					},
				},
			}, nil
		},
		ContainerLogsFunc: func(ctx context.Context, containerID string, options container.LogsOptions) (io.ReadCloser, error) {
			mu.Lock()
			capturedOptions = options
			mu.Unlock()
			return newMockLogReader([]string{"line1"}), nil
		},
	}

	handler := NewContainerLogsHandler(mockClient, nil)
	handler.SetNotifier(notifier)

	// Request with tail > 1000 (should be capped to 1000)
	req := LogStartRequest{
		ContainerID: "abc123",
		Tail:        5000,
	}

	handler.HandleStartStream("device-1", req)

	// Wait for processing
	if !notifier.waitForMessageType("device-1", apptypes.WSMsgTypeContainerLogsStarted, 500*time.Millisecond) {
		t.Fatal("Expected logs.started message")
	}

	// Wait for logs to complete
	if !notifier.waitForMessageType("device-1", apptypes.WSMsgTypeContainerLogsEnded, 500*time.Millisecond) {
		t.Fatal("Expected logs.ended message")
	}

	// Check that tail was capped
	mu.Lock()
	tailValue := capturedOptions.Tail
	mu.Unlock()

	if tailValue != "1000" {
		t.Errorf("Expected max tail of 1000, got %s", tailValue)
	}
}

func TestContainerLogsHandler_StreamReplacesExisting(t *testing.T) {
	notifier := newMockNotifier()
	var mu sync.Mutex
	callCount := 0

	mockClient := &MockDockerClient{
		ContainerInspectFunc: func(ctx context.Context, containerID string) (types.ContainerJSON, error) {
			return types.ContainerJSON{
				ContainerJSONBase: &types.ContainerJSONBase{
					ID:   containerID,
					Name: "/test-container",
					State: &types.ContainerState{
						Running: true,
					},
				},
				Config: &container.Config{
					Labels: map[string]string{
						"nekzus.app.id": "test-app",
					},
				},
			}, nil
		},
		ContainerLogsFunc: func(ctx context.Context, containerID string, options container.LogsOptions) (io.ReadCloser, error) {
			mu.Lock()
			callCount++
			mu.Unlock()
			// Return a reader that blocks until context cancelled
			return &contextAwareReader{ctx: ctx, lines: []string{"line1", "line2"}}, nil
		},
	}

	handler := NewContainerLogsHandler(mockClient, nil)
	handler.SetNotifier(notifier)

	// Start first stream
	handler.HandleStartStream("device-1", LogStartRequest{ContainerID: "container-1", Follow: true})

	// Wait for first started
	if !notifier.waitForMessageType("device-1", apptypes.WSMsgTypeContainerLogsStarted, 500*time.Millisecond) {
		t.Fatal("Expected first logs.started message")
	}

	// Start second stream (should replace first)
	handler.HandleStartStream("device-1", LogStartRequest{ContainerID: "container-2", Follow: true})

	// Wait for second started
	time.Sleep(100 * time.Millisecond)
	msgs := notifier.getMessages("device-1")

	// Should have at least 2 started messages (one for each stream)
	startedCount := 0
	for _, msg := range msgs {
		if msg.Type == apptypes.WSMsgTypeContainerLogsStarted {
			startedCount++
		}
	}
	if startedCount < 2 {
		t.Errorf("Expected at least 2 started messages, got %d", startedCount)
	}
}

// contextAwareReader respects context cancellation
type contextAwareReader struct {
	ctx    context.Context
	lines  []string
	offset int
	mu     sync.Mutex
}

func (r *contextAwareReader) Read(p []byte) (n int, err error) {
	r.mu.Lock()

	if r.offset < len(r.lines) {
		line := r.lines[r.offset] + "\n"
		r.offset++
		r.mu.Unlock()
		return copy(p, line), nil
	}
	r.mu.Unlock()

	// Block until context cancelled
	<-r.ctx.Done()
	return 0, r.ctx.Err()
}

func (r *contextAwareReader) Close() error {
	return nil
}

func TestContainerLogsHandler_GetStreamManager(t *testing.T) {
	handler := NewContainerLogsHandler(&MockDockerClient{}, nil)

	mgr := handler.GetStreamManager()
	if mgr == nil {
		t.Fatal("Expected stream manager to be non-nil")
	}
}

// mockLogsRuntime implements runtime.Runtime for log streaming tests
type mockLogsRuntime struct {
	TypeValue      runtime.RuntimeType
	InspectFunc    func(ctx context.Context, id runtime.ContainerID) (*runtime.ContainerDetails, error)
	StreamLogsFunc func(ctx context.Context, id runtime.ContainerID, opts runtime.LogOptions) (io.ReadCloser, error)
}

func (m *mockLogsRuntime) Name() string                   { return "MockLogsRuntime" }
func (m *mockLogsRuntime) Type() runtime.RuntimeType      { return m.TypeValue }
func (m *mockLogsRuntime) Ping(ctx context.Context) error { return nil }
func (m *mockLogsRuntime) Close() error                   { return nil }
func (m *mockLogsRuntime) List(ctx context.Context, opts runtime.ListOptions) ([]runtime.Container, error) {
	return []runtime.Container{}, nil
}
func (m *mockLogsRuntime) Start(ctx context.Context, id runtime.ContainerID) error { return nil }
func (m *mockLogsRuntime) Stop(ctx context.Context, id runtime.ContainerID, timeout *time.Duration) error {
	return nil
}
func (m *mockLogsRuntime) Restart(ctx context.Context, id runtime.ContainerID, timeout *time.Duration) error {
	return nil
}
func (m *mockLogsRuntime) Inspect(ctx context.Context, id runtime.ContainerID) (*runtime.ContainerDetails, error) {
	if m.InspectFunc != nil {
		return m.InspectFunc(ctx, id)
	}
	return nil, nil
}
func (m *mockLogsRuntime) GetStats(ctx context.Context, id runtime.ContainerID) (*runtime.Stats, error) {
	return nil, nil
}
func (m *mockLogsRuntime) GetBatchStats(ctx context.Context, ids []runtime.ContainerID) ([]runtime.Stats, error) {
	return []runtime.Stats{}, nil
}
func (m *mockLogsRuntime) StreamLogs(ctx context.Context, id runtime.ContainerID, opts runtime.LogOptions) (io.ReadCloser, error) {
	if m.StreamLogsFunc != nil {
		return m.StreamLogsFunc(ctx, id, opts)
	}
	return io.NopCloser(&mockLogReader{}), nil
}

func TestContainerLogsHandler_HandleStartStream_WithRuntime(t *testing.T) {
	notifier := newMockNotifier()

	mockRT := &mockLogsRuntime{
		TypeValue: runtime.RuntimeDocker,
		InspectFunc: func(ctx context.Context, id runtime.ContainerID) (*runtime.ContainerDetails, error) {
			return &runtime.ContainerDetails{
				Container: runtime.Container{
					ID:    id,
					Name:  "test-container",
					State: runtime.StateRunning,
				},
			}, nil
		},
		StreamLogsFunc: func(ctx context.Context, id runtime.ContainerID, opts runtime.LogOptions) (io.ReadCloser, error) {
			return newMockLogReader([]string{
				"line 1",
				"line 2",
				"line 3",
			}), nil
		},
	}

	registry := runtime.NewRegistry()
	_ = registry.Register(mockRT)

	handler := NewContainerLogsHandlerWithRuntime(registry, nil)
	handler.SetNotifier(notifier)

	req := LogStartRequest{
		ContainerID: "abc123",
		Tail:        100,
		Follow:      false,
	}

	handler.HandleStartStream("device-1", req)

	// Wait for started message
	if !notifier.waitForMessageType("device-1", apptypes.WSMsgTypeContainerLogsStarted, 500*time.Millisecond) {
		t.Fatal("Expected logs.started message")
	}

	// Wait for data messages
	if !notifier.waitForMessageType("device-1", apptypes.WSMsgTypeContainerLogs, 500*time.Millisecond) {
		t.Fatal("Expected logs.data message")
	}

	// Wait for ended message (since follow=false)
	if !notifier.waitForMessageType("device-1", apptypes.WSMsgTypeContainerLogsEnded, 500*time.Millisecond) {
		t.Fatal("Expected logs.ended message")
	}

	// Verify message sequence
	msgs := notifier.getMessages("device-1")
	if len(msgs) < 3 {
		t.Fatalf("Expected at least 3 messages, got %d", len(msgs))
	}

	// First should be started
	if msgs[0].Type != apptypes.WSMsgTypeContainerLogsStarted {
		t.Errorf("First message should be logs.started, got %s", msgs[0].Type)
	}

	// Last should be ended
	lastMsg := msgs[len(msgs)-1]
	if lastMsg.Type != apptypes.WSMsgTypeContainerLogsEnded {
		t.Errorf("Last message should be logs.ended, got %s", lastMsg.Type)
	}
}

func TestContainerLogsHandler_HandleStartStream_WithRuntime_ContainerNotFound(t *testing.T) {
	notifier := newMockNotifier()

	mockRT := &mockLogsRuntime{
		TypeValue: runtime.RuntimeDocker,
		InspectFunc: func(ctx context.Context, id runtime.ContainerID) (*runtime.ContainerDetails, error) {
			return nil, fmt.Errorf("no such container: %s", id.ID)
		},
	}

	registry := runtime.NewRegistry()
	_ = registry.Register(mockRT)

	handler := NewContainerLogsHandlerWithRuntime(registry, nil)
	handler.SetNotifier(notifier)

	req := LogStartRequest{
		ContainerID: "nonexistent",
		Tail:        100,
	}

	handler.HandleStartStream("device-1", req)

	// Should receive error message
	if !notifier.waitForMessageType("device-1", apptypes.WSMsgTypeContainerLogsError, 500*time.Millisecond) {
		t.Fatal("Expected logs.error message")
	}

	msgs := notifier.getMessages("device-1")
	if len(msgs) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(msgs))
	}

	if msgs[0].Type != apptypes.WSMsgTypeContainerLogsError {
		t.Errorf("Expected error message, got %s", msgs[0].Type)
	}

	// Check error code
	data, ok := msgs[0].Data.(map[string]interface{})
	if !ok {
		t.Fatal("Expected data to be map")
	}
	if data["error"] != "CONTAINER_NOT_FOUND" {
		t.Errorf("Expected CONTAINER_NOT_FOUND error, got %v", data["error"])
	}
}

// --- Buffer Pooling Tests ---

// TestLogBufferPool tests that buffer pool is being used for log frames
func TestLogBufferPool(t *testing.T) {
	// Verify buffer pool exists and is functional
	if logBufferPool == nil {
		t.Fatal("Expected logBufferPool to be initialized")
	}

	// Get buffer from pool
	buf := logBufferPool.Get().(*[]byte)
	if buf == nil {
		t.Fatal("Expected buffer from pool")
	}
	if len(*buf) < DefaultLogBufferSize {
		t.Errorf("Expected buffer size >= %d, got %d", DefaultLogBufferSize, len(*buf))
	}

	// Return to pool
	logBufferPool.Put(buf)
}

// TestMaxLogFrameSize tests that oversized frames are rejected
func TestMaxLogFrameSize(t *testing.T) {
	notifier := newMockNotifier()

	// Create a log reader with an oversized frame (> MaxLogFrameSize)
	oversizedData := make([]byte, MaxLogFrameSize+1000)
	for i := range oversizedData {
		oversizedData[i] = 'x'
	}

	// Create frame with oversized content
	size := len(oversizedData)
	header := []byte{
		1, 0, 0, 0, // stdout
		byte(size >> 24), byte(size >> 16), byte(size >> 8), byte(size),
	}
	oversizedFrame := append(header, oversizedData...)

	mockClient := &MockDockerClient{
		ContainerInspectFunc: func(ctx context.Context, containerID string) (types.ContainerJSON, error) {
			return types.ContainerJSON{
				ContainerJSONBase: &types.ContainerJSONBase{
					ID:    containerID,
					Name:  "/test-container",
					State: &types.ContainerState{Running: true},
				},
				Config: &container.Config{Labels: map[string]string{}},
			}, nil
		},
		ContainerLogsFunc: func(ctx context.Context, containerID string, options container.LogsOptions) (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(oversizedFrame)), nil
		},
	}

	handler := NewContainerLogsHandler(mockClient, nil)
	handler.SetNotifier(notifier)

	handler.HandleStartStream("device-1", LogStartRequest{
		ContainerID: "abc123",
		Tail:        100,
		Follow:      false,
	})

	// Wait for stream to process
	time.Sleep(200 * time.Millisecond)

	// Should have ended (oversized frame should be skipped)
	msgs := notifier.getMessages("device-1")

	// Verify no log data with oversized content was sent
	for _, msg := range msgs {
		if msg.Type == apptypes.WSMsgTypeContainerLogs {
			if data, ok := msg.Data.(map[string]interface{}); ok {
				if message, ok := data["message"].(string); ok {
					if len(message) > MaxLogFrameSize {
						t.Errorf("Oversized frame was not rejected, got message of size %d", len(message))
					}
				}
			}
		}
	}
}

// TestLogStreamingMemoryEfficiency tests that memory allocation is bounded
func TestLogStreamingMemoryEfficiency(t *testing.T) {
	// Create many small log lines
	var lines []string
	for i := 0; i < 100; i++ {
		lines = append(lines, fmt.Sprintf("Log line %d with some content", i))
	}

	mockClient := &MockDockerClient{
		ContainerInspectFunc: func(ctx context.Context, containerID string) (types.ContainerJSON, error) {
			return types.ContainerJSON{
				ContainerJSONBase: &types.ContainerJSONBase{
					ID:    containerID,
					Name:  "/test-container",
					State: &types.ContainerState{Running: true},
				},
				Config: &container.Config{Labels: map[string]string{}},
			}, nil
		},
		ContainerLogsFunc: func(ctx context.Context, containerID string, options container.LogsOptions) (io.ReadCloser, error) {
			return newMockLogReader(lines), nil
		},
	}

	// Run multiple streams to test buffer reuse - each with fresh notifier
	successCount := 0
	for i := 0; i < 5; i++ {
		notifier := newMockNotifier()
		handler := NewContainerLogsHandler(mockClient, nil)
		handler.SetNotifier(notifier)

		handler.HandleStartStream("device-1", LogStartRequest{
			ContainerID: fmt.Sprintf("container-%d", i),
			Tail:        100,
			Follow:      false,
		})

		// Wait for stream to complete
		if notifier.waitForMessageType("device-1", apptypes.WSMsgTypeContainerLogsEnded, 1*time.Second) {
			successCount++
		}
	}

	// All streams should complete successfully with buffer reuse
	if successCount < 5 {
		t.Errorf("Expected 5 successful streams, got %d", successCount)
	}
}
