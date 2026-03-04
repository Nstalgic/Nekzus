package proxy

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"syscall"
	"testing"
	"time"
)

// TestMapProxyError tests that different error types are mapped to appropriate HTTP status codes
func TestMapProxyError(t *testing.T) {
	tests := []struct {
		name               string
		err                error
		expectedStatus     int
		expectedMessage    string
		expectMetricsLabel string
	}{
		{
			name:               "EOF error",
			err:                io.EOF,
			expectedStatus:     http.StatusBadGateway,
			expectedMessage:    "Bad Gateway",
			expectMetricsLabel: "bad_gateway",
		},
		{
			name:               "context canceled",
			err:                context.Canceled,
			expectedStatus:     499, // Client Closed Request
			expectedMessage:    "Client Closed Request",
			expectMetricsLabel: "client_closed",
		},
		{
			name:               "context deadline exceeded",
			err:                context.DeadlineExceeded,
			expectedStatus:     http.StatusGatewayTimeout,
			expectedMessage:    "Gateway Timeout",
			expectMetricsLabel: "timeout",
		},
		{
			name:               "network timeout error",
			err:                &net.OpError{Op: "dial", Err: os.ErrDeadlineExceeded},
			expectedStatus:     http.StatusGatewayTimeout,
			expectedMessage:    "Gateway Timeout",
			expectMetricsLabel: "timeout",
		},
		{
			name:               "connection refused",
			err:                &net.OpError{Op: "dial", Err: syscall.ECONNREFUSED},
			expectedStatus:     http.StatusBadGateway,
			expectedMessage:    "Bad Gateway",
			expectMetricsLabel: "connection_refused",
		},
		{
			name:               "connection reset",
			err:                &net.OpError{Op: "read", Err: syscall.ECONNRESET},
			expectedStatus:     http.StatusBadGateway,
			expectedMessage:    "Bad Gateway",
			expectMetricsLabel: "connection_reset",
		},
		{
			name:               "DNS lookup failure",
			err:                &net.DNSError{Err: "no such host", Name: "invalid.example.com", IsNotFound: true},
			expectedStatus:     http.StatusBadGateway,
			expectedMessage:    "Bad Gateway",
			expectMetricsLabel: "dns_error",
		},
		{
			name:               "generic network error",
			err:                &net.OpError{Op: "dial", Err: errors.New("network error")},
			expectedStatus:     http.StatusBadGateway,
			expectedMessage:    "Bad Gateway",
			expectMetricsLabel: "network_error",
		},
		{
			name:               "unknown error",
			err:                errors.New("something went wrong"),
			expectedStatus:     http.StatusBadGateway,
			expectedMessage:    "Bad Gateway",
			expectMetricsLabel: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test status code mapping
			status := MapErrorToStatus(tt.err)
			if status != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, status)
			}

			// Test metrics label extraction
			label := GetErrorLabel(tt.err)
			if label != tt.expectMetricsLabel {
				t.Errorf("Expected metrics label %q, got %q", tt.expectMetricsLabel, label)
			}

			// Test user-friendly message
			message := GetErrorMessage(tt.expectedStatus)
			if message != tt.expectedMessage {
				t.Errorf("Expected message %q, got %q", tt.expectedMessage, message)
			}
		})
	}
}

// TestIsTimeoutError tests timeout error detection
func TestIsTimeoutError(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		isTimeout bool
	}{
		{
			name:      "context deadline exceeded",
			err:       context.DeadlineExceeded,
			isTimeout: true,
		},
		{
			name:      "net.OpError with deadline exceeded",
			err:       &net.OpError{Op: "dial", Err: os.ErrDeadlineExceeded},
			isTimeout: true,
		},
		{
			name: "timeout interface implementation",
			err: &timeoutError{
				timeout: true,
			},
			isTimeout: true,
		},
		{
			name:      "EOF is not timeout",
			err:       io.EOF,
			isTimeout: false,
		},
		{
			name:      "context canceled is not timeout",
			err:       context.Canceled,
			isTimeout: false,
		},
		{
			name:      "generic error is not timeout",
			err:       errors.New("error"),
			isTimeout: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsTimeoutError(tt.err)
			if result != tt.isTimeout {
				t.Errorf("Expected IsTimeoutError=%v, got %v", tt.isTimeout, result)
			}
		})
	}
}

// TestIsNetworkError tests network error detection
func TestIsNetworkError(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		isNetwork bool
	}{
		{
			name:      "net.OpError",
			err:       &net.OpError{Op: "dial", Err: errors.New("error")},
			isNetwork: true,
		},
		{
			name:      "DNS error",
			err:       &net.DNSError{Err: "no such host"},
			isNetwork: true,
		},
		{
			name:      "EOF is not network error",
			err:       io.EOF,
			isNetwork: false,
		},
		{
			name:      "context.Canceled is not network error",
			err:       context.Canceled,
			isNetwork: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsNetworkError(tt.err)
			if result != tt.isNetwork {
				t.Errorf("Expected IsNetworkError=%v, got %v", tt.isNetwork, result)
			}
		})
	}
}

// TestProxyErrorHandler tests the error handler in a realistic proxy scenario
func TestProxyErrorHandler(t *testing.T) {
	tests := []struct {
		name           string
		setupHandler   func() http.Handler
		expectedStatus int
	}{
		{
			name: "upstream returns EOF",
			setupHandler: func() http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					// Simulate connection closing unexpectedly
					hj, ok := w.(http.Hijacker)
					if !ok {
						t.Fatal("ResponseWriter doesn't support hijacking")
					}
					conn, _, _ := hj.Hijack()
					conn.Close()
				})
			},
			expectedStatus: http.StatusBadGateway,
		},
		{
			name: "upstream timeout",
			setupHandler: func() http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					// Simulate slow upstream
					time.Sleep(100 * time.Millisecond)
					w.WriteHeader(http.StatusOK)
				})
			},
			expectedStatus: http.StatusGatewayTimeout,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create upstream server
			upstream := httptest.NewServer(tt.setupHandler())
			defer upstream.Close()

			// Create proxy cache and get proxy with error handler
			cache := NewCache()
			target, _ := url.Parse(upstream.URL)

			proxy := cache.GetOrCreate(target)

			// Make request through proxy
			req := httptest.NewRequest("GET", "/test", nil)
			w := httptest.NewRecorder()

			// Set short timeout for timeout test
			if tt.name == "upstream timeout" {
				req = req.WithContext(context.WithValue(req.Context(), http.LocalAddrContextKey, &net.TCPAddr{}))
				ctx, cancel := context.WithTimeout(req.Context(), 10*time.Millisecond)
				defer cancel()
				req = req.WithContext(ctx)
			}

			proxy.ServeHTTP(w, req)

			// Verify status code matches expected error mapping
			if w.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d. Body: %s", tt.expectedStatus, w.Code, w.Body.String())
			}
		})
	}
}

// Helper types for testing
type timeoutError struct {
	timeout bool
}

func (e *timeoutError) Error() string { return "timeout" }
func (e *timeoutError) Timeout() bool { return e.timeout }

// TestMapErrorToStatus_NetworkErrors tests mapping of additional network errors
func TestMapErrorToStatus_NetworkErrors(t *testing.T) {
	tests := []struct {
		name           string
		err            error
		expectedStatus int
		expectedLabel  string
	}{
		{
			name:           "EHOSTUNREACH - host unreachable",
			err:            &net.OpError{Op: "dial", Err: syscall.EHOSTUNREACH},
			expectedStatus: http.StatusServiceUnavailable,
			expectedLabel:  "host_unreachable",
		},
		{
			name:           "ENETUNREACH - network unreachable",
			err:            &net.OpError{Op: "dial", Err: syscall.ENETUNREACH},
			expectedStatus: http.StatusServiceUnavailable,
			expectedLabel:  "network_unreachable",
		},
		{
			name:           "EPIPE - broken pipe",
			err:            &net.OpError{Op: "write", Err: syscall.EPIPE},
			expectedStatus: http.StatusBadGateway,
			expectedLabel:  "broken_pipe",
		},
		{
			name:           "ECONNABORTED - connection aborted",
			err:            &net.OpError{Op: "dial", Err: syscall.ECONNABORTED},
			expectedStatus: http.StatusBadGateway,
			expectedLabel:  "connection_aborted",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status := MapErrorToStatus(tt.err)
			if status != tt.expectedStatus {
				t.Errorf("MapErrorToStatus() = %d, want %d", status, tt.expectedStatus)
			}

			label := GetErrorLabel(tt.err)
			if label != tt.expectedLabel {
				t.Errorf("GetErrorLabel() = %q, want %q", label, tt.expectedLabel)
			}
		})
	}
}
