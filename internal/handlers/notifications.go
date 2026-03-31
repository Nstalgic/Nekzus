package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	apperrors "github.com/nstalgic/nekzus/internal/errors"
	"github.com/nstalgic/nekzus/internal/httputil"
	"github.com/nstalgic/nekzus/internal/notifications"
	"github.com/nstalgic/nekzus/internal/storage"
)

var notifLog = slog.With("package", "handlers", "handler", "notifications")

// NotificationQueue defines the interface for notification queue operations
type NotificationQueue interface {
	RetryNotification(id int64) (bool, error)
}

// NotificationHandler handles notification queue API endpoints
type NotificationHandler struct {
	store *storage.Store
	queue NotificationQueue
}

// NewNotificationHandler creates a new notification handler
func NewNotificationHandler(store *storage.Store) *NotificationHandler {
	return &NotificationHandler{
		store: store,
	}
}

// SetQueue sets the notification queue for delivery operations
func (h *NotificationHandler) SetQueue(queue NotificationQueue) {
	h.queue = queue
}

// HandleListNotifications returns paginated notifications with filters
// GET /api/v1/notifications?status=pending&device_id=xxx&type=xxx&limit=50&offset=0
func (h *NotificationHandler) HandleListNotifications(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		apperrors.WriteJSON(w, apperrors.New("METHOD_NOT_ALLOWED", "Method not allowed", http.StatusMethodNotAllowed))
		return
	}

	query := r.URL.Query()

	filter := storage.NotificationListFilter{
		Status:   query.Get("status"),
		DeviceID: query.Get("device_id"),
		Type:     query.Get("type"),
	}

	if limitStr := query.Get("limit"); limitStr != "" {
		if limit, err := strconv.Atoi(limitStr); err == nil {
			filter.Limit = limit
		}
	}

	if offsetStr := query.Get("offset"); offsetStr != "" {
		if offset, err := strconv.Atoi(offsetStr); err == nil {
			filter.Offset = offset
		}
	}

	result, err := h.store.ListNotifications(filter, notifications.StaleNotificationThreshold)
	if err != nil {
		notifLog.Error("Failed to list notifications", "error", err)
		apperrors.WriteJSON(w, apperrors.Wrap(err, "LIST_FAILED", "Failed to list notifications", http.StatusInternalServerError))
		return
	}

	if err := httputil.WriteJSON(w, http.StatusOK, result); err != nil {
		notifLog.Error("Failed to write response", "error", err)
	}
}

// HandleGetNotificationStats returns notification queue statistics
// GET /api/v1/notifications/stats
func (h *NotificationHandler) HandleGetNotificationStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		apperrors.WriteJSON(w, apperrors.New("METHOD_NOT_ALLOWED", "Method not allowed", http.StatusMethodNotAllowed))
		return
	}

	stats, err := h.store.GetNotificationQueueStats(notifications.StaleNotificationThreshold)
	if err != nil {
		notifLog.Error("Failed to get notification stats", "error", err)
		apperrors.WriteJSON(w, apperrors.Wrap(err, "STATS_FAILED", "Failed to get notification stats", http.StatusInternalServerError))
		return
	}

	// Add enabled flag to indicate if notification delivery is active
	response := map[string]interface{}{
		"totalPending":   stats.TotalPending,
		"totalDelivered": stats.TotalDelivered,
		"totalFailed":    stats.TotalFailed,
		"staleCount":     stats.StaleCount,
		"enabled":        h.queue != nil,
	}

	if err := httputil.WriteJSON(w, http.StatusOK, response); err != nil {
		notifLog.Error("Failed to write response", "error", err)
	}
}

// HandleGetStaleNotifications returns stale notification summaries grouped by device
// GET /api/v1/notifications/stale
func (h *NotificationHandler) HandleGetStaleNotifications(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		apperrors.WriteJSON(w, apperrors.New("METHOD_NOT_ALLOWED", "Method not allowed", http.StatusMethodNotAllowed))
		return
	}

	stale, err := h.store.GetStaleNotifications(notifications.StaleNotificationThreshold)
	if err != nil {
		notifLog.Error("Failed to get stale notifications", "error", err)
		apperrors.WriteJSON(w, apperrors.Wrap(err, "STALE_FAILED", "Failed to get stale notifications", http.StatusInternalServerError))
		return
	}

	if err := httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"staleThresholdHours": int(notifications.StaleNotificationThreshold.Hours()),
		"devices":             stale,
	}); err != nil {
		notifLog.Error("Failed to write response", "error", err)
	}
}

// HandleDismissNotification dismisses a single notification
// DELETE /api/v1/notifications/{id}
func (h *NotificationHandler) HandleDismissNotification(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		apperrors.WriteJSON(w, apperrors.New("METHOD_NOT_ALLOWED", "Method not allowed", http.StatusMethodNotAllowed))
		return
	}

	// Extract ID from path: /api/v1/notifications/123
	path := r.URL.Path
	idStr := strings.TrimPrefix(path, "/api/v1/notifications/")
	idStr = strings.TrimSuffix(idStr, "/")

	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		apperrors.WriteJSON(w, apperrors.New("INVALID_ID", "Invalid notification ID", http.StatusBadRequest))
		return
	}

	if err := h.store.DismissNotification(id); err != nil {
		if strings.Contains(err.Error(), "not found") {
			apperrors.WriteJSON(w, apperrors.New("NOT_FOUND", "Notification not found or already processed", http.StatusNotFound))
			return
		}
		notifLog.Error("Failed to dismiss notification", "id", id, "error", err)
		apperrors.WriteJSON(w, apperrors.Wrap(err, "DISMISS_FAILED", "Failed to dismiss notification", http.StatusInternalServerError))
		return
	}

	notifLog.Info("Dismissed notification", "id", id)

	if err := httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"status":  "dismissed",
		"id":      id,
		"message": "Notification dismissed",
	}); err != nil {
		notifLog.Error("Failed to write response", "error", err)
	}
}

// HandleDismissDeviceNotifications dismisses all pending notifications for a device
// DELETE /api/v1/notifications/device/{deviceId}
func (h *NotificationHandler) HandleDismissDeviceNotifications(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		apperrors.WriteJSON(w, apperrors.New("METHOD_NOT_ALLOWED", "Method not allowed", http.StatusMethodNotAllowed))
		return
	}

	// Extract device ID from path: /api/v1/notifications/device/xxx
	path := r.URL.Path
	deviceID := strings.TrimPrefix(path, "/api/v1/notifications/device/")
	deviceID = strings.TrimSuffix(deviceID, "/")

	if deviceID == "" {
		apperrors.WriteJSON(w, apperrors.New("INVALID_DEVICE_ID", "Device ID required", http.StatusBadRequest))
		return
	}

	count, err := h.store.DismissNotificationsForDevice(deviceID)
	if err != nil {
		notifLog.Error("Failed to dismiss device notifications", "device_id", deviceID, "error", err)
		apperrors.WriteJSON(w, apperrors.Wrap(err, "DISMISS_FAILED", "Failed to dismiss notifications", http.StatusInternalServerError))
		return
	}

	notifLog.Info("Dismissed device notifications", "device_id", deviceID, "count", count)

	if err := httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"status":   "dismissed",
		"deviceId": deviceID,
		"count":    count,
		"message":  "Notifications dismissed for device",
	}); err != nil {
		notifLog.Error("Failed to write response", "error", err)
	}
}

// HandleRetryNotification triggers a retry for a failed/pending notification
// POST /api/v1/notifications/{id}/retry
func (h *NotificationHandler) HandleRetryNotification(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		apperrors.WriteJSON(w, apperrors.New("METHOD_NOT_ALLOWED", "Method not allowed", http.StatusMethodNotAllowed))
		return
	}

	// Extract ID from path: /api/v1/notifications/123/retry
	path := r.URL.Path
	path = strings.TrimPrefix(path, "/api/v1/notifications/")
	path = strings.TrimSuffix(path, "/retry")

	id, err := strconv.ParseInt(path, 10, 64)
	if err != nil {
		apperrors.WriteJSON(w, apperrors.New("INVALID_ID", "Invalid notification ID", http.StatusBadRequest))
		return
	}

	// If queue is available, attempt immediate delivery
	if h.queue != nil {
		notifLog.Info("Attempting retry via queue", "id", id)
		delivered, deliveryErr := h.queue.RetryNotification(id)
		notifLog.Info("Queue retry result", "id", id, "delivered", delivered, "error", deliveryErr)
		if deliveryErr != nil {
			if strings.Contains(deliveryErr.Error(), "not found") {
				apperrors.WriteJSON(w, apperrors.New("NOT_FOUND", "Notification not found", http.StatusNotFound))
				return
			}
			// Handle device not connected - return offline status (not an error)
			if strings.Contains(deliveryErr.Error(), "device not connected") {
				notifLog.Info("Device offline for retry", "id", id)
				if err := httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{
					"status":  "offline",
					"id":      id,
					"message": "Device not connected",
				}); err != nil {
					notifLog.Error("Failed to write response", "error", err)
				}
				return
			}
			// Handle expired notifications
			if strings.Contains(deliveryErr.Error(), "notification expired") {
				notifLog.Info("Notification expired, cannot retry", "id", id)
				apperrors.WriteJSON(w, apperrors.New("EXPIRED", "Notification has expired and cannot be retried", http.StatusGone))
				return
			}
			notifLog.Error("Failed to retry notification", "id", id, "error", deliveryErr)
			apperrors.WriteJSON(w, apperrors.Wrap(deliveryErr, "RETRY_FAILED", "Failed to retry notification", http.StatusInternalServerError))
			return
		}

		status := "queued"
		message := "Notification queued for retry"
		if delivered {
			status = "delivered"
			message = "Notification delivered successfully"
		}

		notifLog.Info("Retry notification", "id", id, "status", status)

		if err := httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{
			"status":  status,
			"id":      id,
			"message": message,
		}); err != nil {
			notifLog.Error("Failed to write response", "error", err)
		}
		return
	}

	// Fallback: Reset the notification status to pending for background retry
	query := `UPDATE notifications SET status = 'pending', retry_count = 0, error_message = NULL WHERE id = ?`
	result, err := h.store.DB().Exec(query, id)
	if err != nil {
		notifLog.Error("Failed to reset notification for retry", "id", id, "error", err)
		apperrors.WriteJSON(w, apperrors.Wrap(err, "RETRY_FAILED", "Failed to retry notification", http.StatusInternalServerError))
		return
	}

	affected, _ := result.RowsAffected()
	if affected == 0 {
		apperrors.WriteJSON(w, apperrors.New("NOT_FOUND", "Notification not found", http.StatusNotFound))
		return
	}

	notifLog.Info("Queued notification for retry", "id", id)

	if err := httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"status":  "queued",
		"id":      id,
		"message": "Notification queued for retry",
	}); err != nil {
		notifLog.Error("Failed to write response", "error", err)
	}
}

// NotificationRetryRequest for bulk retry
type NotificationRetryRequest struct {
	IDs []int64 `json:"ids"`
}

// HandleBulkRetryNotifications retries multiple notifications
// POST /api/v1/notifications/retry
func (h *NotificationHandler) HandleBulkRetryNotifications(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		apperrors.WriteJSON(w, apperrors.New("METHOD_NOT_ALLOWED", "Method not allowed", http.StatusMethodNotAllowed))
		return
	}

	var req NotificationRetryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apperrors.WriteJSON(w, apperrors.New("INVALID_REQUEST", "Invalid request body", http.StatusBadRequest))
		return
	}

	if len(req.IDs) == 0 {
		apperrors.WriteJSON(w, apperrors.New("INVALID_REQUEST", "No notification IDs provided", http.StatusBadRequest))
		return
	}

	// Build query with placeholders
	placeholders := make([]string, len(req.IDs))
	args := make([]interface{}, len(req.IDs))
	for i, id := range req.IDs {
		placeholders[i] = "?"
		args[i] = id
	}

	query := `UPDATE notifications SET status = 'pending', retry_count = 0, error_message = NULL WHERE id IN (` + strings.Join(placeholders, ",") + `)`

	result, err := h.store.DB().Exec(query, args...)
	if err != nil {
		notifLog.Error("Failed to bulk retry notifications", "error", err)
		apperrors.WriteJSON(w, apperrors.Wrap(err, "RETRY_FAILED", "Failed to retry notifications", http.StatusInternalServerError))
		return
	}

	affected, _ := result.RowsAffected()
	notifLog.Info("Bulk queued notifications for retry", "count", affected)

	if err := httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"status":  "queued",
		"count":   affected,
		"message": "Notifications queued for retry",
	}); err != nil {
		notifLog.Error("Failed to write response", "error", err)
	}
}

// HandleClearDeliveredNotifications deletes all delivered notifications
// DELETE /api/v1/notifications/delivered
func (h *NotificationHandler) HandleClearDeliveredNotifications(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		apperrors.WriteJSON(w, apperrors.New("METHOD_NOT_ALLOWED", "Method not allowed", http.StatusMethodNotAllowed))
		return
	}

	count, err := h.store.ClearDeliveredNotifications()
	if err != nil {
		notifLog.Error("Failed to clear delivered notifications", "error", err)
		apperrors.WriteJSON(w, apperrors.Wrap(err, "CLEAR_FAILED", "Failed to clear delivered notifications", http.StatusInternalServerError))
		return
	}

	notifLog.Info("Cleared delivered notifications", "count", count)

	if err := httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"status":  "cleared",
		"count":   count,
		"message": "Delivered notifications cleared",
	}); err != nil {
		notifLog.Error("Failed to write response", "error", err)
	}
}
