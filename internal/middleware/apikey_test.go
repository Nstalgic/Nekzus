package middleware

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/nstalgic/nekzus/internal/storage"
	"github.com/nstalgic/nekzus/internal/types"
)

func TestAPIKeyAuth(t *testing.T) {
	// Create temporary database
	tmpDB := "test_apikey_auth.db"
	defer os.Remove(tmpDB)

	store, err := storage.NewStore(storage.Config{DatabasePath: tmpDB})
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Create test API key
	testKey := "nekzus_test1234567890abcdef"
	keyHash := hashAPIKey(testKey)

	apiKey := &types.APIKey{
		ID:        "test-key-1",
		Name:      "Test Key",
		KeyHash:   keyHash,
		Prefix:    "nekzus_tes",
		Scopes:    []string{"read:catalog", "read:events"},
		CreatedAt: time.Now(),
	}

	if err := store.CreateAPIKey(apiKey); err != nil {
		t.Fatalf("Failed to create test API key: %v", err)
	}

	// Create middleware
	middleware := NewAPIKeyAuth(store, nil)

	// Test handler that verifies API key was validated
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// API key is optional - only check if Authorization header was provided with nekzus_ prefix
		authHeader := r.Header.Get("Authorization")
		if len(authHeader) > 7 && strings.HasPrefix(authHeader[7:], "nekzus_") {
			apiKey := GetAPIKeyFromContext(r.Context())
			if apiKey == nil {
				// This should only happen for invalid keys, which fail before reaching handler
				t.Error("Expected API key in context for valid nekzus_ prefixed keys")
			}
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	})

	wrappedHandler := middleware(testHandler)

	t.Run("ValidAPIKey", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/test", nil)
		req.Header.Set("Authorization", "Bearer "+testKey)
		rec := httptest.NewRecorder()

		wrappedHandler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("ValidAPIKeyWithXAPIKeyHeader", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/test", nil)
		req.Header.Set("X-API-Key", testKey)
		rec := httptest.NewRecorder()

		wrappedHandler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("MissingAPIKey", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/test", nil)
		rec := httptest.NewRecorder()

		// Handler without API key requirement should still work
		// (API key auth is optional, JWT is primary)
		wrappedHandler.ServeHTTP(rec, req)

		// Should pass through if no API key provided (JWT middleware will handle auth)
		if rec.Code == http.StatusUnauthorized {
			// This is expected if API key auth is enforced
			body := rec.Body.String()
			if body != "missing API key\n" {
				t.Errorf("Expected 'missing API key' error, got: %s", body)
			}
		}
	})

	t.Run("InvalidAPIKey", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/test", nil)
		req.Header.Set("Authorization", "Bearer nekzus_invalid_key_123")
		rec := httptest.NewRecorder()

		wrappedHandler.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("Expected status 401, got %d", rec.Code)
		}
	})

	t.Run("RevokedAPIKey", func(t *testing.T) {
		// Create revoked API key
		revokedKey := "nekzus_revoked1234567890"
		revokedHash := hashAPIKey(revokedKey)

		now := time.Now()
		revokedAPIKey := &types.APIKey{
			ID:        "revoked-key",
			Name:      "Revoked Key",
			KeyHash:   revokedHash,
			Prefix:    "nekzus_rev",
			Scopes:    []string{"read:catalog"},
			CreatedAt: time.Now(),
			RevokedAt: &now,
		}

		if err := store.CreateAPIKey(revokedAPIKey); err != nil {
			t.Fatalf("Failed to create revoked API key: %v", err)
		}

		req := httptest.NewRequest("GET", "/api/test", nil)
		req.Header.Set("Authorization", "Bearer "+revokedKey)
		rec := httptest.NewRecorder()

		wrappedHandler.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("Expected status 401 for revoked key, got %d", rec.Code)
		}
	})

	t.Run("ExpiredAPIKey", func(t *testing.T) {
		// Create expired API key
		expiredKey := "nekzus_expired1234567890"
		expiredHash := hashAPIKey(expiredKey)

		pastTime := time.Now().Add(-1 * time.Hour)
		expiredAPIKey := &types.APIKey{
			ID:        "expired-key",
			Name:      "Expired Key",
			KeyHash:   expiredHash,
			Prefix:    "nekzus_exp",
			Scopes:    []string{"read:catalog"},
			CreatedAt: time.Now(),
			ExpiresAt: &pastTime,
		}

		if err := store.CreateAPIKey(expiredAPIKey); err != nil {
			t.Fatalf("Failed to create expired API key: %v", err)
		}

		req := httptest.NewRequest("GET", "/api/test", nil)
		req.Header.Set("Authorization", "Bearer "+expiredKey)
		rec := httptest.NewRecorder()

		wrappedHandler.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("Expected status 401 for expired key, got %d", rec.Code)
		}
	})

	t.Run("LastUsedUpdated", func(t *testing.T) {
		// Create API key
		lastUsedKey := "nekzus_lastused1234567890"
		lastUsedHash := hashAPIKey(lastUsedKey)

		lastUsedAPIKey := &types.APIKey{
			ID:        "lastused-key",
			Name:      "Last Used Key",
			KeyHash:   lastUsedHash,
			Prefix:    "nekzus_las",
			Scopes:    []string{"read:catalog"},
			CreatedAt: time.Now(),
		}

		if err := store.CreateAPIKey(lastUsedAPIKey); err != nil {
			t.Fatalf("Failed to create API key: %v", err)
		}

		req := httptest.NewRequest("GET", "/api/test", nil)
		req.Header.Set("Authorization", "Bearer "+lastUsedKey)
		rec := httptest.NewRecorder()

		wrappedHandler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", rec.Code)
		}

		// Give async update time to complete
		time.Sleep(100 * time.Millisecond)

		// Verify last_used was updated
		retrieved, err := store.GetAPIKey("lastused-key")
		if err != nil {
			t.Fatalf("Failed to retrieve API key: %v", err)
		}
		if retrieved.LastUsedAt == nil {
			t.Error("Expected LastUsedAt to be set")
		}
	})
}

// hashAPIKey creates SHA256 hash of API key
func hashAPIKey(key string) string {
	hash := sha256.Sum256([]byte(key))
	return hex.EncodeToString(hash[:])
}
