package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/nstalgic/nekzus/internal/auth"
	"github.com/nstalgic/nekzus/internal/storage"
	nekzusTypes "github.com/nstalgic/nekzus/internal/types"
)

// TestDeployService_BodyTooLarge tests that oversized request bodies are rejected
func TestDeployService_BodyTooLarge(t *testing.T) {
	handler, cleanup := setupToolboxHandler(t)
	defer cleanup()

	// Create a valid JSON body larger than 1MB (the limit)
	// We use a large string in a valid JSON structure
	largeData := strings.Repeat("a", 2*1024*1024) // 2MB of 'a's
	largeJSON := map[string]string{"data": largeData}
	largeBody, _ := json.Marshal(largeJSON)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/toolbox/deploy", bytes.NewReader(largeBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.DeployService(w, req)

	// Should return 413 Request Entity Too Large
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("Expected status 413, got %d", w.Code)
	}
}

// TestAuthHandler_PairBodyTooLarge tests that oversized pair request bodies are rejected
func TestAuthHandler_PairBodyTooLarge(t *testing.T) {
	// Setup auth handler
	testSecret := "random-jwt-hmac-key-f8e7d6c5b4a39281"
	authMgr, err := auth.NewManager(
		[]byte(testSecret),
		"nekzus",
		"nekzus-mobile",
		[]string{"valid-token"},
	)
	if err != nil {
		t.Fatalf("Failed to create auth manager: %v", err)
	}

	handler := NewAuthHandler(
		authMgr,
		nil, // storage
		nil, // metrics
		nil, // events
		nil, // activity
		nil, // qrLimiter
		nil, // certManager
		"http://localhost:8080",
		"",
		"test-nexus",
		"1.0.0",
		[]string{"discovery", "proxy"},
	)

	// Create a valid JSON body larger than 10KB (the limit for auth endpoints)
	largeData := strings.Repeat("a", 100*1024) // 100KB of 'a's
	largeJSON := map[string]string{"data": largeData}
	largeBody, _ := json.Marshal(largeJSON)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/pair", bytes.NewReader(largeBody))
	req.Header.Set("Authorization", "Bearer valid-token")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.HandlePair(w, req)

	// Should return 413 Request Entity Too Large
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("Expected status 413, got %d: %s", w.Code, w.Body.String())
	}
}

// TestDeviceHandler_UpdateMetadataBodyTooLarge tests that oversized update request bodies are rejected
func TestDeviceHandler_UpdateMetadataBodyTooLarge(t *testing.T) {
	// Create storage
	dbPath := t.TempDir() + "/test.db"
	store, err := storage.NewStore(storage.Config{DatabasePath: dbPath})
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	handler := NewDeviceHandler(store)

	// Create a test device first
	deviceID := "test-device-123"
	if err := store.SaveDevice(deviceID, "Test Device", "ios", "17.0", []string{"read"}); err != nil {
		t.Fatalf("Failed to save device: %v", err)
	}

	// Create a valid JSON body larger than 10KB
	largeData := strings.Repeat("a", 100*1024) // 100KB of 'a's
	largeJSON := map[string]string{"deviceName": largeData}
	largeBody, _ := json.Marshal(largeJSON)

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/devices/"+deviceID, bytes.NewReader(largeBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.HandleUpdateDeviceMetadata(w, req)

	// Should return 413 Request Entity Too Large
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("Expected status 413, got %d: %s", w.Code, w.Body.String())
	}
}

// TestBackupHandler_MethodNotAllowed tests that incorrect HTTP methods are rejected
func TestBackupHandler_CreateBackup_MethodNotAllowed(t *testing.T) {
	handler := &BackupHandler{
		manager:   nil,
		scheduler: nil,
		storage:   nil,
	}

	// Test with GET instead of POST
	req := httptest.NewRequest(http.MethodGet, "/api/v1/backups", nil)
	w := httptest.NewRecorder()

	handler.CreateBackup(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405, got %d", w.Code)
	}
}

func TestBackupHandler_ListBackups_MethodNotAllowed(t *testing.T) {
	handler := &BackupHandler{
		manager:   nil,
		scheduler: nil,
		storage:   nil,
	}

	// Test with POST instead of GET
	req := httptest.NewRequest(http.MethodPost, "/api/v1/backups", nil)
	w := httptest.NewRecorder()

	handler.ListBackups(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405, got %d", w.Code)
	}
}

func TestBackupHandler_GetBackup_MethodNotAllowed(t *testing.T) {
	handler := &BackupHandler{
		manager:   nil,
		scheduler: nil,
		storage:   nil,
	}

	// Test with POST instead of GET
	req := httptest.NewRequest(http.MethodPost, "/api/v1/backups/123", nil)
	req.SetPathValue("id", "123")
	w := httptest.NewRecorder()

	handler.GetBackup(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405, got %d", w.Code)
	}
}

func TestBackupHandler_DeleteBackup_MethodNotAllowed(t *testing.T) {
	handler := &BackupHandler{
		manager:   nil,
		scheduler: nil,
		storage:   nil,
	}

	// Test with POST instead of DELETE
	req := httptest.NewRequest(http.MethodPost, "/api/v1/backups/123", nil)
	req.SetPathValue("id", "123")
	w := httptest.NewRecorder()

	handler.DeleteBackup(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405, got %d", w.Code)
	}
}

func TestBackupHandler_RestoreBackup_MethodNotAllowed(t *testing.T) {
	handler := &BackupHandler{
		manager:   nil,
		scheduler: nil,
		storage:   nil,
	}

	// Test with GET instead of POST
	req := httptest.NewRequest(http.MethodGet, "/api/v1/backups/123/restore", nil)
	req.SetPathValue("id", "123")
	w := httptest.NewRecorder()

	handler.RestoreBackup(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405, got %d", w.Code)
	}
}

func TestBackupHandler_GetSchedulerStatus_MethodNotAllowed(t *testing.T) {
	handler := &BackupHandler{
		manager:   nil,
		scheduler: nil,
		storage:   nil,
	}

	// Test with POST instead of GET
	req := httptest.NewRequest(http.MethodPost, "/api/v1/backups/scheduler/status", nil)
	w := httptest.NewRecorder()

	handler.GetSchedulerStatus(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405, got %d", w.Code)
	}
}

func TestBackupHandler_TriggerBackup_MethodNotAllowed(t *testing.T) {
	handler := &BackupHandler{
		manager:   nil,
		scheduler: nil,
		storage:   nil,
	}

	// Test with GET instead of POST
	req := httptest.NewRequest(http.MethodGet, "/api/v1/backups/scheduler/trigger", nil)
	w := httptest.NewRecorder()

	handler.TriggerBackup(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405, got %d", w.Code)
	}
}

// TestListServices_ContentType tests that ListServices sets Content-Type header
func TestListServices_ContentType(t *testing.T) {
	handler, cleanup := setupToolboxHandler(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/toolbox/services", nil)
	w := httptest.NewRecorder()

	handler.ListServices(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Expected Content-Type 'application/json', got '%s'", contentType)
	}
}

// TestGetService_ContentType tests that GetService sets Content-Type header
func TestGetService_ContentType(t *testing.T) {
	handler, cleanup := setupToolboxHandler(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/toolbox/services/grafana", nil)
	req.SetPathValue("id", "grafana")
	w := httptest.NewRecorder()

	handler.GetService(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Expected Content-Type 'application/json', got '%s'", contentType)
	}
}

// TestGetDeployment_ContentType tests that GetDeployment sets Content-Type header
func TestGetDeployment_ContentType(t *testing.T) {
	handler, cleanup := setupToolboxHandler(t)
	defer cleanup()

	// First create a deployment
	deployment := &nekzusTypes.ToolboxDeployment{
		ID:                "test-deployment-ct",
		ServiceTemplateID: "grafana",
		ServiceName:       "test-grafana",
		Status:            nekzusTypes.DeploymentStatusPending,
		ContainerName:     "test-grafana",
		EnvVars: map[string]string{
			"SERVICE_NAME": "test-grafana",
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := handler.storage.SaveDeployment(deployment); err != nil {
		t.Fatalf("Failed to save deployment: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/toolbox/deployments/test-deployment-ct", nil)
	req.SetPathValue("id", "test-deployment-ct")
	w := httptest.NewRecorder()

	handler.GetDeployment(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Expected Content-Type 'application/json', got '%s'", contentType)
	}
}

// TestListDeployments_ContentType tests that ListDeployments sets Content-Type header
func TestListDeployments_ContentType(t *testing.T) {
	handler, cleanup := setupToolboxHandler(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/toolbox/deployments", nil)
	w := httptest.NewRecorder()

	handler.ListDeployments(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Expected Content-Type 'application/json', got '%s'", contentType)
	}
}

// TestRemoveDeployment_ContentType tests that RemoveDeployment sets Content-Type header
func TestRemoveDeployment_ContentType(t *testing.T) {
	handler, cleanup := setupToolboxHandler(t)
	defer cleanup()

	// Create a deployment
	deployment := &nekzusTypes.ToolboxDeployment{
		ID:                "test-remove-ct",
		ServiceTemplateID: "grafana",
		ServiceName:       "test-remove-grafana",
		Status:            nekzusTypes.DeploymentStatusDeployed,
		ContainerID:       "",
		ContainerName:     "test-remove-grafana",
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
	}

	if err := handler.storage.SaveDeployment(deployment); err != nil {
		t.Fatalf("Failed to save deployment: %v", err)
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/toolbox/deployments/test-remove-ct", nil)
	req.SetPathValue("id", "test-remove-ct")
	w := httptest.NewRecorder()

	handler.RemoveDeployment(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Expected Content-Type 'application/json', got '%s'", contentType)
	}
}

func TestExtractDeviceIDFromPath_Security(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{
			name:     "valid path",
			path:     "/api/v1/devices/abc123",
			expected: "abc123",
		},
		{
			name:     "valid path with trailing slash",
			path:     "/api/v1/devices/abc123/",
			expected: "abc123",
		},
		{
			name:     "too short path",
			path:     "/api/v1/devices",
			expected: "",
		},
		{
			name:     "path traversal attempt",
			path:     "/api/v1/devices/../../../etc/passwd",
			expected: "",
		},
		{
			name:     "path with backslash traversal",
			path:     "/api/v1/devices/..\\..\\etc\\passwd",
			expected: "",
		},
		{
			name:     "empty device ID",
			path:     "/api/v1/devices/",
			expected: "",
		},
		{
			name:     "very long device ID",
			path:     "/api/v1/devices/" + strings.Repeat("a", 200),
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractDeviceIDFromPath(tt.path)
			if result != tt.expected {
				t.Errorf("extractDeviceIDFromPath(%q) = %q, want %q", tt.path, result, tt.expected)
			}
		})
	}
}

// TestContainerHandler_StopTimeout_Validation tests timeout parameter validation
// Note: Stop is now async, so valid timeouts return 202 Accepted (not 200 OK)
func TestContainerHandler_StopTimeout_Validation(t *testing.T) {
	handler := NewContainerHandler(&mockDockerClientTimeout{}, nil)

	tests := []struct {
		name       string
		timeout    string
		wantStatus int
	}{
		{
			name:       "valid timeout",
			timeout:    "30",
			wantStatus: http.StatusAccepted,
		},
		{
			name:       "timeout at minimum boundary",
			timeout:    "1",
			wantStatus: http.StatusAccepted,
		},
		{
			name:       "timeout at maximum boundary",
			timeout:    "300",
			wantStatus: http.StatusAccepted,
		},
		{
			name:       "timeout too small",
			timeout:    "0",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "timeout too large",
			timeout:    "500",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "negative timeout",
			timeout:    "-1",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/containers/abc123/stop?timeout="+tt.timeout, nil)
			w := httptest.NewRecorder()

			handler.HandleStopContainer(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("Expected status %d for timeout=%s, got %d: %s", tt.wantStatus, tt.timeout, w.Code, w.Body.String())
			}
		})
	}
}

// TestContainerHandler_RestartTimeout_Validation tests timeout parameter validation
// Note: Restart is now async, so valid timeouts return 202 Accepted (not 200 OK)
func TestContainerHandler_RestartTimeout_Validation(t *testing.T) {
	handler := NewContainerHandler(&mockDockerClientTimeout{}, nil)

	tests := []struct {
		name       string
		timeout    string
		wantStatus int
	}{
		{
			name:       "valid timeout",
			timeout:    "30",
			wantStatus: http.StatusAccepted,
		},
		{
			name:       "timeout too small",
			timeout:    "0",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "timeout too large",
			timeout:    "301",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/containers/abc123/restart?timeout="+tt.timeout, nil)
			w := httptest.NewRecorder()

			handler.HandleRestartContainer(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("Expected status %d for timeout=%s, got %d: %s", tt.wantStatus, tt.timeout, w.Code, w.Body.String())
			}
		})
	}
}

func TestBulkOperation_ConcurrencyLimit(t *testing.T) {
	// Create a mock client that tracks concurrent operations
	mockClient := &mockDockerClientConcurrent{
		maxConcurrentOps: 0,
		currentOps:       0,
	}

	handler := NewContainerHandler(mockClient, nil)

	// Create 20 containers to test concurrency limit
	containerIDs := make([]string, 20)
	for i := range containerIDs {
		containerIDs[i] = "container-" + string(rune('a'+i))
	}

	reqBody := BulkOperationRequest{
		Action:       "restart",
		ContainerIDs: containerIDs,
	}

	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/containers/batch", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.HandleBatchOperation(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify that max concurrent operations never exceeded the limit
	if mockClient.maxConcurrentOps > maxConcurrentBulkOps {
		t.Errorf("Max concurrent operations (%d) exceeded limit (%d)", mockClient.maxConcurrentOps, maxConcurrentBulkOps)
	}
}

func TestDeployService_DeploymentTimeout(t *testing.T) {
	handler, cleanup := setupToolboxHandler(t)
	defer cleanup()

	deployReq := nekzusTypes.DeploymentRequest{
		ServiceID:   "grafana",
		ServiceName: "timeout-test",
		EnvVars: map[string]string{
			"SERVICE_NAME":      "timeout-test",
			"GF_ADMIN_PASSWORD": "secret",
			"HOST_PORT":         "3000",
		},
		AutoStart: true, // This will trigger the background deployment
	}

	body, _ := json.Marshal(deployReq)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/toolbox/deploy", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.DeployService(w, req)

	// The handler should accept the request
	if w.Code != http.StatusAccepted {
		t.Errorf("Expected status 202, got %d: %s", w.Code, w.Body.String())
	}

	// Wait briefly for goroutine to start
	time.Sleep(100 * time.Millisecond)
}

type mockDockerClient struct{}

func (m *mockDockerClient) ContainerList(ctx context.Context, options container.ListOptions) ([]types.Container, error) {
	return nil, nil
}

func (m *mockDockerClient) ContainerStart(ctx context.Context, containerID string, options container.StartOptions) error {
	return nil
}

func (m *mockDockerClient) ContainerStop(ctx context.Context, containerID string, options container.StopOptions) error {
	return nil
}

func (m *mockDockerClient) ContainerRestart(ctx context.Context, containerID string, options container.StopOptions) error {
	return nil
}

func (m *mockDockerClient) ContainerInspect(ctx context.Context, containerID string) (types.ContainerJSON, error) {
	return types.ContainerJSON{}, nil
}

func (m *mockDockerClient) ContainerStats(ctx context.Context, containerID string, stream bool) (container.StatsResponseReader, error) {
	return container.StatsResponseReader{}, nil
}

func (m *mockDockerClient) ContainerLogs(ctx context.Context, containerID string, options container.LogsOptions) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}

// mockDockerClientTimeout is a mock that succeeds for timeout validation tests
type mockDockerClientTimeout struct{}

func (m *mockDockerClientTimeout) ContainerList(ctx context.Context, options container.ListOptions) ([]types.Container, error) {
	return nil, nil
}

func (m *mockDockerClientTimeout) ContainerStart(ctx context.Context, containerID string, options container.StartOptions) error {
	return nil
}

func (m *mockDockerClientTimeout) ContainerStop(ctx context.Context, containerID string, options container.StopOptions) error {
	return nil
}

func (m *mockDockerClientTimeout) ContainerRestart(ctx context.Context, containerID string, options container.StopOptions) error {
	return nil
}

func (m *mockDockerClientTimeout) ContainerInspect(ctx context.Context, containerID string) (types.ContainerJSON, error) {
	return types.ContainerJSON{
		ContainerJSONBase: &types.ContainerJSONBase{
			Name: "/" + containerID,
		},
	}, nil
}

func (m *mockDockerClientTimeout) ContainerStats(ctx context.Context, containerID string, stream bool) (container.StatsResponseReader, error) {
	return container.StatsResponseReader{}, nil
}

func (m *mockDockerClientTimeout) ContainerLogs(ctx context.Context, containerID string, options container.LogsOptions) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}

// mockDockerClientConcurrent tracks concurrent operations for testing
type mockDockerClientConcurrent struct {
	mu               sync.Mutex
	maxConcurrentOps int
	currentOps       int
}

func (m *mockDockerClientConcurrent) ContainerList(ctx context.Context, options container.ListOptions) ([]types.Container, error) {
	return nil, nil
}

func (m *mockDockerClientConcurrent) ContainerStart(ctx context.Context, containerID string, options container.StartOptions) error {
	m.trackOp()
	return nil
}

func (m *mockDockerClientConcurrent) ContainerStop(ctx context.Context, containerID string, options container.StopOptions) error {
	m.trackOp()
	return nil
}

func (m *mockDockerClientConcurrent) ContainerRestart(ctx context.Context, containerID string, options container.StopOptions) error {
	m.trackOp()
	return nil
}

func (m *mockDockerClientConcurrent) ContainerInspect(ctx context.Context, containerID string) (types.ContainerJSON, error) {
	return types.ContainerJSON{
		ContainerJSONBase: &types.ContainerJSONBase{
			Name: "/" + containerID,
		},
	}, nil
}

func (m *mockDockerClientConcurrent) ContainerStats(ctx context.Context, containerID string, stream bool) (container.StatsResponseReader, error) {
	return container.StatsResponseReader{}, nil
}

func (m *mockDockerClientConcurrent) ContainerLogs(ctx context.Context, containerID string, options container.LogsOptions) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}

func (m *mockDockerClientConcurrent) trackOp() {
	m.mu.Lock()
	m.currentOps++
	if m.currentOps > m.maxConcurrentOps {
		m.maxConcurrentOps = m.currentOps
	}
	m.mu.Unlock()

	// Simulate some work
	time.Sleep(10 * time.Millisecond)

	m.mu.Lock()
	m.currentOps--
	m.mu.Unlock()
}
