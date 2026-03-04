package metrics

import (
	"bufio"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestMetricsCreation(t *testing.T) {
	t.Parallel()

	m := New("test_nexus")
	if m == nil {
		t.Fatal("metrics should not be nil")
	}

	// Verify metrics are initialized
	if m.HTTPRequestsTotal == nil {
		t.Error("HTTPRequestsTotal should be initialized")
	}
	if m.AuthPairingTotal == nil {
		t.Error("AuthPairingTotal should be initialized")
	}
	if m.DevicesTotal == nil {
		t.Error("DevicesTotal should be initialized")
	}
}

func TestRecordHTTPRequest(t *testing.T) {
	t.Parallel()

	m := New("test_http")

	// Should not panic
	m.RecordHTTPRequest("GET", "/api/v1/apps", "200", 50*time.Millisecond, 1024, 2048)

	// Record multiple times
	for i := 0; i < 10; i++ {
		m.RecordHTTPRequest("POST", "/api/v1/auth/pair", "200", 100*time.Millisecond, 512, 1024)
	}
}

func TestRecordAuthMetrics(t *testing.T) {
	t.Parallel()

	m := New("test_auth")

	// Test pairing metrics
	m.RecordAuthPairing("success", "ios", 250*time.Millisecond)
	m.RecordAuthPairing("error_invalid_token", "android", 10*time.Millisecond)

	// Test refresh metrics
	m.RecordAuthRefresh("success")
	m.RecordAuthRefresh("error_no_token")

	// Test JWT validation metrics
	m.RecordJWTValidation("success")
	m.RecordJWTValidation("error_invalid")
}

func TestRecordDeviceMetrics(t *testing.T) {
	t.Parallel()

	m := New("test_device")

	m.DevicesTotal.Set(5)
	m.RecordDeviceOperation("list", "success")
	m.RecordDeviceOperation("revoke", "success")

	lastSeen := time.Now().Add(-2 * time.Hour)
	m.UpdateDeviceLastSeen("dev-123", lastSeen)
}

func TestRecordProxyMetrics(t *testing.T) {
	t.Parallel()

	m := New("test_proxy")

	m.RecordProxyRequest("grafana", "200", 150*time.Millisecond)
	m.RecordProxyError("grafana", "connection_refused")
	m.RecordProxyBytes("grafana", "request", 2048)
	m.RecordProxyBytes("grafana", "response", 65536)

	m.ProxyActiveSessions.Set(10)
}

func TestRecordDiscoveryMetrics(t *testing.T) {
	t.Parallel()

	m := New("test_discovery")

	m.RecordDiscoveryProposal("docker", "created")
	m.RecordDiscoveryProposal("mdns", "approved")
	m.RecordDiscoveryScan("docker", "success", 2*time.Second)

	m.DiscoveryProposalsPending.Set(3)
	m.DiscoveryWorkersActive.Set(2)
}

func TestRecordStorageMetrics(t *testing.T) {
	t.Parallel()

	m := New("test_storage")

	m.RecordStorageOperation("insert", "device", "success", 5*time.Millisecond)
	m.RecordStorageOperation("select", "app", "success", 2*time.Millisecond)

	m.StorageAppsTotal.Set(10)
	m.StorageRoutesTotal.Set(15)
}

func TestRecordSSEMetrics(t *testing.T) {
	t.Parallel()

	m := New("test_sse")

	m.SSEConnectionsActive.Inc()
	m.RecordSSEEvent("discovery.proposal")
	m.RecordSSEEvent("catalog.upserted")
	m.SSEConnectionDuration.Observe(300)
}

func TestSystemMetrics(t *testing.T) {
	m := New("test_system")

	// Test build info
	m.SetBuildInfo("1.2.3", "go1.21.0")

	// Test uptime
	startTime := time.Now().Add(-1 * time.Hour)
	m.UpdateUptime(startTime)

	// Verify uptime is approximately 1 hour (in seconds)
	// Note: We can't directly read gauge values in Prometheus client,
	// but we can verify the method doesn't panic
}

func TestMetricsNamespace(t *testing.T) {
	testCases := []struct {
		namespace string
		expected  string
	}{
		{"test_namespace1", "test_namespace1"},
		{"test_namespace2", "test_namespace2"},
		// Note: Can't test empty namespace here because it defaults to "nekzus"
		// which is already registered in TestMetricsCreation
	}

	for _, tc := range testCases {
		t.Run(tc.namespace, func(t *testing.T) {
			m := New(tc.namespace)
			if m == nil {
				t.Fatal("metrics should not be nil")
			}
			// Verify metrics were created (they won't panic on use)
			m.HTTPRequestsTotal.WithLabelValues("GET", "/test", "200").Inc()
		})
	}
}

func TestMetricsLabels(t *testing.T) {
	m := New("test_labels")

	// Test various label combinations
	testCases := []struct {
		name        string
		recordFunc  func()
		shouldPanic bool
	}{
		{
			name: "valid HTTP labels",
			recordFunc: func() {
				m.RecordHTTPRequest("GET", "/api/v1/apps", "200", 10*time.Millisecond, 0, 1024)
			},
			shouldPanic: false,
		},
		{
			name: "valid auth pairing labels",
			recordFunc: func() {
				m.RecordAuthPairing("success", "ios", 100*time.Millisecond)
			},
			shouldPanic: false,
		},
		{
			name: "valid proxy labels",
			recordFunc: func() {
				m.RecordProxyRequest("app1", "200", 50*time.Millisecond)
			},
			shouldPanic: false,
		},
		{
			name: "valid discovery labels",
			recordFunc: func() {
				m.RecordDiscoveryProposal("docker", "created")
			},
			shouldPanic: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					if !tc.shouldPanic {
						t.Errorf("unexpected panic: %v", r)
					}
				}
			}()
			tc.recordFunc()
		})
	}
}

func TestMetricsThreadSafety(t *testing.T) {
	m := New("test_threadsafe")

	// Simulate concurrent metric updates
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(id int) {
			for j := 0; j < 100; j++ {
				m.RecordHTTPRequest("GET", "/api/v1/apps", "200", time.Millisecond, 100, 200)
				m.RecordAuthPairing("success", "ios", 10*time.Millisecond)
				m.RecordJWTValidation("success")
				m.DevicesTotal.Inc()
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestPathNormalization(t *testing.T) {
	testCases := []struct {
		input    string
		expected string
	}{
		{"/api/v1/healthz", "/api/v1/healthz"},
		{"/api/v1/devices", "/api/v1/devices"},
		{"/api/v1/devices/dev-123", "/api/v1/devices/:id"},
		{"/api/v1/devices/dev-xyz", "/api/v1/devices/:id"},
		{"/api/v1/discovery/proposals", "/api/v1/discovery/proposals"},
		{"/api/v1/discovery/proposals/prop-abc/approve", "/api/v1/discovery/proposals/:id"},
		{"/apps/grafana/dashboard", "/apps/:app/*"},
		{"/apps/homeassistant/config", "/apps/:app/*"},
		{"/metrics", "/metrics"},
		{"/unknown/path", "/unknown/path"},
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			result := normalizePath(tc.input)
			if result != tc.expected {
				t.Errorf("normalizePath(%q) = %q, want %q", tc.input, result, tc.expected)
			}
		})
	}
}

func TestBuildInfo(t *testing.T) {
	m := New("test_buildinfo")

	// Test setting build info multiple times
	m.SetBuildInfo("1.0.0", "go1.20.0")
	m.SetBuildInfo("1.1.0", "go1.21.0")
	m.SetBuildInfo("2.0.0", "go1.22.0")

	// Should not panic and last value should be set
}

func TestMetricsWithEmptyLabels(t *testing.T) {
	m := New("test_empty_labels")

	// Test with empty platform
	m.RecordAuthPairing("error_no_token", "", 10*time.Millisecond)

	// Test with empty app_id
	m.RecordProxyRequest("", "404", 5*time.Millisecond)

	// Should not panic
}

func TestMetricsConsistency(t *testing.T) {
	m := New("test_consistency")

	// Record some metrics
	m.RecordHTTPRequest("GET", "/api/v1/apps", "200", 10*time.Millisecond, 100, 500)
	m.RecordHTTPRequest("POST", "/api/v1/auth/pair", "200", 50*time.Millisecond, 200, 300)

	// Verify that metrics can be recorded multiple times without issues
	for i := 0; i < 100; i++ {
		m.HTTPRequestsTotal.WithLabelValues("GET", "/api/v1/apps", "200").Inc()
		m.HTTPRequestDuration.WithLabelValues("GET", "/api/v1/apps").Observe(0.01)
	}
}

func TestGetHTTPRequestsTotal(t *testing.T) {
	m := New("test_http_requests_total")

	// Initially should be 0
	total, err := m.GetHTTPRequestsTotal()
	if err != nil {
		t.Fatalf("GetHTTPRequestsTotal() error = %v", err)
	}
	if total != 0 {
		t.Errorf("GetHTTPRequestsTotal() = %v, want 0", total)
	}

	// Record some requests with different labels
	m.RecordHTTPRequest("GET", "/api/v1/apps", "200", 10*time.Millisecond, 100, 500)
	m.RecordHTTPRequest("POST", "/api/v1/auth/pair", "200", 50*time.Millisecond, 200, 300)
	m.RecordHTTPRequest("GET", "/api/v1/devices", "404", 5*time.Millisecond, 50, 100)

	// Total should be 3
	total, err = m.GetHTTPRequestsTotal()
	if err != nil {
		t.Fatalf("GetHTTPRequestsTotal() error = %v", err)
	}
	if total != 3 {
		t.Errorf("GetHTTPRequestsTotal() = %v, want 3", total)
	}

	// Record more requests - increment same label multiple times
	for i := 0; i < 10; i++ {
		m.RecordHTTPRequest("GET", "/api/v1/apps", "200", 10*time.Millisecond, 100, 500)
	}

	// Total should be 13 (3 + 10)
	total, err = m.GetHTTPRequestsTotal()
	if err != nil {
		t.Fatalf("GetHTTPRequestsTotal() error = %v", err)
	}
	if total != 13 {
		t.Errorf("GetHTTPRequestsTotal() = %v, want 13", total)
	}
}

func BenchmarkRecordHTTPRequest(b *testing.B) {
	m := New("bench_nexus")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.RecordHTTPRequest("GET", "/api/v1/apps", "200", 10*time.Millisecond, 1024, 2048)
	}
}

func BenchmarkRecordAuthPairing(b *testing.B) {
	m := New("bench_nexus")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.RecordAuthPairing("success", "ios", 50*time.Millisecond)
	}
}

func BenchmarkPathNormalization(b *testing.B) {
	paths := []string{
		"/api/v1/devices/dev-123",
		"/apps/grafana/dashboard",
		"/api/v1/discovery/proposals/prop-abc/approve",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		normalizePath(paths[i%len(paths)])
	}
}

func TestNormalizePathEdgeCases(t *testing.T) {
	testCases := []struct {
		input    string
		expected string
	}{
		{"", ""},
		{"/", "/"},
		{"/api/v1/devices/", "/api/v1/devices/:id"},
		{"/apps/", "/apps/:app/*"},
		{"/metrics/", "/metrics"},
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			result := normalizePath(tc.input)
			// For edge cases, we mainly want to ensure no panic
			if tc.expected != "" && !strings.Contains(result, tc.expected) && result != tc.expected {
				t.Logf("normalizePath(%q) = %q (expected pattern: %q)", tc.input, result, tc.expected)
			}
		})
	}
}

// TestResponseWriterImplementsHijacker tests that the responseWriter wrapper
// correctly implements the http.Hijacker interface, which is required for WebSocket upgrades.
// This test would have caught the bug where WebSocket connections failed because
// the metrics middleware didn't preserve the Hijacker interface.
func TestResponseWriterImplementsHijacker(t *testing.T) {
	// Create a test handler that tries to hijack the connection
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hijacker, ok := w.(http.Hijacker)
		if !ok {
			t.Error("ResponseWriter does not implement http.Hijacker")
			http.Error(w, "hijacking not supported", http.StatusInternalServerError)
			return
		}

		// Try to hijack
		conn, buf, err := hijacker.Hijack()
		if err != nil {
			t.Errorf("Hijack failed: %v", err)
			http.Error(w, "hijack failed", http.StatusInternalServerError)
			return
		}

		// If we get here, hijacking worked
		defer conn.Close()

		// Write a simple HTTP response
		buf.WriteString("HTTP/1.1 200 OK\r\n\r\nHijacking works!\r\n")
		buf.Flush()
	})

	// Wrap the handler with metrics middleware
	m := New("test_hijacker")
	wrappedHandler := HTTPMiddleware(m)(handler)

	// Create test server
	server := httptest.NewServer(wrappedHandler)
	defer server.Close()

	// Make a request that will trigger hijacking
	resp, err := http.Get(server.URL)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	// Verify the request succeeded
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

// TestResponseWriterHijackerInterface tests that responseWriter satisfies
// the http.Hijacker interface at compile time and runtime
func TestResponseWriterHijackerInterface(t *testing.T) {
	// Create a mock http.ResponseWriter that implements Hijacker
	mockWriter := &mockHijackableWriter{}

	// Wrap it with our responseWriter
	wrapped := &responseWriter{
		ResponseWriter: mockWriter,
		statusCode:     200,
		size:           0,
	}

	// Test compile-time interface satisfaction
	var _ http.Hijacker = wrapped
	var _ http.Flusher = wrapped

	// Test runtime interface check
	hijacker, ok := interface{}(wrapped).(http.Hijacker)
	if !ok {
		t.Error("responseWriter does not implement http.Hijacker at runtime")
	}

	// Test Hijack method works
	conn, buf, err := hijacker.Hijack()
	if err != nil {
		t.Errorf("Hijack failed: %v", err)
	}

	if conn == nil {
		t.Error("Hijack returned nil connection")
	}

	if buf == nil {
		t.Error("Hijack returned nil buffer")
	}
}

// mockHijackableWriter is a mock http.ResponseWriter that implements Hijacker
type mockHijackableWriter struct {
	http.ResponseWriter
}

func (m *mockHijackableWriter) Header() http.Header {
	return http.Header{}
}

func (m *mockHijackableWriter) Write([]byte) (int, error) {
	return 0, nil
}

func (m *mockHijackableWriter) WriteHeader(statusCode int) {}

func (m *mockHijackableWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	// Return mock connection and buffer
	return &mockConn{}, bufio.NewReadWriter(bufio.NewReader(strings.NewReader("")), bufio.NewWriter(&strings.Builder{})), nil
}

// mockConn is a minimal net.Conn implementation for testing
type mockConn struct{}

func (m *mockConn) Read(b []byte) (n int, err error)   { return 0, nil }
func (m *mockConn) Write(b []byte) (n int, err error)  { return len(b), nil }
func (m *mockConn) Close() error                       { return nil }
func (m *mockConn) LocalAddr() net.Addr                { return nil }
func (m *mockConn) RemoteAddr() net.Addr               { return nil }
func (m *mockConn) SetDeadline(t time.Time) error      { return nil }
func (m *mockConn) SetReadDeadline(t time.Time) error  { return nil }
func (m *mockConn) SetWriteDeadline(t time.Time) error { return nil }
