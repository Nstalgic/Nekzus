package pool

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewWorkerPool(t *testing.T) {
	tests := []struct {
		name        string
		size        int
		expectPanic bool
	}{
		{
			name:        "valid pool size",
			size:        5,
			expectPanic: false,
		},
		{
			name:        "minimum pool size",
			size:        1,
			expectPanic: false,
		},
		{
			name:        "large pool size",
			size:        100,
			expectPanic: false,
		},
		{
			name:        "zero pool size",
			size:        0,
			expectPanic: true,
		},
		{
			name:        "negative pool size",
			size:        -1,
			expectPanic: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				r := recover()
				if tt.expectPanic && r == nil {
					t.Error("Expected panic but did not panic")
				}
				if !tt.expectPanic && r != nil {
					t.Errorf("Unexpected panic: %v", r)
				}
			}()

			pool := NewWorkerPool(tt.size)
			if pool == nil && !tt.expectPanic {
				t.Error("NewWorkerPool returned nil")
			}
			if pool != nil {
				pool.Stop()
			}
		})
	}
}

func TestWorkerPool_Submit(t *testing.T) {
	t.Run("executes submitted task", func(t *testing.T) {
		pool := NewWorkerPool(2)
		defer pool.Stop()

		var executed atomic.Bool
		pool.Submit(func() {
			executed.Store(true)
		})

		// Wait for task execution
		time.Sleep(50 * time.Millisecond)

		if !executed.Load() {
			t.Error("Task was not executed")
		}
	})

	t.Run("executes multiple tasks", func(t *testing.T) {
		pool := NewWorkerPool(3)
		defer pool.Stop()

		var counter atomic.Int32
		numTasks := 10

		for i := 0; i < numTasks; i++ {
			pool.Submit(func() {
				counter.Add(1)
			})
		}

		// Wait for all tasks
		time.Sleep(100 * time.Millisecond)

		if count := counter.Load(); count != int32(numTasks) {
			t.Errorf("Expected %d tasks executed, got %d", numTasks, count)
		}
	})

	t.Run("tasks execute concurrently", func(t *testing.T) {
		pool := NewWorkerPool(5)
		defer pool.Stop()

		var wg sync.WaitGroup
		start := make(chan struct{})
		numTasks := 5

		wg.Add(numTasks)
		for i := 0; i < numTasks; i++ {
			pool.Submit(func() {
				<-start // Wait for signal
				wg.Done()
			})
		}

		// Release all tasks at once
		close(start)

		// Wait with timeout
		done := make(chan struct{})
		go func() {
			wg.Wait()
			close(done)
		}()

		select {
		case <-done:
			// Success - all tasks completed
		case <-time.After(1 * time.Second):
			t.Error("Tasks did not execute concurrently")
		}
	})
}

func TestWorkerPool_SubmitWithContext(t *testing.T) {
	t.Run("executes task when context is valid", func(t *testing.T) {
		pool := NewWorkerPool(2)
		defer pool.Stop()

		ctx := context.Background()
		var executed atomic.Bool

		err := pool.SubmitWithContext(ctx, func() {
			executed.Store(true)
		})

		if err != nil {
			t.Errorf("SubmitWithContext returned error: %v", err)
		}

		time.Sleep(50 * time.Millisecond)

		if !executed.Load() {
			t.Error("Task was not executed")
		}
	})

	t.Run("returns error when context is cancelled", func(t *testing.T) {
		pool := NewWorkerPool(2)
		defer pool.Stop()

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		err := pool.SubmitWithContext(ctx, func() {
			t.Error("Task should not execute")
		})

		if err == nil {
			t.Error("Expected error for cancelled context")
		}
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Expected context.Canceled error, got %v", err)
		}
	})

	t.Run("returns error when context times out", func(t *testing.T) {
		pool := NewWorkerPool(2)
		defer pool.Stop()

		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
		defer cancel()

		time.Sleep(10 * time.Millisecond) // Ensure timeout

		err := pool.SubmitWithContext(ctx, func() {
			t.Error("Task should not execute")
		})

		if err == nil {
			t.Error("Expected error for timed out context")
		}
	})
}

func TestWorkerPool_Stop(t *testing.T) {
	t.Run("stops accepting new tasks", func(t *testing.T) {
		pool := NewWorkerPool(2)
		pool.Stop()

		// Submitting after stop should not panic, but task won't execute
		var executed atomic.Bool
		pool.Submit(func() {
			executed.Store(true)
		})

		time.Sleep(50 * time.Millisecond)

		if executed.Load() {
			t.Error("Task should not execute after pool is stopped")
		}
	})

	t.Run("waits for pending tasks to complete", func(t *testing.T) {
		pool := NewWorkerPool(2)

		var counter atomic.Int32
		numTasks := 5

		for i := 0; i < numTasks; i++ {
			pool.Submit(func() {
				time.Sleep(20 * time.Millisecond)
				counter.Add(1)
			})
		}

		pool.Stop()

		// After stop, all pending tasks should have completed
		if count := counter.Load(); count != int32(numTasks) {
			t.Errorf("Expected %d tasks completed, got %d", numTasks, count)
		}
	})

	t.Run("is idempotent", func(t *testing.T) {
		pool := NewWorkerPool(2)

		pool.Stop()
		pool.Stop() // Should not panic
		pool.Stop() // Should not panic
	})
}

func TestWorkerPool_PanicRecovery(t *testing.T) {
	t.Run("recovers from panic in task", func(t *testing.T) {
		pool := NewWorkerPool(2)
		defer pool.Stop()

		var executed atomic.Bool

		// Submit task that panics
		pool.Submit(func() {
			panic("test panic")
		})

		// Submit normal task after panic
		pool.Submit(func() {
			executed.Store(true)
		})

		time.Sleep(50 * time.Millisecond)

		if !executed.Load() {
			t.Error("Pool should recover from panic and continue executing tasks")
		}
	})
}

func TestWorkerPool_Concurrency(t *testing.T) {
	t.Run("handles concurrent submissions", func(t *testing.T) {
		pool := NewWorkerPool(10)
		defer pool.Stop()

		var counter atomic.Int32
		numGoroutines := 20
		tasksPerGoroutine := 50

		var wg sync.WaitGroup
		wg.Add(numGoroutines)

		for g := 0; g < numGoroutines; g++ {
			go func() {
				defer wg.Done()
				for i := 0; i < tasksPerGoroutine; i++ {
					pool.Submit(func() {
						counter.Add(1)
					})
				}
			}()
		}

		wg.Wait()

		// Give time for all tasks to complete (including overflow goroutines)
		time.Sleep(100 * time.Millisecond)

		pool.Stop()

		expected := int32(numGoroutines * tasksPerGoroutine)
		if count := counter.Load(); count != expected {
			t.Errorf("Expected %d tasks executed, got %d", expected, count)
		}
	})
}

func TestWorkerPool_Size(t *testing.T) {
	t.Run("returns correct pool size", func(t *testing.T) {
		size := 7
		pool := NewWorkerPool(size)
		defer pool.Stop()

		if pool.Size() != size {
			t.Errorf("Expected pool size %d, got %d", size, pool.Size())
		}
	})
}

func BenchmarkWorkerPool_Submit(b *testing.B) {
	pool := NewWorkerPool(10)
	defer pool.Stop()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		pool.Submit(func() {
			// Simulate small amount of work
			_ = 1 + 1
		})
	}
}

func BenchmarkWorkerPool_SubmitWithContext(b *testing.B) {
	pool := NewWorkerPool(10)
	defer pool.Stop()

	ctx := context.Background()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		pool.SubmitWithContext(ctx, func() {
			_ = 1 + 1
		})
	}
}

func BenchmarkUnboundedGoroutines(b *testing.B) {
	var wg sync.WaitGroup

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = 1 + 1
		}()
	}

	wg.Wait()
}
