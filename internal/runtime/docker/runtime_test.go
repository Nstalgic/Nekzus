package docker

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/nstalgic/nekzus/internal/runtime"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockDockerClient implements the DockerClient interface for testing
type mockDockerClient struct {
	pingErr       error
	containers    []types.Container
	listErr       error
	startErr      error
	stopErr       error
	restartErr    error
	inspectResult types.ContainerJSON
	inspectErr    error
	statsReader   io.ReadCloser
	statsErr      error
	logsReader    io.ReadCloser
	logsErr       error
	startedIDs    []string
	stoppedIDs    []string
	restartedIDs  []string
}

func (m *mockDockerClient) Ping(ctx context.Context) (types.Ping, error) {
	return types.Ping{}, m.pingErr
}

func (m *mockDockerClient) ContainerList(ctx context.Context, options container.ListOptions) ([]types.Container, error) {
	return m.containers, m.listErr
}

func (m *mockDockerClient) ContainerStart(ctx context.Context, containerID string, options container.StartOptions) error {
	m.startedIDs = append(m.startedIDs, containerID)
	return m.startErr
}

func (m *mockDockerClient) ContainerStop(ctx context.Context, containerID string, options container.StopOptions) error {
	m.stoppedIDs = append(m.stoppedIDs, containerID)
	return m.stopErr
}

func (m *mockDockerClient) ContainerRestart(ctx context.Context, containerID string, options container.StopOptions) error {
	m.restartedIDs = append(m.restartedIDs, containerID)
	return m.restartErr
}

func (m *mockDockerClient) ContainerInspect(ctx context.Context, containerID string) (types.ContainerJSON, error) {
	return m.inspectResult, m.inspectErr
}

func (m *mockDockerClient) ContainerStats(ctx context.Context, containerID string, stream bool) (container.StatsResponseReader, error) {
	if m.statsErr != nil {
		return container.StatsResponseReader{}, m.statsErr
	}
	return container.StatsResponseReader{Body: m.statsReader}, nil
}

func (m *mockDockerClient) ContainerLogs(ctx context.Context, containerID string, options container.LogsOptions) (io.ReadCloser, error) {
	return m.logsReader, m.logsErr
}

func (m *mockDockerClient) Close() error {
	return nil
}

func TestDockerRuntime_NewRuntime(t *testing.T) {
	mock := &mockDockerClient{}
	rt := NewRuntime(mock)

	assert.NotNil(t, rt)
	assert.Equal(t, "Docker", rt.Name())
	assert.Equal(t, runtime.RuntimeDocker, rt.Type())
}

func TestDockerRuntime_Ping(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mock := &mockDockerClient{}
		rt := NewRuntime(mock)

		err := rt.Ping(context.Background())
		assert.NoError(t, err)
	})

	t.Run("error", func(t *testing.T) {
		mock := &mockDockerClient{pingErr: errors.New("connection refused")}
		rt := NewRuntime(mock)

		err := rt.Ping(context.Background())
		assert.Error(t, err)
		assert.True(t, runtime.IsUnavailableError(err))
	})
}

func TestDockerRuntime_List(t *testing.T) {
	t.Run("lists containers", func(t *testing.T) {
		mock := &mockDockerClient{
			containers: []types.Container{
				{
					ID:      "abc123def456",
					Names:   []string{"/nginx"},
					Image:   "nginx:latest",
					State:   "running",
					Status:  "Up 2 hours",
					Created: 1234567890,
					Ports: []types.Port{
						{IP: "0.0.0.0", PrivatePort: 80, PublicPort: 8080, Type: "tcp"},
					},
					Labels: map[string]string{
						"nekzus.app.id": "nginx",
					},
				},
				{
					ID:      "xyz789abc012",
					Names:   []string{"/redis"},
					Image:   "redis:7",
					State:   "exited",
					Status:  "Exited (0) 1 hour ago",
					Created: 1234567800,
				},
			},
		}
		rt := NewRuntime(mock)

		containers, err := rt.List(context.Background(), runtime.ListOptions{All: true})

		require.NoError(t, err)
		assert.Len(t, containers, 2)

		// Check first container
		assert.Equal(t, "abc123def456", containers[0].ID.ID)
		assert.Equal(t, runtime.RuntimeDocker, containers[0].ID.Runtime)
		assert.Equal(t, "", containers[0].ID.Namespace)
		assert.Equal(t, "nginx", containers[0].Name)
		assert.Equal(t, "nginx:latest", containers[0].Image)
		assert.Equal(t, runtime.StateRunning, containers[0].State)
		assert.Equal(t, "Up 2 hours", containers[0].Status)
		assert.Len(t, containers[0].Ports, 1)
		assert.Equal(t, 80, containers[0].Ports[0].PrivatePort)
		assert.Equal(t, 8080, containers[0].Ports[0].PublicPort)
		assert.Equal(t, "nginx", containers[0].Labels["nekzus.app.id"])

		// Check second container
		assert.Equal(t, "xyz789abc012", containers[1].ID.ID)
		assert.Equal(t, "redis", containers[1].Name)
		assert.Equal(t, runtime.StateExited, containers[1].State)
	})

	t.Run("error", func(t *testing.T) {
		mock := &mockDockerClient{listErr: errors.New("daemon error")}
		rt := NewRuntime(mock)

		_, err := rt.List(context.Background(), runtime.ListOptions{})
		assert.Error(t, err)
	})

	t.Run("empty list", func(t *testing.T) {
		mock := &mockDockerClient{containers: []types.Container{}}
		rt := NewRuntime(mock)

		containers, err := rt.List(context.Background(), runtime.ListOptions{})

		require.NoError(t, err)
		assert.Empty(t, containers)
	})
}

func TestDockerRuntime_Start(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mock := &mockDockerClient{}
		rt := NewRuntime(mock)

		id := runtime.ContainerID{Runtime: runtime.RuntimeDocker, ID: "abc123"}
		err := rt.Start(context.Background(), id)

		assert.NoError(t, err)
		assert.Contains(t, mock.startedIDs, "abc123")
	})

	t.Run("container not found", func(t *testing.T) {
		mock := &mockDockerClient{startErr: errors.New("No such container: abc123")}
		rt := NewRuntime(mock)

		id := runtime.ContainerID{Runtime: runtime.RuntimeDocker, ID: "abc123"}
		err := rt.Start(context.Background(), id)

		assert.Error(t, err)
		assert.True(t, runtime.IsNotFoundError(err))
	})

	t.Run("error", func(t *testing.T) {
		mock := &mockDockerClient{startErr: errors.New("daemon error")}
		rt := NewRuntime(mock)

		id := runtime.ContainerID{Runtime: runtime.RuntimeDocker, ID: "abc123"}
		err := rt.Start(context.Background(), id)

		assert.Error(t, err)
	})
}

func TestDockerRuntime_Stop(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mock := &mockDockerClient{}
		rt := NewRuntime(mock)

		id := runtime.ContainerID{Runtime: runtime.RuntimeDocker, ID: "abc123"}
		err := rt.Stop(context.Background(), id, nil)

		assert.NoError(t, err)
		assert.Contains(t, mock.stoppedIDs, "abc123")
	})

	t.Run("with timeout", func(t *testing.T) {
		mock := &mockDockerClient{}
		rt := NewRuntime(mock)

		id := runtime.ContainerID{Runtime: runtime.RuntimeDocker, ID: "abc123"}
		timeout := 30 * time.Second
		err := rt.Stop(context.Background(), id, &timeout)

		assert.NoError(t, err)
		assert.Contains(t, mock.stoppedIDs, "abc123")
	})

	t.Run("container not found", func(t *testing.T) {
		mock := &mockDockerClient{stopErr: errors.New("No such container: abc123")}
		rt := NewRuntime(mock)

		id := runtime.ContainerID{Runtime: runtime.RuntimeDocker, ID: "abc123"}
		err := rt.Stop(context.Background(), id, nil)

		assert.Error(t, err)
		assert.True(t, runtime.IsNotFoundError(err))
	})
}

func TestDockerRuntime_Restart(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mock := &mockDockerClient{}
		rt := NewRuntime(mock)

		id := runtime.ContainerID{Runtime: runtime.RuntimeDocker, ID: "abc123"}
		err := rt.Restart(context.Background(), id, nil)

		assert.NoError(t, err)
		assert.Contains(t, mock.restartedIDs, "abc123")
	})

	t.Run("container not found", func(t *testing.T) {
		mock := &mockDockerClient{restartErr: errors.New("No such container: abc123")}
		rt := NewRuntime(mock)

		id := runtime.ContainerID{Runtime: runtime.RuntimeDocker, ID: "abc123"}
		err := rt.Restart(context.Background(), id, nil)

		assert.Error(t, err)
		assert.True(t, runtime.IsNotFoundError(err))
	})
}

func TestDockerRuntime_Inspect(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		inspectResult := types.ContainerJSON{
			ContainerJSONBase: &types.ContainerJSONBase{
				ID:      "abc123def456789",
				Name:    "/nginx",
				Image:   "sha256:abc123",
				Created: "2024-01-01T00:00:00Z",
				State: &types.ContainerState{
					Status:  "running",
					Running: true,
				},
			},
			Config: &container.Config{
				Image: "nginx:latest",
				Env:   []string{"ENV=production"},
				Cmd:   []string{"nginx", "-g", "daemon off;"},
			},
			NetworkSettings: &types.NetworkSettings{
				DefaultNetworkSettings: types.DefaultNetworkSettings{
					IPAddress: "172.17.0.2",
				},
			},
		}
		mock := &mockDockerClient{inspectResult: inspectResult}
		rt := NewRuntime(mock)

		id := runtime.ContainerID{Runtime: runtime.RuntimeDocker, ID: "abc123"}
		details, err := rt.Inspect(context.Background(), id)

		require.NoError(t, err)
		assert.Equal(t, "abc123def456789", details.ID.ID)
		assert.Equal(t, "nginx", details.Name)
		assert.Equal(t, runtime.StateRunning, details.State)
		assert.Contains(t, details.Config.Env, "ENV=production")
		assert.Equal(t, "172.17.0.2", details.NetworkSettings.IPAddress)
		assert.NotNil(t, details.Raw)
	})

	t.Run("container not found", func(t *testing.T) {
		mock := &mockDockerClient{inspectErr: errors.New("No such container: abc123")}
		rt := NewRuntime(mock)

		id := runtime.ContainerID{Runtime: runtime.RuntimeDocker, ID: "abc123"}
		_, err := rt.Inspect(context.Background(), id)

		assert.Error(t, err)
		assert.True(t, runtime.IsNotFoundError(err))
	})
}

func TestDockerRuntime_GetStats(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		// Create mock stats with two samples for CPU delta calculation
		sample1 := map[string]interface{}{
			"cpu_stats": map[string]interface{}{
				"cpu_usage": map[string]interface{}{
					"total_usage":  float64(100000000),
					"percpu_usage": []interface{}{float64(50000000), float64(50000000)},
				},
				"system_cpu_usage": float64(1000000000),
			},
			"precpu_stats": map[string]interface{}{
				"cpu_usage": map[string]interface{}{
					"total_usage":  float64(0),
					"percpu_usage": []interface{}{float64(0), float64(0)},
				},
				"system_cpu_usage": float64(0),
			},
			"memory_stats": map[string]interface{}{
				"usage": float64(104857600),  // 100MB
				"limit": float64(1073741824), // 1GB
			},
			"networks": map[string]interface{}{
				"eth0": map[string]interface{}{
					"rx_bytes": float64(1024),
					"tx_bytes": float64(2048),
				},
			},
		}
		sample2 := map[string]interface{}{
			"cpu_stats": map[string]interface{}{
				"cpu_usage": map[string]interface{}{
					"total_usage":  float64(200000000),
					"percpu_usage": []interface{}{float64(100000000), float64(100000000)},
				},
				"system_cpu_usage": float64(2000000000),
			},
			"precpu_stats": map[string]interface{}{
				"cpu_usage": map[string]interface{}{
					"total_usage":  float64(100000000),
					"percpu_usage": []interface{}{float64(50000000), float64(50000000)},
				},
				"system_cpu_usage": float64(1000000000),
			},
			"memory_stats": map[string]interface{}{
				"usage": float64(104857600),
				"limit": float64(1073741824),
			},
			"networks": map[string]interface{}{
				"eth0": map[string]interface{}{
					"rx_bytes": float64(2048),
					"tx_bytes": float64(4096),
				},
			},
		}

		data1, _ := json.Marshal(sample1)
		data2, _ := json.Marshal(sample2)
		statsData := string(data1) + "\n" + string(data2)

		mock := &mockDockerClient{
			statsReader: io.NopCloser(strings.NewReader(statsData)),
		}
		rt := NewRuntime(mock)

		id := runtime.ContainerID{Runtime: runtime.RuntimeDocker, ID: "abc123def456"}
		stats, err := rt.GetStats(context.Background(), id)

		require.NoError(t, err)
		assert.Equal(t, "abc123def456", stats.ContainerID.ID)
		assert.Greater(t, stats.CPU.Usage, 0.0)
		assert.Equal(t, float64(2), stats.CPU.TotalCores)
		assert.Greater(t, stats.Memory.Usage, 0.0)
		assert.Equal(t, uint64(104857600), stats.Memory.Used)
		assert.Equal(t, uint64(1073741824), stats.Memory.Limit)
		assert.Equal(t, uint64(2048), stats.Network.RxBytes)
		assert.Equal(t, uint64(4096), stats.Network.TxBytes)
	})

	t.Run("container not found", func(t *testing.T) {
		mock := &mockDockerClient{statsErr: errors.New("No such container: abc123")}
		rt := NewRuntime(mock)

		id := runtime.ContainerID{Runtime: runtime.RuntimeDocker, ID: "abc123"}
		_, err := rt.GetStats(context.Background(), id)

		assert.Error(t, err)
		assert.True(t, runtime.IsNotFoundError(err))
	})
}

func TestDockerRuntime_GetBatchStats(t *testing.T) {
	t.Run("empty list", func(t *testing.T) {
		mock := &mockDockerClient{}
		rt := NewRuntime(mock)

		stats, err := rt.GetBatchStats(context.Background(), []runtime.ContainerID{})

		require.NoError(t, err)
		assert.Empty(t, stats)
	})
}

func TestDockerRuntime_StreamLogs(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		logData := "line 1\nline 2\nline 3\n"
		mock := &mockDockerClient{
			logsReader: io.NopCloser(strings.NewReader(logData)),
		}
		rt := NewRuntime(mock)

		id := runtime.ContainerID{Runtime: runtime.RuntimeDocker, ID: "abc123"}
		reader, err := rt.StreamLogs(context.Background(), id, runtime.LogOptions{
			Follow:     true,
			Tail:       100,
			Timestamps: true,
		})

		require.NoError(t, err)
		defer reader.Close()

		data, err := io.ReadAll(reader)
		require.NoError(t, err)
		assert.Equal(t, logData, string(data))
	})

	t.Run("container not found", func(t *testing.T) {
		mock := &mockDockerClient{logsErr: errors.New("No such container: abc123")}
		rt := NewRuntime(mock)

		id := runtime.ContainerID{Runtime: runtime.RuntimeDocker, ID: "abc123"}
		_, err := rt.StreamLogs(context.Background(), id, runtime.LogOptions{})

		assert.Error(t, err)
		assert.True(t, runtime.IsNotFoundError(err))
	})
}

func TestDockerRuntime_Close(t *testing.T) {
	mock := &mockDockerClient{}
	rt := NewRuntime(mock)

	err := rt.Close()
	assert.NoError(t, err)
}

func TestConvertDockerState(t *testing.T) {
	tests := []struct {
		dockerState   string
		expectedState runtime.ContainerState
	}{
		{"running", runtime.StateRunning},
		{"created", runtime.StateCreated},
		{"paused", runtime.StatePaused},
		{"restarting", runtime.StateRestarting},
		{"exited", runtime.StateExited},
		{"dead", runtime.StateStopped},
		{"removing", runtime.StateStopped},
		{"unknown", runtime.StateStopped},
	}

	for _, tt := range tests {
		t.Run(tt.dockerState, func(t *testing.T) {
			result := convertDockerState(tt.dockerState)
			assert.Equal(t, tt.expectedState, result)
		})
	}
}
