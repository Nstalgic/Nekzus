package toolbox

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	composecli "github.com/compose-spec/compose-go/v2/cli"
	composetypes "github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/nstalgic/nekzus/internal/types"
)

// LoadComposeFile loads a Docker Compose file and returns a parsed project.
// If envVars is provided, those variables are used for interpolation.
func (d *Deployer) LoadComposeFile(composePath string, envVars ...map[string]string) (*composetypes.Project, error) {
	// Get absolute path
	absPath, err := filepath.Abs(composePath)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve compose file path: %w", err)
	}

	// Check if file exists
	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("compose file not found: %s", absPath)
	}

	// Load project using compose-go
	projectOptions := composecli.ProjectOptions{
		ConfigPaths: []string{absPath},
		WorkingDir:  filepath.Dir(absPath),
	}

	// Add environment variables for interpolation if provided
	if len(envVars) > 0 && envVars[0] != nil {
		projectOptions.Environment = envVars[0]
	}

	project, err := composecli.ProjectFromOptions(context.Background(), &projectOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to load compose project: %w", err)
	}

	return project, nil
}

// BuildComposeEnvironment builds environment variables for Compose deployment.
func (d *Deployer) BuildComposeEnvironment(template *types.ServiceTemplate, userVars map[string]string) map[string]string {
	envMap := make(map[string]string)

	// Add user-provided environment variables
	for key, value := range userVars {
		envMap[key] = value
	}

	// Add default environment variables if not overridden
	if template.ComposeProject != nil {
		for _, service := range template.ComposeProject.Services {
			for key, value := range service.Environment {
				if value != nil {
					// Only add if not already in user vars
					if _, exists := envMap[key]; !exists {
						envMap[key] = *value
					}
				}
			}
		}
	}

	return envMap
}

// DeployComposeProject deploys a Docker Compose project using Docker API directly.
// Returns the project name on success.
func (d *Deployer) DeployComposeProject(ctx context.Context, template *types.ServiceTemplate, deployment *types.ToolboxDeployment) (string, error) {
	if template.ComposeFilePath == "" {
		return "", fmt.Errorf("template does not have a Compose file path")
	}

	// Use deployment ID as project name for uniqueness
	projectName := deployment.ID

	// Build environment variables from user input
	envVars := d.BuildComposeEnvironment(template, deployment.EnvVars)

	// Load compose file with environment variables for interpolation
	project, err := d.LoadComposeFile(template.ComposeFilePath, envVars)
	if err != nil {
		return "", fmt.Errorf("failed to reload compose file: %w", err)
	}

	// Create networks first (skip external networks)
	for networkName, networkConfig := range project.Networks {
		// Skip external networks - they should already exist
		if bool(networkConfig.External) {
			continue
		}

		fullNetworkName := fmt.Sprintf("%s_%s", projectName, networkName)

		_, err := d.dockerClient.NetworkCreate(ctx, fullNetworkName, network.CreateOptions{
			Driver: networkConfig.Driver,
			Labels: map[string]string{
				"com.docker.compose.project": projectName,
				"com.docker.compose.network": networkName,
			},
		})
		if err != nil && !strings.Contains(err.Error(), "already exists") {
			return "", fmt.Errorf("failed to create network %s: %w", fullNetworkName, err)
		}
	}

	// Pull images and create each service container
	for serviceName, service := range project.Services {
		containerName := fmt.Sprintf("%s_%s_1", projectName, serviceName)

		imageName := service.Image

		// Pull the image if it doesn't exist locally
		pullReader, err := d.dockerClient.ImagePull(ctx, imageName, image.PullOptions{})
		if err != nil {
			// Cleanup on error
			d.RemoveComposeProject(ctx, projectName, true)
			return "", fmt.Errorf("failed to pull image %s: %w", imageName, err)
		}
		// Consume the reader to complete the pull
		io.Copy(io.Discard, pullReader)
		pullReader.Close()

		// Build environment variables
		serviceEnv := []string{}
		for key, value := range service.Environment {
			if value != nil {
				serviceEnv = append(serviceEnv, fmt.Sprintf("%s=%s", key, *value))
			}
		}

		// Build port bindings
		portBindings := nat.PortMap{}
		exposedPorts := nat.PortSet{}
		for _, portConfig := range service.Ports {
			containerPort := portConfig.Target
			hostPort := portConfig.Published

			protocol := portConfig.Protocol
			if protocol == "" {
				protocol = "tcp"
			}

			natPort := nat.Port(fmt.Sprintf("%d/%s", containerPort, protocol))
			exposedPorts[natPort] = struct{}{}
			portBindings[natPort] = []nat.PortBinding{
				{
					HostIP:   "0.0.0.0",
					HostPort: hostPort,
				},
			}
		}

		// Build network connections
		networkConfig := &network.NetworkingConfig{
			EndpointsConfig: map[string]*network.EndpointSettings{},
		}
		for networkName := range service.Networks {
			// Check if network is external
			if netDef, exists := project.Networks[networkName]; exists && bool(netDef.External) {
				// Use actual network name for external networks
				// Add network alias using the service name for DNS resolution
				networkConfig.EndpointsConfig[networkName] = &network.EndpointSettings{
					Aliases: []string{serviceName},
				}
			} else {
				// Use prefixed name for project-specific networks
				fullNetworkName := fmt.Sprintf("%s_%s", projectName, networkName)
				networkConfig.EndpointsConfig[fullNetworkName] = &network.EndpointSettings{
					Aliases: []string{serviceName},
				}
			}
		}

		// Build labels - merge Compose file labels with injected compose metadata
		labels := make(map[string]string)

		// Copy labels from Compose file (includes toolbox and discovery labels)
		for key, value := range service.Labels {
			labels[key] = value
		}

		// Add compose metadata labels
		labels["com.docker.compose.project"] = projectName
		labels["com.docker.compose.service"] = serviceName

		// Create container config
		config := &container.Config{
			Image:        imageName,
			Env:          serviceEnv,
			ExposedPorts: exposedPorts,
			Labels:       labels,
		}

		// Create host config
		hostConfig := &container.HostConfig{
			PortBindings: portBindings,
			RestartPolicy: container.RestartPolicy{
				Name: container.RestartPolicyMode(service.Restart),
			},
		}

		// Default restart policy
		if hostConfig.RestartPolicy.Name == "" {
			hostConfig.RestartPolicy.Name = container.RestartPolicyUnlessStopped
		}

		// Create container
		_, err = d.dockerClient.ContainerCreate(ctx, config, hostConfig, networkConfig, nil, containerName)
		if err != nil {
			// Cleanup on error
			d.RemoveComposeProject(ctx, projectName, true)
			return "", fmt.Errorf("failed to create container %s: %w", containerName, err)
		}
	}

	return projectName, nil
}

// StartComposeProject starts all services in a Compose project.
func (d *Deployer) StartComposeProject(ctx context.Context, projectName string) error {
	// Find all containers for this project
	filterArgs := filters.NewArgs()
	filterArgs.Add("label", fmt.Sprintf("com.docker.compose.project=%s", projectName))

	containers, err := d.dockerClient.ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: filterArgs,
	})
	if err != nil {
		return fmt.Errorf("failed to list containers: %w", err)
	}

	// Start each container
	for _, ctr := range containers {
		if err := d.dockerClient.ContainerStart(ctx, ctr.ID, container.StartOptions{}); err != nil {
			return fmt.Errorf("failed to start container %s: %w", ctr.Names[0], err)
		}
	}

	return nil
}

// StopComposeProject stops all services in a Compose project.
func (d *Deployer) StopComposeProject(ctx context.Context, projectName string) error {
	// Find all containers for this project
	filterArgs := filters.NewArgs()
	filterArgs.Add("label", fmt.Sprintf("com.docker.compose.project=%s", projectName))

	containers, err := d.dockerClient.ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: filterArgs,
	})
	if err != nil {
		return fmt.Errorf("failed to list containers: %w", err)
	}

	// Stop each container
	timeout := 10
	for _, ctr := range containers {
		if err := d.dockerClient.ContainerStop(ctx, ctr.ID, container.StopOptions{Timeout: &timeout}); err != nil {
			if !client.IsErrNotFound(err) {
				return fmt.Errorf("failed to stop container %s: %w", ctr.Names[0], err)
			}
		}
	}

	return nil
}

// RemoveComposeProject removes a Compose project and optionally its volumes.
func (d *Deployer) RemoveComposeProject(ctx context.Context, projectName string, removeVolumes bool) error {
	// Find all containers for this project
	filterArgs := filters.NewArgs()
	filterArgs.Add("label", fmt.Sprintf("com.docker.compose.project=%s", projectName))

	containers, err := d.dockerClient.ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: filterArgs,
	})
	if err != nil {
		return fmt.Errorf("failed to list containers: %w", err)
	}

	// Remove each container
	for _, ctr := range containers {
		removeOptions := container.RemoveOptions{
			RemoveVolumes: removeVolumes,
			Force:         true,
		}
		if err := d.dockerClient.ContainerRemove(ctx, ctr.ID, removeOptions); err != nil {
			if !client.IsErrNotFound(err) {
				return fmt.Errorf("failed to remove container %s: %w", ctr.Names[0], err)
			}
		}
	}

	// Remove networks
	networkFilterArgs := filters.NewArgs()
	networkFilterArgs.Add("label", fmt.Sprintf("com.docker.compose.project=%s", projectName))

	networks, err := d.dockerClient.NetworkList(ctx, network.ListOptions{
		Filters: networkFilterArgs,
	})
	if err == nil {
		for _, net := range networks {
			if err := d.dockerClient.NetworkRemove(ctx, net.ID); err != nil {
				if !strings.Contains(err.Error(), "not found") && !strings.Contains(err.Error(), "No such") {
					fmt.Printf("Warning: failed to remove network %s: %v\n", net.Name, err)
				}
			}
		}
	}

	return nil
}
