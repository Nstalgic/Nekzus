package certmanager

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/nstalgic/nekzus/internal/metrics"
)

var log = slog.With("package", "certmanager")

// Storage is the interface for certificate persistence
type Storage interface {
	StoreCertificate(cert *StoredCertificate) error
	GetCertificate(domain string) (*StoredCertificate, error)
	ListCertificates() ([]*StoredCertificate, error)
	DeleteCertificate(domain string) error
	AddCertificateHistory(entry CertificateHistoryEntry) error
}

// Config contains configuration for the certificate manager
type Config struct {
	DefaultProvider string
	AllowedDomains  []string // Whitelist of allowed domains (empty = allow all)
}

// Manager handles certificate generation, storage, and retrieval
type Manager struct {
	mu              sync.RWMutex
	certs           map[string]*Certificate
	storage         Storage
	providers       map[string]Provider
	metrics         *metrics.Metrics
	config          Config
	defaultProvider string
	fallbackCert    *tls.Certificate // Fallback certificate when no SNI match
}

// New creates a new certificate manager
func New(config Config, storage Storage, metrics *metrics.Metrics) *Manager {
	return &Manager{
		certs:           make(map[string]*Certificate),
		storage:         storage,
		providers:       make(map[string]Provider),
		metrics:         metrics,
		config:          config,
		defaultProvider: config.DefaultProvider,
	}
}

// RegisterProvider registers a certificate provider
func (m *Manager) RegisterProvider(provider Provider) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.providers[provider.Name()] = provider
}

// SetFallbackCertificate sets the default certificate to use when no SNI match is found.
// This is typically the static certificate loaded from disk at startup.
func (m *Manager) SetFallbackCertificate(cert *tls.Certificate) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.fallbackCert = cert
	log.Info("fallback certificate configured for SNI")
}

// GetFallbackCertificate returns the fallback certificate, if set
func (m *Manager) GetFallbackCertificate() *tls.Certificate {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.fallbackCert
}

// HasAnyCertificate returns true if any certificate is available (managed or fallback)
func (m *Manager) HasAnyCertificate() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Check fallback
	if m.fallbackCert != nil {
		return true
	}

	// Check managed certs in memory
	if len(m.certs) > 0 {
		return true
	}

	// Check storage
	if m.storage != nil {
		certs, err := m.storage.ListCertificates()
		if err == nil && len(certs) > 0 {
			return true
		}
	}

	return false
}

// GetFirstAvailableCertificate returns any available certificate for use as a default
// Prefers localhost, then any other certificate
func (m *Manager) GetFirstAvailableCertificate() *tls.Certificate {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Try localhost first (most common for local development)
	preferredDomains := []string{"localhost", "localhost.local"}
	for _, domain := range preferredDomains {
		if cert, exists := m.certs[domain]; exists && !cert.IsExpired() {
			return cert.TLSCert
		}
	}

	// Try any certificate in memory
	for _, cert := range m.certs {
		if !cert.IsExpired() {
			return cert.TLSCert
		}
	}

	// Try loading from storage
	if m.storage != nil {
		// Try preferred domains from storage
		for _, domain := range preferredDomains {
			storedCert, err := m.storage.GetCertificate(domain)
			if err == nil && storedCert != nil {
				cert, err := storedCert.ToCertificate()
				if err == nil && !cert.IsExpired() {
					return cert.TLSCert
				}
			}
		}

		// List all and return first valid
		certs, err := m.storage.ListCertificates()
		if err == nil {
			for _, storedCert := range certs {
				cert, err := storedCert.ToCertificate()
				if err == nil && !cert.IsExpired() {
					return cert.TLSCert
				}
			}
		}
	}

	// Fall back to configured fallback cert
	return m.fallbackCert
}

// Generate creates a new certificate for the given domains
func (m *Manager) Generate(req GenerateRequest) (*Certificate, error) {
	// Validate input
	if len(req.Domains) == 0 {
		return nil, fmt.Errorf("at least one domain is required")
	}

	// Set defaults
	req.Options.SetDefaults()

	// Select provider
	providerName := req.Provider
	if providerName == "" {
		providerName = m.defaultProvider
	}

	m.mu.RLock()
	provider, ok := m.providers[providerName]
	m.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("provider not found: %s", providerName)
	}

	// Validate domains with provider
	if err := provider.Validate(req.Domains); err != nil {
		return nil, fmt.Errorf("domain validation failed: %w", err)
	}

	// Generate certificate
	start := time.Now()
	cert, err := provider.Generate(req.Domains, req.Options)
	duration := time.Since(start)

	if err != nil {
		if m.metrics != nil {
			m.metrics.RecordCertificateGenerationError(providerName, "generation_failed")
		}
		return nil, fmt.Errorf("certificate generation failed: %w", err)
	}

	if m.metrics != nil {
		m.metrics.RecordCertificateGeneration(providerName, "success", duration)
		m.metrics.SetCertificateExpiry(cert.Domain, cert.Metadata.Issuer, time.Until(cert.Metadata.NotAfter).Seconds())
	}

	log.Info("Generated certificate", "domain", req.Domains[0], "duration", duration)

	// Store in memory
	m.mu.Lock()
	m.certs[cert.Domain] = cert
	m.mu.Unlock()

	// Store in database
	if m.storage != nil {
		storedCert := m.toStoredCertificate(cert)
		if err := m.storage.StoreCertificate(storedCert); err != nil {
			log.Warn("Failed to persist certificate", "error", err)
		}

		// Add history entry
		historyEntry := CertificateHistoryEntry{
			Domain:            cert.Domain,
			Action:            "generated",
			Issuer:            cert.Metadata.Issuer,
			FingerprintSHA256: cert.Metadata.Fingerprint,
			CreatedAt:         time.Now(),
			CreatedBy:         req.RequestedBy,
		}
		if err := m.storage.AddCertificateHistory(historyEntry); err != nil {
			log.Warn("Failed to add history entry", "error", err)
		}
	}

	return cert, nil
}

// GetCertificate is called by tls.Config for SNI-based certificate selection
func (m *Manager) GetCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	domain := hello.ServerName

	m.mu.RLock()
	cert, exists := m.certs[domain]
	m.mu.RUnlock()

	// Certificate exists in memory and is not expired
	if exists && !cert.IsExpired() {
		if m.metrics != nil {
			m.metrics.IncrementCertificateServe()
		}
		return cert.TLSCert, nil
	}

	// Try to load from storage
	if m.storage != nil {
		storedCert, err := m.storage.GetCertificate(domain)
		if err == nil && storedCert != nil {
			cert, err := storedCert.ToCertificate()
			if err == nil && !cert.IsExpired() {
				// Cache in memory
				m.mu.Lock()
				m.certs[domain] = cert
				m.mu.Unlock()

				if m.metrics != nil {
					m.metrics.IncrementCertificateServe()
				}
				return cert.TLSCert, nil
			}
		}
	}

	// No SNI-specific certificate found, try fallback
	m.mu.RLock()
	fallback := m.fallbackCert
	m.mu.RUnlock()

	if fallback != nil {
		// Use fallback certificate (static cert from disk)
		log.Debug("using fallback certificate for SNI",
			"requested_domain", domain)
		return fallback, nil
	}

	// Try to find any available certificate (for cases like localhost without SNI)
	// This is called without holding the lock since GetFirstAvailableCertificate acquires it
	if anyCert := m.GetFirstAvailableCertificate(); anyCert != nil {
		log.Debug("using first available certificate for SNI",
			"requested_domain", domain)
		return anyCert, nil
	}

	// No valid certificate available
	if m.metrics != nil {
		m.metrics.IncrementCertificateMiss()
	}
	return nil, fmt.Errorf("no valid certificate for domain: %s", domain)
}

// Get returns a certificate for the given domain
func (m *Manager) Get(domain string) (*Certificate, error) {
	m.mu.RLock()
	cert, exists := m.certs[domain]
	m.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("certificate not found: %s", domain)
	}

	return cert, nil
}

// List returns all certificates
func (m *Manager) List() []*Certificate {
	m.mu.RLock()
	defer m.mu.RUnlock()

	certs := make([]*Certificate, 0, len(m.certs))
	for _, cert := range m.certs {
		certs = append(certs, cert)
	}

	return certs
}

// Delete removes a certificate
func (m *Manager) Delete(domain string) error {
	// Remove from memory
	m.mu.Lock()
	delete(m.certs, domain)
	m.mu.Unlock()

	// Remove from storage
	if m.storage != nil {
		if err := m.storage.DeleteCertificate(domain); err != nil {
			return fmt.Errorf("failed to delete certificate from storage: %w", err)
		}

		// Add history entry
		historyEntry := CertificateHistoryEntry{
			Domain:    domain,
			Action:    "revoked",
			CreatedAt: time.Now(),
			CreatedBy: "system",
		}
		if err := m.storage.AddCertificateHistory(historyEntry); err != nil {
			log.Warn("Failed to add history entry", "error", err)
		}
	}

	return nil
}

// LoadFromStorage loads all certificates from storage into memory
func (m *Manager) LoadFromStorage() error {
	if m.storage == nil {
		return nil
	}

	storedCerts, err := m.storage.ListCertificates()
	if err != nil {
		return fmt.Errorf("failed to list certificates from storage: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	loaded := 0
	for _, storedCert := range storedCerts {
		cert, err := storedCert.ToCertificate()
		if err != nil {
			log.Warn("Failed to load certificate", "domain", storedCert.Domain, "error", err)
			continue
		}

		// Skip expired certificates
		if cert.IsExpired() {
			log.Info("Skipping expired certificate", "domain", cert.Domain)
			continue
		}

		m.certs[cert.Domain] = cert
		loaded++
	}

	log.Info("Loaded certificates from storage", "count", loaded)
	return nil
}

// GetAllCertificates returns all certificates (used for health checks)
func (m *Manager) GetAllCertificates() []*Certificate {
	return m.List()
}

// GetSPKIPins returns the SPKI pins for all available certificates.
// Returns up to 2 pins (primary + backup) for TrustKit iOS compatibility.
// The first pin is the currently active certificate, the second is a backup if available.
func (m *Manager) GetSPKIPins() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	pins := make([]string, 0, 2)
	seenPins := make(map[string]bool)

	// Helper to add unique pins
	addPin := func(spki string) {
		if spki != "" && !seenPins[spki] {
			pins = append(pins, spki)
			seenPins[spki] = true
		}
	}

	// First, try to get SPKI from managed certificates
	for _, cert := range m.certs {
		if cert.TLSCert != nil && len(cert.TLSCert.Certificate) > 0 {
			x509Cert, err := x509.ParseCertificate(cert.TLSCert.Certificate[0])
			if err == nil {
				spkiDER, err := x509.MarshalPKIXPublicKey(x509Cert.PublicKey)
				if err == nil {
					hash := sha256.Sum256(spkiDER)
					spki := "sha256/" + base64.StdEncoding.EncodeToString(hash[:])
					addPin(spki)
				}
			}
		}
		if len(pins) >= 2 {
			return pins
		}
	}

	// Then try fallback certificate
	if m.fallbackCert != nil && len(m.fallbackCert.Certificate) > 0 {
		x509Cert, err := x509.ParseCertificate(m.fallbackCert.Certificate[0])
		if err == nil {
			spkiDER, err := x509.MarshalPKIXPublicKey(x509Cert.PublicKey)
			if err == nil {
				hash := sha256.Sum256(spkiDER)
				spki := "sha256/" + base64.StdEncoding.EncodeToString(hash[:])
				addPin(spki)
			}
		}
	}

	return pins
}

// Renew generates a new certificate for an existing domain, replacing the old one.
// This is used for Nexus-generated (self-signed) certificates.
func (m *Manager) Renew(req RenewRequest) (*RenewResult, error) {
	if req.Domain == "" {
		return nil, fmt.Errorf("domain is required")
	}

	// Get the existing certificate
	m.mu.RLock()
	oldCert, exists := m.certs[req.Domain]
	m.mu.RUnlock()

	if !exists {
		// Try loading from storage
		if m.storage != nil {
			storedCert, err := m.storage.GetCertificate(req.Domain)
			if err != nil || storedCert == nil {
				return nil, fmt.Errorf("certificate not found: %s", req.Domain)
			}
			oldCert, err = storedCert.ToCertificate()
			if err != nil {
				return nil, fmt.Errorf("failed to load certificate: %w", err)
			}
		} else {
			return nil, fmt.Errorf("certificate not found: %s", req.Domain)
		}
	}

	// Store old SPKI for comparison
	oldSPKI := oldCert.Metadata.Fingerprint

	// Generate new certificate with same domains
	newCert, err := m.Generate(GenerateRequest{
		Domains:     oldCert.Metadata.SANs,
		Provider:    m.defaultProvider,
		RequestedBy: req.RequestedBy,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to generate replacement certificate: %w", err)
	}

	// Add history entry for renewal
	if m.storage != nil {
		historyEntry := CertificateHistoryEntry{
			Domain:            req.Domain,
			Action:            "renewed",
			Issuer:            newCert.Metadata.Issuer,
			FingerprintSHA256: newCert.Metadata.Fingerprint,
			CreatedAt:         time.Now(),
			CreatedBy:         req.RequestedBy,
		}
		if err := m.storage.AddCertificateHistory(historyEntry); err != nil {
			log.Warn("Failed to add renewal history entry", "error", err)
		}
	}

	newSPKI := newCert.Metadata.Fingerprint
	spkiChanged := oldSPKI != newSPKI

	log.Info("Certificate renewed",
		"domain", req.Domain,
		"old_spki", oldSPKI[:20]+"...",
		"new_spki", newSPKI[:20]+"...",
		"spki_changed", spkiChanged)

	return &RenewResult{
		OldCert:     oldCert,
		NewCert:     newCert,
		OldSPKI:     oldSPKI,
		NewSPKI:     newSPKI,
		SPKIChanged: spkiChanged,
	}, nil
}

// Replace imports a user-provided certificate to replace an existing one.
// This is used for user-managed certificates (corp CA, purchased, etc.).
func (m *Manager) Replace(req ReplaceRequest) (*ReplaceResult, error) {
	if req.Domain == "" {
		return nil, fmt.Errorf("domain is required")
	}
	if len(req.CertificatePEM) == 0 {
		return nil, fmt.Errorf("certificate PEM is required")
	}
	if len(req.PrivateKeyPEM) == 0 {
		return nil, fmt.Errorf("private key PEM is required")
	}

	// Parse and validate the new certificate
	tlsCert, err := tls.X509KeyPair(req.CertificatePEM, req.PrivateKeyPEM)
	if err != nil {
		return nil, fmt.Errorf("invalid certificate/key pair: %w", err)
	}

	x509Cert, err := x509.ParseCertificate(tlsCert.Certificate[0])
	if err != nil {
		return nil, fmt.Errorf("failed to parse certificate: %w", err)
	}

	// Check if certificate is already expired
	if time.Now().After(x509Cert.NotAfter) {
		return nil, fmt.Errorf("certificate is already expired")
	}

	// Calculate new SPKI
	newSPKI := calculateFingerprint(x509Cert)

	// Get old certificate if it exists
	var oldCert *Certificate
	var oldSPKI string

	m.mu.RLock()
	if existing, exists := m.certs[req.Domain]; exists {
		oldCert = existing
		oldSPKI = existing.Metadata.Fingerprint
	}
	m.mu.RUnlock()

	// If not in memory, try storage
	if oldCert == nil && m.storage != nil {
		storedCert, err := m.storage.GetCertificate(req.Domain)
		if err == nil && storedCert != nil {
			oldCert, _ = storedCert.ToCertificate()
			if oldCert != nil {
				oldSPKI = oldCert.Metadata.Fingerprint
			}
		}
	}

	// Build new certificate object
	newCert := &Certificate{
		Domain:  req.Domain,
		TLSCert: &tlsCert,
		PEM: CertificatePEM{
			Certificate: req.CertificatePEM,
			PrivateKey:  req.PrivateKeyPEM,
		},
		Metadata: CertMetadata{
			Issuer:      x509Cert.Issuer.CommonName,
			NotBefore:   x509Cert.NotBefore,
			NotAfter:    x509Cert.NotAfter,
			SANs:        x509Cert.DNSNames,
			Fingerprint: newSPKI,
		},
	}

	// Store in memory
	m.mu.Lock()
	m.certs[req.Domain] = newCert
	m.mu.Unlock()

	// Store in database
	if m.storage != nil {
		storedCert := m.toStoredCertificate(newCert)
		if err := m.storage.StoreCertificate(storedCert); err != nil {
			log.Warn("Failed to persist replaced certificate", "error", err)
		}

		// Add history entry
		action := "replaced"
		if oldCert == nil {
			action = "imported"
		}
		historyEntry := CertificateHistoryEntry{
			Domain:            req.Domain,
			Action:            action,
			Issuer:            newCert.Metadata.Issuer,
			FingerprintSHA256: newSPKI,
			CreatedAt:         time.Now(),
			CreatedBy:         req.RequestedBy,
		}
		if err := m.storage.AddCertificateHistory(historyEntry); err != nil {
			log.Warn("Failed to add replacement history entry", "error", err)
		}
	}

	// Record metrics
	if m.metrics != nil {
		m.metrics.SetCertificateExpiry(newCert.Domain, newCert.Metadata.Issuer, time.Until(newCert.Metadata.NotAfter).Seconds())
	}

	spkiChanged := oldSPKI != "" && oldSPKI != newSPKI

	log.Info("Certificate replaced",
		"domain", req.Domain,
		"issuer", newCert.Metadata.Issuer,
		"old_spki", truncateSPKI(oldSPKI),
		"new_spki", truncateSPKI(newSPKI),
		"spki_changed", spkiChanged)

	return &ReplaceResult{
		OldCert:     oldCert,
		NewCert:     newCert,
		OldSPKI:     oldSPKI,
		NewSPKI:     newSPKI,
		SPKIChanged: spkiChanged,
	}, nil
}

// IsActiveCertificate checks if the given domain is the currently active TLS certificate
func (m *Manager) IsActiveCertificate(domain string) bool {
	activeCert := m.GetFirstAvailableCertificate()
	if activeCert == nil {
		return false
	}

	// Parse to get the domain
	if len(activeCert.Certificate) == 0 {
		return false
	}

	x509Cert, err := x509.ParseCertificate(activeCert.Certificate[0])
	if err != nil {
		return false
	}

	// Check if domain matches CN or SANs
	if x509Cert.Subject.CommonName == domain {
		return true
	}
	for _, san := range x509Cert.DNSNames {
		if san == domain {
			return true
		}
	}

	return false
}

// truncateSPKI safely truncates an SPKI for logging
func truncateSPKI(spki string) string {
	if spki == "" {
		return "(none)"
	}
	if len(spki) > 20 {
		return spki[:20] + "..."
	}
	return spki
}

// calculateFingerprint calculates the SHA-256 SPKI fingerprint for a certificate
func calculateFingerprint(cert *x509.Certificate) string {
	spkiDER, err := x509.MarshalPKIXPublicKey(cert.PublicKey)
	if err != nil {
		return ""
	}
	hash := sha256.Sum256(spkiDER)
	return "sha256/" + base64.StdEncoding.EncodeToString(hash[:])
}

// toStoredCertificate converts a Certificate to StoredCertificate for database storage
func (m *Manager) toStoredCertificate(cert *Certificate) *StoredCertificate {
	// Convert SANs to JSON
	sansJSON, _ := json.Marshal(cert.Metadata.SANs)

	now := time.Now()

	return &StoredCertificate{
		Domain:                  cert.Domain,
		CertificatePEM:          cert.PEM.Certificate,
		PrivateKeyPEM:           cert.PEM.PrivateKey,
		Issuer:                  cert.Metadata.Issuer,
		NotBefore:               cert.Metadata.NotBefore,
		NotAfter:                cert.Metadata.NotAfter,
		SubjectAlternativeNames: string(sansJSON),
		FingerprintSHA256:       cert.Metadata.Fingerprint,
		CreatedAt:               now,
		UpdatedAt:               now,
		RenewalAttemptCount:     0,
	}
}

// parseTLSCert parses a PEM-encoded certificate and returns a tls.Certificate
func parseTLSCert(certPEM, keyPEM []byte) (*tls.Certificate, error) {
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, err
	}
	return &cert, nil
}

// parseX509Cert parses a tls.Certificate and returns the X.509 certificate
func parseX509Cert(tlsCert *tls.Certificate) (*x509.Certificate, error) {
	if len(tlsCert.Certificate) == 0 {
		return nil, fmt.Errorf("no certificates in chain")
	}

	return x509.ParseCertificate(tlsCert.Certificate[0])
}
