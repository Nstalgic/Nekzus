package backup

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nstalgic/nekzus/internal/storage"
	"github.com/nstalgic/nekzus/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test 1: Create backup manager
func TestNewManager(t *testing.T) {
	tests := []struct {
		name            string
		version         string
		expectedVersion string
	}{
		{
			name:            "with explicit version",
			version:         "1.0.0-test",
			expectedVersion: "1.0.0-test",
		},
		{
			name:            "with empty version defaults to dev",
			version:         "",
			expectedVersion: "dev",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			store, cleanup := setupTestStore(t)
			defer cleanup()

			backupDir := t.TempDir()

			// Act
			manager := NewManager(store, backupDir, tt.version)

			// Assert
			assert.NotNil(t, manager)
			assert.Equal(t, backupDir, manager.backupDir)
			assert.Equal(t, store, manager.storage)
			assert.Equal(t, tt.expectedVersion, manager.version)
		})
	}
}

// Test 2: Create backup - empty database
func TestCreateBackup_EmptyDatabase(t *testing.T) {
	// Arrange
	store, cleanup := setupTestStore(t)
	defer cleanup()

	backupDir := t.TempDir()
	manager := NewManager(store, backupDir, "1.0.0-test")

	// Act
	backup, err := manager.CreateBackup("test-backup")

	// Assert
	require.NoError(t, err)
	assert.NotNil(t, backup)
	assert.Equal(t, "test-backup", backup.Description)
	assert.NotEmpty(t, backup.ID)
	assert.NotZero(t, backup.Timestamp)
	assert.Empty(t, backup.Routes)
	assert.Empty(t, backup.Apps)
	assert.Empty(t, backup.Devices)
}

// Test 3: Create backup - with data
func TestCreateBackup_WithData(t *testing.T) {
	// Arrange
	store, cleanup := setupTestStore(t)
	defer cleanup()

	// Add test data
	app := types.App{
		ID:   "grafana",
		Name: "Grafana",
		Icon: "📊",
		Tags: []string{"monitoring"},
	}
	require.NoError(t, store.SaveApp(app))

	route := types.Route{
		RouteID:  "route:grafana",
		AppID:    "grafana",
		PathBase: "/grafana/",
		To:       "http://grafana:3000",
		Scopes:   []string{"access:grafana"},
	}
	require.NoError(t, store.SaveRoute(route))

	require.NoError(t, store.SaveDevice("device-1", "Test Device", "ios", "17.0", []string{"routes:read"}))

	backupDir := t.TempDir()
	manager := NewManager(store, backupDir, "1.0.0-test")

	// Act
	backup, err := manager.CreateBackup("backup with data")

	// Assert
	require.NoError(t, err)
	assert.Len(t, backup.Apps, 1)
	assert.Equal(t, "grafana", backup.Apps[0].ID)
	assert.Len(t, backup.Routes, 1)
	assert.Equal(t, "route:grafana", backup.Routes[0].RouteID)
	assert.Len(t, backup.Devices, 1)
	assert.Equal(t, "device-1", backup.Devices[0].DeviceID)
}

// Test 4: Save backup to disk
func TestSaveBackup(t *testing.T) {
	// Arrange
	store, cleanup := setupTestStore(t)
	defer cleanup()

	backupDir := t.TempDir()
	manager := NewManager(store, backupDir, "1.0.0-test")

	backup, err := manager.CreateBackup("test save")
	require.NoError(t, err)

	// Act
	err = manager.SaveBackup(backup)

	// Assert
	require.NoError(t, err)

	// Verify file exists
	filename := filepath.Join(backupDir, backup.ID+".json")
	_, err = os.Stat(filename)
	assert.NoError(t, err, "Backup file should exist")

	// Verify file contents
	data, err := os.ReadFile(filename)
	require.NoError(t, err)

	var loaded Snapshot
	err = json.Unmarshal(data, &loaded)
	require.NoError(t, err)
	assert.Equal(t, backup.ID, loaded.ID)
	assert.Equal(t, backup.Description, loaded.Description)
}

// Test 5: List backups
func TestListBackups(t *testing.T) {
	// Arrange
	store, cleanup := setupTestStore(t)
	defer cleanup()

	backupDir := t.TempDir()
	manager := NewManager(store, backupDir, "1.0.0-test")

	// Create multiple backups
	backup1, _ := manager.CreateBackup("backup 1")
	manager.SaveBackup(backup1)

	time.Sleep(10 * time.Millisecond) // Ensure different timestamps

	backup2, _ := manager.CreateBackup("backup 2")
	manager.SaveBackup(backup2)

	// Act
	backups, err := manager.ListBackups()

	// Assert
	require.NoError(t, err)
	require.Len(t, backups, 2)

	// Should be sorted by timestamp descending (newest first)
	assert.Equal(t, backup2.ID, backups[0].ID)
	assert.Equal(t, backup1.ID, backups[1].ID)
}

// Test 6: List backups - empty directory
func TestListBackups_EmptyDirectory(t *testing.T) {
	// Arrange
	store, cleanup := setupTestStore(t)
	defer cleanup()

	backupDir := t.TempDir()
	manager := NewManager(store, backupDir, "1.0.0-test")

	// Act
	backups, err := manager.ListBackups()

	// Assert
	require.NoError(t, err)
	assert.Empty(t, backups)
}

// Test 7: Get backup by ID
func TestGetBackup(t *testing.T) {
	// Arrange
	store, cleanup := setupTestStore(t)
	defer cleanup()

	backupDir := t.TempDir()
	manager := NewManager(store, backupDir, "1.0.0-test")

	backup, _ := manager.CreateBackup("test backup")
	manager.SaveBackup(backup)

	// Act
	loaded, err := manager.GetBackup(backup.ID)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, backup.ID, loaded.ID)
	assert.Equal(t, backup.Description, loaded.Description)
	assert.Equal(t, backup.Timestamp.Unix(), loaded.Timestamp.Unix())
}

// Test 8: Get backup - not found
func TestGetBackup_NotFound(t *testing.T) {
	// Arrange
	store, cleanup := setupTestStore(t)
	defer cleanup()

	backupDir := t.TempDir()
	manager := NewManager(store, backupDir, "1.0.0-test")

	// Act
	_, err := manager.GetBackup("nonexistent-backup")

	// Assert
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// Test 9: Delete backup
func TestDeleteBackup(t *testing.T) {
	// Arrange
	store, cleanup := setupTestStore(t)
	defer cleanup()

	backupDir := t.TempDir()
	manager := NewManager(store, backupDir, "1.0.0-test")

	backup, _ := manager.CreateBackup("test delete")
	manager.SaveBackup(backup)

	// Verify it exists
	_, err := manager.GetBackup(backup.ID)
	require.NoError(t, err)

	// Act
	err = manager.DeleteBackup(backup.ID)

	// Assert
	require.NoError(t, err)

	// Verify it's gone
	_, err = manager.GetBackup(backup.ID)
	assert.Error(t, err)

	// Verify file is deleted
	filename := filepath.Join(backupDir, backup.ID+".json")
	_, err = os.Stat(filename)
	assert.True(t, os.IsNotExist(err))
}

// Test 10: Restore backup - apps and routes
func TestRestoreBackup_AppsAndRoutes(t *testing.T) {
	// Arrange
	store, cleanup := setupTestStore(t)
	defer cleanup()

	backupDir := t.TempDir()
	manager := NewManager(store, backupDir, "1.0.0-test")

	// Create backup with data
	app := types.App{ID: "grafana", Name: "Grafana"}
	store.SaveApp(app)
	route := types.Route{RouteID: "route:grafana", AppID: "grafana", PathBase: "/grafana/", To: "http://grafana:3000"}
	store.SaveRoute(route)

	backup, _ := manager.CreateBackup("restore test")
	manager.SaveBackup(backup)

	// Clear database
	store.DeleteApp("grafana")
	store.DeleteRoute("route:grafana")

	// Act
	err := manager.RestoreBackup(backup.ID, RestoreOptions{
		RestoreApps:   true,
		RestoreRoutes: true,
	})

	// Assert
	require.NoError(t, err)

	// Verify data restored
	apps, _ := store.ListApps()
	assert.Len(t, apps, 1)
	assert.Equal(t, "grafana", apps[0].ID)

	routes, _ := store.ListRoutes()
	assert.Len(t, routes, 1)
	assert.Equal(t, "route:grafana", routes[0].RouteID)
}

// Test 11: Restore backup - selective restore (only apps)
func TestRestoreBackup_SelectiveRestore(t *testing.T) {
	// Arrange
	store, cleanup := setupTestStore(t)
	defer cleanup()

	backupDir := t.TempDir()
	manager := NewManager(store, backupDir, "1.0.0-test")

	// Create backup
	app := types.App{ID: "grafana", Name: "Grafana"}
	store.SaveApp(app)
	route := types.Route{RouteID: "route:grafana", AppID: "grafana", PathBase: "/grafana/", To: "http://grafana:3000"}
	store.SaveRoute(route)

	backup, _ := manager.CreateBackup("selective restore")
	manager.SaveBackup(backup)

	// Clear database
	store.DeleteApp("grafana")
	store.DeleteRoute("route:grafana")

	// Act - Restore only apps
	err := manager.RestoreBackup(backup.ID, RestoreOptions{
		RestoreApps:   true,
		RestoreRoutes: false,
	})

	// Assert
	require.NoError(t, err)

	// Apps should be restored
	apps, _ := store.ListApps()
	assert.Len(t, apps, 1)

	// Routes should NOT be restored
	routes, _ := store.ListRoutes()
	assert.Empty(t, routes)
}

// Test 12: Restore backup - devices
func TestRestoreBackup_Devices(t *testing.T) {
	// Arrange
	store, cleanup := setupTestStore(t)
	defer cleanup()

	backupDir := t.TempDir()
	manager := NewManager(store, backupDir, "1.0.0-test")

	// Create backup with device
	store.SaveDevice("device-1", "Test Device", "ios", "17.0", []string{"routes:read"})

	backup, _ := manager.CreateBackup("device restore")
	manager.SaveBackup(backup)

	// Clear database
	store.DeleteDevice("device-1")

	// Act
	err := manager.RestoreBackup(backup.ID, RestoreOptions{
		RestoreDevices: true,
	})

	// Assert
	require.NoError(t, err)

	// Verify device restored
	devices, _ := store.ListDevices()
	assert.Len(t, devices, 1)
	assert.Equal(t, "device-1", devices[0].ID)
}

// Test 13: Backup includes certificates
func TestCreateBackup_IncludesCertificates(t *testing.T) {
	// Arrange
	store, cleanup := setupTestStore(t)
	defer cleanup()

	backupDir := t.TempDir()
	manager := NewManager(store, backupDir, "1.0.0-test")

	// Add a certificate
	cert := &storage.StoredCertificate{
		Domain:                  "test.local",
		CertificatePEM:          []byte("-----BEGIN CERTIFICATE-----\ntest\n-----END CERTIFICATE-----"),
		PrivateKeyPEM:           []byte("-----BEGIN PRIVATE KEY-----\ntest\n-----END PRIVATE KEY-----"),
		Issuer:                  "self-signed",
		NotBefore:               time.Now(),
		NotAfter:                time.Now().Add(365 * 24 * time.Hour),
		FingerprintSHA256:       "abc123",
		SubjectAlternativeNames: "test.local",
	}
	store.StoreCertificate(cert)

	// Act
	backup, err := manager.CreateBackup("cert backup")

	// Assert
	require.NoError(t, err)
	assert.Len(t, backup.Certificates, 1)
	assert.Equal(t, "test.local", backup.Certificates[0].Domain)
	assert.Equal(t, "abc123", backup.Certificates[0].FingerprintSHA256)
}

// Test 14: Restore certificates
func TestRestoreBackup_Certificates(t *testing.T) {
	// Arrange
	store, cleanup := setupTestStore(t)
	defer cleanup()

	backupDir := t.TempDir()
	manager := NewManager(store, backupDir, "1.0.0-test")

	// Create backup with certificate
	cert := &storage.StoredCertificate{
		Domain:                  "test.local",
		CertificatePEM:          []byte("-----BEGIN CERTIFICATE-----\ntest\n-----END CERTIFICATE-----"),
		PrivateKeyPEM:           []byte("-----BEGIN PRIVATE KEY-----\ntest\n-----END PRIVATE KEY-----"),
		Issuer:                  "self-signed",
		NotBefore:               time.Now(),
		NotAfter:                time.Now().Add(365 * 24 * time.Hour),
		FingerprintSHA256:       "abc123",
		SubjectAlternativeNames: "test.local",
	}
	store.StoreCertificate(cert)

	backup, _ := manager.CreateBackup("cert restore")
	manager.SaveBackup(backup)

	// Clear database
	store.DeleteCertificate("test.local")

	// Act
	err := manager.RestoreBackup(backup.ID, RestoreOptions{
		RestoreCertificates: true,
	})

	// Assert
	require.NoError(t, err)

	// Verify certificate restored
	certs, _ := store.ListCertificates()
	assert.Len(t, certs, 1)
	assert.Equal(t, "test.local", certs[0].Domain)
}

// Test 15: Cleanup old backups (retention policy)
func TestCleanupOldBackups(t *testing.T) {
	// Arrange
	store, cleanup := setupTestStore(t)
	defer cleanup()

	backupDir := t.TempDir()
	manager := NewManager(store, backupDir, "1.0.0-test")

	// Create 5 backups
	for i := 0; i < 5; i++ {
		backup, _ := manager.CreateBackup("backup " + string(rune('A'+i)))
		manager.SaveBackup(backup)
		time.Sleep(10 * time.Millisecond) // Ensure different timestamps
	}

	// Act - Keep only 3 most recent
	deleted, err := manager.CleanupOldBackups(3)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, 2, deleted, "Should delete 2 old backups")

	backups, _ := manager.ListBackups()
	assert.Len(t, backups, 3, "Should have 3 backups remaining")
}

// Helper: setup test store
func setupTestStore(t *testing.T) (*storage.Store, func()) {
	t.Helper()
	store, err := storage.NewStore(storage.Config{
		DatabasePath: "file::memory:?cache=shared",
	})
	require.NoError(t, err)
	return store, func() { store.Close() }
}
