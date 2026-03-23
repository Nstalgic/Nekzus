package proxy

import (
	"bytes"
	"compress/gzip"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestHTMLRewritingResponseWriter_MaxSize tests buffer size limits
func TestHTMLRewritingResponseWriter_MaxSize(t *testing.T) {
	// Verify MaxHTMLBufferSize constant exists
	if MaxHTMLBufferSize <= 0 {
		t.Errorf("MaxHTMLBufferSize should be positive, got %d", MaxHTMLBufferSize)
	}

	// Should be 10MB
	if MaxHTMLBufferSize != 10*1024*1024 {
		t.Errorf("MaxHTMLBufferSize = %d, want %d", MaxHTMLBufferSize, 10*1024*1024)
	}
}

// TestHTMLRewritingResponseWriter_SmallHTML tests normal HTML rewriting
func TestHTMLRewritingResponseWriter_SmallHTML(t *testing.T) {
	w := httptest.NewRecorder()
	rw := NewHTMLRewritingResponseWriter(w, "/apps/test", "/apps/test/")

	// Set HTML content type
	rw.Header().Set("Content-Type", "text/html")
	rw.WriteHeader(http.StatusOK)

	// Write small HTML
	html := `<html><head><script src="/app.js"></script></head><body></body></html>`
	_, err := rw.Write([]byte(html))
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// Flush to send response
	err = rw.FlushHTML()
	if err != nil {
		t.Fatalf("FlushHTML failed: %v", err)
	}

	// Verify path was rewritten
	result := w.Body.String()
	if !strings.Contains(result, `src="/apps/test/app.js"`) {
		t.Errorf("Expected path to be rewritten, got: %s", result)
	}
}

// TestHTMLRewritingResponseWriter_NonHTML tests that non-HTML passes through
func TestHTMLRewritingResponseWriter_NonHTML(t *testing.T) {
	w := httptest.NewRecorder()
	rw := NewHTMLRewritingResponseWriter(w, "/apps/test", "/apps/test/")

	// Set JSON content type
	rw.Header().Set("Content-Type", "application/json")
	rw.WriteHeader(http.StatusOK)

	// Write JSON
	json := `{"url": "/app.js"}`
	_, err := rw.Write([]byte(json))
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// Verify JSON was not modified
	result := w.Body.String()
	if result != json {
		t.Errorf("Expected JSON to pass through unchanged, got: %s", result)
	}
}

// TestHTMLRewritingResponseWriter_FlushHTMLReturnsError tests FlushHTML behavior
func TestHTMLRewritingResponseWriter_FlushHTMLReturnsError(t *testing.T) {
	w := httptest.NewRecorder()
	rw := NewHTMLRewritingResponseWriter(w, "/apps/test", "/apps/test/")

	// Set HTML content type
	rw.Header().Set("Content-Type", "text/html")
	rw.WriteHeader(http.StatusOK)

	// Write HTML
	_, err := rw.Write([]byte("<html></html>"))
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// Flush should succeed
	err = rw.FlushHTML()
	if err != nil {
		t.Errorf("FlushHTML failed: %v", err)
	}
}

// TestHTMLRewritingResponseWriter_ImplementsFlusher tests http.Flusher interface
func TestHTMLRewritingResponseWriter_ImplementsFlusher(t *testing.T) {
	w := httptest.NewRecorder()
	rw := NewHTMLRewritingResponseWriter(w, "/apps/test", "/apps/test/")

	// Should implement http.Flusher
	_, ok := interface{}(rw).(http.Flusher)
	if !ok {
		t.Error("HTMLRewritingResponseWriter should implement http.Flusher")
	}
}

// TestRewriteHTMLPaths tests HTML path rewriting
func TestRewriteHTMLPaths(t *testing.T) {
	testCases := []struct {
		name       string
		html       string
		pathPrefix string
		expected   string
	}{
		{
			name:       "rewrite src attribute",
			html:       `<script src="/app.js"></script>`,
			pathPrefix: "/proxy",
			expected:   `<script src="/proxy/app.js"></script>`,
		},
		{
			name:       "rewrite href attribute",
			html:       `<link href="/style.css">`,
			pathPrefix: "/proxy",
			expected:   `<link href="/proxy/style.css">`,
		},
		{
			name:       "skip external URLs",
			html:       `<script src="https://cdn.example.com/lib.js"></script>`,
			pathPrefix: "/proxy",
			expected:   `<script src="https://cdn.example.com/lib.js"></script>`,
		},
		{
			name:       "skip data URIs",
			html:       `<img src="data:image/png;base64,ABC123">`,
			pathPrefix: "/proxy",
			expected:   `<img src="data:image/png;base64,ABC123">`,
		},
		{
			name:       "skip relative paths",
			html:       `<img src="images/logo.png">`,
			pathPrefix: "/proxy",
			expected:   `<img src="images/logo.png">`,
		},
		{
			name:       "skip api paths",
			html:       `<script src="/api/v1/config"></script>`,
			pathPrefix: "/proxy",
			expected:   `<script src="/api/v1/config"></script>`,
		},
		{
			name:       "already prefixed",
			html:       `<script src="/proxy/app.js"></script>`,
			pathPrefix: "/proxy",
			expected:   `<script src="/proxy/app.js"></script>`,
		},
		// Additional attributes
		{
			name:       "rewrite action attribute",
			html:       `<form action="/submit"></form>`,
			pathPrefix: "/proxy",
			expected:   `<form action="/proxy/submit"></form>`,
		},
		{
			name:       "rewrite formaction attribute",
			html:       `<button formaction="/process"></button>`,
			pathPrefix: "/proxy",
			expected:   `<button formaction="/proxy/process"></button>`,
		},
		{
			name:       "rewrite poster attribute",
			html:       `<video poster="/thumbnail.jpg"></video>`,
			pathPrefix: "/proxy",
			expected:   `<video poster="/proxy/thumbnail.jpg"></video>`,
		},
		{
			name:       "rewrite src at start of tag (no leading whitespace)",
			html:       `<script src="/initialize.js"></script>`,
			pathPrefix: "/apps/sonarrv3",
			expected:   `<script src="/apps/sonarrv3/initialize.js"></script>`,
		},
		{
			name:       "rewrite src attribute without leading space (check src rewriting only)",
			html:       `<body><script src="/initialize.js"></script></body>`,
			pathPrefix: "/apps/sonarrv3/",
			expected:   `<body><script src="/apps/sonarrv3/initialize.js"></script></body>`,
		},
		{
			name:       "rewrite multiple src attributes with mixed spacing",
			html:       `<script src="/first.js"></script><img src="/image.png" /><script src="/second.js"></script>`,
			pathPrefix: "/apps/test/",
			expected:   `<script src="/apps/test/first.js"></script><img src="/apps/test/image.png" /><script src="/apps/test/second.js"></script>`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Use same path for both pathPrefix and requestPath in these tests
			result := rewriteHTMLPaths(tc.html, tc.pathPrefix, tc.pathPrefix)
			if result != tc.expected {
				t.Errorf("rewriteHTMLPaths(%q, %q) = %q, want %q", tc.html, tc.pathPrefix, result, tc.expected)
			}
		})
	}
}

// TestGenerateFetchInterceptor tests JavaScript interceptor generation
func TestGenerateFetchInterceptor(t *testing.T) {
	testCases := []struct {
		name       string
		pathPrefix string
	}{
		{
			name:       "simple prefix",
			pathPrefix: "/apps/memos/",
		},
		{
			name:       "prefix without trailing slash",
			pathPrefix: "/apps/grafana",
		},
		{
			name:       "nested prefix",
			pathPrefix: "/proxy/apps/test/",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			script := generateFetchInterceptor(tc.pathPrefix)

			// Ensure trailing slash for comparison
			expectedPrefix := tc.pathPrefix
			if !strings.HasSuffix(expectedPrefix, "/") {
				expectedPrefix = expectedPrefix + "/"
			}

			// Verify script contains expected elements
			if !strings.Contains(script, "<script>") {
				t.Error("Expected script to start with <script> tag")
			}
			if !strings.Contains(script, "</script>") {
				t.Error("Expected script to end with </script> tag")
			}
			if !strings.Contains(script, "window.fetch") {
				t.Error("Expected script to intercept window.fetch")
			}
			if !strings.Contains(script, "XMLHttpRequest.prototype.open") {
				t.Error("Expected script to intercept XMLHttpRequest")
			}
			if !strings.Contains(script, expectedPrefix) {
				t.Errorf("Expected script to contain prefix %q", expectedPrefix)
			}

			// Verify IIFE pattern
			if !strings.Contains(script, "(function()") {
				t.Error("Expected script to use IIFE pattern")
			}
			if !strings.Contains(script, "})();") {
				t.Error("Expected script to close IIFE")
			}
		})
	}
}

// TestInjectBaseTag tests base tag injection
func TestInjectBaseTag(t *testing.T) {
	testCases := []struct {
		name              string
		html              string
		pathPrefix        string
		expectBase        bool
		expectInterceptor bool
	}{
		{
			name:              "inject base tag into empty head",
			html:              `<html><head></head><body></body></html>`,
			pathPrefix:        "/apps/memos/",
			expectBase:        true,
			expectInterceptor: true,
		},
		{
			name:              "inject into head with existing content",
			html:              `<html><head><title>Test</title></head><body></body></html>`,
			pathPrefix:        "/apps/test/",
			expectBase:        true,
			expectInterceptor: true,
		},
		{
			name:              "existing base tag - inject interceptor only",
			html:              `<html><head><base href="/other/"></head><body></body></html>`,
			pathPrefix:        "/apps/memos/",
			expectBase:        false, // Should not inject new base tag
			expectInterceptor: true,  // But SHOULD inject interceptor for fetch/XHR rewriting
		},
		{
			name:              "handle head tag with attributes",
			html:              `<html><head lang="en"></head><body></body></html>`,
			pathPrefix:        "/proxy/",
			expectBase:        true,
			expectInterceptor: true,
		},
		{
			name:              "do not inject into header tags",
			html:              `<html><head></head><body><header class="nav">Nav</header></body></html>`,
			pathPrefix:        "/apps/test/",
			expectBase:        true,
			expectInterceptor: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Use same path for both pathPrefix and requestPath in these tests
			result := injectBaseTag(tc.html, tc.pathPrefix, tc.pathPrefix)

			// Ensure trailing slash for comparison
			expectedPrefix := tc.pathPrefix
			if !strings.HasSuffix(expectedPrefix, "/") {
				expectedPrefix = expectedPrefix + "/"
			}

			if tc.expectBase {
				expectedBaseTag := `<base href="` + expectedPrefix + `">`
				if !strings.Contains(result, expectedBaseTag) {
					t.Errorf("Expected result to contain base tag %q, got: %s", expectedBaseTag, result)
				}
			}

			if tc.expectInterceptor {
				if !strings.Contains(result, "window.fetch") {
					t.Error("Expected result to contain fetch interceptor")
				}
				if !strings.Contains(result, "XMLHttpRequest.prototype.open") {
					t.Error("Expected result to contain XHR interceptor")
				}
			}

			// If base tag already exists, should not inject new one
			if !tc.expectBase && strings.Contains(strings.ToLower(tc.html), "<base") {
				baseCount := strings.Count(strings.ToLower(result), "<base")
				if baseCount > 1 {
					t.Error("Should not inject multiple base tags")
				}
			}

			// Verify base tag is only injected once (not into <header> tags)
			if tc.expectBase {
				baseCount := strings.Count(result, `<base href="`)
				if baseCount != 1 {
					t.Errorf("Expected exactly 1 base tag, found %d", baseCount)
				}
			}
		})
	}
}

// TestHTMLRewritingResponseWriter_RedirectLocationRewriting tests Location header rewriting for redirects
func TestHTMLRewritingResponseWriter_RedirectLocationRewriting(t *testing.T) {
	testCases := []struct {
		name           string
		pathPrefix     string
		statusCode     int
		locationHeader string
		expectedLoc    string
	}{
		{
			name:           "rewrite relative redirect",
			pathPrefix:     "/apps/uptime-kuma/",
			statusCode:     http.StatusFound,
			locationHeader: "/dashboard",
			expectedLoc:    "/apps/uptime-kuma/dashboard",
		},
		{
			name:           "rewrite 301 redirect",
			pathPrefix:     "/apps/test/",
			statusCode:     http.StatusMovedPermanently,
			locationHeader: "/login",
			expectedLoc:    "/apps/test/login",
		},
		{
			name:           "skip absolute URL",
			pathPrefix:     "/apps/test/",
			statusCode:     http.StatusFound,
			locationHeader: "https://example.com/path",
			expectedLoc:    "https://example.com/path",
		},
		{
			name:           "skip protocol-relative URL",
			pathPrefix:     "/apps/test/",
			statusCode:     http.StatusFound,
			locationHeader: "//cdn.example.com/path",
			expectedLoc:    "//cdn.example.com/path",
		},
		{
			name:           "skip already prefixed",
			pathPrefix:     "/apps/test/",
			statusCode:     http.StatusFound,
			locationHeader: "/apps/test/dashboard",
			expectedLoc:    "/apps/test/dashboard",
		},
		{
			name:           "no rewrite for 200 OK",
			pathPrefix:     "/apps/test/",
			statusCode:     http.StatusOK,
			locationHeader: "/dashboard",
			expectedLoc:    "/dashboard",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			rw := NewHTMLRewritingResponseWriter(w, tc.pathPrefix, tc.pathPrefix)

			// Set Location header and write status
			rw.Header().Set("Location", tc.locationHeader)
			rw.Header().Set("Content-Type", "text/plain")
			rw.WriteHeader(tc.statusCode)

			// Check Location header
			result := w.Header().Get("Location")
			if result != tc.expectedLoc {
				t.Errorf("Location header = %q, want %q", result, tc.expectedLoc)
			}
		})
	}
}

// TestHTMLRewritingResponseWriter_AbsoluteURLRedirectRewriting tests Location header rewriting for absolute URLs
func TestHTMLRewritingResponseWriter_AbsoluteURLRedirectRewriting(t *testing.T) {
	testCases := []struct {
		name           string
		pathPrefix     string
		requestHost    string
		requestScheme  string
		statusCode     int
		locationHeader string
		expectedLoc    string
	}{
		{
			name:           "rewrite absolute URL same host",
			pathPrefix:     "/apps/jackett/",
			requestHost:    "192.168.0.23:8088",
			requestScheme:  "http",
			statusCode:     http.StatusFound,
			locationHeader: "http://192.168.0.23:8088/UI/Login?ReturnUrl=%2FUI%2FDashboard",
			expectedLoc:    "http://192.168.0.23:8088/apps/jackett/UI/Login?ReturnUrl=%2FUI%2FDashboard",
		},
		{
			name:           "rewrite absolute URL same host https",
			pathPrefix:     "/apps/sonarr/",
			requestHost:    "nexus.local:443",
			requestScheme:  "https",
			statusCode:     http.StatusMovedPermanently,
			locationHeader: "https://nexus.local:443/login",
			expectedLoc:    "https://nexus.local:443/apps/sonarr/login",
		},
		{
			name:           "rewrite absolute URL same host no port match",
			pathPrefix:     "/apps/radarr/",
			requestHost:    "192.168.1.100",
			requestScheme:  "http",
			statusCode:     http.StatusFound,
			locationHeader: "http://192.168.1.100:8080/login",
			expectedLoc:    "http://192.168.1.100:8080/apps/radarr/login",
		},
		{
			name:           "skip absolute URL different host",
			pathPrefix:     "/apps/test/",
			requestHost:    "nexus.local:8088",
			requestScheme:  "http",
			statusCode:     http.StatusFound,
			locationHeader: "https://example.com/path",
			expectedLoc:    "https://example.com/path",
		},
		{
			name:           "skip absolute URL already prefixed",
			pathPrefix:     "/apps/jackett/",
			requestHost:    "192.168.0.23:8088",
			requestScheme:  "http",
			statusCode:     http.StatusFound,
			locationHeader: "http://192.168.0.23:8088/apps/jackett/dashboard",
			expectedLoc:    "http://192.168.0.23:8088/apps/jackett/dashboard",
		},
		{
			name:           "still rewrite relative paths",
			pathPrefix:     "/apps/jackett/",
			requestHost:    "192.168.0.23:8088",
			requestScheme:  "http",
			statusCode:     http.StatusFound,
			locationHeader: "/UI/Login",
			expectedLoc:    "/apps/jackett/UI/Login",
		},
		{
			name:           "no host info falls back to relative only",
			pathPrefix:     "/apps/test/",
			requestHost:    "",
			requestScheme:  "",
			statusCode:     http.StatusFound,
			locationHeader: "http://192.168.0.23:8088/login",
			expectedLoc:    "http://192.168.0.23:8088/login",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			rw := NewHTMLRewritingResponseWriterWithHost(w, tc.pathPrefix, tc.pathPrefix, tc.requestHost, tc.requestScheme)

			// Set Location header and write status
			rw.Header().Set("Location", tc.locationHeader)
			rw.Header().Set("Content-Type", "text/plain")
			rw.WriteHeader(tc.statusCode)

			// Check Location header
			result := w.Header().Get("Location")
			if result != tc.expectedLoc {
				t.Errorf("Location header = %q, want %q", result, tc.expectedLoc)
			}
		})
	}
}

// TestHTMLRewritingResponseWriter_SubpathBaseHref tests that base href is set to request path for apps with internal routing
func TestHTMLRewritingResponseWriter_SubpathBaseHref(t *testing.T) {
	// This test verifies that when an app like Transmission redirects to a subpath,
	// the base href is set to the actual request path, not just the route's pathBase.
	// This ensures relative paths like "./transmission-app.js" resolve correctly.

	testCases := []struct {
		name         string
		pathPrefix   string // Route's pathBase (e.g., /apps/transmission/)
		requestPath  string // Actual request path (e.g., /apps/transmission/transmission/web/)
		expectedBase string // Expected base href tag value
	}{
		{
			name:         "request at route root",
			pathPrefix:   "/apps/transmission/",
			requestPath:  "/apps/transmission/",
			expectedBase: "/apps/transmission/",
		},
		{
			name:         "request at subpath - Transmission style",
			pathPrefix:   "/apps/transmission/",
			requestPath:  "/apps/transmission/transmission/web/",
			expectedBase: "/apps/transmission/transmission/web/",
		},
		{
			name:         "request at subpath without trailing slash",
			pathPrefix:   "/apps/test/",
			requestPath:  "/apps/test/ui/dashboard",
			expectedBase: "/apps/test/ui/dashboard/",
		},
		{
			name:         "request at deep subpath",
			pathPrefix:   "/apps/grafana/",
			requestPath:  "/apps/grafana/d/abc123/dashboard",
			expectedBase: "/apps/grafana/d/abc123/dashboard/",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			rw := NewHTMLRewritingResponseWriter(w, tc.pathPrefix, tc.requestPath)

			// Set HTML content type
			rw.Header().Set("Content-Type", "text/html")
			rw.WriteHeader(http.StatusOK)

			// Write HTML with relative paths (like Transmission uses)
			html := `<!doctype html><html><head><title>Test</title></head><body><script src="./app.js"></script></body></html>`
			_, err := rw.Write([]byte(html))
			if err != nil {
				t.Fatalf("Write failed: %v", err)
			}

			// Flush to rewrite
			err = rw.FlushHTML()
			if err != nil {
				t.Fatalf("FlushHTML failed: %v", err)
			}

			result := w.Body.String()

			// Verify base href uses the request path
			expectedBaseTag := `<base href="` + tc.expectedBase + `">`
			if !strings.Contains(result, expectedBaseTag) {
				t.Errorf("Expected base href %q, got: %s", expectedBaseTag, result)
			}

			// Verify JavaScript interceptor still uses pathPrefix for absolute paths
			if !strings.Contains(result, tc.pathPrefix) {
				t.Errorf("Expected interceptor to contain pathPrefix %q", tc.pathPrefix)
			}
		})
	}
}

// TestHTMLRewritingResponseWriter_GzipDecompression tests that gzip-compressed HTML is decompressed before rewriting
func TestHTMLRewritingResponseWriter_GzipDecompression(t *testing.T) {
	// Create gzip-compressed HTML
	originalHTML := `<!doctype html><html><head></head><body><script src="/app.js"></script><link href="/style.css"></body></html>`

	var buf bytes.Buffer
	gzWriter := gzip.NewWriter(&buf)
	_, err := gzWriter.Write([]byte(originalHTML))
	if err != nil {
		t.Fatalf("Failed to gzip compress: %v", err)
	}
	gzWriter.Close()
	compressedHTML := buf.Bytes()

	w := httptest.NewRecorder()
	rw := NewHTMLRewritingResponseWriter(w, "/apps/test/", "/apps/test/")

	// Set headers like a real gzipped response
	rw.Header().Set("Content-Type", "text/html")
	rw.Header().Set("Content-Encoding", "gzip")
	rw.WriteHeader(http.StatusOK)

	// Write compressed content
	_, err = rw.Write(compressedHTML)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// Flush to decompress and rewrite
	err = rw.FlushHTML()
	if err != nil {
		t.Fatalf("FlushHTML failed: %v", err)
	}

	// Verify response
	result := w.Body.String()

	// Should have rewritten paths
	if !strings.Contains(result, `src="/apps/test/app.js"`) {
		t.Errorf("Expected src to be rewritten, got: %s", result)
	}
	if !strings.Contains(result, `href="/apps/test/style.css"`) {
		t.Errorf("Expected href to be rewritten, got: %s", result)
	}

	// Content-Encoding should be removed (we send uncompressed)
	if w.Header().Get("Content-Encoding") != "" {
		t.Errorf("Expected Content-Encoding to be removed, got: %s", w.Header().Get("Content-Encoding"))
	}
}

// --- CSS URL Rewriting Tests ---

// TestCSSURLRewriting tests that CSS url() paths are rewritten in inline styles
func TestCSSURLRewriting(t *testing.T) {
	testCases := []struct {
		name       string
		html       string
		pathPrefix string
		expected   string
	}{
		{
			name:       "inline style background url",
			html:       `<div style="background: url(/images/bg.png)"></div>`,
			pathPrefix: "/apps/test/",
			expected:   `<div style="background: url(/apps/test/images/bg.png)"></div>`,
		},
		{
			name:       "inline style with quotes",
			html:       `<div style="background-image: url('/assets/logo.png')"></div>`,
			pathPrefix: "/apps/test/",
			expected:   `<div style="background-image: url('/apps/test/assets/logo.png')"></div>`,
		},
		{
			name:       "inline style with double quotes",
			html:       `<div style='background: url("/icons/icon.svg")'></div>`,
			pathPrefix: "/apps/test/",
			expected:   `<div style='background: url("/apps/test/icons/icon.svg")'></div>`,
		},
		{
			name:       "style tag with url",
			html:       `<style>.logo { background: url(/assets/logo.png); }</style>`,
			pathPrefix: "/apps/test/",
			expected:   `<style>.logo { background: url(/apps/test/assets/logo.png); }</style>`,
		},
		{
			name:       "multiple urls in style",
			html:       `<style>.a { background: url(/a.png); } .b { background: url(/b.png); }</style>`,
			pathPrefix: "/apps/test/",
			expected:   `<style>.a { background: url(/apps/test/a.png); } .b { background: url(/apps/test/b.png); }</style>`,
		},
		{
			name:       "skip external url",
			html:       `<style>.logo { background: url(https://example.com/logo.png); }</style>`,
			pathPrefix: "/apps/test/",
			expected:   `<style>.logo { background: url(https://example.com/logo.png); }</style>`,
		},
		{
			name:       "skip data uri",
			html:       `<style>.icon { background: url(data:image/png;base64,ABC); }</style>`,
			pathPrefix: "/apps/test/",
			expected:   `<style>.icon { background: url(data:image/png;base64,ABC); }</style>`,
		},
		{
			name:       "skip relative path",
			html:       `<style>.bg { background: url(images/bg.png); }</style>`,
			pathPrefix: "/apps/test/",
			expected:   `<style>.bg { background: url(images/bg.png); }</style>`,
		},
		{
			name:       "font-face src url",
			html:       `<style>@font-face { src: url(/fonts/custom.woff2); }</style>`,
			pathPrefix: "/apps/test/",
			expected:   `<style>@font-face { src: url(/apps/test/fonts/custom.woff2); }</style>`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := rewriteHTMLPaths(tc.html, tc.pathPrefix, tc.pathPrefix)
			if result != tc.expected {
				t.Errorf("CSS URL rewriting failed:\nInput:    %s\nExpected: %s\nGot:      %s", tc.html, tc.expected, result)
			}
		})
	}
}

// TestBaseTagWithInterceptor tests that JS interceptor is injected even when base tag exists
func TestBaseTagWithInterceptor(t *testing.T) {
	testCases := []struct {
		name              string
		html              string
		pathPrefix        string
		expectBase        bool // Should inject NEW base tag
		expectInterceptor bool // Should inject JS interceptor
	}{
		{
			name:              "no existing base tag",
			html:              `<html><head><title>Test</title></head><body></body></html>`,
			pathPrefix:        "/apps/test/",
			expectBase:        true,
			expectInterceptor: true,
		},
		{
			name:              "existing base tag - should still inject interceptor",
			html:              `<html><head><base href="/"><title>Test</title></head><body></body></html>`,
			pathPrefix:        "/apps/test/",
			expectBase:        false, // Don't add another base tag
			expectInterceptor: true,  // But DO add interceptor
		},
		{
			name:              "existing base tag with path - should still inject interceptor",
			html:              `<html><head><base href="/sonarr/"><title>Sonarr</title></head><body></body></html>`,
			pathPrefix:        "/apps/sonarr/",
			expectBase:        false,
			expectInterceptor: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := injectBaseTag(tc.html, tc.pathPrefix, tc.pathPrefix)

			// Check for interceptor (should always be present)
			hasInterceptor := strings.Contains(result, "window.fetch") &&
				strings.Contains(result, "XMLHttpRequest.prototype.open")
			if tc.expectInterceptor && !hasInterceptor {
				t.Errorf("Expected JS interceptor to be injected but it wasn't.\nResult: %s", result)
			}

			// Check for new base tag
			if tc.expectBase {
				expectedBase := `<base href="` + tc.pathPrefix + `">`
				if !strings.Contains(result, expectedBase) {
					t.Errorf("Expected base tag %q but not found.\nResult: %s", expectedBase, result)
				}
			}

			// Verify no duplicate base tags when one already exists
			if !tc.expectBase {
				baseCount := strings.Count(strings.ToLower(result), "<base")
				if baseCount > 1 {
					t.Errorf("Should not have multiple base tags, found %d", baseCount)
				}
			}
		})
	}
}

// TestAPIPathExclusion tests that only Nexus API paths are excluded, not app API paths
func TestAPIPathExclusion(t *testing.T) {
	testCases := []struct {
		name       string
		html       string
		pathPrefix string
		expected   string
	}{
		{
			name:       "nexus api path should NOT be rewritten",
			html:       `<script src="/api/v1/config"></script>`,
			pathPrefix: "/apps/test/",
			expected:   `<script src="/api/v1/config"></script>`,
		},
		{
			name:       "app api path SHOULD be rewritten - v3",
			html:       `<script src="/api/v3/system/status"></script>`,
			pathPrefix: "/apps/sonarr/",
			expected:   `<script src="/apps/sonarr/api/v3/system/status"></script>`,
		},
		{
			name:       "app api path SHOULD be rewritten - no version",
			html:       `<a href="/api/config">Config</a>`,
			pathPrefix: "/apps/test/",
			expected:   `<a href="/apps/test/api/config">Config</a>`,
		},
		{
			name:       "healthz should NOT be rewritten",
			html:       `<script src="/healthz"></script>`,
			pathPrefix: "/apps/test/",
			expected:   `<script src="/healthz"></script>`,
		},
		{
			name:       "app healthz endpoint SHOULD be rewritten",
			html:       `<script src="/health"></script>`,
			pathPrefix: "/apps/test/",
			expected:   `<script src="/apps/test/health"></script>`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := rewriteHTMLPaths(tc.html, tc.pathPrefix, tc.pathPrefix)
			if result != tc.expected {
				t.Errorf("API path exclusion failed:\nInput:    %s\nExpected: %s\nGot:      %s", tc.html, tc.expected, result)
			}
		})
	}
}

// TestFirstAttributeRewriting tests that first attribute in tag (no leading space) is rewritten
func TestFirstAttributeRewriting(t *testing.T) {
	testCases := []struct {
		name       string
		html       string
		pathPrefix string
		expected   string
	}{
		{
			name:       "src as first attribute",
			html:       `<script src="/app.js"></script>`,
			pathPrefix: "/apps/test/",
			expected:   `<script src="/apps/test/app.js"></script>`,
		},
		{
			name:       "href as first attribute",
			html:       `<link href="/style.css" rel="stylesheet">`,
			pathPrefix: "/apps/test/",
			expected:   `<link href="/apps/test/style.css" rel="stylesheet">`,
		},
		{
			name:       "src after other attribute",
			html:       `<script type="module" src="/app.js"></script>`,
			pathPrefix: "/apps/test/",
			expected:   `<script type="module" src="/apps/test/app.js"></script>`,
		},
		{
			name:       "action as first attribute",
			html:       `<form action="/submit" method="post"></form>`,
			pathPrefix: "/apps/test/",
			expected:   `<form action="/apps/test/submit" method="post"></form>`,
		},
		{
			name:       "img src first",
			html:       `<img src="/logo.png" alt="Logo">`,
			pathPrefix: "/apps/test/",
			expected:   `<img src="/apps/test/logo.png" alt="Logo">`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := rewriteHTMLPaths(tc.html, tc.pathPrefix, tc.pathPrefix)
			if result != tc.expected {
				t.Errorf("First attribute rewriting failed:\nInput:    %s\nExpected: %s\nGot:      %s", tc.html, tc.expected, result)
			}
		})
	}
}

// TestDoubleRewritingPrevention tests that paths aren't rewritten twice
func TestDoubleRewritingPrevention(t *testing.T) {
	testCases := []struct {
		name       string
		html       string
		pathPrefix string
		expected   string
	}{
		{
			name:       "already prefixed path",
			html:       `<script src="/apps/test/app.js"></script>`,
			pathPrefix: "/apps/test/",
			expected:   `<script src="/apps/test/app.js"></script>`,
		},
		{
			name:       "similar prefix should not match",
			html:       `<script src="/apps/test-other/app.js"></script>`,
			pathPrefix: "/apps/test/",
			expected:   `<script src="/apps/test/apps/test-other/app.js"></script>`,
		},
		{
			name:       "exact prefix match",
			html:       `<a href="/apps/myapp/">Home</a>`,
			pathPrefix: "/apps/myapp/",
			expected:   `<a href="/apps/myapp/">Home</a>`,
		},
		{
			name:       "prefix with subpath",
			html:       `<a href="/apps/myapp/dashboard">Dashboard</a>`,
			pathPrefix: "/apps/myapp/",
			expected:   `<a href="/apps/myapp/dashboard">Dashboard</a>`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := rewriteHTMLPaths(tc.html, tc.pathPrefix, tc.pathPrefix)
			if result != tc.expected {
				t.Errorf("Double rewriting prevention failed:\nInput:    %s\nExpected: %s\nGot:      %s", tc.html, tc.expected, result)
			}
		})
	}
}

// TestFragmentAndQueryStringHandling tests that fragments and query strings are preserved
func TestFragmentAndQueryStringHandling(t *testing.T) {
	testCases := []struct {
		name       string
		html       string
		pathPrefix string
		expected   string
	}{
		{
			name:       "fragment only",
			html:       `<a href="/page#section">Link</a>`,
			pathPrefix: "/apps/test/",
			expected:   `<a href="/apps/test/page#section">Link</a>`,
		},
		{
			name:       "query string only",
			html:       `<script src="/app.js?v=123"></script>`,
			pathPrefix: "/apps/test/",
			expected:   `<script src="/apps/test/app.js?v=123"></script>`,
		},
		{
			name:       "query string and fragment",
			html:       `<a href="/page?tab=settings#advanced">Settings</a>`,
			pathPrefix: "/apps/test/",
			expected:   `<a href="/apps/test/page?tab=settings#advanced">Settings</a>`,
		},
		{
			name:       "complex query string",
			html:       `<img src="/image?width=100&height=200&format=webp">`,
			pathPrefix: "/apps/test/",
			expected:   `<img src="/apps/test/image?width=100&height=200&format=webp">`,
		},
		{
			name:       "fragment with special chars",
			html:       `<a href="/docs#section-1.2">Docs</a>`,
			pathPrefix: "/apps/test/",
			expected:   `<a href="/apps/test/docs#section-1.2">Docs</a>`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := rewriteHTMLPaths(tc.html, tc.pathPrefix, tc.pathPrefix)
			if result != tc.expected {
				t.Errorf("Fragment/query handling failed:\nInput:    %s\nExpected: %s\nGot:      %s", tc.html, tc.expected, result)
			}
		})
	}
}

// TestMetaRefreshRewriting tests that meta refresh URLs are rewritten
func TestMetaRefreshRewriting(t *testing.T) {
	testCases := []struct {
		name       string
		html       string
		pathPrefix string
		expected   string
	}{
		{
			name:       "meta refresh with url",
			html:       `<meta http-equiv="refresh" content="0;url=/dashboard">`,
			pathPrefix: "/apps/test/",
			expected:   `<meta http-equiv="refresh" content="0;url=/apps/test/dashboard">`,
		},
		{
			name:       "meta refresh with URL uppercase",
			html:       `<meta http-equiv="refresh" content="5;URL=/login">`,
			pathPrefix: "/apps/test/",
			expected:   `<meta http-equiv="refresh" content="5;URL=/apps/test/login">`,
		},
		{
			name:       "meta refresh external url unchanged",
			html:       `<meta http-equiv="refresh" content="0;url=https://example.com">`,
			pathPrefix: "/apps/test/",
			expected:   `<meta http-equiv="refresh" content="0;url=https://example.com">`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := rewriteHTMLPaths(tc.html, tc.pathPrefix, tc.pathPrefix)
			if result != tc.expected {
				t.Errorf("Meta refresh rewriting failed:\nInput:    %s\nExpected: %s\nGot:      %s", tc.html, tc.expected, result)
			}
		})
	}
}

// TestSVGXlinkHrefRewriting tests that SVG xlink:href attributes are rewritten
func TestSVGXlinkHrefRewriting(t *testing.T) {
	testCases := []struct {
		name       string
		html       string
		pathPrefix string
		expected   string
	}{
		{
			name:       "svg use xlink:href",
			html:       `<svg><use xlink:href="/icons.svg#icon-home"></use></svg>`,
			pathPrefix: "/apps/test/",
			expected:   `<svg><use xlink:href="/apps/test/icons.svg#icon-home"></use></svg>`,
		},
		{
			name:       "svg image xlink:href",
			html:       `<svg><image xlink:href="/images/logo.svg"></image></svg>`,
			pathPrefix: "/apps/test/",
			expected:   `<svg><image xlink:href="/apps/test/images/logo.svg"></image></svg>`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := rewriteHTMLPaths(tc.html, tc.pathPrefix, tc.pathPrefix)
			if result != tc.expected {
				t.Errorf("SVG xlink:href rewriting failed:\nInput:    %s\nExpected: %s\nGot:      %s", tc.html, tc.expected, result)
			}
		})
	}
}

// TestSrcsetRewriting tests that srcset attributes are rewritten
func TestSrcsetRewriting(t *testing.T) {
	testCases := []struct {
		name       string
		html       string
		pathPrefix string
		expected   string
	}{
		{
			name:       "simple srcset",
			html:       `<img srcset="/image-1x.png 1x, /image-2x.png 2x">`,
			pathPrefix: "/apps/test/",
			expected:   `<img srcset="/apps/test/image-1x.png 1x, /apps/test/image-2x.png 2x">`,
		},
		{
			name:       "srcset with widths",
			html:       `<img srcset="/small.jpg 320w, /medium.jpg 640w, /large.jpg 1024w">`,
			pathPrefix: "/apps/test/",
			expected:   `<img srcset="/apps/test/small.jpg 320w, /apps/test/medium.jpg 640w, /apps/test/large.jpg 1024w">`,
		},
		{
			name:       "picture source srcset",
			html:       `<source srcset="/mobile.webp" media="(max-width: 600px)">`,
			pathPrefix: "/apps/test/",
			expected:   `<source srcset="/apps/test/mobile.webp" media="(max-width: 600px)">`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := rewriteHTMLPaths(tc.html, tc.pathPrefix, tc.pathPrefix)
			if result != tc.expected {
				t.Errorf("Srcset rewriting failed:\nInput:    %s\nExpected: %s\nGot:      %s", tc.html, tc.expected, result)
			}
		})
	}
}

// TestDataAttributeRewriting tests that data attribute on object/embed is rewritten
func TestDataAttributeRewriting(t *testing.T) {
	testCases := []struct {
		name       string
		html       string
		pathPrefix string
		expected   string
	}{
		{
			name:       "object data attribute",
			html:       `<object data="/video.mp4" type="video/mp4"></object>`,
			pathPrefix: "/apps/test/",
			expected:   `<object data="/apps/test/video.mp4" type="video/mp4"></object>`,
		},
		{
			name:       "embed src attribute",
			html:       `<embed src="/audio.mp3" type="audio/mpeg">`,
			pathPrefix: "/apps/test/",
			expected:   `<embed src="/apps/test/audio.mp3" type="audio/mpeg">`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := rewriteHTMLPaths(tc.html, tc.pathPrefix, tc.pathPrefix)
			if result != tc.expected {
				t.Errorf("Data attribute rewriting failed:\nInput:    %s\nExpected: %s\nGot:      %s", tc.html, tc.expected, result)
			}
		})
	}
}

// TestJSInterceptorContainsRequiredInterceptions tests that JS interceptor has all needed interceptions
func TestJSInterceptorContainsRequiredInterceptions(t *testing.T) {
	interceptor := generateFetchInterceptor("/apps/test/")

	requiredInterceptions := []struct {
		name    string
		pattern string
	}{
		{"fetch interception", "window.fetch"},
		{"XHR interception", "XMLHttpRequest.prototype.open"},
		{"EventSource interception", "EventSource"},
		{"WebSocket interception", "WebSocket"},
		{"setAttribute interception", "Element.prototype.setAttribute"},
		{"URL constructor interception", "window.URL"},
		{"history.pushState interception", "history.pushState"},
		{"history.replaceState interception", "history.replaceState"},
		{"location.assign interception", "location.assign"},
		{"location.replace interception", "location.replace"},
	}

	for _, req := range requiredInterceptions {
		t.Run(req.name, func(t *testing.T) {
			if !strings.Contains(interceptor, req.pattern) {
				t.Errorf("JS interceptor missing %s (pattern: %s)", req.name, req.pattern)
			}
		})
	}
}

// TestPreloadPrefetchRewriting tests that preload/prefetch links are rewritten
func TestPreloadPrefetchRewriting(t *testing.T) {
	testCases := []struct {
		name       string
		html       string
		pathPrefix string
		expected   string
	}{
		{
			name:       "preload font",
			html:       `<link rel="preload" href="/fonts/custom.woff2" as="font">`,
			pathPrefix: "/apps/test/",
			expected:   `<link rel="preload" href="/apps/test/fonts/custom.woff2" as="font">`,
		},
		{
			name:       "prefetch page",
			html:       `<link rel="prefetch" href="/page2.html">`,
			pathPrefix: "/apps/test/",
			expected:   `<link rel="prefetch" href="/apps/test/page2.html">`,
		},
		{
			name:       "modulepreload",
			html:       `<link rel="modulepreload" href="/modules/app.js">`,
			pathPrefix: "/apps/test/",
			expected:   `<link rel="modulepreload" href="/apps/test/modules/app.js">`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := rewriteHTMLPaths(tc.html, tc.pathPrefix, tc.pathPrefix)
			if result != tc.expected {
				t.Errorf("Preload/prefetch rewriting failed:\nInput:    %s\nExpected: %s\nGot:      %s", tc.html, tc.expected, result)
			}
		})
	}
}

// TestRefreshHeaderRewriting tests Refresh header rewriting
func TestRefreshHeaderRewriting(t *testing.T) {
	testCases := []struct {
		name            string
		pathPrefix      string
		requestHost     string
		refreshHeader   string
		expectedRefresh string
	}{
		{
			name:            "rewrite relative URL in refresh",
			pathPrefix:      "/apps/jackett/",
			requestHost:     "192.168.0.23:8088",
			refreshHeader:   "5; url=/dashboard",
			expectedRefresh: "5; url=/apps/jackett/dashboard",
		},
		{
			name:            "rewrite with zero delay",
			pathPrefix:      "/apps/sonarr/",
			requestHost:     "nexus.local",
			refreshHeader:   "0; url=/login",
			expectedRefresh: "0; url=/apps/sonarr/login",
		},
		{
			name:            "rewrite absolute URL same host",
			pathPrefix:      "/apps/radarr/",
			requestHost:     "192.168.1.100:8088",
			refreshHeader:   "3; url=http://192.168.1.100:8088/settings",
			expectedRefresh: "3; url=http://192.168.1.100:8088/apps/radarr/settings",
		},
		{
			name:            "skip external URL",
			pathPrefix:      "/apps/test/",
			requestHost:     "nexus.local",
			refreshHeader:   "5; url=https://example.com/page",
			expectedRefresh: "5; url=https://example.com/page",
		},
		{
			name:            "skip already prefixed",
			pathPrefix:      "/apps/test/",
			requestHost:     "nexus.local",
			refreshHeader:   "5; url=/apps/test/dashboard",
			expectedRefresh: "5; url=/apps/test/dashboard",
		},
		{
			name:            "handle URL= uppercase",
			pathPrefix:      "/apps/test/",
			requestHost:     "nexus.local",
			refreshHeader:   "10; URL=/admin",
			expectedRefresh: "10; URL=/apps/test/admin",
		},
		{
			name:            "no URL component - just delay",
			pathPrefix:      "/apps/test/",
			requestHost:     "nexus.local",
			refreshHeader:   "30",
			expectedRefresh: "30",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			rw := NewHTMLRewritingResponseWriterWithHost(w, tc.pathPrefix, tc.pathPrefix, tc.requestHost, "http")

			rw.Header().Set("Refresh", tc.refreshHeader)
			rw.Header().Set("Content-Type", "text/html")
			rw.WriteHeader(http.StatusOK)

			result := w.Header().Get("Refresh")
			if result != tc.expectedRefresh {
				t.Errorf("Refresh header = %q, want %q", result, tc.expectedRefresh)
			}
		})
	}
}

// TestContentLocationHeaderRewriting tests Content-Location header rewriting
func TestContentLocationHeaderRewriting(t *testing.T) {
	testCases := []struct {
		name        string
		pathPrefix  string
		requestHost string
		header      string
		expected    string
	}{
		{
			name:        "rewrite relative path",
			pathPrefix:  "/apps/api/",
			requestHost: "nexus.local",
			header:      "/resource/123",
			expected:    "/apps/api/resource/123",
		},
		{
			name:        "rewrite absolute URL same host",
			pathPrefix:  "/apps/api/",
			requestHost: "192.168.0.23:8088",
			header:      "http://192.168.0.23:8088/data/items",
			expected:    "http://192.168.0.23:8088/apps/api/data/items",
		},
		{
			name:        "skip external URL",
			pathPrefix:  "/apps/api/",
			requestHost: "nexus.local",
			header:      "https://cdn.example.com/resource",
			expected:    "https://cdn.example.com/resource",
		},
		{
			name:        "skip already prefixed",
			pathPrefix:  "/apps/api/",
			requestHost: "nexus.local",
			header:      "/apps/api/resource",
			expected:    "/apps/api/resource",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			rw := NewHTMLRewritingResponseWriterWithHost(w, tc.pathPrefix, tc.pathPrefix, tc.requestHost, "http")

			rw.Header().Set("Content-Location", tc.header)
			rw.Header().Set("Content-Type", "application/json")
			rw.WriteHeader(http.StatusOK)

			result := w.Header().Get("Content-Location")
			if result != tc.expected {
				t.Errorf("Content-Location header = %q, want %q", result, tc.expected)
			}
		})
	}
}

// TestLinkHeaderRewriting tests RFC 8288 Link header rewriting
func TestLinkHeaderRewriting(t *testing.T) {
	testCases := []struct {
		name        string
		pathPrefix  string
		requestHost string
		linkHeader  string
		expected    string
	}{
		{
			name:        "rewrite single link",
			pathPrefix:  "/apps/api/",
			requestHost: "nexus.local",
			linkHeader:  "</users?page=2>; rel=\"next\"",
			expected:    "</apps/api/users?page=2>; rel=\"next\"",
		},
		{
			name:        "rewrite multiple links",
			pathPrefix:  "/apps/api/",
			requestHost: "nexus.local",
			linkHeader:  "</users?page=1>; rel=\"prev\", </users?page=3>; rel=\"next\"",
			expected:    "</apps/api/users?page=1>; rel=\"prev\", </apps/api/users?page=3>; rel=\"next\"",
		},
		{
			name:        "skip external URLs in link",
			pathPrefix:  "/apps/api/",
			requestHost: "nexus.local",
			linkHeader:  "<https://example.com/docs>; rel=\"help\"",
			expected:    "<https://example.com/docs>; rel=\"help\"",
		},
		{
			name:        "rewrite absolute URL same host",
			pathPrefix:  "/apps/api/",
			requestHost: "192.168.0.23:8088",
			linkHeader:  "<http://192.168.0.23:8088/next>; rel=\"next\"",
			expected:    "<http://192.168.0.23:8088/apps/api/next>; rel=\"next\"",
		},
		{
			name:        "preserve link parameters",
			pathPrefix:  "/apps/api/",
			requestHost: "nexus.local",
			linkHeader:  "</resource>; rel=\"preload\"; as=\"script\"; crossorigin",
			expected:    "</apps/api/resource>; rel=\"preload\"; as=\"script\"; crossorigin",
		},
		{
			name:        "skip already prefixed",
			pathPrefix:  "/apps/api/",
			requestHost: "nexus.local",
			linkHeader:  "</apps/api/users>; rel=\"self\"",
			expected:    "</apps/api/users>; rel=\"self\"",
		},
		{
			name:        "mixed internal and external",
			pathPrefix:  "/apps/api/",
			requestHost: "nexus.local",
			linkHeader:  "</next>; rel=\"next\", <https://docs.example.com>; rel=\"help\"",
			expected:    "</apps/api/next>; rel=\"next\", <https://docs.example.com>; rel=\"help\"",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			rw := NewHTMLRewritingResponseWriterWithHost(w, tc.pathPrefix, tc.pathPrefix, tc.requestHost, "http")

			rw.Header().Set("Link", tc.linkHeader)
			rw.Header().Set("Content-Type", "application/json")
			rw.WriteHeader(http.StatusOK)

			result := w.Header().Get("Link")
			if result != tc.expected {
				t.Errorf("Link header = %q, want %q", result, tc.expected)
			}
		})
	}
}

// TestCSPHeaderRewriting tests Content-Security-Policy header rewriting
func TestCSPHeaderRewriting(t *testing.T) {
	testCases := []struct {
		name        string
		pathPrefix  string
		requestHost string
		cspHeader   string
		expected    string
	}{
		{
			name:        "rewrite script-src paths",
			pathPrefix:  "/apps/myapp/",
			requestHost: "nexus.local",
			cspHeader:   "script-src 'self' /js/app.js",
			expected:    "script-src 'self' /apps/myapp/js/app.js",
		},
		{
			name:        "rewrite multiple directives",
			pathPrefix:  "/apps/myapp/",
			requestHost: "nexus.local",
			cspHeader:   "script-src /scripts/*; style-src /css/*; img-src /images/*",
			expected:    "script-src /apps/myapp/scripts/*; style-src /apps/myapp/css/*; img-src /apps/myapp/images/*",
		},
		{
			name:        "preserve special values",
			pathPrefix:  "/apps/myapp/",
			requestHost: "nexus.local",
			cspHeader:   "script-src 'self' 'unsafe-inline' /js/*",
			expected:    "script-src 'self' 'unsafe-inline' /apps/myapp/js/*",
		},
		{
			name:        "rewrite report-uri",
			pathPrefix:  "/apps/myapp/",
			requestHost: "nexus.local",
			cspHeader:   "default-src 'self'; report-uri /csp-report",
			expected:    "default-src 'self'; report-uri /apps/myapp/csp-report",
		},
		{
			name:        "skip external URLs",
			pathPrefix:  "/apps/myapp/",
			requestHost: "nexus.local",
			cspHeader:   "script-src 'self' https://cdn.example.com",
			expected:    "script-src 'self' https://cdn.example.com",
		},
		{
			name:        "skip already prefixed",
			pathPrefix:  "/apps/myapp/",
			requestHost: "nexus.local",
			cspHeader:   "script-src /apps/myapp/js/*",
			expected:    "script-src /apps/myapp/js/*",
		},
		{
			name:        "handle nonce and hash values",
			pathPrefix:  "/apps/myapp/",
			requestHost: "nexus.local",
			cspHeader:   "script-src 'nonce-abc123' 'sha256-xyz' /js/*",
			expected:    "script-src 'nonce-abc123' 'sha256-xyz' /apps/myapp/js/*",
		},
		{
			name:        "complex real-world CSP",
			pathPrefix:  "/apps/grafana/",
			requestHost: "nexus.local",
			cspHeader:   "default-src 'self'; script-src 'self' 'unsafe-eval' /public/build/*; style-src 'self' 'unsafe-inline'; img-src 'self' data: /avatar/*; connect-src 'self' /api/*",
			expected:    "default-src 'self'; script-src 'self' 'unsafe-eval' /apps/grafana/public/build/*; style-src 'self' 'unsafe-inline'; img-src 'self' data: /apps/grafana/avatar/*; connect-src 'self' /apps/grafana/api/*",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			rw := NewHTMLRewritingResponseWriterWithHost(w, tc.pathPrefix, tc.pathPrefix, tc.requestHost, "http")

			rw.Header().Set("Content-Security-Policy", tc.cspHeader)
			rw.Header().Set("Content-Type", "text/html")
			rw.WriteHeader(http.StatusOK)

			result := w.Header().Get("Content-Security-Policy")
			if result != tc.expected {
				t.Errorf("CSP header = %q, want %q", result, tc.expected)
			}
		})
	}
}

// TestCSPReportOnlyHeaderRewriting tests Content-Security-Policy-Report-Only header rewriting
func TestCSPReportOnlyHeaderRewriting(t *testing.T) {
	w := httptest.NewRecorder()
	rw := NewHTMLRewritingResponseWriterWithHost(w, "/apps/test/", "/apps/test/", "nexus.local", "http")

	rw.Header().Set("Content-Security-Policy-Report-Only", "script-src /js/*; report-uri /csp-report")
	rw.Header().Set("Content-Type", "text/html")
	rw.WriteHeader(http.StatusOK)

	result := w.Header().Get("Content-Security-Policy-Report-Only")
	expected := "script-src /apps/test/js/*; report-uri /apps/test/csp-report"
	if result != expected {
		t.Errorf("CSP-Report-Only header = %q, want %q", result, expected)
	}
}

// TestInjectBaseTag_RewriteExisting tests that existing <base> tags are rewritten
func TestInjectBaseTag_RewriteExisting(t *testing.T) {
	testCases := []struct {
		name         string
		html         string
		pathPrefix   string
		requestPath  string
		expectedBase string
	}{
		{
			name:         "rewrite base href=/ to sub-path",
			html:         `<html><head><base href="/"></head><body></body></html>`,
			pathPrefix:   "/apps/test/",
			requestPath:  "/apps/test/",
			expectedBase: `<base href="/apps/test/">`,
		},
		{
			name:         "rewrite base href with app sub-path",
			html:         `<html><head><base href="/sonarr/"></head><body></body></html>`,
			pathPrefix:   "/apps/sonarr/",
			requestPath:  "/apps/sonarr/",
			expectedBase: `<base href="/apps/sonarr/sonarr/">`,
		},
		{
			name:         "leave already-prefixed base href alone",
			html:         `<html><head><base href="/apps/test/"></head><body></body></html>`,
			pathPrefix:   "/apps/test/",
			requestPath:  "/apps/test/",
			expectedBase: `<base href="/apps/test/">`,
		},
		{
			name:         "rewrite empty base href",
			html:         `<html><head><base href=""></head><body></body></html>`,
			pathPrefix:   "/apps/radarr/",
			requestPath:  "/apps/radarr/",
			expectedBase: `<base href="/apps/radarr/">`,
		},
		{
			name:         "rewrite base with double quotes and extra attributes",
			html:         `<html><head><base href="/" target="_blank"></head><body></body></html>`,
			pathPrefix:   "/apps/test/",
			requestPath:  "/apps/test/",
			expectedBase: `<base href="/apps/test/" target="_blank">`,
		},
		{
			name:         "rewrite base with single quotes",
			html:         `<html><head><base href='/'></head><body></body></html>`,
			pathPrefix:   "/apps/test/",
			requestPath:  "/apps/test/",
			expectedBase: `<base href='/apps/test/'>`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := injectBaseTag(tc.html, tc.pathPrefix, tc.requestPath)

			if !strings.Contains(result, tc.expectedBase) {
				t.Errorf("Expected result to contain %q, got: %s", tc.expectedBase, result)
			}

			// Should still inject the JS interceptor
			if !strings.Contains(result, "window.fetch") {
				t.Error("Expected result to contain fetch interceptor")
			}

			// Should NOT have multiple base tags
			baseCount := strings.Count(strings.ToLower(result), "<base")
			if baseCount != 1 {
				t.Errorf("Expected exactly 1 base tag, found %d in: %s", baseCount, result)
			}
		})
	}
}

// TestRewriteCSSContent tests CSS file content rewriting
func TestRewriteCSSContent(t *testing.T) {
	testCases := []struct {
		name       string
		css        string
		pathPrefix string
		expected   string
	}{
		{
			name:       "rewrite url() with absolute path",
			css:        `body { background: url(/images/bg.png); }`,
			pathPrefix: "/apps/test/",
			expected:   `body { background: url(/apps/test/images/bg.png); }`,
		},
		{
			name:       "rewrite url() with quoted path",
			css:        `@font-face { src: url("/fonts/roboto.woff2"); }`,
			pathPrefix: "/apps/test/",
			expected:   `@font-face { src: url("/apps/test/fonts/roboto.woff2"); }`,
		},
		{
			name:       "rewrite url() with single-quoted path",
			css:        `@font-face { src: url('/fonts/roboto.woff2'); }`,
			pathPrefix: "/apps/test/",
			expected:   `@font-face { src: url('/apps/test/fonts/roboto.woff2'); }`,
		},
		{
			name:       "skip external url()",
			css:        `body { background: url(https://example.com/bg.png); }`,
			pathPrefix: "/apps/test/",
			expected:   `body { background: url(https://example.com/bg.png); }`,
		},
		{
			name:       "skip data URI in url()",
			css:        `body { background: url(data:image/png;base64,ABC123); }`,
			pathPrefix: "/apps/test/",
			expected:   `body { background: url(data:image/png;base64,ABC123); }`,
		},
		{
			name:       "skip relative url()",
			css:        `body { background: url(images/bg.png); }`,
			pathPrefix: "/apps/test/",
			expected:   `body { background: url(images/bg.png); }`,
		},
		{
			name:       "skip already prefixed url()",
			css:        `body { background: url(/apps/test/images/bg.png); }`,
			pathPrefix: "/apps/test/",
			expected:   `body { background: url(/apps/test/images/bg.png); }`,
		},
		{
			name:       "rewrite @import with double quotes",
			css:        `@import "/other.css";`,
			pathPrefix: "/apps/test/",
			expected:   `@import "/apps/test/other.css";`,
		},
		{
			name:       "rewrite @import with single quotes",
			css:        `@import '/reset.css';`,
			pathPrefix: "/apps/test/",
			expected:   `@import '/apps/test/reset.css';`,
		},
		{
			name:       "skip @import with external URL",
			css:        `@import "https://fonts.googleapis.com/css";`,
			pathPrefix: "/apps/test/",
			expected:   `@import "https://fonts.googleapis.com/css";`,
		},
		{
			name:       "skip @import with relative path",
			css:        `@import "other.css";`,
			pathPrefix: "/apps/test/",
			expected:   `@import "other.css";`,
		},
		{
			name:       "rewrite multiple url() in one file",
			css:        `.a { background: url(/img/a.png); } .b { background: url(/img/b.png); }`,
			pathPrefix: "/apps/test/",
			expected:   `.a { background: url(/apps/test/img/a.png); } .b { background: url(/apps/test/img/b.png); }`,
		},
		{
			name:       "prefix without trailing slash",
			css:        `body { background: url(/images/bg.png); }`,
			pathPrefix: "/apps/test",
			expected:   `body { background: url(/apps/test/images/bg.png); }`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := rewriteCSSContent(tc.css, tc.pathPrefix)
			if result != tc.expected {
				t.Errorf("rewriteCSSContent(%q, %q) = %q, want %q", tc.css, tc.pathPrefix, result, tc.expected)
			}
		})
	}
}

// TestCSSResponseRewriting tests that CSS responses are buffered and rewritten end-to-end
func TestCSSResponseRewriting(t *testing.T) {
	w := httptest.NewRecorder()
	rw := NewHTMLRewritingResponseWriter(w, "/apps/test/", "/apps/test/")

	// Set CSS content type
	rw.Header().Set("Content-Type", "text/css")
	rw.WriteHeader(http.StatusOK)

	// Verify isCSS was set
	if !rw.isCSS {
		t.Fatal("Expected isCSS to be true for text/css content type")
	}

	// Write CSS content
	css := `body { background: url(/images/bg.png); } @import "/reset.css";`
	_, err := rw.Write([]byte(css))
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// Flush to send response
	err = rw.FlushHTML()
	if err != nil {
		t.Fatalf("FlushHTML failed: %v", err)
	}

	result := w.Body.String()
	if !strings.Contains(result, `url(/apps/test/images/bg.png)`) {
		t.Errorf("Expected url() to be rewritten, got: %s", result)
	}
	if !strings.Contains(result, `"/apps/test/reset.css"`) {
		t.Errorf("Expected @import to be rewritten, got: %s", result)
	}
}

// TestCSSResponseNotRewrittenWhenBodyRewriteDisabled tests header-only mode skips CSS
func TestCSSResponseNotRewrittenWhenBodyRewriteDisabled(t *testing.T) {
	w := httptest.NewRecorder()
	rw := NewHeaderRewritingResponseWriter(w, "/apps/test/", "nexus.local", "http")

	rw.Header().Set("Content-Type", "text/css")
	rw.WriteHeader(http.StatusOK)

	// When rewriteBody is false, CSS should pass through unchanged
	if rw.isCSS {
		t.Error("Expected isCSS to be false when rewriteBody is disabled")
	}

	css := `body { background: url(/images/bg.png); }`
	_, err := rw.Write([]byte(css))
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	result := w.Body.String()
	if result != css {
		t.Errorf("Expected CSS to pass through unchanged, got: %s", result)
	}
}
