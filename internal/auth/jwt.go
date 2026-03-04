package auth

import (
	"crypto/subtle"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	jwt "github.com/golang-jwt/jwt/v5"
	apperrors "github.com/nstalgic/nekzus/internal/errors"
)

// Manager handles authentication operations
type Manager struct {
	mu         sync.RWMutex
	jwtSecret  []byte
	issuer     string
	audience   string
	bootstrap  *BootstrapStore
	revocation *RevocationList
}

// NewManager creates a new auth manager
func NewManager(jwtSecret []byte, issuer, audience string, bootstrapTokens []string) (*Manager, error) {
	if err := validateJWTSecret(string(jwtSecret)); err != nil {
		return nil, err
	}

	// Call ValidateBootstrapAllowed in NewManager
	if err := ValidateBootstrapAllowed(); err != nil {
		return nil, err
	}

	if issuer == "" {
		issuer = "nekzus"
	}
	if audience == "" {
		audience = "nekzus-mobile"
	}

	return &Manager{
		jwtSecret:  jwtSecret,
		issuer:     issuer,
		audience:   audience,
		bootstrap:  NewBootstrapStore(bootstrapTokens),
		revocation: NewRevocationList(),
	}, nil
}

// SignJWT creates a signed JWT token for a device with given scopes and TTL
func (m *Manager) SignJWT(deviceID string, scopes []string, ttl time.Duration) (string, error) {
	if deviceID == "" {
		return "", errors.New("device ID is required")
	}

	now := time.Now()
	claims := jwt.MapClaims{
		"iss":    m.issuer,
		"aud":    m.audience,
		"sub":    deviceID,
		"scopes": scopes,
		"iat":    now.Unix(),
		"exp":    now.Add(ttl).Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(m.jwtSecret)
	if err != nil {
		return "", fmt.Errorf("failed to sign token: %w", err)
	}

	return signed, nil
}

// ParseJWT validates and parses a JWT token
func (m *Manager) ParseJWT(tokenString string) (*jwt.Token, jwt.MapClaims, error) {
	if tokenString == "" {
		return nil, nil, apperrors.ErrInvalidToken
	}

	parser := jwt.NewParser(jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Name}))
	var claims jwt.MapClaims

	token, err := parser.ParseWithClaims(tokenString, &claims, func(t *jwt.Token) (interface{}, error) {
		return m.jwtSecret, nil
	})

	if err != nil {
		return nil, nil, apperrors.Wrap(err, "INVALID_TOKEN", "Failed to parse token", 401)
	}

	if !token.Valid {
		return nil, nil, apperrors.ErrInvalidToken
	}

	// Verify issuer
	if iss, ok := claims["iss"].(string); !ok || iss != m.issuer {
		return nil, nil, apperrors.Wrap(
			fmt.Errorf("expected issuer %s, got %s", m.issuer, iss),
			"INVALID_ISSUER",
			"Token issuer mismatch",
			401,
		)
	}

	// Verify audience
	if aud, ok := claims["aud"].(string); !ok || aud != m.audience {
		return nil, nil, apperrors.Wrap(
			fmt.Errorf("expected audience %s, got %s", m.audience, aud),
			"INVALID_AUDIENCE",
			"Token audience mismatch",
			401,
		)
	}

	// Check if token is revoked (using token string as JTI for simplicity)
	// In production, you'd use a proper JTI claim
	if m.revocation != nil && m.revocation.IsRevoked(tokenString) {
		return nil, nil, apperrors.NewWithCode(
			"TOKEN_REVOKED",
			apperrors.CodeTokenRevoked,
			"This token has been revoked",
			401,
		)
	}

	// Check if device is revoked
	if deviceID, ok := claims["sub"].(string); ok && deviceID != "" {
		if m.revocation != nil && m.revocation.IsDeviceRevoked(deviceID) {
			return nil, nil, apperrors.NewWithCode(
				"DEVICE_REVOKED",
				apperrors.CodeDeviceRevoked,
				"This device has been revoked",
				401,
			)
		}
	}

	return token, claims, nil
}

// ValidateBootstrap validates a bootstrap token
func (m *Manager) ValidateBootstrap(token string) bool {
	m.mu.RLock()
	bootstrap := m.bootstrap
	m.mu.RUnlock()

	return bootstrap.Validate(token)
}

// GenerateShortLivedToken creates a temporary bootstrap token
func (m *Manager) GenerateShortLivedToken(ttl time.Duration) (string, error) {
	m.mu.RLock()
	bootstrap := m.bootstrap
	m.mu.RUnlock()

	return bootstrap.GenerateShortLived(ttl)
}

// RecordFailedPairing records a failed pairing attempt for a bootstrap token
func (m *Manager) RecordFailedPairing(token string) {
	m.mu.RLock()
	bootstrap := m.bootstrap
	m.mu.RUnlock()

	bootstrap.RecordFailedPairing(token)
}

// RecordSuccessfulPairing records a successful pairing and consumes the token
func (m *Manager) RecordSuccessfulPairing(token string) {
	m.mu.RLock()
	bootstrap := m.bootstrap
	m.mu.RUnlock()

	bootstrap.RecordSuccessfulPairing(token)
}

// IsBootstrapRateLimited returns true if the bootstrap token has been rate limited
func (m *Manager) IsBootstrapRateLimited(token string) bool {
	m.mu.RLock()
	bootstrap := m.bootstrap
	m.mu.RUnlock()

	return bootstrap.IsRateLimited(token)
}

// UpdateBootstrapTokens updates the bootstrap token list (for hot reload)
// Protected with mutex and stops old store
func (m *Manager) UpdateBootstrapTokens(tokens []string) {
	// Create new bootstrap store before acquiring lock
	newBootstrap := NewBootstrapStore(tokens)

	m.mu.Lock()
	oldBootstrap := m.bootstrap
	m.bootstrap = newBootstrap
	m.mu.Unlock()

	// Stop the old bootstrap store after swap
	if oldBootstrap != nil {
		oldBootstrap.Stop()
	}
}

// Stop stops the bootstrap store and revocation list cleanup goroutines
func (m *Manager) Stop() {
	m.mu.RLock()
	bootstrap := m.bootstrap
	revocation := m.revocation
	m.mu.RUnlock()

	if bootstrap != nil {
		bootstrap.Stop()
	}
	if revocation != nil {
		revocation.Stop()
	}
}

// RevokeToken adds a token to the revocation list.
// The token will be automatically removed after expiry.
func (m *Manager) RevokeToken(tokenString string, expiry time.Time) {
	if m.revocation != nil {
		m.revocation.Revoke(tokenString, expiry)
	}
}

// RevokeDevice revokes all tokens for a device.
// All future validation for this device will fail until expiry.
func (m *Manager) RevokeDevice(deviceID string, expiry time.Time) {
	if m.revocation != nil {
		m.revocation.RevokeDevice(deviceID, expiry)
	}
}

// IsTokenRevoked checks if a token has been revoked.
func (m *Manager) IsTokenRevoked(tokenString string) bool {
	if m.revocation == nil {
		return false
	}
	return m.revocation.IsRevoked(tokenString)
}

// IsDeviceRevoked checks if a device has been revoked.
func (m *Manager) IsDeviceRevoked(deviceID string) bool {
	if m.revocation == nil {
		return false
	}
	return m.revocation.IsDeviceRevoked(deviceID)
}

// RevocationStats returns statistics about the revocation list.
func (m *Manager) RevocationStats() map[string]int {
	if m.revocation == nil {
		return map[string]int{}
	}
	return m.revocation.Stats()
}

// ExtractScopes extracts scopes from JWT claims
func ExtractScopes(claims jwt.MapClaims) []string {
	scopeInterface, ok := claims["scopes"]
	if !ok {
		return []string{}
	}

	scopeList, ok := scopeInterface.([]interface{})
	if !ok {
		return []string{}
	}

	scopes := make([]string, 0, len(scopeList))
	for _, scope := range scopeList {
		if scopeStr, ok := scope.(string); ok {
			scopes = append(scopes, scopeStr)
		}
	}

	return scopes
}

// validateJWTSecret ensures the JWT secret meets security requirements
func validateJWTSecret(secret string) error {
	if len(secret) < 32 {
		return errors.New("JWT secret must be at least 32 characters")
	}

	// Check for common weak secrets
	weakSecrets := []string{"secret", "password", "change-me", "dev", "test", "example"}
	lowerSecret := strings.ToLower(secret)
	for _, weak := range weakSecrets {
		if strings.Contains(lowerSecret, weak) && !isDevEnvironment() {
			return fmt.Errorf("JWT secret contains weak pattern '%s'", weak)
		}
	}

	return nil
}

// isDevEnvironment checks if running in development mode
func isDevEnvironment() bool {
	env := strings.ToLower(os.Getenv("ENVIRONMENT"))
	return env == "development" || env == "dev" || env == "test"
}

// ValidateBootstrapAllowed ensures bootstrap bypass is not enabled in production
// This function implements a "fail closed" security model: if ENVIRONMENT is not set
// and bypass is attempted, we assume production and reject the bypass.
func ValidateBootstrapAllowed() error {
	allowAny := os.Getenv("NEKZUS_BOOTSTRAP_ALLOW_ANY") == "1"
	if !allowAny {
		return nil // Not trying to bypass, safe to proceed
	}

	// If trying to bypass, REQUIRE explicit dev environment
	env := strings.ToLower(os.Getenv("ENVIRONMENT"))
	if env == "" {
		// Fail closed: if ENVIRONMENT not set, assume production and reject
		return errors.New("NEKZUS_BOOTSTRAP_ALLOW_ANY requires explicit ENVIRONMENT=development")
	}

	// Only allow bypass in development, dev, or test environments
	if env != "development" && env != "dev" && env != "test" {
		return fmt.Errorf("NEKZUS_BOOTSTRAP_ALLOW_ANY not allowed in environment: %s", env)
	}

	// Log warning when bypass is enabled (helps detect accidental enablement)
	log.Warn("Bootstrap token bypass enabled", "environment", env)
	return nil
}

// DetermineScopes determines appropriate scopes based on device information
// Test tokens are now restricted
func DetermineScopes(devicePlatform string) []string {
	baseScopes := []string{"read:catalog", "read:events"}

	switch strings.ToLower(devicePlatform) {
	case "ios", "android", "web":
		// Mobile apps and embedded webviews get mobile-level access
		// Note: "web" platform is for webviews embedded in mobile apps (mobile-first architecture)
		// access:* wildcard allows access to all proxied services (access:memos, access:grafana, etc.)
		return append(baseScopes, "access:mobile", "access:*", "read:*")
	case "test":
		// Test platform gets read-only access
		return append(baseScopes, "read:*")
	case "testcontainers":
		// Testcontainers get read access plus deployment write
		return append(baseScopes, "read:*", "write:deployments")
	case "debug":
		// Debug platform requires NEKZUS_DEBUG_TOKENS env var
		if os.Getenv("NEKZUS_DEBUG_TOKENS") == "1" {
			return append(baseScopes, "access:admin", "read:*", "write:*")
		}
		// Without env var, get minimal access
		return baseScopes
	default:
		// Unknown platforms get minimal access
		return baseScopes
	}
}

// ConstantTimeCompare performs constant-time comparison to prevent timing attacks
func ConstantTimeCompare(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
