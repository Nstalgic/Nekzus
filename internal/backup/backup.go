package backup

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/nstalgic/nekzus/internal/storage"
	"github.com/nstalgic/nekzus/internal/types"
)

// Snapshot represents a complete backup of Nekzus state
type Snapshot struct {
	ID           string                       `json:"id"`
	Description  string                       `json:"description"`
	Timestamp    time.Time                    `json:"timestamp"`
	Version      string                       `json:"version"`      // Nekzus version
	Apps         []types.App                  `json:"apps"`         // All apps
	Routes       []types.Route                `json:"routes"`       // All routes
	Devices      []BackupDevice               `json:"devices"`      // All devices (without sensitive data)
	Certificates []*storage.StoredCertificate `json:"certificates"` // All certificates
	APIKeys      []BackupAPIKey               `json:"api_keys"`     // All API keys (hashed)
}

// BackupDevice represents a device in a backup (sensitive data removed)
type BackupDevice struct {
	DeviceID  string    `json:"device_id"`
	Name      string    `json:"name"`
	Platform  string    `json:"platform"`
	OSVersion string    `json:"os_version"`
	Scopes    []string  `json:"scopes"`
	CreatedAt time.Time `json:"created_at"`
	// Note: JWT tokens are NOT backed up (must re-pair devices after restore)
}

// BackupAPIKey represents an API key in a backup
type BackupAPIKey struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Prefix    string     `json:"prefix"`
	Scopes    []string   `json:"scopes"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	CreatedBy string     `json:"created_by,omitempty"`
	// Note: Key hash is NOT backed up (keys must be regenerated after restore)
}

// RestoreOptions configures what to restore from a backup
type RestoreOptions struct {
	RestoreApps         bool `json:"restore_apps"`
	RestoreRoutes       bool `json:"restore_routes"`
	RestoreDevices      bool `json:"restore_devices"`
	RestoreCertificates bool `json:"restore_certificates"`
	RestoreAPIKeys      bool `json:"restore_api_keys"`
	OverwriteExisting   bool `json:"overwrite_existing"` // If false, skip items that already exist
}

// Manager handles backup creation, storage, and restoration
type Manager struct {
	storage   *storage.Store
	backupDir string
	version   string
}

// NewManager creates a new backup manager
func NewManager(store *storage.Store, backupDir, version string) *Manager {
	if version == "" {
		version = "dev" // Default for development builds
	}
	return &Manager{
		storage:   store,
		backupDir: backupDir,
		version:   version,
	}
}

// CreateBackup creates a new backup snapshot
func (m *Manager) CreateBackup(description string) (*Snapshot, error) {
	snapshot := &Snapshot{
		ID:          generateBackupID(),
		Description: description,
		Timestamp:   time.Now(),
		Version:     m.version,
	}

	// Backup apps
	apps, err := m.storage.ListApps()
	if err != nil {
		return nil, fmt.Errorf("failed to list apps: %w", err)
	}
	snapshot.Apps = apps

	// Backup routes
	routes, err := m.storage.ListRoutes()
	if err != nil {
		return nil, fmt.Errorf("failed to list routes: %w", err)
	}
	snapshot.Routes = routes

	// Backup devices (without sensitive data)
	devices, err := m.storage.ListDevices()
	if err != nil {
		return nil, fmt.Errorf("failed to list devices: %w", err)
	}
	snapshot.Devices = make([]BackupDevice, len(devices))
	for i, dev := range devices {
		snapshot.Devices[i] = BackupDevice{
			DeviceID:  dev.ID,
			Name:      dev.Name,
			Platform:  dev.Platform,
			OSVersion: dev.PlatformVersion,
			Scopes:    dev.Scopes,
			CreatedAt: dev.PairedAt,
		}
	}

	// Backup certificates
	certs, err := m.storage.ListCertificates()
	if err != nil {
		return nil, fmt.Errorf("failed to list certificates: %w", err)
	}
	snapshot.Certificates = certs

	// Backup API keys (without sensitive data)
	apiKeys, err := m.storage.ListAPIKeys()
	if err != nil {
		return nil, fmt.Errorf("failed to list API keys: %w", err)
	}
	snapshot.APIKeys = make([]BackupAPIKey, len(apiKeys))
	for i, key := range apiKeys {
		snapshot.APIKeys[i] = BackupAPIKey{
			ID:        key.ID,
			Name:      key.Name,
			Prefix:    key.Prefix,
			Scopes:    key.Scopes,
			ExpiresAt: key.ExpiresAt,
			CreatedAt: key.CreatedAt,
			CreatedBy: key.CreatedBy,
		}
	}

	return snapshot, nil
}

// SaveBackup writes a backup snapshot to disk
func (m *Manager) SaveBackup(snapshot *Snapshot) error {
	// Ensure backup directory exists
	if err := os.MkdirAll(m.backupDir, 0755); err != nil {
		return fmt.Errorf("failed to create backup directory: %w", err)
	}

	// Marshal to JSON
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal backup: %w", err)
	}

	// Write to file
	filename := filepath.Join(m.backupDir, snapshot.ID+".json")
	if err := os.WriteFile(filename, data, 0600); err != nil {
		return fmt.Errorf("failed to write backup file: %w", err)
	}

	return nil
}

// ListBackups returns all backups, sorted by timestamp (newest first)
func (m *Manager) ListBackups() ([]Snapshot, error) {
	// Ensure backup directory exists
	if err := os.MkdirAll(m.backupDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create backup directory: %w", err)
	}

	// Read directory
	entries, err := os.ReadDir(m.backupDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read backup directory: %w", err)
	}

	// Load all backups
	var backups []Snapshot
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		filename := filepath.Join(m.backupDir, entry.Name())
		data, err := os.ReadFile(filename)
		if err != nil {
			continue // Skip corrupted files
		}

		var snapshot Snapshot
		if err := json.Unmarshal(data, &snapshot); err != nil {
			continue // Skip invalid JSON
		}

		backups = append(backups, snapshot)
	}

	// Sort by timestamp descending (newest first)
	sort.Slice(backups, func(i, j int) bool {
		return backups[i].Timestamp.After(backups[j].Timestamp)
	})

	return backups, nil
}

// GetBackup loads a specific backup by ID
func (m *Manager) GetBackup(id string) (*Snapshot, error) {
	filename := filepath.Join(m.backupDir, id+".json")

	data, err := os.ReadFile(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("backup not found: %s", id)
		}
		return nil, fmt.Errorf("failed to read backup: %w", err)
	}

	var snapshot Snapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return nil, fmt.Errorf("failed to unmarshal backup: %w", err)
	}

	return &snapshot, nil
}

// DeleteBackup removes a backup by ID
func (m *Manager) DeleteBackup(id string) error {
	filename := filepath.Join(m.backupDir, id+".json")

	if err := os.Remove(filename); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("backup not found: %s", id)
		}
		return fmt.Errorf("failed to delete backup: %w", err)
	}

	return nil
}

// RestoreBackup restores data from a backup
func (m *Manager) RestoreBackup(id string, options RestoreOptions) error {
	// Load backup
	snapshot, err := m.GetBackup(id)
	if err != nil {
		return err
	}

	// Restore apps
	if options.RestoreApps {
		for _, app := range snapshot.Apps {
			if err := m.storage.SaveApp(app); err != nil {
				return fmt.Errorf("failed to restore app %s: %w", app.ID, err)
			}
		}
	}

	// Restore routes
	if options.RestoreRoutes {
		for _, route := range snapshot.Routes {
			if err := m.storage.SaveRoute(route); err != nil {
				return fmt.Errorf("failed to restore route %s: %w", route.RouteID, err)
			}
		}
	}

	// Restore devices
	if options.RestoreDevices {
		for _, dev := range snapshot.Devices {
			// Note: Devices will need to re-pair to get new JWT tokens
			if err := m.storage.SaveDevice(dev.DeviceID, dev.Name, dev.Platform, dev.OSVersion, dev.Scopes); err != nil {
				return fmt.Errorf("failed to restore device %s: %w", dev.DeviceID, err)
			}
		}
	}

	// Restore certificates
	if options.RestoreCertificates {
		for _, cert := range snapshot.Certificates {
			if err := m.storage.StoreCertificate(cert); err != nil {
				return fmt.Errorf("failed to restore certificate %s: %w", cert.Domain, err)
			}
		}
	}

	// Restore API keys (without the actual keys - they need to be regenerated)
	if options.RestoreAPIKeys {
		// Note: Since we don't backup the key hash, API keys need to be manually regenerated
		// We could restore the metadata and mark them as "revoked" or "needs regeneration"
		// For now, skip API key restoration to avoid confusion
	}

	return nil
}

// CleanupOldBackups removes old backups, keeping only the N most recent
func (m *Manager) CleanupOldBackups(keepCount int) (int, error) {
	backups, err := m.ListBackups()
	if err != nil {
		return 0, err
	}

	if len(backups) <= keepCount {
		return 0, nil
	}

	// Delete old backups (backups are already sorted newest first)
	toDelete := backups[keepCount:]
	deleted := 0

	for _, backup := range toDelete {
		if err := m.DeleteBackup(backup.ID); err != nil {
			return deleted, fmt.Errorf("failed to delete backup %s: %w", backup.ID, err)
		}
		deleted++
	}

	return deleted, nil
}

// generateBackupID generates a unique backup ID
func generateBackupID() string {
	return fmt.Sprintf("backup_%d", time.Now().UnixNano())
}

// Now returns the current time (exported for use in handlers)
func Now() time.Time {
	return time.Now()
}
