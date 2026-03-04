package discovery

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	apptypes "github.com/nstalgic/nekzus/internal/types"
)

var dockerLog = slog.With("package", "discovery", "component", "docker")

// Docker Discovery Worker

// Confidence score constants for consistent scoring across workers
const (
	ConfidenceExplicit = 0.95 // Explicitly enabled via label
	ConfidenceMetadata = 0.85 // Has identifying metadata
	ConfidenceInferred = 0.70 // Inferred from patterns
	ConfidenceGeneric  = 0.50 // Generic service
)

// DockerDiscoveryWorker discovers services from Docker containers.
type DockerDiscoveryWorker struct {
	dm                *DiscoveryManager
	cli               *client.Client
	pollInterval      time.Duration
	debouncer         *debouncer
	knownServices     map[string]bool
	ctx               context.Context
	cancel            context.CancelFunc
	includeNetworks   []string // Specific networks to scan
	excludeNetworks   []string // Networks to ignore
	networkMode       string   // "all", "first", "preferred"
	selfContainerID   string   // Current container ID (if running in Docker)
	selfContainerName string   // Current container name (if running in Docker)
	selfHostname      string   // Current hostname
}

// NewDockerDiscoveryWorker creates a Docker discovery worker.
func NewDockerDiscoveryWorker(dm *DiscoveryManager, socketPath string, pollInterval time.Duration) (*DockerDiscoveryWorker, error) {
	ctx, cancel := context.WithCancel(context.Background())

	// Create Docker client with custom socket path if provided
	opts := []client.Opt{
		client.WithAPIVersionNegotiation(),
	}

	// Set socket path explicitly if provided, otherwise use default
	if socketPath != "" {
		opts = append(opts, client.WithHost(socketPath))
	} else {
		// Fall back to environment-based configuration
		opts = append(opts, client.FromEnv)
	}

	cli, err := client.NewClientWithOpts(opts...)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create Docker client: %w", err)
	}

	return &DockerDiscoveryWorker{
		dm:              dm,
		cli:             cli,
		pollInterval:    pollInterval,
		debouncer:       newDebouncer(30 * time.Second),
		knownServices:   make(map[string]bool),
		ctx:             ctx,
		cancel:          cancel,
		includeNetworks: nil,   // Will be set from config
		excludeNetworks: nil,   // Will be set from config
		networkMode:     "all", // Default mode
	}, nil
}

// SetNetworkConfig configures network filtering for the worker.
func (d *DockerDiscoveryWorker) SetNetworkConfig(includeNetworks, excludeNetworks []string, networkMode string) {
	d.includeNetworks = includeNetworks
	d.excludeNetworks = excludeNetworks
	if networkMode != "" {
		d.networkMode = networkMode
	}
}

// SetSelfIdentity configures self-identification to prevent discovering itself.
func (d *DockerDiscoveryWorker) SetSelfIdentity(ident *SelfIdentity) {
	if ident != nil {
		d.selfContainerID = ident.ContainerID
		d.selfContainerName = ident.ContainerName
		d.selfHostname = ident.Hostname
		dockerLog.Info("configured self-exclusion",
			"container_id", truncate(d.selfContainerID, 12),
			"container_name", d.selfContainerName,
			"hostname", d.selfHostname)
	}
}

// truncate returns a truncated version of the string for logging
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// Name returns the worker identifier.
func (d *DockerDiscoveryWorker) Name() string {
	return "docker"
}

// Start begins Docker container discovery.
func (d *DockerDiscoveryWorker) Start(ctx context.Context) error {
	ticker := time.NewTicker(d.pollInterval)
	defer ticker.Stop()

	// Initial scan
	if err := d.scan(); err != nil {
		dockerLog.Error("initial scan failed", "error", err)
		// Publish SSE warning about Docker connectivity issue
		d.dm.PublishWarning(
			"Docker",
			"Docker daemon unavailable",
			fmt.Sprintf("Failed to scan containers: %v. Ensure Docker is running and accessible.", err),
		)
		// Return error to signal that the worker should be removed
		dockerLog.Info("worker terminating due to connectivity failure")
		return fmt.Errorf("docker daemon unavailable: %w", err)
	}

	// Track consecutive failures for error recovery
	failureCount := 0
	maxFailures := 3

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-d.ctx.Done():
			return nil
		case <-ticker.C:
			if err := d.scan(); err != nil {
				failureCount++
				dockerLog.Error("scan failed", "failure_count", failureCount, "max_failures", maxFailures, "error", err)
				if failureCount >= maxFailures {
					dockerLog.Error("too many failures, stopping worker")
					return fmt.Errorf("scan failures exceeded threshold: %w", err)
				}
			} else {
				failureCount = 0
			}
		}
	}
}

// Stop gracefully shuts down the worker.
func (d *DockerDiscoveryWorker) Stop() error {
	d.cancel()
	if d.cli != nil {
		return d.cli.Close()
	}
	return nil
}

// scan queries the Docker API for running containers.
func (d *DockerDiscoveryWorker) scan() error {
	containers, err := d.listContainers()
	if err != nil {
		return fmt.Errorf("failed to list containers: %w", err)
	}

	dockerLog.Info("scanning containers", "total", len(containers))

	runningCount := 0
	for _, container := range containers {
		if container.State != "running" {
			continue
		}
		runningCount++

		d.processContainer(container)
	}

	dockerLog.Info("processed running containers", "count", runningCount)
	return nil
}

// listContainers retrieves running containers from Docker API.
func (d *DockerDiscoveryWorker) listContainers() ([]types.Container, error) {
	containers, err := d.cli.ContainerList(d.ctx, container.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list Docker containers: %w", err)
	}
	return containers, nil
}

// processContainer creates a proposal from a Docker container.
func (d *DockerDiscoveryWorker) processContainer(container types.Container) {
	containerName := "unknown"
	if len(container.Names) > 0 {
		containerName = container.Names[0]
	}

	// Skip self (prevent discovering our own container)
	if d.isSelfContainer(&container) {
		dockerLog.Info("skipping self-container", "name", containerName, "id", container.ID[:12])
		return
	}

	// Skip system containers
	if d.isSystemContainer(&container) {
		dockerLog.Info("skipping system container", "name", containerName)
		return
	}

	// Skip if explicitly disabled
	if d.getLabel(&container, "nekzus.enable", "") == "false" {
		dockerLog.Info("skipping disabled container", "name", containerName)
		return
	}

	// Extract service name from container name or labels
	serviceName := d.getServiceName(&container)
	if serviceName == "" {
		dockerLog.Info("skipping container with no service name", "name", containerName)
		return
	}

	// Validate labels before processing
	if err := d.validateLabels(&container); err != nil {
		dockerLog.Warn("skipping container due to invalid labels", "name", containerName, "error", err)
		return
	}

	dockerLog.Info("processing container", "name", containerName, "service", serviceName)

	// Check for nekzus labels for explicit configuration (optional)
	appID := d.getLabel(&container, "nekzus.app.id", sanitize(serviceName))
	appName := d.getLabel(&container, "nekzus.app.name", serviceName)
	pathBase := d.getLabel(&container, "nekzus.route.path", fmt.Sprintf("/apps/%s/", appID))

	// Get all container IPs across networks
	allIPs := d.getContainerIPs(&container)

	// Apply network filtering
	filteredIPs := d.filterNetworks(allIPs)

	if len(filteredIPs) == 0 {
		dockerLog.Info("no valid networks found for container", "name", containerName, "ips", allIPs)
		return
	}

	dockerLog.Info("container discovered on networks", "name", containerName, "network_count", len(filteredIPs))

	// Determine probe host - use container IP for probing (works from host network mode)
	// Container names only resolve within Docker networks, not from host network
	var probeHost string
	for _, ip := range filteredIPs {
		probeHost = ip
		break
	}
	if probeHost == "" {
		probeHost = d.getContainerName(&container)
	}

	// Get ports from both mapped ports and exposed ports (from Dockerfile EXPOSE)
	ports := container.Ports

	// Also get exposed ports from container inspection to catch unmapped EXPOSE ports
	exposedPorts, err := d.getExposedPorts(container.ID)
	if err != nil {
		dockerLog.Info("failed to inspect container for ports", "name", containerName, "error", err)
	} else if len(exposedPorts) > 0 {
		// Merge exposed ports with mapped ports, avoiding duplicates
		ports = d.mergePorts(ports, exposedPorts)
		dockerLog.Info("merged ports from mapping and inspection", "name", containerName, "mapped", len(container.Ports), "exposed", len(exposedPorts), "total", len(ports))
	}

	// Filter ports to only HTTP-capable ports (unless overridden by labels)
	// Uses HTTP probing to detect services on non-standard ports
	filteredPorts := d.filterPorts(ports, container.Labels, probeHost)
	if len(filteredPorts) == 0 {
		dockerLog.Info("no HTTP ports found for container", "name", containerName, "ports", len(ports), "probe_host", probeHost)
		return
	}

	// Build available ports list with schemes
	// Use PrivatePort (container's internal port) for Docker-to-Docker communication
	// PublicPort is only for external host access
	var availablePorts []apptypes.PortOption
	for _, port := range filteredPorts {
		targetPort := int(port.PrivatePort)
		scheme := d.getLabel(&container, "nekzus.scheme", d.guessScheme(targetPort))
		availablePorts = append(availablePorts, apptypes.PortOption{
			Port:   targetPort,
			Scheme: scheme,
		})
	}

	// Use first port as default
	primaryPort := availablePorts[0]

	// Use container IP for route target (works from host network mode)
	// Container names only resolve within Docker networks, not from host network
	var host string
	for _, ip := range filteredIPs {
		host = ip
		break
	}
	if host == "" {
		host = d.getContainerName(&container)
	}
	if host == "" {
		host = "127.0.0.1"
	}

	// Create unique proposal ID using container name (stable across restarts)
	cName := d.getContainerName(&container)
	if cName == "" {
		cName = container.ID[:12]
	}
	proposalID := generateProposalID("docker", primaryPort.Scheme, cName, primaryPort.Port)

	// Debounce to avoid spam
	if !d.debouncer.ShouldProcess(proposalID) {
		return
	}

	// Get icon with default fallback
	icon := d.getLabel(&container, "nekzus.app.icon", "")
	if icon == "" {
		icon = "📦" // Default Docker icon
	}

	// Get short container ID (12 chars, same as Docker CLI)
	shortContainerID := container.ID
	if len(shortContainerID) > 12 {
		shortContainerID = shortContainerID[:12]
	}

	// Build proposal with all available ports
	proposal := &apptypes.Proposal{
		ID:             proposalID,
		Source:         "docker",
		DetectedScheme: primaryPort.Scheme,
		DetectedHost:   host,
		DetectedPort:   primaryPort.Port,
		AvailablePorts: availablePorts,
		Confidence:     d.calculateConfidence(&container),
		SuggestedApp: apptypes.App{
			ID:   appID,
			Name: appName,
			Icon: icon,
			Tags: d.getTags(&container),
			Endpoints: map[string]string{
				"lan": fmt.Sprintf("%s://%s:%d", primaryPort.Scheme, host, primaryPort.Port),
			},
			DiscoveryMeta: &apptypes.DiscoveryMetadata{
				Source:      "docker",
				ContainerID: shortContainerID,
			},
		},
		SuggestedRoute: apptypes.Route{
			RouteID:         fmt.Sprintf("route:%s", appID),
			AppID:           appID,
			PathBase:        pathBase,
			To:              fmt.Sprintf("%s://%s:%d", primaryPort.Scheme, host, primaryPort.Port),
			ContainerID:     shortContainerID,
			Scopes:          d.getScopes(&container),
			StripPrefix:     d.getLabel(&container, "nekzus.route.strip_prefix", "true") == "true",
			RewriteHTML:     d.getLabel(&container, "nekzus.route.rewrite_html", "true") == "true",
			Websocket:       d.getLabel(&container, "nekzus.route.websocket", "false") == "true",
			SkipHealthCheck: d.getLabel(&container, "nekzus.health.skip", "false") == "true",
		},
		SecurityNotes: d.getSecurityNotes(primaryPort.Scheme, host),
	}

	if len(availablePorts) > 1 {
		dockerLog.Info("multiple ports available for container", "name", containerName, "ports", len(availablePorts), "default_port", primaryPort.Port)
	}

	d.dm.SubmitProposal(proposal)
}

// validateLabels validates container labels before processing.
// Add validation of label values
func (d *DockerDiscoveryWorker) validateLabels(c *types.Container) error {
	appID := d.getLabel(c, "nekzus.app.id", "")
	if appID != "" && !isValidAppID(appID) {
		return fmt.Errorf("invalid app.id: %s", appID)
	}

	pathBase := d.getLabel(c, "nekzus.route.path", "")
	if pathBase != "" && !strings.HasPrefix(pathBase, "/") {
		return fmt.Errorf("route.path must start with /: %s", pathBase)
	}

	return nil
}

// isValidAppID validates that an app ID contains only valid characters.
// Valid: alphanumeric, dashes, underscores; max 64 chars
func isValidAppID(id string) bool {
	if len(id) == 0 || len(id) > 64 {
		return false
	}

	for _, r := range id {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '-' && r != '_' {
			return false
		}
	}

	return true
}

// getServiceName extracts a friendly service name from the container.
func (d *DockerDiscoveryWorker) getServiceName(c *types.Container) string {
	// Check label first
	if name := c.Labels["nekzus.app.name"]; name != "" {
		return name
	}

	// Use container name (strip leading slash)
	if len(c.Names) > 0 {
		name := c.Names[0]
		return strings.TrimPrefix(name, "/")
	}

	// Fallback to image name
	parts := strings.Split(c.Image, ":")
	if len(parts) > 0 {
		imageParts := strings.Split(parts[0], "/")
		return imageParts[len(imageParts)-1]
	}

	return ""
}

// getContainerName retrieves the container's name for DNS resolution.
func (d *DockerDiscoveryWorker) getContainerName(c *types.Container) string {
	if len(c.Names) > 0 {
		// Docker names start with "/", strip it
		name := strings.TrimPrefix(c.Names[0], "/")
		return name
	}
	return ""
}

// getContainerIP retrieves the container's first IP address (legacy, for backward compatibility).
func (d *DockerDiscoveryWorker) getContainerIP(c *types.Container) string {
	for _, network := range c.NetworkSettings.Networks {
		if network.IPAddress != "" {
			return network.IPAddress
		}
	}
	return ""
}

// getContainerIPs retrieves all IP addresses for the container, mapped by network name.
func (d *DockerDiscoveryWorker) getContainerIPs(c *types.Container) map[string]string {
	ips := make(map[string]string)
	if c.NetworkSettings == nil {
		return ips
	}
	for networkName, network := range c.NetworkSettings.Networks {
		if network != nil && network.IPAddress != "" {
			ips[networkName] = network.IPAddress
		}
	}
	return ips
}

// getExposedPorts inspects a container to get ports from EXPOSE directive in Dockerfile.
// This is useful when containers don't have mapped ports but have exposed ports.
func (d *DockerDiscoveryWorker) getExposedPorts(containerID string) ([]types.Port, error) {
	ctx, cancel := context.WithTimeout(d.ctx, 5*time.Second)
	defer cancel()

	inspect, err := d.cli.ContainerInspect(ctx, containerID)
	if err != nil {
		return nil, err
	}

	var ports []types.Port
	// Config.ExposedPorts contains ports from EXPOSE directive
	for portSpec := range inspect.Config.ExposedPorts {
		port := portSpec.Int()
		proto := portSpec.Proto()
		if proto == "" {
			proto = "tcp"
		}
		ports = append(ports, types.Port{
			PrivatePort: uint16(port),
			Type:        proto,
		})
	}

	return ports, nil
}

// mergePorts combines mapped ports and exposed ports, avoiding duplicates.
// Mapped ports take precedence since they have more complete information (PublicPort).
func (d *DockerDiscoveryWorker) mergePorts(mapped, exposed []types.Port) []types.Port {
	// Use a map to track which private ports we've seen
	seen := make(map[uint16]bool)
	result := make([]types.Port, 0, len(mapped)+len(exposed))

	// Add all mapped ports first (they have PublicPort info)
	for _, p := range mapped {
		if p.Type == "tcp" {
			seen[p.PrivatePort] = true
			result = append(result, p)
		}
	}

	// Add exposed ports that aren't already in mapped
	for _, p := range exposed {
		if p.Type == "tcp" && !seen[p.PrivatePort] {
			seen[p.PrivatePort] = true
			result = append(result, p)
		}
	}

	return result
}

// filterNetworks applies network filtering rules based on worker configuration.
func (d *DockerDiscoveryWorker) filterNetworks(networks map[string]string) map[string]string {
	filtered := make(map[string]string)

	// If includeNetworks is specified, only use those networks
	if len(d.includeNetworks) > 0 {
		for _, includeName := range d.includeNetworks {
			if ip, ok := networks[includeName]; ok {
				// Check if it's also in exclude list
				excluded := false
				for _, excludeName := range d.excludeNetworks {
					if includeName == excludeName {
						excluded = true
						break
					}
				}
				if !excluded {
					filtered[includeName] = ip
				}
			}
		}
	} else {
		// No include list, so use all networks except excluded ones
		for networkName, ip := range networks {
			excluded := false
			for _, excludeName := range d.excludeNetworks {
				if networkName == excludeName {
					excluded = true
					break
				}
			}
			if !excluded {
				filtered[networkName] = ip
			}
		}
	}

	// Apply network mode
	switch d.networkMode {
	case "first":
		// Return only the first network
		for networkName, ip := range filtered {
			return map[string]string{networkName: ip}
		}
		return filtered
	case "preferred":
		// Return only the first network from includeNetworks list
		if len(d.includeNetworks) > 0 {
			for _, networkName := range d.includeNetworks {
				if ip, ok := filtered[networkName]; ok {
					return map[string]string{networkName: ip}
				}
			}
		}
		// If no preferred network found, return first available
		for networkName, ip := range filtered {
			return map[string]string{networkName: ip}
		}
		return filtered
	case "all":
		fallthrough
	default:
		return filtered
	}
}

// getLabel retrieves a label value or returns the default.
func (d *DockerDiscoveryWorker) getLabel(c *types.Container, key, defaultValue string) string {
	if val, ok := c.Labels[key]; ok && val != "" {
		return val
	}
	return defaultValue
}

// getTags extracts or generates tags from container labels and metadata.
func (d *DockerDiscoveryWorker) getTags(c *types.Container) []string {
	// Use explicit tags if provided
	if tags := c.Labels["nekzus.app.tags"]; tags != "" {
		return strings.Split(tags, ",")
	}

	// Auto-generate tags based on image and ports
	autoTags := []string{"docker"}

	// Add image-based tags
	imageName := strings.ToLower(c.Image)
	if strings.Contains(imageName, "postgres") || strings.Contains(imageName, "mysql") || strings.Contains(imageName, "mongo") {
		autoTags = append(autoTags, "database")
	} else if strings.Contains(imageName, "nginx") || strings.Contains(imageName, "apache") || strings.Contains(imageName, "httpd") {
		autoTags = append(autoTags, "web", "http")
	} else if strings.Contains(imageName, "redis") || strings.Contains(imageName, "memcached") {
		autoTags = append(autoTags, "cache")
	} else if strings.Contains(imageName, "grafana") || strings.Contains(imageName, "prometheus") {
		autoTags = append(autoTags, "monitoring")
	}

	// Add port-based tags
	for _, port := range c.Ports {
		switch port.PrivatePort {
		case 80, 8080, 8000, 3000:
			autoTags = append(autoTags, "http")
		case 443, 8443:
			autoTags = append(autoTags, "https")
		case 5432:
			autoTags = append(autoTags, "postgres")
		case 3306:
			autoTags = append(autoTags, "mysql")
		case 6379:
			autoTags = append(autoTags, "redis")
		case 27017:
			autoTags = append(autoTags, "mongodb")
		}
	}

	// Remove duplicates
	seen := make(map[string]bool)
	uniqueTags := []string{}
	for _, tag := range autoTags {
		if !seen[tag] {
			seen[tag] = true
			uniqueTags = append(uniqueTags, tag)
		}
	}

	return uniqueTags
}

// getScopes extracts required scopes from container labels.
func (d *DockerDiscoveryWorker) getScopes(c *types.Container) []string {
	if scopes := c.Labels["nekzus.route.scopes"]; scopes != "" {
		return strings.Split(scopes, ",")
	}
	// Default scope based on app ID
	appID := d.getLabel(c, "nekzus.app.id", "")
	if appID != "" {
		return []string{fmt.Sprintf("access:%s", appID)}
	}
	// Return empty array instead of nil to prevent mobile app errors
	return []string{}
}

// guessScheme attempts to determine HTTP vs HTTPS based on port.
func (d *DockerDiscoveryWorker) guessScheme(port int) string {
	switch port {
	case 443, 8443:
		return "https"
	default:
		return "http"
	}
}

// knownNonHTTPPorts contains ports that are definitely not HTTP services.
// These are skipped without probing to save time.
var knownNonHTTPPorts = map[int]bool{
	21:    true, // FTP
	22:    true, // SSH
	23:    true, // Telnet
	25:    true, // SMTP
	53:    true, // DNS
	110:   true, // POP3
	143:   true, // IMAP
	389:   true, // LDAP
	636:   true, // LDAPS
	1883:  true, // MQTT
	3306:  true, // MySQL
	5432:  true, // PostgreSQL
	5672:  true, // RabbitMQ AMQP
	6379:  true, // Redis
	11211: true, // Memcached
	27017: true, // MongoDB
}

// isKnownNonHTTPPort returns true if the port is definitely not HTTP.
func isKnownNonHTTPPort(port int) bool {
	return knownNonHTTPPorts[port]
}

// probeHTTP checks if a host:port responds to HTTP requests.
// Uses a lightweight HEAD request with short timeout.
func probeHTTP(host string, port int) bool {
	// Build URL - try container name first (Docker DNS)
	url := fmt.Sprintf("http://%s:%d/", host, port)

	// Create client with aggressive timeouts for quick probe
	client := &http.Client{
		Timeout: 2 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// Don't follow redirects - we just want to know if it speaks HTTP
			return http.ErrUseLastResponse
		},
	}

	// Use HEAD request - minimal data transfer
	req, err := http.NewRequest(http.MethodHead, url, nil)
	if err != nil {
		return false
	}

	// Set minimal headers
	req.Header.Set("User-Agent", "nekzus-probe/1.0")
	req.Header.Set("Connection", "close")

	resp, err := client.Do(req)
	if err != nil {
		// Connection refused, timeout, etc. - not HTTP or not ready
		dockerLog.Debug("HTTP probe failed", "host", host, "port", port, "error", err)
		return false
	}
	defer resp.Body.Close()

	// Any HTTP response (even 4xx/5xx) means it's an HTTP server
	dockerLog.Debug("HTTP probe succeeded", "host", host, "port", port, "status", resp.StatusCode)
	return true
}

// filterPorts filters container ports to only include HTTP-capable ports.
// Respects label overrides for explicit port selection.
// Uses HTTP probing to detect actual HTTP services on non-standard ports.
func (d *DockerDiscoveryWorker) filterPorts(ports []types.Port, labels map[string]string, probeHost string) []types.Port {
	var filtered []types.Port

	// Check for primary port label (only discover this specific port)
	// Note: primary_port refers to the container's internal port (PrivatePort), not the host mapping
	if primaryPortStr := labels["nekzus.primary_port"]; primaryPortStr != "" {
		var primaryPort int
		if _, err := fmt.Sscanf(primaryPortStr, "%d", &primaryPort); err == nil {
			for _, port := range ports {
				if port.Type != "tcp" {
					continue
				}
				// Compare against PrivatePort (container port), not PublicPort (host port)
				if int(port.PrivatePort) == primaryPort {
					filtered = append(filtered, port)
					return filtered
				}
			}
		}
		return filtered
	}

	// Check for all_ports label (discover all TCP ports)
	if labels["nekzus.discover.all_ports"] == "true" {
		for _, port := range ports {
			if port.Type == "tcp" {
				filtered = append(filtered, port)
			}
		}
		return filtered
	}

	// Default: filter to HTTP-capable ports using probe
	for _, port := range ports {
		if port.Type != "tcp" {
			continue
		}

		targetPort := int(port.PublicPort)
		if targetPort == 0 {
			targetPort = int(port.PrivatePort)
		}

		// Skip known non-HTTP ports without probing
		if isKnownNonHTTPPort(targetPort) {
			dockerLog.Debug("skipping known non-HTTP port",
				"port", targetPort,
				"reason", "known infrastructure port")
			continue
		}

		// Probe the port to check if it speaks HTTP
		if probeHost != "" && probeHTTP(probeHost, targetPort) {
			filtered = append(filtered, port)
		} else if probeHost == "" {
			// No host to probe - skip unknown ports
			dockerLog.Debug("skipping port - no host to probe",
				"port", targetPort)
		} else {
			dockerLog.Debug("skipping port - HTTP probe failed",
				"host", probeHost,
				"port", targetPort)
		}
	}

	return filtered
}

// calculateConfidence determines confidence score based on container metadata.
// Uses standardized confidence score constants
func (d *DockerDiscoveryWorker) calculateConfidence(c *types.Container) float64 {
	confidence := ConfidenceGeneric // Base confidence for any discovered service

	// Higher confidence if explicitly labeled with nekzus
	if c.Labels["nekzus.enable"] == "true" {
		confidence = ConfidenceExplicit
	} else if c.Labels["nekzus.app.id"] != "" {
		confidence = ConfidenceMetadata
	}

	// Boost confidence for well-known images
	imageName := strings.ToLower(c.Image)
	wellKnownImages := []string{
		"nginx", "grafana", "prometheus", "postgres", "mysql",
		"redis", "mongo", "elasticsearch", "kibana", "jenkins",
		"gitlab", "nextcloud", "wordpress", "ghost", "plex",
	}
	for _, known := range wellKnownImages {
		if strings.Contains(imageName, known) {
			confidence += 0.2
			break
		}
	}

	// Boost if has common web ports
	for _, port := range c.Ports {
		if port.PrivatePort == 80 || port.PrivatePort == 8080 || port.PrivatePort == 3000 {
			confidence += 0.1
			break
		}
	}

	// Boost if has Traefik/Caddy/other reverse proxy labels
	if c.Labels["traefik.enable"] == "true" || c.Labels["caddy"] != "" {
		confidence += 0.15
	}

	// Cap at ConfidenceMetadata for non-explicit services, 1.0 for all services
	if c.Labels["nekzus.enable"] != "true" && confidence > ConfidenceMetadata {
		confidence = ConfidenceMetadata
	}

	// Ensure confidence never exceeds 1.0
	if confidence > 1.0 {
		confidence = 1.0
	}

	return confidence
}

// isSelfContainer checks if this container is the current running instance.
// Only matches on container ID and container name, NOT hostname
// to avoid false positives when container names match the hostname.
func (d *DockerDiscoveryWorker) isSelfContainer(c *types.Container) bool {
	// Check by container ID (most reliable)
	if d.selfContainerID != "" && c.ID != "" {
		// Compare first 12 chars (short ID) for compatibility
		containerShortID := c.ID
		if len(containerShortID) > 12 {
			containerShortID = containerShortID[:12]
		}
		selfShortID := d.selfContainerID
		if len(selfShortID) > 12 {
			selfShortID = selfShortID[:12]
		}
		if containerShortID == selfShortID {
			return true
		}
		// Also check full ID match (prefix in both directions)
		if strings.HasPrefix(c.ID, d.selfContainerID) || strings.HasPrefix(d.selfContainerID, c.ID) {
			return true
		}
	}

	// Check by container name (exact match only)
	if d.selfContainerName != "" {
		for _, name := range c.Names {
			cleanName := strings.TrimPrefix(name, "/")
			if cleanName == d.selfContainerName {
				return true
			}
		}
	}

	// REMOVED hostname matching to avoid false positives
	// Previously matched containers with names equal to selfHostname,
	// which caused false positives when container names matched the hostname.

	return false
}

// isSystemContainer checks if this is a system/infrastructure container that should be skipped.
func (d *DockerDiscoveryWorker) isSystemContainer(c *types.Container) bool {
	// Explicitly allow test containers (nekzus.test label)
	if c.Labels["nekzus.test"] != "" {
		return false // Don't skip test containers
	}

	// Skip if explicitly marked to skip
	if c.Labels["nekzus.skip"] == "true" {
		return true
	}

	// Regex patterns to match nekzus containers (any variant)
	// This will match: nekzus, nekzus-test, nekzus-dev, nekzus-1, etc.
	nexusPattern := regexp.MustCompile(`(?i)nekzus`)
	caddyPattern := regexp.MustCompile(`(?i)nekzus[-_]?caddy`)

	// Check container names
	for _, name := range c.Names {
		// Strip leading slash that Docker adds
		cleanName := strings.TrimPrefix(name, "/")
		if nexusPattern.MatchString(cleanName) || caddyPattern.MatchString(cleanName) {
			dockerLog.Debug("skipping Nexus/Caddy container - matched name pattern", "name", cleanName)
			return true
		}
	}

	// Check image name
	imageName := strings.ToLower(c.Image)
	if nexusPattern.MatchString(imageName) || caddyPattern.MatchString(imageName) {
		dockerLog.Debug("skipping Nexus/Caddy container - matched image pattern", "image", imageName)
		return true
	}

	// Skip common infrastructure containers
	systemImagePrefixes := []string{
		"k8s.gcr.io/",
		"gcr.io/k8s",
		"rancher/",
		"portainer/",
		"traefik:",
		"caddy:",
	}

	for _, prefix := range systemImagePrefixes {
		if strings.Contains(imageName, prefix) {
			return true
		}
	}

	return false
}

// getSecurityNotes generates security warnings for the proposal.
func (d *DockerDiscoveryWorker) getSecurityNotes(scheme, host string) []string {
	notes := []string{"Discovered via Docker API", "JWT required"}

	if scheme == "http" {
		notes = append(notes, "Upstream uses HTTP (unencrypted)")
	}

	if strings.HasPrefix(host, "127.") || strings.HasPrefix(host, "172.") || strings.HasPrefix(host, "192.168.") {
		notes = append(notes, "Private network address")
	}

	return notes
}

// Example Docker labels for services:
//
// nekzus.enable=true
// nekzus.app.id=grafana
// nekzus.app.name=Grafana
// nekzus.app.icon=https://grafana.com/icon.png
// nekzus.app.tags=monitoring,metrics
// nekzus.route.path=/apps/grafana/
// nekzus.route.scopes=access:grafana,read:metrics
// nekzus.scheme=http
