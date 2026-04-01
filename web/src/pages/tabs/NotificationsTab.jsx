/**
 * NotificationsTab Component
 *
 * Notification queue management interface for viewing and managing
 * notifications queued for mobile devices.
 *
 * Features:
 * - View all notifications with status filtering
 * - See statistics (pending, delivered, failed)
 * - Identify stale notifications (pending > 24h)
 * - Retry failed notifications
 * - Dismiss notifications
 */

import { useState, useEffect, useCallback } from 'react';
import { notificationsAPI } from '../../services/api';
import { Badge } from '../../components/data-display';
import { ConfirmationModal } from '../../components/modals';
import CustomDropdown from '../../components/forms/CustomDropdown';
import { useSettings } from '../../contexts/SettingsContext';
import { useNotification } from '../../contexts/NotificationContext';

// Status filter options for the dropdown
const statusFilterOptions = [
  { value: '', label: 'All' },
  { value: 'pending', label: 'Pending' },
  { value: 'delivered', label: 'Delivered' },
  { value: 'failed', label: 'Failed' },
  { value: 'dismissed', label: 'Dismissed' },
];

/**
 * Format relative time from timestamp
 */
function formatRelativeTime(timestamp) {
  const now = new Date();
  const date = new Date(timestamp);
  const diff = now - date;
  const minutes = Math.floor(diff / 60000);
  const hours = Math.floor(diff / 3600000);
  const days = Math.floor(diff / 86400000);

  if (minutes < 1) return 'just now';
  if (minutes < 60) return `${minutes}m ago`;
  if (hours < 24) return `${hours}h ago`;
  return `${days}d ago`;
}

/**
 * Get badge variant for notification status
 */
function getStatusBadge(status, isStale) {
  if (isStale && status === 'pending') {
    return { variant: 'warning', label: 'STALE' };
  }
  switch (status) {
    case 'pending':
      return { variant: 'info', label: 'PENDING' };
    case 'delivered':
      return { variant: 'success', label: 'DELIVERED' };
    case 'failed':
      return { variant: 'error', label: 'FAILED' };
    case 'dismissed':
      return { variant: 'secondary', label: 'DISMISSED' };
    default:
      return { variant: 'secondary', label: status?.toUpperCase() };
  }
}

/**
 * NotificationsTab Component
 */
export function NotificationsTab() {
  const { settings } = useSettings();
  const { addNotification } = useNotification();
  const [notifications, setNotifications] = useState([]);
  const [stats, setStats] = useState(null);
  const [staleInfo, setStaleInfo] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [statusFilter, setStatusFilter] = useState('');
  const [selectedNotification, setSelectedNotification] = useState(null);
  const [dismissModalOpen, setDismissModalOpen] = useState(false);
  const [actionInProgress, setActionInProgress] = useState(false);
  const [retryingId, setRetryingId] = useState(null);

  // Fetch all notification data
  const fetchData = useCallback(async () => {
    try {
      setLoading(true);
      setError(null);

      const [listResult, statsResult, staleResult] = await Promise.all([
        notificationsAPI.list({ status: statusFilter, limit: 100 }),
        notificationsAPI.getStats(),
        notificationsAPI.getStale(),
      ]);

      setNotifications(listResult.notifications || []);
      setStats(statsResult);
      setStaleInfo(staleResult);
    } catch (err) {
      setError(err.message || 'Failed to load notifications');
    } finally {
      setLoading(false);
    }
  }, [statusFilter]);

  // Initial load and refresh on filter change
  useEffect(() => {
    fetchData();
  }, [fetchData]);

  // Handle dismiss notification
  const handleDismiss = (notification) => {
    setSelectedNotification(notification);
    if (settings.requireConfirmation) {
      setDismissModalOpen(true);
    } else {
      performDismiss(notification.id);
    }
  };

  // Perform dismiss action
  const performDismiss = async (id) => {
    try {
      setActionInProgress(true);
      await notificationsAPI.dismiss(id);
      await fetchData();
    } catch (err) {
      setError(err.message || 'Failed to dismiss notification');
    } finally {
      setActionInProgress(false);
      setDismissModalOpen(false);
      setSelectedNotification(null);
    }
  };

  // Handle retry notification
  const handleRetry = async (notification) => {
    try {
      setActionInProgress(true);
      setRetryingId(notification.id);
      const result = await notificationsAPI.retry(notification.id);

      // Show feedback based on result status
      if (result.status === 'sent') {
        addNotification({
          severity: 'info',
          message: 'Notification sent, awaiting device confirmation',
          strongText: notification.deviceName,
        });
      } else if (result.status === 'offline') {
        addNotification({
          severity: 'warning',
          message: 'Device is offline',
          strongText: notification.deviceName,
        });
      } else if (result.status === 'queued') {
        addNotification({
          severity: 'info',
          message: 'Notification queued for retry',
          strongText: notification.deviceName,
        });
      }

      await fetchData();
    } catch (err) {
      const errorMessage = err.message || 'Failed to retry notification';
      const isNotFound = err.status === 404 || errorMessage.toLowerCase().includes('not found');

      addNotification({
        severity: 'error',
        message: isNotFound ? 'Notification no longer exists' : errorMessage,
      });

      // Auto-refresh if notification was not found (stale frontend state)
      if (isNotFound) {
        await fetchData();
      } else {
        setError(errorMessage);
      }
    } finally {
      setActionInProgress(false);
      setRetryingId(null);
    }
  };

  // Handle bulk retry all failed
  const handleRetryAllFailed = async () => {
    const failedIds = notifications
      .filter(n => n.status === 'failed')
      .map(n => n.id);

    if (failedIds.length === 0) return;

    try {
      setActionInProgress(true);
      await notificationsAPI.bulkRetry(failedIds);
      await fetchData();
    } catch (err) {
      setError(err.message || 'Failed to retry notifications');
    } finally {
      setActionInProgress(false);
    }
  };

  // Handle clear all delivered notifications
  const handleClearDelivered = async () => {
    try {
      setActionInProgress(true);
      await notificationsAPI.clearDelivered();
      await fetchData();
    } catch (err) {
      setError(err.message || 'Failed to clear delivered notifications');
    } finally {
      setActionInProgress(false);
    }
  };

  // Render loading state
  if (loading && notifications.length === 0) {
    return (
      <div className="notifications-tab">
        <div className="loading-state">Loading notifications...</div>
      </div>
    );
  }

  // Render error state
  if (error && notifications.length === 0) {
    return (
      <div className="notifications-tab">
        <div className="error-state">
          <p className="text-error">{error}</p>
          <button className="btn btn-secondary" onClick={fetchData}>
            Retry
          </button>
        </div>
      </div>
    );
  }

  // Render disabled state when notifications are not enabled
  if (stats && stats.enabled === false) {
    return (
      <div className="notifications-tab notifications-disabled">
        <div className="disabled-overlay">
          <div className="disabled-content">
            <h3>Notifications Disabled</h3>
            <p className="text-secondary">
              The notification queue is not enabled on this server.
            </p>
            <p className="text-secondary" style={{ marginTop: 'var(--space-3)' }}>
              To enable, use one of the following:
            </p>
            <ul className="text-secondary" style={{ marginTop: 'var(--space-2)', textAlign: 'left', display: 'inline-block' }}>
              <li>Config file: <code>notifications.enabled: true</code></li>
              <li>Environment: <code>NEKZUS_NOTIFICATIONS_ENABLED=true</code></li>
            </ul>
          </div>
        </div>
      </div>
    );
  }

  const failedCount = stats?.totalFailed || 0;
  const deliveredCount = stats?.totalDelivered || 0;
  const staleCount = staleInfo?.devices?.length || 0;

  return (
    <div className="notifications-tab">
      {/* Stats Overview */}
      {stats && (
        <div className="notifications-stats">
          <div className="stat-item">
            <span className="stat-label">PENDING</span>
            <span className="stat-value">{stats.totalPending}</span>
          </div>
          <div className="stat-item">
            <span className="stat-label">DELIVERED</span>
            <span className="stat-value">{stats.totalDelivered}</span>
          </div>
          <div className="stat-item">
            <span className="stat-label">FAILED</span>
            <span className="stat-value text-error">{stats.totalFailed}</span>
          </div>
          {staleCount > 0 && (
            <div className="stat-item">
              <span className="stat-label">STALE DEVICES</span>
              <span className="stat-value text-warning">{staleCount}</span>
            </div>
          )}
        </div>
      )}

      {/* Stale Warning */}
      {staleInfo?.devices?.length > 0 && (
        <div className="stale-warning">
          <strong>Stale Notifications:</strong> {staleInfo.devices.length} device(s) have notifications
          pending for more than {staleInfo.staleThresholdHours} hours. These devices may be offline or unreachable.
        </div>
      )}

      {/* Header with filters */}
      <div className="tab-header">
        <div className="filter-group">
          <label>STATUS</label>
          <CustomDropdown
            id="status-filter"
            options={statusFilterOptions}
            value={statusFilter}
            onChange={(val) => setStatusFilter(val)}
          />
        </div>

        <div className="header-actions">
          {failedCount > 0 && (
            <button
              className="btn btn-warning"
              onClick={handleRetryAllFailed}
              disabled={actionInProgress}
            >
              RETRY ALL FAILED ({failedCount})
            </button>
          )}
          {deliveredCount > 0 && (
            <button
              className="btn btn-secondary"
              onClick={handleClearDelivered}
              disabled={actionInProgress}
            >
              CLEAR DELIVERED ({deliveredCount})
            </button>
          )}
          <button
            className="btn btn-secondary"
            onClick={fetchData}
            disabled={loading}
          >
            REFRESH
          </button>
        </div>
      </div>

      {/* Notifications List */}
      {notifications.length > 0 ? (
        <div className="notifications-list">
          <table className="data-table">
            <thead>
              <tr>
                <th>TYPE</th>
                <th>DEVICE</th>
                <th>STATUS</th>
                <th>CREATED</th>
                <th>RETRIES</th>
                <th>ACTIONS</th>
              </tr>
            </thead>
            <tbody>
              {notifications.map((notification) => {
                const statusBadge = getStatusBadge(notification.status, notification.isStale);
                return (
                  <tr key={notification.id} className={notification.isStale ? 'stale-row' : ''}>
                    <td>
                      <code className="notification-type">{notification.type}</code>
                    </td>
                    <td>
                      <span className="device-name">{notification.deviceName}</span>
                    </td>
                    <td>
                      <Badge variant={statusBadge.variant} filled>
                        {statusBadge.label}
                      </Badge>
                    </td>
                    <td>
                      <span className="timestamp" title={new Date(notification.createdAt).toLocaleString()}>
                        {formatRelativeTime(notification.createdAt)}
                      </span>
                    </td>
                    <td>
                      <span className="retry-count">
                        {notification.retryCount}/{notification.maxRetries}
                      </span>
                    </td>
                    <td>
                      <div className="action-buttons">
                        {(notification.status === 'pending' || notification.status === 'failed') && (
                          <>
                            <button
                              className={`btn btn-sm btn-secondary ${retryingId === notification.id ? 'btn-loading' : ''}`}
                              onClick={() => handleRetry(notification)}
                              disabled={actionInProgress}
                              title="Retry delivery"
                            >
                              {retryingId === notification.id ? 'SENDING...' : 'RETRY'}
                            </button>
                            <button
                              className="btn btn-sm btn-error"
                              onClick={() => handleDismiss(notification)}
                              disabled={actionInProgress}
                              title="Dismiss notification"
                            >
                              DISMISS
                            </button>
                          </>
                        )}
                      </div>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      ) : (
        <div className="empty-state">
          <h3>No Notifications</h3>
          <p className="text-secondary">
            {statusFilter
              ? `No ${statusFilter} notifications found.`
              : 'No notifications in the queue.'}
          </p>
        </div>
      )}

      {/* Dismiss Confirmation Modal */}
      <ConfirmationModal
        isOpen={dismissModalOpen}
        onClose={() => {
          setDismissModalOpen(false);
          setSelectedNotification(null);
        }}
        onConfirm={() => performDismiss(selectedNotification?.id)}
        title="Dismiss Notification"
        message={`Are you sure you want to dismiss this notification?`}
        details={
          selectedNotification ? (
            <div className="confirmation-details">
              <div className="detail-row">
                <strong>Type:</strong> <span>{selectedNotification.type}</span>
              </div>
              <div className="detail-row">
                <strong>Device:</strong> <span>{selectedNotification.deviceName}</span>
              </div>
              <div className="detail-row">
                <strong>Status:</strong> <span>{selectedNotification.status}</span>
              </div>
              <p style={{ marginTop: 'var(--spacing-md)', color: 'var(--color-warning)' }}>
                This notification will be marked as dismissed and will not be delivered.
              </p>
            </div>
          ) : null
        }
        danger={true}
      />
    </div>
  );
}
