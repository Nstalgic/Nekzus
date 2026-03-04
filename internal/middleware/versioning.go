package middleware

import (
	"fmt"
	"net/http"
	"time"
)

// APIVersion adds API version header to all responses
// This helps clients identify which version of the API they're using
func APIVersion(version string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-API-Version", version)
			next.ServeHTTP(w, r)
		})
	}
}

// DeprecationOptions configures deprecation headers
type DeprecationOptions struct {
	// Sunset is the date when the endpoint will be removed (format: "2026-01-01")
	Sunset string

	// SuccessorPath is the path to the new endpoint (e.g., "/api/v2/devices")
	SuccessorPath string

	// DeprecationMsg is a human-readable message about the deprecation
	DeprecationMsg string
}

// Deprecated marks an endpoint as deprecated and adds appropriate headers
// Follows RFC 8594 (Deprecation header) and RFC 9110 (Sunset header)
// Fixed to use HTTP-date format for Deprecation header per RFC 8594
func Deprecated(opts DeprecationOptions) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Add Deprecation header (RFC 8594)
			// Value should be "true" or a quoted HTTP-date
			if opts.Sunset != "" {
				sunsetTime := parseSunsetDate(opts.Sunset)
				if !sunsetTime.IsZero() {
					// RFC 8594 specifies the date in HTTP-date format (quoted)
					w.Header().Set("Deprecation", fmt.Sprintf("@%d", sunsetTime.Unix()))
				} else {
					w.Header().Set("Deprecation", "true")
				}
			} else {
				w.Header().Set("Deprecation", "true")
			}

			// Add Sunset header (RFC 8594) - when the endpoint will be removed
			if opts.Sunset != "" {
				sunsetTime := parseSunsetDate(opts.Sunset)
				if !sunsetTime.IsZero() {
					// HTTP-date format (RFC 9110)
					w.Header().Set("Sunset", sunsetTime.Format(http.TimeFormat))
				}
			}

			// Add Link header pointing to successor endpoint
			if opts.SuccessorPath != "" {
				w.Header().Set("Link", fmt.Sprintf("<%s>; rel=\"successor-version\"", opts.SuccessorPath))
			}

			// Add Warning header (RFC 9110) for additional deprecation info
			if opts.DeprecationMsg != "" {
				// Warning format: 299 - "message"
				warning := fmt.Sprintf("299 - \"%s\"", opts.DeprecationMsg)
				w.Header().Set("Warning", warning)
			}

			next.ServeHTTP(w, r)
		})
	}
}

// parseSunsetDate parses sunset date string (YYYY-MM-DD) to time.Time
func parseSunsetDate(sunset string) time.Time {
	t, err := time.Parse("2006-01-02", sunset)
	if err != nil {
		return time.Time{}
	}
	return t
}
