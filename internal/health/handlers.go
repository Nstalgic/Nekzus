package health

import (
	"encoding/json"
	"net/http"
)

// Handler provides HTTP handlers for health check endpoints
type Handler struct {
	manager *Manager
}

// NewHandler creates a new health check handler
func NewHandler(manager *Manager) *Handler {
	return &Handler{
		manager: manager,
	}
}

// HandleLiveness handles the /livez endpoint for Kubernetes liveness probes
// Returns 200 OK if the application is running, regardless of component status
func (h *Handler) HandleLiveness(w http.ResponseWriter, r *http.Request) {
	if h.manager.SimpleLivenessCheck() {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte("NOT OK"))
	}
}

// HandleReadiness handles the /readyz endpoint for Kubernetes readiness probes
// Returns 200 OK only if the system is ready to serve requests (no unhealthy components)
func (h *Handler) HandleReadiness(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if h.manager.IsReady(ctx) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("READY"))
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte("NOT READY"))
	}
}

// HandleHealth handles the detailed health check endpoint
// Returns comprehensive health information about all components
func (h *Handler) HandleHealth(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	health := h.manager.Check(ctx)

	// Set appropriate status code based on overall health
	statusCode := http.StatusOK
	switch health.Status {
	case StatusDegraded:
		statusCode = http.StatusOK // Still serving traffic
	case StatusUnhealthy:
		statusCode = http.StatusServiceUnavailable
	case StatusUnknown:
		statusCode = http.StatusServiceUnavailable
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(health); err != nil {
		// If we can't encode the response, something is very wrong
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
}

// HandleComponentHealth handles requests for a specific component's health
func (h *Handler) HandleComponentHealth(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Extract component name from query parameter or path
	componentName := r.URL.Query().Get("component")
	if componentName == "" {
		http.Error(w, "component parameter required", http.StatusBadRequest)
		return
	}

	componentHealth, err := h.manager.GetComponentHealth(ctx, componentName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	// Set status code based on component health
	statusCode := http.StatusOK
	switch componentHealth.Status {
	case StatusDegraded:
		statusCode = http.StatusOK
	case StatusUnhealthy:
		statusCode = http.StatusServiceUnavailable
	case StatusUnknown:
		statusCode = http.StatusServiceUnavailable
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(componentHealth); err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
}
