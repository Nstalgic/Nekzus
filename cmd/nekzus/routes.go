package main

import (
	"log/slog"
	"net/http"
	"strings"

	apperrors "github.com/nstalgic/nekzus/internal/errors"
	"github.com/nstalgic/nekzus/internal/health"
	"github.com/nstalgic/nekzus/internal/metrics"
	"github.com/nstalgic/nekzus/internal/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// RouteBuilder helps organize route registration by feature group
type RouteBuilder struct {
	app          *Application
	mux          *http.ServeMux
	ipAuth       func(http.Handler) http.Handler
	apiKeyAuth   func(http.Handler) http.Handler
	strictJWT    func(http.Handler) http.Handler
	combinedAuth func(http.Handler) http.Handler
}

// NewRouteBuilder creates a new RouteBuilder for the application
func NewRouteBuilder(app *Application) *RouteBuilder {
	mux := http.NewServeMux()

	// Create base auth middleware
	rawIPAuth := middleware.NewIPBasedAuth(app.services.Auth, app.storage, app.metrics)
	rawAPIKeyAuth := middleware.NewAPIKeyAuth(app.storage, app.metrics)
	rawStrictJWT := middleware.NewStrictJWTAuth(app.services.Auth, app.storage, app.metrics)

	// Request tracker runs AFTER auth so device ID is available in context
	tracker := middleware.RequestTracker(app.storage)

	// Wrap auth middlewares: auth sets device ID in context, then tracker can read it
	ipAuth := func(next http.Handler) http.Handler {
		return rawIPAuth(tracker(next))
	}
	apiKeyAuth := func(next http.Handler) http.Handler {
		return rawAPIKeyAuth(tracker(next))
	}
	strictJWT := func(next http.Handler) http.Handler {
		return rawStrictJWT(tracker(next))
	}
	combinedAuth := func(next http.Handler) http.Handler {
		return rawAPIKeyAuth(rawIPAuth(tracker(next)))
	}

	return &RouteBuilder{
		app:          app,
		mux:          mux,
		ipAuth:       ipAuth,
		apiKeyAuth:   apiKeyAuth,
		strictJWT:    strictJWT,
		combinedAuth: combinedAuth,
	}
}

// Build constructs the final HTTP handler with all middleware applied
func (rb *RouteBuilder) Build() http.Handler {
	// Register all route groups
	rb.registerHealthRoutes()
	rb.registerAuthRoutes()
	rb.registerWebSocketRoutes()
	rb.registerAdminRoutes()
	rb.registerAppRoutes()
	rb.registerDiscoveryRoutes()
	rb.registerDeviceRoutes()
	rb.registerAPIKeyRoutes()
	rb.registerStatsRoutes()
	rb.registerWebhookRoutes()
	rb.registerCertificateRoutes()
	rb.registerBackupRoutes()
	rb.registerToolboxRoutes()
	rb.registerScriptsRoutes()
	rb.registerFederationRoutes()
	rb.registerSystemRoutes()
	rb.registerServiceHealthRoutes()
	rb.registerSessionCookiesRoutes()
	rb.registerNotificationRoutes()
	rb.registerContainerRoutes()
	rb.registerProxyRoutes()
	rb.registerWebUIRoutes()

	// Apply middleware stack (security -> version -> metrics -> logging)
	// Request tracking is applied per-route after auth (see NewRouteBuilder)
	handler := middleware.SecurityHeaders(rb.mux)
	handler = middleware.APIVersion(rb.app.version)(handler)
	handler = metrics.HTTPMiddleware(rb.app.metrics)(handler)
	handler = logMiddleware(handler)

	// Wrap with subdomain routing if configured
	if rb.app.config.Server.BaseDomain != "" {
		handler = rb.app.subdomainMiddleware(handler)
	}

	return handler
}

// registerHealthRoutes registers health check endpoints
func (rb *RouteBuilder) registerHealthRoutes() {
	app := rb.app

	// Metrics endpoint (no auth required, with rate limiting, hot-reloadable)
	rb.mux.Handle("/metrics", app.createDynamicMetricsHandler())

	// Health check endpoints (no auth required, with rate limiting - for Kubernetes/monitoring)
	healthHandler := health.NewHandler(app.services.Health)
	rb.mux.Handle("/healthz", middleware.RateLimit(app.limiters.Health)(http.HandlerFunc(healthHandler.HandleHealth)))
	rb.mux.Handle("/livez", middleware.RateLimit(app.limiters.Health)(http.HandlerFunc(healthHandler.HandleLiveness)))
	rb.mux.Handle("/readyz", middleware.RateLimit(app.limiters.Health)(http.HandlerFunc(healthHandler.HandleReadiness)))
	rb.mux.Handle("/api/v1/healthz", middleware.RateLimit(app.limiters.Health)(http.HandlerFunc(app.handleHealthz)))
}

// registerAuthRoutes registers authentication endpoints
func (rb *RouteBuilder) registerAuthRoutes() {
	app := rb.app

	// QR code generation for mobile pairing (admin-only, rate limited)
	rb.mux.Handle("/api/v1/auth/qr", middleware.RateLimit(app.limiters.QR)(rb.ipAuth(http.HandlerFunc(app.handlers.QR.HandleQRCode))))
	rb.mux.Handle("/api/v1/auth/pair", middleware.RateLimit(app.limiters.Auth)(http.HandlerFunc(app.handlePair)))
	rb.mux.Handle("/api/v1/auth/refresh", middleware.RateLimit(app.limiters.Auth)(http.HandlerFunc(app.handleRefresh)))
	rb.mux.Handle("/pair", middleware.RateLimit(app.limiters.QR)(http.HandlerFunc(app.handlePairWebUI)))

	// V2 pairing flow - short code redemption (POST for security)
	if app.handlers.Pairing != nil {
		rb.mux.Handle("POST /api/v1/pair", middleware.RateLimit(app.limiters.Auth)(http.HandlerFunc(app.handlers.Pairing.HandleRedeemPairingCode)))
		rb.mux.HandleFunc("GET /api/v1/auth/qr/status", app.handlers.Pairing.HandleCodeStatus)
	}

	// Username/password authentication endpoints (for web admin dashboard)
	rb.mux.Handle("/api/v1/auth/setup-status", middleware.RateLimit(app.limiters.Auth)(http.HandlerFunc(app.handleSetupStatus)))
	rb.mux.Handle("/api/v1/auth/setup", middleware.RateLimit(app.limiters.Auth)(http.HandlerFunc(app.handleSetup)))
	rb.mux.Handle("/api/v1/auth/login", middleware.RateLimit(app.limiters.Auth)(http.HandlerFunc(app.handleLogin)))
	rb.mux.Handle("/api/v1/auth/me", middleware.RateLimit(app.limiters.Auth)(http.HandlerFunc(app.handleAuthMe)))
	rb.mux.Handle("/api/v1/auth/logout", middleware.RateLimit(app.limiters.Auth)(http.HandlerFunc(app.handleLogout)))
}

// registerWebSocketRoutes registers WebSocket endpoint
func (rb *RouteBuilder) registerWebSocketRoutes() {
	app := rb.app
	rb.mux.Handle("/api/v1/ws", middleware.RateLimit(app.limiters.WebSocket)(http.HandlerFunc(app.handleWebSocket)))
}

// registerAdminRoutes registers admin-only endpoints
func (rb *RouteBuilder) registerAdminRoutes() {
	app := rb.app

	// Instance info
	rb.mux.Handle("/api/v1/admin/info", rb.ipAuth(http.HandlerFunc(app.handleAdminInfo)))

	// Admin device management (IP-based auth for localhost admin UI)
	rb.mux.Handle("/api/v1/admin/devices", rb.ipAuth(http.HandlerFunc(app.handleAdminDevices)))
	rb.mux.Handle("/api/v1/admin/devices/", rb.ipAuth(http.HandlerFunc(app.handleAdminDeviceActions)))

	// WebSocket debug endpoint (shows connected clients)
	rb.mux.Handle("/api/v1/admin/websocket/clients", rb.ipAuth(http.HandlerFunc(app.handleWebSocketClients)))

	// WebSocket force disconnect endpoint (for debugging stale connections)
	rb.mux.Handle("/api/v1/admin/websocket/disconnect/", rb.ipAuth(http.HandlerFunc(app.handleWebSocketDisconnect)))
}

// registerAppRoutes registers app catalog and routes management
func (rb *RouteBuilder) registerAppRoutes() {
	app := rb.app

	// App catalog
	rb.mux.Handle("/api/v1/apps", rb.ipAuth(http.HandlerFunc(app.handleListApps)))

	// App favicon endpoint
	rb.mux.Handle("/api/v1/apps/", rb.ipAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Extract appID from path: /api/v1/apps/{appId}/favicon
		path := strings.TrimPrefix(r.URL.Path, "/api/v1/apps/")
		if !strings.HasSuffix(path, "/favicon") {
			apperrors.WriteJSON(w, apperrors.New("NOT_FOUND", "Not found", http.StatusNotFound))
			return
		}
		appID := strings.TrimSuffix(path, "/favicon")
		if appID == "" {
			apperrors.WriteJSON(w, apperrors.New("INVALID_APP_ID", "App ID required", http.StatusBadRequest))
			return
		}
		app.handlers.Favicon.HandleFavicon(w, r, appID)
	})))

	// Routes management
	rb.mux.Handle("/api/v1/routes", rb.ipAuth(http.HandlerFunc(app.handleListRoutes)))
	rb.mux.Handle("/api/v1/routes/", rb.ipAuth(http.HandlerFunc(app.handleRouteActions)))
}

// registerDiscoveryRoutes registers discovery proposal endpoints
func (rb *RouteBuilder) registerDiscoveryRoutes() {
	app := rb.app

	// Discovery endpoints - IP-based auth (allow localhost/web UI)
	rb.mux.Handle("/api/v1/discovery/proposals", rb.ipAuth(http.HandlerFunc(app.handleListProposals)))
	rb.mux.Handle("/api/v1/discovery/proposals/", rb.ipAuth(http.HandlerFunc(app.handleProposalActions)))
	rb.mux.Handle("/api/v1/discovery/rediscover", rb.ipAuth(http.HandlerFunc(app.handleRediscover)))
}

// registerDeviceRoutes registers device management endpoints
func (rb *RouteBuilder) registerDeviceRoutes() {
	app := rb.app

	// Device management (IP-based auth for localhost, JWT for external, with rate limiting)
	rb.mux.Handle("/api/v1/devices", middleware.RateLimit(app.limiters.Device)(rb.ipAuth(http.HandlerFunc(app.handleDevices))))
	rb.mux.Handle("/api/v1/devices/", middleware.RateLimit(app.limiters.Device)(rb.ipAuth(http.HandlerFunc(app.handleDeviceActions))))
}

// registerAPIKeyRoutes registers API key management endpoints
func (rb *RouteBuilder) registerAPIKeyRoutes() {
	app := rb.app

	// API key management (IP-based auth for localhost admin UI)
	rb.mux.Handle("/api/v1/apikeys", rb.ipAuth(http.HandlerFunc(app.handleAPIKeys)))
	rb.mux.Handle("/api/v1/apikeys/", rb.ipAuth(http.HandlerFunc(app.handleAPIKeyActions)))
}

// registerStatsRoutes registers stats and activity endpoints
func (rb *RouteBuilder) registerStatsRoutes() {
	app := rb.app

	// Stats and activity
	rb.mux.Handle("/api/v1/stats", rb.ipAuth(http.HandlerFunc(app.handleAdminStats)))
	rb.mux.Handle("/api/v1/activity/recent", rb.ipAuth(http.HandlerFunc(app.handleRecentActivity)))

	// Metrics dashboard (aggregated Prometheus metrics for frontend)
	rb.mux.Handle("/api/v1/metrics/dashboard", rb.ipAuth(http.HandlerFunc(app.handlers.MetricsDashboard.HandleMetricsDashboard)))

	// Audit logs (strict JWT auth - sensitive administrative data)
	rb.mux.Handle("/api/v1/audit-logs", middleware.RateLimit(app.limiters.Device)(rb.strictJWT(http.HandlerFunc(app.handleAuditLogs))))
}

// registerWebhookRoutes registers webhook endpoints
func (rb *RouteBuilder) registerWebhookRoutes() {
	app := rb.app

	// Webhooks (supports both API key and JWT auth for external integrations)
	rb.mux.Handle("/api/v1/webhooks/activity", rb.combinedAuth(http.HandlerFunc(app.handleWebhookActivity)))
	rb.mux.Handle("/api/v1/webhooks/notify", rb.combinedAuth(http.HandlerFunc(app.handleWebhookNotify)))
}

// registerCertificateRoutes registers certificate management endpoints
func (rb *RouteBuilder) registerCertificateRoutes() {
	app := rb.app

	// Certificate management (IP-based auth for localhost admin UI)
	rb.mux.Handle("/api/v1/certificates/generate", rb.ipAuth(http.HandlerFunc(app.handleGenerateCertificate)))
	rb.mux.Handle("/api/v1/certificates/renew", rb.ipAuth(http.HandlerFunc(app.handleRenewCertificate)))
	rb.mux.Handle("/api/v1/certificates/replace", rb.ipAuth(http.HandlerFunc(app.handleReplaceCertificate)))
	rb.mux.Handle("/api/v1/certificates/suggest", rb.ipAuth(http.HandlerFunc(app.handleSuggestCertDomains)))
	rb.mux.Handle("/api/v1/certificates", rb.ipAuth(http.HandlerFunc(app.handleListCertificates)))
	rb.mux.Handle("/api/v1/certificates/", rb.ipAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			app.handleGetCertificate(w, r)
		case http.MethodDelete:
			app.handleDeleteCertificate(w, r)
		default:
			apperrors.WriteJSON(w, apperrors.New("METHOD_NOT_ALLOWED", "Method not allowed", http.StatusMethodNotAllowed))
		}
	})))
}

// registerBackupRoutes registers backup and disaster recovery endpoints
func (rb *RouteBuilder) registerBackupRoutes() {
	app := rb.app

	// Only register if backup handler is available
	if app.handlers.Backup == nil {
		return
	}

	rb.mux.Handle("/api/v1/backups", rb.ipAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			app.handlers.Backup.ListBackups(w, r)
		case http.MethodPost:
			app.handlers.Backup.CreateBackup(w, r)
		default:
			apperrors.WriteJSON(w, apperrors.New("METHOD_NOT_ALLOWED", "Method not allowed", http.StatusMethodNotAllowed))
		}
	})))

	rb.mux.Handle("/api/v1/backups/", rb.ipAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Parse backup ID from path
		path := strings.TrimPrefix(r.URL.Path, "/api/v1/backups/")
		if strings.Contains(path, "/") {
			// Handle sub-paths like /backups/{id}/restore
			parts := strings.SplitN(path, "/", 2)
			if len(parts) == 2 && parts[1] == "restore" {
				if r.Method == http.MethodPost {
					app.handlers.Backup.RestoreBackup(w, r)
				} else {
					apperrors.WriteJSON(w, apperrors.New("METHOD_NOT_ALLOWED", "Method not allowed", http.StatusMethodNotAllowed))
				}
				return
			}
		}
		// Handle /backups/{id} endpoints
		switch r.Method {
		case http.MethodGet:
			app.handlers.Backup.GetBackup(w, r)
		case http.MethodDelete:
			app.handlers.Backup.DeleteBackup(w, r)
		default:
			apperrors.WriteJSON(w, apperrors.New("METHOD_NOT_ALLOWED", "Method not allowed", http.StatusMethodNotAllowed))
		}
	})))

	// Backup scheduler management
	rb.mux.Handle("/api/v1/backups/scheduler/status", rb.ipAuth(http.HandlerFunc(app.handlers.Backup.GetSchedulerStatus)))
	rb.mux.Handle("/api/v1/backups/scheduler/trigger", rb.ipAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			app.handlers.Backup.TriggerBackup(w, r)
		} else {
			apperrors.WriteJSON(w, apperrors.New("METHOD_NOT_ALLOWED", "Method not allowed", http.StatusMethodNotAllowed))
		}
	})))
}

// registerToolboxRoutes registers toolbox endpoints
func (rb *RouteBuilder) registerToolboxRoutes() {
	app := rb.app

	if app.handlers.Toolbox != nil {
		// List all services in the toolbox catalog
		rb.mux.Handle("GET /api/v1/toolbox/services", rb.ipAuth(http.HandlerFunc(app.handlers.Toolbox.ListServices)))

		// Get specific service details
		rb.mux.Handle("GET /api/v1/toolbox/services/{id}", rb.ipAuth(http.HandlerFunc(app.handlers.Toolbox.GetService)))

		// Deploy a service from the toolbox
		rb.mux.Handle("POST /api/v1/toolbox/deploy", rb.ipAuth(http.HandlerFunc(app.handlers.Toolbox.DeployService)))

		// List all deployments
		rb.mux.Handle("GET /api/v1/toolbox/deployments", rb.ipAuth(http.HandlerFunc(app.handlers.Toolbox.ListDeployments)))

		// Get deployment status
		rb.mux.Handle("GET /api/v1/toolbox/deployments/{id}", rb.ipAuth(http.HandlerFunc(app.handlers.Toolbox.GetDeployment)))

		// Remove deployment
		rb.mux.Handle("DELETE /api/v1/toolbox/deployments/{id}", rb.ipAuth(http.HandlerFunc(app.handlers.Toolbox.RemoveDeployment)))
	} else {
		// Register stub endpoints that return empty data when toolbox is disabled
		rb.mux.Handle("GET /api/v1/toolbox/services", rb.ipAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"services":[],"count":0,"enabled":false}`))
		})))

		rb.mux.Handle("GET /api/v1/toolbox/deployments", rb.ipAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"deployments":[],"count":0,"enabled":false}`))
		})))
	}
}

// registerScriptsRoutes registers script execution endpoints
func (rb *RouteBuilder) registerScriptsRoutes() {
	app := rb.app

	// Only register if scripts handler is available
	if app.handlers.Scripts == nil {
		// Register stub endpoints that return empty data when scripts is disabled
		rb.mux.Handle("GET /api/v1/scripts", rb.ipAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"scripts":[],"count":0,"enabled":false}`))
		})))
		rb.mux.Handle("GET /api/v1/scripts/available", rb.ipAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"scripts":[],"enabled":false}`))
		})))
		rb.mux.Handle("GET /api/v1/executions", rb.ipAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"executions":[],"count":0,"enabled":false}`))
		})))
		rb.mux.Handle("GET /api/v1/workflows", rb.ipAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"workflows":[],"count":0,"enabled":false}`))
		})))
		rb.mux.Handle("GET /api/v1/schedules", rb.ipAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"schedules":[],"count":0,"enabled":false}`))
		})))
		return
	}

	h := app.handlers.Scripts

	// Scripts CRUD
	rb.mux.Handle("GET /api/v1/scripts", rb.ipAuth(http.HandlerFunc(h.ListScripts)))
	rb.mux.Handle("POST /api/v1/scripts", rb.ipAuth(http.HandlerFunc(h.RegisterScript)))
	rb.mux.Handle("GET /api/v1/scripts/available", rb.ipAuth(http.HandlerFunc(h.ListAvailableScripts)))
	rb.mux.Handle("GET /api/v1/scripts/{id}", rb.ipAuth(http.HandlerFunc(h.GetScript)))
	rb.mux.Handle("PUT /api/v1/scripts/{id}", rb.ipAuth(http.HandlerFunc(h.UpdateScript)))
	rb.mux.Handle("DELETE /api/v1/scripts/{id}", rb.ipAuth(http.HandlerFunc(h.DeleteScript)))

	// Script execution
	rb.mux.Handle("POST /api/v1/scripts/{id}/execute", rb.ipAuth(http.HandlerFunc(h.ExecuteScript)))
	rb.mux.Handle("POST /api/v1/scripts/{id}/dry-run", rb.ipAuth(http.HandlerFunc(h.DryRunScript)))

	// Executions
	rb.mux.Handle("GET /api/v1/executions", rb.ipAuth(http.HandlerFunc(h.ListExecutions)))
	rb.mux.Handle("GET /api/v1/executions/{id}", rb.ipAuth(http.HandlerFunc(h.GetExecution)))

	// Workflows
	rb.mux.Handle("GET /api/v1/workflows", rb.ipAuth(http.HandlerFunc(h.ListWorkflows)))
	rb.mux.Handle("POST /api/v1/workflows", rb.ipAuth(http.HandlerFunc(h.CreateWorkflow)))
	rb.mux.Handle("GET /api/v1/workflows/{id}", rb.ipAuth(http.HandlerFunc(h.GetWorkflow)))
	rb.mux.Handle("DELETE /api/v1/workflows/{id}", rb.ipAuth(http.HandlerFunc(h.DeleteWorkflow)))

	// Schedules
	rb.mux.Handle("GET /api/v1/schedules", rb.ipAuth(http.HandlerFunc(h.ListSchedules)))
	rb.mux.Handle("POST /api/v1/schedules", rb.ipAuth(http.HandlerFunc(h.CreateSchedule)))
	rb.mux.Handle("GET /api/v1/schedules/{id}", rb.ipAuth(http.HandlerFunc(h.GetSchedule)))
	rb.mux.Handle("DELETE /api/v1/schedules/{id}", rb.ipAuth(http.HandlerFunc(h.DeleteSchedule)))
}

// registerFederationRoutes registers federation P2P endpoints
func (rb *RouteBuilder) registerFederationRoutes() {
	app := rb.app

	// Only register if federation is enabled
	if app.managers.Peers == nil {
		return
	}

	// List all peers in the federation
	rb.mux.Handle("GET /api/v1/federation/peers", rb.ipAuth(http.HandlerFunc(app.handleListPeers)))

	// Get specific peer details
	rb.mux.Handle("GET /api/v1/federation/peers/{id}", rb.ipAuth(http.HandlerFunc(app.handleGetPeer)))

	// Remove/block a peer from the federation
	rb.mux.Handle("DELETE /api/v1/federation/peers/{id}", rb.ipAuth(http.HandlerFunc(app.handleRemovePeer)))

	// Trigger full catalog sync with peers
	rb.mux.Handle("POST /api/v1/federation/sync", rb.ipAuth(http.HandlerFunc(app.handleTriggerSync)))

	// Get federation status and health
	rb.mux.Handle("GET /api/v1/federation/status", rb.ipAuth(http.HandlerFunc(app.handleFederationStatus)))
}

// registerSystemRoutes registers system resource endpoints
func (rb *RouteBuilder) registerSystemRoutes() {
	app := rb.app

	// System resources - accessible by both web UI (IP-based) and mobile (JWT validated)
	// Uses IP-based auth: validates JWT signature/expiration if provided, allows local without JWT.
	// Note: Does not check device revocation status (acceptable for read-only system metrics).
	rb.mux.Handle("/api/v1/system/resources", middleware.RateLimit(app.limiters.Container)(rb.ipAuth(http.HandlerFunc(app.handlers.System.HandleSystemResources))))

	// Quick stats for mobile/widgets (strict JWT auth for mobile, with rate limiting)
	rb.mux.Handle("/api/v1/stats/quick", middleware.RateLimit(app.limiters.Container)(rb.strictJWT(http.HandlerFunc(app.handlers.Stats.HandleQuickStats))))
}

// registerServiceHealthRoutes registers per-service health check endpoints
func (rb *RouteBuilder) registerServiceHealthRoutes() {
	app := rb.app

	// Per-service health check (IP-based auth for web UI, JWT for mobile)
	// GET  /api/v1/services/{appId}/health - Full health info (JSON)
	// HEAD /api/v1/services/{appId}/health - Lightweight check (headers only)
	rb.mux.Handle("/api/v1/services/", middleware.RateLimit(app.limiters.Health)(rb.ipAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Extract appID from path: /api/v1/services/{appId}/health
		path := strings.TrimPrefix(r.URL.Path, "/api/v1/services/")
		if !strings.HasSuffix(path, "/health") {
			apperrors.WriteJSON(w, apperrors.New("NOT_FOUND", "Not found", http.StatusNotFound))
			return
		}
		appID := strings.TrimSuffix(path, "/health")
		if appID == "" {
			apperrors.WriteJSON(w, apperrors.New("INVALID_APP_ID", "App ID required", http.StatusBadRequest))
			return
		}
		app.handlers.ServiceHealth.HandleServiceHealth(w, r, appID)
	}))))
}

// registerSessionCookiesRoutes registers session cookie management endpoints for mobile webview persistence
func (rb *RouteBuilder) registerSessionCookiesRoutes() {
	app := rb.app

	// Only register if handler is available
	if app.handlers.SessionCookies == nil {
		return
	}

	// Session cookies management (strict JWT auth - for mobile app only)
	// GET /api/v1/session-cookies - List stored session summaries (no cookie values)
	// DELETE /api/v1/session-cookies - Clear all sessions for the device
	rb.mux.Handle("/api/v1/session-cookies", rb.strictJWT(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			app.handlers.SessionCookies.List(w, r)
		case http.MethodDelete:
			app.handlers.SessionCookies.DeleteAll(w, r)
		default:
			apperrors.WriteJSON(w, apperrors.New("METHOD_NOT_ALLOWED", "Method not allowed", http.StatusMethodNotAllowed))
		}
	})))

	// DELETE /api/v1/session-cookies/{appId} - Clear sessions for specific app
	rb.mux.Handle("/api/v1/session-cookies/", rb.strictJWT(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			apperrors.WriteJSON(w, apperrors.New("METHOD_NOT_ALLOWED", "Method not allowed", http.StatusMethodNotAllowed))
			return
		}

		// Extract appId from path
		appID := strings.TrimPrefix(r.URL.Path, "/api/v1/session-cookies/")
		if appID == "" {
			apperrors.WriteJSON(w, apperrors.New("INVALID_APP_ID", "App ID required", http.StatusBadRequest))
			return
		}

		app.handlers.SessionCookies.DeleteApp(w, r, appID)
	})))
}

// registerNotificationRoutes registers notification queue management endpoints
func (rb *RouteBuilder) registerNotificationRoutes() {
	app := rb.app

	// Only register if handler is available
	if app.handlers.Notifications == nil {
		return
	}

	h := app.handlers.Notifications

	// List notifications with filters (IP-based auth for web UI)
	// GET /api/v1/notifications?status=pending&device_id=xxx&type=xxx&limit=50&offset=0
	rb.mux.Handle("GET /api/v1/notifications", rb.ipAuth(http.HandlerFunc(h.HandleListNotifications)))

	// Get notification queue statistics (IP-based auth for web UI)
	// GET /api/v1/notifications/stats
	rb.mux.Handle("GET /api/v1/notifications/stats", rb.ipAuth(http.HandlerFunc(h.HandleGetNotificationStats)))

	// Get stale notifications grouped by device (IP-based auth for web UI)
	// GET /api/v1/notifications/stale
	rb.mux.Handle("GET /api/v1/notifications/stale", rb.ipAuth(http.HandlerFunc(h.HandleGetStaleNotifications)))

	// Bulk retry notifications (IP-based auth for web UI)
	// POST /api/v1/notifications/retry
	rb.mux.Handle("POST /api/v1/notifications/retry", rb.ipAuth(http.HandlerFunc(h.HandleBulkRetryNotifications)))

	// Clear all delivered notifications (IP-based auth for web UI)
	// DELETE /api/v1/notifications/delivered
	rb.mux.Handle("DELETE /api/v1/notifications/delivered", rb.ipAuth(http.HandlerFunc(h.HandleClearDeliveredNotifications)))

	// Dismiss all notifications for a device (IP-based auth for web UI)
	// DELETE /api/v1/notifications/device/{deviceId}
	rb.mux.Handle("DELETE /api/v1/notifications/device/", rb.ipAuth(http.HandlerFunc(h.HandleDismissDeviceNotifications)))

	// Individual notification actions (IP-based auth for web UI)
	// DELETE /api/v1/notifications/{id} - Dismiss notification
	// POST /api/v1/notifications/{id}/retry - Retry notification
	rb.mux.Handle("/api/v1/notifications/", rb.ipAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case strings.HasSuffix(path, "/retry") && r.Method == http.MethodPost:
			h.HandleRetryNotification(w, r)
		case r.Method == http.MethodDelete:
			h.HandleDismissNotification(w, r)
		default:
			apperrors.WriteJSON(w, apperrors.New("METHOD_NOT_ALLOWED", "Method not allowed", http.StatusMethodNotAllowed))
		}
	})))
}

// registerContainerRoutes registers container management endpoints
func (rb *RouteBuilder) registerContainerRoutes() {
	app := rb.app

	// Only register if Docker client is available
	if app.handlers.Container == nil {
		return
	}

	// List all containers (IP-based auth - allow localhost/web UI)
	rb.mux.Handle("/api/v1/containers", middleware.RateLimit(app.limiters.Container)(rb.ipAuth(http.HandlerFunc(app.handlers.Container.HandleListContainers))))

	// Bulk operations (IP-based auth - allow localhost/web UI)
	rb.mux.Handle("/api/v1/containers/restart-all", middleware.RateLimit(app.limiters.Container)(rb.ipAuth(http.HandlerFunc(app.handlers.Container.HandleRestartAll))))
	rb.mux.Handle("/api/v1/containers/stop-all", middleware.RateLimit(app.limiters.Container)(rb.ipAuth(http.HandlerFunc(app.handlers.Container.HandleStopAll))))
	rb.mux.Handle("/api/v1/containers/start-all", middleware.RateLimit(app.limiters.Container)(rb.ipAuth(http.HandlerFunc(app.handlers.Container.HandleStartAll))))

	// Batch container stats (IP-based auth - allow localhost/web UI)
	rb.mux.Handle("/api/v1/containers/stats", middleware.RateLimit(app.limiters.Container)(rb.ipAuth(http.HandlerFunc(app.handlers.Container.HandleBatchContainerStats))))

	// Batch export (IP-based auth - allow localhost/web UI)
	if app.handlers.Export != nil {
		rb.mux.Handle("/api/v1/containers/batch/export/preview", middleware.RateLimit(app.limiters.Container)(rb.ipAuth(http.HandlerFunc(app.handlers.Export.HandleBatchPreviewExport))))
		rb.mux.Handle("/api/v1/containers/batch/export", middleware.RateLimit(app.limiters.Container)(rb.ipAuth(http.HandlerFunc(app.handlers.Export.HandleBatchExport))))
	}

	// Batch operations on specific containers (IP-based auth - allow localhost/web UI)
	rb.mux.Handle("/api/v1/containers/batch", middleware.RateLimit(app.limiters.Container)(rb.ipAuth(http.HandlerFunc(app.handlers.Container.HandleBatchOperation))))

	// Container operations (start, stop, restart, inspect, stats, export) - IP-based auth
	rb.mux.Handle("/api/v1/containers/", middleware.RateLimit(app.limiters.Container)(rb.ipAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Route to appropriate handler based on path suffix
		path := r.URL.Path
		switch {
		case strings.HasSuffix(path, "/start"):
			app.handlers.Container.HandleStartContainer(w, r)
		case strings.HasSuffix(path, "/stop"):
			app.handlers.Container.HandleStopContainer(w, r)
		case strings.HasSuffix(path, "/restart"):
			app.handlers.Container.HandleRestartContainer(w, r)
		case strings.HasSuffix(path, "/stats"):
			app.handlers.Container.HandleContainerStats(w, r)
		case strings.HasSuffix(path, "/export/preview"):
			if app.handlers.Export != nil {
				app.handlers.Export.HandlePreviewExport(w, r)
			} else {
				apperrors.WriteJSON(w, apperrors.New("EXPORT_UNAVAILABLE", "Export feature not available", http.StatusServiceUnavailable))
			}
		case strings.HasSuffix(path, "/export"):
			if app.handlers.Export != nil {
				app.handlers.Export.HandleExportContainer(w, r)
			} else {
				apperrors.WriteJSON(w, apperrors.New("EXPORT_UNAVAILABLE", "Export feature not available", http.StatusServiceUnavailable))
			}
		default:
			// Inspect endpoint: /api/v1/containers/{id}
			app.handlers.Container.HandleInspectContainer(w, r)
		}
	}))))
}

// registerProxyRoutes registers reverse proxy endpoint
func (rb *RouteBuilder) registerProxyRoutes() {
	app := rb.app
	rb.mux.Handle("/apps/", rb.ipAuth(http.HandlerFunc(app.handleProxy)))
}

// registerWebUIRoutes registers web UI static file serving
func (rb *RouteBuilder) registerWebUIRoutes() {
	app := rb.app

	if app.isWebUIAvailable() {
		rb.mux.Handle("/", app.serveWebUI())
		slog.Info("Web UI: enabled (embedded)")
	} else {
		slog.Info("Web UI: not available (run 'make build-web' to build)")
	}
}

// createDynamicMetricsHandler creates a metrics handler that can be dynamically enabled/disabled
func (app *Application) createDynamicMetricsHandler() http.Handler {
	// Wrap the actual metrics handler with rate limiting
	metricsHandler := middleware.RateLimit(app.limiters.Metrics)(promhttp.Handler())

	// Return a handler that checks the enabled flag
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if metrics are enabled
		if !app.metricsEnabled.Load() {
			http.NotFound(w, r)
			return
		}
		metricsHandler.ServeHTTP(w, r)
	})
}
