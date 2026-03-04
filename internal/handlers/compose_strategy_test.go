package handlers

import (
	"context"
	"errors"
	"testing"

	"github.com/nstalgic/nekzus/internal/types"
)

// mockDeployer simulates toolbox.Deployer for testing
type mockDeployer struct {
	deployComposeErr    error
	startComposeErr     error
	createContainerErr  error
	startContainerErr   error
	deployedProjectName string
	deployedContainerID string
	startedProjects     []string
	startedContainers   []string
}

func (m *mockDeployer) DeployComposeProject(ctx context.Context, template *types.ServiceTemplate, deployment *types.ToolboxDeployment) (string, error) {
	if m.deployComposeErr != nil {
		return "", m.deployComposeErr
	}
	projectName := "test-project-" + deployment.ServiceName
	m.deployedProjectName = projectName
	return projectName, nil
}

func (m *mockDeployer) StartComposeProject(ctx context.Context, projectName string) error {
	if m.startComposeErr != nil {
		return m.startComposeErr
	}
	m.startedProjects = append(m.startedProjects, projectName)
	return nil
}

func (m *mockDeployer) CreateContainer(ctx context.Context, template *types.ServiceTemplate, deployment *types.ToolboxDeployment) (string, error) {
	if m.createContainerErr != nil {
		return "", m.createContainerErr
	}
	containerID := "container-" + deployment.ServiceName
	m.deployedContainerID = containerID
	return containerID, nil
}

func (m *mockDeployer) StartContainer(ctx context.Context, containerID string) error {
	if m.startContainerErr != nil {
		return m.startContainerErr
	}
	m.startedContainers = append(m.startedContainers, containerID)
	return nil
}

// TestComposeDeploymentStrategy tests Compose-based deployment strategy
func TestComposeDeploymentStrategy(t *testing.T) {
	ctx := context.Background()

	t.Run("successful_compose_deploy", func(t *testing.T) {
		deployer := &mockDeployer{}
		strategy := NewComposeDeploymentStrategy(deployer)

		deployment := &types.ToolboxDeployment{
			ID:          "test-deployment-1",
			ServiceName: "grafana",
		}
		template := &types.ServiceTemplate{
			ID:   "grafana",
			Name: "Grafana",
		}

		// Deploy
		identifier, err := strategy.Deploy(ctx, deployment, template)
		if err != nil {
			t.Fatalf("Deploy failed: %v", err)
		}

		expectedProject := "test-project-grafana"
		if identifier != expectedProject {
			t.Errorf("Expected identifier %s, got %s", expectedProject, identifier)
		}

		if deployer.deployedProjectName != expectedProject {
			t.Errorf("Expected deployed project %s, got %s", expectedProject, deployer.deployedProjectName)
		}
	})

	t.Run("successful_compose_start", func(t *testing.T) {
		deployer := &mockDeployer{}
		strategy := NewComposeDeploymentStrategy(deployer)

		projectName := "test-project-grafana"
		err := strategy.Start(ctx, projectName)
		if err != nil {
			t.Fatalf("Start failed: %v", err)
		}

		if len(deployer.startedProjects) != 1 {
			t.Errorf("Expected 1 started project, got %d", len(deployer.startedProjects))
		}

		if deployer.startedProjects[0] != projectName {
			t.Errorf("Expected started project %s, got %s", projectName, deployer.startedProjects[0])
		}
	})

	t.Run("compose_deploy_error", func(t *testing.T) {
		deployer := &mockDeployer{
			deployComposeErr: errors.New("compose deploy failed"),
		}
		strategy := NewComposeDeploymentStrategy(deployer)

		deployment := &types.ToolboxDeployment{ID: "test", ServiceName: "test"}
		template := &types.ServiceTemplate{ID: "test", Name: "Test"}

		_, err := strategy.Deploy(ctx, deployment, template)
		if err == nil {
			t.Fatal("Expected error, got nil")
		}

		if err.Error() != "compose deploy failed: compose deploy failed" {
			t.Errorf("Unexpected error message: %v", err)
		}
	})

	t.Run("compose_start_error", func(t *testing.T) {
		deployer := &mockDeployer{
			startComposeErr: errors.New("compose start failed"),
		}
		strategy := NewComposeDeploymentStrategy(deployer)

		err := strategy.Start(ctx, "test-project")
		if err == nil {
			t.Fatal("Expected error, got nil")
		}

		if err.Error() != "compose start failed: compose start failed" {
			t.Errorf("Unexpected error message: %v", err)
		}
	})

	t.Run("nil_deployment", func(t *testing.T) {
		deployer := &mockDeployer{}
		strategy := NewComposeDeploymentStrategy(deployer)

		_, err := strategy.Deploy(ctx, nil, &types.ServiceTemplate{ID: "test"})
		if err == nil {
			t.Fatal("Expected error for nil deployment, got nil")
		}
	})

	t.Run("nil_template", func(t *testing.T) {
		deployer := &mockDeployer{}
		strategy := NewComposeDeploymentStrategy(deployer)

		deployment := &types.ToolboxDeployment{ID: "test"}
		_, err := strategy.Deploy(ctx, deployment, nil)
		if err == nil {
			t.Fatal("Expected error for nil template, got nil")
		}
	})

	t.Run("empty_project_name", func(t *testing.T) {
		deployer := &mockDeployer{}
		strategy := NewComposeDeploymentStrategy(deployer)

		err := strategy.Start(ctx, "")
		if err == nil {
			t.Fatal("Expected error for empty project name, got nil")
		}
	})
}

// TestComposeDeploymentStrategy_Interface verifies ComposeDeploymentStrategy implements DeploymentStrategy
func TestComposeDeploymentStrategy_Interface(t *testing.T) {
	deployer := &mockDeployer{}
	var _ DeploymentStrategy = NewComposeDeploymentStrategy(deployer)
}
