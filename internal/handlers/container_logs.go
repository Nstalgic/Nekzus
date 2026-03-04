package handlers

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/nstalgic/nekzus/internal/runtime"
	"github.com/nstalgic/nekzus/internal/storage"
	apptypes "github.com/nstalgic/nekzus/internal/types"
)

var logsLog = slog.With("package", "handlers.logs")

// Buffer pool constants for memory optimization
const (
	// DefaultLogBufferSize is the default size for pooled log buffers (32KB)
	// Most log lines are under 32KB, so this covers typical cases
	DefaultLogBufferSize = 32 * 1024

	// MaxLogFrameSize is the maximum allowed frame size (1MB)
	// Frames larger than this are skipped to prevent memory abuse
	MaxLogFrameSize = 1024 * 1024
)

// logBufferPool provides reusable byte buffers for log frame reading
// This significantly reduces GC pressure during high-volume log streaming
var logBufferPool = &sync.Pool{
	New: func() interface{} {
		buf := make([]byte, DefaultLogBufferSize)
		return &buf
	},
}

// LogStartRequest represents a request to start log streaming
type LogStartRequest struct {
	ContainerID string `json:"containerId"`
	Tail        int    `json:"tail"`
	Follow      bool   `json:"follow"`
	Timestamps  bool   `json:"timestamps"`
}

// LogStopRequest represents a request to stop log streaming
type LogStopRequest struct {
	ContainerID string `json:"containerId"`
}

// logError represents an error during log operations
type logError struct {
	Code    string
	Message string
}

// ContainerLogsHandler handles container log streaming
type ContainerLogsHandler struct {
	client    DockerClient      // Legacy Docker client (for backward compatibility)
	runtimes  *runtime.Registry // Runtime registry (preferred)
	notifier  ContainerNotifier
	streamMgr *LogStreamManager
	storage   *storage.Store
}

// NewContainerLogsHandler creates a new container logs handler with legacy Docker client
func NewContainerLogsHandler(client DockerClient, store *storage.Store) *ContainerLogsHandler {
	return &ContainerLogsHandler{
		client:    client,
		storage:   store,
		streamMgr: NewLogStreamManager(),
	}
}

// NewContainerLogsHandlerWithRuntime creates a new container logs handler with runtime registry
func NewContainerLogsHandlerWithRuntime(runtimes *runtime.Registry, store *storage.Store) *ContainerLogsHandler {
	return &ContainerLogsHandler{
		runtimes:  runtimes,
		storage:   store,
		streamMgr: NewLogStreamManager(),
	}
}

// hasRuntime checks if a runtime registry is configured
func (h *ContainerLogsHandler) hasRuntime() bool {
	return h.runtimes != nil && h.runtimes.GetPrimary() != nil
}

// SetNotifier sets the WebSocket notifier
func (h *ContainerLogsHandler) SetNotifier(notifier ContainerNotifier) {
	h.notifier = notifier
}

// GetStreamManager returns the stream manager for cleanup callbacks
func (h *ContainerLogsHandler) GetStreamManager() *LogStreamManager {
	return h.streamMgr
}

// HandleStartStream handles a request to start log streaming
func (h *ContainerLogsHandler) HandleStartStream(deviceID string, req LogStartRequest) {
	// Validate container exists
	if err := h.validateContainer(req.ContainerID); err != nil {
		h.sendError(deviceID, req.ContainerID, err.Code, err.Message)
		return
	}

	// Apply defaults and bounds
	tail := req.Tail
	if tail <= 0 {
		tail = 100
	}
	if tail > 1000 {
		tail = 1000
	}

	// Start stream (stops any existing stream for this device)
	ctx, _ := h.streamMgr.StartStream(deviceID, req.ContainerID)

	// Send started notification
	h.sendStarted(deviceID, req.ContainerID)

	// Start streaming goroutine
	go h.streamLogs(ctx, deviceID, req.ContainerID, tail, req.Follow, req.Timestamps)
}

// HandleStopStream handles a request to stop log streaming
func (h *ContainerLogsHandler) HandleStopStream(deviceID string, req LogStopRequest) {
	if h.streamMgr.StopStream(deviceID) {
		h.sendEnded(deviceID, req.ContainerID, "stopped", "")
	}
}

// streamLogs streams container logs to the device
func (h *ContainerLogsHandler) streamLogs(ctx context.Context, deviceID, containerID string, tail int, follow bool, timestamps bool) {
	defer h.streamMgr.RemoveStream(deviceID)

	var reader io.ReadCloser
	var err error

	// Use runtime registry if available, otherwise fall back to legacy client
	if h.hasRuntime() {
		logOpts := runtime.LogOptions{
			Follow:     follow,
			Tail:       int64(tail),
			Timestamps: timestamps,
		}
		reader, err = h.runtimes.GetPrimary().StreamLogs(ctx, runtime.ContainerID{
			Runtime: h.runtimes.GetPrimary().Type(),
			ID:      containerID,
		}, logOpts)
	} else {
		// Legacy Docker client path
		logOpts := container.LogsOptions{
			ShowStdout: true,
			ShowStderr: true,
			Follow:     follow,
			Tail:       fmt.Sprintf("%d", tail),
			Timestamps: timestamps,
		}
		reader, err = h.client.ContainerLogs(ctx, containerID, logOpts)
	}

	if err != nil {
		logsLog.Error("failed to get container logs",
			"container_id", containerID,
			"device_id", deviceID,
			"error", err)
		h.sendEnded(deviceID, containerID, "error", err.Error())
		return
	}
	defer reader.Close()

	// Read Docker multiplexed stream
	// Header format: [stream_type(1)][reserved(3)][size(4)]
	// stream_type: 0=stdin, 1=stdout, 2=stderr
	header := make([]byte, 8)

	// Get a reusable buffer from the pool
	bufPtr := logBufferPool.Get().(*[]byte)
	pooledBuf := *bufPtr
	defer logBufferPool.Put(bufPtr)

	for {
		select {
		case <-ctx.Done():
			h.sendEnded(deviceID, containerID, "stopped", "")
			return
		default:
		}

		// Read 8-byte header
		_, err := io.ReadFull(reader, header)
		if err != nil {
			if err == io.EOF {
				// Stream ended naturally
				h.sendEnded(deviceID, containerID, "container_stopped", "")
				return
			}
			if ctx.Err() != nil {
				h.sendEnded(deviceID, containerID, "stopped", "")
				return
			}
			logsLog.Error("error reading log header",
				"container_id", containerID,
				"device_id", deviceID,
				"error", err)
			h.sendEnded(deviceID, containerID, "error", err.Error())
			return
		}

		// Parse header
		streamType := header[0]
		frameSize := int(header[4])<<24 | int(header[5])<<16 | int(header[6])<<8 | int(header[7])

		if frameSize <= 0 {
			continue
		}

		// Validate frame size to prevent memory abuse
		if frameSize > MaxLogFrameSize {
			logsLog.Warn("skipping oversized log frame",
				"container_id", containerID,
				"device_id", deviceID,
				"frame_size", frameSize,
				"max_size", MaxLogFrameSize)
			// Skip the oversized frame by reading and discarding
			if _, err := io.CopyN(io.Discard, reader, int64(frameSize)); err != nil {
				if ctx.Err() != nil {
					h.sendEnded(deviceID, containerID, "stopped", "")
					return
				}
				logsLog.Error("error discarding oversized frame",
					"container_id", containerID,
					"error", err)
			}
			continue
		}

		// Use pooled buffer if large enough, otherwise allocate
		var content []byte
		if frameSize <= len(pooledBuf) {
			content = pooledBuf[:frameSize]
		} else {
			// Frame larger than pool buffer but under max - allocate once
			content = make([]byte, frameSize)
		}

		_, err = io.ReadFull(reader, content)
		if err != nil {
			if ctx.Err() != nil {
				h.sendEnded(deviceID, containerID, "stopped", "")
				return
			}
			logsLog.Error("error reading log content",
				"container_id", containerID,
				"device_id", deviceID,
				"error", err)
			h.sendEnded(deviceID, containerID, "error", err.Error())
			return
		}

		// Determine stream name
		stream := "stdout"
		if streamType == 2 {
			stream = "stderr"
		}

		// Send log line (trim trailing newline for cleaner output)
		// Copy content to new string to avoid keeping reference to pooled buffer
		message := strings.TrimSuffix(string(content), "\n")
		h.sendLogData(deviceID, containerID, stream, message)
	}
}

// validateContainer checks if container exists
func (h *ContainerLogsHandler) validateContainer(containerID string) *logError {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Use runtime registry if available, otherwise fall back to legacy client
	if h.hasRuntime() {
		_, err := h.runtimes.GetPrimary().Inspect(ctx, runtime.ContainerID{
			Runtime: h.runtimes.GetPrimary().Type(),
			ID:      containerID,
		})
		if err != nil {
			errStr := strings.ToLower(err.Error())
			if strings.Contains(errStr, "no such container") || strings.Contains(errStr, "not found") {
				return &logError{Code: "CONTAINER_NOT_FOUND", Message: "Container not found"}
			}
			return &logError{Code: "INTERNAL_ERROR", Message: err.Error()}
		}
		return nil
	}

	// Legacy Docker client path
	_, err := h.client.ContainerInspect(ctx, containerID)
	if err != nil {
		errStr := strings.ToLower(err.Error())
		if strings.Contains(errStr, "no such container") || strings.Contains(errStr, "not found") {
			return &logError{Code: "CONTAINER_NOT_FOUND", Message: "Container not found"}
		}
		return &logError{Code: "INTERNAL_ERROR", Message: err.Error()}
	}

	return nil
}

// sendStarted sends the logs.started message
func (h *ContainerLogsHandler) sendStarted(deviceID, containerID string) {
	if h.notifier == nil {
		return
	}
	h.notifier.SendToDevice(deviceID, apptypes.WebSocketMessage{
		Type: apptypes.WSMsgTypeContainerLogsStarted,
		Data: map[string]interface{}{
			"containerId": containerID,
			"timestamp":   time.Now().Unix(),
		},
	})
}

// sendLogData sends a log data message
func (h *ContainerLogsHandler) sendLogData(deviceID, containerID, stream, message string) {
	if h.notifier == nil {
		return
	}
	h.notifier.SendToDevice(deviceID, apptypes.WebSocketMessage{
		Type: apptypes.WSMsgTypeContainerLogs,
		Data: map[string]interface{}{
			"containerId": containerID,
			"stream":      stream,
			"message":     message,
			"timestamp":   time.Now().Unix(),
		},
	})
}

// sendEnded sends the logs.ended message
func (h *ContainerLogsHandler) sendEnded(deviceID, containerID, reason, message string) {
	if h.notifier == nil {
		return
	}
	data := map[string]interface{}{
		"containerId": containerID,
		"reason":      reason,
		"timestamp":   time.Now().Unix(),
	}
	if message != "" {
		data["message"] = message
	}
	h.notifier.SendToDevice(deviceID, apptypes.WebSocketMessage{
		Type: apptypes.WSMsgTypeContainerLogsEnded,
		Data: data,
	})
}

// sendError sends a logs.error message
func (h *ContainerLogsHandler) sendError(deviceID, containerID, code, message string) {
	if h.notifier == nil {
		return
	}
	h.notifier.SendToDevice(deviceID, apptypes.WebSocketMessage{
		Type: apptypes.WSMsgTypeContainerLogsError,
		Data: map[string]interface{}{
			"containerId": containerID,
			"error":       code,
			"message":     message,
			"timestamp":   time.Now().Unix(),
		},
	})
}
