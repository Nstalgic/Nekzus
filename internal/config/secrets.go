package config

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log/slog"

	"github.com/nstalgic/nekzus/internal/types"
)

// SecretStore defines the interface for retrieving and storing system secrets.
// This allows the config package to work with the storage layer without a direct dependency.
type SecretStore interface {
	GetSystemSecret(key string) (string, error)
	SetSystemSecret(key, value string) error
}

const (
	// JWTSecretKey is the key used to store the JWT secret in the database
	JWTSecretKey = "jwt_hs256_secret"

	// JWTSecretLength is the number of random bytes to generate for the JWT secret
	// 32 bytes = 256 bits of entropy, which produces a ~43 character base64 string
	JWTSecretLength = 32

	// MinJWTSecretLength is the minimum required length for a JWT secret
	MinJWTSecretLength = 32

	// CookieEncryptionKey is the key used to store the cookie encryption secret in the database
	CookieEncryptionKey = "cookie_encryption_key"

	// CookieEncryptionKeyLength is the number of bytes for AES-256-GCM key (32 bytes = 256 bits)
	CookieEncryptionKeyLength = 32
)

// EnsureJWTSecret ensures that a JWT secret is available in the config.
// If the config already has a secret, it's validated and used.
// If no secret is provided, it checks the database for an existing one.
// If no secret exists in the database, a new one is generated and stored.
//
// Returns the (possibly updated) config and any error encountered.
func EnsureJWTSecret(cfg *types.ServerConfig, store SecretStore) error {
	// If user provided a secret via config or env var, use it
	if cfg.Auth.HS256Secret != "" {
		if len(cfg.Auth.HS256Secret) < MinJWTSecretLength {
			return fmt.Errorf("JWT secret must be at least %d characters, got %d",
				MinJWTSecretLength, len(cfg.Auth.HS256Secret))
		}
		log.Info("using JWT secret from configuration")
		return nil
	}

	// No secret in config, try to get from storage
	existingSecret, err := store.GetSystemSecret(JWTSecretKey)
	if err != nil {
		return fmt.Errorf("failed to check for existing JWT secret: %w", err)
	}

	if existingSecret != "" {
		// Found existing secret in database
		cfg.Auth.HS256Secret = existingSecret
		log.Info("using JWT secret from database")
		return nil
	}

	// No secret anywhere, generate a new one
	newSecret, err := generateSecureSecret(JWTSecretLength)
	if err != nil {
		return fmt.Errorf("failed to generate JWT secret: %w", err)
	}

	// Store it in the database for persistence
	if err := store.SetSystemSecret(JWTSecretKey, newSecret); err != nil {
		return fmt.Errorf("failed to store generated JWT secret: %w", err)
	}

	cfg.Auth.HS256Secret = newSecret
	log.Info("auto-generated JWT secret (stored in database)",
		slog.String("note", "this secret will persist across restarts"))

	return nil
}

// generateSecureSecret generates a cryptographically secure random string.
func generateSecureSecret(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}

// GetCookieEncryptionKey retrieves or generates the cookie encryption key.
// The key is stored in the database as a base64-encoded string.
// Returns raw bytes suitable for AES-256-GCM encryption.
func GetCookieEncryptionKey(store SecretStore) ([]byte, error) {
	// Try to get existing key from storage
	existingKey, err := store.GetSystemSecret(CookieEncryptionKey)
	if err != nil {
		return nil, fmt.Errorf("failed to check for existing cookie encryption key: %w", err)
	}

	if existingKey != "" {
		// Decode the base64 key
		key, err := base64.RawURLEncoding.DecodeString(existingKey)
		if err != nil {
			return nil, fmt.Errorf("failed to decode cookie encryption key: %w", err)
		}

		if len(key) != CookieEncryptionKeyLength {
			return nil, fmt.Errorf("cookie encryption key has invalid length: got %d, want %d", len(key), CookieEncryptionKeyLength)
		}

		log.Info("using cookie encryption key from database")
		return key, nil
	}

	// No key exists, generate a new one
	newKey, err := generateSecureSecret(CookieEncryptionKeyLength)
	if err != nil {
		return nil, fmt.Errorf("failed to generate cookie encryption key: %w", err)
	}

	// Store it in the database for persistence
	if err := store.SetSystemSecret(CookieEncryptionKey, newKey); err != nil {
		return nil, fmt.Errorf("failed to store generated cookie encryption key: %w", err)
	}

	// Decode and return the key
	key, err := base64.RawURLEncoding.DecodeString(newKey)
	if err != nil {
		return nil, fmt.Errorf("failed to decode generated cookie encryption key: %w", err)
	}

	log.Info("auto-generated cookie encryption key (stored in database)",
		slog.String("note", "this key will persist across restarts"))

	return key, nil
}
