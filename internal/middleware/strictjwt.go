package middleware

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	jwt "github.com/golang-jwt/jwt/v5"
	"github.com/nstalgic/nekzus/internal/auth"
	"github.com/nstalgic/nekzus/internal/metrics"
	"github.com/nstalgic/nekzus/internal/storage"
)

// Context key for storing device ID from JWT
type strictJWTContextKey string

const deviceIDContextKey strictJWTContextKey = "deviceID"

// NewStrictJWTAuth creates middleware that enforces JWT authentication with revocation checks.
// Unlike IP-based auth, this middleware ALWAYS requires a valid JWT token regardless of source IP.
// It also verifies that the device has not been revoked by checking storage.
//
// This middleware should be used for mobile app endpoints that must enforce strict authentication
// even for requests from local network IPs.
//
// Security guarantees:
// - JWT token is required (no IP-based bypass)
// - JWT signature and expiration are validated
// - Device revocation status is checked in storage
// - Revoked devices receive 401 Unauthorized
func NewStrictJWTAuth(authMgr *auth.Manager, store *storage.Store, m *metrics.Metrics) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Extract JWT token from Authorization header
			token := extractBearerToken(r)

			if token == "" {
				if m != nil {
					m.RecordJWTValidation("error_missing_token")
				}
				http.Error(w, "missing bearer token", http.StatusUnauthorized)
				return
			}

			// Validate JWT token
			_, claims, err := authMgr.ParseJWT(token)
			if err != nil {
				// Differentiate rejection reason for observability
				metricStatus, logMsg := classifyJWTError(err)
				if m != nil {
					m.RecordJWTValidation(metricStatus)
				}
				log.Error(logMsg, "error", err, "remote_addr", r.RemoteAddr)
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}

			// Extract device ID from JWT claims
			deviceID, ok := claims["sub"].(string)
			if !ok || deviceID == "" {
				if m != nil {
					m.RecordJWTValidation("error_missing_subject")
				}
				log.Error("StrictJWT: Token missing device ID in subject claim")
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}

			// Check if storage is available
			// Without storage, we cannot verify device revocation status
			if store == nil {
				log.Error("StrictJWT: Cannot verify device - storage not available", "deviceID", deviceID)
				if m != nil {
					m.RecordJWTValidation("error_storage_unavailable")
				}
				http.Error(w, "storage not available", http.StatusServiceUnavailable)
				return
			}

			// Check device revocation status in storage
			// This is the critical security check that prevents revoked devices from accessing endpoints
			device, err := store.GetDevice(deviceID)
			if err != nil {
				log.Error("StrictJWT: Error checking device", "deviceID", deviceID, "error", err)
				if m != nil {
					m.RecordJWTValidation("error_storage")
				}
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}

			if device == nil {
				// Device not found in storage - it has been revoked
				log.Error("StrictJWT: Device not found in storage (revoked)", "deviceID", deviceID)
				if m != nil {
					m.RecordJWTValidation("error_device_revoked")
				}
				http.Error(w, "device access revoked", http.StatusUnauthorized)
				return
			}

			// Update device last_seen timestamp asynchronously with timeout
			go func(ctx context.Context, id string) {
				// Create context with timeout for database operation
				dbCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer cancel()

				// Use context-aware update (if request cancelled, stop early)
				select {
				case <-dbCtx.Done():
					if dbCtx.Err() == context.Canceled {
						return
					}
					log.Warn("Timeout updating device last_seen", "deviceID", id)
				default:
					if err := store.UpdateDeviceLastSeen(id); err != nil {
						log.Warn("Failed to update last_seen for device", "deviceID", id, "error", err)
					}
				}
			}(r.Context(), deviceID)

			// Authentication successful - record metrics
			if m != nil {
				m.RecordJWTValidation("success")
			}

			// Add device ID to request context for downstream handlers
			ctx := context.WithValue(r.Context(), deviceIDContextKey, deviceID)
			r = r.WithContext(ctx)

			// Proceed to next handler
			next.ServeHTTP(w, r)
		})
	}
}

// extractBearerToken extracts the JWT token from the Authorization header
// Expected format: "Authorization: Bearer <token>"
func extractBearerToken(r *http.Request) string {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return ""
	}

	// Check if header starts with "Bearer "
	if !strings.HasPrefix(authHeader, "Bearer ") {
		return ""
	}

	// Extract token (everything after "Bearer ")
	token := strings.TrimSpace(authHeader[7:])
	return token
}

// GetDeviceIDFromContext retrieves the authenticated device ID from request context
// Returns empty string if not found
func GetDeviceIDFromContext(ctx context.Context) string {
	deviceID, ok := ctx.Value(deviceIDContextKey).(string)
	if !ok {
		return ""
	}
	return deviceID
}

// SetDeviceIDInContext adds a device ID to the context.
// This is primarily useful for testing handlers that depend on device ID.
func SetDeviceIDInContext(ctx context.Context, deviceID string) context.Context {
	return context.WithValue(ctx, deviceIDContextKey, deviceID)
}

// classifyJWTError maps a JWT parsing error to a metric status label and log message.
// This provides differentiated observability for expired, malformed, and signature errors.
func classifyJWTError(err error) (metricStatus string, logMsg string) {
	switch {
	case errors.Is(err, jwt.ErrTokenExpired):
		return "error_token_expired", "StrictJWT: Token expired"
	case errors.Is(err, jwt.ErrTokenMalformed):
		return "error_token_malformed", "StrictJWT: Token malformed"
	case errors.Is(err, jwt.ErrSignatureInvalid):
		return "error_signature_invalid", "StrictJWT: Token signature invalid"
	case errors.Is(err, jwt.ErrTokenNotValidYet):
		return "error_token_not_valid_yet", "StrictJWT: Token not valid yet"
	default:
		// Covers issuer/audience mismatch, revocation, and other parse failures
		return "error_invalid", "StrictJWT: Invalid token"
	}
}
