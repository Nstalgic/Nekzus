package scripts

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func setupWorkflowTestDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	// Script 1: Success
	script1 := `#!/bin/bash
echo "Step 1: Success"
echo "STEP1_OUTPUT=done"
exit 0
`
	if err := os.WriteFile(filepath.Join(dir, "step1.sh"), []byte(script1), 0755); err != nil {
		t.Fatal(err)
	}

	// Script 2: Uses output from step 1
	script2 := `#!/bin/bash
echo "Step 2: Processing"
echo "Received: $STEP1_OUTPUT"
exit 0
`
	if err := os.WriteFile(filepath.Join(dir, "step2.sh"), []byte(script2), 0755); err != nil {
		t.Fatal(err)
	}

	// Script 3: Fails
	script3 := `#!/bin/bash
echo "Step 3: Failing"
exit 1
`
	if err := os.WriteFile(filepath.Join(dir, "step3_fail.sh"), []byte(script3), 0755); err != nil {
		t.Fatal(err)
	}

	// Script 4: Final step
	script4 := `#!/bin/bash
echo "Step 4: Final"
exit 0
`
	if err := os.WriteFile(filepath.Join(dir, "step4.sh"), []byte(script4), 0755); err != nil {
		t.Fatal(err)
	}

	return dir
}

func TestWorkflowRunner_ExecuteWorkflow_Success(t *testing.T) {
	scriptsDir := setupWorkflowTestDir(t)
	manager := NewManager(scriptsDir)
	executor := NewExecutor(manager, ExecutorConfig{
		DefaultTimeout: 30 * time.Second,
		MaxOutputBytes: 1024 * 1024,
	})
	runner := NewWorkflowRunner(executor)

	// Create test scripts map
	scriptsMap := map[string]*Script{
		"step1": {ID: "step1", Name: "Step 1", ScriptPath: "step1.sh", ScriptType: ScriptTypeShell},
		"step2": {ID: "step2", Name: "Step 2", ScriptPath: "step2.sh", ScriptType: ScriptTypeShell},
	}

	workflow := &Workflow{
		ID:   "test-workflow",
		Name: "Test Workflow",
		Steps: []WorkflowStep{
			{ScriptID: "step1", OnFailure: FailureActionStop},
			{ScriptID: "step2", OnFailure: FailureActionStop},
		},
	}

	ctx := context.Background()
	result, err := runner.Execute(ctx, workflow, scriptsMap, nil)
	if err != nil {
		t.Fatalf("Failed to execute workflow: %v", err)
	}

	if result.Status != ExecutionStatusCompleted {
		t.Errorf("Expected status completed, got %s", result.Status)
	}

	if len(result.StepResults) != 2 {
		t.Errorf("Expected 2 step results, got %d", len(result.StepResults))
	}

	// Verify both steps succeeded
	for i, stepResult := range result.StepResults {
		if stepResult.ExitCode != 0 {
			t.Errorf("Step %d: expected exit code 0, got %d", i, stepResult.ExitCode)
		}
	}
}

func TestWorkflowRunner_ExecuteWorkflow_FailureStop(t *testing.T) {
	scriptsDir := setupWorkflowTestDir(t)
	manager := NewManager(scriptsDir)
	executor := NewExecutor(manager, ExecutorConfig{
		DefaultTimeout: 30 * time.Second,
		MaxOutputBytes: 1024 * 1024,
	})
	runner := NewWorkflowRunner(executor)

	scriptsMap := map[string]*Script{
		"step1":      {ID: "step1", Name: "Step 1", ScriptPath: "step1.sh", ScriptType: ScriptTypeShell},
		"step3_fail": {ID: "step3_fail", Name: "Step 3 Fail", ScriptPath: "step3_fail.sh", ScriptType: ScriptTypeShell},
		"step4":      {ID: "step4", Name: "Step 4", ScriptPath: "step4.sh", ScriptType: ScriptTypeShell},
	}

	workflow := &Workflow{
		ID:   "fail-workflow",
		Name: "Fail Workflow",
		Steps: []WorkflowStep{
			{ScriptID: "step1", OnFailure: FailureActionStop},
			{ScriptID: "step3_fail", OnFailure: FailureActionStop}, // This will fail
			{ScriptID: "step4", OnFailure: FailureActionStop},      // Should not run
		},
	}

	ctx := context.Background()
	result, err := runner.Execute(ctx, workflow, scriptsMap, nil)
	if err != nil {
		t.Fatalf("Failed to execute workflow: %v", err)
	}

	if result.Status != ExecutionStatusFailed {
		t.Errorf("Expected status failed, got %s", result.Status)
	}

	// Should have run 2 steps (step1 success, step3_fail failure)
	if len(result.StepResults) != 2 {
		t.Errorf("Expected 2 step results (stopped after failure), got %d", len(result.StepResults))
	}

	// First step should succeed
	if result.StepResults[0].ExitCode != 0 {
		t.Error("First step should have succeeded")
	}

	// Second step should fail
	if result.StepResults[1].ExitCode == 0 {
		t.Error("Second step should have failed")
	}
}

func TestWorkflowRunner_ExecuteWorkflow_FailureContinue(t *testing.T) {
	scriptsDir := setupWorkflowTestDir(t)
	manager := NewManager(scriptsDir)
	executor := NewExecutor(manager, ExecutorConfig{
		DefaultTimeout: 30 * time.Second,
		MaxOutputBytes: 1024 * 1024,
	})
	runner := NewWorkflowRunner(executor)

	scriptsMap := map[string]*Script{
		"step1":      {ID: "step1", Name: "Step 1", ScriptPath: "step1.sh", ScriptType: ScriptTypeShell},
		"step3_fail": {ID: "step3_fail", Name: "Step 3 Fail", ScriptPath: "step3_fail.sh", ScriptType: ScriptTypeShell},
		"step4":      {ID: "step4", Name: "Step 4", ScriptPath: "step4.sh", ScriptType: ScriptTypeShell},
	}

	workflow := &Workflow{
		ID:   "continue-workflow",
		Name: "Continue Workflow",
		Steps: []WorkflowStep{
			{ScriptID: "step1", OnFailure: FailureActionContinue},
			{ScriptID: "step3_fail", OnFailure: FailureActionContinue}, // Fails but continues
			{ScriptID: "step4", OnFailure: FailureActionContinue},      // Should still run
		},
	}

	ctx := context.Background()
	result, err := runner.Execute(ctx, workflow, scriptsMap, nil)
	if err != nil {
		t.Fatalf("Failed to execute workflow: %v", err)
	}

	// Status should still be failed (because a step failed)
	if result.Status != ExecutionStatusFailed {
		t.Errorf("Expected status failed, got %s", result.Status)
	}

	// All 3 steps should have run
	if len(result.StepResults) != 3 {
		t.Errorf("Expected 3 step results (continued after failure), got %d", len(result.StepResults))
	}

	// First step: success
	if result.StepResults[0].ExitCode != 0 {
		t.Error("First step should have succeeded")
	}

	// Second step: failure
	if result.StepResults[1].ExitCode == 0 {
		t.Error("Second step should have failed")
	}

	// Third step: success (ran despite previous failure)
	if result.StepResults[2].ExitCode != 0 {
		t.Error("Third step should have succeeded")
	}
}

func TestWorkflowRunner_ExecuteWorkflow_Cancellation(t *testing.T) {
	scriptsDir := t.TempDir()

	// Create a slow script
	slowScript := `#!/bin/bash
echo "Starting slow step..."
sleep 10
echo "Done"
`
	if err := os.WriteFile(filepath.Join(scriptsDir, "slow.sh"), []byte(slowScript), 0755); err != nil {
		t.Fatal(err)
	}

	manager := NewManager(scriptsDir)
	executor := NewExecutor(manager, ExecutorConfig{
		DefaultTimeout: 30 * time.Second,
		MaxOutputBytes: 1024 * 1024,
	})
	runner := NewWorkflowRunner(executor)

	scriptsMap := map[string]*Script{
		"slow": {ID: "slow", Name: "Slow", ScriptPath: "slow.sh", ScriptType: ScriptTypeShell},
	}

	workflow := &Workflow{
		ID:    "cancel-workflow",
		Name:  "Cancel Workflow",
		Steps: []WorkflowStep{{ScriptID: "slow"}},
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Cancel after a short delay
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	result, err := runner.Execute(ctx, workflow, scriptsMap, nil)
	if err != nil {
		t.Fatalf("Execute should not return error: %v", err)
	}

	if result.Status != ExecutionStatusCancelled {
		t.Errorf("Expected status cancelled, got %s", result.Status)
	}
}

func TestWorkflowRunner_ExecuteWorkflow_WithParameters(t *testing.T) {
	scriptsDir := t.TempDir()

	// Script that uses parameters
	paramScript := `#!/bin/bash
echo "Received: NAME=$NAME, VALUE=$VALUE"
exit 0
`
	if err := os.WriteFile(filepath.Join(scriptsDir, "param.sh"), []byte(paramScript), 0755); err != nil {
		t.Fatal(err)
	}

	manager := NewManager(scriptsDir)
	executor := NewExecutor(manager, ExecutorConfig{
		DefaultTimeout: 30 * time.Second,
		MaxOutputBytes: 1024 * 1024,
	})
	runner := NewWorkflowRunner(executor)

	scriptsMap := map[string]*Script{
		"param": {ID: "param", Name: "Param", ScriptPath: "param.sh", ScriptType: ScriptTypeShell},
	}

	workflow := &Workflow{
		ID:   "param-workflow",
		Name: "Param Workflow",
		Steps: []WorkflowStep{
			{
				ScriptID: "param",
				Parameters: map[string]string{
					"NAME":  "test",
					"VALUE": "123",
				},
			},
		},
	}

	ctx := context.Background()
	result, err := runner.Execute(ctx, workflow, scriptsMap, nil)
	if err != nil {
		t.Fatalf("Failed to execute workflow: %v", err)
	}

	if result.Status != ExecutionStatusCompleted {
		t.Errorf("Expected status completed, got %s", result.Status)
	}

	// Check that parameters were passed
	if !strings.Contains(result.StepResults[0].Output, "NAME=test") {
		t.Error("Expected NAME parameter in output")
	}
	if !strings.Contains(result.StepResults[0].Output, "VALUE=123") {
		t.Error("Expected VALUE parameter in output")
	}
}

func TestWorkflowRunner_MissingScript(t *testing.T) {
	scriptsDir := setupWorkflowTestDir(t)
	manager := NewManager(scriptsDir)
	executor := NewExecutor(manager, ExecutorConfig{
		DefaultTimeout: 30 * time.Second,
		MaxOutputBytes: 1024 * 1024,
	})
	runner := NewWorkflowRunner(executor)

	// Empty scripts map - script won't be found
	scriptsMap := map[string]*Script{}

	workflow := &Workflow{
		ID:    "missing-workflow",
		Name:  "Missing Workflow",
		Steps: []WorkflowStep{{ScriptID: "nonexistent"}},
	}

	ctx := context.Background()
	_, err := runner.Execute(ctx, workflow, scriptsMap, nil)
	if err == nil {
		t.Error("Expected error for missing script")
	}
}
