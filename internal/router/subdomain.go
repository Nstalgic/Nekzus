package router

import (
	"fmt"
	"net"
	"regexp"
	"strings"
)

// subdomainRegex validates DNS label characters: lowercase alphanumeric and hyphens
var subdomainRegex = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)

// ExtractSubdomain extracts the subdomain from a Host header given a base domain.
// For example, ExtractSubdomain("sonarr.nekzus.local:8443", "nekzus.local") returns ("sonarr", true).
// Returns ("", false) if the host doesn't match the base domain or has no subdomain.
func ExtractSubdomain(host, baseDomain string) (string, bool) {
	if host == "" || baseDomain == "" {
		return "", false
	}

	// Strip port from both host and baseDomain
	hostOnly := strings.ToLower(host)
	if h, _, err := net.SplitHostPort(hostOnly); err == nil {
		hostOnly = h
	}

	baseOnly := strings.ToLower(baseDomain)
	if b, _, err := net.SplitHostPort(baseOnly); err == nil {
		baseOnly = b
	}

	// Host must end with ".baseDomain"
	suffix := "." + baseOnly
	if !strings.HasSuffix(hostOnly, suffix) {
		return "", false
	}

	// Extract subdomain (everything before .baseDomain)
	subdomain := hostOnly[:len(hostOnly)-len(suffix)]
	if subdomain == "" {
		return "", false
	}

	return subdomain, true
}

// ValidateSubdomain checks that a subdomain is a valid DNS label.
// Rules: lowercase alphanumeric and hyphens, no leading/trailing hyphens, max 63 chars.
func ValidateSubdomain(sub string) error {
	if sub == "" {
		return fmt.Errorf("subdomain cannot be empty")
	}
	if len(sub) > 63 {
		return fmt.Errorf("subdomain too long (max 63 characters, got %d)", len(sub))
	}
	if !subdomainRegex.MatchString(sub) {
		return fmt.Errorf("subdomain %q is invalid: must be lowercase alphanumeric with hyphens, no leading/trailing hyphens", sub)
	}
	return nil
}
