package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestAPIVersionHeader verifies version header is added to all API responses
func TestAPIVersionHeader(t *testing.T) {
	version := "1.2.0"
	handler := APIVersion(version)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))

	req := httptest.NewRequest("GET", "/api/v1/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if got := w.Header().Get("X-API-Version"); got != version {
		t.Errorf("X-API-Version header = %q, want %q", got, version)
	}
}

// TestDeprecationHeader verifies deprecation headers for deprecated endpoints
func TestDeprecationHeader(t *testing.T) {
	tests := []struct {
		name          string
		sunset        string
		wantDeprecate bool
		wantSunset    string
		wantLink      string
	}{
		{
			name:          "deprecated_with_sunset",
			sunset:        "2026-01-01",
			wantDeprecate: true,
			wantSunset:    "Thu, 01 Jan 2026 00:00:00 GMT",
			wantLink:      `</api/v2/endpoint>; rel="successor-version"`,
		},
		{
			name:          "deprecated_without_sunset",
			sunset:        "",
			wantDeprecate: true,
			wantSunset:    "",
			wantLink:      "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := DeprecationOptions{
				Sunset:         tt.sunset,
				SuccessorPath:  "/api/v2/endpoint",
				DeprecationMsg: "This endpoint is deprecated",
			}

			handler := Deprecated(opts)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			req := httptest.NewRequest("GET", "/api/v1/old", nil)
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			// Check Deprecation header
			if deprecation := w.Header().Get("Deprecation"); deprecation == "" {
				if tt.wantDeprecate {
					t.Error("Missing Deprecation header")
				}
			}

			// Check Sunset header
			if sunset := w.Header().Get("Sunset"); sunset != tt.wantSunset && tt.wantSunset != "" {
				t.Errorf("Sunset header = %q, want %q", sunset, tt.wantSunset)
			}

			// Check Link header for successor
			if link := w.Header().Get("Link"); tt.wantLink != "" && link != tt.wantLink {
				t.Errorf("Link header = %q, want %q", link, tt.wantLink)
			}
		})
	}
}

// TestDeprecationWarning verifies Warning header for deprecated endpoints
func TestDeprecationWarning(t *testing.T) {
	opts := DeprecationOptions{
		DeprecationMsg: "This endpoint will be removed in v2.0",
	}

	handler := Deprecated(opts)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/v1/old", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	warning := w.Header().Get("Warning")
	if warning == "" {
		t.Error("Missing Warning header for deprecated endpoint")
	}

	// Warning format: 299 - "message"
	wantPrefix := "299"
	if len(warning) < len(wantPrefix) || warning[:3] != wantPrefix {
		t.Errorf("Warning header should start with '299', got %q", warning)
	}
}

// TestVersioningChain verifies version and deprecation middleware can be chained
func TestVersioningChain(t *testing.T) {
	version := "1.0.0"
	opts := DeprecationOptions{
		Sunset:         "2026-12-31",
		DeprecationMsg: "Use v2 API",
	}

	handler := APIVersion(version)(
		Deprecated(opts)(
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}),
		),
	)

	req := httptest.NewRequest("GET", "/api/v1/old", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	// Both version and deprecation headers should be present
	if got := w.Header().Get("X-API-Version"); got != version {
		t.Errorf("Missing or incorrect X-API-Version header: %q", got)
	}

	if got := w.Header().Get("Deprecation"); got == "" {
		t.Error("Missing Deprecation header in chained middleware")
	}
}
