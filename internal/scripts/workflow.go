package scripts

import (
	"context"
	"fmt"
	"time"
)

// WorkflowResult holds the result of a workflow execution.
type WorkflowResult struct {
	Status       ExecutionStatus
	StepResults  []*ExecutionResult
	FailedStep   int  // Index of failed step (-1 if none)
	StepsRun     int  // Number of steps that were executed
	HadFailure   bool // Whether any step failed
	Duration     time.Duration
}

// WorkflowRunner executes workflows (sequences of scripts).
type WorkflowRunner struct {
	executor *Executor
}

// NewWorkflowRunner creates a new workflow runner.
func NewWorkflowRunner(executor *Executor) *WorkflowRunner {
	return &WorkflowRunner{
		executor: executor,
	}
}

// Execute runs a workflow with the given scripts and global parameters.
// scriptsMap provides the Script definitions keyed by script ID.
// globalParams are merged with step-specific parameters.
func (r *WorkflowRunner) Execute(ctx context.Context, workflow *Workflow, scriptsMap map[string]*Script, globalParams map[string]string) (*WorkflowResult, error) {
	startTime := time.Now()

	result := &WorkflowResult{
		Status:      ExecutionStatusRunning,
		StepResults: make([]*ExecutionResult, 0, len(workflow.Steps)),
		FailedStep:  -1,
	}

	for i, step := range workflow.Steps {
		// Check for cancellation before each step
		select {
		case <-ctx.Done():
			result.Status = ExecutionStatusCancelled
			result.Duration = time.Since(startTime)
			return result, nil
		default:
		}

		// Get script definition
		script, exists := scriptsMap[step.ScriptID]
		if !exists {
			return nil, fmt.Errorf("script not found: %s (step %d)", step.ScriptID, i+1)
		}

		// Merge parameters: global params + step params (step params take precedence)
		params := mergeParams(globalParams, step.Parameters)

		// Execute the step
		stepResult, err := r.executor.Execute(ctx, script, params, false)
		if err != nil {
			return nil, fmt.Errorf("failed to execute step %d (%s): %w", i+1, step.ScriptID, err)
		}

		result.StepResults = append(result.StepResults, stepResult)
		result.StepsRun = i + 1

		// Check for cancellation
		if stepResult.Cancelled {
			result.Status = ExecutionStatusCancelled
			result.Duration = time.Since(startTime)
			return result, nil
		}

		// Check for timeout
		if stepResult.TimedOut {
			result.Status = ExecutionStatusTimeout
			result.FailedStep = i
			result.HadFailure = true
			result.Duration = time.Since(startTime)
			return result, nil
		}

		// Check for failure
		if stepResult.ExitCode != 0 {
			result.HadFailure = true
			result.FailedStep = i

			if step.OnFailure == FailureActionStop || step.OnFailure == "" {
				// Default is stop on failure
				result.Status = ExecutionStatusFailed
				result.Duration = time.Since(startTime)
				return result, nil
			}
			// OnFailure == FailureActionContinue: continue to next step
		}
	}

	// All steps completed
	if result.HadFailure {
		result.Status = ExecutionStatusFailed
	} else {
		result.Status = ExecutionStatusCompleted
	}
	result.Duration = time.Since(startTime)

	return result, nil
}

// mergeParams merges two parameter maps, with overrides taking precedence.
func mergeParams(base, overrides map[string]string) map[string]string {
	if base == nil && overrides == nil {
		return nil
	}

	result := make(map[string]string)

	// Copy base params
	for k, v := range base {
		result[k] = v
	}

	// Apply overrides
	for k, v := range overrides {
		result[k] = v
	}

	return result
}
