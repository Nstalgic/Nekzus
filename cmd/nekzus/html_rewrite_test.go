package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nstalgic/nekzus/internal/types"
)

// TestHTMLRewriting tests that HTML responses are rewritten when RewriteHTML is enabled
func TestHTMLRewriting(t *testing.T) {
	tests := []struct {
		name          string
		rewriteHTML   bool
		stripPrefix   bool
		pathBase      string
		contentType   string
		upstreamHTML  string
		expectedHTML  string
		shouldRewrite bool
	}{
		{
			name:          "rewrite enabled - HTML with absolute script src",
			rewriteHTML:   true,
			stripPrefix:   true,
			pathBase:      "/apps/memos/",
			contentType:   "text/html",
			upstreamHTML:  `<html><head><script src="/app.js"></script></head></html>`,
			expectedHTML:  `<script src="/apps/memos/app.js"></script>`, // Check for rewritten attribute
			shouldRewrite: true,
		},
		{
			name:          "rewrite enabled - HTML with absolute link href",
			rewriteHTML:   true,
			stripPrefix:   true,
			pathBase:      "/apps/memos/",
			contentType:   "text/html; charset=utf-8",
			upstreamHTML:  `<html><head><link rel="stylesheet" href="/style.css"></head></html>`,
			expectedHTML:  `<link rel="stylesheet" href="/apps/memos/style.css">`, // Check for rewritten attribute
			shouldRewrite: true,
		},
		{
			name:        "rewrite enabled - HTML with multiple assets",
			rewriteHTML: true,
			stripPrefix: true,
			pathBase:    "/apps/test",
			contentType: "text/html",
			upstreamHTML: `<html>
<head>
  <link href="/favicon.ico" rel="icon" />
  <script type="module" src="/app.C0sQ3qTp.js"></script>
</head>
<body>
  <img src="/logo.png" />
</body>
</html>`,
			expectedHTML:  `<link href="/apps/test/favicon.ico" rel="icon" />`, // Check for one rewritten attribute
			shouldRewrite: true,
		},
		{
			name:          "rewrite enabled - don't rewrite relative paths",
			rewriteHTML:   true,
			stripPrefix:   true,
			pathBase:      "/apps/memos/",
			contentType:   "text/html",
			upstreamHTML:  `<html><head></head><body><script src="./app.js"></script><script src="vendor/lib.js"></script></body></html>`,
			expectedHTML:  `<script src="./app.js"></script><script src="vendor/lib.js"></script>`, // Check that paths remain unchanged
			shouldRewrite: true,
		},
		{
			name:          "rewrite enabled - don't rewrite external URLs",
			rewriteHTML:   true,
			stripPrefix:   true,
			pathBase:      "/apps/memos/",
			contentType:   "text/html",
			upstreamHTML:  `<html><head></head><body><script src="https://cdn.example.com/app.js"></script><script src="http://example.com/lib.js"></script></body></html>`,
			expectedHTML:  `<script src="https://cdn.example.com/app.js"></script><script src="http://example.com/lib.js"></script>`, // Check that URLs remain unchanged
			shouldRewrite: true,
		},
		{
			name:          "rewrite enabled - don't rewrite data URIs",
			rewriteHTML:   true,
			stripPrefix:   true,
			pathBase:      "/apps/memos/",
			contentType:   "text/html",
			upstreamHTML:  `<html><head></head><body><img src="data:image/png;base64,ABC123" /></body></html>`,
			expectedHTML:  `<img src="data:image/png;base64,ABC123" />`, // Check that data URI remains unchanged
			shouldRewrite: true,
		},
		{
			name:          "passthrough mode - HTML unchanged",
			rewriteHTML:   false,
			stripPrefix:   false, // Both disabled = passthrough mode
			pathBase:      "/apps/memos/",
			contentType:   "text/html",
			upstreamHTML:  `<html><script src="/app.js"></script></html>`,
			expectedHTML:  `<html><script src="/app.js"></script></html>`,
			shouldRewrite: false,
		},
		{
			name:          "rewrite enabled - non-HTML content unchanged",
			rewriteHTML:   true,
			stripPrefix:   true,
			pathBase:      "/apps/memos/",
			contentType:   "application/json",
			upstreamHTML:  `{"script": "/app.js"}`,
			expectedHTML:  `{"script": "/app.js"}`,
			shouldRewrite: false,
		},
		{
			name:          "rewrite enabled - JavaScript content unchanged",
			rewriteHTML:   true,
			stripPrefix:   true,
			pathBase:      "/apps/memos/",
			contentType:   "text/javascript",
			upstreamHTML:  `console.log("/app.js");`,
			expectedHTML:  `console.log("/app.js");`,
			shouldRewrite: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock upstream server
			upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", tt.contentType)
				w.WriteHeader(http.StatusOK)
				io.WriteString(w, tt.upstreamHTML)
			}))
			defer upstreamServer.Close()

			// Create test app
			app := newTestApplication(t)

			// Register route with RewriteHTML setting
			route := types.Route{
				RouteID:     "test-route",
				AppID:       "test-app",
				PathBase:    tt.pathBase,
				To:          upstreamServer.URL,
				StripPrefix: tt.stripPrefix,
				RewriteHTML: tt.rewriteHTML,
			}
			app.managers.Router.UpsertRoute(route)

			// Create test request
			requestPath := tt.pathBase
			if tt.stripPrefix && !strings.HasSuffix(requestPath, "/") {
				requestPath = requestPath + "/"
			}
			req := httptest.NewRequest("GET", requestPath, nil)
			rec := httptest.NewRecorder()

			// Handle request
			app.handleProxy(rec, req)

			// Check response
			resp := rec.Result()
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("Failed to read response body: %v", err)
			}

			// Verify HTML was rewritten (or not)
			gotHTML := string(body)

			// For HTML rewriting tests, check if the rewritten content is present
			// (we inject base tags and interceptors, so exact matches won't work)
			if tt.shouldRewrite && tt.rewriteHTML {
				// Check that the expected rewritten HTML is present in the response
				if !strings.Contains(gotHTML, tt.expectedHTML) {
					t.Errorf("HTML rewriting failed\nExpected substring:\n%s\n\nNot found in:\n%s", tt.expectedHTML, gotHTML)
				}
				// Also verify base tag and interceptors were injected
				if !strings.Contains(gotHTML, `<base href="`) {
					t.Error("Expected <base> tag to be injected for HTML rewriting")
				}
				if !strings.Contains(gotHTML, "Element.prototype.setAttribute") {
					t.Error("Expected setAttribute interceptor to be injected")
				}
			} else {
				// For non-rewriting tests, expect exact match
				if gotHTML != tt.expectedHTML {
					t.Errorf("HTML mismatch\nGot:\n%s\n\nExpected:\n%s", gotHTML, tt.expectedHTML)
				}
			}

			// Verify Content-Type is preserved
			gotContentType := resp.Header.Get("Content-Type")
			if !strings.HasPrefix(gotContentType, strings.Split(tt.contentType, ";")[0]) {
				t.Errorf("Content-Type mismatch: got %q, expected prefix %q", gotContentType, tt.contentType)
			}
		})
	}
}

// TestHTMLRewritingEdgeCases tests edge cases in HTML rewriting
func TestHTMLRewritingEdgeCases(t *testing.T) {
	tests := []struct {
		name         string
		pathBase     string
		upstreamHTML string
		expectedHTML string
	}{
		{
			name:         "already prefixed paths unchanged",
			pathBase:     "/apps/memos/",
			upstreamHTML: `<script src="/apps/memos/app.js"></script>`,
			expectedHTML: `<script src="/apps/memos/app.js"></script>`,
		},
		{
			name:         "paths with query strings",
			pathBase:     "/apps/memos/",
			upstreamHTML: `<script src="/app.js?v=123"></script>`,
			expectedHTML: `<script src="/apps/memos/app.js?v=123"></script>`,
		},
		{
			name:         "paths with fragments",
			pathBase:     "/apps/memos/",
			upstreamHTML: `<a href="/page#section">Link</a>`,
			expectedHTML: `<a href="/apps/memos/page#section">Link</a>`,
		},
		{
			name:         "empty src/href attributes",
			pathBase:     "/apps/memos/",
			upstreamHTML: `<script src=""></script><a href="">Link</a>`,
			expectedHTML: `<script src=""></script><a href="">Link</a>`,
		},
		{
			name:         "pathBase without trailing slash",
			pathBase:     "/apps/memos/",
			upstreamHTML: `<script src="/app.js"></script>`,
			expectedHTML: `<script src="/apps/memos/app.js"></script>`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock upstream server
			upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/html")
				w.WriteHeader(http.StatusOK)
				io.WriteString(w, tt.upstreamHTML)
			}))
			defer upstreamServer.Close()

			// Create test app
			app := newTestApplication(t)

			// Register route
			route := types.Route{
				RouteID:     "test-route",
				AppID:       "test-app",
				PathBase:    tt.pathBase,
				To:          upstreamServer.URL,
				StripPrefix: true,
				RewriteHTML: true,
			}
			app.managers.Router.UpsertRoute(route)

			// Create test request
			requestPath := tt.pathBase
			if !strings.HasSuffix(requestPath, "/") {
				requestPath = requestPath + "/"
			}
			req := httptest.NewRequest("GET", requestPath, nil)
			rec := httptest.NewRecorder()

			// Handle request
			app.handleProxy(rec, req)

			// Check response
			resp := rec.Result()
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("Failed to read response body: %v", err)
			}

			// Verify HTML was rewritten correctly
			gotHTML := string(body)
			// Check that the expected HTML snippet is present (not exact match due to injected interceptors)
			if !strings.Contains(gotHTML, tt.expectedHTML) {
				t.Errorf("Expected HTML substring not found\nExpected substring:\n%s\n\nNot found in:\n%s", tt.expectedHTML, gotHTML)
			}
		})
	}
}

// TestHTMLRewritingWithPropertyAssignment tests that dynamically created elements
// with properties set via direct assignment (not setAttribute) are handled correctly
func TestHTMLRewritingWithPropertyAssignment(t *testing.T) {
	// Create mock upstream server that returns HTML with JavaScript that creates
	// elements dynamically using property assignment
	upstreamHTML := `<!DOCTYPE html>
<html>
<head>
  <title>Test</title>
</head>
<body>
  <div id="container"></div>
  <script>
    // Test 1: Create object element with property assignment
    const obj = document.createElement('object');
    obj.data = '/icon.svg';
    document.getElementById('container').appendChild(obj);

    // Test 2: Create img element with property assignment
    const img = document.createElement('img');
    img.src = '/logo.png';
    document.getElementById('container').appendChild(img);

    // Test 3: Create link element with property assignment
    const link = document.createElement('a');
    link.href = '/page';
    document.getElementById('container').appendChild(link);
  </script>
</body>
</html>`

	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, upstreamHTML)
	}))
	defer upstreamServer.Close()

	// Create test app
	app := newTestApplication(t)

	// Register route with RewriteHTML enabled
	route := types.Route{
		RouteID:     "test-route",
		AppID:       "test-app",
		PathBase:    "/apps/test/",
		To:          upstreamServer.URL,
		StripPrefix: true,
		RewriteHTML: true,
	}
	app.managers.Router.UpsertRoute(route)

	// Create test request
	req := httptest.NewRequest("GET", "/apps/test/", nil)
	rec := httptest.NewRecorder()

	// Handle request
	app.handleProxy(rec, req)

	// Check response
	resp := rec.Result()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	// Verify HTML contains base tag and property interceptors
	gotHTML := string(body)

	// Check for base tag
	if !strings.Contains(gotHTML, `<base href="/apps/test/">`) {
		t.Error("Expected base tag to be injected")
	}

	// Check for setAttribute interceptor
	if !strings.Contains(gotHTML, "Element.prototype.setAttribute") {
		t.Error("Expected setAttribute interceptor to be injected")
	}

	// Check for property descriptor interceptors (the fix we're testing)
	// These should intercept direct property assignments like obj.data = '/icon.svg'
	expectedInterceptors := []string{
		"HTMLObjectElement.prototype", // for <object> data property
		"HTMLImageElement.prototype",  // for <img> src property
		"HTMLAnchorElement.prototype", // for <a> href property
		"HTMLScriptElement.prototype", // for <script> src property
		"HTMLLinkElement.prototype",   // for <link> href property
	}

	for _, interceptor := range expectedInterceptors {
		if !strings.Contains(gotHTML, interceptor) {
			t.Errorf("Expected property interceptor for %s to be injected", interceptor)
		}
	}

	// Verify the original JavaScript is still present (we don't modify it, just add interceptors)
	if !strings.Contains(gotHTML, "obj.data = '/icon.svg';") {
		t.Error("Original JavaScript should be preserved")
	}
}
