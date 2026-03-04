package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPToHTTPSRedirectHandler(t *testing.T) {
	tests := []struct {
		name           string
		requestURL     string
		host           string
		httpsPort      string
		expectedStatus int
		expectedURL    string
	}{
		{
			name:           "redirects root path to HTTPS",
			requestURL:     "/",
			host:           "example.com",
			httpsPort:      "8443",
			expectedStatus: http.StatusMovedPermanently,
			expectedURL:    "https://example.com:8443/",
		},
		{
			name:           "redirects path with query string",
			requestURL:     "/api/apps?foo=bar",
			host:           "example.com",
			httpsPort:      "8443",
			expectedStatus: http.StatusMovedPermanently,
			expectedURL:    "https://example.com:8443/api/apps?foo=bar",
		},
		{
			name:           "strips HTTP port from host",
			requestURL:     "/test",
			host:           "example.com:8080",
			httpsPort:      "8443",
			expectedStatus: http.StatusMovedPermanently,
			expectedURL:    "https://example.com:8443/test",
		},
		{
			name:           "uses default HTTPS port 443 when configured",
			requestURL:     "/path",
			host:           "example.com",
			httpsPort:      "443",
			expectedStatus: http.StatusMovedPermanently,
			expectedURL:    "https://example.com/path",
		},
		{
			name:           "preserves complex query strings",
			requestURL:     "/search?q=hello+world&page=1",
			host:           "localhost",
			httpsPort:      "8443",
			expectedStatus: http.StatusMovedPermanently,
			expectedURL:    "https://localhost:8443/search?q=hello+world&page=1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := newHTTPToHTTPSRedirectHandler(tt.httpsPort)

			req := httptest.NewRequest(http.MethodGet, tt.requestURL, nil)
			req.Host = tt.host
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			if rr.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, rr.Code)
			}

			location := rr.Header().Get("Location")
			if location != tt.expectedURL {
				t.Errorf("expected Location %q, got %q", tt.expectedURL, location)
			}
		})
	}
}

func TestHTTPToHTTPSRedirectHandler_Methods(t *testing.T) {
	handler := newHTTPToHTTPSRedirectHandler("8443")

	methods := []string{
		http.MethodGet,
		http.MethodPost,
		http.MethodPut,
		http.MethodDelete,
		http.MethodPatch,
		http.MethodHead,
	}

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/test", nil)
			req.Host = "example.com"
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			if rr.Code != http.StatusMovedPermanently {
				t.Errorf("method %s: expected status %d, got %d",
					method, http.StatusMovedPermanently, rr.Code)
			}
		})
	}
}
