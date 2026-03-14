package proxy

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestRewriteCookiePath_CaseInsensitive tests that path rewriting is case-insensitive
func TestRewriteCookiePath_CaseInsensitive(t *testing.T) {
	testCases := []struct {
		name     string
		cookie   string
		prefix   string
		wantPath string
	}{
		{
			name:     "lowercase path",
			cookie:   "session=abc; path=/app; HttpOnly",
			prefix:   "/proxy",
			wantPath: "/proxy/app",
		},
		{
			name:     "uppercase PATH",
			cookie:   "session=abc; PATH=/app; HttpOnly",
			prefix:   "/proxy",
			wantPath: "/proxy/app",
		},
		{
			name:     "mixed case Path",
			cookie:   "session=abc; Path=/app; HttpOnly",
			prefix:   "/proxy",
			wantPath: "/proxy/app",
		},
		{
			name:     "weird case pAtH",
			cookie:   "session=abc; pAtH=/app; HttpOnly",
			prefix:   "/proxy",
			wantPath: "/proxy/app",
		},
		{
			name:     "root path lowercase",
			cookie:   "session=abc; path=/",
			prefix:   "/grafana",
			wantPath: "/grafana",
		},
		{
			name:     "root path uppercase",
			cookie:   "session=abc; PATH=/",
			prefix:   "/grafana",
			wantPath: "/grafana",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := rewriteCookiePath(tc.cookie, tc.prefix)

			// Check that the result contains the expected path
			if !containsPath(result, tc.wantPath) {
				t.Errorf("rewriteCookiePath(%q, %q) = %q, want path=%q", tc.cookie, tc.prefix, result, tc.wantPath)
			}
		})
	}
}

// containsPath checks if cookie string contains Path=value
func containsPath(cookie, expectedPath string) bool {
	// Check for Path=value in the result
	expected := "Path=" + expectedPath
	return contains(cookie, expected)
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// TestRewriteCookiePath_PreservesOtherAttributes tests that other cookie attributes are preserved
func TestRewriteCookiePath_PreservesOtherAttributes(t *testing.T) {
	testCases := []struct {
		name   string
		cookie string
		prefix string
		want   []string // Attributes that should be present
	}{
		{
			name:   "preserves HttpOnly",
			cookie: "session=abc; Path=/; HttpOnly",
			prefix: "/proxy",
			want:   []string{"HttpOnly", "session=abc"},
		},
		{
			name:   "preserves Secure",
			cookie: "session=abc; Path=/; Secure",
			prefix: "/proxy",
			want:   []string{"Secure", "session=abc"},
		},
		{
			name:   "preserves SameSite",
			cookie: "session=abc; Path=/; SameSite=Strict",
			prefix: "/proxy",
			want:   []string{"SameSite=Strict", "session=abc"},
		},
		{
			name:   "preserves Max-Age",
			cookie: "session=abc; Path=/; Max-Age=3600",
			prefix: "/proxy",
			want:   []string{"Max-Age=3600", "session=abc"},
		},
		{
			name:   "preserves multiple attributes",
			cookie: "session=abc; Path=/; Secure; HttpOnly; SameSite=Lax",
			prefix: "/proxy",
			want:   []string{"Secure", "HttpOnly", "SameSite=Lax", "session=abc"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := rewriteCookiePath(tc.cookie, tc.prefix)

			for _, attr := range tc.want {
				if !containsSubstring(result, attr) {
					t.Errorf("rewriteCookiePath result %q missing attribute %q", result, attr)
				}
			}
		})
	}
}

// TestCookieResponseWriter_StripCookies tests cookie stripping functionality
func TestCookieResponseWriter_StripCookies(t *testing.T) {
	w := httptest.NewRecorder()
	cw := NewCookieResponseWriter(w, true, false, "")

	// Set a cookie
	cw.Header().Add("Set-Cookie", "session=abc")
	cw.WriteHeader(http.StatusOK)

	// Cookie should be stripped
	cookies := w.Header().Values("Set-Cookie")
	if len(cookies) != 0 {
		t.Errorf("expected cookies to be stripped, got %v", cookies)
	}
}

// TestCookieResponseWriter_RewritePaths tests cookie path rewriting
func TestCookieResponseWriter_RewritePaths(t *testing.T) {
	w := httptest.NewRecorder()
	cw := NewCookieResponseWriter(w, false, true, "/apps/grafana")

	// Set a cookie with path
	cw.Header().Add("Set-Cookie", "session=abc; Path=/")
	cw.WriteHeader(http.StatusOK)

	// Check the rewritten cookie
	cookies := w.Header().Values("Set-Cookie")
	if len(cookies) != 1 {
		t.Fatalf("expected 1 cookie, got %d", len(cookies))
	}

	if !containsPath(cookies[0], "/apps/grafana") {
		t.Errorf("cookie path not rewritten correctly: %s", cookies[0])
	}
}

// TestCookieResponseWriter_StatusCode tests status code tracking
func TestCookieResponseWriter_StatusCode(t *testing.T) {
	w := httptest.NewRecorder()
	cw := NewCookieResponseWriter(w, false, false, "")

	// Default should be 200
	if cw.StatusCode() != http.StatusOK {
		t.Errorf("default StatusCode() = %d, want 200", cw.StatusCode())
	}

	// After WriteHeader
	cw.WriteHeader(http.StatusCreated)
	if cw.StatusCode() != http.StatusCreated {
		t.Errorf("StatusCode() = %d, want 201", cw.StatusCode())
	}
}

// TestCookieResponseWriter_Flush tests Flusher interface
func TestCookieResponseWriter_ImplementsFlusher(t *testing.T) {
	w := httptest.NewRecorder()
	cw := NewCookieResponseWriter(w, false, false, "")

	// Should implement http.Flusher
	_, ok := interface{}(cw).(http.Flusher)
	if !ok {
		t.Error("CookieResponseWriter should implement http.Flusher")
	}
}

// TestCookieResponseWriter_FlushPassesThrough tests that Flush is called on underlying writer
func TestCookieResponseWriter_FlushPassesThrough(t *testing.T) {
	w := httptest.NewRecorder()
	cw := NewCookieResponseWriter(w, false, false, "")

	// Write some data
	cw.Write([]byte("test"))

	// Flush
	if f, ok := interface{}(cw).(http.Flusher); ok {
		f.Flush()
	}

	// ResponseRecorder's Flush is a no-op, but it shouldn't panic
	if w.Body.String() != "test" {
		t.Errorf("body = %q, want %q", w.Body.String(), "test")
	}
}

// TestRewriteCookieDomain tests that Domain attribute is rewritten or removed
func TestRewriteCookieDomain(t *testing.T) {
	testCases := []struct {
		name          string
		cookie        string
		proxyHost     string
		shouldHave    []string
		shouldNotHave []string
	}{
		{
			name:          "remove domain pointing to backend",
			cookie:        "session=abc; Domain=backend.local; Path=/",
			proxyHost:     "nexus.local",
			shouldHave:    []string{"session=abc", "Path=/"},
			shouldNotHave: []string{"Domain=backend.local"},
		},
		{
			name:          "rewrite domain to proxy host",
			cookie:        "session=abc; Domain=192.168.1.50; Path=/",
			proxyHost:     "nexus.local",
			shouldHave:    []string{"session=abc", "Path=/", "Domain=nexus.local"},
			shouldNotHave: []string{"Domain=192.168.1.50"},
		},
		{
			name:          "preserve domain if it matches proxy",
			cookie:        "session=abc; Domain=nexus.local; Path=/",
			proxyHost:     "nexus.local",
			shouldHave:    []string{"session=abc", "Domain=nexus.local", "Path=/"},
			shouldNotHave: []string{},
		},
		{
			name:          "handle domain with leading dot",
			cookie:        "session=abc; Domain=.backend.local; Path=/",
			proxyHost:     "nexus.local",
			shouldHave:    []string{"session=abc", "Path=/", "Domain=nexus.local"},
			shouldNotHave: []string{"Domain=.backend.local"},
		},
		{
			name:          "no domain attribute - leave as is",
			cookie:        "session=abc; Path=/; HttpOnly",
			proxyHost:     "nexus.local",
			shouldHave:    []string{"session=abc", "Path=/", "HttpOnly"},
			shouldNotHave: []string{},
		},
		{
			name:          "case insensitive DOMAIN",
			cookie:        "session=abc; DOMAIN=backend.local; Path=/",
			proxyHost:     "nexus.local",
			shouldHave:    []string{"session=abc", "Path=/", "Domain=nexus.local"},
			shouldNotHave: []string{"DOMAIN=backend.local"},
		},
		{
			name:          "preserve other attributes when rewriting domain",
			cookie:        "session=abc; Domain=backend.local; Path=/; Secure; HttpOnly; SameSite=Strict",
			proxyHost:     "nexus.local",
			shouldHave:    []string{"session=abc", "Path=/", "Secure", "HttpOnly", "SameSite=Strict", "Domain=nexus.local"},
			shouldNotHave: []string{"Domain=backend.local"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := rewriteCookieDomain(tc.cookie, tc.proxyHost)

			for _, attr := range tc.shouldHave {
				if !containsSubstring(result, attr) {
					t.Errorf("rewriteCookieDomain result %q missing expected %q", result, attr)
				}
			}

			for _, attr := range tc.shouldNotHave {
				if containsSubstring(result, attr) {
					t.Errorf("rewriteCookieDomain result %q should not contain %q", result, attr)
				}
			}
		})
	}
}

// TestCookieResponseWriter_RewriteDomain tests domain rewriting via CookieResponseWriter
func TestCookieResponseWriter_RewriteDomain(t *testing.T) {
	w := httptest.NewRecorder()
	cw := NewCookieResponseWriterWithDomain(w, false, true, "/apps/myapp/", "nexus.local")

	// Set a cookie with domain pointing to backend
	cw.Header().Add("Set-Cookie", "session=abc; Domain=backend.local; Path=/")
	cw.WriteHeader(http.StatusOK)

	// Check the rewritten cookie
	cookies := w.Header().Values("Set-Cookie")
	if len(cookies) != 1 {
		t.Fatalf("expected 1 cookie, got %d", len(cookies))
	}

	// Should have Domain=nexus.local
	if !containsSubstring(cookies[0], "Domain=nexus.local") {
		t.Errorf("cookie domain not rewritten correctly: %s", cookies[0])
	}

	// Should not have Domain=backend.local
	if containsSubstring(cookies[0], "Domain=backend.local") {
		t.Errorf("cookie still contains backend domain: %s", cookies[0])
	}
}
