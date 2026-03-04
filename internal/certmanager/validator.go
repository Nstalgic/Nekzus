package certmanager

import (
	"fmt"
	"net"
	"regexp"
	"strings"
)

// DomainValidator validates domain names for certificate generation
type DomainValidator struct {
	allowPublicDomains bool
	allowedDomains     []string // Whitelist of allowed domains (empty = allow all)
}

// NewDomainValidator creates a new domain validator
func NewDomainValidator(allowPublicDomains bool, allowedDomains []string) *DomainValidator {
	return &DomainValidator{
		allowPublicDomains: allowPublicDomains,
		allowedDomains:     allowedDomains,
	}
}

// Validate checks if the given domains are valid and safe for certificate generation
func (v *DomainValidator) Validate(domains []string) error {
	if len(domains) == 0 {
		return fmt.Errorf("at least one domain is required")
	}

	for _, domain := range domains {
		if err := v.validateSingleDomain(domain); err != nil {
			return fmt.Errorf("invalid domain %q: %w", domain, err)
		}
	}

	return nil
}

// validateSingleDomain validates a single domain name
func (v *DomainValidator) validateSingleDomain(domain string) error {
	// Check for empty domain
	if domain == "" {
		return fmt.Errorf("domain cannot be empty")
	}

	// Check for path traversal attempts
	if strings.Contains(domain, "..") || strings.Contains(domain, "/") || strings.Contains(domain, "\\") {
		return fmt.Errorf("domain contains invalid characters (possible path traversal)")
	}

	// Check domain length (RFC 1035: max 253 characters)
	if len(domain) > 253 {
		return fmt.Errorf("domain too long (max 253 characters)")
	}

	// Check if it's an IP address
	if net.ParseIP(domain) != nil {
		// IP addresses are valid for local network
		return nil
	}

	// Check if it's a wildcard domain
	if strings.HasPrefix(domain, "*.") {
		return v.validateWildcardDomain(domain)
	}

	// Validate domain format
	if err := v.validateDomainFormat(domain); err != nil {
		return err
	}

	// Check whitelist if configured
	if len(v.allowedDomains) > 0 {
		if !v.isAllowedDomain(domain) {
			return fmt.Errorf("domain not in whitelist")
		}
	}

	// Check if it's a local domain
	if v.isLocalDomain(domain) {
		return nil // Local domains are always allowed
	}

	// Check if public domains are allowed
	if !v.allowPublicDomains {
		return fmt.Errorf("public domains not allowed (only .local, localhost, or IP addresses)")
	}

	return nil
}

// validateWildcardDomain validates wildcard domain names
func (v *DomainValidator) validateWildcardDomain(domain string) error {
	// Remove the wildcard prefix
	baseDomain := strings.TrimPrefix(domain, "*.")

	// Check for double wildcards (*.*.example.com)
	if strings.Contains(baseDomain, "*") {
		return fmt.Errorf("multiple wildcards not allowed")
	}

	// Validate the base domain
	return v.validateDomainFormat(baseDomain)
}

// validateDomainFormat validates domain name format per RFC 1035
func (v *DomainValidator) validateDomainFormat(domain string) error {
	// Domain name regex: alphanumeric and hyphens, each label max 63 chars
	// Cannot start or end with hyphen
	labelRegex := regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?$`)

	labels := strings.Split(domain, ".")

	// At least one label required
	if len(labels) == 0 {
		return fmt.Errorf("domain must have at least one label")
	}

	// Validate each label
	for _, label := range labels {
		if label == "" {
			return fmt.Errorf("domain label cannot be empty")
		}

		if len(label) > 63 {
			return fmt.Errorf("domain label %q too long (max 63 characters)", label)
		}

		if !labelRegex.MatchString(label) {
			return fmt.Errorf("domain label %q has invalid format", label)
		}
	}

	return nil
}

// isLocalDomain checks if a domain is for local network use
func (v *DomainValidator) isLocalDomain(domain string) bool {
	// Common local domain patterns
	localPatterns := []string{
		".local",     // mDNS/Bonjour
		".localhost", // RFC 2606
		".internal",  // Common internal domain
		".lan",       // Common LAN domain
		".home",      // Common home network domain
		"localhost",  // Exact match
	}

	domain = strings.ToLower(domain)

	for _, pattern := range localPatterns {
		if strings.HasSuffix(domain, pattern) || domain == strings.TrimPrefix(pattern, ".") {
			return true
		}
	}

	return false
}

// isAllowedDomain checks if a domain is in the whitelist
func (v *DomainValidator) isAllowedDomain(domain string) bool {
	domain = strings.ToLower(domain)

	for _, allowed := range v.allowedDomains {
		allowed = strings.ToLower(allowed)

		// Exact match
		if domain == allowed {
			return true
		}

		// Wildcard match (e.g., *.example.com matches app.example.com)
		if strings.HasPrefix(allowed, "*.") {
			baseDomain := strings.TrimPrefix(allowed, "*.")
			// Must end with ".baseDomain" not just "baseDomain" to avoid matches like "notallowed.com" matching "*.allowed.com"
			if strings.HasSuffix(domain, "."+baseDomain) || domain == baseDomain {
				return true
			}
		}

		// Subdomain match (e.g., example.com matches app.example.com)
		if strings.HasSuffix(domain, "."+allowed) {
			return true
		}
	}

	return false
}

// ValidateDomainOwnership performs additional checks for public domains
// This would typically integrate with DNS verification (for ACME) or other proof of ownership
func (v *DomainValidator) ValidateDomainOwnership(domain string) error {
	// For now, this is a placeholder for future ACME integration
	// In a real implementation, this would:
	// 1. Check DNS TXT records
	// 2. Verify HTTP-01 challenges
	// 3. Check domain control validation
	return nil
}
