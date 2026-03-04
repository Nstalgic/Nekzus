package handlers

import (
	"context"
	"errors"
	"testing"

	"github.com/nstalgic/nekzus/internal/types"
)

// TestContainerDeploymentStrategy tests single-container deployment strategy
func TestContainerDeploymentStrategy(t *testing.T) {
	ctx := context.Background()

	t.Run("successful_container_deploy", func(t *testing.T) {
		deployer := &mockDeployer{}
		strategy := NewContainerDeploymentStrategy(deployer)

		deployment := &types.ToolboxDeployment{
			ID:          "test-deployment-1",
			ServiceName: "pihole",
		}
		template := &types.ServiceTemplate{
			ID:   "pihole",
			Name: "Pi-hole",
		}

		// Deploy
		identifier, err := strategy.Deploy(ctx, deployment, template)
		if err != nil {
			t.Fatalf("Deploy failed: %v", err)
		}

		expectedContainer := "container-pihole"
		if identifier != expectedContainer {
			t.Errorf("Expected identifier %s, got %s", expectedContainer, identifier)
		}

		if deployer.deployedContainerID != expectedContainer {
			t.Errorf("Expected deployed container %s, got %s", expectedContainer, deployer.deployedContainerID)
		}
	})

	t.Run("successful_container_start", func(t *testing.T) {
		deployer := &mockDeployer{}
		strategy := NewContainerDeploymentStrategy(deployer)

		containerID := "container-abc123"
		err := strategy.Start(ctx, containerID)
		if err != nil {
			t.Fatalf("Start failed: %v", err)
		}

		if len(deployer.startedContainers) != 1 {
			t.Errorf("Expected 1 started container, got %d", len(deployer.startedContainers))
		}

		if deployer.startedContainers[0] != containerID {
			t.Errorf("Expected started container %s, got %s", containerID, deployer.startedContainers[0])
		}
	})

	t.Run("container_create_error", func(t *testing.T) {
		deployer := &mockDeployer{
			createContainerErr: errors.New("container create failed"),
		}
		strategy := NewContainerDeploymentStrategy(deployer)

		deployment := &types.ToolboxDeployment{ID: "test", ServiceName: "test"}
		template := &types.ServiceTemplate{ID: "test", Name: "Test"}

		_, err := strategy.Deploy(ctx, deployment, template)
		if err == nil {
			t.Fatal("Expected error, got nil")
		}

		if err.Error() != "container create failed: container create failed" {
			t.Errorf("Unexpected error message: %v", err)
		}
	})

	t.Run("container_start_error", func(t *testing.T) {
		deployer := &mockDeployer{
			startContainerErr: errors.New("container start failed"),
		}
		strategy := NewContainerDeploymentStrategy(deployer)

		err := strategy.Start(ctx, "container-123")
		if err == nil {
			t.Fatal("Expected error, got nil")
		}

		if err.Error() != "container start failed: container start failed" {
			t.Errorf("Unexpected error message: %v", err)
		}
	})

	t.Run("nil_deployment", func(t *testing.T) {
		deployer := &mockDeployer{}
		strategy := NewContainerDeploymentStrategy(deployer)

		_, err := strategy.Deploy(ctx, nil, &types.ServiceTemplate{ID: "test"})
		if err == nil {
			t.Fatal("Expected error for nil deployment, got nil")
		}
	})

	t.Run("nil_template", func(t *testing.T) {
		deployer := &mockDeployer{}
		strategy := NewContainerDeploymentStrategy(deployer)

		deployment := &types.ToolboxDeployment{ID: "test"}
		_, err := strategy.Deploy(ctx, deployment, nil)
		if err == nil {
			t.Fatal("Expected error for nil template, got nil")
		}
	})

	t.Run("empty_container_id", func(t *testing.T) {
		deployer := &mockDeployer{}
		strategy := NewContainerDeploymentStrategy(deployer)

		err := strategy.Start(ctx, "")
		if err == nil {
			t.Fatal("Expected error for empty container ID, got nil")
		}
	})
}

// TestContainerDeploymentStrategy_Interface verifies ContainerDeploymentStrategy implements DeploymentStrategy
func TestContainerDeploymentStrategy_Interface(t *testing.T) {
	deployer := &mockDeployer{}
	var _ DeploymentStrategy = NewContainerDeploymentStrategy(deployer)
}
