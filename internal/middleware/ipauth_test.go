package middleware

import (
	"bufio"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/nstalgic/nekzus/internal/auth"
	"github.com/nstalgic/nekzus/internal/metrics"
	"github.com/nstalgic/nekzus/internal/storage"
)

const testJWTSecret = "my-very-long-and-strong-jwt-key-for-automated-validation-only-12345"

var (
	testMetrics *metrics.Metrics
	testAuth    *auth.Manager
)

func init() {
	// Create shared metrics instance for all tests
	testMetrics = metrics.New("test_middleware")

	// Create shared auth manager for all tests
	var err error
	testAuth, err = auth.NewManager([]byte(testJWTSecret), "nekzus", "nekzus-mobile", nil)
	if err != nil {
		panic("Failed to create test auth manager: " + err.Error())
	}
}

func TestIsLocalRequest(t *testing.T) {
	tests := []struct {
		name       string
		remoteAddr string
		want       bool
	}{
		// Localhost IPv4
		{
			name:       "localhost IPv4",
			remoteAddr: "127.0.0.1:12345",
			want:       true,
		},
		{
			name:       "localhost IPv4 without port",
			remoteAddr: "127.0.0.1",
			want:       true,
		},
		{
			name:       "localhost range 127.x.x.x",
			remoteAddr: "127.0.0.5:8080",
			want:       true,
		},
		// Localhost IPv6
		{
			name:       "localhost IPv6",
			remoteAddr: "[::1]:12345",
			want:       true,
		},
		{
			name:       "localhost IPv6 without port",
			remoteAddr: "::1",
			want:       true,
		},
		// Private IPv4 ranges
		{
			name:       "private 10.x.x.x range",
			remoteAddr: "10.0.0.1:12345",
			want:       true,
		},
		{
			name:       "private 10.x.x.x range (high)",
			remoteAddr: "10.255.255.254:12345",
			want:       true,
		},
		{
			name:       "private 172.16.x.x range",
			remoteAddr: "172.16.0.1:12345",
			want:       true,
		},
		{
			name:       "private 172.31.x.x range (high)",
			remoteAddr: "172.31.255.254:12345",
			want:       true,
		},
		{
			name:       "private 192.168.x.x range",
			remoteAddr: "192.168.1.1:12345",
			want:       true,
		},
		{
			name:       "private 192.168.x.x range (high)",
			remoteAddr: "192.168.255.254:12345",
			want:       true,
		},
		// Private IPv6 ranges
		{
			name:       "private IPv6 fc00::/7",
			remoteAddr: "[fc00::1]:12345",
			want:       true,
		},
		{
			name:       "private IPv6 fd00::/8",
			remoteAddr: "[fd12:3456:789a::1]:12345",
			want:       true,
		},
		// Public/External IPs
		{
			name:       "public IPv4",
			remoteAddr: "8.8.8.8:12345",
			want:       false,
		},
		{
			name:       "public IPv4 (Cloudflare DNS)",
			remoteAddr: "1.1.1.1:12345",
			want:       false,
		},
		{
			name:       "public IPv6",
			remoteAddr: "[2001:4860:4860::8888]:12345",
			want:       false,
		},
		// Edge cases
		{
			name:       "not private 172.15.x.x (before range)",
			remoteAddr: "172.15.255.254:12345",
			want:       false,
		},
		{
			name:       "not private 172.32.x.x (after range)",
			remoteAddr: "172.32.0.1:12345",
			want:       false,
		},
		{
			name:       "not private 11.x.x.x",
			remoteAddr: "11.0.0.1:12345",
			want:       false,
		},
		{
			name:       "not private 193.x.x.x",
			remoteAddr: "193.0.0.1:12345",
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/test", nil)
			req.RemoteAddr = tt.remoteAddr

			got := isLocalRequest(req)
			if got != tt.want {
				t.Errorf("isLocalRequest(%s) = %v, want %v", tt.remoteAddr, got, tt.want)
			}
		})
	}
}

func TestIPBasedAuth_LocalRequestNoJWT(t *testing.T) {
	middleware := NewIPBasedAuth(testAuth, nil, testMetrics)

	// Create handler that will only be called if auth passes
	nextCalled := false
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	})

	// Test: Local request without JWT should pass
	req := httptest.NewRequest("GET", "/api/v1/apps", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()

	middleware(nextHandler).ServeHTTP(w, req)

	if !nextCalled {
		t.Error("Expected next handler to be called for local request without JWT")
	}
	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

func TestIPBasedAuth_LocalRequestWithValidJWT(t *testing.T) {
	middleware := NewIPBasedAuth(testAuth, nil, testMetrics)

	// Create valid JWT
	token, err := testAuth.SignJWT("device-123", []string{"read:catalog"}, 3600*time.Second)
	if err != nil {
		t.Fatalf("Failed to generate JWT: %v", err)
	}

	// Create handler
	nextCalled := false
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	})

	// Test: Local request with valid JWT should also pass
	req := httptest.NewRequest("GET", "/api/v1/apps", nil)
	req.RemoteAddr = "192.168.1.100:12345"
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	middleware(nextHandler).ServeHTTP(w, req)

	if !nextCalled {
		t.Error("Expected next handler to be called for local request with valid JWT")
	}
	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

func TestIPBasedAuth_ExternalRequestNoJWT(t *testing.T) {
	middleware := NewIPBasedAuth(testAuth, nil, testMetrics)

	// Create handler
	nextCalled := false
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	})

	// Test: External request without JWT should be rejected
	req := httptest.NewRequest("GET", "/api/v1/apps", nil)
	req.RemoteAddr = "8.8.8.8:12345"
	w := httptest.NewRecorder()

	middleware(nextHandler).ServeHTTP(w, req)

	if nextCalled {
		t.Error("Expected next handler NOT to be called for external request without JWT")
	}
	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401, got %d", w.Code)
	}
}

func TestIPBasedAuth_ExternalRequestWithValidJWT(t *testing.T) {
	middleware := NewIPBasedAuth(testAuth, nil, testMetrics)

	// Create valid JWT
	token, err := testAuth.SignJWT("device-456", []string{"read:catalog"}, 3600*time.Second)
	if err != nil {
		t.Fatalf("Failed to generate JWT: %v", err)
	}

	// Create handler
	nextCalled := false
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	})

	// Test: External request with valid JWT should pass
	req := httptest.NewRequest("GET", "/api/v1/apps", nil)
	req.RemoteAddr = "8.8.8.8:12345"
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	middleware(nextHandler).ServeHTTP(w, req)

	if !nextCalled {
		t.Error("Expected next handler to be called for external request with valid JWT")
	}
	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

func TestIPBasedAuth_ExternalRequestWithInvalidJWT(t *testing.T) {
	middleware := NewIPBasedAuth(testAuth, nil, testMetrics)

	// Create handler
	nextCalled := false
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	})

	// Test: External request with invalid JWT should be rejected
	req := httptest.NewRequest("GET", "/api/v1/apps", nil)
	req.RemoteAddr = "8.8.8.8:12345"
	req.Header.Set("Authorization", "Bearer invalid-token-12345")
	w := httptest.NewRecorder()

	middleware(nextHandler).ServeHTTP(w, req)

	if nextCalled {
		t.Error("Expected next handler NOT to be called for external request with invalid JWT")
	}
	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401, got %d", w.Code)
	}
}

func TestIPBasedAuth_WithStorage(t *testing.T) {
	// Create temporary storage
	store, err := storage.NewStore(storage.Config{DatabasePath: ":memory:"})
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	// Create a test device
	deviceID := "device-789"
	err = store.SaveDevice(deviceID, "test-device", "", "", []string{"read:catalog"})
	if err != nil {
		t.Fatalf("Failed to create device: %v", err)
	}

	middleware := NewIPBasedAuth(testAuth, store, testMetrics)

	// Create valid JWT for the device
	token, err := testAuth.SignJWT(deviceID, []string{"read:catalog"}, 3600*time.Second)
	if err != nil {
		t.Fatalf("Failed to generate JWT: %v", err)
	}

	// Create handler
	nextCalled := false
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	})

	// Test: External request with valid JWT should update device last_seen
	req := httptest.NewRequest("GET", "/api/v1/apps", nil)
	req.RemoteAddr = "8.8.8.8:12345"
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	middleware(nextHandler).ServeHTTP(w, req)

	if !nextCalled {
		t.Error("Expected next handler to be called")
	}
	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	// Note: Checking last_seen is tricky because it's updated asynchronously
	// We'd need to add a small sleep or synchronization mechanism to test this properly
	// For now, we just verify the request succeeded
}

func TestIPBasedAuth_PrivateIPRanges(t *testing.T) {
	middleware := NewIPBasedAuth(testAuth, nil, testMetrics)

	// Test all major private IP ranges
	privateIPs := []string{
		"10.0.0.1:12345",
		"10.255.255.254:12345",
		"172.16.0.1:12345",
		"172.31.255.254:12345",
		"192.168.0.1:12345",
		"192.168.255.254:12345",
		"127.0.0.1:12345",
		"[::1]:12345",
		"[fc00::1]:12345",
		"[fd12:3456:789a::1]:12345",
	}

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	for _, ip := range privateIPs {
		t.Run("private_"+ip, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/v1/apps", nil)
			req.RemoteAddr = ip
			w := httptest.NewRecorder()

			middleware(nextHandler).ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("Expected private IP %s to be allowed without JWT, got status %d", ip, w.Code)
			}
		})
	}
}

func TestIPBasedAuth_MetricsRecording(t *testing.T) {
	middleware := NewIPBasedAuth(testAuth, nil, testMetrics)

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Test local request (should record local auth)
	req := httptest.NewRequest("GET", "/api/v1/apps", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	middleware(nextHandler).ServeHTTP(w, req)

	// Test external request with JWT (should record JWT validation)
	token, _ := testAuth.SignJWT("device-999", []string{"read:catalog"}, 3600*time.Second)
	req = httptest.NewRequest("GET", "/api/v1/apps", nil)
	req.RemoteAddr = "8.8.8.8:12345"
	req.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()
	middleware(nextHandler).ServeHTTP(w, req)

	// Test external request without JWT (should record error)
	req = httptest.NewRequest("GET", "/api/v1/apps", nil)
	req.RemoteAddr = "8.8.8.8:12345"
	w = httptest.NewRecorder()
	middleware(nextHandler).ServeHTTP(w, req)

	// Note: We're not asserting specific metric values here because that would
	// require exposing internal metrics state. In a real test, you might want to
	// use a mock metrics recorder or check Prometheus metrics endpoint.
}

// TestHijackableResponseWriterImplementsHijacker tests that the hijackableResponseWriter
// correctly implements the http.Hijacker interface, which is required for WebSocket upgrades.
// This test would have caught the bug where WebSocket connections failed because
// the IP-auth middleware didn't preserve the Hijacker interface.
func TestHijackableResponseWriterImplementsHijacker(t *testing.T) {
	middleware := NewIPBasedAuth(testAuth, nil, testMetrics)

	// Create a handler that tries to hijack the connection (like WebSocket upgrade)
	hijackAttempted := false
	hijackSucceeded := false

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hijackAttempted = true

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

		hijackSucceeded = true
		defer conn.Close()

		// Write a simple HTTP response
		buf.WriteString("HTTP/1.1 200 OK\r\n\r\nHijacking works!\r\n")
		buf.Flush()
	})

	// Wrap the handler with IP-based auth middleware
	wrappedHandler := middleware(handler)

	// Create test server
	server := httptest.NewServer(wrappedHandler)
	defer server.Close()

	// Make a request (local IP, so no auth needed)
	resp, err := http.Get(server.URL)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if !hijackAttempted {
		t.Error("Hijack was not attempted")
	}

	if !hijackSucceeded {
		t.Error("Hijack did not succeed")
	}

	// Verify the request succeeded
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

// TestHijackableResponseWriterInterface tests that hijackableResponseWriter
// satisfies the http.Hijacker interface at compile time and runtime
func TestHijackableResponseWriterInterface(t *testing.T) {
	// Create a mock http.ResponseWriter that implements Hijacker
	mockWriter := &mockHijackableWriter{}

	// Wrap it with hijackableResponseWriter
	wrapped := &hijackableResponseWriter{
		ResponseWriter: mockWriter,
	}

	// Test compile-time interface satisfaction
	var _ http.Hijacker = wrapped

	// Test runtime interface check
	hijacker, ok := interface{}(wrapped).(http.Hijacker)
	if !ok {
		t.Error("hijackableResponseWriter does not implement http.Hijacker at runtime")
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
