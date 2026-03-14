package scripts

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"
)

var runnerLog = slog.With("package", "scripts", "component", "runner")

// ErrQueueFull is returned when the execution queue is full.
var ErrQueueFull = errors.New("execution queue is full")

// ErrRunnerNotStarted is returned when submitting to a stopped runner.
var ErrRunnerNotStarted = errors.New("runner not started")

// ExecutionJob represents a script execution request.
type ExecutionJob struct {
	ExecutionID string
	Script      *Script
	Parameters  map[string]string
	DryRun      bool
	TriggeredBy string // Device ID for targeted notification
}

// RunnerConfig holds configuration for the Runner.
type RunnerConfig struct {
	WorkerCount int // Number of concurrent workers
	QueueSize   int // Maximum pending jobs in queue
}

// Storage interface for Runner (subset of storage.Store).
type Storage interface {
	UpdateExecutionStatus(id string, status ExecutionStatus, startedAt, completedAt *time.Time, exitCode *int, output *string, errorMessage string) error
	GetExecution(id string) (*Execution, error)
}

// ExecutionNotifier interface for sending execution notifications.
type ExecutionNotifier interface {
	NotifyExecutionStarted(deviceID, executionID, scriptID, scriptName string)
	NotifyExecutionCompleted(deviceID string, execution *Execution)
	NotifyExecutionFailed(deviceID string, execution *Execution, errorMsg string)
}

// Runner manages async script execution with a worker pool.
type Runner struct {
	executor ScriptExecutor
	storage  Storage
	notifier ExecutionNotifier
	config   RunnerConfig
	queue    chan *ExecutionJob
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	started  bool
	mu       sync.RWMutex
}

// NewRunner creates a new script runner.
func NewRunner(executor ScriptExecutor, storage Storage, notifier ExecutionNotifier, config RunnerConfig) *Runner {
	if config.WorkerCount <= 0 {
		config.WorkerCount = 3
	}
	if config.QueueSize <= 0 {
		config.QueueSize = 100
	}

	return &Runner{
		executor: executor,
		storage:  storage,
		notifier: notifier,
		config:   config,
		queue:    make(chan *ExecutionJob, config.QueueSize),
	}
}

// Start starts the runner workers.
func (r *Runner) Start(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.started {
		return errors.New("runner already started")
	}

	r.ctx, r.cancel = context.WithCancel(ctx)
	r.started = true

	// Start workers
	for i := 0; i < r.config.WorkerCount; i++ {
		r.wg.Add(1)
		go r.worker(i)
	}

	runnerLog.Info("runner started",
		"workers", r.config.WorkerCount,
		"queue_size", r.config.QueueSize)

	return nil
}

// Stop gracefully stops the runner, waiting for in-flight jobs.
func (r *Runner) Stop() {
	r.mu.Lock()
	if !r.started {
		r.mu.Unlock()
		return
	}
	r.started = false
	r.mu.Unlock()

	// Close queue to stop accepting new jobs
	close(r.queue)

	// Wait for workers to finish current jobs
	r.wg.Wait()

	// Cancel context after all workers are done
	if r.cancel != nil {
		r.cancel()
	}

	runnerLog.Info("runner stopped")
}

// Submit adds a job to the execution queue.
func (r *Runner) Submit(job *ExecutionJob) error {
	r.mu.RLock()
	started := r.started
	r.mu.RUnlock()

	if !started {
		return ErrRunnerNotStarted
	}

	select {
	case r.queue <- job:
		runnerLog.Info("job submitted",
			"execution_id", job.ExecutionID,
			"script_id", job.Script.ID,
			"triggered_by", job.TriggeredBy)
		return nil
	default:
		runnerLog.Warn("queue full, rejecting job",
			"execution_id", job.ExecutionID,
			"script_id", job.Script.ID)
		return ErrQueueFull
	}
}

// worker processes jobs from the queue.
func (r *Runner) worker(id int) {
	defer r.wg.Done()

	runnerLog.Debug("worker started", "worker_id", id)

	for job := range r.queue {
		r.executeJob(job)
	}

	runnerLog.Debug("worker stopped", "worker_id", id)
}

// executeJob executes a single job and updates storage/notifications.
func (r *Runner) executeJob(job *ExecutionJob) {
	startTime := time.Now()

	runnerLog.Info("executing job",
		"execution_id", job.ExecutionID,
		"script_id", job.Script.ID,
		"script_name", job.Script.Name)

	// Update status to running
	if err := r.storage.UpdateExecutionStatus(
		job.ExecutionID,
		ExecutionStatusRunning,
		&startTime,
		nil, nil, nil, "",
	); err != nil {
		runnerLog.Error("failed to update status to running",
			"execution_id", job.ExecutionID,
			"error", err)
	}

	// Notify execution started
	if r.notifier != nil && job.TriggeredBy != "" {
		r.notifier.NotifyExecutionStarted(
			job.TriggeredBy,
			job.ExecutionID,
			job.Script.ID,
			job.Script.Name,
		)
	}

	// Execute the script
	result, err := r.executor.Execute(r.ctx, job.Script, job.Parameters, job.DryRun)

	endTime := time.Now()

	// Determine status and prepare update
	var status ExecutionStatus
	var exitCode *int
	var output *string
	var errorMsg string

	if err != nil {
		status = ExecutionStatusFailed
		errorMsg = err.Error()
		runnerLog.Error("script execution error",
			"execution_id", job.ExecutionID,
			"error", err)
	} else if result.TimedOut {
		status = ExecutionStatusTimeout
		output = &result.Output
		errorMsg = "script timed out"
		runnerLog.Warn("script timed out",
			"execution_id", job.ExecutionID,
			"duration", result.Duration)
	} else if result.Cancelled {
		status = ExecutionStatusCancelled
		output = &result.Output
		runnerLog.Info("script cancelled",
			"execution_id", job.ExecutionID)
	} else if result.ExitCode != 0 {
		status = ExecutionStatusFailed
		exitCode = &result.ExitCode
		output = &result.Output
		errorMsg = "script exited with non-zero code"
		runnerLog.Warn("script failed",
			"execution_id", job.ExecutionID,
			"exit_code", result.ExitCode)
	} else {
		status = ExecutionStatusCompleted
		exitCode = &result.ExitCode
		output = &result.Output
		runnerLog.Info("script completed successfully",
			"execution_id", job.ExecutionID,
			"duration", result.Duration)
	}

	// Update storage with results
	if err := r.storage.UpdateExecutionStatus(
		job.ExecutionID,
		status,
		nil,
		&endTime,
		exitCode,
		output,
		errorMsg,
	); err != nil {
		runnerLog.Error("failed to update execution status",
			"execution_id", job.ExecutionID,
			"status", status,
			"error", err)
	}

	// Send notification
	if r.notifier != nil && job.TriggeredBy != "" && job.TriggeredBy != "scheduler" {
		// Get updated execution for notification
		execution, err := r.storage.GetExecution(job.ExecutionID)
		if err != nil {
			runnerLog.Error("failed to get execution for notification",
				"execution_id", job.ExecutionID,
				"error", err)
			return
		}

		if execution != nil {
			if status == ExecutionStatusCompleted {
				r.notifier.NotifyExecutionCompleted(job.TriggeredBy, execution)
			} else {
				r.notifier.NotifyExecutionFailed(job.TriggeredBy, execution, errorMsg)
			}
		}
	}
}

// QueueLength returns the current number of pending jobs.
func (r *Runner) QueueLength() int {
	return len(r.queue)
}

// IsRunning returns whether the runner is started.
func (r *Runner) IsRunning() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.started
}

// SetNotifier sets the execution notifier for sending WebSocket updates.
// Can be called after construction to wire up dependencies.
func (r *Runner) SetNotifier(notifier ExecutionNotifier) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.notifier = notifier
}
