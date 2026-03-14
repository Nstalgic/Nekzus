package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	apperrors "github.com/nstalgic/nekzus/internal/errors"
	"github.com/nstalgic/nekzus/internal/httputil"
	"github.com/nstalgic/nekzus/internal/storage"
	"github.com/nstalgic/nekzus/internal/toolbox"
	"github.com/nstalgic/nekzus/internal/types"
)

var marketlog = slog.With("package", "handlers")

// Body size and timeout limits for toolbox endpoints
const (
	// MaxToolboxRequestBodySize is the maximum size for toolbox API request bodies (1MB)
	MaxToolboxRequestBodySize = 1024 * 1024

	// DeploymentTimeout is the maximum duration for a deployment operation
	DeploymentTimeout = 10 * time.Minute
)

// ToolboxHandler handles toolbox-related HTTP requests.
type ToolboxHandler struct {
	manager  *toolbox.Manager
	deployer *toolbox.Deployer
	storage  *storage.Store
	baseURL  string
}

// NewToolboxHandler creates a new toolbox handler.
func NewToolboxHandler(manager *toolbox.Manager, deployer *toolbox.Deployer, store *storage.Store, baseURL string) *ToolboxHandler {
	return &ToolboxHandler{
		manager:  manager,
		deployer: deployer,
		storage:  store,
		baseURL:  baseURL,
	}
}

// ListServices handles GET /api/v1/toolbox/services
func (h *ToolboxHandler) ListServices(w http.ResponseWriter, r *http.Request) {
	// Get optional category filter
	category := r.URL.Query().Get("category")

	var services []*types.ServiceTemplate
	if category != "" {
		services = h.manager.FilterByCategory(category)
	} else {
		services = h.manager.ListServices()
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"services": services,
		"count":    len(services),
	})
}

// GetService handles GET /api/v1/toolbox/services/{id}
func (h *ToolboxHandler) GetService(w http.ResponseWriter, r *http.Request) {
	serviceID := r.PathValue("id")
	if serviceID == "" {
		apperrors.WriteJSON(w, apperrors.New("MISSING_SERVICE_ID", "Service ID is required", http.StatusBadRequest))
		return
	}

	service, err := h.manager.GetService(serviceID)
	if err != nil {
		apperrors.WriteJSON(w, apperrors.Wrap(err, "SERVICE_NOT_FOUND", "Service not found", http.StatusNotFound))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(service)
}

// DeployService handles POST /api/v1/toolbox/deploy
func (h *ToolboxHandler) DeployService(w http.ResponseWriter, r *http.Request) {
	// Decode and validate request (types.DeploymentRequest doesn't have Validate method,
	// validation is done by ValidateDeploymentRequest below)
	req, err := httputil.DecodeAndValidate[types.DeploymentRequest](r, w, MaxToolboxRequestBodySize)
	if err != nil {
		apperrors.WriteJSON(w, err)
		return
	}

	// Validate request
	if err := h.manager.ValidateDeploymentRequest(req); err != nil {
		apperrors.WriteJSON(w, apperrors.Wrap(err, "INVALID_REQUEST", "Invalid deployment request", http.StatusBadRequest))
		return
	}

	// Always inject BASE_URL from server config (override any user-provided value)
	if req.EnvVars == nil {
		req.EnvVars = make(map[string]string)
	}
	if h.baseURL != "" {
		req.EnvVars["BASE_URL"] = h.baseURL
	}

	// Get service template
	template, err := h.manager.GetService(req.ServiceID)
	if err != nil {
		apperrors.WriteJSON(w, apperrors.Wrap(err, "SERVICE_NOT_FOUND", "Service template not found", http.StatusNotFound))
		return
	}

	// Inject CustomPort as APP_PORT into EnvVars
	if req.CustomPort > 0 {
		// Validate port is in valid range (1-65535)
		if req.CustomPort > 65535 {
			apperrors.WriteJSON(w, apperrors.New("INVALID_PORT", fmt.Sprintf("Port %d exceeds maximum valid port (65535)", req.CustomPort), http.StatusBadRequest))
			return
		}

		req.EnvVars["APP_PORT"] = fmt.Sprintf("%d", req.CustomPort)
	}

	// Validate deployment configuration
	if err := h.deployer.ValidateDeployment(template, req.EnvVars); err != nil {
		apperrors.WriteJSON(w, apperrors.Wrap(err, "VALIDATION_FAILED", "Deployment validation failed", http.StatusBadRequest))
		return
	}

	// Check port conflicts
	if err := h.deployer.CheckPortConflicts(template.DockerConfig.Ports, req.EnvVars); err != nil {
		apperrors.WriteJSON(w, apperrors.Wrap(err, "PORT_CONFLICT", "Port conflict detected", http.StatusConflict))
		return
	}

	// Create deployment record
	deploymentID := generateDeploymentID()
	deployment := &types.ToolboxDeployment{
		ID:                deploymentID,
		ServiceTemplateID: req.ServiceID,
		ServiceName:       req.ServiceName,
		Status:            types.DeploymentStatusPending,
		ContainerName:     h.deployer.GenerateContainerName(req.ServiceName),
		EnvVars:           req.EnvVars,
		CustomImage:       req.CustomImage,
		CustomPort:        req.CustomPort,
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
	}

	// Save deployment to database
	if h.storage != nil {
		if err := h.storage.SaveDeployment(deployment); err != nil {
			apperrors.WriteJSON(w, apperrors.Wrap(err, "SAVE_FAILED", "Failed to save deployment", http.StatusInternalServerError))
			return
		}
	}

	// Start async deployment if requested
	if req.AutoStart {
		go h.performDeployment(deployment, template)
	}

	// Return deployment info
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"deployment_id": deploymentID,
		"status":        deployment.Status,
		"message":       fmt.Sprintf("Deployment '%s' initiated", req.ServiceName),
	})
}

// performDeployment executes the actual Docker deployment in the background
// Refactored to use Strategy pattern for improved testability and maintainability
func (h *ToolboxHandler) performDeployment(deployment *types.ToolboxDeployment, template *types.ServiceTemplate) {
	marketlog.Info("Starting deployment",
		"deployment_id", deployment.ID,
		"service_name", deployment.ServiceName,
		"template_id", template.ID)

	ctx, cancel := context.WithTimeout(context.Background(), DeploymentTimeout)
	defer cancel()

	// Update status to deploying
	h.updateDeploymentStatus(deployment.ID, types.DeploymentStatusDeploying, "")

	// Select deployment strategy based on template type
	strategy := h.selectDeploymentStrategy(template)

	// Execute deployment
	identifier, err := strategy.Deploy(ctx, deployment, template)
	if err != nil {
		h.handleDeploymentError(deployment.ID, ctx, err, "deploy")
		return
	}

	marketlog.Info("Deployment resources created", "deployment_id", deployment.ID, "identifier", identifier)

	// Update deployment with identifier (container ID or project name)
	h.updateDeploymentIdentifier(deployment.ID, identifier, template.ComposeProject != nil)

	// Start the deployment
	if err := strategy.Start(ctx, identifier); err != nil {
		h.handleDeploymentError(deployment.ID, ctx, err, "start")
		return
	}

	// Mark deployment as successful
	h.markDeploymentSuccess(deployment, identifier)
}

// selectDeploymentStrategy chooses the appropriate deployment strategy
func (h *ToolboxHandler) selectDeploymentStrategy(template *types.ServiceTemplate) DeploymentStrategy {
	if template.ComposeProject != nil {
		return NewComposeDeploymentStrategy(h.deployer)
	}
	return NewContainerDeploymentStrategy(h.deployer)
}

// updateDeploymentStatus updates the deployment status in storage
func (h *ToolboxHandler) updateDeploymentStatus(deploymentID, status, errorMsg string) {
	if h.storage != nil {
		if err := h.storage.UpdateDeploymentStatus(deploymentID, status, errorMsg); err != nil {
			marketlog.Error("Failed to update deployment status", "deployment_id", deploymentID, "status", status, "error", err)
		}
	}
}

// updateDeploymentIdentifier updates the deployment with container ID or project name
func (h *ToolboxHandler) updateDeploymentIdentifier(deploymentID, identifier string, isCompose bool) {
	if h.storage == nil {
		return
	}

	var err error
	if isCompose {
		// Project name stored as container_name for compatibility
		err = h.storage.UpdateDeploymentContainer(deploymentID, "", identifier)
	} else {
		// Container ID
		err = h.storage.UpdateDeploymentContainer(deploymentID, identifier, "")
	}

	if err != nil {
		marketlog.Error("Failed to update deployment with identifier", "deployment_id", deploymentID, "error", err)
	}
}

// handleDeploymentError handles deployment failures
func (h *ToolboxHandler) handleDeploymentError(deploymentID string, ctx context.Context, err error, phase string) {
	errorMsg := err.Error()
	if ctx.Err() == context.DeadlineExceeded {
		errorMsg = fmt.Sprintf("Deployment %s timed out after %s", phase, DeploymentTimeout.String())
	}

	marketlog.Error("Deployment failed", "deployment_id", deploymentID, "phase", phase, "error", err)
	h.updateDeploymentStatus(deploymentID, types.DeploymentStatusFailed, errorMsg)
}

// markDeploymentSuccess marks the deployment as successfully deployed
func (h *ToolboxHandler) markDeploymentSuccess(deployment *types.ToolboxDeployment, identifier string) {
	now := time.Now()
	deployment.DeployedAt = &now

	h.updateDeploymentStatus(deployment.ID, types.DeploymentStatusDeployed, "")

	marketlog.Info("Deployment completed successfully", "deployment_id", deployment.ID, "identifier", identifier)
}

// GetDeployment handles GET /api/v1/toolbox/deployments/{id}
func (h *ToolboxHandler) GetDeployment(w http.ResponseWriter, r *http.Request) {
	deploymentID := r.PathValue("id")
	if deploymentID == "" {
		apperrors.WriteJSON(w, apperrors.New("MISSING_DEPLOYMENT_ID", "Deployment ID is required", http.StatusBadRequest))
		return
	}

	if h.storage == nil {
		apperrors.WriteJSON(w, apperrors.New("STORAGE_UNAVAILABLE", "Storage is not available", http.StatusServiceUnavailable))
		return
	}

	deployment, err := h.storage.GetDeployment(deploymentID)
	if err != nil {
		apperrors.WriteJSON(w, apperrors.Wrap(err, "DEPLOYMENT_NOT_FOUND", "Deployment not found", http.StatusNotFound))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(deployment)
}

// ListDeployments handles GET /api/v1/toolbox/deployments
func (h *ToolboxHandler) ListDeployments(w http.ResponseWriter, r *http.Request) {
	if h.storage == nil {
		apperrors.WriteJSON(w, apperrors.New("STORAGE_UNAVAILABLE", "Storage is not available", http.StatusServiceUnavailable))
		return
	}

	// Get optional status filter
	status := r.URL.Query().Get("status")

	var deployments []*types.ToolboxDeployment
	var err error

	if status != "" {
		deployments, err = h.storage.ListDeploymentsByStatus(status)
	} else {
		deployments, err = h.storage.ListDeployments()
	}

	if err != nil {
		apperrors.WriteJSON(w, apperrors.Wrap(err, "LIST_FAILED", "Failed to list deployments", http.StatusInternalServerError))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"deployments": deployments,
		"count":       len(deployments),
	})
}

// RemoveDeployment handles DELETE /api/v1/toolbox/deployments/{id}
func (h *ToolboxHandler) RemoveDeployment(w http.ResponseWriter, r *http.Request) {
	deploymentID := r.PathValue("id")
	if deploymentID == "" {
		apperrors.WriteJSON(w, apperrors.New("MISSING_DEPLOYMENT_ID", "Deployment ID is required", http.StatusBadRequest))
		return
	}

	// Parse removeVolumes query parameter
	removeVolumes := r.URL.Query().Get("removeVolumes") == "true"

	if h.storage == nil {
		apperrors.WriteJSON(w, apperrors.New("STORAGE_UNAVAILABLE", "Storage is not available", http.StatusServiceUnavailable))
		return
	}

	// Get deployment
	deployment, err := h.storage.GetDeployment(deploymentID)
	if err != nil {
		apperrors.WriteJSON(w, apperrors.Wrap(err, "DEPLOYMENT_NOT_FOUND", "Deployment not found", http.StatusNotFound))
		return
	}

	// Remove Docker container/project if it exists
	ctx := context.Background()
	if deployment.ContainerName != "" {
		// Check if this is a Compose project (project name stored in ContainerName field)
		// Compose project names use the deployment ID as the project name
		if strings.HasPrefix(deployment.ContainerName, "deploy_") {
			// This is a Compose project
			if err := h.deployer.RemoveComposeProject(ctx, deployment.ContainerName, removeVolumes); err != nil {
				// Log error but continue with database cleanup
				marketlog.Warn("Failed to remove Compose project", "project_name", deployment.ContainerName, "error", err)
			}
		} else if deployment.ContainerID != "" {
			// This is a legacy single container
			if err := h.deployer.RemoveContainer(ctx, deployment.ContainerID, removeVolumes); err != nil {
				// Log error but continue with database cleanup
				marketlog.Warn("Failed to remove container", "container_id", deployment.ContainerID, "error", err)
			}
		}
	}

	// Remove deployment record
	if err := h.storage.DeleteDeployment(deploymentID); err != nil {
		apperrors.WriteJSON(w, apperrors.Wrap(err, "DELETE_FAILED", "Failed to delete deployment", http.StatusInternalServerError))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "Deployment removed successfully",
	})
}

// generateDeploymentID generates a unique deployment ID
func generateDeploymentID() string {
	return fmt.Sprintf("deploy_%d", time.Now().UnixNano())
}
