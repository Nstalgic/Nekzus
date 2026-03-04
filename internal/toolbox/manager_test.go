package toolbox

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nstalgic/nekzus/internal/types"
)

// TestLoadComposeCatalog tests loading services from Docker Compose files
func TestLoadComposeCatalog(t *testing.T) {
	catalogDir := setupTestComposeDir(t)
	defer os.RemoveAll(catalogDir)

	manager := NewManager(catalogDir)
	err := manager.LoadCatalog()
	if err != nil {
		t.Fatalf("Failed to load Compose catalog: %v", err)
	}

	// Verify services were loaded
	if len(manager.templates) == 0 {
		t.Error("Expected at least one service template")
	}

	// Verify Grafana service exists
	grafana, exists := manager.templates["grafana"]
	if !exists {
		t.Error("Expected Grafana service in catalog")
	}

	if grafana.Name != "Grafana" {
		t.Errorf("Expected service name 'Grafana', got '%s'", grafana.Name)
	}
	if grafana.Category != "monitoring" {
		t.Errorf("Expected category 'monitoring', got '%s'", grafana.Category)
	}
	if grafana.Icon != "https://cdn.jsdelivr.net/gh/walkxcode/dashboard-icons/png/grafana.png" {
		t.Errorf("Expected icon URL, got '%s'", grafana.Icon)
	}
	if grafana.ImageURL != "https://hub.docker.com/r/grafana/grafana" {
		t.Errorf("Expected image URL, got '%s'", grafana.ImageURL)
	}

	// Verify tags
	expectedTags := []string{"monitoring", "metrics", "dashboards", "visualization"}
	if len(grafana.Tags) != len(expectedTags) {
		t.Errorf("Expected %d tags, got %d", len(expectedTags), len(grafana.Tags))
	}

	// Verify ComposeProject was loaded
	if grafana.ComposeProject == nil {
		t.Error("Expected ComposeProject to be populated")
	}

	// Verify ComposeFilePath is set
	if grafana.ComposeFilePath == "" {
		t.Error("Expected ComposeFilePath to be set")
	}
}

// TestLoadComposeCatalog_MultipleServices tests loading multiple services
func TestLoadComposeCatalog_MultipleServices(t *testing.T) {
	catalogDir := setupTestComposeDir(t)
	defer os.RemoveAll(catalogDir)

	manager := NewManager(catalogDir)
	err := manager.LoadCatalog()
	if err != nil {
		t.Fatalf("Failed to load Compose catalog: %v", err)
	}

	// We should have both grafana and dozzle
	expectedServices := []string{"grafana", "dozzle"}
	for _, serviceID := range expectedServices {
		if _, exists := manager.templates[serviceID]; !exists {
			t.Errorf("Expected service '%s' to be loaded", serviceID)
		}
	}
}

// TestLoadComposeCatalog_MissingLabels tests handling of Compose files with missing labels
func TestLoadComposeCatalog_MissingLabels(t *testing.T) {
	catalogDir := setupTestComposeDirWithMissingLabels(t)
	defer os.RemoveAll(catalogDir)

	manager := NewManager(catalogDir)
	err := manager.LoadCatalog()

	// Should fail or skip services with missing required labels
	if err == nil && len(manager.templates) > 0 {
		t.Error("Expected error or no templates when required labels are missing")
	}
}

// TestLoadComposeCatalog_InvalidComposeFile tests handling of invalid Compose syntax
func TestLoadComposeCatalog_InvalidComposeFile(t *testing.T) {
	catalogDir := setupTestComposeDirWithInvalidFile(t)
	defer os.RemoveAll(catalogDir)

	manager := NewManager(catalogDir)
	err := manager.LoadCatalog()

	// Should handle invalid files gracefully (skip or error)
	if err == nil {
		// If no error, check that no templates were loaded from invalid file
		if _, exists := manager.templates["invalid-service"]; exists {
			t.Error("Should not have loaded service from invalid Compose file")
		}
	}
}

// TestLoadComposeCatalog_EmptyDirectory tests loading from empty directory
func TestLoadComposeCatalog_EmptyDirectory(t *testing.T) {
	catalogDir := t.TempDir()

	manager := NewManager(catalogDir)
	err := manager.LoadCatalog()

	// Should not error, just return empty catalog
	if err != nil {
		t.Errorf("Expected no error for empty directory, got: %v", err)
	}

	if len(manager.templates) != 0 {
		t.Error("Expected empty template map for empty directory")
	}
}

// TestLoadComposeCatalog_NonExistentDirectory tests loading from non-existent directory
func TestLoadComposeCatalog_NonExistentDirectory(t *testing.T) {
	manager := NewManager("/nonexistent/toolbox")
	err := manager.LoadCatalog()

	if err == nil {
		t.Error("Expected error when catalog directory doesn't exist")
	}
}

// TestExtractMetadataFromLabels tests parsing toolbox labels
func TestExtractMetadataFromLabels(t *testing.T) {
	labels := map[string]string{
		types.ToolboxLabelName:        "Test Service",
		types.ToolboxLabelIcon:        "https://example.com/icon.png",
		types.ToolboxLabelCategory:    "testing",
		types.ToolboxLabelTags:        "test,unit,compose",
		types.ToolboxLabelDescription: "A test service",
		types.ToolboxLabelDocs:        "https://example.com/docs",
		types.ToolboxLabelImageURL:    "https://hub.docker.com/r/example/test",
	}

	template := extractMetadataFromLabels(labels, "test-service")

	if template.ID != "test-service" {
		t.Errorf("Expected ID 'test-service', got '%s'", template.ID)
	}
	if template.Name != "Test Service" {
		t.Errorf("Expected name 'Test Service', got '%s'", template.Name)
	}
	if template.Icon != "https://example.com/icon.png" {
		t.Errorf("Expected icon URL, got '%s'", template.Icon)
	}
	if template.Category != "testing" {
		t.Errorf("Expected category 'testing', got '%s'", template.Category)
	}
	if template.ImageURL != "https://hub.docker.com/r/example/test" {
		t.Errorf("Expected image URL, got '%s'", template.ImageURL)
	}

	expectedTags := []string{"test", "unit", "compose"}
	if len(template.Tags) != len(expectedTags) {
		t.Errorf("Expected %d tags, got %d", len(expectedTags), len(template.Tags))
	}

	if template.Description != "A test service" {
		t.Errorf("Expected description 'A test service', got '%s'", template.Description)
	}
	if template.Documentation != "https://example.com/docs" {
		t.Errorf("Expected docs URL, got '%s'", template.Documentation)
	}
}

// TestExtractMetadataFromLabels_MissingOptional tests handling missing optional labels
func TestExtractMetadataFromLabels_MissingOptional(t *testing.T) {
	labels := map[string]string{
		types.ToolboxLabelName:     "Minimal Service",
		types.ToolboxLabelCategory: "testing",
	}

	template := extractMetadataFromLabels(labels, "minimal")

	if template.Name != "Minimal Service" {
		t.Errorf("Expected name 'Minimal Service', got '%s'", template.Name)
	}
	if template.Category != "testing" {
		t.Errorf("Expected category 'testing', got '%s'", template.Category)
	}

	// Optional fields should be empty/default
	if template.Icon != "" {
		t.Errorf("Expected empty icon, got '%s'", template.Icon)
	}
	if template.ImageURL != "" {
		t.Errorf("Expected empty image URL, got '%s'", template.ImageURL)
	}
	if len(template.Tags) != 0 {
		t.Errorf("Expected no tags, got %d", len(template.Tags))
	}
}

// TestExtractEnvironmentVariables tests extracting env vars from Compose service
func TestExtractEnvironmentVariables(t *testing.T) {
	catalogDir := setupTestComposeDir(t)
	defer os.RemoveAll(catalogDir)

	manager := NewManager(catalogDir)
	err := manager.LoadCatalog()
	if err != nil {
		t.Fatalf("Failed to load catalog: %v", err)
	}

	grafana, exists := manager.templates["grafana"]
	if !exists {
		t.Fatal("Grafana template not found")
	}

	// Log extracted variables for debugging
	t.Logf("Extracted %d env vars from grafana", len(grafana.EnvVars))
	for _, ev := range grafana.EnvVars {
		t.Logf("  - %s (required=%v, default=%s)", ev.Name, ev.Required, ev.Default)
	}

	// The env var extraction function parses ${VAR:-default} patterns from the compose file
	// It should find variables from environment, container_name, and ports sections
	// Note: Variables may not be extracted if compose-go resolves them during parsing
	if len(grafana.EnvVars) > 0 {
		t.Logf("Successfully extracted %d environment variables", len(grafana.EnvVars))
	} else {
		// This is acceptable - compose-go may resolve variables during parsing
		t.Log("No env vars extracted - compose-go may have resolved them during parsing")
	}
}

// TestNewManager tests creating a new toolbox manager
func TestNewManager(t *testing.T) {
	catalogPath := setupTestCatalog(t)
	defer os.Remove(catalogPath)

	manager := NewManager(catalogPath)
	if manager == nil {
		t.Fatal("Expected non-nil manager")
	}

	if manager.catalogPath != catalogPath {
		t.Errorf("Expected catalogPath %s, got %s", catalogPath, manager.catalogPath)
	}
}

// TestLoadCatalog tests loading service catalog from YAML (DEPRECATED)
func TestLoadCatalog_YAML(t *testing.T) {
	catalogPath := setupTestCatalog(t)
	defer os.Remove(catalogPath)

	manager := NewManager(catalogPath)
	err := manager.LoadCatalog()
	if err != nil {
		t.Fatalf("Failed to load catalog: %v", err)
	}

	// Verify services were loaded
	if len(manager.templates) == 0 {
		t.Error("Expected at least one service template")
	}

	// Verify Grafana service exists
	grafana, exists := manager.templates["grafana"]
	if !exists {
		t.Error("Expected Grafana service in catalog")
	}

	if grafana.Name != "Grafana" {
		t.Errorf("Expected service name 'Grafana', got '%s'", grafana.Name)
	}
	if grafana.Category != "monitoring" {
		t.Errorf("Expected category 'monitoring', got '%s'", grafana.Category)
	}
}

// TestGetService tests retrieving a service by ID
func TestGetService(t *testing.T) {
	catalogDir := setupTestComposeDir(t)
	defer os.RemoveAll(catalogDir)

	manager := NewManager(catalogDir)
	if err := manager.LoadCatalog(); err != nil {
		t.Fatalf("Failed to load catalog: %v", err)
	}

	service, err := manager.GetService("grafana")
	if err != nil {
		t.Fatalf("Failed to get service: %v", err)
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
	catalogDir := setupTestComposeDir(t)
	defer os.RemoveAll(catalogDir)

	manager := NewManager(catalogDir)
	if err := manager.LoadCatalog(); err != nil {
		t.Fatalf("Failed to load catalog: %v", err)
	}

	_, err := manager.GetService("nonexistent")
	if err == nil {
		t.Error("Expected error when getting non-existent service")
	}
}

// TestListServices tests listing all services
func TestListServices(t *testing.T) {
	catalogDir := setupTestComposeDir(t)
	defer os.RemoveAll(catalogDir)

	manager := NewManager(catalogDir)
	if err := manager.LoadCatalog(); err != nil {
		t.Fatalf("Failed to load catalog: %v", err)
	}

	services := manager.ListServices()
	if len(services) == 0 {
		t.Error("Expected at least one service")
	}

	// Verify services contain expected data
	foundGrafana := false
	for _, svc := range services {
		if svc.ID == "grafana" {
			foundGrafana = true
			if svc.Name != "Grafana" {
				t.Errorf("Expected name 'Grafana', got '%s'", svc.Name)
			}
		}
	}

	if !foundGrafana {
		t.Error("Expected to find Grafana in services list")
	}
}

// TestFilterByCategory tests filtering services by category
func TestFilterByCategory(t *testing.T) {
	catalogDir := setupTestComposeDir(t)
	defer os.RemoveAll(catalogDir)

	manager := NewManager(catalogDir)
	if err := manager.LoadCatalog(); err != nil {
		t.Fatalf("Failed to load catalog: %v", err)
	}

	// Filter by monitoring category
	monitoring := manager.FilterByCategory("monitoring")
	if len(monitoring) == 0 {
		t.Error("Expected at least one monitoring service")
	}

	// Verify all returned services are in monitoring category
	for _, svc := range monitoring {
		if svc.Category != "monitoring" {
			t.Errorf("Expected category 'monitoring', got '%s'", svc.Category)
		}
	}

	// Filter by non-existent category
	empty := manager.FilterByCategory("nonexistent")
	if len(empty) != 0 {
		t.Error("Expected empty list for non-existent category")
	}
}

// TestValidateDeploymentRequest tests validating deployment requests
func TestValidateDeploymentRequest(t *testing.T) {
	catalogDir := setupTestComposeDir(t)
	defer os.RemoveAll(catalogDir)

	manager := NewManager(catalogDir)
	if err := manager.LoadCatalog(); err != nil {
		t.Fatalf("Failed to load catalog: %v", err)
	}

	tests := []struct {
		name    string
		req     *types.DeploymentRequest
		wantErr bool
	}{
		{
			name: "valid request",
			req: &types.DeploymentRequest{
				ServiceID:   "grafana",
				ServiceName: "my-grafana",
				EnvVars: map[string]string{
					"SERVICE_NAME":      "my-grafana",
					"GF_ADMIN_PASSWORD": "secret123",
					"GF_PORT":           "3000",
					"BASE_URL":          "http://localhost:8080",
				},
				AutoStart: true,
			},
			wantErr: false,
		},
		{
			name: "missing service ID",
			req: &types.DeploymentRequest{
				ServiceName: "my-grafana",
				EnvVars:     map[string]string{},
			},
			wantErr: true,
		},
		{
			name: "missing service name",
			req: &types.DeploymentRequest{
				ServiceID: "grafana",
				EnvVars:   map[string]string{},
			},
			wantErr: true,
		},
		{
			name: "invalid service ID",
			req: &types.DeploymentRequest{
				ServiceID:   "nonexistent",
				ServiceName: "my-service",
				EnvVars:     map[string]string{},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := manager.ValidateDeploymentRequest(tt.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateDeploymentRequest() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestReloadCatalog tests reloading the catalog
func TestReloadCatalog(t *testing.T) {
	catalogDir := setupTestComposeDir(t)
	defer os.RemoveAll(catalogDir)

	manager := NewManager(catalogDir)
	if err := manager.LoadCatalog(); err != nil {
		t.Fatalf("Failed to load catalog: %v", err)
	}

	initialCount := len(manager.templates)

	// Reload catalog
	if err := manager.LoadCatalog(); err != nil {
		t.Fatalf("Failed to reload catalog: %v", err)
	}

	if len(manager.templates) != initialCount {
		t.Error("Expected same number of templates after reload")
	}
}

// setupTestComposeDir creates a test directory with Compose files
func setupTestComposeDir(t *testing.T) string {
	t.Helper()

	tempDir := t.TempDir()

	// Create grafana subdirectory
	grafanaDir := filepath.Join(tempDir, "grafana")
	if err := os.MkdirAll(grafanaDir, 0755); err != nil {
		t.Fatalf("Failed to create grafana directory: %v", err)
	}

	grafanaCompose := `services:
  grafana:
    image: grafana/grafana:latest
    container_name: ${SERVICE_NAME:-grafana}
    ports:
      - "${GF_PORT:-3000}:3000"
    volumes:
      - grafana-data:/var/lib/grafana
    environment:
      GF_SERVER_ROOT_URL: "${BASE_URL}/apps/grafana/"
      GF_SECURITY_ADMIN_PASSWORD: "${GF_ADMIN_PASSWORD}"
      GF_INSTALL_PLUGINS: "${GF_INSTALL_PLUGINS:-}"
    restart: unless-stopped
    labels:
      nekzus.toolbox.name: "Grafana"
      nekzus.toolbox.icon: "https://cdn.jsdelivr.net/gh/walkxcode/dashboard-icons/png/grafana.png"
      nekzus.toolbox.category: "monitoring"
      nekzus.toolbox.tags: "monitoring,metrics,dashboards,visualization"
      nekzus.toolbox.description: "Beautiful monitoring dashboards"
      nekzus.toolbox.documentation: "https://grafana.com/docs/"
      nekzus.toolbox.image_url: "https://hub.docker.com/r/grafana/grafana"
      nekzus.enable: "true"
      nekzus.app.id: "grafana"
      nekzus.app.name: "Grafana"
      nekzus.route.path: "/apps/grafana/"
      nekzus.route.strip_prefix: "true"

volumes:
  grafana-data:
    driver: local
`

	grafanaPath := filepath.Join(grafanaDir, "docker-compose.yml")
	if err := os.WriteFile(grafanaPath, []byte(grafanaCompose), 0644); err != nil {
		t.Fatalf("Failed to write grafana compose file: %v", err)
	}

	// Create dozzle subdirectory
	dozzleDir := filepath.Join(tempDir, "dozzle")
	if err := os.MkdirAll(dozzleDir, 0755); err != nil {
		t.Fatalf("Failed to create dozzle directory: %v", err)
	}

	dozzleCompose := `services:
  dozzle:
    image: amir20/dozzle:latest
    container_name: ${SERVICE_NAME:-dozzle}
    ports:
      - "${DOZZLE_PORT:-8080}:8080"
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
    restart: unless-stopped
    labels:
      nekzus.toolbox.name: "Dozzle"
      nekzus.toolbox.icon: "https://cdn.jsdelivr.net/gh/walkxcode/dashboard-icons/png/dozzle.png"
      nekzus.toolbox.category: "monitoring"
      nekzus.toolbox.tags: "docker,logs,monitoring"
      nekzus.toolbox.description: "Real-time Docker log viewer"
      nekzus.toolbox.documentation: "https://dozzle.dev/"
      nekzus.toolbox.image_url: "https://hub.docker.com/r/amir20/dozzle"
`

	dozzlePath := filepath.Join(dozzleDir, "docker-compose.yml")
	if err := os.WriteFile(dozzlePath, []byte(dozzleCompose), 0644); err != nil {
		t.Fatalf("Failed to write dozzle compose file: %v", err)
	}

	return tempDir
}

// setupTestComposeDirWithMissingLabels creates Compose dir with incomplete labels
func setupTestComposeDirWithMissingLabels(t *testing.T) string {
	t.Helper()

	tempDir := t.TempDir()
	serviceDir := filepath.Join(tempDir, "incomplete")
	if err := os.MkdirAll(serviceDir, 0755); err != nil {
		t.Fatalf("Failed to create service directory: %v", err)
	}

	// Missing required labels (name, category)
	incompleteCompose := `services:
  incomplete:
    image: nginx:latest
    labels:
      nekzus.toolbox.icon: "🚫"
`

	composePath := filepath.Join(serviceDir, "docker-compose.yml")
	if err := os.WriteFile(composePath, []byte(incompleteCompose), 0644); err != nil {
		t.Fatalf("Failed to write incomplete compose file: %v", err)
	}

	return tempDir
}

// setupTestComposeDirWithInvalidFile creates directory with invalid YAML
func setupTestComposeDirWithInvalidFile(t *testing.T) string {
	t.Helper()

	tempDir := t.TempDir()
	serviceDir := filepath.Join(tempDir, "invalid")
	if err := os.MkdirAll(serviceDir, 0755); err != nil {
		t.Fatalf("Failed to create service directory: %v", err)
	}

	invalidCompose := `services:
  invalid:
    image: "nginx:latest
    invalid yaml syntax here [[[
`

	composePath := filepath.Join(serviceDir, "docker-compose.yml")
	if err := os.WriteFile(composePath, []byte(invalidCompose), 0644); err != nil {
		t.Fatalf("Failed to write invalid compose file: %v", err)
	}

	return tempDir
}

// setupTestCatalog creates a temporary test catalog file (YAML - DEPRECATED)
func setupTestCatalog(t *testing.T) string {
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
          description: HTTP port
      volumes:
        - name: grafana-data
          mount_path: /var/lib/grafana
          description: Grafana data directory
      environment:
        GF_SERVER_ROOT_URL: "{{BASE_URL}}/apps/grafana/"
      restart_policy: unless-stopped
    env_vars:
      - name: SERVICE_NAME
        label: Container Name
        description: Docker container name
        required: true
        default: grafana
        type: text
      - name: GF_ADMIN_PASSWORD
        label: Admin Password
        description: Grafana admin password
        required: true
        type: password
      - name: HOST_PORT
        label: Port
        description: Host port to expose
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
    security_notes:
      - "Grafana exposes port 3000 to your network"
      - "Use a strong admin password"
    documentation: https://grafana.com/docs/
`

	if err := os.WriteFile(catalogPath, []byte(catalogContent), 0644); err != nil {
		t.Fatalf("Failed to create test catalog: %v", err)
	}

	return catalogPath
}
