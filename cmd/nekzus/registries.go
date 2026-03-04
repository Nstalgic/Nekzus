package main

import (
	"github.com/nstalgic/nekzus/internal/activity"
	"github.com/nstalgic/nekzus/internal/auth"
	"github.com/nstalgic/nekzus/internal/backup"
	"github.com/nstalgic/nekzus/internal/certmanager"
	"github.com/nstalgic/nekzus/internal/discovery"
	"github.com/nstalgic/nekzus/internal/federation"
	"github.com/nstalgic/nekzus/internal/handlers"
	"github.com/nstalgic/nekzus/internal/health"
	"github.com/nstalgic/nekzus/internal/jobs"
	"github.com/nstalgic/nekzus/internal/proxy"
	"github.com/nstalgic/nekzus/internal/ratelimit"
	"github.com/nstalgic/nekzus/internal/router"
	"github.com/nstalgic/nekzus/internal/scripts"
	"github.com/nstalgic/nekzus/internal/toolbox"
	"github.com/nstalgic/nekzus/internal/websocket"
)

// ServiceRegistry groups all service managers
// Services represent core business logic components
type ServiceRegistry struct {
	Auth           *auth.Manager
	Discovery      *discovery.DiscoveryManager
	Health         *health.Manager
	Certs          *certmanager.Manager
	Toolbox        *toolbox.Manager
	Scripts        *scripts.Manager
	SessionCookies *proxy.SessionCookieManager // Session cookie persistence for mobile webview
}

// RateLimiterRegistry groups all rate limiters
// Rate limiters control access frequency to different endpoints
type RateLimiterRegistry struct {
	QR        *ratelimit.Limiter // QR code generation rate limiter
	Auth      *ratelimit.Limiter // Authentication endpoint rate limiter
	Device    *ratelimit.Limiter // Device management rate limiter
	Container *ratelimit.Limiter // Container operations rate limiter
	Health    *ratelimit.Limiter // Health check endpoint rate limiter
	Metrics   *ratelimit.Limiter // Metrics endpoint rate limiter
	WebSocket *ratelimit.Limiter // WebSocket connection rate limiter
}

// ManagerRegistry groups operational managers
// Managers handle runtime operations and coordination
type ManagerRegistry struct {
	WebSocket *websocket.Manager      // WebSocket connection manager
	Router    *router.Registry        // Route registry
	Backup    *backup.Manager         // Backup manager
	Peers     *federation.PeerManager // Federation peer manager
	Activity  *activity.Tracker       // Activity tracking
	Pairing   *auth.PairingManager    // Pairing code manager for v2 flow
}

// HandlerRegistry groups HTTP request handlers
// Handlers process incoming HTTP requests
type HandlerRegistry struct {
	Auth             *handlers.AuthHandler
	Container        *handlers.ContainerHandler
	ContainerLogs    *handlers.ContainerLogsHandler
	Export           *handlers.ExportHandler
	System           *handlers.SystemHandler
	Stats            *handlers.StatsHandler
	MetricsDashboard *handlers.MetricsDashboardHandler
	Backup           *handlers.BackupHandler
	Toolbox          *handlers.ToolboxHandler
	Scripts          *handlers.ScriptsHandler
	ServiceHealth    *handlers.ServiceHealthHandler
	Favicon          *handlers.FaviconHandler
	SessionCookies   *handlers.SessionCookiesHandler
	Pairing          *handlers.PairingHandler
	QR               *handlers.QRHandler
	Notifications    *handlers.NotificationHandler
}

// JobRegistry groups background jobs and scheduled tasks
// Jobs perform periodic or background operations
type JobRegistry struct {
	ServiceHealth    *health.ServiceHealthChecker
	OfflineDetection *jobs.OfflineDetectionJob
	BackupScheduler  *backup.Scheduler
	ToolboxDeployer  *toolbox.Deployer
	ScriptScheduler  *scripts.Scheduler
	ScriptRunner     *scripts.Runner
}
