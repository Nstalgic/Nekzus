package health

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"sync"
	"time"

	"github.com/nstalgic/nekzus/internal/storage"
	"github.com/nstalgic/nekzus/internal/types"
)

var log = slog.With("package", "health")

// ServiceHealthChecker performs periodic health checks on registered services
type ServiceHealthChecker struct {
	config            types.HealthChecksConfig
	routeRegistry     RouteRegistry
	storage           *storage.Store
	metrics           ServiceHealthMetrics
	wsManager         WebSocketNotifier
	notificationQueue NotificationQueue
	client            *http.Client
	healthStatus      map[string]*ServiceHealthStatus
	mu                sync.RWMutex
	ctx               context.Context
	cancel            context.CancelFunc
	wg                sync.WaitGroup
}

// RouteRegistry defines the interface for accessing routes
type RouteRegistry interface {
	ListApps() []types.App
	GetRouteByAppID(appID string) (*types.Route, bool)
	GetAppByID(appID string) (*types.App, bool)
}

// ServiceHealthMetrics defines the interface for recording health check metrics
type ServiceHealthMetrics interface {
	RecordServiceHealthCheck(appID, status string, duration time.Duration)
	SetServiceHealthStatus(appID string, status float64) // 0=unknown, 1=healthy, 2=unhealthy
}

// WebSocketNotifier defines the interface for sending WebSocket notifications
type WebSocketNotifier interface {
	PublishHealthChange(appID, appName, proxyPath, status, message string)
}

// NotificationQueue defines the interface for enqueueing notifications
type NotificationQueue interface {
	Enqueue(deviceID string, msgType string, payload json.RawMessage, ttl time.Duration, maxRetries int) error
}

// ServiceHealthStatus holds the current health status of a service
type ServiceHealthStatus struct {
	AppID               string
	Status              string // healthy, unhealthy, unknown
	LastCheckTime       time.Time
	LastSuccessTime     *time.Time
	ConsecutiveFailures int
	ErrorMessage        string
}

// NewServiceHealthChecker creates a new service health checker
func NewServiceHealthChecker(
	config types.HealthChecksConfig,
	routeRegistry RouteRegistry,
	storage *storage.Store,
	metrics ServiceHealthMetrics,
) *ServiceHealthChecker {
	ctx, cancel := context.WithCancel(context.Background())

	// Create HTTP client with timeout and cookie jar
	timeout := 5 * time.Second
	if config.Timeout != "" {
		if d, err := time.ParseDuration(config.Timeout); err == nil {
			timeout = d
		}
	}

	// Create cookie jar to handle apps that require cookies across redirects
	// (e.g., Jackett redirects / -> /UI/Dashboard and requires session cookies)
	jar, _ := cookiejar.New(nil)

	client := &http.Client{
		Timeout: timeout,
		Jar:     jar,
		// Allow following redirects (default behavior)
		// Web applications often redirect / to /dashboard, /login, etc.
		// Health check should follow redirects and check final destination
	}

	return &ServiceHealthChecker{
		config:        config,
		routeRegistry: routeRegistry,
		storage:       storage,
		metrics:       metrics,
		client:        client,
		healthStatus:  make(map[string]*ServiceHealthStatus),
		ctx:           ctx,
		cancel:        cancel,
	}
}

// SetWebSocketNotifier sets the WebSocket notifier for health change events
func (c *ServiceHealthChecker) SetWebSocketNotifier(wsManager WebSocketNotifier) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.wsManager = wsManager
}

// SetNotificationQueue sets the notification queue for health change notifications
func (c *ServiceHealthChecker) SetNotificationQueue(queue NotificationQueue) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.notificationQueue = queue
}

// Start begins periodic health checks
func (c *ServiceHealthChecker) Start() error {
	if !c.config.Enabled {
		log.Info("Service health checks disabled")
		return nil
	}

	// Parse check interval
	interval := 30 * time.Second
	if c.config.Interval != "" {
		if d, err := time.ParseDuration(c.config.Interval); err == nil {
			interval = d
		}
	}

	log.Info("Starting service health checker", "interval", interval, "threshold", c.config.UnhealthyThreshold)

	// Load existing health statuses from storage
	if c.storage != nil {
		if err := c.loadHealthStatusesFromStorage(); err != nil {
			log.Warn("Failed to load health statuses from storage", "error", err)
		}
	}

	// Start periodic health check routine
	c.wg.Add(1)
	go c.checkLoop(interval)

	return nil
}

// Stop gracefully stops health checks
func (c *ServiceHealthChecker) Stop() error {
	log.Info("Stopping service health checker")
	c.cancel()
	c.wg.Wait()
	log.Info("Service health checker stopped")
	return nil
}

// checkLoop runs periodic health checks
func (c *ServiceHealthChecker) checkLoop(interval time.Duration) {
	defer c.wg.Done()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Run initial check immediately
	c.checkAllServices()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			c.checkAllServices()
		}
	}
}

// checkAllServices checks health of all registered services
func (c *ServiceHealthChecker) checkAllServices() {
	apps := c.routeRegistry.ListApps()

	if len(apps) == 0 {
		log.Info("Health check: no apps registered for health checking")
		return
	}

	log.Info("Health check: checking apps", "count", len(apps))

	for _, app := range apps {
		// Check each service concurrently
		go c.checkServiceHealth(app.ID)
	}
}

// checkServiceHealth performs a health check for a single service
func (c *ServiceHealthChecker) checkServiceHealth(appID string) {
	start := time.Now()

	// Get route for the service
	route, ok := c.routeRegistry.GetRouteByAppID(appID)
	if !ok {
		c.updateHealthStatus(appID, "unknown", "no route found for service", start)
		return
	}

	// Skip health check if configured (for origin-validating apps like Joplin)
	if route.SkipHealthCheck {
		log.Debug("Skipping health check (skip_health_check=true)", "appID", appID)
		c.updateHealthStatus(appID, "healthy", "", start)
		return
	}

	// Resolve effective configuration (route-level > per-service > global)
	checkPath := c.getEffectivePath(appID, route)
	timeout := c.getEffectiveTimeout(appID, route)
	expectedCodes := c.getEffectiveStatusCodes(appID, route)

	// Parse target URL
	targetURL, err := url.Parse(route.To)
	if err != nil {
		c.updateHealthStatus(appID, "unknown", fmt.Sprintf("invalid target URL: %v", err), start)
		return
	}

	// Build health check URL
	targetURL.Path = checkPath
	healthCheckURL := targetURL.String()

	// Create request with timeout context
	ctx, cancel := context.WithTimeout(c.ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", healthCheckURL, nil)
	if err != nil {
		c.updateHealthStatus(appID, "unhealthy", fmt.Sprintf("failed to create request: %v", err), start)
		return
	}

	// Perform health check
	resp, err := c.client.Do(req)
	if err != nil {
		c.updateHealthStatus(appID, "unhealthy", fmt.Sprintf("request failed: %v", err), start)
		return
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Warn("Failed to close response body", "appID", appID, "error", err)
		}
	}()

	// Check response status against expected codes
	if c.isExpectedStatus(resp.StatusCode, expectedCodes) {
		c.updateHealthStatus(appID, "healthy", "", start)
	} else {
		errorMsg := fmt.Sprintf("HTTP %d", resp.StatusCode)
		log.Warn("Health check failed", "appID", appID, "error", errorMsg, "url", healthCheckURL)
		c.updateHealthStatus(appID, "unhealthy", errorMsg, start)
	}
}

// getEffectivePath returns the health check path with priority: route-level > per-service > global
func (c *ServiceHealthChecker) getEffectivePath(appID string, route *types.Route) string {
	// Route-level override (highest priority)
	if route != nil && route.HealthCheckPath != "" {
		return route.HealthCheckPath
	}

	// Per-service override from config
	if perService, ok := c.config.PerService[appID]; ok && perService.Path != "" {
		return perService.Path
	}

	// Global config
	if c.config.Path != "" {
		return c.config.Path
	}

	return "/"
}

// getEffectiveTimeout returns the health check timeout with priority: route-level > per-service > global
func (c *ServiceHealthChecker) getEffectiveTimeout(appID string, route *types.Route) time.Duration {
	// Route-level override (highest priority)
	if route != nil && route.HealthCheckTimeout != "" {
		if d, err := time.ParseDuration(route.HealthCheckTimeout); err == nil {
			return d
		}
	}

	// Per-service override from config
	if perService, ok := c.config.PerService[appID]; ok && perService.Timeout != "" {
		if d, err := time.ParseDuration(perService.Timeout); err == nil {
			return d
		}
	}

	// Global config
	if c.config.Timeout != "" {
		if d, err := time.ParseDuration(c.config.Timeout); err == nil {
			return d
		}
	}

	return 5 * time.Second
}

// getEffectiveInterval returns the health check interval with priority: route-level > per-service > global
func (c *ServiceHealthChecker) getEffectiveInterval(appID string, route *types.Route) time.Duration {
	// Route-level override (highest priority)
	if route != nil && route.HealthCheckInterval != "" {
		if d, err := time.ParseDuration(route.HealthCheckInterval); err == nil {
			return d
		}
	}

	// Per-service override from config
	if perService, ok := c.config.PerService[appID]; ok && perService.Interval != "" {
		if d, err := time.ParseDuration(perService.Interval); err == nil {
			return d
		}
	}

	// Global config
	if c.config.Interval != "" {
		if d, err := time.ParseDuration(c.config.Interval); err == nil {
			return d
		}
	}

	return 30 * time.Second
}

// getEffectiveStatusCodes returns expected status codes with priority: route-level > default (200-299)
func (c *ServiceHealthChecker) getEffectiveStatusCodes(appID string, route *types.Route) []int {
	// Route-level override (highest priority)
	if route != nil && len(route.ExpectedStatusCodes) > 0 {
		return route.ExpectedStatusCodes
	}

	// Default: 200-299 range
	return nil // nil means use default 200-299 check
}

// isExpectedStatus checks if status code matches expected codes
func (c *ServiceHealthChecker) isExpectedStatus(statusCode int, expectedCodes []int) bool {
	// If no specific codes defined, use default 200-299 range
	if len(expectedCodes) == 0 {
		return statusCode >= 200 && statusCode < 300
	}

	// Check against specific expected codes
	for _, code := range expectedCodes {
		if statusCode == code {
			return true
		}
	}

	return false
}

// updateHealthStatus updates the health status for a service
func (c *ServiceHealthChecker) updateHealthStatus(appID, status, errorMsg string, startTime time.Time) {
	duration := time.Since(startTime)

	c.mu.Lock()
	defer c.mu.Unlock()

	// Get or create status
	healthStatus, exists := c.healthStatus[appID]
	if !exists {
		healthStatus = &ServiceHealthStatus{
			AppID:  appID,
			Status: "unknown",
		}
		c.healthStatus[appID] = healthStatus
	}

	// Capture previous status for change detection
	previousStatus := healthStatus.Status

	// Update timestamps
	healthStatus.LastCheckTime = time.Now()

	// Update status based on result
	if status == "healthy" {
		healthStatus.Status = "healthy"
		healthStatus.ConsecutiveFailures = 0
		healthStatus.ErrorMessage = ""
		now := time.Now()
		healthStatus.LastSuccessTime = &now
	} else if status == "unhealthy" {
		healthStatus.ConsecutiveFailures++
		healthStatus.ErrorMessage = errorMsg

		// Only mark as unhealthy if threshold is reached
		threshold := c.config.UnhealthyThreshold
		if threshold <= 0 {
			threshold = 3 // Default threshold
		}

		if healthStatus.ConsecutiveFailures >= threshold {
			healthStatus.Status = "unhealthy"
		} else {
			// Still considered healthy until threshold reached
			healthStatus.Status = "healthy"
		}
	} else {
		// Unknown status
		healthStatus.Status = "unknown"
		healthStatus.ErrorMessage = errorMsg
	}

	// Detect status change and notify via WebSocket
	// Don't notify on initial unknown -> healthy/unhealthy transitions (first check)
	statusChanged := previousStatus != healthStatus.Status
	shouldNotify := statusChanged && previousStatus != "unknown" && c.wsManager != nil

	if shouldNotify {
		// Send notification outside the lock to avoid blocking
		notificationMsg := healthStatus.ErrorMessage
		if notificationMsg == "" && healthStatus.Status == "healthy" {
			notificationMsg = "Service recovered"
		}

		// Get app name and proxy path for notification
		notifyAppName := appID
		notifyProxyPath := ""
		if app, ok := c.routeRegistry.GetAppByID(appID); ok {
			if app.Name != "" {
				notifyAppName = app.Name
			}
		}
		if route, ok := c.routeRegistry.GetRouteByAppID(appID); ok {
			notifyProxyPath = route.PathBase
		}

		// Make copies of values for goroutine
		notifyAppID := appID
		notifyStatus := healthStatus.Status
		notifyMsg := notificationMsg

		go c.wsManager.PublishHealthChange(notifyAppID, notifyAppName, notifyProxyPath, notifyStatus, notifyMsg)

		// Enqueue notification for offline devices
		if c.notificationQueue != nil && c.storage != nil {
			go c.enqueueHealthNotification(notifyAppID, notifyStatus, notifyMsg)
		}

		log.Info("Health status changed (notifying mobile apps)", "appID", appID, "from", previousStatus, "to", healthStatus.Status)
	} else if statusChanged {
		log.Info("Health status changed (initial check, no notification)", "appID", appID, "from", previousStatus, "to", healthStatus.Status)
	}

	// Record metrics
	if c.metrics != nil {
		c.metrics.RecordServiceHealthCheck(appID, healthStatus.Status, duration)

		// Convert status to numeric value for gauge
		var statusValue float64
		switch healthStatus.Status {
		case "healthy":
			statusValue = 1
		case "unhealthy":
			statusValue = 2
		default:
			statusValue = 0
		}
		c.metrics.SetServiceHealthStatus(appID, statusValue)
	}

	// Persist to storage
	if c.storage != nil {
		go func() {
			if err := c.storage.SaveServiceHealth(storage.ServiceHealth{
				AppID:               healthStatus.AppID,
				Status:              healthStatus.Status,
				LastCheckTime:       &healthStatus.LastCheckTime,
				LastSuccessTime:     healthStatus.LastSuccessTime,
				ConsecutiveFailures: healthStatus.ConsecutiveFailures,
				ErrorMessage:        healthStatus.ErrorMessage,
			}); err != nil {
				log.Warn("Failed to save service health", "appID", appID, "error", err)
			}
		}()
	}

	log.Info("Health check completed", "appID", appID, "status", healthStatus.Status, "failures", healthStatus.ConsecutiveFailures, "duration", duration)
}

// GetServiceHealth returns the health status for a service
func (c *ServiceHealthChecker) GetServiceHealth(appID string) (*ServiceHealthStatus, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	status, ok := c.healthStatus[appID]
	if !ok {
		return nil, false
	}

	// Return a copy to avoid race conditions
	statusCopy := *status
	return &statusCopy, true
}

// IsServiceHealthy returns true if the service is healthy
func (c *ServiceHealthChecker) IsServiceHealthy(appID string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	status, ok := c.healthStatus[appID]
	if !ok {
		// No health status available - assume healthy
		return true
	}

	return status.Status == "healthy"
}

// loadHealthStatusesFromStorage loads health statuses from storage on startup
func (c *ServiceHealthChecker) loadHealthStatusesFromStorage() error {
	healthStatuses, err := c.storage.ListServiceHealth()
	if err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	for _, health := range healthStatuses {
		c.healthStatus[health.AppID] = &ServiceHealthStatus{
			AppID:               health.AppID,
			Status:              health.Status,
			LastCheckTime:       *health.LastCheckTime,
			LastSuccessTime:     health.LastSuccessTime,
			ConsecutiveFailures: health.ConsecutiveFailures,
			ErrorMessage:        health.ErrorMessage,
		}
	}

	log.Info("Loaded service health statuses from storage", "count", len(healthStatuses))
	return nil
}

// GetAllServiceHealth returns all service health statuses
func (c *ServiceHealthChecker) GetAllServiceHealth() map[string]*ServiceHealthStatus {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Return a copy of the map
	result := make(map[string]*ServiceHealthStatus, len(c.healthStatus))
	for appID, status := range c.healthStatus {
		statusCopy := *status
		result[appID] = &statusCopy
	}

	return result
}

// GetRouteHealthInfo returns computed health check information for a route
func (c *ServiceHealthChecker) GetRouteHealthInfo(appID string, route *types.Route) *types.RouteHealthInfo {
	if route == nil {
		return nil
	}

	// Get effective configuration
	effectivePath := c.getEffectivePath(appID, route)
	effectiveTimeout := c.getEffectiveTimeout(appID, route)
	effectiveInterval := c.getEffectiveInterval(appID, route)
	effectiveCodes := c.getEffectiveStatusCodes(appID, route)

	// Determine config source
	configSource := "global"
	if route.HealthCheckPath != "" || route.HealthCheckTimeout != "" ||
		route.HealthCheckInterval != "" || len(route.ExpectedStatusCodes) > 0 {
		configSource = "route"
	} else if perService, ok := c.config.PerService[appID]; ok &&
		(perService.Path != "" || perService.Timeout != "" || perService.Interval != "") {
		configSource = "per-service"
	}

	// Build probe URL
	probeURL := ""
	if targetURL, err := url.Parse(route.To); err == nil {
		targetURL.Path = effectivePath
		probeURL = targetURL.String()
	}

	// Get current health status
	status := "unknown"
	var lastCheck *time.Time
	var lastError string
	if healthStatus, ok := c.GetServiceHealth(appID); ok {
		status = healthStatus.Status
		if !healthStatus.LastCheckTime.IsZero() {
			lastCheck = &healthStatus.LastCheckTime
		}
		lastError = healthStatus.ErrorMessage
	}

	// Default codes if none specified
	if len(effectiveCodes) == 0 {
		effectiveCodes = []int{200, 201, 202, 203, 204, 205, 206, 207, 208, 226}
	}

	return &types.RouteHealthInfo{
		ProbeURL:          probeURL,
		Status:            status,
		LastCheck:         lastCheck,
		EffectivePath:     effectivePath,
		EffectiveTimeout:  effectiveTimeout.String(),
		EffectiveInterval: effectiveInterval.String(),
		EffectiveCodes:    effectiveCodes,
		ConfigSource:      configSource,
		LastError:         lastError,
	}
}

// MarkAppUnhealthy immediately marks an app as unhealthy and sends notifications.
// Use this when a container is stopped via API to provide immediate feedback
// instead of waiting for the next health check cycle.
// This method is idempotent - calling it multiple times won't spam notifications.
func (c *ServiceHealthChecker) MarkAppUnhealthy(appID, reason string) {
	c.mu.Lock()

	// Get or create status
	healthStatus, exists := c.healthStatus[appID]
	if !exists {
		healthStatus = &ServiceHealthStatus{
			AppID:  appID,
			Status: "unknown",
		}
		c.healthStatus[appID] = healthStatus
	}

	// Capture previous status for change detection
	previousStatus := healthStatus.Status

	// Only send notification if status is actually changing to unhealthy
	statusChanged := previousStatus != "unhealthy"

	// Update status
	healthStatus.Status = "unhealthy"
	healthStatus.ErrorMessage = reason
	healthStatus.LastCheckTime = time.Now()
	healthStatus.ConsecutiveFailures++

	c.mu.Unlock()

	// Send notifications only if status changed (prevents spam)
	if statusChanged && previousStatus != "unknown" {
		// Get app name and proxy path for notification
		appName := appID
		proxyPath := ""
		if app, ok := c.routeRegistry.GetAppByID(appID); ok {
			if app.Name != "" {
				appName = app.Name
			}
		}
		if route, ok := c.routeRegistry.GetRouteByAppID(appID); ok {
			proxyPath = route.PathBase
		}

		if c.wsManager != nil {
			go c.wsManager.PublishHealthChange(appID, appName, proxyPath, "unhealthy", reason)
		}

		// Enqueue notification for offline devices
		if c.notificationQueue != nil && c.storage != nil {
			go c.enqueueHealthNotification(appID, "unhealthy", reason)
		}

		log.Info("App marked unhealthy (immediate notification)", "appID", appID, "reason", reason, "previousStatus", previousStatus)
	} else if statusChanged {
		log.Info("App marked unhealthy (no notification - was unknown)", "appID", appID, "reason", reason)
	} else {
		log.Debug("App already unhealthy (no notification)", "appID", appID)
	}

	// Record metrics
	if c.metrics != nil {
		c.metrics.SetServiceHealthStatus(appID, 2) // 2 = unhealthy
	}

	// Persist to storage
	if c.storage != nil {
		go func() {
			if err := c.storage.SaveServiceHealth(storage.ServiceHealth{
				AppID:               appID,
				Status:              "unhealthy",
				LastCheckTime:       &healthStatus.LastCheckTime,
				ConsecutiveFailures: healthStatus.ConsecutiveFailures,
				ErrorMessage:        reason,
			}); err != nil {
				log.Warn("Failed to save service health", "appID", appID, "error", err)
			}
		}()
	}
}

// enqueueHealthNotification enqueues health notifications for all devices
func (c *ServiceHealthChecker) enqueueHealthNotification(appID, status, message string) {
	// Get all devices from storage
	devices, err := c.storage.ListDevices()
	if err != nil {
		log.Warn("Failed to get devices for health notification", "error", err)
		return
	}

	// Get app name and proxy path from route registry
	appName := appID
	proxyPath := ""
	if app, ok := c.routeRegistry.GetAppByID(appID); ok {
		if app.Name != "" {
			appName = app.Name
		}
	}
	if route, ok := c.routeRegistry.GetRouteByAppID(appID); ok {
		proxyPath = route.PathBase
	}

	// Create notification payload
	payload := map[string]interface{}{
		"title":     "Service Health Alert",
		"body":      message,
		"appId":     appID,
		"appName":   appName,
		"proxyPath": proxyPath,
		"status":    status,
		"timestamp": time.Now().Unix(),
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		log.Warn("Failed to marshal health notification payload", "error", err)
		return
	}

	// Enqueue notification for each device
	successCount := 0
	for _, device := range devices {
		// Only notify unhealthy status (not every healthy check)
		// This reduces notification spam
		if status != "unhealthy" {
			continue
		}

		// TTL: 1 hour for health alerts
		// MaxRetries: 3 attempts
		err := c.notificationQueue.Enqueue(
			device.ID,
			"health_alert",
			json.RawMessage(payloadJSON),
			1*time.Hour,
			3,
		)
		if err != nil {
			log.Warn("Failed to enqueue health notification for device", "deviceID", device.ID, "error", err)
		} else {
			successCount++
		}
	}

	if successCount > 0 {
		log.Info("Enqueued health notification for devices", "count", successCount, "appID", appID, "status", status)
	}
}
