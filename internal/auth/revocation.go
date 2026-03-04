package auth

import (
	"sync"
	"time"
)

// RevocationList maintains a list of revoked JWT tokens.
// Tokens are stored until their original expiry time, then automatically cleaned up.
// This implements an in-memory blacklist (cleared on restart).
type RevocationList struct {
	mu             sync.RWMutex
	revokedTokens  map[string]time.Time // JTI -> expiry time
	revokedDevices map[string]time.Time // deviceID -> expiry time
	cleanupTicker  *time.Ticker
	stopCleanup    chan struct{}
	stopped        bool
}

// NewRevocationList creates a new token revocation list with automatic cleanup.
func NewRevocationList() *RevocationList {
	rl := &RevocationList{
		revokedTokens:  make(map[string]time.Time),
		revokedDevices: make(map[string]time.Time),
		stopCleanup:    make(chan struct{}),
		stopped:        false,
	}

	// Start cleanup goroutine (runs every minute)
	rl.startCleanup(time.Minute)

	return rl
}

// Revoke adds a token to the revocation list.
// The token will be automatically removed after its expiry time.
func (rl *RevocationList) Revoke(tokenJTI string, expiry time.Time) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	rl.revokedTokens[tokenJTI] = expiry
}

// IsRevoked checks if a token has been revoked.
func (rl *RevocationList) IsRevoked(tokenJTI string) bool {
	rl.mu.RLock()
	defer rl.mu.RUnlock()

	expiry, exists := rl.revokedTokens[tokenJTI]
	if !exists {
		return false
	}

	// If the token has expired, it's no longer in the revocation list
	// (cleanup will remove it, but we can also check here)
	if time.Now().After(expiry) {
		return false
	}

	return true
}

// RevokeDevice adds a device to the revocation list.
// All tokens for this device will be considered revoked.
func (rl *RevocationList) RevokeDevice(deviceID string, expiry time.Time) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	rl.revokedDevices[deviceID] = expiry
}

// IsDeviceRevoked checks if a device has been revoked.
func (rl *RevocationList) IsDeviceRevoked(deviceID string) bool {
	rl.mu.RLock()
	defer rl.mu.RUnlock()

	expiry, exists := rl.revokedDevices[deviceID]
	if !exists {
		return false
	}

	// If the expiry has passed, the device is no longer revoked
	if time.Now().After(expiry) {
		return false
	}

	return true
}

// Cleanup removes expired entries from the revocation list.
func (rl *RevocationList) Cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()

	// Clean up expired tokens
	for jti, expiry := range rl.revokedTokens {
		if now.After(expiry) {
			delete(rl.revokedTokens, jti)
		}
	}

	// Clean up expired device revocations
	for deviceID, expiry := range rl.revokedDevices {
		if now.After(expiry) {
			delete(rl.revokedDevices, deviceID)
		}
	}
}

// Stats returns statistics about the revocation list.
func (rl *RevocationList) Stats() map[string]int {
	rl.mu.RLock()
	defer rl.mu.RUnlock()

	return map[string]int{
		"revoked_tokens":  len(rl.revokedTokens),
		"revoked_devices": len(rl.revokedDevices),
	}
}

// startCleanup starts a background goroutine to periodically clean up expired entries.
func (rl *RevocationList) startCleanup(interval time.Duration) {
	rl.cleanupTicker = time.NewTicker(interval)

	go func() {
		for {
			select {
			case <-rl.cleanupTicker.C:
				rl.Cleanup()
			case <-rl.stopCleanup:
				return
			}
		}
	}()
}

// Stop stops the cleanup goroutine.
// Safe to call multiple times.
func (rl *RevocationList) Stop() {
	rl.mu.Lock()
	if rl.stopped {
		rl.mu.Unlock()
		return
	}
	rl.stopped = true
	rl.mu.Unlock()

	if rl.cleanupTicker != nil {
		rl.cleanupTicker.Stop()
	}
	close(rl.stopCleanup)
}
