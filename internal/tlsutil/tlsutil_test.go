package tlsutil

import (
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
)

func TestGenerateSelfSignedCert(t *testing.T) {
	tmpDir := t.TempDir()
	certPath := filepath.Join(tmpDir, "cert.pem")
	keyPath := filepath.Join(tmpDir, "key.pem")

	// Generate certificate
	err := GenerateSelfSignedCert(certPath, keyPath, []string{"localhost", "127.0.0.1"})
	if err != nil {
		t.Fatalf("Failed to generate certificate: %v", err)
	}

	// Verify certificate file exists
	if _, err := os.Stat(certPath); os.IsNotExist(err) {
		t.Fatal("Certificate file was not created")
	}

	// Verify key file exists
	if _, err := os.Stat(keyPath); os.IsNotExist(err) {
		t.Fatal("Key file was not created")
	}

	// Verify certificate is valid
	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		t.Fatalf("Failed to read certificate: %v", err)
	}

	block, _ := pem.Decode(certPEM)
	if block == nil {
		t.Fatal("Failed to decode certificate PEM")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("Failed to parse certificate: %v", err)
	}

	// Check common name
	if cert.Subject.CommonName != "localhost" {
		t.Errorf("Expected CommonName=localhost, got %s", cert.Subject.CommonName)
	}

	// Check SANs
	if len(cert.DNSNames) == 0 && len(cert.IPAddresses) == 0 {
		t.Error("Certificate has no SANs")
	}

	// Check if localhost is in DNSNames
	hasLocalhost := false
	for _, dns := range cert.DNSNames {
		if dns == "localhost" {
			hasLocalhost = true
			break
		}
	}
	if !hasLocalhost {
		t.Error("Certificate does not include localhost in SANs")
	}

	// Verify key is valid
	keyPEM, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatalf("Failed to read key: %v", err)
	}

	keyBlock, _ := pem.Decode(keyPEM)
	if keyBlock == nil {
		t.Fatal("Failed to decode key PEM")
	}
}

func TestGenerateSelfSignedCert_InvalidPaths(t *testing.T) {
	// Try to write to non-existent directory
	err := GenerateSelfSignedCert("/nonexistent/cert.pem", "/nonexistent/key.pem", []string{"localhost"})
	if err == nil {
		t.Error("Expected error when writing to non-existent directory")
	}
}

func TestGenerateSelfSignedCert_CreatesDirs(t *testing.T) {
	tmpDir := t.TempDir()
	certDir := filepath.Join(tmpDir, "nested", "certs")
	certPath := filepath.Join(certDir, "cert.pem")
	keyPath := filepath.Join(certDir, "key.pem")

	// Should create nested directories
	err := GenerateSelfSignedCert(certPath, keyPath, []string{"localhost"})
	if err != nil {
		t.Fatalf("Failed to generate certificate: %v", err)
	}

	// Verify files exist
	if _, err := os.Stat(certPath); os.IsNotExist(err) {
		t.Fatal("Certificate file was not created")
	}
	if _, err := os.Stat(keyPath); os.IsNotExist(err) {
		t.Fatal("Key file was not created")
	}
}

func TestEnsureCertificates(t *testing.T) {
	tmpDir := t.TempDir()
	certPath := filepath.Join(tmpDir, "cert.pem")
	keyPath := filepath.Join(tmpDir, "key.pem")

	tests := []struct {
		name          string
		certPath      string
		keyPath       string
		setupFunc     func()
		expectedGen   bool
		expectedError bool
	}{
		{
			name:          "both files missing - should generate",
			certPath:      certPath,
			keyPath:       keyPath,
			setupFunc:     func() {},
			expectedGen:   true,
			expectedError: false,
		},
		{
			name:     "both files exist - should not generate",
			certPath: certPath + ".existing",
			keyPath:  keyPath + ".existing",
			setupFunc: func() {
				os.WriteFile(certPath+".existing", []byte("fake cert"), 0644)
				os.WriteFile(keyPath+".existing", []byte("fake key"), 0644)
			},
			expectedGen:   false,
			expectedError: false,
		},
		{
			name:     "only cert exists - should error",
			certPath: certPath + ".partial-cert",
			keyPath:  keyPath + ".partial-key",
			setupFunc: func() {
				os.WriteFile(certPath+".partial-cert", []byte("fake cert"), 0644)
			},
			expectedGen:   false,
			expectedError: true,
		},
		{
			name:     "only key exists - should error",
			certPath: certPath + ".partial-key2",
			keyPath:  keyPath + ".partial-key2",
			setupFunc: func() {
				os.WriteFile(keyPath+".partial-key2", []byte("fake key"), 0644)
			},
			expectedGen:   false,
			expectedError: true,
		},
		{
			name:          "empty paths - should skip",
			certPath:      "",
			keyPath:       "",
			setupFunc:     func() {},
			expectedGen:   false,
			expectedError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupFunc()

			generated, err := EnsureCertificates(tt.certPath, tt.keyPath)

			if tt.expectedError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectedError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if generated != tt.expectedGen {
				t.Errorf("Expected generated=%v, got %v", tt.expectedGen, generated)
			}
		})
	}
}

func TestGetHostnames(t *testing.T) {
	hostnames := getHostnames()

	// Should always include localhost
	hasLocalhost := false
	for _, h := range hostnames {
		if h == "localhost" {
			hasLocalhost = true
			break
		}
	}
	if !hasLocalhost {
		t.Error("Expected localhost in hostnames")
	}

	// Should include at least one IP (127.0.0.1)
	hasIP := false
	for _, h := range hostnames {
		if h == "127.0.0.1" {
			hasIP = true
			break
		}
	}
	if !hasIP {
		t.Error("Expected 127.0.0.1 in hostnames")
	}
}

func TestValidateCertificates(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name          string
		setupFunc     func() (certPath, keyPath string)
		expectedError bool
		errorContains string
	}{
		{
			name: "valid certificate and key",
			setupFunc: func() (string, string) {
				certPath := filepath.Join(tmpDir, "valid-cert.pem")
				keyPath := filepath.Join(tmpDir, "valid-key.pem")
				// Generate valid cert/key pair
				if err := GenerateSelfSignedCert(certPath, keyPath, []string{"localhost"}); err != nil {
					t.Fatalf("Failed to generate test certificate: %v", err)
				}
				return certPath, keyPath
			},
			expectedError: false,
		},
		{
			name: "certificate file does not exist",
			setupFunc: func() (string, string) {
				return filepath.Join(tmpDir, "nonexistent-cert.pem"),
					filepath.Join(tmpDir, "nonexistent-key.pem")
			},
			expectedError: true,
			errorContains: "failed to load certificate",
		},
		{
			name: "corrupted certificate file",
			setupFunc: func() (string, string) {
				certPath := filepath.Join(tmpDir, "corrupted-cert.pem")
				keyPath := filepath.Join(tmpDir, "corrupted-key.pem")
				// Write invalid certificate data
				if err := os.WriteFile(certPath, []byte("not a valid certificate"), 0644); err != nil {
					t.Fatalf("Failed to write corrupted cert: %v", err)
				}
				if err := os.WriteFile(keyPath, []byte("not a valid key"), 0600); err != nil {
					t.Fatalf("Failed to write corrupted key: %v", err)
				}
				return certPath, keyPath
			},
			expectedError: true,
			errorContains: "failed to load certificate",
		},
		{
			name: "mismatched certificate and key",
			setupFunc: func() (string, string) {
				// Generate two separate cert/key pairs
				cert1Path := filepath.Join(tmpDir, "cert1.pem")
				key1Path := filepath.Join(tmpDir, "key1.pem")
				cert2Path := filepath.Join(tmpDir, "cert2.pem")
				key2Path := filepath.Join(tmpDir, "key2.pem")

				if err := GenerateSelfSignedCert(cert1Path, key1Path, []string{"localhost"}); err != nil {
					t.Fatalf("Failed to generate first cert: %v", err)
				}
				if err := GenerateSelfSignedCert(cert2Path, key2Path, []string{"localhost"}); err != nil {
					t.Fatalf("Failed to generate second cert: %v", err)
				}

				// Return mismatched pair (cert1 with key2)
				return cert1Path, key2Path
			},
			expectedError: true,
			errorContains: "failed to load certificate",
		},
		{
			name: "empty certificate paths",
			setupFunc: func() (string, string) {
				return "", ""
			},
			expectedError: false, // Empty paths are valid (means no TLS)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			certPath, keyPath := tt.setupFunc()

			err := ValidateCertificates(certPath, keyPath)

			if tt.expectedError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectedError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if tt.expectedError && err != nil && tt.errorContains != "" {
				if !contains(err.Error(), tt.errorContains) {
					t.Errorf("Expected error to contain %q, got: %v", tt.errorContains, err)
				}
			}
		})
	}
}

// Helper function for string contains check
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && (s[0:len(substr)] == substr || contains(s[1:], substr))))
}
