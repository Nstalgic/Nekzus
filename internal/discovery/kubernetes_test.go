package discovery

import (
	"context"
	"strings"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

// TestKubernetesWorker_Name tests the worker's name
func TestKubernetesWorker_Name(t *testing.T) {
	dm := NewDiscoveryManager(&mockProposalStore{}, &mockEventBus{}, nil)

	client := fake.NewSimpleClientset()
	worker, err := NewKubernetesDiscoveryWorker(dm, client, 30*time.Second, "default")
	if err != nil {
		t.Fatalf("Failed to create worker: %v", err)
	}

	if worker.Name() != "kubernetes" {
		t.Errorf("Expected name 'kubernetes', got %s", worker.Name())
	}
}

// TestKubernetesWorker_DiscoverService tests service discovery
func TestKubernetesWorker_DiscoverService(t *testing.T) {
	store := &mockProposalStore{}
	bus := &mockEventBus{}
	dm := NewDiscoveryManager(store, bus, nil)

	// Start the proposal processor
	dm.wg.Add(1)
	go dm.processProposals()
	defer func() {
		close(dm.proposalCh)
		dm.wg.Wait()
	}()

	// Create fake Kubernetes client with a test service
	service := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-service",
			Namespace: "default",
			Labels: map[string]string{
				"nekzus.enable": "true",
			},
		},
		Spec: v1.ServiceSpec{
			Type:      v1.ServiceTypeClusterIP,
			ClusterIP: "10.0.0.1",
			Ports: []v1.ServicePort{
				{
					Name:     "http",
					Port:     80,
					Protocol: v1.ProtocolTCP,
				},
			},
		},
	}

	client := fake.NewSimpleClientset(service)
	worker, err := NewKubernetesDiscoveryWorker(dm, client, 30*time.Second, "default")
	if err != nil {
		t.Fatalf("Failed to create worker: %v", err)
	}

	// Perform a scan
	err = worker.scan()
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	// Wait a bit for async processing
	time.Sleep(100 * time.Millisecond)

	// Check if proposal was added to store
	proposals := store.GetProposals()
	if len(proposals) != 1 {
		t.Fatalf("Expected 1 proposal, got %d", len(proposals))
	}

	proposal := proposals[0]
	if proposal.Source != "kubernetes" {
		t.Errorf("Expected source 'kubernetes', got %s", proposal.Source)
	}
	if proposal.SuggestedApp.Name != "test-service" {
		t.Errorf("Expected app name 'test-service', got %s", proposal.SuggestedApp.Name)
	}
	if proposal.DetectedHost != "test-service.default.svc.cluster.local" {
		t.Errorf("Expected host 'test-service.default.svc.cluster.local', got %s", proposal.DetectedHost)
	}
	if proposal.DetectedPort != 80 {
		t.Errorf("Expected port 80, got %d", proposal.DetectedPort)
	}
}

// TestKubernetesWorker_SkipDisabledService tests that services without enable label are skipped
func TestKubernetesWorker_SkipDisabledService(t *testing.T) {
	dm := NewDiscoveryManager(&mockProposalStore{}, &mockEventBus{}, nil)

	// Create service without nekzus.enable label
	service := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "system-service",
			Namespace: "kube-system",
		},
		Spec: v1.ServiceSpec{
			Type:      v1.ServiceTypeClusterIP,
			ClusterIP: "10.0.0.2",
			Ports: []v1.ServicePort{
				{
					Port: 443,
				},
			},
		},
	}

	client := fake.NewSimpleClientset(service)
	worker, err := NewKubernetesDiscoveryWorker(dm, client, 30*time.Second, "kube-system")
	if err != nil {
		t.Fatalf("Failed to create worker: %v", err)
	}

	err = worker.scan()
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	// Ensure no proposal was submitted
	select {
	case <-dm.proposalCh:
		t.Error("Proposal should not be submitted for disabled service")
	case <-time.After(100 * time.Millisecond):
		// Expected: no proposal
	}
}

// TestKubernetesWorker_MultipleNamespaces tests watching multiple namespaces
func TestKubernetesWorker_MultipleNamespaces(t *testing.T) {
	store := &mockProposalStore{}
	bus := &mockEventBus{}
	dm := NewDiscoveryManager(store, bus, nil)

	// Start the proposal processor
	dm.wg.Add(1)
	go dm.processProposals()
	defer func() {
		close(dm.proposalCh)
		dm.wg.Wait()
	}()

	// Create services in different namespaces
	service1 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "app1",
			Namespace: "namespace1",
			Labels:    map[string]string{"nekzus.enable": "true"},
		},
		Spec: v1.ServiceSpec{
			Type:      v1.ServiceTypeClusterIP,
			ClusterIP: "10.0.0.3",
			Ports:     []v1.ServicePort{{Port: 8080, Protocol: v1.ProtocolTCP}},
		},
	}

	service2 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "app2",
			Namespace: "namespace2",
			Labels:    map[string]string{"nekzus.enable": "true"},
		},
		Spec: v1.ServiceSpec{
			Type:      v1.ServiceTypeClusterIP,
			ClusterIP: "10.0.0.4",
			Ports:     []v1.ServicePort{{Port: 3000, Protocol: v1.ProtocolTCP}},
		},
	}

	client := fake.NewSimpleClientset(service1, service2)
	worker, err := NewKubernetesDiscoveryWorker(dm, client, 30*time.Second, "namespace1", "namespace2")
	if err != nil {
		t.Fatalf("Failed to create worker: %v", err)
	}

	err = worker.scan()
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	// Wait for async processing
	time.Sleep(100 * time.Millisecond)

	// Should have 2 proposals
	proposals := store.GetProposals()
	if len(proposals) != 2 {
		t.Errorf("Expected 2 proposals, got %d", len(proposals))
	}

	for _, proposal := range proposals {
		if proposal.Source != "kubernetes" {
			t.Errorf("Expected source 'kubernetes', got %s", proposal.Source)
		}
	}
}

// TestKubernetesWorker_StartStop tests worker lifecycle
func TestKubernetesWorker_StartStop(t *testing.T) {
	dm := NewDiscoveryManager(&mockProposalStore{}, &mockEventBus{}, nil)

	client := fake.NewSimpleClientset()
	worker, err := NewKubernetesDiscoveryWorker(dm, client, 100*time.Millisecond, "default")
	if err != nil {
		t.Fatalf("Failed to create worker: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	// Start worker
	go worker.Start(ctx)

	// Let it run for a bit
	time.Sleep(200 * time.Millisecond)

	// Stop worker
	err = worker.Stop()
	if err != nil {
		t.Errorf("Stop failed: %v", err)
	}
}

// TestKubernetesWorker_ConfidenceCalculation tests confidence scoring
func TestKubernetesWorker_ConfidenceCalculation(t *testing.T) {
	tests := []struct {
		name          string
		labels        map[string]string
		port          int32
		serviceType   v1.ServiceType
		minConfidence float64
		maxConfidence float64
	}{
		{
			name:          "Explicitly enabled with common port",
			labels:        map[string]string{"nekzus.enable": "true"},
			port:          80,
			serviceType:   v1.ServiceTypeClusterIP,
			minConfidence: 0.95,
			maxConfidence: 1.0,
		},
		{
			name:          "With app ID and LoadBalancer",
			labels:        map[string]string{"nekzus.app.id": "myapp", "nekzus.enable": "true"},
			port:          80,
			serviceType:   v1.ServiceTypeLoadBalancer,
			minConfidence: 0.90, // 0.85 base + 0.1 for LoadBalancer + 0.1 for port
			maxConfidence: 1.0,
		},
		{
			name:          "Enabled with uncommon port",
			labels:        map[string]string{"nekzus.enable": "true"},
			port:          9999,
			serviceType:   v1.ServiceTypeClusterIP,
			minConfidence: 0.95, // Enabled services always get 0.95
			maxConfidence: 0.95,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &mockProposalStore{}
			bus := &mockEventBus{}
			dm := NewDiscoveryManager(store, bus, nil)

			// Start the proposal processor
			dm.wg.Add(1)
			go dm.processProposals()
			defer func() {
				close(dm.proposalCh)
				dm.wg.Wait()
			}()

			service := &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "default",
					Labels:    tt.labels,
				},
				Spec: v1.ServiceSpec{
					Type:      tt.serviceType,
					ClusterIP: "10.0.0.5",
					Ports:     []v1.ServicePort{{Port: tt.port, Protocol: v1.ProtocolTCP}},
				},
			}

			client := fake.NewSimpleClientset(service)
			worker, _ := NewKubernetesDiscoveryWorker(dm, client, 30*time.Second, "default")

			worker.scan()

			// Wait for async processing
			time.Sleep(100 * time.Millisecond)

			proposals := store.GetProposals()
			if len(proposals) != 1 {
				t.Fatalf("Expected 1 proposal, got %d", len(proposals))
			}

			proposal := proposals[0]
			if proposal.Confidence < tt.minConfidence || proposal.Confidence > tt.maxConfidence {
				t.Errorf("Expected confidence between %.2f and %.2f, got %.2f", tt.minConfidence, tt.maxConfidence, proposal.Confidence)
			}
		})
	}
}

// TestKubernetesWorker_SmartLabelInference tests automatic discovery from standard K8s labels
func TestKubernetesWorker_SmartLabelInference(t *testing.T) {
	tests := []struct {
		name           string
		labels         map[string]string
		annotations    map[string]string
		serviceType    v1.ServiceType
		port           int32
		shouldDiscover bool
		expectedAppID  string
		expectedName   string
		expectedTags   []string
	}{
		{
			name: "Standard app.kubernetes.io labels",
			labels: map[string]string{
				"app.kubernetes.io/name":      "grafana",
				"app.kubernetes.io/component": "frontend",
			},
			serviceType:    v1.ServiceTypeClusterIP,
			port:           3000,
			shouldDiscover: true,
			expectedAppID:  "grafana",
			expectedName:   "grafana",
			expectedTags:   []string{"kubernetes", "frontend"},
		},
		{
			name: "LoadBalancer without explicit enable",
			labels: map[string]string{
				"app.kubernetes.io/name": "nginx",
			},
			serviceType:    v1.ServiceTypeLoadBalancer,
			port:           80,
			shouldDiscover: true,
			expectedAppID:  "nginx",
			expectedName:   "nginx",
		},
		{
			name: "NodePort without explicit enable",
			labels: map[string]string{
				"app": "api-server",
			},
			serviceType:    v1.ServiceTypeNodePort,
			port:           8080,
			shouldDiscover: true,
			expectedAppID:  "api_server", // sanitize() converts hyphens to underscores
			expectedName:   "api-server",
		},
		{
			name: "Explicit nekzus label takes precedence",
			labels: map[string]string{
				"nekzus.enable":      "true",
				"nekzus.app.id":      "custom-id",
				"app.kubernetes.io/name": "grafana",
			},
			annotations: map[string]string{
				"nekzus.app.name": "Custom Name",
			},
			serviceType:    v1.ServiceTypeClusterIP,
			port:           3000,
			shouldDiscover: true,
			expectedAppID:  "custom-id",
			expectedName:   "Custom Name",
		},
		{
			name: "Helm chart detection",
			labels: map[string]string{
				"app.kubernetes.io/name":       "prometheus",
				"app.kubernetes.io/managed-by": "Helm",
				"helm.sh/chart":                "prometheus-15.0.0",
			},
			serviceType:    v1.ServiceTypeClusterIP,
			port:           9090,
			shouldDiscover: true,
			expectedAppID:  "prometheus",
			expectedName:   "prometheus",
			expectedTags:   []string{"kubernetes", "helm", "prometheus"},
		},
		{
			name: "Frontend component auto-enabled",
			labels: map[string]string{
				"app.kubernetes.io/name":      "webapp",
				"app.kubernetes.io/component": "frontend",
			},
			serviceType:    v1.ServiceTypeClusterIP,
			port:           80,
			shouldDiscover: true,
			expectedAppID:  "webapp",
			expectedName:   "webapp",
		},
		{
			name: "UI component auto-enabled",
			labels: map[string]string{
				"app.kubernetes.io/name":      "dashboard",
				"app.kubernetes.io/component": "ui",
			},
			serviceType:    v1.ServiceTypeClusterIP,
			port:           8080,
			shouldDiscover: true,
			expectedAppID:  "dashboard",
			expectedName:   "dashboard",
		},
		{
			name: "Simple app label without component",
			labels: map[string]string{
				"app": "backend-service",
			},
			serviceType:    v1.ServiceTypeClusterIP,
			port:           8080,
			shouldDiscover: false, // Should not auto-discover backend services
		},
		{
			name: "Explicit disable takes precedence",
			labels: map[string]string{
				"nekzus.enable":           "false",
				"app.kubernetes.io/component": "frontend",
			},
			serviceType:    v1.ServiceTypeLoadBalancer,
			port:           80,
			shouldDiscover: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &mockProposalStore{}
			bus := &mockEventBus{}
			dm := NewDiscoveryManager(store, bus, nil)

			// Start the proposal processor
			dm.wg.Add(1)
			go dm.processProposals()
			defer func() {
				close(dm.proposalCh)
				dm.wg.Wait()
			}()

			service := &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-service",
					Namespace:   "default",
					Labels:      tt.labels,
					Annotations: tt.annotations,
				},
				Spec: v1.ServiceSpec{
					Type:      tt.serviceType,
					ClusterIP: "10.0.0.100",
					Ports: []v1.ServicePort{
						{
							Port:     tt.port,
							Protocol: v1.ProtocolTCP,
						},
					},
				},
			}

			client := fake.NewSimpleClientset(service)
			worker, err := NewKubernetesDiscoveryWorker(dm, client, 30*time.Second, "default")
			if err != nil {
				t.Fatalf("Failed to create worker: %v", err)
			}

			err = worker.scan()
			if err != nil {
				t.Fatalf("Scan failed: %v", err)
			}

			// Wait for async processing
			time.Sleep(100 * time.Millisecond)

			proposals := store.GetProposals()

			if tt.shouldDiscover {
				if len(proposals) != 1 {
					t.Fatalf("Expected 1 proposal, got %d", len(proposals))
				}

				proposal := proposals[0]
				if proposal.SuggestedApp.ID != tt.expectedAppID {
					t.Errorf("Expected app ID '%s', got '%s'", tt.expectedAppID, proposal.SuggestedApp.ID)
				}
				if proposal.SuggestedApp.Name != tt.expectedName {
					t.Errorf("Expected app name '%s', got '%s'", tt.expectedName, proposal.SuggestedApp.Name)
				}

				// Check tags if specified
				if len(tt.expectedTags) > 0 {
					foundTags := make(map[string]bool)
					for _, tag := range proposal.SuggestedApp.Tags {
						foundTags[tag] = true
					}
					for _, expectedTag := range tt.expectedTags {
						if !foundTags[expectedTag] {
							t.Errorf("Expected tag '%s' not found in %v", expectedTag, proposal.SuggestedApp.Tags)
						}
					}
				}
			} else {
				if len(proposals) != 0 {
					t.Errorf("Expected 0 proposals, got %d", len(proposals))
				}
			}
		})
	}
}

// TestKubernetesWorker_IngressDiscovery tests Ingress-based service discovery
func TestKubernetesWorker_IngressDiscovery(t *testing.T) {
	store := &mockProposalStore{}
	bus := &mockEventBus{}
	dm := NewDiscoveryManager(store, bus, nil)

	// Start the proposal processor
	dm.wg.Add(1)
	go dm.processProposals()
	defer func() {
		close(dm.proposalCh)
		dm.wg.Wait()
	}()

	// Create a backend service
	service := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "grafana",
			Namespace: "monitoring",
		},
		Spec: v1.ServiceSpec{
			Type:      v1.ServiceTypeClusterIP,
			ClusterIP: "10.0.0.50",
			Ports: []v1.ServicePort{
				{
					Name:     "http",
					Port:     3000,
					Protocol: v1.ProtocolTCP,
				},
			},
		},
	}

	// Create an Ingress that routes to the service
	ingress := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "grafana-ingress",
			Namespace: "monitoring",
			Labels: map[string]string{
				"nekzus.enable": "true",
			},
		},
		Spec: networkingv1.IngressSpec{
			Rules: []networkingv1.IngressRule{
				{
					Host: "grafana.example.com",
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{
								{
									Path:     "/grafana",
									PathType: pathTypePtr(networkingv1.PathTypePrefix),
									Backend: networkingv1.IngressBackend{
										Service: &networkingv1.IngressServiceBackend{
											Name: "grafana",
											Port: networkingv1.ServiceBackendPort{
												Number: 3000,
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	client := fake.NewSimpleClientset(service, ingress)
	worker, err := NewKubernetesDiscoveryWorker(dm, client, 30*time.Second, "monitoring")
	if err != nil {
		t.Fatalf("Failed to create worker: %v", err)
	}

	err = worker.scan()
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	// Wait for async processing
	time.Sleep(100 * time.Millisecond)

	proposals := store.GetProposals()
	if len(proposals) != 1 {
		t.Fatalf("Expected 1 proposal from Ingress, got %d", len(proposals))
	}

	proposal := proposals[0]

	// Verify the proposal uses Ingress path
	if !strings.Contains(proposal.SuggestedRoute.PathBase, "/grafana") {
		t.Errorf("Expected path to contain '/grafana', got %s", proposal.SuggestedRoute.PathBase)
	}

	// Verify it discovered the backend service
	if proposal.SuggestedApp.Name != "grafana" {
		t.Errorf("Expected app name 'grafana', got %s", proposal.SuggestedApp.Name)
	}
}

// TestKubernetesWorker_IngressWithTLS tests TLS detection from Ingress
func TestKubernetesWorker_IngressWithTLS(t *testing.T) {
	store := &mockProposalStore{}
	bus := &mockEventBus{}
	dm := NewDiscoveryManager(store, bus, nil)

	dm.wg.Add(1)
	go dm.processProposals()
	defer func() {
		close(dm.proposalCh)
		dm.wg.Wait()
	}()

	service := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "webapp",
			Namespace: "default",
		},
		Spec: v1.ServiceSpec{
			Type:      v1.ServiceTypeClusterIP,
			ClusterIP: "10.0.0.60",
			Ports: []v1.ServicePort{
				{Port: 8080, Protocol: v1.ProtocolTCP},
			},
		},
	}

	// Ingress with TLS configuration
	ingress := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "webapp-ingress",
			Namespace: "default",
			Labels: map[string]string{
				"nekzus.enable": "true",
			},
		},
		Spec: networkingv1.IngressSpec{
			TLS: []networkingv1.IngressTLS{
				{
					Hosts:      []string{"webapp.example.com"},
					SecretName: "webapp-tls",
				},
			},
			Rules: []networkingv1.IngressRule{
				{
					Host: "webapp.example.com",
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{
								{
									Path:     "/",
									PathType: pathTypePtr(networkingv1.PathTypePrefix),
									Backend: networkingv1.IngressBackend{
										Service: &networkingv1.IngressServiceBackend{
											Name: "webapp",
											Port: networkingv1.ServiceBackendPort{Number: 8080},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	client := fake.NewSimpleClientset(service, ingress)
	worker, err := NewKubernetesDiscoveryWorker(dm, client, 30*time.Second, "default")
	if err != nil {
		t.Fatalf("Failed to create worker: %v", err)
	}

	err = worker.scan()
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	proposals := store.GetProposals()
	if len(proposals) != 1 {
		t.Fatalf("Expected 1 proposal, got %d", len(proposals))
	}

	proposal := proposals[0]

	// Should detect HTTPS from TLS config
	if proposal.DetectedScheme != "https" {
		t.Errorf("Expected scheme 'https' from TLS config, got %s", proposal.DetectedScheme)
	}
}

// TestKubernetesWorker_IngressMultiplePaths tests Ingress with multiple paths
func TestKubernetesWorker_IngressMultiplePaths(t *testing.T) {
	store := &mockProposalStore{}
	bus := &mockEventBus{}
	dm := NewDiscoveryManager(store, bus, nil)

	dm.wg.Add(1)
	go dm.processProposals()
	defer func() {
		close(dm.proposalCh)
		dm.wg.Wait()
	}()

	// Two services
	service1 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "default"},
		Spec:       v1.ServiceSpec{Type: v1.ServiceTypeClusterIP, ClusterIP: "10.0.0.70", Ports: []v1.ServicePort{{Port: 8000, Protocol: v1.ProtocolTCP}}},
	}
	service2 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "default"},
		Spec:       v1.ServiceSpec{Type: v1.ServiceTypeClusterIP, ClusterIP: "10.0.0.71", Ports: []v1.ServicePort{{Port: 3000, Protocol: v1.ProtocolTCP}}},
	}

	// Ingress with multiple paths
	ingress := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "multi-path-ingress",
			Namespace: "default",
			Labels:    map[string]string{"nekzus.enable": "true"},
		},
		Spec: networkingv1.IngressSpec{
			Rules: []networkingv1.IngressRule{
				{
					Host: "example.com",
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{
								{
									Path:     "/api",
									PathType: pathTypePtr(networkingv1.PathTypePrefix),
									Backend: networkingv1.IngressBackend{
										Service: &networkingv1.IngressServiceBackend{Name: "api", Port: networkingv1.ServiceBackendPort{Number: 8000}},
									},
								},
								{
									Path:     "/web",
									PathType: pathTypePtr(networkingv1.PathTypePrefix),
									Backend: networkingv1.IngressBackend{
										Service: &networkingv1.IngressServiceBackend{Name: "web", Port: networkingv1.ServiceBackendPort{Number: 3000}},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	client := fake.NewSimpleClientset(service1, service2, ingress)
	worker, err := NewKubernetesDiscoveryWorker(dm, client, 30*time.Second, "default")
	if err != nil {
		t.Fatalf("Failed to create worker: %v", err)
	}

	err = worker.scan()
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	proposals := store.GetProposals()
	// Should have 2 proposals, one for each backend service
	if len(proposals) != 2 {
		t.Fatalf("Expected 2 proposals, got %d", len(proposals))
	}

	// Verify each service has the correct path
	pathMap := make(map[string]string)
	for _, p := range proposals {
		pathMap[p.SuggestedApp.Name] = p.SuggestedRoute.PathBase
	}

	if !strings.Contains(pathMap["api"], "/api") {
		t.Errorf("Expected API path to contain '/api', got %s", pathMap["api"])
	}
	if !strings.Contains(pathMap["web"], "/web") {
		t.Errorf("Expected web path to contain '/web', got %s", pathMap["web"])
	}
}

// Helper function for PathType pointer
func pathTypePtr(pt networkingv1.PathType) *networkingv1.PathType {
	return &pt
}

// TestKubernetesWorker_IstioDetection tests Istio service mesh detection
func TestKubernetesWorker_IstioDetection(t *testing.T) {
	tests := []struct {
		name            string
		labels          map[string]string
		annotations     map[string]string
		serviceType     v1.ServiceType
		port            int32
		shouldDiscover  bool
		expectIstioNote bool
		expectedTags    []string
		minConfidence   float64
	}{
		{
			name: "Istio sidecar injection enabled",
			labels: map[string]string{
				"app.kubernetes.io/name": "myapp",
				"istio-injection":        "enabled",
			},
			serviceType:     v1.ServiceTypeClusterIP,
			port:            8080,
			shouldDiscover:  true,
			expectIstioNote: true,
			expectedTags:    []string{"kubernetes", "istio"},
			minConfidence:   0.70, // Boost for service mesh
		},
		{
			name: "Istio sidecar.istio.io/inject annotation",
			labels: map[string]string{
				"app.kubernetes.io/name": "webapp",
			},
			annotations: map[string]string{
				"sidecar.istio.io/inject": "true",
			},
			serviceType:     v1.ServiceTypeClusterIP,
			port:            80,
			shouldDiscover:  true,
			expectIstioNote: true,
			expectedTags:    []string{"kubernetes", "istio"},
			minConfidence:   0.70,
		},
		{
			name: "Istio version label",
			labels: map[string]string{
				"app.kubernetes.io/name": "api",
				"istio.io/rev":           "1-18-0",
			},
			serviceType:     v1.ServiceTypeClusterIP,
			port:            8000,
			shouldDiscover:  true,
			expectIstioNote: true,
			expectedTags:    []string{"kubernetes", "istio"},
			minConfidence:   0.70,
		},
		{
			name: "Service mesh enabled without Istio",
			labels: map[string]string{
				"app.kubernetes.io/name":          "service",
				"service.istio.io/canonical-name": "myservice",
			},
			serviceType:     v1.ServiceTypeClusterIP,
			port:            3000,
			shouldDiscover:  true,
			expectIstioNote: true,
			expectedTags:    []string{"kubernetes", "istio"},
			minConfidence:   0.70,
		},
		{
			name: "No Istio labels",
			labels: map[string]string{
				"app.kubernetes.io/name": "regular-app",
			},
			serviceType:     v1.ServiceTypeClusterIP,
			port:            8080,
			shouldDiscover:  false, // ClusterIP without frontend component or service mesh
			expectIstioNote: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &mockProposalStore{}
			bus := &mockEventBus{}
			dm := NewDiscoveryManager(store, bus, nil)

			dm.wg.Add(1)
			go dm.processProposals()
			defer func() {
				close(dm.proposalCh)
				dm.wg.Wait()
			}()

			service := &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-service",
					Namespace:   "default",
					Labels:      tt.labels,
					Annotations: tt.annotations,
				},
				Spec: v1.ServiceSpec{
					Type:      tt.serviceType,
					ClusterIP: "10.0.0.100",
					Ports: []v1.ServicePort{
						{Port: tt.port, Protocol: v1.ProtocolTCP},
					},
				},
			}

			client := fake.NewSimpleClientset(service)
			worker, err := NewKubernetesDiscoveryWorker(dm, client, 30*time.Second, "default")
			if err != nil {
				t.Fatalf("Failed to create worker: %v", err)
			}

			err = worker.scan()
			if err != nil {
				t.Fatalf("Scan failed: %v", err)
			}

			time.Sleep(100 * time.Millisecond)

			proposals := store.GetProposals()

			if tt.shouldDiscover {
				if len(proposals) != 1 {
					t.Fatalf("Expected 1 proposal, got %d", len(proposals))
				}

				proposal := proposals[0]

				// Check confidence boost for service mesh
				if tt.minConfidence > 0 && proposal.Confidence < tt.minConfidence {
					t.Errorf("Expected confidence >= %.2f, got %.2f", tt.minConfidence, proposal.Confidence)
				}

				// Check for Istio tag
				if tt.expectedTags != nil {
					foundTags := make(map[string]bool)
					for _, tag := range proposal.SuggestedApp.Tags {
						foundTags[tag] = true
					}
					for _, expectedTag := range tt.expectedTags {
						if !foundTags[expectedTag] {
							t.Errorf("Expected tag '%s' not found in %v", expectedTag, proposal.SuggestedApp.Tags)
						}
					}
				}

				// Check for Istio security note
				if tt.expectIstioNote {
					hasIstioNote := false
					for _, note := range proposal.SecurityNotes {
						if strings.Contains(strings.ToLower(note), "istio") || strings.Contains(strings.ToLower(note), "service mesh") {
							hasIstioNote = true
							break
						}
					}
					if !hasIstioNote {
						t.Errorf("Expected Istio/service mesh note in security notes, got %v", proposal.SecurityNotes)
					}
				}
			} else {
				if len(proposals) != 0 {
					t.Errorf("Expected 0 proposals, got %d", len(proposals))
				}
			}
		})
	}
}

// TestKubernetesWorker_NamespaceLabels tests namespace-level discovery configuration
func TestKubernetesWorker_NamespaceLabels(t *testing.T) {
	tests := []struct {
		name            string
		namespaceLabels map[string]string
		serviceLabels   map[string]string
		shouldDiscover  bool
		expectedTags    []string
		expectedScopes  []string
	}{
		{
			name: "Namespace with nekzus.enable=true",
			namespaceLabels: map[string]string{
				"nekzus.enable": "true",
			},
			serviceLabels: map[string]string{
				"app": "myapp",
			},
			shouldDiscover: true,
			expectedTags:   []string{"kubernetes"},
		},
		{
			name: "Namespace tags inherited by service",
			namespaceLabels: map[string]string{
				"nekzus.enable":   "true",
				"nekzus.app.tags": "production,api",
			},
			serviceLabels: map[string]string{
				"app.kubernetes.io/name": "api-server",
			},
			shouldDiscover: true,
			expectedTags:   []string{"kubernetes", "production", "api"},
		},
		{
			name: "Namespace scopes inherited by service",
			namespaceLabels: map[string]string{
				"nekzus.enable":       "true",
				"nekzus.route.scopes": "access:team-a,read:metrics",
			},
			serviceLabels: map[string]string{
				"app.kubernetes.io/name": "dashboard",
			},
			shouldDiscover: true,
			expectedScopes: []string{"access:team-a", "read:metrics"},
		},
		{
			name: "Service labels override namespace labels",
			namespaceLabels: map[string]string{
				"nekzus.enable":   "true",
				"nekzus.app.tags": "namespace-tag",
			},
			serviceLabels: map[string]string{
				"app.kubernetes.io/name": "webapp",
				"nekzus.app.tags":    "service-tag",
			},
			shouldDiscover: true,
			expectedTags:   []string{"kubernetes", "service-tag"}, // Service tag overrides namespace tag
		},
		{
			name: "Namespace disable overrides everything",
			namespaceLabels: map[string]string{
				"nekzus.enable": "false",
			},
			serviceLabels: map[string]string{
				"app.kubernetes.io/name":      "myapp",
				"app.kubernetes.io/component": "frontend",
			},
			shouldDiscover: false, // Namespace disable takes precedence
		},
		{
			name: "Service can override namespace enable",
			namespaceLabels: map[string]string{
				"nekzus.enable": "true",
			},
			serviceLabels: map[string]string{
				"app":               "backend",
				"nekzus.enable": "false",
			},
			shouldDiscover: false, // Service explicitly disabled
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &mockProposalStore{}
			bus := &mockEventBus{}
			dm := NewDiscoveryManager(store, bus, nil)

			dm.wg.Add(1)
			go dm.processProposals()
			defer func() {
				close(dm.proposalCh)
				dm.wg.Wait()
			}()

			// Create namespace with labels
			namespace := &v1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "test-namespace",
					Labels: tt.namespaceLabels,
				},
			}

			service := &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-service",
					Namespace: "test-namespace",
					Labels:    tt.serviceLabels,
				},
				Spec: v1.ServiceSpec{
					Type:      v1.ServiceTypeClusterIP,
					ClusterIP: "10.0.0.100",
					Ports: []v1.ServicePort{
						{Port: 8080, Protocol: v1.ProtocolTCP},
					},
				},
			}

			client := fake.NewSimpleClientset(namespace, service)
			worker, err := NewKubernetesDiscoveryWorker(dm, client, 30*time.Second, "test-namespace")
			if err != nil {
				t.Fatalf("Failed to create worker: %v", err)
			}

			err = worker.scan()
			if err != nil {
				t.Fatalf("Scan failed: %v", err)
			}

			time.Sleep(100 * time.Millisecond)

			proposals := store.GetProposals()

			if tt.shouldDiscover {
				if len(proposals) != 1 {
					t.Fatalf("Expected 1 proposal, got %d", len(proposals))
				}

				proposal := proposals[0]

				// Check tags
				if tt.expectedTags != nil {
					foundTags := make(map[string]bool)
					for _, tag := range proposal.SuggestedApp.Tags {
						foundTags[tag] = true
					}
					for _, expectedTag := range tt.expectedTags {
						if !foundTags[expectedTag] {
							t.Errorf("Expected tag '%s' not found in %v", expectedTag, proposal.SuggestedApp.Tags)
						}
					}
				}

				// Check scopes
				if tt.expectedScopes != nil {
					foundScopes := make(map[string]bool)
					for _, scope := range proposal.SuggestedRoute.Scopes {
						foundScopes[scope] = true
					}
					for _, expectedScope := range tt.expectedScopes {
						if !foundScopes[expectedScope] {
							t.Errorf("Expected scope '%s' not found in %v", expectedScope, proposal.SuggestedRoute.Scopes)
						}
					}
				}
			} else {
				if len(proposals) != 0 {
					t.Errorf("Expected 0 proposals, got %d", len(proposals))
				}
			}
		})
	}
}

// TestKubernetesWorker_CommonHelmCharts tests recognition of popular Helm charts
func TestKubernetesWorker_CommonHelmCharts(t *testing.T) {
	tests := []struct {
		name          string
		chartName     string
		appName       string
		expectedName  string
		expectedTags  []string
		minConfidence float64
	}{
		{
			name:          "Grafana Helm chart",
			chartName:     "grafana-7.0.0",
			appName:       "grafana",
			expectedName:  "grafana", // Uses app.kubernetes.io/name, not friendly name
			expectedTags:  []string{"kubernetes", "helm", "monitoring", "grafana"},
			minConfidence: 0.79, // Account for floating point precision
		},
		{
			name:          "Prometheus Helm chart",
			chartName:     "prometheus-15.10.0",
			appName:       "prometheus",
			expectedName:  "prometheus", // Uses app.kubernetes.io/name, not friendly name
			expectedTags:  []string{"kubernetes", "helm", "monitoring", "prometheus"},
			minConfidence: 0.79, // Account for floating point precision
		},
		{
			name:          "ArgoCD Helm chart",
			chartName:     "argo-cd-5.0.0",
			appName:       "argocd-server",
			expectedName:  "argocd-server", // Uses app.kubernetes.io/name, not friendly name
			expectedTags:  []string{"kubernetes", "helm", "cicd", "argocd"},
			minConfidence: 0.79, // Account for floating point precision
		},
		{
			name:          "PostgreSQL Helm chart (should not auto-discover)",
			chartName:     "postgresql-12.0.0",
			appName:       "postgresql",
			expectedName:  "",
			expectedTags:  nil,
			minConfidence: 0,
		},
		{
			name:          "Redis Helm chart (should not auto-discover)",
			chartName:     "redis-17.0.0",
			appName:       "redis",
			expectedName:  "",
			expectedTags:  nil,
			minConfidence: 0,
		},
		{
			name:          "NGINX Ingress Controller",
			chartName:     "ingress-nginx-4.5.0",
			appName:       "ingress-nginx-controller",
			expectedName:  "ingress-nginx-controller", // Uses app.kubernetes.io/name, not friendly name
			expectedTags:  []string{"kubernetes", "helm", "ingress", "nginx"},
			minConfidence: 0.79, // Account for floating point precision
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &mockProposalStore{}
			bus := &mockEventBus{}
			dm := NewDiscoveryManager(store, bus, nil)

			dm.wg.Add(1)
			go dm.processProposals()
			defer func() {
				close(dm.proposalCh)
				dm.wg.Wait()
			}()

			service := &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      tt.appName,
					Namespace: "default",
					Labels: map[string]string{
						"app.kubernetes.io/name":       tt.appName,
						"app.kubernetes.io/managed-by": "Helm",
						"helm.sh/chart":                tt.chartName,
					},
				},
				Spec: v1.ServiceSpec{
					Type:      v1.ServiceTypeClusterIP,
					ClusterIP: "10.0.0.100",
					Ports: []v1.ServicePort{
						{Port: 3000, Protocol: v1.ProtocolTCP},
					},
				},
			}

			client := fake.NewSimpleClientset(service)
			worker, err := NewKubernetesDiscoveryWorker(dm, client, 30*time.Second, "default")
			if err != nil {
				t.Fatalf("Failed to create worker: %v", err)
			}

			err = worker.scan()
			if err != nil {
				t.Fatalf("Scan failed: %v", err)
			}

			time.Sleep(100 * time.Millisecond)

			proposals := store.GetProposals()

			shouldDiscover := tt.minConfidence > 0

			if shouldDiscover {
				if len(proposals) != 1 {
					t.Fatalf("Expected 1 proposal, got %d", len(proposals))
				}

				proposal := proposals[0]

				// Check friendly name
				if tt.expectedName != "" && proposal.SuggestedApp.Name != tt.expectedName {
					t.Errorf("Expected app name '%s', got '%s'", tt.expectedName, proposal.SuggestedApp.Name)
				}

				// Check confidence boost for well-known charts
				if proposal.Confidence < tt.minConfidence {
					t.Errorf("Expected confidence >= %.2f, got %.2f", tt.minConfidence, proposal.Confidence)
				}

				// Check tags
				if tt.expectedTags != nil {
					foundTags := make(map[string]bool)
					for _, tag := range proposal.SuggestedApp.Tags {
						foundTags[tag] = true
					}
					for _, expectedTag := range tt.expectedTags {
						if !foundTags[expectedTag] {
							t.Errorf("Expected tag '%s' not found in %v", expectedTag, proposal.SuggestedApp.Tags)
						}
					}
				}
			} else {
				// Should not auto-discover backend services like databases
				if len(proposals) != 0 {
					t.Errorf("Expected 0 proposals for backend service, got %d", len(proposals))
				}
			}
		})
	}
}
