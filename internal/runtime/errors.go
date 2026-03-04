package runtime

import (
	"errors"
	"fmt"
)

// Common runtime errors
var (
	// ErrContainerNotFound indicates the container was not found
	ErrContainerNotFound = errors.New("container not found")

	// ErrRuntimeUnavailable indicates the runtime is not available
	ErrRuntimeUnavailable = errors.New("runtime unavailable")

	// ErrMetricsUnavailable indicates metrics are not available (e.g., K8s Metrics Server not installed)
	ErrMetricsUnavailable = errors.New("metrics unavailable")

	// ErrOperationNotSupported indicates the operation is not supported by this runtime
	ErrOperationNotSupported = errors.New("operation not supported")

	// ErrInvalidContainerID indicates an invalid container ID was provided
	ErrInvalidContainerID = errors.New("invalid container ID")

	// ErrContainerAlreadyRunning indicates the container is already running
	ErrContainerAlreadyRunning = errors.New("container already running")

	// ErrContainerAlreadyStopped indicates the container is already stopped
	ErrContainerAlreadyStopped = errors.New("container already stopped")
)

// RuntimeError wraps an error with runtime context
type RuntimeError struct {
	// Runtime is the runtime type where the error occurred
	Runtime RuntimeType
	// Op is the operation that failed
	Op string
	// ContainerID is the container involved (if applicable)
	ContainerID *ContainerID
	// Err is the underlying error
	Err error
}

// Error returns the error message
func (e *RuntimeError) Error() string {
	if e.ContainerID != nil {
		return fmt.Sprintf("%s: %s on %s: %v", e.Runtime, e.Op, e.ContainerID.String(), e.Err)
	}
	return fmt.Sprintf("%s: %s: %v", e.Runtime, e.Op, e.Err)
}

// Unwrap returns the underlying error
func (e *RuntimeError) Unwrap() error {
	return e.Err
}

// NewRuntimeError creates a new RuntimeError
func NewRuntimeError(runtime RuntimeType, op string, err error) *RuntimeError {
	return &RuntimeError{
		Runtime: runtime,
		Op:      op,
		Err:     err,
	}
}

// NewContainerError creates a RuntimeError for a container operation
func NewContainerError(runtime RuntimeType, op string, id ContainerID, err error) *RuntimeError {
	return &RuntimeError{
		Runtime:     runtime,
		Op:          op,
		ContainerID: &id,
		Err:         err,
	}
}

// IsNotFoundError checks if the error indicates a container was not found
func IsNotFoundError(err error) bool {
	return errors.Is(err, ErrContainerNotFound)
}

// IsUnavailableError checks if the error indicates a runtime is unavailable
func IsUnavailableError(err error) bool {
	return errors.Is(err, ErrRuntimeUnavailable) || errors.Is(err, ErrMetricsUnavailable)
}
