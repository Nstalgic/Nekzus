package certmanager

// Provider is the interface for certificate providers (self-signed, ACME, etc.)
type Provider interface {
	// Generate creates a new certificate for the given domains
	Generate(domains []string, opts GenerateOptions) (*Certificate, error)

	// Renew attempts to renew an existing certificate
	Renew(cert *Certificate) (*Certificate, error)

	// Validate checks if this provider can issue certs for these domains
	Validate(domains []string) error

	// Name returns provider identifier ("self-signed", "letsencrypt")
	Name() string
}
