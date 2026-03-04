package handlers

import (
	"context"
	"fmt"

	"github.com/nstalgic/nekzus/internal/types"
)

// ComposeDeployer defines the interface for Compose-based deployment operations
type ComposeDeployer interface {
	DeployComposeProject(ctx context.Context, template *types.ServiceTemplate, deployment *types.ToolboxDeployment) (string, error)
	StartComposeProject(ctx context.Context, projectName string) error
}

// ComposeDeploymentStrategy implements DeploymentStrategy for Docker Compose projects
type ComposeDeploymentStrategy struct {
	deployer ComposeDeployer
}

// NewComposeDeploymentStrategy creates a new Compose deployment strategy
func NewComposeDeploymentStrategy(deployer ComposeDeployer) *ComposeDeploymentStrategy {
	return &ComposeDeploymentStrategy{
		deployer: deployer,
	}
}

// Deploy creates a Docker Compose project
func (s *ComposeDeploymentStrategy) Deploy(ctx context.Context, deployment *types.ToolboxDeployment, template *types.ServiceTemplate) (string, error) {
	if deployment == nil {
		return "", fmt.Errorf("deployment cannot be nil")
	}
	if template == nil {
		return "", fmt.Errorf("template cannot be nil")
	}

	projectName, err := s.deployer.DeployComposeProject(ctx, template, deployment)
	if err != nil {
		return "", fmt.Errorf("compose deploy failed: %w", err)
	}

	return projectName, nil
}

// Start initiates a Docker Compose project
func (s *ComposeDeploymentStrategy) Start(ctx context.Context, projectName string) error {
	if projectName == "" {
		return fmt.Errorf("project name cannot be empty")
	}

	if err := s.deployer.StartComposeProject(ctx, projectName); err != nil {
		return fmt.Errorf("compose start failed: %w", err)
	}

	return nil
}
