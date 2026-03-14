package scripts

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"time"
)

// ExecutorConfig holds executor configuration.
type ExecutorConfig struct {
	DefaultTimeout time.Duration // Default execution timeout
	MaxOutputBytes int           // Maximum output size before truncation
}

// ExecutionResult holds the result of a script execution.
type ExecutionResult struct {
	ExitCode  int
	Output    string
	TimedOut  bool
	Cancelled bool
	Duration  time.Duration
}

// Executor handles script execution.
type Executor struct {
	manager *Manager
	config  ExecutorConfig
}

// NewExecutor creates a new script executor.
func NewExecutor(manager *Manager, config ExecutorConfig) *Executor {
	if config.DefaultTimeout == 0 {
		config.DefaultTimeout = 5 * time.Minute
	}
	if config.MaxOutputBytes == 0 {
		config.MaxOutputBytes = 10 * 1024 * 1024 // 10MB
	}

	return &Executor{
		manager: manager,
		config:  config,
	}
}

// Execute runs a script with the given parameters.
func (e *Executor) Execute(ctx context.Context, script *Script, params map[string]string, dryRun bool) (*ExecutionResult, error) {
	// Validate script exists
	if err := e.manager.ValidateScriptExists(script); err != nil {
		return nil, err
	}

	// Determine timeout
	timeout := time.Duration(script.TimeoutSeconds) * time.Second
	if timeout == 0 {
		timeout = e.config.DefaultTimeout
	}

	// Create context with timeout
	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Get full script path
	scriptPath := e.manager.GetScriptPath(script)

	// Build command based on script type
	var cmd *exec.Cmd
	switch script.ScriptType {
	case ScriptTypeShell:
		cmd = exec.CommandContext(execCtx, "/bin/bash", scriptPath)
	case ScriptTypePython:
		cmd = exec.CommandContext(execCtx, "python3", scriptPath)
	case ScriptTypeGoBinary:
		cmd = exec.CommandContext(execCtx, scriptPath)
	default:
		// Default to executing directly
		cmd = exec.CommandContext(execCtx, scriptPath)
	}

	// Ensure cmd.Wait returns promptly after the process is killed,
	// even if child processes still hold I/O pipes open.
	cmd.WaitDelay = 500 * time.Millisecond

	// Build environment
	cmd.Env = e.manager.BuildEnvironment(script, params, dryRun)

	// Capture output (combined stdout and stderr)
	var outputBuffer bytes.Buffer
	cmd.Stdout = &outputBuffer
	cmd.Stderr = &outputBuffer

	// Track execution time
	startTime := time.Now()

	// Run the command
	err := cmd.Run()
	duration := time.Since(startTime)

	// Build result
	result := &ExecutionResult{
		Duration: duration,
	}

	// Get output (with truncation if needed)
	output := outputBuffer.String()
	if len(output) > e.config.MaxOutputBytes {
		truncationMsg := fmt.Sprintf("\n\n[Output truncated at %d bytes]", e.config.MaxOutputBytes)
		output = output[:e.config.MaxOutputBytes] + truncationMsg
	}
	result.Output = output

	// Handle execution result
	if err != nil {
		// Check if it was a context error (timeout or cancellation)
		if execCtx.Err() == context.DeadlineExceeded {
			result.TimedOut = true
			result.ExitCode = -1
			return result, nil
		}
		if execCtx.Err() == context.Canceled || ctx.Err() == context.Canceled {
			result.Cancelled = true
			result.ExitCode = -1
			return result, nil
		}

		// Get exit code from error
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else {
			// Other error (e.g., command not found)
			result.ExitCode = -1
		}
		return result, nil
	}

	// Success
	result.ExitCode = 0
	return result, nil
}

// ExecuteAsync executes a script asynchronously and returns a channel for the result.
func (e *Executor) ExecuteAsync(ctx context.Context, script *Script, params map[string]string, dryRun bool) <-chan *ExecutionResult {
	resultChan := make(chan *ExecutionResult, 1)

	go func() {
		defer close(resultChan)
		result, err := e.Execute(ctx, script, params, dryRun)
		if err != nil {
			// Return error as a failed result
			resultChan <- &ExecutionResult{
				ExitCode: -1,
				Output:   fmt.Sprintf("Execution error: %v", err),
			}
			return
		}
		resultChan <- result
	}()

	return resultChan
}
