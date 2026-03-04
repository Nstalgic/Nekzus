package middleware

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestLimitRequestBody_AllowsSmallBody verifies small bodies are allowed
func TestLimitRequestBody_AllowsSmallBody(t *testing.T) {
	maxSize := int64(1024) // 1KB

	var bodyContent []byte
	handler := LimitRequestBody(maxSize)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error
		bodyContent, err = io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("Failed to read body: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))

	// Create a body smaller than limit
	body := bytes.Repeat([]byte("a"), 100)
	req := httptest.NewRequest("POST", "/api/test", bytes.NewReader(body))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}

	if len(bodyContent) != 100 {
		t.Errorf("Body length = %d, want 100", len(bodyContent))
	}
}

// TestLimitRequestBody_BlocksLargeBody verifies large bodies trigger error
func TestLimitRequestBody_BlocksLargeBody(t *testing.T) {
	maxSize := int64(100) // 100 bytes

	handler := LimitRequestBody(maxSize)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := io.ReadAll(r.Body)
		if err != nil {
			// Expected error when body exceeds limit
			http.Error(w, "request too large", http.StatusRequestEntityTooLarge)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))

	// Create a body larger than limit
	body := bytes.Repeat([]byte("a"), 200)
	req := httptest.NewRequest("POST", "/api/test", bytes.NewReader(body))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	// Handler should get error when reading body and return 413
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusRequestEntityTooLarge)
	}
}

// TestLimitRequestBody_ExactLimit verifies body at exact limit is allowed
func TestLimitRequestBody_ExactLimit(t *testing.T) {
	maxSize := int64(100)

	handler := LimitRequestBody(maxSize)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "request too large", http.StatusRequestEntityTooLarge)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))

	// Create body exactly at limit
	body := bytes.Repeat([]byte("a"), 100)
	req := httptest.NewRequest("POST", "/api/test", bytes.NewReader(body))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d for exact limit", w.Code, http.StatusOK)
	}
}

// TestLimitRequestBody_EmptyBody verifies empty bodies work
func TestLimitRequestBody_EmptyBody(t *testing.T) {
	maxSize := int64(100)

	handler := LimitRequestBody(maxSize)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "error", http.StatusInternalServerError)
			return
		}
		if len(body) != 0 {
			t.Errorf("Body should be empty, got %d bytes", len(body))
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("POST", "/api/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}
}

// TestLimitRequestBody_DefaultLimit verifies default limit is used when 0 is passed
func TestLimitRequestBody_DefaultLimit(t *testing.T) {
	// Pass 0 to use default
	handler := LimitRequestBody(0)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "request too large", http.StatusRequestEntityTooLarge)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))

	// Body smaller than default 1MB should work
	body := bytes.Repeat([]byte("a"), 1000)
	req := httptest.NewRequest("POST", "/api/test", bytes.NewReader(body))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}
}

// TestLimitRequestBody_StreamingRead verifies partial reads work
func TestLimitRequestBody_StreamingRead(t *testing.T) {
	maxSize := int64(1000)

	handler := LimitRequestBody(maxSize)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Read in chunks
		buf := make([]byte, 100)
		totalRead := 0
		for {
			n, err := r.Body.Read(buf)
			totalRead += n
			if err == io.EOF {
				break
			}
			if err != nil {
				http.Error(w, "read error", http.StatusInternalServerError)
				return
			}
		}
		if totalRead != 500 {
			t.Errorf("Total read = %d, want 500", totalRead)
		}
		w.WriteHeader(http.StatusOK)
	}))

	body := strings.Repeat("a", 500)
	req := httptest.NewRequest("POST", "/api/test", strings.NewReader(body))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}
}

// TestLimitRequestBody_GETRequest verifies GET requests work (no body)
func TestLimitRequestBody_GETRequest(t *testing.T) {
	maxSize := int64(100)

	handler := LimitRequestBody(maxSize)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}
}
