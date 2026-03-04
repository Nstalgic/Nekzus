package health

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/nstalgic/nekzus/internal/pool"
)

// Status represents the health status of a component
type Status string

const (
	StatusHealthy   Status = "healthy"
	StatusDegraded  Status = "degraded"
	StatusUnhealthy Status = "unhealthy"
	StatusUnknown   Status = "unknown"
)

// ComponentHealth represents the health status of a single component
type ComponentHealth struct {
	Status      Status                 `json:"status"`
	Message     string                 `json:"message,omitempty"`
	LastChecked time.Time              `json:"lastChecked"`
	Duration    time.Duration          `json:"duration,omitempty"`
	Details     map[string]interface{} `json:"details,omitempty"`
}

// SystemHealth represents the overall system health
type SystemHealth struct {
	Status     Status                     `json:"status"`
	Version    string                     `json:"version"`
	Uptime     time.Duration              `json:"uptime"`
	Timestamp  time.Time                  `json:"timestamp"`
	Components map[string]ComponentHealth `json:"components"`
}

// Checker is an interface for health check implementations
type Checker interface {
	// Check performs the health check and returns the result
	Check(ctx context.Context) ComponentHealth

	// Name returns the component name
	Name() string
}

// MetricsRecorder is an interface for recording health check metrics
type MetricsRecorder interface {
	RecordHealthCheck(component, status string, duration time.Duration)
}

// Manager manages health checks for multiple components
type Manager struct {
	checkers   map[string]Checker
	cache      map[string]ComponentHealth
	cacheTTL   time.Duration
	version    string
	startTime  time.Time
	metrics    MetricsRecorder
	workerPool *pool.WorkerPool
	mu         sync.RWMutex
}

// NewManager creates a new health check manager
func NewManager(version string, startTime time.Time, metrics MetricsRecorder) *Manager {
	return &Manager{
		checkers:   make(map[string]Checker),
		cache:      make(map[string]ComponentHealth),
		cacheTTL:   5 * time.Second, // Cache results for 5 seconds
		version:    version,
		startTime:  startTime,
		workerPool: pool.NewWorkerPool(10), // Pool of 10 workers for health checks
		metrics:    metrics,
	}
}

// Stop gracefully shuts down the health manager
func (m *Manager) Stop() {
	if m.workerPool != nil {
		m.workerPool.Stop()
	}
}

// RegisterChecker registers a health checker
func (m *Manager) RegisterChecker(checker Checker) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.checkers[checker.Name()] = checker
}

// Check performs health checks on all registered components
func (m *Manager) Check(ctx context.Context) SystemHealth {
	m.mu.RLock()
	checkers := make(map[string]Checker, len(m.checkers))
	for name, checker := range m.checkers {
		checkers[name] = checker
	}
	m.mu.RUnlock()

	// Run checks concurrently
	results := make(chan struct {
		name   string
		health ComponentHealth
	}, len(checkers))

	checkCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	for name, checker := range checkers {
		wg.Add(1)
		// Use worker pool instead of unbounded goroutines
		m.workerPool.Submit(func(n string, c Checker) func() {
			return func() {
				defer wg.Done()
				health := m.checkWithCache(checkCtx, n, c)
				results <- struct {
					name   string
					health ComponentHealth
				}{n, health}
			}
		}(name, checker))
	}

	// Wait for all checks to complete
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results
	components := make(map[string]ComponentHealth)
	for result := range results {
		components[result.name] = result.health
	}

	// Determine overall status
	overallStatus := m.calculateOverallStatus(components)

	return SystemHealth{
		Status:     overallStatus,
		Version:    m.version,
		Uptime:     time.Since(m.startTime),
		Timestamp:  time.Now(),
		Components: components,
	}
}

// checkWithCache checks a component with caching
func (m *Manager) checkWithCache(ctx context.Context, name string, checker Checker) ComponentHealth {
	m.mu.RLock()
	cached, exists := m.cache[name]
	m.mu.RUnlock()

	// Return cached result only if healthy and still valid
	// Always refresh unhealthy status to detect recovery quickly
	if exists && time.Since(cached.LastChecked) < m.cacheTTL && cached.Status == StatusHealthy {
		return cached
	}

	// Perform actual check
	start := time.Now()
	health := checker.Check(ctx)
	health.Duration = time.Since(start)
	health.LastChecked = time.Now()

	// Record metrics
	if m.metrics != nil {
		m.metrics.RecordHealthCheck(name, string(health.Status), health.Duration)
	}

	// Update cache
	m.mu.Lock()
	m.cache[name] = health
	m.mu.Unlock()

	return health
}

// calculateOverallStatus determines the overall system status based on component statuses
func (m *Manager) calculateOverallStatus(components map[string]ComponentHealth) Status {
	if len(components) == 0 {
		return StatusUnknown
	}

	hasUnhealthy := false
	hasDegraded := false

	for _, comp := range components {
		switch comp.Status {
		case StatusUnhealthy:
			hasUnhealthy = true
		case StatusDegraded:
			hasDegraded = true
		}
	}

	if hasUnhealthy {
		return StatusUnhealthy
	}
	if hasDegraded {
		return StatusDegraded
	}
	return StatusHealthy
}

// IsHealthy returns true if the system is healthy (not unhealthy)
func (m *Manager) IsHealthy(ctx context.Context) bool {
	health := m.Check(ctx)
	return health.Status != StatusUnhealthy
}

// IsReady returns true if the system is ready to serve requests
// (all critical components are healthy or degraded)
func (m *Manager) IsReady(ctx context.Context) bool {
	return m.IsHealthy(ctx)
}

// SimpleLivenessCheck returns true if the application is running
// This is a lightweight check that doesn't query components
func (m *Manager) SimpleLivenessCheck() bool {
	return true // If we can execute this, we're alive
}

// GetComponentHealth returns the health of a specific component
func (m *Manager) GetComponentHealth(ctx context.Context, name string) (ComponentHealth, error) {
	m.mu.RLock()
	checker, exists := m.checkers[name]
	m.mu.RUnlock()

	if !exists {
		return ComponentHealth{}, fmt.Errorf("component %s not found", name)
	}

	return m.checkWithCache(ctx, name, checker), nil
}

// ClearCache clears the health check cache
func (m *Manager) ClearCache() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cache = make(map[string]ComponentHealth)
}

// SetCacheTTL sets the cache time-to-live duration
func (m *Manager) SetCacheTTL(ttl time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cacheTTL = ttl
}

// ListComponents returns a list of registered component names
func (m *Manager) ListComponents() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	names := make([]string, 0, len(m.checkers))
	for name := range m.checkers {
		names = append(names, name)
	}
	return names
}
