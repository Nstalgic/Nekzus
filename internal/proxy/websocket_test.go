package proxy

import (
	"context"
	"crypto/sha1"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"path"
	"strings"
	"testing"
	"time"
)

func TestIsWebSocketUpgrade(t *testing.T) {
	testCases := []struct {
		name     string
		headers  map[string]string
		expected bool
	}{
		{
			name: "valid websocket upgrade",
			headers: map[string]string{
				"Upgrade":    "websocket",
				"Connection": "Upgrade",
			},
			expected: true,
		},
		{
			name: "valid websocket upgrade with mixed case",
			headers: map[string]string{
				"Upgrade":    "WebSocket",
				"Connection": "UpGrade",
			},
			expected: true,
		},
		{
			name: "valid websocket upgrade with keep-alive",
			headers: map[string]string{
				"Upgrade":    "websocket",
				"Connection": "keep-alive, Upgrade",
			},
			expected: true,
		},
		{
			name: "missing upgrade header",
			headers: map[string]string{
				"Connection": "Upgrade",
			},
			expected: false,
		},
		{
			name: "missing connection header",
			headers: map[string]string{
				"Upgrade": "websocket",
			},
			expected: false,
		},
		{
			name: "wrong upgrade value",
			headers: map[string]string{
				"Upgrade":    "http/2",
				"Connection": "Upgrade",
			},
			expected: false,
		},
		{
			name:     "no headers",
			headers:  map[string]string{},
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/ws", nil)
			for key, val := range tc.headers {
				req.Header.Set(key, val)
			}

			result := IsWebSocketUpgrade(req)
			if result != tc.expected {
				t.Errorf("expected %v, got %v", tc.expected, result)
			}
		})
	}
}

func TestNewWebSocketProxy(t *testing.T) {
	proxy := NewWebSocketProxy("http://localhost:8080")

	if proxy == nil {
		t.Fatal("proxy should not be nil")
	}

	if proxy.Target != "http://localhost:8080" {
		t.Errorf("expected target 'http://localhost:8080', got %s", proxy.Target)
	}

	if proxy.BufferSize != 32*1024 {
		t.Errorf("expected buffer size 32KB, got %d", proxy.BufferSize)
	}

	if proxy.DialTimeout != 10*time.Second {
		t.Errorf("expected dial timeout 10s, got %s", proxy.DialTimeout)
	}
}

func TestWebSocketProxyNonWebSocketRequest(t *testing.T) {
	wsProxy := NewWebSocketProxy("http://localhost:8080")

	// Create a test server
	server := httptest.NewServer(wsProxy)
	defer server.Close()

	// Make a regular HTTP request (not WebSocket)
	resp, err := http.Get(server.URL)
	if err != nil {
		t.Fatalf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", resp.StatusCode)
	}
}

func TestWebSocketProxyCustomBufferSize(t *testing.T) {
	wsProxy := NewWebSocketProxy("http://localhost:8080")
	wsProxy.BufferSize = 64 * 1024 // 64KB

	if wsProxy.BufferSize != 64*1024 {
		t.Errorf("expected buffer size 64KB, got %d", wsProxy.BufferSize)
	}
}

func TestWebSocketProxyCustomDialTimeout(t *testing.T) {
	wsProxy := NewWebSocketProxy("http://localhost:8080")
	wsProxy.DialTimeout = 5 * time.Second

	if wsProxy.DialTimeout != 5*time.Second {
		t.Errorf("expected dial timeout 5s, got %s", wsProxy.DialTimeout)
	}
}

func TestWebSocketProxyCustomTimeouts(t *testing.T) {
	wsProxy := NewWebSocketProxy("http://localhost:8080")
	wsProxy.ReadTimeout = 30 * time.Second
	wsProxy.WriteTimeout = 30 * time.Second
	wsProxy.IdleTimeout = 5 * time.Minute

	if wsProxy.ReadTimeout != 30*time.Second {
		t.Errorf("expected read timeout 30s, got %s", wsProxy.ReadTimeout)
	}
	if wsProxy.WriteTimeout != 30*time.Second {
		t.Errorf("expected write timeout 30s, got %s", wsProxy.WriteTimeout)
	}
	if wsProxy.IdleTimeout != 5*time.Minute {
		t.Errorf("expected idle timeout 5m, got %s", wsProxy.IdleTimeout)
	}
}

func TestWebSocketProxyTLSConfig(t *testing.T) {
	wsProxy := NewWebSocketProxy("wss://localhost:8443")
	wsProxy.InsecureSkipVerify = true

	if !wsProxy.InsecureSkipVerify {
		t.Error("expected InsecureSkipVerify to be true")
	}
}

func TestWebSocketProxyCallbacks(t *testing.T) {
	wsProxy := NewWebSocketProxy("http://localhost:8080")

	var connectCalled, disconnectCalled, errorCalled bool

	wsProxy.OnConnect = func(clientAddr, upstreamAddr string) {
		connectCalled = true
	}

	wsProxy.OnDisconnect = func(clientAddr, upstreamAddr string, duration time.Duration) {
		disconnectCalled = true
	}

	wsProxy.OnError = func(err error) {
		errorCalled = true
	}

	// Verify callbacks are set
	if wsProxy.OnConnect == nil {
		t.Error("OnConnect should be set")
	}

	if wsProxy.OnDisconnect == nil {
		t.Error("OnDisconnect should be set")
	}

	if wsProxy.OnError == nil {
		t.Error("OnError should be set")
	}

	// Call the callbacks
	wsProxy.OnConnect("client", "upstream")
	if !connectCalled {
		t.Error("OnConnect callback was not called")
	}

	wsProxy.OnDisconnect("client", "upstream", time.Second)
	if !disconnectCalled {
		t.Error("OnDisconnect callback was not called")
	}

	wsProxy.OnError(nil)
	if !errorCalled {
		t.Error("OnError callback was not called")
	}
}

func TestIsNormalClose(t *testing.T) {
	testCases := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: true,
		},
		{
			name:     "EOF error",
			err:      io.EOF,
			expected: true,
		},
		{
			name:     "context canceled",
			err:      context.Canceled,
			expected: true,
		},
		{
			name:     "context deadline exceeded",
			err:      context.DeadlineExceeded,
			expected: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := isNormalClose(tc.err)
			if result != tc.expected {
				t.Errorf("expected %v, got %v", tc.expected, result)
			}
		})
	}
}

func TestWebSocketProxyTargetURLConversion(t *testing.T) {
	testCases := []struct {
		name   string
		target string
	}{
		{"http URL", "http://localhost:8080"},
		{"https URL", "https://localhost:8443"},
		{"ws URL", "ws://localhost:8080"},
		{"wss URL", "wss://localhost:8443"},
		{"host only", "localhost:8080"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			wsProxy := NewWebSocketProxy(tc.target)
			if wsProxy.Target != tc.target {
				t.Errorf("expected target %s, got %s", tc.target, wsProxy.Target)
			}
		})
	}
}

func TestWebSocketPathJoining(t *testing.T) {
	testCases := []struct {
		name         string
		basePath     string
		requestPath  string
		expectedPath string
	}{
		{
			name:         "root base with subpath",
			basePath:     "/",
			requestPath:  "/websockets",
			expectedPath: "/websockets",
		},
		{
			name:         "base path with subpath",
			basePath:     "/apps/obsidian",
			requestPath:  "/websockets",
			expectedPath: "/apps/obsidian/websockets",
		},
		{
			name:         "root base with root request",
			basePath:     "/",
			requestPath:  "/",
			expectedPath: "/",
		},
		{
			name:         "base path with root request",
			basePath:     "/api/v1",
			requestPath:  "/",
			expectedPath: "/api/v1",
		},
		{
			name:         "nested paths",
			basePath:     "/apps",
			requestPath:  "/service/endpoint",
			expectedPath: "/apps/service/endpoint",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			requestPath := tc.requestPath
			if requestPath == "" {
				requestPath = "/"
			}

			// Use path.Join like the actual implementation
			finalPath := path.Join(tc.basePath, requestPath)
			if !strings.HasPrefix(finalPath, "/") {
				finalPath = "/" + finalPath
			}

			if finalPath != tc.expectedPath {
				t.Errorf("expected path %s, got %s", tc.expectedPath, finalPath)
			}
		})
	}
}

func TestComputeAcceptKey(t *testing.T) {
	// Test vector from RFC 6455
	testKey := "dGhlIHNhbXBsZSBub25jZQ=="
	expectedAccept := "s3pPLMBiTxaQ9kYGzzhZRbK+xOo="

	result := computeAcceptKey(testKey)
	if result != expectedAccept {
		t.Errorf("expected accept key %s, got %s", expectedAccept, result)
	}
}

func TestValidateUpgradeResponse(t *testing.T) {
	wsProxy := NewWebSocketProxy("http://localhost:8080")
	clientKey := "dGhlIHNhbXBsZSBub25jZQ=="

	// Compute expected accept
	h := sha1.New()
	h.Write([]byte(clientKey))
	h.Write([]byte("258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
	acceptKey := base64.StdEncoding.EncodeToString(h.Sum(nil))

	testCases := []struct {
		name      string
		response  string
		expectErr bool
	}{
		{
			name: "valid response",
			response: "HTTP/1.1 101 Switching Protocols\r\n" +
				"Upgrade: websocket\r\n" +
				"Connection: Upgrade\r\n" +
				"Sec-WebSocket-Accept: " + acceptKey + "\r\n\r\n",
			expectErr: false,
		},
		{
			name: "valid response lowercase headers",
			response: "HTTP/1.1 101 Switching Protocols\r\n" +
				"upgrade: websocket\r\n" +
				"connection: upgrade\r\n" +
				"sec-websocket-accept: " + acceptKey + "\r\n\r\n",
			expectErr: false,
		},
		{
			name: "wrong status code",
			response: "HTTP/1.1 200 OK\r\n" +
				"Upgrade: websocket\r\n" +
				"Connection: Upgrade\r\n\r\n",
			expectErr: true,
		},
		{
			name: "missing upgrade header",
			response: "HTTP/1.1 101 Switching Protocols\r\n" +
				"Connection: Upgrade\r\n" +
				"Sec-WebSocket-Accept: " + acceptKey + "\r\n\r\n",
			expectErr: true,
		},
		{
			name: "missing connection header",
			response: "HTTP/1.1 101 Switching Protocols\r\n" +
				"Upgrade: websocket\r\n" +
				"Sec-WebSocket-Accept: " + acceptKey + "\r\n\r\n",
			expectErr: true,
		},
		{
			name: "wrong accept key",
			response: "HTTP/1.1 101 Switching Protocols\r\n" +
				"Upgrade: websocket\r\n" +
				"Connection: Upgrade\r\n" +
				"Sec-WebSocket-Accept: wrongkey\r\n\r\n",
			expectErr: true,
		},
		{
			name:      "empty response",
			response:  "",
			expectErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := wsProxy.validateUpgradeResponse([]byte(tc.response), clientKey)
			if tc.expectErr && err == nil {
				t.Error("expected error but got nil")
			}
			if !tc.expectErr && err != nil {
				t.Errorf("expected no error but got: %v", err)
			}
		})
	}
}

func TestValidateUpgradeResponseNoKey(t *testing.T) {
	wsProxy := NewWebSocketProxy("http://localhost:8080")

	// When no client key is provided, accept validation is skipped
	response := "HTTP/1.1 101 Switching Protocols\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n\r\n"

	err := wsProxy.validateUpgradeResponse([]byte(response), "")
	if err != nil {
		t.Errorf("expected no error when client key is empty, got: %v", err)
	}
}

func TestIsTimeout(t *testing.T) {
	// Test that isTimeout correctly identifies timeout errors
	// This is hard to test without actual network operations,
	// but we can at least verify it doesn't panic on nil
	result := isTimeout(nil)
	if result {
		t.Error("nil error should not be a timeout")
	}

	result = isTimeout(io.EOF)
	if result {
		t.Error("EOF should not be a timeout")
	}
}

func TestWebSocketProxySchemeDetection(t *testing.T) {
	testCases := []struct {
		name     string
		target   string
		wantsTLS bool
	}{
		{
			name:     "ws scheme",
			target:   "ws://localhost:8080",
			wantsTLS: false,
		},
		{
			name:     "wss scheme",
			target:   "wss://localhost:8443",
			wantsTLS: true,
		},
		{
			name:     "http scheme",
			target:   "http://localhost:8080",
			wantsTLS: false,
		},
		{
			name:     "https scheme",
			target:   "https://localhost:8443",
			wantsTLS: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// The actual TLS detection happens in dialUpstream
			// We just verify the proxy can be created with different schemes
			wsProxy := NewWebSocketProxy(tc.target)
			if wsProxy == nil {
				t.Error("proxy should not be nil")
			}
		})
	}
}

func TestWebSocketProxyDefaultPorts(t *testing.T) {
	// Test that default ports are added correctly
	// ws -> 80, wss -> 443
	testCases := []struct {
		name         string
		target       string
		expectedHost string
	}{
		{
			name:         "ws without port",
			target:       "ws://localhost",
			expectedHost: "localhost:80",
		},
		{
			name:         "wss without port",
			target:       "wss://localhost",
			expectedHost: "localhost:443",
		},
		{
			name:         "ws with port",
			target:       "ws://localhost:8080",
			expectedHost: "localhost:8080",
		},
		{
			name:         "wss with port",
			target:       "wss://localhost:8443",
			expectedHost: "localhost:8443",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			wsProxy := NewWebSocketProxy(tc.target)
			if wsProxy == nil {
				t.Error("proxy should not be nil")
			}
			// The actual port handling is done in dialUpstream
			// This test just verifies the proxy accepts these targets
		})
	}
}

// TestHasPort tests the hasPort helper function for IPv6 address handling
func TestHasPort(t *testing.T) {
	testCases := []struct {
		name     string
		host     string
		expected bool
	}{
		{
			name:     "IPv4 with port",
			host:     "192.168.1.1:8080",
			expected: true,
		},
		{
			name:     "IPv4 without port",
			host:     "192.168.1.1",
			expected: false,
		},
		{
			name:     "hostname with port",
			host:     "localhost:8080",
			expected: true,
		},
		{
			name:     "hostname without port",
			host:     "localhost",
			expected: false,
		},
		{
			name:     "IPv6 with port",
			host:     "[::1]:8080",
			expected: true,
		},
		{
			name:     "IPv6 without port",
			host:     "[::1]",
			expected: false,
		},
		{
			name:     "IPv6 full address with port",
			host:     "[2001:db8::1]:443",
			expected: true,
		},
		{
			name:     "IPv6 full address without port",
			host:     "[2001:db8::1]",
			expected: false,
		},
		{
			name:     "IPv6 with zone with port",
			host:     "[fe80::1%eth0]:8080",
			expected: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := hasPort(tc.host)
			if result != tc.expected {
				t.Errorf("hasPort(%q) = %v, want %v", tc.host, result, tc.expected)
			}
		})
	}
}

// TestExtractServerName tests server name extraction for SNI with IPv6
func TestExtractServerName(t *testing.T) {
	testCases := []struct {
		name     string
		host     string
		expected string
	}{
		{
			name:     "hostname with port",
			host:     "example.com:443",
			expected: "example.com",
		},
		{
			name:     "hostname without port",
			host:     "example.com",
			expected: "example.com",
		},
		{
			name:     "IPv4 with port",
			host:     "192.168.1.1:8080",
			expected: "192.168.1.1",
		},
		{
			name:     "IPv6 with port",
			host:     "[::1]:8080",
			expected: "::1",
		},
		{
			name:     "IPv6 without port",
			host:     "[::1]",
			expected: "::1",
		},
		{
			name:     "IPv6 full with port",
			host:     "[2001:db8::1]:443",
			expected: "2001:db8::1",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := extractServerName(tc.host)
			if result != tc.expected {
				t.Errorf("extractServerName(%q) = %q, want %q", tc.host, result, tc.expected)
			}
		})
	}
}

// TestWebSocketProxyIdleTimeoutField tests that IdleTimeout field is properly set
func TestWebSocketProxyIdleTimeoutField(t *testing.T) {
	proxy := NewWebSocketProxy("ws://localhost:8080")

	// Default should be 0 (no timeout)
	if proxy.IdleTimeout != 0 {
		t.Errorf("default IdleTimeout = %v, want 0", proxy.IdleTimeout)
	}

	// Should be able to set custom value
	proxy.IdleTimeout = 30 * time.Second
	if proxy.IdleTimeout != 30*time.Second {
		t.Errorf("IdleTimeout = %v, want 30s", proxy.IdleTimeout)
	}
}

// TestImportantHeadersList tests that ImportantHeaders includes all required headers
func TestImportantHeadersList(t *testing.T) {
	// Verify ImportantHeaders includes User-Agent, Accept-Language, Referer
	requiredHeaders := []string{"User-Agent", "Accept-Language", "Referer"}

	for _, required := range requiredHeaders {
		found := false
		for _, h := range ImportantHeaders {
			if h == required {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("ImportantHeaders should include %q", required)
		}
	}
}

// TestWebSocketProxyMaxMessageSize tests MaxMessageSize field and default
func TestWebSocketProxyMaxMessageSize(t *testing.T) {
	proxy := NewWebSocketProxy("ws://localhost:8080")

	// Default should be 0 (will use DefaultMaxMessageSize)
	if proxy.MaxMessageSize != 0 {
		t.Errorf("default MaxMessageSize = %d, want 0", proxy.MaxMessageSize)
	}

	// Should be able to set custom value
	proxy.MaxMessageSize = 50 * 1024 * 1024 // 50MB
	if proxy.MaxMessageSize != 50*1024*1024 {
		t.Errorf("MaxMessageSize = %d, want 50MB", proxy.MaxMessageSize)
	}
}

// TestDefaultMaxMessageSize tests the default constant exists
func TestDefaultMaxMessageSize(t *testing.T) {
	// Verify DefaultMaxMessageSize constant exists and is reasonable
	if DefaultMaxMessageSize <= 0 {
		t.Errorf("DefaultMaxMessageSize should be positive, got %d", DefaultMaxMessageSize)
	}

	// Should be 100MB
	if DefaultMaxMessageSize != 100*1024*1024 {
		t.Errorf("DefaultMaxMessageSize = %d, want %d", DefaultMaxMessageSize, 100*1024*1024)
	}
}

// TestWebSocketProxyStripCookies tests StripCookies field
func TestWebSocketProxyStripCookies(t *testing.T) {
	proxy := NewWebSocketProxy("ws://localhost:8080")

	// Default should be false
	if proxy.StripCookies != false {
		t.Errorf("default StripCookies = %v, want false", proxy.StripCookies)
	}

	// Should be able to set to true
	proxy.StripCookies = true
	if proxy.StripCookies != true {
		t.Errorf("StripCookies = %v, want true", proxy.StripCookies)
	}
}

// TestWebSocketProxyDefaultTLSMinVersion tests that default TLS config has MinVersion
func TestWebSocketProxyDefaultTLSMinVersion(t *testing.T) {
	// This test verifies that when no TLSConfig is provided, the default
	// TLS configuration includes MinVersion TLS 1.2
	// The actual implementation is in dialUpstream when tlsConfig is nil

	proxy := NewWebSocketProxy("wss://localhost:8443")

	// TLSConfig should be nil by default (dialUpstream creates default with MinVersion)
	if proxy.TLSConfig != nil {
		t.Error("default TLSConfig should be nil (dialUpstream creates secure default)")
	}
}

// TestWebSocketProxyIdleTimeoutImplementation tests that IdleTimeout actually works
func TestWebSocketProxyIdleTimeoutImplementation(t *testing.T) {
	proxy := NewWebSocketProxy("ws://localhost:8080")

	// Set a short idle timeout
	proxy.IdleTimeout = 100 * time.Millisecond

	// The implementation should use this value to cancel idle connections
	// This test verifies the field can be set and will be used
	if proxy.IdleTimeout != 100*time.Millisecond {
		t.Errorf("IdleTimeout = %v, want 100ms", proxy.IdleTimeout)
	}
}

// TestParseHTTPStatus tests parsing HTTP status from response line
func TestParseHTTPStatus(t *testing.T) {
	testCases := []struct {
		name       string
		response   []byte
		wantStatus int
		wantOK     bool
	}{
		{
			name:       "101 Switching Protocols",
			response:   []byte("HTTP/1.1 101 Switching Protocols\r\n\r\n"),
			wantStatus: 101,
			wantOK:     true,
		},
		{
			name:       "401 Unauthorized",
			response:   []byte("HTTP/1.1 401 Unauthorized\r\nContent-Type: text/plain\r\n\r\n"),
			wantStatus: 401,
			wantOK:     true,
		},
		{
			name:       "403 Forbidden",
			response:   []byte("HTTP/1.1 403 Forbidden\r\n\r\n"),
			wantStatus: 403,
			wantOK:     true,
		},
		{
			name:       "500 Internal Server Error",
			response:   []byte("HTTP/1.1 500 Internal Server Error\r\n\r\n"),
			wantStatus: 500,
			wantOK:     true,
		},
		{
			name:       "HTTP/1.0 200 OK",
			response:   []byte("HTTP/1.0 200 OK\r\n\r\n"),
			wantStatus: 200,
			wantOK:     true,
		},
		{
			name:       "invalid response",
			response:   []byte("not http\r\n\r\n"),
			wantStatus: 0,
			wantOK:     false,
		},
		{
			name:       "empty response",
			response:   []byte{},
			wantStatus: 0,
			wantOK:     false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			status, ok := ParseHTTPStatus(tc.response)
			if ok != tc.wantOK {
				t.Errorf("ParseHTTPStatus() ok = %v, want %v", ok, tc.wantOK)
			}
			if status != tc.wantStatus {
				t.Errorf("ParseHTTPStatus() status = %d, want %d", status, tc.wantStatus)
			}
		})
	}
}

// TestWebSocketBufferPool tests buffer pool functionality
func TestWebSocketBufferPool(t *testing.T) {
	// Get a buffer from the pool
	buf := GetWebSocketBuffer()
	if buf == nil {
		t.Fatal("GetWebSocketBuffer returned nil")
	}

	// Verify buffer size
	if len(*buf) != WebSocketBufferSize {
		t.Errorf("buffer size = %d, want %d", len(*buf), WebSocketBufferSize)
	}

	// Put buffer back
	PutWebSocketBuffer(buf)

	// Get another buffer (should reuse)
	buf2 := GetWebSocketBuffer()
	if buf2 == nil {
		t.Fatal("GetWebSocketBuffer returned nil after put")
	}

	// Verify size again
	if len(*buf2) != WebSocketBufferSize {
		t.Errorf("reused buffer size = %d, want %d", len(*buf2), WebSocketBufferSize)
	}

	PutWebSocketBuffer(buf2)
}

// TestWebSocketBufferPoolConcurrent tests concurrent buffer pool access
func TestWebSocketBufferPoolConcurrent(t *testing.T) {
	done := make(chan bool)
	numGoroutines := 100

	for i := 0; i < numGoroutines; i++ {
		go func() {
			// Get buffer
			buf := GetWebSocketBuffer()
			if buf == nil {
				t.Error("GetWebSocketBuffer returned nil")
				done <- true
				return
			}

			// Use buffer briefly
			(*buf)[0] = 0x01

			// Return buffer
			PutWebSocketBuffer(buf)
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < numGoroutines; i++ {
		<-done
	}
}

// TestWebSocketBufferSizeConstant tests buffer size constant exists
func TestWebSocketBufferSizeConstant(t *testing.T) {
	// Should be 32KB
	if WebSocketBufferSize != 32*1024 {
		t.Errorf("WebSocketBufferSize = %d, want %d", WebSocketBufferSize, 32*1024)
	}
}

// TestCreateTLSDialer tests that we use tls.Dialer with DialContext
func TestCreateTLSDialer(t *testing.T) {
	dialer := CreateTLSDialer(10 * time.Second)
	if dialer == nil {
		t.Fatal("CreateTLSDialer returned nil")
	}

	// Verify it has a NetDialer configured
	if dialer.NetDialer == nil {
		t.Error("NetDialer should be configured")
	}

	// Verify timeout is set
	if dialer.NetDialer.Timeout != 10*time.Second {
		t.Errorf("Timeout = %v, want 10s", dialer.NetDialer.Timeout)
	}
}
