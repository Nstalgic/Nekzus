package certmanager

import (
	"github.com/nstalgic/nekzus/internal/storage"
)

// StorageAdapter adapts storage.Store to implement certmanager.Storage interface
// This allows certmanager to use the storage package without tight coupling
type StorageAdapter struct {
	store *storage.Store
}

// NewStorageAdapter creates a new storage adapter
func NewStorageAdapter(store *storage.Store) *StorageAdapter {
	return &StorageAdapter{store: store}
}

// StoreCertificate converts and stores a certificate
func (a *StorageAdapter) StoreCertificate(cert *StoredCertificate) error {
	storageCert := &storage.StoredCertificate{
		ID:                      cert.ID,
		Domain:                  cert.Domain,
		CertificatePEM:          cert.CertificatePEM,
		PrivateKeyPEM:           cert.PrivateKeyPEM,
		Issuer:                  cert.Issuer,
		NotBefore:               cert.NotBefore,
		NotAfter:                cert.NotAfter,
		SubjectAlternativeNames: cert.SubjectAlternativeNames,
		FingerprintSHA256:       cert.FingerprintSHA256,
		CreatedAt:               cert.CreatedAt,
		UpdatedAt:               cert.UpdatedAt,
		RenewalAttemptCount:     cert.RenewalAttemptCount,
		LastRenewalAttempt:      cert.LastRenewalAttempt,
		LastRenewalError:        cert.LastRenewalError,
	}
	return a.store.StoreCertificate(storageCert)
}

// GetCertificate retrieves and converts a certificate
func (a *StorageAdapter) GetCertificate(domain string) (*StoredCertificate, error) {
	storageCert, err := a.store.GetCertificate(domain)
	if err != nil {
		return nil, err
	}
	if storageCert == nil {
		return nil, nil
	}

	return &StoredCertificate{
		ID:                      storageCert.ID,
		Domain:                  storageCert.Domain,
		CertificatePEM:          storageCert.CertificatePEM,
		PrivateKeyPEM:           storageCert.PrivateKeyPEM,
		Issuer:                  storageCert.Issuer,
		NotBefore:               storageCert.NotBefore,
		NotAfter:                storageCert.NotAfter,
		SubjectAlternativeNames: storageCert.SubjectAlternativeNames,
		FingerprintSHA256:       storageCert.FingerprintSHA256,
		CreatedAt:               storageCert.CreatedAt,
		UpdatedAt:               storageCert.UpdatedAt,
		RenewalAttemptCount:     storageCert.RenewalAttemptCount,
		LastRenewalAttempt:      storageCert.LastRenewalAttempt,
		LastRenewalError:        storageCert.LastRenewalError,
	}, nil
}

// ListCertificates retrieves and converts all certificates
func (a *StorageAdapter) ListCertificates() ([]*StoredCertificate, error) {
	storageCerts, err := a.store.ListCertificates()
	if err != nil {
		return nil, err
	}

	certs := make([]*StoredCertificate, len(storageCerts))
	for i, sc := range storageCerts {
		certs[i] = &StoredCertificate{
			ID:                      sc.ID,
			Domain:                  sc.Domain,
			CertificatePEM:          sc.CertificatePEM,
			PrivateKeyPEM:           sc.PrivateKeyPEM,
			Issuer:                  sc.Issuer,
			NotBefore:               sc.NotBefore,
			NotAfter:                sc.NotAfter,
			SubjectAlternativeNames: sc.SubjectAlternativeNames,
			FingerprintSHA256:       sc.FingerprintSHA256,
			CreatedAt:               sc.CreatedAt,
			UpdatedAt:               sc.UpdatedAt,
			RenewalAttemptCount:     sc.RenewalAttemptCount,
			LastRenewalAttempt:      sc.LastRenewalAttempt,
			LastRenewalError:        sc.LastRenewalError,
		}
	}

	return certs, nil
}

// DeleteCertificate deletes a certificate
func (a *StorageAdapter) DeleteCertificate(domain string) error {
	return a.store.DeleteCertificate(domain)
}

// AddCertificateHistory converts and adds a history entry
func (a *StorageAdapter) AddCertificateHistory(entry CertificateHistoryEntry) error {
	storageEntry := storage.CertificateHistoryEntry{
		ID:                entry.ID,
		Domain:            entry.Domain,
		Action:            entry.Action,
		Issuer:            entry.Issuer,
		FingerprintSHA256: entry.FingerprintSHA256,
		CreatedAt:         entry.CreatedAt,
		CreatedBy:         entry.CreatedBy,
		Metadata:          entry.Metadata,
	}
	return a.store.AddCertificateHistory(storageEntry)
}
