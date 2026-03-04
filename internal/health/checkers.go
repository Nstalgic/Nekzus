package health

import (
	"context"
	"database/sql"
	"fmt"
	"runtime"
	"time"
)

// StorageChecker checks the health of the storage layer
type StorageChecker struct {
	db *sql.DB
}

// NewStorageChecker creates a new storage health checker
func NewStorageChecker(db *sql.DB) *StorageChecker {
	return &StorageChecker{db: db}
}

// Name returns the component name
func (c *StorageChecker) Name() string {
	return "storage"
}

// Check performs the storage health check
func (c *StorageChecker) Check(ctx context.Context) ComponentHealth {
	if c.db == nil {
		return ComponentHealth{
			Status:  StatusDegraded,
			Message: "storage not configured (in-memory mode)",
		}
	}

	// Ping database
	pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	if err := c.db.PingContext(pingCtx); err != nil {
		return ComponentHealth{
			Status:  StatusUnhealthy,
			Message: fmt.Sprintf("database ping failed: %v", err),
		}
	}

	// Check database stats
	stats := c.db.Stats()
	details := map[string]interface{}{
		"openConnections":    stats.OpenConnections,
		"inUse":              stats.InUse,
		"idle":               stats.Idle,
		"waitCount":          stats.WaitCount,
		"maxOpenConnections": stats.MaxOpenConnections,
		"maxIdleClosed":      stats.MaxIdleClosed,
		"maxLifetimeClosed":  stats.MaxLifetimeClosed,
	}

	// Check if we're experiencing connection issues
	if stats.WaitCount > 100 {
		return ComponentHealth{
			Status:  StatusDegraded,
			Message: "high database connection wait count",
			Details: details,
		}
	}

	return ComponentHealth{
		Status:  StatusHealthy,
		Message: "database operational",
		Details: details,
	}
}

// DiscoveryChecker checks the health of the discovery system
type DiscoveryChecker struct {
	workersActive func() int
	enabled       bool
}

// NewDiscoveryChecker creates a new discovery health checker
func NewDiscoveryChecker(enabled bool, workersActive func() int) *DiscoveryChecker {
	return &DiscoveryChecker{
		workersActive: workersActive,
		enabled:       enabled,
	}
}

// Name returns the component name
func (c *DiscoveryChecker) Name() string {
	return "discovery"
}

// Check performs the discovery health check
func (c *DiscoveryChecker) Check(ctx context.Context) ComponentHealth {
	if !c.enabled {
		return ComponentHealth{
			Status:  StatusHealthy,
			Message: "discovery disabled",
			Details: map[string]interface{}{
				"enabled": false,
			},
		}
	}

	activeWorkers := 0
	if c.workersActive != nil {
		activeWorkers = c.workersActive()
	}

	details := map[string]interface{}{
		"enabled":       c.enabled,
		"activeWorkers": activeWorkers,
	}

	if activeWorkers == 0 {
		return ComponentHealth{
			Status:  StatusDegraded,
			Message: "no active discovery workers",
			Details: details,
		}
	}

	return ComponentHealth{
		Status:  StatusHealthy,
		Message: fmt.Sprintf("%d worker(s) active", activeWorkers),
		Details: details,
	}
}

// AuthChecker checks the health of the authentication system
type AuthChecker struct {
	validateToken func() error
}

// NewAuthChecker creates a new auth health checker
func NewAuthChecker(validateToken func() error) *AuthChecker {
	return &AuthChecker{
		validateToken: validateToken,
	}
}

// Name returns the component name
func (c *AuthChecker) Name() string {
	return "authentication"
}

// Check performs the auth health check
func (c *AuthChecker) Check(ctx context.Context) ComponentHealth {
	if c.validateToken == nil {
		return ComponentHealth{
			Status:  StatusHealthy,
			Message: "auth system operational",
		}
	}

	if err := c.validateToken(); err != nil {
		return ComponentHealth{
			Status:  StatusUnhealthy,
			Message: fmt.Sprintf("auth validation failed: %v", err),
		}
	}

	return ComponentHealth{
		Status:  StatusHealthy,
		Message: "auth system operational",
	}
}

// ProxyCacheChecker checks the health of the proxy cache
type ProxyCacheChecker struct {
	cacheSize func() int
}

// NewProxyCacheChecker creates a new proxy cache health checker
func NewProxyCacheChecker(cacheSize func() int) *ProxyCacheChecker {
	return &ProxyCacheChecker{
		cacheSize: cacheSize,
	}
}

// Name returns the component name
func (c *ProxyCacheChecker) Name() string {
	return "proxy_cache"
}

// Check performs the proxy cache health check
func (c *ProxyCacheChecker) Check(ctx context.Context) ComponentHealth {
	size := 0
	if c.cacheSize != nil {
		size = c.cacheSize()
	}

	details := map[string]interface{}{
		"cachedProxies": size,
	}

	// Warn if cache is getting very large (potential memory issue)
	if size > 100 {
		return ComponentHealth{
			Status:  StatusDegraded,
			Message: fmt.Sprintf("large cache size: %d proxies", size),
			Details: details,
		}
	}

	return ComponentHealth{
		Status:  StatusHealthy,
		Message: fmt.Sprintf("%d cached proxies", size),
		Details: details,
	}
}

// EventBusChecker checks the health of the SSE event bus
type EventBusChecker struct {
	activeConnections func() int
}

// NewEventBusChecker creates a new event bus health checker
func NewEventBusChecker(activeConnections func() int) *EventBusChecker {
	return &EventBusChecker{
		activeConnections: activeConnections,
	}
}

// Name returns the component name
func (c *EventBusChecker) Name() string {
	return "event_bus"
}

// Check performs the event bus health check
func (c *EventBusChecker) Check(ctx context.Context) ComponentHealth {
	connections := 0
	if c.activeConnections != nil {
		connections = c.activeConnections()
	}

	details := map[string]interface{}{
		"activeConnections": connections,
	}

	// Warn if too many connections (potential resource issue)
	if connections > 1000 {
		return ComponentHealth{
			Status:  StatusDegraded,
			Message: fmt.Sprintf("high connection count: %d", connections),
			Details: details,
		}
	}

	return ComponentHealth{
		Status:  StatusHealthy,
		Message: fmt.Sprintf("%d active WebSocket connections", connections),
		Details: details,
	}
}

// MemoryChecker checks system memory usage
type MemoryChecker struct {
	maxMemoryMB int64
}

// NewMemoryChecker creates a new memory health checker
func NewMemoryChecker(maxMemoryMB int64) *MemoryChecker {
	return &MemoryChecker{
		maxMemoryMB: maxMemoryMB,
	}
}

// Name returns the component name
func (c *MemoryChecker) Name() string {
	return "memory"
}

// Check performs the memory health check
func (c *MemoryChecker) Check(ctx context.Context) ComponentHealth {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	allocMB := m.Alloc / 1024 / 1024
	sysMB := m.Sys / 1024 / 1024

	details := map[string]interface{}{
		"allocMB":      allocMB,
		"totalAllocMB": m.TotalAlloc / 1024 / 1024,
		"sysMB":        sysMB,
		"numGC":        m.NumGC,
		"goroutines":   runtime.NumGoroutine(),
	}

	// Check if memory usage is too high
	if c.maxMemoryMB > 0 && int64(allocMB) > c.maxMemoryMB {
		return ComponentHealth{
			Status:  StatusDegraded,
			Message: fmt.Sprintf("high memory usage: %d MB (max: %d MB)", allocMB, c.maxMemoryMB),
			Details: details,
		}
	}

	return ComponentHealth{
		Status:  StatusHealthy,
		Message: fmt.Sprintf("memory usage: %d MB", allocMB),
		Details: details,
	}
}

// CertificateInfo contains information about a certificate
type CertificateInfo struct {
	Domain   string
	NotAfter time.Time
	Issuer   string
}

// CertificateChecker checks the health of SSL/TLS certificates
type CertificateChecker struct {
	getCertificates func() []CertificateInfo
}

// NewCertificateChecker creates a new certificate health checker
func NewCertificateChecker(getCertificates func() []CertificateInfo) *CertificateChecker {
	return &CertificateChecker{
		getCertificates: getCertificates,
	}
}

// Name returns the component name
func (c *CertificateChecker) Name() string {
	return "certificates"
}

// Check performs the certificate health check
func (c *CertificateChecker) Check(ctx context.Context) ComponentHealth {
	if c.getCertificates == nil {
		return ComponentHealth{
			Status:  StatusHealthy,
			Message: "certificate management not configured",
		}
	}

	certs := c.getCertificates()
	if len(certs) == 0 {
		return ComponentHealth{
			Status:  StatusHealthy,
			Message: "no certificates to monitor",
			Details: map[string]interface{}{
				"totalCertificates": 0,
			},
		}
	}

	now := time.Now()
	var expired []string
	var expiringSoon []map[string]interface{}

	for _, cert := range certs {
		// Add small buffer (1 minute) to account for timing differences in tests/calculations
		timeUntilExpiry := time.Until(cert.NotAfter) + time.Minute
		daysUntilExpiry := int(timeUntilExpiry.Hours() / 24)

		if cert.NotAfter.Before(now) || cert.NotAfter.Equal(now) {
			// Certificate is expired or expiring today
			expired = append(expired, cert.Domain)
		} else if daysUntilExpiry < 30 {
			// Certificate expiring within 30 days (less than 30 full days remaining)
			// This triggers for 0-29 days, but not for 30+ days
			expiringSoon = append(expiringSoon, map[string]interface{}{
				"domain":        cert.Domain,
				"daysRemaining": daysUntilExpiry,
				"expiresAt":     cert.NotAfter,
				"issuer":        cert.Issuer,
			})
		}
	}

	details := map[string]interface{}{
		"totalCertificates": len(certs),
	}

	// Unhealthy if any certificates are expired
	if len(expired) > 0 {
		details["expired"] = expired
		if len(expiringSoon) > 0 {
			details["expiringSoon"] = expiringSoon
		}
		return ComponentHealth{
			Status:  StatusUnhealthy,
			Message: fmt.Sprintf("%d certificate(s) expired", len(expired)),
			Details: details,
		}
	}

	// Degraded if any certificates are expiring soon
	if len(expiringSoon) > 0 {
		details["expiringSoon"] = expiringSoon

		// Find the certificate expiring soonest
		minDays := 999
		for _, cert := range expiringSoon {
			days := cert["daysRemaining"].(int)
			if days < minDays {
				minDays = days
			}
		}

		return ComponentHealth{
			Status:  StatusDegraded,
			Message: fmt.Sprintf("%d certificate(s) expiring soon (soonest in %d days)", len(expiringSoon), minDays),
			Details: details,
		}
	}

	// All certificates are healthy
	return ComponentHealth{
		Status:  StatusHealthy,
		Message: fmt.Sprintf("all %d certificate(s) valid", len(certs)),
		Details: details,
	}
}
