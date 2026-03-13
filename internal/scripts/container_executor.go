package scripts

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/google/uuid"
)

// DockerClientForExecution defines the Docker client interface for script execution.
type DockerClientForExecution interface {
	ContainerCreate(ctx context.Context, config *container.Config, hostConfig *container.HostConfig, containerName string) (container.CreateResponse, error)
	ContainerStart(ctx context.Context, containerID string, options container.StartOptions) error
	ContainerWait(ctx context.Context, containerID string, condition container.WaitCondition) (<-chan container.WaitResponse, <-chan error)
	ContainerLogs(ctx context.Context, containerID string, options container.LogsOptions) (io.ReadCloser, error)
	ContainerRemove(ctx context.Context, containerID string, options container.RemoveOptions) error
	ContainerStop(ctx context.Context, containerID string, options container.StopOptions) error
	ImagePull(ctx context.Context, refStr string) (io.ReadCloser, error)
}

// ContainerExecutorConfig holds configuration for container-based script execution.
type ContainerExecutorConfig struct {
	DefaultTimeout time.Duration // Default execution timeout
	MaxOutputBytes int           // Maximum output size before truncation
	ShellImage     string        // Docker image for shell scripts (default: alpine:3.20)
	PythonImage    string        // Docker image for Python scripts (default: python:3.12-alpine)
	ScriptsMountPath string      // Mount path for scripts inside container (default: /scripts)
}

// ContainerExecutor executes scripts inside Docker containers.
type ContainerExecutor struct {
	client     DockerClientForExecution
	scriptsDir string // Host path to scripts directory
	config     ContainerExecutorConfig
	log        *slog.Logger
}

// NewContainerExecutor creates a new container-based script executor.
func NewContainerExecutor(client DockerClientForExecution, scriptsDir string, config ContainerExecutorConfig) *ContainerExecutor {
	if config.DefaultTimeout == 0 {
		config.DefaultTimeout = 5 * time.Minute
	}
	if config.MaxOutputBytes == 0 {
		config.MaxOutputBytes = 10 * 1024 * 1024 // 10MB
	}
	if config.ShellImage == "" {
		config.ShellImage = "alpine:3.20"
	}
	if config.PythonImage == "" {
		config.PythonImage = "python:3.12-alpine"
	}
	if config.ScriptsMountPath == "" {
		config.ScriptsMountPath = "/scripts"
	}

	return &ContainerExecutor{
		client:     client,
		scriptsDir: scriptsDir,
		config:     config,
		log:        slog.With("package", "scripts", "component", "container_executor"),
	}
}

// Execute runs a script inside a Docker container.
func (e *ContainerExecutor) Execute(ctx context.Context, script *Script, params map[string]string, dryRun bool) (*ExecutionResult, error) {
	// Determine timeout
	timeout := time.Duration(script.TimeoutSeconds) * time.Second
	if timeout == 0 {
		timeout = e.config.DefaultTimeout
	}

	// Create context with timeout
	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Generate unique container name
	containerName := fmt.Sprintf("nekzus-script-%s-%s", script.ID, uuid.New().String()[:8])

	// Build container configuration
	containerConfig, hostConfig := e.buildContainerConfig(script, params, dryRun)

	// Ensure the image is available (pull if needed)
	if err := e.ensureImage(execCtx, containerConfig.Image); err != nil {
		return nil, fmt.Errorf("failed to ensure image %s: %w", containerConfig.Image, err)
	}

	// Track execution time
	startTime := time.Now()

	// Create container
	e.log.Debug("creating script container",
		"script_id", script.ID,
		"container_name", containerName,
		"image", containerConfig.Image,
	)

	resp, err := e.client.ContainerCreate(execCtx, containerConfig, hostConfig, containerName)
	if err != nil {
		return nil, fmt.Errorf("failed to create container: %w", err)
	}
	containerID := resp.ID

	// Ensure cleanup
	defer func() {
		// Use background context for cleanup to ensure it completes
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()

		if err := e.client.ContainerRemove(cleanupCtx, containerID, container.RemoveOptions{Force: true}); err != nil {
			e.log.Warn("failed to remove script container",
				"container_id", containerID,
				"error", err,
			)
		}
	}()

	// Start container
	if err := e.client.ContainerStart(execCtx, containerID, container.StartOptions{}); err != nil {
		return nil, fmt.Errorf("failed to start container: %w", err)
	}

	// Wait for container to finish
	statusCh, errCh := e.client.ContainerWait(execCtx, containerID, container.WaitConditionNotRunning)

	result := &ExecutionResult{}

	select {
	case err := <-errCh:
		if err != nil {
			result.Duration = time.Since(startTime)

			// Check if it was a context error
			if execCtx.Err() == context.DeadlineExceeded {
				result.TimedOut = true
				result.ExitCode = -1

				// Stop the container
				stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer stopCancel()
				_ = e.client.ContainerStop(stopCtx, containerID, container.StopOptions{})

				// Get partial output
				result.Output = e.getContainerOutput(containerID)
				return result, nil
			}

			if execCtx.Err() == context.Canceled || ctx.Err() == context.Canceled {
				result.Cancelled = true
				result.ExitCode = -1

				// Stop the container
				stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer stopCancel()
				_ = e.client.ContainerStop(stopCtx, containerID, container.StopOptions{})

				result.Output = e.getContainerOutput(containerID)
				return result, nil
			}

			return nil, fmt.Errorf("error waiting for container: %w", err)
		}

	case status := <-statusCh:
		result.Duration = time.Since(startTime)
		result.ExitCode = int(status.StatusCode)
		result.Output = e.getContainerOutput(containerID)

	case <-execCtx.Done():
		result.Duration = time.Since(startTime)

		if execCtx.Err() == context.DeadlineExceeded {
			result.TimedOut = true
		} else {
			result.Cancelled = true
		}
		result.ExitCode = -1

		// Stop the container
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer stopCancel()
		_ = e.client.ContainerStop(stopCtx, containerID, container.StopOptions{})

		result.Output = e.getContainerOutput(containerID)
	}

	e.log.Debug("script execution completed",
		"script_id", script.ID,
		"exit_code", result.ExitCode,
		"duration", result.Duration,
		"timed_out", result.TimedOut,
		"cancelled", result.Cancelled,
	)

	return result, nil
}

// buildContainerConfig creates Docker container configuration for script execution.
func (e *ContainerExecutor) buildContainerConfig(script *Script, params map[string]string, dryRun bool) (*container.Config, *container.HostConfig) {
	// Determine image and command based on script type
	image := e.config.ShellImage
	var cmd []string
	scriptPath := fmt.Sprintf("%s/%s", e.config.ScriptsMountPath, script.ScriptPath)

	switch script.ScriptType {
	case ScriptTypeShell:
		image = e.config.ShellImage
		cmd = []string{"/bin/sh", scriptPath}
	case ScriptTypePython:
		image = e.config.PythonImage
		cmd = []string{"python3", scriptPath}
	case ScriptTypeGoBinary:
		// Go binaries execute directly
		image = e.config.ShellImage // Use alpine for libc compatibility
		cmd = []string{scriptPath}
	default:
		// Default to shell
		cmd = []string{"/bin/sh", scriptPath}
	}

	// Build environment variables
	env := e.buildEnvironment(script, params, dryRun)

	containerConfig := &container.Config{
		Image: image,
		Cmd:   cmd,
		Env:   env,
	}

	hostConfig := &container.HostConfig{
		Binds: []string{
			fmt.Sprintf("%s:%s:ro", e.scriptsDir, e.config.ScriptsMountPath),
		},
		AutoRemove:  false, // We remove manually after getting logs
		NetworkMode: "none", // Isolate script from network by default
	}

	return containerConfig, hostConfig
}

// buildEnvironment creates environment variables for script execution.
func (e *ContainerExecutor) buildEnvironment(script *Script, params map[string]string, dryRun bool) []string {
	env := make([]string, 0)

	// Add script-defined environment variables
	for k, v := range script.Environment {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}

	// Add parameters as environment variables
	for k, v := range params {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}

	// Add dry run flag
	if dryRun {
		env = append(env, "DRY_RUN=true")
	}

	// Add script metadata
	env = append(env, fmt.Sprintf("SCRIPT_ID=%s", script.ID))
	env = append(env, fmt.Sprintf("SCRIPT_NAME=%s", script.Name))

	return env
}

// getContainerOutput retrieves and optionally truncates container output.
func (e *ContainerExecutor) getContainerOutput(containerID string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	logs, err := e.client.ContainerLogs(ctx, containerID, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
	})
	if err != nil {
		e.log.Warn("failed to get container logs",
			"container_id", containerID,
			"error", err,
		)
		return ""
	}
	defer logs.Close()

	output, err := io.ReadAll(logs)
	if err != nil {
		e.log.Warn("failed to read container logs",
			"container_id", containerID,
			"error", err,
		)
		return ""
	}

	// Docker multiplexes stdout/stderr with 8-byte headers
	// Strip the headers for cleaner output
	cleaned := stripDockerLogHeaders(output)

	// Truncate if necessary
	if len(cleaned) > e.config.MaxOutputBytes {
		truncationMsg := fmt.Sprintf("\n\n[Output truncated at %d bytes]", e.config.MaxOutputBytes)
		cleaned = cleaned[:e.config.MaxOutputBytes] + truncationMsg
	}

	return cleaned
}

// stripDockerLogHeaders removes Docker log multiplexing headers from output.
// Docker prepends 8-byte headers to stdout/stderr when reading logs.
func stripDockerLogHeaders(data []byte) string {
	var result strings.Builder
	result.Grow(len(data))

	i := 0
	for i < len(data) {
		// Check if we have a valid header (8 bytes)
		if i+8 <= len(data) {
			// Header format: [type(1), 0, 0, 0, size(4)]
			// type: 1 = stdout, 2 = stderr
			headerType := data[i]
			if headerType == 1 || headerType == 2 {
				// Read size (big-endian uint32)
				size := int(data[i+4])<<24 | int(data[i+5])<<16 | int(data[i+6])<<8 | int(data[i+7])
				i += 8

				// Read the content
				if i+size <= len(data) {
					result.Write(data[i : i+size])
					i += size
					continue
				}
			}
		}

		// If we can't parse a header, just copy the remaining data
		result.Write(data[i:])
		break
	}

	return result.String()
}

// ensureImage pulls the Docker image if it doesn't exist locally.
func (e *ContainerExecutor) ensureImage(ctx context.Context, image string) error {
	e.log.Debug("ensuring image is available", "image", image)

	reader, err := e.client.ImagePull(ctx, image)
	if err != nil {
		return fmt.Errorf("failed to pull image: %w", err)
	}
	defer reader.Close()

	// Drain the reader to complete the pull
	// The pull progress is streamed as JSON, we just need to consume it
	_, err = io.Copy(io.Discard, reader)
	if err != nil {
		return fmt.Errorf("failed to complete image pull: %w", err)
	}

	e.log.Info("image ready", "image", image)
	return nil
}
