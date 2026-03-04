package certmanager

import (
	"strings"
	"testing"
)

func TestDomainValidator_ValidLocalDomains(t *testing.T) {
	validator := NewDomainValidator(false, nil)

	validDomains := []string{
		"localhost",
		"nas.local",
		"server.local",
		"app.internal",
		"router.lan",
		"printer.home",
		"192.168.1.1",
		"::1",
		"2001:db8::1",
	}

	for _, domain := range validDomains {
		t.Run(domain, func(t *testing.T) {
			err := validator.Validate([]string{domain})
			if err != nil {
				t.Errorf("Expected domain %q to be valid, got error: %v", domain, err)
			}
		})
	}
}

func TestDomainValidator_InvalidDomains(t *testing.T) {
	validator := NewDomainValidator(false, nil)

	invalidDomains := []struct {
		domain      string
		description string
	}{
		{"", "empty domain"},
		{"../etc/passwd", "path traversal"},
		{"../../etc", "path traversal"},
		{"domain/path", "forward slash"},
		{"domain\\path", "backslash"},
		{"*.*.local", "double wildcard"},
		{"-invalid.local", "starts with hyphen"},
		{"invalid-.local", "ends with hyphen"},
		{strings.Repeat("a", 64) + ".local", "label too long"},
		{strings.Repeat("a", 254), "domain too long"},
		{"domain..local", "empty label"},
		{"", "empty string"},
	}

	for _, tc := range invalidDomains {
		t.Run(tc.description, func(t *testing.T) {
			err := validator.Validate([]string{tc.domain})
			if err == nil {
				t.Errorf("Expected domain %q to be invalid (%s), but it was accepted", tc.domain, tc.description)
			}
		})
	}
}

func TestDomainValidator_PublicDomainsBlocked(t *testing.T) {
	validator := NewDomainValidator(false, nil) // Public domains NOT allowed

	publicDomains := []string{
		"google.com",
		"example.com",
		"malicious-site.net",
		"attacker.org",
	}

	for _, domain := range publicDomains {
		t.Run(domain, func(t *testing.T) {
			err := validator.Validate([]string{domain})
			if err == nil {
				t.Errorf("Expected public domain %q to be blocked", domain)
			}
			if !strings.Contains(err.Error(), "public domains not allowed") {
				t.Errorf("Expected error about public domains, got: %v", err)
			}
		})
	}
}

func TestDomainValidator_PublicDomainsAllowed(t *testing.T) {
	validator := NewDomainValidator(true, nil) // Public domains allowed

	publicDomains := []string{
		"example.com",
		"my-app.com",
		"subdomain.example.org",
	}

	for _, domain := range publicDomains {
		t.Run(domain, func(t *testing.T) {
			err := validator.Validate([]string{domain})
			if err != nil {
				t.Errorf("Expected public domain %q to be allowed, got error: %v", domain, err)
			}
		})
	}
}

func TestDomainValidator_Whitelist(t *testing.T) {
	validator := NewDomainValidator(true, []string{"example.com", "*.allowed.com"})

	tests := []struct {
		domain      string
		shouldAllow bool
	}{
		{"example.com", true},         // Exact match
		{"www.example.com", true},     // Subdomain of allowed
		{"app.allowed.com", true},     // Wildcard match
		{"sub.app.allowed.com", true}, // Nested subdomain
		{"notallowed.com", false},     // Not in whitelist
		{"example.org", false},        // Different TLD
		{"malicious.com", false},      // Not allowed
	}

	for _, tt := range tests {
		t.Run(tt.domain, func(t *testing.T) {
			err := validator.Validate([]string{tt.domain})
			if tt.shouldAllow && err != nil {
				t.Errorf("Expected domain %q to be allowed (whitelist), got error: %v", tt.domain, err)
			}
			if !tt.shouldAllow && err == nil {
				t.Errorf("Expected domain %q to be blocked (not in whitelist)", tt.domain)
			}
		})
	}
}

func TestDomainValidator_WildcardDomains(t *testing.T) {
	validator := NewDomainValidator(false, nil)

	tests := []struct {
		domain string
		valid  bool
	}{
		{"*.local", true},         // Valid wildcard
		{"*.example.local", true}, // Valid nested wildcard
		{"*.*.local", false},      // Double wildcard (invalid)
		{"*", false},              // Just asterisk (invalid)
		{"*local", false},         // No dot after wildcard (invalid)
	}

	for _, tt := range tests {
		t.Run(tt.domain, func(t *testing.T) {
			err := validator.Validate([]string{tt.domain})
			if tt.valid && err != nil {
				t.Errorf("Expected wildcard domain %q to be valid, got error: %v", tt.domain, err)
			}
			if !tt.valid && err == nil {
				t.Errorf("Expected wildcard domain %q to be invalid", tt.domain)
			}
		})
	}
}

func TestDomainValidator_MultipleDomains(t *testing.T) {
	validator := NewDomainValidator(false, nil)

	// All valid
	err := validator.Validate([]string{"app1.local", "app2.local", "192.168.1.1"})
	if err != nil {
		t.Errorf("Expected all domains to be valid, got error: %v", err)
	}

	// One invalid should fail all
	err = validator.Validate([]string{"app1.local", "google.com", "app3.local"})
	if err == nil {
		t.Error("Expected validation to fail when one domain is invalid")
	}
}

func TestDomainValidator_EmptyList(t *testing.T) {
	validator := NewDomainValidator(false, nil)

	err := validator.Validate([]string{})
	if err == nil {
		t.Error("Expected error for empty domain list")
	}
}

func TestDomainValidator_IsLocalDomain(t *testing.T) {
	validator := NewDomainValidator(false, nil)

	localDomains := []string{
		"localhost",
		"nas.local",
		"server.LOCAL", // Case insensitive
		"app.internal",
		"router.lan",
		"printer.home",
	}

	for _, domain := range localDomains {
		t.Run(domain, func(t *testing.T) {
			if !validator.isLocalDomain(domain) {
				t.Errorf("Expected %q to be recognized as local domain", domain)
			}
		})
	}

	publicDomains := []string{
		"example.com",
		"google.com",
		"app.production.com",
	}

	for _, domain := range publicDomains {
		t.Run(domain, func(t *testing.T) {
			if validator.isLocalDomain(domain) {
				t.Errorf("Expected %q to NOT be recognized as local domain", domain)
			}
		})
	}
}

func TestDomainValidator_DomainFormat(t *testing.T) {
	validator := NewDomainValidator(true, nil)

	validFormats := []string{
		"a.com",
		"example.com",
		"sub.example.com",
		"very-long-subdomain-name.example.com",
		"a1.b2.c3.example.com",
		"123.456.789.com", // Numbers are valid
	}

	for _, domain := range validFormats {
		t.Run(domain, func(t *testing.T) {
			err := validator.validateDomainFormat(domain)
			if err != nil {
				t.Errorf("Expected domain format %q to be valid, got error: %v", domain, err)
			}
		})
	}

	invalidFormats := []string{
		"-invalid.com",  // Starts with hyphen
		"invalid-.com",  // Ends with hyphen
		"in..valid.com", // Empty label
		".invalid.com",  // Starts with dot
		"invalid.com.",  // Ends with dot (technically valid in DNS but not for certs)
		"invalid..com",  // Double dot
	}

	for _, domain := range invalidFormats {
		t.Run(domain, func(t *testing.T) {
			err := validator.validateDomainFormat(domain)
			if err == nil {
				t.Errorf("Expected domain format %q to be invalid", domain)
			}
		})
	}
}

func TestDomainValidator_Integration(t *testing.T) {
	// Test realistic scenarios

	t.Run("Local network deployment", func(t *testing.T) {
		validator := NewDomainValidator(false, nil)

		domains := []string{
			"nas.local",
			"pi-hole.local",
			"192.168.1.100",
		}

		err := validator.Validate(domains)
		if err != nil {
			t.Errorf("Expected local network domains to be valid, got: %v", err)
		}
	})

	t.Run("Cloud deployment with whitelist", func(t *testing.T) {
		validator := NewDomainValidator(true, []string{"myapp.com"})

		// Should allow whitelisted domain
		err := validator.Validate([]string{"api.myapp.com"})
		if err != nil {
			t.Errorf("Expected whitelisted domain to be valid, got: %v", err)
		}

		// Should block non-whitelisted
		err = validator.Validate([]string{"attacker.com"})
		if err == nil {
			t.Error("Expected non-whitelisted domain to be blocked")
		}
	})

	t.Run("Wildcard certificate for multiple services", func(t *testing.T) {
		validator := NewDomainValidator(false, nil)

		err := validator.Validate([]string{"*.services.local"})
		if err != nil {
			t.Errorf("Expected wildcard local domain to be valid, got: %v", err)
		}
	})
}
