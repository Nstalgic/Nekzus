package middleware

import (
	"net/http"
)

// DefaultMaxBodySize is the default maximum request body size (1MB)
const DefaultMaxBodySize = 1 << 20 // 1MB

// LimitRequestBody creates middleware that limits the maximum request body size
// Add request body size limiting to prevent memory exhaustion attacks
func LimitRequestBody(maxSize int64) func(http.Handler) http.Handler {
	if maxSize <= 0 {
		maxSize = DefaultMaxBodySize
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Use http.MaxBytesReader which returns an error when limit is exceeded
			// This automatically sends a 413 Request Entity Too Large response
			r.Body = http.MaxBytesReader(w, r.Body, maxSize)

			next.ServeHTTP(w, r)
		})
	}
}
