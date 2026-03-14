package scripts

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func setupExecutorTestDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	// Create a simple test script that echoes and exits
	echoScript := `#!/bin/bash
echo "Hello from test script"
echo "PARAM1=$PARAM1"
exit 0
`
	if err := os.WriteFile(filepath.Join(dir, "echo.sh"), []byte(echoScript), 0755); err != nil {
		t.Fatal(err)
	}

	// Create a script that sleeps (for timeout testing)
	sleepScript := `#!/bin/bash
echo "Starting sleep..."
sleep 10
echo "Sleep done"
exit 0
`
	if err := os.WriteFile(filepath.Join(dir, "sleep.sh"), []byte(sleepScript), 0755); err != nil {
		t.Fatal(err)
	}

	// Create a script that fails
	failScript := `#!/bin/bash
echo "About to fail" >&2
exit 1
`
	if err := os.WriteFile(filepath.Join(dir, "fail.sh"), []byte(failScript), 0755); err != nil {
		t.Fatal(err)
	}

	// Create a dry run aware script
	dryRunScript := `#!/bin/bash
if [ "$DRY_RUN" = "true" ]; then
    echo "DRY RUN: Would do something"
    exit 0
fi
echo "Actually doing something"
exit 0
`
	if err := os.WriteFile(filepath.Join(dir, "dryrun.sh"), []byte(dryRunScript), 0755); err != nil {
		t.Fatal(err)
	}

	return dir
}

func TestExecutor_ExecuteScript_Success(t *testing.T) {
	scriptsDir := setupExecutorTestDir(t)
	manager := NewManager(scriptsDir)
	executor := NewExecutor(manager, ExecutorConfig{
		DefaultTimeout: 30 * time.Second,
		MaxOutputBytes: 1024 * 1024,
	})

	script := &Script{
		ID:             "echo-test",
		Name:           "Echo Test",
		ScriptPath:     "echo.sh",
		ScriptType:     ScriptTypeShell,
		TimeoutSeconds: 30,
	}

	params := map[string]string{
		"PARAM1": "test_value",
	}

	ctx := context.Background()
	result, err := executor.Execute(ctx, script, params, false)
	if err != nil {
		t.Fatalf("Failed to execute script: %v", err)
	}

	if result.ExitCode != 0 {
		t.Errorf("Expected exit code 0, got %d", result.ExitCode)
	}

	if !strings.Contains(result.Output, "Hello from test script") {
		t.Errorf("Expected output to contain 'Hello from test script', got: %s", result.Output)
	}

	if !strings.Contains(result.Output, "PARAM1=test_value") {
		t.Errorf("Expected output to contain 'PARAM1=test_value', got: %s", result.Output)
	}
}

func TestExecutor_ExecuteScript_Failure(t *testing.T) {
	scriptsDir := setupExecutorTestDir(t)
	manager := NewManager(scriptsDir)
	executor := NewExecutor(manager, ExecutorConfig{
		DefaultTimeout: 30 * time.Second,
		MaxOutputBytes: 1024 * 1024,
	})

	script := &Script{
		ID:             "fail-test",
		Name:           "Fail Test",
		ScriptPath:     "fail.sh",
		ScriptType:     ScriptTypeShell,
		TimeoutSeconds: 30,
	}

	ctx := context.Background()
	result, err := executor.Execute(ctx, script, nil, false)
	if err != nil {
		t.Fatalf("Execute should not return error for script failure: %v", err)
	}

	if result.ExitCode != 1 {
		t.Errorf("Expected exit code 1, got %d", result.ExitCode)
	}

	if !strings.Contains(result.Output, "About to fail") {
		t.Errorf("Expected output to contain stderr, got: %s", result.Output)
	}
}

func TestExecutor_ExecuteScript_Timeout(t *testing.T) {
	scriptsDir := setupExecutorTestDir(t)
	manager := NewManager(scriptsDir)
	executor := NewExecutor(manager, ExecutorConfig{
		DefaultTimeout: 1 * time.Second, // Short timeout
		MaxOutputBytes: 1024 * 1024,
	})

	script := &Script{
		ID:             "timeout-test",
		Name:           "Timeout Test",
		ScriptPath:     "sleep.sh",
		ScriptType:     ScriptTypeShell,
		TimeoutSeconds: 1, // 1 second timeout
	}

	ctx := context.Background()
	result, err := executor.Execute(ctx, script, nil, false)
	if err != nil {
		t.Fatalf("Execute should not return error for timeout: %v", err)
	}

	if !result.TimedOut {
		t.Error("Expected script to time out")
	}

	// Output should contain what was produced before timeout
	if !strings.Contains(result.Output, "Starting sleep...") {
		t.Errorf("Expected partial output, got: %s", result.Output)
	}
}

func TestExecutor_ExecuteScript_Cancellation(t *testing.T) {
	scriptsDir := setupExecutorTestDir(t)
	manager := NewManager(scriptsDir)
	executor := NewExecutor(manager, ExecutorConfig{
		DefaultTimeout: 30 * time.Second,
		MaxOutputBytes: 1024 * 1024,
	})

	script := &Script{
		ID:             "cancel-test",
		Name:           "Cancel Test",
		ScriptPath:     "sleep.sh",
		ScriptType:     ScriptTypeShell,
		TimeoutSeconds: 30,
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Cancel after a short delay
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	result, err := executor.Execute(ctx, script, nil, false)
	if err != nil {
		t.Fatalf("Execute should not return error for cancellation: %v", err)
	}

	if !result.Cancelled {
		t.Error("Expected script to be cancelled")
	}
}

func TestExecutor_ExecuteScript_DryRun(t *testing.T) {
	scriptsDir := setupExecutorTestDir(t)
	manager := NewManager(scriptsDir)
	executor := NewExecutor(manager, ExecutorConfig{
		DefaultTimeout: 30 * time.Second,
		MaxOutputBytes: 1024 * 1024,
	})

	script := &Script{
		ID:             "dryrun-test",
		Name:           "Dry Run Test",
		ScriptPath:     "dryrun.sh",
		ScriptType:     ScriptTypeShell,
		TimeoutSeconds: 30,
	}

	ctx := context.Background()

	// Test with dry run enabled
	result, err := executor.Execute(ctx, script, nil, true)
	if err != nil {
		t.Fatalf("Failed to execute dry run: %v", err)
	}

	if !strings.Contains(result.Output, "DRY RUN") {
		t.Errorf("Expected dry run output, got: %s", result.Output)
	}

	// Test without dry run
	result2, err := executor.Execute(ctx, script, nil, false)
	if err != nil {
		t.Fatalf("Failed to execute without dry run: %v", err)
	}

	if !strings.Contains(result2.Output, "Actually doing") {
		t.Errorf("Expected actual execution output, got: %s", result2.Output)
	}
}

func TestExecutor_ExecuteScript_NonExistent(t *testing.T) {
	scriptsDir := setupExecutorTestDir(t)
	manager := NewManager(scriptsDir)
	executor := NewExecutor(manager, ExecutorConfig{
		DefaultTimeout: 30 * time.Second,
		MaxOutputBytes: 1024 * 1024,
	})

	script := &Script{
		ID:             "nonexistent",
		Name:           "Non-existent",
		ScriptPath:     "does-not-exist.sh",
		ScriptType:     ScriptTypeShell,
		TimeoutSeconds: 30,
	}

	ctx := context.Background()
	_, err := executor.Execute(ctx, script, nil, false)
	if err == nil {
		t.Error("Expected error for non-existent script")
	}
}

func TestExecutor_OutputTruncation(t *testing.T) {
	scriptsDir := t.TempDir()

	// Create a script that produces a lot of output
	bigOutputScript := `#!/bin/bash
for i in $(seq 1 1000); do
    echo "Line $i: AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
done
`
	if err := os.WriteFile(filepath.Join(scriptsDir, "bigoutput.sh"), []byte(bigOutputScript), 0755); err != nil {
		t.Fatal(err)
	}

	manager := NewManager(scriptsDir)
	executor := NewExecutor(manager, ExecutorConfig{
		DefaultTimeout: 30 * time.Second,
		MaxOutputBytes: 1000, // Small limit
	})

	script := &Script{
		ID:             "big-output",
		Name:           "Big Output",
		ScriptPath:     "bigoutput.sh",
		ScriptType:     ScriptTypeShell,
		TimeoutSeconds: 30,
	}

	ctx := context.Background()
	result, err := executor.Execute(ctx, script, nil, false)
	if err != nil {
		t.Fatalf("Failed to execute script: %v", err)
	}

	if len(result.Output) > 1100 { // Some buffer for truncation message
		t.Errorf("Expected output to be truncated, got %d bytes", len(result.Output))
	}

	if !strings.Contains(result.Output, "truncated") {
		t.Error("Expected truncation notice in output")
	}
}
