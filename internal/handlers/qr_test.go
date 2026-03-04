package handlers

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/nstalgic/nekzus/internal/auth"
	"github.com/nstalgic/nekzus/internal/ratelimit"
)

// mockCertManager implements CertificateManager for testing
type mockCertManager struct {
	firstAvailableCert *tls.Certificate
	fallbackCert       *tls.Certificate
}

func (m *mockCertManager) GetFirstAvailableCertificate() *tls.Certificate {
	return m.firstAvailableCert
}

func (m *mockCertManager) GetFallbackCertificate() *tls.Certificate {
	return m.fallbackCert
}

// generateTestCert creates a self-signed certificate for testing
func generateTestCert(t *testing.T) (*tls.Certificate, string) {
	t.Helper()

	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate private key: %v", err)
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: "test.local",
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		t.Fatalf("Failed to create certificate: %v", err)
	}

	cert := &tls.Certificate{
		Certificate: [][]byte{certDER},
		PrivateKey:  privateKey,
	}

	// Calculate expected SPKI
	x509Cert, _ := x509.ParseCertificate(certDER)
	spkiDER, _ := x509.MarshalPKIXPublicKey(x509Cert.PublicKey)
	hash := sha256.Sum256(spkiDER)
	expectedSPKI := "sha256/" + base64.StdEncoding.EncodeToString(hash[:])

	return cert, expectedSPKI
}

func TestCalculateSPKI_ManagedCert(t *testing.T) {
	// Setup: Mock certManager with a valid certificate
	cert, expectedSPKI := generateTestCert(t)

	mockCM := &mockCertManager{
		firstAvailableCert: cert,
	}

	// Create auth handler with no file path but with cert manager
	authMgr, err := auth.NewManager(
		[]byte("random-jwt-hmac-key-f8e7d6c5b4a39281-only"),
		"test-issuer",
		"test-audience",
		[]string{},
	)
	if err != nil {
		t.Fatalf("Failed to create auth manager: %v", err)
	}

	handler := NewAuthHandler(
		authMgr,
		nil, // storage
		nil, // metrics
		nil, // events
		nil, // activity
		ratelimit.NewLimiter(1.0, 5),
		mockCM, // certManager
		"https://test:8443",
		"", // empty tlsCertPath - no file
		"test-nexus-id",
		"1.0.0",
		[]string{"test"},
	)

	// Call calculateSPKI
	spki, err := handler.calculateSPKI()
	if err != nil {
		t.Fatalf("calculateSPKI returned error: %v", err)
	}

	// Assert: SPKI should match expected
	if spki != expectedSPKI {
		t.Errorf("SPKI mismatch:\ngot:  %s\nwant: %s", spki, expectedSPKI)
	}
}

func TestCalculateSPKI_FallbackCert(t *testing.T) {
	// Setup: Mock certManager with only fallback certificate
	cert, expectedSPKI := generateTestCert(t)

	mockCM := &mockCertManager{
		firstAvailableCert: nil,
		fallbackCert:       cert,
	}

	authMgr, _ := auth.NewManager(
		[]byte("random-jwt-hmac-key-f8e7d6c5b4a39281-only"),
		"test-issuer",
		"test-audience",
		[]string{},
	)

	handler := NewAuthHandler(
		authMgr,
		nil, nil, nil, nil,
		ratelimit.NewLimiter(1.0, 5),
		mockCM,
		"https://test:8443",
		"",
		"test-nexus-id",
		"1.0.0",
		[]string{"test"},
	)

	spki, err := handler.calculateSPKI()
	if err != nil {
		t.Fatalf("calculateSPKI returned error: %v", err)
	}

	if spki != expectedSPKI {
		t.Errorf("SPKI mismatch:\ngot:  %s\nwant: %s", spki, expectedSPKI)
	}
}

func TestCalculateSPKI_NoCerts(t *testing.T) {
	// Setup: No certs available
	mockCM := &mockCertManager{
		firstAvailableCert: nil,
		fallbackCert:       nil,
	}

	authMgr, _ := auth.NewManager(
		[]byte("random-jwt-hmac-key-f8e7d6c5b4a39281-only"),
		"test-issuer",
		"test-audience",
		[]string{},
	)

	handler := NewAuthHandler(
		authMgr,
		nil, nil, nil, nil,
		ratelimit.NewLimiter(1.0, 5),
		mockCM,
		"https://test:8443",
		"",
		"test-nexus-id",
		"1.0.0",
		[]string{"test"},
	)

	spki, err := handler.calculateSPKI()
	if err != nil {
		t.Fatalf("calculateSPKI returned error: %v", err)
	}

	// Should return empty string when no certs
	if spki != "" {
		t.Errorf("Expected empty SPKI, got: %s", spki)
	}
}

func TestCalculateSPKI_NilCertManager(t *testing.T) {
	// Setup: nil cert manager and no file path
	authMgr, _ := auth.NewManager(
		[]byte("random-jwt-hmac-key-f8e7d6c5b4a39281-only"),
		"test-issuer",
		"test-audience",
		[]string{},
	)

	handler := NewAuthHandler(
		authMgr,
		nil, nil, nil, nil,
		ratelimit.NewLimiter(1.0, 5),
		nil, // nil certManager
		"https://test:8443",
		"",
		"test-nexus-id",
		"1.0.0",
		[]string{"test"},
	)

	spki, err := handler.calculateSPKI()
	if err != nil {
		t.Fatalf("calculateSPKI returned error: %v", err)
	}

	if spki != "" {
		t.Errorf("Expected empty SPKI with nil cert manager, got: %s", spki)
	}
}

func TestHandleQRCode_IncludesSPKIFromManagedCert(t *testing.T) {
	// Setup: Mock certManager with a valid certificate
	cert, expectedSPKI := generateTestCert(t)

	mockCM := &mockCertManager{
		firstAvailableCert: cert,
	}

	authMgr, _ := auth.NewManager(
		[]byte("random-jwt-hmac-key-f8e7d6c5b4a39281-only"),
		"test-issuer",
		"test-audience",
		[]string{},
	)

	handler := NewAuthHandler(
		authMgr,
		nil, nil, nil, nil,
		ratelimit.NewLimiter(1.0, 5),
		mockCM,
		"https://test:8443",
		"", // no file path
		"test-nexus-id",
		"1.0.0",
		[]string{"catalog"},
	)

	// Create request
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/qr", nil)
	w := httptest.NewRecorder()

	// Call handler
	handler.HandleQRCode(w, req)

	// Check status
	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	// Parse response
	var response map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// Assert SPKI is present and correct
	spki, ok := response["spki"].(string)
	if !ok {
		t.Fatal("Response missing 'spki' field")
	}

	if spki != expectedSPKI {
		t.Errorf("SPKI mismatch:\ngot:  %s\nwant: %s", spki, expectedSPKI)
	}
}

func TestHandleQRCode_EmptySPKIWhenNoCerts(t *testing.T) {
	mockCM := &mockCertManager{
		firstAvailableCert: nil,
		fallbackCert:       nil,
	}

	authMgr, _ := auth.NewManager(
		[]byte("random-jwt-hmac-key-f8e7d6c5b4a39281-only"),
		"test-issuer",
		"test-audience",
		[]string{},
	)

	handler := NewAuthHandler(
		authMgr,
		nil, nil, nil, nil,
		ratelimit.NewLimiter(1.0, 5),
		mockCM,
		"http://test:8080", // http - no TLS
		"",
		"test-nexus-id",
		"1.0.0",
		[]string{"catalog"},
	)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/qr", nil)
	w := httptest.NewRecorder()

	handler.HandleQRCode(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", w.Code)
	}

	var response map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// SPKI should be empty string
	spki, ok := response["spki"].(string)
	if !ok {
		t.Fatal("Response missing 'spki' field")
	}

	if spki != "" {
		t.Errorf("Expected empty SPKI, got: %s", spki)
	}
}

func TestCalculateSPKI_FileCertPriority(t *testing.T) {
	// Setup: Both file cert and managed cert exist
	// File cert should take priority

	// Create a temp cert file
	fileCert, fileExpectedSPKI := generateTestCert(t)
	managedCert, _ := generateTestCert(t) // Different cert

	// Write file cert to temp file
	tmpFile, err := os.CreateTemp("", "test-cert-*.pem")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	// Encode cert as PEM
	x509Cert, _ := x509.ParseCertificate(fileCert.Certificate[0])
	pemData := encodeCertToPEM(x509Cert)
	if _, err := tmpFile.Write(pemData); err != nil {
		t.Fatalf("Failed to write cert: %v", err)
	}
	tmpFile.Close()

	mockCM := &mockCertManager{
		firstAvailableCert: managedCert,
	}

	authMgr, _ := auth.NewManager(
		[]byte("random-jwt-hmac-key-f8e7d6c5b4a39281-only"),
		"test-issuer",
		"test-audience",
		[]string{},
	)

	handler := NewAuthHandler(
		authMgr,
		nil, nil, nil, nil,
		ratelimit.NewLimiter(1.0, 5),
		mockCM,
		"https://test:8443",
		tmpFile.Name(), // file path set
		"test-nexus-id",
		"1.0.0",
		[]string{"test"},
	)

	spki, err := handler.calculateSPKI()
	if err != nil {
		t.Fatalf("calculateSPKI returned error: %v", err)
	}

	// Should return file cert's SPKI, not managed cert's
	if spki != fileExpectedSPKI {
		t.Errorf("Expected file cert SPKI to take priority.\ngot:  %s\nwant: %s", spki, fileExpectedSPKI)
	}
}

// encodeCertToPEM encodes an x509 certificate to PEM format
func encodeCertToPEM(cert *x509.Certificate) []byte {
	return []byte("-----BEGIN CERTIFICATE-----\n" +
		base64.StdEncoding.EncodeToString(cert.Raw) +
		"\n-----END CERTIFICATE-----\n")
}

func TestHandleQRCode_SPKIRequiredForHTTPS(t *testing.T) {
	// Per security spec: SPKI is MANDATORY for HTTPS connections
	// QR code generation should fail if using HTTPS but no cert is available

	mockCM := &mockCertManager{
		firstAvailableCert: nil,
		fallbackCert:       nil,
	}

	authMgr, _ := auth.NewManager(
		[]byte("random-jwt-hmac-key-f8e7d6c5b4a39281-only"),
		"test-issuer",
		"test-audience",
		[]string{},
	)

	handler := NewAuthHandler(
		authMgr,
		nil, nil, nil, nil,
		ratelimit.NewLimiter(1.0, 5),
		mockCM,
		"https://test:8443", // HTTPS - SPKI required
		"",
		"test-nexus-id",
		"1.0.0",
		[]string{"catalog"},
	)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/qr", nil)
	w := httptest.NewRecorder()

	handler.HandleQRCode(w, req)

	// Should fail with error since SPKI is required for HTTPS
	if w.Code != http.StatusInternalServerError {
		t.Errorf("Expected status 500 for HTTPS without SPKI, got %d", w.Code)
	}

	// Check error response
	var response map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	errorCode, ok := response["error"].(string)
	if !ok || errorCode != "SPKI_REQUIRED" {
		t.Errorf("Expected error code SPKI_REQUIRED, got %v", response["error"])
	}
}

func TestHandleQRCode_SPKIOptionalForHTTP(t *testing.T) {
	// SPKI is optional for HTTP connections (development mode)

	mockCM := &mockCertManager{
		firstAvailableCert: nil,
		fallbackCert:       nil,
	}

	authMgr, _ := auth.NewManager(
		[]byte("random-jwt-hmac-key-f8e7d6c5b4a39281-only"),
		"test-issuer",
		"test-audience",
		[]string{},
	)

	handler := NewAuthHandler(
		authMgr,
		nil, nil, nil, nil,
		ratelimit.NewLimiter(1.0, 5),
		mockCM,
		"http://test:8080", // HTTP - SPKI optional
		"",
		"test-nexus-id",
		"1.0.0",
		[]string{"catalog"},
	)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/qr", nil)
	w := httptest.NewRecorder()

	handler.HandleQRCode(w, req)

	// Should succeed with empty SPKI for HTTP
	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200 for HTTP without SPKI, got %d: %s", w.Code, w.Body.String())
	}

	var response map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// SPKI should be empty string
	spki, ok := response["spki"].(string)
	if !ok {
		t.Fatal("Response missing 'spki' field")
	}

	if spki != "" {
		t.Errorf("Expected empty SPKI for HTTP, got: %s", spki)
	}
}

// TestHandleQRCode_IncludesSPKIBackup tests that QR codes include backup SPKI
// when a backup certificate is configured
func TestHandleQRCode_IncludesSPKIBackup(t *testing.T) {
	// Setup: Mock certManager with both primary and backup certificates
	primaryCert, primarySPKI := generateTestCert(t)
	backupCert, backupSPKI := generateTestCert(t)

	mockCM := &mockCertManagerWithBackup{
		firstAvailableCert: primaryCert,
		fallbackCert:       nil,
		backupCert:         backupCert,
	}

	authMgr, _ := auth.NewManager(
		[]byte("random-jwt-hmac-key-f8e7d6c5b4a39281-only"),
		"test-issuer",
		"test-audience",
		[]string{},
	)

	handler := NewAuthHandlerWithBackupCert(
		authMgr,
		nil, nil, nil, nil,
		ratelimit.NewLimiter(1.0, 5),
		mockCM,
		"https://test:8443",
		"",
		"test-nexus-id",
		"1.0.0",
		[]string{"catalog"},
	)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/qr", nil)
	w := httptest.NewRecorder()

	handler.HandleQRCode(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var response map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// Assert primary SPKI
	spki, ok := response["spki"].(string)
	if !ok || spki != primarySPKI {
		t.Errorf("Primary SPKI mismatch:\ngot:  %s\nwant: %s", spki, primarySPKI)
	}

	// Assert backup SPKI
	spkiBackup, ok := response["spkiBackup"].(string)
	if !ok || spkiBackup != backupSPKI {
		t.Errorf("Backup SPKI mismatch:\ngot:  %s\nwant: %s", spkiBackup, backupSPKI)
	}
}

// TestHandleQRCode_NoBackupSPKIWhenNotConfigured tests that spkiBackup is empty
// when no backup certificate is configured
func TestHandleQRCode_NoBackupSPKIWhenNotConfigured(t *testing.T) {
	primaryCert, _ := generateTestCert(t)

	mockCM := &mockCertManagerWithBackup{
		firstAvailableCert: primaryCert,
		fallbackCert:       nil,
		backupCert:         nil, // No backup
	}

	authMgr, _ := auth.NewManager(
		[]byte("random-jwt-hmac-key-f8e7d6c5b4a39281-only"),
		"test-issuer",
		"test-audience",
		[]string{},
	)

	handler := NewAuthHandlerWithBackupCert(
		authMgr,
		nil, nil, nil, nil,
		ratelimit.NewLimiter(1.0, 5),
		mockCM,
		"https://test:8443",
		"",
		"test-nexus-id",
		"1.0.0",
		[]string{"catalog"},
	)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/qr", nil)
	w := httptest.NewRecorder()

	handler.HandleQRCode(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", w.Code)
	}

	var response map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// spkiBackup should be empty string or not present
	spkiBackup, exists := response["spkiBackup"]
	if exists && spkiBackup != "" {
		t.Errorf("Expected empty or missing spkiBackup, got: %v", spkiBackup)
	}
}

// mockCertManagerWithBackup extends mockCertManager with backup certificate support
type mockCertManagerWithBackup struct {
	firstAvailableCert *tls.Certificate
	fallbackCert       *tls.Certificate
	backupCert         *tls.Certificate
}

func (m *mockCertManagerWithBackup) GetFirstAvailableCertificate() *tls.Certificate {
	return m.firstAvailableCert
}

func (m *mockCertManagerWithBackup) GetFallbackCertificate() *tls.Certificate {
	return m.fallbackCert
}

func (m *mockCertManagerWithBackup) GetBackupCertificate() *tls.Certificate {
	return m.backupCert
}
