package storage

import (
	"os"
	"testing"
)

func TestSystemSecrets(t *testing.T) {
	// Create temp database
	tmpFile, err := os.CreateTemp("", "nexus-test-*.db")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	store, err := NewStore(Config{DatabasePath: tmpFile.Name()})
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	t.Run("GetSystemSecret_NotExists", func(t *testing.T) {
		value, err := store.GetSystemSecret("nonexistent_key")
		if err != nil {
			t.Errorf("GetSystemSecret() error = %v, want nil", err)
		}
		if value != "" {
			t.Errorf("GetSystemSecret() = %q, want empty string", value)
		}
	})

	t.Run("SetSystemSecret_New", func(t *testing.T) {
		err := store.SetSystemSecret("test_key", "test_value")
		if err != nil {
			t.Errorf("SetSystemSecret() error = %v", err)
		}

		value, err := store.GetSystemSecret("test_key")
		if err != nil {
			t.Errorf("GetSystemSecret() error = %v", err)
		}
		if value != "test_value" {
			t.Errorf("GetSystemSecret() = %q, want %q", value, "test_value")
		}
	})

	t.Run("SetSystemSecret_Update", func(t *testing.T) {
		// First set
		err := store.SetSystemSecret("update_key", "original_value")
		if err != nil {
			t.Fatalf("SetSystemSecret() error = %v", err)
		}

		// Update
		err = store.SetSystemSecret("update_key", "updated_value")
		if err != nil {
			t.Errorf("SetSystemSecret() update error = %v", err)
		}

		value, err := store.GetSystemSecret("update_key")
		if err != nil {
			t.Errorf("GetSystemSecret() error = %v", err)
		}
		if value != "updated_value" {
			t.Errorf("GetSystemSecret() = %q, want %q", value, "updated_value")
		}
	})

	t.Run("DeleteSystemSecret", func(t *testing.T) {
		// Set a secret
		err := store.SetSystemSecret("delete_key", "to_be_deleted")
		if err != nil {
			t.Fatalf("SetSystemSecret() error = %v", err)
		}

		// Verify it exists
		value, _ := store.GetSystemSecret("delete_key")
		if value != "to_be_deleted" {
			t.Fatalf("Secret was not set correctly")
		}

		// Delete it
		err = store.DeleteSystemSecret("delete_key")
		if err != nil {
			t.Errorf("DeleteSystemSecret() error = %v", err)
		}

		// Verify it's gone
		value, err = store.GetSystemSecret("delete_key")
		if err != nil {
			t.Errorf("GetSystemSecret() after delete error = %v", err)
		}
		if value != "" {
			t.Errorf("GetSystemSecret() after delete = %q, want empty string", value)
		}
	})

	t.Run("DeleteSystemSecret_NotExists", func(t *testing.T) {
		// Should not error when deleting non-existent key
		err := store.DeleteSystemSecret("never_existed")
		if err != nil {
			t.Errorf("DeleteSystemSecret() on non-existent key error = %v", err)
		}
	})

	t.Run("SetSystemSecret_EmptyValue", func(t *testing.T) {
		// Setting empty value should work
		err := store.SetSystemSecret("empty_key", "")
		if err != nil {
			t.Errorf("SetSystemSecret() with empty value error = %v", err)
		}

		value, err := store.GetSystemSecret("empty_key")
		if err != nil {
			t.Errorf("GetSystemSecret() error = %v", err)
		}
		if value != "" {
			t.Errorf("GetSystemSecret() = %q, want empty string", value)
		}
	})
}

func TestSystemSecrets_Persistence(t *testing.T) {
	// Create temp database
	tmpFile, err := os.CreateTemp("", "nexus-test-persist-*.db")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	dbPath := tmpFile.Name()
	defer os.Remove(dbPath)
	tmpFile.Close()

	// Create store and set a secret
	store1, err := NewStore(Config{DatabasePath: dbPath})
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	err = store1.SetSystemSecret("persist_key", "persist_value")
	if err != nil {
		t.Fatalf("SetSystemSecret() error = %v", err)
	}
	store1.Close()

	// Reopen store and verify secret persisted
	store2, err := NewStore(Config{DatabasePath: dbPath})
	if err != nil {
		t.Fatalf("Failed to reopen store: %v", err)
	}
	defer store2.Close()

	value, err := store2.GetSystemSecret("persist_key")
	if err != nil {
		t.Errorf("GetSystemSecret() after reopen error = %v", err)
	}
	if value != "persist_value" {
		t.Errorf("GetSystemSecret() after reopen = %q, want %q", value, "persist_value")
	}
}
