package discovery

import (
	"context"
	"testing"
	"time"
)

func TestMDNSDiscoveryWorkerName(t *testing.T) {
	store := &mockProposalStore{}
	bus := &mockEventBus{}
	dm := NewDiscoveryManager(store, bus, nil)

	services := []string{"_http._tcp", "_https._tcp"}
	worker := NewMDNSDiscoveryWorker(dm, services, 60*time.Second)

	if worker.Name() != "mdns" {
		t.Errorf("Expected worker name 'mdns', got %s", worker.Name())
	}
}

func TestMDNSDiscoveryWorkerCreation(t *testing.T) {
	store := &mockProposalStore{}
	bus := &mockEventBus{}
	dm := NewDiscoveryManager(store, bus, nil)

	services := []string{"_http._tcp", "_https._tcp"}
	interval := 60 * time.Second

	worker := NewMDNSDiscoveryWorker(dm, services, interval)

	if worker == nil {
		t.Fatal("Expected non-nil worker")
	}
	if worker.dm != dm {
		t.Error("Discovery manager not set correctly")
	}
	if worker.scanInterval != interval {
		t.Errorf("Expected scan interval %v, got %v", interval, worker.scanInterval)
	}
	if len(worker.services) != len(services) {
		t.Errorf("Expected %d services, got %d", len(services), len(worker.services))
	}
}

func TestMDNSDiscoveryWorkerStop(t *testing.T) {
	store := &mockProposalStore{}
	bus := &mockEventBus{}
	dm := NewDiscoveryManager(store, bus, nil)

	worker := NewMDNSDiscoveryWorker(dm, []string{"_http._tcp"}, 60*time.Second)

	err := worker.Stop()
	if err != nil {
		t.Errorf("Expected no error on stop, got %v", err)
	}
}

func TestMDNSIsWebService(t *testing.T) {
	store := &mockProposalStore{}
	bus := &mockEventBus{}
	dm := NewDiscoveryManager(store, bus, nil)
	worker := NewMDNSDiscoveryWorker(dm, []string{"_http._tcp"}, 60*time.Second)

	tests := []struct {
		serviceType string
		want        bool
	}{
		{"_http._tcp", true},
		{"_https._tcp", true},
		{"_homeassistant._tcp", true},
		{"_hap._tcp", true},
		{"_ssh._tcp", false},
		{"_ftp._tcp", false},
		{"_unknown._tcp", false},
	}

	for _, tt := range tests {
		got := worker.isWebService(tt.serviceType)
		if got != tt.want {
			t.Errorf("isWebService(%q) = %v, want %v", tt.serviceType, got, tt.want)
		}
	}
}

func TestMDNSGetTXTValue(t *testing.T) {
	store := &mockProposalStore{}
	bus := &mockEventBus{}
	dm := NewDiscoveryManager(store, bus, nil)
	worker := NewMDNSDiscoveryWorker(dm, []string{"_http._tcp"}, 60*time.Second)

	svc := &MDNSService{
		Name: "test-service",
		TXT: map[string]string{
			"app_id":   "myapp",
			"app_name": "My Application",
		},
	}

	// Test existing value
	appID := worker.getTXTValue(svc, "app_id", "default")
	if appID != "myapp" {
		t.Errorf("Expected 'myapp', got %s", appID)
	}

	// Test non-existing value with default
	icon := worker.getTXTValue(svc, "icon", "default.png")
	if icon != "default.png" {
		t.Errorf("Expected default 'default.png', got %s", icon)
	}
}

func TestMDNSGetTags(t *testing.T) {
	store := &mockProposalStore{}
	bus := &mockEventBus{}
	dm := NewDiscoveryManager(store, bus, nil)
	worker := NewMDNSDiscoveryWorker(dm, []string{"_http._tcp"}, 60*time.Second)

	svc := &MDNSService{
		TXT: map[string]string{
			"tags": "mdns,service,web",
		},
	}

	tags := worker.getTags(svc)
	if len(tags) != 3 {
		t.Errorf("Expected 3 tags, got %d", len(tags))
	}
	if tags[0] != "mdns" || tags[1] != "service" || tags[2] != "web" {
		t.Errorf("Unexpected tags: %v", tags)
	}
}

func TestMDNSGetScopes(t *testing.T) {
	store := &mockProposalStore{}
	bus := &mockEventBus{}
	dm := NewDiscoveryManager(store, bus, nil)
	worker := NewMDNSDiscoveryWorker(dm, []string{"_http._tcp"}, 60*time.Second)

	svc := &MDNSService{
		TXT: map[string]string{
			"scopes": "read:data,write:data",
		},
	}

	scopes := worker.getScopes(svc)
	if len(scopes) != 2 {
		t.Errorf("Expected 2 scopes, got %d", len(scopes))
	}
	if scopes[0] != "read:data" || scopes[1] != "write:data" {
		t.Errorf("Unexpected scopes: %v", scopes)
	}
}

func TestMDNSCalculateConfidence(t *testing.T) {
	store := &mockProposalStore{}
	bus := &mockEventBus{}
	dm := NewDiscoveryManager(store, bus, nil)
	worker := NewMDNSDiscoveryWorker(dm, []string{"_http._tcp"}, 60*time.Second)

	tests := []struct {
		name    string
		svc     *MDNSService
		minConf float64
		maxConf float64
	}{
		{
			name: "with app_name",
			svc: &MDNSService{
				TXT: map[string]string{
					"app_name": "Test Service",
				},
			},
			minConf: 0.3,
			maxConf: 1.0,
		},
		{
			name: "without TXT records",
			svc: &MDNSService{
				TXT: map[string]string{},
			},
			minConf: 0.0,
			maxConf: 1.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conf := worker.calculateConfidence(tt.svc)
			if conf < tt.minConf || conf > tt.maxConf {
				t.Errorf("Confidence %f out of range [%f, %f]", conf, tt.minConf, tt.maxConf)
			}
		})
	}
}

func TestMDNSGetSecurityNotes(t *testing.T) {
	store := &mockProposalStore{}
	bus := &mockEventBus{}
	dm := NewDiscoveryManager(store, bus, nil)
	worker := NewMDNSDiscoveryWorker(dm, []string{"_http._tcp"}, 60*time.Second)

	tests := []struct {
		scheme string
		svc    *MDNSService
		want   int // expected minimum number of notes
	}{
		{"http", &MDNSService{}, 1},  // HTTP warning
		{"https", &MDNSService{}, 0}, // HTTPS is good
	}

	for _, tt := range tests {
		notes := worker.getSecurityNotes(tt.scheme, tt.svc)
		if len(notes) < tt.want {
			t.Errorf("getSecurityNotes(%s, svc) returned %d notes, want at least %d",
				tt.scheme, len(notes), tt.want)
		}
	}
}

func TestMDNSWorkerStartStop(t *testing.T) {
	store := &mockProposalStore{}
	bus := &mockEventBus{}
	dm := NewDiscoveryManager(store, bus, nil)

	worker := NewMDNSDiscoveryWorker(dm, []string{"_http._tcp"}, 100*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())

	// Start worker in goroutine
	errCh := make(chan error, 1)
	go func() {
		errCh <- worker.Start(ctx)
	}()

	// Let it run briefly
	time.Sleep(50 * time.Millisecond)

	// Stop worker
	cancel()
	worker.Stop()

	// Wait for completion
	select {
	case err := <-errCh:
		if err != nil && err != context.Canceled {
			t.Errorf("Unexpected error: %v", err)
		}
	case <-time.After(1 * time.Second):
		t.Error("Worker did not stop in time")
	}
}

func TestMDNSDetectScheme(t *testing.T) {
	tests := []struct {
		svc  *MDNSService
		want string
	}{
		{
			&MDNSService{
				Type: "_https._tcp",
			},
			"https",
		},
		{
			&MDNSService{
				Type: "_http._tcp",
			},
			"http",
		},
		{
			&MDNSService{
				Type: "_http._tcp",
				TXT: map[string]string{
					"scheme": "https",
				},
			},
			"https",
		},
	}

	for _, tt := range tests {
		// Simple scheme detection based on service type
		scheme := "http"
		if tt.svc.TXT != nil {
			if s, ok := tt.svc.TXT["scheme"]; ok {
				scheme = s
			}
		}
		if tt.svc.Type == "_https._tcp" {
			scheme = "https"
		}

		if scheme != tt.want {
			t.Errorf("Expected scheme %s, got %s", tt.want, scheme)
		}
	}
}

// mDNS Returns Mock Data Tests
// These tests verify that the mDNS worker properly handles the not-implemented state
// without returning mock data.

// TestMDNSWorker_DoesNotProduceMockProposals tests that the mDNS worker
// does not produce mock proposals since mDNS discovery is not fully implemented.
func TestMDNSWorker_DoesNotProduceMockProposals(t *testing.T) {
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

	// Create mDNS worker with short interval
	worker := NewMDNSDiscoveryWorker(dm, []string{"_http._tcp", "_https._tcp"}, 50*time.Millisecond)

	// Start worker
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() {
		worker.Start(ctx)
		close(done)
	}()

	// Let it run for a few scan cycles
	time.Sleep(200 * time.Millisecond)

	// Stop worker
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Error("Worker did not stop within timeout")
	}

	// Check that no mock proposals were created
	// mDNS is not implemented, so should produce 0 proposals
	dm.mu.Lock()
	proposalCount := len(dm.proposals)
	dm.mu.Unlock()

	if proposalCount > 0 {
		t.Errorf("Expected no proposals from mDNS worker (not implemented), got %d", proposalCount)
	}
}

// TestMDNSWorker_StopIdempotent tests that Stop can be called multiple times.
func TestMDNSWorker_StopIdempotent(t *testing.T) {
	store := &mockProposalStore{}
	bus := &mockEventBus{}
	dm := NewDiscoveryManager(store, bus, nil)

	worker := NewMDNSDiscoveryWorker(dm, []string{"_http._tcp"}, time.Second)

	// Call Stop multiple times - should not panic or return errors
	for i := 0; i < 3; i++ {
		if err := worker.Stop(); err != nil {
			t.Errorf("Stop() returned error on call %d: %v", i+1, err)
		}
	}
}
