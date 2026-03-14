package scripts

import (
	"context"
	"time"
)

// ScriptExecutor is the interface for executing scripts.
// Both Executor (local) and ContainerExecutor implement this.
type ScriptExecutor interface {
	Execute(ctx context.Context, script *Script, params map[string]string, dryRun bool) (*ExecutionResult, error)
}

// Script represents a registered script in the system.
// Scripts are stored on the filesystem; this struct contains metadata.
type Script struct {
	ID             string            `json:"id"`
	Name           string            `json:"name"`
	Description    string            `json:"description,omitempty"`
	Category       string            `json:"category"`
	ScriptPath     string            `json:"scriptPath"`     // Relative path in scripts directory
	ScriptType     ScriptType        `json:"scriptType"`     // shell, go_binary, python
	TimeoutSeconds int               `json:"timeoutSeconds"` // Execution timeout (default: 300)
	Parameters     []ScriptParameter `json:"parameters,omitempty"`
	Environment    map[string]string `json:"environment,omitempty"` // Default environment variables
	AllowedScopes  []string          `json:"allowedScopes,omitempty"`
	DryRunCommand  string            `json:"dryRunCommand,omitempty"` // Command/flag for dry run mode
	CreatedBy      string            `json:"createdBy,omitempty"`
	CreatedAt      time.Time         `json:"createdAt"`
	UpdatedAt      time.Time         `json:"updatedAt"`
}

// ScriptType represents the type of script.
type ScriptType string

const (
	ScriptTypeShell    ScriptType = "shell"
	ScriptTypeGoBinary ScriptType = "go_binary"
	ScriptTypePython   ScriptType = "python"
)

// ScriptParameter defines a user-configurable parameter for a script.
type ScriptParameter struct {
	Name        string   `json:"name"`                  // Environment variable name
	Label       string   `json:"label"`                 // User-friendly label
	Description string   `json:"description,omitempty"` // Help text
	Type        string   `json:"type"`                  // text, password, number, boolean, select
	Required    bool     `json:"required"`
	Default     string   `json:"default,omitempty"`
	Options     []string `json:"options,omitempty"`    // Options for select type
	Validation  string   `json:"validation,omitempty"` // Regex pattern for validation
}

// Execution represents a script execution record.
type Execution struct {
	ID           string            `json:"id"`
	ScriptID     string            `json:"scriptId"`
	WorkflowID   string            `json:"workflowId,omitempty"`   // Set if part of a workflow
	WorkflowExID string            `json:"workflowExId,omitempty"` // Workflow execution ID
	Status       ExecutionStatus   `json:"status"`
	IsDryRun     bool              `json:"isDryRun"`
	TriggeredBy  string            `json:"triggeredBy"` // device_id, "scheduler", or "api_key:{id}"
	TriggeredIP  string            `json:"triggeredIp,omitempty"`
	Parameters   map[string]string `json:"parameters,omitempty"`
	Output       string            `json:"output,omitempty"`
	ExitCode     *int              `json:"exitCode,omitempty"`
	ErrorMessage string            `json:"errorMessage,omitempty"`
	StartedAt    *time.Time        `json:"startedAt,omitempty"`
	CompletedAt  *time.Time        `json:"completedAt,omitempty"`
	CreatedAt    time.Time         `json:"createdAt"`
}

// ExecutionStatus represents the status of an execution.
type ExecutionStatus string

const (
	ExecutionStatusPending   ExecutionStatus = "pending"
	ExecutionStatusRunning   ExecutionStatus = "running"
	ExecutionStatusCompleted ExecutionStatus = "completed"
	ExecutionStatusFailed    ExecutionStatus = "failed"
	ExecutionStatusCancelled ExecutionStatus = "cancelled"
	ExecutionStatusTimeout   ExecutionStatus = "timeout"
)

// Workflow represents a chain of scripts to execute sequentially.
type Workflow struct {
	ID            string         `json:"id"`
	Name          string         `json:"name"`
	Description   string         `json:"description,omitempty"`
	Steps         []WorkflowStep `json:"steps"`
	AllowedScopes []string       `json:"allowedScopes,omitempty"`
	CreatedBy     string         `json:"createdBy,omitempty"`
	CreatedAt     time.Time      `json:"createdAt"`
	UpdatedAt     time.Time      `json:"updatedAt"`
}

// WorkflowStep defines a single step in a workflow.
type WorkflowStep struct {
	ScriptID   string            `json:"scriptId"`
	Parameters map[string]string `json:"parameters,omitempty"`
	OnFailure  FailureAction     `json:"onFailure"` // stop or continue
}

// FailureAction defines what to do when a workflow step fails.
type FailureAction string

const (
	FailureActionStop     FailureAction = "stop"
	FailureActionContinue FailureAction = "continue"
)

// WorkflowExecution represents a workflow execution record.
type WorkflowExecution struct {
	ID          string          `json:"id"`
	WorkflowID  string          `json:"workflowId"`
	Status      ExecutionStatus `json:"status"`
	CurrentStep int             `json:"currentStep"`
	TriggeredBy string          `json:"triggeredBy"`
	StartedAt   *time.Time      `json:"startedAt,omitempty"`
	CompletedAt *time.Time      `json:"completedAt,omitempty"`
	CreatedAt   time.Time       `json:"createdAt"`
}

// Schedule represents a cron-based schedule for scripts or workflows.
type Schedule struct {
	ID             string            `json:"id"`
	ScriptID       string            `json:"scriptId,omitempty"`   // Either ScriptID or WorkflowID
	WorkflowID     string            `json:"workflowId,omitempty"` // Either ScriptID or WorkflowID
	CronExpression string            `json:"cronExpression"`
	Parameters     map[string]string `json:"parameters,omitempty"` // Default parameters for scheduled runs
	Enabled        bool              `json:"enabled"`
	LastRunAt      *time.Time        `json:"lastRunAt,omitempty"`
	NextRunAt      *time.Time        `json:"nextRunAt,omitempty"`
	CreatedBy      string            `json:"createdBy,omitempty"`
	CreatedAt      time.Time         `json:"createdAt"`
}

// AvailableScript represents a script file found in the scripts directory
// that has not yet been registered.
type AvailableScript struct {
	Path       string     `json:"path"`       // Relative path from scripts directory
	Name       string     `json:"name"`       // Filename
	ScriptType ScriptType `json:"scriptType"` // Detected type
	Size       int64      `json:"size"`       // File size in bytes
	ModTime    time.Time  `json:"modTime"`    // Last modification time
}

// ExecuteRequest represents a request to execute a script.
type ExecuteRequest struct {
	Parameters map[string]string `json:"parameters,omitempty"`
	DryRun     bool              `json:"dryRun,omitempty"`
}

// ScriptConfig holds script execution configuration.
type ScriptConfig struct {
	Enabled        bool   `yaml:"enabled" json:"enabled"`
	ScriptsDir     string `yaml:"scripts_dir" json:"scripts_dir"`         // Directory containing scripts
	DefaultTimeout int    `yaml:"default_timeout" json:"default_timeout"` // Default timeout in seconds
	MaxMemoryMB    int    `yaml:"max_memory_mb" json:"max_memory_mb"`     // Max memory in MB
	MaxOutputBytes int    `yaml:"max_output_bytes" json:"max_output_bytes"`
	WorkerCount    int    `yaml:"worker_count" json:"worker_count"` // Number of concurrent workers
}

// DefaultScriptConfig returns the default configuration.
func DefaultScriptConfig() ScriptConfig {
	return ScriptConfig{
		Enabled:        false,
		ScriptsDir:     "scripts",
		DefaultTimeout: 300,
		MaxMemoryMB:    512,
		MaxOutputBytes: 10 * 1024 * 1024, // 10MB
		WorkerCount:    3,
	}
}
