package middleware

import "net/http"

// SecurityHeaders adds security headers to all responses as per NEXUS_SECURITY_IMPL.md.
// Headers added:
//   - Strict-Transport-Security: Prevents protocol downgrade attacks
//   - X-Content-Type-Options: Prevents MIME sniffing
//   - X-Frame-Options: Prevents clickjacking
//   - Cache-Control: Prevents sensitive data caching
//
// These headers are set BEFORE the handler runs, allowing handlers to override
// Cache-Control if needed (e.g., for static assets).
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Set security headers
		// These are set first so handlers can override if needed
		w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Cache-Control", "no-store")

		next.ServeHTTP(w, r)
	})
}
