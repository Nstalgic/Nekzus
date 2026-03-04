package validation

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// Compiled regular expressions for validation
var (
	idRegex    = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
	scopeRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]+:[a-zA-Z0-9_*-]+(:[a-zA-Z0-9_*-]+)*$`)
	htmlRegex  = regexp.MustCompile(`<[^>]*>`)
)

// Allowed platforms
var allowedPlatforms = map[string]bool{
	"ios":     true,
	"android": true,
	"web":     true,
	"desktop": true,
}

// ValidateID validates an ID (app ID, device ID, route ID, etc.)
// Rules:
// - Must not be empty
// - Must be alphanumeric with hyphens or underscores
// - Max length: 100 characters
func ValidateID(id string) error {
	if id == "" {
		return fmt.Errorf("ID cannot be empty")
	}

	if len(id) > 100 {
		return fmt.Errorf("ID exceeds maximum length of 100 characters")
	}

	if !idRegex.MatchString(id) {
		return fmt.Errorf("ID must contain only alphanumeric characters, hyphens, and underscores")
	}

	return nil
}

// ValidateURL validates a URL
// Rules:
// - Must not be empty
// - Must be a valid URL
// - Scheme must be http or https
// - Must not contain XSS vectors
func ValidateURL(rawURL string) error {
	if rawURL == "" {
		return fmt.Errorf("URL cannot be empty")
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	// Only allow HTTP and HTTPS schemes
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("URL scheme must be http or https, got: %s", u.Scheme)
	}

	// Ensure host is present
	if u.Host == "" {
		return fmt.Errorf("URL must have a host")
	}

	return nil
}

// ValidatePath validates a URL path
// Rules:
// - Must not be empty
// - Must start with /
// - Must not contain path traversal attempts (..)
// - Must not contain null bytes
// - Max length: 500 characters
func ValidatePath(path string) error {
	if path == "" {
		return fmt.Errorf("path cannot be empty")
	}

	if !strings.HasPrefix(path, "/") {
		return fmt.Errorf("path must start with /")
	}

	if len(path) > 500 {
		return fmt.Errorf("path exceeds maximum length of 500 characters")
	}

	// Check for null bytes
	if strings.Contains(path, "\x00") {
		return fmt.Errorf("path contains null byte")
	}

	// Check for path traversal
	if strings.Contains(path, "..") {
		return fmt.Errorf("path contains path traversal attempt")
	}

	// Check for encoded path traversal
	if strings.Contains(strings.ToLower(path), "%2e") {
		return fmt.Errorf("path contains encoded path traversal attempt")
	}

	return nil
}

// ValidateName validates a name (app name, device name, etc.)
// Rules:
// - Must not be empty (after trimming)
// - Must not exceed max length
// - Must not contain HTML tags or angle brackets
func ValidateName(name string, maxLen int) error {
	trimmed := strings.TrimSpace(name)

	if trimmed == "" {
		return fmt.Errorf("name cannot be empty")
	}

	if len(name) > maxLen {
		return fmt.Errorf("name exceeds maximum length of %d characters", maxLen)
	}

	// Check for HTML tags or angle brackets (XSS prevention)
	if strings.ContainsAny(name, "<>") {
		return fmt.Errorf("name cannot contain angle brackets")
	}

	return nil
}

// ValidatePlatform validates a platform string
// Rules:
// - Must be one of: ios, android, web, desktop
func ValidatePlatform(platform string) error {
	if platform == "" {
		return fmt.Errorf("platform cannot be empty")
	}

	if !allowedPlatforms[platform] {
		return fmt.Errorf("invalid platform: %s (allowed: ios, android, web, desktop)", platform)
	}

	return nil
}

// ValidateScope validates a scope string
// Rules:
// - Must follow format: action:resource or action:resource:subresource
// - Can use wildcards (*)
// - Must not be empty
func ValidateScope(scope string) error {
	if scope == "" {
		return fmt.Errorf("scope cannot be empty")
	}

	if !scopeRegex.MatchString(scope) {
		return fmt.Errorf("invalid scope format (expected: action:resource)")
	}

	return nil
}

// ValidateDuration validates a duration string
// Rules:
// - Must be a valid Go duration format (e.g., "30s", "5m", "2h")
func ValidateDuration(dur string) error {
	if dur == "" {
		return fmt.Errorf("duration cannot be empty")
	}

	_, err := time.ParseDuration(dur)
	if err != nil {
		return fmt.Errorf("invalid duration format: %w", err)
	}

	return nil
}

// SanitizeString removes potentially dangerous content from a string
// - Removes HTML tags
// - Removes null bytes
// - Trims leading/trailing whitespace
func SanitizeString(s string) string {
	// Remove HTML tags
	s = htmlRegex.ReplaceAllString(s, "")

	// Remove null bytes
	s = strings.ReplaceAll(s, "\x00", "")

	// Trim whitespace
	s = strings.TrimSpace(s)

	return s
}
