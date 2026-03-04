package httputil

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestExtractBearerToken(t *testing.T) {
	tests := []struct {
		name     string
		header   string
		expected string
	}{
		{
			name:     "valid bearer token",
			header:   "Bearer abc123def456",
			expected: "abc123def456",
		},
		{
			name:     "empty header",
			header:   "",
			expected: "",
		},
		{
			name:     "missing Bearer prefix",
			header:   "abc123",
			expected: "",
		},
		{
			name:     "lowercase bearer",
			header:   "bearer abc123",
			expected: "",
		},
		{
			name:     "Bearer without space",
			header:   "Bearerabc123",
			expected: "",
		},
		{
			name:     "Bearer with empty token",
			header:   "Bearer ",
			expected: "",
		},
		{
			name:     "short header",
			header:   "Bear",
			expected: "",
		},
		{
			name:     "token with special characters",
			header:   "Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0",
			expected: "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			if tt.header != "" {
				req.Header.Set("Authorization", tt.header)
			}

			got := ExtractBearerToken(req)
			if got != tt.expected {
				t.Errorf("ExtractBearerToken() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestExtractClientIP(t *testing.T) {
	tests := []struct {
		name       string
		remoteAddr string
		realIP     string
		forwarded  string
		wantPrefix string // We'll check if result starts with this
	}{
		{
			name:       "direct connection",
			remoteAddr: "192.168.1.100:12345",
			realIP:     "",
			forwarded:  "",
			wantPrefix: "192.168.1.100",
		},
		{
			name:       "X-Real-IP header",
			remoteAddr: "10.0.0.1:12345",
			realIP:     "203.0.113.1",
			forwarded:  "",
			wantPrefix: "203.0.113.1",
		},
		{
			name:       "X-Forwarded-For single IP",
			remoteAddr: "10.0.0.1:12345",
			realIP:     "",
			forwarded:  "203.0.113.1",
			wantPrefix: "203.0.113.1",
		},
		{
			name:       "X-Forwarded-For multiple IPs",
			remoteAddr: "10.0.0.1:12345",
			realIP:     "",
			forwarded:  "203.0.113.1, 198.51.100.1, 10.0.0.1",
			wantPrefix: "203.0.113.1",
		},
		{
			name:       "X-Real-IP takes precedence over X-Forwarded-For",
			remoteAddr: "10.0.0.1:12345",
			realIP:     "203.0.113.1",
			forwarded:  "198.51.100.1",
			wantPrefix: "203.0.113.1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			req.RemoteAddr = tt.remoteAddr
			if tt.realIP != "" {
				req.Header.Set("X-Real-IP", tt.realIP)
			}
			if tt.forwarded != "" {
				req.Header.Set("X-Forwarded-For", tt.forwarded)
			}

			got := ExtractClientIP(req)
			if !strings.HasPrefix(got, tt.wantPrefix) {
				t.Errorf("ExtractClientIP() = %q, want prefix %q", got, tt.wantPrefix)
			}
		})
	}
}

func TestGenerateRandomID(t *testing.T) {
	tests := []struct {
		name       string
		byteLength int
	}{
		{"8 bytes", 8},
		{"16 bytes", 16},
		{"32 bytes", 32},
		{"64 bytes", 64},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Generate multiple IDs and ensure they're different
			ids := make(map[string]bool)
			for i := 0; i < 100; i++ {
				id := GenerateRandomID(tt.byteLength)

				// Check ID is not empty
				if id == "" {
					t.Error("GenerateRandomID() returned empty string")
				}

				// Check ID is unique
				if ids[id] {
					t.Errorf("GenerateRandomID() returned duplicate ID: %s", id)
				}
				ids[id] = true

				// Check ID has reasonable length (base64 encoded)
				// Base64 encoding increases size by ~33%
				minLen := (tt.byteLength * 4) / 3
				if len(id) < minLen {
					t.Errorf("GenerateRandomID() length = %d, want at least %d", len(id), minLen)
				}
			}

			// Ensure we generated 100 unique IDs
			if len(ids) != 100 {
				t.Errorf("Generated %d unique IDs, want 100", len(ids))
			}
		})
	}
}

func TestGenerateRandomToken(t *testing.T) {
	// Test that GenerateRandomToken is an alias for GenerateRandomID
	token1 := GenerateRandomToken(16)
	token2 := GenerateRandomToken(16)

	if token1 == "" {
		t.Error("GenerateRandomToken() returned empty string")
	}

	if token1 == token2 {
		t.Error("GenerateRandomToken() returned same token twice")
	}
}

func BenchmarkExtractBearerToken(b *testing.B) {
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ExtractBearerToken(req)
	}
}

func BenchmarkExtractClientIP(b *testing.B) {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "192.168.1.100:12345"
	req.Header.Set("X-Forwarded-For", "203.0.113.1, 198.51.100.1")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ExtractClientIP(req)
	}
}

func BenchmarkGenerateRandomID(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = GenerateRandomID(16)
	}
}
