package scripts

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

var schedulerLog = slog.With("package", "scripts.scheduler")

// ScheduleStorage defines the storage interface needed by the scheduler.
type ScheduleStorage interface {
	ListEnabledSchedules() ([]*Schedule, error)
	UpdateScheduleLastRun(id string, lastRun, nextRun time.Time) error
	SaveExecution(exec *Execution) error
	UpdateExecutionStatus(id string, status ExecutionStatus, startedAt, completedAt *time.Time, exitCode *int, output *string, errorMessage string) error
}

// SchedulerConfig holds scheduler configuration.
type SchedulerConfig struct {
	CheckInterval time.Duration // How often to check for due schedules
}

// Scheduler manages scheduled script executions.
type Scheduler struct {
	executor *Executor
	storage  ScheduleStorage
	config   SchedulerConfig
	scripts  map[string]*Script

	mu      sync.Mutex
	running bool
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

// NewScheduler creates a new script scheduler.
func NewScheduler(executor *Executor, storage ScheduleStorage, config SchedulerConfig) *Scheduler {
	if config.CheckInterval == 0 {
		config.CheckInterval = 1 * time.Minute
	}

	return &Scheduler{
		executor: executor,
		storage:  storage,
		config:   config,
		scripts:  make(map[string]*Script),
	}
}

// SetScripts sets the available scripts map.
func (s *Scheduler) SetScripts(scripts map[string]*Script) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.scripts = scripts
}

// Start begins the scheduler.
func (s *Scheduler) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return fmt.Errorf("scheduler is already running")
	}

	s.ctx, s.cancel = context.WithCancel(ctx)
	s.running = true

	s.wg.Add(1)
	go s.run()

	schedulerLog.Info("Script scheduler started", "interval", s.config.CheckInterval)
	return nil
}

// Stop stops the scheduler.
func (s *Scheduler) Stop() error {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return fmt.Errorf("scheduler is not running")
	}
	s.mu.Unlock()

	s.cancel()
	s.wg.Wait()

	s.mu.Lock()
	s.running = false
	s.mu.Unlock()

	schedulerLog.Info("Script scheduler stopped")
	return nil
}

// run is the main scheduler loop.
func (s *Scheduler) run() {
	defer s.wg.Done()

	ticker := time.NewTicker(s.config.CheckInterval)
	defer ticker.Stop()

	// Run immediately on start
	s.checkSchedules()

	for {
		select {
		case <-ticker.C:
			s.checkSchedules()
		case <-s.ctx.Done():
			return
		}
	}
}

// checkSchedules checks for due schedules and executes them.
func (s *Scheduler) checkSchedules() {
	schedules, err := s.storage.ListEnabledSchedules()
	if err != nil {
		schedulerLog.Error("Failed to list schedules", "error", err)
		return
	}

	now := time.Now()

	for _, schedule := range schedules {
		if !schedule.Enabled {
			continue
		}

		// Check if schedule is due
		if schedule.NextRunAt != nil && schedule.NextRunAt.After(now) {
			continue // Not due yet
		}

		// Execute the scheduled script
		s.executeSchedule(schedule)
	}
}

// executeSchedule runs a scheduled script.
func (s *Scheduler) executeSchedule(schedule *Schedule) {
	s.mu.Lock()
	script, exists := s.scripts[schedule.ScriptID]
	s.mu.Unlock()

	if !exists {
		schedulerLog.Warn("Script not found for schedule", "schedule_id", schedule.ID, "script_id", schedule.ScriptID)
		return
	}

	// Create execution record
	execID := uuid.New().String()
	now := time.Now()
	execution := &Execution{
		ID:          execID,
		ScriptID:    schedule.ScriptID,
		Status:      ExecutionStatusPending,
		TriggeredBy: "scheduler:" + schedule.ID,
		Parameters:  schedule.Parameters,
		CreatedAt:   now,
	}

	if err := s.storage.SaveExecution(execution); err != nil {
		schedulerLog.Error("Failed to save execution record", "error", err)
		return
	}

	// Update execution status to running
	startTime := time.Now()
	if err := s.storage.UpdateExecutionStatus(execID, ExecutionStatusRunning, &startTime, nil, nil, nil, ""); err != nil {
		schedulerLog.Error("Failed to update execution status", "error", err)
	}

	// Execute the script
	result, err := s.executor.Execute(s.ctx, script, schedule.Parameters, false)

	// Update execution with results
	endTime := time.Now()
	var status ExecutionStatus
	var exitCode *int
	var output *string
	var errorMsg string

	if err != nil {
		status = ExecutionStatusFailed
		errorMsg = err.Error()
	} else if result.TimedOut {
		status = ExecutionStatusTimeout
		output = &result.Output
	} else if result.Cancelled {
		status = ExecutionStatusCancelled
		output = &result.Output
	} else if result.ExitCode != 0 {
		status = ExecutionStatusFailed
		exitCode = &result.ExitCode
		output = &result.Output
	} else {
		status = ExecutionStatusCompleted
		exitCode = &result.ExitCode
		output = &result.Output
	}

	if err := s.storage.UpdateExecutionStatus(execID, status, nil, &endTime, exitCode, output, errorMsg); err != nil {
		schedulerLog.Error("Failed to update execution status", "error", err)
	}

	// Calculate next run time
	cron, err := ParseCronExpression(schedule.CronExpression)
	if err != nil {
		schedulerLog.Error("Invalid cron expression", "expression", schedule.CronExpression, "error", err)
		return
	}

	nextRun := cron.NextRun(time.Now())
	if err := s.storage.UpdateScheduleLastRun(schedule.ID, now, nextRun); err != nil {
		schedulerLog.Error("Failed to update schedule last run", "error", err)
	}

	schedulerLog.Info("Scheduled execution completed",
		"schedule_id", schedule.ID,
		"script_id", schedule.ScriptID,
		"status", status,
		"next_run", nextRun,
	)
}

// CronExpression represents a parsed cron expression.
type CronExpression struct {
	Minutes    []int // 0-59
	Hours      []int // 0-23
	DaysOfMonth []int // 1-31
	Months     []int // 1-12
	DaysOfWeek []int // 0-6 (0 = Sunday)
}

// ParseCronExpression parses a cron expression string.
// Supports standard 5-field format: minute hour day-of-month month day-of-week
func ParseCronExpression(expr string) (*CronExpression, error) {
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return nil, fmt.Errorf("cron expression must have exactly 5 fields, got %d", len(fields))
	}

	cron := &CronExpression{}
	var err error

	// Parse each field
	cron.Minutes, err = parseCronField(fields[0], 0, 59)
	if err != nil {
		return nil, fmt.Errorf("invalid minute field: %w", err)
	}

	cron.Hours, err = parseCronField(fields[1], 0, 23)
	if err != nil {
		return nil, fmt.Errorf("invalid hour field: %w", err)
	}

	cron.DaysOfMonth, err = parseCronField(fields[2], 1, 31)
	if err != nil {
		return nil, fmt.Errorf("invalid day-of-month field: %w", err)
	}

	cron.Months, err = parseCronField(fields[3], 1, 12)
	if err != nil {
		return nil, fmt.Errorf("invalid month field: %w", err)
	}

	cron.DaysOfWeek, err = parseCronField(fields[4], 0, 6)
	if err != nil {
		return nil, fmt.Errorf("invalid day-of-week field: %w", err)
	}

	return cron, nil
}

// parseCronField parses a single cron field.
func parseCronField(field string, min, max int) ([]int, error) {
	// Handle wildcard
	if field == "*" {
		values := make([]int, max-min+1)
		for i := range values {
			values[i] = min + i
		}
		return values, nil
	}

	var values []int

	// Handle comma-separated values
	parts := strings.Split(field, ",")
	for _, part := range parts {
		// Handle step values (*/n or m-n/s)
		if strings.Contains(part, "/") {
			stepParts := strings.Split(part, "/")
			if len(stepParts) != 2 {
				return nil, fmt.Errorf("invalid step format: %s", part)
			}

			step, err := strconv.Atoi(stepParts[1])
			if err != nil || step <= 0 {
				return nil, fmt.Errorf("invalid step value: %s", stepParts[1])
			}

			var start, end int
			if stepParts[0] == "*" {
				start, end = min, max
			} else if strings.Contains(stepParts[0], "-") {
				rangeParts := strings.Split(stepParts[0], "-")
				start, _ = strconv.Atoi(rangeParts[0])
				end, _ = strconv.Atoi(rangeParts[1])
			} else {
				start, _ = strconv.Atoi(stepParts[0])
				end = max
			}

			for i := start; i <= end; i += step {
				if i >= min && i <= max {
					values = append(values, i)
				}
			}
			continue
		}

		// Handle ranges (m-n)
		if strings.Contains(part, "-") {
			rangeParts := strings.Split(part, "-")
			if len(rangeParts) != 2 {
				return nil, fmt.Errorf("invalid range format: %s", part)
			}

			start, err := strconv.Atoi(rangeParts[0])
			if err != nil {
				return nil, fmt.Errorf("invalid range start: %s", rangeParts[0])
			}

			end, err := strconv.Atoi(rangeParts[1])
			if err != nil {
				return nil, fmt.Errorf("invalid range end: %s", rangeParts[1])
			}

			if start < min || end > max || start > end {
				return nil, fmt.Errorf("range out of bounds: %s", part)
			}

			for i := start; i <= end; i++ {
				values = append(values, i)
			}
			continue
		}

		// Handle single value
		val, err := strconv.Atoi(part)
		if err != nil {
			return nil, fmt.Errorf("invalid value: %s", part)
		}
		if val < min || val > max {
			return nil, fmt.Errorf("value out of range: %d (must be %d-%d)", val, min, max)
		}
		values = append(values, val)
	}

	if len(values) == 0 {
		return nil, fmt.Errorf("no valid values in field: %s", field)
	}

	return values, nil
}

// NextRun calculates the next run time after the given time.
func (c *CronExpression) NextRun(after time.Time) time.Time {
	// Start from the next minute
	t := after.Truncate(time.Minute).Add(time.Minute)

	// Search for up to 4 years (to handle edge cases)
	maxIterations := 365 * 24 * 60 * 4

	for i := 0; i < maxIterations; i++ {
		// Check if this time matches the cron expression
		if c.matches(t) {
			return t
		}
		t = t.Add(time.Minute)
	}

	// Fallback (should never happen with valid cron expressions)
	return after.Add(time.Minute)
}

// matches checks if a time matches the cron expression.
func (c *CronExpression) matches(t time.Time) bool {
	if !contains(c.Minutes, t.Minute()) {
		return false
	}
	if !contains(c.Hours, t.Hour()) {
		return false
	}
	if !contains(c.DaysOfMonth, t.Day()) {
		return false
	}
	if !contains(c.Months, int(t.Month())) {
		return false
	}
	if !contains(c.DaysOfWeek, int(t.Weekday())) {
		return false
	}
	return true
}

// contains checks if a slice contains a value.
func contains(slice []int, value int) bool {
	for _, v := range slice {
		if v == value {
			return true
		}
	}
	return false
}
