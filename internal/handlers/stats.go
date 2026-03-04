package handlers

import (
	"log/slog"
	"net/http"
	"time"

	apperrors "github.com/nstalgic/nekzus/internal/errors"
	"github.com/nstalgic/nekzus/internal/httputil"
	"github.com/nstalgic/nekzus/internal/metrics"
	"github.com/nstalgic/nekzus/internal/storage"
	apptypes "github.com/nstalgic/nekzus/internal/types"
)

var statslog = slog.With("package", "handlers")

// Router interface for stats handler
type Router interface {
	ListRoutes() []apptypes.Route
}

// Storage interface for stats handler
type Storage interface {
	ListDevices() ([]storage.DeviceInfo, error)
	ListApps() ([]apptypes.App, error)
	GetTotalRequestsToday() (int, error)
}

// StatsHandler handles stats endpoints
type StatsHandler struct {
	router  Router
	storage Storage
	metrics *metrics.Metrics
}

// NewStatsHandler creates a new stats handler
func NewStatsHandler(router Router, storage Storage, m *metrics.Metrics) *StatsHandler {
	return &StatsHandler{
		router:  router,
		storage: storage,
		metrics: m,
	}
}

// QuickStatsResponse represents quick stats for mobile/dashboard
type QuickStatsResponse struct {
	ServicesTotal   int     `json:"servicesTotal"`
	ServicesOnline  int     `json:"servicesOnline"`
	ServicesOffline int     `json:"servicesOffline"`
	DevicesTotal    int     `json:"devicesTotal"`
	DevicesOnline   int     `json:"devicesOnline"`
	RoutesTotal     int     `json:"routesTotal"`
	RequestsToday   int     `json:"requestsToday"`
	AvgLatency      float64 `json:"avgLatency"`    // In milliseconds
	UptimePercent   float64 `json:"uptimePercent"` // Percentage (0-100)
}

// HandleQuickStats returns lightweight stats optimized for mobile/widgets
// GET /api/v1/stats/quick
func (h *StatsHandler) HandleQuickStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		apperrors.WriteJSON(w, apperrors.New("METHOD_NOT_ALLOWED", "Method not allowed", http.StatusMethodNotAllowed))
		return
	}

	stats := QuickStatsResponse{
		// Initialize with zeros
		ServicesTotal:   0,
		ServicesOnline:  0,
		ServicesOffline: 0,
		DevicesTotal:    0,
		DevicesOnline:   0,
		RoutesTotal:     0,
		RequestsToday:   0,
		AvgLatency:      0,
		UptimePercent:   100.0, // Default to 100% until health checks implemented
	}

	// Get metrics-based stats
	if h.metrics != nil {
		stats.AvgLatency = h.metrics.GetAverageLatency()
		stats.UptimePercent = h.metrics.GetUptimePercent()
	}

	// Get services count
	if h.storage != nil {
		if apps, err := h.storage.ListApps(); err == nil {
			stats.ServicesTotal = len(apps)

			// TODO: Get actual health status from health checker
			// For now, assume all services are online
			stats.ServicesOnline = stats.ServicesTotal
			stats.ServicesOffline = 0
		} else {
			statslog.Warn("Failed to get apps for quick stats", "error", err)
		}
	}

	// Get devices count
	if h.storage != nil {
		if devices, err := h.storage.ListDevices(); err == nil {
			stats.DevicesTotal = len(devices)

			// Count devices seen in last 5 minutes as "online"
			now := time.Now()
			onlineCount := 0
			for _, device := range devices {
				if !device.LastSeen.IsZero() {
					diffMins := int(now.Sub(device.LastSeen).Minutes())
					if diffMins < 5 {
						onlineCount++
					}
				}
			}
			stats.DevicesOnline = onlineCount
		} else {
			statslog.Warn("Failed to get devices for quick stats", "error", err)
		}
	}

	// Get routes count
	if h.router != nil {
		routes := h.router.ListRoutes()
		stats.RoutesTotal = len(routes)
	}

	// Get total requests today
	if h.storage != nil {
		if totalRequests, err := h.storage.GetTotalRequestsToday(); err == nil {
			stats.RequestsToday = totalRequests
		} else {
			statslog.Warn("Failed to get total requests today", "error", err)
		}
	}

	if err := httputil.WriteJSON(w, http.StatusOK, stats); err != nil {
		statslog.Error("Error encoding JSON response", "error", err)
	}
}
