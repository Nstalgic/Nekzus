package auth

import (
	"testing"

	"github.com/golang-jwt/jwt/v5"
)

func TestHasAllScopes(t *testing.T) {
	tests := []struct {
		name           string
		tokenScopes    []string
		requiredScopes []string
		want           bool
	}{
		{
			name:           "no scopes required",
			tokenScopes:    []string{"read:api"},
			requiredScopes: []string{},
			want:           true,
		},
		{
			name:           "exact match - single scope",
			tokenScopes:    []string{"read:api"},
			requiredScopes: []string{"read:api"},
			want:           true,
		},
		{
			name:           "exact match - multiple scopes",
			tokenScopes:    []string{"read:api", "write:api", "delete:api"},
			requiredScopes: []string{"read:api", "write:api"},
			want:           true,
		},
		{
			name:           "missing required scope",
			tokenScopes:    []string{"read:api"},
			requiredScopes: []string{"read:api", "write:api"},
			want:           false,
		},
		{
			name:           "wildcard matches specific scope",
			tokenScopes:    []string{"read:*"},
			requiredScopes: []string{"read:api"},
			want:           true,
		},
		{
			name:           "wildcard matches multiple specific scopes",
			tokenScopes:    []string{"read:*"},
			requiredScopes: []string{"read:api", "read:webapp", "read:devices"},
			want:           true,
		},
		{
			name:           "wildcard and exact scopes combined",
			tokenScopes:    []string{"read:*", "write:api"},
			requiredScopes: []string{"read:devices", "write:api"},
			want:           true,
		},
		{
			name:           "wildcard doesn't match different prefix",
			tokenScopes:    []string{"read:*"},
			requiredScopes: []string{"write:api"},
			want:           false,
		},
		{
			name:           "multiple wildcards",
			tokenScopes:    []string{"read:*", "write:*"},
			requiredScopes: []string{"read:api", "write:devices"},
			want:           true,
		},
		{
			name:           "empty token scopes",
			tokenScopes:    []string{},
			requiredScopes: []string{"read:api"},
			want:           false,
		},
		{
			name:           "admin wildcard matches everything",
			tokenScopes:    []string{"admin:*"},
			requiredScopes: []string{"admin:users", "admin:devices", "admin:settings"},
			want:           true,
		},
		{
			name:           "partial prefix match doesn't count",
			tokenScopes:    []string{"read:*"},
			requiredScopes: []string{"readonly:api"},
			want:           false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Build claims with scopes as []interface{}
			scopesInterface := make([]interface{}, len(tt.tokenScopes))
			for i, scope := range tt.tokenScopes {
				scopesInterface[i] = scope
			}

			claims := jwt.MapClaims{
				"scopes": scopesInterface,
				"sub":    "test-device",
			}

			got := HasAllScopes(claims, tt.requiredScopes)
			if got != tt.want {
				t.Errorf("HasAllScopes() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHasAllScopes_InvalidClaimsFormat(t *testing.T) {
	tests := []struct {
		name   string
		claims jwt.MapClaims
		want   bool
	}{
		{
			name: "scopes is string instead of array",
			claims: jwt.MapClaims{
				"scopes": "read:api",
			},
			want: false,
		},
		{
			name: "scopes is number",
			claims: jwt.MapClaims{
				"scopes": 123,
			},
			want: false,
		},
		{
			name: "scopes is nil",
			claims: jwt.MapClaims{
				"scopes": nil,
			},
			want: false,
		},
		{
			name:   "scopes claim missing",
			claims: jwt.MapClaims{},
			want:   false,
		},
		{
			name: "scopes array contains non-strings",
			claims: jwt.MapClaims{
				"scopes": []interface{}{"write:api", 123, nil},
			},
			want: false, // "read:api" won't be found (only write:api is valid)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HasAllScopes(tt.claims, []string{"read:api"})
			if got != tt.want {
				t.Errorf("HasAllScopes() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHasAllScopesFromAny(t *testing.T) {
	tests := []struct {
		name           string
		tokenScopes    any
		requiredScopes []string
		want           bool
	}{
		{
			name:           "valid array - exact match",
			tokenScopes:    []interface{}{"read:api", "write:api"},
			requiredScopes: []string{"read:api"},
			want:           true,
		},
		{
			name:           "valid array - wildcard match",
			tokenScopes:    []interface{}{"read:*"},
			requiredScopes: []string{"read:api", "read:devices"},
			want:           true,
		},
		{
			name:           "invalid type - string",
			tokenScopes:    "read:api",
			requiredScopes: []string{"read:api"},
			want:           false,
		},
		{
			name:           "invalid type - map",
			tokenScopes:    map[string]string{"scope": "read:api"},
			requiredScopes: []string{"read:api"},
			want:           false,
		},
		{
			name:           "empty required scopes",
			tokenScopes:    []interface{}{"read:api"},
			requiredScopes: []string{},
			want:           true,
		},
		{
			name:           "array with non-strings",
			tokenScopes:    []interface{}{"read:api", 123, "write:api"},
			requiredScopes: []string{"write:api"},
			want:           true, // write:api is present
		},
		{
			name:           "nil token scopes",
			tokenScopes:    nil,
			requiredScopes: []string{"read:api"},
			want:           false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HasAllScopesFromAny(tt.tokenScopes, tt.requiredScopes)
			if got != tt.want {
				t.Errorf("HasAllScopesFromAny() = %v, want %v", got, tt.want)
			}
		})
	}
}

func BenchmarkHasAllScopes(b *testing.B) {
	claims := jwt.MapClaims{
		"scopes": []interface{}{"read:*", "write:api", "delete:devices"},
		"sub":    "test-device",
	}
	requiredScopes := []string{"read:api", "write:api", "read:devices"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = HasAllScopes(claims, requiredScopes)
	}
}

func BenchmarkHasAllScopesFromAny(b *testing.B) {
	tokenScopes := []interface{}{"read:*", "write:api", "delete:devices"}
	requiredScopes := []string{"read:api", "write:api", "read:devices"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = HasAllScopesFromAny(tokenScopes, requiredScopes)
	}
}
