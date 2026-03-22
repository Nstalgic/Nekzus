package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"
)

var log = slog.With("package", "auth")

// MaxPairingAttempts is the maximum number of failed pairing attempts per token
const MaxPairingAttempts = 3

// BootstrapStore manages bootstrap tokens for device pairing
type BootstrapStore struct {
	mu               sync.RWMutex
	permanentTokens  [][]byte
	shortLivedTokens map[string]time.Time
	failedAttempts   map[string]int  // Track failed pairing attempts per token
	rateLimited      map[string]bool // Track tokens that have been rate limited
	cleanupTicker    *time.Ticker
	stopCleanup      chan struct{}
	stopped          bool // Track if store has been stopped
}

// NewBootstrapStore creates a new bootstrap token store
func NewBootstrapStore(tokens []string) *BootstrapStore {
	bs := &BootstrapStore{
		permanentTokens:  make([][]byte, 0, len(tokens)),
		shortLivedTokens: make(map[string]time.Time),
		failedAttempts:   make(map[string]int),
		rateLimited:      make(map[string]bool),
		stopCleanup:      make(chan struct{}),
		stopped:          false,
	}

	// Convert permanent tokens to byte arrays for constant-time comparison
	for _, token := range tokens {
		if token != "" {
			bs.permanentTokens = append(bs.permanentTokens, []byte(token))
		}
	}

	// Start background cleanup
	bs.startCleanup(time.Minute)

	return bs
}

// Validate checks if a bootstrap token is valid
func (bs *BootstrapStore) Validate(token string) bool {
	if token == "" {
		log.Warn("Bootstrap validation failed - empty token")
		return false
	}

	bs.mu.RLock()
	shortLivedCount := len(bs.shortLivedTokens)
	permanentCount := len(bs.permanentTokens)
	bs.mu.RUnlock()

	log.Debug("bootstrap validation attempt",
		"token_prefix", token[:min(10, len(token))]+"...",
		"short_lived_tokens", shortLivedCount,
		"permanent_tokens", permanentCount,
		"has_env_token", os.Getenv("NEKZUS_BOOTSTRAP_TOKEN") != "")

	// Check short-lived tokens first
	if bs.validateShortLived(token) {
		log.Debug("Short-lived bootstrap token validated successfully")
		return true
	}

	// Check permanent tokens
	if bs.validatePermanent(token) {
		log.Debug("Permanent bootstrap token validated successfully")
		return true
	}

	// Check environment variable token
	if envToken := os.Getenv("NEKZUS_BOOTSTRAP_TOKEN"); envToken != "" {
		if subtle.ConstantTimeCompare([]byte(token), []byte(envToken)) == 1 {
			log.Debug("Environment bootstrap token validated successfully")
			return true
		}
	}

	// Development bypass (should be blocked in production by ValidateBootstrapAllowed)
	if os.Getenv("NEKZUS_BOOTSTRAP_ALLOW_ANY") == "1" {
		log.Warn("Bootstrap bypass enabled - accepting any token")
		return true
	}

	log.Warn("Bootstrap validation failed - invalid token",
		"checked_short_lived", shortLivedCount,
		"checked_permanent", permanentCount)
	return false
}

// validateShortLived checks short-lived tokens
func (bs *BootstrapStore) validateShortLived(token string) bool {
	bs.mu.RLock()
	expiry, exists := bs.shortLivedTokens[token]
	isRateLimited := bs.rateLimited[token]
	failedCount := bs.failedAttempts[token]
	bs.mu.RUnlock()

	// Check if rate limited (too many failed attempts)
	if isRateLimited {
		log.Debug("short-lived token rejected: rate limited",
			"failed_attempts", failedCount)
		return false
	}

	if !exists {
		log.Debug("short-lived token not found in store")
		return false
	}

	// Check if expired
	now := time.Now()
	if now.After(expiry) {
		log.Debug("short-lived token expired",
			"expired_ago", now.Sub(expiry).String())
		// Clean up expired token
		bs.mu.Lock()
		delete(bs.shortLivedTokens, token)
		delete(bs.failedAttempts, token)
		delete(bs.rateLimited, token)
		bs.mu.Unlock()
		return false
	}

	log.Debug("short-lived token valid",
		"ttl_remaining", expiry.Sub(now).String())
	return true
}

// validatePermanent checks permanent tokens using constant-time comparison
func (bs *BootstrapStore) validatePermanent(token string) bool {
	bs.mu.RLock()
	defer bs.mu.RUnlock()

	tokenBytes := []byte(token)
	for _, permToken := range bs.permanentTokens {
		if subtle.ConstantTimeCompare(tokenBytes, permToken) == 1 {
			return true
		}
	}

	return false
}

// GenerateShortLived creates a short-lived bootstrap token
func (bs *BootstrapStore) GenerateShortLived(ttl time.Duration) (string, error) {
	token, err := generateRandomToken(32)
	if err != nil {
		return "", fmt.Errorf("failed to generate token: %w", err)
	}

	token = "temp_" + token
	expiry := time.Now().Add(ttl)

	bs.mu.Lock()
	bs.shortLivedTokens[token] = expiry
	bs.mu.Unlock()

	return token, nil
}

// startCleanup starts a background goroutine to clean up expired tokens
func (bs *BootstrapStore) startCleanup(interval time.Duration) {
	bs.cleanupTicker = time.NewTicker(interval)

	go func() {
		for {
			select {
			case <-bs.cleanupTicker.C:
				bs.cleanupExpired()
			case <-bs.stopCleanup:
				return
			}
		}
	}()
}

// cleanupExpired removes expired short-lived tokens
func (bs *BootstrapStore) cleanupExpired() {
	bs.mu.Lock()
	defer bs.mu.Unlock()

	now := time.Now()
	expiredCount := 0

	for token, expiry := range bs.shortLivedTokens {
		if now.After(expiry) {
			delete(bs.shortLivedTokens, token)
			expiredCount++
		}
	}

	if expiredCount > 0 {
		log.Debug("Cleaned up expired bootstrap tokens", "count", expiredCount)
	}
}

// Stop stops the cleanup goroutine
// Protected against double stop with stopped flag
func (bs *BootstrapStore) Stop() {
	bs.mu.Lock()
	if bs.stopped {
		bs.mu.Unlock()
		return
	}
	bs.stopped = true
	bs.mu.Unlock()

	if bs.cleanupTicker != nil {
		bs.cleanupTicker.Stop()
	}
	close(bs.stopCleanup)
}

// Stats returns statistics about the bootstrap store
func (bs *BootstrapStore) Stats() map[string]int {
	bs.mu.RLock()
	defer bs.mu.RUnlock()

	return map[string]int{
		"permanent_tokens":   len(bs.permanentTokens),
		"short_lived_tokens": len(bs.shortLivedTokens),
	}
}

// RecordFailedPairing records a failed pairing attempt for a token.
// After MaxPairingAttempts (3) failed attempts, the token is invalidated.
func (bs *BootstrapStore) RecordFailedPairing(token string) {
	bs.mu.Lock()
	defer bs.mu.Unlock()

	bs.failedAttempts[token]++
	if bs.failedAttempts[token] >= MaxPairingAttempts {
		bs.rateLimited[token] = true
		// Also remove from short-lived tokens to invalidate it
		delete(bs.shortLivedTokens, token)
		log.Warn("Bootstrap token rate limited after too many failed attempts",
			"attempts", bs.failedAttempts[token])
	}
}

// RecordSuccessfulPairing records a successful pairing and consumes the token.
// Short-lived tokens are one-time use; after successful pairing, the token is invalidated.
func (bs *BootstrapStore) RecordSuccessfulPairing(token string) {
	bs.mu.Lock()
	defer bs.mu.Unlock()

	// Clear failure tracking
	delete(bs.failedAttempts, token)
	delete(bs.rateLimited, token)

	// Consume the token (short-lived tokens are one-time use)
	delete(bs.shortLivedTokens, token)
}

// IsRateLimited returns true if the token has been rate limited due to too many failed attempts.
func (bs *BootstrapStore) IsRateLimited(token string) bool {
	bs.mu.RLock()
	defer bs.mu.RUnlock()

	return bs.rateLimited[token]
}

// generateRandomToken generates a cryptographically random token
func generateRandomToken(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}
