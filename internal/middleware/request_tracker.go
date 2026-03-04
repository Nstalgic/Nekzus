package middleware

import (
	"net/http"
	"time"
)

// RequestStorage defines the interface for storing request metrics
type RequestStorage interface {
	IncrementRequestCount(deviceID string, latency time.Duration, bytes int64, isError bool) error
}

// responseWriter wraps http.ResponseWriter to capture response metrics
type responseWriter struct {
	http.ResponseWriter
	statusCode    int
	bytesWritten  int64
	headerWritten bool
}

func newResponseWriter(w http.ResponseWriter) *responseWriter {
	return &responseWriter{
		ResponseWriter: w,
		statusCode:     http.StatusOK, // Default status if WriteHeader is never called
		bytesWritten:   0,
		headerWritten:  false,
	}
}

func (rw *responseWriter) WriteHeader(code int) {
	if !rw.headerWritten {
		rw.statusCode = code
		rw.headerWritten = true
		rw.ResponseWriter.WriteHeader(code)
	}
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	if !rw.headerWritten {
		rw.WriteHeader(http.StatusOK)
	}
	n, err := rw.ResponseWriter.Write(b)
	rw.bytesWritten += int64(n)
	return n, err
}

// Flush implements http.Flusher interface for SSE/streaming support
// This is required for Server-Sent Events and chunked responses to work properly
func (rw *responseWriter) Flush() {
	if f, ok := rw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// RequestTracker returns middleware that tracks request metrics per device
func RequestTracker(storage RequestStorage) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Try to get device ID from JWT context first (from auth middleware)
			// then fall back to X-Device-ID header
			deviceID := GetDeviceIDFromContext(r.Context())
			if deviceID == "" {
				deviceID = r.Header.Get("X-Device-ID")
			}

			// Skip tracking if no storage or no device ID
			if storage == nil || deviceID == "" {
				next.ServeHTTP(w, r)
				return
			}

			// Wrap response writer to capture metrics
			rw := newResponseWriter(w)
			startTime := time.Now()

			// Count request bytes (from Content-Length header)
			requestBytes := r.ContentLength
			if requestBytes < 0 {
				requestBytes = 0
			}

			// Serve the request
			next.ServeHTTP(rw, r)

			// Measure latency
			latency := time.Since(startTime)

			// Calculate total bytes transferred (request + response)
			totalBytes := requestBytes + rw.bytesWritten

			// Determine if this is an error response (4xx or 5xx)
			isError := rw.statusCode >= 400

			// Track the request asynchronously to avoid blocking the response
			go func() {
				_ = storage.IncrementRequestCount(deviceID, latency, totalBytes, isError)
				// Errors are silently ignored to not impact request processing
			}()
		})
	}
}
