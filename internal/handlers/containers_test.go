package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/nstalgic/nekzus/internal/runtime"
	"github.com/nstalgic/nekzus/internal/storage"
	nekzustypes "github.com/nstalgic/nekzus/internal/types"
)

// MockDockerClient implements a mock Docker client for testing
type MockDockerClient struct {
	ContainerListFunc    func(ctx context.Context, options container.ListOptions) ([]types.Container, error)
	ContainerStartFunc   func(ctx context.Context, containerID string, options container.StartOptions) error
	ContainerStopFunc    func(ctx context.Context, containerID string, options container.StopOptions) error
	ContainerRestartFunc func(ctx context.Context, containerID string, options container.StopOptions) error
	ContainerInspectFunc func(ctx context.Context, containerID string) (types.ContainerJSON, error)
	ContainerStatsFunc   func(ctx context.Context, containerID string, stream bool) (container.StatsResponseReader, error)
	ContainerLogsFunc    func(ctx context.Context, containerID string, options container.LogsOptions) (io.ReadCloser, error)
}

func (m *MockDockerClient) ContainerList(ctx context.Context, options container.ListOptions) ([]types.Container, error) {
	if m.ContainerListFunc != nil {
		return m.ContainerListFunc(ctx, options)
	}
	return []types.Container{}, nil
}

func (m *MockDockerClient) ContainerStart(ctx context.Context, containerID string, options container.StartOptions) error {
	if m.ContainerStartFunc != nil {
		return m.ContainerStartFunc(ctx, containerID, options)
	}
	return nil
}

func (m *MockDockerClient) ContainerStop(ctx context.Context, containerID string, options container.StopOptions) error {
	if m.ContainerStopFunc != nil {
		return m.ContainerStopFunc(ctx, containerID, options)
	}
	return nil
}

func (m *MockDockerClient) ContainerRestart(ctx context.Context, containerID string, options container.StopOptions) error {
	if m.ContainerRestartFunc != nil {
		return m.ContainerRestartFunc(ctx, containerID, options)
	}
	return nil
}

func (m *MockDockerClient) ContainerInspect(ctx context.Context, containerID string) (types.ContainerJSON, error) {
	if m.ContainerInspectFunc != nil {
		return m.ContainerInspectFunc(ctx, containerID)
	}
	return types.ContainerJSON{}, nil
}

func (m *MockDockerClient) ContainerStats(ctx context.Context, containerID string, stream bool) (container.StatsResponseReader, error) {
	if m.ContainerStatsFunc != nil {
		return m.ContainerStatsFunc(ctx, containerID, stream)
	}
	return container.StatsResponseReader{}, nil
}

func (m *MockDockerClient) ContainerLogs(ctx context.Context, containerID string, options container.LogsOptions) (io.ReadCloser, error) {
	if m.ContainerLogsFunc != nil {
		return m.ContainerLogsFunc(ctx, containerID, options)
	}
	return io.NopCloser(strings.NewReader("")), nil
}

// MockRuntime implements runtime.Runtime for testing
type MockRuntime struct {
	NameValue         string
	TypeValue         runtime.RuntimeType
	PingErr           error
	CloseErr          error
	ListFunc          func(ctx context.Context, opts runtime.ListOptions) ([]runtime.Container, error)
	StartFunc         func(ctx context.Context, id runtime.ContainerID) error
	StopFunc          func(ctx context.Context, id runtime.ContainerID, timeout *time.Duration) error
	RestartFunc       func(ctx context.Context, id runtime.ContainerID, timeout *time.Duration) error
	InspectFunc       func(ctx context.Context, id runtime.ContainerID) (*runtime.ContainerDetails, error)
	GetStatsFunc      func(ctx context.Context, id runtime.ContainerID) (*runtime.Stats, error)
	GetBatchStatsFunc func(ctx context.Context, ids []runtime.ContainerID) ([]runtime.Stats, error)
	StreamLogsFunc    func(ctx context.Context, id runtime.ContainerID, opts runtime.LogOptions) (io.ReadCloser, error)
}

func (m *MockRuntime) Name() string {
	if m.NameValue != "" {
		return m.NameValue
	}
	return "MockRuntime"
}

func (m *MockRuntime) Type() runtime.RuntimeType {
	if m.TypeValue != "" {
		return m.TypeValue
	}
	return runtime.RuntimeDocker
}

func (m *MockRuntime) Ping(ctx context.Context) error { return m.PingErr }
func (m *MockRuntime) Close() error                   { return m.CloseErr }

func (m *MockRuntime) List(ctx context.Context, opts runtime.ListOptions) ([]runtime.Container, error) {
	if m.ListFunc != nil {
		return m.ListFunc(ctx, opts)
	}
	return []runtime.Container{}, nil
}

func (m *MockRuntime) Start(ctx context.Context, id runtime.ContainerID) error {
	if m.StartFunc != nil {
		return m.StartFunc(ctx, id)
	}
	return nil
}

func (m *MockRuntime) Stop(ctx context.Context, id runtime.ContainerID, timeout *time.Duration) error {
	if m.StopFunc != nil {
		return m.StopFunc(ctx, id, timeout)
	}
	return nil
}

func (m *MockRuntime) Restart(ctx context.Context, id runtime.ContainerID, timeout *time.Duration) error {
	if m.RestartFunc != nil {
		return m.RestartFunc(ctx, id, timeout)
	}
	return nil
}

func (m *MockRuntime) Inspect(ctx context.Context, id runtime.ContainerID) (*runtime.ContainerDetails, error) {
	if m.InspectFunc != nil {
		return m.InspectFunc(ctx, id)
	}
	return nil, nil
}

func (m *MockRuntime) GetStats(ctx context.Context, id runtime.ContainerID) (*runtime.Stats, error) {
	if m.GetStatsFunc != nil {
		return m.GetStatsFunc(ctx, id)
	}
	return nil, nil
}

func (m *MockRuntime) GetBatchStats(ctx context.Context, ids []runtime.ContainerID) ([]runtime.Stats, error) {
	if m.GetBatchStatsFunc != nil {
		return m.GetBatchStatsFunc(ctx, ids)
	}
	return []runtime.Stats{}, nil
}

func (m *MockRuntime) StreamLogs(ctx context.Context, id runtime.ContainerID, opts runtime.LogOptions) (io.ReadCloser, error) {
	if m.StreamLogsFunc != nil {
		return m.StreamLogsFunc(ctx, id, opts)
	}
	return io.NopCloser(strings.NewReader("")), nil
}

// createMockRegistry creates a runtime registry with the given mock runtime
func createMockRegistry(rt runtime.Runtime) *runtime.Registry {
	reg := runtime.NewRegistry()
	if rt != nil {
		_ = reg.Register(rt)
	}
	return reg
}

// MockHealthNotifier implements ServiceHealthNotifier for testing
type MockHealthNotifier struct {
	mu    sync.Mutex
	calls []struct {
		AppID  string
		Reason string
	}
}

func (m *MockHealthNotifier) MarkAppUnhealthy(appID, reason string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, struct {
		AppID  string
		Reason string
	}{AppID: appID, Reason: reason})
}

func (m *MockHealthNotifier) GetCalls() []struct {
	AppID  string
	Reason string
} {
	m.mu.Lock()
	defer m.mu.Unlock()
	// Return a copy to avoid race conditions
	result := make([]struct {
		AppID  string
		Reason string
	}, len(m.calls))
	copy(result, m.calls)
	return result
}

// TestContainerHandler_HandleStartContainer tests starting a container
// Note: Start/Stop/Restart handlers now return 202 Accepted immediately and run async.
// Docker errors are reported via WebSocket notification, not HTTP response.
func TestContainerHandler_HandleStartContainer(t *testing.T) {
	tests := []struct {
		name           string
		method         string
		containerID    string
		mockStartFunc  func(ctx context.Context, containerID string, options container.StartOptions) error
		expectedStatus int
		expectedErr    bool
	}{
		{
			name:        "accepts start request",
			method:      http.MethodPost,
			containerID: "test-container-123",
			mockStartFunc: func(ctx context.Context, containerID string, options container.StartOptions) error {
				return nil
			},
			expectedStatus: http.StatusAccepted,
			expectedErr:    false,
		},
		{
			name:        "accepts start request even for nonexistent container",
			method:      http.MethodPost,
			containerID: "nonexistent",
			mockStartFunc: func(ctx context.Context, containerID string, options container.StartOptions) error {
				return fmt.Errorf("No such container: %s", containerID)
			},
			expectedStatus: http.StatusAccepted, // Error will be sent via WebSocket
			expectedErr:    false,
		},
		{
			name:           "method not allowed",
			method:         http.MethodGet,
			containerID:    "test-container",
			expectedStatus: http.StatusMethodNotAllowed,
			expectedErr:    true,
		},
		{
			name:           "empty container ID",
			method:         http.MethodPost,
			containerID:    "",
			expectedStatus: http.StatusBadRequest,
			expectedErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock Docker client
			mockClient := &MockDockerClient{
				ContainerStartFunc: tt.mockStartFunc,
			}

			// Create handler
			handler := NewContainerHandler(mockClient, nil)

			// Create request
			url := fmt.Sprintf("/api/v1/containers/%s/start", tt.containerID)
			req := httptest.NewRequest(tt.method, url, nil)
			w := httptest.NewRecorder()

			// Execute handler
			handler.HandleStartContainer(w, req)

			// Check status code
			if w.Code != tt.expectedStatus {
				t.Errorf("HandleStartContainer() status = %d, want %d", w.Code, tt.expectedStatus)
			}

			// For accepted cases, verify response structure
			if !tt.expectedErr && w.Code == http.StatusAccepted {
				var response map[string]interface{}
				if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
					t.Errorf("Failed to decode response: %v", err)
				}
				if response["status"] != "accepted" {
					t.Errorf("Expected status 'accepted', got %v", response["status"])
				}
				if response["containerId"] != tt.containerID {
					t.Errorf("Expected containerId %s, got %v", tt.containerID, response["containerId"])
				}
			}
		})
	}
}

// TestContainerHandler_HandleStopContainer tests stopping a container
// Note: Stop handler now returns 202 Accepted immediately and runs async.
// Docker errors are reported via WebSocket notification, not HTTP response.
func TestContainerHandler_HandleStopContainer(t *testing.T) {
	tests := []struct {
		name           string
		method         string
		containerID    string
		mockStopFunc   func(ctx context.Context, containerID string, options container.StopOptions) error
		expectedStatus int
		expectedErr    bool
	}{
		{
			name:        "accepts stop request",
			method:      http.MethodPost,
			containerID: "running-container",
			mockStopFunc: func(ctx context.Context, containerID string, options container.StopOptions) error {
				return nil
			},
			expectedStatus: http.StatusAccepted,
			expectedErr:    false,
		},
		{
			name:        "accepts stop request even for nonexistent container",
			method:      http.MethodPost,
			containerID: "nonexistent",
			mockStopFunc: func(ctx context.Context, containerID string, options container.StopOptions) error {
				return fmt.Errorf("No such container: %s", containerID)
			},
			expectedStatus: http.StatusAccepted, // Error will be sent via WebSocket
			expectedErr:    false,
		},
		{
			name:           "method not allowed",
			method:         http.MethodGet,
			containerID:    "test-container",
			expectedStatus: http.StatusMethodNotAllowed,
			expectedErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := &MockDockerClient{
				ContainerStopFunc: tt.mockStopFunc,
			}

			handler := NewContainerHandler(mockClient, nil)

			url := fmt.Sprintf("/api/v1/containers/%s/stop", tt.containerID)
			req := httptest.NewRequest(tt.method, url, nil)
			w := httptest.NewRecorder()

			handler.HandleStopContainer(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("HandleStopContainer() status = %d, want %d", w.Code, tt.expectedStatus)
			}

			if !tt.expectedErr && w.Code == http.StatusAccepted {
				var response map[string]interface{}
				if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
					t.Errorf("Failed to decode response: %v", err)
				}
				if response["status"] != "accepted" {
					t.Errorf("Expected status 'accepted', got %v", response["status"])
				}
			}
		})
	}
}

// TestContainerHandler_StopNotifiesHealthChecker tests that stopping a container
// triggers an immediate health notification when healthNotifier is set
func TestContainerHandler_StopNotifiesHealthChecker(t *testing.T) {
	testAppID := "test-app-id"

	// Create mock runtime that returns container with app ID label
	mockRuntime := &MockRuntime{
		TypeValue: runtime.RuntimeDocker,
		InspectFunc: func(ctx context.Context, id runtime.ContainerID) (*runtime.ContainerDetails, error) {
			return &runtime.ContainerDetails{
				Container: runtime.Container{
					ID:     id,
					Name:   "test-container",
					Labels: map[string]string{"nekzus.app.id": testAppID},
				},
			}, nil
		},
		StopFunc: func(ctx context.Context, id runtime.ContainerID, timeout *time.Duration) error {
			return nil
		},
	}

	// Create handler with runtime registry
	registry := createMockRegistry(mockRuntime)
	handler := NewContainerHandlerWithRuntime(registry, nil)

	// Set health notifier
	healthNotifier := &MockHealthNotifier{}
	handler.SetHealthNotifier(healthNotifier)

	// Make request
	url := "/api/v1/containers/test-container-123/stop"
	req := httptest.NewRequest(http.MethodPost, url, nil)
	w := httptest.NewRecorder()

	handler.HandleStopContainer(w, req)

	// Should return 202 Accepted
	if w.Code != http.StatusAccepted {
		t.Errorf("HandleStopContainer() status = %d, want %d", w.Code, http.StatusAccepted)
	}

	// Wait a bit for the async goroutine to complete
	time.Sleep(100 * time.Millisecond)

	// Check that health notifier was called
	calls := healthNotifier.GetCalls()
	if len(calls) != 1 {
		t.Errorf("Expected 1 health notification, got %d", len(calls))
		return
	}

	if calls[0].AppID != testAppID {
		t.Errorf("Expected app ID %q, got %q", testAppID, calls[0].AppID)
	}

	if calls[0].Reason != "Container stopped" {
		t.Errorf("Expected reason 'Container stopped', got %q", calls[0].Reason)
	}
}

// TestContainerHandler_StopNoHealthNotificationWithoutAppID tests that no health
// notification is sent when container doesn't have an app ID label
func TestContainerHandler_StopNoHealthNotificationWithoutAppID(t *testing.T) {
	// Create mock runtime that returns container without app ID label
	mockRuntime := &MockRuntime{
		TypeValue: runtime.RuntimeDocker,
		InspectFunc: func(ctx context.Context, id runtime.ContainerID) (*runtime.ContainerDetails, error) {
			return &runtime.ContainerDetails{
				Container: runtime.Container{
					ID:     id,
					Name:   "test-container",
					Labels: map[string]string{}, // No app ID label
				},
			}, nil
		},
		StopFunc: func(ctx context.Context, id runtime.ContainerID, timeout *time.Duration) error {
			return nil
		},
	}

	// Create handler with runtime registry
	registry := createMockRegistry(mockRuntime)
	handler := NewContainerHandlerWithRuntime(registry, nil)

	// Set health notifier
	healthNotifier := &MockHealthNotifier{}
	handler.SetHealthNotifier(healthNotifier)

	// Make request
	url := "/api/v1/containers/test-container-123/stop"
	req := httptest.NewRequest(http.MethodPost, url, nil)
	w := httptest.NewRecorder()

	handler.HandleStopContainer(w, req)

	// Should return 202 Accepted
	if w.Code != http.StatusAccepted {
		t.Errorf("HandleStopContainer() status = %d, want %d", w.Code, http.StatusAccepted)
	}

	// Wait a bit for the async goroutine to complete
	time.Sleep(100 * time.Millisecond)

	// Check that health notifier was NOT called (no app ID)
	calls := healthNotifier.GetCalls()
	if len(calls) != 0 {
		t.Errorf("Expected 0 health notifications (no app ID), got %d", len(calls))
	}
}

// TestContainerHandler_HandleRestartContainer tests restarting a container
// Note: Restart handler now returns 202 Accepted immediately and runs async.
// Docker errors are reported via WebSocket notification, not HTTP response.
func TestContainerHandler_HandleRestartContainer(t *testing.T) {
	tests := []struct {
		name            string
		method          string
		containerID     string
		mockRestartFunc func(ctx context.Context, containerID string, options container.StopOptions) error
		expectedStatus  int
		expectedErr     bool
	}{
		{
			name:        "accepts restart request",
			method:      http.MethodPost,
			containerID: "test-container",
			mockRestartFunc: func(ctx context.Context, containerID string, options container.StopOptions) error {
				return nil
			},
			expectedStatus: http.StatusAccepted,
			expectedErr:    false,
		},
		{
			name:        "accepts restart request even for nonexistent container",
			method:      http.MethodPost,
			containerID: "nonexistent",
			mockRestartFunc: func(ctx context.Context, containerID string, options container.StopOptions) error {
				return fmt.Errorf("No such container: %s", containerID)
			},
			expectedStatus: http.StatusAccepted, // Error will be sent via WebSocket
			expectedErr:    false,
		},
		{
			name:           "method not allowed",
			method:         http.MethodGet,
			containerID:    "test-container",
			expectedStatus: http.StatusMethodNotAllowed,
			expectedErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := &MockDockerClient{
				ContainerRestartFunc: tt.mockRestartFunc,
			}

			handler := NewContainerHandler(mockClient, nil)

			url := fmt.Sprintf("/api/v1/containers/%s/restart", tt.containerID)
			req := httptest.NewRequest(tt.method, url, nil)
			w := httptest.NewRecorder()

			handler.HandleRestartContainer(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("HandleRestartContainer() status = %d, want %d", w.Code, tt.expectedStatus)
			}

			if !tt.expectedErr && w.Code == http.StatusAccepted {
				var response map[string]interface{}
				if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
					t.Errorf("Failed to decode response: %v", err)
				}
				if response["status"] != "accepted" {
					t.Errorf("Expected status 'accepted', got %v", response["status"])
				}
			}
		})
	}
}

// TestContainerHandler_HandleInspectContainer tests inspecting a container
func TestContainerHandler_HandleInspectContainer(t *testing.T) {
	tests := []struct {
		name           string
		method         string
		containerID    string
		mockRuntime    *MockRuntime
		expectedStatus int
		expectedErr    bool
	}{
		{
			name:        "successful inspect",
			method:      http.MethodGet,
			containerID: "test-container",
			mockRuntime: &MockRuntime{
				TypeValue: runtime.RuntimeDocker,
				InspectFunc: func(ctx context.Context, id runtime.ContainerID) (*runtime.ContainerDetails, error) {
					return &runtime.ContainerDetails{
						Container: runtime.Container{
							ID:    id,
							Name:  "test-container",
							Image: "nginx:latest",
							State: runtime.StateRunning,
						},
						Raw: types.ContainerJSON{
							ContainerJSONBase: &types.ContainerJSONBase{
								ID:   id.ID,
								Name: "/test-container",
								State: &types.ContainerState{
									Running: true,
									Status:  "running",
								},
							},
							Config: &container.Config{
								Image: "nginx:latest",
							},
						},
					}, nil
				},
			},
			expectedStatus: http.StatusOK,
			expectedErr:    false,
		},
		{
			name:        "container not found",
			method:      http.MethodGet,
			containerID: "nonexistent",
			mockRuntime: &MockRuntime{
				TypeValue: runtime.RuntimeDocker,
				InspectFunc: func(ctx context.Context, id runtime.ContainerID) (*runtime.ContainerDetails, error) {
					return nil, runtime.NewContainerError(runtime.RuntimeDocker, "inspect", id, runtime.ErrContainerNotFound)
				},
			},
			expectedStatus: http.StatusNotFound,
			expectedErr:    true,
		},
		{
			name:           "method not allowed",
			method:         http.MethodPost,
			containerID:    "test-container",
			mockRuntime:    &MockRuntime{TypeValue: runtime.RuntimeDocker},
			expectedStatus: http.StatusMethodNotAllowed,
			expectedErr:    true,
		},
		{
			name:           "empty container ID",
			method:         http.MethodGet,
			containerID:    "",
			mockRuntime:    &MockRuntime{TypeValue: runtime.RuntimeDocker},
			expectedStatus: http.StatusBadRequest,
			expectedErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registry := createMockRegistry(tt.mockRuntime)
			handler := NewContainerHandlerWithRuntime(registry, nil)

			url := fmt.Sprintf("/api/v1/containers/%s", tt.containerID)
			req := httptest.NewRequest(tt.method, url, nil)
			w := httptest.NewRecorder()

			handler.HandleInspectContainer(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("HandleInspectContainer() status = %d, want %d", w.Code, tt.expectedStatus)
			}

			if !tt.expectedErr && w.Code == http.StatusOK {
				var response types.ContainerJSON
				if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
					t.Errorf("Failed to decode response: %v", err)
				}
				if response.ID != tt.containerID {
					t.Errorf("Expected container ID %s, got %s", tt.containerID, response.ID)
				}
			}
		})
	}
}

// TestContainerHandler_HandleInspectContainer_ResponseStructure tests the structure of inspect response
func TestContainerHandler_HandleInspectContainer_ResponseStructure(t *testing.T) {
	mockRuntime := &MockRuntime{
		TypeValue: runtime.RuntimeDocker,
		InspectFunc: func(ctx context.Context, id runtime.ContainerID) (*runtime.ContainerDetails, error) {
			return &runtime.ContainerDetails{
				Container: runtime.Container{
					ID:    id,
					Name:  "grafana",
					Image: "grafana/grafana:latest",
					State: runtime.StateRunning,
					Labels: map[string]string{
						"nekzus.enable":   "true",
						"nekzus.app.id":   "grafana",
						"nekzus.app.name": "Grafana",
					},
				},
				Raw: types.ContainerJSON{
					ContainerJSONBase: &types.ContainerJSONBase{
						ID:   "abc123",
						Name: "/grafana",
						State: &types.ContainerState{
							Running:   true,
							Status:    "running",
							StartedAt: "2025-01-01T00:00:00.000000000Z",
						},
					},
					Config: &container.Config{
						Image: "grafana/grafana:latest",
						Env: []string{
							"GF_SECURITY_ADMIN_PASSWORD=admin",
							"GF_INSTALL_PLUGINS=grafana-clock-panel",
						},
						Labels: map[string]string{
							"nekzus.enable":   "true",
							"nekzus.app.id":   "grafana",
							"nekzus.app.name": "Grafana",
						},
					},
				},
			}, nil
		},
	}

	registry := createMockRegistry(mockRuntime)
	handler := NewContainerHandlerWithRuntime(registry, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/containers/abc123", nil)
	w := httptest.NewRecorder()

	handler.HandleInspectContainer(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", w.Code)
	}

	var response types.ContainerJSON
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// Verify response structure
	if response.ID != "abc123" {
		t.Errorf("Expected ID 'abc123', got %s", response.ID)
	}
	if response.Name != "/grafana" {
		t.Errorf("Expected name '/grafana', got %s", response.Name)
	}
	if response.State.Running != true {
		t.Errorf("Expected running state true, got %v", response.State.Running)
	}
	if response.Config.Image != "grafana/grafana:latest" {
		t.Errorf("Expected image 'grafana/grafana:latest', got %s", response.Config.Image)
	}
	if len(response.Config.Env) != 2 {
		t.Errorf("Expected 2 env vars, got %d", len(response.Config.Env))
	}
	if len(response.Config.Labels) != 3 {
		t.Errorf("Expected 3 labels, got %d", len(response.Config.Labels))
	}
}

// mockStatsReader implements io.ReadCloser for simulating stats stream
type mockStatsReader struct {
	data  [][]byte // Multiple stats samples
	index int
	delay time.Duration
}

func (m *mockStatsReader) Read(p []byte) (n int, err error) {
	if m.index >= len(m.data) {
		return 0, io.EOF
	}

	// Simulate delay between samples
	if m.index > 0 && m.delay > 0 {
		time.Sleep(m.delay)
	}

	sample := m.data[m.index]
	m.index++

	copy(p, sample)
	return len(sample), nil
}

func (m *mockStatsReader) Close() error {
	return nil
}

// TestContainerHandler_HandleContainerStats tests container stats with runtime abstraction
func TestContainerHandler_HandleContainerStats(t *testing.T) {
	tests := []struct {
		name           string
		method         string
		containerID    string
		mockRuntime    *MockRuntime
		expectedStatus int
		expectedErr    bool
		validateCPU    bool
		minCPUPercent  float64
	}{
		{
			name:        "successful stats with CPU calculation",
			method:      http.MethodGet,
			containerID: "test-container",
			mockRuntime: &MockRuntime{
				TypeValue: runtime.RuntimeDocker,
				GetStatsFunc: func(ctx context.Context, id runtime.ContainerID) (*runtime.Stats, error) {
					return &runtime.Stats{
						ContainerID: id,
						CPU: runtime.CPUStats{
							Usage:      40.0, // 40% CPU
							CoresUsed:  1.6,
							TotalCores: 4.0,
						},
						Memory: runtime.MemoryStats{
							Usage:     19.53, // ~20% memory
							Used:      104857600,
							Limit:     536870912,
							Available: 432013312,
						},
						Network: runtime.NetworkStats{
							RxBytes: 2048000,
							TxBytes: 4096000,
						},
						Timestamp: time.Now().Unix(),
					}, nil
				},
			},
			expectedStatus: http.StatusOK,
			expectedErr:    false,
			validateCPU:    true,
			minCPUPercent:  5.0,
		},
		{
			name:        "container not found",
			method:      http.MethodGet,
			containerID: "nonexistent",
			mockRuntime: &MockRuntime{
				TypeValue: runtime.RuntimeDocker,
				GetStatsFunc: func(ctx context.Context, id runtime.ContainerID) (*runtime.Stats, error) {
					return nil, runtime.NewContainerError(runtime.RuntimeDocker, "stats", id, runtime.ErrContainerNotFound)
				},
			},
			expectedStatus: http.StatusNotFound,
			expectedErr:    true,
		},
		{
			name:           "method not allowed",
			method:         http.MethodPost,
			containerID:    "test-container",
			mockRuntime:    &MockRuntime{TypeValue: runtime.RuntimeDocker},
			expectedStatus: http.StatusMethodNotAllowed,
			expectedErr:    true,
		},
		{
			name:           "empty container ID",
			method:         http.MethodGet,
			containerID:    "",
			mockRuntime:    &MockRuntime{TypeValue: runtime.RuntimeDocker},
			expectedStatus: http.StatusBadRequest,
			expectedErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registry := createMockRegistry(tt.mockRuntime)
			handler := NewContainerHandlerWithRuntime(registry, nil)

			url := fmt.Sprintf("/api/v1/containers/%s/stats", tt.containerID)
			req := httptest.NewRequest(tt.method, url, nil)
			w := httptest.NewRecorder()

			handler.HandleContainerStats(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("HandleContainerStats() status = %d, want %d", w.Code, tt.expectedStatus)
			}

			if !tt.expectedErr && w.Code == http.StatusOK {
				var response ContainerStatsResponse
				if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
					t.Errorf("Failed to decode response: %v", err)
				}

				// Verify response structure
				if response.ContainerID == "" {
					t.Error("Expected containerId field in response")
				}
				if response.Timestamp == 0 {
					t.Error("Expected timestamp field in response")
				}

				// Verify CPU structure
				if response.CPU.TotalCores == 0 {
					t.Error("Expected cpu.totalCores > 0")
				}

				// Verify memory structure
				if response.Memory.Limit == 0 {
					t.Error("Expected memory.limit > 0")
				}

				// Validate CPU is non-zero if requested
				if tt.validateCPU {
					if response.CPU.Usage < tt.minCPUPercent {
						t.Errorf("Expected CPU percent >= %.2f%%, got %.2f%%", tt.minCPUPercent, response.CPU.Usage)
					}
					t.Logf("CPU percent: %.2f%%", response.CPU.Usage)
				}
			}
		})
	}
}

// TestExtractContainerIDFromPath tests container ID extraction from URL path
func TestExtractContainerIDFromPath(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{
			name: "start endpoint",
			path: "/api/v1/containers/abc123/start",
			want: "abc123",
		},
		{
			name: "stop endpoint",
			path: "/api/v1/containers/def456/stop",
			want: "def456",
		},
		{
			name: "restart endpoint",
			path: "/api/v1/containers/xyz789/restart",
			want: "xyz789",
		},
		{
			name: "inspect endpoint",
			path: "/api/v1/containers/container-name",
			want: "container-name",
		},
		{
			name: "trailing slash",
			path: "/api/v1/containers/test123/start/",
			want: "test123", // Container ID is always at index 4
		},
		{
			name: "short path",
			path: "/api/v1",
			want: "",
		},
		{
			name: "empty path",
			path: "",
			want: "",
		},
		{
			name: "long container ID",
			path: "/api/v1/containers/sha256:1234567890abcdef/start",
			want: "sha256:1234567890abcdef",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractContainerIDFromPath(tt.path)
			if got != tt.want {
				t.Errorf("extractContainerIDFromPath(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

// TestContainerHandler_WithTimeout tests that operations are accepted and run async
func TestContainerHandler_WithTimeout(t *testing.T) {
	// This test verifies that the handler returns 202 Accepted for async operations
	mockClient := &MockDockerClient{
		ContainerStartFunc: func(ctx context.Context, containerID string, options container.StartOptions) error {
			return nil
		},
	}

	handler := NewContainerHandler(mockClient, nil)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/containers/test/start", nil)
	w := httptest.NewRecorder()

	handler.HandleStartContainer(w, req)

	if w.Code != http.StatusAccepted {
		t.Errorf("Expected status 202, got %d", w.Code)
	}
}

// TestContainerHandler_StopOptions tests stop timeout parameter validation
// Note: Stop is now async, so we only test that valid params are accepted (202) and invalid rejected (400)
func TestContainerHandler_StopOptions(t *testing.T) {
	tests := []struct {
		name           string
		queryParams    string
		expectedStatus int
	}{
		{
			name:           "default timeout",
			queryParams:    "",
			expectedStatus: http.StatusAccepted,
		},
		{
			name:           "custom timeout",
			queryParams:    "?timeout=30",
			expectedStatus: http.StatusAccepted,
		},
		{
			name:           "zero timeout rejected",
			queryParams:    "?timeout=0",
			expectedStatus: http.StatusBadRequest, // Invalid timeout
		},
		{
			name:           "minimum valid timeout",
			queryParams:    "?timeout=1",
			expectedStatus: http.StatusAccepted,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := &MockDockerClient{
				ContainerStopFunc: func(ctx context.Context, containerID string, options container.StopOptions) error {
					return nil
				},
			}

			handler := NewContainerHandler(mockClient, nil)
			url := "/api/v1/containers/test/stop" + tt.queryParams
			req := httptest.NewRequest(http.MethodPost, url, nil)
			w := httptest.NewRecorder()

			handler.HandleStopContainer(w, req)

			// Check status code
			if w.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d: %s", tt.expectedStatus, w.Code, w.Body.String())
			}
		})
	}
}

// Helper function to create int pointer
func intPtr(i int) *int {
	return &i
}

// TestContainerHandler_ErrorMessages tests that async operations always return 202 Accepted
// Note: Error messages are now sent via WebSocket notification, not HTTP response
func TestContainerHandler_ErrorMessages(t *testing.T) {
	tests := []struct {
		name     string
		mockFunc func(ctx context.Context, containerID string, options container.StartOptions) error
	}{
		{
			name: "no such container - still accepted",
			mockFunc: func(ctx context.Context, containerID string, options container.StartOptions) error {
				return fmt.Errorf("No such container: %s", containerID)
			},
		},
		{
			name: "already started - still accepted",
			mockFunc: func(ctx context.Context, containerID string, options container.StartOptions) error {
				return fmt.Errorf("container already started")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := &MockDockerClient{
				ContainerStartFunc: tt.mockFunc,
			}

			handler := NewContainerHandler(mockClient, nil)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/containers/test/start", nil)
			w := httptest.NewRecorder()

			handler.HandleStartContainer(w, req)

			// Async operations always return 202 Accepted
			if w.Code != http.StatusAccepted {
				t.Errorf("Expected status 202 Accepted, got %d", w.Code)
			}

			// Response should contain "accepted" status
			var response map[string]interface{}
			if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
				t.Errorf("Failed to decode response: %v", err)
			}
			if response["status"] != "accepted" {
				t.Errorf("Expected status 'accepted', got %v", response["status"])
			}
		})
	}
}

// TestContainerHandler_ListsAllContainers tests that all containers are returned regardless of labels
func TestContainerHandler_ListsAllContainers(t *testing.T) {
	// Create mock storage
	tmpDB := "test_container_list.db"
	defer func() {
		// Clean up
		if err := os.Remove(tmpDB); err != nil && !os.IsNotExist(err) {
			t.Logf("Warning: failed to remove test database: %v", err)
		}
	}()

	store, err := storage.NewStore(storage.Config{DatabasePath: tmpDB})
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Mock Docker client with various containers
	mockClient := &MockDockerClient{
		ContainerListFunc: func(ctx context.Context, options container.ListOptions) ([]types.Container, error) {
			return []types.Container{
				{
					ID:    "abc123",
					Names: []string{"/grafana"},
					Image: "grafana/grafana:latest",
					State: "running",
					Labels: map[string]string{
						"nekzus.app.id": "grafana",
					},
				},
				{
					ID:    "def456",
					Names: []string{"/prometheus"},
					Image: "prom/prometheus:latest",
					State: "running",
					Labels: map[string]string{
						"nekzus.app.id": "prometheus",
					},
				},
				{
					ID:    "ghi789",
					Names: []string{"/nginx"},
					Image: "nginx:latest",
					State: "running",
					Labels: map[string]string{
						"nekzus.app.id": "nginx",
					},
				},
				{
					ID:     "jkl012",
					Names:  []string{"/no-label-container"},
					Image:  "redis:latest",
					State:  "running",
					Labels: map[string]string{
						// No nekzus.app.id label
					},
				},
			}, nil
		},
	}

	handler := NewContainerHandler(mockClient, store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/containers", nil)
	rec := httptest.NewRecorder()

	handler.HandleListContainers(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var containers []ContainerListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &containers); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	// Should return ALL 4 containers regardless of labels
	if len(containers) != 4 {
		t.Errorf("Expected 4 containers, got %d", len(containers))
	}

	// Verify all containers are returned
	foundContainers := make(map[string]bool)
	for _, c := range containers {
		foundContainers[c.Name] = true
	}

	expectedNames := []string{"grafana", "prometheus", "nginx", "no-label-container"}
	for _, name := range expectedNames {
		if !foundContainers[name] {
			t.Errorf("Expected to find container %q", name)
		}
	}
}

// TestContainerHandler_FilterByApprovedApps_NoStorage tests that filtering still works when storage is nil
func TestContainerHandler_FilterByApprovedApps_NoStorage(t *testing.T) {
	// Mock Docker client
	mockClient := &MockDockerClient{
		ContainerListFunc: func(ctx context.Context, options container.ListOptions) ([]types.Container, error) {
			return []types.Container{
				{
					ID:    "abc123",
					Names: []string{"/grafana"},
					Image: "grafana/grafana:latest",
					State: "running",
					Labels: map[string]string{
						"nekzus.app.id": "grafana",
					},
				},
			}, nil
		},
	}

	// Handler with nil storage - should return all containers (backward compatibility)
	handler := NewContainerHandler(mockClient, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/containers", nil)
	rec := httptest.NewRecorder()

	handler.HandleListContainers(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var containers []ContainerListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &containers); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	// Should return all containers when storage is nil (no filtering)
	if len(containers) != 1 {
		t.Errorf("Expected 1 container, got %d", len(containers))
	}
}

// TestContainerHandler_ListsAllContainers_WithEmptyStorage tests that all containers are returned even with empty storage
func TestContainerHandler_ListsAllContainers_WithEmptyStorage(t *testing.T) {
	// Create mock storage with NO registered apps
	tmpDB := "test_container_empty_storage.db"
	defer func() {
		if err := os.Remove(tmpDB); err != nil && !os.IsNotExist(err) {
			t.Logf("Warning: failed to remove test database: %v", err)
		}
	}()

	store, err := storage.NewStore(storage.Config{DatabasePath: tmpDB})
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Mock Docker client with containers
	mockClient := &MockDockerClient{
		ContainerListFunc: func(ctx context.Context, options container.ListOptions) ([]types.Container, error) {
			return []types.Container{
				{
					ID:    "abc123",
					Names: []string{"/grafana"},
					Image: "grafana/grafana:latest",
					State: "running",
					Labels: map[string]string{
						"nekzus.app.id": "grafana",
					},
				},
				{
					ID:    "def456",
					Names: []string{"/prometheus"},
					Image: "prom/prometheus:latest",
					State: "running",
					Labels: map[string]string{
						"nekzus.app.id": "prometheus",
					},
				},
			}, nil
		},
	}

	handler := NewContainerHandler(mockClient, store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/containers", nil)
	rec := httptest.NewRecorder()

	handler.HandleListContainers(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var containers []ContainerListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &containers); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	// Should return ALL containers regardless of registered apps
	if len(containers) != 2 {
		t.Errorf("Expected 2 containers, got %d", len(containers))
	}
}

// TestParseContainerRequest tests parsing container requests with runtime and namespace
func TestParseContainerRequest(t *testing.T) {
	handler := NewContainerHandler(nil, nil)

	tests := []struct {
		name              string
		path              string
		queryParams       string
		expectedContainer string
		expectedRuntime   string
		expectedNamespace string
	}{
		{
			name:              "Docker-style path",
			path:              "/api/v1/containers/abc123/start",
			queryParams:       "",
			expectedContainer: "abc123",
			expectedRuntime:   "",
			expectedNamespace: "",
		},
		{
			name:              "Docker-style with runtime query param",
			path:              "/api/v1/containers/abc123/start",
			queryParams:       "runtime=docker",
			expectedContainer: "abc123",
			expectedRuntime:   "docker",
			expectedNamespace: "",
		},
		{
			name:              "K8s-style path",
			path:              "/api/v1/containers/default/nginx-pod/restart",
			queryParams:       "",
			expectedContainer: "nginx-pod",
			expectedRuntime:   "kubernetes",
			expectedNamespace: "default",
		},
		{
			name:              "K8s-style with explicit runtime",
			path:              "/api/v1/containers/production/web-app/stop",
			queryParams:       "runtime=kubernetes",
			expectedContainer: "web-app",
			expectedRuntime:   "kubernetes",
			expectedNamespace: "production",
		},
		{
			name:              "Query params override path namespace",
			path:              "/api/v1/containers/default/app/start",
			queryParams:       "namespace=staging",
			expectedContainer: "app",
			expectedRuntime:   "kubernetes",
			expectedNamespace: "staging",
		},
		{
			name:              "Docker path with namespace query",
			path:              "/api/v1/containers/abc123/stats",
			queryParams:       "runtime=kubernetes&namespace=default",
			expectedContainer: "abc123",
			expectedRuntime:   "kubernetes",
			expectedNamespace: "default",
		},
		{
			name:              "Empty path returns empty container",
			path:              "/api/v1",
			queryParams:       "",
			expectedContainer: "",
			expectedRuntime:   "",
			expectedNamespace: "",
		},
		{
			name:              "Inspect endpoint (no action)",
			path:              "/api/v1/containers/abc123",
			queryParams:       "",
			expectedContainer: "abc123",
			expectedRuntime:   "",
			expectedNamespace: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := tt.path
			if tt.queryParams != "" {
				url += "?" + tt.queryParams
			}
			req := httptest.NewRequest(http.MethodGet, url, nil)

			containerID, runtimeType, namespace := handler.parseContainerRequest(req)

			if containerID != tt.expectedContainer {
				t.Errorf("containerID = %q, want %q", containerID, tt.expectedContainer)
			}
			if runtimeType != tt.expectedRuntime {
				t.Errorf("runtimeType = %q, want %q", runtimeType, tt.expectedRuntime)
			}
			if namespace != tt.expectedNamespace {
				t.Errorf("namespace = %q, want %q", namespace, tt.expectedNamespace)
			}
		})
	}
}

// TestIsContainerAction tests the container action detection
func TestIsContainerAction(t *testing.T) {
	tests := []struct {
		segment  string
		expected bool
	}{
		{"start", true},
		{"stop", true},
		{"restart", true},
		{"stats", true},
		{"logs", true},
		{"inspect", false},
		{"delete", false},
		{"create", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.segment, func(t *testing.T) {
			result := isContainerAction(tt.segment)
			if result != tt.expected {
				t.Errorf("isContainerAction(%q) = %v, want %v", tt.segment, result, tt.expected)
			}
		})
	}
}

// TestCalculateCPUPercent tests the CPU percentage calculation
func TestCalculateCPUPercent(t *testing.T) {
	tests := []struct {
		name         string
		stats        *StatsJSON
		expectedCPU  float64
		allowedDelta float64
	}{
		{
			name: "valid CPU calculation",
			stats: &StatsJSON{
				CPUStats: struct {
					CPUUsage struct {
						TotalUsage  uint64   `json:"total_usage"`
						PercpuUsage []uint64 `json:"percpu_usage"`
					} `json:"cpu_usage"`
					SystemUsage uint64 `json:"system_cpu_usage"`
				}{
					CPUUsage: struct {
						TotalUsage  uint64   `json:"total_usage"`
						PercpuUsage []uint64 `json:"percpu_usage"`
					}{
						TotalUsage:  200000000,
						PercpuUsage: []uint64{50000000, 50000000, 50000000, 50000000}, // 4 CPUs
					},
					SystemUsage: 2000000000,
				},
				PreCPUStats: struct {
					CPUUsage struct {
						TotalUsage  uint64   `json:"total_usage"`
						PercpuUsage []uint64 `json:"percpu_usage"`
					} `json:"cpu_usage"`
					SystemUsage uint64 `json:"system_cpu_usage"`
				}{
					CPUUsage: struct {
						TotalUsage  uint64   `json:"total_usage"`
						PercpuUsage []uint64 `json:"percpu_usage"`
					}{
						TotalUsage:  100000000,
						PercpuUsage: []uint64{25000000, 25000000, 25000000, 25000000},
					},
					SystemUsage: 1000000000,
				},
			},
			expectedCPU:  40.0, // (100M / 1B) * 4 CPUs * 100 = 40%
			allowedDelta: 0.1,
		},
		{
			name: "zero system delta",
			stats: &StatsJSON{
				CPUStats: struct {
					CPUUsage struct {
						TotalUsage  uint64   `json:"total_usage"`
						PercpuUsage []uint64 `json:"percpu_usage"`
					} `json:"cpu_usage"`
					SystemUsage uint64 `json:"system_cpu_usage"`
				}{
					CPUUsage: struct {
						TotalUsage  uint64   `json:"total_usage"`
						PercpuUsage []uint64 `json:"percpu_usage"`
					}{
						TotalUsage:  100000000,
						PercpuUsage: []uint64{25000000, 25000000, 25000000, 25000000},
					},
					SystemUsage: 1000000000,
				},
				PreCPUStats: struct {
					CPUUsage struct {
						TotalUsage  uint64   `json:"total_usage"`
						PercpuUsage []uint64 `json:"percpu_usage"`
					} `json:"cpu_usage"`
					SystemUsage uint64 `json:"system_cpu_usage"`
				}{
					CPUUsage: struct {
						TotalUsage  uint64   `json:"total_usage"`
						PercpuUsage []uint64 `json:"percpu_usage"`
					}{
						TotalUsage:  100000000,
						PercpuUsage: []uint64{25000000, 25000000, 25000000, 25000000},
					},
					SystemUsage: 1000000000, // Same as current
				},
			},
			expectedCPU:  0.0,
			allowedDelta: 0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cpuPercent := calculateCPUPercent(tt.stats)
			diff := cpuPercent - tt.expectedCPU
			if diff < 0 {
				diff = -diff
			}
			if diff > tt.allowedDelta {
				t.Errorf("calculateCPUPercent() = %.2f, want %.2f (delta %.2f)", cpuPercent, tt.expectedCPU, diff)
			}
		})
	}
}

// createMockStatsReader creates a mock stats reader with two samples for batch testing
func createMockStatsReaderForBatch(numCPUs int) *mockStatsReader {
	sample1 := StatsJSON{
		CPUStats: struct {
			CPUUsage struct {
				TotalUsage  uint64   `json:"total_usage"`
				PercpuUsage []uint64 `json:"percpu_usage"`
			} `json:"cpu_usage"`
			SystemUsage uint64 `json:"system_cpu_usage"`
		}{
			CPUUsage: struct {
				TotalUsage  uint64   `json:"total_usage"`
				PercpuUsage []uint64 `json:"percpu_usage"`
			}{
				TotalUsage:  100000000,
				PercpuUsage: make([]uint64, numCPUs),
			},
			SystemUsage: 1000000000,
		},
		MemoryStats: struct {
			Usage uint64 `json:"usage"`
			Limit uint64 `json:"limit"`
		}{
			Usage: 52428800,
			Limit: 268435456,
		},
		Networks: map[string]struct {
			RxBytes uint64 `json:"rx_bytes"`
			TxBytes uint64 `json:"tx_bytes"`
		}{
			"eth0": {RxBytes: 1024000, TxBytes: 512000},
		},
	}

	sample2 := StatsJSON{
		CPUStats: struct {
			CPUUsage struct {
				TotalUsage  uint64   `json:"total_usage"`
				PercpuUsage []uint64 `json:"percpu_usage"`
			} `json:"cpu_usage"`
			SystemUsage uint64 `json:"system_cpu_usage"`
		}{
			CPUUsage: struct {
				TotalUsage  uint64   `json:"total_usage"`
				PercpuUsage []uint64 `json:"percpu_usage"`
			}{
				TotalUsage:  150000000,
				PercpuUsage: make([]uint64, numCPUs),
			},
			SystemUsage: 2000000000,
		},
		MemoryStats: struct {
			Usage uint64 `json:"usage"`
			Limit uint64 `json:"limit"`
		}{
			Usage: 52428800,
			Limit: 268435456,
		},
		Networks: map[string]struct {
			RxBytes uint64 `json:"rx_bytes"`
			TxBytes uint64 `json:"tx_bytes"`
		}{
			"eth0": {RxBytes: 1024000, TxBytes: 512000},
		},
	}

	sample1JSON, _ := json.Marshal(sample1)
	sample2JSON, _ := json.Marshal(sample2)

	return &mockStatsReader{
		data:  [][]byte{append(sample1JSON, '\n'), append(sample2JSON, '\n')},
		delay: 10 * time.Millisecond,
	}
}

// TestContainerHandler_HandleBatchContainerStats tests batch container stats
func TestContainerHandler_HandleBatchContainerStats(t *testing.T) {
	mockRuntime := &MockRuntime{
		TypeValue: runtime.RuntimeDocker,
		ListFunc: func(ctx context.Context, opts runtime.ListOptions) ([]runtime.Container, error) {
			return []runtime.Container{
				{
					ID:    runtime.ContainerID{Runtime: runtime.RuntimeDocker, ID: "abc123def456"},
					Name:  "grafana",
					State: runtime.StateRunning,
				},
				{
					ID:    runtime.ContainerID{Runtime: runtime.RuntimeDocker, ID: "def456ghi789"},
					Name:  "prometheus",
					State: runtime.StateRunning,
				},
				{
					ID:    runtime.ContainerID{Runtime: runtime.RuntimeDocker, ID: "stopped123456"},
					Name:  "stopped",
					State: runtime.StateExited,
				},
			}, nil
		},
		GetStatsFunc: func(ctx context.Context, id runtime.ContainerID) (*runtime.Stats, error) {
			return &runtime.Stats{
				ContainerID: id,
				CPU:         runtime.CPUStats{Usage: 10.0, CoresUsed: 0.4, TotalCores: 4.0},
				Memory:      runtime.MemoryStats{Usage: 20.0, Used: 104857600, Limit: 536870912},
				Network:     runtime.NetworkStats{RxBytes: 1024000, TxBytes: 2048000},
				Timestamp:   time.Now().Unix(),
			}, nil
		},
	}

	registry := createMockRegistry(mockRuntime)
	handler := NewContainerHandlerWithRuntime(registry, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/containers/stats", nil)
	w := httptest.NewRecorder()

	handler.HandleBatchContainerStats(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var response BatchContainerStatsResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// Should only return stats for running containers (2, not 3)
	if len(response.Containers) != 2 {
		t.Errorf("Expected 2 containers, got %d", len(response.Containers))
	}

	// Check timestamp
	if response.Timestamp == 0 {
		t.Error("Expected non-zero timestamp")
	}

	// Verify container IDs are short (12 chars)
	for _, c := range response.Containers {
		if len(c.ContainerID) > 12 {
			t.Errorf("Expected short container ID (<=12 chars), got %d chars: %s", len(c.ContainerID), c.ContainerID)
		}
	}
}

// TestContainerHandler_HandleBatchContainerStats_EmptyList tests batch stats with no containers
func TestContainerHandler_HandleBatchContainerStats_EmptyList(t *testing.T) {
	mockRuntime := &MockRuntime{
		TypeValue: runtime.RuntimeDocker,
		ListFunc: func(ctx context.Context, opts runtime.ListOptions) ([]runtime.Container, error) {
			return []runtime.Container{}, nil
		},
	}

	registry := createMockRegistry(mockRuntime)
	handler := NewContainerHandlerWithRuntime(registry, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/containers/stats", nil)
	w := httptest.NewRecorder()

	handler.HandleBatchContainerStats(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", w.Code)
	}

	var response BatchContainerStatsResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// Should return empty array, not null
	if response.Containers == nil {
		t.Error("Expected empty array, got nil")
	}
	if len(response.Containers) != 0 {
		t.Errorf("Expected 0 containers, got %d", len(response.Containers))
	}
}

// TestContainerHandler_HandleBatchContainerStats_MethodNotAllowed tests POST method rejection
func TestContainerHandler_HandleBatchContainerStats_MethodNotAllowed(t *testing.T) {
	mockRuntime := &MockRuntime{TypeValue: runtime.RuntimeDocker}

	registry := createMockRegistry(mockRuntime)
	handler := NewContainerHandlerWithRuntime(registry, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/containers/stats", nil)
	w := httptest.NewRecorder()

	handler.HandleBatchContainerStats(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405, got %d", w.Code)
	}
}

// TestContainerHandler_HandleBatchContainerStats_WithApprovedApps tests filtering by approved apps
func TestContainerHandler_HandleBatchContainerStats_WithApprovedApps(t *testing.T) {
	// Create mock storage with approved apps
	tmpDB := "test_batch_stats_approved.db"
	defer os.Remove(tmpDB)

	store, err := storage.NewStore(storage.Config{DatabasePath: tmpDB})
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Add only grafana as approved
	if err := store.SaveApp(nekzustypes.App{ID: "grafana", Name: "Grafana"}); err != nil {
		t.Fatalf("Failed to save app: %v", err)
	}

	mockRuntime := &MockRuntime{
		TypeValue: runtime.RuntimeDocker,
		ListFunc: func(ctx context.Context, opts runtime.ListOptions) ([]runtime.Container, error) {
			return []runtime.Container{
				{
					ID:     runtime.ContainerID{Runtime: runtime.RuntimeDocker, ID: "abc123456789"},
					Name:   "grafana",
					State:  runtime.StateRunning,
					Labels: map[string]string{"nekzus.app.id": "grafana"},
				},
				{
					ID:     runtime.ContainerID{Runtime: runtime.RuntimeDocker, ID: "def456789012"},
					Name:   "prometheus",
					State:  runtime.StateRunning,
					Labels: map[string]string{"nekzus.app.id": "prometheus"}, // Not approved
				},
				{
					ID:     runtime.ContainerID{Runtime: runtime.RuntimeDocker, ID: "ghi789012345"},
					Name:   "no-label",
					State:  runtime.StateRunning,
					Labels: map[string]string{}, // No app ID label
				},
			}, nil
		},
		GetStatsFunc: func(ctx context.Context, id runtime.ContainerID) (*runtime.Stats, error) {
			return &runtime.Stats{
				ContainerID: id,
				CPU:         runtime.CPUStats{Usage: 10.0, CoresUsed: 0.4, TotalCores: 4.0},
				Memory:      runtime.MemoryStats{Usage: 20.0, Used: 104857600, Limit: 536870912},
				Network:     runtime.NetworkStats{RxBytes: 1024000, TxBytes: 2048000},
				Timestamp:   time.Now().Unix(),
			}, nil
		},
	}

	registry := createMockRegistry(mockRuntime)
	handler := NewContainerHandlerWithRuntime(registry, store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/containers/stats", nil)
	w := httptest.NewRecorder()

	handler.HandleBatchContainerStats(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var response BatchContainerStatsResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// Should only return stats for grafana (approved)
	if len(response.Containers) != 1 {
		t.Errorf("Expected 1 container (grafana only), got %d", len(response.Containers))
	}
}

// TestContainerHandler_HandleBatchContainerStats_ContinuesOnError tests batch continues when one container fails
func TestContainerHandler_HandleBatchContainerStats_ContinuesOnError(t *testing.T) {
	mockRuntime := &MockRuntime{
		TypeValue: runtime.RuntimeDocker,
		ListFunc: func(ctx context.Context, opts runtime.ListOptions) ([]runtime.Container, error) {
			return []runtime.Container{
				{ID: runtime.ContainerID{Runtime: runtime.RuntimeDocker, ID: "container1234"}, Name: "c1", State: runtime.StateRunning},
				{ID: runtime.ContainerID{Runtime: runtime.RuntimeDocker, ID: "container2345"}, Name: "c2", State: runtime.StateRunning},
				{ID: runtime.ContainerID{Runtime: runtime.RuntimeDocker, ID: "container3456"}, Name: "c3", State: runtime.StateRunning},
			}, nil
		},
		GetStatsFunc: func(ctx context.Context, id runtime.ContainerID) (*runtime.Stats, error) {
			if id.ID == "container2345" {
				return nil, fmt.Errorf("stats error for container2")
			}
			return &runtime.Stats{
				ContainerID: id,
				CPU:         runtime.CPUStats{Usage: 10.0, CoresUsed: 0.4, TotalCores: 4.0},
				Memory:      runtime.MemoryStats{Usage: 20.0, Used: 104857600, Limit: 536870912},
				Network:     runtime.NetworkStats{RxBytes: 1024000, TxBytes: 2048000},
				Timestamp:   time.Now().Unix(),
			}, nil
		},
	}

	registry := createMockRegistry(mockRuntime)
	handler := NewContainerHandlerWithRuntime(registry, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/containers/stats", nil)
	w := httptest.NewRecorder()

	handler.HandleBatchContainerStats(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", w.Code)
	}

	var response BatchContainerStatsResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// Should return stats for 2 containers (excluding the one with error)
	if len(response.Containers) != 2 {
		t.Errorf("Expected 2 containers, got %d", len(response.Containers))
	}
}

// TestHandleListContainers_ReturnsAllContainers verifies that all containers are returned
// regardless of route linkage (filtering is handled by frontend)
func TestHandleListContainers_ReturnsAllContainers(t *testing.T) {
	// Create a mock runtime that returns containers
	mockRuntime := &MockRuntime{
		TypeValue: runtime.RuntimeDocker,
		ListFunc: func(ctx context.Context, opts runtime.ListOptions) ([]runtime.Container, error) {
			return []runtime.Container{
				{
					ID:    runtime.ContainerID{Runtime: runtime.RuntimeDocker, ID: "abc123def456"},
					Name:  "grafana",
					Image: "grafana/grafana:latest",
					State: runtime.StateRunning,
				},
				{
					ID:    runtime.ContainerID{Runtime: runtime.RuntimeDocker, ID: "def456ghi789"},
					Name:  "prometheus",
					Image: "prom/prometheus:latest",
					State: runtime.StateRunning,
				},
				{
					ID:    runtime.ContainerID{Runtime: runtime.RuntimeDocker, ID: "ghi789jkl012"},
					Name:  "redis",
					Image: "redis:latest",
					State: runtime.StateRunning,
				},
			}, nil
		},
	}

	registry := createMockRegistry(mockRuntime)

	// Create storage with routes explicitly linked to containers
	tmpDB := t.TempDir() + "/test.db"
	store, err := storage.NewStore(storage.Config{DatabasePath: tmpDB})
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	// Save apps first (required for routes)
	if err := store.SaveApp(nekzustypes.App{ID: "grafana", Name: "Grafana"}); err != nil {
		t.Fatalf("Failed to save app: %v", err)
	}
	if err := store.SaveApp(nekzustypes.App{ID: "prometheus", Name: "Prometheus"}); err != nil {
		t.Fatalf("Failed to save app: %v", err)
	}

	// Save routes with explicit container IDs - only for grafana and prometheus, NOT redis
	if err := store.SaveRoute(nekzustypes.Route{
		RouteID:     "route-grafana",
		AppID:       "grafana",
		PathBase:    "/apps/grafana/",
		To:          "http://192.168.0.100:3000",
		ContainerID: "abc123def456", // Explicit link to container
	}); err != nil {
		t.Fatalf("Failed to save route: %v", err)
	}
	if err := store.SaveRoute(nekzustypes.Route{
		RouteID:     "route-prometheus",
		AppID:       "prometheus",
		PathBase:    "/apps/prometheus/",
		To:          "http://192.168.0.101:9090",
		ContainerID: "def456ghi789", // Explicit link to container
	}); err != nil {
		t.Fatalf("Failed to save route: %v", err)
	}

	handler := NewContainerHandlerWithRuntime(registry, store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/containers", nil)
	w := httptest.NewRecorder()

	handler.HandleListContainers(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var response []ContainerListResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// Should return ALL 3 containers (filtering is done by frontend, not backend)
	if len(response) != 3 {
		t.Errorf("Expected 3 containers (all containers), got %d", len(response))
		for i, c := range response {
			t.Logf("Container %d: %s (%s)", i, c.Name, c.ID)
		}
	}

	// Verify all containers are present
	names := make(map[string]bool)
	for _, c := range response {
		names[c.Name] = true
	}

	if !names["grafana"] {
		t.Error("Expected grafana container to be in response")
	}
	if !names["prometheus"] {
		t.Error("Expected prometheus container to be in response")
	}
	if !names["redis"] {
		t.Error("Expected redis container to be in response (backend returns all, frontend filters)")
	}
}

// TestHandleListContainers_ShowsAllContainers verifies that all containers are returned
// regardless of whether they have the nekzus.app.id label
func TestHandleListContainers_ShowsAllContainers(t *testing.T) {
	// Create a mock runtime that returns containers with and without labels
	mockRuntime := &MockRuntime{
		TypeValue: runtime.RuntimeDocker,
		ListFunc: func(ctx context.Context, opts runtime.ListOptions) ([]runtime.Container, error) {
			return []runtime.Container{
				{
					ID:    runtime.ContainerID{Runtime: runtime.RuntimeDocker, ID: "container1"},
					Name:  "labeled-container",
					Image: "nginx:latest",
					State: runtime.StateRunning,
					Labels: map[string]string{
						"nekzus.app.id": "my-app",
					},
				},
				{
					ID:    runtime.ContainerID{Runtime: runtime.RuntimeDocker, ID: "container2"},
					Name:  "unlabeled-container",
					Image: "redis:latest",
					State: runtime.StateRunning,
					Labels: map[string]string{
						"some.other.label": "value",
					},
				},
				{
					ID:     runtime.ContainerID{Runtime: runtime.RuntimeDocker, ID: "container3"},
					Name:   "no-labels-container",
					Image:  "postgres:latest",
					State:  runtime.StateRunning,
					Labels: map[string]string{},
				},
			}, nil
		},
	}

	registry := createMockRegistry(mockRuntime)

	// Create handler with storage (to trigger the filtering code path)
	tmpDB := t.TempDir() + "/test.db"
	store, err := storage.NewStore(storage.Config{DatabasePath: tmpDB})
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	handler := NewContainerHandlerWithRuntime(registry, store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/containers", nil)
	w := httptest.NewRecorder()

	handler.HandleListContainers(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var response []ContainerListResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// Should return ALL 3 containers, not just the labeled one
	if len(response) != 3 {
		t.Errorf("Expected 3 containers (all containers regardless of labels), got %d", len(response))
		for i, c := range response {
			t.Logf("Container %d: %s (%s)", i, c.Name, c.ID)
		}
	}

	// Verify all containers are present
	names := make(map[string]bool)
	for _, c := range response {
		names[c.Name] = true
	}

	expectedNames := []string{"labeled-container", "unlabeled-container", "no-labels-container"}
	for _, name := range expectedNames {
		if !names[name] {
			t.Errorf("Expected container %q to be in response", name)
		}
	}
}
