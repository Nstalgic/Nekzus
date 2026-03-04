package handlers

import (
	"context"

	"github.com/nstalgic/nekzus/internal/types"
)

// DeploymentStrategy defines the interface for deploying toolbox services.
// This interface enables testing and allows different deployment strategies
// (Compose-based vs single-container) to be implemented independently.
type DeploymentStrategy interface {
	// Deploy creates the deployment resources (containers, networks, volumes).
	// Returns an identifier for the deployment (container ID or project name).
	Deploy(ctx context.Context, deployment *types.ToolboxDeployment, template *types.ServiceTemplate) (string, error)

	// Start initiates the deployed resources.
	// The identifier is returned from Deploy.
	Start(ctx context.Context, identifier string) error
}
