package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/nstalgic/nekzus/internal/storage"
	"github.com/nstalgic/nekzus/internal/types"
)

func TestAPIKeyHandlers(t *testing.T) {
	// Create temporary database
	tmpDB := "test_apikey_handlers.db"
	defer os.Remove(tmpDB)

	store, err := storage.NewStore(storage.Config{DatabasePath: tmpDB})
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	handler := NewAPIKeyHandler(store)

	t.Run("CreateAPIKey", func(t *testing.T) {
		reqBody := types.APIKeyRequest{
			Name:   "Test API Key",
			Scopes: []string{"read:catalog", "read:events"},
		}

		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest("POST", "/api/v1/apikeys", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		handler.HandleCreateAPIKey(rec, req)

		if rec.Code != http.StatusCreated {
			t.Errorf("Expected status 201, got %d: %s", rec.Code, rec.Body.String())
		}

		// Parse response
		var response types.APIKeyResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
			t.Fatalf("Failed to parse response: %v", err)
		}

		// Verify response contains key (only on creation)
		if response.Key == "" {
			t.Error("Expected plaintext key in response")
		}
		if !startsWithPrefix(response.Key, "nekzus_") {
			t.Errorf("Expected key to start with nekzus_, got: %s", response.Key)
		}
		if response.Name != reqBody.Name {
			t.Errorf("Expected name %s, got %s", reqBody.Name, response.Name)
		}
		if len(response.Scopes) != len(reqBody.Scopes) {
			t.Errorf("Expected %d scopes, got %d", len(reqBody.Scopes), len(response.Scopes))
		}
	})

	t.Run("CreateAPIKeyWithExpiration", func(t *testing.T) {
		expiresAt := time.Now().Add(24 * time.Hour)
		reqBody := types.APIKeyRequest{
			Name:      "Expiring Key",
			Scopes:    []string{"read:catalog"},
			ExpiresAt: &expiresAt,
		}

		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest("POST", "/api/v1/apikeys", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		handler.HandleCreateAPIKey(rec, req)

		if rec.Code != http.StatusCreated {
			t.Errorf("Expected status 201, got %d: %s", rec.Code, rec.Body.String())
		}

		var response types.APIKeyResponse
		json.Unmarshal(rec.Body.Bytes(), &response)

		if response.ExpiresAt == nil {
			t.Error("Expected ExpiresAt to be set")
		}
	})

	t.Run("CreateAPIKeyInvalidRequest", func(t *testing.T) {
		reqBody := types.APIKeyRequest{
			Name:   "", // Empty name should fail validation
			Scopes: []string{"read:catalog"},
		}

		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest("POST", "/api/v1/apikeys", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		handler.HandleCreateAPIKey(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("Expected status 400, got %d", rec.Code)
		}
	})

	t.Run("ListAPIKeys", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/apikeys", nil)
		rec := httptest.NewRecorder()

		handler.HandleListAPIKeys(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d: %s", rec.Code, rec.Body.String())
		}

		var keys []*types.APIKey
		if err := json.Unmarshal(rec.Body.Bytes(), &keys); err != nil {
			t.Fatalf("Failed to parse response: %v", err)
		}

		// Should have at least 1 key from previous test
		if len(keys) < 1 {
			t.Errorf("Expected at least 1 key, got %d", len(keys))
		}

		// Verify keys don't include hash
		for _, key := range keys {
			if key.KeyHash != "" {
				t.Error("API key hash should not be returned in list")
			}
		}
	})

	t.Run("GetAPIKey", func(t *testing.T) {
		// Create a key first
		apiKey := &types.APIKey{
			ID:        "test-get-key",
			Name:      "Get Test Key",
			KeyHash:   "test-hash",
			Prefix:    "nekzus_get",
			Scopes:    []string{"read:catalog"},
			CreatedAt: time.Now(),
		}
		store.CreateAPIKey(apiKey)

		req := httptest.NewRequest("GET", "/api/v1/apikeys/test-get-key", nil)
		rec := httptest.NewRecorder()

		handler.HandleGetAPIKey(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d: %s", rec.Code, rec.Body.String())
		}

		var response types.APIKey
		if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
			t.Fatalf("Failed to parse response: %v", err)
		}

		if response.ID != apiKey.ID {
			t.Errorf("Expected ID %s, got %s", apiKey.ID, response.ID)
		}
	})

	t.Run("RevokeAPIKey", func(t *testing.T) {
		// Create a key first
		apiKey := &types.APIKey{
			ID:        "test-revoke-key",
			Name:      "Revoke Test Key",
			KeyHash:   "test-hash-revoke",
			Prefix:    "nekzus_rvk",
			Scopes:    []string{"read:catalog"},
			CreatedAt: time.Now(),
		}
		store.CreateAPIKey(apiKey)

		req := httptest.NewRequest("DELETE", "/api/v1/apikeys/test-revoke-key", nil)
		rec := httptest.NewRecorder()

		handler.HandleRevokeAPIKey(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d: %s", rec.Code, rec.Body.String())
		}

		// Verify key was revoked
		revoked, err := store.GetAPIKey("test-revoke-key")
		if err != nil {
			t.Fatalf("Failed to get revoked key: %v", err)
		}
		if revoked.RevokedAt == nil {
			t.Error("Expected RevokedAt to be set")
		}
	})

	t.Run("DeleteAPIKey", func(t *testing.T) {
		// Create a key first
		apiKey := &types.APIKey{
			ID:        "test-delete-key",
			Name:      "Delete Test Key",
			KeyHash:   "test-hash-delete",
			Prefix:    "nekzus_del",
			Scopes:    []string{"read:catalog"},
			CreatedAt: time.Now(),
		}
		store.CreateAPIKey(apiKey)

		req := httptest.NewRequest("DELETE", "/api/v1/apikeys/test-delete-key?permanent=true", nil)
		rec := httptest.NewRecorder()

		handler.HandleRevokeAPIKey(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d: %s", rec.Code, rec.Body.String())
		}

		// Verify key was deleted
		deleted, err := store.GetAPIKey("test-delete-key")
		if err != nil {
			t.Fatalf("Failed to check deleted key: %v", err)
		}
		if deleted != nil {
			t.Error("Expected key to be deleted")
		}
	})

	t.Run("GetNonexistentAPIKey", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/apikeys/nonexistent", nil)
		rec := httptest.NewRecorder()

		handler.HandleGetAPIKey(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Errorf("Expected status 404, got %d", rec.Code)
		}
	})
}

func startsWithPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
