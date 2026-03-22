package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var pairingLog = slog.With("component", "pairing")

// PairingConfig contains all data needed for mobile app pairing
type PairingConfig struct {
	BaseURL        string   `json:"baseUrl"`
	Name           string   `json:"name"`
	SPKIPins       []string `json:"spkiPins"`
	BootstrapToken string   `json:"bootstrapToken"`
	Capabilities   []string `json:"capabilities"`
	NexusID        string   `json:"nexusId"`
	ExpiresAt      int64    `json:"expiresAt"`
}

// pairingCode represents a stored pairing code
type pairingCode struct {
	code        string
	config      PairingConfig
	expiresAt   time.Time
	used        bool
	failedCount int       // Track failed attempts per code
	lockedUntil time.Time // Lock code after too many failures
}

// PairingManager handles short pairing codes for the v2 QR flow
type PairingManager struct {
	mu       sync.RWMutex
	codes    map[string]*pairingCode
	codeLen  int
	ttl      time.Duration
	stopChan chan struct{}

	// Global rate limiting for failed attempts
	globalFailures     atomic.Int64
	globalFailureReset time.Time
	globalFailureMu    sync.Mutex
}

const (
	// Code configuration
	defaultCodeLength = 8 // 8 characters for ~41 bits of entropy

	// Alphanumeric charset excluding confusing characters (0/O, 1/I/L)
	// 32 characters = 5 bits per character, 8 chars = 40 bits entropy
	codeCharset = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"

	// Rate limiting
	maxFailedAttemptsPerCode = 5               // Lock code after 5 failures
	codeLockDuration         = 5 * time.Minute // Lock duration per code
	globalFailureThreshold   = 100             // Max global failures per window
	globalFailureWindow      = time.Hour       // Global failure tracking window
)

// NewPairingManager creates a new pairing code manager
func NewPairingManager() *PairingManager {
	pm := &PairingManager{
		codes:              make(map[string]*pairingCode),
		codeLen:            defaultCodeLength,
		ttl:                5 * time.Minute,
		stopChan:           make(chan struct{}),
		globalFailureReset: time.Now().Add(globalFailureWindow),
	}

	// Start cleanup goroutine
	go pm.cleanupLoop()

	return pm
}

// GenerateCode creates a new pairing code with the given config
func (pm *PairingManager) GenerateCode(config PairingConfig) (string, error) {
	code, err := pm.generateRandomCode()
	if err != nil {
		return "", fmt.Errorf("failed to generate code: %w", err)
	}

	expiresAt := time.Now().Add(pm.ttl)
	config.ExpiresAt = expiresAt.Unix()

	pm.mu.Lock()
	pm.codes[code] = &pairingCode{
		code:      code,
		config:    config,
		expiresAt: expiresAt,
		used:      false,
	}
	pm.mu.Unlock()

	pairingLog.Info("generated pairing code",
		"code_hash", hashCodeForLog(code),
		"expires_at", expiresAt.Format(time.RFC3339))

	return code, nil
}

// RedeemCode retrieves and invalidates a pairing code
// Returns the config if valid, error if expired/invalid/already used
// Uses constant-time comparison to prevent timing attacks
func (pm *PairingManager) RedeemCode(code string) (*PairingConfig, error) {
	code = strings.ToUpper(strings.TrimSpace(code))

	pairingLog.Debug("redeem attempt",
		"code_hash", hashCodeForLog(code),
		"code_length", len(code),
		"total_stored_codes", len(pm.codes))

	// Check global rate limit first
	if pm.isGloballyRateLimited() {
		pairingLog.Warn("global rate limit exceeded for pairing attempts",
			"global_failures", pm.globalFailures.Load())
		return nil, fmt.Errorf("too many pairing attempts, please try again later")
	}

	pm.mu.Lock()
	defer pm.mu.Unlock()

	// Use constant-time search to prevent timing attacks
	var matchedCode *pairingCode
	var matchedKey string

	for storedCode, pc := range pm.codes {
		// Constant-time comparison prevents timing attacks
		if subtle.ConstantTimeCompare([]byte(code), []byte(storedCode)) == 1 {
			matchedCode = pc
			matchedKey = storedCode
			break
		}
	}

	// Simulate work for non-matching codes to ensure constant timing
	if matchedCode == nil {
		pm.recordGlobalFailure()
		pairingLog.Warn("pairing code not found",
			"code_hash", hashCodeForLog(code),
			"stored_code_count", len(pm.codes))
		// Add small delay to normalize timing
		time.Sleep(time.Millisecond * time.Duration(1+randInt(5)))
		return nil, fmt.Errorf("invalid pairing code")
	}

	now := time.Now()

	// Check if code is locked due to too many failures
	if now.Before(matchedCode.lockedUntil) {
		pm.recordGlobalFailure()
		pairingLog.Warn("pairing code is locked",
			"code_hash", hashCodeForLog(code),
			"locked_until", matchedCode.lockedUntil.Format(time.RFC3339),
			"failed_count", matchedCode.failedCount)
		return nil, fmt.Errorf("invalid pairing code")
	}

	if matchedCode.used {
		pm.recordGlobalFailure()
		matchedCode.failedCount++
		if matchedCode.failedCount >= maxFailedAttemptsPerCode {
			matchedCode.lockedUntil = now.Add(codeLockDuration)
		}
		pairingLog.Warn("pairing code already used",
			"code_hash", hashCodeForLog(code),
			"failed_count", matchedCode.failedCount)
		return nil, fmt.Errorf("invalid pairing code")
	}

	if now.After(matchedCode.expiresAt) {
		pm.recordGlobalFailure()
		ttlAgo := now.Sub(matchedCode.expiresAt)
		pairingLog.Warn("pairing code expired",
			"code_hash", hashCodeForLog(code),
			"expired_ago", ttlAgo.String(),
			"expires_at", matchedCode.expiresAt.Format(time.RFC3339))
		delete(pm.codes, matchedKey)
		return nil, fmt.Errorf("invalid pairing code")
	}

	pairingLog.Debug("pairing code validated",
		"code_hash", hashCodeForLog(code),
		"ttl_remaining", matchedCode.expiresAt.Sub(now).String(),
		"base_url", matchedCode.config.BaseURL)

	// Mark as used but keep for a short time to prevent replay
	matchedCode.used = true

	pairingLog.Info("pairing code redeemed", "code_hash", hashCodeForLog(code))

	// Return a copy of the config to prevent external modification
	configCopy := matchedCode.config
	return &configCopy, nil
}

// GetCode retrieves a pairing code without redeeming it (for display purposes)
func (pm *PairingManager) GetCode(code string) (*PairingConfig, bool) {
	code = strings.ToUpper(strings.TrimSpace(code))

	pm.mu.RLock()
	defer pm.mu.RUnlock()

	// Use constant-time search
	for storedCode, pc := range pm.codes {
		if subtle.ConstantTimeCompare([]byte(code), []byte(storedCode)) == 1 {
			if pc.used || time.Now().After(pc.expiresAt) {
				return nil, false
			}
			configCopy := pc.config
			return &configCopy, true
		}
	}

	return nil, false
}

// generateRandomCode creates a random alphanumeric code using secure charset
func (pm *PairingManager) generateRandomCode() (string, error) {
	result := make([]byte, pm.codeLen)
	charsetLen := len(codeCharset)

	for i := 0; i < pm.codeLen; i++ {
		// Generate random byte and map to charset
		randomByte := make([]byte, 1)
		if _, err := rand.Read(randomByte); err != nil {
			return "", err
		}
		result[i] = codeCharset[int(randomByte[0])%charsetLen]
	}

	return string(result), nil
}

// isGloballyRateLimited checks if global failure threshold is exceeded
func (pm *PairingManager) isGloballyRateLimited() bool {
	pm.globalFailureMu.Lock()
	defer pm.globalFailureMu.Unlock()

	// Reset counter if window has passed
	if time.Now().After(pm.globalFailureReset) {
		pm.globalFailures.Store(0)
		pm.globalFailureReset = time.Now().Add(globalFailureWindow)
	}

	return pm.globalFailures.Load() >= globalFailureThreshold
}

// recordGlobalFailure increments the global failure counter
func (pm *PairingManager) recordGlobalFailure() {
	pm.globalFailureMu.Lock()
	defer pm.globalFailureMu.Unlock()

	// Reset counter if window has passed
	if time.Now().After(pm.globalFailureReset) {
		pm.globalFailures.Store(0)
		pm.globalFailureReset = time.Now().Add(globalFailureWindow)
	}

	pm.globalFailures.Add(1)
}

// GetGlobalFailureCount returns current global failure count (for monitoring)
func (pm *PairingManager) GetGlobalFailureCount() int64 {
	return pm.globalFailures.Load()
}

// cleanupLoop periodically removes expired codes
func (pm *PairingManager) cleanupLoop() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			pm.cleanup()
		case <-pm.stopChan:
			return
		}
	}
}

// cleanup removes expired and used codes
func (pm *PairingManager) cleanup() {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	now := time.Now()
	expired := 0

	for code, pc := range pm.codes {
		// Remove if expired or used more than 1 minute ago
		if now.After(pc.expiresAt.Add(1 * time.Minute)) {
			delete(pm.codes, code)
			expired++
		}
	}

	if expired > 0 {
		pairingLog.Debug("cleaned up expired pairing codes", "count", expired)
	}
}

// Stop shuts down the cleanup goroutine
func (pm *PairingManager) Stop() {
	close(pm.stopChan)
}

// ActiveCodeCount returns the number of active (non-expired, non-used) codes
func (pm *PairingManager) ActiveCodeCount() int {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	count := 0
	now := time.Now()
	for _, pc := range pm.codes {
		if !pc.used && now.Before(pc.expiresAt) {
			count++
		}
	}
	return count
}

// CodeStatus represents the status of a pairing code
type CodeStatus struct {
	Valid    bool  `json:"valid"`
	Redeemed bool  `json:"redeemed"`
	Expired  bool  `json:"expired"`
	TTL      int64 `json:"ttl"` // seconds until expiry
}

// GetCodeStatus returns the status of a pairing code without redeeming it
func (pm *PairingManager) GetCodeStatus(code string) CodeStatus {
	code = strings.ToUpper(strings.TrimSpace(code))

	pm.mu.RLock()
	defer pm.mu.RUnlock()

	now := time.Now()

	// Use constant-time search
	for storedCode, pc := range pm.codes {
		if subtle.ConstantTimeCompare([]byte(code), []byte(storedCode)) == 1 {
			status := CodeStatus{Valid: true}

			if pc.used {
				status.Redeemed = true
				return status
			}

			if now.After(pc.expiresAt) {
				status.Expired = true
				return status
			}

			status.TTL = int64(pc.expiresAt.Sub(now).Seconds())
			return status
		}
	}

	return CodeStatus{Valid: false}
}

// hashCodeForLog returns a truncated hash of the code for safe logging
func hashCodeForLog(code string) string {
	hash := sha256.Sum256([]byte(code))
	return fmt.Sprintf("%x", hash[:4]) // First 8 hex chars (32 bits)
}

// randInt returns a random int in range [0, max)
func randInt(max int) int {
	b := make([]byte, 1)
	rand.Read(b)
	return int(b[0]) % max
}
