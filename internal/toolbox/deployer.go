package toolbox

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/nstalgic/nekzus/internal/certvolume"
	"github.com/nstalgic/nekzus/internal/types"
)

// Deployer handles Docker container deployment for toolbox services.
type Deployer struct {
	dockerClient *client.Client
	dataDir      string // Container path for creating directories
	hostDataDir  string // Host path for bind mounts (Docker-in-Docker)
}

// VolumePath represents a volume mount configuration.
type VolumePath struct {
	HostPath      string
	ContainerPath string
}

// NewDeployer creates a new Docker deployer.
// dataDir: path for creating directories (container path when running in Docker)
// hostDataDir: path for bind mounts (host path for Docker-in-Docker). If empty, uses dataDir.
func NewDeployer(dataDir string, hostDataDir string) (*Deployer, error) {
	// Create Docker client
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("failed to create Docker client: %w", err)
	}

	// Convert data directory to absolute path
	absDataDir, err := filepath.Abs(dataDir)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve absolute path for data directory: %w", err)
	}

	// Ensure data directory exists
	if err := os.MkdirAll(absDataDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create data directory: %w", err)
	}

	// If no host data dir specified, use the same as data dir (non-Docker-in-Docker scenario)
	absHostDataDir := hostDataDir
	if absHostDataDir == "" {
		absHostDataDir = absDataDir
	} else {
		// Convert host data directory to absolute path if it's relative
		if !filepath.IsAbs(absHostDataDir) {
			absHostDataDir, err = filepath.Abs(absHostDataDir)
			if err != nil {
				return nil, fmt.Errorf("failed to resolve absolute path for host data directory: %w", err)
			}
		}
	}

	return &Deployer{
		dockerClient: cli,
		dataDir:      absDataDir,
		hostDataDir:  absHostDataDir,
	}, nil
}

// Close closes the Docker client connection.
func (d *Deployer) Close() error {
	if d.dockerClient != nil {
		return d.dockerClient.Close()
	}
	return nil
}

// ValidateDeployment validates a deployment configuration.
func (d *Deployer) ValidateDeployment(template *types.ServiceTemplate, envVars map[string]string) error {
	if template == nil {
		return fmt.Errorf("template cannot be nil")
	}

	// Check if using Compose or legacy Docker config
	if template.ComposeProject != nil {
		// Validate Compose project
		if len(template.ComposeProject.Services) == 0 {
			return fmt.Errorf("Compose project must have at least one service")
		}
		return nil
	}

	// Legacy validation for single-container deployments
	if template.DockerConfig.Image == "" {
		return fmt.Errorf("Docker image is required")
	}

	return nil
}

// RenderTemplate replaces {{VAR}} placeholders with actual values.
func (d *Deployer) RenderTemplate(template string, vars map[string]string) string {
	result := template
	for key, value := range vars {
		placeholder := fmt.Sprintf("{{%s}}", key)
		result = strings.ReplaceAll(result, placeholder, value)
	}
	return result
}

// CheckPortConflicts checks if ports are available on the host.
func (d *Deployer) CheckPortConflicts(ports []types.PortMapping, envVars map[string]string) error {
	for _, portMapping := range ports {
		hostPort := portMapping.HostDefault

		// Check if port is overridden in env vars
		if override, exists := envVars["HOST_PORT"]; exists {
			// Parse override port
			var overridePort int
			fmt.Sscanf(override, "%d", &overridePort)
			if overridePort > 0 {
				hostPort = overridePort
			}
		}

		// Try to listen on port to check availability
		protocol := portMapping.Protocol
		if protocol == "" {
			protocol = "tcp"
		}

		addr := fmt.Sprintf(":%d", hostPort)
		listener, err := net.Listen(protocol, addr)
		if err != nil {
			return fmt.Errorf("port %d is already in use", hostPort)
		}
		listener.Close()
	}

	return nil
}

// GenerateContainerName generates a unique container name.
func (d *Deployer) GenerateContainerName(serviceName string) string {
	sanitized := d.SanitizeContainerName(serviceName)
	// Could add timestamp or random suffix for uniqueness if needed
	return sanitized
}

// SanitizeContainerName converts a service name to a valid Docker container name.
func (d *Deployer) SanitizeContainerName(name string) string {
	// Convert to lowercase
	name = strings.ToLower(name)

	// Replace spaces and underscores with hyphens
	name = strings.ReplaceAll(name, " ", "-")
	name = strings.ReplaceAll(name, "_", "-")

	// Remove special characters (keep only alphanumeric and hyphens)
	reg := regexp.MustCompile(`[^a-z0-9-]`)
	name = reg.ReplaceAllString(name, "")

	// Remove consecutive hyphens
	reg = regexp.MustCompile(`-+`)
	name = reg.ReplaceAllString(name, "-")

	// Trim hyphens from start and end
	name = strings.Trim(name, "-")

	return name
}

// BuildVolumePaths generates host and container volume paths.
// It also ensures the host directories exist.
func (d *Deployer) BuildVolumePaths(serviceName string, volumes []types.VolumeMapping) []VolumePath {
	paths := make([]VolumePath, 0, len(volumes))

	for _, vol := range volumes {
		// Create directory at dataDir path (where this container can write)
		containerPath := filepath.Join(d.dataDir, serviceName, vol.Name)
		if err := os.MkdirAll(containerPath, 0755); err != nil {
			// Log error but continue - Docker will fail gracefully if mount fails
			fmt.Printf("Warning: failed to create volume directory %s: %v\n", containerPath, err)
		}

		// Use hostDataDir for bind mount (what Docker daemon needs)
		hostPath := filepath.Join(d.hostDataDir, serviceName, vol.Name)

		paths = append(paths, VolumePath{
			HostPath:      hostPath,
			ContainerPath: vol.MountPath,
		})
	}

	return paths
}

// BuildEnvironment builds the container environment variables.
func (d *Deployer) BuildEnvironment(template *types.ServiceTemplate, userVars map[string]string) []string {
	env := []string{}

	// Add default environment variables from template
	for key, value := range template.DockerConfig.Environment {
		// Render template variables
		rendered := d.RenderTemplate(value, userVars)
		env = append(env, fmt.Sprintf("%s=%s", key, rendered))
	}

	// Add user-provided environment variables
	for key, value := range userVars {
		// Skip if already set from defaults
		found := false
		for _, e := range env {
			if strings.HasPrefix(e, key+"=") {
				found = true
				break
			}
		}
		if !found {
			env = append(env, fmt.Sprintf("%s=%s", key, value))
		}
	}

	return env
}

// CreateContainerConfig creates Docker container and host configurations.
func (d *Deployer) CreateContainerConfig(template *types.ServiceTemplate, containerName string, userVars map[string]string) (*container.Config, *container.HostConfig) {
	// Build environment variables
	env := d.BuildEnvironment(template, userVars)

	// Create container config
	config := &container.Config{
		Image: template.DockerConfig.Image,
		Env:   env,
	}

	// Build port bindings
	portBindings := nat.PortMap{}
	exposedPorts := nat.PortSet{}

	for _, portMapping := range template.DockerConfig.Ports {
		hostPort := portMapping.HostDefault

		// Check for port override in user vars
		if override, exists := userVars["HOST_PORT"]; exists {
			var overridePort int
			fmt.Sscanf(override, "%d", &overridePort)
			if overridePort > 0 {
				hostPort = overridePort
			}
		}

		protocol := portMapping.Protocol
		if protocol == "" {
			protocol = "tcp"
		}

		containerPort := nat.Port(fmt.Sprintf("%d/%s", portMapping.Container, protocol))
		exposedPorts[containerPort] = struct{}{}

		portBindings[containerPort] = []nat.PortBinding{
			{
				HostIP:   "0.0.0.0",
				HostPort: fmt.Sprintf("%d", hostPort),
			},
		}
	}

	config.ExposedPorts = exposedPorts

	// Build volume mounts
	mounts := []mount.Mount{}
	volumePaths := d.BuildVolumePaths(containerName, template.DockerConfig.Volumes)

	for _, vol := range volumePaths {
		mounts = append(mounts, mount.Mount{
			Type:   mount.TypeBind,
			Source: vol.HostPath,
			Target: vol.ContainerPath,
		})
	}

	// Create host config
	hostConfig := &container.HostConfig{
		PortBindings: portBindings,
		Mounts:       mounts,
		RestartPolicy: container.RestartPolicy{
			Name: container.RestartPolicyMode(template.DockerConfig.RestartPolicy),
		},
	}

	// Default to "unless-stopped" if not specified
	if hostConfig.RestartPolicy.Name == "" {
		hostConfig.RestartPolicy.Name = container.RestartPolicyUnlessStopped
	}

	return config, hostConfig
}

// CreateContainer creates and optionally starts a Docker container.
func (d *Deployer) CreateContainer(ctx context.Context, template *types.ServiceTemplate, deployment *types.ToolboxDeployment) (string, error) {
	containerName := d.GenerateContainerName(deployment.ServiceName)

	// Create container configuration
	config, hostConfig := d.CreateContainerConfig(template, containerName, deployment.EnvVars)

	// Create container
	resp, err := d.dockerClient.ContainerCreate(ctx, config, hostConfig, nil, nil, containerName)
	if err != nil {
		return "", fmt.Errorf("failed to create container: %w", err)
	}

	return resp.ID, nil
}

// StartContainer starts a Docker container.
func (d *Deployer) StartContainer(ctx context.Context, containerID string) error {
	if err := d.dockerClient.ContainerStart(ctx, containerID, container.StartOptions{}); err != nil {
		return fmt.Errorf("failed to start container: %w", err)
	}
	return nil
}

// StopContainer stops a Docker container.
func (d *Deployer) StopContainer(ctx context.Context, containerID string) error {
	// Use default timeout
	timeout := 10
	if err := d.dockerClient.ContainerStop(ctx, containerID, container.StopOptions{Timeout: &timeout}); err != nil {
		return fmt.Errorf("failed to stop container: %w", err)
	}
	return nil
}

// RemoveContainer removes a Docker container and optionally its volumes.
func (d *Deployer) RemoveContainer(ctx context.Context, containerID string, removeVolumes bool) error {
	options := container.RemoveOptions{
		RemoveVolumes: removeVolumes,
		Force:         true, // Force removal even if container is running
	}

	if err := d.dockerClient.ContainerRemove(ctx, containerID, options); err != nil {
		// Don't return error for non-existent containers (idempotent)
		if !client.IsErrNotFound(err) {
			return fmt.Errorf("failed to remove container: %w", err)
		}
	}

	return nil
}

// CreateContainerConfigWithCerts creates Docker container config with Nexus CA cert volume mounted.
// This enables TLS trust for service-to-service communication.
func (d *Deployer) CreateContainerConfigWithCerts(template *types.ServiceTemplate, containerName string, userVars map[string]string) (*container.Config, *container.HostConfig) {
	// Get base config
	config, hostConfig := d.CreateContainerConfig(template, containerName, userVars)

	// Add cert volume mount
	certMount := certvolume.GetCertMountConfig()
	hostConfig.Mounts = append(hostConfig.Mounts, certMount)

	// Add cert environment variables
	certEnv := certvolume.GetCertEnvironment()
	for key, value := range certEnv {
		config.Env = append(config.Env, fmt.Sprintf("%s=%s", key, value))
	}

	return config, hostConfig
}

// BuildComposeEnvironmentWithCerts builds environment variables including cert paths.
func (d *Deployer) BuildComposeEnvironmentWithCerts(template *types.ServiceTemplate, userVars map[string]string) map[string]string {
	// Get base environment
	env := d.BuildComposeEnvironment(template, userVars)

	// Add cert environment variables
	certEnv := certvolume.GetCertEnvironment()
	for key, value := range certEnv {
		env[key] = value
	}

	return env
}
