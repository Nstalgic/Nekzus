package handlers

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	containerPkg "github.com/docker/docker/api/types/container"
	apperrors "github.com/nstalgic/nekzus/internal/errors"
	"github.com/nstalgic/nekzus/internal/export"
	"github.com/nstalgic/nekzus/internal/httputil"
)

var exportlog = slog.With("package", "handlers", "handler", "export")

// ExportHandler handles container export endpoints
type ExportHandler struct {
	client DockerClient
}

// NewExportHandler creates a new export handler
func NewExportHandler(client DockerClient) *ExportHandler {
	return &ExportHandler{
		client: client,
	}
}

// ExportRequest represents a request to export a container configuration
type ExportRequest struct {
	SanitizeSecrets *bool `json:"sanitize_secrets"`
	IncludeVolumes  *bool `json:"include_volumes"`
	IncludeNetworks *bool `json:"include_networks"`
}

// BatchExportRequest represents a request to export multiple containers
type BatchExportRequest struct {
	ContainerIDs    []string `json:"container_ids"`
	StackName       string   `json:"stack_name"`
	SanitizeSecrets *bool    `json:"sanitize_secrets"`
	IncludeVolumes  *bool    `json:"include_volumes"`
	IncludeNetworks *bool    `json:"include_networks"`
}

// ExportResponse represents the response from an export operation
type ExportResponse struct {
	Format      string   `json:"format"`
	Content     string   `json:"content"`
	Filename    string   `json:"filename"`
	Warnings    []string `json:"warnings,omitempty"`
	EnvContent  string   `json:"env_content,omitempty"`
	EnvFilename string   `json:"env_filename,omitempty"`
}

// HandleExportContainer exports a container configuration to Docker Compose format
// POST /api/v1/containers/{containerId}/export
func (h *ExportHandler) HandleExportContainer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		apperrors.WriteJSON(w, apperrors.New("METHOD_NOT_ALLOWED", "Method not allowed", http.StatusMethodNotAllowed))
		return
	}

	// Extract container ID from path
	containerID := extractContainerIDForExport(r.URL.Path)
	if containerID == "" {
		apperrors.WriteJSON(w, apperrors.New("INVALID_CONTAINER_ID", "Container ID required", http.StatusBadRequest))
		return
	}

	// Parse request body
	var req ExportRequest
	if r.Body != nil && r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			apperrors.WriteJSON(w, apperrors.New("INVALID_REQUEST_BODY", "Invalid JSON in request body", http.StatusBadRequest))
			return
		}
	}

	// Set defaults - sanitize secrets by default, include volumes and networks
	// Note: If request body is empty, these will be false, which is a reasonable default
	// Client can explicitly set them to true if needed

	// Create context with timeout
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	// Inspect container
	containerJSON, err := h.client.ContainerInspect(ctx, containerID)
	if err != nil {
		exportlog.Error("Error inspecting container for export", "container_id", containerID, "error", err)

		if isContainerNotFoundError(err) {
			apperrors.WriteJSON(w, apperrors.New(
				"CONTAINER_NOT_FOUND",
				"container not found",
				http.StatusNotFound,
			))
			return
		}

		apperrors.WriteJSON(w, apperrors.Wrap(
			err,
			"CONTAINER_INSPECT_FAILED",
			"failed to inspect container",
			http.StatusInternalServerError,
		))
		return
	}

	// Build export options with defaults
	// Default to sanitizing secrets and including volumes/networks
	options := export.ExportOptions{
		SanitizeSecrets: true,
		IncludeVolumes:  true,
		IncludeNetworks: true,
	}

	// Override with explicit request values if provided
	if req.SanitizeSecrets != nil {
		options.SanitizeSecrets = *req.SanitizeSecrets
	}
	if req.IncludeVolumes != nil {
		options.IncludeVolumes = *req.IncludeVolumes
	}
	if req.IncludeNetworks != nil {
		options.IncludeNetworks = *req.IncludeNetworks
	}

	// Export to compose with env file support
	result, err := export.ExportToComposeWithEnv(&containerJSON, options)
	if err != nil {
		exportlog.Error("Error exporting container to compose", "container_id", containerID, "error", err)
		apperrors.WriteJSON(w, apperrors.Wrap(
			err,
			"EXPORT_FAILED",
			"failed to export container configuration",
			http.StatusInternalServerError,
		))
		return
	}

	exportlog.Info("Exported container to compose",
		"container_id", containerID,
		"filename", result.Filename,
		"has_env_file", result.EnvFilename != "",
		"warnings_count", len(result.Warnings),
	)

	// Build response
	response := ExportResponse{
		Format:      result.Format,
		Content:     result.Content,
		Filename:    result.Filename,
		Warnings:    result.Warnings,
		EnvContent:  result.EnvContent,
		EnvFilename: result.EnvFilename,
	}

	if err := httputil.WriteJSON(w, http.StatusOK, response); err != nil {
		exportlog.Error("Error encoding JSON response", "error", err)
	}
}

// HandleBatchExport exports multiple container configurations to a single Docker Compose file
// POST /api/v1/containers/batch/export
// Query params:
//   - format: "json" (default) or "zip" - when "zip", returns a ZIP file containing compose and env files
func (h *ExportHandler) HandleBatchExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		apperrors.WriteJSON(w, apperrors.New("METHOD_NOT_ALLOWED", "Method not allowed", http.StatusMethodNotAllowed))
		return
	}

	// Check for format query parameter
	outputFormat := r.URL.Query().Get("format")

	// Parse request body
	var req BatchExportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apperrors.WriteJSON(w, apperrors.New("INVALID_REQUEST_BODY", "Invalid JSON in request body", http.StatusBadRequest))
		return
	}

	// Validate container IDs
	if len(req.ContainerIDs) == 0 {
		apperrors.WriteJSON(w, apperrors.New("INVALID_REQUEST", "At least one container ID is required", http.StatusBadRequest))
		return
	}

	// Build export options with defaults
	options := export.ExportOptions{
		SanitizeSecrets: true,
		IncludeVolumes:  true,
		IncludeNetworks: true,
	}

	// Override with explicit request values if provided
	if req.SanitizeSecrets != nil {
		options.SanitizeSecrets = *req.SanitizeSecrets
	}
	if req.IncludeVolumes != nil {
		options.IncludeVolumes = *req.IncludeVolumes
	}
	if req.IncludeNetworks != nil {
		options.IncludeNetworks = *req.IncludeNetworks
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second) // Longer timeout for batch
	defer cancel()

	// Inspect all containers
	var containers []*containerPkg.InspectResponse
	var warnings []string
	var failedIDs []string

	for _, containerID := range req.ContainerIDs {
		containerJSON, err := h.client.ContainerInspect(ctx, containerID)
		if err != nil {
			exportlog.Warn("Failed to inspect container for batch export",
				"container_id", containerID,
				"error", err,
			)
			failedIDs = append(failedIDs, containerID)
			warnings = append(warnings, "Failed to inspect container '"+containerID+"': "+err.Error())
			continue
		}
		containers = append(containers, &containerJSON)
	}

	// If all containers failed, return error
	if len(containers) == 0 {
		apperrors.WriteJSON(w, apperrors.New(
			"ALL_CONTAINERS_FAILED",
			"Failed to inspect any of the specified containers",
			http.StatusNotFound,
		))
		return
	}

	// Generate stack name if not provided
	stackName := req.StackName
	if stackName == "" {
		stackName = "exported-stack"
	}

	// Export to compose with env file support
	result, err := export.BatchExportToComposeWithEnv(containers, options, stackName)
	if err != nil {
		exportlog.Error("Error batch exporting containers to compose", "error", err)
		apperrors.WriteJSON(w, apperrors.Wrap(
			err,
			"BATCH_EXPORT_FAILED",
			"failed to export container configurations",
			http.StatusInternalServerError,
		))
		return
	}

	// Merge warnings
	result.Warnings = append(warnings, result.Warnings...)

	exportlog.Info("Batch exported containers to compose",
		"container_count", len(containers),
		"failed_count", len(failedIDs),
		"filename", result.Filename,
		"has_env_file", result.EnvFilename != "",
		"warnings_count", len(result.Warnings),
	)

	// If ZIP format requested, create and return ZIP file
	if outputFormat == "zip" {
		zipData, zipFilename, err := export.CreateZipBundle(result, stackName)
		if err != nil {
			exportlog.Error("Error creating ZIP bundle", "error", err)
			apperrors.WriteJSON(w, apperrors.Wrap(
				err,
				"ZIP_CREATION_FAILED",
				"failed to create ZIP bundle",
				http.StatusInternalServerError,
			))
			return
		}

		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", "attachment; filename=\""+zipFilename+"\"")
		w.WriteHeader(http.StatusOK)
		w.Write(zipData)
		return
	}

	// Default: return JSON response
	response := ExportResponse{
		Format:      result.Format,
		Content:     result.Content,
		Filename:    result.Filename,
		Warnings:    result.Warnings,
		EnvContent:  result.EnvContent,
		EnvFilename: result.EnvFilename,
	}

	// Return partial content status if some containers failed
	statusCode := http.StatusOK
	if len(failedIDs) > 0 {
		statusCode = http.StatusPartialContent
	}

	if err := httputil.WriteJSON(w, statusCode, response); err != nil {
		exportlog.Error("Error encoding JSON response", "error", err)
	}
}

// HandlePreviewExport generates a preview of the export without triggering download
// POST /api/v1/containers/{containerId}/export/preview
func (h *ExportHandler) HandlePreviewExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		apperrors.WriteJSON(w, apperrors.New("METHOD_NOT_ALLOWED", "Method not allowed", http.StatusMethodNotAllowed))
		return
	}

	// Extract container ID from path
	containerID := extractContainerIDForPreview(r.URL.Path)
	if containerID == "" {
		apperrors.WriteJSON(w, apperrors.New("INVALID_CONTAINER_ID", "Container ID required", http.StatusBadRequest))
		return
	}

	// Parse request body
	var req ExportRequest
	if r.Body != nil && r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			apperrors.WriteJSON(w, apperrors.New("INVALID_REQUEST_BODY", "Invalid JSON in request body", http.StatusBadRequest))
			return
		}
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	// Inspect container
	containerJSON, err := h.client.ContainerInspect(ctx, containerID)
	if err != nil {
		exportlog.Error("Error inspecting container for preview", "container_id", containerID, "error", err)

		if isContainerNotFoundError(err) {
			apperrors.WriteJSON(w, apperrors.New(
				"CONTAINER_NOT_FOUND",
				"container not found",
				http.StatusNotFound,
			))
			return
		}

		apperrors.WriteJSON(w, apperrors.Wrap(
			err,
			"CONTAINER_INSPECT_FAILED",
			"failed to inspect container",
			http.StatusInternalServerError,
		))
		return
	}

	// Build export options with defaults
	options := export.ExportOptions{
		SanitizeSecrets: true,
		IncludeVolumes:  true,
		IncludeNetworks: true,
	}

	// Override with explicit request values if provided
	if req.SanitizeSecrets != nil {
		options.SanitizeSecrets = *req.SanitizeSecrets
	}
	if req.IncludeVolumes != nil {
		options.IncludeVolumes = *req.IncludeVolumes
	}
	if req.IncludeNetworks != nil {
		options.IncludeNetworks = *req.IncludeNetworks
	}

	// Export to compose with env file support
	result, err := export.ExportToComposeWithEnv(&containerJSON, options)
	if err != nil {
		exportlog.Error("Error generating export preview", "container_id", containerID, "error", err)
		apperrors.WriteJSON(w, apperrors.Wrap(
			err,
			"PREVIEW_FAILED",
			"failed to generate export preview",
			http.StatusInternalServerError,
		))
		return
	}

	// Build response (same as export, just used for preview display)
	response := ExportResponse{
		Format:      result.Format,
		Content:     result.Content,
		Filename:    result.Filename,
		Warnings:    result.Warnings,
		EnvContent:  result.EnvContent,
		EnvFilename: result.EnvFilename,
	}

	if err := httputil.WriteJSON(w, http.StatusOK, response); err != nil {
		exportlog.Error("Error encoding JSON response", "error", err)
	}
}

// HandleBatchPreviewExport generates a preview of the batch export without triggering download
// POST /api/v1/containers/batch/export/preview
func (h *ExportHandler) HandleBatchPreviewExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		apperrors.WriteJSON(w, apperrors.New("METHOD_NOT_ALLOWED", "Method not allowed", http.StatusMethodNotAllowed))
		return
	}

	// Parse request body
	var req BatchExportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apperrors.WriteJSON(w, apperrors.New("INVALID_REQUEST_BODY", "Invalid JSON in request body", http.StatusBadRequest))
		return
	}

	// Validate container IDs
	if len(req.ContainerIDs) == 0 {
		apperrors.WriteJSON(w, apperrors.New("INVALID_REQUEST", "At least one container ID is required", http.StatusBadRequest))
		return
	}

	// Build export options with defaults
	options := export.ExportOptions{
		SanitizeSecrets: true,
		IncludeVolumes:  true,
		IncludeNetworks: true,
	}

	// Override with explicit request values if provided
	if req.SanitizeSecrets != nil {
		options.SanitizeSecrets = *req.SanitizeSecrets
	}
	if req.IncludeVolumes != nil {
		options.IncludeVolumes = *req.IncludeVolumes
	}
	if req.IncludeNetworks != nil {
		options.IncludeNetworks = *req.IncludeNetworks
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	// Inspect all containers
	var containers []*containerPkg.InspectResponse
	var warnings []string

	for _, containerID := range req.ContainerIDs {
		containerJSON, err := h.client.ContainerInspect(ctx, containerID)
		if err != nil {
			warnings = append(warnings, "Failed to inspect container '"+containerID+"': "+err.Error())
			continue
		}
		containers = append(containers, &containerJSON)
	}

	// If all containers failed, return error
	if len(containers) == 0 {
		apperrors.WriteJSON(w, apperrors.New(
			"ALL_CONTAINERS_FAILED",
			"Failed to inspect any of the specified containers",
			http.StatusNotFound,
		))
		return
	}

	// Generate stack name if not provided
	stackName := req.StackName
	if stackName == "" {
		stackName = "exported-stack"
	}

	// Export to compose with env file support
	result, err := export.BatchExportToComposeWithEnv(containers, options, stackName)
	if err != nil {
		exportlog.Error("Error generating batch preview", "error", err)
		apperrors.WriteJSON(w, apperrors.Wrap(
			err,
			"PREVIEW_FAILED",
			"failed to generate batch export preview",
			http.StatusInternalServerError,
		))
		return
	}

	// Merge warnings
	result.Warnings = append(warnings, result.Warnings...)

	// Build response
	response := ExportResponse{
		Format:      result.Format,
		Content:     result.Content,
		Filename:    result.Filename,
		Warnings:    result.Warnings,
		EnvContent:  result.EnvContent,
		EnvFilename: result.EnvFilename,
	}

	if err := httputil.WriteJSON(w, http.StatusOK, response); err != nil {
		exportlog.Error("Error encoding JSON response", "error", err)
	}
}

// extractContainerIDForPreview extracts container ID from URL path for preview endpoints
// Path format: /api/v1/containers/{containerId}/export/preview
func extractContainerIDForPreview(path string) string {
	// Remove trailing slash if present
	path = strings.TrimSuffix(path, "/")

	// Split path and get segments
	parts := strings.Split(path, "/")
	if len(parts) < 7 {
		return ""
	}

	// Container ID is at index 4
	// Example: /api/v1/containers/abc123/export/preview
	// parts = ["", "api", "v1", "containers", "abc123", "export", "preview"]
	//          0    1      2      3            4         5         6

	// Verify the last segment is "preview" and second-to-last is "export"
	if parts[len(parts)-1] != "preview" || parts[len(parts)-2] != "export" {
		return ""
	}

	return parts[4]
}

// extractContainerIDForExport extracts container ID from URL path for export endpoints
// Path format: /api/v1/containers/{containerId}/export
func extractContainerIDForExport(path string) string {
	// Remove trailing slash if present
	path = strings.TrimSuffix(path, "/")

	// Split path and get segments
	parts := strings.Split(path, "/")
	if len(parts) < 6 {
		return ""
	}

	// Container ID is at index 4 (after /api/v1/containers/)
	// Example: /api/v1/containers/abc123/export
	// parts = ["", "api", "v1", "containers", "abc123", "export"]
	//          0    1      2      3            4         5

	// Verify the last segment is "export"
	if parts[len(parts)-1] != "export" {
		return ""
	}

	return parts[4]
}
