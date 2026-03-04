package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestCORS_AllowedOrigin verifies that allowed origins receive CORS headers
func TestCORS_AllowedOrigin(t *testing.T) {
	opts := CORSOptions{
		AllowedOrigins: []string{"https://example.com", "https://app.example.com"},
		AllowedMethods: []string{"GET", "POST"},
		AllowedHeaders: []string{"Content-Type"},
	}

	handler := CORS(opts)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("Origin", "https://example.com")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "https://example.com" {
		t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, "https://example.com")
	}
}

// TestCORS_DisallowedOrigin verifies that disallowed origins don't receive CORS headers
func TestCORS_DisallowedOrigin(t *testing.T) {
	opts := CORSOptions{
		AllowedOrigins: []string{"https://example.com"},
	}

	handler := CORS(opts)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("Origin", "https://evil.com")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Access-Control-Allow-Origin should be empty for disallowed origin, got %q", got)
	}
}

// TestCORS_WildcardOrigin verifies that wildcard allows all origins
func TestCORS_WildcardOrigin(t *testing.T) {
	opts := CORSOptions{
		AllowedOrigins: []string{"*"},
	}

	handler := CORS(opts)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("Origin", "https://any-site.com")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "https://any-site.com" {
		t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, "https://any-site.com")
	}
}

// TestCORS_PreflightRequest verifies OPTIONS preflight handling
func TestCORS_PreflightRequest(t *testing.T) {
	opts := CORSOptions{
		AllowedOrigins: []string{"https://example.com"},
		AllowedMethods: []string{"GET", "POST", "PUT"},
		AllowedHeaders: []string{"Content-Type", "Authorization"},
		MaxAge:         3600,
	}

	handler := CORS(opts)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Handler should not be called for preflight request")
	}))

	req := httptest.NewRequest("OPTIONS", "/api/test", nil)
	req.Header.Set("Origin", "https://example.com")
	req.Header.Set("Access-Control-Request-Method", "POST")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("Preflight status = %d, want %d", w.Code, http.StatusNoContent)
	}

	if got := w.Header().Get("Access-Control-Allow-Methods"); got == "" {
		t.Error("Missing Access-Control-Allow-Methods header")
	}

	if got := w.Header().Get("Access-Control-Allow-Headers"); got == "" {
		t.Error("Missing Access-Control-Allow-Headers header")
	}

	if got := w.Header().Get("Access-Control-Max-Age"); got != "3600" {
		t.Errorf("Access-Control-Max-Age = %q, want %q", got, "3600")
	}
}

// TestCORS_Credentials verifies credentials support
func TestCORS_Credentials(t *testing.T) {
	opts := CORSOptions{
		AllowedOrigins:   []string{"https://example.com"},
		AllowCredentials: true,
	}

	handler := CORS(opts)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("Origin", "https://example.com")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if got := w.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Errorf("Access-Control-Allow-Credentials = %q, want %q", got, "true")
	}
}

// TestCORS_ExposedHeaders verifies exposed headers are set
func TestCORS_ExposedHeaders(t *testing.T) {
	opts := CORSOptions{
		AllowedOrigins: []string{"https://example.com"},
		ExposedHeaders: []string{"X-Custom-Header", "X-Another-Header"},
	}

	handler := CORS(opts)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("Origin", "https://example.com")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if got := w.Header().Get("Access-Control-Expose-Headers"); got == "" {
		t.Error("Missing Access-Control-Expose-Headers header")
	}
}

// TestCORS_VaryHeader verifies Vary: Origin header is set
func TestCORS_VaryHeader(t *testing.T) {
	opts := CORSOptions{
		AllowedOrigins: []string{"https://example.com"},
	}

	handler := CORS(opts)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("Origin", "https://example.com")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if got := w.Header().Get("Vary"); got != "Origin" {
		t.Errorf("Vary = %q, want %q", got, "Origin")
	}
}

// TestCORS_NoOrigin verifies requests without Origin header pass through
func TestCORS_NoOrigin(t *testing.T) {
	opts := CORSOptions{
		AllowedOrigins: []string{"https://example.com"},
	}

	handlerCalled := false
	handler := CORS(opts)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/test", nil)
	// No Origin header
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if !handlerCalled {
		t.Error("Handler should be called for requests without Origin")
	}

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Access-Control-Allow-Origin should be empty for no-origin request, got %q", got)
	}
}

// TestCORS_DefaultOptions verifies default options are sensible
func TestCORS_DefaultOptions(t *testing.T) {
	opts := DefaultCORSOptions()

	if len(opts.AllowedOrigins) == 0 {
		t.Error("DefaultCORSOptions should have allowed origins")
	}

	if len(opts.AllowedMethods) == 0 {
		t.Error("DefaultCORSOptions should have allowed methods")
	}

	if len(opts.AllowedHeaders) == 0 {
		t.Error("DefaultCORSOptions should have allowed headers")
	}
}
