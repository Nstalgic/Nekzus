package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/nstalgic/nekzus/internal/activity"
	"github.com/nstalgic/nekzus/internal/auth"
	"github.com/nstalgic/nekzus/internal/certmanager"
	"github.com/nstalgic/nekzus/internal/handlers"
	"github.com/nstalgic/nekzus/internal/proxy"
	"github.com/nstalgic/nekzus/internal/ratelimit"
	"github.com/nstalgic/nekzus/internal/router"
	"github.com/nstalgic/nekzus/internal/storage"
	"github.com/nstalgic/nekzus/internal/types"
	"github.com/nstalgic/nekzus/internal/websocket"
)

// newTestAppWithCertManager creates a test app with cert manager
func newTestAppWithCertManager(t *testing.T) (*Application, func()) {
	t.Helper()

	// Create temporary database
	tmpFile, err := os.CreateTemp("", "nekzus-test-*.db")
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

	// Create cert manager
	certMgr := certmanager.New(
		certmanager.Config{
			DefaultProvider: "self-signed",
		},
		certmanager.NewStorageAdapter(store),
		nil, // metrics
	)
	certMgr.RegisterProvider(certmanager.NewSelfSignedProvider())

	app := &Application{
		config: types.ServerConfig{},
		services: &ServiceRegistry{
			Auth:  authMgr,
			Certs: certMgr,
		},
		limiters: &RateLimiterRegistry{
			QR: ratelimit.NewLimiter(1.0, 5),
		},
		managers: &ManagerRegistry{
			Router:    router.NewRegistry(store),
			WebSocket: websocket.NewManager(testMetrics, store),
			Activity:  activity.NewTracker(store),
		},
		jobs:         &JobRegistry{}, // Empty jobs registry for tests
		storage:      store,
		metrics:      testMetrics,
		proxyCache:   proxy.NewCache(),
		nekzusID:     "test-nekzus",
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

// generateTestToken generates a JWT token for testing
func generateTestToken(t *testing.T, app *Application, deviceID string) string {
	t.Helper()
	token, err := app.services.Auth.SignJWT(deviceID, []string{"read", "write"}, 1*time.Hour)
	if err != nil {
		t.Fatalf("Failed to generate test token: %v", err)
	}
	return token
}

func TestCertificates_Generate_Unauthorized(t *testing.T) {
	app, cleanup := newTestAppWithCertManager(t)
	defer cleanup()

	mux := http.NewServeMux()
	mux.HandleFunc("/api/certificates/generate", app.handleGenerateCertificate)

	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Try to generate certificate without auth
	body := `{"domains":["test.local"]}`
	req, _ := http.NewRequest("POST", srv.URL+"/api/certificates/generate", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusUnauthorized {
		t.Errorf("Expected 401 Unauthorized, got: %d", res.StatusCode)
	}
}

func TestCertificates_Generate_Success(t *testing.T) {
	app, cleanup := newTestAppWithCertManager(t)
	defer cleanup()

	// Generate JWT token
	token := generateTestToken(t, app, "test-device-1")

	mux := http.NewServeMux()
	mux.HandleFunc("/api/certificates/generate", app.handleGenerateCertificate)

	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Generate certificate with auth
	body := `{"domains":["test.local","app.local"],"provider":"self-signed"}`
	req, _ := http.NewRequest("POST", srv.URL+"/api/certificates/generate", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Errorf("Expected 200 OK, got: %d", res.StatusCode)
	}

	// Parse response
	var response map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}

	// Check response structure
	if response["success"] != true {
		t.Error("Expected success: true")
	}

	cert, ok := response["certificate"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected certificate object in response")
	}

	if cert["domain"] != "test.local" {
		t.Errorf("Expected domain test.local, got: %v", cert["domain"])
	}

	if cert["issuer"] != "self-signed" {
		t.Errorf("Expected issuer self-signed, got: %v", cert["issuer"])
	}

	// Verify SANs include both domains
	sans, ok := cert["sans"].([]interface{})
	if !ok || len(sans) != 2 {
		t.Errorf("Expected 2 SANs, got: %v", sans)
	}
}

func TestCertificates_Generate_InvalidDomain(t *testing.T) {
	app, cleanup := newTestAppWithCertManager(t)
	defer cleanup()

	token := generateTestToken(t, app, "test-device-1")

	mux := http.NewServeMux()
	mux.HandleFunc("/api/certificates/generate", app.handleGenerateCertificate)

	srv := httptest.NewServer(mux)
	defer srv.Close()

	invalidDomains := []string{
		`{"domains":["../etc/passwd"]}`, // Path traversal
		`{"domains":["google.com"]}`,    // Public domain (not allowed by default)
		`{"domains":[""]}`,              // Empty domain
		`{"domains":[]}`,                // No domains
	}

	for _, body := range invalidDomains {
		req, _ := http.NewRequest("POST", srv.URL+"/api/certificates/generate", bytes.NewBufferString(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")

		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()

		if res.StatusCode == http.StatusOK {
			t.Errorf("Expected error for invalid domain: %s", body)
		}
	}
}

func TestCertificates_List(t *testing.T) {
	app, cleanup := newTestAppWithCertManager(t)
	defer cleanup()

	token := generateTestToken(t, app, "test-device-1")

	// Generate some certificates first
	_, err := app.services.Certs.Generate(certmanager.GenerateRequest{
		Domains: []string{"app1.local"},
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = app.services.Certs.Generate(certmanager.GenerateRequest{
		Domains: []string{"app2.local"},
	})
	if err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/certificates", app.handleListCertificates)

	srv := httptest.NewServer(mux)
	defer srv.Close()

	// List certificates
	req, _ := http.NewRequest("GET", srv.URL+"/api/certificates", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Errorf("Expected 200 OK, got: %d", res.StatusCode)
	}

	// Parse response
	var response map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}

	certs, ok := response["certificates"].([]interface{})
	if !ok {
		t.Fatal("Expected certificates array in response")
	}

	if len(certs) != 2 {
		t.Errorf("Expected 2 certificates, got: %d", len(certs))
	}
}

func TestCertificates_Get(t *testing.T) {
	app, cleanup := newTestAppWithCertManager(t)
	defer cleanup()

	token := generateTestToken(t, app, "test-device-1")

	// Generate a certificate
	_, err := app.services.Certs.Generate(certmanager.GenerateRequest{
		Domains: []string{"test.local"},
	})
	if err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/certificates/", app.handleGetCertificate)

	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Get certificate
	req, _ := http.NewRequest("GET", srv.URL+"/api/certificates/test.local", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Errorf("Expected 200 OK, got: %d", res.StatusCode)
	}

	// Parse response
	var response map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}

	cert, ok := response["certificate"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected certificate object in response")
	}

	if cert["domain"] != "test.local" {
		t.Errorf("Expected domain test.local, got: %v", cert["domain"])
	}
}

func TestCertificates_Get_NotFound(t *testing.T) {
	app, cleanup := newTestAppWithCertManager(t)
	defer cleanup()

	token := generateTestToken(t, app, "test-device-1")

	mux := http.NewServeMux()
	mux.HandleFunc("/api/certificates/", app.handleGetCertificate)

	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Try to get non-existent certificate
	req, _ := http.NewRequest("GET", srv.URL+"/api/certificates/nonexistent.local", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusNotFound {
		t.Errorf("Expected 404 Not Found, got: %d", res.StatusCode)
	}
}

func TestCertificates_Delete(t *testing.T) {
	app, cleanup := newTestAppWithCertManager(t)
	defer cleanup()

	token := generateTestToken(t, app, "test-device-1")

	// Generate two certificates (need at least 2 to allow deletion)
	_, err := app.services.Certs.Generate(certmanager.GenerateRequest{
		Domains: []string{"keep.local"},
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = app.services.Certs.Generate(certmanager.GenerateRequest{
		Domains: []string{"to-delete.local"},
	})
	if err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/certificates/", app.handleDeleteCertificate)

	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Delete certificate (not the active one)
	req, _ := http.NewRequest("DELETE", srv.URL+"/api/certificates/to-delete.local", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		t.Errorf("Expected 200 OK, got: %d, body: %s", res.StatusCode, body)
	}

	// Verify certificate is deleted
	_, err = app.services.Certs.Get("to-delete.local")
	if err == nil {
		t.Error("Expected certificate to be deleted")
	}

	// Verify the other certificate still exists
	_, err = app.services.Certs.Get("keep.local")
	if err != nil {
		t.Error("Expected keep.local certificate to still exist")
	}
}

func TestCertificates_Delete_Unauthorized(t *testing.T) {
	app, cleanup := newTestAppWithCertManager(t)
	defer cleanup()

	// Generate a certificate
	_, err := app.services.Certs.Generate(certmanager.GenerateRequest{
		Domains: []string{"protected.local"},
	})
	if err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/certificates/", app.handleDeleteCertificate)

	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Try to delete without auth
	req, _ := http.NewRequest("DELETE", srv.URL+"/api/certificates/protected.local", nil)

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusUnauthorized {
		t.Errorf("Expected 401 Unauthorized, got: %d", res.StatusCode)
	}

	// Verify certificate still exists
	_, err = app.services.Certs.Get("protected.local")
	if err != nil {
		t.Error("Expected certificate to still exist")
	}
}

func TestCertificates_PersistenceAcrossRestart(t *testing.T) {
	// Create temporary database that persists
	tmpFile, err := os.CreateTemp("", "nekzus-test-*.db")
	if err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()
	dbPath := tmpFile.Name()
	defer os.Remove(dbPath)

	// First app instance - generate certificate
	{
		store, err := storage.NewStore(storage.Config{DatabasePath: dbPath})
		if err != nil {
			t.Fatal(err)
		}

		certMgr := certmanager.New(
			certmanager.Config{DefaultProvider: "self-signed"},
			certmanager.NewStorageAdapter(store),
			nil,
		)
		certMgr.RegisterProvider(certmanager.NewSelfSignedProvider())

		_, err = certMgr.Generate(certmanager.GenerateRequest{
			Domains: []string{"persistent.local"},
		})
		if err != nil {
			t.Fatal(err)
		}

		store.Close()
	}

	// Second app instance - load from storage
	{
		store, err := storage.NewStore(storage.Config{DatabasePath: dbPath})
		if err != nil {
			t.Fatal(err)
		}
		defer store.Close()

		certMgr := certmanager.New(
			certmanager.Config{DefaultProvider: "self-signed"},
			certmanager.NewStorageAdapter(store),
			nil,
		)
		certMgr.RegisterProvider(certmanager.NewSelfSignedProvider())

		// Load certificates from storage
		if err := certMgr.LoadFromStorage(); err != nil {
			t.Fatal(err)
		}

		// Certificate should be available
		cert, err := certMgr.Get("persistent.local")
		if err != nil {
			t.Fatalf("Certificate not restored from storage: %v", err)
		}

		if cert.Domain != "persistent.local" {
			t.Errorf("Expected domain persistent.local, got: %s", cert.Domain)
		}
	}
}

func TestCertificates_Suggest(t *testing.T) {
	app, cleanup := newTestAppWithCertManager(t)
	defer cleanup()

	token := generateTestToken(t, app, "test-device-1")

	mux := http.NewServeMux()
	mux.HandleFunc("/api/certificates/suggest", app.handleSuggestCertDomains)

	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Request suggested domains
	req, _ := http.NewRequest("GET", srv.URL+"/api/certificates/suggest", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Errorf("Expected 200 OK, got: %d", res.StatusCode)
	}

	// Parse response
	var response map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}

	// Should have suggestions array
	suggestions, ok := response["suggestions"].([]interface{})
	if !ok {
		t.Fatal("Expected suggestions array in response")
	}

	// Should contain at least localhost
	if len(suggestions) == 0 {
		t.Error("Expected at least one suggestion")
	}

	// First suggestion should be localhost
	if suggestions[0].(string) != "localhost" {
		t.Errorf("Expected first suggestion to be 'localhost', got: %s", suggestions[0])
	}

	// Should have count
	count, ok := response["count"].(float64)
	if !ok {
		t.Fatal("Expected count in response")
	}

	if int(count) != len(suggestions) {
		t.Errorf("Count mismatch: got %d, suggestions length: %d", int(count), len(suggestions))
	}
}

func TestCertificates_Suggest_Unauthorized(t *testing.T) {
	app, cleanup := newTestAppWithCertManager(t)
	defer cleanup()

	mux := http.NewServeMux()
	mux.HandleFunc("/api/certificates/suggest", app.handleSuggestCertDomains)

	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Request without token
	req, _ := http.NewRequest("GET", srv.URL+"/api/certificates/suggest", nil)

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusUnauthorized {
		t.Errorf("Expected 401 Unauthorized, got: %d", res.StatusCode)
	}
}

func TestTLS_IsTLSEnabled_Default(t *testing.T) {
	app, cleanup := newTestAppWithCertManager(t)
	defer cleanup()

	// TLS should be disabled by default
	if app.IsTLSEnabled() {
		t.Error("Expected TLS to be disabled by default")
	}
}

func TestTLS_UpgradeToTLS_NoCertificate(t *testing.T) {
	app, cleanup := newTestAppWithCertManager(t)
	defer cleanup()

	// Initialize channels for upgrade mechanism
	app.tlsUpgrade = make(chan struct{}, 1)

	// Upgrade should fail without certificates
	err := app.UpgradeToTLS()
	if err == nil {
		t.Error("Expected error when upgrading without certificates")
	}
}

func TestTLS_UpgradeToTLS_WithCertificate(t *testing.T) {
	app, cleanup := newTestAppWithCertManager(t)
	defer cleanup()

	// Initialize channels for upgrade mechanism
	app.tlsUpgrade = make(chan struct{}, 1)

	// Generate a certificate
	_, err := app.services.Certs.Generate(certmanager.GenerateRequest{
		Domains: []string{"localhost"},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Upgrade should succeed with certificate
	err = app.UpgradeToTLS()
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}

	// Check that upgrade signal was sent
	select {
	case <-app.tlsUpgrade:
		// Signal received as expected
	default:
		t.Error("Expected upgrade signal to be sent")
	}
}

func TestTLS_UpgradeToTLS_AlreadyEnabled(t *testing.T) {
	app, cleanup := newTestAppWithCertManager(t)
	defer cleanup()

	// Initialize channels for upgrade mechanism
	app.tlsUpgrade = make(chan struct{}, 1)

	// Mark TLS as already enabled
	app.tlsEnabled.Store(true)

	// Upgrade should be a no-op when already enabled
	err := app.UpgradeToTLS()
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}

	// Check that no upgrade signal was sent
	select {
	case <-app.tlsUpgrade:
		t.Error("Expected no upgrade signal when TLS already enabled")
	default:
		// No signal as expected
	}
}

func TestTLS_Generate_TriggersUpgrade(t *testing.T) {
	app, cleanup := newTestAppWithCertManager(t)
	defer cleanup()

	// Initialize channels for upgrade mechanism
	app.tlsUpgrade = make(chan struct{}, 1)

	token := generateTestToken(t, app, "test-device-1")

	mux := http.NewServeMux()
	mux.HandleFunc("/api/certificates/generate", app.handleGenerateCertificate)

	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Generate certificate with auth
	body := `{"domains":["localhost"],"provider":"self-signed"}`
	req, _ := http.NewRequest("POST", srv.URL+"/api/certificates/generate", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Errorf("Expected 200 OK, got: %d", res.StatusCode)
	}

	// Parse response
	var response map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}

	// Check tls_upgraded field
	tlsUpgraded, ok := response["tls_upgraded"].(bool)
	if !ok {
		t.Fatal("Expected tls_upgraded field in response")
	}

	if !tlsUpgraded {
		t.Error("Expected tls_upgraded to be true")
	}

	// Check that upgrade signal was sent
	select {
	case <-app.tlsUpgrade:
		// Signal received as expected
	default:
		t.Error("Expected upgrade signal to be sent after certificate generation")
	}
}

func TestTLS_Generate_NoUpgradeWhenTLSEnabled(t *testing.T) {
	app, cleanup := newTestAppWithCertManager(t)
	defer cleanup()

	// Initialize channels for upgrade mechanism
	app.tlsUpgrade = make(chan struct{}, 1)

	// Mark TLS as already enabled
	app.tlsEnabled.Store(true)

	token := generateTestToken(t, app, "test-device-1")

	mux := http.NewServeMux()
	mux.HandleFunc("/api/certificates/generate", app.handleGenerateCertificate)

	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Generate certificate with auth
	body := `{"domains":["localhost"],"provider":"self-signed"}`
	req, _ := http.NewRequest("POST", srv.URL+"/api/certificates/generate", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Errorf("Expected 200 OK, got: %d", res.StatusCode)
	}

	// Parse response
	var response map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}

	// Check tls_upgraded field should be false
	tlsUpgraded, ok := response["tls_upgraded"].(bool)
	if !ok {
		t.Fatal("Expected tls_upgraded field in response")
	}

	if tlsUpgraded {
		t.Error("Expected tls_upgraded to be false when TLS already enabled")
	}

	// Check that no upgrade signal was sent
	select {
	case <-app.tlsUpgrade:
		t.Error("Expected no upgrade signal when TLS already enabled")
	default:
		// No signal as expected
	}
}

// TestQRCode_IncludesSPKIAfterCertGeneration verifies that the QR code includes
// the correct SPKI after a certificate is generated dynamically.
// This tests the flow: start insecure -> generate cert -> QR code has SPKI
func TestQRCode_IncludesSPKIAfterCertGeneration(t *testing.T) {
	app, cleanup := newTestAppWithCertManager(t)
	defer cleanup()
	app.tlsUpgrade = make(chan struct{}, 1)

	// Create auth handler with cert manager
	authHandler := handlers.NewAuthHandler(
		app.services.Auth,
		app.storage,
		app.metrics,
		app.managers.WebSocket,
		app.managers.Activity,
		app.limiters.QR,
		app.services.Certs, // cert manager for SPKI calculation
		app.baseURL,
		"", // no file path
		app.nekzusID,
		app.version,
		app.capabilities,
	)
	app.handlers = &HandlerRegistry{
		Auth: authHandler,
	}

	// Step 1: Check QR code before cert generation - should have empty SPKI
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/qr", app.handleQRCode)
	mux.HandleFunc("/api/certificates/generate", app.handleGenerateCertificate)

	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Get QR code before cert generation
	req1, _ := http.NewRequest("GET", srv.URL+"/api/v1/auth/qr", nil)
	res1, err := http.DefaultClient.Do(req1)
	if err != nil {
		t.Fatal(err)
	}
	defer res1.Body.Close()

	var qrBefore map[string]interface{}
	if err := json.NewDecoder(res1.Body).Decode(&qrBefore); err != nil {
		t.Fatalf("Failed to decode QR response: %v", err)
	}

	spkiBefore, _ := qrBefore["spki"].(string)
	if spkiBefore != "" {
		t.Errorf("Expected empty SPKI before cert generation, got: %s", spkiBefore)
	}

	// Step 2: Generate a certificate
	token := generateTestToken(t, app, "test-device")
	certReq := `{"domains":["localhost"]}`
	req2, _ := http.NewRequest("POST", srv.URL+"/api/certificates/generate", bytes.NewBufferString(certReq))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Authorization", "Bearer "+token)

	res2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	defer res2.Body.Close()

	if res2.StatusCode != http.StatusOK {
		t.Fatalf("Certificate generation failed: %d", res2.StatusCode)
	}

	var certResponse map[string]interface{}
	if err := json.NewDecoder(res2.Body).Decode(&certResponse); err != nil {
		t.Fatalf("Failed to decode cert response: %v", err)
	}

	// Get the generated cert fingerprint
	certInfo, _ := certResponse["certificate"].(map[string]interface{})
	expectedSPKI, _ := certInfo["fingerprint"].(string)
	if expectedSPKI == "" {
		t.Fatal("Certificate response missing fingerprint")
	}

	// Step 3: Check QR code after cert generation - should have SPKI
	req3, _ := http.NewRequest("GET", srv.URL+"/api/v1/auth/qr", nil)
	res3, err := http.DefaultClient.Do(req3)
	if err != nil {
		t.Fatal(err)
	}
	defer res3.Body.Close()

	var qrAfter map[string]interface{}
	if err := json.NewDecoder(res3.Body).Decode(&qrAfter); err != nil {
		t.Fatalf("Failed to decode QR response: %v", err)
	}

	spkiAfter, _ := qrAfter["spki"].(string)
	if spkiAfter == "" {
		t.Error("Expected SPKI after cert generation, got empty string")
	}

	// SPKI should match the generated cert's fingerprint
	if spkiAfter != expectedSPKI {
		t.Errorf("SPKI mismatch:\nQR SPKI:   %s\nCert SPKI: %s", spkiAfter, expectedSPKI)
	}

	t.Logf("Successfully verified QR code includes SPKI after cert generation: %s", spkiAfter[:30]+"...")
}

func TestTLS_UpdateBaseURL_ChangesScheme(t *testing.T) {
	app, cleanup := newTestAppWithCertManager(t)
	defer cleanup()

	// Set initial HTTP base URL
	app.baseURL = "http://192.168.1.100:8080"

	// Verify initial state
	if app.baseURL != "http://192.168.1.100:8080" {
		t.Fatalf("Expected initial baseURL to be http://192.168.1.100:8080, got %s", app.baseURL)
	}

	// Call UpdateBaseURL
	app.UpdateBaseURL()

	// Verify URL was updated to HTTPS
	expected := "https://192.168.1.100:8080"
	if app.baseURL != expected {
		t.Errorf("Expected baseURL to be %s after UpdateBaseURL, got %s", expected, app.baseURL)
	}

	// Calling UpdateBaseURL again should be a no-op (already https)
	app.UpdateBaseURL()
	if app.baseURL != expected {
		t.Errorf("Expected baseURL to remain %s after second UpdateBaseURL, got %s", expected, app.baseURL)
	}
}

func TestTLS_UpdateBaseURL_PropagesToAuthHandler(t *testing.T) {
	app, cleanup := newTestAppWithCertManager(t)
	defer cleanup()

	// Set initial HTTP base URL
	app.baseURL = "http://192.168.1.100:8080"

	// Initialize handlers registry if nil
	if app.handlers == nil {
		app.handlers = &HandlerRegistry{}
	}

	// Initialize auth handler with http URL
	app.handlers.Auth = handlers.NewAuthHandler(
		app.services.Auth,
		app.storage,
		app.metrics,
		nil, // events
		nil, // activity
		nil, // qrLimiter
		app.services.Certs,
		app.baseURL,
		"", // tlsCertPath
		"test-nexus-id",
		"1.0.0",
		[]string{},
	)

	// Call UpdateBaseURL
	app.UpdateBaseURL()

	// The AuthHandler's baseURL should also be updated
	// We can't directly access the private field, but we verify the app.baseURL changed
	expected := "https://192.168.1.100:8080"
	if app.baseURL != expected {
		t.Errorf("Expected app.baseURL to be %s, got %s", expected, app.baseURL)
	}
}

func TestCertificates_Renew(t *testing.T) {
	app, cleanup := newTestAppWithCertManager(t)
	defer cleanup()

	token := generateTestToken(t, app, "test-device-1")

	// First generate a certificate to renew
	_, err := app.services.Certs.Generate(certmanager.GenerateRequest{
		Domains:     []string{"renew-test.local"},
		RequestedBy: "test",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Get the original certificate
	origCert, err := app.services.Certs.Get("renew-test.local")
	if err != nil {
		t.Fatal(err)
	}
	origSPKI := origCert.Metadata.Fingerprint

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/certificates/renew", app.handleRenewCertificate)

	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Renew certificate
	reqBody := `{"domain": "renew-test.local"}`
	req, _ := http.NewRequest("POST", srv.URL+"/api/v1/certificates/renew", strings.NewReader(reqBody))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		t.Fatalf("Expected 200 OK, got: %d, body: %s", res.StatusCode, body)
	}

	var response map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}

	// Verify response structure
	if response["success"] != true {
		t.Error("Expected success to be true")
	}

	rotation, ok := response["rotation"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected rotation object in response")
	}

	// SPKI should have changed (new key generated)
	if rotation["spki_changed"] != true {
		t.Error("Expected spki_changed to be true for renewal")
	}

	// Old SPKI should match original
	if rotation["old_spki"] != origSPKI {
		t.Errorf("Expected old_spki to be %s, got %s", origSPKI, rotation["old_spki"])
	}

	// New SPKI should be different
	if rotation["new_spki"] == origSPKI {
		t.Error("Expected new_spki to be different from old_spki")
	}

	t.Logf("Certificate renewed successfully, SPKI changed from %s... to %s...",
		origSPKI[:20], rotation["new_spki"].(string)[:20])
}

func TestCertificates_Renew_NotFound(t *testing.T) {
	app, cleanup := newTestAppWithCertManager(t)
	defer cleanup()

	token := generateTestToken(t, app, "test-device-1")

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/certificates/renew", app.handleRenewCertificate)

	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Try to renew non-existent certificate
	reqBody := `{"domain": "nonexistent.local"}`
	req, _ := http.NewRequest("POST", srv.URL+"/api/v1/certificates/renew", strings.NewReader(reqBody))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected 400 Bad Request, got: %d", res.StatusCode)
	}
}

func TestCertificates_Replace(t *testing.T) {
	app, cleanup := newTestAppWithCertManager(t)
	defer cleanup()

	token := generateTestToken(t, app, "test-device-1")

	// First generate a certificate to replace
	_, err := app.services.Certs.Generate(certmanager.GenerateRequest{
		Domains:     []string{"replace-test.local"},
		RequestedBy: "test",
	})
	if err != nil {
		t.Fatal(err)
	}

	origCert, _ := app.services.Certs.Get("replace-test.local")
	origSPKI := origCert.Metadata.Fingerprint

	// Generate a new certificate to use as replacement
	newCert, err := app.services.Certs.Generate(certmanager.GenerateRequest{
		Domains:     []string{"new-replace-test.local"},
		RequestedBy: "test",
	})
	if err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/certificates/replace", app.handleReplaceCertificate)

	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Replace certificate
	reqBody := map[string]string{
		"domain":      "replace-test.local",
		"certificate": string(newCert.PEM.Certificate),
		"privateKey":  string(newCert.PEM.PrivateKey),
	}
	reqBytes, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", srv.URL+"/api/v1/certificates/replace", bytes.NewReader(reqBytes))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		t.Fatalf("Expected 200 OK, got: %d, body: %s", res.StatusCode, body)
	}

	var response map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}

	// Verify response
	if response["success"] != true {
		t.Error("Expected success to be true")
	}

	rotation, ok := response["rotation"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected rotation object in response")
	}

	// SPKI should have changed
	if rotation["spki_changed"] != true {
		t.Error("Expected spki_changed to be true for replacement")
	}

	// Old SPKI should match original
	if rotation["old_spki"] != origSPKI {
		t.Errorf("Expected old_spki to be %s, got %s", origSPKI, rotation["old_spki"])
	}

	t.Logf("Certificate replaced successfully")
}

func TestCertificates_Replace_ExpiredCert(t *testing.T) {
	app, cleanup := newTestAppWithCertManager(t)
	defer cleanup()

	token := generateTestToken(t, app, "test-device-1")

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/certificates/replace", app.handleReplaceCertificate)

	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Try to replace with an expired certificate (we'll use invalid PEM to simulate)
	reqBody := map[string]string{
		"domain":      "test.local",
		"certificate": "-----BEGIN CERTIFICATE-----\ninvalid\n-----END CERTIFICATE-----",
		"privateKey":  "-----BEGIN PRIVATE KEY-----\ninvalid\n-----END PRIVATE KEY-----",
	}
	reqBytes, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", srv.URL+"/api/v1/certificates/replace", bytes.NewReader(reqBytes))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	// Should fail with invalid certificate
	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected 400 Bad Request for invalid cert, got: %d", res.StatusCode)
	}
}

func TestCertificates_Delete_BlocksActiveCert(t *testing.T) {
	app, cleanup := newTestAppWithCertManager(t)
	defer cleanup()

	token := generateTestToken(t, app, "test-device-1")

	// Generate a single certificate
	_, err := app.services.Certs.Generate(certmanager.GenerateRequest{
		Domains:     []string{"only-cert.local"},
		RequestedBy: "test",
	})
	if err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/certificates/", app.handleDeleteCertificate)

	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Try to delete the only certificate
	req, _ := http.NewRequest("DELETE", srv.URL+"/api/certificates/only-cert.local", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	// Should be blocked with 409 Conflict
	if res.StatusCode != http.StatusConflict {
		body, _ := io.ReadAll(res.Body)
		t.Errorf("Expected 409 Conflict when deleting only active cert, got: %d, body: %s", res.StatusCode, body)
	}

	// Verify the certificate still exists
	_, err = app.services.Certs.Get("only-cert.local")
	if err != nil {
		t.Error("Certificate should still exist after blocked delete")
	}
}

func TestCertificates_Delete_AllowsWithMultipleCerts(t *testing.T) {
	app, cleanup := newTestAppWithCertManager(t)
	defer cleanup()

	token := generateTestToken(t, app, "test-device-1")

	// Generate two certificates
	_, err := app.services.Certs.Generate(certmanager.GenerateRequest{
		Domains:     []string{"cert1.local"},
		RequestedBy: "test",
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = app.services.Certs.Generate(certmanager.GenerateRequest{
		Domains:     []string{"cert2.local"},
		RequestedBy: "test",
	})
	if err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/certificates/", app.handleDeleteCertificate)

	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Should be able to delete one when we have two
	req, _ := http.NewRequest("DELETE", srv.URL+"/api/certificates/cert2.local", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		t.Errorf("Expected 200 OK when deleting non-active cert, got: %d, body: %s", res.StatusCode, body)
	}

	// Verify the certificate was deleted
	_, err = app.services.Certs.Get("cert2.local")
	if err == nil {
		t.Error("Certificate should have been deleted")
	}

	// Verify the other certificate still exists
	_, err = app.services.Certs.Get("cert1.local")
	if err != nil {
		t.Error("First certificate should still exist")
	}
}
