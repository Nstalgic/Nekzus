package certmanager

import (
	"crypto/tls"
	"testing"
	"time"
)

// Mock storage for testing
type mockStorage struct {
	certificates map[string]*StoredCertificate
	history      []CertificateHistoryEntry
}

func (m *mockStorage) StoreCertificate(cert *StoredCertificate) error {
	if m.certificates == nil {
		m.certificates = make(map[string]*StoredCertificate)
	}
	m.certificates[cert.Domain] = cert
	return nil
}

func (m *mockStorage) GetCertificate(domain string) (*StoredCertificate, error) {
	cert, ok := m.certificates[domain]
	if !ok {
		return nil, nil
	}
	return cert, nil
}

func (m *mockStorage) ListCertificates() ([]*StoredCertificate, error) {
	certs := make([]*StoredCertificate, 0, len(m.certificates))
	for _, cert := range m.certificates {
		certs = append(certs, cert)
	}
	return certs, nil
}

func (m *mockStorage) DeleteCertificate(domain string) error {
	delete(m.certificates, domain)
	return nil
}

func (m *mockStorage) AddCertificateHistory(entry CertificateHistoryEntry) error {
	m.history = append(m.history, entry)
	return nil
}

// Mock provider for testing
type mockProvider struct {
	name         string
	generateFunc func(domains []string, opts GenerateOptions) (*Certificate, error)
	renewFunc    func(cert *Certificate) (*Certificate, error)
	validateFunc func(domains []string) error
}

func (m *mockProvider) Generate(domains []string, opts GenerateOptions) (*Certificate, error) {
	if m.generateFunc != nil {
		return m.generateFunc(domains, opts)
	}
	return nil, nil
}

func (m *mockProvider) Renew(cert *Certificate) (*Certificate, error) {
	if m.renewFunc != nil {
		return m.renewFunc(cert)
	}
	return nil, nil
}

func (m *mockProvider) Validate(domains []string) error {
	if m.validateFunc != nil {
		return m.validateFunc(domains)
	}
	return nil
}

func (m *mockProvider) Name() string {
	return m.name
}

func TestNewManager(t *testing.T) {
	storage := &mockStorage{}

	manager := New(Config{
		DefaultProvider: "mock",
	}, storage, nil)

	if manager == nil {
		t.Fatal("Expected non-nil manager")
	}

	if len(manager.certs) != 0 {
		t.Errorf("Expected empty cert map, got: %d certs", len(manager.certs))
	}
}

func TestManager_RegisterProvider(t *testing.T) {
	storage := &mockStorage{}
	manager := New(Config{}, storage, nil)

	provider := &mockProvider{name: "test-provider"}
	manager.RegisterProvider(provider)

	if _, ok := manager.providers["test-provider"]; !ok {
		t.Error("Provider not registered")
	}
}

func TestManager_Generate_Success(t *testing.T) {
	storage := &mockStorage{}
	manager := New(Config{DefaultProvider: "mock"}, storage, nil)

	// Register mock provider that generates a certificate
	provider := &mockProvider{
		name: "mock",
		generateFunc: func(domains []string, opts GenerateOptions) (*Certificate, error) {
			return &Certificate{
				Domain: domains[0],
				Metadata: CertMetadata{
					Issuer:    "mock",
					NotBefore: time.Now(),
					NotAfter:  time.Now().Add(365 * 24 * time.Hour),
					SANs:      domains,
				},
			}, nil
		},
	}
	manager.RegisterProvider(provider)

	// Generate certificate
	cert, err := manager.Generate(GenerateRequest{
		Domains:  []string{"test.local"},
		Provider: "mock",
	})

	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if cert == nil {
		t.Fatal("Expected non-nil certificate")
	}

	if cert.Domain != "test.local" {
		t.Errorf("Expected domain test.local, got: %s", cert.Domain)
	}
}

func TestManager_Generate_ValidatesEmptyDomains(t *testing.T) {
	storage := &mockStorage{}
	manager := New(Config{DefaultProvider: "mock"}, storage, nil)

	provider := &mockProvider{name: "mock"}
	manager.RegisterProvider(provider)

	// Try to generate with empty domains
	_, err := manager.Generate(GenerateRequest{
		Domains:  []string{},
		Provider: "mock",
	})

	if err == nil {
		t.Error("Expected error for empty domains")
	}
}

func TestManager_GetCertificate_SNI(t *testing.T) {
	storage := &mockStorage{}
	manager := New(Config{DefaultProvider: "mock"}, storage, nil)

	// Register mock provider
	provider := &mockProvider{
		name: "mock",
		generateFunc: func(domains []string, opts GenerateOptions) (*Certificate, error) {
			// Create a real TLS certificate for testing
			tlsCert := &tls.Certificate{
				Certificate: [][]byte{[]byte("mock-cert")},
			}
			return &Certificate{
				Domain:  domains[0],
				TLSCert: tlsCert,
				Metadata: CertMetadata{
					Issuer:    "mock",
					NotBefore: time.Now(),
					NotAfter:  time.Now().Add(365 * 24 * time.Hour),
					SANs:      domains,
				},
			}, nil
		},
	}
	manager.RegisterProvider(provider)

	// Generate certificate for specific domain
	_, err := manager.Generate(GenerateRequest{
		Domains:  []string{"app1.local"},
		Provider: "mock",
	})
	if err != nil {
		t.Fatalf("Failed to generate certificate: %v", err)
	}

	// Test SNI selection
	hello := &tls.ClientHelloInfo{ServerName: "app1.local"}
	cert, err := manager.GetCertificate(hello)

	if err != nil {
		t.Fatalf("Expected certificate, got error: %v", err)
	}

	if cert == nil {
		t.Fatal("Expected non-nil certificate")
	}
}

func TestManager_GetCertificate_NotFound(t *testing.T) {
	storage := &mockStorage{}
	manager := New(Config{}, storage, nil)

	// Request certificate for domain that doesn't exist (no fallback set)
	hello := &tls.ClientHelloInfo{ServerName: "nonexistent.local"}
	cert, err := manager.GetCertificate(hello)

	if err == nil {
		t.Error("Expected error for nonexistent domain without fallback")
	}

	if cert != nil {
		t.Error("Expected nil certificate for nonexistent domain without fallback")
	}
}

func TestManager_GetCertificate_Fallback(t *testing.T) {
	storage := &mockStorage{}
	manager := New(Config{DefaultProvider: "self-signed"}, storage, nil)
	manager.RegisterProvider(NewSelfSignedProvider())

	// Generate a fallback certificate
	fallbackCert, err := manager.Generate(GenerateRequest{
		Domains: []string{"fallback.local"},
	})
	if err != nil {
		t.Fatalf("Failed to generate fallback cert: %v", err)
	}

	// Set the fallback certificate
	manager.SetFallbackCertificate(fallbackCert.TLSCert)

	// Verify GetFallbackCertificate works
	if manager.GetFallbackCertificate() == nil {
		t.Error("Expected fallback certificate to be set")
	}

	// Request certificate for domain that doesn't exist
	hello := &tls.ClientHelloInfo{ServerName: "unknown.local"}
	cert, err := manager.GetCertificate(hello)

	// Should succeed and return fallback
	if err != nil {
		t.Errorf("Expected no error with fallback set, got: %v", err)
	}

	if cert == nil {
		t.Error("Expected fallback certificate to be returned")
	}

	// The returned cert should be the fallback
	if cert != fallbackCert.TLSCert {
		t.Error("Expected returned certificate to be the fallback")
	}
}

func TestManager_GetCertificate_SNI_Priority(t *testing.T) {
	storage := &mockStorage{}
	manager := New(Config{DefaultProvider: "self-signed"}, storage, nil)
	manager.RegisterProvider(NewSelfSignedProvider())

	// Generate a fallback certificate
	fallbackCert, err := manager.Generate(GenerateRequest{
		Domains: []string{"fallback.local"},
	})
	if err != nil {
		t.Fatalf("Failed to generate fallback cert: %v", err)
	}
	manager.SetFallbackCertificate(fallbackCert.TLSCert)

	// Generate a specific certificate for a domain
	specificCert, err := manager.Generate(GenerateRequest{
		Domains: []string{"specific.local"},
	})
	if err != nil {
		t.Fatalf("Failed to generate specific cert: %v", err)
	}

	// Request the specific domain - should get specific cert, not fallback
	hello := &tls.ClientHelloInfo{ServerName: "specific.local"}
	cert, err := manager.GetCertificate(hello)

	if err != nil {
		t.Fatalf("GetCertificate failed: %v", err)
	}

	// Should return the specific certificate, not the fallback
	if cert == fallbackCert.TLSCert {
		t.Error("Expected specific certificate, got fallback")
	}

	if cert != specificCert.TLSCert {
		t.Error("Expected specific certificate to be returned")
	}

	// Request unknown domain - should get fallback
	hello2 := &tls.ClientHelloInfo{ServerName: "unknown.local"}
	cert2, err := manager.GetCertificate(hello2)

	if err != nil {
		t.Fatalf("GetCertificate for unknown domain failed: %v", err)
	}

	if cert2 != fallbackCert.TLSCert {
		t.Error("Expected fallback certificate for unknown domain")
	}
}

func TestManager_StoragePersistence(t *testing.T) {
	storage := &mockStorage{}

	// Create first manager instance
	manager1 := New(Config{DefaultProvider: "self-signed"}, storage, nil)

	// Use real self-signed provider
	provider := NewSelfSignedProvider()
	manager1.RegisterProvider(provider)

	// Generate certificate
	cert1, err := manager1.Generate(GenerateRequest{
		Domains:  []string{"persistent.local"},
		Provider: "self-signed",
	})
	if err != nil {
		t.Fatalf("Failed to generate certificate: %v", err)
	}

	// Simulate restart - create new manager instance with same storage
	manager2 := New(Config{DefaultProvider: "self-signed"}, storage, nil)
	manager2.RegisterProvider(NewSelfSignedProvider())

	// Load certificates from storage
	if err := manager2.LoadFromStorage(); err != nil {
		t.Fatalf("Failed to load from storage: %v", err)
	}

	// Certificate should be available
	hello := &tls.ClientHelloInfo{ServerName: "persistent.local"}
	cert2, err := manager2.GetCertificate(hello)

	if err != nil {
		t.Fatalf("Certificate not restored from storage: %v", err)
	}

	if cert2 == nil {
		t.Fatal("Expected non-nil certificate after reload")
	}

	// Fingerprint should be non-empty
	if cert1.Metadata.Fingerprint == "" {
		t.Error("Expected non-empty fingerprint")
	}
}

func TestManager_List(t *testing.T) {
	storage := &mockStorage{}
	manager := New(Config{DefaultProvider: "mock"}, storage, nil)

	provider := &mockProvider{
		name: "mock",
		generateFunc: func(domains []string, opts GenerateOptions) (*Certificate, error) {
			return &Certificate{
				Domain: domains[0],
				Metadata: CertMetadata{
					Issuer:    "mock",
					NotBefore: time.Now(),
					NotAfter:  time.Now().Add(365 * 24 * time.Hour),
					SANs:      domains,
				},
			}, nil
		},
	}
	manager.RegisterProvider(provider)

	// Generate multiple certificates
	domains := []string{"app1.local", "app2.local", "app3.local"}
	for _, domain := range domains {
		_, err := manager.Generate(GenerateRequest{
			Domains:  []string{domain},
			Provider: "mock",
		})
		if err != nil {
			t.Fatalf("Failed to generate certificate for %s: %v", domain, err)
		}
	}

	// List all certificates
	certs := manager.List()

	if len(certs) != 3 {
		t.Errorf("Expected 3 certificates, got: %d", len(certs))
	}

	// Verify all domains are present
	domainMap := make(map[string]bool)
	for _, cert := range certs {
		domainMap[cert.Domain] = true
	}

	for _, domain := range domains {
		if !domainMap[domain] {
			t.Errorf("Domain %s not found in list", domain)
		}
	}
}

func TestManager_Delete(t *testing.T) {
	storage := &mockStorage{}
	manager := New(Config{DefaultProvider: "mock"}, storage, nil)

	provider := &mockProvider{
		name: "mock",
		generateFunc: func(domains []string, opts GenerateOptions) (*Certificate, error) {
			return &Certificate{
				Domain: domains[0],
				Metadata: CertMetadata{
					Issuer:    "mock",
					NotBefore: time.Now(),
					NotAfter:  time.Now().Add(365 * 24 * time.Hour),
				},
			}, nil
		},
	}
	manager.RegisterProvider(provider)

	// Generate certificate
	_, err := manager.Generate(GenerateRequest{
		Domains:  []string{"to-delete.local"},
		Provider: "mock",
	})
	if err != nil {
		t.Fatalf("Failed to generate certificate: %v", err)
	}

	// Verify it exists
	hello := &tls.ClientHelloInfo{ServerName: "to-delete.local"}
	_, err = manager.GetCertificate(hello)
	if err != nil {
		t.Error("Certificate should exist before deletion")
	}

	// Delete certificate
	err = manager.Delete("to-delete.local")
	if err != nil {
		t.Fatalf("Failed to delete certificate: %v", err)
	}

	// Verify it's gone
	_, err = manager.GetCertificate(hello)
	if err == nil {
		t.Error("Certificate should not exist after deletion")
	}
}

func TestCertificate_IsExpired(t *testing.T) {
	cert := &Certificate{
		Metadata: CertMetadata{
			NotAfter: time.Now().Add(-24 * time.Hour), // Expired yesterday
		},
	}

	if !cert.IsExpired() {
		t.Error("Certificate should be expired")
	}
}

func TestCertificate_IsExpiringSoon(t *testing.T) {
	cert := &Certificate{
		Metadata: CertMetadata{
			NotAfter: time.Now().Add(15 * 24 * time.Hour), // Expires in 15 days
		},
	}

	if !cert.IsExpiringSoon(30 * 24 * time.Hour) {
		t.Error("Certificate should be expiring soon (within 30 days)")
	}

	if cert.IsExpiringSoon(7 * 24 * time.Hour) {
		t.Error("Certificate should not be expiring soon (within 7 days)")
	}
}

func TestCertificate_NeedsRenewal(t *testing.T) {
	tests := []struct {
		name        string
		expiresIn   time.Duration
		shouldRenew bool
	}{
		{"Expires in 60 days", 60 * 24 * time.Hour, false},
		{"Expires in 29 days", 29 * 24 * time.Hour, true},
		{"Expires in 1 day", 24 * time.Hour, true},
		{"Already expired", -24 * time.Hour, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cert := &Certificate{
				Metadata: CertMetadata{
					NotAfter: time.Now().Add(tt.expiresIn),
				},
			}

			if cert.NeedsRenewal() != tt.shouldRenew {
				t.Errorf("Expected NeedsRenewal() = %v for %s", tt.shouldRenew, tt.name)
			}
		})
	}
}

func TestGenerateOptions_SetDefaults(t *testing.T) {
	tests := []struct {
		name     string
		input    GenerateOptions
		expected GenerateOptions
	}{
		{
			name:  "Empty options",
			input: GenerateOptions{},
			expected: GenerateOptions{
				ValidityDays: 365,
				KeyType:      "ecdsa",
				KeySize:      256,
			},
		},
		{
			name: "RSA options",
			input: GenerateOptions{
				KeyType: "rsa",
			},
			expected: GenerateOptions{
				ValidityDays: 365,
				KeyType:      "rsa",
				KeySize:      2048,
			},
		},
		{
			name: "Custom validity",
			input: GenerateOptions{
				ValidityDays: 730,
			},
			expected: GenerateOptions{
				ValidityDays: 730,
				KeyType:      "ecdsa",
				KeySize:      256,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := tt.input
			opts.SetDefaults()

			if opts.ValidityDays != tt.expected.ValidityDays {
				t.Errorf("ValidityDays: expected %d, got %d", tt.expected.ValidityDays, opts.ValidityDays)
			}
			if opts.KeyType != tt.expected.KeyType {
				t.Errorf("KeyType: expected %s, got %s", tt.expected.KeyType, opts.KeyType)
			}
			if opts.KeySize != tt.expected.KeySize {
				t.Errorf("KeySize: expected %d, got %d", tt.expected.KeySize, opts.KeySize)
			}
		})
	}
}
