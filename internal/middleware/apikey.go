package middleware

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/nstalgic/nekzus/internal/metrics"
	"github.com/nstalgic/nekzus/internal/storage"
	"github.com/nstalgic/nekzus/internal/types"
)

var log = slog.With("package", "middleware")

// Context key for storing API key info
type contextKey string

const apiKeyContextKey contextKey = "apiKey"

// extractAPIKey retrieves the API key from Authorization header or X-API-Key header
func extractAPIKey(r *http.Request) string {
	// Try Authorization header first (Bearer token)
	authHeader := r.Header.Get("Authorization")
	if len(authHeader) > 7 && strings.HasPrefix(authHeader, "Bearer ") {
		token := strings.TrimSpace(authHeader[7:])
		// Check if it looks like an API key (starts with nekzus_)
		if strings.HasPrefix(token, "nekzus_") {
			return token
		}
	}

	// Try X-API-Key header
	apiKey := r.Header.Get("X-API-Key")
	if apiKey != "" {
		return strings.TrimSpace(apiKey)
	}

	return ""
}

// hashAPIKey creates SHA256 hash of API key for lookup
func HashAPIKey(key string) string {
	hash := sha256.Sum256([]byte(key))
	return hex.EncodeToString(hash[:])
}

// GetAPIKeyFromContext retrieves the API key from request context
func GetAPIKeyFromContext(ctx context.Context) *types.APIKey {
	apiKey, ok := ctx.Value(apiKeyContextKey).(*types.APIKey)
	if !ok {
		return nil
	}
	return apiKey
}

// NewAPIKeyAuth creates middleware that validates API keys
// This middleware is optional - if no API key is provided, the request passes through
// If an API key IS provided, it must be valid
func NewAPIKeyAuth(store *storage.Store, m *metrics.Metrics) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Extract API key if present
			apiKeyStr := extractAPIKey(r)

			// If no API key provided, pass through (JWT middleware will handle auth)
			if apiKeyStr == "" {
				next.ServeHTTP(w, r)
				return
			}

			// API key provided - must validate it
			if store == nil {
				http.Error(w, "API key authentication not available", http.StatusServiceUnavailable)
				return
			}

			// Hash the API key for lookup
			keyHash := HashAPIKey(apiKeyStr)

			// Look up API key in storage
			apiKey, err := store.GetAPIKeyByHash(keyHash)
			if err != nil {
				log.Error("Error looking up API key", "error", err)
				if m != nil {
					m.RecordJWTValidation("error_apikey_lookup")
				}
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}

			// API key not found
			if apiKey == nil {
				if m != nil {
					m.RecordJWTValidation("error_apikey_invalid")
				}
				http.Error(w, "invalid API key", http.StatusUnauthorized)
				return
			}

			// Check if revoked
			if apiKey.RevokedAt != nil {
				if m != nil {
					m.RecordJWTValidation("error_apikey_revoked")
				}
				http.Error(w, "API key has been revoked", http.StatusUnauthorized)
				return
			}

			// Check if expired
			if apiKey.ExpiresAt != nil && apiKey.ExpiresAt.Before(time.Now()) {
				if m != nil {
					m.RecordJWTValidation("error_apikey_expired")
				}
				http.Error(w, "API key has expired", http.StatusUnauthorized)
				return
			}

			// Valid API key - record success
			if m != nil {
				m.RecordJWTValidation("success_apikey")
			}

			// Update last used timestamp asynchronously with timeout
			// This prevents goroutines from hanging on shutdown
			go func(keyID string) {
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer cancel()

				// Use select to respect timeout
				select {
				case <-ctx.Done():
					if ctx.Err() == context.DeadlineExceeded {
						log.Warn("Timeout updating API key last_used", "keyID", keyID)
					}
					return
				default:
					if err := store.UpdateAPIKeyLastUsed(keyID); err != nil {
						log.Warn("Failed to update API key last_used", "keyID", keyID, "error", err)
					}
				}
			}(apiKey.ID)

			// Add API key to request context
			ctx := context.WithValue(r.Context(), apiKeyContextKey, apiKey)
			r = r.WithContext(ctx)

			next.ServeHTTP(w, r)
		})
	}
}
