package metrics

import (
	"bufio"
	"net"
	"net/http"
	"strconv"
	"time"
)

// responseWriter wraps http.ResponseWriter to capture status code and size
type responseWriter struct {
	http.ResponseWriter
	statusCode int
	size       int64
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	n, err := rw.ResponseWriter.Write(b)
	rw.size += int64(n)
	return n, err
}

// Flush implements http.Flusher for SSE support
func (rw *responseWriter) Flush() {
	if flusher, ok := rw.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

// Hijack implements http.Hijacker for WebSocket support
func (rw *responseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := rw.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, http.ErrNotSupported
	}
	return hijacker.Hijack()
}

// Ensure responseWriter implements required interfaces
var _ http.Hijacker = (*responseWriter)(nil)
var _ http.Flusher = (*responseWriter)(nil)

// HTTPMiddleware returns an HTTP middleware that records metrics
func HTTPMiddleware(m *Metrics) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Track in-flight requests
			m.HTTPRequestsInFlight.Inc()
			defer m.HTTPRequestsInFlight.Dec()

			// Wrap response writer to capture metrics
			wrapped := &responseWriter{
				ResponseWriter: w,
				statusCode:     200, // Default status
				size:           0,
			}

			// Record start time
			start := time.Now()

			// Process request
			next.ServeHTTP(wrapped, r)

			// Record metrics
			duration := time.Since(start)
			status := strconv.Itoa(wrapped.statusCode)
			path := normalizePath(r.URL.Path)

			m.RecordHTTPRequest(
				r.Method,
				path,
				status,
				duration,
				r.ContentLength,
				wrapped.size,
			)
		})
	}
}

// normalizePath normalizes URL paths to prevent cardinality explosion
// Examples:
//
//	/api/v1/devices/dev-123 -> /api/v1/devices/:id
//	/apps/grafana/dashboard -> /apps/:app/*
func normalizePath(path string) string {
	// Handle specific known patterns
	switch {
	case path == "/api/v1/healthz":
		return "/api/v1/healthz"
	case path == "/api/v1/auth/qr":
		return "/api/v1/auth/qr"
	case path == "/api/v1/auth/pair":
		return "/api/v1/auth/pair"
	case path == "/api/v1/auth/refresh":
		return "/api/v1/auth/refresh"
	case path == "/api/v1/apps":
		return "/api/v1/apps"
	case path == "/api/v1/events":
		return "/api/v1/events"
	case path == "/api/v1/admin/info":
		return "/api/v1/admin/info"
	case path == "/api/v1/devices":
		return "/api/v1/devices"
	case path == "/api/v1/discovery/proposals":
		return "/api/v1/discovery/proposals"
	case path == "/pair":
		return "/pair"
	case path == "/metrics":
		return "/metrics"
	}

	// Match patterns with IDs
	if len(path) > 16 && path[:16] == "/api/v1/devices/" {
		return "/api/v1/devices/:id"
	}
	if len(path) > 28 && path[:28] == "/api/v1/discovery/proposals/" {
		return "/api/v1/discovery/proposals/:id"
	}
	if len(path) > 6 && path[:6] == "/apps/" {
		return "/apps/:app/*"
	}

	// Default
	return path
}
