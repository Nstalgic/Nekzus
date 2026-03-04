package proxy

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/nstalgic/nekzus/internal/certmanager"
)

func TestNewCache(t *testing.T) {
	cache := NewCache()
	if cache == nil {
		t.Fatal("expected non-nil cache")
	}

	if len(cache.proxies) != 0 {
		t.Errorf("expected empty cache, got %d entries", len(cache.proxies))
	}
}

func TestCache_GetOrCreate(t *testing.T) {
	cache := NewCache()

	target, _ := url.Parse("http://localhost:8080")

	// First call should create proxy
	proxy1 := cache.GetOrCreate(target)
	if proxy1 == nil {
		t.Fatal("expected non-nil proxy")
	}

	// Second call should return same proxy
	proxy2 := cache.GetOrCreate(target)
	if proxy1 != proxy2 {
		t.Error("expected same proxy instance from cache")
	}

	stats := cache.Stats()
	if stats["cached_proxies"] != 1 {
		t.Errorf("expected 1 cached proxy, got %d", stats["cached_proxies"])
	}
}

func TestCache_GetOrCreate_DifferentTargets(t *testing.T) {
	cache := NewCache()

	target1, _ := url.Parse("http://localhost:8080")
	target2, _ := url.Parse("http://localhost:9090")

	proxy1 := cache.GetOrCreate(target1)
	proxy2 := cache.GetOrCreate(target2)

	if proxy1 == proxy2 {
		t.Error("expected different proxies for different targets")
	}

	stats := cache.Stats()
	if stats["cached_proxies"] != 2 {
		t.Errorf("expected 2 cached proxies, got %d", stats["cached_proxies"])
	}
}

func TestCache_Clear(t *testing.T) {
	cache := NewCache()

	target1, _ := url.Parse("http://localhost:8080")
	target2, _ := url.Parse("http://localhost:9090")

	cache.GetOrCreate(target1)
	cache.GetOrCreate(target2)

	cache.Clear()

	stats := cache.Stats()
	if stats["cached_proxies"] != 0 {
		t.Errorf("expected 0 cached proxies after clear, got %d", stats["cached_proxies"])
	}
}

func TestCache_Remove(t *testing.T) {
	cache := NewCache()

	target1, _ := url.Parse("http://localhost:8080")
	target2, _ := url.Parse("http://localhost:9090")

	cache.GetOrCreate(target1)
	cache.GetOrCreate(target2)

	cache.Remove(target1.String())

	stats := cache.Stats()
	if stats["cached_proxies"] != 1 {
		t.Errorf("expected 1 cached proxy after remove, got %d", stats["cached_proxies"])
	}
}

func TestSanitizeHeaders(t *testing.T) {
	req := httptest.NewRequest("GET", "http://example.com", nil)
	req.Header.Set("Authorization", "Bearer token123")
	req.Header.Set("Cookie", "session=abc")
	req.Header.Set("X-Api-Key", "secret")
	req.Header.Set("X-Auth-Token", "token")
	req.Header.Set("Content-Type", "application/json")

	SanitizeHeaders(req)

	// Sensitive headers should be removed
	if req.Header.Get("Authorization") != "" {
		t.Error("Authorization header should be removed")
	}
	if req.Header.Get("Cookie") != "" {
		t.Error("Cookie header should be removed")
	}
	if req.Header.Get("X-Api-Key") != "" {
		t.Error("X-Api-Key header should be removed")
	}
	if req.Header.Get("X-Auth-Token") != "" {
		t.Error("X-Auth-Token header should be removed")
	}

	// Non-sensitive headers should remain
	if req.Header.Get("Content-Type") != "application/json" {
		t.Error("Content-Type header should be preserved")
	}
}

func TestSetForwardedHeaders(t *testing.T) {
	req := httptest.NewRequest("GET", "http://example.com", nil)
	req.Host = "original-host.com"
	clientIP := "192.168.1.100"

	SetForwardedHeaders(req, clientIP)

	if req.Header.Get("X-Forwarded-For") != clientIP {
		t.Errorf("X-Forwarded-For = %q, want %q", req.Header.Get("X-Forwarded-For"), clientIP)
	}

	if req.Header.Get("X-Forwarded-Host") != "original-host.com" {
		t.Errorf("X-Forwarded-Host = %q, want %q", req.Header.Get("X-Forwarded-Host"), "original-host.com")
	}

	if req.Header.Get("X-Forwarded-Proto") != "http" {
		t.Errorf("X-Forwarded-Proto = %q, want %q", req.Header.Get("X-Forwarded-Proto"), "http")
	}
}

// TestCache_ConcurrentGetOrCreate tests concurrent GetOrCreate calls
func TestCache_ConcurrentGetOrCreate(t *testing.T) {
	cache := NewCache()

	// Create a test URL
	targetURL, _ := url.Parse("http://localhost:8080")

	// Track proxy instances
	proxies := make(chan *httputil.ReverseProxy, 100)
	done := make(chan bool)

	// Launch 100 goroutines requesting the same proxy
	for i := 0; i < 100; i++ {
		go func() {
			proxy := cache.GetOrCreate(targetURL)
			proxies <- proxy
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 100; i++ {
		<-done
	}
	close(proxies)

	// Verify all proxies are the same instance (cached)
	var firstProxy *httputil.ReverseProxy
	for proxy := range proxies {
		if firstProxy == nil {
			firstProxy = proxy
		} else if proxy != firstProxy {
			t.Error("GetOrCreate returned different proxy instances for same URL")
		}
	}

	// Verify only one proxy was created
	stats := cache.Stats()
	if stats["cached_proxies"] != 1 {
		t.Errorf("Expected 1 cached proxy, got %d", stats["cached_proxies"])
	}
}

// TestCache_ConcurrentDifferentTargets tests concurrent GetOrCreate for different URLs
func TestCache_ConcurrentDifferentTargets(t *testing.T) {
	cache := NewCache()

	done := make(chan bool)
	numTargets := 50

	// Launch goroutines for different targets
	for i := 0; i < numTargets; i++ {
		go func(port int) {
			targetURL, _ := url.Parse(fmt.Sprintf("http://localhost:%d", 8080+port))
			cache.GetOrCreate(targetURL)
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < numTargets; i++ {
		<-done
	}

	// Verify all proxies were created
	stats := cache.Stats()
	if stats["cached_proxies"] != numTargets {
		t.Errorf("Expected %d cached proxies, got %d", numTargets, stats["cached_proxies"])
	}
}

// TestCache_ConcurrentStats tests concurrent Stats calls
func TestCache_ConcurrentStats(t *testing.T) {
	cache := NewCache()

	// Pre-populate with some proxies
	for i := 0; i < 10; i++ {
		targetURL, _ := url.Parse(fmt.Sprintf("http://localhost:%d", 8080+i))
		cache.GetOrCreate(targetURL)
	}

	done := make(chan bool)
	statsResults := make(chan map[string]int, 50)

	// Launch 50 goroutines calling Stats concurrently
	for i := 0; i < 50; i++ {
		go func() {
			stats := cache.Stats()
			statsResults <- stats
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 50; i++ {
		<-done
	}
	close(statsResults)

	// Verify all Stats calls returned valid data
	for stats := range statsResults {
		if stats["cached_proxies"] != 10 {
			t.Errorf("Expected 10 cached_proxies, got %d", stats["cached_proxies"])
		}
	}
}

// TestCache_ConcurrentClear tests Clear during concurrent GetOrCreate
func TestCache_ConcurrentClear(t *testing.T) {
	cache := NewCache()

	done := make(chan bool)

	// Goroutine 1: Continuously create proxies
	go func() {
		for i := 0; i < 100; i++ {
			targetURL, _ := url.Parse(fmt.Sprintf("http://localhost:%d", 8080+(i%10)))
			cache.GetOrCreate(targetURL)
		}
		done <- true
	}()

	// Goroutine 2: Clear cache halfway through
	go func() {
		for i := 0; i < 5; i++ {
			cache.Clear()
		}
		done <- true
	}()

	// Goroutine 3: Call Stats
	go func() {
		for i := 0; i < 50; i++ {
			cache.Stats()
		}
		done <- true
	}()

	// Wait for all goroutines
	for i := 0; i < 3; i++ {
		<-done
	}

	// Cache should still be functional
	stats := cache.Stats()
	if stats["cached_proxies"] < 0 {
		t.Error("Invalid stats after concurrent operations")
	}
}

// TestCache_ConcurrentRemove tests Remove during concurrent GetOrCreate
func TestCache_ConcurrentRemove(t *testing.T) {
	cache := NewCache()

	done := make(chan bool)
	targetURL1, _ := url.Parse("http://localhost:8080")
	targetURL2, _ := url.Parse("http://localhost:9090")

	// Goroutine 1: Repeatedly create proxy for target1
	go func() {
		for i := 0; i < 100; i++ {
			cache.GetOrCreate(targetURL1)
		}
		done <- true
	}()

	// Goroutine 2: Repeatedly create proxy for target2
	go func() {
		for i := 0; i < 100; i++ {
			cache.GetOrCreate(targetURL2)
		}
		done <- true
	}()

	// Goroutine 3: Repeatedly remove target1
	go func() {
		for i := 0; i < 50; i++ {
			cache.Remove(targetURL1.String())
		}
		done <- true
	}()

	// Goroutine 4: Call Stats
	go func() {
		for i := 0; i < 50; i++ {
			cache.Stats()
		}
		done <- true
	}()

	// Wait for all goroutines
	for i := 0; i < 4; i++ {
		<-done
	}

	// Cache should still be functional
	stats := cache.Stats()
	if stats["cached_proxies"] < 0 || stats["cached_proxies"] > 2 {
		t.Errorf("Expected 0-2 cached proxies, got %d", stats["cached_proxies"])
	}
}

// TestCache_GetOrCreate_HasTransport tests that proxy has properly configured transport
func TestCache_GetOrCreate_HasTransport(t *testing.T) {
	cache := NewCache()
	target, _ := url.Parse("http://localhost:8080")

	proxy := cache.GetOrCreate(target)
	if proxy == nil {
		t.Fatal("expected non-nil proxy")
	}

	// Check that Transport is configured
	if proxy.Transport == nil {
		t.Fatal("expected proxy to have Transport configured")
	}

	transport, ok := proxy.Transport.(*http.Transport)
	if !ok {
		t.Fatal("expected Transport to be *http.Transport")
	}

	// Check timeouts are configured
	if transport.IdleConnTimeout != 90*time.Second {
		t.Errorf("IdleConnTimeout = %v, want 90s", transport.IdleConnTimeout)
	}

	if transport.ExpectContinueTimeout != 1*time.Second {
		t.Errorf("ExpectContinueTimeout = %v, want 1s", transport.ExpectContinueTimeout)
	}

	if transport.ResponseHeaderTimeout != 30*time.Second {
		t.Errorf("ResponseHeaderTimeout = %v, want 30s", transport.ResponseHeaderTimeout)
	}
}

// TestCache_GetOrCreate_TLSMinVersion tests that TLS MinVersion is set
func TestCache_GetOrCreate_TLSMinVersion(t *testing.T) {
	cache := NewCache()
	target, _ := url.Parse("https://localhost:8443")

	proxy := cache.GetOrCreate(target)
	if proxy.Transport == nil {
		t.Fatal("expected proxy to have Transport configured")
	}

	transport, ok := proxy.Transport.(*http.Transport)
	if !ok {
		t.Fatal("expected Transport to be *http.Transport")
	}

	if transport.TLSClientConfig == nil {
		t.Fatal("expected TLSClientConfig to be configured")
	}

	if transport.TLSClientConfig.MinVersion != tls.VersionTLS12 {
		t.Errorf("TLS MinVersion = %v, want TLS 1.2 (%v)", transport.TLSClientConfig.MinVersion, tls.VersionTLS12)
	}
}

// TestCache_GetOrCreate_ConnectionPoolLimits tests connection pool limits
func TestCache_GetOrCreate_ConnectionPoolLimits(t *testing.T) {
	cache := NewCache()
	target, _ := url.Parse("http://localhost:8080")

	proxy := cache.GetOrCreate(target)
	if proxy.Transport == nil {
		t.Fatal("expected proxy to have Transport configured")
	}

	transport, ok := proxy.Transport.(*http.Transport)
	if !ok {
		t.Fatal("expected Transport to be *http.Transport")
	}

	if transport.MaxIdleConns != 100 {
		t.Errorf("MaxIdleConns = %d, want 100", transport.MaxIdleConns)
	}

	if transport.MaxIdleConnsPerHost != 10 {
		t.Errorf("MaxIdleConnsPerHost = %d, want 10", transport.MaxIdleConnsPerHost)
	}

	if transport.MaxConnsPerHost != 100 {
		t.Errorf("MaxConnsPerHost = %d, want 100", transport.MaxConnsPerHost)
	}
}

// TestCache_GetOrCreate_FlushInterval tests that FlushInterval is set for SSE
func TestCache_GetOrCreate_FlushInterval(t *testing.T) {
	cache := NewCache()
	target, _ := url.Parse("http://localhost:8080")

	proxy := cache.GetOrCreate(target)

	// FlushInterval should be -1 for immediate flushing (SSE support)
	if proxy.FlushInterval != -1 {
		t.Errorf("FlushInterval = %v, want -1 for immediate SSE flushing", proxy.FlushInterval)
	}
}

// TestRemoveHopByHopHeaders tests removal of hop-by-hop headers
func TestRemoveHopByHopHeaders(t *testing.T) {
	req := httptest.NewRequest("GET", "http://example.com", nil)

	// Set hop-by-hop headers that should be removed
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Keep-Alive", "timeout=5")
	req.Header.Set("Proxy-Authenticate", "Basic")
	req.Header.Set("Proxy-Authorization", "Basic xyz")
	req.Header.Set("Proxy-Connection", "keep-alive")
	req.Header.Set("Te", "trailers")
	req.Header.Set("Trailer", "X-Checksum")
	req.Header.Set("Transfer-Encoding", "chunked")
	req.Header.Set("Upgrade", "websocket")

	// Set end-to-end headers that should be preserved
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Custom-Header", "value")
	req.Header.Set("Accept", "application/json")

	RemoveHopByHopHeaders(req.Header)

	// Hop-by-hop headers should be removed
	hopByHop := []string{
		"Connection", "Keep-Alive", "Proxy-Authenticate",
		"Proxy-Authorization", "Proxy-Connection", "Te",
		"Trailer", "Transfer-Encoding", "Upgrade",
	}
	for _, h := range hopByHop {
		if req.Header.Get(h) != "" {
			t.Errorf("hop-by-hop header %q should be removed", h)
		}
	}

	// End-to-end headers should be preserved
	if req.Header.Get("Content-Type") != "application/json" {
		t.Error("Content-Type header should be preserved")
	}
	if req.Header.Get("X-Custom-Header") != "value" {
		t.Error("X-Custom-Header should be preserved")
	}
	if req.Header.Get("Accept") != "application/json" {
		t.Error("Accept header should be preserved")
	}
}

// TestRemoveHopByHopHeaders_ConnectionHeader tests removal of headers listed in Connection header
func TestRemoveHopByHopHeaders_ConnectionHeader(t *testing.T) {
	req := httptest.NewRequest("GET", "http://example.com", nil)

	// Connection header lists additional hop-by-hop headers
	req.Header.Set("Connection", "keep-alive, X-Custom-Hop")
	req.Header.Set("Keep-Alive", "timeout=5")
	req.Header.Set("X-Custom-Hop", "should-be-removed")
	req.Header.Set("X-Other", "should-stay")

	RemoveHopByHopHeaders(req.Header)

	// Headers listed in Connection should be removed
	if req.Header.Get("X-Custom-Hop") != "" {
		t.Error("X-Custom-Hop header listed in Connection should be removed")
	}
	if req.Header.Get("Keep-Alive") != "" {
		t.Error("Keep-Alive header should be removed")
	}

	// Other headers should stay
	if req.Header.Get("X-Other") != "should-stay" {
		t.Error("X-Other header should be preserved")
	}
}

// TestSetForwardedHeaders_WithPort tests X-Forwarded-Port header
func TestSetForwardedHeaders_WithPort(t *testing.T) {
	testCases := []struct {
		name         string
		host         string
		expectedPort string
	}{
		{
			name:         "explicit port 8080",
			host:         "example.com:8080",
			expectedPort: "8080",
		},
		{
			name:         "explicit port 443",
			host:         "example.com:443",
			expectedPort: "443",
		},
		{
			name:         "explicit port 80",
			host:         "example.com:80",
			expectedPort: "80",
		},
		{
			name:         "no port defaults to 80 for http",
			host:         "example.com",
			expectedPort: "80",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "http://example.com", nil)
			req.Host = tc.host

			SetForwardedHeaders(req, "192.168.1.100")

			port := req.Header.Get("X-Forwarded-Port")
			if port != tc.expectedPort {
				t.Errorf("X-Forwarded-Port = %q, want %q", port, tc.expectedPort)
			}
		})
	}
}

// TestSetForwardedHeaders_HTTPSPort tests X-Forwarded-Port for HTTPS requests
func TestSetForwardedHeaders_HTTPSPort(t *testing.T) {
	req := httptest.NewRequest("GET", "https://example.com", nil)
	req.Host = "example.com"
	// Simulate TLS connection
	req.TLS = &tls.ConnectionState{}

	SetForwardedHeaders(req, "192.168.1.100")

	// For HTTPS without explicit port, should be 443
	port := req.Header.Get("X-Forwarded-Port")
	if port != "443" {
		t.Errorf("X-Forwarded-Port = %q, want 443", port)
	}

	// Also verify proto is https
	proto := req.Header.Get("X-Forwarded-Proto")
	if proto != "https" {
		t.Errorf("X-Forwarded-Proto = %q, want https", proto)
	}
}

// TestFormatProxyError tests error formatting with request ID
func TestFormatProxyError(t *testing.T) {
	testCases := []struct {
		name      string
		requestID string
		host      string
		err       error
		wantID    bool
	}{
		{
			name:      "with request ID",
			requestID: "req-123",
			host:      "localhost:8080",
			err:       fmt.Errorf("connection refused"),
			wantID:    true,
		},
		{
			name:      "without request ID",
			requestID: "",
			host:      "localhost:8080",
			err:       fmt.Errorf("timeout"),
			wantID:    false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			msg := FormatProxyError(tc.requestID, tc.host, tc.err)

			if tc.wantID && !strings.Contains(msg, tc.requestID) {
				t.Errorf("FormatProxyError should include request ID %q, got: %s", tc.requestID, msg)
			}
			if !strings.Contains(msg, tc.host) {
				t.Errorf("FormatProxyError should include host %q, got: %s", tc.host, msg)
			}
		})
	}
}

// TestNewCacheWithMaxSize tests creating cache with LRU eviction
func TestNewCacheWithMaxSize(t *testing.T) {
	cache := NewCacheWithMaxSize(5)
	if cache == nil {
		t.Fatal("expected non-nil cache")
	}

	if cache.maxSize != 5 {
		t.Errorf("maxSize = %d, want 5", cache.maxSize)
	}

	if len(cache.proxies) != 0 {
		t.Errorf("expected empty cache, got %d entries", len(cache.proxies))
	}
}

// TestCache_LRUEviction tests LRU eviction when cache is at capacity
func TestCache_LRUEviction(t *testing.T) {
	cache := NewCacheWithMaxSize(3)

	// Create 3 proxies (fill cache)
	target1, _ := url.Parse("http://localhost:8001")
	target2, _ := url.Parse("http://localhost:8002")
	target3, _ := url.Parse("http://localhost:8003")

	cache.GetOrCreate(target1)
	cache.GetOrCreate(target2)
	cache.GetOrCreate(target3)

	stats := cache.Stats()
	if stats["cached_proxies"] != 3 {
		t.Errorf("expected 3 cached proxies, got %d", stats["cached_proxies"])
	}

	// Access target1 to make it recently used
	cache.GetOrCreate(target1)

	// Add 4th proxy - should evict target2 (oldest)
	target4, _ := url.Parse("http://localhost:8004")
	cache.GetOrCreate(target4)

	stats = cache.Stats()
	if stats["cached_proxies"] != 3 {
		t.Errorf("expected 3 cached proxies after eviction, got %d", stats["cached_proxies"])
	}

	// target1, target3, target4 should be present, target2 should be evicted
	// Getting target2 should create a new proxy
	proxy2Before := cache.GetOrCreate(target2)

	// Add another to cause eviction
	target5, _ := url.Parse("http://localhost:8005")
	cache.GetOrCreate(target5)

	// target3 should be evicted (oldest not recently accessed)
	// Verify we still have 3 proxies
	stats = cache.Stats()
	if stats["cached_proxies"] != 3 {
		t.Errorf("expected 3 cached proxies, got %d", stats["cached_proxies"])
	}

	_ = proxy2Before // prevent unused variable warning
}

// TestCache_LRUEviction_AccessOrder tests that access order is properly tracked
func TestCache_LRUEviction_AccessOrder(t *testing.T) {
	cache := NewCacheWithMaxSize(2)

	target1, _ := url.Parse("http://localhost:8001")
	target2, _ := url.Parse("http://localhost:8002")
	target3, _ := url.Parse("http://localhost:8003")

	// Add target1 and target2
	proxy1 := cache.GetOrCreate(target1)
	cache.GetOrCreate(target2)

	// Access target1 again to make it most recently used
	cache.GetOrCreate(target1)

	// Add target3 - should evict target2 (least recently used)
	cache.GetOrCreate(target3)

	// target1 should still return the same proxy
	proxy1After := cache.GetOrCreate(target1)
	if proxy1 != proxy1After {
		t.Error("target1 proxy should not have been evicted")
	}

	stats := cache.Stats()
	if stats["cached_proxies"] != 2 {
		t.Errorf("expected 2 cached proxies, got %d", stats["cached_proxies"])
	}
}

// TestCache_Stats_WithEvictions tests Stats returns eviction count
func TestCache_Stats_WithEvictions(t *testing.T) {
	cache := NewCacheWithMaxSize(2)

	target1, _ := url.Parse("http://localhost:8001")
	target2, _ := url.Parse("http://localhost:8002")
	target3, _ := url.Parse("http://localhost:8003")

	cache.GetOrCreate(target1)
	cache.GetOrCreate(target2)
	cache.GetOrCreate(target3) // Should evict target1

	stats := cache.Stats()
	if stats["evictions"] != 1 {
		t.Errorf("expected 1 eviction, got %d", stats["evictions"])
	}
}

// BenchmarkCache_LRUUpdateAccessOrder benchmarks the updateAccessOrder operation
// This should be O(1) with the new container/list implementation
func BenchmarkCache_LRUUpdateAccessOrder(b *testing.B) {
	cache := NewCacheWithMaxSize(1000)

	// Pre-populate cache with 100 entries
	for i := 0; i < 100; i++ {
		target, _ := url.Parse(fmt.Sprintf("http://localhost:%d", 8000+i))
		cache.GetOrCreate(target)
	}

	// Benchmark accessing cached entries (which triggers updateAccessOrder)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		target, _ := url.Parse(fmt.Sprintf("http://localhost:%d", 8000+(i%100)))
		cache.GetOrCreate(target)
	}
}

// BenchmarkCache_LRUEviction benchmarks the eviction process
func BenchmarkCache_LRUEviction(b *testing.B) {
	cache := NewCacheWithMaxSize(100)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		target, _ := url.Parse(fmt.Sprintf("http://localhost:%d", 8000+i))
		cache.GetOrCreate(target)
	}
}

// TestCacheWithCA_AddCA tests adding a custom CA to the cache
func TestCacheWithCA_AddCA(t *testing.T) {
	cache := NewCacheWithCA()

	// Generate a real CA certificate using certmanager
	provider := certmanager.NewSelfSignedProvider()
	ca, err := provider.GenerateCA(certmanager.CAOptions{
		CommonName: "Test CA",
	})
	if err != nil {
		t.Fatalf("Failed to generate test CA: %v", err)
	}

	err = cache.AddCA(ca.PEM.Certificate)
	if err != nil {
		t.Fatalf("Failed to add CA: %v", err)
	}

	if cache.RootCAs() == nil {
		t.Error("Expected RootCAs to be set after AddCA")
	}
}

// TestCacheWithCA_InvalidCA tests adding an invalid CA certificate
func TestCacheWithCA_InvalidCA(t *testing.T) {
	cache := NewCacheWithCA()

	err := cache.AddCA([]byte("not a certificate"))
	if err == nil {
		t.Error("Expected error for invalid certificate")
	}
}

// TestCacheWithCA_EmptyCA tests adding an empty CA certificate
func TestCacheWithCA_EmptyCA(t *testing.T) {
	cache := NewCacheWithCA()

	err := cache.AddCA([]byte{})
	if err == nil {
		t.Error("Expected error for empty certificate")
	}
}

// TestCacheWithCA_GetOrCreate_UsesCA tests that proxies created by CacheWithCA use custom RootCAs
func TestCacheWithCA_GetOrCreate_UsesCA(t *testing.T) {
	cache := NewCacheWithCA()

	// Generate a real CA certificate
	provider := certmanager.NewSelfSignedProvider()
	ca, err := provider.GenerateCA(certmanager.CAOptions{
		CommonName: "Test CA",
	})
	if err != nil {
		t.Fatalf("Failed to generate test CA: %v", err)
	}

	err = cache.AddCA(ca.PEM.Certificate)
	if err != nil {
		t.Fatalf("Failed to add CA: %v", err)
	}

	target, _ := url.Parse("https://backend.local:8443")
	proxy := cache.GetOrCreate(target)

	transport, ok := proxy.Transport.(*http.Transport)
	if !ok {
		t.Fatal("expected Transport to be *http.Transport")
	}

	if transport.TLSClientConfig == nil {
		t.Fatal("expected TLSClientConfig to be configured")
	}

	// Verify TLS settings
	if transport.TLSClientConfig.MinVersion != tls.VersionTLS12 {
		t.Errorf("TLS MinVersion = %v, want TLS 1.2", transport.TLSClientConfig.MinVersion)
	}

	// Verify RootCAs is set
	if transport.TLSClientConfig.RootCAs == nil {
		t.Error("Expected RootCAs to be configured in proxy transport")
	}
}

// TestCacheWithCA_MultipleCA tests adding multiple CA certificates
func TestCacheWithCA_MultipleCA(t *testing.T) {
	cache := NewCacheWithCA()

	// Generate two different CAs
	provider := certmanager.NewSelfSignedProvider()

	ca1, err := provider.GenerateCA(certmanager.CAOptions{
		CommonName: "Test CA 1",
	})
	if err != nil {
		t.Fatalf("Failed to generate CA 1: %v", err)
	}

	ca2, err := provider.GenerateCA(certmanager.CAOptions{
		CommonName: "Test CA 2",
	})
	if err != nil {
		t.Fatalf("Failed to generate CA 2: %v", err)
	}

	// Add both CAs
	if err := cache.AddCA(ca1.PEM.Certificate); err != nil {
		t.Fatalf("Failed to add CA 1: %v", err)
	}
	if err := cache.AddCA(ca2.PEM.Certificate); err != nil {
		t.Fatalf("Failed to add CA 2: %v", err)
	}

	// Verify the pool contains our CAs
	if cache.RootCAs() == nil {
		t.Error("Expected RootCAs to be set")
	}
}
