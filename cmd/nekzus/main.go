package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/docker/docker/client"
	"github.com/nstalgic/nekzus/internal/activity"
	"github.com/nstalgic/nekzus/internal/auth"
	"github.com/nstalgic/nekzus/internal/certmanager"
	"github.com/nstalgic/nekzus/internal/config"
	"github.com/nstalgic/nekzus/internal/constants"
	"github.com/nstalgic/nekzus/internal/crypto"
	"github.com/nstalgic/nekzus/internal/discovery"
	"github.com/nstalgic/nekzus/internal/handlers"
	"github.com/nstalgic/nekzus/internal/health"
	"github.com/nstalgic/nekzus/internal/jobs"
	"github.com/nstalgic/nekzus/internal/logger"
	"github.com/nstalgic/nekzus/internal/metrics"
	"github.com/nstalgic/nekzus/internal/notifications"
	"github.com/nstalgic/nekzus/internal/proxy"
	"github.com/nstalgic/nekzus/internal/ratelimit"
	"github.com/nstalgic/nekzus/internal/router"
	"github.com/nstalgic/nekzus/internal/scripts"
	"github.com/nstalgic/nekzus/internal/storage"
	"github.com/nstalgic/nekzus/internal/tlsutil"
	"github.com/nstalgic/nekzus/internal/types"
	"github.com/nstalgic/nekzus/internal/websocket"
)

// Version is set via ldflags during build: -ldflags="-X main.version=v1.2.3"
var version = "dev"

// Package-level logger with structured context
var log = slog.With("package", "main")

// Application holds the main application state
type Application struct {
	// Configuration
	config        types.ServerConfig
	configPath    string
	configWatcher *config.Watcher

	// Registries
	services *ServiceRegistry
	limiters *RateLimiterRegistry
	managers *ManagerRegistry
	handlers *HandlerRegistry
	jobs     *JobRegistry

	// Core Infrastructure
	storage            *storage.Store
	metrics            *metrics.Metrics
	proxyCache         *proxy.Cache
	dockerClient       *client.Client
	httpServer         *http.Server
	httpRedirectServer *http.Server

	// Notifications
	notificationQueue   *notifications.Queue
	notificationService *notifications.Service
	wsDeliverer         *notifications.WebSocketDeliverer
	ackTracker          *notifications.ACKTracker

	// Metadata
	metricsEnabled atomic.Bool // Thread-safe flag for enabling/disabling metrics endpoint
	nekzusID       string
	baseURL        string
	version        string
	capabilities   []string
	startTime      time.Time

	// TLS hot-reload support
	tlsEnabled   atomic.Bool   // Whether server is currently running with TLS
	tlsUpgrade   chan struct{} // Signal to upgrade from HTTP to HTTPS
	serverErrors chan error    // Channel to communicate server errors
	serverDone   chan struct{} // Signal that server goroutine has exited
}

// NewApplication creates and initializes the application
func NewApplication(cfg types.ServerConfig, configPath string) (*Application, error) {
	// Validate bootstrap settings
	if err := auth.ValidateBootstrapAllowed(); err != nil {
		return nil, err
	}

	// Initialize storage
	// Ensure database directory exists
	dbDir := filepath.Dir(cfg.Storage.DatabasePath)
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	store, err := storage.NewStore(storage.Config{
		DatabasePath: cfg.Storage.DatabasePath,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize storage: %w", err)
	}

	log.Info("storage initialized",
		"database_path", cfg.Storage.DatabasePath)

	// Initialize user schema for username/password authentication
	if err := store.InitializeUserSchema(); err != nil {
		store.Close()
		return nil, fmt.Errorf("failed to initialize user schema: %w", err)
	}

	// Check if initial setup is required
	users, _ := store.ListUsers()
	if len(users) == 0 {
		log.Info("initial setup required - no users found, visit the web UI to create your first admin account")
	}

	// Ensure JWT secret is available (auto-generate if not provided)
	if err := config.EnsureJWTSecret(&cfg, store); err != nil {
		store.Close()
		return nil, fmt.Errorf("failed to ensure JWT secret: %w", err)
	}

	// Get cookie encryption key for session cookie persistence
	cookieEncryptionKey, err := config.GetCookieEncryptionKey(store)
	if err != nil {
		store.Close()
		return nil, fmt.Errorf("failed to get cookie encryption key: %w", err)
	}

	// Create cookie encryptor for session cookie persistence
	cookieEncryptor, err := crypto.NewCookieEncryptor(cookieEncryptionKey)
	if err != nil {
		store.Close()
		return nil, fmt.Errorf("failed to create cookie encryptor: %w", err)
	}

	// Create session cookie manager for mobile webview persistence
	sessionCookieMgr := proxy.NewSessionCookieManager(store, cookieEncryptor, cookieEncryptionKey)
	log.Info("session cookie manager initialized for mobile webview persistence")

	// Create auth manager
	authMgr, err := auth.NewManager(
		[]byte(cfg.Auth.HS256Secret),
		cfg.Auth.Issuer,
		cfg.Auth.Audience,
		cfg.Bootstrap.Tokens,
	)
	if err != nil {
		store.Close()
		return nil, err
	}

	// Initialize metrics first (needed by cert manager)
	startTime := time.Now()
	m := metrics.New("nekzus")
	m.SetBuildInfo("1.0.0", runtime.Version())
	m.AuthBootstrapTokensActive.Set(float64(len(cfg.Bootstrap.Tokens)))

	// Initialize certificate manager
	certMgr := certmanager.New(
		certmanager.Config{
			DefaultProvider: "self-signed",
		},
		certmanager.NewStorageAdapter(store),
		m,
	)
	certMgr.RegisterProvider(certmanager.NewSelfSignedProvider())

	// Load existing certificates from storage
	if err := certMgr.LoadFromStorage(); err != nil {
		log.Warn("failed to load certificates from storage",
			"error", err)
	}

	// Generate unique instance ID
	nekzusID := "nkz_" + randToken(12)

	// Resolve base URL with priority: env var > config file > auto-detect
	resolver := config.NewResolver()
	baseURL := resolver.ResolveBaseURL(cfg, getLocalNetworkIP)
	if resolver.WasAutoDetected(cfg) {
		log.Info("auto-detected base URL (set in config file or NEKZUS_BASE_URL env var to override)",
			"base_url", baseURL)
	} else {
		log.Info("using configured base URL",
			"base_url", baseURL)
	}

	// Update config with resolved baseURL so all components use the same value
	cfg.Server.BaseURL = baseURL

	// Initialize health manager
	healthMgr := health.NewManager("1.0.0", startTime, m)

	// Initialize service health checker (if enabled in config)
	var serviceHealthChecker *health.ServiceHealthChecker

	// Initialize offline detection job
	offlineThreshold := 5 * time.Minute // Devices offline if not seen for 5 minutes
	offlineInterval := 30 * time.Second // Check every 30 seconds
	offlineDetectionJob := jobs.NewOfflineDetectionJob(store, m, offlineThreshold, offlineInterval)

	// Create component builder for complex component initialization
	componentBuilder := NewComponentBuilder(cfg, store, m, startTime, version)

	// Initialize Docker client for container management (optional)
	dockerClient, containerHandler, containerLogsHandler, exportHandler := componentBuilder.BuildDockerClient()

	// Initialize system handler (always available)
	// If hostRootPath is set, metrics will be read from host via mounted /proc
	systemHandler := handlers.NewSystemHandler(cfg.Storage.DatabasePath, cfg.System.HostRootPath)

	// Define capabilities
	capabilities := []string{"catalog", "events", "proxy", "discovery"}

	// Initialize auth handler (always available)
	activityTracker := activity.NewTracker(store)
	authHandler := handlers.NewAuthHandler(
		authMgr,
		store,
		m,
		nil, // events publisher will be set to websocket manager after app creation
		activityTracker,
		nil,     // QR rate limiter will be set after app creation
		certMgr, // certificate manager for SPKI calculation
		baseURL,
		cfg.Server.TLSCert,
		nekzusID,
		version,
		capabilities,
	)

	// Initialize stats handler (requires router and storage)
	var statsHandler *handlers.StatsHandler

	app := &Application{
		config:     cfg,
		configPath: configPath,
		services: &ServiceRegistry{
			Auth:           authMgr,
			Health:         healthMgr,
			Certs:          certMgr,
			SessionCookies: sessionCookieMgr,
			// Discovery and Toolbox will be set later
		},
		limiters: &RateLimiterRegistry{
			QR:        ratelimit.NewLimiter(constants.QRLimitRate, constants.QRLimitBurst),
			Auth:      ratelimit.NewLimiter(constants.AuthLimitRate, constants.AuthLimitBurst),
			Device:    ratelimit.NewLimiter(constants.DeviceLimitRate, constants.DeviceLimitBurst),
			Container: ratelimit.NewLimiter(constants.ContainerLimitRate, constants.ContainerLimitBurst),
			Health:    ratelimit.NewLimiter(constants.HealthLimitRate, constants.HealthLimitBurst),
			Metrics:   ratelimit.NewLimiter(constants.MetricsLimitRate, constants.MetricsLimitBurst),
			WebSocket: ratelimit.NewLimiter(constants.WebSocketLimitRate, constants.WebSocketLimitBurst),
		},
		managers: &ManagerRegistry{
			WebSocket: websocket.NewManager(m, store),
			Router:    router.NewRegistry(store),
			Activity:  activityTracker,
			Pairing:   auth.NewPairingManager(),
			// Backup and Peers will be set later
		},
		handlers: &HandlerRegistry{
			Auth:          authHandler,
			Container:     containerHandler,
			ContainerLogs: containerLogsHandler,
			Export:        exportHandler,
			System:        systemHandler,
			Stats:         statsHandler,
			// Backup and Toolbox will be set later
		},
		jobs: &JobRegistry{
			ServiceHealth:    serviceHealthChecker,
			OfflineDetection: offlineDetectionJob,
			// BackupScheduler and ToolboxDeployer will be set later
		},
		storage:      store,
		metrics:      m,
		proxyCache:   proxy.NewCache(),
		dockerClient: dockerClient,
		nekzusID:     nekzusID,
		baseURL:      baseURL,
		version:      version,
		capabilities: capabilities,
		startTime:    startTime,
	}

	// Initialize metrics endpoint state from config
	app.metricsEnabled.Store(cfg.Metrics.Enabled)

	// Initialize stats handler (now that we have the router)
	app.handlers.Stats = handlers.NewStatsHandler(app.managers.Router, store, m)

	// Initialize metrics dashboard handler
	app.handlers.MetricsDashboard = handlers.NewMetricsDashboardHandler(m)

	// Initialize QR handler for v2 pairing flow
	app.handlers.QR = handlers.NewQRHandler(
		authMgr,
		app.limiters.QR,
		baseURL,
		cfg.Server.TLSCert,
		nekzusID,
		capabilities,
	)
	app.handlers.QR.SetPairingManager(app.managers.Pairing)
	app.handlers.QR.SetSPKIProvider(certMgr)

	// Initialize pairing handler for short code redemption
	app.handlers.Pairing = handlers.NewPairingHandler(app.managers.Pairing, app.limiters.Auth)

	// Initialize session cookies handler for mobile session persistence management
	app.handlers.SessionCookies = handlers.NewSessionCookiesHandler(store)

	// Initialize notification handler for notification queue management
	app.handlers.Notifications = handlers.NewNotificationHandler(store)

	// Update auth handler with websocket manager and QR rate limiter (now that we have them)
	app.handlers.Auth = handlers.NewAuthHandler(
		authMgr,
		store,
		m,
		app.managers.WebSocket, // events publisher
		activityTracker,
		app.limiters.QR, // QR rate limiter
		certMgr,         // certificate manager for SPKI calculation
		baseURL,
		cfg.Server.TLSCert,
		nekzusID,
		version,
		capabilities,
	)

	// Set WebSocket disconnecter for auth handler to clean up stale connections on re-pair
	if app.managers.WebSocket != nil {
		app.handlers.Auth.SetWebSocketDisconnecter(app.managers.WebSocket)
	}

	// Initialize service health checker (now that we have the router)
	if cfg.HealthChecks.Enabled {
		app.jobs.ServiceHealth = health.NewServiceHealthChecker(
			cfg.HealthChecks,
			app.managers.Router,
			store,
			m,
		)
		// Set WebSocket notifier if available
		if app.managers.WebSocket != nil {
			app.jobs.ServiceHealth.SetWebSocketNotifier(app.managers.WebSocket)
			slog.Info("Service health checker: WebSocket notifications enabled")
		}
	}

	// Initialize service health handler (works with or without health checker)
	app.handlers.ServiceHealth = handlers.NewServiceHealthHandler(
		app.managers.Router,
		app.jobs.ServiceHealth,
	)

	// Initialize favicon handler (for serving app favicons)
	app.handlers.Favicon = handlers.NewFaviconHandler(app.managers.Router, 24*time.Hour)

	// Set WebSocket notifier on container handler for async operation callbacks
	if app.handlers.Container != nil && app.managers.WebSocket != nil {
		app.handlers.Container.SetNotifier(app.managers.WebSocket)
		slog.Info("Container handler: WebSocket notifications enabled")
	}

	// Set health notifier on container handler for immediate health updates on stop
	if app.handlers.Container != nil && app.jobs.ServiceHealth != nil {
		app.handlers.Container.SetHealthNotifier(app.jobs.ServiceHealth)
		slog.Info("Container handler: Health notifications enabled")
	}

	// Set WebSocket notifier on container logs handler for log streaming
	if app.handlers.ContainerLogs != nil && app.managers.WebSocket != nil {
		app.handlers.ContainerLogs.SetNotifier(app.managers.WebSocket)
		slog.Info("Container logs handler: WebSocket notifications enabled")
	}

	// Initialize WebSocket notification deliverer with ACK tracking
	if cfg.Notifications.Enabled && app.managers.WebSocket != nil {
		wsAdapter := websocket.NewManagerAdapter(app.managers.WebSocket)

		// Parse ACK timeout (default 30 seconds)
		ackTimeout := 30 * time.Second
		if cfg.Notifications.ACKTimeout != "" {
			parsed, err := time.ParseDuration(cfg.Notifications.ACKTimeout)
			if err != nil {
				log.Warn("invalid ack_timeout, using default 30s",
					"ack_timeout", cfg.Notifications.ACKTimeout,
					"error", err)
			} else {
				ackTimeout = parsed
			}
		}

		// Create ACK tracker with callbacks for ACK and timeout handling
		app.ackTracker = notifications.NewACKTracker(notifications.ACKTrackerConfig{
			ACKTimeout:    ackTimeout,
			CheckInterval: 5 * time.Second,
			OnACK: func(storageID int64) {
				// Mark notification as delivered when client ACKs
				if store != nil && storageID > 0 {
					if err := store.MarkNotificationDelivered(storageID); err != nil {
						log.Error("failed to mark notification delivered on ACK",
							"storage_id", storageID,
							"error", err)
					} else {
						log.Info("notification marked delivered on client ACK",
							"storage_id", storageID)
					}
				}
			},
			OnTimeout: func(notifID, deviceID, msgType string, payload json.RawMessage) {
				// Notification was sent but client didn't ACK - will be retried on reconnect
				log.Warn("notification ACK timed out, will retry on device reconnect",
					"notification_id", notifID,
					"device_id", deviceID,
					"type", msgType)
			},
		})

		// Create deliverer with ACK tracking
		app.wsDeliverer = notifications.NewWebSocketDelivererWithACK(wsAdapter, app.ackTracker)
		log.Info("notifications websocket deliverer initialized with ACK tracking",
			"ack_timeout", ackTimeout)

		// Parse retry interval
		retryInterval := 5 * time.Minute // default
		if cfg.Notifications.Queue.RetryInterval != "" {
			parsed, err := time.ParseDuration(cfg.Notifications.Queue.RetryInterval)
			if err != nil {
				log.Warn("invalid retry_interval, using default 5m",
					"retry_interval", cfg.Notifications.Queue.RetryInterval,
					"error", err)
			} else {
				retryInterval = parsed
			}
		}

		queueConfig := notifications.QueueConfig{
			WorkerCount:   cfg.Notifications.Queue.WorkerCount,
			BufferSize:    cfg.Notifications.Queue.BufferSize,
			RetryInterval: retryInterval,
		}

		// Set defaults if not configured
		if queueConfig.WorkerCount == 0 {
			queueConfig.WorkerCount = 4
		}
		if queueConfig.BufferSize == 0 {
			queueConfig.BufferSize = 1000
		}

		app.notificationQueue = notifications.NewQueue(queueConfig, store, app.wsDeliverer)
		log.Info("notifications queue initialized",
			"workers", queueConfig.WorkerCount,
			"buffer", queueConfig.BufferSize)

		// Set connectivity checker for event-based retry
		app.notificationQueue.SetConnectivityChecker(wsAdapter)
		log.Info("notifications connectivity checker enabled")

		// Create notification service with device lister adapter
		deviceLister := notifications.NewDeviceListerAdapter(func() ([]notifications.DeviceInfo, error) {
			devices, err := store.ListDevices()
			if err != nil {
				return nil, err
			}
			result := make([]notifications.DeviceInfo, len(devices))
			for i, d := range devices {
				result[i] = notifications.DeviceInfo{ID: d.ID, Name: d.Name}
			}
			return result, nil
		})
		app.notificationService = notifications.NewService(
			app.notificationQueue,
			deviceLister,
			notifications.DefaultServiceConfig(),
		)
		log.Info("notification service initialized")

		// Set notification queue on service health checker
		if app.jobs.ServiceHealth != nil {
			app.jobs.ServiceHealth.SetNotificationQueue(app.notificationQueue)
			log.Info("health checker notifications enabled")
		}

		// Set notification queue on notification handler for retry delivery
		if app.handlers.Notifications != nil {
			app.handlers.Notifications.SetQueue(app.notificationQueue)
			log.Info("notification handler delivery enabled")
		}
	}

	// Initialize backup system
	if backupComps := componentBuilder.BuildBackupComponents(); backupComps != nil {
		app.managers.Backup = backupComps.Manager
		app.jobs.BackupScheduler = backupComps.Scheduler
		app.handlers.Backup = backupComps.Handler
	}

	// Initialize toolbox if enabled
	if toolboxComps := componentBuilder.BuildToolboxComponents(); toolboxComps != nil {
		app.services.Toolbox = toolboxComps.Manager
		app.jobs.ToolboxDeployer = toolboxComps.Deployer
		app.handlers.Toolbox = toolboxComps.Handler
	}

	// Initialize scripts if enabled
	// Pass Docker client to enable container-based script execution
	if scriptsComps := componentBuilder.BuildScriptsComponents(dockerClient); scriptsComps != nil {
		app.services.Scripts = scriptsComps.Manager
		app.jobs.ScriptScheduler = scriptsComps.Scheduler
		app.jobs.ScriptRunner = scriptsComps.Runner
		app.handlers.Scripts = scriptsComps.Handler

		// Wire up WebSocket notifier for async script execution notifications
		if app.managers.WebSocket != nil && scriptsComps.Runner != nil {
			wsAdapter := websocket.NewManagerAdapter(app.managers.WebSocket)
			notifier := scripts.NewWebSocketNotifier(wsAdapter, app.notificationQueue, scripts.NotifierConfig{
				TTL:              24 * time.Hour,
				MaxRetries:       3,
				MaxOutputInNotif: 10 * 1024, // 10KB max output in notification
			})
			scriptsComps.Runner.SetNotifier(notifier)
			log.Info("script runner WebSocket notifications enabled")
		}
	}

	// Initialize federation (P2P service discovery)
	app.managers.Peers, _ = componentBuilder.BuildFederationPeerManager(app.nekzusID, app.managers.WebSocket)

	// Wire federation catalog sync callbacks
	WireFederationCallbacks(app.managers.Peers, app.managers.Router)

	// Wire proxy cache eviction callbacks
	WireProxyCacheCallbacks(app.proxyCache, app.managers.Router)

	// Register health checkers
	app.registerHealthCheckers()

	// Load static routes and apps from config (these will be persisted)
	for _, route := range cfg.Routes {
		if err := app.managers.Router.UpsertRoute(route); err != nil {
			log.Warn("failed to save route",
				"route_id", route.RouteID,
				"error", err)
		}
	}
	for _, appConfig := range cfg.Apps {
		if err := app.managers.Router.UpsertApp(appConfig); err != nil {
			log.Warn("failed to save app",
				"app_id", appConfig.ID,
				"error", err)
		}
	}

	// Initialize discovery
	app.services.Discovery = discovery.NewDiscoveryManager(app, app, app)
	if err := app.setupDiscovery(); err != nil {
		store.Close()
		return nil, err
	}

	return app, nil
}

// setupDiscovery configures discovery workers
func (app *Application) setupDiscovery() error {
	if !app.config.Discovery.Enabled {
		return nil
	}

	// Docker discovery
	if app.config.Discovery.Docker.Enabled {
		interval, err := time.ParseDuration(app.config.Discovery.Docker.PollInterval)
		if err != nil {
			interval = 30 * time.Second
		}

		dockerWorker, err := discovery.NewDockerDiscoveryWorker(
			app.services.Discovery,
			app.config.Discovery.Docker.SocketPath,
			interval,
		)
		if err != nil {
			log.Warn("failed to create docker discovery worker",
				"error", err)
			log.Info("docker discovery will be disabled, ensure docker is running and accessible")

			// Publish event for configuration warning
			if app.managers.WebSocket != nil {
				warningMsg := fmt.Sprintf("Docker Discovery - Docker socket unavailable: %v", err)
				app.managers.WebSocket.PublishConfigWarning(warningMsg)
			}
		} else {
			// Configure network filtering
			dockerWorker.SetNetworkConfig(
				app.config.Discovery.Docker.Networks,
				app.config.Discovery.Docker.ExcludeNetworks,
				app.config.Discovery.Docker.NetworkMode,
			)

			// Configure self-identification to prevent discovering itself
			selfIdent := discovery.DetectSelfIdentity()
			dockerWorker.SetSelfIdentity(selfIdent)

			app.services.Discovery.RegisterWorker(dockerWorker)

			// Log network configuration
			if len(app.config.Discovery.Docker.Networks) > 0 {
				log.Info("docker worker configured",
					"networks", app.config.Discovery.Docker.Networks,
					"mode", app.config.Discovery.Docker.NetworkMode,
					"component", logger.CompDiscovery)
			} else if len(app.config.Discovery.Docker.ExcludeNetworks) > 0 {
				log.Info("docker worker excluding networks",
					"exclude_networks", app.config.Discovery.Docker.ExcludeNetworks,
					"component", logger.CompDiscovery)
			} else {
				log.Info("registered docker worker",
					"interval", interval,
					"mode", app.config.Discovery.Docker.NetworkMode,
					"component", logger.CompDiscovery)
			}
		}
	}

	// mDNS discovery
	if app.config.Discovery.MDNS.Enabled {
		interval, err := time.ParseDuration(app.config.Discovery.MDNS.ScanInterval)
		if err != nil {
			interval = 60 * time.Second
		}

		mdnsWorker := discovery.NewMDNSDiscoveryWorker(
			app.services.Discovery,
			app.config.Discovery.MDNS.Services,
			interval,
		)
		app.services.Discovery.RegisterWorker(mdnsWorker)
		log.Info("registered mdns worker",
			"interval", interval,
			"component", logger.CompDiscovery)
	}

	// Kubernetes discovery
	if app.config.Discovery.Kubernetes.Enabled {
		interval, err := time.ParseDuration(app.config.Discovery.Kubernetes.PollInterval)
		if err != nil {
			interval = 30 * time.Second
		}

		k8sWorker, err := discovery.NewKubernetesDiscoveryWorkerFromConfig(
			app.services.Discovery,
			app.config.Discovery.Kubernetes.Kubeconfig,
			interval,
			app.config.Discovery.Kubernetes.Namespaces...,
		)
		if err != nil {
			log.Warn("failed to create kubernetes discovery worker",
				"error", err)
			log.Info("kubernetes discovery will be disabled, ensure kubeconfig is accessible or running in-cluster")

			// Publish event for configuration warning
			if app.managers.WebSocket != nil {
				warningMsg := fmt.Sprintf("Kubernetes Discovery - Kubernetes API unavailable: %v", err)
				app.managers.WebSocket.PublishConfigWarning(warningMsg)
			}
		} else {
			app.services.Discovery.RegisterWorker(k8sWorker)
			if len(app.config.Discovery.Kubernetes.Namespaces) > 0 {
				log.Info("registered kubernetes worker",
					"interval", interval,
					"namespaces", app.config.Discovery.Kubernetes.Namespaces,
					"component", logger.CompDiscovery)
			} else {
				log.Info("registered kubernetes worker (all namespaces)",
					"interval", interval,
					"component", logger.CompDiscovery)
			}
		}
	}

	return nil
}

// registerHealthCheckers registers all health checkers
func (app *Application) registerHealthCheckers() {
	// Storage health checker
	if app.storage != nil {
		app.services.Health.RegisterChecker(health.NewStorageChecker(app.storage.DB()))
	}

	// Discovery health checker
	workersActive := func() int {
		// Get actual active worker count from discovery manager
		if app.services.Discovery != nil {
			return app.services.Discovery.ActiveWorkerCount()
		}
		return 0
	}
	app.services.Health.RegisterChecker(health.NewDiscoveryChecker(app.config.Discovery.Enabled, workersActive))

	// Auth health checker
	validateToken := func() error {
		// Simple validation - check if auth manager is initialized
		if app.services.Auth == nil {
			return fmt.Errorf("auth manager not initialized")
		}
		return nil
	}
	app.services.Health.RegisterChecker(health.NewAuthChecker(validateToken))

	// Proxy cache health checker
	cacheSize := func() int {
		if app.proxyCache != nil {
			stats := app.proxyCache.Stats()
			return stats["cached_proxies"]
		}
		return 0
	}
	app.services.Health.RegisterChecker(health.NewProxyCacheChecker(cacheSize))

	// WebSocket manager health checker
	activeConnections := func() int {
		if app.managers.WebSocket != nil {
			return app.managers.WebSocket.ActiveConnections()
		}
		return 0
	}
	app.services.Health.RegisterChecker(health.NewEventBusChecker(activeConnections))

	// Memory health checker (max 512MB)
	app.services.Health.RegisterChecker(health.NewMemoryChecker(constants.HealthCheckMemoryThresholdMB))

	// Certificate health checker
	if app.services.Certs != nil {
		getCertificates := func() []health.CertificateInfo {
			certs := app.services.Certs.GetAllCertificates()
			certInfos := make([]health.CertificateInfo, len(certs))
			for i, cert := range certs {
				certInfos[i] = health.CertificateInfo{
					Domain:   cert.Domain,
					NotAfter: cert.Metadata.NotAfter,
					Issuer:   cert.Metadata.Issuer,
				}
			}
			return certInfos
		}
		app.services.Health.RegisterChecker(health.NewCertificateChecker(getCertificates))
	}
}

// setupRoutes configures HTTP routes using the RouteBuilder pattern
func (app *Application) setupRoutes() http.Handler {
	builder := NewRouteBuilder(app)
	return builder.Build()
}

// Run starts the application
func (app *Application) Run() error {
	// Initialize and start config watcher
	if app.configPath != "" {
		watcher, err := config.NewWatcher(app.configPath, app.config, app.metrics)
		if err != nil {
			log.Warn("failed to create config watcher",
				"error", err)
		} else {
			app.configWatcher = watcher
			watcher.RegisterReloadHandler(app.handleConfigReload)

			if err := watcher.Start(); err != nil {
				log.Warn("failed to start config watcher",
					"error", err)
			}
		}
	}

	// Start metrics updater
	metricsTicker := time.NewTicker(15 * time.Second)
	defer metricsTicker.Stop()

	go func() {
		for range metricsTicker.C {
			app.updateSystemMetrics()
		}
	}()

	// Start discovery
	if app.config.Discovery.Enabled {
		if err := app.services.Discovery.Start(); err != nil {
			log.Warn("discovery failed to start",
				"error", err)
		}
	}

	// Start service health checker
	if app.jobs.ServiceHealth != nil {
		if err := app.jobs.ServiceHealth.Start(); err != nil {
			log.Warn("service health checker failed to start",
				"error", err)
		} else {
			log.Info("service health checker started",
				"interval", app.config.HealthChecks.Interval)
		}
	}

	// Start offline detection job
	if app.jobs.OfflineDetection != nil {
		if err := app.jobs.OfflineDetection.Start(); err != nil {
			log.Warn("offline detection job failed to start",
				"error", err)
		} else {
			log.Info("offline detection started",
				"threshold", "5m",
				"interval", "30s")
		}
	}

	// Start backup scheduler
	if app.jobs.BackupScheduler != nil {
		if err := app.jobs.BackupScheduler.Start(); err != nil {
			log.Warn("backup scheduler failed to start",
				"error", err)
		} else {
			log.Info("backup scheduler started")
		}
	}

	// Start script scheduler
	if app.jobs.ScriptScheduler != nil {
		// Load scripts from storage and set on scheduler
		registeredScripts, err := app.storage.ListScripts()
		if err != nil {
			log.Warn("failed to load scripts for scheduler",
				"error", err)
		} else {
			scriptsMap := make(map[string]*scripts.Script)
			for i := range registeredScripts {
				scriptsMap[registeredScripts[i].ID] = registeredScripts[i]
			}
			app.jobs.ScriptScheduler.SetScripts(scriptsMap)
		}

		ctx := context.Background()
		if err := app.jobs.ScriptScheduler.Start(ctx); err != nil {
			log.Warn("script scheduler failed to start",
				"error", err)
		} else {
			log.Info("script scheduler started")
		}
	}

	// Start script runner for async execution
	if app.jobs.ScriptRunner != nil {
		ctx := context.Background()
		if err := app.jobs.ScriptRunner.Start(ctx); err != nil {
			log.Warn("script runner failed to start",
				"error", err)
		} else {
			log.Info("script runner started")
		}
	}

	// Start notification queue
	if app.notificationQueue != nil {
		ctx := context.Background()
		if err := app.notificationQueue.Start(ctx); err != nil {
			log.Warn("notification queue failed to start",
				"error", err)
		} else {
			log.Info("notification queue started")
		}
	}

	// Set up device connect callback for notification retry
	if app.notificationQueue != nil && app.managers.WebSocket != nil {
		app.managers.WebSocket.SetOnDeviceConnect(func(deviceID string) {
			// When device reconnects, retry any pending notifications
			if err := app.notificationQueue.RetryDevice(deviceID); err != nil {
				log.Warn("failed to retry notifications for device",
					"device_id", deviceID,
					"error", err)
			} else {
				log.Info("retrying pending notifications for reconnected device",
					"device_id", deviceID)
			}
		})
		log.Info("device connect callback registered")
	}

	// Start federation peer manager
	if app.managers.Peers != nil {
		ctx := context.Background()
		if err := app.managers.Peers.Start(ctx); err != nil {
			log.Warn("federation peer manager failed to start",
				"error", err)
		} else {
			log.Info("federation peer manager started (discovering peers)",
				"component", logger.CompFederation)
		}
	}

	// Setup HTTP server
	app.httpServer = &http.Server{
		Addr:              app.config.Server.Addr,
		Handler:           app.setupRoutes(),
		ReadTimeout:       15 * time.Second,
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20, // 1 MB
	}

	// Initialize TLS hot-reload channels
	app.tlsUpgrade = make(chan struct{}, 1)
	app.serverDone = make(chan struct{})

	// Channel to listen for errors from the server
	serverErrors := make(chan error, 1)
	app.serverErrors = serverErrors

	// Start server in goroutine
	go func() {
		log.Info("nekzus starting",
			"version", app.version,
			"address", app.config.Server.Addr)

		// Determine if we should use TLS
		useTLS := false
		var tlsSource string

		if app.config.Server.TLSCert != "" && app.config.Server.TLSKey != "" {
			// Load the static certificate as fallback for SNI
			staticCert, err := tls.LoadX509KeyPair(
				app.config.Server.TLSCert,
				app.config.Server.TLSKey,
			)
			if err != nil {
				serverErrors <- fmt.Errorf("failed to load TLS certificate: %w", err)
				return
			}

			// Set the static cert as fallback for SNI misses
			app.services.Certs.SetFallbackCertificate(&staticCert)
			useTLS = true
			tlsSource = "file:" + app.config.Server.TLSCert
		} else if app.services.Certs.HasAnyCertificate() {
			// No static cert configured, but we have managed certificates
			// Auto-enable TLS using managed certificates
			useTLS = true
			tlsSource = "managed"
			log.Info("auto-enabling TLS using managed certificate")
		}

		if useTLS {
			// Configure TLS with SNI support
			app.httpServer.TLSConfig = &tls.Config{
				GetCertificate: app.services.Certs.GetCertificate,
				MinVersion:     tls.VersionTLS12,
			}

			// Mark TLS as enabled
			app.tlsEnabled.Store(true)

			log.Info("starting with tls (SNI enabled)",
				"source", tlsSource)

			// Use ListenAndServeTLS with empty strings since we're using GetCertificate
			err := app.httpServer.ListenAndServeTLS("", "")
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				serverErrors <- err
			}
		} else {
			log.Warn("starting without tls (http only - not recommended for production)")
			log.Info("tip: generate a certificate in Settings to auto-enable TLS, or restart server after generating")
			err := app.httpServer.ListenAndServe()
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				serverErrors <- err
			}
		}
		close(app.serverDone)
	}()

	// Channel to listen for interrupt signals
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	// Track if TLS upgrade is in progress
	tlsUpgradeInProgress := false

	// Event loop: handle server errors, shutdown signals, and TLS upgrades
	for {
		select {
		case err := <-serverErrors:
			if err != nil {
				return err
			}
		case <-app.serverDone:
			// Server goroutine exited cleanly
			if tlsUpgradeInProgress {
				// TLS upgrade in progress - the old server shut down, new one will start
				// Reset the done channel for the new server
				app.serverDone = make(chan struct{})
				tlsUpgradeInProgress = false
				continue
			}
			// Normal shutdown
			return nil
		case <-app.tlsUpgrade:
			// TLS upgrade requested - mark upgrade in progress and restart server
			tlsUpgradeInProgress = true
			go app.restartServerWithTLS(serverErrors)
		case sig := <-shutdown:
			log.Info("received signal, starting graceful shutdown",
				"signal", sig)

			// Create context with timeout for shutdown
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			// Attempt graceful shutdown
			if err := app.Shutdown(ctx); err != nil {
				return err
			}
			return nil
		}
	}
}

// Shutdown gracefully stops the application
func (app *Application) Shutdown(ctx context.Context) error {
	log.Info("shutting down")

	// Stop config watcher first
	if app.configWatcher != nil {
		if err := app.configWatcher.Stop(); err != nil {
			log.Error("config watcher shutdown error",
				"error", err)
		}
	}

	// Stop discovery
	if app.services.Discovery != nil {
		log.Info("stopping discovery workers")
		if err := app.services.Discovery.Stop(); err != nil {
			log.Error("discovery shutdown error",
				"error", err)
		}
	}

	// Stop service health checker
	if app.jobs.ServiceHealth != nil {
		log.Info("stopping service health checker")
		if err := app.jobs.ServiceHealth.Stop(); err != nil {
			log.Error("service health checker shutdown error",
				"error", err)
		}
	}

	// Stop offline detection job
	if app.jobs.OfflineDetection != nil {
		log.Info("stopping offline detection job")
		if err := app.jobs.OfflineDetection.Stop(); err != nil {
			log.Error("offline detection job shutdown error",
				"error", err)
		}
	}

	// Stop backup scheduler
	if app.jobs.BackupScheduler != nil {
		log.Info("stopping backup scheduler")
		if err := app.jobs.BackupScheduler.Stop(); err != nil {
			log.Error("backup scheduler shutdown error",
				"error", err)
		}
	}

	// Stop script scheduler
	if app.jobs.ScriptScheduler != nil {
		log.Info("stopping script scheduler")
		if err := app.jobs.ScriptScheduler.Stop(); err != nil {
			log.Error("script scheduler shutdown error",
				"error", err)
		}
	}

	// Stop script runner
	if app.jobs.ScriptRunner != nil {
		log.Info("stopping script runner")
		app.jobs.ScriptRunner.Stop()
	}

	// Stop notification queue
	if app.notificationQueue != nil {
		log.Info("stopping notification queue")
		app.notificationQueue.Stop()
	}

	// Stop ACK tracker
	if app.ackTracker != nil {
		log.Info("stopping notification ACK tracker")
		app.ackTracker.Stop()
	}

	// Stop federation peer manager
	if app.managers.Peers != nil {
		log.Info("stopping federation peer manager")
		if err := app.managers.Peers.Stop(); err != nil {
			log.Error("federation peer manager shutdown error",
				"error", err)
		}
	}

	// Stop pairing manager
	if app.managers.Pairing != nil {
		log.Info("stopping pairing manager")
		app.managers.Pairing.Stop()
	}

	// Stop rate limiters
	if app.limiters.QR != nil {
		app.limiters.QR.Stop()
	}
	if app.limiters.Auth != nil {
		app.limiters.Auth.Stop()
	}
	if app.limiters.Device != nil {
		app.limiters.Device.Stop()
	}
	if app.limiters.Container != nil {
		app.limiters.Container.Stop()
	}
	if app.limiters.Health != nil {
		app.limiters.Health.Stop()
	}
	if app.limiters.Metrics != nil {
		app.limiters.Metrics.Stop()
	}
	if app.limiters.WebSocket != nil {
		app.limiters.WebSocket.Stop()
	}

	// Close Docker client
	// Shutdown HTTP server first to stop accepting new requests
	if app.httpServer != nil {
		log.Info("stopping http server")
		if err := app.httpServer.Shutdown(ctx); err != nil {
			log.Error("http server shutdown error",
				"error", err)
			// Force close if graceful shutdown fails
			_ = app.httpServer.Close()
		}
	}

	if app.dockerClient != nil {
		log.Info("closing docker client")
		if err := app.dockerClient.Close(); err != nil {
			log.Error("docker client close error",
				"error", err)
		}
	}

	// Close storage last, after all clients have disconnected
	if app.storage != nil {
		log.Info("closing database")
		if err := app.storage.Close(); err != nil {
			log.Error("storage shutdown error",
				"error", err)
		}
	}

	log.Info("shutdown complete")
	return nil
}

// IsTLSEnabled returns whether the server is currently running with TLS
func (app *Application) IsTLSEnabled() bool {
	return app.tlsEnabled.Load()
}

// UpdateBaseURL updates the base URL after TLS upgrade.
// This changes http:// to https:// and propagates to all handlers.
func (app *Application) UpdateBaseURL() {
	if strings.HasPrefix(app.baseURL, "http://") {
		app.baseURL = strings.Replace(app.baseURL, "http://", "https://", 1)
		log.Info("base URL updated for TLS", "base_url", app.baseURL)

		// Update the AuthHandler's base URL for QR code generation
		if app.handlers != nil && app.handlers.Auth != nil {
			app.handlers.Auth.SetBaseURL(app.baseURL)
		}

		// Update the QRHandler's base URL for v2 pairing flow
		if app.handlers != nil && app.handlers.QR != nil {
			app.handlers.QR.SetBaseURL(app.baseURL)
		}
	}
}

// UpgradeToTLS signals the server to upgrade from HTTP to HTTPS
// This is called after a certificate is generated while running in HTTP mode
func (app *Application) UpgradeToTLS() error {
	if app.tlsEnabled.Load() {
		log.Debug("TLS already enabled, no upgrade needed")
		return nil
	}

	// Verify we have a certificate available
	if !app.services.Certs.HasAnyCertificate() {
		return fmt.Errorf("no certificate available for TLS upgrade")
	}

	// Signal the server to upgrade
	select {
	case app.tlsUpgrade <- struct{}{}:
		log.Info("TLS upgrade signal sent")
		return nil
	default:
		// Channel is full or closed, upgrade already in progress
		log.Debug("TLS upgrade already in progress")
		return nil
	}
}

// restartServerWithTLS gracefully restarts the server with TLS enabled
func (app *Application) restartServerWithTLS(serverErrors chan error) {
	log.Info("upgrading server to TLS")

	// Shutdown the current HTTP server gracefully
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := app.httpServer.Shutdown(ctx); err != nil {
		log.Error("error shutting down HTTP server for TLS upgrade",
			"error", err)
		// Force close if graceful shutdown fails
		_ = app.httpServer.Close()
	}

	// Create new server with TLS config
	app.httpServer = &http.Server{
		Addr:              app.config.Server.Addr,
		Handler:           app.setupRoutes(),
		ReadTimeout:       15 * time.Second,
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20, // 1 MB
		TLSConfig: &tls.Config{
			GetCertificate: app.services.Certs.GetCertificate,
			MinVersion:     tls.VersionTLS12,
		},
	}

	// Mark TLS as enabled
	app.tlsEnabled.Store(true)

	// Update base URL to use https://
	app.UpdateBaseURL()

	log.Info("starting server with TLS (hot-reload)",
		"address", app.config.Server.Addr,
		"base_url", app.baseURL,
		"source", "managed")

	// Start the new HTTPS server
	err := app.httpServer.ListenAndServeTLS("", "")
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		serverErrors <- err
	}
	// Signal that server has exited
	close(app.serverDone)
}

// AddProposal implements discovery.ProposalStore interface
func (app *Application) AddProposal(p *types.Proposal) {
	// Persist proposal to storage
	if app.storage != nil {
		if err := app.storage.SaveProposal(*p); err != nil {
			log.Warn("failed to save proposal",
				"proposal_id", p.ID,
				"error", err)
		}
	}
	log.Info("new proposal",
		"proposal_id", p.ID,
		"source", p.Source,
		"confidence", p.Confidence,
		"component", logger.CompDiscovery)
}

// Publish implements discovery.EventBus interface
func (app *Application) Publish(ev discovery.Event) {
	if app.managers.WebSocket != nil {
		app.managers.WebSocket.PublishDiscoveryEvent(ev)
	}
}

// HasRouteForApp implements discovery.RouteChecker interface
// Returns true if the app already has a route configured
func (app *Application) HasRouteForApp(appID string) bool {
	if app.managers.Router == nil {
		return false
	}
	_, exists := app.managers.Router.GetRouteByAppID(appID)
	return exists
}

// updateSystemMetrics updates system-level metrics
func (app *Application) updateSystemMetrics() {
	// Update uptime
	app.metrics.UpdateUptime(app.startTime)

	// Update storage metrics
	if app.storage != nil {
		if devices, err := app.storage.ListDevices(); err == nil {
			app.metrics.DevicesTotal.Set(float64(len(devices)))

			// Update device last seen ages
			for _, device := range devices {
				app.metrics.UpdateDeviceLastSeen(device.ID, device.LastSeen)
			}
		}

		// Count apps
		apps := app.managers.Router.ListApps()
		app.metrics.StorageAppsTotal.Set(float64(len(apps)))

		// Update discovery metrics
		if proposals, err := app.storage.ListProposals(); err == nil {
			app.metrics.DiscoveryProposalsPending.Set(float64(len(proposals)))
		}
	}
}

// handleConfigReload is called when configuration file changes
func (app *Application) handleConfigReload(oldConfig, newConfig types.ServerConfig) error {
	log.Info("config reload: applying changes")

	// Reload routes and apps
	if err := app.managers.Router.ReplaceRoutes(newConfig.Routes); err != nil {
		return fmt.Errorf("failed to reload routes: %w", err)
	}
	if err := app.managers.Router.ReplaceApps(newConfig.Apps); err != nil {
		return fmt.Errorf("failed to reload apps: %w", err)
	}

	// Reload bootstrap tokens
	if len(newConfig.Bootstrap.Tokens) != len(oldConfig.Bootstrap.Tokens) {
		app.services.Auth.UpdateBootstrapTokens(newConfig.Bootstrap.Tokens)
		app.metrics.AuthBootstrapTokensActive.Set(float64(len(newConfig.Bootstrap.Tokens)))
		log.Info("config reload: updated bootstrap tokens",
			"count", len(newConfig.Bootstrap.Tokens))
	}

	// Reload discovery intervals (requires restart of discovery workers)
	if newConfig.Discovery.Enabled != oldConfig.Discovery.Enabled ||
		newConfig.Discovery.Docker.PollInterval != oldConfig.Discovery.Docker.PollInterval ||
		newConfig.Discovery.MDNS.ScanInterval != oldConfig.Discovery.MDNS.ScanInterval ||
		newConfig.Discovery.Kubernetes.PollInterval != oldConfig.Discovery.Kubernetes.PollInterval {

		log.Info("config reload: discovery settings changed, restarting workers")

		// Stop existing discovery
		if app.services.Discovery != nil {
			if err := app.services.Discovery.Stop(); err != nil {
				log.Warn("failed to stop discovery",
					"error", err)
			}
		}

		// Update config
		app.config.Discovery = newConfig.Discovery

		// Reinitialize discovery with new settings
		app.services.Discovery = discovery.NewDiscoveryManager(app, app, app)
		if err := app.setupDiscovery(); err != nil {
			return fmt.Errorf("failed to reinitialize discovery: %w", err)
		}

		// Restart if enabled
		if newConfig.Discovery.Enabled {
			if err := app.services.Discovery.Start(); err != nil {
				log.Warn("failed to restart discovery",
					"error", err)
			}
		}
	}

	// Reload health check settings (requires restart of health checker)
	if newConfig.HealthChecks.Enabled != oldConfig.HealthChecks.Enabled ||
		newConfig.HealthChecks.Interval != oldConfig.HealthChecks.Interval ||
		newConfig.HealthChecks.Timeout != oldConfig.HealthChecks.Timeout ||
		newConfig.HealthChecks.UnhealthyThreshold != oldConfig.HealthChecks.UnhealthyThreshold {

		log.Info("config reload: health check settings changed, restarting checker")

		// Stop existing health checker
		if app.jobs.ServiceHealth != nil {
			if err := app.jobs.ServiceHealth.Stop(); err != nil {
				log.Warn("failed to stop health checker",
					"error", err)
			}
		}

		// Update config
		app.config.HealthChecks = newConfig.HealthChecks

		// Reinitialize if enabled
		if newConfig.HealthChecks.Enabled {
			app.jobs.ServiceHealth = health.NewServiceHealthChecker(
				newConfig.HealthChecks,
				app.managers.Router,
				app.storage,
				app.metrics,
			)

			// Set WebSocket notifier if available
			if app.managers.WebSocket != nil {
				app.jobs.ServiceHealth.SetWebSocketNotifier(app.managers.WebSocket)
			}

			if err := app.jobs.ServiceHealth.Start(); err != nil {
				log.Warn("failed to restart health checker",
					"error", err)
			} else {
				log.Info("config reload: health checker restarted",
					"interval", newConfig.HealthChecks.Interval)
			}
		}
	}

	// Reload metrics endpoint settings
	if oldConfig.Metrics.Enabled != newConfig.Metrics.Enabled {
		app.metricsEnabled.Store(newConfig.Metrics.Enabled)

		if newConfig.Metrics.Enabled {
			log.Info("config reload: metrics endpoint enabled",
				"path", newConfig.Metrics.Path)
		} else {
			log.Info("config reload: metrics endpoint disabled")
		}
	}

	// Update the stored config
	app.config = newConfig

	// Publish event for config reload
	if app.managers.WebSocket != nil {
		app.managers.WebSocket.PublishConfigReload()
	}

	// Add to activity feed
	if app.managers.Activity != nil {
		app.managers.Activity.Add(types.ActivityEvent{
			ID:        "config_reload_" + fmt.Sprint(time.Now().Unix()),
			Type:      "config_reload",
			Icon:      "RefreshCw",
			IconClass: "",
			Message:   "Configuration reloaded",
			Timestamp: time.Now().UnixMilli(),
		})
	}

	log.Info("config reload: completed successfully")
	return nil
}

// getLocalNetworkIP detects the local network IP address for QR code pairing
// Returns the first non-loopback, non-Docker IPv4 address found, or "localhost" as fallback
func getLocalNetworkIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		log.Warn("failed to detect network interfaces, using localhost",
			"error", err)
		return "localhost"
	}

	var dockerIP string

	// Find the first non-loopback IPv4 address, preferring non-Docker IPs
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipv4 := ipnet.IP.To4(); ipv4 != nil {
				ip := ipv4.String()

				// Skip common Docker network ranges (172.17-31.x.x)
				if strings.HasPrefix(ip, "172.") {
					octets := strings.Split(ip, ".")
					if len(octets) >= 2 {
						// Parse second octet
						second := 0
						fmt.Sscanf(octets[1], "%d", &second)
						// Docker uses 172.17.0.0/16 through 172.31.0.0/16
						if second >= 17 && second <= 31 {
							dockerIP = ip // Save as fallback but keep looking
							continue
						}
					}
				}

				// Prefer 192.168.x.x or 10.x.x.x (common home/office networks)
				if strings.HasPrefix(ip, "192.168.") || strings.HasPrefix(ip, "10.") {
					return ip
				}

				// Return any other non-Docker private IP
				if strings.HasPrefix(ip, "172.") {
					octets := strings.Split(ip, ".")
					if len(octets) >= 2 {
						second := 0
						fmt.Sscanf(octets[1], "%d", &second)
						// Use 172.x ranges outside Docker's allocation (172.16.x.x or 172.32+)
						if second == 16 || second >= 32 {
							return ip
						}
					}
				}
			}
		}
	}

	// If we only found Docker IP, use it as last resort
	if dockerIP != "" {
		log.Warn("only docker network ip found, mobile devices may not be able to connect",
			"ip", dockerIP)
		return dockerIP
	}

	// Fallback to localhost if no network IP found
	log.Warn("no network ip addresses found, using localhost")
	return "localhost"
}

// runHealthCheck performs a health check against the running server.
// Returns 0 if healthy, 1 if unhealthy.
func runHealthCheck(addr string) int {
	client := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	url := strings.TrimSuffix(addr, "/") + "/api/v1/healthz"
	resp, err := client.Get(url)
	if err != nil {
		fmt.Fprintf(os.Stderr, "health check failed: %v\n", err)
		return 1
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		fmt.Println("healthy")
		return 0
	}

	fmt.Fprintf(os.Stderr, "unhealthy: status %d\n", resp.StatusCode)
	return 1
}

func main() {
	// Parse flags
	var (
		cfgFile     = flag.String("config", "configs/config.example.yaml", "path to config file")
		insecure    = flag.Bool("insecure-http", false, "serve HTTP without TLS (dev only)")
		healthCheck = flag.Bool("health", false, "perform health check and exit")
		healthAddr  = flag.String("health-addr", "http://localhost:8080", "address for health check")
	)
	flag.Parse()

	// Health check mode - used by Docker HEALTHCHECK
	if *healthCheck {
		os.Exit(runHealthCheck(*healthAddr))
	}

	// Initialize structured logging
	logLevel := slog.LevelInfo
	if os.Getenv("NEKZUS_DEBUG") == "true" || os.Getenv("NEKZUS_DEBUG") == "1" {
		logLevel = slog.LevelDebug
	}
	logger.SetupText(logLevel)
	slog.Info("Nekzus starting", "log_level", logLevel.String())

	// Load and validate configuration
	cfg, err := config.Load(*cfgFile)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		slog.Error("Configuration error", "error", err)
		os.Exit(1)
	}

	// Override TLS settings if insecure flag is set
	if *insecure {
		cfg.Server.TLSCert = ""
		cfg.Server.TLSKey = ""
	}

	// Auto-generate TLS certificates if needed
	if cfg.Server.TLSCert != "" && cfg.Server.TLSKey != "" {
		generated, err := tlsutil.EnsureCertificates(cfg.Server.TLSCert, cfg.Server.TLSKey)
		if err != nil {
			slog.Error("TLS certificate error: %v", "error", err)
			os.Exit(1)
		}
		if generated {
			log.Info("generated self-signed tls certificate",
				"cert", cfg.Server.TLSCert,
				"key", cfg.Server.TLSKey)
			log.Info("certificate valid for 1 year with SANs for localhost and local IPs")
		}

		// Validate that certificates are loadable
		if err := tlsutil.ValidateCertificates(cfg.Server.TLSCert, cfg.Server.TLSKey); err != nil {
			slog.Error("TLS certificate validation failed: %v", "error", err)
			os.Exit(1)
		}
		slog.Info("TLS certificates validated successfully")
	}

	// Set defaults and apply environment overrides
	config.SetDefaults(&cfg)
	config.ApplyEnvOverrides(&cfg)

	// Create application
	app, err := NewApplication(cfg, *cfgFile)
	if err != nil {
		slog.Error("Failed to initialize application: %v", "error", err)
		os.Exit(1)
	}

	// Run application
	if err := app.Run(); err != nil {
		slog.Error("Application error: %v", "error", err)
		os.Exit(1)
	}
}
