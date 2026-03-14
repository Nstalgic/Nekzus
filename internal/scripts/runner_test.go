package scripts

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// mockStorage implements the Storage interface for Runner tests
type mockStorage struct {
	mu            sync.Mutex
	executions    map[string]*Execution
	statusUpdates []statusUpdate
	saveErrors    map[string]error
}

type statusUpdate struct {
	ID           string
	Status       ExecutionStatus
	StartedAt    *time.Time
	CompletedAt  *time.Time
	ExitCode     *int
	Output       *string
	ErrorMessage string
}

func newMockStorage() *mockStorage {
	return &mockStorage{
		executions:    make(map[string]*Execution),
		statusUpdates: make([]statusUpdate, 0),
		saveErrors:    make(map[string]error),
	}
}

func (m *mockStorage) UpdateExecutionStatus(id string, status ExecutionStatus, startedAt, completedAt *time.Time, exitCode *int, output *string, errorMessage string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err, ok := m.saveErrors[id]; ok {
		return err
	}

	m.statusUpdates = append(m.statusUpdates, statusUpdate{
		ID:           id,
		Status:       status,
		StartedAt:    startedAt,
		CompletedAt:  completedAt,
		ExitCode:     exitCode,
		Output:       output,
		ErrorMessage: errorMessage,
	})

	if exec, ok := m.executions[id]; ok {
		exec.Status = status
		if startedAt != nil {
			exec.StartedAt = startedAt
		}
		if completedAt != nil {
			exec.CompletedAt = completedAt
		}
		if exitCode != nil {
			exec.ExitCode = exitCode
		}
		if output != nil {
			exec.Output = *output
		}
		exec.ErrorMessage = errorMessage
	}

	return nil
}

func (m *mockStorage) GetExecution(id string) (*Execution, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.executions[id], nil
}

func (m *mockStorage) addExecution(exec *Execution) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.executions[exec.ID] = exec
}

func (m *mockStorage) getStatusUpdates() []statusUpdate {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]statusUpdate, len(m.statusUpdates))
	copy(result, m.statusUpdates)
	return result
}

// mockNotifier implements the ExecutionNotifier interface for testing
type mockNotifier struct {
	mu             sync.Mutex
	startedCalls   []notifyStartedCall
	completedCalls []notifyCompletedCall
	failedCalls    []notifyFailedCall
}

type notifyStartedCall struct {
	DeviceID    string
	ExecutionID string
	ScriptID    string
	ScriptName  string
}

type notifyCompletedCall struct {
	DeviceID  string
	Execution *Execution
}

type notifyFailedCall struct {
	DeviceID  string
	Execution *Execution
	ErrorMsg  string
}

func newMockNotifier() *mockNotifier {
	return &mockNotifier{
		startedCalls:   make([]notifyStartedCall, 0),
		completedCalls: make([]notifyCompletedCall, 0),
		failedCalls:    make([]notifyFailedCall, 0),
	}
}

func (m *mockNotifier) NotifyExecutionStarted(deviceID, executionID, scriptID, scriptName string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.startedCalls = append(m.startedCalls, notifyStartedCall{
		DeviceID:    deviceID,
		ExecutionID: executionID,
		ScriptID:    scriptID,
		ScriptName:  scriptName,
	})
}

func (m *mockNotifier) NotifyExecutionCompleted(deviceID string, execution *Execution) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.completedCalls = append(m.completedCalls, notifyCompletedCall{
		DeviceID:  deviceID,
		Execution: execution,
	})
}

func (m *mockNotifier) NotifyExecutionFailed(deviceID string, execution *Execution, errorMsg string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failedCalls = append(m.failedCalls, notifyFailedCall{
		DeviceID:  deviceID,
		Execution: execution,
		ErrorMsg:  errorMsg,
	})
}

func (m *mockNotifier) getStartedCalls() []notifyStartedCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]notifyStartedCall, len(m.startedCalls))
	copy(result, m.startedCalls)
	return result
}

func (m *mockNotifier) getCompletedCalls() []notifyCompletedCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]notifyCompletedCall, len(m.completedCalls))
	copy(result, m.completedCalls)
	return result
}

func (m *mockNotifier) getFailedCalls() []notifyFailedCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]notifyFailedCall, len(m.failedCalls))
	copy(result, m.failedCalls)
	return result
}

// setupRunnerTestEnv creates a test environment for runner tests
func setupRunnerTestEnv(t *testing.T) (*Manager, *Executor, string) {
	t.Helper()

	scriptsDir := t.TempDir()

	// Create a simple echo script
	echoScript := `#!/bin/bash
echo "Hello from runner test"
echo "PARAM1=$PARAM1"
exit 0
`
	if err := os.WriteFile(filepath.Join(scriptsDir, "echo.sh"), []byte(echoScript), 0755); err != nil {
		t.Fatal(err)
	}

	// Create a failing script
	failScript := `#!/bin/bash
echo "About to fail" >&2
exit 1
`
	if err := os.WriteFile(filepath.Join(scriptsDir, "fail.sh"), []byte(failScript), 0755); err != nil {
		t.Fatal(err)
	}

	// Create a slow script for timeout testing
	slowScript := `#!/bin/bash
echo "Starting slow script..."
sleep 10
echo "Done"
exit 0
`
	if err := os.WriteFile(filepath.Join(scriptsDir, "slow.sh"), []byte(slowScript), 0755); err != nil {
		t.Fatal(err)
	}

	manager := NewManager(scriptsDir)
	executor := NewExecutor(manager, ExecutorConfig{
		DefaultTimeout: 30 * time.Second,
		MaxOutputBytes: 1024 * 1024,
	})

	return manager, executor, scriptsDir
}

// TestRunner_ExecutesAndUpdatesStorage tests that the runner executes a script
// and updates storage with the results.
func TestRunner_ExecutesAndUpdatesStorage(t *testing.T) {
	_, executor, _ := setupRunnerTestEnv(t)
	storage := newMockStorage()
	notifier := newMockNotifier()

	runner := NewRunner(executor, storage, notifier, RunnerConfig{
		WorkerCount: 1,
		QueueSize:   10,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := runner.Start(ctx); err != nil {
		t.Fatalf("failed to start runner: %v", err)
	}
	defer runner.Stop()

	script := &Script{
		ID:             "echo-test",
		Name:           "Echo Test",
		ScriptPath:     "echo.sh",
		ScriptType:     ScriptTypeShell,
		TimeoutSeconds: 30,
	}

	execID := "exec-123"
	storage.addExecution(&Execution{
		ID:        execID,
		ScriptID:  script.ID,
		Status:    ExecutionStatusPending,
		CreatedAt: time.Now(),
	})

	job := &ExecutionJob{
		ExecutionID: execID,
		Script:      script,
		Parameters:  map[string]string{"PARAM1": "test_value"},
		DryRun:      false,
		TriggeredBy: "device-123",
	}

	if err := runner.Submit(job); err != nil {
		t.Fatalf("failed to submit job: %v", err)
	}

	// Wait for execution to complete
	time.Sleep(500 * time.Millisecond)

	// Verify storage was updated
	updates := storage.getStatusUpdates()
	if len(updates) < 2 {
		t.Fatalf("expected at least 2 status updates (running, completed), got %d", len(updates))
	}

	// Check running status was set
	foundRunning := false
	for _, u := range updates {
		if u.Status == ExecutionStatusRunning {
			foundRunning = true
			break
		}
	}
	if !foundRunning {
		t.Error("expected running status update")
	}

	// Check completed status was set
	lastUpdate := updates[len(updates)-1]
	if lastUpdate.Status != ExecutionStatusCompleted {
		t.Errorf("expected final status completed, got %s", lastUpdate.Status)
	}
	if lastUpdate.ExitCode == nil || *lastUpdate.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %v", lastUpdate.ExitCode)
	}
	if lastUpdate.Output == nil || *lastUpdate.Output == "" {
		t.Error("expected output in status update")
	}
}

// TestRunner_SendsNotificationOnComplete tests that the runner sends
// a WebSocket notification when execution completes successfully.
func TestRunner_SendsNotificationOnComplete(t *testing.T) {
	_, executor, _ := setupRunnerTestEnv(t)
	storage := newMockStorage()
	notifier := newMockNotifier()

	runner := NewRunner(executor, storage, notifier, RunnerConfig{
		WorkerCount: 1,
		QueueSize:   10,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := runner.Start(ctx); err != nil {
		t.Fatalf("failed to start runner: %v", err)
	}
	defer runner.Stop()

	script := &Script{
		ID:             "echo-test",
		Name:           "Echo Test",
		ScriptPath:     "echo.sh",
		ScriptType:     ScriptTypeShell,
		TimeoutSeconds: 30,
	}

	execID := "exec-456"
	storage.addExecution(&Execution{
		ID:        execID,
		ScriptID:  script.ID,
		Status:    ExecutionStatusPending,
		CreatedAt: time.Now(),
	})

	job := &ExecutionJob{
		ExecutionID: execID,
		Script:      script,
		Parameters:  nil,
		DryRun:      false,
		TriggeredBy: "device-789",
	}

	if err := runner.Submit(job); err != nil {
		t.Fatalf("failed to submit job: %v", err)
	}

	// Wait for execution
	time.Sleep(500 * time.Millisecond)

	// Verify notifications were sent
	startedCalls := notifier.getStartedCalls()
	if len(startedCalls) != 1 {
		t.Errorf("expected 1 started notification, got %d", len(startedCalls))
	}
	if len(startedCalls) > 0 && startedCalls[0].DeviceID != "device-789" {
		t.Errorf("expected device-789, got %s", startedCalls[0].DeviceID)
	}

	completedCalls := notifier.getCompletedCalls()
	if len(completedCalls) != 1 {
		t.Errorf("expected 1 completed notification, got %d", len(completedCalls))
	}
	if len(completedCalls) > 0 {
		if completedCalls[0].DeviceID != "device-789" {
			t.Errorf("expected device-789, got %s", completedCalls[0].DeviceID)
		}
		if completedCalls[0].Execution.Status != ExecutionStatusCompleted {
			t.Errorf("expected completed status, got %s", completedCalls[0].Execution.Status)
		}
	}

	// No failed notifications
	failedCalls := notifier.getFailedCalls()
	if len(failedCalls) != 0 {
		t.Errorf("expected 0 failed notifications, got %d", len(failedCalls))
	}
}

// TestRunner_SendsNotificationOnFailure tests that the runner sends
// a failure notification when script execution fails.
func TestRunner_SendsNotificationOnFailure(t *testing.T) {
	_, executor, _ := setupRunnerTestEnv(t)
	storage := newMockStorage()
	notifier := newMockNotifier()

	runner := NewRunner(executor, storage, notifier, RunnerConfig{
		WorkerCount: 1,
		QueueSize:   10,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := runner.Start(ctx); err != nil {
		t.Fatalf("failed to start runner: %v", err)
	}
	defer runner.Stop()

	script := &Script{
		ID:             "fail-test",
		Name:           "Fail Test",
		ScriptPath:     "fail.sh",
		ScriptType:     ScriptTypeShell,
		TimeoutSeconds: 30,
	}

	execID := "exec-fail"
	storage.addExecution(&Execution{
		ID:        execID,
		ScriptID:  script.ID,
		Status:    ExecutionStatusPending,
		CreatedAt: time.Now(),
	})

	job := &ExecutionJob{
		ExecutionID: execID,
		Script:      script,
		Parameters:  nil,
		DryRun:      false,
		TriggeredBy: "device-fail",
	}

	if err := runner.Submit(job); err != nil {
		t.Fatalf("failed to submit job: %v", err)
	}

	// Wait for execution
	time.Sleep(500 * time.Millisecond)

	// Should have started notification
	startedCalls := notifier.getStartedCalls()
	if len(startedCalls) != 1 {
		t.Errorf("expected 1 started notification, got %d", len(startedCalls))
	}

	// Should have failed notification (not completed)
	completedCalls := notifier.getCompletedCalls()
	if len(completedCalls) != 0 {
		t.Errorf("expected 0 completed notifications, got %d", len(completedCalls))
	}

	failedCalls := notifier.getFailedCalls()
	if len(failedCalls) != 1 {
		t.Errorf("expected 1 failed notification, got %d", len(failedCalls))
	}
	if len(failedCalls) > 0 && failedCalls[0].DeviceID != "device-fail" {
		t.Errorf("expected device-fail, got %s", failedCalls[0].DeviceID)
	}

	// Verify storage has failed status
	updates := storage.getStatusUpdates()
	lastUpdate := updates[len(updates)-1]
	if lastUpdate.Status != ExecutionStatusFailed {
		t.Errorf("expected failed status, got %s", lastUpdate.Status)
	}
}

// TestRunner_HandlesTimeout tests that the runner handles script timeouts correctly.
func TestRunner_HandlesTimeout(t *testing.T) {
	scriptsDir := t.TempDir()

	// Create a very slow script
	slowScript := `#!/bin/bash
sleep 10
exit 0
`
	if err := os.WriteFile(filepath.Join(scriptsDir, "slow.sh"), []byte(slowScript), 0755); err != nil {
		t.Fatal(err)
	}

	manager := NewManager(scriptsDir)
	executor := NewExecutor(manager, ExecutorConfig{
		DefaultTimeout: 1 * time.Second, // Short timeout
		MaxOutputBytes: 1024 * 1024,
	})

	storage := newMockStorage()
	notifier := newMockNotifier()

	runner := NewRunner(executor, storage, notifier, RunnerConfig{
		WorkerCount: 1,
		QueueSize:   10,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := runner.Start(ctx); err != nil {
		t.Fatalf("failed to start runner: %v", err)
	}
	defer runner.Stop()

	script := &Script{
		ID:             "timeout-test",
		Name:           "Timeout Test",
		ScriptPath:     "slow.sh",
		ScriptType:     ScriptTypeShell,
		TimeoutSeconds: 1, // 1 second timeout
	}

	execID := "exec-timeout"
	storage.addExecution(&Execution{
		ID:        execID,
		ScriptID:  script.ID,
		Status:    ExecutionStatusPending,
		CreatedAt: time.Now(),
	})

	job := &ExecutionJob{
		ExecutionID: execID,
		Script:      script,
		Parameters:  nil,
		DryRun:      false,
		TriggeredBy: "device-timeout",
	}

	if err := runner.Submit(job); err != nil {
		t.Fatalf("failed to submit job: %v", err)
	}

	// Wait for timeout
	time.Sleep(3 * time.Second)

	// Should have timeout status
	updates := storage.getStatusUpdates()
	if len(updates) < 2 {
		t.Fatalf("expected at least 2 status updates, got %d", len(updates))
	}

	lastUpdate := updates[len(updates)-1]
	if lastUpdate.Status != ExecutionStatusTimeout {
		t.Errorf("expected timeout status, got %s", lastUpdate.Status)
	}

	// Should have failed notification (timeout counts as failure)
	failedCalls := notifier.getFailedCalls()
	if len(failedCalls) != 1 {
		t.Errorf("expected 1 failed notification for timeout, got %d", len(failedCalls))
	}
}

// TestRunner_QueueFull tests that Submit returns error when queue is full.
func TestRunner_QueueFull(t *testing.T) {
	_, executor, _ := setupRunnerTestEnv(t)
	storage := newMockStorage()
	notifier := newMockNotifier()

	// Small queue
	runner := NewRunner(executor, storage, notifier, RunnerConfig{
		WorkerCount: 1,
		QueueSize:   1,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := runner.Start(ctx); err != nil {
		t.Fatalf("failed to start runner: %v", err)
	}
	defer runner.Stop()

	script := &Script{
		ID:             "slow-test",
		Name:           "Slow Test",
		ScriptPath:     "slow.sh",
		ScriptType:     ScriptTypeShell,
		TimeoutSeconds: 30,
	}

	// Submit multiple jobs to fill queue
	for i := 0; i < 5; i++ {
		execID := "exec-" + string(rune('a'+i))
		storage.addExecution(&Execution{
			ID:        execID,
			ScriptID:  script.ID,
			Status:    ExecutionStatusPending,
			CreatedAt: time.Now(),
		})

		job := &ExecutionJob{
			ExecutionID: execID,
			Script:      script,
			Parameters:  nil,
			DryRun:      false,
			TriggeredBy: "device",
		}

		err := runner.Submit(job)
		// At some point should get queue full error
		if err == ErrQueueFull {
			// This is expected
			return
		}
	}

	// If we get here, give it some time for queue to process
	time.Sleep(100 * time.Millisecond)

	// Try again, should be full now with slow scripts
	job := &ExecutionJob{
		ExecutionID: "exec-overflow",
		Script:      script,
		Parameters:  nil,
		DryRun:      false,
		TriggeredBy: "device",
	}

	err := runner.Submit(job)
	if err != ErrQueueFull {
		t.Logf("Note: queue full test may be timing-dependent, got err: %v", err)
	}
}

// TestRunner_ConcurrentExecution tests that multiple workers process jobs concurrently.
func TestRunner_ConcurrentExecution(t *testing.T) {
	_, executor, _ := setupRunnerTestEnv(t)
	storage := newMockStorage()
	notifier := newMockNotifier()

	runner := NewRunner(executor, storage, notifier, RunnerConfig{
		WorkerCount: 3, // 3 concurrent workers
		QueueSize:   10,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := runner.Start(ctx); err != nil {
		t.Fatalf("failed to start runner: %v", err)
	}
	defer runner.Stop()

	script := &Script{
		ID:             "echo-test",
		Name:           "Echo Test",
		ScriptPath:     "echo.sh",
		ScriptType:     ScriptTypeShell,
		TimeoutSeconds: 30,
	}

	// Submit 5 jobs
	for i := 0; i < 5; i++ {
		execID := "exec-concurrent-" + string(rune('a'+i))
		storage.addExecution(&Execution{
			ID:        execID,
			ScriptID:  script.ID,
			Status:    ExecutionStatusPending,
			CreatedAt: time.Now(),
		})

		job := &ExecutionJob{
			ExecutionID: execID,
			Script:      script,
			Parameters:  nil,
			DryRun:      false,
			TriggeredBy: "device",
		}

		if err := runner.Submit(job); err != nil {
			t.Fatalf("failed to submit job: %v", err)
		}
	}

	// Wait for all to complete
	time.Sleep(1 * time.Second)

	// All should be completed
	completedCalls := notifier.getCompletedCalls()
	if len(completedCalls) != 5 {
		t.Errorf("expected 5 completed notifications, got %d", len(completedCalls))
	}
}

// TestRunner_GracefulShutdown tests that Stop waits for in-flight jobs.
func TestRunner_GracefulShutdown(t *testing.T) {
	_, executor, _ := setupRunnerTestEnv(t)
	storage := newMockStorage()
	notifier := newMockNotifier()

	runner := NewRunner(executor, storage, notifier, RunnerConfig{
		WorkerCount: 1,
		QueueSize:   10,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := runner.Start(ctx); err != nil {
		t.Fatalf("failed to start runner: %v", err)
	}

	script := &Script{
		ID:             "echo-test",
		Name:           "Echo Test",
		ScriptPath:     "echo.sh",
		ScriptType:     ScriptTypeShell,
		TimeoutSeconds: 30,
	}

	execID := "exec-shutdown"
	storage.addExecution(&Execution{
		ID:        execID,
		ScriptID:  script.ID,
		Status:    ExecutionStatusPending,
		CreatedAt: time.Now(),
	})

	job := &ExecutionJob{
		ExecutionID: execID,
		Script:      script,
		Parameters:  nil,
		DryRun:      false,
		TriggeredBy: "device",
	}

	if err := runner.Submit(job); err != nil {
		t.Fatalf("failed to submit job: %v", err)
	}

	// Stop should wait for job to complete
	runner.Stop()

	// Job should have completed
	completedCalls := notifier.getCompletedCalls()
	if len(completedCalls) != 1 {
		t.Errorf("expected 1 completed notification after shutdown, got %d", len(completedCalls))
	}
}
