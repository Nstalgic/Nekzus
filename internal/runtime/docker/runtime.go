// Package docker provides a Docker implementation of the runtime.Runtime interface
package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/nstalgic/nekzus/internal/runtime"
)

// DockerClient defines the interface for Docker operations
type DockerClient interface {
	Ping(ctx context.Context) (types.Ping, error)
	ContainerList(ctx context.Context, options container.ListOptions) ([]types.Container, error)
	ContainerStart(ctx context.Context, containerID string, options container.StartOptions) error
	ContainerStop(ctx context.Context, containerID string, options container.StopOptions) error
	ContainerRestart(ctx context.Context, containerID string, options container.StopOptions) error
	ContainerInspect(ctx context.Context, containerID string) (types.ContainerJSON, error)
	ContainerStats(ctx context.Context, containerID string, stream bool) (container.StatsResponseReader, error)
	ContainerLogs(ctx context.Context, containerID string, options container.LogsOptions) (io.ReadCloser, error)
	Close() error
}

// Runtime implements runtime.Runtime for Docker
type Runtime struct {
	client DockerClient
}

// Ensure Runtime implements runtime.Runtime
var _ runtime.Runtime = (*Runtime)(nil)

// NewRuntime creates a new Docker runtime
func NewRuntime(client DockerClient) *Runtime {
	return &Runtime{client: client}
}

// Name returns the runtime name
func (r *Runtime) Name() string {
	return "Docker"
}

// Type returns the runtime type
func (r *Runtime) Type() runtime.RuntimeType {
	return runtime.RuntimeDocker
}

// Ping checks if Docker is available
func (r *Runtime) Ping(ctx context.Context) error {
	_, err := r.client.Ping(ctx)
	if err != nil {
		return runtime.NewRuntimeError(runtime.RuntimeDocker, "ping", runtime.ErrRuntimeUnavailable)
	}
	return nil
}

// Close releases Docker client resources
func (r *Runtime) Close() error {
	return r.client.Close()
}

// GetClient returns the underlying Docker client
func (r *Runtime) GetClient() DockerClient {
	return r.client
}

// List returns containers matching the options
func (r *Runtime) List(ctx context.Context, opts runtime.ListOptions) ([]runtime.Container, error) {
	listOpts := container.ListOptions{
		All: opts.All,
	}

	containers, err := r.client.ContainerList(ctx, listOpts)
	if err != nil {
		return nil, runtime.NewRuntimeError(runtime.RuntimeDocker, "list", err)
	}

	result := make([]runtime.Container, 0, len(containers))
	for _, c := range containers {
		result = append(result, convertContainer(c))
	}

	return result, nil
}

// Start starts a container
func (r *Runtime) Start(ctx context.Context, id runtime.ContainerID) error {
	err := r.client.ContainerStart(ctx, id.ID, container.StartOptions{})
	if err != nil {
		if isNotFoundError(err) {
			return runtime.NewContainerError(runtime.RuntimeDocker, "start", id, runtime.ErrContainerNotFound)
		}
		return runtime.NewContainerError(runtime.RuntimeDocker, "start", id, err)
	}
	return nil
}

// Stop stops a container
func (r *Runtime) Stop(ctx context.Context, id runtime.ContainerID, timeout *time.Duration) error {
	stopOpts := container.StopOptions{}
	if timeout != nil {
		seconds := int(timeout.Seconds())
		stopOpts.Timeout = &seconds
	}

	err := r.client.ContainerStop(ctx, id.ID, stopOpts)
	if err != nil {
		if isNotFoundError(err) {
			return runtime.NewContainerError(runtime.RuntimeDocker, "stop", id, runtime.ErrContainerNotFound)
		}
		return runtime.NewContainerError(runtime.RuntimeDocker, "stop", id, err)
	}
	return nil
}

// Restart restarts a container
func (r *Runtime) Restart(ctx context.Context, id runtime.ContainerID, timeout *time.Duration) error {
	stopOpts := container.StopOptions{}
	if timeout != nil {
		seconds := int(timeout.Seconds())
		stopOpts.Timeout = &seconds
	}

	err := r.client.ContainerRestart(ctx, id.ID, stopOpts)
	if err != nil {
		if isNotFoundError(err) {
			return runtime.NewContainerError(runtime.RuntimeDocker, "restart", id, runtime.ErrContainerNotFound)
		}
		return runtime.NewContainerError(runtime.RuntimeDocker, "restart", id, err)
	}
	return nil
}

// Inspect returns detailed container information
func (r *Runtime) Inspect(ctx context.Context, id runtime.ContainerID) (*runtime.ContainerDetails, error) {
	containerJSON, err := r.client.ContainerInspect(ctx, id.ID)
	if err != nil {
		if isNotFoundError(err) {
			return nil, runtime.NewContainerError(runtime.RuntimeDocker, "inspect", id, runtime.ErrContainerNotFound)
		}
		return nil, runtime.NewContainerError(runtime.RuntimeDocker, "inspect", id, err)
	}

	return convertContainerDetails(containerJSON), nil
}

// GetStats returns container resource usage statistics
func (r *Runtime) GetStats(ctx context.Context, id runtime.ContainerID) (*runtime.Stats, error) {
	// Create context with timeout for stats collection
	statsCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// Get stats with streaming to collect two samples
	statsResponse, err := r.client.ContainerStats(statsCtx, id.ID, true)
	if err != nil {
		if isNotFoundError(err) {
			return nil, runtime.NewContainerError(runtime.RuntimeDocker, "stats", id, runtime.ErrContainerNotFound)
		}
		return nil, runtime.NewContainerError(runtime.RuntimeDocker, "stats", id, err)
	}
	defer statsResponse.Body.Close()

	// Decode two samples for accurate CPU calculation
	decoder := json.NewDecoder(statsResponse.Body)

	var prevStats statsJSON
	if err := decoder.Decode(&prevStats); err != nil {
		return nil, runtime.NewContainerError(runtime.RuntimeDocker, "stats", id, err)
	}

	var currStats statsJSON
	if err := decoder.Decode(&currStats); err != nil {
		return nil, runtime.NewContainerError(runtime.RuntimeDocker, "stats", id, err)
	}

	return calculateStats(id, prevStats, currStats), nil
}

// GetBatchStats returns stats for multiple containers
func (r *Runtime) GetBatchStats(ctx context.Context, ids []runtime.ContainerID) ([]runtime.Stats, error) {
	if len(ids) == 0 {
		return []runtime.Stats{}, nil
	}

	results := make([]runtime.Stats, 0, len(ids))
	for _, id := range ids {
		stats, err := r.GetStats(ctx, id)
		if err != nil {
			// Skip containers that fail to get stats
			continue
		}
		results = append(results, *stats)
	}

	return results, nil
}

// BulkRestart restarts multiple containers concurrently
func (r *Runtime) BulkRestart(ctx context.Context, ids []runtime.ContainerID, timeout *time.Duration) ([]runtime.BulkResult, error) {
	if len(ids) == 0 {
		return []runtime.BulkResult{}, nil
	}

	// Use a semaphore to limit concurrency
	const maxConcurrency = 10
	sem := make(chan struct{}, maxConcurrency)

	results := make([]runtime.BulkResult, len(ids))
	var wg sync.WaitGroup

	for i, id := range ids {
		wg.Add(1)
		go func(idx int, containerID runtime.ContainerID) {
			defer wg.Done()

			// Acquire semaphore
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				results[idx] = runtime.BulkResult{
					ContainerID: containerID,
					Success:     false,
					Error:       ctx.Err().Error(),
				}
				return
			}

			// Perform restart
			err := r.Restart(ctx, containerID, timeout)
			if err != nil {
				results[idx] = runtime.BulkResult{
					ContainerID: containerID,
					Success:     false,
					Error:       err.Error(),
				}
			} else {
				results[idx] = runtime.BulkResult{
					ContainerID: containerID,
					Success:     true,
				}
			}
		}(i, id)
	}

	wg.Wait()
	return results, nil
}

// StreamLogs returns a reader for container logs
func (r *Runtime) StreamLogs(ctx context.Context, id runtime.ContainerID, opts runtime.LogOptions) (io.ReadCloser, error) {
	logOpts := container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     opts.Follow,
		Timestamps: opts.Timestamps,
	}

	if opts.Tail > 0 {
		logOpts.Tail = fmt.Sprintf("%d", opts.Tail)
	}

	if !opts.Since.IsZero() {
		logOpts.Since = opts.Since.Format(time.RFC3339)
	}

	reader, err := r.client.ContainerLogs(ctx, id.ID, logOpts)
	if err != nil {
		if isNotFoundError(err) {
			return nil, runtime.NewContainerError(runtime.RuntimeDocker, "logs", id, runtime.ErrContainerNotFound)
		}
		return nil, runtime.NewContainerError(runtime.RuntimeDocker, "logs", id, err)
	}

	return reader, nil
}

// statsJSON represents Docker stats response
type statsJSON struct {
	CPUStats struct {
		CPUUsage struct {
			TotalUsage  uint64   `json:"total_usage"`
			PercpuUsage []uint64 `json:"percpu_usage"`
		} `json:"cpu_usage"`
		SystemUsage uint64 `json:"system_cpu_usage"`
	} `json:"cpu_stats"`
	PreCPUStats struct {
		CPUUsage struct {
			TotalUsage  uint64   `json:"total_usage"`
			PercpuUsage []uint64 `json:"percpu_usage"`
		} `json:"cpu_usage"`
		SystemUsage uint64 `json:"system_cpu_usage"`
	} `json:"precpu_stats"`
	MemoryStats struct {
		Usage uint64 `json:"usage"`
		Limit uint64 `json:"limit"`
	} `json:"memory_stats"`
	Networks map[string]struct {
		RxBytes uint64 `json:"rx_bytes"`
		TxBytes uint64 `json:"tx_bytes"`
	} `json:"networks"`
}

// calculateStats computes stats from two Docker stats samples
func calculateStats(id runtime.ContainerID, prev, curr statsJSON) *runtime.Stats {
	// Calculate CPU percentage
	cpuDelta := float64(curr.CPUStats.CPUUsage.TotalUsage - prev.CPUStats.CPUUsage.TotalUsage)
	systemDelta := float64(curr.CPUStats.SystemUsage - prev.CPUStats.SystemUsage)
	numCPUs := len(curr.CPUStats.CPUUsage.PercpuUsage)
	if numCPUs == 0 {
		numCPUs = 1
	}

	cpuPercent := 0.0
	coresUsed := 0.0
	if systemDelta > 0.0 && cpuDelta > 0.0 {
		cpuPercent = (cpuDelta / systemDelta) * float64(numCPUs) * 100.0
		coresUsed = (cpuDelta / systemDelta) * float64(numCPUs)
	}

	// Calculate memory
	memoryPercent := 0.0
	memoryAvailable := uint64(0)
	if curr.MemoryStats.Limit > 0 {
		memoryPercent = float64(curr.MemoryStats.Usage) / float64(curr.MemoryStats.Limit) * 100.0
		if curr.MemoryStats.Limit > curr.MemoryStats.Usage {
			memoryAvailable = curr.MemoryStats.Limit - curr.MemoryStats.Usage
		}
	}

	// Calculate network totals
	var networkRx, networkTx uint64
	for _, net := range curr.Networks {
		networkRx += net.RxBytes
		networkTx += net.TxBytes
	}

	return &runtime.Stats{
		ContainerID: id,
		CPU: runtime.CPUStats{
			Usage:      cpuPercent,
			CoresUsed:  coresUsed,
			TotalCores: float64(numCPUs),
		},
		Memory: runtime.MemoryStats{
			Usage:     memoryPercent,
			Used:      curr.MemoryStats.Usage,
			Limit:     curr.MemoryStats.Limit,
			Available: memoryAvailable,
		},
		Network: runtime.NetworkStats{
			RxBytes: networkRx,
			TxBytes: networkTx,
		},
		Timestamp: time.Now().Unix(),
	}
}

// convertContainer converts a Docker container to runtime.Container
func convertContainer(c types.Container) runtime.Container {
	// Get container name (remove leading slash)
	name := "unknown"
	if len(c.Names) > 0 {
		name = strings.TrimPrefix(c.Names[0], "/")
	}

	// Convert ports
	ports := make([]runtime.PortBinding, 0, len(c.Ports))
	for _, p := range c.Ports {
		ports = append(ports, runtime.PortBinding{
			IP:          p.IP,
			PrivatePort: int(p.PrivatePort),
			PublicPort:  int(p.PublicPort),
			Protocol:    p.Type,
		})
	}

	return runtime.Container{
		ID: runtime.ContainerID{
			Runtime: runtime.RuntimeDocker,
			ID:      c.ID,
		},
		Name:    name,
		Image:   c.Image,
		State:   convertDockerState(c.State),
		Status:  c.Status,
		Created: c.Created,
		Ports:   ports,
		Labels:  c.Labels,
	}
}

// convertContainerDetails converts Docker ContainerJSON to runtime.ContainerDetails
func convertContainerDetails(c types.ContainerJSON) *runtime.ContainerDetails {
	details := &runtime.ContainerDetails{
		Container: runtime.Container{
			ID: runtime.ContainerID{
				Runtime: runtime.RuntimeDocker,
				ID:      c.ID,
			},
			Name:   strings.TrimPrefix(c.Name, "/"),
			Labels: make(map[string]string),
		},
		Raw: c,
	}

	if c.Config != nil {
		details.Image = c.Config.Image
		details.Config = runtime.ContainerConfig{
			Env:        c.Config.Env,
			Cmd:        c.Config.Cmd,
			WorkingDir: c.Config.WorkingDir,
			User:       c.Config.User,
		}
		details.Labels = c.Config.Labels
	}

	if c.State != nil {
		details.State = convertDockerStateFromStatus(c.State.Status)
		details.Status = c.State.Status
	}

	if c.NetworkSettings != nil {
		details.NetworkSettings = &runtime.NetworkSettings{
			IPAddress: c.NetworkSettings.IPAddress,
			Gateway:   c.NetworkSettings.Gateway,
			Networks:  make(map[string]runtime.NetworkInfo),
		}
		for name, net := range c.NetworkSettings.Networks {
			details.NetworkSettings.Networks[name] = runtime.NetworkInfo{
				IPAddress: net.IPAddress,
				Gateway:   net.Gateway,
				NetworkID: net.NetworkID,
			}
		}
	}

	// Convert mounts
	for _, m := range c.Mounts {
		details.Mounts = append(details.Mounts, runtime.Mount{
			Type:        string(m.Type),
			Source:      m.Source,
			Destination: m.Destination,
			ReadOnly:    !m.RW,
		})
	}

	return details
}

// convertDockerState converts Docker container state string to runtime.ContainerState
func convertDockerState(state string) runtime.ContainerState {
	switch strings.ToLower(state) {
	case "running":
		return runtime.StateRunning
	case "created":
		return runtime.StateCreated
	case "paused":
		return runtime.StatePaused
	case "restarting":
		return runtime.StateRestarting
	case "exited":
		return runtime.StateExited
	case "dead", "removing":
		return runtime.StateStopped
	default:
		return runtime.StateStopped
	}
}

// convertDockerStateFromStatus converts Docker container status to runtime.ContainerState
func convertDockerStateFromStatus(status string) runtime.ContainerState {
	return convertDockerState(status)
}

// isNotFoundError checks if an error indicates a container was not found
func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	errMsg := strings.ToLower(err.Error())
	return strings.Contains(errMsg, "no such container") ||
		strings.Contains(errMsg, "not found")
}
