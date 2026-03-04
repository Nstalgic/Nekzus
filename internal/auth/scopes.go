package auth

import (
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

// HasAllScopes verifies that token claims contain all required scopes.
// Supports wildcard scopes (e.g., "read:*" matches "read:api", "read:webapp1", etc.)
//
// This function checks both exact matches and wildcard patterns.
// A token scope of "read:*" will match any required scope starting with "read:".
//
// Example:
//   - Token scopes: ["read:*", "write:api"]
//   - Required: ["read:devices", "write:api"]
//   - Result: true (read:* matches read:devices, write:api matches exactly)
func HasAllScopes(claims jwt.MapClaims, requiredScopes []string) bool {
	if len(requiredScopes) == 0 {
		return true // No scopes required
	}

	// JWT scopes are stored as []interface{} in MapClaims
	scopeList, ok := claims["scopes"].([]interface{})
	if !ok {
		return false
	}

	// Build set of token scopes for O(1) lookup
	tokenScopeSet := make(map[string]struct{}, len(scopeList))
	for _, scope := range scopeList {
		if scopeStr, ok := scope.(string); ok {
			tokenScopeSet[scopeStr] = struct{}{}
		}
	}

	// Verify all required scopes are present (with wildcard support)
	for _, required := range requiredScopes {
		// Check for exact match
		if _, ok := tokenScopeSet[required]; ok {
			continue
		}

		// Check for wildcard match (e.g., "read:*" matches "read:api")
		matched := false
		for tokenScope := range tokenScopeSet {
			if strings.HasSuffix(tokenScope, ":*") {
				prefix := strings.TrimSuffix(tokenScope, "*")
				if strings.HasPrefix(required, prefix) {
					matched = true
					break
				}
			}
		}

		if !matched {
			return false
		}
	}

	return true
}

// HasAllScopesFromAny verifies that tokenScopes (as any type) contains all required scopes.
// This is a convenience wrapper for HasAllScopes that accepts the scopes claim directly
// instead of the full claims map.
//
// Supports wildcard scopes (e.g., "read:*" matches "read:api", "read:webapp1", etc.)
func HasAllScopesFromAny(tokenScopes any, requiredScopes []string) bool {
	if len(requiredScopes) == 0 {
		return true
	}

	arr, ok := tokenScopes.([]interface{})
	if !ok {
		return false
	}

	// Build a set of token scopes
	tokenScopeSet := make(map[string]struct{}, len(arr))
	for _, v := range arr {
		if s, ok := v.(string); ok {
			tokenScopeSet[s] = struct{}{}
		}
	}

	// Check each needed scope
	for _, required := range requiredScopes {
		// Check for exact match
		if _, ok := tokenScopeSet[required]; ok {
			continue
		}

		// Check for wildcard match (e.g., "read:*" matches "read:api")
		matched := false
		for tokenScope := range tokenScopeSet {
			if strings.HasSuffix(tokenScope, ":*") {
				prefix := strings.TrimSuffix(tokenScope, "*")
				if strings.HasPrefix(required, prefix) {
					matched = true
					break
				}
			}
		}

		if !matched {
			return false
		}
	}

	return true
}
