package certmanager

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"time"
)

// SelfSignedProvider generates self-signed certificates
type SelfSignedProvider struct{}

// NewSelfSignedProvider creates a new self-signed certificate provider
func NewSelfSignedProvider() *SelfSignedProvider {
	return &SelfSignedProvider{}
}

// Name returns the provider identifier
func (p *SelfSignedProvider) Name() string {
	return "self-signed"
}

// Validate checks if this provider can issue certs for these domains
func (p *SelfSignedProvider) Validate(domains []string) error {
	if len(domains) == 0 {
		return fmt.Errorf("at least one domain is required")
	}
	return nil
}

// Generate creates a new self-signed certificate
func (p *SelfSignedProvider) Generate(domains []string, opts GenerateOptions) (*Certificate, error) {
	if err := p.Validate(domains); err != nil {
		return nil, err
	}

	// Set defaults
	opts.SetDefaults()

	// Generate private key
	var privateKey interface{}
	var err error

	if opts.KeyType == "rsa" {
		privateKey, err = rsa.GenerateKey(rand.Reader, opts.KeySize)
	} else {
		// Default to ECDSA P-256
		privateKey, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to generate private key: %w", err)
	}

	// Create certificate template
	notBefore := time.Now()
	notAfter := notBefore.Add(time.Duration(opts.ValidityDays) * 24 * time.Hour)

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return nil, fmt.Errorf("failed to generate serial number: %w", err)
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName:   domains[0],
			Organization: []string{"Nekzus"},
		},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	// Add all domains as SANs
	for _, domain := range domains {
		if ip := net.ParseIP(domain); ip != nil {
			template.IPAddresses = append(template.IPAddresses, ip)
		} else {
			template.DNSNames = append(template.DNSNames, domain)
		}
	}

	// Create certificate
	var publicKey interface{}
	switch k := privateKey.(type) {
	case *ecdsa.PrivateKey:
		publicKey = &k.PublicKey
	case *rsa.PrivateKey:
		publicKey = &k.PublicKey
	default:
		return nil, fmt.Errorf("unsupported key type")
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, publicKey, privateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create certificate: %w", err)
	}

	// Encode certificate to PEM
	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: derBytes,
	})

	// Encode private key to PEM
	var keyPEM []byte
	switch k := privateKey.(type) {
	case *ecdsa.PrivateKey:
		privBytes, err := x509.MarshalECPrivateKey(k)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal EC private key: %w", err)
		}
		keyPEM = pem.EncodeToMemory(&pem.Block{
			Type:  "EC PRIVATE KEY",
			Bytes: privBytes,
		})
	case *rsa.PrivateKey:
		privBytes := x509.MarshalPKCS1PrivateKey(k)
		keyPEM = pem.EncodeToMemory(&pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: privBytes,
		})
	}

	// Create TLS certificate
	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("failed to create TLS certificate: %w", err)
	}

	// Parse X.509 certificate for metadata
	x509Cert, err := x509.ParseCertificate(tlsCert.Certificate[0])
	if err != nil {
		return nil, fmt.Errorf("failed to parse certificate: %w", err)
	}

	// Calculate SPKI fingerprint (for certificate pinning)
	spki, err := x509.MarshalPKIXPublicKey(x509Cert.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal public key: %w", err)
	}
	hash := sha256.Sum256(spki)
	fingerprint := "sha256/" + base64.StdEncoding.EncodeToString(hash[:])

	// Create certificate metadata
	metadata := CertMetadata{
		Issuer:      "self-signed",
		NotBefore:   x509Cert.NotBefore,
		NotAfter:    x509Cert.NotAfter,
		SANs:        domains,
		Fingerprint: fingerprint,
	}

	return &Certificate{
		Domain:  domains[0],
		TLSCert: &tlsCert,
		PEM: CertificatePEM{
			Certificate: certPEM,
			PrivateKey:  keyPEM,
		},
		Metadata: metadata,
	}, nil
}

// Renew creates a new certificate to replace an expiring one
func (p *SelfSignedProvider) Renew(cert *Certificate) (*Certificate, error) {
	// For self-signed certificates, renewal is just generating a new certificate
	opts := GenerateOptions{
		ValidityDays: 365,
		KeyType:      "ecdsa",
		KeySize:      256,
	}

	return p.Generate(cert.Metadata.SANs, opts)
}

// CompareCertificates checks if two certificates are equivalent (for testing)
func CompareCertificates(cert1PEM, cert2PEM []byte) bool {
	return bytes.Equal(cert1PEM, cert2PEM)
}
