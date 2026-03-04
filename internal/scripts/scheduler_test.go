package scripts

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

// Mock storage for testing
type mockScheduleStorage struct {
	schedules       []*Schedule
	executionsSaved int32
}

func (m *mockScheduleStorage) ListEnabledSchedules() ([]*Schedule, error) {
	return m.schedules, nil
}

func (m *mockScheduleStorage) UpdateScheduleLastRun(id string, lastRun, nextRun time.Time) error {
	for _, s := range m.schedules {
		if s.ID == id {
			s.LastRunAt = &lastRun
			s.NextRunAt = &nextRun
		}
	}
	return nil
}

func (m *mockScheduleStorage) GetScript(id string) (*Script, error) {
	return nil, nil
}

func (m *mockScheduleStorage) SaveExecution(exec *Execution) error {
	atomic.AddInt32(&m.executionsSaved, 1)
	return nil
}

func (m *mockScheduleStorage) UpdateExecutionStatus(id string, status ExecutionStatus, startedAt, completedAt *time.Time, exitCode *int, output *string, errorMessage string) error {
	return nil
}

func setupSchedulerTestDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	script := `#!/bin/bash
echo "Scheduled execution at $(date)"
exit 0
`
	if err := os.WriteFile(filepath.Join(dir, "scheduled.sh"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}

	return dir
}

func TestScheduler_ParseCron(t *testing.T) {
	tests := []struct {
		name       string
		expression string
		wantErr    bool
	}{
		{"every minute", "* * * * *", false},
		{"every hour", "0 * * * *", false},
		{"daily at 2am", "0 2 * * *", false},
		{"every 5 minutes", "*/5 * * * *", false},
		{"weekdays at 9am", "0 9 * * 1-5", false},
		{"invalid - too few fields", "* * *", true},
		{"invalid - too many fields", "* * * * * * *", true},
		{"invalid - bad value", "60 * * * *", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseCronExpression(tt.expression)
			if tt.wantErr && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

func TestScheduler_NextRun(t *testing.T) {
	// Test "every minute" - next run should be within 60 seconds
	cron, err := ParseCronExpression("* * * * *")
	if err != nil {
		t.Fatalf("Failed to parse cron: %v", err)
	}

	now := time.Now()
	next := cron.NextRun(now)

	// Next run should be at most 60 seconds away
	diff := next.Sub(now)
	if diff > 60*time.Second || diff < 0 {
		t.Errorf("Next run for '* * * * *' should be within 60 seconds, got %v", diff)
	}

	// Next run should be at :00 seconds
	if next.Second() != 0 {
		t.Errorf("Expected seconds to be 0, got %d", next.Second())
	}
}

func TestScheduler_NewScheduler(t *testing.T) {
	scriptsDir := setupSchedulerTestDir(t)
	manager := NewManager(scriptsDir)
	executor := NewExecutor(manager, ExecutorConfig{
		DefaultTimeout: 30 * time.Second,
		MaxOutputBytes: 1024 * 1024,
	})

	storage := &mockScheduleStorage{}

	scheduler := NewScheduler(executor, storage, SchedulerConfig{
		CheckInterval: 1 * time.Minute,
	})

	if scheduler == nil {
		t.Fatal("Expected scheduler to be created")
	}
}

func TestScheduler_StartStop(t *testing.T) {
	scriptsDir := setupSchedulerTestDir(t)
	manager := NewManager(scriptsDir)
	executor := NewExecutor(manager, ExecutorConfig{
		DefaultTimeout: 30 * time.Second,
		MaxOutputBytes: 1024 * 1024,
	})

	storage := &mockScheduleStorage{}

	scheduler := NewScheduler(executor, storage, SchedulerConfig{
		CheckInterval: 100 * time.Millisecond,
	})

	ctx := context.Background()

	// Start scheduler
	if err := scheduler.Start(ctx); err != nil {
		t.Fatalf("Failed to start scheduler: %v", err)
	}

	// Try to start again - should fail
	if err := scheduler.Start(ctx); err == nil {
		t.Error("Expected error when starting already running scheduler")
	}

	// Wait a bit
	time.Sleep(50 * time.Millisecond)

	// Stop scheduler
	if err := scheduler.Stop(); err != nil {
		t.Fatalf("Failed to stop scheduler: %v", err)
	}

	// Try to stop again - should fail
	if err := scheduler.Stop(); err == nil {
		t.Error("Expected error when stopping already stopped scheduler")
	}
}

func TestScheduler_ExecutesScheduledScript(t *testing.T) {
	scriptsDir := setupSchedulerTestDir(t)
	manager := NewManager(scriptsDir)
	executor := NewExecutor(manager, ExecutorConfig{
		DefaultTimeout: 30 * time.Second,
		MaxOutputBytes: 1024 * 1024,
	})

	// Create a schedule that should run immediately (next run in past)
	pastTime := time.Now().Add(-1 * time.Minute)
	storage := &mockScheduleStorage{
		schedules: []*Schedule{
			{
				ID:             "test-schedule",
				ScriptID:       "scheduled",
				CronExpression: "* * * * *",
				Enabled:        true,
				NextRunAt:      &pastTime,
			},
		},
	}

	// Create a scripts map for the scheduler
	scriptsMap := map[string]*Script{
		"scheduled": {
			ID:             "scheduled",
			Name:           "Scheduled Script",
			ScriptPath:     "scheduled.sh",
			ScriptType:     ScriptTypeShell,
			TimeoutSeconds: 30,
		},
	}

	scheduler := NewScheduler(executor, storage, SchedulerConfig{
		CheckInterval: 50 * time.Millisecond,
	})
	scheduler.SetScripts(scriptsMap)

	ctx := context.Background()
	if err := scheduler.Start(ctx); err != nil {
		t.Fatalf("Failed to start scheduler: %v", err)
	}

	// Wait for scheduler to run
	time.Sleep(200 * time.Millisecond)

	if err := scheduler.Stop(); err != nil {
		t.Fatalf("Failed to stop scheduler: %v", err)
	}

	// Check that execution was saved
	if atomic.LoadInt32(&storage.executionsSaved) == 0 {
		t.Error("Expected at least one execution to be saved")
	}
}

func TestScheduler_SkipsDisabledSchedules(t *testing.T) {
	scriptsDir := setupSchedulerTestDir(t)
	manager := NewManager(scriptsDir)
	executor := NewExecutor(manager, ExecutorConfig{
		DefaultTimeout: 30 * time.Second,
		MaxOutputBytes: 1024 * 1024,
	})

	pastTime := time.Now().Add(-1 * time.Minute)
	storage := &mockScheduleStorage{
		schedules: []*Schedule{
			{
				ID:             "disabled-schedule",
				ScriptID:       "scheduled",
				CronExpression: "* * * * *",
				Enabled:        false, // Disabled!
				NextRunAt:      &pastTime,
			},
		},
	}

	scheduler := NewScheduler(executor, storage, SchedulerConfig{
		CheckInterval: 50 * time.Millisecond,
	})

	ctx := context.Background()
	if err := scheduler.Start(ctx); err != nil {
		t.Fatalf("Failed to start scheduler: %v", err)
	}

	time.Sleep(150 * time.Millisecond)

	if err := scheduler.Stop(); err != nil {
		t.Fatalf("Failed to stop scheduler: %v", err)
	}

	// Disabled schedules should not be returned by ListEnabledSchedules
	// so no executions should be saved
	if atomic.LoadInt32(&storage.executionsSaved) != 0 {
		t.Error("Expected no executions for disabled schedule")
	}
}
