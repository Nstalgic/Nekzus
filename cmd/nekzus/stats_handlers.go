package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	apperrors "github.com/nstalgic/nekzus/internal/errors"
	"github.com/nstalgic/nekzus/internal/httputil"
	"github.com/nstalgic/nekzus/internal/storage"
)

// handleAdminStats returns aggregated system statistics
func (app *Application) handleAdminStats(w http.ResponseWriter, r *http.Request) {
	// Aggregate stats from various sources
	stats := make(map[string]interface{})

	// Routes count
	routes := app.managers.Router.ListRoutes()
	routesCount := len(routes)
	stats["routes"] = map[string]interface{}{
		"value":   routesCount,
		"trend":   fmt.Sprintf("%d active", routesCount),
		"trendUp": routesCount > 0,
	}

	// Devices count
	devicesCount := 0
	devicesOnline := 0
	if app.storage != nil {
		if devices, err := app.storage.ListDevices(); err == nil {
			devicesCount = len(devices)
			// Count online devices (seen in last 5 minutes)
			now := time.Now()
			for _, device := range devices {
				if !device.LastSeen.IsZero() {
					diffMins := int(now.Sub(device.LastSeen).Minutes())
					if diffMins < 5 {
						devicesOnline++
					}
				}
			}
		}
	}
	devicesTrend := "No devices"
	if devicesOnline > 0 {
		devicesTrend = fmt.Sprintf("%d online now", devicesOnline)
	} else if devicesCount > 0 {
		devicesTrend = fmt.Sprintf("%d paired", devicesCount)
	}
	stats["devices"] = map[string]interface{}{
		"value":   devicesCount,
		"trend":   devicesTrend,
		"trendUp": devicesOnline > 0,
	}

	// Discoveries count (pending proposals)
	discoveriesCount := 0
	if app.storage != nil {
		if proposals, err := app.storage.ListProposals(); err == nil {
			discoveriesCount = len(proposals)
		}
	}
	discoveriesTrend := "No pending"
	if discoveriesCount > 0 {
		discoveriesTrend = fmt.Sprintf("%d pending review", discoveriesCount)
	}
	stats["discoveries"] = map[string]interface{}{
		"value":   discoveriesCount,
		"trend":   discoveriesTrend,
		"trendUp": false,
	}

	// Requests count (from Prometheus metrics)
	requestsCount := 0
	requestsTrend := "No data"
	if app.metrics != nil {
		if total, err := app.metrics.GetHTTPRequestsTotal(); err == nil {
			requestsCount = int(total)
			if requestsCount > 0 {
				requestsTrend = fmt.Sprintf("%d total", requestsCount)
			}
		}
	}
	stats["requests"] = map[string]interface{}{
		"value":   requestsCount,
		"trend":   requestsTrend,
		"trendUp": requestsCount > 0,
	}

	if err := httputil.WriteJSON(w, http.StatusOK, stats); err != nil {
		log.Error("failed to encode json response", "error", err)
	}
}

// handleRecentActivity returns the last 10 activity events
func (app *Application) handleRecentActivity(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		apperrors.WriteJSON(w, apperrors.New("METHOD_NOT_ALLOWED", "Method not allowed", http.StatusMethodNotAllowed))
		return
	}

	// Check if pagination params are provided
	limitParam := r.URL.Query().Get("limit")
	offsetParam := r.URL.Query().Get("offset")
	usePagination := limitParam != "" || offsetParam != ""

	// Get all events
	allEvents := app.managers.Activity.Get()

	// If no pagination requested, return array for backward compatibility
	if !usePagination {
		if err := httputil.WriteJSON(w, http.StatusOK, allEvents); err != nil {
			log.Error("failed to encode json response", "error", err)
		}
		return
	}

	// Parse pagination parameters
	limit := parsePaginationParam(limitParam, -1)  // -1 = no limit
	offset := parsePaginationParam(offsetParam, 0) // 0 = start from beginning

	total := len(allEvents)

	// Apply pagination
	start := offset
	if start < 0 {
		start = 0
	}
	if start > total {
		start = total
	}

	end := total
	if limit > 0 {
		end = start + limit
		if end > total {
			end = total
		}
	} else if limit == 0 {
		// limit=0 means return empty list
		end = start
	}

	events := allEvents[start:end]

	// Return paginated response
	response := map[string]interface{}{
		"activities": events,
		"total":      total,
		"limit":      limit,
		"offset":     offset,
	}

	if err := httputil.WriteJSON(w, http.StatusOK, response); err != nil {
		log.Error("failed to encode json response", "error", err)
	}
}

// parsePaginationParam parses pagination query parameter with default value
func parsePaginationParam(value string, defaultValue int) int {
	if value == "" {
		return defaultValue
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 {
		return defaultValue
	}
	return parsed
}

// handleAuditLogs handles GET /api/v1/audit-logs
// Supports filtering by action and actor, plus pagination
func (app *Application) handleAuditLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		apperrors.WriteJSON(w, apperrors.New("METHOD_NOT_ALLOWED", "Method not allowed", http.StatusMethodNotAllowed))
		return
	}

	// Parse query parameters
	query := r.URL.Query()
	action := query.Get("action")
	actor := query.Get("actor")

	// Parse pagination parameters
	limit := 100 // default
	if limitStr := query.Get("limit"); limitStr != "" {
		if parsed, err := strconv.Atoi(limitStr); err == nil && parsed > 0 && parsed <= 1000 {
			limit = parsed
		}
	}

	offset := 0
	if offsetStr := query.Get("offset"); offsetStr != "" {
		if parsed, err := strconv.Atoi(offsetStr); err == nil && parsed >= 0 {
			offset = parsed
		}
	}

	// Retrieve audit logs based on filters
	var logs []storage.AuditEvent
	var err error

	if action != "" {
		// Filter by action
		logs, err = app.storage.ListAuditLogsByAction(storage.Action(action), limit, offset)
	} else if actor != "" {
		// Filter by actor
		logs, err = app.storage.ListAuditLogsByActor(actor, limit, offset)
	} else {
		// No filter - return all
		logs, err = app.storage.ListAuditLogs(limit, offset)
	}

	if err != nil {
		log.Error("failed to retrieve audit logs", "error", err)
		apperrors.WriteJSON(w, apperrors.Wrap(err, "AUDIT_LOGS_RETRIEVE_FAILED", "Failed to retrieve audit logs", http.StatusInternalServerError))
		return
	}

	// Return as JSON
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"logs":   logs,
		"limit":  limit,
		"offset": offset,
		"count":  len(logs),
	})
}
