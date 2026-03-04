package auth

import (
	"sync"
	"testing"
	"time"
)

func TestBootstrapStore_Validate(t *testing.T) {
	tokens := []string{"token1", "token2", "secret-bootstrap"}
	store := NewBootstrapStore(tokens)
	defer store.Stop()

	tests := []struct {
		name  string
		token string
		want  bool
	}{
		{
			name:  "valid permanent token",
			token: "token1",
			want:  true,
		},
		{
			name:  "another valid token",
			token: "token2",
			want:  true,
		},
		{
			name:  "invalid token",
			token: "invalid",
			want:  false,
		},
		{
			name:  "empty token",
			token: "",
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := store.Validate(tt.token)
			if got != tt.want {
				t.Errorf("Validate() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBootstrapStore_ShortLived(t *testing.T) {
	store := NewBootstrapStore([]string{})
	defer store.Stop()

	// Generate short-lived token
	token, err := store.GenerateShortLived(100 * time.Millisecond)
	if err != nil {
		t.Fatalf("GenerateShortLived() error = %v", err)
	}

	if token == "" {
		t.Fatal("expected non-empty token")
	}

	// Should be valid immediately
	if !store.Validate(token) {
		t.Error("token should be valid immediately after generation")
	}

	// Wait for expiry
	time.Sleep(150 * time.Millisecond)

	// Should be invalid after expiry
	if store.Validate(token) {
		t.Error("token should be invalid after expiry")
	}
}

func TestBootstrapStore_Cleanup(t *testing.T) {
	store := NewBootstrapStore([]string{})
	defer store.Stop()

	// Generate multiple tokens
	for i := 0; i < 5; i++ {
		_, err := store.GenerateShortLived(50 * time.Millisecond)
		if err != nil {
			t.Fatalf("GenerateShortLived() error = %v", err)
		}
	}

	stats := store.Stats()
	if stats["short_lived_tokens"] != 5 {
		t.Errorf("expected 5 short-lived tokens, got %d", stats["short_lived_tokens"])
	}

	// Wait for cleanup (tokens expire after 50ms, cleanup runs periodically)
	time.Sleep(200 * time.Millisecond)

	// Manually trigger cleanup by trying to validate an expired token
	store.cleanupExpired()

	stats = store.Stats()
	if stats["short_lived_tokens"] != 0 {
		t.Errorf("expected 0 short-lived tokens after cleanup, got %d", stats["short_lived_tokens"])
	}
}

func TestBootstrapStore_Stats(t *testing.T) {
	tokens := []string{"token1", "token2"}
	store := NewBootstrapStore(tokens)
	defer store.Stop()

	stats := store.Stats()

	if stats["permanent_tokens"] != 2 {
		t.Errorf("expected 2 permanent tokens, got %d", stats["permanent_tokens"])
	}

	if stats["short_lived_tokens"] != 0 {
		t.Errorf("expected 0 short-lived tokens, got %d", stats["short_lived_tokens"])
	}

	// Add short-lived token
	_, _ = store.GenerateShortLived(time.Minute)

	stats = store.Stats()
	if stats["short_lived_tokens"] != 1 {
		t.Errorf("expected 1 short-lived token, got %d", stats["short_lived_tokens"])
	}
}

func TestGenerateRandomToken(t *testing.T) {
	token1, err := generateRandomToken(16)
	if err != nil {
		t.Fatalf("generateRandomToken() error = %v", err)
	}

	token2, err := generateRandomToken(16)
	if err != nil {
		t.Fatalf("generateRandomToken() error = %v", err)
	}

	// Tokens should be different
	if token1 == token2 {
		t.Error("expected different tokens, got identical")
	}

	// Tokens should be non-empty
	if token1 == "" || token2 == "" {
		t.Error("expected non-empty tokens")
	}
}

// Panic on Double Stop
func TestBootstrapStore_DoubleStop(t *testing.T) {
	store := NewBootstrapStore([]string{"token1"})

	// First stop should work
	store.Stop()

	// Second stop should not panic
	store.Stop()
}

// Concurrent Stop calls
func TestBootstrapStore_ConcurrentStop(t *testing.T) {
	store := NewBootstrapStore([]string{"token1"})

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			store.Stop()
		}()
	}

	wg.Wait()
}

// Test that Stop properly stops the cleanup goroutine
func TestBootstrapStore_StopCleansUp(t *testing.T) {
	store := NewBootstrapStore([]string{"token1"})

	// Generate some short-lived tokens
	_, err := store.GenerateShortLived(time.Hour)
	if err != nil {
		t.Fatalf("GenerateShortLived() error = %v", err)
	}

	// Stop should not panic
	store.Stop()

	// After stop, we shouldn't be able to use the store (but it shouldn't panic)
	// Just verify stop completed without panicking
}

// Test concurrent operations don't cause race conditions
func TestBootstrapStore_ConcurrentOperations(t *testing.T) {
	store := NewBootstrapStore([]string{"token1", "token2"})
	defer store.Stop()

	var wg sync.WaitGroup
	done := make(chan struct{})

	// Multiple goroutines validating
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-done:
					return
				default:
					store.Validate("token1")
				}
			}
		}()
	}

	// Multiple goroutines generating short-lived tokens
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-done:
					return
				default:
					store.GenerateShortLived(time.Second)
				}
			}
		}()
	}

	// Multiple goroutines getting stats
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-done:
					return
				default:
					store.Stats()
				}
			}
		}()
	}

	// Let it run for a bit
	time.Sleep(100 * time.Millisecond)
	close(done)
	wg.Wait()
}

// Test pairing attempt limits per bootstrap token (NEXUS_SECURITY_IMPL.md item 3)
func TestBootstrapStore_PairingAttemptLimits(t *testing.T) {
	store := NewBootstrapStore([]string{})
	defer store.Stop()

	// Generate a short-lived token
	token, err := store.GenerateShortLived(5 * time.Minute)
	if err != nil {
		t.Fatalf("GenerateShortLived() error = %v", err)
	}

	// Token should be valid initially
	if !store.Validate(token) {
		t.Error("token should be valid initially")
	}

	// Record 3 failed pairing attempts
	for i := 0; i < 3; i++ {
		store.RecordFailedPairing(token)
	}

	// Token should be invalidated after 3 failed attempts
	if store.Validate(token) {
		t.Error("token should be invalidated after 3 failed pairing attempts")
	}

	// IsRateLimited should return true
	if !store.IsRateLimited(token) {
		t.Error("IsRateLimited should return true after 3 failed attempts")
	}
}

func TestBootstrapStore_PairingAttempts_NotRateLimitedBeforeLimit(t *testing.T) {
	store := NewBootstrapStore([]string{})
	defer store.Stop()

	// Generate a short-lived token
	token, err := store.GenerateShortLived(5 * time.Minute)
	if err != nil {
		t.Fatalf("GenerateShortLived() error = %v", err)
	}

	// Record 2 failed attempts (under limit of 3)
	store.RecordFailedPairing(token)
	store.RecordFailedPairing(token)

	// Token should still be valid
	if !store.Validate(token) {
		t.Error("token should still be valid after 2 failed attempts")
	}

	// IsRateLimited should return false
	if store.IsRateLimited(token) {
		t.Error("IsRateLimited should return false before reaching limit")
	}
}

func TestBootstrapStore_PairingAttempts_SuccessResetsCounter(t *testing.T) {
	store := NewBootstrapStore([]string{})
	defer store.Stop()

	// Generate a short-lived token
	token, err := store.GenerateShortLived(5 * time.Minute)
	if err != nil {
		t.Fatalf("GenerateShortLived() error = %v", err)
	}

	// Record 2 failed attempts
	store.RecordFailedPairing(token)
	store.RecordFailedPairing(token)

	// Successful pairing should clear failure count
	store.RecordSuccessfulPairing(token)

	// Should not be rate limited
	if store.IsRateLimited(token) {
		t.Error("IsRateLimited should return false after successful pairing")
	}

	// Token is consumed after successful pairing and should be invalid
	// (short-lived tokens are one-time use)
	if store.Validate(token) {
		t.Error("token should be consumed after successful pairing")
	}
}
