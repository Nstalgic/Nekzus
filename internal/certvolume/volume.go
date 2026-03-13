// Package certvolume manages Docker volumes for certificate distribution to containers.
package certvolume

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"log/slog"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/volume"
)

var log = slog.With("package", "certvolume")

const (
	// CertVolumeName is the name of the Docker volume for certificates
	CertVolumeName = "nekzus-certs"
	// CertMountPath is the default mount path inside containers
	CertMountPath = "/certs"
	// CACertFile is the filename for the CA certificate
	CACertFile = "ca.crt"
	// CertFile is the filename for the service certificate
	CertFile = "cert.crt"
	// KeyFile is the filename for the private key
	KeyFile = "cert.key"
)

// DockerClient defines the interface for Docker volume operations
type DockerClient interface {
	VolumeList(ctx context.Context, options volume.ListOptions) (volume.ListResponse, error)
	VolumeCreate(ctx context.Context, options volume.CreateOptions) (volume.Volume, error)
	ContainerCreate(ctx context.Context, config *container.Config, hostConfig *container.HostConfig, networkingConfig interface{}, platform interface{}, containerName string) (container.CreateResponse, error)
	ContainerStart(ctx context.Context, containerID string, options container.StartOptions) error
	ContainerWait(ctx context.Context, containerID string, condition container.WaitCondition) (<-chan container.WaitResponse, <-chan error)
	ContainerRemove(ctx context.Context, containerID string, options container.RemoveOptions) error
	ContainerLogs(ctx context.Context, containerID string, options container.LogsOptions) (io.ReadCloser, error)
}

// CertData contains certificate data to write to the volume
type CertData struct {
	CACert []byte // CA certificate PEM
	Cert   []byte // Service certificate PEM
	Key    []byte // Private key PEM
}

// Manager handles certificate volume operations
type Manager struct {
	client DockerClient
}

// NewManager creates a new certificate volume manager
func NewManager(client DockerClient) *Manager {
	return &Manager{client: client}
}

// GetCertVolumeName returns the certificate volume name
func GetCertVolumeName() string {
	return CertVolumeName
}

// EnsureCertVolume creates the certificate volume if it doesn't exist
func (m *Manager) EnsureCertVolume(ctx context.Context) error {
	exists, err := m.VolumeExists(ctx)
	if err != nil {
		return fmt.Errorf("failed to check volume existence: %w", err)
	}

	if exists {
		log.Debug("certificate volume already exists", "name", CertVolumeName)
		return nil
	}

	// Create the volume
	_, err = m.client.VolumeCreate(ctx, volume.CreateOptions{
		Name: CertVolumeName,
		Labels: map[string]string{
			"nekzus.managed": "true",
			"nekzus.purpose": "certificates",
		},
	})

	if err != nil {
		return fmt.Errorf("failed to create certificate volume: %w", err)
	}

	log.Info("created certificate volume", "name", CertVolumeName)
	return nil
}

// VolumeExists checks if the certificate volume exists
func (m *Manager) VolumeExists(ctx context.Context) (bool, error) {
	resp, err := m.client.VolumeList(ctx, volume.ListOptions{})
	if err != nil {
		return false, fmt.Errorf("failed to list volumes: %w", err)
	}

	for _, vol := range resp.Volumes {
		if vol.Name == CertVolumeName {
			return true, nil
		}
	}

	return false, nil
}

// WriteCertsToVolume writes certificate data to the Docker volume
// Uses a helper container with Alpine to write files
func (m *Manager) WriteCertsToVolume(ctx context.Context, data CertData) error {
	// Validate input
	if len(data.CACert) == 0 {
		return fmt.Errorf("CA certificate is required")
	}
	if len(data.Cert) == 0 {
		return fmt.Errorf("certificate is required")
	}
	if len(data.Key) == 0 {
		return fmt.Errorf("private key is required")
	}

	// Create a helper container to write files to the volume
	// We use Alpine with a shell command to write the files
	containerName := "nekzus-cert-writer"

	// Base64 encode the certificates for safe passing through shell
	caB64 := base64.StdEncoding.EncodeToString(data.CACert)
	certB64 := base64.StdEncoding.EncodeToString(data.Cert)
	keyB64 := base64.StdEncoding.EncodeToString(data.Key)

	// Shell script to decode and write certificates
	script := fmt.Sprintf(`
echo '%s' | base64 -d > /certs/%s && \
echo '%s' | base64 -d > /certs/%s && \
echo '%s' | base64 -d > /certs/%s && \
chmod 644 /certs/%s /certs/%s && \
chmod 600 /certs/%s && \
echo "Certificates written successfully"
`, caB64, CACertFile, certB64, CertFile, keyB64, KeyFile, CACertFile, CertFile, KeyFile)

	config := &container.Config{
		Image: "alpine:latest",
		Cmd:   []string{"sh", "-c", script},
	}

	hostConfig := &container.HostConfig{
		Mounts: []mount.Mount{
			{
				Type:   mount.TypeVolume,
				Source: CertVolumeName,
				Target: CertMountPath,
			},
		},
		AutoRemove: false, // We'll remove it manually after getting logs
	}

	// Create container
	resp, err := m.client.ContainerCreate(ctx, config, hostConfig, nil, nil, containerName)
	if err != nil {
		return fmt.Errorf("failed to create cert writer container: %w", err)
	}
	containerID := resp.ID

	// Ensure cleanup
	defer func() {
		removeErr := m.client.ContainerRemove(ctx, containerID, container.RemoveOptions{Force: true})
		if removeErr != nil {
			log.Warn("failed to remove cert writer container", "error", removeErr)
		}
	}()

	// Start container
	if err := m.client.ContainerStart(ctx, containerID, container.StartOptions{}); err != nil {
		return fmt.Errorf("failed to start cert writer container: %w", err)
	}

	// Wait for container to finish
	statusCh, errCh := m.client.ContainerWait(ctx, containerID, container.WaitConditionNotRunning)
	select {
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("error waiting for cert writer: %w", err)
		}
	case status := <-statusCh:
		if status.StatusCode != 0 {
			// Get logs for debugging
			logs, logErr := m.client.ContainerLogs(ctx, containerID, container.LogsOptions{
				ShowStdout: true,
				ShowStderr: true,
			})
			if logErr == nil && logs != nil {
				logBytes, readErr := io.ReadAll(logs)
				logs.Close()
				if readErr == nil && len(logBytes) > 0 {
					return fmt.Errorf("cert writer exited with code %d: %s", status.StatusCode, string(logBytes))
				}
			}
			return fmt.Errorf("cert writer exited with code %d", status.StatusCode)
		}
	case <-ctx.Done():
		return ctx.Err()
	}

	log.Info("certificates written to volume", "volume", CertVolumeName)
	return nil
}

// GetCertMountConfig returns the mount configuration for containers
func GetCertMountConfig() mount.Mount {
	return mount.Mount{
		Type:     mount.TypeVolume,
		Source:   CertVolumeName,
		Target:   CertMountPath,
		ReadOnly: true,
	}
}

// GetCertEnvironment returns environment variables for certificate paths
func GetCertEnvironment() map[string]string {
	return map[string]string{
		"NEKZUS_CA_CERT":      CertMountPath + "/" + CACertFile,
		"NEKZUS_CERT":         CertMountPath + "/" + CertFile,
		"NEKZUS_KEY":          CertMountPath + "/" + KeyFile,
		"NEKZUS_CERT_DIR":     CertMountPath,
		"SSL_CERT_FILE":       CertMountPath + "/" + CACertFile, // Standard env for many apps
		"NODE_EXTRA_CA_CERTS": CertMountPath + "/" + CACertFile, // Node.js
	}
}

// MountConfig represents a simplified mount configuration
type MountConfig struct {
	Source   string
	Target   string
	ReadOnly bool
}

// GetMountConfigForCompose returns the mount configuration for Docker Compose services
func GetMountConfigForCompose() MountConfig {
	return MountConfig{
		Source:   CertVolumeName,
		Target:   CertMountPath,
		ReadOnly: true,
	}
}

// GetComposeVolumeSpec returns the volume specification string for Docker Compose files
// Format: volume_name:/path:ro
func GetComposeVolumeSpec() string {
	return CertVolumeName + ":" + CertMountPath + ":ro"
}
