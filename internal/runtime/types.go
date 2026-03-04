// Package runtime provides container runtime abstraction for Docker and Kubernetes
package runtime

import (
	"time"
)

// RuntimeType identifies the container runtime
type RuntimeType string

const (
	// RuntimeDocker represents Docker container runtime
	RuntimeDocker RuntimeType = "docker"
	// RuntimeKubernetes represents Kubernetes container runtime
	RuntimeKubernetes RuntimeType = "kubernetes"
)

// ContainerID uniquely identifies a container across runtimes
type ContainerID struct {
	// Runtime identifies which runtime manages this container
	Runtime RuntimeType `json:"runtime"`
	// ID is the container/pod identifier (Docker: container ID, K8s: pod name)
	ID string `json:"id"`
	// Namespace is the K8s namespace (empty for Docker)
	Namespace string `json:"namespace,omitempty"`
}

// String returns a string representation of the container ID
func (c ContainerID) String() string {
	if c.Namespace != "" {
		return string(c.Runtime) + "/" + c.Namespace + "/" + c.ID
	}
	return string(c.Runtime) + "/" + c.ID
}

// ShortID returns a shortened version of the ID for display
func (c ContainerID) ShortID() string {
	if len(c.ID) > 12 {
		return c.ID[:12]
	}
	return c.ID
}

// ContainerState represents the state of a container
type ContainerState string

const (
	// StateRunning indicates the container is running
	StateRunning ContainerState = "running"
	// StateStopped indicates the container is stopped
	StateStopped ContainerState = "stopped"
	// StatePaused indicates the container is paused (Docker)
	StatePaused ContainerState = "paused"
	// StateRestarting indicates the container is restarting
	StateRestarting ContainerState = "restarting"
	// StatePending indicates the container is pending (K8s)
	StatePending ContainerState = "pending"
	// StateFailed indicates the container has failed (K8s)
	StateFailed ContainerState = "failed"
	// StateCreated indicates the container is created but not started
	StateCreated ContainerState = "created"
	// StateExited indicates the container has exited
	StateExited ContainerState = "exited"
)

// Container represents a container across different runtimes
type Container struct {
	// ID uniquely identifies this container
	ID ContainerID `json:"id"`
	// Name is the human-readable container name
	Name string `json:"name"`
	// Image is the container image
	Image string `json:"image"`
	// State is the current container state
	State ContainerState `json:"state"`
	// Status is a human-readable status string
	Status string `json:"status"`
	// Created is the creation timestamp (Unix)
	Created int64 `json:"created"`
	// Ports are the port bindings
	Ports []PortBinding `json:"ports"`
	// Labels are the container labels
	Labels map[string]string `json:"labels"`
}

// PortBinding represents a port mapping
type PortBinding struct {
	// IP is the host IP (may be empty)
	IP string `json:"ip,omitempty"`
	// PrivatePort is the container port
	PrivatePort int `json:"privatePort"`
	// PublicPort is the host port (0 if not mapped)
	PublicPort int `json:"publicPort,omitempty"`
	// Protocol is the port protocol (tcp, udp)
	Protocol string `json:"protocol"`
}

// Stats represents container resource usage statistics
type Stats struct {
	// ContainerID identifies the container
	ContainerID ContainerID `json:"containerId"`
	// CPU contains CPU usage statistics
	CPU CPUStats `json:"cpu"`
	// Memory contains memory usage statistics
	Memory MemoryStats `json:"memory"`
	// Network contains network I/O statistics
	Network NetworkStats `json:"network"`
	// Timestamp is when these stats were collected (Unix)
	Timestamp int64 `json:"timestamp"`
}

// CPUStats contains CPU usage information
type CPUStats struct {
	// Usage is the CPU usage percentage (0-100 per core, can exceed 100 on multi-core)
	Usage float64 `json:"usage"`
	// CoresUsed is the number of CPU cores being used
	CoresUsed float64 `json:"coresUsed"`
	// TotalCores is the total number of CPU cores available
	TotalCores float64 `json:"totalCores"`
}

// MemoryStats contains memory usage information
type MemoryStats struct {
	// Usage is the memory usage percentage (0-100)
	Usage float64 `json:"usage"`
	// Used is the bytes of memory used
	Used uint64 `json:"used"`
	// Limit is the memory limit in bytes
	Limit uint64 `json:"limit"`
	// Available is the available memory in bytes
	Available uint64 `json:"available"`
}

// NetworkStats contains network I/O information
type NetworkStats struct {
	// RxBytes is the total bytes received
	RxBytes uint64 `json:"rx"`
	// TxBytes is the total bytes transmitted
	TxBytes uint64 `json:"tx"`
}

// ContainerDetails contains detailed container information
type ContainerDetails struct {
	// Container is the basic container information
	Container
	// Config contains container configuration
	Config ContainerConfig `json:"config,omitempty"`
	// NetworkSettings contains network information
	NetworkSettings *NetworkSettings `json:"networkSettings,omitempty"`
	// Mounts contains volume mount information
	Mounts []Mount `json:"mounts,omitempty"`
	// Raw contains the raw runtime-specific response (Docker ContainerJSON, K8s Pod)
	Raw interface{} `json:"raw,omitempty"`
}

// ContainerConfig contains container configuration
type ContainerConfig struct {
	// Env contains environment variables
	Env []string `json:"env,omitempty"`
	// Cmd contains the command
	Cmd []string `json:"cmd,omitempty"`
	// WorkingDir is the working directory
	WorkingDir string `json:"workingDir,omitempty"`
	// User is the user to run as
	User string `json:"user,omitempty"`
}

// NetworkSettings contains container network information
type NetworkSettings struct {
	// IPAddress is the container's IP address
	IPAddress string `json:"ipAddress,omitempty"`
	// Gateway is the gateway IP
	Gateway string `json:"gateway,omitempty"`
	// Networks contains network-specific settings
	Networks map[string]NetworkInfo `json:"networks,omitempty"`
}

// NetworkInfo contains network-specific information
type NetworkInfo struct {
	// IPAddress is the IP address in this network
	IPAddress string `json:"ipAddress,omitempty"`
	// Gateway is the gateway IP for this network
	Gateway string `json:"gateway,omitempty"`
	// NetworkID is the network identifier
	NetworkID string `json:"networkId,omitempty"`
}

// Mount represents a volume mount
type Mount struct {
	// Type is the mount type (bind, volume, tmpfs)
	Type string `json:"type"`
	// Source is the source path or volume name
	Source string `json:"source"`
	// Destination is the container path
	Destination string `json:"destination"`
	// ReadOnly indicates if the mount is read-only
	ReadOnly bool `json:"readOnly"`
}

// ListOptions configures container listing
type ListOptions struct {
	// All includes stopped containers (default: only running)
	All bool
	// Filters are label filters (key=value)
	Filters map[string]string
	// Namespace is the K8s namespace (empty = all namespaces)
	Namespace string
}

// LogOptions configures log streaming
type LogOptions struct {
	// Follow streams logs in real-time
	Follow bool
	// Tail is the number of lines to return from the end
	Tail int64
	// Since returns logs since this time
	Since time.Time
	// Timestamps includes timestamps in output
	Timestamps bool
	// Container is the specific container name (K8s: for multi-container pods)
	Container string
}

// BulkResult represents the result of a bulk operation on a single container
type BulkResult struct {
	// ContainerID is the container that was operated on
	ContainerID ContainerID `json:"containerId"`
	// Success indicates if the operation succeeded
	Success bool `json:"success"`
	// Error contains the error message if failed
	Error string `json:"error,omitempty"`
}
