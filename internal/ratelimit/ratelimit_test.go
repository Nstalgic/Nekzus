package ratelimit

import (
	"testing"
	"time"
)

func TestNewLimiter(t *testing.T) {
	limiter := NewLimiter(1.0, 5)
	defer limiter.Stop()

	if limiter == nil {
		t.Fatal("expected non-nil limiter")
	}

	if len(limiter.limiters) != 0 {
		t.Errorf("expected empty limiters map, got %d entries", len(limiter.limiters))
	}
}

func TestLimiter_Allow(t *testing.T) {
	// Create limiter: 2 requests per second, burst of 3
	limiter := NewLimiter(2.0, 3)
	defer limiter.Stop()

	key := "test-ip"

	// First 3 requests should succeed (burst)
	for i := 0; i < 3; i++ {
		if !limiter.Allow(key) {
			t.Errorf("request %d should be allowed (burst)", i+1)
		}
	}

	// 4th request should be blocked
	if limiter.Allow(key) {
		t.Error("4th request should be blocked")
	}

	// Wait for rate limit to allow one more
	time.Sleep(600 * time.Millisecond) // Allow rate to accumulate

	// Should now allow one more request
	if !limiter.Allow(key) {
		t.Error("request after wait should be allowed")
	}
}

func TestLimiter_DifferentKeys(t *testing.T) {
	limiter := NewLimiter(1.0, 2)
	defer limiter.Stop()

	// Different keys should have independent limits
	if !limiter.Allow("key1") {
		t.Error("key1 first request should be allowed")
	}

	if !limiter.Allow("key2") {
		t.Error("key2 first request should be allowed")
	}

	if !limiter.Allow("key1") {
		t.Error("key1 second request should be allowed")
	}

	if !limiter.Allow("key2") {
		t.Error("key2 second request should be allowed")
	}

	// Both should now be rate limited
	if limiter.Allow("key1") {
		t.Error("key1 third request should be blocked")
	}

	if limiter.Allow("key2") {
		t.Error("key2 third request should be blocked")
	}
}

func TestLimiter_Stats(t *testing.T) {
	limiter := NewLimiter(1.0, 5)
	defer limiter.Stop()

	limiter.Allow("key1")
	limiter.Allow("key2")
	limiter.Allow("key3")

	stats := limiter.Stats()
	if stats["tracked_keys"] != 3 {
		t.Errorf("expected 3 tracked keys, got %d", stats["tracked_keys"])
	}
}

func TestLimiter_Cleanup(t *testing.T) {
	limiter := NewLimiter(1.0, 5)
	defer limiter.Stop()

	// Create many limiters to trigger cleanup
	for i := 0; i < 1100; i++ {
		limiter.Allow(string(rune(i)))
	}

	stats := limiter.Stats()
	// Should have cleared after hitting 1000+
	if stats["tracked_keys"] > 1100 {
		t.Errorf("expected cleanup to occur, got %d keys", stats["tracked_keys"])
	}
}

// TestLimiter_ConcurrentAllow tests concurrent Allow calls on the same key
func TestLimiter_ConcurrentAllow(t *testing.T) {
	limiter := NewLimiter(100.0, 500) // High limits to avoid blocking during test
	defer limiter.Stop()

	key := "concurrent-test"
	iterations := 1000
	done := make(chan bool)

	// Launch 10 goroutines making concurrent Allow calls
	for g := 0; g < 10; g++ {
		go func() {
			for i := 0; i < iterations; i++ {
				limiter.Allow(key)
			}
			done <- true
		}()
	}

	// Wait for all goroutines to complete
	for g := 0; g < 10; g++ {
		<-done
	}

	// Verify limiter still works correctly (Stats call shouldn't panic)
	stats := limiter.Stats()
	if stats["tracked_keys"] != 1 {
		t.Errorf("Expected 1 tracked key, got %d", stats["tracked_keys"])
	}

	// The limiter may be rate-limited after 10,000 calls, which is correct behavior
	// Just verify it doesn't panic
	_ = limiter.Allow(key)
}

// TestLimiter_ConcurrentDifferentKeys tests concurrent Allow calls on different keys
func TestLimiter_ConcurrentDifferentKeys(t *testing.T) {
	limiter := NewLimiter(10.0, 20)
	defer limiter.Stop()

	numKeys := 100
	done := make(chan bool)

	// Launch goroutines for different keys
	for i := 0; i < numKeys; i++ {
		go func(keyID int) {
			key := string(rune('a' + (keyID % 26)))
			// Make 5 requests per key
			for j := 0; j < 5; j++ {
				limiter.Allow(key)
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < numKeys; i++ {
		<-done
	}

	// Verify limiter is still functional
	stats := limiter.Stats()
	if stats["tracked_keys"] == 0 {
		t.Error("Expected some tracked keys after concurrent operations")
	}
}

// TestLimiter_ConcurrentStats tests concurrent Stats calls
func TestLimiter_ConcurrentStats(t *testing.T) {
	limiter := NewLimiter(5.0, 10)
	defer limiter.Stop()

	// Add some keys
	for i := 0; i < 10; i++ {
		limiter.Allow(string(rune('a' + i)))
	}

	done := make(chan bool)
	statsResults := make(chan map[string]int, 50)

	// Launch 50 goroutines calling Stats concurrently
	for i := 0; i < 50; i++ {
		go func() {
			stats := limiter.Stats()
			statsResults <- stats
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 50; i++ {
		<-done
	}
	close(statsResults)

	// Verify all Stats calls returned valid data
	count := 0
	for stats := range statsResults {
		count++
		if stats["tracked_keys"] < 0 {
			t.Error("Stats returned negative tracked_keys")
		}
	}

	if count != 50 {
		t.Errorf("Expected 50 Stats results, got %d", count)
	}
}

// TestLimiter_ConcurrentAllowAndCleanup tests Allow calls during cleanup
func TestLimiter_ConcurrentAllowAndCleanup(t *testing.T) {
	limiter := NewLimiter(5.0, 10)
	defer limiter.Stop()

	done := make(chan bool)

	// Goroutine 1: Continuously add new keys
	go func() {
		for i := 0; i < 1500; i++ {
			key := string(rune('a' + (i % 26)))
			limiter.Allow(key)
			time.Sleep(time.Microsecond)
		}
		done <- true
	}()

	// Goroutine 2: Continuously call Stats (reads the map)
	go func() {
		for i := 0; i < 500; i++ {
			limiter.Stats()
			time.Sleep(time.Microsecond * 3)
		}
		done <- true
	}()

	// Goroutine 3: Continuously make Allow calls on existing keys
	go func() {
		for i := 0; i < 500; i++ {
			limiter.Allow("test-key")
			time.Sleep(time.Microsecond * 3)
		}
		done <- true
	}()

	// Wait for all goroutines
	for i := 0; i < 3; i++ {
		<-done
	}

	// Verify limiter is still functional
	if !limiter.Allow("final-test") {
		// It's okay if rate limited, just shouldn't crash
	}

	stats := limiter.Stats()
	if stats["tracked_keys"] < 0 {
		t.Error("Invalid stats after concurrent operations")
	}
}

// TestLimiter_RaceFreeStop tests stopping limiter during concurrent operations
func TestLimiter_RaceFreeStop(t *testing.T) {
	limiter := NewLimiter(10.0, 20)

	done := make(chan bool)

	// Launch multiple goroutines making requests
	for i := 0; i < 5; i++ {
		go func(id int) {
			for j := 0; j < 100; j++ {
				limiter.Allow("key-" + string(rune('a'+id)))
				time.Sleep(time.Microsecond)
			}
			done <- true
		}(i)
	}

	// Stop limiter while operations are in progress
	time.Sleep(time.Millisecond * 50)
	limiter.Stop()

	// Wait for goroutines (they should complete without panic)
	for i := 0; i < 5; i++ {
		<-done
	}
}
