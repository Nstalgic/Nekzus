package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	apperrors "github.com/nstalgic/nekzus/internal/errors"
	"github.com/nstalgic/nekzus/internal/httputil"
	"github.com/nstalgic/nekzus/internal/security"
	"github.com/nstalgic/nekzus/internal/types"
)

// handleListProposals returns all pending discovery proposals
func (app *Application) handleListProposals(w http.ResponseWriter, r *http.Request) {
	// Get proposals from storage if available
	var result []types.Proposal
	if app.storage != nil {
		proposals, err := app.storage.ListProposals()
		if err != nil {
			log.Error("failed to list proposals from storage", "error", err)
			apperrors.WriteJSON(w, apperrors.Wrap(err, "PROPOSALS_LIST_FAILED", "Failed to list discovery proposals", http.StatusInternalServerError))
			return
		}
		result = proposals
	} else {
		// Fallback to in-memory proposals from discovery
		proposals := app.services.Discovery.GetProposals()
		result = make([]types.Proposal, 0, len(proposals))
		for _, p := range proposals {
			result = append(result, *p)
		}
	}

	if err := httputil.WriteJSON(w, http.StatusOK, result); err != nil {
		log.Error("failed to encode json response", "error", err)
	}
}

// handleProposalActions handles approve/dismiss actions on proposals
func (app *Application) handleProposalActions(w http.ResponseWriter, r *http.Request) {
	// Extract proposal ID and action from path
	// Support both /api/v1/... (JWT protected) and /admin/api/v1/... (admin UI)
	path := r.URL.Path
	log.Debug("processing proposal action", "path", path)

	// Try admin prefix first
	if strings.HasPrefix(path, "/admin/api/v1/discovery/proposals/") {
		path = strings.TrimPrefix(path, "/admin/api/v1/discovery/proposals/")
	} else if strings.HasPrefix(path, "/api/v1/discovery/proposals/") {
		path = strings.TrimPrefix(path, "/api/v1/discovery/proposals/")
	} else {
		log.Debug("path doesn't match expected prefix", "path", r.URL.Path)
		http.NotFound(w, r)
		return
	}

	log.Debug("trimmed path", "path", path)
	parts := strings.Split(path, "/")
	log.Debug("path parts", "parts", parts, "count", len(parts))

	if len(parts) < 2 {
		log.Debug("not enough parts in path", "parts", parts)
		http.NotFound(w, r)
		return
	}

	proposalID := parts[0]
	action := parts[1]
	log.Debug("extracted proposal action", "proposal_id", proposalID, "action", action)

	switch action {
	case "approve":
		app.handleApproveProposal(w, r, proposalID)
	case "dismiss":
		app.handleDismissProposal(w, r, proposalID)
	default:
		log.Debug("unknown action", "action", action)
		http.NotFound(w, r)
	}
}

// getProposal retrieves a proposal by ID from storage or in-memory cache
func (app *Application) getProposal(proposalID string) (*types.Proposal, error) {
	if app.storage != nil {
		return app.storage.GetProposal(proposalID)
	}

	// Fallback to in-memory proposals from discovery
	proposals := app.services.Discovery.GetProposals()
	for _, p := range proposals {
		if p.ID == proposalID {
			return p, nil
		}
	}
	return nil, nil // Not found, but no error
}

// approveRequest represents the optional request body for approving a proposal
type approveRequest struct {
	Port int `json:"port,omitempty"` // Optional: specific port to use from AvailablePorts
}

// handleApproveProposal approves a discovery proposal
func (app *Application) handleApproveProposal(w http.ResponseWriter, r *http.Request, proposalID string) {
	// Parse optional request body for port selection
	var req approveRequest
	if r.Body != nil && r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			// Ignore decode errors - port selection is optional
			log.Debug("no port selection in request body", "error", err)
		}
	}

	// Get proposal from storage or in-memory cache
	proposal, err := app.getProposal(proposalID)
	if err != nil {
		log.Error("failed to fetch proposal", "proposal_id", proposalID, "error", err)
		apperrors.WriteJSON(w, apperrors.Wrap(err, "PROPOSAL_FETCH_FAILED", "Failed to fetch discovery proposal", http.StatusInternalServerError))
		return
	}

	if proposal == nil {
		// Proposal doesn't exist - it may have been already approved or dismissed
		// This is not an error; return success to make the operation idempotent
		if err := httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{
			"status":  "already_processed",
			"message": "Proposal has already been processed or does not exist",
			"id":      proposalID,
		}); err != nil {
			log.Error("failed to encode json response", "error", err)
		}
		return
	}

	// If a specific port was requested, validate and apply it
	if req.Port > 0 {
		if err := app.applyPortSelection(proposal, req.Port); err != nil {
			log.Warn("invalid port selection", "proposal_id", proposalID, "port", req.Port, "error", err)
			apperrors.WriteJSON(w, apperrors.New("INVALID_PORT", err.Error(), http.StatusBadRequest))
			return
		}
		log.Info("using selected port", "proposal_id", proposalID, "port", req.Port)
	}

	// Register the app (now returns error)
	if err := app.managers.Router.UpsertApp(proposal.SuggestedApp); err != nil {
		log.Error("failed to register app", "error", err)
		apperrors.WriteJSON(w, apperrors.Wrap(err, "APP_REGISTRATION_FAILED", "Failed to register application", http.StatusInternalServerError))
		return
	}

	// Register the route (now returns error)
	if err := app.managers.Router.UpsertRoute(proposal.SuggestedRoute); err != nil {
		log.Error("failed to register route", "error", err)
		apperrors.WriteJSON(w, apperrors.Wrap(err, "ROUTE_REGISTRATION_FAILED", "Failed to register route", http.StatusInternalServerError))
		return
	}

	// Remove from proposals (both storage and in-memory)
	// Use DismissProposal to prevent it from reappearing on next discovery scan
	if app.storage != nil {
		if err := app.storage.DeleteProposal(proposalID); err != nil {
			log.Warn("failed to delete proposal from storage", "error", err)
		}
	}
	app.services.Discovery.DismissProposal(proposalID)

	// Check for port exposure if this is a Docker discovery
	if security.ShouldCheckPortExposure(proposal.Source, app.dockerClient) {
		app.checkAndNotifyPortExposure(proposal)
	}

	// Publish event
	if app.managers.WebSocket != nil {
		app.managers.WebSocket.Broadcast(types.WebSocketMessage{
			Type: "app_registered",
			Data: map[string]interface{}{
				"appId":      proposal.SuggestedApp.ID,
				"appName":    proposal.SuggestedApp.Name,
				"proxyPath":  proposal.SuggestedRoute.PathBase,
				"proposalId": proposalID,
			},
			Timestamp: time.Now(),
		})
	}

	// Add to activity feed
	if app.managers.Activity != nil {
		app.managers.Activity.Add(types.ActivityEvent{
			ID:        "app_registered_" + proposal.SuggestedApp.ID,
			Type:      "app_registered",
			Icon:      "CheckCircle2",
			IconClass: "success",
			Message:   "Registered: " + proposal.SuggestedApp.Name,
			Timestamp: time.Now().UnixMilli(),
		})
	}

	log.Info("approved proposal", "proposal_id", proposalID, "app_id", proposal.SuggestedApp.ID, "route", proposal.SuggestedRoute.PathBase)

	if err := httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"status": "approved",
		"id":     proposalID,
		"app":    proposal.SuggestedApp,
		"route":  proposal.SuggestedRoute,
	}); err != nil {
		log.Error("failed to encode json response", "error", err)
	}
}

// checkAndNotifyPortExposure checks for port exposure and sends notifications
func (app *Application) checkAndNotifyPortExposure(proposal *types.Proposal) {
	// Extract container name/ID from the proposal's detected host
	containerID := security.ExtractContainerID(proposal.DetectedHost, proposal.Source)
	if containerID == "" {
		// Cannot determine container, skip check
		log.Debug("cannot extract container id from host", "host", proposal.DetectedHost)
		return
	}

	// Inspect container ports using Docker API
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	analysis, err := security.CheckContainerPortExposure(ctx, app.dockerClient, containerID)
	if err != nil {
		log.Warn("port exposure check failed", "app_id", proposal.SuggestedApp.ID, "error", err)
		return
	}

	// Log the result
	security.LogPortExposureCheck(proposal.SuggestedApp.ID, analysis)

	// Only notify if there's a risk
	if analysis.OverallRisk == security.RiskNone {
		return
	}

	// Send WebSocket notification
	if app.managers.WebSocket != nil {
		riskLevel := string(analysis.OverallRisk)
		bindings := make([]map[string]interface{}, 0, len(analysis.Bindings))
		for _, binding := range analysis.Bindings {
			bindings = append(bindings, map[string]interface{}{
				"port":      binding.Port,
				"hostPort":  binding.HostPort,
				"binding":   binding.HostIP,
				"riskLevel": string(binding.Risk),
				"message":   binding.Message,
			})
		}

		app.managers.WebSocket.PublishPortExposureWarning(
			proposal.SuggestedApp.ID,
			proposal.SuggestedApp.Name,
			riskLevel,
			analysis.Summary,
			bindings,
			analysis.Recommendations,
		)

		// Enqueue notification for offline devices
		if app.notificationQueue != nil && app.storage != nil {
			go app.enqueuePortExposureNotification(
				proposal.SuggestedApp.ID,
				proposal.SuggestedApp.Name,
				riskLevel,
				analysis.Summary,
				bindings,
				analysis.Recommendations,
			)
		}
	}

	// Add to activity feed
	if app.managers.Activity != nil {
		icon := "AlertTriangle"
		iconClass := "warning"
		if analysis.OverallRisk == security.RiskCritical {
			icon = "AlertOctagon"
			iconClass = "error"
		}

		app.managers.Activity.Add(types.ActivityEvent{
			ID:        "port_exposure_" + proposal.SuggestedApp.ID,
			Type:      "security_warning",
			Icon:      icon,
			IconClass: iconClass,
			Message:   fmt.Sprintf("Security: %s - %s", proposal.SuggestedApp.Name, analysis.Summary),
			Details:   strings.Join(analysis.Recommendations, "; "),
			Timestamp: time.Now().UnixMilli(),
		})
	}
}

// applyPortSelection validates and applies a user-selected port to the proposal.
// Updates the proposal's route and app endpoints to use the selected port.
func (app *Application) applyPortSelection(proposal *types.Proposal, selectedPort int) error {
	// Find the selected port in available ports
	var selectedOption *types.PortOption
	for _, opt := range proposal.AvailablePorts {
		if opt.Port == selectedPort {
			selectedOption = &opt
			break
		}
	}

	if selectedOption == nil {
		return fmt.Errorf("port %d is not in the available ports list", selectedPort)
	}

	// Update the proposal with the selected port
	proposal.DetectedPort = selectedOption.Port
	proposal.DetectedScheme = selectedOption.Scheme

	// Update the route target URL
	proposal.SuggestedRoute.To = fmt.Sprintf("%s://%s:%d",
		selectedOption.Scheme,
		proposal.DetectedHost,
		selectedOption.Port,
	)

	// Update app endpoint
	if proposal.SuggestedApp.Endpoints == nil {
		proposal.SuggestedApp.Endpoints = make(map[string]string)
	}
	proposal.SuggestedApp.Endpoints["lan"] = proposal.SuggestedRoute.To

	return nil
}

// handleDismissProposal dismisses a discovery proposal
func (app *Application) handleDismissProposal(w http.ResponseWriter, r *http.Request, proposalID string) {
	// Check if proposal exists
	proposal, err := app.getProposal(proposalID)
	if err != nil {
		log.Error("failed to fetch proposal", "proposal_id", proposalID, "error", err)
		apperrors.WriteJSON(w, apperrors.Wrap(err, "PROPOSAL_FETCH_FAILED", "Failed to fetch discovery proposal", http.StatusInternalServerError))
		return
	}

	if proposal == nil {
		// Proposal doesn't exist - it may have been already dismissed or approved
		// This is not an error; return success to make the operation idempotent
		if err := httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{
			"status":  "already_processed",
			"message": "Proposal has already been processed or does not exist",
			"id":      proposalID,
		}); err != nil {
			log.Error("failed to encode json response", "error", err)
		}
		return
	}

	// Remove from proposals (both storage and in-memory)
	// Use DismissProposal to prevent it from reappearing on next discovery scan
	if app.storage != nil {
		if err := app.storage.DeleteProposal(proposalID); err != nil {
			log.Warn("failed to delete proposal from storage", "error", err)
		}
	}
	app.services.Discovery.DismissProposal(proposalID)

	// Publish event
	if app.managers.WebSocket != nil {
		app.managers.WebSocket.Broadcast(types.WebSocketMessage{
			Type: "proposal_dismissed",
			Data: map[string]interface{}{
				"proposalId": proposalID,
			},
			Timestamp: time.Now(),
		})
	}

	// Add to activity feed
	if app.managers.Activity != nil {
		app.managers.Activity.Add(types.ActivityEvent{
			ID:        "proposal_dismissed_" + proposalID,
			Type:      "proposal_dismissed",
			Icon:      "X",
			IconClass: "",
			Message:   "Proposal dismissed: " + proposalID,
			Timestamp: time.Now().UnixMilli(),
		})
	}

	log.Info("dismissed proposal", "proposal_id", proposalID)

	if err := httputil.WriteJSON(w, http.StatusOK, map[string]string{
		"status": "dismissed",
		"id":     proposalID,
	}); err != nil {
		log.Error("failed to encode json response", "error", err)
	}
}

// handleRediscover triggers a fresh discovery scan by clearing dismissed and active proposals
func (app *Application) handleRediscover(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		apperrors.WriteJSON(w, apperrors.New("METHOD_NOT_ALLOWED", "Method not allowed", http.StatusMethodNotAllowed))
		return
	}

	// Clear dismissed and active proposals in discovery manager
	dismissedCount, activeCount := app.services.Discovery.Rediscover()

	// Clear proposals from storage if available
	if app.storage != nil {
		if err := app.storage.ClearProposals(); err != nil {
			log.Warn("failed to clear proposals from storage", "error", err)
		}
	}

	log.Info("rediscovery triggered", "dismissed_cleared", dismissedCount, "active_cleared", activeCount)

	// Publish event to notify clients
	if app.managers.WebSocket != nil {
		app.managers.WebSocket.Broadcast(types.WebSocketMessage{
			Type: "rediscovery_triggered",
			Data: map[string]interface{}{
				"dismissedCleared": dismissedCount,
				"activeCleared":    activeCount,
			},
			Timestamp: time.Now(),
		})
	}

	// Add to activity feed
	if app.managers.Activity != nil {
		app.managers.Activity.Add(types.ActivityEvent{
			ID:        "rediscovery_" + time.Now().Format("20060102150405"),
			Type:      "rediscovery_triggered",
			Icon:      "RefreshCw",
			IconClass: "",
			Message:   fmt.Sprintf("Discovery scan triggered (cleared %d dismissed, %d active proposals)", dismissedCount, activeCount),
			Timestamp: time.Now().UnixMilli(),
		})
	}

	if err := httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"status":           "success",
		"message":          "Rediscovery triggered. Discovery workers will scan for new services.",
		"dismissedCleared": dismissedCount,
		"activeCleared":    activeCount,
	}); err != nil {
		log.Error("failed to encode json response", "error", err)
	}
}

// enqueuePortExposureNotification enqueues port exposure notifications for all devices
func (app *Application) enqueuePortExposureNotification(
	appID, appName, riskLevel, summary string,
	bindings []map[string]interface{},
	recommendations []string,
) {
	// Get all devices from storage
	devices, err := app.storage.ListDevices()
	if err != nil {
		log.Warn("failed to get devices for port exposure notification", "error", err)
		return
	}

	// Create notification payload
	payload := map[string]interface{}{
		"title":           "Security Alert: Port Exposure Detected",
		"body":            summary,
		"appId":           appID,
		"appName":         appName,
		"riskLevel":       riskLevel,
		"bindings":        bindings,
		"recommendations": recommendations,
		"timestamp":       time.Now().Unix(),
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		log.Warn("failed to marshal port exposure notification payload", "error", err)
		return
	}

	// Enqueue notification for each device
	successCount := 0
	for _, device := range devices {
		// TTL: 24 hours for security alerts (longer than health alerts)
		// MaxRetries: 5 attempts (more retries for security issues)
		err := app.notificationQueue.Enqueue(
			device.ID,
			"port_exposure_warning",
			json.RawMessage(payloadJSON),
			24*time.Hour,
			5,
		)
		if err != nil {
			log.Warn("failed to enqueue port exposure notification", "device_id", device.ID, "error", err)
		} else {
			successCount++
		}
	}

	if successCount > 0 {
		log.Info("enqueued port exposure notification", "devices", successCount, "app_name", appName, "risk_level", riskLevel)
	}
}
