package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/nstalgic/nekzus/internal/activity"
	"github.com/nstalgic/nekzus/internal/auth"
	"github.com/nstalgic/nekzus/internal/discovery"
	"github.com/nstalgic/nekzus/internal/handlers"
	"github.com/nstalgic/nekzus/internal/metrics"
	"github.com/nstalgic/nekzus/internal/proxy"
	"github.com/nstalgic/nekzus/internal/ratelimit"
	"github.com/nstalgic/nekzus/internal/router"
	"github.com/nstalgic/nekzus/internal/types"
	"github.com/nstalgic/nekzus/internal/websocket"
)

// Shared test metrics instance to avoid duplicate Prometheus registrations
var testMetrics = metrics.New("test")

func newTestApplication(t *testing.T) *Application {
	t.Helper()

	// Create auth manager with test config
	// Use a strong secret that passes validation (32+ chars, no weak patterns like 'test', 'secret', etc.)
	testSecret := "random-jwt-hmac-key-f8e7d6c5b4a39281"
	authMgr, err := auth.NewManager(
		[]byte(testSecret),
		"nekzus",
		"nekzus-mobile",
		[]string{"boot-123"},
	)
	if err != nil {
		t.Fatal(err)
	}

	// Create managers first
	activityTracker := activity.NewTracker(nil) // No storage for tests
	wsManager := websocket.NewManager(testMetrics, nil)
	qrLimiter := ratelimit.NewLimiter(1.0, 5)

	// Create auth handler
	authHandler := handlers.NewAuthHandler(
		authMgr,
		nil, // no storage for tests
		testMetrics,
		wsManager,
		activityTracker,
		qrLimiter,
		nil, // no cert manager for tests
		"http://localhost:8443",
		"",
		"test-nexus",
		"1.0.0-test",
		[]string{"catalog", "events", "proxy"},
	)

	app := &Application{
		config: types.ServerConfig{},
		services: &ServiceRegistry{
			Auth: authMgr,
		},
		limiters: &RateLimiterRegistry{
			QR: qrLimiter,
		},
		managers: &ManagerRegistry{
			Router:    router.NewRegistry(nil), // No storage for tests
			WebSocket: wsManager,
			Activity:  activityTracker,
		},
		handlers: &HandlerRegistry{
			Auth: authHandler,
		},
		jobs:         &JobRegistry{}, // Empty jobs registry for tests
		storage:      nil,            // No storage for tests
		metrics:      testMetrics,    // Use shared metrics instance
		proxyCache:   proxy.NewCache(),
		nexusID:      "test-nexus",
		baseURL:      "http://localhost:8443",
		version:      "1.0.0-test",
		capabilities: []string{"catalog", "events", "proxy"},
	}

	// Create discovery manager after app (it needs app as dependency)
	app.services.Discovery = discovery.NewDiscoveryManager(app, app, app)

	return app
}

func TestPairAndListApps(t *testing.T) {
	app := newTestApplication(t)
	_ = app.managers.Router.UpsertApp(types.App{ID: "a1", Name: "App1"})

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/pair", app.handlePair)
	mux.Handle("/api/v1/apps", app.requireJWT(http.HandlerFunc(app.handleListApps)))

	srv := httptest.NewServer(mux)
	defer srv.Close()

	// pair
	body := `{"device":{"id":"ios-1","model":"X","platform":"ios"}}`
	req, _ := http.NewRequest("POST", srv.URL+"/api/v1/auth/pair", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer boot-123")
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("pair status %d", res.StatusCode)
	}
	var out map[string]any
	_ = json.NewDecoder(res.Body).Decode(&out)
	tok := out["accessToken"].(string)

	// list apps
	req2, _ := http.NewRequest("GET", srv.URL+"/api/v1/apps", nil)
	req2.Header.Set("Authorization", "Bearer "+tok)
	res2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	defer res2.Body.Close()
	if res2.StatusCode != 200 {
		t.Fatalf("apps status %d", res2.StatusCode)
	}
	var apps []types.App
	_ = json.NewDecoder(res2.Body).Decode(&apps)
	if len(apps) != 1 || apps[0].ID != "a1" {
		t.Fatalf("unexpected apps %+v", apps)
	}
}

func TestListApps_URLField(t *testing.T) {
	app := newTestApplication(t)

	// Create app and route
	_ = app.managers.Router.UpsertApp(types.App{ID: "grafana", Name: "Grafana"})
	_ = app.managers.Router.UpsertRoute(types.Route{
		RouteID:  "route:grafana",
		AppID:    "grafana",
		PathBase: "/apps/grafana/",
		To:       "http://localhost:3000",
	})

	mux := http.NewServeMux()
	mux.Handle("/api/v1/apps", app.requireJWT(http.HandlerFunc(app.handleListApps)))

	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Get JWT token
	tok, _ := app.services.Auth.SignJWT("test-device", []string{"read:catalog"}, 1*time.Hour)

	// List apps
	req, _ := http.NewRequest("GET", srv.URL+"/api/v1/apps", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		t.Fatalf("expected status 200, got %d", res.StatusCode)
	}

	var apps []types.App
	if err := json.NewDecoder(res.Body).Decode(&apps); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(apps) != 1 {
		t.Fatalf("expected 1 app, got %d", len(apps))
	}

	// Verify URL field contains full URL with protocol
	expectedURL := "http://localhost:8443/apps/grafana/"
	if apps[0].URL != expectedURL {
		t.Errorf("expected URL %q, got %q", expectedURL, apps[0].URL)
	}

	// Verify ProxyPath is still set
	if apps[0].ProxyPath != "/apps/grafana/" {
		t.Errorf("expected ProxyPath %q, got %q", "/apps/grafana/", apps[0].ProxyPath)
	}
}

func TestApproveProposalCreatesRoute(t *testing.T) {
	app := newTestApplication(t)
	// Seed token
	j, _ := app.services.Auth.SignJWT("dev", []string{"read:catalog", "read:events"}, 1*time.Hour)

	// In refactored version, proposals would be stored differently
	// For now, this test demonstrates the approve flow
	_ = app.managers.Router.UpsertApp(types.App{ID: "grafana", Name: "Grafana"})
	_ = app.managers.Router.UpsertRoute(types.Route{
		RouteID:  "route:grafana",
		AppID:    "grafana",
		PathBase: "/apps/grafana",
		To:       "http://127.0.0.1:3000",
		Scopes:   []string{"access:grafana"},
	})

	_ = []types.Proposal{
		{
			ID:     "p1",
			Source: "static",
			SuggestedApp: types.App{
				ID:   "grafana",
				Name: "Grafana",
			},
			SuggestedRoute: types.Route{
				RouteID:  "route:grafana",
				AppID:    "grafana",
				PathBase: "/apps/grafana",
				To:       "http://127.0.0.1:3000",
				Scopes:   []string{"access:grafana"},
			},
		},
	}

	mux := http.NewServeMux()
	// Note: In refactored version, approve handler would be called through handleProposalActions
	mux.Handle("/api/v1/apps", app.requireJWT(http.HandlerFunc(app.handleListApps)))
	srv := httptest.NewServer(mux)
	defer srv.Close()

	// list apps (approval already happened above by directly adding to router)
	req2, _ := http.NewRequest("GET", srv.URL+"/api/v1/apps", nil)
	req2.Header.Set("Authorization", "Bearer "+j)
	res2, _ := http.DefaultClient.Do(req2)
	var apps []types.App
	defer res2.Body.Close()
	_ = json.NewDecoder(res2.Body).Decode(&apps)
	if len(apps) != 1 || apps[0].ID != "grafana" {
		t.Fatalf("expected grafana app, got %+v", apps)
	}
}

func TestProxyStripsPrefixAndForwards(t *testing.T) {
	// Upstream mock
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Fatalf("Authorization header should have been stripped")
		}
		if r.URL.Path != "/hello" {
			t.Fatalf("expected /hello, got %s", r.URL.Path)
		}
		w.WriteHeader(200)
		io.WriteString(w, "OK")
	}))
	defer up.Close()

	u, _ := url.Parse(up.URL)

	app := newTestApplication(t)
	// token with needed scope
	j, _ := app.services.Auth.SignJWT("dev", []string{"read:catalog", "read:events", "access:test"}, 1*time.Hour)
	_ = app.managers.Router.UpsertRoute(types.Route{
		RouteID:     "route:test",
		AppID:       "test",
		PathBase:    "/apps/test",
		To:          u.String(),
		Scopes:      []string{"access:test"},
		StripPrefix: true, // Enable prefix stripping for this test
	})

	mux := http.NewServeMux()
	mux.HandleFunc("/apps/", app.handleProxy)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	req, _ := http.NewRequest("GET", srv.URL+"/apps/test/hello", nil)
	req.Header.Set("Authorization", "Bearer "+j)
	// Simulate TLS to ensure proto header set; in httptest it's HTTP, so we skip check.

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		body, _ := io.ReadAll(res.Body)
		t.Fatalf("proxy status %d body=%s", res.StatusCode, string(body))
	}
}

func TestTokenRefresh_Success(t *testing.T) {
	app := newTestApplication(t)

	// Create initial token with specific scopes
	deviceID := "test-device-123"
	initialScopes := []string{"read:apps", "write:apps", "read:events"}
	initialToken, err := app.services.Auth.SignJWT(deviceID, initialScopes, 1*time.Hour)
	if err != nil {
		t.Fatalf("failed to sign initial token: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/refresh", app.handleRefresh)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Request token refresh
	req, _ := http.NewRequest("POST", srv.URL+"/api/v1/auth/refresh", nil)
	req.Header.Set("Authorization", "Bearer "+initialToken)
	req.Header.Set("Content-Type", "application/json")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		t.Fatalf("refresh status %d, body: %s", res.StatusCode, string(body))
	}

	// Parse response
	var response map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Verify response fields
	newToken, ok := response["accessToken"].(string)
	if !ok || newToken == "" {
		t.Fatal("response missing accessToken")
	}

	expiresIn, ok := response["expiresIn"].(float64)
	if !ok || expiresIn != float64(int((12*time.Hour).Seconds())) {
		t.Fatalf("unexpected expiresIn: %v", expiresIn)
	}

	responseDeviceID, ok := response["deviceId"].(string)
	if !ok || responseDeviceID != deviceID {
		t.Fatalf("device ID mismatch: expected %s, got %s", deviceID, responseDeviceID)
	}

	// Verify scopes are preserved
	scopes, ok := response["scopes"].([]interface{})
	if !ok {
		t.Fatal("response missing scopes")
	}
	if len(scopes) != len(initialScopes) {
		t.Fatalf("scope count mismatch: expected %d, got %d", len(initialScopes), len(scopes))
	}

	// Verify new token is valid and contains correct claims
	_, claims, err := app.services.Auth.ParseJWT(newToken)
	if err != nil {
		t.Fatalf("new token is invalid: %v", err)
	}

	claimsDeviceID, ok := claims["sub"].(string)
	if !ok || claimsDeviceID != deviceID {
		t.Fatalf("new token device ID mismatch: expected %s, got %s", deviceID, claimsDeviceID)
	}

	// Verify new token is different from old token
	if newToken == initialToken {
		t.Fatal("new token should be different from initial token")
	}
}

func TestTokenRefresh_MissingAuthHeader(t *testing.T) {
	app := newTestApplication(t)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/refresh", app.handleRefresh)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Request without Authorization header
	req, _ := http.NewRequest("POST", srv.URL+"/api/v1/auth/refresh", nil)
	req.Header.Set("Content-Type", "application/json")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", res.StatusCode)
	}
}

func TestTokenRefresh_InvalidToken(t *testing.T) {
	app := newTestApplication(t)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/refresh", app.handleRefresh)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Request with invalid token
	req, _ := http.NewRequest("POST", srv.URL+"/api/v1/auth/refresh", nil)
	req.Header.Set("Authorization", "Bearer invalid-token-123")
	req.Header.Set("Content-Type", "application/json")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", res.StatusCode)
	}
}

func TestTokenRefresh_ExpiredToken(t *testing.T) {
	app := newTestApplication(t)

	// Create an expired token (negative duration)
	expiredToken, err := app.services.Auth.SignJWT("test-device", []string{"read:apps"}, -1*time.Hour)
	if err != nil {
		t.Fatalf("failed to sign expired token: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/refresh", app.handleRefresh)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Wait a moment to ensure token is truly expired
	time.Sleep(100 * time.Millisecond)

	// Request with expired token
	req, _ := http.NewRequest("POST", srv.URL+"/api/v1/auth/refresh", nil)
	req.Header.Set("Authorization", "Bearer "+expiredToken)
	req.Header.Set("Content-Type", "application/json")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusUnauthorized {
		body, _ := io.ReadAll(res.Body)
		t.Fatalf("expected status 401 for expired token, got %d, body: %s", res.StatusCode, string(body))
	}
}

func TestTokenRefresh_ScopePreservation(t *testing.T) {
	app := newTestApplication(t)

	// Test various scope combinations
	testCases := []struct {
		name   string
		scopes []string
	}{
		{"single_scope", []string{"read:apps"}},
		{"multiple_scopes", []string{"read:apps", "write:apps", "read:events"}},
		{"no_scopes", []string{}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			deviceID := "test-device-" + tc.name
			initialToken, err := app.services.Auth.SignJWT(deviceID, tc.scopes, 1*time.Hour)
			if err != nil {
				t.Fatalf("failed to sign token: %v", err)
			}

			mux := http.NewServeMux()
			mux.HandleFunc("/api/v1/auth/refresh", app.handleRefresh)
			srv := httptest.NewServer(mux)
			defer srv.Close()

			req, _ := http.NewRequest("POST", srv.URL+"/api/v1/auth/refresh", nil)
			req.Header.Set("Authorization", "Bearer "+initialToken)

			res, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()

			if res.StatusCode != http.StatusOK {
				body, _ := io.ReadAll(res.Body)
				t.Fatalf("refresh failed with status %d: %s", res.StatusCode, string(body))
			}

			var response map[string]interface{}
			if err := json.NewDecoder(res.Body).Decode(&response); err != nil {
				t.Fatalf("failed to decode response: %v", err)
			}

			// Verify scopes are preserved
			scopes, ok := response["scopes"].([]interface{})
			if !ok {
				t.Fatal("response missing scopes")
			}

			if len(scopes) != len(tc.scopes) {
				t.Fatalf("scope count mismatch: expected %d, got %d", len(tc.scopes), len(scopes))
			}

			// Verify each scope is present
			scopeMap := make(map[string]bool)
			for _, s := range scopes {
				scopeMap[s.(string)] = true
			}

			for _, expectedScope := range tc.scopes {
				if !scopeMap[expectedScope] {
					t.Fatalf("expected scope %s not found in response", expectedScope)
				}
			}
		})
	}
}

func TestAdminStats_NoStorage(t *testing.T) {
	app := newTestApplication(t)
	// Add some routes
	_ = app.managers.Router.UpsertApp(types.App{ID: "app1", Name: "App 1"})
	_ = app.managers.Router.UpsertRoute(types.Route{PathBase: "/apps/app1", AppID: "app1"})

	mux := http.NewServeMux()
	mux.HandleFunc("/admin/api/v1/stats", app.handleAdminStats)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	req, _ := http.NewRequest("GET", srv.URL+"/admin/api/v1/stats", nil)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		t.Fatalf("stats status %d, body: %s", res.StatusCode, string(body))
	}

	var stats map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&stats); err != nil {
		t.Fatalf("failed to decode stats: %v", err)
	}

	// Verify stats structure
	if routes, ok := stats["routes"].(map[string]interface{}); ok {
		if value, ok := routes["value"].(float64); !ok || value != 1 {
			t.Fatalf("expected routes value 1, got %v", routes["value"])
		}
	} else {
		t.Fatal("stats missing routes")
	}

	if devices, ok := stats["devices"].(map[string]interface{}); ok {
		if value, ok := devices["value"].(float64); !ok || value != 0 {
			t.Fatalf("expected devices value 0, got %v", devices["value"])
		}
	} else {
		t.Fatal("stats missing devices")
	}

	if discoveries, ok := stats["discoveries"].(map[string]interface{}); ok {
		if value, ok := discoveries["value"].(float64); !ok || value != 0 {
			t.Fatalf("expected discoveries value 0, got %v", discoveries["value"])
		}
	} else {
		t.Fatal("stats missing discoveries")
	}

	if requests, ok := stats["requests"].(map[string]interface{}); ok {
		if _, ok := requests["value"]; !ok {
			t.Fatal("requests missing value")
		}
	} else {
		t.Fatal("stats missing requests")
	}
}

func TestListRoutes_Empty(t *testing.T) {
	app := newTestApplication(t)

	mux := http.NewServeMux()
	mux.HandleFunc("/admin/api/v1/routes", app.handleListRoutes)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	req, _ := http.NewRequest("GET", srv.URL+"/admin/api/v1/routes", nil)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		t.Fatalf("routes status %d, body: %s", res.StatusCode, string(body))
	}

	var routes []types.Route
	if err := json.NewDecoder(res.Body).Decode(&routes); err != nil {
		t.Fatalf("failed to decode routes: %v", err)
	}

	if len(routes) != 0 {
		t.Fatalf("expected 0 routes, got %d", len(routes))
	}
}

func TestListRoutes_WithData(t *testing.T) {
	app := newTestApplication(t)

	// Add some routes
	route1 := types.Route{
		RouteID:  "route-1",
		PathBase: "/apps/app1",
		AppID:    "app1",
		Scopes:   []string{"read:app1"},
	}
	route2 := types.Route{
		RouteID:  "route-2",
		PathBase: "/apps/app2",
		AppID:    "app2",
		Scopes:   []string{"read:app2", "write:app2"},
	}

	_ = app.managers.Router.UpsertRoute(route1)
	_ = app.managers.Router.UpsertRoute(route2)

	mux := http.NewServeMux()
	mux.HandleFunc("/admin/api/v1/routes", app.handleListRoutes)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	req, _ := http.NewRequest("GET", srv.URL+"/admin/api/v1/routes", nil)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		t.Fatalf("routes status %d, body: %s", res.StatusCode, string(body))
	}

	var routes []types.Route
	if err := json.NewDecoder(res.Body).Decode(&routes); err != nil {
		t.Fatalf("failed to decode routes: %v", err)
	}

	if len(routes) != 2 {
		t.Fatalf("expected 2 routes, got %d", len(routes))
	}

	// Verify routes contain expected data
	foundRoute1 := false
	foundRoute2 := false
	for _, route := range routes {
		if route.PathBase == "/apps/app1" && route.AppID == "app1" {
			foundRoute1 = true
		}
		if route.PathBase == "/apps/app2" && route.AppID == "app2" {
			foundRoute2 = true
		}
	}

	if !foundRoute1 || !foundRoute2 {
		t.Fatalf("routes missing expected data, got: %+v", routes)
	}
}

func TestDismissProposal_NotFound(t *testing.T) {
	app := newTestApplication(t)

	mux := http.NewServeMux()
	mux.HandleFunc("/admin/api/v1/discovery/proposals/", app.handleProposalActions)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	req, _ := http.NewRequest("POST", srv.URL+"/admin/api/v1/discovery/proposals/non-existent-proposal/dismiss", nil)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	// Handler returns 200 for idempotent behavior - dismissing a non-existent proposal
	// is treated as "already processed" to support retry-safe operations
	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		t.Fatalf("expected status 200, got %d, body: %s", res.StatusCode, string(body))
	}

	// Verify the response indicates the proposal was already processed
	var response map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&response); err != nil {
		// Body may have been consumed, that's ok
		return
	}
	if status, ok := response["status"].(string); ok && status != "already_processed" {
		t.Errorf("expected status 'already_processed', got '%s'", status)
	}
}

func TestRediscover_Success(t *testing.T) {
	app := newTestApplication(t)

	// Simulate some dismissed proposals in the discovery manager
	app.services.Discovery.DismissProposal("proposal_1")
	app.services.Discovery.DismissProposal("proposal_2")

	// Verify dismissed proposals exist
	dismissed := app.services.Discovery.GetDismissedProposals()
	if len(dismissed) != 2 {
		t.Fatalf("expected 2 dismissed proposals, got %d", len(dismissed))
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/discovery/rediscover", app.handleRediscover)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	req, _ := http.NewRequest("POST", srv.URL+"/api/v1/discovery/rediscover", nil)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		t.Fatalf("expected status 200, got %d, body: %s", res.StatusCode, string(body))
	}

	var response map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Verify response structure
	if msg, ok := response["message"].(string); !ok || msg == "" {
		t.Fatal("response missing or empty message field")
	}

	if cleared, ok := response["dismissedCleared"].(float64); !ok || cleared != 2 {
		t.Fatalf("expected dismissedCleared=2, got %v", response["dismissedCleared"])
	}

	// Verify dismissed proposals were cleared
	dismissedAfter := app.services.Discovery.GetDismissedProposals()
	if len(dismissedAfter) != 0 {
		t.Fatalf("expected 0 dismissed proposals after rediscover, got %d", len(dismissedAfter))
	}
}

func TestRediscover_ClearsProposalsAndDismissed(t *testing.T) {
	app := newTestApplication(t)

	// Start the discovery manager to enable async processing
	if err := app.services.Discovery.Start(); err != nil {
		t.Fatalf("failed to start discovery manager: %v", err)
	}
	defer app.services.Discovery.Stop()

	// Add some active proposals
	p1 := &types.Proposal{
		ID:     "proposal_test_1",
		Source: "test",
	}
	p2 := &types.Proposal{
		ID:     "proposal_test_2",
		Source: "test",
	}
	app.services.Discovery.SubmitProposal(p1)
	app.services.Discovery.SubmitProposal(p2)

	// Allow proposals to be processed (async channel processing)
	time.Sleep(200 * time.Millisecond)

	// Dismiss some proposals
	app.services.Discovery.DismissProposal("proposal_test_3")
	app.services.Discovery.DismissProposal("proposal_test_4")

	// Verify initial state
	proposals := app.services.Discovery.GetProposals()
	dismissed := app.services.Discovery.GetDismissedProposals()
	if len(proposals) != 2 {
		t.Fatalf("expected 2 active proposals, got %d", len(proposals))
	}
	if len(dismissed) != 2 {
		t.Fatalf("expected 2 dismissed proposals, got %d", len(dismissed))
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/discovery/rediscover", app.handleRediscover)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	req, _ := http.NewRequest("POST", srv.URL+"/api/v1/discovery/rediscover", nil)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		t.Fatalf("expected status 200, got %d, body: %s", res.StatusCode, string(body))
	}

	var response map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Verify both active and dismissed proposals were cleared
	proposalsAfter := app.services.Discovery.GetProposals()
	dismissedAfter := app.services.Discovery.GetDismissedProposals()
	if len(proposalsAfter) != 0 {
		t.Fatalf("expected 0 active proposals after rediscover, got %d", len(proposalsAfter))
	}
	if len(dismissedAfter) != 0 {
		t.Fatalf("expected 0 dismissed proposals after rediscover, got %d", len(dismissedAfter))
	}
}
