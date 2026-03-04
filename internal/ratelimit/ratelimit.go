package ratelimit

import (
	"sort"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// limiterEntry tracks a rate limiter and its last access time
type limiterEntry struct {
	limiter    *rate.Limiter
	lastAccess time.Time
}

// Limiter provides per-IP rate limiting
type Limiter struct {
	mu            sync.RWMutex
	limiters      map[string]*limiterEntry
	rate          rate.Limit
	burst         int
	cleanupTicker *time.Ticker
	stop          chan struct{}
	maxEntries    int           // Maximum number of tracked keys
	ttl           time.Duration // Time-to-live for inactive entries
}

// NewLimiter creates a new rate limiter
// rps: requests per second
// burst: maximum burst size
func NewLimiter(rps float64, burst int) *Limiter {
	l := &Limiter{
		limiters:   make(map[string]*limiterEntry),
		rate:       rate.Limit(rps),
		burst:      burst,
		stop:       make(chan struct{}),
		maxEntries: 10000,           // Maximum tracked IPs before cleanup
		ttl:        5 * time.Minute, // Remove entries inactive for 5 minutes
	}

	// Start cleanup goroutine to remove old limiters
	l.cleanupTicker = time.NewTicker(time.Minute)
	go l.cleanupRoutine()

	return l
}

// Allow checks if a request from the given key (e.g., IP address) is allowed.
// Optimized to use RLock for the common case (limiter already exists).
func (l *Limiter) Allow(key string) bool {
	now := time.Now()

	l.mu.Lock()
	entry, exists := l.limiters[key]
	if !exists {
		entry = &limiterEntry{
			limiter:    rate.NewLimiter(l.rate, l.burst),
			lastAccess: now,
		}
		l.limiters[key] = entry
	} else {
		entry.lastAccess = now
	}
	limiter := entry.limiter
	l.mu.Unlock()

	return limiter.Allow()
}

// RateLimitState contains rate limit information for headers (RFC 6585)
type RateLimitState struct {
	Limit     int   // Maximum number of requests allowed
	Remaining int   // Number of requests remaining in current window
	ResetAt   int64 // Unix timestamp when the rate limit resets
}

// GetState returns the current rate limit state for a key
// Support RFC 6585 RateLimit-* headers
func (l *Limiter) GetState(key string) RateLimitState {
	l.mu.RLock()
	entry, exists := l.limiters[key]
	l.mu.RUnlock()

	if !exists {
		// No limiter for this key yet, return full capacity
		return RateLimitState{
			Limit:     l.burst,
			Remaining: l.burst,
			ResetAt:   time.Now().Add(time.Second).Unix(),
		}
	}

	// Get tokens available from the limiter
	tokens := int(entry.limiter.Tokens())
	if tokens < 0 {
		tokens = 0
	}
	if tokens > l.burst {
		tokens = l.burst
	}

	// Calculate reset time (when one more token will be available)
	// Token bucket fills at 'rate' tokens per second
	resetDuration := time.Duration(float64(time.Second) / float64(l.rate))
	resetTime := time.Now().Add(resetDuration).Unix()

	return RateLimitState{
		Limit:     l.burst,
		Remaining: tokens,
		ResetAt:   resetTime,
	}
}

// cleanupRoutine periodically removes limiters that haven't been used recently
func (l *Limiter) cleanupRoutine() {
	for {
		select {
		case <-l.cleanupTicker.C:
			l.cleanupExpired()
		case <-l.stop:
			return
		}
	}
}

// cleanupExpired removes limiters for IPs that haven't been seen recently
func (l *Limiter) cleanupExpired() {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	expiredKeys := make([]string, 0)

	// Find expired entries (not accessed within TTL)
	for key, entry := range l.limiters {
		if now.Sub(entry.lastAccess) > l.ttl {
			expiredKeys = append(expiredKeys, key)
		}
	}

	// Remove expired entries
	for _, key := range expiredKeys {
		delete(l.limiters, key)
	}

	// If still over max capacity after TTL cleanup, remove oldest entries
	if len(l.limiters) > l.maxEntries {
		// Find oldest entries to remove
		type keyTime struct {
			key        string
			lastAccess time.Time
		}
		entries := make([]keyTime, 0, len(l.limiters))
		for key, entry := range l.limiters {
			entries = append(entries, keyTime{key, entry.lastAccess})
		}

		// Sort by lastAccess time (oldest first)
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].lastAccess.Before(entries[j].lastAccess)
		})

		// Remove oldest entries to get back under maxEntries
		toRemove := len(l.limiters) - l.maxEntries
		for i := 0; i < toRemove && i < len(entries); i++ {
			delete(l.limiters, entries[i].key)
		}
	}
}

// Stop stops the cleanup goroutine
func (l *Limiter) Stop() {
	l.cleanupTicker.Stop()
	close(l.stop)
}

// Stats returns statistics about the rate limiter
func (l *Limiter) Stats() map[string]int {
	l.mu.RLock()
	defer l.mu.RUnlock()

	return map[string]int{
		"tracked_keys": len(l.limiters),
	}
}

// Burst returns the burst limit
func (l *Limiter) Burst() int {
	return l.burst
}

// Rate returns the rate limit in requests per second
func (l *Limiter) Rate() float64 {
	return float64(l.rate)
}
