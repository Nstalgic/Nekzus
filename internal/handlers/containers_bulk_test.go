package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
)

// MockDockerClientBulk is a mock implementation for bulk operations testing
type MockDockerClientBulk struct {
	mu             sync.Mutex
	containers     []types.Container
	restartedIDs   []string
	stoppedIDs     []string
	startedIDs     []string
	shouldFailID   string
	shouldFailWith error
}

func (m *MockDockerClientBulk) ContainerList(ctx context.Context, options container.ListOptions) ([]types.Container, error) {
	return m.containers, nil
}

func (m *MockDockerClientBulk) ContainerRestart(ctx context.Context, containerID string, options container.StopOptions) error {
	if containerID == m.shouldFailID {
		return m.shouldFailWith
	}
	m.mu.Lock()
	m.restartedIDs = append(m.restartedIDs, containerID)
	m.mu.Unlock()
	return nil
}

func (m *MockDockerClientBulk) ContainerStop(ctx context.Context, containerID string, options container.StopOptions) error {
	if containerID == m.shouldFailID {
		return m.shouldFailWith
	}
	m.mu.Lock()
	m.stoppedIDs = append(m.stoppedIDs, containerID)
	m.mu.Unlock()
	return nil
}

func (m *MockDockerClientBulk) ContainerStart(ctx context.Context, containerID string, options container.StartOptions) error {
	if containerID == m.shouldFailID {
		return m.shouldFailWith
	}
	m.mu.Lock()
	m.startedIDs = append(m.startedIDs, containerID)
	m.mu.Unlock()
	return nil
}

func (m *MockDockerClientBulk) ContainerInspect(ctx context.Context, containerID string) (types.ContainerJSON, error) {
	// Find container name from containers list
	name := containerID
	for _, c := range m.containers {
		if c.ID == containerID {
			if len(c.Names) > 0 {
				name = c.Names[0]
			}
			break
		}
	}

	return types.ContainerJSON{
		ContainerJSONBase: &types.ContainerJSONBase{
			Name: name,
		},
	}, nil
}

func (m *MockDockerClientBulk) ContainerStats(ctx context.Context, containerID string, stream bool) (container.StatsResponseReader, error) {
	return container.StatsResponseReader{}, nil
}

func (m *MockDockerClientBulk) ContainerLogs(ctx context.Context, containerID string, options container.LogsOptions) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}

// TestHandleRestartAll tests restarting all containers
func TestHandleRestartAll(t *testing.T) {
	tests := []struct {
		name           string
		method         string
		containers     []types.Container
		shouldFailID   string
		shouldFailWith error
		expectedStatus int
		checkResponse  func(t *testing.T, body map[string]interface{})
	}{
		{
			name:   "successful restart all",
			method: http.MethodPost,
			containers: []types.Container{
				{ID: "container1", Names: []string{"/plex"}},
				{ID: "container2", Names: []string{"/grafana"}},
				{ID: "container3", Names: []string{"/nextcloud"}},
			},
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, body map[string]interface{}) {
				if msg, ok := body["message"].(string); !ok || msg == "" {
					t.Error("Expected message in response")
				}
				if results, ok := body["results"].([]interface{}); !ok || len(results) != 3 {
					t.Errorf("Expected 3 results, got %v", results)
				}
			},
		},
		{
			name:   "partial failure",
			method: http.MethodPost,
			containers: []types.Container{
				{ID: "container1", Names: []string{"/plex"}},
				{ID: "container2", Names: []string{"/grafana"}},
				{ID: "container3", Names: []string{"/nextcloud"}},
			},
			shouldFailID:   "container2",
			shouldFailWith: errors.New("container is unhealthy"),
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, body map[string]interface{}) {
				results, ok := body["results"].([]interface{})
				if !ok {
					t.Fatal("Expected results array")
				}

				// Check that we have results for all containers
				if len(results) != 3 {
					t.Errorf("Expected 3 results, got %d", len(results))
				}

				// Check that container2 failed
				successCount := 0
				for _, r := range results {
					result := r.(map[string]interface{})
					if result["success"].(bool) {
						successCount++
					}
				}

				if successCount != 2 {
					t.Errorf("Expected 2 successful restarts, got %d", successCount)
				}
			},
		},
		{
			name:           "no containers",
			method:         http.MethodPost,
			containers:     []types.Container{},
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, body map[string]interface{}) {
				results, ok := body["results"].([]interface{})
				if !ok || len(results) != 0 {
					t.Error("Expected empty results for no containers")
				}
			},
		},
		{
			name:           "GET method not allowed",
			method:         http.MethodGet,
			expectedStatus: http.StatusMethodNotAllowed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := &MockDockerClientBulk{
				containers:     tt.containers,
				shouldFailID:   tt.shouldFailID,
				shouldFailWith: tt.shouldFailWith,
			}

			handler := NewContainerHandler(mockClient, nil)

			req := httptest.NewRequest(tt.method, "/api/v1/containers/restart-all", nil)
			w := httptest.NewRecorder()

			handler.HandleRestartAll(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, w.Code)
			}

			if tt.expectedStatus == http.StatusOK && tt.checkResponse != nil {
				var response map[string]interface{}
				if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
					t.Fatalf("Failed to decode response: %v", err)
				}
				tt.checkResponse(t, response)
			}
		})
	}
}

// TestHandleStopAll tests stopping all containers
func TestHandleStopAll(t *testing.T) {
	mockClient := &MockDockerClientBulk{
		containers: []types.Container{
			{ID: "container1", Names: []string{"/plex"}},
			{ID: "container2", Names: []string{"/grafana"}},
		},
	}

	handler := NewContainerHandler(mockClient, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/containers/stop-all", nil)
	w := httptest.NewRecorder()

	handler.HandleStopAll(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	results := response["results"].([]interface{})
	if len(results) != 2 {
		t.Errorf("Expected 2 results, got %d", len(results))
	}

	// Verify containers were stopped
	if len(mockClient.stoppedIDs) != 2 {
		t.Errorf("Expected 2 containers stopped, got %d", len(mockClient.stoppedIDs))
	}
}

// TestHandleStartAll tests starting all containers
func TestHandleStartAll(t *testing.T) {
	mockClient := &MockDockerClientBulk{
		containers: []types.Container{
			{ID: "container1", Names: []string{"/plex"}, State: "exited"},
			{ID: "container2", Names: []string{"/grafana"}, State: "exited"},
		},
	}

	handler := NewContainerHandler(mockClient, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/containers/start-all", nil)
	w := httptest.NewRecorder()

	handler.HandleStartAll(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// Verify containers were started
	if len(mockClient.startedIDs) != 2 {
		t.Errorf("Expected 2 containers started, got %d", len(mockClient.startedIDs))
	}
}

// TestHandleBatchOperation tests batch container operations
func TestHandleBatchOperation(t *testing.T) {
	tests := []struct {
		name           string
		body           string
		expectedStatus int
		checkResponse  func(t *testing.T, body map[string]interface{})
	}{
		{
			name: "restart batch",
			body: `{
				"action": "restart",
				"containerIds": ["container1", "container2"]
			}`,
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, body map[string]interface{}) {
				results := body["results"].([]interface{})
				if len(results) != 2 {
					t.Errorf("Expected 2 results, got %d", len(results))
				}
			},
		},
		{
			name: "stop batch",
			body: `{
				"action": "stop",
				"containerIds": ["container1"]
			}`,
			expectedStatus: http.StatusOK,
		},
		{
			name: "start batch",
			body: `{
				"action": "start",
				"containerIds": ["container1"]
			}`,
			expectedStatus: http.StatusOK,
		},
		{
			name: "invalid action",
			body: `{
				"action": "invalid",
				"containerIds": ["container1"]
			}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "empty container list",
			body: `{
				"action": "restart",
				"containerIds": []
			}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "invalid JSON",
			body:           `{invalid json}`,
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := &MockDockerClientBulk{
				containers: []types.Container{
					{ID: "container1", Names: []string{"/plex"}},
					{ID: "container2", Names: []string{"/grafana"}},
				},
			}

			handler := NewContainerHandler(mockClient, nil)

			req := httptest.NewRequest(http.MethodPost, "/api/v1/containers/batch", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			handler.HandleBatchOperation(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, w.Code)
			}

			if tt.expectedStatus == http.StatusOK && tt.checkResponse != nil {
				var response map[string]interface{}
				if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
					t.Fatalf("Failed to decode response: %v", err)
				}
				tt.checkResponse(t, response)
			}
		})
	}
}

// TestBulkOperationConcurrency tests concurrent bulk operations
func TestBulkOperationConcurrency(t *testing.T) {
	mockClient := &MockDockerClientBulk{
		containers: []types.Container{
			{ID: "container1", Names: []string{"/plex"}},
			{ID: "container2", Names: []string{"/grafana"}},
		},
	}

	handler := NewContainerHandler(mockClient, nil)

	// Run 10 concurrent restart-all operations
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/containers/restart-all", nil)
			w := httptest.NewRecorder()
			handler.HandleRestartAll(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("Expected status 200, got %d", w.Code)
			}
			done <- true
		}()
	}

	// Wait for all operations to complete
	for i := 0; i < 10; i++ {
		<-done
	}
}
