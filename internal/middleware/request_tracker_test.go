package middleware

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockStorage implements the storage interface for testing
type MockStorage struct {
	mu                          sync.Mutex
	done                        chan struct{}
	IncrementRequestCountCalled bool
	LastDeviceID                string
	LastLatency                 time.Duration
	LastBytes                   int64
	LastIsError                 bool
}

func newMockStorage() *MockStorage {
	return &MockStorage{
		done: make(chan struct{}, 1),
	}
}

func (m *MockStorage) IncrementRequestCount(deviceID string, latency time.Duration, bytes int64, isError bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.IncrementRequestCountCalled = true
	m.LastDeviceID = deviceID
	m.LastLatency = latency
	m.LastBytes = bytes
	m.LastIsError = isError

	// Signal completion
	select {
	case m.done <- struct{}{}:
	default:
	}

	return nil
}

func (m *MockStorage) Wait(timeout time.Duration) bool {
	select {
	case <-m.done:
		return true
	case <-time.After(timeout):
		return false
	}
}

func (m *MockStorage) GetLastDeviceID() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.LastDeviceID
}

func (m *MockStorage) GetLastLatency() time.Duration {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.LastLatency
}

func (m *MockStorage) GetLastBytes() int64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.LastBytes
}

func (m *MockStorage) GetLastIsError() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.LastIsError
}

func (m *MockStorage) WasCalled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.IncrementRequestCountCalled
}

// Test 2.1: Middleware tracks successful request
func TestRequestTracker_SuccessfulRequest(t *testing.T) {
	// Arrange
	mock := newMockStorage()
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("test response"))
	})

	tracker := RequestTracker(mock)(handler)

	req := httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("X-Device-ID", "device_123")
	rec := httptest.NewRecorder()

	// Act
	tracker.ServeHTTP(rec, req)

	// Wait for async tracking to complete
	assert.True(t, mock.Wait(100*time.Millisecond), "Tracking should complete within timeout")

	// Assert
	assert.True(t, mock.WasCalled())
	assert.Equal(t, "device_123", mock.GetLastDeviceID())
	assert.Greater(t, mock.GetLastLatency(), time.Duration(0))
	assert.Greater(t, mock.GetLastBytes(), int64(0)) // Request + response bytes
	assert.False(t, mock.GetLastIsError())
}

// Test 2.2: Middleware tracks error request (4xx)
func TestRequestTracker_ClientError(t *testing.T) {
	// Arrange
	mock := newMockStorage()
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("bad request"))
	})

	tracker := RequestTracker(mock)(handler)

	req := httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("X-Device-ID", "device_123")
	rec := httptest.NewRecorder()

	// Act
	tracker.ServeHTTP(rec, req)

	// Wait for async tracking to complete
	assert.True(t, mock.Wait(100*time.Millisecond), "Tracking should complete within timeout")

	// Assert
	assert.True(t, mock.WasCalled())
	assert.Equal(t, "device_123", mock.GetLastDeviceID())
	assert.True(t, mock.GetLastIsError(), "4xx status should be marked as error")
}

// Test 2.3: Middleware tracks server error (5xx)
func TestRequestTracker_ServerError(t *testing.T) {
	// Arrange
	mock := newMockStorage()
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("server error"))
	})

	tracker := RequestTracker(mock)(handler)

	req := httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("X-Device-ID", "device_123")
	rec := httptest.NewRecorder()

	// Act
	tracker.ServeHTTP(rec, req)

	// Assert
	assert.True(t, mock.Wait(100*time.Millisecond), "Tracking should complete within timeout")
	assert.True(t, mock.WasCalled())
	assert.Equal(t, "device_123", mock.GetLastDeviceID())
	assert.True(t, mock.GetLastIsError(), "5xx status should be marked as error")
}

// Test 2.4: Middleware skips tracking when no device ID
func TestRequestTracker_NoDeviceID(t *testing.T) {
	// Arrange
	mock := newMockStorage()
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("test response"))
	})

	tracker := RequestTracker(mock)(handler)

	req := httptest.NewRequest("GET", "/api/test", nil)
	// No X-Device-ID header set
	rec := httptest.NewRecorder()

	// Act
	tracker.ServeHTTP(rec, req)

	// Small delay to ensure goroutine would have run if it was going to
	time.Sleep(10 * time.Millisecond)

	// Assert
	assert.False(t, mock.WasCalled(), "Should not track requests without device ID")
}

// Test 2.5: Middleware counts bytes transferred
func TestRequestTracker_BytesCounting(t *testing.T) {
	// Arrange
	mock := newMockStorage()
	requestBody := []byte("request payload")
	responseBody := []byte("response payload")

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write(responseBody)
	})

	tracker := RequestTracker(mock)(handler)

	req := httptest.NewRequest("POST", "/api/test", bytes.NewReader(requestBody))
	req.Header.Set("X-Device-ID", "device_123")
	req.Header.Set("Content-Length", "15") // Length of requestBody
	rec := httptest.NewRecorder()

	// Act
	tracker.ServeHTTP(rec, req)

	// Assert
	assert.True(t, mock.Wait(100*time.Millisecond), "Tracking should complete within timeout")
	assert.True(t, mock.WasCalled())
	expectedBytes := int64(len(requestBody) + len(responseBody))
	assert.Equal(t, expectedBytes, mock.GetLastBytes())
}

// Test 2.6: Middleware measures latency accurately
func TestRequestTracker_LatencyMeasurement(t *testing.T) {
	// Arrange
	mock := newMockStorage()
	handlerDelay := 50 * time.Millisecond

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(handlerDelay)
		w.WriteHeader(http.StatusOK)
	})

	tracker := RequestTracker(mock)(handler)

	req := httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("X-Device-ID", "device_123")
	rec := httptest.NewRecorder()

	// Act
	tracker.ServeHTTP(rec, req)

	// Assert
	assert.True(t, mock.Wait(100*time.Millisecond), "Tracking should complete within timeout")
	assert.True(t, mock.WasCalled())
	assert.GreaterOrEqual(t, mock.GetLastLatency(), handlerDelay, "Measured latency should be at least the handler delay")
}

// Test 2.7: Middleware handles nil storage gracefully
func TestRequestTracker_NilStorage(t *testing.T) {
	// Arrange
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("test response"))
	})

	tracker := RequestTracker(nil)(handler)

	req := httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("X-Device-ID", "device_123")
	rec := httptest.NewRecorder()

	// Act & Assert (should not panic)
	require.NotPanics(t, func() {
		tracker.ServeHTTP(rec, req)
	})

	assert.Equal(t, http.StatusOK, rec.Code)
}

// Test 2.8: Middleware preserves response status
func TestRequestTracker_PreservesStatusCode(t *testing.T) {
	// Arrange
	testCases := []int{
		http.StatusOK,
		http.StatusCreated,
		http.StatusNoContent,
		http.StatusBadRequest,
		http.StatusNotFound,
		http.StatusInternalServerError,
	}

	for _, expectedStatus := range testCases {
		t.Run(http.StatusText(expectedStatus), func(t *testing.T) {
			mock := newMockStorage()
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(expectedStatus)
			})

			tracker := RequestTracker(mock)(handler)

			req := httptest.NewRequest("GET", "/api/test", nil)
			req.Header.Set("X-Device-ID", "device_123")
			rec := httptest.NewRecorder()

			// Act
			tracker.ServeHTTP(rec, req)

			// Assert
			assert.Equal(t, expectedStatus, rec.Code, "Middleware should preserve response status")
		})
	}
}

// Test 2.9: Middleware supports GET, POST, PUT, DELETE methods
func TestRequestTracker_SupportsAllMethods(t *testing.T) {
	methods := []string{"GET", "POST", "PUT", "DELETE", "PATCH"}

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			mock := newMockStorage()
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			tracker := RequestTracker(mock)(handler)

			var body io.Reader
			if method == "POST" || method == "PUT" || method == "PATCH" {
				body = bytes.NewReader([]byte("test body"))
			}

			req := httptest.NewRequest(method, "/api/test", body)
			req.Header.Set("X-Device-ID", "device_123")
			rec := httptest.NewRecorder()

			// Act
			tracker.ServeHTTP(rec, req)

			// Wait for async tracking to complete
			assert.True(t, mock.Wait(100*time.Millisecond), "Tracking should complete within timeout for %s", method)

			// Assert
			assert.True(t, mock.WasCalled(), "%s method should be tracked", method)
		})
	}
}

// Test that RequestTracker extracts device ID from JWT context FIRST, then falls back to header
func TestRequestTracker_ExtractsDeviceFromJWTContext(t *testing.T) {
	// Arrange
	mock := newMockStorage()
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	tracker := RequestTracker(mock)(handler)

	// Create request with device ID in context (simulating JWT middleware set it)
	req := httptest.NewRequest("GET", "/api/test", nil)
	ctx := context.WithValue(req.Context(), deviceIDContextKey, "device_from_jwt")
	req = req.WithContext(ctx)
	// Also set header to verify context takes precedence
	req.Header.Set("X-Device-ID", "device_from_header")
	rec := httptest.NewRecorder()

	// Act
	tracker.ServeHTTP(rec, req)

	// Assert
	assert.True(t, mock.Wait(100*time.Millisecond), "Tracking should complete within timeout")
	assert.True(t, mock.WasCalled())
	// Should use JWT context device ID, not header
	assert.Equal(t, "device_from_jwt", mock.GetLastDeviceID(), "Should prefer JWT context over header")
}

// Test that RequestTracker falls back to header when context is empty
func TestRequestTracker_FallsBackToHeaderWhenNoContext(t *testing.T) {
	// Arrange
	mock := newMockStorage()
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	tracker := RequestTracker(mock)(handler)

	req := httptest.NewRequest("GET", "/api/test", nil)
	// No context set, only header
	req.Header.Set("X-Device-ID", "device_from_header")
	rec := httptest.NewRecorder()

	// Act
	tracker.ServeHTTP(rec, req)

	// Assert
	assert.True(t, mock.Wait(100*time.Millisecond), "Tracking should complete within timeout")
	assert.True(t, mock.WasCalled())
	assert.Equal(t, "device_from_header", mock.GetLastDeviceID())
}

// Test that RequestTracker's responseWriter implements http.Flusher for SSE/streaming
func TestRequestTracker_ImplementsFlusher(t *testing.T) {
	// Arrange
	mock := newMockStorage()

	var flusherAvailable atomic.Bool
	var flushCalled atomic.Bool

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if Flusher interface is available
		flusher, ok := w.(http.Flusher)
		flusherAvailable.Store(ok)

		if ok {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("event: test\ndata: hello\n\n"))
			flusher.Flush() // For SSE, we need to flush immediately
			flushCalled.Store(true)
		} else {
			t.Error("ResponseWriter does not implement http.Flusher - SSE/streaming will break")
			http.Error(w, "flushing not supported", http.StatusInternalServerError)
		}
	})

	tracker := RequestTracker(mock)(handler)

	// Use test server to get real ResponseWriter
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Header.Set("X-Device-ID", "device_123")
		tracker.ServeHTTP(w, r)
	}))
	defer server.Close()

	// Act
	resp, err := http.Get(server.URL)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Assert
	assert.True(t, flusherAvailable.Load(), "ResponseWriter should implement http.Flusher for SSE/streaming")
	assert.True(t, flushCalled.Load(), "Flush should be callable")
}

// Test SSE streaming scenario
func TestRequestTracker_SSEStreaming(t *testing.T) {
	// Arrange
	mock := newMockStorage()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Set SSE headers
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(http.StatusOK)

		// Check for Flusher
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Error("Flusher not available - SSE won't work")
			return
		}

		// Send an event
		w.Write([]byte("event: message\ndata: test\n\n"))
		flusher.Flush()
	})

	tracker := RequestTracker(mock)(handler)

	// Use test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Header.Set("X-Device-ID", "device_sse")
		tracker.ServeHTTP(w, r)
	}))
	defer server.Close()

	// Act
	resp, err := http.Get(server.URL)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Assert
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "text/event-stream", resp.Header.Get("Content-Type"))
}

// Test 2.10: Original test - Middleware extracts device ID from context (backward compatible)
func TestRequestTracker_ExtractsDeviceFromContext(t *testing.T) {
	// Arrange
	mock := newMockStorage()
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	tracker := RequestTracker(mock)(handler)

	req := httptest.NewRequest("GET", "/api/test", nil)
	// Simulate JWT middleware setting device ID in context
	// (This test assumes we'll add context support for device ID extraction)
	req.Header.Set("X-Device-ID", "device_from_context")
	rec := httptest.NewRecorder()

	// Act
	tracker.ServeHTTP(rec, req)

	// Assert
	assert.True(t, mock.Wait(100*time.Millisecond), "Tracking should complete within timeout")
	assert.True(t, mock.WasCalled())
	assert.Equal(t, "device_from_context", mock.GetLastDeviceID())
}
