package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nstalgic/nekzus/internal/scripts"
	"github.com/nstalgic/nekzus/internal/storage"
)

// setupScriptsTestEnv creates a test environment for scripts handler tests
func setupScriptsTestEnv(t *testing.T) (*storage.Store, *scripts.Manager, *scripts.Executor, string, func()) {
	t.Helper()

	// Create temp database
	tmpFile, err := os.CreateTemp("", "nekzus-scripts-test-*.db")
	if err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()
	dbPath := tmpFile.Name()

	store, err := storage.NewStore(storage.Config{
		DatabasePath: dbPath,
	})
	if err != nil {
		os.Remove(dbPath)
		t.Fatal(err)
	}

	// Create temp scripts directory
	scriptsDir := t.TempDir()

	// Create a simple test script
	echoScript := `#!/bin/bash
echo "Hello from test script"
echo "PARAM1=$PARAM1"
exit 0
`
	if err := os.WriteFile(filepath.Join(scriptsDir, "echo.sh"), []byte(echoScript), 0755); err != nil {
		t.Fatal(err)
	}

	// Create a slow script for async testing
	slowScript := `#!/bin/bash
echo "Starting..."
sleep 2
echo "Done"
exit 0
`
	if err := os.WriteFile(filepath.Join(scriptsDir, "slow.sh"), []byte(slowScript), 0755); err != nil {
		t.Fatal(err)
	}

	manager := scripts.NewManager(scriptsDir)
	executor := scripts.NewExecutor(manager, scripts.ExecutorConfig{
		DefaultTimeout: 30 * time.Second,
		MaxOutputBytes: 1024 * 1024,
	})

	cleanup := func() {
		store.Close()
		os.Remove(dbPath)
	}

	return store, manager, executor, scriptsDir, cleanup
}

// registerTestScript registers a test script in storage
func registerTestScript(t *testing.T, store *storage.Store, id, path string) *scripts.Script {
	t.Helper()
	script := &scripts.Script{
		ID:             id,
		Name:           "Test Script " + id,
		Description:    "A test script",
		Category:       "test",
		ScriptPath:     path,
		ScriptType:     scripts.ScriptTypeShell,
		TimeoutSeconds: 30,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	if err := store.SaveScript(script); err != nil {
		t.Fatalf("Failed to save test script: %v", err)
	}
	return script
}

// TestExecuteScript_SyncMode_ReturnsFullResult tests that sync mode (default)
// blocks until execution completes and returns the full result.
func TestExecuteScript_SyncMode_ReturnsFullResult(t *testing.T) {
	store, manager, executor, _, cleanup := setupScriptsTestEnv(t)
	defer cleanup()

	// Register test script
	script := registerTestScript(t, store, "echo-test", "echo.sh")

	handler := NewScriptsHandler(manager, executor, nil, nil, nil, store)

	// Execute without async param (sync mode)
	body := bytes.NewReader([]byte(`{"parameters": {"PARAM1": "test_value"}}`))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/scripts/"+script.ID+"/execute", body)
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", script.ID)
	w := httptest.NewRecorder()

	handler.ExecuteScript(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var result scripts.Execution
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	// Verify sync mode returns completed execution
	if result.Status != scripts.ExecutionStatusCompleted {
		t.Errorf("expected status completed, got %s", result.Status)
	}
	if result.Output == "" {
		t.Error("expected output in sync response")
	}
	if result.ExitCode == nil || *result.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %v", result.ExitCode)
	}
}

// TestExecuteScript_AsyncMode_ReturnsImmediately tests that async mode
// returns immediately with an execution ID and 202 status.
func TestExecuteScript_AsyncMode_ReturnsImmediately(t *testing.T) {
	store, manager, executor, _, cleanup := setupScriptsTestEnv(t)
	defer cleanup()

	// Register slow test script
	script := registerTestScript(t, store, "slow-test", "slow.sh")

	// Create a mock runner that captures submitted jobs
	mockRunner := &mockScriptRunner{
		submitCalled: make(chan *scripts.ExecutionJob, 1),
	}

	handler := NewScriptsHandler(manager, executor, mockRunner, nil, nil, store)

	// Execute with async=true
	body := bytes.NewReader([]byte(`{}`))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/scripts/"+script.ID+"/execute?async=true", body)
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", script.ID)
	w := httptest.NewRecorder()

	start := time.Now()
	handler.ExecuteScript(w, req)
	elapsed := time.Since(start)

	// Should return quickly (not wait for 2 second script)
	if elapsed > 500*time.Millisecond {
		t.Errorf("async execution took too long: %v", elapsed)
	}

	// Should return 202 Accepted
	if w.Code != http.StatusAccepted {
		t.Errorf("expected status 202, got %d: %s", w.Code, w.Body.String())
	}

	var result map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	// Verify async response format
	if result["executionId"] == nil {
		t.Error("expected executionId in async response")
	}
	if result["status"] != "pending" {
		t.Errorf("expected status pending, got %v", result["status"])
	}
	if result["async"] != true {
		t.Errorf("expected async=true, got %v", result["async"])
	}
	if result["pollUrl"] == nil {
		t.Error("expected pollUrl in async response")
	}

	// Verify runner was called
	select {
	case job := <-mockRunner.submitCalled:
		if job.Script.ID != script.ID {
			t.Errorf("expected script ID %s, got %s", script.ID, job.Script.ID)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("expected runner Submit to be called")
	}
}

// TestExecuteScript_AsyncMode_ValidatesBeforeDispatch tests that parameter
// validation happens before async dispatch.
func TestExecuteScript_AsyncMode_ValidatesBeforeDispatch(t *testing.T) {
	store, manager, executor, _, cleanup := setupScriptsTestEnv(t)
	defer cleanup()

	// Register script with required parameter
	script := &scripts.Script{
		ID:             "param-test",
		Name:           "Param Test",
		Category:       "test",
		ScriptPath:     "echo.sh",
		ScriptType:     scripts.ScriptTypeShell,
		TimeoutSeconds: 30,
		Parameters: []scripts.ScriptParameter{
			{
				Name:     "REQUIRED_PARAM",
				Label:    "Required Parameter",
				Type:     "text",
				Required: true,
			},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := store.SaveScript(script); err != nil {
		t.Fatal(err)
	}

	mockRunner := &mockScriptRunner{
		submitCalled: make(chan *scripts.ExecutionJob, 1),
	}

	handler := NewScriptsHandler(manager, executor, mockRunner, nil, nil, store)

	// Execute with async=true but missing required parameter
	body := bytes.NewReader([]byte(`{}`))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/scripts/"+script.ID+"/execute?async=true", body)
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", script.ID)
	w := httptest.NewRecorder()

	handler.ExecuteScript(w, req)

	// Should return 400 Bad Request (validation error before dispatch)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d: %s", w.Code, w.Body.String())
	}

	// Runner should NOT be called
	select {
	case <-mockRunner.submitCalled:
		t.Error("runner should not be called for validation failures")
	case <-time.After(50 * time.Millisecond):
		// Expected: no submission
	}
}

// TestExecuteScript_AsyncMode_ScriptNotFound tests 404 for non-existent script.
func TestExecuteScript_AsyncMode_ScriptNotFound(t *testing.T) {
	store, manager, executor, _, cleanup := setupScriptsTestEnv(t)
	defer cleanup()

	handler := NewScriptsHandler(manager, executor, nil, nil, nil, store)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/scripts/nonexistent/execute?async=true", nil)
	req.SetPathValue("id", "nonexistent")
	w := httptest.NewRecorder()

	handler.ExecuteScript(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", w.Code)
	}
}

// TestExecuteScript_AsyncMode_MissingScriptID tests 400 for missing script ID.
func TestExecuteScript_AsyncMode_MissingScriptID(t *testing.T) {
	store, manager, executor, _, cleanup := setupScriptsTestEnv(t)
	defer cleanup()

	handler := NewScriptsHandler(manager, executor, nil, nil, nil, store)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/scripts//execute?async=true", nil)
	req.SetPathValue("id", "")
	w := httptest.NewRecorder()

	handler.ExecuteScript(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

// TestExecuteScript_AsyncMode_RunnerQueueFull tests error handling when runner queue is full.
func TestExecuteScript_AsyncMode_RunnerQueueFull(t *testing.T) {
	store, manager, executor, _, cleanup := setupScriptsTestEnv(t)
	defer cleanup()

	script := registerTestScript(t, store, "echo-test", "echo.sh")

	// Create runner that returns queue full error
	mockRunner := &mockScriptRunner{
		submitError: scripts.ErrQueueFull,
	}

	handler := NewScriptsHandler(manager, executor, mockRunner, nil, nil, store)

	body := bytes.NewReader([]byte(`{}`))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/scripts/"+script.ID+"/execute?async=true", body)
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", script.ID)
	w := httptest.NewRecorder()

	handler.ExecuteScript(w, req)

	// Should return 503 Service Unavailable
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status 503, got %d: %s", w.Code, w.Body.String())
	}
}

// TestExecuteScript_AsyncFalseExplicit tests that async=false explicitly triggers sync mode.
func TestExecuteScript_AsyncFalseExplicit(t *testing.T) {
	store, manager, executor, _, cleanup := setupScriptsTestEnv(t)
	defer cleanup()

	script := registerTestScript(t, store, "echo-test", "echo.sh")

	handler := NewScriptsHandler(manager, executor, nil, nil, nil, store)

	body := bytes.NewReader([]byte(`{}`))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/scripts/"+script.ID+"/execute?async=false", body)
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", script.ID)
	w := httptest.NewRecorder()

	handler.ExecuteScript(w, req)

	// Should return 200 OK (sync mode)
	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var result scripts.Execution
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	// Should have full execution result
	if result.Status != scripts.ExecutionStatusCompleted {
		t.Errorf("expected status completed, got %s", result.Status)
	}
}

// TestExecuteScript_AsyncMode_CreatesExecutionRecord tests that execution record
// is created before async dispatch.
func TestExecuteScript_AsyncMode_CreatesExecutionRecord(t *testing.T) {
	store, manager, executor, _, cleanup := setupScriptsTestEnv(t)
	defer cleanup()

	script := registerTestScript(t, store, "echo-test", "echo.sh")

	mockRunner := &mockScriptRunner{
		submitCalled: make(chan *scripts.ExecutionJob, 1),
	}

	handler := NewScriptsHandler(manager, executor, mockRunner, nil, nil, store)

	body := bytes.NewReader([]byte(`{}`))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/scripts/"+script.ID+"/execute?async=true", body)
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", script.ID)
	w := httptest.NewRecorder()

	handler.ExecuteScript(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("expected status 202, got %d", w.Code)
	}

	var result map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &result)
	execID := result["executionId"].(string)

	// Verify execution record exists in storage
	exec, err := store.GetExecution(execID)
	if err != nil {
		t.Fatalf("failed to get execution: %v", err)
	}
	if exec == nil {
		t.Fatal("execution record not found in storage")
	}
	if exec.Status != scripts.ExecutionStatusPending {
		t.Errorf("expected status pending, got %s", exec.Status)
	}
}

// mockScriptRunner is a mock implementation of the Runner interface for testing
type mockScriptRunner struct {
	submitCalled chan *scripts.ExecutionJob
	submitError  error
}

func (m *mockScriptRunner) Submit(job *scripts.ExecutionJob) error {
	if m.submitError != nil {
		return m.submitError
	}
	if m.submitCalled != nil {
		m.submitCalled <- job
	}
	return nil
}

func (m *mockScriptRunner) Start(ctx context.Context) error {
	return nil
}

func (m *mockScriptRunner) Stop() {
}
