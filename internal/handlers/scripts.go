package handlers

import (
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	apperrors "github.com/nstalgic/nekzus/internal/errors"
	"github.com/nstalgic/nekzus/internal/httputil"
	"github.com/nstalgic/nekzus/internal/scripts"
	"github.com/nstalgic/nekzus/internal/storage"
)

var scriptsLog = slog.With("package", "handlers.scripts")

// Body size limits for scripts endpoints
const (
	// MaxScriptRequestBodySize is the maximum size for script API request bodies (256KB)
	MaxScriptRequestBodySize = 256 * 1024

	// ScriptExecutionTimeout is the maximum duration for script execution via API
	ScriptExecutionTimeout = 10 * time.Minute
)

// ScriptRunner interface for async script execution.
type ScriptRunner interface {
	Submit(job *scripts.ExecutionJob) error
}

// ScriptsHandler handles script-related HTTP requests.
type ScriptsHandler struct {
	manager   *scripts.Manager
	executor  *scripts.Executor
	runner    ScriptRunner
	workflow  *scripts.WorkflowRunner
	scheduler *scripts.Scheduler
	storage   *storage.Store
}

// NewScriptsHandler creates a new scripts handler.
func NewScriptsHandler(
	manager *scripts.Manager,
	executor *scripts.Executor,
	runner ScriptRunner,
	workflow *scripts.WorkflowRunner,
	scheduler *scripts.Scheduler,
	store *storage.Store,
) *ScriptsHandler {
	return &ScriptsHandler{
		manager:   manager,
		executor:  executor,
		runner:    runner,
		workflow:  workflow,
		scheduler: scheduler,
		storage:   store,
	}
}

// --- Script Endpoints ---

// ListScripts handles GET /api/v1/scripts
func (h *ScriptsHandler) ListScripts(w http.ResponseWriter, r *http.Request) {
	category := r.URL.Query().Get("category")

	var scriptsList []*scripts.Script
	var err error

	if category != "" {
		scriptsList, err = h.storage.ListScriptsByCategory(category)
	} else {
		scriptsList, err = h.storage.ListScripts()
	}

	if err != nil {
		apperrors.WriteJSON(w, apperrors.Wrap(err, "LIST_FAILED", "Failed to list scripts", http.StatusInternalServerError))
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"scripts": scriptsList,
		"count":   len(scriptsList),
	})
}

// ListAvailableScripts handles GET /api/v1/scripts/available
// Returns scripts found in the scripts directory that haven't been registered yet.
func (h *ScriptsHandler) ListAvailableScripts(w http.ResponseWriter, r *http.Request) {
	available, err := h.manager.ScanDirectory()
	if err != nil {
		apperrors.WriteJSON(w, apperrors.Wrap(err, "SCAN_FAILED", "Failed to scan scripts directory", http.StatusInternalServerError))
		return
	}

	// Get registered scripts to filter out
	registered, err := h.storage.ListScripts()
	if err != nil {
		apperrors.WriteJSON(w, apperrors.Wrap(err, "LIST_FAILED", "Failed to list registered scripts", http.StatusInternalServerError))
		return
	}

	// Build set of registered paths
	registeredPaths := make(map[string]bool)
	for _, s := range registered {
		registeredPaths[s.ScriptPath] = true
	}

	// Filter out registered scripts
	var unregistered []scripts.AvailableScript
	for _, a := range available {
		if !registeredPaths[a.Path] {
			unregistered = append(unregistered, a)
		}
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"available": unregistered,
		"count":     len(unregistered),
	})
}

// GetScript handles GET /api/v1/scripts/{id}
func (h *ScriptsHandler) GetScript(w http.ResponseWriter, r *http.Request) {
	scriptID := r.PathValue("id")
	if scriptID == "" {
		apperrors.WriteJSON(w, apperrors.New("MISSING_SCRIPT_ID", "Script ID is required", http.StatusBadRequest))
		return
	}

	script, err := h.storage.GetScript(scriptID)
	if err != nil {
		apperrors.WriteJSON(w, apperrors.Wrap(err, "GET_FAILED", "Failed to get script", http.StatusInternalServerError))
		return
	}
	if script == nil {
		apperrors.WriteJSON(w, apperrors.New("SCRIPT_NOT_FOUND", "Script not found", http.StatusNotFound))
		return
	}

	httputil.WriteJSON(w, http.StatusOK, script)
}

// RegisterScriptRequest represents a request to register a script.
type RegisterScriptRequest struct {
	Name           string                    `json:"name"`
	Description    string                    `json:"description,omitempty"`
	Category       string                    `json:"category"`
	ScriptPath     string                    `json:"scriptPath"`
	TimeoutSeconds int                       `json:"timeoutSeconds,omitempty"`
	Parameters     []scripts.ScriptParameter `json:"parameters,omitempty"`
	Environment    map[string]string         `json:"environment,omitempty"`
	AllowedScopes  []string                  `json:"allowedScopes,omitempty"`
	DryRunCommand  string                    `json:"dryRunCommand,omitempty"`
}

// RegisterScript handles POST /api/v1/scripts
func (h *ScriptsHandler) RegisterScript(w http.ResponseWriter, r *http.Request) {
	var req RegisterScriptRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, MaxScriptRequestBodySize)).Decode(&req); err != nil {
		apperrors.WriteJSON(w, apperrors.New("INVALID_JSON", "Invalid request body", http.StatusBadRequest))
		return
	}

	// Validate required fields
	if req.Name == "" {
		apperrors.WriteJSON(w, apperrors.New("MISSING_NAME", "Script name is required", http.StatusBadRequest))
		return
	}
	if req.ScriptPath == "" {
		apperrors.WriteJSON(w, apperrors.New("MISSING_PATH", "Script path is required", http.StatusBadRequest))
		return
	}
	if req.Category == "" {
		req.Category = "general"
	}
	if req.TimeoutSeconds == 0 {
		req.TimeoutSeconds = 300 // Default 5 minutes
	}

	// Generate script ID from name
	scriptID := generateScriptID(req.Name)

	// Check if script already exists
	existing, _ := h.storage.GetScript(scriptID)
	if existing != nil {
		apperrors.WriteJSON(w, apperrors.New("SCRIPT_EXISTS", "A script with this name already exists", http.StatusConflict))
		return
	}

	// Detect script type
	scriptType := h.manager.DetectScriptType(req.ScriptPath)

	// Create script object
	now := time.Now()
	script := &scripts.Script{
		ID:             scriptID,
		Name:           req.Name,
		Description:    req.Description,
		Category:       req.Category,
		ScriptPath:     req.ScriptPath,
		ScriptType:     scriptType,
		TimeoutSeconds: req.TimeoutSeconds,
		Parameters:     req.Parameters,
		Environment:    req.Environment,
		AllowedScopes:  req.AllowedScopes,
		DryRunCommand:  req.DryRunCommand,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	// Validate script exists on filesystem
	if err := h.manager.ValidateScriptExists(script); err != nil {
		apperrors.WriteJSON(w, apperrors.Wrap(err, "SCRIPT_NOT_FOUND", "Script file not found", http.StatusBadRequest))
		return
	}

	// Save to storage
	if err := h.storage.SaveScript(script); err != nil {
		apperrors.WriteJSON(w, apperrors.Wrap(err, "SAVE_FAILED", "Failed to save script", http.StatusInternalServerError))
		return
	}

	scriptsLog.Info("Script registered", "id", scriptID, "name", req.Name, "path", req.ScriptPath)

	httputil.WriteJSON(w, http.StatusCreated, script)
}

// UpdateScript handles PUT /api/v1/scripts/{id}
func (h *ScriptsHandler) UpdateScript(w http.ResponseWriter, r *http.Request) {
	scriptID := r.PathValue("id")
	if scriptID == "" {
		apperrors.WriteJSON(w, apperrors.New("MISSING_SCRIPT_ID", "Script ID is required", http.StatusBadRequest))
		return
	}

	// Get existing script
	existing, err := h.storage.GetScript(scriptID)
	if err != nil {
		apperrors.WriteJSON(w, apperrors.Wrap(err, "GET_FAILED", "Failed to get script", http.StatusInternalServerError))
		return
	}
	if existing == nil {
		apperrors.WriteJSON(w, apperrors.New("SCRIPT_NOT_FOUND", "Script not found", http.StatusNotFound))
		return
	}

	var req RegisterScriptRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, MaxScriptRequestBodySize)).Decode(&req); err != nil {
		apperrors.WriteJSON(w, apperrors.New("INVALID_JSON", "Invalid request body", http.StatusBadRequest))
		return
	}

	// Update fields if provided
	if req.Name != "" {
		existing.Name = req.Name
	}
	if req.Description != "" {
		existing.Description = req.Description
	}
	if req.Category != "" {
		existing.Category = req.Category
	}
	if req.ScriptPath != "" {
		existing.ScriptPath = req.ScriptPath
		existing.ScriptType = h.manager.DetectScriptType(req.ScriptPath)
	}
	if req.TimeoutSeconds > 0 {
		existing.TimeoutSeconds = req.TimeoutSeconds
	}
	if req.Parameters != nil {
		existing.Parameters = req.Parameters
	}
	if req.Environment != nil {
		existing.Environment = req.Environment
	}
	if req.AllowedScopes != nil {
		existing.AllowedScopes = req.AllowedScopes
	}
	if req.DryRunCommand != "" {
		existing.DryRunCommand = req.DryRunCommand
	}
	existing.UpdatedAt = time.Now()

	// Validate script exists
	if err := h.manager.ValidateScriptExists(existing); err != nil {
		apperrors.WriteJSON(w, apperrors.Wrap(err, "SCRIPT_NOT_FOUND", "Script file not found", http.StatusBadRequest))
		return
	}

	// Save updated script
	if err := h.storage.SaveScript(existing); err != nil {
		apperrors.WriteJSON(w, apperrors.Wrap(err, "SAVE_FAILED", "Failed to save script", http.StatusInternalServerError))
		return
	}

	scriptsLog.Info("Script updated", "id", scriptID)

	httputil.WriteJSON(w, http.StatusOK, existing)
}

// DeleteScript handles DELETE /api/v1/scripts/{id}
func (h *ScriptsHandler) DeleteScript(w http.ResponseWriter, r *http.Request) {
	scriptID := r.PathValue("id")
	if scriptID == "" {
		apperrors.WriteJSON(w, apperrors.New("MISSING_SCRIPT_ID", "Script ID is required", http.StatusBadRequest))
		return
	}

	// Check if script exists
	existing, err := h.storage.GetScript(scriptID)
	if err != nil {
		apperrors.WriteJSON(w, apperrors.Wrap(err, "GET_FAILED", "Failed to get script", http.StatusInternalServerError))
		return
	}
	if existing == nil {
		apperrors.WriteJSON(w, apperrors.New("SCRIPT_NOT_FOUND", "Script not found", http.StatusNotFound))
		return
	}

	// Delete script
	if err := h.storage.DeleteScript(scriptID); err != nil {
		apperrors.WriteJSON(w, apperrors.Wrap(err, "DELETE_FAILED", "Failed to delete script", http.StatusInternalServerError))
		return
	}

	scriptsLog.Info("Script deleted", "id", scriptID)

	w.WriteHeader(http.StatusNoContent)
}

// --- Execution Endpoints ---

// ExecuteScript handles POST /api/v1/scripts/{id}/execute
func (h *ScriptsHandler) ExecuteScript(w http.ResponseWriter, r *http.Request) {
	scriptID := r.PathValue("id")
	if scriptID == "" {
		apperrors.WriteJSON(w, apperrors.New("MISSING_SCRIPT_ID", "Script ID is required", http.StatusBadRequest))
		return
	}

	// Get script
	script, err := h.storage.GetScript(scriptID)
	if err != nil {
		apperrors.WriteJSON(w, apperrors.Wrap(err, "GET_FAILED", "Failed to get script", http.StatusInternalServerError))
		return
	}
	if script == nil {
		apperrors.WriteJSON(w, apperrors.New("SCRIPT_NOT_FOUND", "Script not found", http.StatusNotFound))
		return
	}

	// Parse request
	var req scripts.ExecuteRequest
	if r.ContentLength > 0 {
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, MaxScriptRequestBodySize)).Decode(&req); err != nil {
			apperrors.WriteJSON(w, apperrors.New("INVALID_JSON", "Invalid request body", http.StatusBadRequest))
			return
		}
	}

	// Validate parameters
	if err := h.manager.ValidateParameters(script, req.Parameters); err != nil {
		apperrors.WriteJSON(w, apperrors.Wrap(err, "INVALID_PARAMS", "Parameter validation failed", http.StatusBadRequest))
		return
	}

	// Apply defaults
	params := h.manager.ApplyDefaults(script, req.Parameters)

	// Check for async execution mode
	asyncMode := r.URL.Query().Get("async") == "true"

	// Create execution record
	execID := uuid.New().String()
	now := time.Now()
	execution := &scripts.Execution{
		ID:          execID,
		ScriptID:    scriptID,
		Status:      scripts.ExecutionStatusPending,
		IsDryRun:    req.DryRun,
		TriggeredBy: getTriggeredBy(r),
		TriggeredIP: r.RemoteAddr,
		Parameters:  params,
		CreatedAt:   now,
	}

	if err := h.storage.SaveExecution(execution); err != nil {
		apperrors.WriteJSON(w, apperrors.Wrap(err, "SAVE_FAILED", "Failed to save execution record", http.StatusInternalServerError))
		return
	}

	// Async execution: submit to runner and return immediately
	if asyncMode && h.runner != nil {
		job := &scripts.ExecutionJob{
			ExecutionID: execID,
			Script:      script,
			Parameters:  params,
			DryRun:      req.DryRun,
			TriggeredBy: getTriggeredBy(r),
		}

		if err := h.runner.Submit(job); err != nil {
			if err == scripts.ErrQueueFull {
				apperrors.WriteJSON(w, apperrors.New("QUEUE_FULL", "Execution queue is full, try again later", http.StatusServiceUnavailable))
				return
			}
			apperrors.WriteJSON(w, apperrors.Wrap(err, "SUBMIT_FAILED", "Failed to submit execution job", http.StatusInternalServerError))
			return
		}

		scriptsLog.Info("Script execution queued (async)",
			"id", scriptID,
			"execution_id", execID)

		httputil.WriteJSON(w, http.StatusAccepted, map[string]interface{}{
			"executionId": execID,
			"scriptId":    scriptID,
			"status":      "pending",
			"async":       true,
			"pollUrl":     "/api/v1/executions/" + execID,
		})
		return
	}

	// Sync execution: execute and wait for result
	ctx, cancel := context.WithTimeout(r.Context(), ScriptExecutionTimeout)
	defer cancel()

	// Update status to running
	startTime := time.Now()
	h.storage.UpdateExecutionStatus(execID, scripts.ExecutionStatusRunning, &startTime, nil, nil, nil, "")

	result, err := h.executor.Execute(ctx, script, params, req.DryRun)

	// Update execution with results
	endTime := time.Now()
	var status scripts.ExecutionStatus
	var exitCode *int
	var output *string
	var errorMsg string

	if err != nil {
		status = scripts.ExecutionStatusFailed
		errorMsg = err.Error()
	} else if result.TimedOut {
		status = scripts.ExecutionStatusTimeout
		output = &result.Output
	} else if result.Cancelled {
		status = scripts.ExecutionStatusCancelled
		output = &result.Output
	} else if result.ExitCode != 0 {
		status = scripts.ExecutionStatusFailed
		exitCode = &result.ExitCode
		output = &result.Output
	} else {
		status = scripts.ExecutionStatusCompleted
		exitCode = &result.ExitCode
		output = &result.Output
	}

	h.storage.UpdateExecutionStatus(execID, status, nil, &endTime, exitCode, output, errorMsg)

	// Get updated execution
	execution, _ = h.storage.GetExecution(execID)

	scriptsLog.Info("Script executed", "id", scriptID, "execution_id", execID, "status", status)

	httputil.WriteJSON(w, http.StatusOK, execution)
}

// DryRunScript handles POST /api/v1/scripts/{id}/dry-run
func (h *ScriptsHandler) DryRunScript(w http.ResponseWriter, r *http.Request) {
	scriptID := r.PathValue("id")
	if scriptID == "" {
		apperrors.WriteJSON(w, apperrors.New("MISSING_SCRIPT_ID", "Script ID is required", http.StatusBadRequest))
		return
	}

	// Get script
	script, err := h.storage.GetScript(scriptID)
	if err != nil {
		apperrors.WriteJSON(w, apperrors.Wrap(err, "GET_FAILED", "Failed to get script", http.StatusInternalServerError))
		return
	}
	if script == nil {
		apperrors.WriteJSON(w, apperrors.New("SCRIPT_NOT_FOUND", "Script not found", http.StatusNotFound))
		return
	}

	// Parse request
	var req scripts.ExecuteRequest
	if r.ContentLength > 0 {
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, MaxScriptRequestBodySize)).Decode(&req); err != nil {
			apperrors.WriteJSON(w, apperrors.New("INVALID_JSON", "Invalid request body", http.StatusBadRequest))
			return
		}
	}

	// Force dry run
	req.DryRun = true

	// Validate and apply defaults
	if err := h.manager.ValidateParameters(script, req.Parameters); err != nil {
		apperrors.WriteJSON(w, apperrors.Wrap(err, "INVALID_PARAMS", "Parameter validation failed", http.StatusBadRequest))
		return
	}
	params := h.manager.ApplyDefaults(script, req.Parameters)

	// Execute dry run
	ctx, cancel := context.WithTimeout(r.Context(), ScriptExecutionTimeout)
	defer cancel()

	result, err := h.executor.Execute(ctx, script, params, true)
	if err != nil {
		apperrors.WriteJSON(w, apperrors.Wrap(err, "EXECUTION_FAILED", "Dry run failed", http.StatusInternalServerError))
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"dryRun":   true,
		"exitCode": result.ExitCode,
		"output":   result.Output,
		"duration": result.Duration.String(),
	})
}

// ListExecutions handles GET /api/v1/executions
func (h *ScriptsHandler) ListExecutions(w http.ResponseWriter, r *http.Request) {
	scriptID := r.URL.Query().Get("scriptId")
	status := r.URL.Query().Get("status")

	limit := 50
	offset := 0
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}
	if o := r.URL.Query().Get("offset"); o != "" {
		if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
			offset = parsed
		}
	}

	executions, err := h.storage.ListExecutions(scriptID, status, limit, offset)
	if err != nil {
		apperrors.WriteJSON(w, apperrors.Wrap(err, "LIST_FAILED", "Failed to list executions", http.StatusInternalServerError))
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"executions": executions,
		"count":      len(executions),
	})
}

// GetExecution handles GET /api/v1/executions/{id}
func (h *ScriptsHandler) GetExecution(w http.ResponseWriter, r *http.Request) {
	execID := r.PathValue("id")
	if execID == "" {
		apperrors.WriteJSON(w, apperrors.New("MISSING_EXECUTION_ID", "Execution ID is required", http.StatusBadRequest))
		return
	}

	execution, err := h.storage.GetExecution(execID)
	if err != nil {
		apperrors.WriteJSON(w, apperrors.Wrap(err, "GET_FAILED", "Failed to get execution", http.StatusInternalServerError))
		return
	}
	if execution == nil {
		apperrors.WriteJSON(w, apperrors.New("EXECUTION_NOT_FOUND", "Execution not found", http.StatusNotFound))
		return
	}

	httputil.WriteJSON(w, http.StatusOK, execution)
}

// --- Workflow Endpoints ---

// ListWorkflows handles GET /api/v1/workflows
func (h *ScriptsHandler) ListWorkflows(w http.ResponseWriter, r *http.Request) {
	workflows, err := h.storage.ListWorkflows()
	if err != nil {
		apperrors.WriteJSON(w, apperrors.Wrap(err, "LIST_FAILED", "Failed to list workflows", http.StatusInternalServerError))
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"workflows": workflows,
		"count":     len(workflows),
	})
}

// GetWorkflow handles GET /api/v1/workflows/{id}
func (h *ScriptsHandler) GetWorkflow(w http.ResponseWriter, r *http.Request) {
	workflowID := r.PathValue("id")
	if workflowID == "" {
		apperrors.WriteJSON(w, apperrors.New("MISSING_WORKFLOW_ID", "Workflow ID is required", http.StatusBadRequest))
		return
	}

	workflow, err := h.storage.GetWorkflow(workflowID)
	if err != nil {
		apperrors.WriteJSON(w, apperrors.Wrap(err, "GET_FAILED", "Failed to get workflow", http.StatusInternalServerError))
		return
	}
	if workflow == nil {
		apperrors.WriteJSON(w, apperrors.New("WORKFLOW_NOT_FOUND", "Workflow not found", http.StatusNotFound))
		return
	}

	httputil.WriteJSON(w, http.StatusOK, workflow)
}

// CreateWorkflowRequest represents a request to create a workflow.
type CreateWorkflowRequest struct {
	Name          string                 `json:"name"`
	Description   string                 `json:"description,omitempty"`
	Steps         []scripts.WorkflowStep `json:"steps"`
	AllowedScopes []string               `json:"allowedScopes,omitempty"`
}

// CreateWorkflow handles POST /api/v1/workflows
func (h *ScriptsHandler) CreateWorkflow(w http.ResponseWriter, r *http.Request) {
	var req CreateWorkflowRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, MaxScriptRequestBodySize)).Decode(&req); err != nil {
		apperrors.WriteJSON(w, apperrors.New("INVALID_JSON", "Invalid request body", http.StatusBadRequest))
		return
	}

	if req.Name == "" {
		apperrors.WriteJSON(w, apperrors.New("MISSING_NAME", "Workflow name is required", http.StatusBadRequest))
		return
	}
	if len(req.Steps) == 0 {
		apperrors.WriteJSON(w, apperrors.New("MISSING_STEPS", "At least one step is required", http.StatusBadRequest))
		return
	}

	workflowID := generateScriptID(req.Name)
	now := time.Now()

	workflow := &scripts.Workflow{
		ID:            workflowID,
		Name:          req.Name,
		Description:   req.Description,
		Steps:         req.Steps,
		AllowedScopes: req.AllowedScopes,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	if err := h.storage.SaveWorkflow(workflow); err != nil {
		apperrors.WriteJSON(w, apperrors.Wrap(err, "SAVE_FAILED", "Failed to save workflow", http.StatusInternalServerError))
		return
	}

	scriptsLog.Info("Workflow created", "id", workflowID, "name", req.Name)

	httputil.WriteJSON(w, http.StatusCreated, workflow)
}

// DeleteWorkflow handles DELETE /api/v1/workflows/{id}
func (h *ScriptsHandler) DeleteWorkflow(w http.ResponseWriter, r *http.Request) {
	workflowID := r.PathValue("id")
	if workflowID == "" {
		apperrors.WriteJSON(w, apperrors.New("MISSING_WORKFLOW_ID", "Workflow ID is required", http.StatusBadRequest))
		return
	}

	if err := h.storage.DeleteWorkflow(workflowID); err != nil {
		apperrors.WriteJSON(w, apperrors.Wrap(err, "DELETE_FAILED", "Failed to delete workflow", http.StatusInternalServerError))
		return
	}

	scriptsLog.Info("Workflow deleted", "id", workflowID)

	w.WriteHeader(http.StatusNoContent)
}

// --- Schedule Endpoints ---

// ListSchedules handles GET /api/v1/schedules
func (h *ScriptsHandler) ListSchedules(w http.ResponseWriter, r *http.Request) {
	schedules, err := h.storage.ListSchedules()
	if err != nil {
		apperrors.WriteJSON(w, apperrors.Wrap(err, "LIST_FAILED", "Failed to list schedules", http.StatusInternalServerError))
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"schedules": schedules,
		"count":     len(schedules),
	})
}

// GetSchedule handles GET /api/v1/schedules/{id}
func (h *ScriptsHandler) GetSchedule(w http.ResponseWriter, r *http.Request) {
	scheduleID := r.PathValue("id")
	if scheduleID == "" {
		apperrors.WriteJSON(w, apperrors.New("MISSING_SCHEDULE_ID", "Schedule ID is required", http.StatusBadRequest))
		return
	}

	schedule, err := h.storage.GetSchedule(scheduleID)
	if err != nil {
		apperrors.WriteJSON(w, apperrors.Wrap(err, "GET_FAILED", "Failed to get schedule", http.StatusInternalServerError))
		return
	}
	if schedule == nil {
		apperrors.WriteJSON(w, apperrors.New("SCHEDULE_NOT_FOUND", "Schedule not found", http.StatusNotFound))
		return
	}

	httputil.WriteJSON(w, http.StatusOK, schedule)
}

// CreateScheduleRequest represents a request to create a schedule.
type CreateScheduleRequest struct {
	ScriptID       string            `json:"scriptId,omitempty"`
	WorkflowID     string            `json:"workflowId,omitempty"`
	CronExpression string            `json:"cronExpression"`
	Parameters     map[string]string `json:"parameters,omitempty"`
	Enabled        bool              `json:"enabled"`
}

// CreateSchedule handles POST /api/v1/schedules
func (h *ScriptsHandler) CreateSchedule(w http.ResponseWriter, r *http.Request) {
	var req CreateScheduleRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, MaxScriptRequestBodySize)).Decode(&req); err != nil {
		apperrors.WriteJSON(w, apperrors.New("INVALID_JSON", "Invalid request body", http.StatusBadRequest))
		return
	}

	if req.ScriptID == "" && req.WorkflowID == "" {
		apperrors.WriteJSON(w, apperrors.New("MISSING_TARGET", "Either scriptId or workflowId is required", http.StatusBadRequest))
		return
	}
	if req.CronExpression == "" {
		apperrors.WriteJSON(w, apperrors.New("MISSING_CRON", "Cron expression is required", http.StatusBadRequest))
		return
	}

	// Validate cron expression
	cron, err := scripts.ParseCronExpression(req.CronExpression)
	if err != nil {
		apperrors.WriteJSON(w, apperrors.Wrap(err, "INVALID_CRON", "Invalid cron expression", http.StatusBadRequest))
		return
	}

	scheduleID := uuid.New().String()
	nextRun := cron.NextRun(time.Now())
	now := time.Now()

	schedule := &scripts.Schedule{
		ID:             scheduleID,
		ScriptID:       req.ScriptID,
		WorkflowID:     req.WorkflowID,
		CronExpression: req.CronExpression,
		Parameters:     req.Parameters,
		Enabled:        req.Enabled,
		NextRunAt:      &nextRun,
		CreatedAt:      now,
	}

	if err := h.storage.SaveSchedule(schedule); err != nil {
		apperrors.WriteJSON(w, apperrors.Wrap(err, "SAVE_FAILED", "Failed to save schedule", http.StatusInternalServerError))
		return
	}

	scriptsLog.Info("Schedule created", "id", scheduleID, "cron", req.CronExpression)

	httputil.WriteJSON(w, http.StatusCreated, schedule)
}

// DeleteSchedule handles DELETE /api/v1/schedules/{id}
func (h *ScriptsHandler) DeleteSchedule(w http.ResponseWriter, r *http.Request) {
	scheduleID := r.PathValue("id")
	if scheduleID == "" {
		apperrors.WriteJSON(w, apperrors.New("MISSING_SCHEDULE_ID", "Schedule ID is required", http.StatusBadRequest))
		return
	}

	if err := h.storage.DeleteSchedule(scheduleID); err != nil {
		apperrors.WriteJSON(w, apperrors.Wrap(err, "DELETE_FAILED", "Failed to delete schedule", http.StatusInternalServerError))
		return
	}

	scriptsLog.Info("Schedule deleted", "id", scheduleID)

	w.WriteHeader(http.StatusNoContent)
}

// --- Helper Functions ---

// generateScriptID generates a URL-safe ID from a name.
func generateScriptID(name string) string {
	// Convert to lowercase and replace spaces with hyphens
	id := strings.ToLower(name)
	id = strings.ReplaceAll(id, " ", "-")

	// Remove non-alphanumeric characters except hyphens
	var result strings.Builder
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			result.WriteRune(r)
		}
	}

	return result.String()
}

// getTriggeredBy extracts the triggering entity from the request context.
func getTriggeredBy(r *http.Request) string {
	// Try to get device ID from context (set by auth middleware)
	if deviceID := r.Context().Value("deviceID"); deviceID != nil {
		return deviceID.(string)
	}

	// Try to get API key ID from context
	if apiKeyID := r.Context().Value("apiKeyID"); apiKeyID != nil {
		return "api_key:" + apiKeyID.(string)
	}

	// For IP-authenticated requests (web UI), use client IP
	clientIP := r.RemoteAddr
	if xForwardedFor := r.Header.Get("X-Forwarded-For"); xForwardedFor != "" {
		clientIP = xForwardedFor
	}
	// Strip port if present
	if host, _, err := net.SplitHostPort(clientIP); err == nil {
		clientIP = host
	}

	return "web:" + clientIP
}
