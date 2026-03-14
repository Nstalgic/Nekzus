package main

import (
	"fmt"
	"net/url"
	"os"
	"time"

	"github.com/docker/docker/client"
	"github.com/nstalgic/nekzus/internal/backup"
	"github.com/nstalgic/nekzus/internal/federation"
	"github.com/nstalgic/nekzus/internal/handlers"
	"github.com/nstalgic/nekzus/internal/health"
	"github.com/nstalgic/nekzus/internal/metrics"
	"github.com/nstalgic/nekzus/internal/notifications"
	"github.com/nstalgic/nekzus/internal/proxy"
	"github.com/nstalgic/nekzus/internal/router"
	"github.com/nstalgic/nekzus/internal/runtime"
	dockerruntime "github.com/nstalgic/nekzus/internal/runtime/docker"
	k8sruntime "github.com/nstalgic/nekzus/internal/runtime/kubernetes"
	"github.com/nstalgic/nekzus/internal/scripts"
	"github.com/nstalgic/nekzus/internal/storage"
	"github.com/nstalgic/nekzus/internal/toolbox"
	"github.com/nstalgic/nekzus/internal/types"
	"github.com/nstalgic/nekzus/internal/websocket"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	metricsv "k8s.io/metrics/pkg/client/clientset/versioned"
)

// ComponentBuilder helps build application components with common dependencies
type ComponentBuilder struct {
	cfg       types.ServerConfig
	store     *storage.Store
	metrics   *metrics.Metrics
	startTime time.Time
	version   string
}

// NewComponentBuilder creates a new ComponentBuilder
func NewComponentBuilder(cfg types.ServerConfig, store *storage.Store, m *metrics.Metrics, startTime time.Time, version string) *ComponentBuilder {
	return &ComponentBuilder{
		cfg:       cfg,
		store:     store,
		metrics:   m,
		startTime: startTime,
		version:   version,
	}
}

// ContainerComponents holds container management components
type ContainerComponents struct {
	DockerClient     *client.Client
	RuntimeRegistry  *runtime.Registry
	ContainerHandler *handlers.ContainerHandler
	LogsHandler      *handlers.ContainerLogsHandler
	ExportHandler    *handlers.ExportHandler
}

// BuildDockerClient creates a Docker client if Docker discovery is enabled
// Deprecated: Use BuildContainerComponents instead for runtime abstraction
func (cb *ComponentBuilder) BuildDockerClient() (*client.Client, *handlers.ContainerHandler, *handlers.ContainerLogsHandler, *handlers.ExportHandler) {
	comps := cb.BuildContainerComponents()
	if comps == nil {
		return nil, nil, nil, nil
	}
	return comps.DockerClient, comps.ContainerHandler, comps.LogsHandler, comps.ExportHandler
}

// BuildContainerComponents creates container management components with runtime abstraction
func (cb *ComponentBuilder) BuildContainerComponents() *ContainerComponents {
	// Check if any container runtime is enabled
	dockerEnabled := cb.cfg.Discovery.Docker.Enabled
	k8sEnabled := cb.cfg.Discovery.Kubernetes.Enabled

	if !dockerEnabled && !k8sEnabled {
		return nil
	}

	// Create runtime registry
	runtimeRegistry := runtime.NewRegistry()
	var dockerClient *client.Client

	// Register Docker runtime if enabled
	if dockerEnabled {
		opts := []client.Opt{
			client.WithAPIVersionNegotiation(),
		}
		if cb.cfg.Discovery.Docker.SocketPath != "" {
			opts = append(opts, client.WithHost(cb.cfg.Discovery.Docker.SocketPath))
		} else {
			opts = append(opts, client.FromEnv)
		}

		cli, err := client.NewClientWithOpts(opts...)
		if err != nil {
			log.Warn("failed to create docker client for container management", "error", err)
		} else {
			dockerClient = cli
			dockerRT := dockerruntime.NewRuntime(cli)
			if err := runtimeRegistry.Register(dockerRT); err != nil {
				log.Warn("failed to register docker runtime", "error", err)
			} else {
				log.Info("docker runtime registered",
					"socket", cb.cfg.Discovery.Docker.SocketPath)
			}
		}
	}

	// Register Kubernetes runtime if enabled
	if k8sEnabled {
		k8sClient, metricsClient, err := cb.createKubernetesClients()
		if err != nil {
			log.Warn("failed to create kubernetes client for container management", "error", err)
		} else {
			cfg := &k8sruntime.Config{
				Namespaces: cb.cfg.Discovery.Kubernetes.Namespaces,
			}
			k8sRT := k8sruntime.NewRuntimeWithConfig(k8sClient, metricsClient, cfg)
			if err := runtimeRegistry.Register(k8sRT); err != nil {
				log.Warn("failed to register kubernetes runtime", "error", err)
			} else {
				log.Info("kubernetes runtime registered",
					"namespaces", cb.cfg.Discovery.Kubernetes.Namespaces,
					"metrics", metricsClient != nil)
			}
		}
	}

	// Verify at least one runtime is available
	if len(runtimeRegistry.Available()) == 0 {
		log.Warn("no container runtimes available")
		log.Info("container management endpoints will be disabled")
		return nil
	}

	// Create handlers with runtime support
	containerHandler := handlers.NewContainerHandlerWithRuntime(runtimeRegistry, cb.store)
	containerLogsHandler := handlers.NewContainerLogsHandlerWithRuntime(runtimeRegistry, cb.store)

	// Export handler requires Docker client (Docker-specific feature)
	var exportHandler *handlers.ExportHandler
	if dockerClient != nil {
		exportHandler = handlers.NewExportHandler(dockerClient)
	}

	log.Info("container management enabled",
		"runtimes", runtimeRegistry.Available(),
		"primary", runtimeRegistry.GetPrimary().Name())

	return &ContainerComponents{
		DockerClient:     dockerClient,
		RuntimeRegistry:  runtimeRegistry,
		ContainerHandler: containerHandler,
		LogsHandler:      containerLogsHandler,
		ExportHandler:    exportHandler,
	}
}

// createKubernetesClients creates Kubernetes core and metrics clients from config
func (cb *ComponentBuilder) createKubernetesClients() (kubernetes.Interface, metricsv.Interface, error) {
	var config *rest.Config
	var err error

	kubeconfigPath := cb.cfg.Discovery.Kubernetes.Kubeconfig
	if kubeconfigPath == "" {
		// Try in-cluster config first
		config, err = rest.InClusterConfig()
		if err != nil {
			// Fall back to default kubeconfig location
			config, err = clientcmd.BuildConfigFromFlags("", clientcmd.RecommendedHomeFile)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to create kubernetes config: %w", err)
			}
		}
	} else {
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfigPath)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to load kubeconfig from %s: %w", kubeconfigPath, err)
		}
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	// Try to create metrics client (optional - may not be available)
	var metricsClient metricsv.Interface
	mc, err := metricsv.NewForConfig(config)
	if err != nil {
		log.Warn("failed to create kubernetes metrics client (stats will be unavailable)", "error", err)
	} else {
		metricsClient = mc
	}

	return clientset, metricsClient, nil
}

// BuildHealthManager creates the health manager
func (cb *ComponentBuilder) BuildHealthManager() *health.Manager {
	return health.NewManager("1.0.0", cb.startTime, cb.metrics)
}

// BuildServiceHealthChecker creates the service health checker
func (cb *ComponentBuilder) BuildServiceHealthChecker(router *router.Registry) *health.ServiceHealthChecker {
	if !cb.cfg.HealthChecks.Enabled {
		return nil
	}

	checker := health.NewServiceHealthChecker(
		cb.cfg.HealthChecks,
		router,
		cb.store,
		cb.metrics,
	)
	return checker
}

// BuildNotificationQueue creates the notification queue
func (cb *ComponentBuilder) BuildNotificationQueue(wsManager *websocket.Manager) *notifications.Queue {
	if !cb.cfg.Notifications.Enabled || wsManager == nil {
		return nil
	}

	wsAdapter := websocket.NewManagerAdapter(wsManager)
	wsDeliverer := notifications.NewWebSocketDeliverer(wsAdapter)
	log.Info("websocket deliverer initialized")

	// Parse retry interval
	retryInterval := 5 * time.Minute // default
	if cb.cfg.Notifications.Queue.RetryInterval != "" {
		parsed, err := time.ParseDuration(cb.cfg.Notifications.Queue.RetryInterval)
		if err != nil {
			log.Warn("invalid retry_interval, using default 5m", "retry_interval", cb.cfg.Notifications.Queue.RetryInterval, "error", err)
		} else {
			retryInterval = parsed
		}
	}

	queueConfig := notifications.QueueConfig{
		WorkerCount:   cb.cfg.Notifications.Queue.WorkerCount,
		BufferSize:    cb.cfg.Notifications.Queue.BufferSize,
		RetryInterval: retryInterval,
	}

	// Set defaults if not configured
	if queueConfig.WorkerCount == 0 {
		queueConfig.WorkerCount = 4
	}
	if queueConfig.BufferSize == 0 {
		queueConfig.BufferSize = 1000
	}

	queue := notifications.NewQueue(queueConfig, cb.store, wsDeliverer)
	log.Info("notification queue initialized", "workers", queueConfig.WorkerCount, "buffer", queueConfig.BufferSize, "retry", queueConfig.RetryInterval)

	return queue
}

// BackupComponents holds backup-related components
type BackupComponents struct {
	Manager   *backup.Manager
	Scheduler *backup.Scheduler
	Handler   *handlers.BackupHandler
}

// BuildBackupComponents creates backup-related components
func (cb *ComponentBuilder) BuildBackupComponents() *BackupComponents {
	if !cb.cfg.Backup.Enabled {
		return nil
	}

	// Ensure backup directory exists
	if err := os.MkdirAll(cb.cfg.Backup.Directory, 0755); err != nil {
		log.Warn("failed to create backup directory", "error", err)
		return nil
	}

	// Create backup manager
	manager := backup.NewManager(cb.store, cb.cfg.Backup.Directory, cb.version)
	log.Info("backup manager initialized", "directory", cb.cfg.Backup.Directory, "version", cb.version)

	// Parse schedule interval
	scheduleInterval := 24 * time.Hour // default
	if cb.cfg.Backup.Schedule != "" {
		parsed, err := time.ParseDuration(cb.cfg.Backup.Schedule)
		if err != nil {
			log.Warn("invalid backup schedule, using default 24h", "schedule", cb.cfg.Backup.Schedule, "error", err)
		} else {
			scheduleInterval = parsed
		}
	}

	// Set default retention if not configured
	retention := cb.cfg.Backup.Retention
	if retention == 0 {
		retention = 7 // Keep 7 backups by default
	}

	// Create backup scheduler
	scheduler := backup.NewScheduler(manager, scheduleInterval, retention)
	log.Info("backup scheduler created", "interval", scheduleInterval, "retention", retention)

	// Create backup handler
	handler := handlers.NewBackupHandler(manager, scheduler, cb.store)
	log.Info("backup handler initialized")

	return &BackupComponents{
		Manager:   manager,
		Scheduler: scheduler,
		Handler:   handler,
	}
}

// ToolboxComponents holds toolbox-related components
type ToolboxComponents struct {
	Manager  *toolbox.Manager
	Deployer *toolbox.Deployer
	Handler  *handlers.ToolboxHandler
}

// BuildToolboxComponents creates toolbox-related components
func (cb *ComponentBuilder) BuildToolboxComponents() *ToolboxComponents {
	if !cb.cfg.Toolbox.Enabled {
		return nil
	}

	// Ensure toolbox data directory exists
	if err := os.MkdirAll(cb.cfg.Toolbox.DataDir, 0755); err != nil {
		log.Warn("failed to create toolbox data directory", "error", err)
		return nil
	}

	// Create toolbox manager
	// Prefer CatalogDir (Compose-based) over deprecated CatalogPath (YAML)
	catalogPath := cb.cfg.Toolbox.CatalogDir
	if catalogPath == "" {
		catalogPath = cb.cfg.Toolbox.CatalogPath
	}

	manager := toolbox.NewManager(catalogPath)
	if err := manager.LoadCatalog(); err != nil {
		log.Warn("failed to load toolbox catalog", "error", err)
		return nil
	}

	services := manager.ListServices()
	log.Info("toolbox catalog loaded", "services", len(services))

	// Create toolbox deployer
	deployer, err := toolbox.NewDeployer(cb.cfg.Toolbox.DataDir, cb.cfg.Toolbox.HostDataDir)
	if err != nil {
		log.Warn("failed to create toolbox deployer", "error", err)
		return nil
	}

	log.Info("toolbox deployer initialized", "dataDir", cb.cfg.Toolbox.DataDir, "hostDataDir", cb.cfg.Toolbox.HostDataDir)

	// Create toolbox handler with base URL for service configuration
	handler := handlers.NewToolboxHandler(manager, deployer, cb.store, cb.cfg.Server.BaseURL)
	log.Info("toolbox handler initialized", "baseURL", cb.cfg.Server.BaseURL)

	return &ToolboxComponents{
		Manager:  manager,
		Deployer: deployer,
		Handler:  handler,
	}
}

// ScriptsComponents holds script execution related components
type ScriptsComponents struct {
	Manager           *scripts.Manager
	Executor          *scripts.Executor
	ContainerExecutor *scripts.ContainerExecutor
	Runner            *scripts.Runner
	Scheduler         *scripts.Scheduler
	Handler           *handlers.ScriptsHandler
}

// BuildScriptsComponents creates script execution related components
// If dockerClient is provided and Docker is available, scripts will execute in containers
func (cb *ComponentBuilder) BuildScriptsComponents(dockerClient *client.Client) *ScriptsComponents {
	if !cb.cfg.Scripts.Enabled {
		return nil
	}

	// Ensure scripts directory exists
	if cb.cfg.Scripts.Directory == "" {
		log.Warn("scripts directory not configured")
		return nil
	}

	if err := os.MkdirAll(cb.cfg.Scripts.Directory, 0755); err != nil {
		log.Warn("failed to create scripts directory", "error", err)
		return nil
	}

	// Create script manager
	manager := scripts.NewManager(cb.cfg.Scripts.Directory)
	log.Info("script manager initialized", "directory", cb.cfg.Scripts.Directory)

	// Set default timeout
	defaultTimeout := cb.cfg.Scripts.DefaultTimeout
	if defaultTimeout == 0 {
		defaultTimeout = 300 // 5 minutes
	}

	// Set default max output bytes
	maxOutputBytes := cb.cfg.Scripts.MaxOutputBytes
	if maxOutputBytes == 0 {
		maxOutputBytes = 10 * 1024 * 1024 // 10MB
	}

	// Create local executor (used for workflow runner and as fallback)
	executor := scripts.NewExecutor(manager, scripts.ExecutorConfig{
		DefaultTimeout: time.Duration(defaultTimeout) * time.Second,
		MaxOutputBytes: maxOutputBytes,
	})
	log.Info("script executor initialized", "timeout", defaultTimeout, "max_output", maxOutputBytes)

	// Create workflow runner
	workflowRunner := scripts.NewWorkflowRunner(executor)
	log.Info("script workflow runner initialized")

	// Determine which executor to use for the async runner
	// If Docker is available, use container executor for better isolation
	var runnerExecutor scripts.ScriptExecutor = executor
	var containerExecutor *scripts.ContainerExecutor

	if dockerClient != nil {
		// Get the host scripts directory for Docker bind mount
		scriptsHostDir := cb.cfg.Scripts.HostDirectory
		if scriptsHostDir == "" {
			scriptsHostDir = cb.cfg.Scripts.Directory
		}

		dockerAdapter := scripts.NewDockerClientAdapter(dockerClient)
		containerExecutor = scripts.NewContainerExecutor(dockerAdapter, scriptsHostDir, scripts.ContainerExecutorConfig{
			DefaultTimeout:   time.Duration(defaultTimeout) * time.Second,
			MaxOutputBytes:   maxOutputBytes,
			ShellImage:       "alpine:3.20",
			PythonImage:      "python:3.12-alpine",
			ScriptsMountPath: "/scripts",
		})
		runnerExecutor = containerExecutor
		log.Info("container executor initialized",
			"scripts_host_dir", scriptsHostDir,
			"shell_image", "alpine:3.20",
			"python_image", "python:3.12-alpine")
	}

	// Create async execution runner (notifier will be set later when WebSocket is available)
	runner := scripts.NewRunner(runnerExecutor, cb.store, nil, scripts.RunnerConfig{
		WorkerCount: 3,
		QueueSize:   100,
	})
	log.Info("script runner initialized",
		"workers", 3,
		"queue_size", 100,
		"container_mode", dockerClient != nil)

	// Create scheduler (uses local executor for scheduled jobs)
	scheduler := scripts.NewScheduler(executor, cb.store, scripts.SchedulerConfig{
		CheckInterval: 1 * time.Minute,
	})
	log.Info("script scheduler initialized")

	// Create handler with runner for async execution support
	handler := handlers.NewScriptsHandler(manager, executor, runner, workflowRunner, scheduler, cb.store)
	log.Info("scripts handler initialized")

	return &ScriptsComponents{
		Manager:           manager,
		Executor:          executor,
		ContainerExecutor: containerExecutor,
		Runner:            runner,
		Scheduler:         scheduler,
		Handler:           handler,
	}
}

// BuildFederationPeerManager creates the federation peer manager
func (cb *ComponentBuilder) BuildFederationPeerManager(nexusID string, wsManager *websocket.Manager) (*federation.PeerManager, error) {
	if !cb.cfg.Federation.Enabled {
		return nil, nil
	}

	log.Info("initializing peer-to-peer service discovery")

	// Determine API address for peer announcement
	apiAddr := cb.cfg.Server.BaseURL
	if apiAddr == "" {
		// Auto-detect local IP
		apiAddr = fmt.Sprintf("http://%s", cb.cfg.Server.Addr)
	}

	// Parse gossip port from config (default: 7946)
	gossipPort := cb.cfg.Federation.GossipPort
	if gossipPort == 0 {
		gossipPort = 7946
	}

	// Parse sync intervals
	fullSyncInterval, err := time.ParseDuration(cb.cfg.Federation.Sync.FullSyncInterval)
	if err != nil {
		fullSyncInterval = 5 * time.Minute
	}
	antiEntropyPeriod, err := time.ParseDuration(cb.cfg.Federation.Sync.AntiEntropyPeriod)
	if err != nil {
		antiEntropyPeriod = 1 * time.Minute
	}

	// Create federation config
	fedConfig := federation.Config{
		LocalPeerID:         nexusID,
		LocalPeerName:       "Nekzus Instance",
		APIAddress:          apiAddr,
		GossipBindAddr:      "0.0.0.0",
		GossipBindPort:      gossipPort,
		GossipAdvertiseAddr: "",
		GossipAdvertisePort: gossipPort,
		ClusterSecret:       cb.cfg.Federation.ClusterSecret,
		MDNSEnabled:         cb.cfg.Federation.MDNSEnabled,
		BootstrapPeers:      cb.cfg.Federation.BootstrapPeers,
		FullSyncInterval:    fullSyncInterval,
		AntiEntropyPeriod:   antiEntropyPeriod,
		PeerTimeout:         30 * time.Second,
		AllowRemoteRoutes:   cb.cfg.Federation.AllowRemoteRoutes,
	}

	// Create peer manager
	peerManager, err := federation.NewPeerManager(fedConfig, cb.store, wsManager, cb.metrics)
	if err != nil {
		log.Warn("failed to create peer manager", "error", err)
		log.Info("federation disabled due to initialization error")
		return nil, err
	}

	log.Info("peer manager created", "peer_id", nexusID, "gossip_port", gossipPort)
	return peerManager, nil
}

// WireFederationCallbacks sets up federation catalog sync callbacks
func WireFederationCallbacks(peerManager *federation.PeerManager, router *router.Registry) {
	if peerManager == nil {
		return
	}

	catalogSyncer := peerManager.GetCatalogSyncer()
	router.SetFederationCallbacks(
		func(appPtr *types.App) {
			// Called when app is added/updated
			if err := catalogSyncer.OnLocalServiceAdded(appPtr); err != nil {
				log.Warn("failed to sync app add", "error", err)
			}
		},
		func(appID string) {
			// Called when app is removed
			if err := catalogSyncer.OnLocalServiceDeleted(appID); err != nil {
				log.Warn("failed to sync app removal", "error", err)
			}
		},
	)
	log.Info("catalog sync callbacks wired")
}

// WireProxyCacheCallbacks sets up proxy cache eviction callbacks
func WireProxyCacheCallbacks(proxyCache *proxy.Cache, router *router.Registry) {
	if proxyCache == nil {
		return
	}

	router.SetProxyCacheCallbacks(
		func(route types.Route) {
			// Called when route is removed - evict from proxy cache
			if target, err := url.Parse(route.To); err == nil {
				proxyCache.Delete(target)
				log.Info("evicted proxy for removed route", "route_id", route.RouteID, "target", route.To)
			}
		},
		func(oldRoute, newRoute types.Route) {
			// Called when route target changes - evict old proxy
			if oldTarget, err := url.Parse(oldRoute.To); err == nil {
				proxyCache.Delete(oldTarget)
				log.Info("evicted proxy for updated route", "route_id", oldRoute.RouteID, "old_target", oldRoute.To, "new_target", newRoute.To)
			}
		},
	)
	log.Info("proxy cache eviction callbacks wired")
}
