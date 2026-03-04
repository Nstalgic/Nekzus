package scripts

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
)

// MockDockerClient implements DockerClientForExecution for testing
type MockDockerClient struct {
	ContainerCreateFunc func(ctx context.Context, config *container.Config, hostConfig *container.HostConfig, containerName string) (container.CreateResponse, error)
	ContainerStartFunc  func(ctx context.Context, containerID string, options container.StartOptions) error
	ContainerWaitFunc   func(ctx context.Context, containerID string, condition container.WaitCondition) (<-chan container.WaitResponse, <-chan error)
	ContainerLogsFunc   func(ctx context.Context, containerID string, options container.LogsOptions) (io.ReadCloser, error)
	ContainerRemoveFunc func(ctx context.Context, containerID string, options container.RemoveOptions) error
	ContainerStopFunc   func(ctx context.Context, containerID string, options container.StopOptions) error
	ImagePullFunc       func(ctx context.Context, refStr string) (io.ReadCloser, error)
}

func (m *MockDockerClient) ContainerCreate(ctx context.Context, config *container.Config, hostConfig *container.HostConfig, containerName string) (container.CreateResponse, error) {
	if m.ContainerCreateFunc != nil {
		return m.ContainerCreateFunc(ctx, config, hostConfig, containerName)
	}
	return container.CreateResponse{ID: "test-container-id"}, nil
}

func (m *MockDockerClient) ContainerStart(ctx context.Context, containerID string, options container.StartOptions) error {
	if m.ContainerStartFunc != nil {
		return m.ContainerStartFunc(ctx, containerID, options)
	}
	return nil
}

func (m *MockDockerClient) ContainerWait(ctx context.Context, containerID string, condition container.WaitCondition) (<-chan container.WaitResponse, <-chan error) {
	if m.ContainerWaitFunc != nil {
		return m.ContainerWaitFunc(ctx, containerID, condition)
	}
	statusCh := make(chan container.WaitResponse, 1)
	errCh := make(chan error, 1)
	statusCh <- container.WaitResponse{StatusCode: 0}
	return statusCh, errCh
}

func (m *MockDockerClient) ContainerLogs(ctx context.Context, containerID string, options container.LogsOptions) (io.ReadCloser, error) {
	if m.ContainerLogsFunc != nil {
		return m.ContainerLogsFunc(ctx, containerID, options)
	}
	return io.NopCloser(strings.NewReader("test output")), nil
}

func (m *MockDockerClient) ContainerRemove(ctx context.Context, containerID string, options container.RemoveOptions) error {
	if m.ContainerRemoveFunc != nil {
		return m.ContainerRemoveFunc(ctx, containerID, options)
	}
	return nil
}

func (m *MockDockerClient) ContainerStop(ctx context.Context, containerID string, options container.StopOptions) error {
	if m.ContainerStopFunc != nil {
		return m.ContainerStopFunc(ctx, containerID, options)
	}
	return nil
}

func (m *MockDockerClient) ImagePull(ctx context.Context, refStr string) (io.ReadCloser, error) {
	if m.ImagePullFunc != nil {
		return m.ImagePullFunc(ctx, refStr)
	}
	// Default: return empty reader (image already exists)
	return io.NopCloser(strings.NewReader("")), nil
}

func TestContainerExecutor_Execute_Success(t *testing.T) {
	client := &MockDockerClient{
		ContainerCreateFunc: func(ctx context.Context, config *container.Config, hostConfig *container.HostConfig, containerName string) (container.CreateResponse, error) {
			// Verify script is mounted
			if len(hostConfig.Binds) == 0 {
				t.Error("Expected script directory to be mounted")
			}
			// Verify correct image for shell script
			if config.Image != "alpine:3.20" {
				t.Errorf("Expected alpine:3.20 image, got %s", config.Image)
			}
			return container.CreateResponse{ID: "test-123"}, nil
		},
		ContainerWaitFunc: func(ctx context.Context, containerID string, condition container.WaitCondition) (<-chan container.WaitResponse, <-chan error) {
			statusCh := make(chan container.WaitResponse, 1)
			errCh := make(chan error, 1)
			statusCh <- container.WaitResponse{StatusCode: 0}
			return statusCh, errCh
		},
		ContainerLogsFunc: func(ctx context.Context, containerID string, options container.LogsOptions) (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader("Hello from container\n")), nil
		},
	}

	exec := NewContainerExecutor(client, "/app/scripts", ContainerExecutorConfig{
		DefaultTimeout: 30 * time.Second,
		MaxOutputBytes: 1024 * 1024,
		ShellImage:     "alpine:3.20",
		PythonImage:    "python:3.12-alpine",
	})

	script := &Script{
		ID:             "test-script",
		Name:           "Test Script",
		ScriptPath:     "hello.sh",
		ScriptType:     ScriptTypeShell,
		TimeoutSeconds: 30,
	}

	ctx := context.Background()
	result, err := exec.Execute(ctx, script, nil, false)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result.ExitCode != 0 {
		t.Errorf("Expected exit code 0, got %d", result.ExitCode)
	}

	if !strings.Contains(result.Output, "Hello from container") {
		t.Errorf("Expected output to contain 'Hello from container', got: %s", result.Output)
	}
}

func TestContainerExecutor_Execute_Failure(t *testing.T) {
	client := &MockDockerClient{
		ContainerWaitFunc: func(ctx context.Context, containerID string, condition container.WaitCondition) (<-chan container.WaitResponse, <-chan error) {
			statusCh := make(chan container.WaitResponse, 1)
			errCh := make(chan error, 1)
			statusCh <- container.WaitResponse{StatusCode: 1}
			return statusCh, errCh
		},
		ContainerLogsFunc: func(ctx context.Context, containerID string, options container.LogsOptions) (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader("Script error\n")), nil
		},
	}

	exec := NewContainerExecutor(client, "/app/scripts", ContainerExecutorConfig{
		DefaultTimeout: 30 * time.Second,
		MaxOutputBytes: 1024 * 1024,
		ShellImage:     "alpine:3.20",
	})

	script := &Script{
		ID:             "fail-script",
		Name:           "Fail Script",
		ScriptPath:     "fail.sh",
		ScriptType:     ScriptTypeShell,
		TimeoutSeconds: 30,
	}

	ctx := context.Background()
	result, err := exec.Execute(ctx, script, nil, false)
	if err != nil {
		t.Fatalf("Execute should not return error for script failure: %v", err)
	}

	if result.ExitCode != 1 {
		t.Errorf("Expected exit code 1, got %d", result.ExitCode)
	}
}

func TestContainerExecutor_Execute_Timeout(t *testing.T) {
	client := &MockDockerClient{
		ContainerWaitFunc: func(ctx context.Context, containerID string, condition container.WaitCondition) (<-chan container.WaitResponse, <-chan error) {
			// Simulate container running forever
			statusCh := make(chan container.WaitResponse)
			errCh := make(chan error, 1)
			go func() {
				<-ctx.Done()
				errCh <- ctx.Err()
			}()
			return statusCh, errCh
		},
		ContainerLogsFunc: func(ctx context.Context, containerID string, options container.LogsOptions) (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader("Partial output before timeout\n")), nil
		},
	}

	exec := NewContainerExecutor(client, "/app/scripts", ContainerExecutorConfig{
		DefaultTimeout: 100 * time.Millisecond, // Short timeout for test
		MaxOutputBytes: 1024 * 1024,
		ShellImage:     "alpine:3.20",
	})

	script := &Script{
		ID:             "timeout-script",
		Name:           "Timeout Script",
		ScriptPath:     "sleep.sh",
		ScriptType:     ScriptTypeShell,
		TimeoutSeconds: 0, // Use default timeout
	}

	ctx := context.Background()
	result, err := exec.Execute(ctx, script, nil, false)
	if err != nil {
		t.Fatalf("Execute should not return error for timeout: %v", err)
	}

	if !result.TimedOut {
		t.Error("Expected script to time out")
	}

	if result.ExitCode != -1 {
		t.Errorf("Expected exit code -1 for timeout, got %d", result.ExitCode)
	}
}

func TestContainerExecutor_Execute_Cancellation(t *testing.T) {
	client := &MockDockerClient{
		ContainerWaitFunc: func(ctx context.Context, containerID string, condition container.WaitCondition) (<-chan container.WaitResponse, <-chan error) {
			statusCh := make(chan container.WaitResponse)
			errCh := make(chan error, 1)
			go func() {
				<-ctx.Done()
				errCh <- ctx.Err()
			}()
			return statusCh, errCh
		},
	}

	exec := NewContainerExecutor(client, "/app/scripts", ContainerExecutorConfig{
		DefaultTimeout: 30 * time.Second,
		MaxOutputBytes: 1024 * 1024,
		ShellImage:     "alpine:3.20",
	})

	script := &Script{
		ID:             "cancel-script",
		Name:           "Cancel Script",
		ScriptPath:     "sleep.sh",
		ScriptType:     ScriptTypeShell,
		TimeoutSeconds: 30,
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Cancel after a short delay
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	result, err := exec.Execute(ctx, script, nil, false)
	if err != nil {
		t.Fatalf("Execute should not return error for cancellation: %v", err)
	}

	if !result.Cancelled {
		t.Error("Expected script to be cancelled")
	}
}

func TestContainerExecutor_Execute_PythonScript(t *testing.T) {
	var capturedImage string

	client := &MockDockerClient{
		ContainerCreateFunc: func(ctx context.Context, config *container.Config, hostConfig *container.HostConfig, containerName string) (container.CreateResponse, error) {
			capturedImage = config.Image
			return container.CreateResponse{ID: "test-py-123"}, nil
		},
		ContainerWaitFunc: func(ctx context.Context, containerID string, condition container.WaitCondition) (<-chan container.WaitResponse, <-chan error) {
			statusCh := make(chan container.WaitResponse, 1)
			errCh := make(chan error, 1)
			statusCh <- container.WaitResponse{StatusCode: 0}
			return statusCh, errCh
		},
		ContainerLogsFunc: func(ctx context.Context, containerID string, options container.LogsOptions) (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader("Python output\n")), nil
		},
	}

	exec := NewContainerExecutor(client, "/app/scripts", ContainerExecutorConfig{
		DefaultTimeout: 30 * time.Second,
		MaxOutputBytes: 1024 * 1024,
		ShellImage:     "alpine:3.20",
		PythonImage:    "python:3.12-alpine",
	})

	script := &Script{
		ID:             "python-script",
		Name:           "Python Script",
		ScriptPath:     "process.py",
		ScriptType:     ScriptTypePython,
		TimeoutSeconds: 30,
	}

	ctx := context.Background()
	_, err := exec.Execute(ctx, script, nil, false)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if capturedImage != "python:3.12-alpine" {
		t.Errorf("Expected python:3.12-alpine image, got %s", capturedImage)
	}
}

func TestContainerExecutor_Execute_WithParams(t *testing.T) {
	var capturedEnv []string

	client := &MockDockerClient{
		ContainerCreateFunc: func(ctx context.Context, config *container.Config, hostConfig *container.HostConfig, containerName string) (container.CreateResponse, error) {
			capturedEnv = config.Env
			return container.CreateResponse{ID: "test-env-123"}, nil
		},
		ContainerWaitFunc: func(ctx context.Context, containerID string, condition container.WaitCondition) (<-chan container.WaitResponse, <-chan error) {
			statusCh := make(chan container.WaitResponse, 1)
			errCh := make(chan error, 1)
			statusCh <- container.WaitResponse{StatusCode: 0}
			return statusCh, errCh
		},
		ContainerLogsFunc: func(ctx context.Context, containerID string, options container.LogsOptions) (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader("")), nil
		},
	}

	exec := NewContainerExecutor(client, "/app/scripts", ContainerExecutorConfig{
		DefaultTimeout: 30 * time.Second,
		MaxOutputBytes: 1024 * 1024,
		ShellImage:     "alpine:3.20",
	})

	script := &Script{
		ID:             "env-script",
		Name:           "Env Script",
		ScriptPath:     "env.sh",
		ScriptType:     ScriptTypeShell,
		TimeoutSeconds: 30,
		Environment: map[string]string{
			"SCRIPT_VAR": "from_script",
		},
	}

	params := map[string]string{
		"PARAM1": "value1",
		"PARAM2": "value2",
	}

	ctx := context.Background()
	_, err := exec.Execute(ctx, script, params, false)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// Check environment variables were set
	envMap := make(map[string]string)
	for _, e := range capturedEnv {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 {
			envMap[parts[0]] = parts[1]
		}
	}

	if envMap["PARAM1"] != "value1" {
		t.Errorf("Expected PARAM1=value1, got %s", envMap["PARAM1"])
	}
	if envMap["SCRIPT_VAR"] != "from_script" {
		t.Errorf("Expected SCRIPT_VAR=from_script, got %s", envMap["SCRIPT_VAR"])
	}
}

func TestContainerExecutor_Execute_DryRun(t *testing.T) {
	var capturedEnv []string

	client := &MockDockerClient{
		ContainerCreateFunc: func(ctx context.Context, config *container.Config, hostConfig *container.HostConfig, containerName string) (container.CreateResponse, error) {
			capturedEnv = config.Env
			return container.CreateResponse{ID: "test-dry-123"}, nil
		},
		ContainerWaitFunc: func(ctx context.Context, containerID string, condition container.WaitCondition) (<-chan container.WaitResponse, <-chan error) {
			statusCh := make(chan container.WaitResponse, 1)
			errCh := make(chan error, 1)
			statusCh <- container.WaitResponse{StatusCode: 0}
			return statusCh, errCh
		},
		ContainerLogsFunc: func(ctx context.Context, containerID string, options container.LogsOptions) (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader("")), nil
		},
	}

	exec := NewContainerExecutor(client, "/app/scripts", ContainerExecutorConfig{
		DefaultTimeout: 30 * time.Second,
		MaxOutputBytes: 1024 * 1024,
		ShellImage:     "alpine:3.20",
	})

	script := &Script{
		ID:             "dry-script",
		Name:           "Dry Run Script",
		ScriptPath:     "script.sh",
		ScriptType:     ScriptTypeShell,
		TimeoutSeconds: 30,
	}

	ctx := context.Background()
	_, err := exec.Execute(ctx, script, nil, true)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// Check DRY_RUN environment variable was set
	envMap := make(map[string]string)
	for _, e := range capturedEnv {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 {
			envMap[parts[0]] = parts[1]
		}
	}

	if envMap["DRY_RUN"] != "true" {
		t.Error("Expected DRY_RUN=true for dry run execution")
	}
}

func TestContainerExecutor_Execute_ContainerCreateError(t *testing.T) {
	client := &MockDockerClient{
		ContainerCreateFunc: func(ctx context.Context, config *container.Config, hostConfig *container.HostConfig, containerName string) (container.CreateResponse, error) {
			return container.CreateResponse{}, errors.New("failed to create container")
		},
	}

	exec := NewContainerExecutor(client, "/app/scripts", ContainerExecutorConfig{
		DefaultTimeout: 30 * time.Second,
		MaxOutputBytes: 1024 * 1024,
		ShellImage:     "alpine:3.20",
	})

	script := &Script{
		ID:             "error-script",
		Name:           "Error Script",
		ScriptPath:     "script.sh",
		ScriptType:     ScriptTypeShell,
		TimeoutSeconds: 30,
	}

	ctx := context.Background()
	_, err := exec.Execute(ctx, script, nil, false)
	if err == nil {
		t.Error("Expected error for container create failure")
	}
}

func TestContainerExecutor_ContainerCleanup(t *testing.T) {
	var containerRemoved bool
	var containerStopped bool

	client := &MockDockerClient{
		ContainerCreateFunc: func(ctx context.Context, config *container.Config, hostConfig *container.HostConfig, containerName string) (container.CreateResponse, error) {
			return container.CreateResponse{ID: "test-cleanup-123"}, nil
		},
		ContainerWaitFunc: func(ctx context.Context, containerID string, condition container.WaitCondition) (<-chan container.WaitResponse, <-chan error) {
			statusCh := make(chan container.WaitResponse, 1)
			errCh := make(chan error, 1)
			statusCh <- container.WaitResponse{StatusCode: 0}
			return statusCh, errCh
		},
		ContainerLogsFunc: func(ctx context.Context, containerID string, options container.LogsOptions) (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader("")), nil
		},
		ContainerStopFunc: func(ctx context.Context, containerID string, options container.StopOptions) error {
			containerStopped = true
			return nil
		},
		ContainerRemoveFunc: func(ctx context.Context, containerID string, options container.RemoveOptions) error {
			containerRemoved = true
			return nil
		},
	}

	exec := NewContainerExecutor(client, "/app/scripts", ContainerExecutorConfig{
		DefaultTimeout: 30 * time.Second,
		MaxOutputBytes: 1024 * 1024,
		ShellImage:     "alpine:3.20",
	})

	script := &Script{
		ID:             "cleanup-script",
		Name:           "Cleanup Script",
		ScriptPath:     "script.sh",
		ScriptType:     ScriptTypeShell,
		TimeoutSeconds: 30,
	}

	ctx := context.Background()
	_, err := exec.Execute(ctx, script, nil, false)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if !containerRemoved {
		t.Error("Expected container to be removed after execution")
	}

	// Stop is called on timeout/cancellation, not success
	_ = containerStopped
}

func TestContainerExecutor_GoBinaryScript(t *testing.T) {
	var capturedCmd []string

	client := &MockDockerClient{
		ContainerCreateFunc: func(ctx context.Context, config *container.Config, hostConfig *container.HostConfig, containerName string) (container.CreateResponse, error) {
			capturedCmd = config.Cmd
			return container.CreateResponse{ID: "test-go-123"}, nil
		},
		ContainerWaitFunc: func(ctx context.Context, containerID string, condition container.WaitCondition) (<-chan container.WaitResponse, <-chan error) {
			statusCh := make(chan container.WaitResponse, 1)
			errCh := make(chan error, 1)
			statusCh <- container.WaitResponse{StatusCode: 0}
			return statusCh, errCh
		},
		ContainerLogsFunc: func(ctx context.Context, containerID string, options container.LogsOptions) (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader("")), nil
		},
	}

	exec := NewContainerExecutor(client, "/app/scripts", ContainerExecutorConfig{
		DefaultTimeout: 30 * time.Second,
		MaxOutputBytes: 1024 * 1024,
		ShellImage:     "alpine:3.20",
	})

	script := &Script{
		ID:             "go-binary",
		Name:           "Go Binary",
		ScriptPath:     "myprogram",
		ScriptType:     ScriptTypeGoBinary,
		TimeoutSeconds: 30,
	}

	ctx := context.Background()
	_, err := exec.Execute(ctx, script, nil, false)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// Go binaries should be executed directly
	if len(capturedCmd) == 0 || capturedCmd[0] != "/scripts/myprogram" {
		t.Errorf("Expected direct execution of /scripts/myprogram, got %v", capturedCmd)
	}
}
