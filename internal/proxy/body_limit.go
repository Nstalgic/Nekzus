package proxy

import "net/http"

// DefaultMaxRequestBodySize is the default maximum request body size (100MB)
const DefaultMaxRequestBodySize int64 = 100 * 1024 * 1024

// MaxBodyReader wraps the request body with a size limiter.
// Returns the limited body that will return an error if the limit is exceeded.
// If maxBytes is <= 0, uses DefaultMaxRequestBodySize.
func MaxBodyReader(w http.ResponseWriter, r *http.Request, maxBytes int64) {
	if maxBytes <= 0 {
		maxBytes = DefaultMaxRequestBodySize
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
}
