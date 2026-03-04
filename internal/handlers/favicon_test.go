package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestFaviconHandler_FetchFromService tests fetching favicon from service
func TestFaviconHandler_FetchFromService(t *testing.T) {
	// Create a mock upstream server that serves a favicon
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/favicon.ico" {
			w.Header().Set("Content-Type", "image/x-icon")
			w.Write([]byte("fake-ico-data"))
			return
		}
		http.NotFound(w, r)
	}))
	defer upstream.Close()

	// Create favicon handler
	handler := NewFaviconHandler(nil, 24*time.Hour)

	// Create a mock app registry that returns our test app
	handler.SetAppResolver(func(appID string) (string, bool) {
		if appID == "test-app" {
			return upstream.URL, true
		}
		return "", false
	})

	// Request favicon
	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/test-app/favicon", nil)
	w := httptest.NewRecorder()

	handler.HandleFavicon(w, req, "test-app")

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	if w.Header().Get("Content-Type") != "image/x-icon" {
		t.Errorf("Expected Content-Type image/x-icon, got %s", w.Header().Get("Content-Type"))
	}

	if w.Body.String() != "fake-ico-data" {
		t.Errorf("Expected favicon data, got %s", w.Body.String())
	}
}

// TestFaviconHandler_FetchPNG tests fetching favicon.png fallback
func TestFaviconHandler_FetchPNG(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/favicon.png" {
			w.Header().Set("Content-Type", "image/png")
			w.Write([]byte("fake-png-data"))
			return
		}
		http.NotFound(w, r)
	}))
	defer upstream.Close()

	handler := NewFaviconHandler(nil, 24*time.Hour)
	handler.SetAppResolver(func(appID string) (string, bool) {
		if appID == "test-app" {
			return upstream.URL, true
		}
		return "", false
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/test-app/favicon", nil)
	w := httptest.NewRecorder()

	handler.HandleFavicon(w, req, "test-app")

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	if w.Header().Get("Content-Type") != "image/png" {
		t.Errorf("Expected Content-Type image/png, got %s", w.Header().Get("Content-Type"))
	}
}

// TestFaviconHandler_ParseHTMLForIcon tests parsing HTML for icon link
func TestFaviconHandler_ParseHTMLForIcon(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(`<!DOCTYPE html>
<html>
<head>
	<link rel="icon" href="/assets/app-icon.png">
</head>
<body>Hello</body>
</html>`))
			return
		}
		if r.URL.Path == "/assets/app-icon.png" {
			w.Header().Set("Content-Type", "image/png")
			w.Write([]byte("html-icon-data"))
			return
		}
		http.NotFound(w, r)
	}))
	defer upstream.Close()

	handler := NewFaviconHandler(nil, 24*time.Hour)
	handler.SetAppResolver(func(appID string) (string, bool) {
		if appID == "test-app" {
			return upstream.URL, true
		}
		return "", false
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/test-app/favicon", nil)
	w := httptest.NewRecorder()

	handler.HandleFavicon(w, req, "test-app")

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	if w.Body.String() != "html-icon-data" {
		t.Errorf("Expected html-icon-data, got %s", w.Body.String())
	}
}

// TestFaviconHandler_Cache tests that favicons are cached
func TestFaviconHandler_Cache(t *testing.T) {
	fetchCount := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/favicon.ico" {
			fetchCount++
			w.Header().Set("Content-Type", "image/x-icon")
			w.Write([]byte("cached-favicon"))
			return
		}
		http.NotFound(w, r)
	}))
	defer upstream.Close()

	handler := NewFaviconHandler(nil, 24*time.Hour)
	handler.SetAppResolver(func(appID string) (string, bool) {
		if appID == "test-app" {
			return upstream.URL, true
		}
		return "", false
	})

	// First request - should fetch
	req1 := httptest.NewRequest(http.MethodGet, "/api/v1/apps/test-app/favicon", nil)
	w1 := httptest.NewRecorder()
	handler.HandleFavicon(w1, req1, "test-app")

	if w1.Code != http.StatusOK {
		t.Fatalf("First request failed: %d", w1.Code)
	}

	// Second request - should use cache
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/apps/test-app/favicon", nil)
	w2 := httptest.NewRecorder()
	handler.HandleFavicon(w2, req2, "test-app")

	if w2.Code != http.StatusOK {
		t.Fatalf("Second request failed: %d", w2.Code)
	}

	if fetchCount != 1 {
		t.Errorf("Expected 1 fetch (cached), got %d", fetchCount)
	}
}

// TestFaviconHandler_AppNotFound tests 404 for unknown app
func TestFaviconHandler_AppNotFound(t *testing.T) {
	handler := NewFaviconHandler(nil, 24*time.Hour)
	handler.SetAppResolver(func(appID string) (string, bool) {
		return "", false
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/unknown-app/favicon", nil)
	w := httptest.NewRecorder()

	handler.HandleFavicon(w, req, "unknown-app")

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", w.Code)
	}
}

// TestFaviconHandler_ServiceUnavailable tests handling when service is down
func TestFaviconHandler_ServiceUnavailable(t *testing.T) {
	handler := NewFaviconHandler(nil, 24*time.Hour)
	handler.SetAppResolver(func(appID string) (string, bool) {
		if appID == "dead-app" {
			return "http://localhost:59999", true // Non-existent port
		}
		return "", false
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/dead-app/favicon", nil)
	w := httptest.NewRecorder()

	handler.HandleFavicon(w, req, "dead-app")

	// Should return 404 or 502 when service unavailable
	if w.Code != http.StatusNotFound && w.Code != http.StatusBadGateway {
		t.Errorf("Expected status 404 or 502, got %d", w.Code)
	}
}

// TestFaviconHandler_MethodNotAllowed tests that only GET is allowed
func TestFaviconHandler_MethodNotAllowed(t *testing.T) {
	handler := NewFaviconHandler(nil, 24*time.Hour)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/test-app/favicon", nil)
	w := httptest.NewRecorder()

	handler.HandleFavicon(w, req, "test-app")

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405, got %d", w.Code)
	}
}

// TestFaviconHandler_CacheHeaders tests cache control headers
func TestFaviconHandler_CacheHeaders(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/favicon.ico" {
			w.Header().Set("Content-Type", "image/x-icon")
			w.Write([]byte("favicon"))
			return
		}
		http.NotFound(w, r)
	}))
	defer upstream.Close()

	handler := NewFaviconHandler(nil, 24*time.Hour)
	handler.SetAppResolver(func(appID string) (string, bool) {
		return upstream.URL, true
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/test-app/favicon", nil)
	w := httptest.NewRecorder()

	handler.HandleFavicon(w, req, "test-app")

	// Should have cache headers
	cacheControl := w.Header().Get("Cache-Control")
	if cacheControl == "" {
		t.Error("Expected Cache-Control header")
	}
}

// TestParseFaviconFromHTML tests the HTML parsing function
func TestParseFaviconFromHTML(t *testing.T) {
	tests := []struct {
		name     string
		html     string
		expected string
	}{
		{
			name:     "link rel icon",
			html:     `<link rel="icon" href="/favicon.ico">`,
			expected: "/favicon.ico",
		},
		{
			name:     "link rel shortcut icon",
			html:     `<link rel="shortcut icon" href="/icon.png">`,
			expected: "/icon.png",
		},
		{
			name:     "link rel apple-touch-icon",
			html:     `<link rel="apple-touch-icon" href="/apple-icon.png">`,
			expected: "/apple-icon.png",
		},
		{
			name:     "double quotes",
			html:     `<link rel="icon" href="/favicon.ico">`,
			expected: "/favicon.ico",
		},
		{
			name:     "single quotes",
			html:     `<link rel='icon' href='/favicon.ico'>`,
			expected: "/favicon.ico",
		},
		{
			name:     "no icon",
			html:     `<html><head><title>Test</title></head></html>`,
			expected: "",
		},
		{
			name:     "complex HTML",
			html:     `<!DOCTYPE html><html><head><meta charset="utf-8"><link rel="stylesheet" href="/style.css"><link rel="icon" type="image/png" href="/assets/logo.png"></head></html>`,
			expected: "/assets/logo.png",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseFaviconFromHTML(tt.html)
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

// --- Favicon Improvements Tests ---

// TestFaviconHandler_CORSHeaders tests that CORS headers are set for mobile apps
func TestFaviconHandler_CORSHeaders(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/favicon.ico" {
			w.Header().Set("Content-Type", "image/x-icon")
			w.Write([]byte("favicon"))
			return
		}
		http.NotFound(w, r)
	}))
	defer upstream.Close()

	handler := NewFaviconHandler(nil, 24*time.Hour)
	handler.SetAppResolver(func(appID string) (string, bool) {
		return upstream.URL, true
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/test-app/favicon", nil)
	w := httptest.NewRecorder()

	handler.HandleFavicon(w, req, "test-app")

	// Verify CORS headers are set
	if w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Errorf("Expected Access-Control-Allow-Origin: *, got %q", w.Header().Get("Access-Control-Allow-Origin"))
	}
}

// TestFaviconHandler_FailureCacheShortTTL tests that failures have shorter cache TTL
func TestFaviconHandler_FailureCacheShortTTL(t *testing.T) {
	fetchCount := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fetchCount++
		http.NotFound(w, r)
	}))
	defer upstream.Close()

	// Use short failure TTL for testing
	handler := NewFaviconHandler(nil, 24*time.Hour)
	handler.failureCacheTTL = 100 * time.Millisecond // Short TTL for test
	handler.SetAppResolver(func(appID string) (string, bool) {
		return upstream.URL, true
	})

	// First request - should try to fetch (tries multiple paths)
	req1 := httptest.NewRequest(http.MethodGet, "/api/v1/apps/test-app/favicon", nil)
	w1 := httptest.NewRecorder()
	handler.HandleFavicon(w1, req1, "test-app")

	// Handler tries /favicon.ico, /favicon.png, then HTML parse - so 3+ fetches
	firstAttemptFetches := fetchCount
	if firstAttemptFetches < 1 {
		t.Errorf("Expected at least 1 fetch on first request, got %d", firstAttemptFetches)
	}

	// Second immediate request - should use cache (no new fetches)
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/apps/test-app/favicon", nil)
	w2 := httptest.NewRecorder()
	handler.HandleFavicon(w2, req2, "test-app")

	// Fetch count should remain the same (cached failure)
	if fetchCount != firstAttemptFetches {
		t.Errorf("Expected cached response, fetch count before: %d, after: %d", firstAttemptFetches, fetchCount)
	}

	// Wait for failure cache to expire
	time.Sleep(150 * time.Millisecond)

	// Third request after TTL - should retry (new fetches)
	req3 := httptest.NewRequest(http.MethodGet, "/api/v1/apps/test-app/favicon", nil)
	w3 := httptest.NewRecorder()
	handler.HandleFavicon(w3, req3, "test-app")

	if fetchCount <= firstAttemptFetches {
		t.Errorf("Expected retry after failure cache TTL expired, fetch count before: %d, after: %d", firstAttemptFetches, fetchCount)
	}
}

// TestFaviconHandler_RedirectWithRelativePath tests favicon discovery when app redirects
// to a subpath and uses relative favicon URLs (like Transmission which redirects to /transmission/web/)
func TestFaviconHandler_RedirectWithRelativePath(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			// Redirect to subpath (like Transmission does)
			http.Redirect(w, r, "/transmission/web/", http.StatusFound)
		case "/transmission/web/":
			// Serve HTML with relative favicon path
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(`<!DOCTYPE html>
<html>
<head>
	<link rel="icon" href="./images/favicon.ico">
</head>
<body>Transmission Web</body>
</html>`))
		case "/transmission/web/images/favicon.ico":
			// Serve the favicon
			w.Header().Set("Content-Type", "image/x-icon")
			w.Write([]byte("transmission-favicon"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	handler := NewFaviconHandler(nil, 24*time.Hour)
	handler.SetAppResolver(func(appID string) (string, bool) {
		if appID == "transmission" {
			return upstream.URL, true
		}
		return "", false
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/transmission/favicon", nil)
	w := httptest.NewRecorder()

	handler.HandleFavicon(w, req, "transmission")

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	if w.Body.String() != "transmission-favicon" {
		t.Errorf("Expected transmission-favicon, got %s", w.Body.String())
	}
}

// TestFaviconHandler_ContentTypeSniff tests X-Content-Type-Options header
func TestFaviconHandler_ContentTypeSniff(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/favicon.ico" {
			w.Header().Set("Content-Type", "image/x-icon")
			w.Write([]byte("favicon"))
			return
		}
		http.NotFound(w, r)
	}))
	defer upstream.Close()

	handler := NewFaviconHandler(nil, 24*time.Hour)
	handler.SetAppResolver(func(appID string) (string, bool) {
		return upstream.URL, true
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/test-app/favicon", nil)
	w := httptest.NewRecorder()

	handler.HandleFavicon(w, req, "test-app")

	// Verify security header
	if w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("Expected X-Content-Type-Options: nosniff, got %q", w.Header().Get("X-Content-Type-Options"))
	}
}
