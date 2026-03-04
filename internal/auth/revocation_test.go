package auth

import (
	"testing"
	"time"
)

// Test 1: Token revocation list - add and check revoked tokens
func TestRevocationList_RevokeAndCheck(t *testing.T) {
	rl := NewRevocationList()
	defer rl.Stop()

	tokenJTI := "test-token-jti-123"
	expiry := time.Now().Add(1 * time.Hour)

	// Token should not be revoked initially
	if rl.IsRevoked(tokenJTI) {
		t.Error("Token should not be revoked initially")
	}

	// Revoke the token
	rl.Revoke(tokenJTI, expiry)

	// Token should now be revoked
	if !rl.IsRevoked(tokenJTI) {
		t.Error("Token should be revoked after Revoke() call")
	}
}

// Test 2: Token revocation list - expired tokens are cleaned up
func TestRevocationList_ExpiredTokensCleanup(t *testing.T) {
	rl := NewRevocationList()
	defer rl.Stop()

	tokenJTI := "expired-token-jti"
	expiry := time.Now().Add(-1 * time.Hour) // Already expired

	// Revoke with past expiry
	rl.Revoke(tokenJTI, expiry)

	// Run cleanup
	rl.Cleanup()

	// Token should be removed after cleanup (not in list)
	if rl.IsRevoked(tokenJTI) {
		t.Error("Expired token should be cleaned up")
	}
}

// Test 3: Token revocation list - multiple tokens
func TestRevocationList_MultipleTokens(t *testing.T) {
	rl := NewRevocationList()
	defer rl.Stop()

	tokens := []string{"token-1", "token-2", "token-3"}
	expiry := time.Now().Add(1 * time.Hour)

	// Revoke all tokens
	for _, token := range tokens {
		rl.Revoke(token, expiry)
	}

	// All should be revoked
	for _, token := range tokens {
		if !rl.IsRevoked(token) {
			t.Errorf("Token %s should be revoked", token)
		}
	}

	// Unrelated token should not be revoked
	if rl.IsRevoked("other-token") {
		t.Error("Unrelated token should not be revoked")
	}
}

// Test 4: Token revocation list - stats
func TestRevocationList_Stats(t *testing.T) {
	rl := NewRevocationList()
	defer rl.Stop()

	expiry := time.Now().Add(1 * time.Hour)

	// Add some tokens
	rl.Revoke("token-1", expiry)
	rl.Revoke("token-2", expiry)
	rl.Revoke("token-3", expiry)

	stats := rl.Stats()
	if stats["revoked_tokens"] != 3 {
		t.Errorf("Expected 3 revoked tokens, got %d", stats["revoked_tokens"])
	}
}

// Test 5: Token revocation list - concurrent access
func TestRevocationList_ConcurrentAccess(t *testing.T) {
	rl := NewRevocationList()
	defer rl.Stop()

	done := make(chan bool)
	expiry := time.Now().Add(1 * time.Hour)

	// Multiple goroutines revoking tokens
	for i := 0; i < 10; i++ {
		go func(id int) {
			for j := 0; j < 100; j++ {
				rl.Revoke("concurrent-token", expiry)
				rl.IsRevoked("concurrent-token")
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Should still work correctly
	if !rl.IsRevoked("concurrent-token") {
		t.Error("Token should be revoked after concurrent operations")
	}
}

// Test 6: Revoke by device ID - revokes all tokens for a device
func TestRevocationList_RevokeByDeviceID(t *testing.T) {
	rl := NewRevocationList()
	defer rl.Stop()

	deviceID := "device-123"
	expiry := time.Now().Add(1 * time.Hour)

	// Revoke all tokens for a device
	rl.RevokeDevice(deviceID, expiry)

	// Construct a token JTI that includes the device ID (simulating how we'd check)
	// The manager will check device revocation separately
	if !rl.IsDeviceRevoked(deviceID) {
		t.Error("Device should be revoked")
	}
}

// Test 7: Double stop should not panic
func TestRevocationList_DoubleStop(t *testing.T) {
	rl := NewRevocationList()

	// First stop
	rl.Stop()

	// Second stop should not panic
	rl.Stop()
}

// Test 8: Manager integration - revoked tokens should fail validation
func TestManager_RevokedTokenFails(t *testing.T) {
	// Create manager with a valid JWT key (no weak patterns)
	mgr, err := NewManager(
		[]byte("xk7f9q2m4n8b1v3c5z0w6y8u0i2o4p6a"),
		"test-issuer",
		"test-audience",
		[]string{},
	)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}
	defer mgr.Stop()

	// Sign a token
	token, err := mgr.SignJWT("test-device", []string{"read"}, time.Hour)
	if err != nil {
		t.Fatalf("Failed to sign JWT: %v", err)
	}

	// Token should be valid initially
	_, _, err = mgr.ParseJWT(token)
	if err != nil {
		t.Errorf("Token should be valid initially: %v", err)
	}

	// Revoke the token
	mgr.RevokeToken(token, time.Now().Add(time.Hour))

	// Token should now fail validation
	_, _, err = mgr.ParseJWT(token)
	if err == nil {
		t.Error("Revoked token should fail validation")
	}
}

// Test 9: Manager integration - revoked device tokens fail validation
func TestManager_RevokedDeviceTokensFail(t *testing.T) {
	mgr, err := NewManager(
		[]byte("xk7f9q2m4n8b1v3c5z0w6y8u0i2o4p6a"),
		"test-issuer",
		"test-audience",
		[]string{},
	)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}
	defer mgr.Stop()

	deviceID := "device-to-revoke"

	// Sign a token for the device
	token, err := mgr.SignJWT(deviceID, []string{"read"}, time.Hour)
	if err != nil {
		t.Fatalf("Failed to sign JWT: %v", err)
	}

	// Token should be valid initially
	_, _, err = mgr.ParseJWT(token)
	if err != nil {
		t.Errorf("Token should be valid initially: %v", err)
	}

	// Revoke the device
	mgr.RevokeDevice(deviceID, time.Now().Add(time.Hour))

	// Token should now fail validation
	_, _, err = mgr.ParseJWT(token)
	if err == nil {
		t.Error("Token for revoked device should fail validation")
	}
}
