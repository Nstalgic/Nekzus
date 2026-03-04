package certmanager

import (
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"net"
	"strings"
	"testing"
	"time"
)

func TestSelfSignedProvider_Name(t *testing.T) {
	provider := NewSelfSignedProvider()
	if provider.Name() != "self-signed" {
		t.Errorf("Expected name 'self-signed', got: %s", provider.Name())
	}
}

func TestSelfSignedProvider_Validate(t *testing.T) {
	provider := NewSelfSignedProvider()

	tests := []struct {
		name      string
		domains   []string
		expectErr bool
	}{
		{
			name:      "valid single domain",
			domains:   []string{"test.local"},
			expectErr: false,
		},
		{
			name:      "valid multiple domains",
			domains:   []string{"app1.local", "app2.local"},
			expectErr: false,
		},
		{
			name:      "empty domains",
			domains:   []string{},
			expectErr: true,
		},
		{
			name:      "nil domains",
			domains:   nil,
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := provider.Validate(tt.domains)
			if tt.expectErr && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectErr && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

func TestSelfSignedProvider_Generate_ECDSA(t *testing.T) {
	provider := NewSelfSignedProvider()

	cert, err := provider.Generate([]string{"test.local"}, GenerateOptions{
		KeyType:      "ecdsa",
		ValidityDays: 365,
	})

	if err != nil {
		t.Fatalf("Failed to generate ECDSA certificate: %v", err)
	}

	// Verify certificate was created
	if cert == nil {
		t.Fatal("Expected certificate, got nil")
	}

	// Verify domain
	if cert.Domain != "test.local" {
		t.Errorf("Expected domain test.local, got: %s", cert.Domain)
	}

	// Verify TLS certificate exists
	if cert.TLSCert == nil {
		t.Fatal("Expected TLS certificate, got nil")
	}

	// Parse the certificate and verify key type
	x509Cert, err := x509.ParseCertificate(cert.TLSCert.Certificate[0])
	if err != nil {
		t.Fatalf("Failed to parse certificate: %v", err)
	}

	// Verify it's an ECDSA key
	if _, ok := x509Cert.PublicKey.(*ecdsa.PublicKey); !ok {
		t.Error("Expected ECDSA public key")
	}

	// Verify PEM data
	if len(cert.PEM.Certificate) == 0 {
		t.Error("Expected certificate PEM data")
	}
	if len(cert.PEM.PrivateKey) == 0 {
		t.Error("Expected private key PEM data")
	}

	// Verify key PEM type
	block, _ := pem.Decode(cert.PEM.PrivateKey)
	if block == nil {
		t.Fatal("Failed to decode private key PEM")
	}
	if block.Type != "EC PRIVATE KEY" {
		t.Errorf("Expected 'EC PRIVATE KEY', got: %s", block.Type)
	}
}

func TestSelfSignedProvider_Generate_RSA(t *testing.T) {
	provider := NewSelfSignedProvider()

	cert, err := provider.Generate([]string{"test.local"}, GenerateOptions{
		KeyType:      "rsa",
		KeySize:      2048,
		ValidityDays: 365,
	})

	if err != nil {
		t.Fatalf("Failed to generate RSA certificate: %v", err)
	}

	// Parse the certificate and verify key type
	x509Cert, err := x509.ParseCertificate(cert.TLSCert.Certificate[0])
	if err != nil {
		t.Fatalf("Failed to parse certificate: %v", err)
	}

	// Verify it's an RSA key
	rsaKey, ok := x509Cert.PublicKey.(*rsa.PublicKey)
	if !ok {
		t.Fatal("Expected RSA public key")
	}

	// Verify key size (2048 bits = 256 bytes)
	if rsaKey.Size() != 256 {
		t.Errorf("Expected RSA key size 256 bytes (2048 bits), got: %d", rsaKey.Size())
	}

	// Verify key PEM type
	block, _ := pem.Decode(cert.PEM.PrivateKey)
	if block == nil {
		t.Fatal("Failed to decode private key PEM")
	}
	if block.Type != "RSA PRIVATE KEY" {
		t.Errorf("Expected 'RSA PRIVATE KEY', got: %s", block.Type)
	}
}

func TestSelfSignedProvider_Generate_ValidityPeriod(t *testing.T) {
	provider := NewSelfSignedProvider()

	tests := []struct {
		name         string
		validityDays int
	}{
		{"30 days", 30},
		{"90 days", 90},
		{"365 days", 365},
		{"730 days (2 years)", 730},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cert, err := provider.Generate([]string{"test.local"}, GenerateOptions{
				ValidityDays: tt.validityDays,
			})

			if err != nil {
				t.Fatalf("Failed to generate certificate: %v", err)
			}

			// Parse certificate
			x509Cert, err := x509.ParseCertificate(cert.TLSCert.Certificate[0])
			if err != nil {
				t.Fatalf("Failed to parse certificate: %v", err)
			}

			// Verify validity period (allow 1 second tolerance)
			expectedDuration := time.Duration(tt.validityDays) * 24 * time.Hour
			actualDuration := x509Cert.NotAfter.Sub(x509Cert.NotBefore)
			tolerance := time.Second

			if actualDuration < expectedDuration-tolerance || actualDuration > expectedDuration+tolerance {
				t.Errorf("Expected validity %v, got: %v", expectedDuration, actualDuration)
			}

			// Verify metadata matches
			if cert.Metadata.NotBefore.IsZero() {
				t.Error("Expected NotBefore to be set")
			}
			if cert.Metadata.NotAfter.IsZero() {
				t.Error("Expected NotAfter to be set")
			}
		})
	}
}

func TestSelfSignedProvider_Generate_SANs(t *testing.T) {
	provider := NewSelfSignedProvider()

	domains := []string{
		"app.local",
		"api.local",
		"*.services.local",
	}

	cert, err := provider.Generate(domains, GenerateOptions{})

	if err != nil {
		t.Fatalf("Failed to generate certificate: %v", err)
	}

	// Parse certificate
	x509Cert, err := x509.ParseCertificate(cert.TLSCert.Certificate[0])
	if err != nil {
		t.Fatalf("Failed to parse certificate: %v", err)
	}

	// Verify all domains are in SANs
	for _, domain := range domains {
		found := false
		for _, dns := range x509Cert.DNSNames {
			if dns == domain {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Domain %s not found in certificate SANs", domain)
		}
	}

	// Verify metadata SANs
	if len(cert.Metadata.SANs) != len(domains) {
		t.Errorf("Expected %d SANs in metadata, got: %d", len(domains), len(cert.Metadata.SANs))
	}
}

func TestSelfSignedProvider_Generate_IPAddresses(t *testing.T) {
	provider := NewSelfSignedProvider()

	domains := []string{
		"app.local",
		"192.168.1.100",
		"10.0.0.1",
		"::1",
		"2001:db8::1",
	}

	cert, err := provider.Generate(domains, GenerateOptions{})

	if err != nil {
		t.Fatalf("Failed to generate certificate: %v", err)
	}

	// Parse certificate
	x509Cert, err := x509.ParseCertificate(cert.TLSCert.Certificate[0])
	if err != nil {
		t.Fatalf("Failed to parse certificate: %v", err)
	}

	// Verify DNS names
	if len(x509Cert.DNSNames) != 1 || x509Cert.DNSNames[0] != "app.local" {
		t.Errorf("Expected 1 DNS name 'app.local', got: %v", x509Cert.DNSNames)
	}

	// Verify IP addresses (4 IPs: 192.168.1.100, 10.0.0.1, ::1, 2001:db8::1)
	if len(x509Cert.IPAddresses) != 4 {
		t.Errorf("Expected 4 IP addresses, got: %d", len(x509Cert.IPAddresses))
	}

	// Check specific IPs are present
	expectedIPs := []string{"192.168.1.100", "10.0.0.1", "::1", "2001:db8::1"}
	for _, ipStr := range expectedIPs {
		expectedIP := net.ParseIP(ipStr)
		found := false
		for _, ip := range x509Cert.IPAddresses {
			if ip.Equal(expectedIP) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("IP address %s not found in certificate", ipStr)
		}
	}
}

func TestSelfSignedProvider_Generate_Fingerprint(t *testing.T) {
	provider := NewSelfSignedProvider()

	cert, err := provider.Generate([]string{"test.local"}, GenerateOptions{})

	if err != nil {
		t.Fatalf("Failed to generate certificate: %v", err)
	}

	// Verify fingerprint exists and has correct format
	if cert.Metadata.Fingerprint == "" {
		t.Fatal("Expected fingerprint to be set")
	}

	if !strings.HasPrefix(cert.Metadata.Fingerprint, "sha256/") {
		t.Errorf("Expected fingerprint to start with 'sha256/', got: %s", cert.Metadata.Fingerprint)
	}

	// Verify fingerprint is valid base64
	fingerprintData := strings.TrimPrefix(cert.Metadata.Fingerprint, "sha256/")
	decoded, err := base64.StdEncoding.DecodeString(fingerprintData)
	if err != nil {
		t.Errorf("Fingerprint is not valid base64: %v", err)
	}

	// SHA-256 hash should be 32 bytes
	if len(decoded) != sha256.Size {
		t.Errorf("Expected fingerprint hash size %d bytes, got: %d", sha256.Size, len(decoded))
	}

	// Verify fingerprint is reproducible from the certificate
	x509Cert, _ := x509.ParseCertificate(cert.TLSCert.Certificate[0])
	spki, _ := x509.MarshalPKIXPublicKey(x509Cert.PublicKey)
	hash := sha256.Sum256(spki)
	expectedFingerprint := "sha256/" + base64.StdEncoding.EncodeToString(hash[:])

	if cert.Metadata.Fingerprint != expectedFingerprint {
		t.Errorf("Fingerprint mismatch: expected %s, got %s", expectedFingerprint, cert.Metadata.Fingerprint)
	}
}

func TestSelfSignedProvider_Generate_CertificateMetadata(t *testing.T) {
	provider := NewSelfSignedProvider()

	cert, err := provider.Generate([]string{"test.local"}, GenerateOptions{})

	if err != nil {
		t.Fatalf("Failed to generate certificate: %v", err)
	}

	// Verify issuer
	if cert.Metadata.Issuer != "self-signed" {
		t.Errorf("Expected issuer 'self-signed', got: %s", cert.Metadata.Issuer)
	}

	// Verify certificate properties
	x509Cert, _ := x509.ParseCertificate(cert.TLSCert.Certificate[0])

	// Check common name
	if x509Cert.Subject.CommonName != "test.local" {
		t.Errorf("Expected CommonName 'test.local', got: %s", x509Cert.Subject.CommonName)
	}

	// Check organization
	if len(x509Cert.Subject.Organization) == 0 || x509Cert.Subject.Organization[0] != "Nekzus" {
		t.Errorf("Expected Organization 'Nekzus', got: %v", x509Cert.Subject.Organization)
	}

	// Check key usage
	expectedKeyUsage := x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature
	if x509Cert.KeyUsage != expectedKeyUsage {
		t.Errorf("Expected key usage %v, got: %v", expectedKeyUsage, x509Cert.KeyUsage)
	}

	// Check extended key usage
	if len(x509Cert.ExtKeyUsage) != 1 || x509Cert.ExtKeyUsage[0] != x509.ExtKeyUsageServerAuth {
		t.Errorf("Expected ExtKeyUsage [ServerAuth], got: %v", x509Cert.ExtKeyUsage)
	}
}

func TestSelfSignedProvider_Generate_DefaultOptions(t *testing.T) {
	provider := NewSelfSignedProvider()

	// Generate with empty options - should use defaults
	cert, err := provider.Generate([]string{"test.local"}, GenerateOptions{})

	if err != nil {
		t.Fatalf("Failed to generate certificate: %v", err)
	}

	// Parse certificate
	x509Cert, _ := x509.ParseCertificate(cert.TLSCert.Certificate[0])

	// Default should be ECDSA
	if _, ok := x509Cert.PublicKey.(*ecdsa.PublicKey); !ok {
		t.Error("Default key type should be ECDSA")
	}

	// Default validity should be 365 days
	expectedDuration := 365 * 24 * time.Hour
	actualDuration := x509Cert.NotAfter.Sub(x509Cert.NotBefore)
	tolerance := time.Second

	if actualDuration < expectedDuration-tolerance || actualDuration > expectedDuration+tolerance {
		t.Errorf("Default validity should be 365 days, got: %v", actualDuration)
	}
}

func TestSelfSignedProvider_Renew(t *testing.T) {
	provider := NewSelfSignedProvider()

	// Generate original certificate
	originalCert, err := provider.Generate([]string{"app.local", "api.local"}, GenerateOptions{
		ValidityDays: 30,
	})

	if err != nil {
		t.Fatalf("Failed to generate original certificate: %v", err)
	}

	// Renew the certificate
	renewedCert, err := provider.Renew(originalCert)

	if err != nil {
		t.Fatalf("Failed to renew certificate: %v", err)
	}

	// Verify renewed certificate exists
	if renewedCert == nil {
		t.Fatal("Expected renewed certificate, got nil")
	}

	// Verify domain is preserved
	if renewedCert.Domain != originalCert.Domain {
		t.Errorf("Domain mismatch: expected %s, got %s", originalCert.Domain, renewedCert.Domain)
	}

	// Verify SANs are preserved
	if len(renewedCert.Metadata.SANs) != len(originalCert.Metadata.SANs) {
		t.Errorf("SANs count mismatch: expected %d, got %d",
			len(originalCert.Metadata.SANs), len(renewedCert.Metadata.SANs))
	}

	for i, san := range originalCert.Metadata.SANs {
		if renewedCert.Metadata.SANs[i] != san {
			t.Errorf("SAN mismatch at index %d: expected %s, got %s", i, san, renewedCert.Metadata.SANs[i])
		}
	}

	// Verify renewed certificate has fresh validity (365 days from now)
	renewedX509, _ := x509.ParseCertificate(renewedCert.TLSCert.Certificate[0])
	expectedValidity := 365 * 24 * time.Hour
	actualValidity := renewedX509.NotAfter.Sub(renewedX509.NotBefore)
	tolerance := time.Second

	if actualValidity < expectedValidity-tolerance || actualValidity > expectedValidity+tolerance {
		t.Errorf("Renewed certificate should have 365-day validity, got: %v", actualValidity)
	}

	// Verify fingerprints are different (new key generated)
	if renewedCert.Metadata.Fingerprint == originalCert.Metadata.Fingerprint {
		t.Error("Renewed certificate should have a different fingerprint (new key)")
	}

	// Verify certificates are different
	if CompareCertificates(originalCert.PEM.Certificate, renewedCert.PEM.Certificate) {
		t.Error("Renewed certificate should be different from original")
	}
}

func TestSelfSignedProvider_Generate_TLSUsable(t *testing.T) {
	provider := NewSelfSignedProvider()

	cert, err := provider.Generate([]string{"test.local"}, GenerateOptions{})

	if err != nil {
		t.Fatalf("Failed to generate certificate: %v", err)
	}

	// Verify the TLS certificate can be used in a TLS config
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{*cert.TLSCert},
	}

	if len(tlsConfig.Certificates) != 1 {
		t.Error("Expected 1 certificate in TLS config")
	}
}

func TestSelfSignedProvider_Generate_PEMReloadable(t *testing.T) {
	provider := NewSelfSignedProvider()

	cert, err := provider.Generate([]string{"test.local"}, GenerateOptions{})

	if err != nil {
		t.Fatalf("Failed to generate certificate: %v", err)
	}

	// Verify PEM can be decoded and reloaded
	_, err = tls.X509KeyPair(cert.PEM.Certificate, cert.PEM.PrivateKey)
	if err != nil {
		t.Errorf("Failed to reload certificate from PEM: %v", err)
	}
}

func TestCompareCertificates(t *testing.T) {
	provider := NewSelfSignedProvider()

	cert1, _ := provider.Generate([]string{"test1.local"}, GenerateOptions{})
	cert2, _ := provider.Generate([]string{"test2.local"}, GenerateOptions{})

	// Same certificates should be equal
	if !CompareCertificates(cert1.PEM.Certificate, cert1.PEM.Certificate) {
		t.Error("Same certificate should be equal")
	}

	// Different certificates should not be equal
	if CompareCertificates(cert1.PEM.Certificate, cert2.PEM.Certificate) {
		t.Error("Different certificates should not be equal")
	}
}
