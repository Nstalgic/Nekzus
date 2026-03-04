package middleware

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRecovery(t *testing.T) {
	tests := []struct {
		name           string
		handler        http.HandlerFunc
		expectPanic    bool
		expectedStatus int
		expectedBody   string
	}{
		{
			name: "normal request - no panic",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("success"))
			},
			expectPanic:    false,
			expectedStatus: http.StatusOK,
			expectedBody:   "success",
		},
		{
			name: "panic with string",
			handler: func(w http.ResponseWriter, r *http.Request) {
				panic("something went wrong")
			},
			expectPanic:    true,
			expectedStatus: http.StatusInternalServerError,
			expectedBody:   "Internal Server Error",
		},
		{
			name: "panic with error",
			handler: func(w http.ResponseWriter, r *http.Request) {
				panic(fmt.Errorf("database error"))
			},
			expectPanic:    true,
			expectedStatus: http.StatusInternalServerError,
			expectedBody:   "Internal Server Error",
		},
		{
			name: "panic with struct",
			handler: func(w http.ResponseWriter, r *http.Request) {
				panic(struct{ Msg string }{Msg: "custom panic"})
			},
			expectPanic:    true,
			expectedStatus: http.StatusInternalServerError,
			expectedBody:   "Internal Server Error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a test recorder to capture logs
			var logOutput strings.Builder
			logFunc := func(format string, args ...interface{}) {
				fmt.Fprintf(&logOutput, format, args...)
			}

			// Wrap the handler with recovery middleware
			wrapped := Recovery(logFunc)(tt.handler)

			// Create test request and response recorder
			req := httptest.NewRequest("GET", "/test", nil)
			rec := httptest.NewRecorder()

			// Execute the request
			wrapped.ServeHTTP(rec, req)

			// Check status code
			if rec.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, rec.Code)
			}

			// Check response body
			body := strings.TrimSpace(rec.Body.String())
			if body != tt.expectedBody {
				t.Errorf("Expected body %q, got %q", tt.expectedBody, body)
			}

			// Check if panic was logged
			logStr := logOutput.String()
			if tt.expectPanic && !strings.Contains(logStr, "Panic recovered") {
				t.Error("Expected panic to be logged")
			}
			if tt.expectPanic && !strings.Contains(logStr, "Stack trace") {
				t.Error("Expected stack trace to be logged")
			}
		})
	}
}

func TestRecoveryWithMetrics(t *testing.T) {
	// Mock metrics recorder
	panicCount := 0
	metricsFunc := func() {
		panicCount++
	}

	// Handler that panics
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("test panic")
	})

	// Wrap with recovery middleware
	wrapped := RecoveryWithMetrics(func(string, ...interface{}) {}, metricsFunc)(handler)

	// Execute request
	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	// Check metrics were recorded
	if panicCount != 1 {
		t.Errorf("Expected panic metric to be recorded once, got %d", panicCount)
	}

	// Check response
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("Expected status 500, got %d", rec.Code)
	}
}

func TestRecoveryPreservesHeaders(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Custom-Header", "value")
		w.WriteHeader(http.StatusOK)
		panic("panic after headers")
	})

	wrapped := Recovery(func(string, ...interface{}) {})(handler)

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	// Headers set before panic should be preserved
	if rec.Header().Get("X-Custom-Header") != "value" {
		t.Error("Expected custom header to be preserved")
	}

	// Status should be what was written before panic
	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200 (written before panic), got %d", rec.Code)
	}
}

// Test that Recovery middleware preserves http.Hijacker interface for WebSocket upgrades
func TestRecovery_PreservesHijacker(t *testing.T) {
	hijackAttempted := false
	hijackSucceeded := false

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hijackAttempted = true

		hijacker, ok := w.(http.Hijacker)
		if !ok {
			t.Error("ResponseWriter does not implement http.Hijacker - WebSocket upgrade will fail")
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

	// Wrap handler with Recovery middleware
	wrapped := Recovery(func(string, ...interface{}) {})(handler)

	// Create test server
	server := httptest.NewServer(wrapped)
	defer server.Close()

	// Make a request
	resp, err := http.Get(server.URL)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if !hijackAttempted {
		t.Error("Hijack was not attempted")
	}

	if !hijackSucceeded {
		t.Error("Hijack did not succeed - Recovery middleware breaks WebSocket support")
	}
}

// Test that the ResponseWriter wrapped by Recovery implements http.Hijacker
func TestRecovery_HijackableResponseWriterInterface(t *testing.T) {
	// Create a mock http.ResponseWriter that implements Hijacker
	mockWriter := &mockRecoveryHijackableWriter{}

	// The recovery middleware should preserve the Hijacker interface
	// We'll verify this by checking if the handler receives a writer that implements Hijacker

	hijackerAvailable := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, ok := w.(http.Hijacker)
		hijackerAvailable = ok
		w.WriteHeader(http.StatusOK)
	})

	wrapped := Recovery(func(string, ...interface{}) {})(handler)

	// Use httptest.Server which provides a real ResponseWriter with Hijacker
	server := httptest.NewServer(wrapped)
	defer server.Close()

	resp, err := http.Get(server.URL)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if !hijackerAvailable {
		t.Error("Recovery middleware should preserve http.Hijacker interface")
	}

	// Suppress unused variable warning
	_ = mockWriter
}

// mockRecoveryHijackableWriter is a mock http.ResponseWriter that implements Hijacker
type mockRecoveryHijackableWriter struct {
	http.ResponseWriter
}

func (m *mockRecoveryHijackableWriter) Header() http.Header {
	return http.Header{}
}

func (m *mockRecoveryHijackableWriter) Write([]byte) (int, error) {
	return 0, nil
}

func (m *mockRecoveryHijackableWriter) WriteHeader(statusCode int) {}

func (m *mockRecoveryHijackableWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return &mockRecoveryConn{}, bufio.NewReadWriter(bufio.NewReader(strings.NewReader("")), bufio.NewWriter(&strings.Builder{})), nil
}

// mockRecoveryConn is a minimal net.Conn implementation for testing
type mockRecoveryConn struct{}

func (m *mockRecoveryConn) Read(b []byte) (n int, err error)   { return 0, nil }
func (m *mockRecoveryConn) Write(b []byte) (n int, err error)  { return len(b), nil }
func (m *mockRecoveryConn) Close() error                       { return nil }
func (m *mockRecoveryConn) LocalAddr() net.Addr                { return nil }
func (m *mockRecoveryConn) RemoteAddr() net.Addr               { return nil }
func (m *mockRecoveryConn) SetDeadline(t time.Time) error      { return nil }
func (m *mockRecoveryConn) SetReadDeadline(t time.Time) error  { return nil }
func (m *mockRecoveryConn) SetWriteDeadline(t time.Time) error { return nil }

func BenchmarkRecovery(b *testing.B) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	wrapped := Recovery(func(string, ...interface{}) {})(handler)

	req := httptest.NewRequest("GET", "/test", nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rec := httptest.NewRecorder()
		wrapped.ServeHTTP(rec, req)
	}
}

func BenchmarkRecoveryWithPanic(b *testing.B) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("benchmark panic")
	})

	wrapped := Recovery(func(string, ...interface{}) {})(handler)

	req := httptest.NewRequest("GET", "/test", nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rec := httptest.NewRecorder()
		wrapped.ServeHTTP(rec, req)
	}
}
