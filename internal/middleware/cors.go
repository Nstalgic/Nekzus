package middleware

import (
	"net/http"
	"strings"
)

// CORSOptions configures CORS middleware behavior
type CORSOptions struct {
	// AllowedOrigins is a list of origins that are allowed to make cross-origin requests
	AllowedOrigins []string

	// AllowedMethods is a list of methods the client is allowed to use
	AllowedMethods []string

	// AllowedHeaders is a list of non-simple headers the client is allowed to use
	AllowedHeaders []string

	// ExposedHeaders is a list of headers that are safe to expose to the API
	ExposedHeaders []string

	// AllowCredentials indicates whether the request can include user credentials
	AllowCredentials bool

	// MaxAge indicates how long (in seconds) the results of a preflight request can be cached
	MaxAge int
}

// DefaultCORSOptions returns sensible default CORS options
func DefaultCORSOptions() CORSOptions {
	return CORSOptions{
		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS", "PATCH"},
		AllowedHeaders: []string{"Content-Type", "Authorization", "X-Request-ID", "X-Device-ID"},
		ExposedHeaders: []string{"X-API-Version", "X-Request-ID", "RateLimit-Limit", "RateLimit-Remaining", "RateLimit-Reset"},
		MaxAge:         86400, // 24 hours
	}
}

// CORS creates middleware that handles CORS headers
// Add CORS middleware for web UI support
func CORS(opts CORSOptions) func(http.Handler) http.Handler {
	// Build allowed origins map for O(1) lookup
	allowedOrigins := make(map[string]bool)
	allowAll := false
	for _, origin := range opts.AllowedOrigins {
		if origin == "*" {
			allowAll = true
		}
		allowedOrigins[origin] = true
	}

	// Pre-build header values
	methodsHeader := strings.Join(opts.AllowedMethods, ", ")
	headersHeader := strings.Join(opts.AllowedHeaders, ", ")
	exposedHeader := strings.Join(opts.ExposedHeaders, ", ")
	maxAgeHeader := ""
	if opts.MaxAge > 0 {
		maxAgeHeader = itoa(opts.MaxAge)
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			// Check if origin is allowed
			if origin != "" {
				if allowAll {
					w.Header().Set("Access-Control-Allow-Origin", origin)
				} else if allowedOrigins[origin] {
					w.Header().Set("Access-Control-Allow-Origin", origin)
				}

				// Set Vary header to indicate origin-dependent response
				w.Header().Add("Vary", "Origin")

				// Set credentials header if enabled
				if opts.AllowCredentials {
					w.Header().Set("Access-Control-Allow-Credentials", "true")
				}

				// Set exposed headers
				if exposedHeader != "" {
					w.Header().Set("Access-Control-Expose-Headers", exposedHeader)
				}
			}

			// Handle preflight requests
			if r.Method == http.MethodOptions {
				if origin != "" && (allowAll || allowedOrigins[origin]) {
					w.Header().Set("Access-Control-Allow-Methods", methodsHeader)
					w.Header().Set("Access-Control-Allow-Headers", headersHeader)
					if maxAgeHeader != "" {
						w.Header().Set("Access-Control-Max-Age", maxAgeHeader)
					}
				}
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// itoa converts int to string without importing strconv
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var result []byte
	negative := i < 0
	if negative {
		i = -i
	}
	for i > 0 {
		result = append([]byte{byte('0' + i%10)}, result...)
		i /= 10
	}
	if negative {
		result = append([]byte{'-'}, result...)
	}
	return string(result)
}
