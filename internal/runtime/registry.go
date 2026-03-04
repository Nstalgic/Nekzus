package runtime

import (
	"errors"
	"fmt"
	"sync"
)

var (
	// ErrRuntimeAlreadyRegistered indicates a runtime type is already registered
	ErrRuntimeAlreadyRegistered = errors.New("runtime already registered")

	// ErrNilRuntime indicates a nil runtime was provided
	ErrNilRuntime = errors.New("runtime cannot be nil")
)

// Registry manages available container runtimes
type Registry struct {
	mu       sync.RWMutex
	runtimes map[RuntimeType]Runtime
	primary  RuntimeType
}

// NewRegistry creates a new runtime registry
func NewRegistry() *Registry {
	return &Registry{
		runtimes: make(map[RuntimeType]Runtime),
	}
}

// Register adds a runtime to the registry
// The first registered runtime becomes the primary
func (r *Registry) Register(rt Runtime) error {
	if rt == nil {
		return ErrNilRuntime
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	runtimeType := rt.Type()
	if _, exists := r.runtimes[runtimeType]; exists {
		return fmt.Errorf("%w: %s", ErrRuntimeAlreadyRegistered, runtimeType)
	}

	r.runtimes[runtimeType] = rt

	// First registered runtime becomes primary
	if r.primary == "" {
		r.primary = runtimeType
	}

	return nil
}

// Get returns the runtime for the given type
func (r *Registry) Get(t RuntimeType) (Runtime, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	rt, exists := r.runtimes[t]
	if !exists {
		return nil, fmt.Errorf("%w: %s", ErrRuntimeUnavailable, t)
	}

	return rt, nil
}

// GetPrimary returns the primary runtime
// Returns nil if no runtimes are registered
func (r *Registry) GetPrimary() Runtime {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.primary == "" {
		return nil
	}

	return r.runtimes[r.primary]
}

// SetPrimary changes the primary runtime
func (r *Registry) SetPrimary(t RuntimeType) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.runtimes[t]; !exists {
		return fmt.Errorf("%w: %s", ErrRuntimeUnavailable, t)
	}

	r.primary = t
	return nil
}

// SelectForContainer returns the runtime that manages the given container
func (r *Registry) SelectForContainer(id ContainerID) (Runtime, error) {
	return r.Get(id.Runtime)
}

// Available returns the list of registered runtime types
func (r *Registry) Available() []RuntimeType {
	r.mu.RLock()
	defer r.mu.RUnlock()

	types := make([]RuntimeType, 0, len(r.runtimes))
	for t := range r.runtimes {
		types = append(types, t)
	}

	return types
}

// Close releases resources for all registered runtimes
func (r *Registry) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	var errs []error
	for _, rt := range r.runtimes {
		if err := rt.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors closing runtimes: %v", errs)
	}

	return nil
}
