package discovery

import (
	"sync"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

// Kubernetes Namespace Cache Race Tests
// These tests verify that the namespaceCache map is accessed safely under concurrent use.

// TestKubernetesWorker_NamespaceCacheRace tests concurrent reads and writes
// to the namespace cache to detect data races.
// Run with: go test -race ./internal/discovery/... -run TestKubernetesWorker_NamespaceCacheRace
func TestKubernetesWorker_NamespaceCacheRace(t *testing.T) {
	store := &mockProposalStore{}
	bus := &mockEventBus{}
	dm := NewDiscoveryManager(store, bus, nil)

	// Start proposal processor
	dm.wg.Add(1)
	go dm.processProposals()
	defer func() {
		close(dm.proposalCh)
		dm.wg.Wait()
	}()

	// Create namespaces
	ns1 := &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "namespace-1",
			Labels: map[string]string{
				"nekzus.enable": "true",
			},
		},
	}
	ns2 := &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "namespace-2",
			Labels: map[string]string{
				"nekzus.enable": "true",
			},
		},
	}

	// Create services in both namespaces
	svc1 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "service-1",
			Namespace: "namespace-1",
			Labels: map[string]string{
				"nekzus.enable": "true",
			},
		},
		Spec: v1.ServiceSpec{
			Type:      v1.ServiceTypeClusterIP,
			ClusterIP: "10.0.0.1",
			Ports: []v1.ServicePort{
				{Port: 8080, Protocol: v1.ProtocolTCP},
			},
		},
	}
	svc2 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "service-2",
			Namespace: "namespace-2",
			Labels: map[string]string{
				"nekzus.enable": "true",
			},
		},
		Spec: v1.ServiceSpec{
			Type:      v1.ServiceTypeClusterIP,
			ClusterIP: "10.0.0.2",
			Ports: []v1.ServicePort{
				{Port: 8080, Protocol: v1.ProtocolTCP},
			},
		},
	}

	client := fake.NewSimpleClientset(ns1, ns2, svc1, svc2)
	worker, err := NewKubernetesDiscoveryWorker(dm, client, 30*time.Second, "namespace-1", "namespace-2")
	if err != nil {
		t.Fatalf("Failed to create worker: %v", err)
	}

	// Run concurrent scans to trigger race conditions
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				err := worker.scan()
				if err != nil {
					t.Errorf("Scan failed: %v", err)
				}
			}
		}()
	}

	wg.Wait()
}

// TestKubernetesWorker_ConcurrentNamespaceCacheAccess tests that isEnabled
// can safely read from the namespace cache while scan() is writing to it.
func TestKubernetesWorker_ConcurrentNamespaceCacheAccess(t *testing.T) {
	store := &mockProposalStore{}
	bus := &mockEventBus{}
	dm := NewDiscoveryManager(store, bus, nil)

	// Start proposal processor
	dm.wg.Add(1)
	go dm.processProposals()
	defer func() {
		close(dm.proposalCh)
		dm.wg.Wait()
	}()

	// Create multiple namespaces
	var objects []interface{}
	for i := 0; i < 5; i++ {
		ns := &v1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespaceName(i),
				Labels: map[string]string{
					"nekzus.enable": "true",
				},
			},
		}
		objects = append(objects, ns)

		// Create a service in each namespace
		svc := &v1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "service",
				Namespace: namespaceName(i),
				Labels: map[string]string{
					"app.kubernetes.io/name": "test-app",
				},
			},
			Spec: v1.ServiceSpec{
				Type:      v1.ServiceTypeClusterIP,
				ClusterIP: "10.0.0." + string(rune('1'+i)),
				Ports: []v1.ServicePort{
					{Port: 8080, Protocol: v1.ProtocolTCP},
				},
			},
		}
		objects = append(objects, svc)
	}

	// Convert to runtime.Objects for the fake client
	runtimeObjects := make([]interface{}, 0, len(objects))
	for _, obj := range objects {
		runtimeObjects = append(runtimeObjects, obj)
	}

	// Create client with all objects
	client := fake.NewSimpleClientset(
		objects[0].(*v1.Namespace), objects[1].(*v1.Service),
		objects[2].(*v1.Namespace), objects[3].(*v1.Service),
		objects[4].(*v1.Namespace), objects[5].(*v1.Service),
		objects[6].(*v1.Namespace), objects[7].(*v1.Service),
		objects[8].(*v1.Namespace), objects[9].(*v1.Service),
	)

	namespaces := []string{
		namespaceName(0), namespaceName(1), namespaceName(2),
		namespaceName(3), namespaceName(4),
	}

	worker, err := NewKubernetesDiscoveryWorker(dm, client, 30*time.Second, namespaces...)
	if err != nil {
		t.Fatalf("Failed to create worker: %v", err)
	}

	// Start multiple concurrent scanners
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				worker.scan()
			}
		}()
	}

	// Also read from cache concurrently
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			svc := &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-svc",
					Namespace: namespaceName(idx % 5),
				},
			}
			for j := 0; j < 100; j++ {
				worker.isEnabled(svc)
			}
		}(i)
	}

	wg.Wait()
}

// TestKubernetesWorker_NamespaceCacheReadDuringWrite tests simultaneous
// read and write operations to verify proper synchronization.
func TestKubernetesWorker_NamespaceCacheReadDuringWrite(t *testing.T) {
	store := &mockProposalStore{}
	bus := &mockEventBus{}
	dm := NewDiscoveryManager(store, bus, nil)

	// Start proposal processor
	dm.wg.Add(1)
	go dm.processProposals()
	defer func() {
		close(dm.proposalCh)
		dm.wg.Wait()
	}()

	// Create namespace and services
	ns := &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-ns",
			Labels: map[string]string{
				"nekzus.enable":   "true",
				"nekzus.app.tags": "production",
			},
		},
	}

	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-service",
			Namespace: "test-ns",
			Labels: map[string]string{
				"nekzus.enable": "true",
			},
		},
		Spec: v1.ServiceSpec{
			Type:      v1.ServiceTypeClusterIP,
			ClusterIP: "10.0.0.1",
			Ports: []v1.ServicePort{
				{Port: 80, Protocol: v1.ProtocolTCP},
			},
		},
	}

	client := fake.NewSimpleClientset(ns, svc)
	worker, err := NewKubernetesDiscoveryWorker(dm, client, 30*time.Second, "test-ns")
	if err != nil {
		t.Fatalf("Failed to create worker: %v", err)
	}

	// Run concurrent operations
	var wg sync.WaitGroup

	// Writers (scan populates the cache)
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				worker.scan()
			}
		}()
	}

	// Readers (these functions read from the cache)
	testService := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "reader-test",
			Namespace: "test-ns",
		},
	}

	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				worker.isEnabled(testService)
				worker.getLabel(testService, "nekzus.app.tags", "")
				worker.getTags(testService)
				worker.getScopes(testService)
			}
		}()
	}

	wg.Wait()
}

// Helper function to generate namespace names
func namespaceName(i int) string {
	return "test-namespace-" + string(rune('a'+i))
}
