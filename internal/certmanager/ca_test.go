package certmanager

import (
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"testing"
	"time"
)

func TestGenerateCA_Success(t *testing.T) {
	provider := NewSelfSignedProvider()

	ca, err := provider.GenerateCA(CAOptions{
		CommonName:   "Nexus Local CA",
		Organization: "Nekzus",
		ValidityDays: 3650, // 10 years
	})

	if err != nil {
		t.Fatalf("Failed to generate CA: %v", err)
	}

	if ca == nil {
		t.Fatal("Expected non-nil CA")
	}

	// Verify it's a CA certificate
	if !ca.Certificate.IsCA {
		t.Error("Expected IsCA to be true")
	}

	// Verify basic constraints
	if !ca.Certificate.BasicConstraintsValid {
		t.Error("Expected BasicConstraintsValid to be true")
	}

	// Verify key usage includes cert signing
	if ca.Certificate.KeyUsage&x509.KeyUsageCertSign == 0 {
		t.Error("Expected KeyUsageCertSign")
	}

	// Verify common name
	if ca.Certificate.Subject.CommonName != "Nexus Local CA" {
		t.Errorf("Expected CN 'Nexus Local CA', got: %s", ca.Certificate.Subject.CommonName)
	}

	// Verify organization
	if len(ca.Certificate.Subject.Organization) == 0 || ca.Certificate.Subject.Organization[0] != "Nekzus" {
		t.Errorf("Expected Organization 'Nekzus', got: %v", ca.Certificate.Subject.Organization)
	}

	// Verify validity period (approximately 10 years)
	expectedExpiry := time.Now().Add(3650 * 24 * time.Hour)
	if ca.Certificate.NotAfter.Before(expectedExpiry.Add(-24*time.Hour)) ||
		ca.Certificate.NotAfter.After(expectedExpiry.Add(24*time.Hour)) {
		t.Errorf("Expected ~10 year validity, got NotAfter: %v", ca.Certificate.NotAfter)
	}

	// Verify PEM encoding is valid
	if len(ca.PEM.Certificate) == 0 {
		t.Error("Expected non-empty certificate PEM")
	}
	if len(ca.PEM.PrivateKey) == 0 {
		t.Error("Expected non-empty private key PEM")
	}

	// Verify fingerprint is set
	if ca.Fingerprint == "" {
		t.Error("Expected non-empty fingerprint")
	}
}

func TestGenerateCA_DefaultOptions(t *testing.T) {
	provider := NewSelfSignedProvider()

	// Generate with empty options - should use defaults
	ca, err := provider.GenerateCA(CAOptions{})

	if err != nil {
		t.Fatalf("Failed to generate CA with defaults: %v", err)
	}

	// Should default to "Nexus CA" common name
	if ca.Certificate.Subject.CommonName != "Nexus CA" {
		t.Errorf("Expected default CN 'Nexus CA', got: %s", ca.Certificate.Subject.CommonName)
	}

	// Should default to Nekzus organization
	if len(ca.Certificate.Subject.Organization) == 0 || ca.Certificate.Subject.Organization[0] != "Nekzus" {
		t.Errorf("Expected default Organization 'Nekzus', got: %v", ca.Certificate.Subject.Organization)
	}

	// Should default to ~10 year validity
	minExpiry := time.Now().Add(3640 * 24 * time.Hour)
	if ca.Certificate.NotAfter.Before(minExpiry) {
		t.Errorf("Expected ~10 year default validity, got NotAfter: %v", ca.Certificate.NotAfter)
	}
}

func TestGenerateCA_ECDSA(t *testing.T) {
	provider := NewSelfSignedProvider()

	ca, err := provider.GenerateCA(CAOptions{
		KeyType: "ecdsa",
	})

	if err != nil {
		t.Fatalf("Failed to generate ECDSA CA: %v", err)
	}

	// Verify key type is ECDSA
	_, ok := ca.PrivateKey.(*ecdsa.PrivateKey)
	if !ok {
		t.Error("Expected ECDSA private key")
	}

	// Verify PEM block type
	block, _ := pem.Decode(ca.PEM.PrivateKey)
	if block == nil {
		t.Fatal("Failed to decode private key PEM")
	}
	if block.Type != "EC PRIVATE KEY" {
		t.Errorf("Expected 'EC PRIVATE KEY' PEM type, got: %s", block.Type)
	}
}

func TestGenerateCA_RSA(t *testing.T) {
	provider := NewSelfSignedProvider()

	ca, err := provider.GenerateCA(CAOptions{
		KeyType: "rsa",
		KeySize: 4096,
	})

	if err != nil {
		t.Fatalf("Failed to generate RSA CA: %v", err)
	}

	// Verify key type is RSA
	rsaKey, ok := ca.PrivateKey.(*rsa.PrivateKey)
	if !ok {
		t.Error("Expected RSA private key")
	}

	// Verify key size
	if rsaKey.N.BitLen() != 4096 {
		t.Errorf("Expected 4096-bit RSA key, got: %d bits", rsaKey.N.BitLen())
	}

	// Verify PEM block type
	block, _ := pem.Decode(ca.PEM.PrivateKey)
	if block == nil {
		t.Fatal("Failed to decode private key PEM")
	}
	if block.Type != "RSA PRIVATE KEY" {
		t.Errorf("Expected 'RSA PRIVATE KEY' PEM type, got: %s", block.Type)
	}
}

func TestSignCertificate_SingleDomain(t *testing.T) {
	provider := NewSelfSignedProvider()

	// First generate a CA
	ca, err := provider.GenerateCA(CAOptions{})
	if err != nil {
		t.Fatalf("Failed to generate CA: %v", err)
	}

	// Sign a certificate for a single domain
	cert, err := provider.SignWithCA(ca, SignRequest{
		Domains:      []string{"app.local"},
		ValidityDays: 365,
	})

	if err != nil {
		t.Fatalf("Failed to sign certificate: %v", err)
	}

	if cert == nil {
		t.Fatal("Expected non-nil certificate")
	}

	// Verify domain
	if cert.Domain != "app.local" {
		t.Errorf("Expected domain 'app.local', got: %s", cert.Domain)
	}

	// Verify it's NOT a CA certificate
	x509Cert, err := x509.ParseCertificate(cert.TLSCert.Certificate[0])
	if err != nil {
		t.Fatalf("Failed to parse certificate: %v", err)
	}

	if x509Cert.IsCA {
		t.Error("Expected leaf certificate, not CA")
	}

	// Verify issuer matches CA
	if cert.Metadata.Issuer != ca.Certificate.Subject.CommonName {
		t.Errorf("Expected issuer '%s', got: %s", ca.Certificate.Subject.CommonName, cert.Metadata.Issuer)
	}

	// Verify SANs
	if len(x509Cert.DNSNames) != 1 || x509Cert.DNSNames[0] != "app.local" {
		t.Errorf("Expected SAN 'app.local', got: %v", x509Cert.DNSNames)
	}
}

func TestSignCertificate_Wildcard(t *testing.T) {
	provider := NewSelfSignedProvider()

	ca, err := provider.GenerateCA(CAOptions{})
	if err != nil {
		t.Fatalf("Failed to generate CA: %v", err)
	}

	// Sign a wildcard certificate
	cert, err := provider.SignWithCA(ca, SignRequest{
		Domains:      []string{"*.nexus.local", "nexus.local"},
		ValidityDays: 365,
	})

	if err != nil {
		t.Fatalf("Failed to sign wildcard certificate: %v", err)
	}

	// Parse certificate
	x509Cert, err := x509.ParseCertificate(cert.TLSCert.Certificate[0])
	if err != nil {
		t.Fatalf("Failed to parse certificate: %v", err)
	}

	// Verify wildcard is in SANs
	hasWildcard := false
	hasBase := false
	for _, san := range x509Cert.DNSNames {
		if san == "*.nexus.local" {
			hasWildcard = true
		}
		if san == "nexus.local" {
			hasBase = true
		}
	}

	if !hasWildcard {
		t.Error("Expected wildcard *.nexus.local in SANs")
	}
	if !hasBase {
		t.Error("Expected base nexus.local in SANs")
	}
}

func TestSignCertificate_MultipleDomains(t *testing.T) {
	provider := NewSelfSignedProvider()

	ca, err := provider.GenerateCA(CAOptions{})
	if err != nil {
		t.Fatalf("Failed to generate CA: %v", err)
	}

	domains := []string{"app1.local", "app2.local", "api.local"}
	cert, err := provider.SignWithCA(ca, SignRequest{
		Domains:      domains,
		ValidityDays: 365,
	})

	if err != nil {
		t.Fatalf("Failed to sign multi-domain certificate: %v", err)
	}

	// Parse certificate
	x509Cert, err := x509.ParseCertificate(cert.TLSCert.Certificate[0])
	if err != nil {
		t.Fatalf("Failed to parse certificate: %v", err)
	}

	// Verify all domains are in SANs
	sanMap := make(map[string]bool)
	for _, san := range x509Cert.DNSNames {
		sanMap[san] = true
	}

	for _, domain := range domains {
		if !sanMap[domain] {
			t.Errorf("Expected domain %s in SANs", domain)
		}
	}
}

func TestSignCertificate_WithIPAddresses(t *testing.T) {
	provider := NewSelfSignedProvider()

	ca, err := provider.GenerateCA(CAOptions{})
	if err != nil {
		t.Fatalf("Failed to generate CA: %v", err)
	}

	// Sign certificate with IP addresses
	cert, err := provider.SignWithCA(ca, SignRequest{
		Domains:      []string{"app.local", "192.168.1.100", "::1"},
		ValidityDays: 365,
	})

	if err != nil {
		t.Fatalf("Failed to sign certificate with IPs: %v", err)
	}

	// Parse certificate
	x509Cert, err := x509.ParseCertificate(cert.TLSCert.Certificate[0])
	if err != nil {
		t.Fatalf("Failed to parse certificate: %v", err)
	}

	// Verify DNS names
	if len(x509Cert.DNSNames) != 1 || x509Cert.DNSNames[0] != "app.local" {
		t.Errorf("Expected DNS name 'app.local', got: %v", x509Cert.DNSNames)
	}

	// Verify IP addresses
	if len(x509Cert.IPAddresses) != 2 {
		t.Errorf("Expected 2 IP addresses, got: %d", len(x509Cert.IPAddresses))
	}
}

func TestSignCertificate_VerifyChain(t *testing.T) {
	provider := NewSelfSignedProvider()

	ca, err := provider.GenerateCA(CAOptions{})
	if err != nil {
		t.Fatalf("Failed to generate CA: %v", err)
	}

	cert, err := provider.SignWithCA(ca, SignRequest{
		Domains:      []string{"app.local"},
		ValidityDays: 365,
	})

	if err != nil {
		t.Fatalf("Failed to sign certificate: %v", err)
	}

	// Parse the signed certificate
	x509Cert, err := x509.ParseCertificate(cert.TLSCert.Certificate[0])
	if err != nil {
		t.Fatalf("Failed to parse certificate: %v", err)
	}

	// Create a cert pool with the CA
	roots := x509.NewCertPool()
	roots.AddCert(ca.Certificate)

	// Verify the certificate chain
	opts := x509.VerifyOptions{
		Roots: roots,
	}

	_, err = x509Cert.Verify(opts)
	if err != nil {
		t.Errorf("Certificate verification failed: %v", err)
	}
}

func TestSignCertificate_EmptyDomains(t *testing.T) {
	provider := NewSelfSignedProvider()

	ca, err := provider.GenerateCA(CAOptions{})
	if err != nil {
		t.Fatalf("Failed to generate CA: %v", err)
	}

	// Try to sign with empty domains - should fail
	_, err = provider.SignWithCA(ca, SignRequest{
		Domains:      []string{},
		ValidityDays: 365,
	})

	if err == nil {
		t.Error("Expected error for empty domains")
	}
}

func TestSignCertificate_NilCA(t *testing.T) {
	provider := NewSelfSignedProvider()

	// Try to sign with nil CA - should fail
	_, err := provider.SignWithCA(nil, SignRequest{
		Domains:      []string{"app.local"},
		ValidityDays: 365,
	})

	if err == nil {
		t.Error("Expected error for nil CA")
	}
}

func TestSignCertificate_ValidityPeriod(t *testing.T) {
	provider := NewSelfSignedProvider()

	ca, err := provider.GenerateCA(CAOptions{})
	if err != nil {
		t.Fatalf("Failed to generate CA: %v", err)
	}

	validityDays := 90
	cert, err := provider.SignWithCA(ca, SignRequest{
		Domains:      []string{"app.local"},
		ValidityDays: validityDays,
	})

	if err != nil {
		t.Fatalf("Failed to sign certificate: %v", err)
	}

	// Parse certificate
	x509Cert, err := x509.ParseCertificate(cert.TLSCert.Certificate[0])
	if err != nil {
		t.Fatalf("Failed to parse certificate: %v", err)
	}

	// Verify validity period
	expectedExpiry := time.Now().Add(time.Duration(validityDays) * 24 * time.Hour)
	if x509Cert.NotAfter.Before(expectedExpiry.Add(-24*time.Hour)) ||
		x509Cert.NotAfter.After(expectedExpiry.Add(24*time.Hour)) {
		t.Errorf("Expected ~%d day validity, got NotAfter: %v", validityDays, x509Cert.NotAfter)
	}
}

func TestSignCertificate_DefaultValidity(t *testing.T) {
	provider := NewSelfSignedProvider()

	ca, err := provider.GenerateCA(CAOptions{})
	if err != nil {
		t.Fatalf("Failed to generate CA: %v", err)
	}

	// Sign with 0 validity days - should use default
	cert, err := provider.SignWithCA(ca, SignRequest{
		Domains:      []string{"app.local"},
		ValidityDays: 0,
	})

	if err != nil {
		t.Fatalf("Failed to sign certificate: %v", err)
	}

	// Parse certificate
	x509Cert, err := x509.ParseCertificate(cert.TLSCert.Certificate[0])
	if err != nil {
		t.Fatalf("Failed to parse certificate: %v", err)
	}

	// Should default to 365 days
	minExpiry := time.Now().Add(360 * 24 * time.Hour)
	if x509Cert.NotAfter.Before(minExpiry) {
		t.Errorf("Expected default ~365 day validity, got NotAfter: %v", x509Cert.NotAfter)
	}
}

func TestExportCertBundle(t *testing.T) {
	provider := NewSelfSignedProvider()

	ca, err := provider.GenerateCA(CAOptions{})
	if err != nil {
		t.Fatalf("Failed to generate CA: %v", err)
	}

	cert, err := provider.SignWithCA(ca, SignRequest{
		Domains:      []string{"*.nexus.local", "nexus.local"},
		ValidityDays: 365,
	})

	if err != nil {
		t.Fatalf("Failed to sign certificate: %v", err)
	}

	// Export the bundle
	bundle := ExportCertBundle(ca, cert)

	// Verify CA cert is present
	if len(bundle.CACert) == 0 {
		t.Error("Expected non-empty CA certificate")
	}

	// Verify cert is present
	if len(bundle.Cert) == 0 {
		t.Error("Expected non-empty certificate")
	}

	// Verify key is present
	if len(bundle.Key) == 0 {
		t.Error("Expected non-empty private key")
	}

	// Verify PEM blocks are valid
	caBlock, _ := pem.Decode(bundle.CACert)
	if caBlock == nil || caBlock.Type != "CERTIFICATE" {
		t.Error("Invalid CA certificate PEM")
	}

	certBlock, _ := pem.Decode(bundle.Cert)
	if certBlock == nil || certBlock.Type != "CERTIFICATE" {
		t.Error("Invalid certificate PEM")
	}

	keyBlock, _ := pem.Decode(bundle.Key)
	if keyBlock == nil {
		t.Error("Invalid private key PEM")
	}
}

func TestCAReloadFromPEM(t *testing.T) {
	provider := NewSelfSignedProvider()

	// Generate original CA
	originalCA, err := provider.GenerateCA(CAOptions{
		CommonName: "Test CA",
	})
	if err != nil {
		t.Fatalf("Failed to generate CA: %v", err)
	}

	// Reload CA from PEM
	reloadedCA, err := LoadCAFromPEM(originalCA.PEM.Certificate, originalCA.PEM.PrivateKey)
	if err != nil {
		t.Fatalf("Failed to reload CA from PEM: %v", err)
	}

	// Verify they're equivalent
	if reloadedCA.Certificate.Subject.CommonName != originalCA.Certificate.Subject.CommonName {
		t.Errorf("CN mismatch: expected %s, got %s",
			originalCA.Certificate.Subject.CommonName,
			reloadedCA.Certificate.Subject.CommonName)
	}

	if reloadedCA.Fingerprint != originalCA.Fingerprint {
		t.Errorf("Fingerprint mismatch: expected %s, got %s",
			originalCA.Fingerprint,
			reloadedCA.Fingerprint)
	}

	// Verify reloaded CA can sign certificates
	cert, err := provider.SignWithCA(reloadedCA, SignRequest{
		Domains:      []string{"app.local"},
		ValidityDays: 365,
	})
	if err != nil {
		t.Fatalf("Failed to sign with reloaded CA: %v", err)
	}

	if cert == nil {
		t.Error("Expected non-nil certificate from reloaded CA")
	}
}
