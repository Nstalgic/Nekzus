package certmanager

import (
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

// GenerateCA creates a new Certificate Authority certificate
func (p *SelfSignedProvider) GenerateCA(opts CAOptions) (*CACertificate, error) {
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
		return nil, fmt.Errorf("failed to generate CA private key: %w", err)
	}

	// Create CA certificate template
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
			CommonName:   opts.CommonName,
			Organization: []string{opts.Organization},
		},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            1,
		MaxPathLenZero:        false,
	}

	// Get public key
	var publicKey interface{}
	switch k := privateKey.(type) {
	case *ecdsa.PrivateKey:
		publicKey = &k.PublicKey
	case *rsa.PrivateKey:
		publicKey = &k.PublicKey
	default:
		return nil, fmt.Errorf("unsupported key type")
	}

	// Self-sign the CA certificate
	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, publicKey, privateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create CA certificate: %w", err)
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

	// Parse the certificate for the return struct
	x509Cert, err := x509.ParseCertificate(derBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse CA certificate: %w", err)
	}

	// Calculate SPKI fingerprint
	spki, err := x509.MarshalPKIXPublicKey(x509Cert.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal public key: %w", err)
	}
	hash := sha256.Sum256(spki)
	fingerprint := "sha256/" + base64.StdEncoding.EncodeToString(hash[:])

	return &CACertificate{
		Certificate: x509Cert,
		PrivateKey:  privateKey,
		PEM: CertificatePEM{
			Certificate: certPEM,
			PrivateKey:  keyPEM,
		},
		Fingerprint: fingerprint,
		CreatedAt:   time.Now(),
	}, nil
}

// SignWithCA signs a certificate request using the CA
func (p *SelfSignedProvider) SignWithCA(ca *CACertificate, req SignRequest) (*Certificate, error) {
	if ca == nil {
		return nil, fmt.Errorf("CA certificate is required")
	}

	if len(req.Domains) == 0 {
		return nil, fmt.Errorf("at least one domain is required")
	}

	req.SetDefaults()

	// Generate private key for the new certificate
	var privateKey interface{}
	var err error

	if req.KeyType == "rsa" {
		privateKey, err = rsa.GenerateKey(rand.Reader, req.KeySize)
	} else {
		privateKey, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to generate private key: %w", err)
	}

	// Create certificate template
	notBefore := time.Now()
	notAfter := notBefore.Add(time.Duration(req.ValidityDays) * 24 * time.Hour)

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return nil, fmt.Errorf("failed to generate serial number: %w", err)
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName:   req.Domains[0],
			Organization: []string{"Nekzus"},
		},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IsCA:                  false,
	}

	// Add all domains as SANs
	for _, domain := range req.Domains {
		if ip := net.ParseIP(domain); ip != nil {
			template.IPAddresses = append(template.IPAddresses, ip)
		} else {
			template.DNSNames = append(template.DNSNames, domain)
		}
	}

	// Get public key
	var publicKey interface{}
	switch k := privateKey.(type) {
	case *ecdsa.PrivateKey:
		publicKey = &k.PublicKey
	case *rsa.PrivateKey:
		publicKey = &k.PublicKey
	default:
		return nil, fmt.Errorf("unsupported key type")
	}

	// Sign with CA
	derBytes, err := x509.CreateCertificate(rand.Reader, &template, ca.Certificate, publicKey, ca.PrivateKey)
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

	// Calculate SPKI fingerprint
	spki, err := x509.MarshalPKIXPublicKey(x509Cert.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal public key: %w", err)
	}
	hash := sha256.Sum256(spki)
	fingerprint := "sha256/" + base64.StdEncoding.EncodeToString(hash[:])

	// Create certificate metadata
	metadata := CertMetadata{
		Issuer:      ca.Certificate.Subject.CommonName,
		NotBefore:   x509Cert.NotBefore,
		NotAfter:    x509Cert.NotAfter,
		SANs:        req.Domains,
		Fingerprint: fingerprint,
	}

	return &Certificate{
		Domain:  req.Domains[0],
		TLSCert: &tlsCert,
		PEM: CertificatePEM{
			Certificate: certPEM,
			PrivateKey:  keyPEM,
			CAChain:     ca.PEM.Certificate,
		},
		Metadata: metadata,
	}, nil
}

// ExportCertBundle creates a bundle with CA cert, service cert, and key
func ExportCertBundle(ca *CACertificate, cert *Certificate) CertBundle {
	return CertBundle{
		CACert: ca.PEM.Certificate,
		Cert:   cert.PEM.Certificate,
		Key:    cert.PEM.PrivateKey,
	}
}

// LoadCAFromPEM loads a CA certificate from PEM-encoded data
func LoadCAFromPEM(certPEM, keyPEM []byte) (*CACertificate, error) {
	// Parse certificate
	certBlock, _ := pem.Decode(certPEM)
	if certBlock == nil {
		return nil, fmt.Errorf("failed to decode certificate PEM")
	}

	x509Cert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse certificate: %w", err)
	}

	// Verify it's a CA certificate
	if !x509Cert.IsCA {
		return nil, fmt.Errorf("certificate is not a CA")
	}

	// Parse private key
	keyBlock, _ := pem.Decode(keyPEM)
	if keyBlock == nil {
		return nil, fmt.Errorf("failed to decode private key PEM")
	}

	var privateKey interface{}
	switch keyBlock.Type {
	case "EC PRIVATE KEY":
		privateKey, err = x509.ParseECPrivateKey(keyBlock.Bytes)
	case "RSA PRIVATE KEY":
		privateKey, err = x509.ParsePKCS1PrivateKey(keyBlock.Bytes)
	default:
		return nil, fmt.Errorf("unsupported private key type: %s", keyBlock.Type)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %w", err)
	}

	// Calculate SPKI fingerprint
	spki, err := x509.MarshalPKIXPublicKey(x509Cert.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal public key: %w", err)
	}
	hash := sha256.Sum256(spki)
	fingerprint := "sha256/" + base64.StdEncoding.EncodeToString(hash[:])

	return &CACertificate{
		Certificate: x509Cert,
		PrivateKey:  privateKey,
		PEM: CertificatePEM{
			Certificate: certPEM,
			PrivateKey:  keyPEM,
		},
		Fingerprint: fingerprint,
		CreatedAt:   x509Cert.NotBefore,
	}, nil
}
