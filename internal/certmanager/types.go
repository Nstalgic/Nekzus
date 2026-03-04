package certmanager

import (
	"crypto"
	"crypto/tls"
	"crypto/x509"
	"time"
)

// Certificate represents a managed certificate with metadata
type Certificate struct {
	Domain   string
	TLSCert  *tls.Certificate
	PEM      CertificatePEM
	Metadata CertMetadata
}

// CertificatePEM contains PEM-encoded certificate data
type CertificatePEM struct {
	Certificate []byte
	PrivateKey  []byte
	CAChain     []byte
}

// CertMetadata contains certificate metadata
type CertMetadata struct {
	Issuer      string
	NotBefore   time.Time
	NotAfter    time.Time
	SANs        []string
	Fingerprint string // SHA-256 fingerprint for SPKI
}

// IsExpired returns true if the certificate has expired
func (c *Certificate) IsExpired() bool {
	return time.Now().After(c.Metadata.NotAfter)
}

// IsExpiringSoon returns true if the certificate expires within the given duration
func (c *Certificate) IsExpiringSoon(duration time.Duration) bool {
	return time.Until(c.Metadata.NotAfter) < duration
}

// NeedsRenewal returns true if the certificate should be renewed
// Renewal is triggered 30 days before expiry
func (c *Certificate) NeedsRenewal() bool {
	return c.IsExpiringSoon(30 * 24 * time.Hour)
}

// DaysUntilExpiry returns the number of days until the certificate expires
func (c *Certificate) DaysUntilExpiry() float64 {
	return time.Until(c.Metadata.NotAfter).Hours() / 24
}

// GenerateRequest contains parameters for certificate generation
type GenerateRequest struct {
	Domains     []string
	Provider    string // "self-signed" or "acme" (future)
	Options     GenerateOptions
	RequestedBy string // Device ID or "system"
}

// GenerateOptions contains optional parameters for certificate generation
type GenerateOptions struct {
	ValidityDays int    // Number of days certificate is valid (default: 365)
	KeyType      string // "ecdsa" or "rsa" (default: "ecdsa")
	KeySize      int    // Key size in bits (default: 256 for ECDSA, 2048 for RSA)
}

// SetDefaults sets default values for GenerateOptions
func (opts *GenerateOptions) SetDefaults() {
	if opts.ValidityDays == 0 {
		opts.ValidityDays = 365
	}
	if opts.KeyType == "" {
		opts.KeyType = "ecdsa"
	}
	if opts.KeySize == 0 {
		if opts.KeyType == "ecdsa" {
			opts.KeySize = 256
		} else {
			opts.KeySize = 2048
		}
	}
}

// StoredCertificate represents a certificate stored in the database
// This is an internal type for certmanager - storage package has its own version
type StoredCertificate struct {
	ID                      int64
	Domain                  string
	CertificatePEM          []byte
	PrivateKeyPEM           []byte
	Issuer                  string
	NotBefore               time.Time
	NotAfter                time.Time
	SubjectAlternativeNames string // JSON array
	FingerprintSHA256       string
	CreatedAt               time.Time
	UpdatedAt               time.Time
	RenewalAttemptCount     int
	LastRenewalAttempt      *time.Time
	LastRenewalError        *string
}

// ToCertificate converts a StoredCertificate to a Certificate
func (sc *StoredCertificate) ToCertificate() (*Certificate, error) {
	tlsCert, err := tls.X509KeyPair(sc.CertificatePEM, sc.PrivateKeyPEM)
	if err != nil {
		return nil, err
	}

	// Parse certificate to extract SANs
	x509Cert, err := x509.ParseCertificate(tlsCert.Certificate[0])
	if err != nil {
		return nil, err
	}

	return &Certificate{
		Domain:  sc.Domain,
		TLSCert: &tlsCert,
		PEM: CertificatePEM{
			Certificate: sc.CertificatePEM,
			PrivateKey:  sc.PrivateKeyPEM,
		},
		Metadata: CertMetadata{
			Issuer:      sc.Issuer,
			NotBefore:   sc.NotBefore,
			NotAfter:    sc.NotAfter,
			SANs:        x509Cert.DNSNames,
			Fingerprint: sc.FingerprintSHA256,
		},
	}, nil
}

// CertificateHistoryEntry represents an audit log entry
type CertificateHistoryEntry struct {
	ID                int64
	Domain            string
	Action            string // "generated", "renewed", "revoked"
	Issuer            string
	FingerprintSHA256 string
	CreatedAt         time.Time
	CreatedBy         string // Device ID or "system"
	Metadata          string // JSON for additional context
}

// CACertificate represents a Certificate Authority certificate
type CACertificate struct {
	Certificate *x509.Certificate
	PrivateKey  crypto.PrivateKey
	PEM         CertificatePEM
	Fingerprint string
	CreatedAt   time.Time
}

// CAOptions contains options for CA generation
type CAOptions struct {
	CommonName   string // Default: "Nekzus CA"
	Organization string // Default: "Nekzus"
	ValidityDays int    // Default: 3650 (10 years)
	KeyType      string // "ecdsa" or "rsa" (default: "ecdsa")
	KeySize      int    // Key size in bits (default: 256 for ECDSA, 4096 for RSA)
}

// SetDefaults sets default values for CAOptions
func (opts *CAOptions) SetDefaults() {
	if opts.CommonName == "" {
		opts.CommonName = "Nekzus CA"
	}
	if opts.Organization == "" {
		opts.Organization = "Nekzus"
	}
	if opts.ValidityDays == 0 {
		opts.ValidityDays = 3650 // 10 years
	}
	if opts.KeyType == "" {
		opts.KeyType = "ecdsa"
	}
	if opts.KeySize == 0 {
		if opts.KeyType == "ecdsa" {
			opts.KeySize = 256
		} else {
			opts.KeySize = 4096 // Stronger RSA for CA
		}
	}
}

// SignRequest contains parameters for signing a certificate with a CA
type SignRequest struct {
	Domains      []string
	ValidityDays int    // Default: 365
	KeyType      string // "ecdsa" or "rsa" (default: "ecdsa")
	KeySize      int    // Key size in bits
}

// SetDefaults sets default values for SignRequest
func (req *SignRequest) SetDefaults() {
	if req.ValidityDays == 0 {
		req.ValidityDays = 365
	}
	if req.KeyType == "" {
		req.KeyType = "ecdsa"
	}
	if req.KeySize == 0 {
		if req.KeyType == "ecdsa" {
			req.KeySize = 256
		} else {
			req.KeySize = 2048
		}
	}
}

// CertBundle contains all certificates needed for TLS
type CertBundle struct {
	CACert []byte // CA certificate PEM
	Cert   []byte // Service certificate PEM
	Key    []byte // Service private key PEM
}

// RenewRequest contains parameters for certificate renewal
type RenewRequest struct {
	Domain      string // Domain of cert to renew
	RequestedBy string // Device ID or "system"
}

// RenewResult contains the result of a certificate renewal
type RenewResult struct {
	OldCert     *Certificate // The replaced certificate (for reference)
	NewCert     *Certificate // The new certificate
	OldSPKI     string       // Old SPKI for grace period
	NewSPKI     string       // New SPKI for devices to pin
	SPKIChanged bool         // Whether the SPKI changed (key rotation)
}

// ReplaceRequest contains parameters for replacing a certificate with user-provided cert
type ReplaceRequest struct {
	Domain         string // Domain to replace (or new domain if different)
	CertificatePEM []byte // New certificate PEM
	PrivateKeyPEM  []byte // New private key PEM
	RequestedBy    string // Device ID or "system"
}

// ReplaceResult contains the result of a certificate replacement
type ReplaceResult struct {
	OldCert     *Certificate // The replaced certificate (nil if new)
	NewCert     *Certificate // The new certificate
	OldSPKI     string       // Old SPKI (empty if new)
	NewSPKI     string       // New SPKI
	SPKIChanged bool         // Whether the SPKI changed
}
