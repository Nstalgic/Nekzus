package discovery

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	apptypes "github.com/nstalgic/nekzus/internal/types"
	v1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

var k8sLog = slog.With("package", "discovery", "component", "kubernetes")

// Kubernetes Discovery Worker

// KubernetesDiscoveryWorker discovers services from Kubernetes clusters.
type KubernetesDiscoveryWorker struct {
	dm               *DiscoveryManager
	client           kubernetes.Interface
	pollInterval     time.Duration
	debouncer        *debouncer
	namespaces       []string
	namespaceCache   map[string]*v1.Namespace // Cache of namespaces for label inheritance
	namespaceCacheMu sync.RWMutex             // Protects namespaceCache from concurrent access
	ctx              context.Context
	cancel           context.CancelFunc
}

// NewKubernetesDiscoveryWorker creates a Kubernetes discovery worker.
// If namespaces is empty, it will watch all namespaces.
func NewKubernetesDiscoveryWorker(dm *DiscoveryManager, client kubernetes.Interface, pollInterval time.Duration, namespaces ...string) (*KubernetesDiscoveryWorker, error) {
	ctx, cancel := context.WithCancel(context.Background())

	if len(namespaces) == 0 {
		namespaces = []string{metav1.NamespaceAll}
	}

	return &KubernetesDiscoveryWorker{
		dm:             dm,
		client:         client,
		pollInterval:   pollInterval,
		debouncer:      newDebouncer(30 * time.Second),
		namespaces:     namespaces,
		namespaceCache: make(map[string]*v1.Namespace),
		ctx:            ctx,
		cancel:         cancel,
	}, nil
}

// NewKubernetesDiscoveryWorkerFromConfig creates a worker from kubeconfig.
func NewKubernetesDiscoveryWorkerFromConfig(dm *DiscoveryManager, kubeconfigPath string, pollInterval time.Duration, namespaces ...string) (*KubernetesDiscoveryWorker, error) {
	var config *rest.Config
	var err error

	if kubeconfigPath == "" {
		// Try in-cluster config first
		config, err = rest.InClusterConfig()
		if err != nil {
			// Fall back to default kubeconfig location
			config, err = clientcmd.BuildConfigFromFlags("", clientcmd.RecommendedHomeFile)
			if err != nil {
				return nil, fmt.Errorf("failed to create Kubernetes config: %w", err)
			}
		}
	} else {
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfigPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load kubeconfig from %s: %w", kubeconfigPath, err)
		}
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	return NewKubernetesDiscoveryWorker(dm, clientset, pollInterval, namespaces...)
}

// Name returns the worker identifier.
func (k *KubernetesDiscoveryWorker) Name() string {
	return "kubernetes"
}

// Start begins Kubernetes service discovery.
func (k *KubernetesDiscoveryWorker) Start(ctx context.Context) error {
	ticker := time.NewTicker(k.pollInterval)
	defer ticker.Stop()

	// Initial scan
	if err := k.scan(); err != nil {
		k8sLog.Error("initial scan failed", "error", err)
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-k.ctx.Done():
			return nil
		case <-ticker.C:
			if err := k.scan(); err != nil {
				k8sLog.Error("scan failed", "error", err)
			}
		}
	}
}

// Stop gracefully shuts down the worker.
func (k *KubernetesDiscoveryWorker) Stop() error {
	k.cancel()
	return nil
}

// scan queries the Kubernetes API for services and ingresses.
func (k *KubernetesDiscoveryWorker) scan() error {
	for _, namespace := range k.namespaces {
		// Fetch and cache namespace object for label inheritance
		if namespace != metav1.NamespaceAll {
			ns, err := k.client.CoreV1().Namespaces().Get(k.ctx, namespace, metav1.GetOptions{})
			if err != nil {
				k8sLog.Error("failed to get namespace", "namespace", namespace, "error", err)
			} else {
				k.namespaceCacheMu.Lock()
				k.namespaceCache[namespace] = ns
				k.namespaceCacheMu.Unlock()
			}
		}

		// Scan services
		services, err := k.client.CoreV1().Services(namespace).List(k.ctx, metav1.ListOptions{})
		if err != nil {
			k8sLog.Error("failed to list services", "namespace", namespace, "error", err)
			continue
		}

		k8sLog.Info("scanning services", "count", len(services.Items), "namespace", namespace)

		for _, service := range services.Items {
			k.processService(&service)
		}

		// Scan ingresses
		ingresses, err := k.client.NetworkingV1().Ingresses(namespace).List(k.ctx, metav1.ListOptions{})
		if err != nil {
			k8sLog.Error("failed to list ingresses", "namespace", namespace, "error", err)
			continue
		}

		k8sLog.Info("scanning ingresses", "count", len(ingresses.Items), "namespace", namespace)

		for _, ingress := range ingresses.Items {
			k.processIngress(&ingress, services.Items)
		}
	}

	return nil
}

// processService creates a proposal from a Kubernetes service.
func (k *KubernetesDiscoveryWorker) processService(service *v1.Service) {
	// Skip if not explicitly enabled
	if !k.isEnabled(service) {
		return
	}

	// Skip headless services (ClusterIP = None)
	if service.Spec.ClusterIP == "None" || service.Spec.ClusterIP == "" {
		k8sLog.Debug("skipping headless service", "namespace", service.Namespace, "name", service.Name)
		return
	}

	// Extract service metadata
	serviceName := k.getServiceName(service)
	appID := k.getLabel(service, "nekzus.app.id", sanitize(serviceName))
	appName := k.getLabel(service, "nekzus.app.name", serviceName)
	pathBase := k.getLabel(service, "nekzus.route.path", fmt.Sprintf("/apps/%s/", appID))

	k8sLog.Debug("processing service", "namespace", service.Namespace, "name", service.Name)

	// Create proposals for each port
	for _, port := range service.Spec.Ports {
		if port.Protocol != v1.ProtocolTCP {
			continue
		}

		targetPort := int(port.Port)
		scheme := k.getLabel(service, "nekzus.scheme", k.guessScheme(targetPort, port.Name))

		// Build Kubernetes DNS name
		host := k.buildServiceDNS(service)

		// Create unique proposal ID
		proposalID := generateProposalID("kubernetes", scheme, service.Namespace+"/"+service.Name, targetPort)

		// Debounce to avoid spam
		if !k.debouncer.ShouldProcess(proposalID) {
			continue
		}

		// Build proposal
		proposal := &apptypes.Proposal{
			ID:             proposalID,
			Source:         "kubernetes",
			DetectedScheme: scheme,
			DetectedHost:   host,
			DetectedPort:   targetPort,
			Confidence:     k.calculateConfidence(service),
			SuggestedApp: apptypes.App{
				ID:   appID,
				Name: appName,
				Icon: k.getLabel(service, "nekzus.app.icon", ""),
				Tags: k.getTags(service),
				Endpoints: map[string]string{
					"cluster": fmt.Sprintf("%s://%s:%d", scheme, host, targetPort),
				},
			},
			SuggestedRoute: apptypes.Route{
				RouteID:  fmt.Sprintf("route:%s", appID),
				AppID:    appID,
				PathBase: pathBase,
				To:       fmt.Sprintf("%s://%s:%d", scheme, host, targetPort),
				Scopes:   k.getScopes(service),
			},
			SecurityNotes: k.getSecurityNotes(scheme, service),
		}

		k.dm.SubmitProposal(proposal)
	}
}

// processIngress creates proposals from Ingress objects and their backend services.
func (k *KubernetesDiscoveryWorker) processIngress(ingress *networkingv1.Ingress, services []v1.Service) {
	// Check if Ingress is enabled for discovery
	if !k.isIngressEnabled(ingress) {
		return
	}

	k8sLog.Debug("processing ingress", "namespace", ingress.Namespace, "name", ingress.Name)

	// Detect if TLS is configured
	hasTLS := len(ingress.Spec.TLS) > 0

	// Process each rule and path
	for _, rule := range ingress.Spec.Rules {
		if rule.HTTP == nil {
			continue
		}

		for _, path := range rule.HTTP.Paths {
			// Find the backend service
			if path.Backend.Service == nil {
				continue
			}

			serviceName := path.Backend.Service.Name
			servicePort := int(path.Backend.Service.Port.Number)

			// Find the service in the list
			service := k.findService(services, serviceName, ingress.Namespace)
			if service == nil {
				k8sLog.Debug("ingress backend service not found", "service", serviceName)
				continue
			}

			// Extract metadata from Ingress or Service
			appID := k.getIngressAppID(ingress, service, serviceName)
			appName := k.getIngressAppName(ingress, service, serviceName)

			// Use Ingress path as the route path
			pathBase := path.Path
			if pathBase == "" || pathBase == "/" {
				pathBase = fmt.Sprintf("/apps/%s/", appID)
			}

			// Determine scheme from TLS config
			scheme := "http"
			if hasTLS {
				scheme = "https"
			}

			// Override with explicit label if provided
			if explicitScheme := k.getLabel(service, "nekzus.scheme", ""); explicitScheme != "" {
				scheme = explicitScheme
			}

			// Build host (use Ingress host if specified, otherwise service DNS)
			host := k.buildServiceDNS(service)
			if rule.Host != "" {
				host = rule.Host
			}

			// Create unique proposal ID
			proposalID := generateProposalID("kubernetes-ingress", scheme, fmt.Sprintf("%s/%s%s", ingress.Namespace, ingress.Name, pathBase), servicePort)

			// Debounce
			if !k.debouncer.ShouldProcess(proposalID) {
				continue
			}

			// Build proposal
			proposal := &apptypes.Proposal{
				ID:             proposalID,
				Source:         "kubernetes",
				DetectedScheme: scheme,
				DetectedHost:   host,
				DetectedPort:   servicePort,
				Confidence:     k.calculateIngressConfidence(ingress, service),
				SuggestedApp: apptypes.App{
					ID:   appID,
					Name: appName,
					Icon: k.getIngressLabel(ingress, service, "nekzus.app.icon", ""),
					Tags: k.getIngressTags(ingress, service),
					Endpoints: map[string]string{
						"cluster": fmt.Sprintf("%s://%s:%d", scheme, k.buildServiceDNS(service), servicePort),
					},
				},
				SuggestedRoute: apptypes.Route{
					RouteID:  fmt.Sprintf("route:%s", appID),
					AppID:    appID,
					PathBase: pathBase,
					To:       fmt.Sprintf("%s://%s:%d", scheme, k.buildServiceDNS(service), servicePort),
					Scopes:   k.getIngressScopes(ingress, service),
				},
				SecurityNotes: k.getIngressSecurityNotes(scheme, ingress, service),
			}

			k.dm.SubmitProposal(proposal)
		}
	}
}

// isIngressEnabled checks if an Ingress should be used for discovery.
func (k *KubernetesDiscoveryWorker) isIngressEnabled(ingress *networkingv1.Ingress) bool {
	// Explicitly enabled
	if ingress.Labels["nekzus.enable"] == "true" {
		return true
	}

	// Explicitly disabled
	if ingress.Labels["nekzus.enable"] == "false" {
		return false
	}

	// Skip system namespaces
	systemNamespaces := []string{"kube-system", "kube-public", "kube-node-lease"}
	for _, ns := range systemNamespaces {
		if ingress.Namespace == ns {
			return false
		}
	}

	// Auto-enable for ingresses with standard labels
	if _, hasAppName := ingress.Labels["app.kubernetes.io/name"]; hasAppName {
		return true
	}

	return false
}

// findService locates a service by name in the service list.
// Returns a copy of the service to avoid returning a pointer to a slice element.
func (k *KubernetesDiscoveryWorker) findService(services []v1.Service, name, namespace string) *v1.Service {
	for i := range services {
		if services[i].Name == name && services[i].Namespace == namespace {
			svc := services[i] // Copy to avoid pointer to slice element
			return &svc
		}
	}
	return nil
}

// getIngressAppID extracts app ID from Ingress or Service labels.
func (k *KubernetesDiscoveryWorker) getIngressAppID(ingress *networkingv1.Ingress, service *v1.Service, defaultValue string) string {
	// Check Ingress labels first
	if val := ingress.Labels["nekzus.app.id"]; val != "" {
		return val
	}
	if val := ingress.Annotations["nekzus.app.id"]; val != "" {
		return val
	}

	// Fall back to service labels
	return k.getLabel(service, "nekzus.app.id", sanitize(defaultValue))
}

// getIngressAppName extracts app name from Ingress or Service.
func (k *KubernetesDiscoveryWorker) getIngressAppName(ingress *networkingv1.Ingress, service *v1.Service, defaultValue string) string {
	// Check Ingress labels/annotations
	if val := ingress.Labels["nekzus.app.name"]; val != "" {
		return val
	}
	if val := ingress.Annotations["nekzus.app.name"]; val != "" {
		return val
	}
	if val := ingress.Labels["app.kubernetes.io/name"]; val != "" {
		return val
	}

	// Fall back to service
	return k.getServiceName(service)
}

// getIngressLabel retrieves a label from Ingress or Service.
func (k *KubernetesDiscoveryWorker) getIngressLabel(ingress *networkingv1.Ingress, service *v1.Service, key, defaultValue string) string {
	// Check Ingress first
	if val := ingress.Labels[key]; val != "" {
		return val
	}
	if val := ingress.Annotations[key]; val != "" {
		return val
	}

	// Fall back to service
	return k.getLabel(service, key, defaultValue)
}

// getIngressTags generates tags from Ingress and Service.
func (k *KubernetesDiscoveryWorker) getIngressTags(ingress *networkingv1.Ingress, service *v1.Service) []string {
	tags := []string{"kubernetes", "ingress"}

	// Add namespace if not default
	if ingress.Namespace != "default" {
		tags = append(tags, ingress.Namespace)
	}

	// Add TLS tag if configured
	if len(ingress.Spec.TLS) > 0 {
		tags = append(tags, "tls")
	}

	// Merge with service tags
	serviceTags := k.getTags(service)
	for _, tag := range serviceTags {
		if tag != "kubernetes" && !contains(tags, tag) {
			tags = append(tags, tag)
		}
	}

	return tags
}

// getIngressScopes extracts scopes from Ingress or Service.
func (k *KubernetesDiscoveryWorker) getIngressScopes(ingress *networkingv1.Ingress, service *v1.Service) []string {
	// Check Ingress annotations
	if scopes := ingress.Annotations["nekzus.route.scopes"]; scopes != "" {
		return strings.Split(scopes, ",")
	}

	// Fall back to service
	return k.getScopes(service)
}

// calculateIngressConfidence determines confidence for Ingress-discovered services.
// Uses standardized confidence score constants
func (k *KubernetesDiscoveryWorker) calculateIngressConfidence(ingress *networkingv1.Ingress, service *v1.Service) float64 {
	// Higher base confidence for Ingress-exposed services (between metadata and explicit)
	confidence := 0.8

	// Explicit labeling boosts confidence
	if ingress.Labels["nekzus.enable"] == "true" {
		confidence = ConfidenceExplicit
	}

	// TLS configuration boosts confidence
	if len(ingress.Spec.TLS) > 0 {
		confidence += 0.05
	}

	// Cap at 1.0
	if confidence > 1.0 {
		confidence = 1.0
	}

	return confidence
}

// getIngressSecurityNotes generates security notes for Ingress proposals.
func (k *KubernetesDiscoveryWorker) getIngressSecurityNotes(scheme string, ingress *networkingv1.Ingress, service *v1.Service) []string {
	notes := []string{
		"Discovered via Ingress",
		"JWT required",
		fmt.Sprintf("Namespace: %s", ingress.Namespace),
	}

	if scheme == "https" {
		notes = append(notes, "TLS termination at Ingress")
	} else {
		notes = append(notes, "No TLS configured (HTTP only)")
	}

	// Note if backend service uses different protocol
	if len(service.Spec.Ports) > 0 {
		serviceScheme := k.guessScheme(int(service.Spec.Ports[0].Port), service.Spec.Ports[0].Name)
		if serviceScheme != scheme {
			notes = append(notes, fmt.Sprintf("Backend service uses %s", serviceScheme))
		}
	}

	return notes
}

// getNamespaceFromCache safely retrieves a namespace from the cache.
func (k *KubernetesDiscoveryWorker) getNamespaceFromCache(namespace string) *v1.Namespace {
	k.namespaceCacheMu.RLock()
	defer k.namespaceCacheMu.RUnlock()
	return k.namespaceCache[namespace]
}

// isEnabled checks if the service should be discovered.
func (k *KubernetesDiscoveryWorker) isEnabled(service *v1.Service) bool {
	// Check namespace-level enable/disable first
	if ns := k.getNamespaceFromCache(service.Namespace); ns != nil {
		if ns.Labels["nekzus.enable"] == "false" {
			// Namespace-level disable takes precedence unless service explicitly enables
			if service.Labels["nekzus.enable"] != "true" {
				return false
			}
		}
		if ns.Labels["nekzus.enable"] == "true" {
			// Namespace-level enable, unless service explicitly disables
			if service.Labels["nekzus.enable"] == "false" {
				return false
			}
			// Service inherits namespace enable
			return true
		}
	}

	// Service-level explicit enable
	if service.Labels["nekzus.enable"] == "true" {
		return true
	}

	// Service-level explicit disable
	if service.Labels["nekzus.enable"] == "false" {
		return false
	}

	// Skip system namespaces by default
	systemNamespaces := []string{"kube-system", "kube-public", "kube-node-lease"}
	for _, ns := range systemNamespaces {
		if service.Namespace == ns {
			return false
		}
	}

	// Smart label inference: Auto-discover based on standard Kubernetes labels

	// 1. Istio/Service Mesh detection (high confidence for service mesh)
	if k.hasIstioSidecar(service) {
		return true
	}

	// 2. LoadBalancer and NodePort services are usually user-facing
	if service.Spec.Type == v1.ServiceTypeLoadBalancer || service.Spec.Type == v1.ServiceTypeNodePort {
		// Must have some identifying label
		if k.hasIdentifyingLabel(service) {
			return true
		}
	}

	// 3. Services with frontend/ui component labels
	component := service.Labels["app.kubernetes.io/component"]
	if component == "frontend" || component == "ui" || component == "web" {
		return true
	}

	// 4. Services with app.kubernetes.io/name (standard Kubernetes label)
	if _, hasAppName := service.Labels["app.kubernetes.io/name"]; hasAppName {
		// Also check if it has a component suggesting it's user-facing
		if component == "frontend" || component == "ui" || component == "web" || component == "dashboard" {
			return true
		}
		// Also check if it's a known Helm chart (must be user-facing, not database)
		if k.isUserFacingHelmChart(service) {
			return true
		}
	}

	// If no explicit label and doesn't match smart inference, don't discover
	return false
}

// hasIstioSidecar checks if the service has Istio sidecar injection enabled.
func (k *KubernetesDiscoveryWorker) hasIstioSidecar(service *v1.Service) bool {
	// Check namespace-level Istio injection
	if ns := k.getNamespaceFromCache(service.Namespace); ns != nil {
		if ns.Labels["istio-injection"] == "enabled" {
			return true
		}
	}

	// Check service-level Istio labels
	if service.Labels["istio-injection"] == "enabled" {
		return true
	}
	if service.Labels["istio.io/rev"] != "" {
		return true
	}
	if service.Labels["service.istio.io/canonical-name"] != "" {
		return true
	}

	// Check annotations
	if service.Annotations["sidecar.istio.io/inject"] == "true" {
		return true
	}

	return false
}

// isUserFacingHelmChart determines if a Helm chart is user-facing (vs backend like databases).
func (k *KubernetesDiscoveryWorker) isUserFacingHelmChart(service *v1.Service) bool {
	chartName := service.Labels["helm.sh/chart"]
	if chartName == "" {
		return false
	}

	// Extract chart base name (remove version)
	chartBase := strings.Split(chartName, "-")[0]

	// List of backend charts that should NOT be auto-discovered
	backendCharts := []string{
		"postgresql", "postgres", "mysql", "mariadb", "mongodb", "mongo",
		"redis", "memcached", "elasticsearch", "cassandra", "kafka",
		"rabbitmq", "nats", "etcd", "consul", "vault",
	}

	for _, backend := range backendCharts {
		if chartBase == backend {
			return false
		}
	}

	// If it's a Helm chart and not a backend service, likely user-facing
	return true
}

// hasIdentifyingLabel checks if service has labels that identify it
func (k *KubernetesDiscoveryWorker) hasIdentifyingLabel(service *v1.Service) bool {
	// Standard Kubernetes labels
	if _, ok := service.Labels["app.kubernetes.io/name"]; ok {
		return true
	}
	if _, ok := service.Labels["app"]; ok {
		return true
	}
	if _, ok := service.Labels["app.kubernetes.io/instance"]; ok {
		return true
	}
	return false
}

// getServiceName extracts a friendly service name.
func (k *KubernetesDiscoveryWorker) getServiceName(service *v1.Service) string {
	// Priority order:
	// 1. Explicit nekzus labels (highest priority)
	if name := service.Labels["nekzus.app.name"]; name != "" {
		return name
	}
	if name := service.Annotations["nekzus.app.name"]; name != "" {
		return name
	}

	// 2. Standard Kubernetes labels
	if name := service.Labels["app.kubernetes.io/name"]; name != "" {
		return name
	}

	// 3. Common app label
	if name := service.Labels["app"]; name != "" {
		return name
	}

	// 4. Check if it's a well-known Helm chart - use friendly name as fallback
	if chartName := service.Labels["helm.sh/chart"]; chartName != "" {
		if friendlyName := k.getHelmChartFriendlyName(chartName); friendlyName != "" {
			return friendlyName
		}
	}

	// 5. Fall back to service name
	return service.Name
}

// getHelmChartFriendlyName returns a user-friendly name for well-known Helm charts.
func (k *KubernetesDiscoveryWorker) getHelmChartFriendlyName(chartName string) string {
	chartBase := strings.Split(chartName, "-")[0]

	friendlyNames := map[string]string{
		"grafana":     "Grafana",
		"prometheus":  "Prometheus",
		"argocd":      "ArgoCD",
		"argo":        "ArgoCD",
		"loki":        "Loki",
		"tempo":       "Tempo",
		"jaeger":      "Jaeger",
		"nginx":       "NGINX Ingress Controller",
		"ingress":     "NGINX Ingress Controller",
		"cert":        "Cert Manager",
		"external":    "External DNS",
		"harbor":      "Harbor",
		"vault":       "Vault",
		"consul":      "Consul",
		"linkerd":     "Linkerd",
		"jenkins":     "Jenkins",
		"gitlab":      "GitLab",
		"sonarqube":   "SonarQube",
		"nexus":       "Nexus Repository",
		"artifactory": "Artifactory",
	}

	if friendlyName, ok := friendlyNames[chartBase]; ok {
		return friendlyName
	}

	return ""
}

// buildServiceDNS creates the full Kubernetes DNS name for the service.
func (k *KubernetesDiscoveryWorker) buildServiceDNS(service *v1.Service) string {
	// Format: <service-name>.<namespace>.svc.cluster.local
	return fmt.Sprintf("%s.%s.svc.cluster.local", service.Name, service.Namespace)
}

// getLabel retrieves a label or annotation value or returns the default.
// Checks labels first, then annotations (since annotations allow more complex values).
// Also supports smart inference from standard Kubernetes labels and namespace inheritance.
func (k *KubernetesDiscoveryWorker) getLabel(service *v1.Service, key, defaultValue string) string {
	// Check explicit nekzus labels first (service level)
	if val, ok := service.Labels[key]; ok && val != "" {
		return val
	}
	// Check annotations for values that don't fit label constraints
	if val, ok := service.Annotations[key]; ok && val != "" {
		return val
	}

	// Check namespace-level labels (inheritance)
	if ns := k.getNamespaceFromCache(service.Namespace); ns != nil {
		if val, ok := ns.Labels[key]; ok && val != "" {
			return val
		}
	}

	// Smart inference for app.id from standard labels
	if key == "nekzus.app.id" {
		// Try app.kubernetes.io/name
		if val := service.Labels["app.kubernetes.io/name"]; val != "" {
			return sanitize(val)
		}
		// Try simple app label
		if val := service.Labels["app"]; val != "" {
			return sanitize(val)
		}
	}

	return defaultValue
}

// getTags extracts or generates tags from service labels and metadata.
func (k *KubernetesDiscoveryWorker) getTags(service *v1.Service) []string {
	// Check for explicit tags (service-level takes precedence over namespace-level)
	var explicitTags string
	if tags := service.Annotations["nekzus.app.tags"]; tags != "" {
		explicitTags = tags
	} else if tags := service.Labels["nekzus.app.tags"]; tags != "" {
		explicitTags = tags
	}

	// If explicit tags found, use them but always include "kubernetes" tag
	if explicitTags != "" {
		tags := strings.Split(explicitTags, ",")
		// Always prepend "kubernetes" tag
		result := []string{"kubernetes"}
		for _, tag := range tags {
			if tag != "kubernetes" && !contains(result, tag) {
				result = append(result, tag)
			}
		}
		return result
	}

	// Auto-generate tags
	autoTags := []string{"kubernetes"}

	// Add Istio/Service Mesh tags
	if k.hasIstioSidecar(service) {
		autoTags = append(autoTags, "istio")
	}

	// Add namespace-level tags (inheritance)
	if ns := k.getNamespaceFromCache(service.Namespace); ns != nil {
		if nsTags := ns.Labels["nekzus.app.tags"]; nsTags != "" {
			for _, tag := range strings.Split(nsTags, ",") {
				if !contains(autoTags, tag) {
					autoTags = append(autoTags, tag)
				}
			}
		}
	}

	// Add component-based tags from standard K8s labels
	if component := service.Labels["app.kubernetes.io/component"]; component != "" {
		autoTags = append(autoTags, component)
	}

	// Add Helm-related tags
	if chartName := service.Labels["helm.sh/chart"]; chartName != "" {
		autoTags = append(autoTags, "helm")
		// Add specific chart tags
		chartTags := k.getHelmChartTags(chartName)
		for _, tag := range chartTags {
			if !contains(autoTags, tag) {
				autoTags = append(autoTags, tag)
			}
		}
	}
	if managedBy := service.Labels["app.kubernetes.io/managed-by"]; managedBy == "Helm" {
		if !contains(autoTags, "helm") {
			autoTags = append(autoTags, "helm")
		}
	}

	// Add namespace as tag
	if service.Namespace != "default" {
		autoTags = append(autoTags, service.Namespace)
	}

	// Add service type as tag
	switch service.Spec.Type {
	case v1.ServiceTypeLoadBalancer:
		autoTags = append(autoTags, "loadbalancer")
	case v1.ServiceTypeNodePort:
		autoTags = append(autoTags, "nodeport")
	case v1.ServiceTypeClusterIP:
		autoTags = append(autoTags, "clusterip")
	}

	// Add port-based tags
	for _, port := range service.Spec.Ports {
		portName := strings.ToLower(port.Name)
		if portName != "" {
			autoTags = append(autoTags, portName)
		}

		switch port.Port {
		case 80, 8080, 8000, 3000:
			if !contains(autoTags, "http") {
				autoTags = append(autoTags, "http")
			}
		case 443, 8443:
			if !contains(autoTags, "https") {
				autoTags = append(autoTags, "https")
			}
		case 5432:
			autoTags = append(autoTags, "postgres")
		case 3306:
			autoTags = append(autoTags, "mysql")
		case 6379:
			autoTags = append(autoTags, "redis")
		case 27017:
			autoTags = append(autoTags, "mongodb")
		case 9090:
			if !contains(autoTags, "prometheus") {
				autoTags = append(autoTags, "prometheus")
			}
		case 3100:
			autoTags = append(autoTags, "loki")
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

// getHelmChartTags returns tags specific to well-known Helm charts.
func (k *KubernetesDiscoveryWorker) getHelmChartTags(chartName string) []string {
	chartBase := strings.Split(chartName, "-")[0]

	chartTags := map[string][]string{
		"grafana":     {"monitoring", "grafana"},
		"prometheus":  {"monitoring", "prometheus"},
		"argocd":      {"cicd", "argocd"},
		"argo":        {"cicd", "argocd"},
		"loki":        {"logging", "loki"},
		"tempo":       {"tracing", "tempo"},
		"jaeger":      {"tracing", "jaeger"},
		"nginx":       {"ingress", "nginx"},
		"ingress":     {"ingress", "nginx"},
		"cert":        {"certificates", "cert-manager"},
		"external":    {"dns"},
		"harbor":      {"registry", "harbor"},
		"vault":       {"secrets", "vault"},
		"consul":      {"service-mesh", "consul"},
		"linkerd":     {"service-mesh", "linkerd"},
		"jenkins":     {"cicd", "jenkins"},
		"gitlab":      {"cicd", "gitlab"},
		"sonarqube":   {"cicd", "sonarqube"},
		"nexus":       {"artifacts", "nexus"},
		"artifactory": {"artifacts", "artifactory"},
	}

	if tags, ok := chartTags[chartBase]; ok {
		return tags
	}

	return nil
}

// getScopes extracts required scopes from service labels.
func (k *KubernetesDiscoveryWorker) getScopes(service *v1.Service) []string {
	// Service-level scopes take precedence
	if scopes := service.Annotations["nekzus.route.scopes"]; scopes != "" {
		return strings.Split(scopes, ",")
	}
	if scopes := service.Labels["nekzus.route.scopes"]; scopes != "" {
		return strings.Split(scopes, ",")
	}

	// Check namespace-level scopes (inheritance)
	if ns := k.getNamespaceFromCache(service.Namespace); ns != nil {
		if scopes := ns.Labels["nekzus.route.scopes"]; scopes != "" {
			return strings.Split(scopes, ",")
		}
	}

	// Default scope based on app ID
	appID := k.getLabel(service, "nekzus.app.id", "")
	if appID != "" {
		return []string{fmt.Sprintf("access:%s", appID)}
	}
	return nil
}

// guessScheme attempts to determine HTTP vs HTTPS based on port and name.
func (k *KubernetesDiscoveryWorker) guessScheme(port int, portName string) string {
	portNameLower := strings.ToLower(portName)
	if strings.Contains(portNameLower, "https") || strings.Contains(portNameLower, "tls") {
		return "https"
	}

	switch port {
	case 443, 8443:
		return "https"
	default:
		return "http"
	}
}

// calculateConfidence determines confidence score based on service metadata.
// Uses standardized confidence score constants
func (k *KubernetesDiscoveryWorker) calculateConfidence(service *v1.Service) float64 {
	confidence := ConfidenceGeneric // Base confidence for any discovered service

	// Higher confidence if explicitly labeled with nekzus
	if service.Labels["nekzus.enable"] == "true" {
		confidence = ConfidenceExplicit
	} else if service.Labels["nekzus.app.id"] != "" {
		confidence = ConfidenceMetadata
	}

	// Boost confidence for Istio/Service Mesh (high confidence for production services)
	if k.hasIstioSidecar(service) {
		confidence += 0.2
	}

	// Boost confidence for well-known Helm charts
	if chartName := service.Labels["helm.sh/chart"]; chartName != "" {
		if k.getHelmChartFriendlyName(chartName) != "" {
			confidence += 0.2
		}
	}

	// Boost confidence for LoadBalancer or NodePort services (more likely to be user-facing)
	if service.Spec.Type == v1.ServiceTypeLoadBalancer || service.Spec.Type == v1.ServiceTypeNodePort {
		confidence += 0.1
	}

	// Boost if has common web ports
	for _, port := range service.Spec.Ports {
		if port.Port == 80 || port.Port == 8080 || port.Port == 3000 || port.Port == 443 {
			confidence += 0.1
			break
		}
	}

	// Cap at 1.0
	if confidence > 1.0 {
		confidence = 1.0
	}

	return confidence
}

// getSecurityNotes generates security warnings for the proposal.
func (k *KubernetesDiscoveryWorker) getSecurityNotes(scheme string, service *v1.Service) []string {
	notes := []string{
		"Discovered via Kubernetes API",
		"JWT required",
		fmt.Sprintf("Namespace: %s", service.Namespace),
	}

	// Istio/Service Mesh notes
	if k.hasIstioSidecar(service) {
		notes = append(notes, "Service mesh (Istio) enabled - mTLS enforced between services")
	}

	if scheme == "http" {
		notes = append(notes, "Upstream uses HTTP (unencrypted)")
	}

	if service.Spec.Type == v1.ServiceTypeLoadBalancer {
		notes = append(notes, "LoadBalancer service (may be externally accessible)")
	}

	if service.Spec.Type == v1.ServiceTypeNodePort {
		notes = append(notes, "NodePort service (accessible on all nodes)")
	}

	return notes
}

// contains checks if a string slice contains a value
func contains(slice []string, value string) bool {
	for _, item := range slice {
		if item == value {
			return true
		}
	}
	return false
}

// Example Kubernetes service labels:
//
// labels:
//   nekzus.enable: "true"
//   nekzus.app.id: "grafana"
//   nekzus.app.name: "Grafana"
//   nekzus.app.icon: "https://grafana.com/icon.png"
//   nekzus.app.tags: "monitoring,metrics"
//   nekzus.route.path: "/apps/grafana/"
//   nekzus.route.scopes: "access:grafana,read:metrics"
//   nekzus.scheme: "http"
