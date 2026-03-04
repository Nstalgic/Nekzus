package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nstalgic/nekzus/internal/storage"
	"github.com/nstalgic/nekzus/internal/toolbox"
	"github.com/nstalgic/nekzus/internal/types"
)

// TestListServices tests listing all toolbox services
func TestListServices(t *testing.T) {
	handler, cleanup := setupToolboxHandler(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/toolbox/services", nil)
	w := httptest.NewRecorder()

	handler.ListServices(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response struct {
		Services []*types.ServiceTemplate `json:"services"`
		Count    int                      `json:"count"`
	}

	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response.Count == 0 {
		t.Error("Expected at least one service")
	}

	if len(response.Services) != response.Count {
		t.Errorf("Expected %d services, got %d", response.Count, len(response.Services))
	}
}

// TestGetService tests getting a specific service
func TestGetService(t *testing.T) {
	handler, cleanup := setupToolboxHandler(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/toolbox/services/grafana", nil)
	req.SetPathValue("id", "grafana")
	w := httptest.NewRecorder()

	handler.GetService(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var service types.ServiceTemplate
	if err := json.NewDecoder(w.Body).Decode(&service); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if service.ID != "grafana" {
		t.Errorf("Expected service ID 'grafana', got '%s'", service.ID)
	}
	if service.Name != "Grafana" {
		t.Errorf("Expected service name 'Grafana', got '%s'", service.Name)
	}
}

// TestGetService_NotFound tests getting non-existent service
func TestGetService_NotFound(t *testing.T) {
	handler, cleanup := setupToolboxHandler(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/toolbox/services/nonexistent", nil)
	req.SetPathValue("id", "nonexistent")
	w := httptest.NewRecorder()

	handler.GetService(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", w.Code)
	}
}

// TestDeployService tests deploying a service
func TestDeployService(t *testing.T) {
	handler, cleanup := setupToolboxHandler(t)
	defer cleanup()

	deployReq := types.DeploymentRequest{
		ServiceID:   "grafana",
		ServiceName: "my-grafana",
		EnvVars: map[string]string{
			"SERVICE_NAME":      "my-grafana",
			"GF_ADMIN_PASSWORD": "secret123",
			"HOST_PORT":         "3000",
		},
		AutoStart: false, // Don't actually start in test
	}

	body, _ := json.Marshal(deployReq)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/toolbox/deploy", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.DeployService(w, req)

	if w.Code != http.StatusAccepted {
		t.Errorf("Expected status 202, got %d: %s", w.Code, w.Body.String())
	}

	var response struct {
		DeploymentID string `json:"deployment_id"`
		Status       string `json:"status"`
		Message      string `json:"message"`
	}

	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response.DeploymentID == "" {
		t.Error("Expected non-empty deployment ID")
	}
	if response.Status != types.DeploymentStatusPending {
		t.Errorf("Expected status '%s', got '%s'", types.DeploymentStatusPending, response.Status)
	}
}

// TestDeployService_InvalidRequest tests deployment with invalid request
func TestDeployService_InvalidRequest(t *testing.T) {
	handler, cleanup := setupToolboxHandler(t)
	defer cleanup()

	tests := []struct {
		name    string
		request types.DeploymentRequest
		wantErr bool
	}{
		{
			name: "missing service ID",
			request: types.DeploymentRequest{
				ServiceName: "my-service",
				EnvVars:     map[string]string{},
			},
			wantErr: true,
		},
		{
			name: "missing service name",
			request: types.DeploymentRequest{
				ServiceID: "grafana",
				EnvVars:   map[string]string{},
			},
			wantErr: true,
		},
		{
			name: "invalid service ID",
			request: types.DeploymentRequest{
				ServiceID:   "nonexistent",
				ServiceName: "my-service",
				EnvVars:     map[string]string{},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.request)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/toolbox/deploy", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			handler.DeployService(w, req)

			if tt.wantErr && w.Code == http.StatusAccepted {
				t.Errorf("Expected error status, got %d", w.Code)
			}
		})
	}
}

// TestGetDeployment tests getting deployment status
func TestGetDeployment(t *testing.T) {
	handler, cleanup := setupToolboxHandler(t)
	defer cleanup()

	// First create a deployment
	deployment := &types.ToolboxDeployment{
		ID:                "test-deployment-123",
		ServiceTemplateID: "grafana",
		ServiceName:       "test-grafana",
		Status:            types.DeploymentStatusPending,
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

	req := httptest.NewRequest(http.MethodGet, "/api/v1/toolbox/deployments/test-deployment-123", nil)
	req.SetPathValue("id", "test-deployment-123")
	w := httptest.NewRecorder()

	handler.GetDeployment(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var retrieved types.ToolboxDeployment
	if err := json.NewDecoder(w.Body).Decode(&retrieved); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if retrieved.ID != deployment.ID {
		t.Errorf("Expected ID '%s', got '%s'", deployment.ID, retrieved.ID)
	}
}

// TestListDeployments tests listing all deployments
func TestListDeployments(t *testing.T) {
	handler, cleanup := setupToolboxHandler(t)
	defer cleanup()

	// Create test deployments
	deployments := []*types.ToolboxDeployment{
		{
			ID:                "deploy-1",
			ServiceTemplateID: "grafana",
			ServiceName:       "grafana-1",
			Status:            types.DeploymentStatusDeployed,
			ContainerName:     "grafana-1",
			CreatedAt:         time.Now(),
			UpdatedAt:         time.Now(),
		},
		{
			ID:                "deploy-2",
			ServiceTemplateID: "pihole",
			ServiceName:       "pihole-1",
			Status:            types.DeploymentStatusPending,
			ContainerName:     "pihole-1",
			CreatedAt:         time.Now(),
			UpdatedAt:         time.Now(),
		},
	}

	for _, d := range deployments {
		if err := handler.storage.SaveDeployment(d); err != nil {
			t.Fatalf("Failed to save deployment: %v", err)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/toolbox/deployments", nil)
	w := httptest.NewRecorder()

	handler.ListDeployments(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response struct {
		Deployments []*types.ToolboxDeployment `json:"deployments"`
		Count       int                        `json:"count"`
	}

	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response.Count != 2 {
		t.Errorf("Expected 2 deployments, got %d", response.Count)
	}
}

// TestRemoveDeployment tests removing a deployment
func TestRemoveDeployment(t *testing.T) {
	handler, cleanup := setupToolboxHandler(t)
	defer cleanup()

	// Create a deployment
	deployment := &types.ToolboxDeployment{
		ID:                "test-remove-123",
		ServiceTemplateID: "grafana",
		ServiceName:       "test-remove-grafana",
		Status:            types.DeploymentStatusDeployed,
		ContainerID:       "", // No actual container in test
		ContainerName:     "test-remove-grafana",
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
	}

	if err := handler.storage.SaveDeployment(deployment); err != nil {
		t.Fatalf("Failed to save deployment: %v", err)
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/toolbox/deployments/test-remove-123?removeVolumes=false", nil)
	req.SetPathValue("id", "test-remove-123")
	w := httptest.NewRecorder()

	handler.RemoveDeployment(w, req)

	// Should succeed even without actual Docker container
	if w.Code != http.StatusOK && w.Code != http.StatusNoContent {
		t.Errorf("Expected status 200 or 204, got %d: %s", w.Code, w.Body.String())
	}
}

func setupToolboxHandler(t *testing.T) (*ToolboxHandler, func()) {
	t.Helper()

	// Create temporary catalog
	catalogPath := createTestCatalog(t)

	// Create toolbox manager
	manager := toolbox.NewManager(catalogPath)
	if err := manager.LoadCatalog(); err != nil {
		t.Fatalf("Failed to load catalog: %v", err)
	}

	// Create deployer
	deployer, err := toolbox.NewDeployer(t.TempDir()+"/toolbox-data", "")
	if err != nil {
		t.Fatalf("Failed to create deployer: %v", err)
	}

	// Create storage
	dbPath := t.TempDir() + "/test.db"
	store, err := storage.NewStore(storage.Config{DatabasePath: dbPath})
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}

	// Create handler with test base URL
	handler := NewToolboxHandler(manager, deployer, store, "https://test.local:8080")

	cleanup := func() {
		deployer.Close()
		store.Close()
		os.Remove(catalogPath)
	}

	return handler, cleanup
}

// TestDeployService_CustomPortInjection tests that custom_port is injected as APP_PORT
func TestDeployService_CustomPortInjection(t *testing.T) {
	handler, cleanup := setupToolboxHandler(t)
	defer cleanup()

	// Deploy with custom port - CustomPort should be injected as APP_PORT
	deployReq := types.DeploymentRequest{
		ServiceID:   "grafana",
		ServiceName: "my-grafana-custom-port",
		EnvVars: map[string]string{
			"SERVICE_NAME":      "my-grafana-custom-port",
			"GF_ADMIN_PASSWORD": "secret123",
			"HOST_PORT":         "3000", // Required by test catalog
		},
		CustomPort: 9999, // Custom port should be injected as APP_PORT
		AutoStart:  false,
	}

	body, _ := json.Marshal(deployReq)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/toolbox/deploy", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.DeployService(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("Expected status 202, got %d: %s", w.Code, w.Body.String())
	}

	var response struct {
		DeploymentID string `json:"deployment_id"`
	}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// Retrieve the deployment and verify APP_PORT was injected
	deployment, err := handler.storage.GetDeployment(response.DeploymentID)
	if err != nil {
		t.Fatalf("Failed to get deployment: %v", err)
	}

	// Verify APP_PORT was injected into EnvVars
	if appPort, exists := deployment.EnvVars["APP_PORT"]; !exists {
		t.Error("Expected APP_PORT to be injected into EnvVars when CustomPort is set")
	} else if appPort != "9999" {
		t.Errorf("Expected APP_PORT='9999', got '%s'", appPort)
	}

	// Verify CustomPort is stored
	if deployment.CustomPort != 9999 {
		t.Errorf("Expected CustomPort=9999, got %d", deployment.CustomPort)
	}
}

func createTestCatalog(t *testing.T) string {
	t.Helper()

	tempDir := t.TempDir()
	catalogPath := filepath.Join(tempDir, "test-catalog.yaml")

	catalogContent := `services:
  - id: grafana
    name: Grafana
    description: Beautiful monitoring dashboards
    icon: 📊
    category: monitoring
    tags: [monitoring, metrics, dashboards]
    difficulty: beginner
    docker_config:
      image: grafana/grafana:latest
      ports:
        - container: 3000
          host_default: 3000
          protocol: tcp
      volumes:
        - name: grafana-data
          mount_path: /var/lib/grafana
      environment:
        GF_SERVER_ROOT_URL: "{{BASE_URL}}/apps/grafana/"
      restart_policy: unless-stopped
    env_vars:
      - name: SERVICE_NAME
        label: Container Name
        required: true
        default: grafana
        type: text
      - name: GF_ADMIN_PASSWORD
        label: Admin Password
        required: true
        type: password
      - name: HOST_PORT
        label: Port
        required: true
        default: "3000"
        type: number
    default_route:
      path_base: /apps/grafana/
      scopes: [read, write]
      websocket: false
      strip_prefix: true
    resources:
      min_cpu: "1"
      min_ram: 512MB
      min_disk: 1GB
`

	if err := os.WriteFile(catalogPath, []byte(catalogContent), 0644); err != nil {
		t.Fatalf("Failed to create test catalog: %v", err)
	}

	return catalogPath
}
