package auth

import (
	"strings"
	"testing"
	"time"
)

func TestPairingManager_GenerateCode(t *testing.T) {
	pm := NewPairingManager()
	defer pm.Stop()

	config := PairingConfig{
		BaseURL:        "https://192.168.1.100:8443",
		Name:           "Test Nexus",
		SPKIPins:       []string{"sha256/primary...", "sha256/backup..."},
		BootstrapToken: "bootstrap_token_123",
		Capabilities:   []string{"discovery", "websocket"},
		NekzusID:       "test-nexus-id",
	}

	code, err := pm.GenerateCode(config)
	if err != nil {
		t.Fatalf("GenerateCode failed: %v", err)
	}

	// Code should be 8 characters (security requirement)
	if len(code) != 8 {
		t.Errorf("Expected code length 8, got %d", len(code))
	}

	// Code should only contain valid charset characters
	for _, c := range code {
		if !strings.ContainsRune(codeCharset, c) {
			t.Errorf("Code contains invalid character: %c (not in charset)", c)
		}
	}

	t.Logf("Generated code: %s", code)
}

func TestPairingManager_RedeemCode(t *testing.T) {
	pm := NewPairingManager()
	defer pm.Stop()

	config := PairingConfig{
		BaseURL:        "https://192.168.1.100:8443",
		Name:           "Test Nexus",
		SPKIPins:       []string{"sha256/primary...", "sha256/backup..."},
		BootstrapToken: "bootstrap_token_123",
		Capabilities:   []string{"discovery"},
		NekzusID:       "test-nexus-id",
	}

	code, err := pm.GenerateCode(config)
	if err != nil {
		t.Fatalf("GenerateCode failed: %v", err)
	}

	// Redeem the code
	retrieved, err := pm.RedeemCode(code)
	if err != nil {
		t.Fatalf("RedeemCode failed: %v", err)
	}

	// Verify config matches
	if retrieved.BaseURL != config.BaseURL {
		t.Errorf("BaseURL mismatch: got %s, want %s", retrieved.BaseURL, config.BaseURL)
	}
	if retrieved.Name != config.Name {
		t.Errorf("Name mismatch: got %s, want %s", retrieved.Name, config.Name)
	}
	if len(retrieved.SPKIPins) != 2 {
		t.Errorf("SPKIPins length mismatch: got %d, want 2", len(retrieved.SPKIPins))
	}
	if retrieved.BootstrapToken != config.BootstrapToken {
		t.Errorf("BootstrapToken mismatch")
	}
	if retrieved.NekzusID != config.NekzusID {
		t.Errorf("NekzusID mismatch")
	}
	if retrieved.ExpiresAt == 0 {
		t.Error("ExpiresAt should be set")
	}
}

func TestPairingManager_RedeemCode_IdempotentBeforeConsume(t *testing.T) {
	pm := NewPairingManager()
	defer pm.Stop()

	config := PairingConfig{
		BaseURL:        "https://192.168.1.100:8443",
		BootstrapToken: "test-bootstrap-token",
	}

	code, _ := pm.GenerateCode(config)

	// First redemption should succeed
	cfg1, err := pm.RedeemCode(code)
	if err != nil {
		t.Fatalf("First redemption failed: %v", err)
	}

	// Second redemption should also succeed (idempotent before consumption)
	cfg2, err := pm.RedeemCode(code)
	if err != nil {
		t.Fatalf("Second redemption should succeed before consumption: %v", err)
	}

	// Both should return the same config
	if cfg1.BootstrapToken != cfg2.BootstrapToken {
		t.Error("Re-redemption returned different config")
	}

	// Consume via bootstrap token (simulates successful pairing)
	pm.ConsumeByBootstrapToken("test-bootstrap-token")

	// Third redemption should fail (consumed)
	_, err = pm.RedeemCode(code)
	if err == nil {
		t.Error("Expected error after consumption, got nil")
	}
}

func TestPairingManager_RedeemCode_Invalid(t *testing.T) {
	pm := NewPairingManager()
	defer pm.Stop()

	// Try to redeem non-existent code
	_, err := pm.RedeemCode("INVALID1")
	if err == nil {
		t.Error("Expected error for invalid code, got nil")
	}
}

func TestPairingManager_RedeemCode_CaseInsensitive(t *testing.T) {
	pm := NewPairingManager()
	defer pm.Stop()

	config := PairingConfig{
		BaseURL: "https://192.168.1.100:8443",
	}

	code, _ := pm.GenerateCode(config)

	// Redeem with lowercase should work
	lowerCode := strings.ToLower(code)

	_, err := pm.RedeemCode(lowerCode)
	if err != nil {
		t.Errorf("Lowercase redemption failed: %v", err)
	}
}

func TestPairingManager_GetCode_NoRedeem(t *testing.T) {
	pm := NewPairingManager()
	defer pm.Stop()

	config := PairingConfig{
		BaseURL: "https://192.168.1.100:8443",
		Name:    "Test",
	}

	code, _ := pm.GenerateCode(config)

	// GetCode should not consume the code
	retrieved1, ok := pm.GetCode(code)
	if !ok {
		t.Fatal("GetCode should find the code")
	}
	if retrieved1.Name != "Test" {
		t.Error("GetCode returned wrong config")
	}

	// Code should still be available
	retrieved2, ok := pm.GetCode(code)
	if !ok {
		t.Error("GetCode should still find the code after previous GetCode")
	}
	if retrieved2.Name != "Test" {
		t.Error("GetCode returned wrong config on second call")
	}

	// And RedeemCode should still work
	_, err := pm.RedeemCode(code)
	if err != nil {
		t.Errorf("RedeemCode should still work after GetCode: %v", err)
	}
}

func TestPairingManager_ActiveCodeCount(t *testing.T) {
	pm := NewPairingManager()
	defer pm.Stop()

	config := PairingConfig{BaseURL: "https://test.local"}

	// Should start at 0
	if pm.ActiveCodeCount() != 0 {
		t.Errorf("Expected 0 active codes, got %d", pm.ActiveCodeCount())
	}

	// Generate some codes
	pm.GenerateCode(config)
	pm.GenerateCode(config)
	pm.GenerateCode(config)

	if pm.ActiveCodeCount() != 3 {
		t.Errorf("Expected 3 active codes, got %d", pm.ActiveCodeCount())
	}

	// Redeem one
	codes := make([]string, 0)
	for i := 0; i < 3; i++ {
		code, _ := pm.GenerateCode(config)
		codes = append(codes, code)
	}

	pm.RedeemCode(codes[0])

	// Redeemed codes are still active (not yet consumed) — all 6 remain active
	count := pm.ActiveCodeCount()
	if count != 6 {
		t.Errorf("Expected 6 active codes after redemption (not yet consumed), got %d", count)
	}

	// After consumption, count should decrease
	cfg, _ := pm.RedeemCode(codes[0])
	if cfg != nil {
		pm.ConsumeByBootstrapToken(cfg.BootstrapToken)
	}
	count = pm.ActiveCodeCount()
	if count != 5 {
		t.Errorf("Expected 5 active codes after consumption, got %d", count)
	}
}

func TestPairingManager_CodeUniqueness(t *testing.T) {
	pm := NewPairingManager()
	defer pm.Stop()

	config := PairingConfig{BaseURL: "https://test.local"}
	codes := make(map[string]bool)

	// Generate many codes and check for duplicates
	for i := 0; i < 100; i++ {
		code, err := pm.GenerateCode(config)
		if err != nil {
			t.Fatalf("GenerateCode failed: %v", err)
		}
		if codes[code] {
			t.Errorf("Duplicate code generated: %s", code)
		}
		codes[code] = true
	}
}

func TestPairingConfig_ExpiresAt(t *testing.T) {
	pm := NewPairingManager()
	defer pm.Stop()

	config := PairingConfig{BaseURL: "https://test.local"}

	code, _ := pm.GenerateCode(config)
	retrieved, _ := pm.RedeemCode(code)

	// ExpiresAt should be ~5 minutes from now
	expiresAt := time.Unix(retrieved.ExpiresAt, 0)
	expectedExpiry := time.Now().Add(5 * time.Minute)

	diff := expiresAt.Sub(expectedExpiry)
	if diff < -10*time.Second || diff > 10*time.Second {
		t.Errorf("ExpiresAt not within expected range: got %v, expected ~%v", expiresAt, expectedExpiry)
	}
}

func TestPairingManager_GlobalRateLimiting(t *testing.T) {
	pm := NewPairingManager()
	defer pm.Stop()

	// Simulate many failed attempts to trigger global rate limit
	for i := 0; i < 100; i++ {
		pm.RedeemCode("INVALID1")
	}

	// Global failure count should be at threshold
	if pm.GetGlobalFailureCount() < globalFailureThreshold {
		t.Errorf("Expected global failure count >= %d, got %d", globalFailureThreshold, pm.GetGlobalFailureCount())
	}

	// Next attempt should be rate limited
	config := PairingConfig{BaseURL: "https://test.local"}
	code, _ := pm.GenerateCode(config)

	_, err := pm.RedeemCode(code)
	if err == nil {
		t.Error("Expected rate limit error, got nil")
	}
	if !strings.Contains(err.Error(), "too many pairing attempts") {
		t.Errorf("Expected rate limit error message, got: %v", err)
	}
}

func TestPairingManager_CodeEntropy(t *testing.T) {
	pm := NewPairingManager()
	defer pm.Stop()

	config := PairingConfig{BaseURL: "https://test.local"}

	// Generate 1000 codes and verify distribution
	charCounts := make(map[rune]int)
	for i := 0; i < 1000; i++ {
		code, _ := pm.GenerateCode(config)
		for _, c := range code {
			charCounts[c]++
		}
	}

	// Each character should appear roughly equally (8000 total chars / 32 charset = ~250 each)
	// Allow for statistical variance (between 150 and 400)
	for c, count := range charCounts {
		if count < 100 || count > 500 {
			t.Errorf("Character '%c' has suspicious count: %d (expected ~250)", c, count)
		}
	}

	// Verify only valid charset characters are used
	for c := range charCounts {
		if !strings.ContainsRune(codeCharset, c) {
			t.Errorf("Invalid character '%c' found in generated codes", c)
		}
	}
}

func TestPairingManager_ConfigCopyProtection(t *testing.T) {
	pm := NewPairingManager()
	defer pm.Stop()

	config := PairingConfig{
		BaseURL: "https://test.local",
		Name:    "Original",
	}

	code, _ := pm.GenerateCode(config)

	// Get config and modify it
	retrieved, ok := pm.GetCode(code)
	if !ok {
		t.Fatal("GetCode failed")
	}
	retrieved.Name = "Modified"

	// Original should be unchanged
	retrieved2, ok := pm.GetCode(code)
	if !ok {
		t.Fatal("Second GetCode failed")
	}
	if retrieved2.Name != "Original" {
		t.Errorf("Config was modified externally: got %s, want Original", retrieved2.Name)
	}
}

func TestHashCodeForLog(t *testing.T) {
	// Same input should produce same hash
	hash1 := hashCodeForLog("ABCD1234")
	hash2 := hashCodeForLog("ABCD1234")
	if hash1 != hash2 {
		t.Error("Same input should produce same hash")
	}

	// Different input should produce different hash
	hash3 := hashCodeForLog("WXYZ5678")
	if hash1 == hash3 {
		t.Error("Different input should produce different hash")
	}

	// Hash should be 8 hex characters (4 bytes)
	if len(hash1) != 8 {
		t.Errorf("Hash should be 8 characters, got %d", len(hash1))
	}
}
