package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestRequestID_GeneratesNewID verifies middleware generates ID when not present
func TestRequestID_GeneratesNewID(t *testing.T) {
	handler := RequestID()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check context has request ID
		requestID := GetRequestIDFromContext(r.Context())
		if requestID == "" {
			t.Error("Request ID should be in context")
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	// Check response header
	if got := w.Header().Get(RequestIDHeader); got == "" {
		t.Error("Missing X-Request-ID header in response")
	}
}

// TestRequestID_PreservesExistingID verifies middleware preserves existing ID from upstream
func TestRequestID_PreservesExistingID(t *testing.T) {
	existingID := "upstream-request-id-12345"

	var capturedID string
	handler := RequestID()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedID = GetRequestIDFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set(RequestIDHeader, existingID)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	// Should preserve the existing ID
	if capturedID != existingID {
		t.Errorf("Context request ID = %q, want %q", capturedID, existingID)
	}

	// Response should also have the same ID
	if got := w.Header().Get(RequestIDHeader); got != existingID {
		t.Errorf("Response header X-Request-ID = %q, want %q", got, existingID)
	}
}

// TestRequestID_UniqueIDs verifies each request gets a unique ID
func TestRequestID_UniqueIDs(t *testing.T) {
	handler := RequestID()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	ids := make(map[string]bool)

	// Make 100 requests and verify all IDs are unique
	for i := 0; i < 100; i++ {
		req := httptest.NewRequest("GET", "/api/test", nil)
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		id := w.Header().Get(RequestIDHeader)
		if id == "" {
			t.Errorf("Request %d: missing X-Request-ID", i)
			continue
		}

		if ids[id] {
			t.Errorf("Request %d: duplicate request ID %q", i, id)
		}
		ids[id] = true
	}
}

// TestRequestID_IDFormat verifies the generated ID has expected format
func TestRequestID_IDFormat(t *testing.T) {
	handler := RequestID()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	id := w.Header().Get(RequestIDHeader)

	// Should be 16 hex characters (8 bytes)
	if len(id) != 16 {
		t.Errorf("Request ID length = %d, want 16", len(id))
	}

	// Should only contain hex characters
	for _, c := range id {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Errorf("Request ID contains non-hex character: %c", c)
			break
		}
	}
}

// TestRequestID_ContextEmpty verifies GetRequestIDFromContext returns empty for no context
func TestRequestID_ContextEmpty(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/test", nil)

	// No middleware applied, so no request ID in context
	id := GetRequestIDFromContext(req.Context())
	if id != "" {
		t.Errorf("Expected empty request ID, got %q", id)
	}
}

// TestRequestID_MiddlewareChain verifies request ID works in middleware chain
func TestRequestID_MiddlewareChain(t *testing.T) {
	// Chain: RequestID -> APIVersion -> Handler
	handler := RequestID()(
		APIVersion("1.0.0")(
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requestID := GetRequestIDFromContext(r.Context())
				if requestID == "" {
					t.Error("Request ID not available in chained handler")
				}
				w.WriteHeader(http.StatusOK)
			}),
		),
	)

	req := httptest.NewRequest("GET", "/api/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	// Both headers should be present
	if w.Header().Get(RequestIDHeader) == "" {
		t.Error("Missing X-Request-ID in response")
	}
	if w.Header().Get("X-API-Version") == "" {
		t.Error("Missing X-API-Version in response")
	}
}
