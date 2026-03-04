package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSecurityHeaders_AllHeadersPresent(t *testing.T) {
	handler := SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	tests := []struct {
		header   string
		expected string
	}{
		{"Strict-Transport-Security", "max-age=31536000; includeSubDomains"},
		{"X-Content-Type-Options", "nosniff"},
		{"X-Frame-Options", "DENY"},
		{"Cache-Control", "no-store"},
	}

	for _, tt := range tests {
		t.Run(tt.header, func(t *testing.T) {
			got := w.Header().Get(tt.header)
			if got != tt.expected {
				t.Errorf("%s = %q, want %q", tt.header, got, tt.expected)
			}
		})
	}
}

func TestSecurityHeaders_PassesThrough(t *testing.T) {
	called := false
	handler := SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("test"))
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if !called {
		t.Error("Handler was not called")
	}

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}

	if w.Body.String() != "test" {
		t.Errorf("Body = %q, want %q", w.Body.String(), "test")
	}
}

func TestSecurityHeaders_PreservesOtherHeaders(t *testing.T) {
	handler := SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Custom-Header", "custom-value")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	// Security headers should be present
	if w.Header().Get("Strict-Transport-Security") == "" {
		t.Error("Missing Strict-Transport-Security header")
	}

	// Custom headers should also be present
	if w.Header().Get("X-Custom-Header") != "custom-value" {
		t.Error("Custom header was lost")
	}

	if w.Header().Get("Content-Type") != "application/json" {
		t.Error("Content-Type header was lost")
	}
}

func TestSecurityHeaders_DoesNotOverrideExisting(t *testing.T) {
	// If the application sets a custom Cache-Control, we should NOT override it
	handler := SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "max-age=3600")
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	// Security middleware sets headers BEFORE handler runs, so handler can override
	// This means the handler's Cache-Control should win
	got := w.Header().Get("Cache-Control")
	if got != "max-age=3600" {
		t.Errorf("Cache-Control = %q, want %q (handler should be able to override)", got, "max-age=3600")
	}
}
