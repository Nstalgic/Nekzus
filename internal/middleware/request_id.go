package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
)

// Context key for storing request ID
type requestIDContextKey struct{}

// RequestIDHeader is the standard header name for request ID
const RequestIDHeader = "X-Request-ID"

// RequestID creates middleware that adds a unique request ID to each request
// Add request ID tracking for distributed tracing and debugging
func RequestID() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Check if request already has an ID (from upstream proxy)
			requestID := r.Header.Get(RequestIDHeader)

			// Generate new ID if not present
			if requestID == "" {
				requestID = generateRequestID()
			}

			// Set request ID in response header
			w.Header().Set(RequestIDHeader, requestID)

			// Add to request context
			ctx := context.WithValue(r.Context(), requestIDContextKey{}, requestID)
			r = r.WithContext(ctx)

			next.ServeHTTP(w, r)
		})
	}
}

// GetRequestIDFromContext retrieves the request ID from request context
func GetRequestIDFromContext(ctx context.Context) string {
	requestID, ok := ctx.Value(requestIDContextKey{}).(string)
	if !ok {
		return ""
	}
	return requestID
}

// generateRequestID creates a cryptographically secure random ID
// Format: 16 hex characters (8 bytes = 64 bits of randomness)
func generateRequestID() string {
	b := make([]byte, 8)
	_, err := rand.Read(b)
	if err != nil {
		// Fallback to a timestamp-based ID if crypto/rand fails
		// This should never happen in practice
		return "error-fallback-id"
	}
	return hex.EncodeToString(b)
}
