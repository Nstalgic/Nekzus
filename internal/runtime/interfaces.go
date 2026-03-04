package runtime

import (
	"context"
	"io"
	"time"
)

// Runtime represents a container runtime (Docker, Kubernetes, etc.)
type Runtime interface {
	// Name returns a human-readable name for this runtime
	Name() string

	// Type returns the runtime type identifier
	Type() RuntimeType

	// Ping checks if the runtime is available and responsive
	Ping(ctx context.Context) error

	// Close releases any resources held by the runtime
	Close() error

	// ContainerManager provides container lifecycle operations
	ContainerManager

	// LogStreamer provides log streaming capabilities
	LogStreamer

	// StatsCollector provides resource usage statistics
	StatsCollector

	// Inspector provides detailed container inspection
	Inspector
}

// ContainerManager handles container lifecycle operations
type ContainerManager interface {
	// List returns containers matching the options
	List(ctx context.Context, opts ListOptions) ([]Container, error)

	// Start starts a stopped container
	// For K8s: scales the parent deployment to 1 replica
	Start(ctx context.Context, id ContainerID) error

	// Stop stops a running container
	// For K8s: scales the parent deployment to 0 replicas
	Stop(ctx context.Context, id ContainerID, timeout *time.Duration) error

	// Restart restarts a container
	// For K8s: deletes the pod (controller recreates it)
	Restart(ctx context.Context, id ContainerID, timeout *time.Duration) error
}

// LogStreamer handles container log streaming
type LogStreamer interface {
	// StreamLogs returns a reader for container logs
	// The caller is responsible for closing the reader
	StreamLogs(ctx context.Context, id ContainerID, opts LogOptions) (io.ReadCloser, error)
}

// StatsCollector handles resource usage statistics
type StatsCollector interface {
	// GetStats returns current resource usage for a container
	GetStats(ctx context.Context, id ContainerID) (*Stats, error)

	// GetBatchStats returns stats for multiple containers efficiently
	GetBatchStats(ctx context.Context, ids []ContainerID) ([]Stats, error)
}

// Inspector provides detailed container information
type Inspector interface {
	// Inspect returns detailed information about a container
	Inspect(ctx context.Context, id ContainerID) (*ContainerDetails, error)
}

// BulkOperator provides bulk container operations
type BulkOperator interface {
	// BulkRestart restarts multiple containers concurrently
	BulkRestart(ctx context.Context, ids []ContainerID, timeout *time.Duration) ([]BulkResult, error)
}
