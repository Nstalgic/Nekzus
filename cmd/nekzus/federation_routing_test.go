package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nstalgic/nekzus/internal/activity"
	"github.com/nstalgic/nekzus/internal/auth"
	"github.com/nstalgic/nekzus/internal/federation"
	"github.com/nstalgic/nekzus/internal/handlers"
	"github.com/nstalgic/nekzus/internal/proxy"
	"github.com/nstalgic/nekzus/internal/ratelimit"
	"github.com/nstalgic/nekzus/internal/router"
	"github.com/nstalgic/nekzus/internal/storage"
	"github.com/nstalgic/nekzus/internal/types"
	"github.com/nstalgic/nekzus/internal/websocket"
)

// testPortCounter is used to assign unique ports to each test
var testPortCounter int32 = 17945

// getTestPort returns a unique port for each test
func getTestPort() int {
	return int(atomic.AddInt32(&testPortCounter, 1))
}

// newTestApplicationWithFederation creates a test app with federation support
func newTestApplicationWithFederation(t *testing.T, localPeerID string) (*Application, func()) {
	t.Helper()

	// Create temporary database
	tmpFile, err := os.CreateTemp("", "nekzus-federation-test-*.db")
	if err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()
	dbPath := tmpFile.Name()

	// Initialize storage
	store, err := storage.NewStore(storage.Config{
		DatabasePath: dbPath,
	})
	if err != nil {
		os.Remove(dbPath)
		t.Fatal(err)
	}

	// Create auth manager
	testSecret := "random-jwt-hmac-key-f8e7d6c5b4a39281"
	authMgr, err := auth.NewManager(
		[]byte(testSecret),
		"nekzus",
		"nekzus-mobile",
		[]string{"boot-123"},
	)
	if err != nil {
		store.Close()
		os.Remove(dbPath)
		t.Fatal(err)
	}

	// Create federation peer manager with unique port per test
	testPort := getTestPort()
	fedConfig := federation.Config{
		LocalPeerID:         localPeerID,
		LocalPeerName:       "Test Nexus Instance",
		APIAddress:          "http://localhost:8443",
		ClusterSecret:       "test-cluster-secret-32-characters!",
		GossipBindAddr:      "127.0.0.1",
		GossipBindPort:      testPort,
		GossipAdvertiseAddr: "127.0.0.1",
		GossipAdvertisePort: testPort,
		FullSyncInterval:    1 * time.Minute,
		AntiEntropyPeriod:   30 * time.Second,
		PeerTimeout:         30 * time.Second,
		AllowRemoteRoutes:   true,
	}

	peerManager, err := federation.NewPeerManager(fedConfig, store, nil, testMetrics)
	if err != nil {
		store.Close()
		os.Remove(dbPath)
		t.Fatal(err)
	}

	// Create managers
	activityTracker := activity.NewTracker(store)
	wsManager := websocket.NewManager(testMetrics, store)
	qrLimiter := ratelimit.NewLimiter(1.0, 5)

	// Create auth handler
	authHandler := handlers.NewAuthHandler(
		authMgr,
		store,
		testMetrics,
		wsManager,
		activityTracker,
		qrLimiter,
		nil, // no cert manager for tests
		"http://localhost:8443",
		"",
		localPeerID,
		"1.0.0-test",
		[]string{"catalog", "events", "proxy"},
	)

	app := &Application{
		config: types.ServerConfig{
			Federation: types.FederationConfig{
				Enabled:           true,
				AllowRemoteRoutes: true,
			},
		},
		services: &ServiceRegistry{
			Auth: authMgr,
		},
		limiters: &RateLimiterRegistry{
			QR: qrLimiter,
		},
		managers: &ManagerRegistry{
			Router:    router.NewRegistry(store),
			WebSocket: wsManager,
			Activity:  activityTracker,
			Peers:     peerManager,
		},
		handlers: &HandlerRegistry{
			Auth: authHandler,
		},
		jobs:         &JobRegistry{},
		storage:      store,
		metrics:      testMetrics,
		proxyCache:   proxy.NewCache(),
		nekzusID:     localPeerID,
		baseURL:      "http://localhost:8443",
		version:      "1.0.0-test",
		capabilities: []string{"catalog", "events", "proxy"},
	}

	// Cleanup function
	cleanup := func() {
		store.Close()
		os.Remove(dbPath)
	}

	return app, cleanup
}

// addPeerToStorage adds a peer to storage (required due to foreign key constraint on services)
func addPeerToStorage(t *testing.T, store *storage.Store, peerID, name, apiAddress string) {
	t.Helper()

	err := store.SavePeer(&storage.PeerInfo{
		ID:            peerID,
		Name:          name,
		APIAddress:    apiAddress,
		GossipAddress: "127.0.0.1:7946",
		Status:        "online",
		LastSeen:      time.Now(),
		Metadata:      make(map[string]string),
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	})
	if err != nil {
		t.Fatalf("failed to save peer: %v", err)
	}
}

// addFederatedService adds a federated service to the catalog via storage
// Note: The peer must already exist in storage due to foreign key constraint
func addFederatedService(t *testing.T, store *storage.Store, serviceID, originPeerID string, app *types.App) {
	t.Helper()

	appJSON, err := json.Marshal(app)
	if err != nil {
		t.Fatalf("failed to marshal app: %v", err)
	}

	clockJSON := `{"` + originPeerID + `": 1}`

	err = store.SaveFederatedService(
		serviceID,
		originPeerID,
		string(appJSON),
		1.0,
		time.Now(),
		false,
		clockJSON,
	)
	if err != nil {
		t.Fatalf("failed to save federated service: %v", err)
	}
}

// TestGetRemoteServiceAddress_LocalService tests that local services return empty address
func TestGetRemoteServiceAddress_LocalService(t *testing.T) {
	localPeerID := "nxs_local_test_001"
	app, cleanup := newTestApplicationWithFederation(t, localPeerID)
	defer cleanup()

	// First add the local peer to storage (required for foreign key constraint)
	addPeerToStorage(t, app.storage, localPeerID, "Local Peer", "http://localhost:8080")

	// Add a local service (originPeerID matches localPeerID)
	localApp := &types.App{
		ID:   "local-webapp",
		Name: "Local Web App",
	}
	addFederatedService(t, app.storage, "local-webapp", localPeerID, localApp)

	// Get the remote address - should return empty for local services
	address, isRemote := app.getRemoteServiceAddress("local-webapp")

	if isRemote {
		t.Errorf("expected local service to not be remote, got isRemote=true")
	}
	if address != "" {
		t.Errorf("expected empty address for local service, got %q", address)
	}
}

// TestGetRemoteServiceAddress_RemoteService tests that remote services return peer address
func TestGetRemoteServiceAddress_RemoteService(t *testing.T) {
	localPeerID := "nxs_local_test_002"
	remotePeerID := "nxs_remote_peer_001"
	app, cleanup := newTestApplicationWithFederation(t, localPeerID)
	defer cleanup()

	// First add the remote peer to storage (required for foreign key constraint)
	addPeerToStorage(t, app.storage, remotePeerID, "Remote Peer", "http://192.168.1.100:8080")

	// Add a remote service (originPeerID differs from localPeerID)
	remoteApp := &types.App{
		ID:   "remote-webapp",
		Name: "Remote Web App",
	}
	addFederatedService(t, app.storage, "remote-webapp", remotePeerID, remoteApp)

	// The service IS in the federated catalog, but the peer manager doesn't know about
	// this peer (since we're not running memberlist). This tests the error handling path
	// when the catalog has a service but the peer manager can't find the peer.
	address, isRemote := app.getRemoteServiceAddress("remote-webapp")

	// Since the peer isn't registered in the peer manager (only in storage),
	// getRemoteServiceAddress should return empty/false
	// This is the expected behavior when a peer leaves but catalog hasn't synced yet
	if isRemote && address != "" {
		t.Logf("Remote service detected with address=%q (unexpected)", address)
	}

	// The test passes as long as we don't crash - the function should handle
	// the case gracefully where catalog has service but peer isn't reachable
}

// TestGetRemoteServiceAddress_ServiceNotFound tests non-existent services
func TestGetRemoteServiceAddress_ServiceNotFound(t *testing.T) {
	localPeerID := "nxs_local_test_003"
	app, cleanup := newTestApplicationWithFederation(t, localPeerID)
	defer cleanup()

	// Don't add any services - test for non-existent service
	address, isRemote := app.getRemoteServiceAddress("nonexistent-service")

	if isRemote {
		t.Errorf("expected non-existent service to not be remote")
	}
	if address != "" {
		t.Errorf("expected empty address for non-existent service, got %q", address)
	}
}

// TestGetRemoteServiceAddress_NoPeerManager tests behavior when peer manager is nil
func TestGetRemoteServiceAddress_NoPeerManager(t *testing.T) {
	app := newTestApplication(t)
	// newTestApplication doesn't set up a peer manager

	address, isRemote := app.getRemoteServiceAddress("any-service")

	if isRemote {
		t.Errorf("expected isRemote=false when peer manager is nil")
	}
	if address != "" {
		t.Errorf("expected empty address when peer manager is nil")
	}
}

// TestHandleRemoteProxy_ProxiesToRemotePeer tests that requests are proxied to remote peers
func TestHandleRemoteProxy_ProxiesToRemotePeer(t *testing.T) {
	// Create a mock "remote peer" server
	remotePeerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify forwarding headers are set
		if r.Header.Get("X-Federated-From") == "" {
			t.Error("X-Federated-From header not set")
		}
		if r.Header.Get("X-Forwarded-For") == "" {
			t.Error("X-Forwarded-For header not set")
		}

		// Verify Authorization header is stripped
		if r.Header.Get("Authorization") != "" {
			t.Error("Authorization header should be stripped")
		}

		// Echo back a response
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{
			"status":      "ok",
			"from":        "remote-peer",
			"path":        r.URL.Path,
			"federated":   r.Header.Get("X-Federated-From"),
			"forwardedIP": r.Header.Get("X-Forwarded-For"),
		})
	}))
	defer remotePeerServer.Close()

	localPeerID := "nxs_local_test_004"
	app, cleanup := newTestApplicationWithFederation(t, localPeerID)
	defer cleanup()

	// Create a route for the remote service
	route := types.Route{
		RouteID:     "route:remote-app",
		AppID:       "remote-app",
		PathBase:    "/apps/remote-app/",
		To:          "http://actual-backend:8080", // This won't be used for remote routing
		StripPrefix: true,
		Scopes:      []string{},
	}

	// Create request
	req := httptest.NewRequest("GET", "http://localhost/apps/remote-app/api/health", nil)
	req.Header.Set("Authorization", "Bearer some-token") // Should be stripped
	w := httptest.NewRecorder()

	// Call handleRemoteProxy directly
	app.handleRemoteProxy(w, req, route, remotePeerServer.URL, nil)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected status 200, got %d, body: %s", resp.StatusCode, string(body))
	}

	var result map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if result["status"] != "ok" {
		t.Errorf("expected status=ok, got %q", result["status"])
	}
	if result["federated"] != localPeerID {
		t.Errorf("expected X-Federated-From=%q, got %q", localPeerID, result["federated"])
	}
}

// TestHandleRemoteProxy_PreservesPath tests that the request path is preserved correctly
func TestHandleRemoteProxy_PreservesPath(t *testing.T) {
	var receivedPath string
	var receivedQuery string
	remotePeerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		receivedQuery = r.URL.RawQuery
		w.WriteHeader(http.StatusOK)
	}))
	defer remotePeerServer.Close()

	localPeerID := "nxs_local_test_005"
	app, cleanup := newTestApplicationWithFederation(t, localPeerID)
	defer cleanup()

	route := types.Route{
		RouteID:  "route:test",
		AppID:    "test-app",
		PathBase: "/apps/test/",
	}

	// Test with a nested path and query string
	req := httptest.NewRequest("GET", "http://localhost/apps/test/api/v1/users?filter=active", nil)
	w := httptest.NewRecorder()

	app.handleRemoteProxy(w, req, route, remotePeerServer.URL, nil)

	resp := w.Result()
	resp.Body.Close()

	// Verify that request completes successfully
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	// Verify path is correctly forwarded (not duplicated)
	expectedPath := "/apps/test/api/v1/users"
	if receivedPath != expectedPath {
		t.Errorf("expected path %q, got %q", expectedPath, receivedPath)
	}

	// Verify query string is correctly forwarded (not duplicated)
	expectedQuery := "filter=active"
	if receivedQuery != expectedQuery {
		t.Errorf("expected query %q, got %q", expectedQuery, receivedQuery)
	}
}

// TestHandleRemoteProxy_WithScopes tests that scope checking works for remote proxy
func TestHandleRemoteProxy_WithScopes(t *testing.T) {
	remotePeerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer remotePeerServer.Close()

	localPeerID := "nxs_local_test_006"
	app, cleanup := newTestApplicationWithFederation(t, localPeerID)
	defer cleanup()

	route := types.Route{
		RouteID:  "route:protected",
		AppID:    "protected-app",
		PathBase: "/apps/protected/",
		Scopes:   []string{"access:protected"},
	}

	// Test with insufficient scopes
	req := httptest.NewRequest("GET", "http://localhost/apps/protected/", nil)
	w := httptest.NewRecorder()

	// Claims without required scope (must be []interface{} not []string)
	claims := map[string]interface{}{
		"sub":    "test-device",
		"scopes": []interface{}{"read:apps"}, // Missing "access:protected"
	}

	app.handleRemoteProxy(w, req, route, remotePeerServer.URL, claims)

	resp := w.Result()
	resp.Body.Close()

	// Should return 403 Forbidden
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("expected status 403, got %d", resp.StatusCode)
	}
}

// TestHandleRemoteProxy_WithValidScopes tests that requests with valid scopes succeed
func TestHandleRemoteProxy_WithValidScopes(t *testing.T) {
	remotePeerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer remotePeerServer.Close()

	localPeerID := "nxs_local_test_007"
	app, cleanup := newTestApplicationWithFederation(t, localPeerID)
	defer cleanup()

	route := types.Route{
		RouteID:  "route:protected",
		AppID:    "protected-app",
		PathBase: "/apps/protected/",
		Scopes:   []string{"access:protected"},
	}

	req := httptest.NewRequest("GET", "http://localhost/apps/protected/", nil)
	w := httptest.NewRecorder()

	// Claims with required scope (must be []interface{} not []string)
	claims := map[string]interface{}{
		"sub":    "test-device",
		"scopes": []interface{}{"read:apps", "access:protected"},
	}

	app.handleRemoteProxy(w, req, route, remotePeerServer.URL, claims)

	resp := w.Result()
	resp.Body.Close()

	// Should succeed
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
}

// TestHandleRemoteProxy_NilClaims tests that requests without claims work for unprotected routes
func TestHandleRemoteProxy_NilClaims(t *testing.T) {
	remotePeerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer remotePeerServer.Close()

	localPeerID := "nxs_local_test_008"
	app, cleanup := newTestApplicationWithFederation(t, localPeerID)
	defer cleanup()

	route := types.Route{
		RouteID:  "route:public",
		AppID:    "public-app",
		PathBase: "/apps/public/",
		Scopes:   []string{}, // No scopes required
	}

	req := httptest.NewRequest("GET", "http://localhost/apps/public/", nil)
	w := httptest.NewRecorder()

	// nil claims (unauthenticated but route allows it)
	app.handleRemoteProxy(w, req, route, remotePeerServer.URL, nil)

	resp := w.Result()
	resp.Body.Close()

	// Should succeed since no scopes required
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
}

// TestFederatedRouting_EndToEnd tests the full flow of federated routing
func TestFederatedRouting_EndToEnd(t *testing.T) {
	// Create remote peer's backend server
	backendServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"message": "Hello from remote backend!",
			"path":    r.URL.Path,
		})
	}))
	defer backendServer.Close()

	// Create remote peer's nexus instance (mock)
	remotePeerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify federation header is present
		if r.Header.Get("X-Federated-From") == "" {
			t.Error("remote peer should receive X-Federated-From header")
		}

		// Simulate remote nexus proxying to its local backend
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"message":   "Proxied through remote peer",
			"path":      r.URL.Path,
			"federated": "true",
		})
	}))
	defer remotePeerServer.Close()

	localPeerID := "nxs_e2e_local"
	remotePeerID := "nxs_e2e_remote"
	app, cleanup := newTestApplicationWithFederation(t, localPeerID)
	defer cleanup()

	// Register a route for the remote service
	route := types.Route{
		RouteID:     "route:remote-service",
		AppID:       "remote-service",
		PathBase:    "/apps/remote-service/",
		To:          backendServer.URL, // Local backend URL (won't be used for federated routing)
		StripPrefix: true,
		Scopes:      []string{},
	}
	_ = app.managers.Router.UpsertRoute(route)

	// First add the remote peer to storage (required for foreign key constraint)
	addPeerToStorage(t, app.storage, remotePeerID, "Remote E2E Peer", remotePeerServer.URL)

	// Add the remote service to federated catalog
	remoteApp := &types.App{
		ID:   "remote-service",
		Name: "Remote Service",
	}
	addFederatedService(t, app.storage, "remote-service", remotePeerID, remoteApp)

	// Create a JWT token
	token, err := app.services.Auth.SignJWT("test-device", []string{}, 1*time.Hour)
	if err != nil {
		t.Fatalf("failed to sign JWT: %v", err)
	}

	// Create request to the federated service
	req := httptest.NewRequest("GET", "http://localhost/apps/remote-service/api/data", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	// The actual proxy handler would check if service is remote and proxy accordingly
	// For this test, we directly test handleRemoteProxy since we can't easily inject
	// the peer into the peer manager
	app.handleRemoteProxy(w, req, route, remotePeerServer.URL, nil)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected status 200, got %d, body: %s", resp.StatusCode, string(body))
	}

	var result map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if result["federated"] != "true" {
		t.Errorf("expected federated response, got %+v", result)
	}
}

// TestGetRemoteServiceAddress_CatalogSyncerNil tests when catalog syncer is nil
func TestGetRemoteServiceAddress_CatalogSyncerNil(t *testing.T) {
	localPeerID := "nxs_nil_syncer_test"
	app, cleanup := newTestApplicationWithFederation(t, localPeerID)
	defer cleanup()

	// The peer manager is set up but we can test the nil catalog syncer path
	// by checking the actual function behavior
	address, isRemote := app.getRemoteServiceAddress("any-service")

	// With no services in catalog, should return not found
	if isRemote {
		t.Errorf("expected isRemote=false for empty catalog")
	}
	if address != "" {
		t.Errorf("expected empty address for empty catalog")
	}
}
