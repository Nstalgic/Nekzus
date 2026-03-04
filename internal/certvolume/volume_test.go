package certvolume

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/volume"
)

// MockDockerClient implements DockerClient for testing
type MockDockerClient struct {
	VolumeListFunc      func(ctx context.Context, options volume.ListOptions) (volume.ListResponse, error)
	VolumeCreateFunc    func(ctx context.Context, options volume.CreateOptions) (volume.Volume, error)
	ContainerCreateFunc func(ctx context.Context, config *container.Config, hostConfig *container.HostConfig, networkingConfig interface{}, platform interface{}, containerName string) (container.CreateResponse, error)
	ContainerStartFunc  func(ctx context.Context, containerID string, options container.StartOptions) error
	ContainerWaitFunc   func(ctx context.Context, containerID string, condition container.WaitCondition) (<-chan container.WaitResponse, <-chan error)
	ContainerRemoveFunc func(ctx context.Context, containerID string, options container.RemoveOptions) error
	ContainerLogsFunc   func(ctx context.Context, containerID string, options container.LogsOptions) (io.ReadCloser, error)
}

func (m *MockDockerClient) VolumeList(ctx context.Context, options volume.ListOptions) (volume.ListResponse, error) {
	if m.VolumeListFunc != nil {
		return m.VolumeListFunc(ctx, options)
	}
	return volume.ListResponse{}, nil
}

func (m *MockDockerClient) VolumeCreate(ctx context.Context, options volume.CreateOptions) (volume.Volume, error) {
	if m.VolumeCreateFunc != nil {
		return m.VolumeCreateFunc(ctx, options)
	}
	return volume.Volume{Name: options.Name}, nil
}

func (m *MockDockerClient) ContainerCreate(ctx context.Context, config *container.Config, hostConfig *container.HostConfig, networkingConfig interface{}, platform interface{}, containerName string) (container.CreateResponse, error) {
	if m.ContainerCreateFunc != nil {
		return m.ContainerCreateFunc(ctx, config, hostConfig, networkingConfig, platform, containerName)
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
	respCh := make(chan container.WaitResponse, 1)
	errCh := make(chan error)
	respCh <- container.WaitResponse{StatusCode: 0}
	return respCh, errCh
}

func (m *MockDockerClient) ContainerRemove(ctx context.Context, containerID string, options container.RemoveOptions) error {
	if m.ContainerRemoveFunc != nil {
		return m.ContainerRemoveFunc(ctx, containerID, options)
	}
	return nil
}

func (m *MockDockerClient) ContainerLogs(ctx context.Context, containerID string, options container.LogsOptions) (io.ReadCloser, error) {
	if m.ContainerLogsFunc != nil {
		return m.ContainerLogsFunc(ctx, containerID, options)
	}
	return io.NopCloser(strings.NewReader("")), nil
}

func TestGetCertVolumeName(t *testing.T) {
	name := GetCertVolumeName()
	if name != "nexus-certs" {
		t.Errorf("Expected volume name 'nexus-certs', got: %s", name)
	}
}

func TestEnsureCertVolume_CreateNew(t *testing.T) {
	created := false
	mock := &MockDockerClient{
		VolumeListFunc: func(ctx context.Context, options volume.ListOptions) (volume.ListResponse, error) {
			// No existing volumes
			return volume.ListResponse{Volumes: []*volume.Volume{}}, nil
		},
		VolumeCreateFunc: func(ctx context.Context, options volume.CreateOptions) (volume.Volume, error) {
			if options.Name != "nexus-certs" {
				t.Errorf("Expected volume name 'nexus-certs', got: %s", options.Name)
			}
			created = true
			return volume.Volume{Name: options.Name}, nil
		},
	}

	mgr := NewManager(mock)
	err := mgr.EnsureCertVolume(context.Background())

	if err != nil {
		t.Fatalf("EnsureCertVolume failed: %v", err)
	}

	if !created {
		t.Error("Expected volume to be created")
	}
}

func TestEnsureCertVolume_AlreadyExists(t *testing.T) {
	created := false
	mock := &MockDockerClient{
		VolumeListFunc: func(ctx context.Context, options volume.ListOptions) (volume.ListResponse, error) {
			// Volume already exists
			return volume.ListResponse{
				Volumes: []*volume.Volume{
					{Name: "nexus-certs"},
				},
			}, nil
		},
		VolumeCreateFunc: func(ctx context.Context, options volume.CreateOptions) (volume.Volume, error) {
			created = true
			return volume.Volume{Name: options.Name}, nil
		},
	}

	mgr := NewManager(mock)
	err := mgr.EnsureCertVolume(context.Background())

	if err != nil {
		t.Fatalf("EnsureCertVolume failed: %v", err)
	}

	if created {
		t.Error("Volume should not be created when it already exists")
	}
}

func TestEnsureCertVolume_ListError(t *testing.T) {
	mock := &MockDockerClient{
		VolumeListFunc: func(ctx context.Context, options volume.ListOptions) (volume.ListResponse, error) {
			return volume.ListResponse{}, errors.New("Docker daemon unavailable")
		},
	}

	mgr := NewManager(mock)
	err := mgr.EnsureCertVolume(context.Background())

	if err == nil {
		t.Error("Expected error when Docker daemon unavailable")
	}
}

func TestEnsureCertVolume_CreateError(t *testing.T) {
	mock := &MockDockerClient{
		VolumeListFunc: func(ctx context.Context, options volume.ListOptions) (volume.ListResponse, error) {
			return volume.ListResponse{Volumes: []*volume.Volume{}}, nil
		},
		VolumeCreateFunc: func(ctx context.Context, options volume.CreateOptions) (volume.Volume, error) {
			return volume.Volume{}, errors.New("failed to create volume")
		},
	}

	mgr := NewManager(mock)
	err := mgr.EnsureCertVolume(context.Background())

	if err == nil {
		t.Error("Expected error when volume creation fails")
	}
}

func TestWriteCertsToVolume_Success(t *testing.T) {
	containerStarted := false
	containerWaited := false
	containerRemoved := false

	mock := &MockDockerClient{
		ContainerCreateFunc: func(ctx context.Context, config *container.Config, hostConfig *container.HostConfig, networkingConfig interface{}, platform interface{}, containerName string) (container.CreateResponse, error) {
			// Verify cert data is passed via environment or command
			if config.Image != "alpine:latest" {
				t.Errorf("Expected alpine:latest image, got: %s", config.Image)
			}
			return container.CreateResponse{ID: "helper-container"}, nil
		},
		ContainerStartFunc: func(ctx context.Context, containerID string, options container.StartOptions) error {
			containerStarted = true
			return nil
		},
		ContainerWaitFunc: func(ctx context.Context, containerID string, condition container.WaitCondition) (<-chan container.WaitResponse, <-chan error) {
			containerWaited = true
			respCh := make(chan container.WaitResponse, 1)
			errCh := make(chan error)
			respCh <- container.WaitResponse{StatusCode: 0}
			return respCh, errCh
		},
		ContainerRemoveFunc: func(ctx context.Context, containerID string, options container.RemoveOptions) error {
			containerRemoved = true
			return nil
		},
	}

	mgr := NewManager(mock)
	err := mgr.WriteCertsToVolume(context.Background(), CertData{
		CACert: []byte("-----BEGIN CERTIFICATE-----\nCA CERT\n-----END CERTIFICATE-----"),
		Cert:   []byte("-----BEGIN CERTIFICATE-----\nSERVICE CERT\n-----END CERTIFICATE-----"),
		Key:    []byte("-----BEGIN PRIVATE KEY-----\nPRIVATE KEY\n-----END PRIVATE KEY-----"),
	})

	if err != nil {
		t.Fatalf("WriteCertsToVolume failed: %v", err)
	}

	if !containerStarted {
		t.Error("Expected helper container to be started")
	}
	if !containerWaited {
		t.Error("Expected helper container to be waited on")
	}
	if !containerRemoved {
		t.Error("Expected helper container to be removed")
	}
}

func TestWriteCertsToVolume_ContainerCreateError(t *testing.T) {
	mock := &MockDockerClient{
		ContainerCreateFunc: func(ctx context.Context, config *container.Config, hostConfig *container.HostConfig, networkingConfig interface{}, platform interface{}, containerName string) (container.CreateResponse, error) {
			return container.CreateResponse{}, errors.New("image not found")
		},
	}

	mgr := NewManager(mock)
	err := mgr.WriteCertsToVolume(context.Background(), CertData{
		CACert: []byte("CA"),
		Cert:   []byte("CERT"),
		Key:    []byte("KEY"),
	})

	if err == nil {
		t.Error("Expected error when container creation fails")
	}
}

func TestWriteCertsToVolume_ContainerExitError(t *testing.T) {
	mock := &MockDockerClient{
		ContainerWaitFunc: func(ctx context.Context, containerID string, condition container.WaitCondition) (<-chan container.WaitResponse, <-chan error) {
			respCh := make(chan container.WaitResponse, 1)
			errCh := make(chan error)
			// Non-zero exit code
			respCh <- container.WaitResponse{StatusCode: 1}
			return respCh, errCh
		},
	}

	mgr := NewManager(mock)
	err := mgr.WriteCertsToVolume(context.Background(), CertData{
		CACert: []byte("CA"),
		Cert:   []byte("CERT"),
		Key:    []byte("KEY"),
	})

	if err == nil {
		t.Error("Expected error when container exits with non-zero code")
	}
}

func TestWriteCertsToVolume_EmptyCerts(t *testing.T) {
	mock := &MockDockerClient{}
	mgr := NewManager(mock)

	// Empty CA cert
	err := mgr.WriteCertsToVolume(context.Background(), CertData{
		CACert: []byte{},
		Cert:   []byte("CERT"),
		Key:    []byte("KEY"),
	})
	if err == nil {
		t.Error("Expected error for empty CA cert")
	}

	// Empty cert
	err = mgr.WriteCertsToVolume(context.Background(), CertData{
		CACert: []byte("CA"),
		Cert:   []byte{},
		Key:    []byte("KEY"),
	})
	if err == nil {
		t.Error("Expected error for empty cert")
	}

	// Empty key
	err = mgr.WriteCertsToVolume(context.Background(), CertData{
		CACert: []byte("CA"),
		Cert:   []byte("CERT"),
		Key:    []byte{},
	})
	if err == nil {
		t.Error("Expected error for empty key")
	}
}

func TestGetCertMountConfig(t *testing.T) {
	mount := GetCertMountConfig()

	if mount.Source != "nexus-certs" {
		t.Errorf("Expected source 'nexus-certs', got: %s", mount.Source)
	}

	if mount.Target != "/certs" {
		t.Errorf("Expected target '/certs', got: %s", mount.Target)
	}

	if !mount.ReadOnly {
		t.Error("Expected mount to be read-only")
	}
}

func TestGetCertEnvironment(t *testing.T) {
	env := GetCertEnvironment()

	expected := map[string]string{
		"NEKZUS_CA_CERT":       "/certs/ca.crt",
		"NEKZUS_CERT":          "/certs/cert.crt",
		"NEKZUS_KEY":           "/certs/cert.key",
		"NEKZUS_CERT_DIR":      "/certs",
		"SSL_CERT_FILE":       "/certs/ca.crt",
		"NODE_EXTRA_CA_CERTS": "/certs/ca.crt",
	}

	for key, expectedVal := range expected {
		if val, ok := env[key]; !ok {
			t.Errorf("Missing environment variable: %s", key)
		} else if val != expectedVal {
			t.Errorf("Expected %s=%s, got: %s", key, expectedVal, val)
		}
	}
}

func TestVolumeExists(t *testing.T) {
	tests := []struct {
		name     string
		volumes  []*volume.Volume
		expected bool
	}{
		{
			name:     "Volume exists",
			volumes:  []*volume.Volume{{Name: "nexus-certs"}},
			expected: true,
		},
		{
			name:     "Volume not exists",
			volumes:  []*volume.Volume{{Name: "other-volume"}},
			expected: false,
		},
		{
			name:     "Empty volume list",
			volumes:  []*volume.Volume{},
			expected: false,
		},
		{
			name: "Multiple volumes, target exists",
			volumes: []*volume.Volume{
				{Name: "vol1"},
				{Name: "nexus-certs"},
				{Name: "vol2"},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &MockDockerClient{
				VolumeListFunc: func(ctx context.Context, options volume.ListOptions) (volume.ListResponse, error) {
					return volume.ListResponse{Volumes: tt.volumes}, nil
				},
			}

			mgr := NewManager(mock)
			exists, err := mgr.VolumeExists(context.Background())

			if err != nil {
				t.Fatalf("VolumeExists failed: %v", err)
			}

			if exists != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, exists)
			}
		})
	}
}

// TestGetMountConfigForCompose tests getting mount config for Compose services
func TestGetMountConfigForCompose(t *testing.T) {
	config := GetMountConfigForCompose()

	// Verify the config produces a valid Compose volume format
	if config.Source != CertVolumeName {
		t.Errorf("Expected source '%s', got: %s", CertVolumeName, config.Source)
	}

	if config.Target != CertMountPath {
		t.Errorf("Expected target '%s', got: %s", CertMountPath, config.Target)
	}

	if !config.ReadOnly {
		t.Error("Expected mount to be read-only")
	}
}

// TestGetComposeVolumeSpec tests getting the volume spec string for Compose files
func TestGetComposeVolumeSpec(t *testing.T) {
	spec := GetComposeVolumeSpec()

	// Should be in format: volume_name:/path:ro
	expected := "nexus-certs:/certs:ro"
	if spec != expected {
		t.Errorf("Expected spec '%s', got: %s", expected, spec)
	}
}
