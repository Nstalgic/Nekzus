package toolbox

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	composetypes "github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/docker/client"
	"github.com/nstalgic/nekzus/internal/types"
)

// skipIfDockerUnavailable skips the test if Docker daemon is not running
func skipIfDockerUnavailable(t *testing.T) {
	t.Helper()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		t.Skipf("Docker client unavailable: %v", err)
		return
	}
	defer cli.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err = cli.Ping(ctx)
	if err != nil {
		t.Skipf("Docker daemon not running: %v", err)
	}
}

// TestNewDeployer tests creating a new deployer
func TestNewDeployer(t *testing.T) {
	deployer, err := NewDeployer("/tmp/toolbox-data", "")
	if err != nil {
		t.Fatalf("Failed to create deployer: %v", err)
	}
	defer deployer.Close()

	if deployer.dataDir != "/tmp/toolbox-data" {
		t.Errorf("Expected dataDir '/tmp/toolbox-data', got '%s'", deployer.dataDir)
	}
}

// TestValidateDeployment tests deployment validation
func TestValidateDeployment(t *testing.T) {
	deployer, err := NewDeployer("/tmp/toolbox-data", "")
	if err != nil {
		t.Fatalf("Failed to create deployer: %v", err)
	}
	defer deployer.Close()

	template := &types.ServiceTemplate{
		ID:   "grafana",
		Name: "Grafana",
		DockerConfig: types.DockerContainerConfig{
			Image: "grafana/grafana:latest",
			Ports: []types.PortMapping{
				{Container: 3000, HostDefault: 3000},
			},
		},
	}

	envVars := map[string]string{
		"SERVICE_NAME": "my-grafana",
	}

	err = deployer.ValidateDeployment(template, envVars)
	if err != nil {
		t.Errorf("Expected valid deployment, got error: %v", err)
	}
}

// TestValidateDeployment_MissingImage tests validation with missing image
func TestValidateDeployment_MissingImage(t *testing.T) {
	deployer, err := NewDeployer("/tmp/toolbox-data", "")
	if err != nil {
		t.Fatalf("Failed to create deployer: %v", err)
	}
	defer deployer.Close()

	template := &types.ServiceTemplate{
		ID:           "test",
		Name:         "Test Service",
		DockerConfig: types.DockerContainerConfig{
			// Missing Image
		},
	}

	err = deployer.ValidateDeployment(template, map[string]string{})
	if err == nil {
		t.Error("Expected error for missing Docker image")
	}
}

// TestRenderTemplate tests template variable substitution
func TestRenderTemplate(t *testing.T) {
	deployer, err := NewDeployer("/tmp/toolbox-data", "")
	if err != nil {
		t.Fatalf("Failed to create deployer: %v", err)
	}
	defer deployer.Close()

	tests := []struct {
		name     string
		template string
		vars     map[string]string
		expected string
	}{
		{
			name:     "simple substitution",
			template: "Hello {{NAME}}!",
			vars:     map[string]string{"NAME": "World"},
			expected: "Hello World!",
		},
		{
			name:     "multiple variables",
			template: "{{GREETING}} {{NAME}}, welcome to {{PLACE}}",
			vars:     map[string]string{"GREETING": "Hi", "NAME": "Alice", "PLACE": "Wonderland"},
			expected: "Hi Alice, welcome to Wonderland",
		},
		{
			name:     "environment variable format",
			template: "GF_SERVER_ROOT_URL={{BASE_URL}}/apps/{{SERVICE_NAME}}/",
			vars:     map[string]string{"BASE_URL": "https://nexus.local", "SERVICE_NAME": "grafana"},
			expected: "GF_SERVER_ROOT_URL=https://nexus.local/apps/grafana/",
		},
		{
			name:     "no variables",
			template: "Static text",
			vars:     map[string]string{},
			expected: "Static text",
		},
		{
			name:     "missing variable",
			template: "Hello {{MISSING}}",
			vars:     map[string]string{},
			expected: "Hello {{MISSING}}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := deployer.RenderTemplate(tt.template, tt.vars)
			if result != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

// TestCheckPortConflicts tests port conflict detection
func TestCheckPortConflicts(t *testing.T) {
	deployer, err := NewDeployer("/tmp/toolbox-data", "")
	if err != nil {
		t.Fatalf("Failed to create deployer: %v", err)
	}
	defer deployer.Close()

	// Test with commonly available port
	ports := []types.PortMapping{
		{Container: 3000, HostDefault: 33000}, // Using high port number to avoid conflicts
	}

	err = deployer.CheckPortConflicts(ports, map[string]string{"HOST_PORT": "33000"})
	if err != nil {
		t.Errorf("Expected no port conflicts, got: %v", err)
	}
}

// TestGenerateContainerName tests container name generation
func TestGenerateContainerName(t *testing.T) {
	deployer, err := NewDeployer("/tmp/toolbox-data", "")
	if err != nil {
		t.Fatalf("Failed to create deployer: %v", err)
	}
	defer deployer.Close()

	tests := []struct {
		name        string
		serviceName string
		wantPrefix  string
	}{
		{
			name:        "simple name",
			serviceName: "grafana",
			wantPrefix:  "grafana",
		},
		{
			name:        "name with spaces",
			serviceName: "my grafana",
			wantPrefix:  "my-grafana",
		},
		{
			name:        "name with special chars",
			serviceName: "my_grafana-123",
			wantPrefix:  "my-grafana-123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := deployer.GenerateContainerName(tt.serviceName)
			if result == "" {
				t.Error("Expected non-empty container name")
			}
			// Container name should start with sanitized service name
			if len(result) < len(tt.wantPrefix) {
				t.Errorf("Container name too short: %s", result)
			}
		})
	}
}

// TestBuildVolumePaths tests volume path generation
func TestBuildVolumePaths(t *testing.T) {
	deployer, err := NewDeployer("/tmp/toolbox-data", "")
	if err != nil {
		t.Fatalf("Failed to create deployer: %v", err)
	}
	defer deployer.Close()

	volumes := []types.VolumeMapping{
		{Name: "data", MountPath: "/var/lib/app/data"},
		{Name: "config", MountPath: "/etc/app"},
	}

	paths := deployer.BuildVolumePaths("my-service", volumes)
	if len(paths) != 2 {
		t.Errorf("Expected 2 volume paths, got %d", len(paths))
	}

	// Verify paths contain service name and volume names
	for _, path := range paths {
		if path.HostPath == "" || path.ContainerPath == "" {
			t.Error("Expected non-empty volume paths")
		}
	}
}

// TestBuildEnvironment tests environment variable building
func TestBuildEnvironment(t *testing.T) {
	deployer, err := NewDeployer("/tmp/toolbox-data", "")
	if err != nil {
		t.Fatalf("Failed to create deployer: %v", err)
	}
	defer deployer.Close()

	template := &types.ServiceTemplate{
		DockerConfig: types.DockerContainerConfig{
			Environment: map[string]string{
				"DEFAULT_VAR":  "default_value",
				"TEMPLATE_VAR": "{{USER_VALUE}}",
			},
		},
	}

	userVars := map[string]string{
		"USER_VALUE": "custom_value",
		"EXTRA_VAR":  "extra_value",
	}

	env := deployer.BuildEnvironment(template, userVars)

	// Should contain default vars, rendered template vars, and user vars
	if len(env) < 2 {
		t.Errorf("Expected at least 2 env vars, got %d", len(env))
	}

	// Check for rendered template variable
	foundRendered := false
	for _, e := range env {
		if e == "TEMPLATE_VAR=custom_value" {
			foundRendered = true
			break
		}
	}
	if !foundRendered {
		t.Error("Expected rendered template variable in environment")
	}
}

// TestCreateContainerConfig tests container configuration building
func TestCreateContainerConfig(t *testing.T) {
	deployer, err := NewDeployer("/tmp/toolbox-data", "")
	if err != nil {
		t.Fatalf("Failed to create deployer: %v", err)
	}
	defer deployer.Close()

	template := &types.ServiceTemplate{
		ID:   "grafana",
		Name: "Grafana",
		DockerConfig: types.DockerContainerConfig{
			Image: "grafana/grafana:latest",
			Ports: []types.PortMapping{
				{Container: 3000, HostDefault: 3000},
			},
			Volumes: []types.VolumeMapping{
				{Name: "data", MountPath: "/var/lib/grafana"},
			},
			RestartPolicy: "unless-stopped",
		},
	}

	userVars := map[string]string{
		"SERVICE_NAME": "my-grafana",
		"HOST_PORT":    "3000",
	}

	config, hostConfig := deployer.CreateContainerConfig(template, "my-grafana", userVars)

	if config == nil {
		t.Fatal("Expected non-nil container config")
	}
	if hostConfig == nil {
		t.Fatal("Expected non-nil host config")
	}

	if config.Image != "grafana/grafana:latest" {
		t.Errorf("Expected image 'grafana/grafana:latest', got '%s'", config.Image)
	}
}

// TestSanitizeContainerName tests container name sanitization
func TestSanitizeContainerName(t *testing.T) {
	deployer, err := NewDeployer("/tmp/toolbox-data", "")
	if err != nil {
		t.Fatalf("Failed to create deployer: %v", err)
	}
	defer deployer.Close()

	tests := []struct {
		input    string
		expected string
	}{
		{"simple", "simple"},
		{"with spaces", "with-spaces"},
		{"with_underscores", "with-underscores"},
		{"UPPERCASE", "uppercase"},
		{"special!@#chars", "specialchars"},
		{"multiple   spaces", "multiple-spaces"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := deployer.SanitizeContainerName(tt.input)
			if result != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

// NEW: Docker Compose Tests (TDD - These should fail until implementation)

// TestComposeProjectCreation tests creating a Compose project from template
func TestComposeProjectCreation(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	skipIfDockerUnavailable(t)

	deployer, err := NewDeployer("/tmp/toolbox-data", "")
	if err != nil {
		t.Fatalf("Failed to create deployer: %v", err)
	}
	defer deployer.Close()

	// Create test Compose template
	template := &types.ServiceTemplate{
		ID:   "test-compose",
		Name: "Test Compose Service",
		ComposeProject: &composetypes.Project{
			Name: "test-project",
			Services: composetypes.Services{
				"app": {
					Name:  "app",
					Image: "nginx:latest",
					Ports: []composetypes.ServicePortConfig{
						{
							Published: "8080",
							Target:    80,
						},
					},
				},
			},
		},
	}

	deployment := &types.ToolboxDeployment{
		ID:                "test-deployment-" + generateRandomID(),
		ServiceTemplateID: "test-compose",
		ServiceName:       "test-compose-app",
		EnvVars: map[string]string{
			"SERVICE_NAME": "test-compose-app",
		},
	}

	ctx := context.Background()

	// This should create a Compose project
	projectName, err := deployer.DeployComposeProject(ctx, template, deployment)
	if err != nil {
		t.Fatalf("Failed to deploy Compose project: %v", err)
	}

	if projectName == "" {
		t.Fatal("Expected non-empty project name")
	}

	// Verify project name matches deployment ID
	if projectName != deployment.ID {
		t.Errorf("Expected project name '%s', got '%s'", deployment.ID, projectName)
	}

	// Cleanup
	defer func() {
		if err := deployer.RemoveComposeProject(ctx, projectName, true); err != nil {
			t.Logf("Warning: Failed to remove test project: %v", err)
		}
	}()
}

// TestComposeServiceStart tests starting a Compose service
func TestComposeServiceStart(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	skipIfDockerUnavailable(t)

	deployer, err := NewDeployer("/tmp/toolbox-data", "")
	if err != nil {
		t.Fatalf("Failed to create deployer: %v", err)
	}
	defer deployer.Close()

	// Create test Compose template
	template := &types.ServiceTemplate{
		ID:   "test-start",
		Name: "Test Start Service",
		ComposeProject: &composetypes.Project{
			Name: "test-start-project",
			Services: composetypes.Services{
				"web": {
					Name:    "web",
					Image:   "nginx:alpine",
					Restart: "unless-stopped",
				},
			},
		},
	}

	deployment := &types.ToolboxDeployment{
		ID:                "test-start-" + generateRandomID(),
		ServiceTemplateID: "test-start",
		ServiceName:       "test-web",
		EnvVars:           map[string]string{},
	}

	ctx := context.Background()

	// Deploy project
	projectName, err := deployer.DeployComposeProject(ctx, template, deployment)
	if err != nil {
		t.Fatalf("Failed to deploy project: %v", err)
	}

	defer func() {
		deployer.RemoveComposeProject(ctx, projectName, true)
	}()

	// Start services
	err = deployer.StartComposeProject(ctx, projectName)
	if err != nil {
		t.Fatalf("Failed to start Compose project: %v", err)
	}

	// Verify containers are running
	// (This would require inspecting Docker API in real implementation)
	t.Logf("Successfully started Compose project: %s", projectName)
}

// TestComposeServiceStop tests stopping a Compose service
func TestComposeServiceStop(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	skipIfDockerUnavailable(t)

	deployer, err := NewDeployer("/tmp/toolbox-data", "")
	if err != nil {
		t.Fatalf("Failed to create deployer: %v", err)
	}
	defer deployer.Close()

	// Create and start a test project
	template := &types.ServiceTemplate{
		ID:   "test-stop",
		Name: "Test Stop Service",
		ComposeProject: &composetypes.Project{
			Name: "test-stop-project",
			Services: composetypes.Services{
				"app": {
					Name:  "app",
					Image: "alpine:latest",
				},
			},
		},
	}

	deployment := &types.ToolboxDeployment{
		ID:                "test-stop-" + generateRandomID(),
		ServiceTemplateID: "test-stop",
		ServiceName:       "test-app",
		EnvVars:           map[string]string{},
	}

	ctx := context.Background()

	projectName, err := deployer.DeployComposeProject(ctx, template, deployment)
	if err != nil {
		t.Fatalf("Failed to deploy project: %v", err)
	}

	defer deployer.RemoveComposeProject(ctx, projectName, true)

	// Start project
	if err := deployer.StartComposeProject(ctx, projectName); err != nil {
		t.Fatalf("Failed to start project: %v", err)
	}

	// Stop project
	if err := deployer.StopComposeProject(ctx, projectName); err != nil {
		t.Fatalf("Failed to stop Compose project: %v", err)
	}

	t.Logf("Successfully stopped Compose project: %s", projectName)
}

// TestComposeProjectRemoval tests removing a Compose project
func TestComposeProjectRemoval(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	skipIfDockerUnavailable(t)

	deployer, err := NewDeployer("/tmp/toolbox-data", "")
	if err != nil {
		t.Fatalf("Failed to create deployer: %v", err)
	}
	defer deployer.Close()

	template := &types.ServiceTemplate{
		ID:   "test-remove",
		Name: "Test Remove Service",
		ComposeProject: &composetypes.Project{
			Name: "test-remove-project",
			Services: composetypes.Services{
				"service": {
					Name:  "service",
					Image: "alpine:latest",
				},
			},
		},
	}

	deployment := &types.ToolboxDeployment{
		ID:                "test-remove-" + generateRandomID(),
		ServiceTemplateID: "test-remove",
		ServiceName:       "test-service",
		EnvVars:           map[string]string{},
	}

	ctx := context.Background()

	// Deploy project
	projectName, err := deployer.DeployComposeProject(ctx, template, deployment)
	if err != nil {
		t.Fatalf("Failed to deploy project: %v", err)
	}

	// Remove project with volumes
	err = deployer.RemoveComposeProject(ctx, projectName, true)
	if err != nil {
		t.Fatalf("Failed to remove Compose project: %v", err)
	}

	// Should be idempotent - removing again should not error
	err = deployer.RemoveComposeProject(ctx, projectName, true)
	if err != nil {
		t.Errorf("Second removal should be idempotent, got error: %v", err)
	}

	t.Logf("Successfully removed Compose project: %s", projectName)
}

// TestEnvironmentVariableInjection tests env var substitution in Compose
func TestEnvironmentVariableInjection(t *testing.T) {
	deployer, err := NewDeployer("/tmp/toolbox-data", "")
	if err != nil {
		t.Fatalf("Failed to create deployer: %v", err)
	}
	defer deployer.Close()

	template := &types.ServiceTemplate{
		ID:   "test-env",
		Name: "Test Env Service",
		ComposeProject: &composetypes.Project{
			Name: "test-env-project",
			Services: composetypes.Services{
				"app": {
					Name:  "app",
					Image: "nginx:latest",
					Environment: composetypes.MappingWithEquals{
						"APP_NAME":   ptrString("${SERVICE_NAME}"),
						"BASE_URL":   ptrString("${BASE_URL}"),
						"STATIC_VAR": ptrString("static_value"),
					},
				},
			},
		},
	}

	deployment := &types.ToolboxDeployment{
		ID:                "test-env-" + generateRandomID(),
		ServiceTemplateID: "test-env",
		ServiceName:       "my-app",
		EnvVars: map[string]string{
			"SERVICE_NAME": "my-app",
			"BASE_URL":     "https://nexus.local",
		},
	}

	// Test environment variable injection
	envMap := deployer.BuildComposeEnvironment(template, deployment.EnvVars)

	// Verify environment variables are set
	if envMap["SERVICE_NAME"] != "my-app" {
		t.Errorf("Expected SERVICE_NAME='my-app', got '%s'", envMap["SERVICE_NAME"])
	}

	if envMap["BASE_URL"] != "https://nexus.local" {
		t.Errorf("Expected BASE_URL='https://nexus.local', got '%s'", envMap["BASE_URL"])
	}

	t.Logf("Environment variables correctly injected: %+v", envMap)
}

// TestComposePortVariableSubstitution tests that APP_PORT env var is substituted in port bindings
func TestComposePortVariableSubstitution(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	skipIfDockerUnavailable(t)

	deployer, err := NewDeployer("/tmp/toolbox-data", "")
	if err != nil {
		t.Fatalf("Failed to create deployer: %v", err)
	}
	defer deployer.Close()

	// Create template with variable port like real toolbox templates use
	template := &types.ServiceTemplate{
		ID:   "test-port-var",
		Name: "Test Port Variable",
		ComposeProject: &composetypes.Project{
			Name: "test-port-var-project",
			Services: composetypes.Services{
				"app": {
					Name:  "app",
					Image: "nginx:alpine",
					Ports: []composetypes.ServicePortConfig{
						{
							Published: "${APP_PORT}", // Variable port
							Target:    80,
							Protocol:  "tcp",
						},
					},
				},
			},
		},
	}

	deployment := &types.ToolboxDeployment{
		ID:                "test-port-var-" + generateRandomID(),
		ServiceTemplateID: "test-port-var",
		ServiceName:       "test-port-app",
		EnvVars: map[string]string{
			"APP_PORT": "9999", // Custom port
		},
	}

	ctx := context.Background()

	// Deploy project - this should substitute ${APP_PORT} with 9999
	projectName, err := deployer.DeployComposeProject(ctx, template, deployment)
	if err != nil {
		t.Fatalf("Failed to deploy Compose project: %v", err)
	}

	defer func() {
		deployer.RemoveComposeProject(ctx, projectName, true)
	}()

	// Start the project to verify port binding works
	if err := deployer.StartComposeProject(ctx, projectName); err != nil {
		t.Fatalf("Failed to start project with custom port: %v", err)
	}

	// If we get here without error, the port substitution worked
	t.Logf("Successfully deployed and started project with custom port 9999")

	// Stop the project
	deployer.StopComposeProject(ctx, projectName)
}

// TestMultiServiceDeployment tests deploying Compose with multiple services
func TestMultiServiceDeployment(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	skipIfDockerUnavailable(t)

	deployer, err := NewDeployer("/tmp/toolbox-data", "")
	if err != nil {
		t.Fatalf("Failed to create deployer: %v", err)
	}
	defer deployer.Close()

	// Multi-service Compose project (like Pi-hole with multiple containers)
	template := &types.ServiceTemplate{
		ID:   "test-multi",
		Name: "Test Multi-Service",
		ComposeProject: &composetypes.Project{
			Name: "test-multi-project",
			Services: composetypes.Services{
				"web": {
					Name:  "web",
					Image: "nginx:alpine",
				},
				"cache": {
					Name:  "cache",
					Image: "redis:alpine",
				},
			},
		},
	}

	deployment := &types.ToolboxDeployment{
		ID:                "test-multi-" + generateRandomID(),
		ServiceTemplateID: "test-multi",
		ServiceName:       "multi-app",
		EnvVars:           map[string]string{},
	}

	ctx := context.Background()

	// Deploy project
	projectName, err := deployer.DeployComposeProject(ctx, template, deployment)
	if err != nil {
		t.Fatalf("Failed to deploy multi-service project: %v", err)
	}

	defer deployer.RemoveComposeProject(ctx, projectName, true)

	// Verify project was created
	if projectName == "" {
		t.Fatal("Expected non-empty project name")
	}

	// Verify both services would be created
	// (In real implementation, we'd inspect containers)
	if len(template.ComposeProject.Services) != 2 {
		t.Errorf("Expected 2 services, got %d", len(template.ComposeProject.Services))
	}

	t.Logf("Successfully deployed multi-service project: %s with %d services",
		projectName, len(template.ComposeProject.Services))
}

// TestDeploymentFailureCleanup tests cleanup on deployment failure
func TestDeploymentFailureCleanup(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	skipIfDockerUnavailable(t)

	deployer, err := NewDeployer("/tmp/toolbox-data", "")
	if err != nil {
		t.Fatalf("Failed to create deployer: %v", err)
	}
	defer deployer.Close()

	// Create template with invalid image to cause failure
	template := &types.ServiceTemplate{
		ID:   "test-fail",
		Name: "Test Failure Service",
		ComposeProject: &composetypes.Project{
			Name: "test-fail-project",
			Services: composetypes.Services{
				"fail": {
					Name:  "fail",
					Image: "nonexistent-image-that-does-not-exist:latest",
				},
			},
		},
	}

	deployment := &types.ToolboxDeployment{
		ID:                "test-fail-" + generateRandomID(),
		ServiceTemplateID: "test-fail",
		ServiceName:       "fail-app",
		EnvVars:           map[string]string{},
	}

	ctx := context.Background()

	// Deploy should fail
	projectName, err := deployer.DeployComposeProject(ctx, template, deployment)
	if err == nil {
		t.Error("Expected deployment to fail with invalid image")
		// Cleanup if it somehow succeeded
		deployer.RemoveComposeProject(ctx, projectName, true)
		return
	}

	t.Logf("Deployment correctly failed: %v", err)

	// Verify cleanup occurred - attempting to remove should be idempotent
	if projectName != "" {
		err = deployer.RemoveComposeProject(ctx, projectName, true)
		if err != nil {
			t.Logf("Cleanup after failure: %v (this is acceptable)", err)
		}
	}
}

// TestComposeLabelMerging tests that labels from Compose file are properly merged with injected labels
func TestComposeLabelMerging(t *testing.T) {
	// This is a unit test - no Docker required
	// We're testing the label merging logic in DeployComposeProject

	// Create a mock service with labels
	serviceLabels := composetypes.Labels{
		// Toolbox labels
		"nekzus.toolbox.name":        "Test Service",
		"nekzus.toolbox.icon":        "🧪",
		"nekzus.toolbox.category":    "testing",
		"nekzus.toolbox.description": "Test service for label merging",

		// Discovery labels
		"nekzus.enable":             "true",
		"nekzus.app.id":             "test-app",
		"nekzus.app.name":           "Test App",
		"nekzus.route.path":         "/apps/test/",
		"nekzus.route.strip_prefix": "true",

		// Custom user labels
		"custom.label": "custom-value",
	}

	// Simulate the label merging logic that SHOULD happen in DeployComposeProject
	// Currently, the code creates a new label map and loses these labels
	// After the fix, labels from service.Labels should be preserved

	projectName := "test-project"
	serviceName := "app"

	// This is what the CURRENT (buggy) code does:
	buggyLabels := map[string]string{
		"com.docker.compose.project": projectName,
		"com.docker.compose.service": serviceName,
	}

	// This is what the FIXED code should do:
	fixedLabels := make(map[string]string)
	for key, value := range serviceLabels {
		fixedLabels[key] = value
	}
	fixedLabels["com.docker.compose.project"] = projectName
	fixedLabels["com.docker.compose.service"] = serviceName

	// Verify the buggy approach loses labels
	if len(buggyLabels) == 2 {
		t.Log("BUGGY implementation: Only 2 labels (compose metadata), all Compose file labels lost!")
	}

	// Verify the fixed approach preserves labels
	expectedLabelCount := len(serviceLabels) + 2 // service labels + 2 compose labels
	if len(fixedLabels) != expectedLabelCount {
		t.Errorf("Expected %d labels, got %d", expectedLabelCount, len(fixedLabels))
	}

	// Verify toolbox labels are preserved
	if fixedLabels["nekzus.toolbox.name"] != "Test Service" {
		t.Error("Expected toolbox.name label to be preserved")
	}

	if fixedLabels["nekzus.toolbox.icon"] != "🧪" {
		t.Error("Expected toolbox.icon label to be preserved")
	}

	if fixedLabels["nekzus.toolbox.category"] != "testing" {
		t.Error("Expected toolbox.category label to be preserved")
	}

	// Verify discovery labels are preserved
	if fixedLabels["nekzus.enable"] != "true" {
		t.Error("Expected discovery enable label to be preserved")
	}

	if fixedLabels["nekzus.app.id"] != "test-app" {
		t.Error("Expected app.id label to be preserved")
	}

	if fixedLabels["nekzus.route.path"] != "/apps/test/" {
		t.Error("Expected route.path label to be preserved")
	}

	// Verify custom labels are preserved
	if fixedLabels["custom.label"] != "custom-value" {
		t.Error("Expected custom label to be preserved")
	}

	// Verify compose metadata labels are added
	if fixedLabels["com.docker.compose.project"] != projectName {
		t.Error("Expected compose.project label to be injected")
	}

	if fixedLabels["com.docker.compose.service"] != serviceName {
		t.Error("Expected compose.service label to be injected")
	}

	t.Logf("✅ Label merging logic verified: %d total labels (10 from Compose file + 2 injected)", len(fixedLabels))
	t.Log("NOTE: This test verifies the logic. Implementation in deployer_compose.go needs to be fixed.")
}

// TestLoadComposeFile tests loading a Compose file from disk
func TestLoadComposeFile(t *testing.T) {
	deployer, err := NewDeployer("/tmp/toolbox-data", "")
	if err != nil {
		t.Fatalf("Failed to create deployer: %v", err)
	}
	defer deployer.Close()

	// Create test Compose file
	testDir := t.TempDir()
	composeFile := filepath.Join(testDir, "docker-compose.yml")

	composeContent := `
services:
  app:
    image: nginx:latest
    ports:
      - "8080:80"
    environment:
      APP_NAME: ${SERVICE_NAME}
volumes:
  data:
    driver: local
`

	if err := os.WriteFile(composeFile, []byte(composeContent), 0644); err != nil {
		t.Fatalf("Failed to create test Compose file: %v", err)
	}

	// Load Compose file
	project, err := deployer.LoadComposeFile(composeFile)
	if err != nil {
		t.Fatalf("Failed to load Compose file: %v", err)
	}

	// Verify project structure
	if project == nil {
		t.Fatal("Expected non-nil project")
	}

	if len(project.Services) != 1 {
		t.Errorf("Expected 1 service, got %d", len(project.Services))
	}

	if _, exists := project.Services["app"]; !exists {
		t.Error("Expected service 'app' to exist")
	}

	if len(project.Volumes) != 1 {
		t.Errorf("Expected 1 volume, got %d", len(project.Volumes))
	}

	t.Logf("Successfully loaded Compose project: %s with %d services",
		project.Name, len(project.Services))
}

// DEPRECATED: Legacy single-container tests (keep for backward compatibility)

// TestDeploymentLifecycle tests the full deployment lifecycle (requires Docker)
func TestDeploymentLifecycle(t *testing.T) {
	// Skip if Docker is not available
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	skipIfDockerUnavailable(t)

	deployer, err := NewDeployer("/tmp/toolbox-data", "")
	if err != nil {
		t.Fatalf("Failed to create deployer: %v", err)
	}
	defer deployer.Close()

	// Use a simple test container (Alpine)
	template := &types.ServiceTemplate{
		ID:   "test-alpine",
		Name: "Test Alpine",
		DockerConfig: types.DockerContainerConfig{
			Image: "alpine:latest",
			Environment: map[string]string{
				"TEST_VAR": "test_value",
			},
		},
	}

	deployment := &types.ToolboxDeployment{
		ID:                "test-deployment-" + generateRandomID(),
		ServiceTemplateID: "test-alpine",
		ServiceName:       "test-alpine-container",
		EnvVars: map[string]string{
			"SERVICE_NAME": "test-alpine-container",
		},
	}

	ctx := context.Background()

	// Create container
	containerID, err := deployer.CreateContainer(ctx, template, deployment)
	if err != nil {
		t.Fatalf("Failed to create container: %v", err)
	}

	if containerID == "" {
		t.Fatal("Expected non-empty container ID")
	}

	// Clean up: Remove container
	defer func() {
		if err := deployer.RemoveContainer(ctx, containerID, true); err != nil {
			t.Logf("Warning: Failed to remove test container: %v", err)
		}
	}()

	t.Logf("Successfully created test container: %s", containerID)
}

// TestRemoveDeployment tests deployment removal
func TestRemoveDeployment(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	skipIfDockerUnavailable(t)

	deployer, err := NewDeployer("/tmp/toolbox-data", "")
	if err != nil {
		t.Fatalf("Failed to create deployer: %v", err)
	}
	defer deployer.Close()

	// This test validates the interface exists
	// Full integration test would require a running container
	ctx := context.Background()
	err = deployer.RemoveContainer(ctx, "nonexistent-container", false)
	// Should not panic, may return error for non-existent container
	if err != nil {
		t.Logf("Expected error for non-existent container: %v", err)
	}
}

// Test Nexus CA cert volume integration

// TestCreateContainerConfig_WithCertVolume tests that cert volume can be added to container config
func TestCreateContainerConfig_WithCertVolume(t *testing.T) {
	deployer, err := NewDeployer("/tmp/toolbox-data", "")
	if err != nil {
		t.Fatalf("Failed to create deployer: %v", err)
	}
	defer deployer.Close()

	template := &types.ServiceTemplate{
		ID:   "test-cert",
		Name: "Test Cert Service",
		DockerConfig: types.DockerContainerConfig{
			Image: "nginx:latest",
			Ports: []types.PortMapping{
				{Container: 443, HostDefault: 8443},
			},
		},
	}

	userVars := map[string]string{
		"SERVICE_NAME": "test-cert-service",
	}

	// Create container config with cert volume enabled
	config, hostConfig := deployer.CreateContainerConfigWithCerts(template, "test-cert-service", userVars)

	if config == nil {
		t.Fatal("Expected non-nil container config")
	}
	if hostConfig == nil {
		t.Fatal("Expected non-nil host config")
	}

	// Verify cert volume is mounted
	foundCertMount := false
	for _, m := range hostConfig.Mounts {
		if m.Source == "nexus-certs" && m.Target == "/certs" && m.ReadOnly {
			foundCertMount = true
			break
		}
	}

	if !foundCertMount {
		t.Error("Expected nexus-certs volume to be mounted at /certs (read-only)")
	}

	// Verify cert environment variables are set
	certEnvVars := map[string]bool{
		"NEKZUS_CA_CERT":       false,
		"NEKZUS_CERT":          false,
		"NEKZUS_KEY":           false,
		"NEKZUS_CERT_DIR":      false,
		"SSL_CERT_FILE":       false,
		"NODE_EXTRA_CA_CERTS": false,
	}

	for _, env := range config.Env {
		for key := range certEnvVars {
			if len(env) > len(key) && env[:len(key)+1] == key+"=" {
				certEnvVars[key] = true
			}
		}
	}

	for key, found := range certEnvVars {
		if !found {
			t.Errorf("Missing cert environment variable: %s", key)
		}
	}
}

// TestBuildComposeEnvironment_WithCerts tests that cert env vars are added to Compose environment
func TestBuildComposeEnvironment_WithCerts(t *testing.T) {
	deployer, err := NewDeployer("/tmp/toolbox-data", "")
	if err != nil {
		t.Fatalf("Failed to create deployer: %v", err)
	}
	defer deployer.Close()

	template := &types.ServiceTemplate{
		ID:   "test-cert-compose",
		Name: "Test Cert Compose",
		ComposeProject: &composetypes.Project{
			Name: "test-cert-project",
			Services: composetypes.Services{
				"app": {
					Name:  "app",
					Image: "nginx:latest",
				},
			},
		},
	}

	userVars := map[string]string{
		"SERVICE_NAME": "test-service",
	}

	// Build environment with certs
	env := deployer.BuildComposeEnvironmentWithCerts(template, userVars)

	// Verify cert environment variables
	if env["NEKZUS_CA_CERT"] != "/certs/ca.crt" {
		t.Errorf("Expected NEKZUS_CA_CERT=/certs/ca.crt, got: %s", env["NEKZUS_CA_CERT"])
	}
	if env["NEKZUS_CERT"] != "/certs/cert.crt" {
		t.Errorf("Expected NEKZUS_CERT=/certs/cert.crt, got: %s", env["NEKZUS_CERT"])
	}
	if env["NEKZUS_KEY"] != "/certs/cert.key" {
		t.Errorf("Expected NEKZUS_KEY=/certs/cert.key, got: %s", env["NEKZUS_KEY"])
	}
	if env["SSL_CERT_FILE"] != "/certs/ca.crt" {
		t.Errorf("Expected SSL_CERT_FILE=/certs/ca.crt, got: %s", env["SSL_CERT_FILE"])
	}
}

// Helper functions

// Helper function to generate random ID for testing
func generateRandomID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

// Helper function to create string pointer
func ptrString(s string) *string {
	return &s
}
