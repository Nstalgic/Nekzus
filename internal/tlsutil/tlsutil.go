package tlsutil

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"log/slog"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

var log = slog.With("package", "tlsutil")

// GenerateSelfSignedCert generates a self-signed TLS certificate with the given SANs
func GenerateSelfSignedCert(certPath, keyPath string, hostnames []string) error {
	// Generate private key
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return fmt.Errorf("failed to generate private key: %w", err)
	}

	// Create certificate template
	notBefore := time.Now()
	notAfter := notBefore.Add(365 * 24 * time.Hour) // Valid for 1 year

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return fmt.Errorf("failed to generate serial number: %w", err)
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName:   "localhost",
			Organization: []string{"Nekzus"},
		},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	// Add SANs
	for _, h := range hostnames {
		if ip := net.ParseIP(h); ip != nil {
			template.IPAddresses = append(template.IPAddresses, ip)
		} else {
			template.DNSNames = append(template.DNSNames, h)
		}
	}

	// Create certificate
	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return fmt.Errorf("failed to create certificate: %w", err)
	}

	// Ensure directories exist
	if err := os.MkdirAll(filepath.Dir(certPath), 0755); err != nil {
		return fmt.Errorf("failed to create cert directory: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(keyPath), 0755); err != nil {
		return fmt.Errorf("failed to create key directory: %w", err)
	}

	// Write certificate
	certOut, err := os.Create(certPath)
	if err != nil {
		return fmt.Errorf("failed to create cert file: %w", err)
	}
	defer func() {
		if err := certOut.Close(); err != nil {
			log.Warn("failed to close cert file",
				"error", err)
		}
	}()

	if err := pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes}); err != nil {
		return fmt.Errorf("failed to write cert: %w", err)
	}

	// Write private key
	keyOut, err := os.OpenFile(keyPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("failed to create key file: %w", err)
	}
	defer func() {
		if err := keyOut.Close(); err != nil {
			log.Warn("failed to close key file",
				"error", err)
		}
	}()

	privBytes, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return fmt.Errorf("failed to marshal private key: %w", err)
	}

	if err := pem.Encode(keyOut, &pem.Block{Type: "EC PRIVATE KEY", Bytes: privBytes}); err != nil {
		return fmt.Errorf("failed to write key: %w", err)
	}

	return nil
}

// EnsureCertificates checks if TLS certificates exist, and generates them if needed
// Returns (generated bool, error)
func EnsureCertificates(certPath, keyPath string) (bool, error) {
	// If paths are empty, skip (user doesn't want TLS)
	if certPath == "" || keyPath == "" {
		return false, nil
	}

	// Check if both files exist
	certExists := fileExists(certPath)
	keyExists := fileExists(keyPath)

	if certExists && keyExists {
		// Both exist, nothing to do
		return false, nil
	}

	if certExists != keyExists {
		// Only one exists - this is an error state
		return false, fmt.Errorf("certificate and key must both exist or both be missing (cert=%v, key=%v)", certExists, keyExists)
	}

	// Neither exists - generate new certificates
	hostnames := getHostnames()
	if err := GenerateSelfSignedCert(certPath, keyPath, hostnames); err != nil {
		return false, fmt.Errorf("failed to generate self-signed certificate: %w", err)
	}

	return true, nil
}

// fileExists checks if a file exists
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// getHostnames returns a list of hostnames/IPs for the certificate SANs
func getHostnames() []string {
	hostnames := []string{"localhost", "127.0.0.1", "::1"}

	// Try to get the system hostname
	if hostname, err := os.Hostname(); err == nil && hostname != "" {
		hostnames = append(hostnames, hostname)
	}

	// Get local IPs
	addrs, err := net.InterfaceAddrs()
	if err == nil {
		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
				if ipnet.IP.To4() != nil {
					hostnames = append(hostnames, ipnet.IP.String())
				}
			}
		}
	}

	return hostnames
}

// ValidateCertificates validates that certificate and key files are valid and can be loaded.
// If both paths are empty, returns nil (TLS disabled).
// If paths are specified, attempts to load them using tls.LoadX509KeyPair to verify validity.
func ValidateCertificates(certPath, keyPath string) error {
	// If paths are empty, skip validation (TLS disabled)
	if certPath == "" && keyPath == "" {
		return nil
	}

	// If only one is specified, that's an error
	if certPath == "" || keyPath == "" {
		return fmt.Errorf("both certificate and key paths must be specified or both must be empty")
	}

	// Attempt to load the certificate and key
	_, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return fmt.Errorf("failed to load certificate and key: %w", err)
	}

	return nil
}
