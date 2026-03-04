package storage

import (
	"testing"
	"time"

	"github.com/nstalgic/nekzus/internal/scripts"
)

// Test helpers for scripts tests
func setupScriptsTestDB(t *testing.T) *Store {
	t.Helper()
	dbPath := t.TempDir() + "/scripts_test.db"
	store, err := NewStore(Config{DatabasePath: dbPath})
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	return store
}

func cleanupScriptsTestDB(t *testing.T, store *Store) {
	t.Helper()
	if store != nil {
		store.Close()
	}
}

// Script CRUD Tests

func TestSaveScript(t *testing.T) {
	store := setupScriptsTestDB(t)
	defer cleanupScriptsTestDB(t, store)

	script := &scripts.Script{
		ID:             "restart-service",
		Name:           "Restart Service",
		Description:    "Restarts a Docker container by name",
		Category:       "operations",
		ScriptPath:     "deploy/restart-service.sh",
		ScriptType:     scripts.ScriptTypeShell,
		TimeoutSeconds: 60,
		Parameters: []scripts.ScriptParameter{
			{
				Name:     "CONTAINER",
				Label:    "Container Name",
				Type:     "text",
				Required: true,
			},
		},
		Environment: map[string]string{
			"DOCKER_HOST": "unix:///var/run/docker.sock",
		},
		AllowedScopes: []string{"scripts:execute"},
		DryRunCommand: "--dry-run",
		CreatedBy:     "device123",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	err := store.SaveScript(script)
	if err != nil {
		t.Fatalf("Failed to save script: %v", err)
	}

	// Retrieve and verify
	retrieved, err := store.GetScript(script.ID)
	if err != nil {
		t.Fatalf("Failed to get script: %v", err)
	}

	if retrieved.ID != script.ID {
		t.Errorf("Expected ID %s, got %s", script.ID, retrieved.ID)
	}
	if retrieved.Name != script.Name {
		t.Errorf("Expected Name %s, got %s", script.Name, retrieved.Name)
	}
	if retrieved.ScriptPath != script.ScriptPath {
		t.Errorf("Expected ScriptPath %s, got %s", script.ScriptPath, retrieved.ScriptPath)
	}
	if retrieved.ScriptType != script.ScriptType {
		t.Errorf("Expected ScriptType %s, got %s", script.ScriptType, retrieved.ScriptType)
	}
	if retrieved.TimeoutSeconds != script.TimeoutSeconds {
		t.Errorf("Expected TimeoutSeconds %d, got %d", script.TimeoutSeconds, retrieved.TimeoutSeconds)
	}
	if len(retrieved.Parameters) != 1 {
		t.Errorf("Expected 1 parameter, got %d", len(retrieved.Parameters))
	}
	if retrieved.Parameters[0].Name != "CONTAINER" {
		t.Errorf("Expected parameter name CONTAINER, got %s", retrieved.Parameters[0].Name)
	}
	if retrieved.DryRunCommand != "--dry-run" {
		t.Errorf("Expected DryRunCommand --dry-run, got %s", retrieved.DryRunCommand)
	}
}

func TestGetScript_NotFound(t *testing.T) {
	store := setupScriptsTestDB(t)
	defer cleanupScriptsTestDB(t, store)

	script, err := store.GetScript("nonexistent")
	if err != nil {
		t.Fatalf("Expected no error for nonexistent script, got: %v", err)
	}
	if script != nil {
		t.Error("Expected nil for nonexistent script")
	}
}

func TestListScripts(t *testing.T) {
	store := setupScriptsTestDB(t)
	defer cleanupScriptsTestDB(t, store)

	testScripts := []*scripts.Script{
		{
			ID:             "script-1",
			Name:           "Script One",
			Category:       "operations",
			ScriptPath:     "ops/script1.sh",
			ScriptType:     scripts.ScriptTypeShell,
			TimeoutSeconds: 60,
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		},
		{
			ID:             "script-2",
			Name:           "Script Two",
			Category:       "backup",
			ScriptPath:     "backup/script2.sh",
			ScriptType:     scripts.ScriptTypeShell,
			TimeoutSeconds: 300,
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		},
		{
			ID:             "script-3",
			Name:           "Script Three",
			Category:       "operations",
			ScriptPath:     "ops/script3",
			ScriptType:     scripts.ScriptTypeGoBinary,
			TimeoutSeconds: 120,
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		},
	}

	for _, s := range testScripts {
		if err := store.SaveScript(s); err != nil {
			t.Fatalf("Failed to save script: %v", err)
		}
	}

	retrieved, err := store.ListScripts()
	if err != nil {
		t.Fatalf("Failed to list scripts: %v", err)
	}

	if len(retrieved) != 3 {
		t.Errorf("Expected 3 scripts, got %d", len(retrieved))
	}
}

func TestListScriptsByCategory(t *testing.T) {
	store := setupScriptsTestDB(t)
	defer cleanupScriptsTestDB(t, store)

	testScripts := []*scripts.Script{
		{ID: "s1", Name: "S1", Category: "operations", ScriptPath: "s1.sh", ScriptType: scripts.ScriptTypeShell, CreatedAt: time.Now(), UpdatedAt: time.Now()},
		{ID: "s2", Name: "S2", Category: "backup", ScriptPath: "s2.sh", ScriptType: scripts.ScriptTypeShell, CreatedAt: time.Now(), UpdatedAt: time.Now()},
		{ID: "s3", Name: "S3", Category: "operations", ScriptPath: "s3.sh", ScriptType: scripts.ScriptTypeShell, CreatedAt: time.Now(), UpdatedAt: time.Now()},
	}

	for _, s := range testScripts {
		if err := store.SaveScript(s); err != nil {
			t.Fatalf("Failed to save script: %v", err)
		}
	}

	ops, err := store.ListScriptsByCategory("operations")
	if err != nil {
		t.Fatalf("Failed to list scripts by category: %v", err)
	}

	if len(ops) != 2 {
		t.Errorf("Expected 2 operations scripts, got %d", len(ops))
	}
}

func TestUpdateScript(t *testing.T) {
	store := setupScriptsTestDB(t)
	defer cleanupScriptsTestDB(t, store)

	script := &scripts.Script{
		ID:             "update-test",
		Name:           "Original Name",
		Category:       "general",
		ScriptPath:     "test.sh",
		ScriptType:     scripts.ScriptTypeShell,
		TimeoutSeconds: 60,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	if err := store.SaveScript(script); err != nil {
		t.Fatalf("Failed to save script: %v", err)
	}

	// Update the script
	script.Name = "Updated Name"
	script.TimeoutSeconds = 120
	script.Description = "Added description"

	if err := store.SaveScript(script); err != nil {
		t.Fatalf("Failed to update script: %v", err)
	}

	retrieved, err := store.GetScript(script.ID)
	if err != nil {
		t.Fatalf("Failed to get script: %v", err)
	}

	if retrieved.Name != "Updated Name" {
		t.Errorf("Expected name 'Updated Name', got '%s'", retrieved.Name)
	}
	if retrieved.TimeoutSeconds != 120 {
		t.Errorf("Expected timeout 120, got %d", retrieved.TimeoutSeconds)
	}
	if retrieved.Description != "Added description" {
		t.Errorf("Expected description 'Added description', got '%s'", retrieved.Description)
	}
}

func TestDeleteScript(t *testing.T) {
	store := setupScriptsTestDB(t)
	defer cleanupScriptsTestDB(t, store)

	script := &scripts.Script{
		ID:         "delete-test",
		Name:       "Delete Test",
		Category:   "general",
		ScriptPath: "test.sh",
		ScriptType: scripts.ScriptTypeShell,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	if err := store.SaveScript(script); err != nil {
		t.Fatalf("Failed to save script: %v", err)
	}

	if err := store.DeleteScript(script.ID); err != nil {
		t.Fatalf("Failed to delete script: %v", err)
	}

	retrieved, err := store.GetScript(script.ID)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	if retrieved != nil {
		t.Error("Expected script to be deleted")
	}
}

// Execution CRUD Tests

func TestSaveExecution(t *testing.T) {
	store := setupScriptsTestDB(t)
	defer cleanupScriptsTestDB(t, store)

	// First save a script (foreign key)
	script := &scripts.Script{
		ID:         "test-script",
		Name:       "Test Script",
		Category:   "general",
		ScriptPath: "test.sh",
		ScriptType: scripts.ScriptTypeShell,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	if err := store.SaveScript(script); err != nil {
		t.Fatalf("Failed to save script: %v", err)
	}

	now := time.Now()
	execution := &scripts.Execution{
		ID:          "exec-123",
		ScriptID:    "test-script",
		Status:      scripts.ExecutionStatusPending,
		IsDryRun:    false,
		TriggeredBy: "device123",
		TriggeredIP: "192.168.1.100",
		Parameters: map[string]string{
			"CONTAINER": "nginx",
		},
		CreatedAt: now,
	}

	err := store.SaveExecution(execution)
	if err != nil {
		t.Fatalf("Failed to save execution: %v", err)
	}

	retrieved, err := store.GetExecution(execution.ID)
	if err != nil {
		t.Fatalf("Failed to get execution: %v", err)
	}

	if retrieved.ID != execution.ID {
		t.Errorf("Expected ID %s, got %s", execution.ID, retrieved.ID)
	}
	if retrieved.ScriptID != execution.ScriptID {
		t.Errorf("Expected ScriptID %s, got %s", execution.ScriptID, retrieved.ScriptID)
	}
	if retrieved.Status != scripts.ExecutionStatusPending {
		t.Errorf("Expected status pending, got %s", retrieved.Status)
	}
	if retrieved.TriggeredBy != "device123" {
		t.Errorf("Expected triggeredBy device123, got %s", retrieved.TriggeredBy)
	}
}

func TestUpdateExecutionStatus(t *testing.T) {
	store := setupScriptsTestDB(t)
	defer cleanupScriptsTestDB(t, store)

	// Save script and execution
	script := &scripts.Script{
		ID: "test-script", Name: "Test", Category: "general",
		ScriptPath: "test.sh", ScriptType: scripts.ScriptTypeShell,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := store.SaveScript(script); err != nil {
		t.Fatalf("Failed to save script: %v", err)
	}

	execution := &scripts.Execution{
		ID:          "exec-status-test",
		ScriptID:    "test-script",
		Status:      scripts.ExecutionStatusPending,
		TriggeredBy: "device123",
		CreatedAt:   time.Now(),
	}
	if err := store.SaveExecution(execution); err != nil {
		t.Fatalf("Failed to save execution: %v", err)
	}

	// Update to running
	startTime := time.Now()
	err := store.UpdateExecutionStatus(execution.ID, scripts.ExecutionStatusRunning, &startTime, nil, nil, nil, "")
	if err != nil {
		t.Fatalf("Failed to update execution status: %v", err)
	}

	retrieved, err := store.GetExecution(execution.ID)
	if err != nil {
		t.Fatalf("Failed to get execution: %v", err)
	}

	if retrieved.Status != scripts.ExecutionStatusRunning {
		t.Errorf("Expected status running, got %s", retrieved.Status)
	}
	if retrieved.StartedAt == nil {
		t.Error("Expected StartedAt to be set")
	}

	// Update to completed with exit code
	endTime := time.Now()
	exitCode := 0
	output := "Script completed successfully"
	err = store.UpdateExecutionStatus(execution.ID, scripts.ExecutionStatusCompleted, nil, &endTime, &exitCode, &output, "")
	if err != nil {
		t.Fatalf("Failed to update execution status: %v", err)
	}

	retrieved, err = store.GetExecution(execution.ID)
	if err != nil {
		t.Fatalf("Failed to get execution: %v", err)
	}

	if retrieved.Status != scripts.ExecutionStatusCompleted {
		t.Errorf("Expected status completed, got %s", retrieved.Status)
	}
	if retrieved.ExitCode == nil || *retrieved.ExitCode != 0 {
		t.Error("Expected exit code 0")
	}
	if retrieved.Output != output {
		t.Errorf("Expected output '%s', got '%s'", output, retrieved.Output)
	}
}

func TestListExecutions(t *testing.T) {
	store := setupScriptsTestDB(t)
	defer cleanupScriptsTestDB(t, store)

	script := &scripts.Script{
		ID: "test-script", Name: "Test", Category: "general",
		ScriptPath: "test.sh", ScriptType: scripts.ScriptTypeShell,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := store.SaveScript(script); err != nil {
		t.Fatalf("Failed to save script: %v", err)
	}

	for i := 0; i < 5; i++ {
		exec := &scripts.Execution{
			ID:          "exec-" + string(rune('a'+i)),
			ScriptID:    "test-script",
			Status:      scripts.ExecutionStatusCompleted,
			TriggeredBy: "device123",
			CreatedAt:   time.Now().Add(time.Duration(i) * time.Second),
		}
		if err := store.SaveExecution(exec); err != nil {
			t.Fatalf("Failed to save execution: %v", err)
		}
	}

	// List all executions
	executions, err := store.ListExecutions("", "", 10, 0)
	if err != nil {
		t.Fatalf("Failed to list executions: %v", err)
	}

	if len(executions) != 5 {
		t.Errorf("Expected 5 executions, got %d", len(executions))
	}
}

func TestListExecutionsByScript(t *testing.T) {
	store := setupScriptsTestDB(t)
	defer cleanupScriptsTestDB(t, store)

	// Create two scripts
	for _, id := range []string{"script-a", "script-b"} {
		script := &scripts.Script{
			ID: id, Name: id, Category: "general",
			ScriptPath: id + ".sh", ScriptType: scripts.ScriptTypeShell,
			CreatedAt: time.Now(), UpdatedAt: time.Now(),
		}
		if err := store.SaveScript(script); err != nil {
			t.Fatalf("Failed to save script: %v", err)
		}
	}

	// Create executions for both scripts
	for i := 0; i < 3; i++ {
		exec := &scripts.Execution{
			ID: "exec-a-" + string(rune('0'+i)), ScriptID: "script-a",
			Status: scripts.ExecutionStatusCompleted, TriggeredBy: "device123",
			CreatedAt: time.Now(),
		}
		if err := store.SaveExecution(exec); err != nil {
			t.Fatalf("Failed to save execution: %v", err)
		}
	}
	for i := 0; i < 2; i++ {
		exec := &scripts.Execution{
			ID: "exec-b-" + string(rune('0'+i)), ScriptID: "script-b",
			Status: scripts.ExecutionStatusCompleted, TriggeredBy: "device123",
			CreatedAt: time.Now(),
		}
		if err := store.SaveExecution(exec); err != nil {
			t.Fatalf("Failed to save execution: %v", err)
		}
	}

	// List executions for script-a only
	executions, err := store.ListExecutions("script-a", "", 10, 0)
	if err != nil {
		t.Fatalf("Failed to list executions: %v", err)
	}

	if len(executions) != 3 {
		t.Errorf("Expected 3 executions for script-a, got %d", len(executions))
	}
}

// Workflow CRUD Tests

func TestSaveWorkflow(t *testing.T) {
	store := setupScriptsTestDB(t)
	defer cleanupScriptsTestDB(t, store)

	workflow := &scripts.Workflow{
		ID:          "deploy-pipeline",
		Name:        "Deploy Pipeline",
		Description: "Full deployment workflow",
		Steps: []scripts.WorkflowStep{
			{ScriptID: "backup-db", OnFailure: scripts.FailureActionStop},
			{ScriptID: "pull-latest", Parameters: map[string]string{"BRANCH": "main"}},
			{ScriptID: "restart-app", OnFailure: scripts.FailureActionContinue},
		},
		AllowedScopes: []string{"workflows:execute"},
		CreatedBy:     "admin",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	err := store.SaveWorkflow(workflow)
	if err != nil {
		t.Fatalf("Failed to save workflow: %v", err)
	}

	retrieved, err := store.GetWorkflow(workflow.ID)
	if err != nil {
		t.Fatalf("Failed to get workflow: %v", err)
	}

	if retrieved.ID != workflow.ID {
		t.Errorf("Expected ID %s, got %s", workflow.ID, retrieved.ID)
	}
	if retrieved.Name != workflow.Name {
		t.Errorf("Expected Name %s, got %s", workflow.Name, retrieved.Name)
	}
	if len(retrieved.Steps) != 3 {
		t.Errorf("Expected 3 steps, got %d", len(retrieved.Steps))
	}
	if retrieved.Steps[0].ScriptID != "backup-db" {
		t.Errorf("Expected first step script ID 'backup-db', got '%s'", retrieved.Steps[0].ScriptID)
	}
	if retrieved.Steps[1].Parameters["BRANCH"] != "main" {
		t.Errorf("Expected BRANCH parameter 'main', got '%s'", retrieved.Steps[1].Parameters["BRANCH"])
	}
}

func TestListWorkflows(t *testing.T) {
	store := setupScriptsTestDB(t)
	defer cleanupScriptsTestDB(t, store)

	workflows := []*scripts.Workflow{
		{ID: "wf-1", Name: "Workflow 1", Steps: []scripts.WorkflowStep{{ScriptID: "s1"}}, CreatedAt: time.Now(), UpdatedAt: time.Now()},
		{ID: "wf-2", Name: "Workflow 2", Steps: []scripts.WorkflowStep{{ScriptID: "s2"}}, CreatedAt: time.Now(), UpdatedAt: time.Now()},
	}

	for _, wf := range workflows {
		if err := store.SaveWorkflow(wf); err != nil {
			t.Fatalf("Failed to save workflow: %v", err)
		}
	}

	retrieved, err := store.ListWorkflows()
	if err != nil {
		t.Fatalf("Failed to list workflows: %v", err)
	}

	if len(retrieved) != 2 {
		t.Errorf("Expected 2 workflows, got %d", len(retrieved))
	}
}

func TestDeleteWorkflow(t *testing.T) {
	store := setupScriptsTestDB(t)
	defer cleanupScriptsTestDB(t, store)

	workflow := &scripts.Workflow{
		ID:        "delete-test",
		Name:      "Delete Test",
		Steps:     []scripts.WorkflowStep{{ScriptID: "s1"}},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := store.SaveWorkflow(workflow); err != nil {
		t.Fatalf("Failed to save workflow: %v", err)
	}

	if err := store.DeleteWorkflow(workflow.ID); err != nil {
		t.Fatalf("Failed to delete workflow: %v", err)
	}

	retrieved, err := store.GetWorkflow(workflow.ID)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	if retrieved != nil {
		t.Error("Expected workflow to be deleted")
	}
}

// Schedule CRUD Tests

func TestSaveSchedule(t *testing.T) {
	store := setupScriptsTestDB(t)
	defer cleanupScriptsTestDB(t, store)

	nextRun := time.Now().Add(1 * time.Hour)
	schedule := &scripts.Schedule{
		ID:             "schedule-1",
		ScriptID:       "backup-db",
		CronExpression: "0 2 * * *", // 2am daily
		Parameters:     map[string]string{"BACKUP_PATH": "/backups"},
		Enabled:        true,
		NextRunAt:      &nextRun,
		CreatedBy:      "admin",
		CreatedAt:      time.Now(),
	}

	err := store.SaveSchedule(schedule)
	if err != nil {
		t.Fatalf("Failed to save schedule: %v", err)
	}

	retrieved, err := store.GetSchedule(schedule.ID)
	if err != nil {
		t.Fatalf("Failed to get schedule: %v", err)
	}

	if retrieved.ID != schedule.ID {
		t.Errorf("Expected ID %s, got %s", schedule.ID, retrieved.ID)
	}
	if retrieved.ScriptID != schedule.ScriptID {
		t.Errorf("Expected ScriptID %s, got %s", schedule.ScriptID, retrieved.ScriptID)
	}
	if retrieved.CronExpression != schedule.CronExpression {
		t.Errorf("Expected CronExpression %s, got %s", schedule.CronExpression, retrieved.CronExpression)
	}
	if !retrieved.Enabled {
		t.Error("Expected schedule to be enabled")
	}
}

func TestListEnabledSchedules(t *testing.T) {
	store := setupScriptsTestDB(t)
	defer cleanupScriptsTestDB(t, store)

	schedules := []*scripts.Schedule{
		{ID: "sch-1", ScriptID: "s1", CronExpression: "* * * * *", Enabled: true, CreatedAt: time.Now()},
		{ID: "sch-2", ScriptID: "s2", CronExpression: "* * * * *", Enabled: false, CreatedAt: time.Now()},
		{ID: "sch-3", ScriptID: "s3", CronExpression: "* * * * *", Enabled: true, CreatedAt: time.Now()},
	}

	for _, sch := range schedules {
		if err := store.SaveSchedule(sch); err != nil {
			t.Fatalf("Failed to save schedule: %v", err)
		}
	}

	enabled, err := store.ListEnabledSchedules()
	if err != nil {
		t.Fatalf("Failed to list enabled schedules: %v", err)
	}

	if len(enabled) != 2 {
		t.Errorf("Expected 2 enabled schedules, got %d", len(enabled))
	}
}

func TestUpdateScheduleLastRun(t *testing.T) {
	store := setupScriptsTestDB(t)
	defer cleanupScriptsTestDB(t, store)

	schedule := &scripts.Schedule{
		ID:             "sch-update",
		ScriptID:       "s1",
		CronExpression: "* * * * *",
		Enabled:        true,
		CreatedAt:      time.Now(),
	}

	if err := store.SaveSchedule(schedule); err != nil {
		t.Fatalf("Failed to save schedule: %v", err)
	}

	lastRun := time.Now()
	nextRun := lastRun.Add(1 * time.Minute)

	err := store.UpdateScheduleLastRun(schedule.ID, lastRun, nextRun)
	if err != nil {
		t.Fatalf("Failed to update schedule last run: %v", err)
	}

	retrieved, err := store.GetSchedule(schedule.ID)
	if err != nil {
		t.Fatalf("Failed to get schedule: %v", err)
	}

	if retrieved.LastRunAt == nil {
		t.Error("Expected LastRunAt to be set")
	}
	if retrieved.NextRunAt == nil {
		t.Error("Expected NextRunAt to be set")
	}
}

func TestDeleteSchedule(t *testing.T) {
	store := setupScriptsTestDB(t)
	defer cleanupScriptsTestDB(t, store)

	schedule := &scripts.Schedule{
		ID:             "delete-test",
		ScriptID:       "s1",
		CronExpression: "* * * * *",
		Enabled:        true,
		CreatedAt:      time.Now(),
	}

	if err := store.SaveSchedule(schedule); err != nil {
		t.Fatalf("Failed to save schedule: %v", err)
	}

	if err := store.DeleteSchedule(schedule.ID); err != nil {
		t.Fatalf("Failed to delete schedule: %v", err)
	}

	retrieved, err := store.GetSchedule(schedule.ID)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	if retrieved != nil {
		t.Error("Expected schedule to be deleted")
	}
}

// Workflow Execution Tests

func TestSaveWorkflowExecution(t *testing.T) {
	store := setupScriptsTestDB(t)
	defer cleanupScriptsTestDB(t, store)

	// Save workflow first
	workflow := &scripts.Workflow{
		ID:        "test-wf",
		Name:      "Test Workflow",
		Steps:     []scripts.WorkflowStep{{ScriptID: "s1"}},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := store.SaveWorkflow(workflow); err != nil {
		t.Fatalf("Failed to save workflow: %v", err)
	}

	wfExec := &scripts.WorkflowExecution{
		ID:          "wf-exec-1",
		WorkflowID:  "test-wf",
		Status:      scripts.ExecutionStatusRunning,
		CurrentStep: 0,
		TriggeredBy: "device123",
		CreatedAt:   time.Now(),
	}

	err := store.SaveWorkflowExecution(wfExec)
	if err != nil {
		t.Fatalf("Failed to save workflow execution: %v", err)
	}

	retrieved, err := store.GetWorkflowExecution(wfExec.ID)
	if err != nil {
		t.Fatalf("Failed to get workflow execution: %v", err)
	}

	if retrieved.ID != wfExec.ID {
		t.Errorf("Expected ID %s, got %s", wfExec.ID, retrieved.ID)
	}
	if retrieved.Status != scripts.ExecutionStatusRunning {
		t.Errorf("Expected status running, got %s", retrieved.Status)
	}
}

func TestUpdateWorkflowExecutionStep(t *testing.T) {
	store := setupScriptsTestDB(t)
	defer cleanupScriptsTestDB(t, store)

	workflow := &scripts.Workflow{
		ID:        "test-wf",
		Name:      "Test",
		Steps:     []scripts.WorkflowStep{{ScriptID: "s1"}, {ScriptID: "s2"}},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := store.SaveWorkflow(workflow); err != nil {
		t.Fatalf("Failed to save workflow: %v", err)
	}

	wfExec := &scripts.WorkflowExecution{
		ID:          "wf-exec-step",
		WorkflowID:  "test-wf",
		Status:      scripts.ExecutionStatusRunning,
		CurrentStep: 0,
		TriggeredBy: "device123",
		CreatedAt:   time.Now(),
	}
	if err := store.SaveWorkflowExecution(wfExec); err != nil {
		t.Fatalf("Failed to save workflow execution: %v", err)
	}

	// Advance to step 1
	err := store.UpdateWorkflowExecutionStep(wfExec.ID, 1, scripts.ExecutionStatusRunning)
	if err != nil {
		t.Fatalf("Failed to update workflow execution step: %v", err)
	}

	retrieved, err := store.GetWorkflowExecution(wfExec.ID)
	if err != nil {
		t.Fatalf("Failed to get workflow execution: %v", err)
	}

	if retrieved.CurrentStep != 1 {
		t.Errorf("Expected current step 1, got %d", retrieved.CurrentStep)
	}
}

// Cascade Delete Tests

func TestCascadeDeleteScript_DeletesExecutions(t *testing.T) {
	store := setupScriptsTestDB(t)
	defer cleanupScriptsTestDB(t, store)

	script := &scripts.Script{
		ID: "cascade-test", Name: "Cascade Test", Category: "general",
		ScriptPath: "test.sh", ScriptType: scripts.ScriptTypeShell,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := store.SaveScript(script); err != nil {
		t.Fatalf("Failed to save script: %v", err)
	}

	execution := &scripts.Execution{
		ID: "exec-cascade", ScriptID: "cascade-test",
		Status: scripts.ExecutionStatusCompleted, TriggeredBy: "device123",
		CreatedAt: time.Now(),
	}
	if err := store.SaveExecution(execution); err != nil {
		t.Fatalf("Failed to save execution: %v", err)
	}

	// Delete script - should cascade to executions
	if err := store.DeleteScript(script.ID); err != nil {
		t.Fatalf("Failed to delete script: %v", err)
	}

	// Verify execution was deleted
	exec, err := store.GetExecution(execution.ID)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	if exec != nil {
		t.Error("Expected execution to be cascade deleted")
	}
}
