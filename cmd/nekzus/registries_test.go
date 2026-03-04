package main

import (
	"testing"

	"github.com/nstalgic/nekzus/internal/activity"
	"github.com/nstalgic/nekzus/internal/auth"
	"github.com/nstalgic/nekzus/internal/backup"
	"github.com/nstalgic/nekzus/internal/certmanager"
	"github.com/nstalgic/nekzus/internal/discovery"
	"github.com/nstalgic/nekzus/internal/federation"
	"github.com/nstalgic/nekzus/internal/handlers"
	"github.com/nstalgic/nekzus/internal/health"
	"github.com/nstalgic/nekzus/internal/jobs"
	"github.com/nstalgic/nekzus/internal/ratelimit"
	"github.com/nstalgic/nekzus/internal/router"
	"github.com/nstalgic/nekzus/internal/toolbox"
	"github.com/nstalgic/nekzus/internal/websocket"
)

// TestServiceRegistry tests the ServiceRegistry structure
func TestServiceRegistry(t *testing.T) {
	t.Run("creation_with_all_services", func(t *testing.T) {
		registry := &ServiceRegistry{
			Auth:      &auth.Manager{},
			Discovery: &discovery.DiscoveryManager{},
			Health:    &health.Manager{},
			Certs:     &certmanager.Manager{},
			Toolbox:   &toolbox.Manager{},
		}

		if registry.Auth == nil {
			t.Error("Expected Auth to be set")
		}
		if registry.Discovery == nil {
			t.Error("Expected Discovery to be set")
		}
		if registry.Health == nil {
			t.Error("Expected Health to be set")
		}
		if registry.Certs == nil {
			t.Error("Expected Certs to be set")
		}
		if registry.Toolbox == nil {
			t.Error("Expected Toolbox to be set")
		}
	})

	t.Run("creation_with_nil_services", func(t *testing.T) {
		// Should be able to create registry with nil services (graceful degradation)
		registry := &ServiceRegistry{}

		if registry.Auth != nil {
			t.Error("Expected Auth to be nil")
		}
		if registry.Discovery != nil {
			t.Error("Expected Discovery to be nil")
		}
	})
}

// TestRateLimiterRegistry tests the RateLimiterRegistry structure
func TestRateLimiterRegistry(t *testing.T) {
	t.Run("creation_with_all_limiters", func(t *testing.T) {
		registry := &RateLimiterRegistry{
			QR:        ratelimit.NewLimiter(1, 5),
			Auth:      ratelimit.NewLimiter(1, 5),
			Device:    ratelimit.NewLimiter(1, 5),
			Container: ratelimit.NewLimiter(1, 5),
			Health:    ratelimit.NewLimiter(1, 5),
			Metrics:   ratelimit.NewLimiter(1, 5),
			WebSocket: ratelimit.NewLimiter(1, 5),
		}

		if registry.QR == nil {
			t.Error("Expected QR limiter to be set")
		}
		if registry.Auth == nil {
			t.Error("Expected Auth limiter to be set")
		}
		if registry.Device == nil {
			t.Error("Expected Device limiter to be set")
		}
		if registry.Container == nil {
			t.Error("Expected Container limiter to be set")
		}
		if registry.Health == nil {
			t.Error("Expected Health limiter to be set")
		}
		if registry.Metrics == nil {
			t.Error("Expected Metrics limiter to be set")
		}
		if registry.WebSocket == nil {
			t.Error("Expected WebSocket limiter to be set")
		}
	})

	t.Run("creation_with_nil_limiters", func(t *testing.T) {
		// Should be able to create registry with nil limiters
		registry := &RateLimiterRegistry{}

		if registry.QR != nil {
			t.Error("Expected QR limiter to be nil")
		}
	})
}

// TestManagerRegistry tests the ManagerRegistry structure
func TestManagerRegistry(t *testing.T) {
	t.Run("creation_with_all_managers", func(t *testing.T) {
		registry := &ManagerRegistry{
			WebSocket: &websocket.Manager{},
			Router:    &router.Registry{},
			Backup:    &backup.Manager{},
			Peers:     &federation.PeerManager{},
			Activity:  &activity.Tracker{},
		}

		if registry.WebSocket == nil {
			t.Error("Expected WebSocket to be set")
		}
		if registry.Router == nil {
			t.Error("Expected Router to be set")
		}
		if registry.Backup == nil {
			t.Error("Expected Backup to be set")
		}
		if registry.Peers == nil {
			t.Error("Expected Peers to be set")
		}
		if registry.Activity == nil {
			t.Error("Expected Activity to be set")
		}
	})

	t.Run("creation_with_nil_managers", func(t *testing.T) {
		registry := &ManagerRegistry{}

		if registry.WebSocket != nil {
			t.Error("Expected WebSocket to be nil")
		}
	})
}

// TestHandlerRegistry tests the HandlerRegistry structure
func TestHandlerRegistry(t *testing.T) {
	t.Run("creation_with_all_handlers", func(t *testing.T) {
		registry := &HandlerRegistry{
			Container: &handlers.ContainerHandler{},
			System:    &handlers.SystemHandler{},
			Stats:     &handlers.StatsHandler{},
			Backup:    &handlers.BackupHandler{},
			Toolbox:   &handlers.ToolboxHandler{},
		}

		if registry.Container == nil {
			t.Error("Expected Container handler to be set")
		}
		if registry.System == nil {
			t.Error("Expected System handler to be set")
		}
		if registry.Stats == nil {
			t.Error("Expected Stats handler to be set")
		}
		if registry.Backup == nil {
			t.Error("Expected Backup handler to be set")
		}
		if registry.Toolbox == nil {
			t.Error("Expected Toolbox handler to be set")
		}
	})

	t.Run("creation_with_nil_handlers", func(t *testing.T) {
		registry := &HandlerRegistry{}

		if registry.Container != nil {
			t.Error("Expected Container handler to be nil")
		}
	})
}

// TestJobRegistry tests the JobRegistry structure
func TestJobRegistry(t *testing.T) {
	t.Run("creation_with_all_jobs", func(t *testing.T) {
		registry := &JobRegistry{
			ServiceHealth:    &health.ServiceHealthChecker{},
			OfflineDetection: &jobs.OfflineDetectionJob{},
			BackupScheduler:  &backup.Scheduler{},
			ToolboxDeployer:  &toolbox.Deployer{},
		}

		if registry.ServiceHealth == nil {
			t.Error("Expected ServiceHealth to be set")
		}
		if registry.OfflineDetection == nil {
			t.Error("Expected OfflineDetection to be set")
		}
		if registry.BackupScheduler == nil {
			t.Error("Expected BackupScheduler to be set")
		}
		if registry.ToolboxDeployer == nil {
			t.Error("Expected ToolboxDeployer to be set")
		}
	})

	t.Run("creation_with_nil_jobs", func(t *testing.T) {
		registry := &JobRegistry{}

		if registry.ServiceHealth != nil {
			t.Error("Expected ServiceHealth to be nil")
		}
	})
}

// TestRegistryIntegration tests that registries can be composed together
func TestRegistryIntegration(t *testing.T) {
	t.Run("all_registries_together", func(t *testing.T) {
		services := &ServiceRegistry{
			Auth: &auth.Manager{},
		}
		limiters := &RateLimiterRegistry{
			QR: ratelimit.NewLimiter(1, 5),
		}
		managers := &ManagerRegistry{
			Router: &router.Registry{},
		}
		handlers := &HandlerRegistry{
			System: &handlers.SystemHandler{},
		}
		jobs := &JobRegistry{
			ServiceHealth: &health.ServiceHealthChecker{},
		}

		// Verify fields are accessible
		if services.Auth == nil {
			t.Error("Expected Auth service to be accessible")
		}
		if limiters.QR == nil {
			t.Error("Expected QR limiter to be accessible")
		}
		if managers.Router == nil {
			t.Error("Expected Router manager to be accessible")
		}
		if handlers.System == nil {
			t.Error("Expected System handler to be accessible")
		}
		if jobs.ServiceHealth == nil {
			t.Error("Expected ServiceHealth job to be accessible")
		}
	})
}
