package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/nstalgic/nekzus/internal/backup"
	apperrors "github.com/nstalgic/nekzus/internal/errors"
	"github.com/nstalgic/nekzus/internal/storage"
)

// BackupHandler handles backup-related HTTP requests
type BackupHandler struct {
	manager   *backup.Manager
	scheduler *backup.Scheduler
	storage   *storage.Store
}

// NewBackupHandler creates a new backup handler
func NewBackupHandler(manager *backup.Manager, scheduler *backup.Scheduler, store *storage.Store) *BackupHandler {
	return &BackupHandler{
		manager:   manager,
		scheduler: scheduler,
		storage:   store,
	}
}

// CreateBackup handles POST /api/v1/backups
func (h *BackupHandler) CreateBackup(w http.ResponseWriter, r *http.Request) {
	// Add HTTP method validation
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		apperrors.WriteJSON(w, apperrors.New("METHOD_NOT_ALLOWED", "Method not allowed", http.StatusMethodNotAllowed))
		return
	}

	// Parse request body
	var req struct {
		Description string `json:"description"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apperrors.WriteJSON(w, apperrors.New("INVALID_JSON", "Invalid JSON body", http.StatusBadRequest))
		return
	}

	// Validate description
	if req.Description == "" {
		req.Description = fmt.Sprintf("Manual backup at %s", backup.Now().Format("2006-01-02 15:04:05"))
	}

	// Create backup
	snapshot, err := h.manager.CreateBackup(req.Description)
	if err != nil {
		apperrors.WriteJSON(w, apperrors.Wrap(err, "BACKUP_CREATE_FAILED", "Failed to create backup", http.StatusInternalServerError))
		return
	}

	// Save to disk
	if err := h.manager.SaveBackup(snapshot); err != nil {
		apperrors.WriteJSON(w, apperrors.Wrap(err, "BACKUP_SAVE_FAILED", "Failed to save backup", http.StatusInternalServerError))
		return
	}

	// Return snapshot
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(snapshot)
}

// ListBackups handles GET /api/v1/backups
func (h *BackupHandler) ListBackups(w http.ResponseWriter, r *http.Request) {
	// Add HTTP method validation
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		apperrors.WriteJSON(w, apperrors.New("METHOD_NOT_ALLOWED", "Method not allowed", http.StatusMethodNotAllowed))
		return
	}

	// List all backups
	backups, err := h.manager.ListBackups()
	if err != nil {
		apperrors.WriteJSON(w, apperrors.Wrap(err, "BACKUP_LIST_FAILED", "Failed to list backups", http.StatusInternalServerError))
		return
	}

	// Return backups
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"backups": backups,
		"count":   len(backups),
	})
}

// GetBackup handles GET /api/v1/backups/:id
func (h *BackupHandler) GetBackup(w http.ResponseWriter, r *http.Request) {
	// Add HTTP method validation
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		apperrors.WriteJSON(w, apperrors.New("METHOD_NOT_ALLOWED", "Method not allowed", http.StatusMethodNotAllowed))
		return
	}

	// Extract backup ID from URL
	backupID := r.PathValue("id")
	if backupID == "" {
		apperrors.WriteJSON(w, apperrors.New("MISSING_BACKUP_ID", "Backup ID is required", http.StatusBadRequest))
		return
	}

	// Get backup
	snapshot, err := h.manager.GetBackup(backupID)
	if err != nil {
		apperrors.WriteJSON(w, apperrors.Wrap(err, "BACKUP_NOT_FOUND", "Backup not found", http.StatusNotFound))
		return
	}

	// Return snapshot
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(snapshot)
}

// DeleteBackup handles DELETE /api/v1/backups/:id
func (h *BackupHandler) DeleteBackup(w http.ResponseWriter, r *http.Request) {
	// Add HTTP method validation
	if r.Method != http.MethodDelete {
		w.Header().Set("Allow", http.MethodDelete)
		apperrors.WriteJSON(w, apperrors.New("METHOD_NOT_ALLOWED", "Method not allowed", http.StatusMethodNotAllowed))
		return
	}

	// Extract backup ID from URL
	backupID := r.PathValue("id")
	if backupID == "" {
		apperrors.WriteJSON(w, apperrors.New("MISSING_BACKUP_ID", "Backup ID is required", http.StatusBadRequest))
		return
	}

	// Delete backup
	if err := h.manager.DeleteBackup(backupID); err != nil {
		apperrors.WriteJSON(w, apperrors.Wrap(err, "BACKUP_DELETE_FAILED", "Failed to delete backup", http.StatusInternalServerError))
		return
	}

	// Return success
	w.WriteHeader(http.StatusNoContent)
}

// RestoreBackup handles POST /api/v1/backups/:id/restore
func (h *BackupHandler) RestoreBackup(w http.ResponseWriter, r *http.Request) {
	// Add HTTP method validation
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		apperrors.WriteJSON(w, apperrors.New("METHOD_NOT_ALLOWED", "Method not allowed", http.StatusMethodNotAllowed))
		return
	}

	// Extract backup ID from URL
	backupID := r.PathValue("id")
	if backupID == "" {
		apperrors.WriteJSON(w, apperrors.New("MISSING_BACKUP_ID", "Backup ID is required", http.StatusBadRequest))
		return
	}

	// Parse restore options
	var options backup.RestoreOptions
	if err := json.NewDecoder(r.Body).Decode(&options); err != nil {
		apperrors.WriteJSON(w, apperrors.New("INVALID_JSON", "Invalid JSON body", http.StatusBadRequest))
		return
	}

	// Restore backup
	if err := h.manager.RestoreBackup(backupID, options); err != nil {
		apperrors.WriteJSON(w, apperrors.Wrap(err, "BACKUP_RESTORE_FAILED", "Failed to restore backup", http.StatusInternalServerError))
		return
	}

	// Return success
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "Backup restored successfully",
		"options": options,
	})
}

// GetSchedulerStatus handles GET /api/v1/backups/scheduler/status
func (h *BackupHandler) GetSchedulerStatus(w http.ResponseWriter, r *http.Request) {
	// Add HTTP method validation
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		apperrors.WriteJSON(w, apperrors.New("METHOD_NOT_ALLOWED", "Method not allowed", http.StatusMethodNotAllowed))
		return
	}

	if h.scheduler == nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"enabled": false,
			"message": "Backup scheduler is not configured",
		})
		return
	}

	// Get scheduler status
	status := h.scheduler.Status()

	// Return status
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"enabled": true,
		"status":  status,
	})
}

// TriggerBackup handles POST /api/v1/backups/scheduler/trigger
func (h *BackupHandler) TriggerBackup(w http.ResponseWriter, r *http.Request) {
	// Add HTTP method validation
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		apperrors.WriteJSON(w, apperrors.New("METHOD_NOT_ALLOWED", "Method not allowed", http.StatusMethodNotAllowed))
		return
	}

	if h.scheduler == nil {
		apperrors.WriteJSON(w, apperrors.New("SCHEDULER_NOT_CONFIGURED", "Backup scheduler is not configured", http.StatusServiceUnavailable))
		return
	}

	// Parse request body
	var req struct {
		Description string `json:"description"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apperrors.WriteJSON(w, apperrors.New("INVALID_JSON", "Invalid JSON body", http.StatusBadRequest))
		return
	}

	if req.Description == "" {
		req.Description = fmt.Sprintf("Manual trigger at %s", backup.Now().Format("2006-01-02 15:04:05"))
	}

	// Trigger backup
	snapshot, err := h.scheduler.TriggerBackup(req.Description)
	if err != nil {
		apperrors.WriteJSON(w, apperrors.Wrap(err, "BACKUP_TRIGGER_FAILED", "Failed to trigger backup", http.StatusInternalServerError))
		return
	}

	// Return snapshot
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(snapshot)
}
