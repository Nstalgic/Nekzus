package storage

import (
	"os"
	"testing"
	"time"

	"github.com/nstalgic/nekzus/internal/types"
)

// TestAPIKeyStorage tests API key storage operations
func TestAPIKeyStorage(t *testing.T) {
	// Create temporary database
	tmpDB := "test_apikey.db"
	defer os.Remove(tmpDB)

	store, err := NewStore(Config{DatabasePath: tmpDB})
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	t.Run("CreateAPIKey", func(t *testing.T) {
		apiKey := &types.APIKey{
			ID:        "key-123",
			Name:      "Test API Key",
			KeyHash:   "hash123",
			Prefix:    "nekzus_abc",
			Scopes:    []string{"read:catalog", "read:events"},
			CreatedAt: time.Now(),
			CreatedBy: "test-device",
		}

		err := store.CreateAPIKey(apiKey)
		if err != nil {
			t.Fatalf("Failed to create API key: %v", err)
		}

		// Verify it was created
		retrieved, err := store.GetAPIKey(apiKey.ID)
		if err != nil {
			t.Fatalf("Failed to get API key: %v", err)
		}
		if retrieved == nil {
			t.Fatal("Expected API key to exist")
		}
		if retrieved.Name != apiKey.Name {
			t.Errorf("Expected name %s, got %s", apiKey.Name, retrieved.Name)
		}
		if retrieved.KeyHash != apiKey.KeyHash {
			t.Errorf("Expected key hash %s, got %s", apiKey.KeyHash, retrieved.KeyHash)
		}
		if len(retrieved.Scopes) != len(apiKey.Scopes) {
			t.Errorf("Expected %d scopes, got %d", len(apiKey.Scopes), len(retrieved.Scopes))
		}
	})

	t.Run("GetAPIKeyByHash", func(t *testing.T) {
		apiKey := &types.APIKey{
			ID:        "key-456",
			Name:      "Hash Test Key",
			KeyHash:   "hash456",
			Prefix:    "nekzus_def",
			Scopes:    []string{"read:*"},
			CreatedAt: time.Now(),
		}

		err := store.CreateAPIKey(apiKey)
		if err != nil {
			t.Fatalf("Failed to create API key: %v", err)
		}

		// Retrieve by hash
		retrieved, err := store.GetAPIKeyByHash("hash456")
		if err != nil {
			t.Fatalf("Failed to get API key by hash: %v", err)
		}
		if retrieved == nil {
			t.Fatal("Expected API key to exist")
		}
		if retrieved.ID != apiKey.ID {
			t.Errorf("Expected ID %s, got %s", apiKey.ID, retrieved.ID)
		}

		// Test non-existent hash
		notFound, err := store.GetAPIKeyByHash("nonexistent")
		if err != nil {
			t.Fatalf("Unexpected error for non-existent hash: %v", err)
		}
		if notFound != nil {
			t.Error("Expected nil for non-existent hash")
		}
	})

	t.Run("ListAPIKeys", func(t *testing.T) {
		// Create multiple API keys
		keys := []*types.APIKey{
			{
				ID:        "key-list-1",
				Name:      "List Test 1",
				KeyHash:   "hash-list-1",
				Prefix:    "nekzus_l1",
				Scopes:    []string{"read:catalog"},
				CreatedAt: time.Now(),
			},
			{
				ID:        "key-list-2",
				Name:      "List Test 2",
				KeyHash:   "hash-list-2",
				Prefix:    "nekzus_l2",
				Scopes:    []string{"read:events"},
				CreatedAt: time.Now(),
			},
		}

		for _, key := range keys {
			if err := store.CreateAPIKey(key); err != nil {
				t.Fatalf("Failed to create API key: %v", err)
			}
		}

		// List all keys
		allKeys, err := store.ListAPIKeys()
		if err != nil {
			t.Fatalf("Failed to list API keys: %v", err)
		}

		// Should have at least 2 keys (may have more from previous tests)
		if len(allKeys) < 2 {
			t.Errorf("Expected at least 2 keys, got %d", len(allKeys))
		}

		// Verify our keys are in the list
		foundCount := 0
		for _, key := range allKeys {
			if key.ID == "key-list-1" || key.ID == "key-list-2" {
				foundCount++
			}
		}
		if foundCount != 2 {
			t.Errorf("Expected to find 2 test keys, found %d", foundCount)
		}
	})

	t.Run("RevokeAPIKey", func(t *testing.T) {
		apiKey := &types.APIKey{
			ID:        "key-revoke",
			Name:      "Revoke Test Key",
			KeyHash:   "hash-revoke",
			Prefix:    "nekzus_rvk",
			Scopes:    []string{"read:catalog"},
			CreatedAt: time.Now(),
		}

		err := store.CreateAPIKey(apiKey)
		if err != nil {
			t.Fatalf("Failed to create API key: %v", err)
		}

		// Revoke the key
		err = store.RevokeAPIKey(apiKey.ID)
		if err != nil {
			t.Fatalf("Failed to revoke API key: %v", err)
		}

		// Verify it was revoked
		retrieved, err := store.GetAPIKey(apiKey.ID)
		if err != nil {
			t.Fatalf("Failed to get API key: %v", err)
		}
		if retrieved == nil {
			t.Fatal("Expected API key to exist")
		}
		if retrieved.RevokedAt == nil {
			t.Error("Expected RevokedAt to be set")
		}
	})

	t.Run("UpdateAPIKeyLastUsed", func(t *testing.T) {
		apiKey := &types.APIKey{
			ID:        "key-lastused",
			Name:      "Last Used Test Key",
			KeyHash:   "hash-lastused",
			Prefix:    "nekzus_lus",
			Scopes:    []string{"read:catalog"},
			CreatedAt: time.Now(),
		}

		err := store.CreateAPIKey(apiKey)
		if err != nil {
			t.Fatalf("Failed to create API key: %v", err)
		}

		// Update last used
		err = store.UpdateAPIKeyLastUsed(apiKey.ID)
		if err != nil {
			t.Fatalf("Failed to update last used: %v", err)
		}

		// Verify it was updated
		retrieved, err := store.GetAPIKey(apiKey.ID)
		if err != nil {
			t.Fatalf("Failed to get API key: %v", err)
		}
		if retrieved == nil {
			t.Fatal("Expected API key to exist")
		}
		if retrieved.LastUsedAt == nil {
			t.Error("Expected LastUsedAt to be set")
		}
	})

	t.Run("DeleteAPIKey", func(t *testing.T) {
		apiKey := &types.APIKey{
			ID:        "key-delete",
			Name:      "Delete Test Key",
			KeyHash:   "hash-delete",
			Prefix:    "nekzus_del",
			Scopes:    []string{"read:catalog"},
			CreatedAt: time.Now(),
		}

		err := store.CreateAPIKey(apiKey)
		if err != nil {
			t.Fatalf("Failed to create API key: %v", err)
		}

		// Delete the key
		err = store.DeleteAPIKey(apiKey.ID)
		if err != nil {
			t.Fatalf("Failed to delete API key: %v", err)
		}

		// Verify it was deleted
		retrieved, err := store.GetAPIKey(apiKey.ID)
		if err != nil {
			t.Fatalf("Failed to get API key: %v", err)
		}
		if retrieved != nil {
			t.Error("Expected API key to be deleted")
		}
	})

	t.Run("ExpirationHandling", func(t *testing.T) {
		past := time.Now().Add(-1 * time.Hour)
		future := time.Now().Add(1 * time.Hour)

		expiredKey := &types.APIKey{
			ID:        "key-expired",
			Name:      "Expired Key",
			KeyHash:   "hash-expired",
			Prefix:    "nekzus_exp",
			Scopes:    []string{"read:catalog"},
			CreatedAt: time.Now(),
			ExpiresAt: &past,
		}

		validKey := &types.APIKey{
			ID:        "key-valid",
			Name:      "Valid Key",
			KeyHash:   "hash-valid",
			Prefix:    "nekzus_val",
			Scopes:    []string{"read:catalog"},
			CreatedAt: time.Now(),
			ExpiresAt: &future,
		}

		if err := store.CreateAPIKey(expiredKey); err != nil {
			t.Fatalf("Failed to create expired key: %v", err)
		}
		if err := store.CreateAPIKey(validKey); err != nil {
			t.Fatalf("Failed to create valid key: %v", err)
		}

		// Verify expiration times are stored correctly
		retrievedExpired, err := store.GetAPIKey(expiredKey.ID)
		if err != nil {
			t.Fatalf("Failed to get expired key: %v", err)
		}
		if retrievedExpired.ExpiresAt == nil {
			t.Error("Expected ExpiresAt to be set")
		}

		retrievedValid, err := store.GetAPIKey(validKey.ID)
		if err != nil {
			t.Fatalf("Failed to get valid key: %v", err)
		}
		if retrievedValid.ExpiresAt == nil {
			t.Error("Expected ExpiresAt to be set")
		}
	})
}
