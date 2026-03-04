package handlers

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	apperrors "github.com/nstalgic/nekzus/internal/errors"
	"github.com/nstalgic/nekzus/internal/httputil"
	"github.com/nstalgic/nekzus/internal/middleware"
	"github.com/nstalgic/nekzus/internal/runtime"
	"github.com/nstalgic/nekzus/internal/storage"
	apptypes "github.com/nstalgic/nekzus/internal/types"
)

var containerlog = slog.With("package", "handlers")

// Timeout validation constants
const (
	// minTimeout is the minimum allowed timeout in seconds
	minTimeout = 1
	// maxTimeout is the maximum allowed timeout in seconds
	maxTimeout = 300
)

// DockerClient defines the interface for Docker operations
type DockerClient interface {
	ContainerList(ctx context.Context, options container.ListOptions) ([]types.Container, error)
	ContainerStart(ctx context.Context, containerID string, options container.StartOptions) error
	ContainerStop(ctx context.Context, containerID string, options container.StopOptions) error
	ContainerRestart(ctx context.Context, containerID string, options container.StopOptions) error
	ContainerInspect(ctx context.Context, containerID string) (types.ContainerJSON, error)
	ContainerStats(ctx context.Context, containerID string, stream bool) (container.StatsResponseReader, error)
	ContainerLogs(ctx context.Context, containerID string, options container.LogsOptions) (io.ReadCloser, error)
}

// ContainerNotifier defines the interface for sending container operation notifications
type ContainerNotifier interface {
	SendToDevice(deviceID string, message interface{}) error
}

// ServiceHealthNotifier defines the interface for notifying about service health changes
type ServiceHealthNotifier interface {
	MarkAppUnhealthy(appID, reason string)
}

// ContainerHandler handles container management endpoints
type ContainerHandler struct {
	client         DockerClient      // Legacy Docker client (for backward compatibility)
	runtimes       *runtime.Registry // Runtime registry (preferred)
	storage        *storage.Store
	notifier       ContainerNotifier
	healthNotifier ServiceHealthNotifier // Notifies about service health changes on container stop
}

// NewContainerHandler creates a new container handler with legacy Docker client
func NewContainerHandler(client DockerClient, store *storage.Store) *ContainerHandler {
	return &ContainerHandler{
		client:  client,
		storage: store,
	}
}

// NewContainerHandlerWithRuntime creates a new container handler with runtime registry
func NewContainerHandlerWithRuntime(runtimes *runtime.Registry, store *storage.Store) *ContainerHandler {
	return &ContainerHandler{
		runtimes: runtimes,
		storage:  store,
	}
}

// GetRuntimeRegistry returns the runtime registry (may be nil if using legacy Docker client)
func (h *ContainerHandler) GetRuntimeRegistry() *runtime.Registry {
	return h.runtimes
}

// getPrimaryRuntime returns the primary runtime if available
func (h *ContainerHandler) getPrimaryRuntime() runtime.Runtime {
	if h.runtimes == nil {
		return nil
	}
	return h.runtimes.GetPrimary()
}

// hasRuntime checks if a runtime registry is configured
func (h *ContainerHandler) hasRuntime() bool {
	return h.runtimes != nil && h.runtimes.GetPrimary() != nil
}

// SetNotifier sets the WebSocket notifier for async operation callbacks
func (h *ContainerHandler) SetNotifier(notifier ContainerNotifier) {
	h.notifier = notifier
}

// SetHealthNotifier sets the health notifier for immediate health status updates on container stop
func (h *ContainerHandler) SetHealthNotifier(notifier ServiceHealthNotifier) {
	h.healthNotifier = notifier
}

// ContainerListResponse represents a container in the list response
type ContainerListResponse struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Image     string            `json:"image"`
	State     string            `json:"state"`
	Status    string            `json:"status"`
	Created   int64             `json:"created"`
	Ports     []PortBinding     `json:"ports"`
	Labels    map[string]string `json:"labels"`
	Runtime   string            `json:"runtime,omitempty"`   // Runtime type (docker, kubernetes)
	Namespace string            `json:"namespace,omitempty"` // K8s namespace
}

// PortBinding represents a port mapping
type PortBinding struct {
	IP          string `json:"ip,omitempty"`
	PrivatePort int    `json:"privatePort"`
	PublicPort  int    `json:"publicPort,omitempty"`
	Type        string `json:"type"`
}

// StatsJSON represents container resource usage statistics
type StatsJSON struct {
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

// HandleListContainers lists all containers from configured runtimes
// GET /api/v1/containers
// Query params:
//   - runtime: filter by runtime (docker, kubernetes)
//   - namespace: K8s namespace filter
func (h *ContainerHandler) HandleListContainers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		apperrors.WriteJSON(w, apperrors.New("METHOD_NOT_ALLOWED", "Method not allowed", http.StatusMethodNotAllowed))
		return
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	// Use runtime interface if available, otherwise fall back to legacy Docker client
	if h.hasRuntime() {
		h.listContainersWithRuntime(ctx, w, r)
		return
	}

	// Legacy Docker client path
	h.listContainersWithDockerClient(ctx, w)
}

// listContainersWithRuntime lists containers using the runtime interface
func (h *ContainerHandler) listContainersWithRuntime(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	// Parse query params
	runtimeFilter := r.URL.Query().Get("runtime")
	namespaceFilter := r.URL.Query().Get("namespace")

	// Get list options
	opts := runtime.ListOptions{
		All:       true,
		Namespace: namespaceFilter,
	}

	var allContainers []runtime.Container

	// If runtime filter specified, only query that runtime
	if runtimeFilter != "" {
		rt, err := h.runtimes.Get(runtime.RuntimeType(runtimeFilter))
		if err != nil {
			apperrors.WriteJSON(w, apperrors.New("INVALID_RUNTIME", "Invalid runtime specified", http.StatusBadRequest))
			return
		}
		containers, err := rt.List(ctx, opts)
		if err != nil {
			containerlog.Error("Error listing containers", "runtime", runtimeFilter, "error", err)
			apperrors.WriteJSON(w, apperrors.Wrap(err, "CONTAINER_LIST_FAILED", "failed to list containers", http.StatusInternalServerError))
			return
		}
		allContainers = containers
	} else {
		// Query all registered runtimes
		for _, rtType := range h.runtimes.Available() {
			rt, err := h.runtimes.Get(rtType)
			if err != nil {
				continue
			}
			containers, err := rt.List(ctx, opts)
			if err != nil {
				containerlog.Warn("Error listing containers from runtime", "runtime", rtType, "error", err)
				continue
			}
			allContainers = append(allContainers, containers...)
		}
	}

	// Transform containers to response format
	response := make([]ContainerListResponse, 0, len(allContainers))
	for _, c := range allContainers {
		// Transform ports
		ports := make([]PortBinding, 0, len(c.Ports))
		for _, p := range c.Ports {
			ports = append(ports, PortBinding{
				IP:          p.IP,
				PrivatePort: p.PrivatePort,
				PublicPort:  p.PublicPort,
				Type:        p.Protocol,
			})
		}

		response = append(response, ContainerListResponse{
			ID:        c.ID.ShortID(),
			Name:      c.Name,
			Image:     c.Image,
			State:     string(c.State),
			Status:    c.Status,
			Created:   c.Created,
			Ports:     ports,
			Labels:    c.Labels,
			Runtime:   string(c.ID.Runtime),
			Namespace: c.ID.Namespace,
		})
	}

	containerlog.Info("Listed containers", "count", len(response))

	if err := httputil.WriteJSON(w, http.StatusOK, response); err != nil {
		containerlog.Error("Error encoding JSON response", "error", err)
	}
}

// listContainersWithDockerClient lists containers using the legacy Docker client
func (h *ContainerHandler) listContainersWithDockerClient(ctx context.Context, w http.ResponseWriter) {
	// List all containers (running and stopped)
	containers, err := h.client.ContainerList(ctx, container.ListOptions{
		All: true,
	})
	if err != nil {
		containerlog.Error("Error listing containers", "error", err)
		apperrors.WriteJSON(w, apperrors.Wrap(
			err,
			"CONTAINER_LIST_FAILED",
			"failed to list containers",
			http.StatusInternalServerError,
		))
		return
	}

	// Transform to response format
	response := make([]ContainerListResponse, 0, len(containers))
	for _, c := range containers {
		// Get short ID for display (max 12 chars)
		shortID := c.ID
		if len(shortID) > 12 {
			shortID = shortID[:12]
		}

		// Get container name (remove leading slash)
		name := "unknown"
		if len(c.Names) > 0 {
			name = strings.TrimPrefix(c.Names[0], "/")
		}

		// Transform ports
		ports := make([]PortBinding, 0, len(c.Ports))
		for _, p := range c.Ports {
			ports = append(ports, PortBinding{
				IP:          p.IP,
				PrivatePort: int(p.PrivatePort),
				PublicPort:  int(p.PublicPort),
				Type:        p.Type,
			})
		}

		response = append(response, ContainerListResponse{
			ID:      shortID,
			Name:    name,
			Image:   c.Image,
			State:   c.State,
			Status:  c.Status,
			Created: c.Created,
			Ports:   ports,
			Labels:  c.Labels,
			Runtime: string(runtime.RuntimeDocker),
		})
	}

	containerlog.Info("Listed containers", "count", len(response))

	if err := httputil.WriteJSON(w, http.StatusOK, response); err != nil {
		containerlog.Error("Error encoding JSON response", "error", err)
	}
}

// HandleStartContainer starts a container asynchronously
// POST /api/v1/containers/{containerId}/start
// POST /api/v1/containers/{namespace}/{pod}/start (K8s)
// Query params: runtime=docker|kubernetes
// Returns 202 Accepted immediately, sends WebSocket notification on completion
func (h *ContainerHandler) HandleStartContainer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		apperrors.WriteJSON(w, apperrors.New("METHOD_NOT_ALLOWED", "Method not allowed", http.StatusMethodNotAllowed))
		return
	}

	// Parse container ID and optional runtime/namespace
	containerID, runtimeType, namespace := h.parseContainerRequest(r)
	if containerID == "" {
		apperrors.WriteJSON(w, apperrors.New("INVALID_CONTAINER_ID", "Container ID required", http.StatusBadRequest))
		return
	}

	// Get device ID from context for notification callback
	deviceID := middleware.GetDeviceIDFromContext(r.Context())

	// Build response with runtime info if specified
	response := map[string]interface{}{
		"status":      "accepted",
		"containerId": containerID,
		"message":     "Container start initiated",
		"timestamp":   time.Now().Unix(),
	}
	if runtimeType != "" {
		response["runtime"] = runtimeType
	}
	if namespace != "" {
		response["namespace"] = namespace
	}

	// Return 202 Accepted immediately
	if err := httputil.WriteJSON(w, http.StatusAccepted, response); err != nil {
		containerlog.Error("Error encoding JSON response", "error", err)
		return
	}

	// Execute start in background
	go h.executeContainerStart(containerID, deviceID, runtimeType, namespace)
}

// executeContainerStart performs the container start and sends notification
func (h *ContainerHandler) executeContainerStart(containerID string, deviceID string, runtimeType string, namespace string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var err error

	// Use runtime interface if available and runtime is specified
	if h.hasRuntime() && runtimeType != "" {
		rt, rtErr := h.runtimes.Get(runtime.RuntimeType(runtimeType))
		if rtErr != nil {
			err = rtErr
		} else {
			id := runtime.ContainerID{
				Runtime:   runtime.RuntimeType(runtimeType),
				ID:        containerID,
				Namespace: namespace,
			}
			err = rt.Start(ctx, id)
		}
	} else if h.hasRuntime() {
		// Use primary runtime
		rt := h.getPrimaryRuntime()
		id := runtime.ContainerID{
			Runtime:   rt.Type(),
			ID:        containerID,
			Namespace: namespace,
		}
		err = rt.Start(ctx, id)
	} else if h.client != nil {
		// Legacy Docker client path
		err = h.client.ContainerStart(ctx, containerID, container.StartOptions{})
	} else {
		err = apperrors.New("NO_RUNTIME", "No runtime configured", http.StatusInternalServerError)
	}

	var notification apptypes.WebSocketMessage
	if err != nil {
		containerlog.Error("Error starting container", "container_id", containerID, "error", err)

		errorCode := "CONTAINER_START_FAILED"
		errorMsg := "Failed to start container"
		if isContainerNotFoundError(err) {
			errorCode = "CONTAINER_NOT_FOUND"
			errorMsg = "Container not found"
		} else if strings.Contains(err.Error(), "already started") {
			errorCode = "CONTAINER_ALREADY_STARTED"
			errorMsg = "Container already started"
		}

		notification = apptypes.WebSocketMessage{
			Type: "container.start.failed",
			Data: map[string]interface{}{
				"containerId": containerID,
				"error":       errorCode,
				"message":     errorMsg,
				"timestamp":   time.Now().Unix(),
			},
		}
	} else {
		containerlog.Info("Started container", "container_id", containerID)

		notification = apptypes.WebSocketMessage{
			Type: "container.start.completed",
			Data: map[string]interface{}{
				"containerId": containerID,
				"status":      "started",
				"message":     "Container started successfully",
				"timestamp":   time.Now().Unix(),
			},
		}
	}

	// Send notification to requesting device
	if h.notifier != nil && deviceID != "" {
		if err := h.notifier.SendToDevice(deviceID, notification); err != nil {
			containerlog.Warn("Failed to send start notification",
				"device_id", deviceID,
				"container_id", containerID,
				"error", err)
		} else {
			containerlog.Info("Sent start notification",
				"device_id", deviceID,
				"container_id", containerID,
				"type", notification.Type)
		}
	} else {
		containerlog.Warn("Cannot send start notification",
			"notifier_nil", h.notifier == nil,
			"device_id_empty", deviceID == "",
			"container_id", containerID)
	}
}

// HandleStopContainer stops a container asynchronously
// POST /api/v1/containers/{containerId}/stop?timeout=10
// POST /api/v1/containers/{namespace}/{pod}/stop (K8s)
// Query params: runtime=docker|kubernetes, timeout=<seconds>
// Returns 202 Accepted immediately, sends WebSocket notification on completion
func (h *ContainerHandler) HandleStopContainer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		apperrors.WriteJSON(w, apperrors.New("METHOD_NOT_ALLOWED", "Method not allowed", http.StatusMethodNotAllowed))
		return
	}

	// Parse container ID and optional runtime/namespace
	containerID, runtimeType, namespace := h.parseContainerRequest(r)
	if containerID == "" {
		apperrors.WriteJSON(w, apperrors.New("INVALID_CONTAINER_ID", "Container ID required", http.StatusBadRequest))
		return
	}

	// Parse optional timeout parameter with validation
	var timeout *int
	if timeoutStr := r.URL.Query().Get("timeout"); timeoutStr != "" {
		if t, err := strconv.Atoi(timeoutStr); err == nil {
			// Validate timeout bounds
			if t < minTimeout || t > maxTimeout {
				apperrors.WriteJSON(w, apperrors.New("INVALID_TIMEOUT",
					"timeout must be between 1 and 300 seconds",
					http.StatusBadRequest))
				return
			}
			timeout = &t
		}
	}

	// Get device ID from context for notification callback
	deviceID := middleware.GetDeviceIDFromContext(r.Context())

	// Build response with runtime info if specified
	response := map[string]interface{}{
		"status":      "accepted",
		"containerId": containerID,
		"message":     "Container stop initiated",
		"timestamp":   time.Now().Unix(),
	}
	if timeout != nil {
		response["timeout"] = *timeout
	}
	if runtimeType != "" {
		response["runtime"] = runtimeType
	}
	if namespace != "" {
		response["namespace"] = namespace
	}

	if err := httputil.WriteJSON(w, http.StatusAccepted, response); err != nil {
		containerlog.Error("Error encoding JSON response", "error", err)
		return
	}

	// Execute stop in background
	go h.executeContainerStop(containerID, timeout, deviceID, runtimeType, namespace)
}

// executeContainerStop performs the container stop and sends notification
func (h *ContainerHandler) executeContainerStop(containerID string, timeout *int, deviceID string, runtimeType string, namespace string) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	var err error
	var appID string // App ID from container labels for health notification

	// Convert timeout to time.Duration for runtime interface
	var timeoutDuration *time.Duration
	if timeout != nil {
		d := time.Duration(*timeout) * time.Second
		timeoutDuration = &d
	}

	// Use runtime interface if available and runtime is specified
	if h.hasRuntime() && runtimeType != "" {
		rt, rtErr := h.runtimes.Get(runtime.RuntimeType(runtimeType))
		if rtErr != nil {
			err = rtErr
		} else {
			id := runtime.ContainerID{
				Runtime:   runtime.RuntimeType(runtimeType),
				ID:        containerID,
				Namespace: namespace,
			}
			// Get app ID from container labels before stopping
			appID = h.getAppIDFromContainer(ctx, rt, id)
			err = rt.Stop(ctx, id, timeoutDuration)
		}
	} else if h.hasRuntime() {
		// Use primary runtime
		rt := h.getPrimaryRuntime()
		id := runtime.ContainerID{
			Runtime:   rt.Type(),
			ID:        containerID,
			Namespace: namespace,
		}
		// Get app ID from container labels before stopping
		appID = h.getAppIDFromContainer(ctx, rt, id)
		err = rt.Stop(ctx, id, timeoutDuration)
	} else if h.client != nil {
		// Legacy Docker client path
		// Get app ID from container labels before stopping
		if info, inspectErr := h.client.ContainerInspect(ctx, containerID); inspectErr == nil && info.Config != nil {
			appID = info.Config.Labels["nekzus.app.id"]
		}
		stopOptions := container.StopOptions{
			Timeout: timeout,
		}
		err = h.client.ContainerStop(ctx, containerID, stopOptions)
	} else {
		err = apperrors.New("NO_RUNTIME", "No runtime configured", http.StatusInternalServerError)
	}

	var notification apptypes.WebSocketMessage
	if err != nil {
		containerlog.Error("Error stopping container", "container_id", containerID, "error", err)

		errorCode := "CONTAINER_STOP_FAILED"
		errorMsg := "Failed to stop container"
		if isContainerNotFoundError(err) {
			errorCode = "CONTAINER_NOT_FOUND"
			errorMsg = "Container not found"
		} else if strings.Contains(err.Error(), "already stopped") {
			errorCode = "CONTAINER_ALREADY_STOPPED"
			errorMsg = "Container already stopped"
		}

		notification = apptypes.WebSocketMessage{
			Type: "container.stop.failed",
			Data: map[string]interface{}{
				"containerId": containerID,
				"error":       errorCode,
				"message":     errorMsg,
				"timestamp":   time.Now().Unix(),
			},
		}
	} else {
		containerlog.Info("Stopped container", "container_id", containerID)

		notification = apptypes.WebSocketMessage{
			Type: "container.stop.completed",
			Data: map[string]interface{}{
				"containerId": containerID,
				"status":      "stopped",
				"message":     "Container stopped successfully",
				"timestamp":   time.Now().Unix(),
			},
		}

		// Immediately mark app as unhealthy for faster mobile notification
		if h.healthNotifier != nil && appID != "" {
			h.healthNotifier.MarkAppUnhealthy(appID, "Container stopped")
			containerlog.Info("Marked app unhealthy after container stop",
				"container_id", containerID,
				"app_id", appID)
		}
	}

	// Send notification to requesting device
	if h.notifier != nil && deviceID != "" {
		if err := h.notifier.SendToDevice(deviceID, notification); err != nil {
			containerlog.Warn("Failed to send stop notification",
				"device_id", deviceID,
				"container_id", containerID,
				"error", err)
		} else {
			containerlog.Info("Sent stop notification",
				"device_id", deviceID,
				"container_id", containerID,
				"type", notification.Type)
		}
	} else {
		containerlog.Warn("Cannot send stop notification",
			"notifier_nil", h.notifier == nil,
			"device_id_empty", deviceID == "",
			"container_id", containerID)
	}
}

// HandleRestartContainer restarts a container asynchronously
// POST /api/v1/containers/{containerId}/restart?timeout=10
// POST /api/v1/containers/{namespace}/{pod}/restart (K8s)
// Query params: runtime=docker|kubernetes, timeout=<seconds>
// Returns 202 Accepted immediately, sends WebSocket notification on completion
func (h *ContainerHandler) HandleRestartContainer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		apperrors.WriteJSON(w, apperrors.New("METHOD_NOT_ALLOWED", "Method not allowed", http.StatusMethodNotAllowed))
		return
	}

	// Parse container ID and optional runtime/namespace
	containerID, runtimeType, namespace := h.parseContainerRequest(r)
	if containerID == "" {
		apperrors.WriteJSON(w, apperrors.New("INVALID_CONTAINER_ID", "Container ID required", http.StatusBadRequest))
		return
	}

	// Parse optional timeout parameter with validation
	var timeout *int
	if timeoutStr := r.URL.Query().Get("timeout"); timeoutStr != "" {
		if t, err := strconv.Atoi(timeoutStr); err == nil {
			// Validate timeout bounds
			if t < minTimeout || t > maxTimeout {
				apperrors.WriteJSON(w, apperrors.New("INVALID_TIMEOUT",
					"timeout must be between 1 and 300 seconds",
					http.StatusBadRequest))
				return
			}
			timeout = &t
		}
	}

	// Get device ID from context for notification callback
	deviceID := middleware.GetDeviceIDFromContext(r.Context())

	// Build response with runtime info if specified
	response := map[string]interface{}{
		"status":      "accepted",
		"containerId": containerID,
		"message":     "Container restart initiated",
		"timestamp":   time.Now().Unix(),
	}
	if timeout != nil {
		response["timeout"] = *timeout
	}
	if runtimeType != "" {
		response["runtime"] = runtimeType
	}
	if namespace != "" {
		response["namespace"] = namespace
	}

	if err := httputil.WriteJSON(w, http.StatusAccepted, response); err != nil {
		containerlog.Error("Error encoding JSON response", "error", err)
		return
	}

	// Execute restart in background
	go h.executeContainerRestart(containerID, timeout, deviceID, runtimeType, namespace)
}

// executeContainerRestart performs the container restart and sends notification
func (h *ContainerHandler) executeContainerRestart(containerID string, timeout *int, deviceID string, runtimeType string, namespace string) {
	containerlog.Info("Executing container restart in background",
		"container_id", containerID,
		"device_id", deviceID,
		"has_notifier", h.notifier != nil)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	var err error

	// Convert timeout to time.Duration for runtime interface
	var timeoutDuration *time.Duration
	if timeout != nil {
		d := time.Duration(*timeout) * time.Second
		timeoutDuration = &d
	}

	// Use runtime interface if available and runtime is specified
	if h.hasRuntime() && runtimeType != "" {
		rt, rtErr := h.runtimes.Get(runtime.RuntimeType(runtimeType))
		if rtErr != nil {
			err = rtErr
		} else {
			id := runtime.ContainerID{
				Runtime:   runtime.RuntimeType(runtimeType),
				ID:        containerID,
				Namespace: namespace,
			}
			err = rt.Restart(ctx, id, timeoutDuration)
		}
	} else if h.hasRuntime() {
		// Use primary runtime
		rt := h.getPrimaryRuntime()
		id := runtime.ContainerID{
			Runtime:   rt.Type(),
			ID:        containerID,
			Namespace: namespace,
		}
		err = rt.Restart(ctx, id, timeoutDuration)
	} else if h.client != nil {
		// Legacy Docker client path
		stopOptions := container.StopOptions{
			Timeout: timeout,
		}
		err = h.client.ContainerRestart(ctx, containerID, stopOptions)
	} else {
		err = apperrors.New("NO_RUNTIME", "No runtime configured", http.StatusInternalServerError)
	}

	var notification apptypes.WebSocketMessage
	if err != nil {
		containerlog.Error("Error restarting container", "container_id", containerID, "error", err)

		errorCode := "CONTAINER_RESTART_FAILED"
		errorMsg := "Failed to restart container"
		if isContainerNotFoundError(err) {
			errorCode = "CONTAINER_NOT_FOUND"
			errorMsg = "Container not found"
		}

		notification = apptypes.WebSocketMessage{
			Type: "container.restart.failed",
			Data: map[string]interface{}{
				"containerId": containerID,
				"error":       errorCode,
				"message":     errorMsg,
				"timestamp":   time.Now().Unix(),
			},
		}
	} else {
		containerlog.Info("Restarted container", "container_id", containerID)

		notification = apptypes.WebSocketMessage{
			Type: "container.restart.completed",
			Data: map[string]interface{}{
				"containerId": containerID,
				"status":      "restarted",
				"message":     "Container restarted successfully",
				"timestamp":   time.Now().Unix(),
			},
		}
	}

	// Send notification to requesting device
	if h.notifier != nil && deviceID != "" {
		if err := h.notifier.SendToDevice(deviceID, notification); err != nil {
			containerlog.Warn("Failed to send restart notification",
				"device_id", deviceID,
				"container_id", containerID,
				"error", err)
		} else {
			containerlog.Info("Sent restart notification",
				"device_id", deviceID,
				"container_id", containerID,
				"type", notification.Type)
		}
	} else {
		containerlog.Warn("Cannot send restart notification",
			"notifier_nil", h.notifier == nil,
			"device_id_empty", deviceID == "",
			"container_id", containerID)
	}
}

// HandleInspectContainer returns detailed information about a container
// GET /api/v1/containers/{containerId}
// GET /api/v1/containers/{namespace}/{pod} (K8s)
// Query params: runtime=docker|kubernetes
func (h *ContainerHandler) HandleInspectContainer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		apperrors.WriteJSON(w, apperrors.New("METHOD_NOT_ALLOWED", "Method not allowed", http.StatusMethodNotAllowed))
		return
	}

	// Parse container ID and optional runtime/namespace
	containerID, runtimeType, namespace := h.parseContainerRequest(r)
	if containerID == "" {
		apperrors.WriteJSON(w, apperrors.New("INVALID_CONTAINER_ID", "Container ID required", http.StatusBadRequest))
		return
	}

	// Check if runtime is available
	if !h.hasRuntime() {
		apperrors.WriteJSON(w, apperrors.New("RUNTIME_UNAVAILABLE", "Container runtime not available", http.StatusServiceUnavailable))
		return
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	// Get the appropriate runtime
	var rt runtime.Runtime
	if runtimeType != "" {
		var err error
		rt, err = h.runtimes.Get(runtime.RuntimeType(runtimeType))
		if err != nil {
			apperrors.WriteJSON(w, apperrors.New("INVALID_RUNTIME", "Invalid runtime specified", http.StatusBadRequest))
			return
		}
	} else {
		rt = h.getPrimaryRuntime()
	}

	// Build ContainerID with runtime info
	id := runtime.ContainerID{
		Runtime:   rt.Type(),
		ID:        containerID,
		Namespace: namespace,
	}

	// Use runtime abstraction to inspect container
	details, err := rt.Inspect(ctx, id)
	if err != nil {
		containerlog.Error("Error inspecting container", "container_id", containerID, "runtime", rt.Type(), "error", err)

		if isContainerNotFoundError(err) {
			apperrors.WriteJSON(w, apperrors.New(
				"CONTAINER_NOT_FOUND",
				"container not found",
				http.StatusNotFound,
			))
			return
		}

		apperrors.WriteJSON(w, apperrors.Wrap(
			err,
			"CONTAINER_INSPECT_FAILED",
			"failed to inspect container",
			http.StatusInternalServerError,
		))
		return
	}

	containerlog.Info("Inspected container", "container_id", containerID, "runtime", rt.Type())

	// For Docker, return the raw Docker API format for backward compatibility
	// For other runtimes, return the normalized ContainerDetails format
	if rt.Type() == runtime.RuntimeDocker && details.Raw != nil {
		if err := httputil.WriteJSON(w, http.StatusOK, details.Raw); err != nil {
			containerlog.Error("Error encoding JSON response", "error", err)
		}
		return
	}

	// Return normalized format
	if err := httputil.WriteJSON(w, http.StatusOK, details); err != nil {
		containerlog.Error("Error encoding JSON response", "error", err)
	}
}

// ContainerStatsResponse represents container resource usage statistics
type ContainerStatsResponse struct {
	ContainerID string `json:"containerId"`
	CPU         struct {
		Usage      float64 `json:"usage"`
		CoresUsed  float64 `json:"coresUsed"`
		TotalCores float64 `json:"totalCores"`
	} `json:"cpu"`
	Memory struct {
		Usage     float64 `json:"usage"`
		Used      uint64  `json:"used"`
		Limit     uint64  `json:"limit"`
		Available uint64  `json:"available"`
	} `json:"memory"`
	Network struct {
		Rx uint64 `json:"rx"`
		Tx uint64 `json:"tx"`
	} `json:"network"`
	Timestamp int64 `json:"timestamp"`
}

// BatchContainerStatsResponse represents stats for multiple containers
type BatchContainerStatsResponse struct {
	Containers []ContainerStatsResponse `json:"containers"`
	Timestamp  int64                    `json:"timestamp"`
}

// HandleContainerStats returns resource usage statistics for a container
// GET /api/v1/containers/{containerId}/stats
// GET /api/v1/containers/{namespace}/{pod}/stats (K8s)
// Query params: runtime=docker|kubernetes
func (h *ContainerHandler) HandleContainerStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		apperrors.WriteJSON(w, apperrors.New("METHOD_NOT_ALLOWED", "Method not allowed", http.StatusMethodNotAllowed))
		return
	}

	// Parse container ID and optional runtime/namespace
	containerID, runtimeType, namespace := h.parseContainerRequest(r)
	if containerID == "" {
		apperrors.WriteJSON(w, apperrors.New("INVALID_CONTAINER_ID", "Container ID required", http.StatusBadRequest))
		return
	}

	// Check if runtime is available
	if !h.hasRuntime() {
		apperrors.WriteJSON(w, apperrors.New("RUNTIME_UNAVAILABLE", "Container runtime not available", http.StatusServiceUnavailable))
		return
	}

	var stats *ContainerStatsResponse
	var err error

	// Use runtime interface
	if runtimeType != "" {
		stats, err = h.getContainerStatsWithRuntime(r.Context(), containerID, runtimeType, namespace)
	} else {
		// Use primary runtime
		rt := h.getPrimaryRuntime()
		stats, err = h.getContainerStatsWithRuntime(r.Context(), containerID, string(rt.Type()), namespace)
	}

	if err != nil {
		if isContainerNotFoundError(err) {
			apperrors.WriteJSON(w, apperrors.New(
				"CONTAINER_NOT_FOUND",
				"container not found",
				http.StatusNotFound,
			))
			return
		}
		apperrors.WriteJSON(w, apperrors.Wrap(
			err,
			"CONTAINER_STATS_FAILED",
			"failed to get container stats",
			http.StatusInternalServerError,
		))
		return
	}

	if err := httputil.WriteJSON(w, http.StatusOK, stats); err != nil {
		containerlog.Error("Error encoding JSON response", "error", err)
	}
}

// HandleBatchContainerStats returns resource usage statistics for all running containers
// GET /api/v1/containers/stats
func (h *ContainerHandler) HandleBatchContainerStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		apperrors.WriteJSON(w, apperrors.New("METHOD_NOT_ALLOWED", "Method not allowed", http.StatusMethodNotAllowed))
		return
	}

	// Check if runtime is available
	if !h.hasRuntime() {
		apperrors.WriteJSON(w, apperrors.New("RUNTIME_UNAVAILABLE", "Container runtime not available", http.StatusServiceUnavailable))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	// Get primary runtime
	rt := h.getPrimaryRuntime()

	// List all containers using runtime abstraction
	containers, err := rt.List(ctx, runtime.ListOptions{All: true})
	if err != nil {
		containerlog.Error("Error listing containers for batch stats", "error", err)
		apperrors.WriteJSON(w, apperrors.Wrap(
			err,
			"CONTAINER_LIST_FAILED",
			"failed to list containers",
			http.StatusInternalServerError,
		))
		return
	}

	// Get approved app IDs if storage is available
	approvedAppIDs := make(map[string]bool)
	if h.storage != nil {
		apps, err := h.storage.ListApps()
		if err != nil {
			containerlog.Warn("Failed to load approved apps for batch stats", "error", err)
		} else {
			for _, app := range apps {
				approvedAppIDs[app.ID] = true
			}
		}
	}

	// Collect stats for running containers
	var statsResults []ContainerStatsResponse
	for _, c := range containers {
		// Only get stats for running containers
		if c.State != runtime.StateRunning {
			continue
		}

		// Filter by approved apps if storage is available
		if h.storage != nil {
			appID, hasLabel := c.Labels["nekzus.app.id"]
			if !hasLabel || !approvedAppIDs[appID] {
				continue
			}
		}

		// Use runtime abstraction to get stats
		rtStats, err := rt.GetStats(ctx, c.ID)
		if err != nil {
			containerlog.Warn("Failed to get stats for container",
				"container_id", c.ID.ID,
				"error", err)
			continue
		}

		// Convert runtime.Stats to ContainerStatsResponse
		statsResults = append(statsResults, ContainerStatsResponse{
			ContainerID: c.ID.ID,
			CPU: struct {
				Usage      float64 `json:"usage"`
				CoresUsed  float64 `json:"coresUsed"`
				TotalCores float64 `json:"totalCores"`
			}{
				Usage:      rtStats.CPU.Usage,
				CoresUsed:  rtStats.CPU.CoresUsed,
				TotalCores: rtStats.CPU.TotalCores,
			},
			Memory: struct {
				Usage     float64 `json:"usage"`
				Used      uint64  `json:"used"`
				Limit     uint64  `json:"limit"`
				Available uint64  `json:"available"`
			}{
				Usage:     rtStats.Memory.Usage,
				Used:      rtStats.Memory.Used,
				Limit:     rtStats.Memory.Limit,
				Available: rtStats.Memory.Available,
			},
			Network: struct {
				Rx uint64 `json:"rx"`
				Tx uint64 `json:"tx"`
			}{
				Rx: rtStats.Network.RxBytes,
				Tx: rtStats.Network.TxBytes,
			},
			Timestamp: rtStats.Timestamp,
		})
	}

	response := BatchContainerStatsResponse{
		Containers: statsResults,
		Timestamp:  time.Now().Unix(),
	}

	// Ensure containers is never null in JSON
	if response.Containers == nil {
		response.Containers = []ContainerStatsResponse{}
	}

	containerlog.Info("Got batch container stats", "count", len(statsResults))

	if err := httputil.WriteJSON(w, http.StatusOK, response); err != nil {
		containerlog.Error("Error encoding JSON response", "error", err)
	}
}

// getContainerStatsWithRuntime fetches stats using the runtime interface
func (h *ContainerHandler) getContainerStatsWithRuntime(ctx context.Context, containerID string, runtimeType string, namespace string) (*ContainerStatsResponse, error) {
	rt, err := h.runtimes.Get(runtime.RuntimeType(runtimeType))
	if err != nil {
		return nil, err
	}

	id := runtime.ContainerID{
		Runtime:   runtime.RuntimeType(runtimeType),
		ID:        containerID,
		Namespace: namespace,
	}

	stats, err := rt.GetStats(ctx, id)
	if err != nil {
		containerlog.Error("Error getting stats for container", "container_id", containerID, "error", err)
		return nil, err
	}

	// Convert runtime.Stats to ContainerStatsResponse
	shortID := containerID
	if len(shortID) > 12 {
		shortID = shortID[:12]
	}

	response := &ContainerStatsResponse{
		ContainerID: shortID,
		Timestamp:   stats.Timestamp,
	}
	response.CPU.Usage = stats.CPU.Usage
	response.CPU.CoresUsed = stats.CPU.CoresUsed
	response.CPU.TotalCores = stats.CPU.TotalCores
	response.Memory.Usage = stats.Memory.Usage
	response.Memory.Used = stats.Memory.Used
	response.Memory.Limit = stats.Memory.Limit
	response.Memory.Available = stats.Memory.Available
	response.Network.Rx = stats.Network.RxBytes
	response.Network.Tx = stats.Network.TxBytes

	containerlog.Debug("Got container stats via runtime",
		"container_id", shortID,
		"runtime", runtimeType,
		"cpu_percent", stats.CPU.Usage,
		"memory_percent", stats.Memory.Usage)

	return response, nil
}

// calculateCPUPercent calculates CPU usage percentage from Docker stats
func calculateCPUPercent(stats *StatsJSON) float64 {
	cpuDelta := float64(stats.CPUStats.CPUUsage.TotalUsage - stats.PreCPUStats.CPUUsage.TotalUsage)
	systemDelta := float64(stats.CPUStats.SystemUsage - stats.PreCPUStats.SystemUsage)

	if systemDelta > 0.0 && cpuDelta > 0.0 {
		return (cpuDelta / systemDelta) * float64(len(stats.CPUStats.CPUUsage.PercpuUsage)) * 100.0
	}
	return 0.0
}

// extractContainerIDFromPath extracts container ID from URL path
// Path formats:
//   - /api/v1/containers/{containerId}
//   - /api/v1/containers/{containerId}/start
//   - /api/v1/containers/{containerId}/stop
//   - /api/v1/containers/{containerId}/restart
func extractContainerIDFromPath(path string) string {
	// Remove trailing slash if present
	path = strings.TrimSuffix(path, "/")

	// Split path and get segments
	parts := strings.Split(path, "/")
	if len(parts) < 5 {
		return ""
	}

	// Container ID is at index 4 (after /api/v1/containers/)
	// Example: /api/v1/containers/abc123/start
	// parts = ["", "api", "v1", "containers", "abc123", "start"]
	//          0    1      2      3            4         5
	return parts[4]
}

// isContainerNotFoundError checks if the error indicates container not found
func isContainerNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	errMsg := strings.ToLower(err.Error())
	return strings.Contains(errMsg, "no such container") ||
		strings.Contains(errMsg, "not found")
}

// parseContainerRequest extracts container ID, runtime type, and namespace from a request
// Supports both query params and path-based formats:
// - /api/v1/containers/{id}/action?runtime=kubernetes&namespace=default
// - /api/v1/containers/{namespace}/{pod}/action (K8s-style path)
func (h *ContainerHandler) parseContainerRequest(r *http.Request) (containerID string, runtimeType string, namespace string) {
	// Get runtime and namespace from query params
	runtimeType = r.URL.Query().Get("runtime")
	namespace = r.URL.Query().Get("namespace")

	// Parse path to extract container ID (and possibly namespace from K8s-style path)
	path := strings.TrimSuffix(r.URL.Path, "/")
	parts := strings.Split(path, "/")

	// Path formats:
	// /api/v1/containers/{id}/action - Docker style
	// /api/v1/containers/{namespace}/{pod}/action - K8s style
	// parts = ["", "api", "v1", "containers", ...]

	if len(parts) < 5 {
		return "", runtimeType, namespace
	}

	// Check if this is a K8s-style path (6+ parts with action at end)
	// /api/v1/containers/{namespace}/{pod}/{action}
	if len(parts) >= 7 && isContainerAction(parts[len(parts)-1]) {
		// K8s-style path: namespace is at index 4, pod at index 5
		if namespace == "" {
			namespace = parts[4]
		}
		containerID = parts[5]
		// Default to kubernetes runtime for K8s-style paths
		if runtimeType == "" {
			runtimeType = string(runtime.RuntimeKubernetes)
		}
	} else {
		// Docker-style path: container ID at index 4
		containerID = parts[4]
	}

	return containerID, runtimeType, namespace
}

// isContainerAction checks if a path segment is a container action
func isContainerAction(segment string) bool {
	actions := []string{"start", "stop", "restart", "stats", "logs"}
	for _, a := range actions {
		if segment == a {
			return true
		}
	}
	return false
}

// getAppIDFromContainer extracts the app ID from container labels using the runtime interface
func (h *ContainerHandler) getAppIDFromContainer(ctx context.Context, rt runtime.Runtime, id runtime.ContainerID) string {
	details, err := rt.Inspect(ctx, id)
	if err != nil {
		containerlog.Debug("Failed to inspect container for app ID",
			"container_id", id.ID,
			"error", err)
		return ""
	}
	return details.Labels["nekzus.app.id"]
}
