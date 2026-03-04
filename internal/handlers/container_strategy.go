package handlers

import (
	"context"
	"fmt"

	"github.com/nstalgic/nekzus/internal/types"
)

// ContainerDeployer defines the interface for single-container deployment operations
type ContainerDeployer interface {
	CreateContainer(ctx context.Context, template *types.ServiceTemplate, deployment *types.ToolboxDeployment) (string, error)
	StartContainer(ctx context.Context, containerID string) error
}

// ContainerDeploymentStrategy implements DeploymentStrategy for single Docker containers
type ContainerDeploymentStrategy struct {
	deployer ContainerDeployer
}

// NewContainerDeploymentStrategy creates a new container deployment strategy
func NewContainerDeploymentStrategy(deployer ContainerDeployer) *ContainerDeploymentStrategy {
	return &ContainerDeploymentStrategy{
		deployer: deployer,
	}
}

// Deploy creates a Docker container
func (s *ContainerDeploymentStrategy) Deploy(ctx context.Context, deployment *types.ToolboxDeployment, template *types.ServiceTemplate) (string, error) {
	if deployment == nil {
		return "", fmt.Errorf("deployment cannot be nil")
	}
	if template == nil {
		return "", fmt.Errorf("template cannot be nil")
	}

	containerID, err := s.deployer.CreateContainer(ctx, template, deployment)
	if err != nil {
		return "", fmt.Errorf("container create failed: %w", err)
	}

	return containerID, nil
}

// Start initiates a Docker container
func (s *ContainerDeploymentStrategy) Start(ctx context.Context, containerID string) error {
	if containerID == "" {
		return fmt.Errorf("container ID cannot be empty")
	}

	if err := s.deployer.StartContainer(ctx, containerID); err != nil {
		return fmt.Errorf("container start failed: %w", err)
	}

	return nil
}
