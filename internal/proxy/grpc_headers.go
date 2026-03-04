package proxy

import (
	"bufio"
	"net"
	"net/http"
	"strings"
)

// Ensure GRPCHeaderResponseWriter implements http.Flusher and http.Hijacker
var _ http.Flusher = (*GRPCHeaderResponseWriter)(nil)
var _ http.Hijacker = (*GRPCHeaderResponseWriter)(nil)

// GRPCHeaderResponseWriter wraps http.ResponseWriter to add gRPC-Web CORS headers
type GRPCHeaderResponseWriter struct {
	http.ResponseWriter
	headerWritten bool
}

// NewGRPCHeaderResponseWriter creates a response writer that adds gRPC CORS headers
func NewGRPCHeaderResponseWriter(w http.ResponseWriter) *GRPCHeaderResponseWriter {
	return &GRPCHeaderResponseWriter{
		ResponseWriter: w,
		headerWritten:  false,
	}
}

// WriteHeader adds gRPC CORS headers before writing the status code
func (w *GRPCHeaderResponseWriter) WriteHeader(statusCode int) {
	if !w.headerWritten {
		w.addGRPCHeaders()
		w.headerWritten = true
	}
	w.ResponseWriter.WriteHeader(statusCode)
}

// Write ensures headers are written before body
func (w *GRPCHeaderResponseWriter) Write(data []byte) (int, error) {
	if !w.headerWritten {
		w.addGRPCHeaders()
		w.headerWritten = true
	}
	return w.ResponseWriter.Write(data)
}

// addGRPCHeaders adds required CORS headers for gRPC-Web
func (w *GRPCHeaderResponseWriter) addGRPCHeaders() {
	header := w.ResponseWriter.Header()

	// Add gRPC-Web CORS headers
	// These headers must be exposed for gRPC-Web clients to read gRPC status
	existing := header.Get("Access-Control-Expose-Headers")
	grpcHeaders := "grpc-status, grpc-message, grpc-status-details-bin"

	if existing == "" {
		header.Set("Access-Control-Expose-Headers", grpcHeaders)
	} else {
		// Check if all required gRPC headers are present before appending
		// Use comma-separated check to avoid false positives (e.g., "grpc-status-details-bin" contains "grpc-status")
		hasGrpcStatus := containsHeader(existing, "grpc-status")
		hasGrpcMessage := containsHeader(existing, "grpc-message")
		hasGrpcDetails := containsHeader(existing, "grpc-status-details-bin")

		if !(hasGrpcStatus && hasGrpcMessage && hasGrpcDetails) {
			header.Set("Access-Control-Expose-Headers", existing+", "+grpcHeaders)
		}
	}

	// Add CORS headers if not already set (for gRPC-Web preflight)
	if header.Get("Access-Control-Allow-Origin") == "" {
		header.Set("Access-Control-Allow-Origin", "*")
	}

	if header.Get("Access-Control-Allow-Methods") == "" {
		header.Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
	}

	if header.Get("Access-Control-Allow-Headers") == "" {
		header.Set("Access-Control-Allow-Headers", "Content-Type, X-Grpc-Web, X-User-Agent, grpc-timeout")
	}
}

// containsHeader checks if a comma-separated header list contains a specific header
func containsHeader(headerList, header string) bool {
	// Split by comma and check each part
	parts := strings.Split(headerList, ",")
	for _, part := range parts {
		if strings.TrimSpace(part) == header {
			return true
		}
	}
	return false
}

// Flush implements http.Flusher for streaming support
func (w *GRPCHeaderResponseWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Hijack implements http.Hijacker for WebSocket upgrade support
func (w *GRPCHeaderResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hj, ok := w.ResponseWriter.(http.Hijacker); ok {
		return hj.Hijack()
	}
	return nil, nil, http.ErrNotSupported
}
