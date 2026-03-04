package middleware

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/nstalgic/nekzus/internal/ratelimit"
)

// TestRateLimitMiddleware verifies rate limiting blocks excessive requests
func TestRateLimitMiddleware(t *testing.T) {
	limiter := ratelimit.NewLimiter(1.0, 2) // 1 req/sec, burst 2
	defer limiter.Stop()

	handler := RateLimit(limiter)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))

	// First request should succeed
	req1 := httptest.NewRequest("GET", "/test", nil)
	req1.RemoteAddr = "192.168.1.100:12345"
	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, req1)

	if w1.Code != http.StatusOK {
		t.Errorf("First request: got status %d, want %d", w1.Code, http.StatusOK)
	}

	// Second request should succeed (within burst)
	req2 := httptest.NewRequest("GET", "/test", nil)
	req2.RemoteAddr = "192.168.1.100:12345"
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Errorf("Second request: got status %d, want %d", w2.Code, http.StatusOK)
	}

	// Third request should be rate limited
	req3 := httptest.NewRequest("GET", "/test", nil)
	req3.RemoteAddr = "192.168.1.100:12345"
	w3 := httptest.NewRecorder()
	handler.ServeHTTP(w3, req3)

	if w3.Code != http.StatusTooManyRequests {
		t.Errorf("Third request: got status %d, want %d", w3.Code, http.StatusTooManyRequests)
	}

	// Check Retry-After header
	retryAfter := w3.Header().Get("Retry-After")
	if retryAfter == "" {
		t.Error("Missing Retry-After header on rate limited response")
	}
}

// TestRateLimitDifferentIPs verifies different IPs are tracked separately
func TestRateLimitDifferentIPs(t *testing.T) {
	limiter := ratelimit.NewLimiter(1.0, 1) // 1 req/sec, burst 1
	defer limiter.Stop()

	handler := RateLimit(limiter)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Request from IP 1
	req1 := httptest.NewRequest("GET", "/test", nil)
	req1.RemoteAddr = "192.168.1.100:12345"
	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, req1)

	if w1.Code != http.StatusOK {
		t.Errorf("IP1 request 1: got status %d, want %d", w1.Code, http.StatusOK)
	}

	// Second request from IP 1 should be blocked
	req2 := httptest.NewRequest("GET", "/test", nil)
	req2.RemoteAddr = "192.168.1.100:12345"
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)

	if w2.Code != http.StatusTooManyRequests {
		t.Errorf("IP1 request 2: got status %d, want %d", w2.Code, http.StatusTooManyRequests)
	}

	// Request from IP 2 should succeed (different limiter)
	req3 := httptest.NewRequest("GET", "/test", nil)
	req3.RemoteAddr = "192.168.1.200:12345"
	w3 := httptest.NewRecorder()
	handler.ServeHTTP(w3, req3)

	if w3.Code != http.StatusOK {
		t.Errorf("IP2 request 1: got status %d, want %d", w3.Code, http.StatusOK)
	}
}

// TestRateLimitIPv6 verifies IPv6 addresses are handled correctly
func TestRateLimitIPv6(t *testing.T) {
	limiter := ratelimit.NewLimiter(1.0, 1)
	defer limiter.Stop()

	handler := RateLimit(limiter)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// IPv6 request
	req1 := httptest.NewRequest("GET", "/test", nil)
	req1.RemoteAddr = "[2001:db8::1]:12345"
	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, req1)

	if w1.Code != http.StatusOK {
		t.Errorf("IPv6 first request: got status %d, want %d", w1.Code, http.StatusOK)
	}

	// Second IPv6 request should be blocked
	req2 := httptest.NewRequest("GET", "/test", nil)
	req2.RemoteAddr = "[2001:db8::1]:12345"
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)

	if w2.Code != http.StatusTooManyRequests {
		t.Errorf("IPv6 second request: got status %d, want %d", w2.Code, http.StatusTooManyRequests)
	}
}

// TestRateLimitXForwardedFor verifies X-Forwarded-For header is used when present
func TestRateLimitXForwardedFor(t *testing.T) {
	limiter := ratelimit.NewLimiter(1.0, 1)
	defer limiter.Stop()

	handler := RateLimit(limiter)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Request with X-Forwarded-For header
	req1 := httptest.NewRequest("GET", "/test", nil)
	req1.RemoteAddr = "127.0.0.1:12345"
	req1.Header.Set("X-Forwarded-For", "203.0.113.1")
	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, req1)

	if w1.Code != http.StatusOK {
		t.Errorf("First request: got status %d, want %d", w1.Code, http.StatusOK)
	}

	// Second request from same X-Forwarded-For should be blocked
	req2 := httptest.NewRequest("GET", "/test", nil)
	req2.RemoteAddr = "127.0.0.1:12345"
	req2.Header.Set("X-Forwarded-For", "203.0.113.1")
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)

	if w2.Code != http.StatusTooManyRequests {
		t.Errorf("Second request: got status %d, want %d", w2.Code, http.StatusTooManyRequests)
	}
}

// TestRateLimitRFC6585Headers verifies RFC 6585 RateLimit-* headers are set
// Rate limit headers should be RFC compliant
func TestRateLimitRFC6585Headers(t *testing.T) {
	limiter := ratelimit.NewLimiter(1.0, 5) // 1 req/sec, burst 5
	defer limiter.Stop()

	handler := RateLimit(limiter)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// First request - should have rate limit headers
	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "192.168.1.100:12345"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	// Check RateLimit-Limit header
	limitHeader := w.Header().Get("RateLimit-Limit")
	if limitHeader == "" {
		t.Error("Missing RateLimit-Limit header")
	} else {
		limit, err := strconv.Atoi(limitHeader)
		if err != nil {
			t.Errorf("Invalid RateLimit-Limit value: %s", limitHeader)
		}
		if limit != 5 {
			t.Errorf("RateLimit-Limit = %d, want 5", limit)
		}
	}

	// Check RateLimit-Remaining header
	remainingHeader := w.Header().Get("RateLimit-Remaining")
	if remainingHeader == "" {
		t.Error("Missing RateLimit-Remaining header")
	} else {
		remaining, err := strconv.Atoi(remainingHeader)
		if err != nil {
			t.Errorf("Invalid RateLimit-Remaining value: %s", remainingHeader)
		}
		// After first request, should have 4 or 5 remaining (depends on timing)
		if remaining < 0 || remaining > 5 {
			t.Errorf("RateLimit-Remaining = %d, should be 0-5", remaining)
		}
	}

	// Check RateLimit-Reset header
	resetHeader := w.Header().Get("RateLimit-Reset")
	if resetHeader == "" {
		t.Error("Missing RateLimit-Reset header")
	} else {
		resetTime, err := strconv.ParseInt(resetHeader, 10, 64)
		if err != nil {
			t.Errorf("Invalid RateLimit-Reset value: %s", resetHeader)
		}
		// Reset time should be in the future (or very close to now)
		if resetTime < 0 {
			t.Errorf("RateLimit-Reset = %d, should be positive Unix timestamp", resetTime)
		}
	}
}

// TestRateLimitHeadersOnLimitExceeded verifies headers are present even when limit exceeded
func TestRateLimitHeadersOnLimitExceeded(t *testing.T) {
	limiter := ratelimit.NewLimiter(1.0, 1) // 1 req/sec, burst 1
	defer limiter.Stop()

	handler := RateLimit(limiter)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// First request consumes the token
	req1 := httptest.NewRequest("GET", "/test", nil)
	req1.RemoteAddr = "192.168.1.100:12345"
	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, req1)

	// Second request should be rate limited but still have headers
	req2 := httptest.NewRequest("GET", "/test", nil)
	req2.RemoteAddr = "192.168.1.100:12345"
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)

	if w2.Code != http.StatusTooManyRequests {
		t.Errorf("Status = %d, want %d", w2.Code, http.StatusTooManyRequests)
	}

	// All rate limit headers should still be present
	if w2.Header().Get("RateLimit-Limit") == "" {
		t.Error("Missing RateLimit-Limit header on 429 response")
	}
	if w2.Header().Get("RateLimit-Remaining") == "" {
		t.Error("Missing RateLimit-Remaining header on 429 response")
	}
	if w2.Header().Get("RateLimit-Reset") == "" {
		t.Error("Missing RateLimit-Reset header on 429 response")
	}
	if w2.Header().Get("Retry-After") == "" {
		t.Error("Missing Retry-After header on 429 response")
	}
}
