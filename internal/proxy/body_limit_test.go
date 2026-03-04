package proxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMaxBodyReader_UnderLimit(t *testing.T) {
	w := httptest.NewRecorder()
	body := strings.NewReader("small body")
	r := httptest.NewRequest("POST", "/", body)

	MaxBodyReader(w, r, 1024) // 1KB limit

	// Should be able to read entire body
	data, err := io.ReadAll(r.Body)
	if err != nil {
		t.Errorf("unexpected error reading body: %v", err)
	}

	if string(data) != "small body" {
		t.Errorf("got %q, want %q", string(data), "small body")
	}
}

func TestMaxBodyReader_AtLimit(t *testing.T) {
	w := httptest.NewRecorder()
	body := strings.NewReader(strings.Repeat("a", 100))
	r := httptest.NewRequest("POST", "/", body)

	MaxBodyReader(w, r, 100) // Exact limit

	// Should be able to read entire body
	data, err := io.ReadAll(r.Body)
	if err != nil {
		t.Errorf("unexpected error reading body: %v", err)
	}

	if len(data) != 100 {
		t.Errorf("got %d bytes, want 100", len(data))
	}
}

func TestMaxBodyReader_OverLimit(t *testing.T) {
	w := httptest.NewRecorder()
	body := strings.NewReader(strings.Repeat("a", 200))
	r := httptest.NewRequest("POST", "/", body)

	MaxBodyReader(w, r, 100) // 100 byte limit

	// Reading should fail when limit exceeded
	_, err := io.ReadAll(r.Body)
	if err == nil {
		t.Error("expected error when body exceeds limit")
	}

	// Error should be http.ErrBodyReadAfterClose or similar
	if err.Error() != "http: request body too large" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestMaxBodyReader_DefaultLimit(t *testing.T) {
	w := httptest.NewRecorder()
	body := strings.NewReader("test")
	r := httptest.NewRequest("POST", "/", body)

	// When maxBytes is 0, should use default
	MaxBodyReader(w, r, 0)

	// Should still be able to read small body
	data, err := io.ReadAll(r.Body)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if string(data) != "test" {
		t.Errorf("got %q, want %q", string(data), "test")
	}
}

func TestMaxBodyReader_NegativeLimit(t *testing.T) {
	w := httptest.NewRecorder()
	body := strings.NewReader("test")
	r := httptest.NewRequest("POST", "/", body)

	// When maxBytes is negative, should use default
	MaxBodyReader(w, r, -1)

	// Should still be able to read small body
	data, err := io.ReadAll(r.Body)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if string(data) != "test" {
		t.Errorf("got %q, want %q", string(data), "test")
	}
}

func TestDefaultMaxRequestBodySize(t *testing.T) {
	// Verify the default is 100MB
	expected := int64(100 * 1024 * 1024)
	if DefaultMaxRequestBodySize != expected {
		t.Errorf("DefaultMaxRequestBodySize = %d, want %d", DefaultMaxRequestBodySize, expected)
	}
}

// TestMaxBodyReader_HandlerIntegration tests usage in a handler context
func TestMaxBodyReader_HandlerIntegration(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		MaxBodyReader(w, r, 10) // 10 byte limit

		_, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
			return
		}

		w.WriteHeader(http.StatusOK)
	})

	// Test small request passes
	t.Run("small request passes", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/", strings.NewReader("small"))
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
		}
	})

	// Test large request fails
	t.Run("large request fails", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/", strings.NewReader("this is way too large"))
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusRequestEntityTooLarge {
			t.Errorf("status = %d, want %d", w.Code, http.StatusRequestEntityTooLarge)
		}
	})
}
