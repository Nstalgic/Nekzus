package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nstalgic/nekzus/internal/types"
)

// TestProxyResponseCookieHandling tests that response cookies are handled according to route configuration
func TestProxyResponseCookieHandling(t *testing.T) {
	tests := []struct {
		name                 string
		stripResponseCookies bool
		upstreamCookies      []string // Cookies that upstream sets
		expectCookies        []string // Cookies we expect client to receive
	}{
		{
			name:                 "strip response cookies enabled - single cookie removed",
			stripResponseCookies: true,
			upstreamCookies:      []string{"session=abc123; Path=/; HttpOnly"},
			expectCookies:        []string{}, // Expect no cookies
		},
		{
			name:                 "strip response cookies enabled - multiple cookies removed",
			stripResponseCookies: true,
			upstreamCookies: []string{
				"session=abc123; Path=/; HttpOnly",
				"tracking=xyz789; Domain=.example.com",
				"preferences=dark_mode; Max-Age=31536000",
			},
			expectCookies: []string{}, // Expect no cookies
		},
		{
			name:                 "strip response cookies disabled - cookies preserved",
			stripResponseCookies: false,
			upstreamCookies: []string{
				"session=abc123; Path=/; HttpOnly",
				"tracking=xyz789; Domain=.example.com",
			},
			expectCookies: []string{
				"session=abc123; Path=/; HttpOnly",
				"tracking=xyz789; Domain=.example.com",
			},
		},
		{
			name:                 "strip response cookies disabled - no cookies from upstream",
			stripResponseCookies: false,
			upstreamCookies:      []string{},
			expectCookies:        []string{},
		},
		{
			name:                 "strip response cookies enabled - no cookies from upstream",
			stripResponseCookies: true,
			upstreamCookies:      []string{},
			expectCookies:        []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create upstream test server that sets cookies
			upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Set all upstream cookies
				for _, cookie := range tt.upstreamCookies {
					w.Header().Add("Set-Cookie", cookie)
				}
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprintf(w, `{"status":"ok"}`)
			}))
			defer upstreamServer.Close()

			// Create test application with route
			app := newTestApplication(t)
			route := types.Route{
				RouteID:              "test-route",
				AppID:                "test-app",
				PathBase:             "/test/",
				To:                   upstreamServer.URL,
				StripPrefix:          true,
				StripResponseCookies: tt.stripResponseCookies,
			}
			app.managers.Router.UpsertRoute(route)

			// Create request to proxy endpoint
			req := httptest.NewRequest("GET", "http://localhost/test/api", nil)
			w := httptest.NewRecorder()

			// Handle proxy request
			app.handleProxy(w, req)

			// Check response status
			if w.Code != http.StatusOK {
				t.Fatalf("Expected status 200, got %d. Body: %s", w.Code, w.Body.String())
			}

			// Parse response to ensure it's valid
			var response map[string]string
			if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
				t.Fatalf("Failed to decode response: %v", err)
			}

			// Check Set-Cookie headers in response
			receivedCookies := w.Header().Values("Set-Cookie")

			// Verify cookie count
			if len(receivedCookies) != len(tt.expectCookies) {
				t.Errorf("Expected %d cookies, got %d. Received: %v",
					len(tt.expectCookies), len(receivedCookies), receivedCookies)
			}

			// Verify each expected cookie is present
			for i, expectedCookie := range tt.expectCookies {
				if i >= len(receivedCookies) {
					t.Errorf("Missing expected cookie: %s", expectedCookie)
					continue
				}
				if receivedCookies[i] != expectedCookie {
					t.Errorf("Cookie mismatch at index %d:\n  expected: %s\n  got: %s",
						i, expectedCookie, receivedCookies[i])
				}
			}
		})
	}
}

// TestProxyCookiePathRewriting tests cookie path rewriting when strip_prefix is enabled
func TestProxyCookiePathRewriting(t *testing.T) {
	tests := []struct {
		name               string
		stripPrefix        bool
		pathBase           string
		upstreamCookie     string // Cookie that upstream sets
		expectCookie       string // Cookie we expect client to receive
		rewriteCookiePaths bool   // Whether to rewrite cookie paths
	}{
		{
			name:               "rewrite enabled - adds prefix to cookie path",
			stripPrefix:        true,
			pathBase:           "/apps/grafana",
			rewriteCookiePaths: true,
			upstreamCookie:     "session=abc; Path=/",
			expectCookie:       "session=abc; Path=/apps/grafana/",
		},
		{
			name:               "rewrite enabled - combines paths correctly",
			stripPrefix:        true,
			pathBase:           "/apps/grafana",
			rewriteCookiePaths: true,
			upstreamCookie:     "session=abc; Path=/api",
			expectCookie:       "session=abc; Path=/apps/grafana/api",
		},
		{
			name:               "rewrite disabled - preserves original path",
			stripPrefix:        true,
			pathBase:           "/apps/grafana",
			rewriteCookiePaths: false,
			upstreamCookie:     "session=abc; Path=/",
			expectCookie:       "session=abc; Path=/",
		},
		{
			name:               "no strip prefix - no path rewriting",
			stripPrefix:        false,
			pathBase:           "/apps/grafana",
			rewriteCookiePaths: true,
			upstreamCookie:     "session=abc; Path=/",
			expectCookie:       "session=abc; Path=/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create upstream test server that sets a cookie
			upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Set-Cookie", tt.upstreamCookie)
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprintf(w, `{"status":"ok"}`)
			}))
			defer upstreamServer.Close()

			// Create test application with route
			app := newTestApplication(t)
			route := types.Route{
				RouteID:            "test-route",
				AppID:              "test-app",
				PathBase:           tt.pathBase,
				To:                 upstreamServer.URL,
				StripPrefix:        tt.stripPrefix,
				RewriteCookiePaths: tt.rewriteCookiePaths,
			}
			app.managers.Router.UpsertRoute(route)

			// Create request to proxy endpoint
			// Since strip_prefix=true adds trailing slash to route, request must match
			requestPath := tt.pathBase
			if tt.stripPrefix && !strings.HasSuffix(requestPath, "/") {
				requestPath = requestPath + "/"
			}
			req := httptest.NewRequest("GET", "http://localhost"+requestPath+"/api", nil)
			w := httptest.NewRecorder()

			// Handle proxy request
			app.handleProxy(w, req)

			// Check response status
			if w.Code != http.StatusOK {
				t.Fatalf("Expected status 200, got %d", w.Code)
			}

			// Check Set-Cookie header
			receivedCookie := w.Header().Get("Set-Cookie")
			if receivedCookie != tt.expectCookie {
				t.Errorf("Cookie mismatch:\n  expected: %s\n  got: %s",
					tt.expectCookie, receivedCookie)
			}
		})
	}
}

// TestProxySecurityHeaders tests that security-sensitive headers are handled correctly
func TestProxySecurityHeaders(t *testing.T) {
	tests := []struct {
		name                    string
		upstreamSetHeaders      map[string]string // Headers upstream sets in RESPONSE
		expectUpstreamReceived  map[string]string // Headers upstream should RECEIVE in request
		expectClientReceived    map[string]string // Headers client should receive in response
		notExpectClientReceived []string          // Headers client should NOT receive in response
	}{
		{
			name: "authorization header stripped from request",
			upstreamSetHeaders: map[string]string{
				"Content-Type": "application/json",
			},
			expectUpstreamReceived: map[string]string{
				"X-Forwarded-For": "192.168.1.100", // We set this
			},
			expectClientReceived: map[string]string{
				"Content-Type": "application/json",
			},
			notExpectClientReceived: []string{"Authorization"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Track what upstream received
			receivedHeaders := make(map[string]string)
			upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Record all headers upstream received
				for key := range tt.expectUpstreamReceived {
					receivedHeaders[key] = r.Header.Get(key)
				}
				receivedHeaders["Authorization"] = r.Header.Get("Authorization")

				// Set response headers
				for key, value := range tt.upstreamSetHeaders {
					w.Header().Set(key, value)
				}
				w.WriteHeader(http.StatusOK)
			}))
			defer upstreamServer.Close()

			// Create test application
			app := newTestApplication(t)
			route := types.Route{
				RouteID:     "test-route",
				AppID:       "test-app",
				PathBase:    "/test/",
				To:          upstreamServer.URL,
				StripPrefix: true,
			}
			app.managers.Router.UpsertRoute(route)

			// Create request with Authorization header
			req := httptest.NewRequest("GET", "http://localhost/test/api", nil)
			req.Header.Set("Authorization", "Bearer secret-token")
			req.RemoteAddr = "192.168.1.100:54321"
			w := httptest.NewRecorder()

			// Handle proxy request
			app.handleProxy(w, req)

			// Verify Authorization was stripped before forwarding to upstream
			if receivedHeaders["Authorization"] != "" {
				t.Errorf("Authorization header should have been stripped, but upstream received: %s",
					receivedHeaders["Authorization"])
			}

			// Verify upstream received expected headers
			for key, expectedValue := range tt.expectUpstreamReceived {
				actualValue := receivedHeaders[key]
				if actualValue != expectedValue {
					t.Errorf("Upstream should have received header %s=%q, got %q",
						key, expectedValue, actualValue)
				}
			}

			// Verify client received expected response headers
			for key, expectedValue := range tt.expectClientReceived {
				actualValue := w.Header().Get(key)
				if actualValue != expectedValue {
					t.Errorf("Client should have received header %s=%q, got %q",
						key, expectedValue, actualValue)
				}
			}

			// Verify client did NOT receive certain headers
			for _, key := range tt.notExpectClientReceived {
				if w.Header().Get(key) != "" {
					t.Errorf("Client should not have received header %s, but got: %s",
						key, w.Header().Get(key))
				}
			}
		})
	}
}
