package runtime

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockRuntime implements Runtime for testing
type mockRuntime struct {
	name        string
	runtimeType RuntimeType
	pingErr     error
	containers  []Container
	listErr     error
}

func (m *mockRuntime) Name() string                   { return m.name }
func (m *mockRuntime) Type() RuntimeType              { return m.runtimeType }
func (m *mockRuntime) Ping(ctx context.Context) error { return m.pingErr }
func (m *mockRuntime) Close() error                   { return nil }

func (m *mockRuntime) List(ctx context.Context, opts ListOptions) ([]Container, error) {
	return m.containers, m.listErr
}
func (m *mockRuntime) Start(ctx context.Context, id ContainerID) error { return nil }
func (m *mockRuntime) Stop(ctx context.Context, id ContainerID, timeout *time.Duration) error {
	return nil
}
func (m *mockRuntime) Restart(ctx context.Context, id ContainerID, timeout *time.Duration) error {
	return nil
}
func (m *mockRuntime) Inspect(ctx context.Context, id ContainerID) (*ContainerDetails, error) {
	return nil, nil
}
func (m *mockRuntime) GetStats(ctx context.Context, id ContainerID) (*Stats, error) {
	return nil, nil
}
func (m *mockRuntime) GetBatchStats(ctx context.Context, ids []ContainerID) ([]Stats, error) {
	return nil, nil
}
func (m *mockRuntime) StreamLogs(ctx context.Context, id ContainerID, opts LogOptions) (io.ReadCloser, error) {
	return nil, nil
}

func TestRegistry_NewRegistry(t *testing.T) {
	reg := NewRegistry()
	assert.NotNil(t, reg)
	assert.Empty(t, reg.Available())
}

func TestRegistry_Register(t *testing.T) {
	t.Run("registers runtime", func(t *testing.T) {
		reg := NewRegistry()
		mock := &mockRuntime{name: "Docker", runtimeType: RuntimeDocker}

		err := reg.Register(mock)
		require.NoError(t, err)

		available := reg.Available()
		assert.Contains(t, available, RuntimeDocker)
	})

	t.Run("sets first as primary", func(t *testing.T) {
		reg := NewRegistry()
		mock := &mockRuntime{name: "Docker", runtimeType: RuntimeDocker}

		err := reg.Register(mock)
		require.NoError(t, err)

		primary := reg.GetPrimary()
		assert.NotNil(t, primary)
		assert.Equal(t, RuntimeDocker, primary.Type())
	})

	t.Run("does not override primary", func(t *testing.T) {
		reg := NewRegistry()
		docker := &mockRuntime{name: "Docker", runtimeType: RuntimeDocker}
		k8s := &mockRuntime{name: "Kubernetes", runtimeType: RuntimeKubernetes}

		err := reg.Register(docker)
		require.NoError(t, err)
		err = reg.Register(k8s)
		require.NoError(t, err)

		primary := reg.GetPrimary()
		assert.Equal(t, RuntimeDocker, primary.Type())
	})

	t.Run("rejects duplicate runtime type", func(t *testing.T) {
		reg := NewRegistry()
		docker1 := &mockRuntime{name: "Docker", runtimeType: RuntimeDocker}
		docker2 := &mockRuntime{name: "Docker2", runtimeType: RuntimeDocker}

		err := reg.Register(docker1)
		require.NoError(t, err)

		err = reg.Register(docker2)
		assert.Error(t, err)
	})

	t.Run("rejects nil runtime", func(t *testing.T) {
		reg := NewRegistry()
		err := reg.Register(nil)
		assert.Error(t, err)
	})
}

func TestRegistry_Get(t *testing.T) {
	t.Run("returns registered runtime", func(t *testing.T) {
		reg := NewRegistry()
		mock := &mockRuntime{name: "Docker", runtimeType: RuntimeDocker}
		reg.Register(mock)

		rt, err := reg.Get(RuntimeDocker)
		require.NoError(t, err)
		assert.Equal(t, mock, rt)
	})

	t.Run("returns error for unregistered runtime", func(t *testing.T) {
		reg := NewRegistry()

		_, err := reg.Get(RuntimeDocker)
		assert.Error(t, err)
		assert.True(t, errors.Is(err, ErrRuntimeUnavailable))
	})
}

func TestRegistry_GetPrimary(t *testing.T) {
	t.Run("returns primary runtime", func(t *testing.T) {
		reg := NewRegistry()
		mock := &mockRuntime{name: "Docker", runtimeType: RuntimeDocker}
		reg.Register(mock)

		primary := reg.GetPrimary()
		assert.Equal(t, mock, primary)
	})

	t.Run("returns nil when no runtimes registered", func(t *testing.T) {
		reg := NewRegistry()
		primary := reg.GetPrimary()
		assert.Nil(t, primary)
	})
}

func TestRegistry_SetPrimary(t *testing.T) {
	t.Run("changes primary runtime", func(t *testing.T) {
		reg := NewRegistry()
		docker := &mockRuntime{name: "Docker", runtimeType: RuntimeDocker}
		k8s := &mockRuntime{name: "Kubernetes", runtimeType: RuntimeKubernetes}
		reg.Register(docker)
		reg.Register(k8s)

		err := reg.SetPrimary(RuntimeKubernetes)
		require.NoError(t, err)

		primary := reg.GetPrimary()
		assert.Equal(t, RuntimeKubernetes, primary.Type())
	})

	t.Run("returns error for unregistered runtime", func(t *testing.T) {
		reg := NewRegistry()

		err := reg.SetPrimary(RuntimeDocker)
		assert.Error(t, err)
	})
}

func TestRegistry_SelectForContainer(t *testing.T) {
	t.Run("returns runtime matching container ID", func(t *testing.T) {
		reg := NewRegistry()
		docker := &mockRuntime{name: "Docker", runtimeType: RuntimeDocker}
		k8s := &mockRuntime{name: "Kubernetes", runtimeType: RuntimeKubernetes}
		reg.Register(docker)
		reg.Register(k8s)

		id := ContainerID{Runtime: RuntimeKubernetes, ID: "pod-abc123", Namespace: "default"}
		rt, err := reg.SelectForContainer(id)

		require.NoError(t, err)
		assert.Equal(t, RuntimeKubernetes, rt.Type())
	})

	t.Run("returns error for unregistered runtime", func(t *testing.T) {
		reg := NewRegistry()
		docker := &mockRuntime{name: "Docker", runtimeType: RuntimeDocker}
		reg.Register(docker)

		id := ContainerID{Runtime: RuntimeKubernetes, ID: "pod-abc123"}
		_, err := reg.SelectForContainer(id)

		assert.Error(t, err)
	})
}

func TestRegistry_Available(t *testing.T) {
	reg := NewRegistry()
	docker := &mockRuntime{name: "Docker", runtimeType: RuntimeDocker}
	k8s := &mockRuntime{name: "Kubernetes", runtimeType: RuntimeKubernetes}
	reg.Register(docker)
	reg.Register(k8s)

	available := reg.Available()
	assert.Len(t, available, 2)
	assert.Contains(t, available, RuntimeDocker)
	assert.Contains(t, available, RuntimeKubernetes)
}

func TestRegistry_Close(t *testing.T) {
	reg := NewRegistry()
	docker := &mockRuntime{name: "Docker", runtimeType: RuntimeDocker}
	reg.Register(docker)

	err := reg.Close()
	assert.NoError(t, err)
}

func TestRegistry_Concurrency(t *testing.T) {
	reg := NewRegistry()
	docker := &mockRuntime{name: "Docker", runtimeType: RuntimeDocker}
	reg.Register(docker)

	// Test concurrent access
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			reg.GetPrimary()
			reg.Available()
			reg.Get(RuntimeDocker)
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}
