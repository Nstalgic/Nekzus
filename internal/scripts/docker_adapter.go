package scripts

import (
	"context"
	"io"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
)

// DockerClientAdapter adapts the Docker client to implement DockerClientForExecution.
type DockerClientAdapter struct {
	client *client.Client
}

// NewDockerClientAdapter creates a new Docker client adapter.
func NewDockerClientAdapter(cli *client.Client) *DockerClientAdapter {
	return &DockerClientAdapter{client: cli}
}

// ContainerCreate creates a Docker container.
func (a *DockerClientAdapter) ContainerCreate(ctx context.Context, config *container.Config, hostConfig *container.HostConfig, containerName string) (container.CreateResponse, error) {
	return a.client.ContainerCreate(ctx, config, hostConfig, nil, nil, containerName)
}

// ContainerStart starts a Docker container.
func (a *DockerClientAdapter) ContainerStart(ctx context.Context, containerID string, options container.StartOptions) error {
	return a.client.ContainerStart(ctx, containerID, options)
}

// ContainerWait waits for a Docker container to exit.
func (a *DockerClientAdapter) ContainerWait(ctx context.Context, containerID string, condition container.WaitCondition) (<-chan container.WaitResponse, <-chan error) {
	return a.client.ContainerWait(ctx, containerID, condition)
}

// ContainerLogs retrieves container logs.
func (a *DockerClientAdapter) ContainerLogs(ctx context.Context, containerID string, options container.LogsOptions) (io.ReadCloser, error) {
	return a.client.ContainerLogs(ctx, containerID, options)
}

// ContainerRemove removes a Docker container.
func (a *DockerClientAdapter) ContainerRemove(ctx context.Context, containerID string, options container.RemoveOptions) error {
	return a.client.ContainerRemove(ctx, containerID, options)
}

// ContainerStop stops a Docker container.
func (a *DockerClientAdapter) ContainerStop(ctx context.Context, containerID string, options container.StopOptions) error {
	return a.client.ContainerStop(ctx, containerID, options)
}

// ImagePull pulls a Docker image from a registry.
func (a *DockerClientAdapter) ImagePull(ctx context.Context, refStr string) (io.ReadCloser, error) {
	return a.client.ImagePull(ctx, refStr, image.PullOptions{})
}
