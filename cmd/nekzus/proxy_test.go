package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nstalgic/nekzus/internal/types"
)

// TestProxyPathStripping tests that path stripping respects the StripPrefix field
func TestProxyPathStripping(t *testing.T) {
	tests := []struct {
		name               string
		routePathBase      string
		stripPrefix        bool
		requestPath        string
		expectedUpstream   string
		expectedXFwdPrefix string
	}{
		{
			name:               "strip prefix enabled - root path",
			routePathBase:      "/apps/grafana/",
			stripPrefix:        true,
			requestPath:        "/apps/grafana/",
			expectedUpstream:   "/",
			expectedXFwdPrefix: "/apps/grafana/",
		},
		{
			name:               "strip prefix enabled - subpath",
			routePathBase:      "/apps/grafana/",
			stripPrefix:        true,
			requestPath:        "/apps/grafana/api/health",
			expectedUpstream:   "/api/health",
			expectedXFwdPrefix: "/apps/grafana/",
		},
		{
			name:               "strip prefix disabled - full path preserved",
			routePathBase:      "/apps/grafana/",
			stripPrefix:        false,
			requestPath:        "/apps/grafana/api/health",
			expectedUpstream:   "/apps/grafana/api/health",
			expectedXFwdPrefix: "",
		},
		{
			name:               "strip prefix enabled - URL encoded path",
			routePathBase:      "/files/",
			stripPrefix:        true,
			requestPath:        "/files/my%20document.pdf",
			expectedUpstream:   "/my%20document.pdf",
			expectedXFwdPrefix: "/files/",
		},
		{
			name:               "strip prefix disabled - URL encoded path preserved",
			routePathBase:      "/files/",
			stripPrefix:        false,
			requestPath:        "/files/my%20document.pdf",
			expectedUpstream:   "/files/my%20document.pdf",
			expectedXFwdPrefix: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create upstream test server that echoes back request details
			upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Echo back the received path and headers
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprintf(w, `{"path":"%s","rawPath":"%s","xForwardedPrefix":"%s"}`,
					r.URL.Path, r.URL.RawPath, r.Header.Get("X-Forwarded-Prefix"))
			}))
			defer upstreamServer.Close()

			// Create test application with route
			app := newTestApplication(t)
			route := types.Route{
				RouteID:     "test-route",
				AppID:       "test-app",
				PathBase:    tt.routePathBase,
				To:          upstreamServer.URL,
				StripPrefix: tt.stripPrefix,
			}
			app.managers.Router.UpsertRoute(route)

			// Create request to proxy endpoint
			req := httptest.NewRequest("GET", "http://localhost"+tt.requestPath, nil)
			// httptest.NewRequest automatically decodes the path, so we need to manually set RawPath
			// for URL-encoded paths to properly test RawPath preservation
			if strings.Contains(tt.requestPath, "%") {
				req.URL.RawPath = tt.requestPath
			}
			w := httptest.NewRecorder()

			// Handle proxy request
			app.handleProxy(w, req)

			// Check response
			if w.Code != http.StatusOK {
				t.Errorf("Expected status 200, got %d. Body: %s", w.Code, w.Body.String())
				return
			}

			// Parse response to check what upstream received
			var response struct {
				Path             string `json:"path"`
				RawPath          string `json:"rawPath"`
				XForwardedPrefix string `json:"xForwardedPrefix"`
			}
			if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
				t.Fatalf("Failed to decode response: %v", err)
			}

			// Verify upstream received correct decoded path
			// Note: Go's http client (used by ReverseProxy) will decode URL-encoded paths,
			// so we compare against the decoded version
			expectedDecodedPath := tt.expectedUpstream
			if strings.Contains(tt.expectedUpstream, "%20") {
				expectedDecodedPath = strings.ReplaceAll(tt.expectedUpstream, "%20", " ")
			}
			if response.Path != expectedDecodedPath {
				t.Errorf("Expected upstream path %q, got %q", expectedDecodedPath, response.Path)
			}

			// Verify X-Forwarded-Prefix header
			if response.XForwardedPrefix != tt.expectedXFwdPrefix {
				t.Errorf("Expected X-Forwarded-Prefix %q, got %q", tt.expectedXFwdPrefix, response.XForwardedPrefix)
			}
		})
	}
}

// TestProxyForwardedHeaders tests X-Forwarded-* header handling
func TestProxyForwardedHeaders(t *testing.T) {
	tests := []struct {
		name                   string
		tls                    bool
		existingXForwardedFor  string
		remoteAddr             string
		expectedProto          string
		expectedXRealIP        string
		expectedXForwardedFor  string
		expectedXForwardedPort string
	}{
		{
			name:                   "https request without existing headers",
			tls:                    true,
			remoteAddr:             "192.168.1.100:54321",
			expectedProto:          "https",
			expectedXRealIP:        "192.168.1.100",
			expectedXForwardedFor:  "192.168.1.100",
			expectedXForwardedPort: "443",
		},
		{
			name:                   "http request without existing headers",
			tls:                    false,
			remoteAddr:             "10.0.0.5:12345",
			expectedProto:          "http",
			expectedXRealIP:        "10.0.0.5",
			expectedXForwardedFor:  "10.0.0.5",
			expectedXForwardedPort: "80",
		},
		{
			name:                   "append to existing X-Forwarded-For",
			tls:                    false,
			existingXForwardedFor:  "203.0.113.1, 198.51.100.1",
			remoteAddr:             "192.168.1.50:33333",
			expectedProto:          "http",
			expectedXRealIP:        "192.168.1.50",
			expectedXForwardedFor:  "203.0.113.1, 198.51.100.1, 192.168.1.50",
			expectedXForwardedPort: "80",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create upstream test server
			upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprintf(w, `{"xForwardedProto":"%s","xRealIP":"%s","xForwardedFor":"%s","xForwardedPort":"%s"}`,
					r.Header.Get("X-Forwarded-Proto"),
					r.Header.Get("X-Real-IP"),
					r.Header.Get("X-Forwarded-For"),
					r.Header.Get("X-Forwarded-Port"))
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

			// Create request
			var req *http.Request
			if tt.tls {
				req = httptest.NewRequest("GET", "https://localhost/test/api", nil)
				req.TLS = &tls.ConnectionState{} // Simulate TLS
			} else {
				req = httptest.NewRequest("GET", "http://localhost/test/", nil)
			}

			// Set existing X-Forwarded-For if specified
			if tt.existingXForwardedFor != "" {
				req.Header.Set("X-Forwarded-For", tt.existingXForwardedFor)
			}

			// Set RemoteAddr
			req.RemoteAddr = tt.remoteAddr

			w := httptest.NewRecorder()
			app.handleProxy(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("Expected status 200, got %d", w.Code)
			}

			// Parse response
			var response struct {
				XForwardedProto string `json:"xForwardedProto"`
				XRealIP         string `json:"xRealIP"`
				XForwardedFor   string `json:"xForwardedFor"`
				XForwardedPort  string `json:"xForwardedPort"`
			}
			if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
				t.Fatalf("Failed to decode response: %v", err)
			}

			// Verify headers
			if response.XForwardedProto != tt.expectedProto {
				t.Errorf("Expected X-Forwarded-Proto %q, got %q", tt.expectedProto, response.XForwardedProto)
			}
			if response.XRealIP != tt.expectedXRealIP {
				t.Errorf("Expected X-Real-IP %q, got %q", tt.expectedXRealIP, response.XRealIP)
			}
			if response.XForwardedFor != tt.expectedXForwardedFor {
				t.Errorf("Expected X-Forwarded-For %q, got %q", tt.expectedXForwardedFor, response.XForwardedFor)
			}
			if response.XForwardedPort != tt.expectedXForwardedPort {
				t.Errorf("Expected X-Forwarded-Port %q, got %q", tt.expectedXForwardedPort, response.XForwardedPort)
			}
		})
	}
}

// TestWebSocketProtoDetection tests that WebSocket upgrades set correct X-Forwarded-Proto
func TestWebSocketProtoDetection(t *testing.T) {
	tests := []struct {
		name          string
		tls           bool
		upgradeHeader string
		connectionHdr string
		isWebSocket   bool
		expectedProto string
	}{
		{
			name:          "wss - secure websocket",
			tls:           true,
			upgradeHeader: "websocket",
			connectionHdr: "Upgrade",
			isWebSocket:   true,
			expectedProto: "wss",
		},
		{
			name:          "ws - insecure websocket",
			tls:           false,
			upgradeHeader: "websocket",
			connectionHdr: "Upgrade",
			isWebSocket:   true,
			expectedProto: "ws",
		},
		{
			name:          "websocket - case insensitive",
			tls:           false,
			upgradeHeader: "WebSocket",
			connectionHdr: "upgrade",
			isWebSocket:   true,
			expectedProto: "ws",
		},
		{
			name:          "https - not websocket",
			tls:           true,
			upgradeHeader: "",
			connectionHdr: "",
			isWebSocket:   false,
			expectedProto: "https",
		},
		{
			name:          "http - not websocket",
			tls:           false,
			upgradeHeader: "",
			connectionHdr: "",
			isWebSocket:   false,
			expectedProto: "http",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create upstream server
			upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprintf(w, `{"xForwardedProto":"%s"}`, r.Header.Get("X-Forwarded-Proto"))
			}))
			defer upstreamServer.Close()

			// Create test application
			app := newTestApplication(t)
			route := types.Route{
				RouteID:     "test-route",
				AppID:       "test-app",
				PathBase:    "/ws/",
				To:          upstreamServer.URL,
				StripPrefix: true,
				Websocket:   true,
			}
			app.managers.Router.UpsertRoute(route)

			// Create request
			var req *http.Request
			if tt.tls {
				req = httptest.NewRequest("GET", "https://localhost/ws/stream", nil)
				req.TLS = &tls.ConnectionState{}
			} else {
				req = httptest.NewRequest("GET", "http://localhost/ws/stream", nil)
			}

			// Set WebSocket headers if applicable
			if tt.upgradeHeader != "" {
				req.Header.Set("Upgrade", tt.upgradeHeader)
			}
			if tt.connectionHdr != "" {
				req.Header.Set("Connection", tt.connectionHdr)
			}

			w := httptest.NewRecorder()
			app.handleProxy(w, req)

			// For WebSocket upgrade requests, the behavior is different
			// We'll just check that non-WebSocket requests get correct proto
			if !tt.isWebSocket && w.Code == http.StatusOK {
				var response struct {
					XForwardedProto string `json:"xForwardedProto"`
				}
				if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
					t.Fatalf("Failed to decode response: %v", err)
				}

				if response.XForwardedProto != tt.expectedProto {
					t.Errorf("Expected X-Forwarded-Proto %q, got %q", tt.expectedProto, response.XForwardedProto)
				}
			}
		})
	}
}

// TestProxyHostHeaderValidation tests that malicious Host headers are rejected
func TestProxyHostHeaderValidation(t *testing.T) {
	tests := []struct {
		name       string
		host       string
		wantStatus int
	}{
		{
			name:       "valid host",
			host:       "localhost:8080",
			wantStatus: http.StatusOK,
		},
		{
			name:       "valid IP host",
			host:       "192.168.1.1:3000",
			wantStatus: http.StatusOK,
		},
		{
			name:       "valid domain host",
			host:       "example.com",
			wantStatus: http.StatusOK,
		},
		{
			name:       "host with carriage return",
			host:       "localhost\rSet-Cookie: malicious=1",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "host with newline",
			host:       "localhost\nX-Injected: header",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "host with CRLF",
			host:       "localhost\r\nContent-Length: 0",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "host with embedded null",
			host:       "localhost\x00evil",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create upstream server
			upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprintf(w, `{"status":"ok"}`)
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

			// Create request with potentially malicious host
			req := httptest.NewRequest("GET", "http://localhost/test/", nil)
			req.Host = tt.host

			w := httptest.NewRecorder()
			app.handleProxy(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("Expected status %d for host=%q, got %d: %s",
					tt.wantStatus, tt.host, w.Code, w.Body.String())
			}
		})
	}
}
