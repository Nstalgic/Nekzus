package handlers

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types/container"
	apperrors "github.com/nstalgic/nekzus/internal/errors"
	"github.com/nstalgic/nekzus/internal/httputil"
	"github.com/nstalgic/nekzus/internal/runtime"
)

var log = slog.With("package", "handlers")

// Bulk operation concurrency limit
const (
	// maxConcurrentBulkOps is the maximum number of concurrent Docker operations
	maxConcurrentBulkOps = 5
)

// BulkOperationRequest represents a bulk container operation request
type BulkOperationRequest struct {
	Action       string   `json:"action"`       // "start", "stop", or "restart"
	ContainerIDs []string `json:"containerIds"` // List of container IDs
	Timeout      *int     `json:"timeout"`      // Optional timeout in seconds
}

// BulkOperationResult represents the result of a bulk operation on a single container
type BulkOperationResult struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

// BulkOperationResponse represents the response for bulk operations
type BulkOperationResponse struct {
	Message string                `json:"message"`
	Total   int                   `json:"total"`
	Success int                   `json:"success"`
	Failed  int                   `json:"failed"`
	Results []BulkOperationResult `json:"results"`
}

// bulkContainer is an internal type for bulk operations that works with both runtime and legacy client
type bulkContainer struct {
	id        string
	name      string
	state     string
	runtime   runtime.Runtime
	runtimeID runtime.ContainerID
}

// HandleRestartAll restarts all containers
// POST /api/v1/containers/restart-all
func (h *ContainerHandler) HandleRestartAll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		apperrors.WriteJSON(w, apperrors.New("METHOD_NOT_ALLOWED", "Method not allowed", http.StatusMethodNotAllowed))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
	defer cancel()

	// Get all containers using runtime or legacy client
	var containerList []bulkContainer
	var err error

	if h.hasRuntime() {
		rt := h.getPrimaryRuntime()
		containers, listErr := rt.List(ctx, runtime.ListOptions{All: true})
		if listErr != nil {
			err = listErr
		} else {
			for _, c := range containers {
				containerList = append(containerList, bulkContainer{
					id:        c.ID.ID,
					name:      c.Name,
					runtime:   rt,
					runtimeID: c.ID,
				})
			}
		}
	} else if h.client != nil {
		containers, listErr := h.client.ContainerList(ctx, container.ListOptions{All: true})
		if listErr != nil {
			err = listErr
		} else {
			for _, c := range containers {
				name := "unknown"
				if len(c.Names) > 0 {
					name = strings.TrimPrefix(c.Names[0], "/")
				}
				containerList = append(containerList, bulkContainer{
					id:   c.ID,
					name: name,
				})
			}
		}
	} else {
		apperrors.WriteJSON(w, apperrors.New("NO_RUNTIME", "No runtime configured", http.StatusInternalServerError))
		return
	}

	if err != nil {
		log.Error("Error listing containers for restart-all", "error", err)
		apperrors.WriteJSON(w, apperrors.Wrap(
			err,
			"CONTAINER_LIST_FAILED",
			"failed to list containers",
			http.StatusInternalServerError,
		))
		return
	}

	// Restart each container with concurrency limit
	results := make([]BulkOperationResult, len(containerList))
	var successCount int
	var mu sync.Mutex
	defaultTimeout := 10 * time.Second

	// Use semaphore to limit concurrent operations
	sem := make(chan struct{}, maxConcurrentBulkOps)
	var wg sync.WaitGroup

	for i, c := range containerList {
		wg.Add(1)
		go func(idx int, cont bulkContainer) {
			defer wg.Done()

			// Acquire semaphore
			sem <- struct{}{}
			defer func() { <-sem }()

			shortID := cont.id
			if len(shortID) > 12 {
				shortID = shortID[:12]
			}

			// Attempt restart using runtime or legacy client
			var opErr error
			if cont.runtime != nil {
				opErr = cont.runtime.Restart(ctx, cont.runtimeID, &defaultTimeout)
			} else if h.client != nil {
				timeoutSecs := int(defaultTimeout.Seconds())
				opErr = h.client.ContainerRestart(ctx, cont.id, container.StopOptions{
					Timeout: &timeoutSecs,
				})
			}

			result := BulkOperationResult{
				ID:      shortID,
				Name:    cont.name,
				Success: opErr == nil,
			}

			if opErr != nil {
				result.Error = opErr.Error()
				log.Error("Failed to restart container", "name", cont.name, "id", shortID, "error", opErr)
			} else {
				mu.Lock()
				successCount++
				mu.Unlock()
				log.Info("Restarted container", "name", cont.name, "id", shortID)
			}

			results[idx] = result
		}(i, c)
	}

	wg.Wait()

	response := BulkOperationResponse{
		Message: "Restart all operation completed",
		Total:   len(containerList),
		Success: successCount,
		Failed:  len(containerList) - successCount,
		Results: results,
	}

	if err := httputil.WriteJSON(w, http.StatusOK, response); err != nil {
		log.Error("Error encoding JSON response", "error", err)
	}
}

// HandleStopAll stops all containers
// POST /api/v1/containers/stop-all
func (h *ContainerHandler) HandleStopAll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		apperrors.WriteJSON(w, apperrors.New("METHOD_NOT_ALLOWED", "Method not allowed", http.StatusMethodNotAllowed))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
	defer cancel()

	// Get all running containers using runtime or legacy client
	var containerList []bulkContainer
	var err error

	if h.hasRuntime() {
		rt := h.getPrimaryRuntime()
		containers, listErr := rt.List(ctx, runtime.ListOptions{All: false}) // Only running
		if listErr != nil {
			err = listErr
		} else {
			for _, c := range containers {
				containerList = append(containerList, bulkContainer{
					id:        c.ID.ID,
					name:      c.Name,
					runtime:   rt,
					runtimeID: c.ID,
				})
			}
		}
	} else if h.client != nil {
		containers, listErr := h.client.ContainerList(ctx, container.ListOptions{All: false})
		if listErr != nil {
			err = listErr
		} else {
			for _, c := range containers {
				name := "unknown"
				if len(c.Names) > 0 {
					name = strings.TrimPrefix(c.Names[0], "/")
				}
				containerList = append(containerList, bulkContainer{
					id:   c.ID,
					name: name,
				})
			}
		}
	} else {
		apperrors.WriteJSON(w, apperrors.New("NO_RUNTIME", "No runtime configured", http.StatusInternalServerError))
		return
	}

	if err != nil {
		log.Error("Error listing containers for stop-all", "error", err)
		apperrors.WriteJSON(w, apperrors.Wrap(
			err,
			"CONTAINER_LIST_FAILED",
			"failed to list containers",
			http.StatusInternalServerError,
		))
		return
	}

	// Stop each container with concurrency limit
	results := make([]BulkOperationResult, len(containerList))
	var successCount int
	var mu sync.Mutex
	defaultTimeout := 10 * time.Second

	// Use semaphore to limit concurrent operations
	sem := make(chan struct{}, maxConcurrentBulkOps)
	var wg sync.WaitGroup

	for i, c := range containerList {
		wg.Add(1)
		go func(idx int, cont bulkContainer) {
			defer wg.Done()

			// Acquire semaphore
			sem <- struct{}{}
			defer func() { <-sem }()

			shortID := cont.id
			if len(shortID) > 12 {
				shortID = shortID[:12]
			}

			// Attempt stop using runtime or legacy client
			var opErr error
			if cont.runtime != nil {
				opErr = cont.runtime.Stop(ctx, cont.runtimeID, &defaultTimeout)
			} else if h.client != nil {
				timeoutSecs := int(defaultTimeout.Seconds())
				opErr = h.client.ContainerStop(ctx, cont.id, container.StopOptions{
					Timeout: &timeoutSecs,
				})
			}

			result := BulkOperationResult{
				ID:      shortID,
				Name:    cont.name,
				Success: opErr == nil,
			}

			if opErr != nil {
				result.Error = opErr.Error()
				log.Error("Failed to stop container", "name", cont.name, "id", shortID, "error", opErr)
			} else {
				mu.Lock()
				successCount++
				mu.Unlock()
				log.Info("Stopped container", "name", cont.name, "id", shortID)
			}

			results[idx] = result
		}(i, c)
	}

	wg.Wait()

	response := BulkOperationResponse{
		Message: "Stop all operation completed",
		Total:   len(containerList),
		Success: successCount,
		Failed:  len(containerList) - successCount,
		Results: results,
	}

	if err := httputil.WriteJSON(w, http.StatusOK, response); err != nil {
		log.Error("Error encoding JSON response", "error", err)
	}
}

// HandleStartAll starts all stopped containers
// POST /api/v1/containers/start-all
func (h *ContainerHandler) HandleStartAll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		apperrors.WriteJSON(w, apperrors.New("METHOD_NOT_ALLOWED", "Method not allowed", http.StatusMethodNotAllowed))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
	defer cancel()

	// Get all containers using runtime or legacy client, then filter to stopped
	var containerList []bulkContainer
	var err error

	if h.hasRuntime() {
		rt := h.getPrimaryRuntime()
		containers, listErr := rt.List(ctx, runtime.ListOptions{All: true})
		if listErr != nil {
			err = listErr
		} else {
			for _, c := range containers {
				// Filter to only stopped containers
				if c.State != runtime.StateRunning {
					containerList = append(containerList, bulkContainer{
						id:        c.ID.ID,
						name:      c.Name,
						state:     string(c.State),
						runtime:   rt,
						runtimeID: c.ID,
					})
				}
			}
		}
	} else if h.client != nil {
		containers, listErr := h.client.ContainerList(ctx, container.ListOptions{All: true})
		if listErr != nil {
			err = listErr
		} else {
			for _, c := range containers {
				// Filter to only stopped containers
				if c.State != "running" {
					name := "unknown"
					if len(c.Names) > 0 {
						name = strings.TrimPrefix(c.Names[0], "/")
					}
					containerList = append(containerList, bulkContainer{
						id:    c.ID,
						name:  name,
						state: c.State,
					})
				}
			}
		}
	} else {
		apperrors.WriteJSON(w, apperrors.New("NO_RUNTIME", "No runtime configured", http.StatusInternalServerError))
		return
	}

	if err != nil {
		log.Error("Error listing containers for start-all", "error", err)
		apperrors.WriteJSON(w, apperrors.Wrap(
			err,
			"CONTAINER_LIST_FAILED",
			"failed to list containers",
			http.StatusInternalServerError,
		))
		return
	}

	// Start each stopped container with concurrency limit
	results := make([]BulkOperationResult, len(containerList))
	var successCount int
	var mu sync.Mutex

	// Use semaphore to limit concurrent operations
	sem := make(chan struct{}, maxConcurrentBulkOps)
	var wg sync.WaitGroup

	for i, c := range containerList {
		wg.Add(1)
		go func(idx int, cont bulkContainer) {
			defer wg.Done()

			// Acquire semaphore
			sem <- struct{}{}
			defer func() { <-sem }()

			shortID := cont.id
			if len(shortID) > 12 {
				shortID = shortID[:12]
			}

			// Attempt start using runtime or legacy client
			var opErr error
			if cont.runtime != nil {
				opErr = cont.runtime.Start(ctx, cont.runtimeID)
			} else if h.client != nil {
				opErr = h.client.ContainerStart(ctx, cont.id, container.StartOptions{})
			}

			result := BulkOperationResult{
				ID:      shortID,
				Name:    cont.name,
				Success: opErr == nil,
			}

			if opErr != nil {
				result.Error = opErr.Error()
				log.Error("Failed to start container", "name", cont.name, "id", shortID, "error", opErr)
			} else {
				mu.Lock()
				successCount++
				mu.Unlock()
				log.Info("Started container", "name", cont.name, "id", shortID)
			}

			results[idx] = result
		}(i, c)
	}

	wg.Wait()

	response := BulkOperationResponse{
		Message: "Start all operation completed",
		Total:   len(containerList),
		Success: successCount,
		Failed:  len(containerList) - successCount,
		Results: results,
	}

	if err := httputil.WriteJSON(w, http.StatusOK, response); err != nil {
		log.Error("Error encoding JSON response", "error", err)
	}
}

// HandleBatchOperation performs an operation on a batch of specific containers
// POST /api/v1/containers/batch
func (h *ContainerHandler) HandleBatchOperation(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		apperrors.WriteJSON(w, apperrors.New("METHOD_NOT_ALLOWED", "Method not allowed", http.StatusMethodNotAllowed))
		return
	}

	// Parse request body
	var req BulkOperationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apperrors.WriteJSON(w, apperrors.New(
			"INVALID_REQUEST_BODY",
			"invalid request body",
			http.StatusBadRequest,
		))
		return
	}

	// Validate action
	if req.Action != "start" && req.Action != "stop" && req.Action != "restart" {
		apperrors.WriteJSON(w, apperrors.New(
			"INVALID_ACTION",
			"action must be 'start', 'stop', or 'restart'",
			http.StatusBadRequest,
		))
		return
	}

	// Validate container IDs
	if len(req.ContainerIDs) == 0 {
		apperrors.WriteJSON(w, apperrors.New(
			"EMPTY_CONTAINER_LIST",
			"containerIds cannot be empty",
			http.StatusBadRequest,
		))
		return
	}

	// Check if we have a runtime available
	if !h.hasRuntime() && h.client == nil {
		apperrors.WriteJSON(w, apperrors.New("NO_RUNTIME", "No runtime configured", http.StatusInternalServerError))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
	defer cancel()

	// Perform operation on each container with concurrency limit
	results := make([]BulkOperationResult, len(req.ContainerIDs))
	var successCount int
	var mu sync.Mutex
	defaultTimeoutSecs := 10
	if req.Timeout != nil {
		defaultTimeoutSecs = *req.Timeout
	}
	defaultTimeout := time.Duration(defaultTimeoutSecs) * time.Second

	// Use semaphore to limit concurrent operations
	sem := make(chan struct{}, maxConcurrentBulkOps)
	var wg sync.WaitGroup

	for i, containerID := range req.ContainerIDs {
		wg.Add(1)
		go func(idx int, cID string) {
			defer wg.Done()

			// Acquire semaphore
			sem <- struct{}{}
			defer func() { <-sem }()

			shortID := cID
			if len(shortID) > 12 {
				shortID = shortID[:12]
			}

			var name string
			var opErr error

			if h.hasRuntime() {
				rt := h.getPrimaryRuntime()
				runtimeID := runtime.ContainerID{
					Runtime: rt.Type(),
					ID:      cID,
				}

				// Get container info for name
				details, inspectErr := rt.Inspect(ctx, runtimeID)
				if inspectErr == nil && details != nil {
					name = details.Name
				} else {
					name = cID
				}

				// Perform the requested action
				switch req.Action {
				case "start":
					opErr = rt.Start(ctx, runtimeID)
				case "stop":
					opErr = rt.Stop(ctx, runtimeID, &defaultTimeout)
				case "restart":
					opErr = rt.Restart(ctx, runtimeID, &defaultTimeout)
				}
			} else {
				// Legacy Docker client path
				containerJSON, inspectErr := h.client.ContainerInspect(ctx, cID)
				if inspectErr == nil {
					name = strings.TrimPrefix(containerJSON.Name, "/")
				} else {
					name = cID
				}

				// Perform the requested action
				switch req.Action {
				case "start":
					opErr = h.client.ContainerStart(ctx, cID, container.StartOptions{})
				case "stop":
					opErr = h.client.ContainerStop(ctx, cID, container.StopOptions{
						Timeout: &defaultTimeoutSecs,
					})
				case "restart":
					opErr = h.client.ContainerRestart(ctx, cID, container.StopOptions{
						Timeout: &defaultTimeoutSecs,
					})
				}
			}

			result := BulkOperationResult{
				ID:      shortID,
				Name:    name,
				Success: opErr == nil,
			}

			if opErr != nil {
				result.Error = opErr.Error()
				log.Error("Failed to perform container action", "action", req.Action, "name", name, "id", shortID, "error", opErr)
			} else {
				mu.Lock()
				successCount++
				mu.Unlock()
				log.Info("Successfully performed container action", "action", req.Action, "name", name, "id", shortID)
			}

			results[idx] = result
		}(i, containerID)
	}

	wg.Wait()

	response := BulkOperationResponse{
		Message: "Batch operation completed",
		Total:   len(req.ContainerIDs),
		Success: successCount,
		Failed:  len(req.ContainerIDs) - successCount,
		Results: results,
	}

	if err := httputil.WriteJSON(w, http.StatusOK, response); err != nil {
		log.Error("Error encoding JSON response", "error", err)
	}
}
