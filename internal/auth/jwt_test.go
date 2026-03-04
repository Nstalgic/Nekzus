package auth

import (
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestNewManager(t *testing.T) {
	tests := []struct {
		name        string
		jwtSecret   string
		issuer      string
		audience    string
		tokens      []string
		wantErr     bool
		errContains string
	}{
		{
			name:      "valid config",
			jwtSecret: strings.Repeat("a", 32),
			issuer:    "test-issuer",
			audience:  "test-audience",
			tokens:    []string{"token1"},
			wantErr:   false,
		},
		{
			name:        "secret too short",
			jwtSecret:   "short",
			wantErr:     true,
			errContains: "at least 32 characters",
		},
		{
			name:        "weak secret in prod",
			jwtSecret:   "this-secret-contains-password-12345",
			wantErr:     true,
			errContains: "weak pattern",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr, err := NewManager([]byte(tt.jwtSecret), tt.issuer, tt.audience, tt.tokens)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				} else if !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("error %q should contain %q", err.Error(), tt.errContains)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if mgr == nil {
				t.Fatal("expected manager, got nil")
			}

			// Clean up bootstrap store
			mgr.Stop()

			if mgr.issuer != tt.issuer && tt.issuer != "" {
				t.Errorf("issuer = %q, want %q", mgr.issuer, tt.issuer)
			}
		})
	}
}

// ValidateBootstrapAllowed should be called in NewManager
func TestNewManager_ValidateBootstrapAllowed(t *testing.T) {
	// Save original environment
	originalEnv := os.Getenv("ENVIRONMENT")
	originalBypass := os.Getenv("NEKZUS_BOOTSTRAP_ALLOW_ANY")
	defer func() {
		os.Setenv("ENVIRONMENT", originalEnv)
		os.Setenv("NEKZUS_BOOTSTRAP_ALLOW_ANY", originalBypass)
	}()

	// Test that NEKZUS_BOOTSTRAP_ALLOW_ANY=1 in production causes error
	os.Setenv("ENVIRONMENT", "production")
	os.Setenv("NEKZUS_BOOTSTRAP_ALLOW_ANY", "1")

	mgr, err := NewManager(
		[]byte(strings.Repeat("a", 32)),
		"test-issuer",
		"test-audience",
		[]string{},
	)

	if err == nil {
		if mgr != nil {
			mgr.Stop()
		}
		t.Error("expected error when NEKZUS_BOOTSTRAP_ALLOW_ANY=1 in production, got nil")
	}

	if err != nil && !strings.Contains(err.Error(), "not allowed in environment: production") {
		t.Errorf("unexpected error: %v", err)
	}

	// Test that it's allowed in dev environment
	os.Setenv("ENVIRONMENT", "development")
	mgr, err = NewManager(
		[]byte(strings.Repeat("a", 32)),
		"test-issuer",
		"test-audience",
		[]string{},
	)

	if err != nil {
		t.Errorf("unexpected error in dev environment: %v", err)
	}

	if mgr != nil {
		mgr.Stop()
	}
}

func TestSignAndParseJWT(t *testing.T) {
	mgr, err := NewManager(
		[]byte(strings.Repeat("a", 32)),
		"test-issuer",
		"test-audience",
		[]string{},
	)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}
	defer mgr.Stop()

	deviceID := "test-device-123"
	scopes := []string{"read:catalog", "write:data"}
	ttl := 1 * time.Hour

	// Sign JWT
	token, err := mgr.SignJWT(deviceID, scopes, ttl)
	if err != nil {
		t.Fatalf("SignJWT failed: %v", err)
	}

	if token == "" {
		t.Fatal("expected non-empty token")
	}

	// Parse JWT
	_, claims, err := mgr.ParseJWT(token)
	if err != nil {
		t.Fatalf("ParseJWT failed: %v", err)
	}

	// Verify claims
	if sub, _ := claims["sub"].(string); sub != deviceID {
		t.Errorf("sub = %q, want %q", sub, deviceID)
	}

	if iss, _ := claims["iss"].(string); iss != "test-issuer" {
		t.Errorf("iss = %q, want %q", iss, "test-issuer")
	}

	if aud, _ := claims["aud"].(string); aud != "test-audience" {
		t.Errorf("aud = %q, want %q", aud, "test-audience")
	}

	extractedScopes := ExtractScopes(claims)
	if len(extractedScopes) != len(scopes) {
		t.Errorf("got %d scopes, want %d", len(extractedScopes), len(scopes))
	}
}

func TestParseJWT_Invalid(t *testing.T) {
	mgr, _ := NewManager(
		[]byte(strings.Repeat("a", 32)),
		"test-issuer",
		"test-audience",
		[]string{},
	)
	defer mgr.Stop()

	tests := []struct {
		name    string
		token   string
		wantErr bool
	}{
		{
			name:    "empty token",
			token:   "",
			wantErr: true,
		},
		{
			name:    "malformed token",
			token:   "not.a.token",
			wantErr: true,
		},
		{
			name:    "random string",
			token:   "random-string",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := mgr.ParseJWT(tt.token)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseJWT() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestParseJWT_IssuerMismatch
func TestParseJWT_IssuerMismatch(t *testing.T) {
	// Create a manager with specific issuer
	mgr1, err := NewManager(
		[]byte(strings.Repeat("a", 32)),
		"issuer-1",
		"test-audience",
		[]string{},
	)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}
	defer mgr1.Stop()

	// Create another manager with different issuer
	mgr2, err := NewManager(
		[]byte(strings.Repeat("a", 32)),
		"issuer-2",
		"test-audience",
		[]string{},
	)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}
	defer mgr2.Stop()

	// Sign with mgr1
	token, err := mgr1.SignJWT("device-123", []string{"read"}, time.Hour)
	if err != nil {
		t.Fatalf("SignJWT failed: %v", err)
	}

	// Parse with mgr2 (different issuer) should fail
	_, _, err = mgr2.ParseJWT(token)
	if err == nil {
		t.Error("expected error for issuer mismatch, got nil")
	}

	if err != nil && !strings.Contains(err.Error(), "issuer") {
		t.Errorf("expected issuer error, got: %v", err)
	}
}

// TestParseJWT_AudienceMismatch
func TestParseJWT_AudienceMismatch(t *testing.T) {
	// Create a manager with specific audience
	mgr1, err := NewManager(
		[]byte(strings.Repeat("a", 32)),
		"test-issuer",
		"audience-1",
		[]string{},
	)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}
	defer mgr1.Stop()

	// Create another manager with different audience
	mgr2, err := NewManager(
		[]byte(strings.Repeat("a", 32)),
		"test-issuer",
		"audience-2",
		[]string{},
	)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}
	defer mgr2.Stop()

	// Sign with mgr1
	token, err := mgr1.SignJWT("device-123", []string{"read"}, time.Hour)
	if err != nil {
		t.Fatalf("SignJWT failed: %v", err)
	}

	// Parse with mgr2 (different audience) should fail
	_, _, err = mgr2.ParseJWT(token)
	if err == nil {
		t.Error("expected error for audience mismatch, got nil")
	}

	if err != nil && !strings.Contains(err.Error(), "audience") {
		t.Errorf("expected audience error, got: %v", err)
	}
}

// TestParseJWT_Expired
func TestParseJWT_Expired(t *testing.T) {
	mgr, err := NewManager(
		[]byte(strings.Repeat("a", 32)),
		"test-issuer",
		"test-audience",
		[]string{},
	)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}
	defer mgr.Stop()

	// Sign with very short TTL
	token, err := mgr.SignJWT("device-123", []string{"read"}, 1*time.Millisecond)
	if err != nil {
		t.Fatalf("SignJWT failed: %v", err)
	}

	// Wait for token to expire
	time.Sleep(10 * time.Millisecond)

	// Parse should fail due to expiration
	_, _, err = mgr.ParseJWT(token)
	if err == nil {
		t.Error("expected error for expired token, got nil")
	}
}

// Race Condition in UpdateBootstrapTokens
func TestManager_UpdateBootstrapTokens_RaceCondition(t *testing.T) {
	mgr, err := NewManager(
		[]byte(strings.Repeat("a", 32)),
		"test-issuer",
		"test-audience",
		[]string{"initial-token"},
	)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}
	defer mgr.Stop()

	// Test concurrent access
	var wg sync.WaitGroup
	done := make(chan struct{})

	// Run multiple goroutines updating tokens
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for {
				select {
				case <-done:
					return
				default:
					tokens := []string{"token-" + string(rune('A'+n))}
					mgr.UpdateBootstrapTokens(tokens)
				}
			}
		}(i)
	}

	// Run multiple goroutines validating tokens
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-done:
					return
				default:
					mgr.ValidateBootstrap("any-token")
				}
			}
		}()
	}

	// Let it run for a bit
	time.Sleep(100 * time.Millisecond)
	close(done)
	wg.Wait()
}

// Test tokens should be restricted
func TestDetermineScopes(t *testing.T) {
	// Save original env
	originalDebug := os.Getenv("NEKZUS_DEBUG_TOKENS")
	defer os.Setenv("NEKZUS_DEBUG_TOKENS", originalDebug)

	tests := []struct {
		name     string
		platform string
		env      map[string]string
		want     []string
	}{
		{
			name:     "ios",
			platform: "ios",
			want:     []string{"read:catalog", "read:events", "access:mobile", "access:*", "read:*"},
		},
		{
			name:     "android",
			platform: "android",
			want:     []string{"read:catalog", "read:events", "access:mobile", "access:*", "read:*"},
		},
		{
			name:     "web",
			platform: "web",
			want:     []string{"read:catalog", "read:events", "access:mobile", "access:*", "read:*"},
		},
		{
			name:     "unknown",
			platform: "unknown",
			want:     []string{"read:catalog", "read:events"},
		},
		{
			name:     "test platform - restricted",
			platform: "test",
			want:     []string{"read:catalog", "read:events", "read:*"},
		},
		{
			name:     "testcontainers platform - restricted",
			platform: "testcontainers",
			want:     []string{"read:catalog", "read:events", "read:*", "write:deployments"},
		},
		{
			name:     "debug platform without env - minimal",
			platform: "debug",
			want:     []string{"read:catalog", "read:events"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear debug env for most tests
			os.Setenv("NEKZUS_DEBUG_TOKENS", "")

			got := DetermineScopes(tt.platform)
			if len(got) != len(tt.want) {
				t.Errorf("got %d scopes %v, want %d scopes %v", len(got), got, len(tt.want), tt.want)
				return
			}
			for i, scope := range tt.want {
				if got[i] != scope {
					t.Errorf("scope[%d] = %q, want %q", i, got[i], scope)
				}
			}
		})
	}
}

// Test debug platform with env enabled
func TestDetermineScopes_DebugWithEnv(t *testing.T) {
	// Save original env
	originalDebug := os.Getenv("NEKZUS_DEBUG_TOKENS")
	defer os.Setenv("NEKZUS_DEBUG_TOKENS", originalDebug)

	// Enable debug tokens
	os.Setenv("NEKZUS_DEBUG_TOKENS", "1")

	got := DetermineScopes("debug")
	want := []string{"read:catalog", "read:events", "access:admin", "read:*", "write:*"}

	if len(got) != len(want) {
		t.Errorf("got %d scopes %v, want %d scopes %v", len(got), got, len(want), want)
		return
	}

	for i, scope := range want {
		if got[i] != scope {
			t.Errorf("scope[%d] = %q, want %q", i, got[i], scope)
		}
	}
}

func TestValidateJWTSecret(t *testing.T) {
	tests := []struct {
		name    string
		secret  string
		wantErr bool
	}{
		{
			name:    "valid long secret",
			secret:  strings.Repeat("a", 32),
			wantErr: false,
		},
		{
			name:    "too short",
			secret:  "short",
			wantErr: true,
		},
		{
			name:    "exactly 32 chars",
			secret:  strings.Repeat("x", 32),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateJWTSecret(tt.secret)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateJWTSecret() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// Test that Manager.Stop properly stops the bootstrap store
func TestManager_Stop(t *testing.T) {
	mgr, err := NewManager(
		[]byte(strings.Repeat("a", 32)),
		"test-issuer",
		"test-audience",
		[]string{"token1"},
	)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	// Stop should not panic
	mgr.Stop()

	// Double stop should not panic
	mgr.Stop()
}

// Test concurrent UpdateBootstrapTokens stops old stores
func TestManager_UpdateBootstrapTokens_StopsOldStore(t *testing.T) {
	mgr, err := NewManager(
		[]byte(strings.Repeat("a", 32)),
		"test-issuer",
		"test-audience",
		[]string{"initial-token"},
	)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}
	defer mgr.Stop()

	// Update tokens multiple times - old stores should be stopped
	for i := 0; i < 5; i++ {
		mgr.UpdateBootstrapTokens([]string{"token-" + string(rune('A'+i))})
	}

	// Final validation should work
	if !mgr.ValidateBootstrap("token-E") {
		t.Error("expected token-E to be valid")
	}
}

// Test that manually created token with wrong issuer fails
func TestParseJWT_ManualTokenWrongIssuer(t *testing.T) {
	mgr, err := NewManager(
		[]byte(strings.Repeat("a", 32)),
		"correct-issuer",
		"test-audience",
		[]string{},
	)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}
	defer mgr.Stop()

	// Manually create a token with wrong issuer
	claims := jwt.MapClaims{
		"iss":    "wrong-issuer",
		"aud":    "test-audience",
		"sub":    "device-123",
		"scopes": []string{"read"},
		"iat":    time.Now().Unix(),
		"exp":    time.Now().Add(time.Hour).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(strings.Repeat("a", 32)))
	if err != nil {
		t.Fatalf("failed to sign token: %v", err)
	}

	_, _, err = mgr.ParseJWT(tokenString)
	if err == nil {
		t.Error("expected error for wrong issuer")
	}
}

// Test that manually created token with wrong audience fails
func TestParseJWT_ManualTokenWrongAudience(t *testing.T) {
	mgr, err := NewManager(
		[]byte(strings.Repeat("a", 32)),
		"test-issuer",
		"correct-audience",
		[]string{},
	)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}
	defer mgr.Stop()

	// Manually create a token with wrong audience
	claims := jwt.MapClaims{
		"iss":    "test-issuer",
		"aud":    "wrong-audience",
		"sub":    "device-123",
		"scopes": []string{"read"},
		"iat":    time.Now().Unix(),
		"exp":    time.Now().Add(time.Hour).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(strings.Repeat("a", 32)))
	if err != nil {
		t.Fatalf("failed to sign token: %v", err)
	}

	_, _, err = mgr.ParseJWT(tokenString)
	if err == nil {
		t.Error("expected error for wrong audience")
	}
}

func TestValidateBootstrapAllowed(t *testing.T) {
	tests := []struct {
		name           string
		allowAnyEnv    string
		environmentEnv string
		wantErr        bool
		errContains    string
	}{
		{
			name:           "bypass not set - should pass",
			allowAnyEnv:    "",
			environmentEnv: "",
			wantErr:        false,
		},
		{
			name:           "bypass disabled explicitly - should pass",
			allowAnyEnv:    "0",
			environmentEnv: "",
			wantErr:        false,
		},
		{
			name:           "bypass enabled in development - should pass",
			allowAnyEnv:    "1",
			environmentEnv: "development",
			wantErr:        false,
		},
		{
			name:           "bypass enabled in dev - should pass",
			allowAnyEnv:    "1",
			environmentEnv: "dev",
			wantErr:        false,
		},
		{
			name:           "bypass enabled in test - should pass",
			allowAnyEnv:    "1",
			environmentEnv: "test",
			wantErr:        false,
		},
		{
			name:           "bypass enabled without ENVIRONMENT set - should fail (fail closed)",
			allowAnyEnv:    "1",
			environmentEnv: "",
			wantErr:        true,
			errContains:    "requires explicit ENVIRONMENT=development",
		},
		{
			name:           "bypass enabled in production - should fail",
			allowAnyEnv:    "1",
			environmentEnv: "production",
			wantErr:        true,
			errContains:    "not allowed in environment: production",
		},
		{
			name:           "bypass enabled in staging - should fail",
			allowAnyEnv:    "1",
			environmentEnv: "staging",
			wantErr:        true,
			errContains:    "not allowed in environment: staging",
		},
		{
			name:           "bypass enabled with mixed case DEVELOPMENT - should pass",
			allowAnyEnv:    "1",
			environmentEnv: "DEVELOPMENT",
			wantErr:        false,
		},
		{
			name:           "bypass enabled with mixed case PRODUCTION - should fail",
			allowAnyEnv:    "1",
			environmentEnv: "PRODUCTION",
			wantErr:        true,
			errContains:    "not allowed in environment: production",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save original env vars
			origAllowAny := os.Getenv("NEKZUS_BOOTSTRAP_ALLOW_ANY")
			origEnvironment := os.Getenv("ENVIRONMENT")

			// Set test env vars
			if tt.allowAnyEnv != "" {
				os.Setenv("NEKZUS_BOOTSTRAP_ALLOW_ANY", tt.allowAnyEnv)
			} else {
				os.Unsetenv("NEKZUS_BOOTSTRAP_ALLOW_ANY")
			}

			if tt.environmentEnv != "" {
				os.Setenv("ENVIRONMENT", tt.environmentEnv)
			} else {
				os.Unsetenv("ENVIRONMENT")
			}

			// Restore env vars after test
			defer func() {
				if origAllowAny != "" {
					os.Setenv("NEKZUS_BOOTSTRAP_ALLOW_ANY", origAllowAny)
				} else {
					os.Unsetenv("NEKZUS_BOOTSTRAP_ALLOW_ANY")
				}
				if origEnvironment != "" {
					os.Setenv("ENVIRONMENT", origEnvironment)
				} else {
					os.Unsetenv("ENVIRONMENT")
				}
			}()

			// Run validation
			err := ValidateBootstrapAllowed()

			// Check results
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errContains)
					return
				}
				if !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("expected error containing %q, got %q", tt.errContains, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("expected no error, got %v", err)
				}
			}
		})
	}
}
