package handlers

import (
	"log/slog"
	"net/http"
	"strconv"
	"time"

	apperrors "github.com/nstalgic/nekzus/internal/errors"
	"github.com/nstalgic/nekzus/internal/health"
	"github.com/nstalgic/nekzus/internal/httputil"
	"github.com/nstalgic/nekzus/internal/types"
)

var serviceHealthLog = slog.With("package", "handlers.service_health")

// ServiceHealthRouteRegistry defines the interface for accessing routes
type ServiceHealthRouteRegistry interface {
	GetRouteByAppID(appID string) (*types.Route, bool)
}

// ServiceHealthChecker defines the interface for checking service health
type ServiceHealthChecker interface {
	GetServiceHealth(appID string) (*health.ServiceHealthStatus, bool)
	IsServiceHealthy(appID string) bool
}

// ServiceHealthHandler handles per-service health check requests
type ServiceHealthHandler struct {
	routeRegistry ServiceHealthRouteRegistry
	healthChecker ServiceHealthChecker
}

// NewServiceHealthHandler creates a new service health handler
func NewServiceHealthHandler(
	routeRegistry ServiceHealthRouteRegistry,
	healthChecker ServiceHealthChecker,
) *ServiceHealthHandler {
	return &ServiceHealthHandler{
		routeRegistry: routeRegistry,
		healthChecker: healthChecker,
	}
}

// ServiceHealthResponse represents the health status response
type ServiceHealthResponse struct {
	AppID               string     `json:"appId"`
	Status              string     `json:"status"`
	LastCheckTime       *time.Time `json:"lastCheckTime,omitempty"`
	LastSuccessTime     *time.Time `json:"lastSuccessTime,omitempty"`
	ConsecutiveFailures int        `json:"consecutiveFailures"`
	Message             string     `json:"message,omitempty"`
}

// HandleServiceHealth handles GET/HEAD requests for service health
// GET  /api/v1/services/{appId}/health - Full health info (JSON)
// HEAD /api/v1/services/{appId}/health - Lightweight check (headers only)
func (h *ServiceHealthHandler) HandleServiceHealth(w http.ResponseWriter, r *http.Request, appID string) {
	// Only allow GET and HEAD
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		apperrors.WriteJSON(w, apperrors.New(
			"METHOD_NOT_ALLOWED",
			"Method not allowed",
			http.StatusMethodNotAllowed,
		))
		return
	}

	// Check if service/route exists
	_, exists := h.routeRegistry.GetRouteByAppID(appID)
	if !exists {
		apperrors.WriteJSON(w, apperrors.New(
			"SERVICE_NOT_FOUND",
			"Service not found",
			http.StatusNotFound,
		))
		return
	}

	// Get health status
	var response ServiceHealthResponse
	var httpStatus int

	if healthStatus, ok := h.healthChecker.GetServiceHealth(appID); ok {
		response = ServiceHealthResponse{
			AppID:               appID,
			Status:              healthStatus.Status,
			LastCheckTime:       &healthStatus.LastCheckTime,
			LastSuccessTime:     healthStatus.LastSuccessTime,
			ConsecutiveFailures: healthStatus.ConsecutiveFailures,
			Message:             healthStatus.ErrorMessage,
		}

		// Determine HTTP status based on health
		switch healthStatus.Status {
		case "healthy":
			httpStatus = http.StatusOK
		case "unhealthy":
			httpStatus = http.StatusServiceUnavailable
		default: // unknown
			httpStatus = http.StatusOK
		}
	} else {
		// No health status available - return unknown
		response = ServiceHealthResponse{
			AppID:               appID,
			Status:              "unknown",
			ConsecutiveFailures: 0,
			Message:             "No health check data available",
		}
		httpStatus = http.StatusOK
	}

	// Set health headers (useful for HEAD requests and caching)
	w.Header().Set("X-Health-Status", response.Status)
	if response.LastCheckTime != nil {
		w.Header().Set("X-Last-Check", response.LastCheckTime.Format(time.RFC3339))
	}
	w.Header().Set("X-Consecutive-Failures", strconv.Itoa(response.ConsecutiveFailures))
	w.Header().Set("Cache-Control", "no-cache")

	// For HEAD requests, just return status code with headers
	if r.Method == http.MethodHead {
		w.WriteHeader(httpStatus)
		return
	}

	// For GET requests, return JSON body
	if err := httputil.WriteJSON(w, httpStatus, response); err != nil {
		serviceHealthLog.Error("failed to encode json response", "error", err)
	}
}
