package config

import (
	"errors"
	"strings"
	"testing"

	"github.com/nstalgic/nekzus/internal/types"
)

// errSimulated is used for testing error paths
var errSimulated = errors.New("simulated error")

// mockSecretStore implements SecretStore for testing
type mockSecretStore struct {
	secrets map[string]string
	getErr  error
	setErr  error
}

func newMockSecretStore() *mockSecretStore {
	return &mockSecretStore{
		secrets: make(map[string]string),
	}
}

func (m *mockSecretStore) GetSystemSecret(key string) (string, error) {
	if m.getErr != nil {
		return "", m.getErr
	}
	return m.secrets[key], nil
}

func (m *mockSecretStore) SetSystemSecret(key, value string) error {
	if m.setErr != nil {
		return m.setErr
	}
	m.secrets[key] = value
	return nil
}

func TestEnsureJWTSecret_UserProvided(t *testing.T) {
	store := newMockSecretStore()
	cfg := &types.ServerConfig{}
	cfg.Auth.HS256Secret = "this-is-a-valid-secret-that-is-at-least-32-chars"

	err := EnsureJWTSecret(cfg, store)
	if err != nil {
		t.Errorf("EnsureJWTSecret() error = %v, want nil", err)
	}

	// Secret should remain unchanged
	if cfg.Auth.HS256Secret != "this-is-a-valid-secret-that-is-at-least-32-chars" {
		t.Errorf("Secret was modified unexpectedly")
	}

	// Nothing should be stored in the database
	if len(store.secrets) != 0 {
		t.Errorf("Unexpected secrets stored in database: %v", store.secrets)
	}
}

func TestEnsureJWTSecret_UserProvidedTooShort(t *testing.T) {
	store := newMockSecretStore()
	cfg := &types.ServerConfig{}
	cfg.Auth.HS256Secret = "short"

	err := EnsureJWTSecret(cfg, store)
	if err == nil {
		t.Error("EnsureJWTSecret() error = nil, want error for short secret")
	}
	if !strings.Contains(err.Error(), "at least") {
		t.Errorf("Error message should mention minimum length, got: %v", err)
	}
}

func TestEnsureJWTSecret_LoadFromDatabase(t *testing.T) {
	store := newMockSecretStore()
	store.secrets[JWTSecretKey] = "stored-secret-from-database-that-is-long-enough"

	cfg := &types.ServerConfig{}

	err := EnsureJWTSecret(cfg, store)
	if err != nil {
		t.Errorf("EnsureJWTSecret() error = %v, want nil", err)
	}

	// Secret should be loaded from database
	if cfg.Auth.HS256Secret != "stored-secret-from-database-that-is-long-enough" {
		t.Errorf("Secret = %q, want value from database", cfg.Auth.HS256Secret)
	}
}

func TestEnsureJWTSecret_AutoGenerate(t *testing.T) {
	store := newMockSecretStore()
	cfg := &types.ServerConfig{}

	err := EnsureJWTSecret(cfg, store)
	if err != nil {
		t.Errorf("EnsureJWTSecret() error = %v, want nil", err)
	}

	// Secret should be generated
	if cfg.Auth.HS256Secret == "" {
		t.Error("Secret should be auto-generated, got empty string")
	}

	// Generated secret should be at least MinJWTSecretLength
	if len(cfg.Auth.HS256Secret) < MinJWTSecretLength {
		t.Errorf("Generated secret length = %d, want >= %d", len(cfg.Auth.HS256Secret), MinJWTSecretLength)
	}

	// Secret should be stored in database
	storedSecret, ok := store.secrets[JWTSecretKey]
	if !ok {
		t.Error("Secret was not stored in database")
	}
	if storedSecret != cfg.Auth.HS256Secret {
		t.Errorf("Stored secret = %q, want %q", storedSecret, cfg.Auth.HS256Secret)
	}
}

func TestEnsureJWTSecret_AutoGenerateUnique(t *testing.T) {
	// Verify that multiple calls generate unique secrets
	secrets := make(map[string]bool)

	for i := 0; i < 10; i++ {
		store := newMockSecretStore()
		cfg := &types.ServerConfig{}

		err := EnsureJWTSecret(cfg, store)
		if err != nil {
			t.Fatalf("EnsureJWTSecret() error = %v", err)
		}

		if secrets[cfg.Auth.HS256Secret] {
			t.Error("Generated duplicate secret")
		}
		secrets[cfg.Auth.HS256Secret] = true
	}
}

func TestEnsureJWTSecret_UserProvidedTakesPrecedence(t *testing.T) {
	store := newMockSecretStore()
	// Pre-populate database with a different secret
	store.secrets[JWTSecretKey] = "database-secret-that-should-not-be-used"

	cfg := &types.ServerConfig{}
	cfg.Auth.HS256Secret = "user-provided-secret-that-is-long-enough"

	err := EnsureJWTSecret(cfg, store)
	if err != nil {
		t.Errorf("EnsureJWTSecret() error = %v, want nil", err)
	}

	// User-provided secret should take precedence
	if cfg.Auth.HS256Secret != "user-provided-secret-that-is-long-enough" {
		t.Errorf("Secret = %q, want user-provided value", cfg.Auth.HS256Secret)
	}
}

func TestGenerateSecureSecret(t *testing.T) {
	t.Run("GeneratesCorrectLength", func(t *testing.T) {
		secret, err := generateSecureSecret(32)
		if err != nil {
			t.Errorf("generateSecureSecret() error = %v", err)
		}
		// Base64 encoded 32 bytes = 43 characters (with raw URL encoding)
		if len(secret) < 32 {
			t.Errorf("Generated secret length = %d, want >= 32", len(secret))
		}
	})

	t.Run("GeneratesUniqueValues", func(t *testing.T) {
		seen := make(map[string]bool)
		for i := 0; i < 100; i++ {
			secret, err := generateSecureSecret(32)
			if err != nil {
				t.Fatalf("generateSecureSecret() error = %v", err)
			}
			if seen[secret] {
				t.Error("Generated duplicate secret")
			}
			seen[secret] = true
		}
	})
}

func TestGetCookieEncryptionKey_AutoGenerate(t *testing.T) {
	store := newMockSecretStore()

	key, err := GetCookieEncryptionKey(store)
	if err != nil {
		t.Errorf("GetCookieEncryptionKey() error = %v, want nil", err)
	}

	// Key should be generated with correct length
	if len(key) != CookieEncryptionKeyLength {
		t.Errorf("Key length = %d, want %d", len(key), CookieEncryptionKeyLength)
	}

	// Key should be stored in database
	if _, ok := store.secrets[CookieEncryptionKey]; !ok {
		t.Error("Key was not stored in database")
	}
}

func TestGetCookieEncryptionKey_LoadFromDatabase(t *testing.T) {
	store := newMockSecretStore()

	// First call generates and stores the key
	key1, err := GetCookieEncryptionKey(store)
	if err != nil {
		t.Fatalf("GetCookieEncryptionKey() error = %v", err)
	}

	// Second call should retrieve the same key
	key2, err := GetCookieEncryptionKey(store)
	if err != nil {
		t.Fatalf("GetCookieEncryptionKey() second call error = %v", err)
	}

	// Keys should be identical
	if string(key1) != string(key2) {
		t.Error("Keys from consecutive calls should be identical")
	}
}

func TestGetCookieEncryptionKey_Unique(t *testing.T) {
	// Verify that multiple stores generate unique keys
	keys := make(map[string]bool)

	for i := 0; i < 10; i++ {
		store := newMockSecretStore()

		key, err := GetCookieEncryptionKey(store)
		if err != nil {
			t.Fatalf("GetCookieEncryptionKey() error = %v", err)
		}

		keyStr := string(key)
		if keys[keyStr] {
			t.Error("Generated duplicate key")
		}
		keys[keyStr] = true
	}
}

func TestGetCookieEncryptionKey_GetError(t *testing.T) {
	store := newMockSecretStore()
	store.getErr = errSimulated

	_, err := GetCookieEncryptionKey(store)
	if err == nil {
		t.Error("GetCookieEncryptionKey() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "failed to check") {
		t.Errorf("Error message should mention check failure, got: %v", err)
	}
}

func TestGetCookieEncryptionKey_SetError(t *testing.T) {
	store := newMockSecretStore()
	store.setErr = errSimulated

	_, err := GetCookieEncryptionKey(store)
	if err == nil {
		t.Error("GetCookieEncryptionKey() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "failed to store") {
		t.Errorf("Error message should mention store failure, got: %v", err)
	}
}

func TestGetCookieEncryptionKey_ValidKeyFromDatabase(t *testing.T) {
	store := newMockSecretStore()

	// First, generate a key and store it
	key1, err := GetCookieEncryptionKey(store)
	if err != nil {
		t.Fatalf("GetCookieEncryptionKey() error = %v", err)
	}

	// Create a new store with the same stored key
	store2 := newMockSecretStore()
	store2.secrets[CookieEncryptionKey] = store.secrets[CookieEncryptionKey]

	// Retrieve from the second store
	key2, err := GetCookieEncryptionKey(store2)
	if err != nil {
		t.Fatalf("GetCookieEncryptionKey() from store2 error = %v", err)
	}

	// Keys should be identical
	if string(key1) != string(key2) {
		t.Error("Keys retrieved from different stores with same secret should be identical")
	}
}
