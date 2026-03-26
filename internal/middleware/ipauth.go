package middleware

import (
	"bufio"
	"context"
	"net"
	"net/http"
	"time"

	"github.com/nstalgic/nekzus/internal/auth"
	"github.com/nstalgic/nekzus/internal/httputil"
	"github.com/nstalgic/nekzus/internal/metrics"
	"github.com/nstalgic/nekzus/internal/storage"
)

// hijackableResponseWriter wraps http.ResponseWriter and implements http.Hijacker
// This is necessary for WebSocket upgrades to work properly through middleware
type hijackableResponseWriter struct {
	http.ResponseWriter
}

// Hijack implements http.Hijacker interface
func (h *hijackableResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := h.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, http.ErrNotSupported
	}
	return hijacker.Hijack()
}

// Ensure hijackableResponseWriter implements required interfaces
var _ http.Hijacker = (*hijackableResponseWriter)(nil)

// isLocalRequest checks if the request originates from localhost or a private IP range
func isLocalRequest(r *http.Request) bool {
	// Extract IP from RemoteAddr
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		// If no port, try parsing as-is
		host = r.RemoteAddr
	}

	ip := net.ParseIP(host)
	if ip == nil {
		// Failed to parse IP, treat as external for security
		log.Warn("Failed to parse IP from RemoteAddr", "remoteAddr", r.RemoteAddr)
		return false
	}

	// Check if IP is loopback (127.0.0.0/8 for IPv4, ::1 for IPv6)
	if ip.IsLoopback() {
		return true
	}

	// Check if IP is in private ranges
	// IPv4: 10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16
	// IPv6: fc00::/7 (which includes fd00::/8)
	if ip.IsPrivate() {
		return true
	}

	// Explicitly check for Docker bridge networks (172.17.0.0/16 to 172.31.0.0/16)
	// Docker typically uses 172.17-172.31 ranges for bridge networks
	if ip.To4() != nil {
		ip4 := ip.To4()
		// Check if in 172.16.0.0/12 range (172.16.0.0 - 172.31.255.255)
		if ip4[0] == 172 && ip4[1] >= 16 && ip4[1] <= 31 {
			return true
		}
	}

	return false
}

// NewIPBasedAuth creates middleware that conditionally requires JWT based on request origin.
// Local requests (localhost/LAN) with JWT tokens are validated and tracked.
// Local requests without JWT tokens are allowed (for browser/admin UI).
// External requests always require valid JWT authentication.
// Now adds device ID to context like strictJWT does
func NewIPBasedAuth(authMgr *auth.Manager, store *storage.Store, m *metrics.Metrics) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Wrap ResponseWriter to preserve Hijacker interface for WebSocket upgrades
			hw := &hijackableResponseWriter{ResponseWriter: w}

			// Extract JWT token if present
			token := httputil.ExtractBearerToken(r)

			// Check if request is from local network
			if isLocalRequest(r) {
				// Local request with JWT token - validate and track activity
				if token != "" {
					_, claims, err := authMgr.ParseJWT(token)
					if err != nil {
						// Token is invalid or expired — still allow the local request through
						// (local network is trusted) but skip device activity tracking.
						// This prevents the mobile app from getting 401s on local network
						// when its token expires, which would trigger a credential wipe.
						log.Debug("Local request with invalid/expired JWT, allowing without device tracking", "error", err)
						if m != nil {
							m.RecordLocalAuth("expired_jwt_passthrough")
						}
						next.ServeHTTP(hw, r)
						return
					}

					// Valid JWT from local network - track device activity
					if m != nil {
						m.RecordJWTValidation("success")
					}

					// Add device ID to context
					if deviceID, ok := claims["sub"].(string); ok && deviceID != "" {
						ctx := context.WithValue(r.Context(), deviceIDContextKey, deviceID)
						r = r.WithContext(ctx)

						// Update device last_seen asynchronously with context timeout
						if store != nil {
							go func(ctx context.Context, id string) {
								// Create context with timeout for database operation
								dbCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
								defer cancel()

								// Use context-aware update (if request cancelled, stop early)
								select {
								case <-dbCtx.Done():
									if dbCtx.Err() == context.Canceled {
										// Request was cancelled, skip update
										return
									}
									log.Warn("Timeout updating device last_seen", "deviceID", id)
								default:
									if err := store.UpdateDeviceLastSeen(id); err != nil {
										log.Warn("Failed to update device last_seen", "deviceID", id, "error", err)
									}
								}
							}(r.Context(), deviceID)
						}
					}
				} else {
					// Local request without JWT - allow (for browser/admin UI)
					if m != nil {
						m.RecordLocalAuth("success")
					}
				}

				next.ServeHTTP(hw, r)
				return
			}

			// External request - require JWT validation
			if token == "" {
				if m != nil {
					m.RecordJWTValidation("error_no_token")
				}
				http.Error(hw, "missing bearer token", http.StatusUnauthorized)
				return
			}

			// Parse and validate JWT
			_, claims, err := authMgr.ParseJWT(token)
			if err != nil {
				if m != nil {
					m.RecordJWTValidation("error_invalid")
				}
				http.Error(hw, "invalid token: "+err.Error(), http.StatusUnauthorized)
				return
			}

			// Record successful validation
			if m != nil {
				m.RecordJWTValidation("success")
			}

			// Add device ID to context for external requests too
			// Track device activity if storage is available
			if deviceID, ok := claims["sub"].(string); ok && deviceID != "" {
				ctx := context.WithValue(r.Context(), deviceIDContextKey, deviceID)
				r = r.WithContext(ctx)

				// Add context timeout for external auth goroutines
				if store != nil {
					// Update last_seen asynchronously with timeout to avoid blocking request
					go func(id string) {
						ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
						defer cancel()

						// Use select to respect timeout
						select {
						case <-ctx.Done():
							if ctx.Err() == context.DeadlineExceeded {
								log.Warn("Timeout updating device last_seen", "deviceID", id)
							}
							return
						default:
							if err := store.UpdateDeviceLastSeen(id); err != nil {
								log.Warn("Failed to update device last_seen", "deviceID", id, "error", err)
							}
						}
					}(deviceID)
				}
			}

			next.ServeHTTP(hw, r)
		})
	}
}
