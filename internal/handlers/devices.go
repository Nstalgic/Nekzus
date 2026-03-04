package handlers

import (
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	apperrors "github.com/nstalgic/nekzus/internal/errors"
	"github.com/nstalgic/nekzus/internal/httputil"
	"github.com/nstalgic/nekzus/internal/storage"
	"github.com/nstalgic/nekzus/internal/types"
)

var devicelog = slog.With("package", "handlers")

// Body size limits
const (
	// MaxDeviceRequestBodySize is the maximum size for device API request bodies (10KB)
	MaxDeviceRequestBodySize = 10 * 1024

	// MaxDeviceIDLength is the maximum length for a device ID
	MaxDeviceIDLength = 100
)

// WebSocketManager interface for disconnecting devices and publishing events
type WebSocketManager interface {
	DisconnectDevice(deviceID string) int
	PublishDeviceRevoked(deviceID string)
}

// DeviceHandler handles device management endpoints
type DeviceHandler struct {
	store     *storage.Store
	wsManager WebSocketManager
	activity  ActivityTracker
}

// UpdateDeviceMetadataRequest represents a device metadata update request
type UpdateDeviceMetadataRequest struct {
	DeviceName string `json:"deviceName"`
}

// Validate validates the update request
func (r *UpdateDeviceMetadataRequest) Validate() error {
	if r.DeviceName == "" {
		return apperrors.New("VALIDATION_ERROR", "deviceName cannot be empty", http.StatusBadRequest)
	}

	if len(r.DeviceName) > maxDeviceModelLength {
		return apperrors.New("VALIDATION_ERROR", "deviceName too long (max 255 characters)", http.StatusBadRequest)
	}

	return nil
}

// NewDeviceHandler creates a new device handler
func NewDeviceHandler(store *storage.Store) *DeviceHandler {
	return &DeviceHandler{
		store:     store,
		wsManager: nil,
		activity:  nil,
	}
}

// SetWebSocketManager sets the WebSocket manager for device event publishing
func (h *DeviceHandler) SetWebSocketManager(wsManager WebSocketManager) {
	h.wsManager = wsManager
}

// SetActivityTracker sets the activity tracker for feed updates
func (h *DeviceHandler) SetActivityTracker(activity ActivityTracker) {
	h.activity = activity
}

// HandleListDevices returns all paired devices with optional pagination
// GET /api/v1/devices?limit=10&offset=0
// If no pagination params provided, returns array for backward compatibility
func (h *DeviceHandler) HandleListDevices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		apperrors.WriteJSON(w, apperrors.New("METHOD_NOT_ALLOWED", "Method not allowed", http.StatusMethodNotAllowed))
		return
	}

	// Check if pagination params are provided
	limitParam := r.URL.Query().Get("limit")
	offsetParam := r.URL.Query().Get("offset")
	usePagination := limitParam != "" || offsetParam != ""

	// Get all devices
	allDevices, err := h.store.ListDevices()
	if err != nil {
		devicelog.Error("Error listing devices", "error", err)
		apperrors.WriteJSON(w, apperrors.Wrap(err, "DEVICE_LIST_FAILED", "Failed to list devices", http.StatusInternalServerError))
		return
	}

	// If no pagination requested, return array for backward compatibility
	if !usePagination {
		if err := httputil.WriteJSON(w, http.StatusOK, allDevices); err != nil {
			devicelog.Error("Error encoding JSON response", "error", err)
		}
		return
	}

	// Parse pagination parameters
	limit := parsePaginationParam(limitParam, -1)  // -1 = no limit
	offset := parsePaginationParam(offsetParam, 0) // 0 = start from beginning

	total := len(allDevices)

	// Apply pagination
	start := offset
	if start < 0 {
		start = 0
	}
	if start > total {
		start = total
	}

	end := total
	if limit > 0 {
		end = start + limit
		if end > total {
			end = total
		}
	} else if limit == 0 {
		// limit=0 means return empty list
		end = start
	}

	devices := allDevices[start:end]

	// Return paginated response
	response := map[string]interface{}{
		"devices": devices,
		"total":   total,
		"limit":   limit,
		"offset":  offset,
	}

	if err := httputil.WriteJSON(w, http.StatusOK, response); err != nil {
		devicelog.Error("Error encoding JSON response", "error", err)
	}
}

// parsePaginationParam parses pagination query parameter with default value
func parsePaginationParam(value string, defaultValue int) int {
	if value == "" {
		return defaultValue
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 {
		return defaultValue
	}
	return parsed
}

// HandleGetDevice returns details for a specific device
// GET /api/v1/devices/{deviceId}
func (h *DeviceHandler) HandleGetDevice(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		apperrors.WriteJSON(w, apperrors.New("METHOD_NOT_ALLOWED", "Method not allowed", http.StatusMethodNotAllowed))
		return
	}

	// Extract device ID from path
	deviceID := extractDeviceIDFromPath(r.URL.Path)
	if deviceID == "" {
		apperrors.WriteJSON(w, apperrors.New("INVALID_DEVICE_ID", "Device ID required", http.StatusBadRequest))
		return
	}

	device, err := h.store.GetDevice(deviceID)
	if err != nil {
		devicelog.Error("Error getting device", "device_id", deviceID, "error", err)
		apperrors.WriteJSON(w, apperrors.Wrap(err, "DEVICE_FETCH_FAILED", "Failed to get device", http.StatusInternalServerError))
		return
	}

	if device == nil {
		apperrors.WriteJSON(w, apperrors.New("DEVICE_NOT_FOUND", "Device not found", http.StatusNotFound))
		return
	}

	if err := httputil.WriteJSON(w, http.StatusOK, device); err != nil {
		devicelog.Error("Error encoding JSON response", "error", err)
	}
}

// HandleRevokeDevice revokes access for a device
// DELETE /api/v1/devices/{deviceId}
func (h *DeviceHandler) HandleRevokeDevice(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		apperrors.WriteJSON(w, apperrors.New("METHOD_NOT_ALLOWED", "Method not allowed", http.StatusMethodNotAllowed))
		return
	}

	// Extract device ID from path
	deviceID := extractDeviceIDFromPath(r.URL.Path)
	if deviceID == "" {
		apperrors.WriteJSON(w, apperrors.New("INVALID_DEVICE_ID", "Device ID required", http.StatusBadRequest))
		return
	}

	// Check if device exists
	device, err := h.store.GetDevice(deviceID)
	if err != nil {
		devicelog.Error("Error getting device", "device_id", deviceID, "error", err)
		apperrors.WriteJSON(w, apperrors.Wrap(err, "DEVICE_FETCH_FAILED", "Failed to get device", http.StatusInternalServerError))
		return
	}

	if device == nil {
		apperrors.WriteJSON(w, apperrors.New("DEVICE_NOT_FOUND", "Device not found", http.StatusNotFound))
		return
	}

	// Revoke device
	if err := h.store.DeleteDevice(deviceID); err != nil {
		devicelog.Error("Error revoking device", "device_id", deviceID, "error", err)
		apperrors.WriteJSON(w, apperrors.Wrap(err, "DEVICE_REVOKE_FAILED", "Failed to revoke device", http.StatusInternalServerError))
		return
	}

	// Clean up pending notifications for revoked device
	deletedNotifs, err := h.store.DeleteNotificationsForDevice(deviceID)
	if err != nil {
		devicelog.Warn("Failed to clean up notifications for revoked device", "device_id", deviceID, "error", err)
	} else if deletedNotifs > 0 {
		devicelog.Info("Cleaned up pending notifications for revoked device", "device_id", deviceID, "count", deletedNotifs)
	}

	devicelog.Info("Revoked device", "device_id", deviceID, "device_name", device.Name)

	// Disconnect WebSocket connections for this device
	if h.wsManager != nil {
		disconnected := h.wsManager.DisconnectDevice(deviceID)
		devicelog.Info("Disconnected WebSocket connections for revoked device", "device_id", deviceID, "connection_count", disconnected)

		// Publish event for device.revoked to other clients
		h.wsManager.PublishDeviceRevoked(deviceID)
	}

	// Add to activity feed
	if h.activity != nil {
		if err := h.activity.Add(types.ActivityEvent{
			ID:        "device_revoked_" + deviceID,
			Type:      "device_revoked",
			Icon:      "XCircle",
			IconClass: "",
			Message:   "Device revoked: " + deviceID,
			Timestamp: time.Now().UnixMilli(),
		}); err != nil {
			devicelog.Warn("Failed to add activity event", "error", err)
		}
	}

	if err := httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"status":    "revoked",
		"deviceId":  deviceID,
		"message":   "Device access has been revoked. Existing tokens are now invalid.",
		"revokedAt": "now", // Could add actual timestamp
	}); err != nil {
		devicelog.Error("Error encoding JSON response", "error", err)
	}
}

// HandleUpdateDeviceMetadata updates device metadata (name, etc.)
// PATCH /api/v1/devices/{deviceId}
func (h *DeviceHandler) HandleUpdateDeviceMetadata(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		apperrors.WriteJSON(w, apperrors.New("METHOD_NOT_ALLOWED", "Method not allowed", http.StatusMethodNotAllowed))
		return
	}

	// Extract device ID from path
	deviceID := extractDeviceIDFromPath(r.URL.Path)
	if deviceID == "" {
		apperrors.WriteJSON(w, apperrors.New("INVALID_DEVICE_ID", "Device ID required", http.StatusBadRequest))
		return
	}

	// Get existing device
	device, err := h.store.GetDevice(deviceID)
	if err != nil {
		devicelog.Error("Error getting device", "device_id", deviceID, "error", err)
		apperrors.WriteJSON(w, apperrors.Wrap(err, "DEVICE_FETCH_FAILED", "Failed to get device", http.StatusInternalServerError))
		return
	}

	if device == nil {
		apperrors.WriteJSON(w, apperrors.New("DEVICE_NOT_FOUND", "Device not found", http.StatusNotFound))
		return
	}

	// Decode and validate request
	req, err := httputil.DecodeAndValidate[UpdateDeviceMetadataRequest](r, w, MaxDeviceRequestBodySize)
	if err != nil {
		apperrors.WriteJSON(w, err)
		return
	}

	// Update device (preserve platform and platformVersion)
	if err := h.store.SaveDevice(deviceID, req.DeviceName, device.Platform, device.PlatformVersion, device.Scopes); err != nil {
		devicelog.Error("Error updating device", "device_id", deviceID, "error", err)
		apperrors.WriteJSON(w, apperrors.Wrap(err, "DEVICE_UPDATE_FAILED", "Failed to update device", http.StatusInternalServerError))
		return
	}

	devicelog.Info("Updated device", "device_id", deviceID, "new_name", req.DeviceName)

	// Return updated device
	updatedDevice, _ := h.store.GetDevice(deviceID)
	if err := httputil.WriteJSON(w, http.StatusOK, updatedDevice); err != nil {
		devicelog.Error("Error encoding JSON response", "error", err)
	}
}

// UpdatePinsRequest represents a request to update device SPKI pins
type UpdatePinsRequest struct {
	NewPin string `json:"newPin"`
}

// Validate validates the update pins request
func (r *UpdatePinsRequest) Validate() error {
	if r.NewPin == "" {
		return apperrors.New("VALIDATION_ERROR", "newPin is required", http.StatusBadRequest)
	}

	// Pin must start with sha256/
	if !strings.HasPrefix(r.NewPin, "sha256/") {
		return apperrors.New("VALIDATION_ERROR", "newPin must start with 'sha256/'", http.StatusBadRequest)
	}

	// Reasonable length check (sha256 base64 is ~44 chars + prefix)
	if len(r.NewPin) > 100 {
		return apperrors.New("VALIDATION_ERROR", "newPin too long", http.StatusBadRequest)
	}

	return nil
}

// HandleUpdatePins updates SPKI pins for a device
// POST /api/v1/devices/{deviceId}/pins
// This allows certificate rotation without requiring re-pairing.
func (h *DeviceHandler) HandleUpdatePins(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		apperrors.WriteJSON(w, apperrors.New("METHOD_NOT_ALLOWED", "Method not allowed", http.StatusMethodNotAllowed))
		return
	}

	// Extract device ID from path (handles /api/v1/devices/{deviceId}/pins)
	deviceID := extractDeviceIDFromPinsPath(r.URL.Path)
	if deviceID == "" {
		apperrors.WriteJSON(w, apperrors.New("INVALID_DEVICE_ID", "Device ID required", http.StatusBadRequest))
		return
	}

	// Check if device exists
	device, err := h.store.GetDevice(deviceID)
	if err != nil {
		devicelog.Error("Error getting device", "device_id", deviceID, "error", err)
		apperrors.WriteJSON(w, apperrors.Wrap(err, "DEVICE_FETCH_FAILED", "Failed to get device", http.StatusInternalServerError))
		return
	}

	if device == nil {
		apperrors.WriteJSON(w, apperrors.New("DEVICE_NOT_FOUND", "Device not found", http.StatusNotFound))
		return
	}

	// Decode and validate request
	req, err := httputil.DecodeAndValidate[UpdatePinsRequest](r, w, MaxDeviceRequestBodySize)
	if err != nil {
		apperrors.WriteJSON(w, err)
		return
	}

	// Store the new pin for the device
	// For now, we return success with the active pins
	// In a full implementation, this would persist pins to storage
	activePins := []string{req.NewPin}

	devicelog.Info("Updated SPKI pins for device", "device_id", deviceID, "new_pin", req.NewPin[:20]+"...")

	if err := httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"success":    true,
		"activePins": activePins,
	}); err != nil {
		devicelog.Error("Error encoding JSON response", "error", err)
	}
}

// extractDeviceIDFromPinsPath extracts device ID from /api/v1/devices/{deviceId}/pins path
func extractDeviceIDFromPinsPath(path string) string {
	// Remove trailing slash if present
	path = strings.TrimSuffix(path, "/")

	// Check for /pins suffix
	if !strings.HasSuffix(path, "/pins") {
		return ""
	}

	// Remove /pins suffix
	path = strings.TrimSuffix(path, "/pins")

	// Now extract device ID using existing function
	return extractDeviceIDFromPath(path)
}

// extractDeviceIDFromPath extracts device ID from URL path
// Supports both standard and admin paths:
//   - /api/v1/devices/{deviceId}
//   - /api/v1/admin/devices/{deviceId}
//
// Improved validation with path traversal protection
func extractDeviceIDFromPath(path string) string {
	// Remove trailing slash if present
	path = strings.TrimSuffix(path, "/")

	// Try to extract device ID from supported path prefixes
	var id string
	switch {
	case strings.HasPrefix(path, "/api/v1/admin/devices/"):
		id = strings.TrimPrefix(path, "/api/v1/admin/devices/")
	case strings.HasPrefix(path, "/api/v1/devices/"):
		id = strings.TrimPrefix(path, "/api/v1/devices/")
	default:
		return ""
	}

	// Check for path traversal attempts
	if strings.Contains(id, "/") || strings.Contains(id, "\\") || strings.Contains(id, "..") {
		return ""
	}

	// Validate ID is not empty and not too long
	if id == "" || len(id) > MaxDeviceIDLength {
		return ""
	}

	return id
}
