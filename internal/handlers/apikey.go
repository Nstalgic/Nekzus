package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	apperrors "github.com/nstalgic/nekzus/internal/errors"
	"github.com/nstalgic/nekzus/internal/httputil"
	"github.com/nstalgic/nekzus/internal/middleware"
	"github.com/nstalgic/nekzus/internal/storage"
	"github.com/nstalgic/nekzus/internal/types"
)

var apikeylog = slog.With("package", "handlers")

// APIKeyHandler handles API key management endpoints
type APIKeyHandler struct {
	store *storage.Store
}

// NewAPIKeyHandler creates a new API key handler
func NewAPIKeyHandler(store *storage.Store) *APIKeyHandler {
	return &APIKeyHandler{
		store: store,
	}
}

// HandleCreateAPIKey creates a new API key
// POST /api/v1/apikeys
func (h *APIKeyHandler) HandleCreateAPIKey(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		apperrors.WriteJSON(w, apperrors.New("METHOD_NOT_ALLOWED", "Method not allowed", http.StatusMethodNotAllowed))
		return
	}

	// Parse request body
	var req types.APIKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apperrors.WriteJSON(w, apperrors.New("INVALID_REQUEST", "Invalid request body", http.StatusBadRequest))
		return
	}

	// Validate request
	if err := validateAPIKeyRequest(&req); err != nil {
		apperrors.WriteJSON(w, err)
		return
	}

	// Generate API key
	apiKey, plaintextKey, err := generateAPIKey(&req, "")
	if err != nil {
		apikeylog.Error("Failed to generate API key", "error", err)
		apperrors.WriteJSON(w, apperrors.New("GENERATION_ERROR", "Failed to generate API key", http.StatusInternalServerError))
		return
	}

	// Save to database
	if err := h.store.CreateAPIKey(apiKey); err != nil {
		apikeylog.Error("Failed to save API key", "error", err)
		apperrors.WriteJSON(w, apperrors.New("STORAGE_ERROR", "Failed to save API key", http.StatusInternalServerError))
		return
	}

	// Return response with plaintext key (only time it's returned)
	response := types.APIKeyResponse{
		APIKey: *apiKey,
		Key:    plaintextKey,
	}

	if err := httputil.WriteJSON(w, http.StatusCreated, response); err != nil {
		apikeylog.Error("Error encoding JSON response", "error", err)
	}
}

// HandleListAPIKeys lists all API keys
// GET /api/v1/apikeys
func (h *APIKeyHandler) HandleListAPIKeys(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		apperrors.WriteJSON(w, apperrors.New("METHOD_NOT_ALLOWED", "Method not allowed", http.StatusMethodNotAllowed))
		return
	}

	keys, err := h.store.ListAPIKeys()
	if err != nil {
		apikeylog.Error("Failed to list API keys", "error", err)
		apperrors.WriteJSON(w, apperrors.Wrap(err, "APIKEY_LIST_FAILED", "Failed to list API keys", http.StatusInternalServerError))
		return
	}

	// Strip key hashes from response
	for _, key := range keys {
		key.KeyHash = ""
	}

	if err := httputil.WriteJSON(w, http.StatusOK, keys); err != nil {
		apikeylog.Error("Error encoding JSON response", "error", err)
	}
}

// HandleGetAPIKey retrieves a specific API key
// GET /api/v1/apikeys/:id
func (h *APIKeyHandler) HandleGetAPIKey(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		apperrors.WriteJSON(w, apperrors.New("METHOD_NOT_ALLOWED", "Method not allowed", http.StatusMethodNotAllowed))
		return
	}

	// Extract API key ID from URL path
	keyID := extractIDFromPath(r.URL.Path, "/api/v1/apikeys/")
	if keyID == "" {
		apperrors.WriteJSON(w, apperrors.New("INVALID_REQUEST", "API key ID is required", http.StatusBadRequest))
		return
	}

	apiKey, err := h.store.GetAPIKey(keyID)
	if err != nil {
		apikeylog.Error("Failed to get API key", "error", err)
		apperrors.WriteJSON(w, apperrors.Wrap(err, "APIKEY_FETCH_FAILED", "Failed to get API key", http.StatusInternalServerError))
		return
	}

	if apiKey == nil {
		apperrors.WriteJSON(w, apperrors.New("NOT_FOUND", "API key not found", http.StatusNotFound))
		return
	}

	// Strip key hash from response
	apiKey.KeyHash = ""

	if err := httputil.WriteJSON(w, http.StatusOK, apiKey); err != nil {
		apikeylog.Error("Error encoding JSON response", "error", err)
	}
}

// HandleRevokeAPIKey revokes or permanently deletes an API key
// DELETE /api/v1/apikeys/:id?permanent=true
func (h *APIKeyHandler) HandleRevokeAPIKey(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		apperrors.WriteJSON(w, apperrors.New("METHOD_NOT_ALLOWED", "Method not allowed", http.StatusMethodNotAllowed))
		return
	}

	// Extract API key ID from URL path
	keyID := extractIDFromPath(r.URL.Path, "/api/v1/apikeys/")
	if keyID == "" {
		apperrors.WriteJSON(w, apperrors.New("INVALID_REQUEST", "API key ID is required", http.StatusBadRequest))
		return
	}

	// Check if permanent deletion is requested
	permanent := r.URL.Query().Get("permanent") == "true"

	var err error
	if permanent {
		// Permanently delete the key
		err = h.store.DeleteAPIKey(keyID)
	} else {
		// Soft delete (revoke)
		err = h.store.RevokeAPIKey(keyID)
	}

	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			apperrors.WriteJSON(w, apperrors.New("NOT_FOUND", "API key not found", http.StatusNotFound))
			return
		}
		apikeylog.Error("Failed to revoke/delete API key", "error", err)
		apperrors.WriteJSON(w, apperrors.Wrap(err, "APIKEY_REVOKE_FAILED", "Failed to revoke API key", http.StatusInternalServerError))
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"message":"API key revoked successfully"}`))
}

// validateAPIKeyRequest validates an API key creation request
func validateAPIKeyRequest(req *types.APIKeyRequest) error {
	if req.Name == "" {
		return apperrors.New("VALIDATION_ERROR", "name is required", http.StatusBadRequest)
	}

	if len(req.Name) > 255 {
		return apperrors.New("VALIDATION_ERROR", "name too long (max 255 characters)", http.StatusBadRequest)
	}

	if len(req.Scopes) == 0 {
		return apperrors.New("VALIDATION_ERROR", "at least one scope is required", http.StatusBadRequest)
	}

	// Validate scopes
	validScopes := map[string]bool{
		"read:catalog": true,
		"read:events":  true,
		"read:*":       true,
		"write:*":      true,
		"access:admin": true,
	}

	for _, scope := range req.Scopes {
		if !validScopes[scope] {
			return apperrors.New("VALIDATION_ERROR", "invalid scope: "+scope, http.StatusBadRequest)
		}
	}

	// Validate expiration if provided
	if req.ExpiresAt != nil && req.ExpiresAt.Before(time.Now()) {
		return apperrors.New("VALIDATION_ERROR", "expiration must be in the future", http.StatusBadRequest)
	}

	return nil
}

// generateAPIKey generates a new API key with secure random bytes
func generateAPIKey(req *types.APIKeyRequest, createdBy string) (*types.APIKey, string, error) {
	// Generate random bytes for key
	keyBytes := make([]byte, 32)
	if _, err := rand.Read(keyBytes); err != nil {
		return nil, "", err
	}

	// Encode as hex
	keyHex := hex.EncodeToString(keyBytes)

	// Format as nekzus_<hex>
	plaintextKey := "nekzus_" + keyHex

	// Hash the key for storage
	keyHash := middleware.HashAPIKey(plaintextKey)

	// Generate key ID
	idBytes := make([]byte, 16)
	rand.Read(idBytes)
	keyID := "key_" + hex.EncodeToString(idBytes)

	// Create API key object
	apiKey := &types.APIKey{
		ID:        keyID,
		Name:      req.Name,
		KeyHash:   keyHash,
		Prefix:    plaintextKey[:8], // First 8 chars for identification
		Scopes:    req.Scopes,
		CreatedAt: time.Now(),
		CreatedBy: createdBy,
		ExpiresAt: req.ExpiresAt,
	}

	return apiKey, plaintextKey, nil
}

// extractIDFromPath extracts the ID from a URL path
func extractIDFromPath(path, prefix string) string {
	if !strings.HasPrefix(path, prefix) {
		return ""
	}
	return strings.TrimPrefix(path, prefix)
}
