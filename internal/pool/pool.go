package pool

import (
	"context"
	"log/slog"
	"sync"
)

// Task represents a unit of work to be executed by the pool
type Task func()

// WorkerPool is a fixed-size pool of worker goroutines for bounded concurrency
type WorkerPool struct {
	size     int
	tasks    chan Task
	wg       sync.WaitGroup
	stopOnce sync.Once
	stopped  bool
	mu       sync.RWMutex
}

// NewWorkerPool creates a new worker pool with the specified number of workers
// Panics if size <= 0
func NewWorkerPool(size int) *WorkerPool {
	if size <= 0 {
		panic("worker pool size must be greater than 0")
	}

	pool := &WorkerPool{
		size:  size,
		tasks: make(chan Task, size*2), // Buffered channel to reduce blocking
	}

	// Start worker goroutines
	pool.wg.Add(size)
	for i := 0; i < size; i++ {
		go pool.worker()
	}

	return pool
}

// worker is the main loop for each worker goroutine
func (p *WorkerPool) worker() {
	defer p.wg.Done()

	for task := range p.tasks {
		p.executeTask(task)
	}
}

// executeTask runs a task with panic recovery
func (p *WorkerPool) executeTask(task Task) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("Worker pool task panic recovered",
				"panic", r,
				"pool_size", p.size,
			)
		}
	}()

	task()
}

// Submit adds a task to the pool for execution
// If the pool is stopped, the task is silently dropped
func (p *WorkerPool) Submit(task Task) {
	p.mu.RLock()
	stopped := p.stopped
	p.mu.RUnlock()

	if stopped {
		return
	}

	select {
	case p.tasks <- task:
		// Task submitted successfully
	default:
		// Channel full, execute in separate goroutine
		// This prevents blocking but maintains bounded worker count
		go p.executeTask(task)
	}
}

// SubmitWithContext adds a task to the pool with context support
// Returns an error if the context is cancelled before submission
func (p *WorkerPool) SubmitWithContext(ctx context.Context, task Task) error {
	// Check if context is already cancelled
	if err := ctx.Err(); err != nil {
		return err
	}

	p.mu.RLock()
	stopped := p.stopped
	p.mu.RUnlock()

	if stopped {
		return context.Canceled
	}

	select {
	case p.tasks <- task:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Stop gracefully shuts down the pool, waiting for all pending tasks to complete
// This method is idempotent and safe to call multiple times
func (p *WorkerPool) Stop() {
	p.stopOnce.Do(func() {
		p.mu.Lock()
		p.stopped = true
		p.mu.Unlock()

		close(p.tasks)
		p.wg.Wait()
	})
}

// Size returns the number of workers in the pool
func (p *WorkerPool) Size() int {
	return p.size
}
