package middleware

import (
	"net/http"
	"strconv"

	"github.com/nstalgic/nekzus/internal/httputil"
	"github.com/nstalgic/nekzus/internal/ratelimit"
)

// RateLimit creates middleware that limits requests per IP address
// Adds RFC 6585 RateLimit-* headers
func RateLimit(limiter *ratelimit.Limiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Extract client IP
			clientIP := httputil.ExtractClientIP(r)

			// Get rate limit state for RFC 6585 headers
			state := limiter.GetState(clientIP)

			// Always set RateLimit headers (RFC 6585 draft)
			w.Header().Set("RateLimit-Limit", strconv.Itoa(state.Limit))
			w.Header().Set("RateLimit-Remaining", strconv.Itoa(state.Remaining))
			w.Header().Set("RateLimit-Reset", strconv.FormatInt(state.ResetAt, 10))

			// Check rate limit
			if !limiter.Allow(clientIP) {
				w.Header().Set("Retry-After", "1")
				http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
