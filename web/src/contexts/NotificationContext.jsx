import { createContext, useContext, useState, useEffect, useCallback } from 'react';
import { useSettings } from './SettingsContext';

const NotificationContext = createContext(null);

const STORAGE_KEY = 'nekzus-notifications';
const MAX_NOTIFICATIONS = 50; // Limit stored notifications

/**
 * Notification shape:
 * {
 *   id: string,
 *   severity: 'info' | 'success' | 'warning' | 'error',
 *   message: string,
 *   strongText?: string,
 *   link?: { text: string, href: string },
 *   error?: Error,        // Optional error object for detailed display (when showErrorDetails is enabled)
 *   timestamp: number,
 *   dismissed: boolean,   // Permanently dismissed from bell dropdown
 *   toasted: boolean      // Has been shown as a toast (prevents re-showing)
 * }
 */

export function NotificationProvider({ children }) {
  const { settings } = useSettings();
  const [notifications, setNotifications] = useState(() => {
    // Load notifications from localStorage on mount
    try {
      const saved = localStorage.getItem(STORAGE_KEY);
      if (saved) {
        const parsed = JSON.parse(saved);
        return Array.isArray(parsed) ? parsed : [];
      }
    } catch (error) {
      console.error('Failed to load notifications from localStorage:', error);
    }
    return [];
  });

  // Save notifications to localStorage whenever they change
  useEffect(() => {
    try {
      localStorage.setItem(STORAGE_KEY, JSON.stringify(notifications));
    } catch (error) {
      console.error('Failed to save notifications to localStorage:', error);
    }
  }, [notifications]);

  // Add a new notification
  const addNotification = useCallback((notification) => {
    // Check notification preferences before adding
    if (notification.type) {
      switch (notification.type) {
        case 'discovery':
          if (!settings.notifyNewDiscoveries) return null;
          break;
        case 'device_offline':
          if (!settings.notifyDeviceOffline) return null;
          break;
        case 'certificate_expiry':
          if (!settings.notifyCertificateExpiry) return null;
          break;
        case 'route_status':
          if (!settings.notifyRouteStatusChange) return null;
          break;
        case 'system_health':
          if (!settings.notifySystemHealth) return null;
          break;
        // Allow notifications without a type or unknown types
        default:
          break;
      }
    }

    const newNotification = {
      ...notification,
      id: `notif-${Date.now()}-${Math.random().toString(36).substr(2, 9)}`,
      timestamp: Date.now(),
      dismissed: false,
      toasted: false,  // Not yet shown as toast
    };

    setNotifications((prev) => {
      const updated = [newNotification, ...prev];
      // Keep only the most recent MAX_NOTIFICATIONS
      return updated.slice(0, MAX_NOTIFICATIONS);
    });

    return newNotification.id;
  }, [settings]);

  // Mark notification as toasted (visual toast dismissed, but keep in bell dropdown)
  const markAsToasted = useCallback((id) => {
    setNotifications((prev) =>
      prev.map((notification) =>
        notification.id === id
          ? { ...notification, toasted: true }
          : notification
      )
    );
  }, []);

  // Dismiss a specific notification (permanently remove from bell dropdown)
  const dismissNotification = useCallback((id) => {
    setNotifications((prev) =>
      prev.map((notification) =>
        notification.id === id
          ? { ...notification, dismissed: true }
          : notification
      )
    );
  }, []);

  // Dismiss all notifications
  const dismissAll = useCallback(() => {
    setNotifications((prev) =>
      prev.map((notification) => ({ ...notification, dismissed: true }))
    );
  }, []);

  // Remove dismissed notifications
  const clearDismissed = useCallback(() => {
    setNotifications((prev) =>
      prev.filter((notification) => !notification.dismissed)
    );
  }, []);

  // Get count of undismissed notifications
  const unreadCount = notifications.filter((n) => !n.dismissed).length;

  // Get notifications by severity
  const getNotificationsBySeverity = useCallback(
    (severity) => {
      return notifications.filter((n) => n.severity === severity);
    },
    [notifications]
  );

  const value = {
    notifications,
    addNotification,
    markAsToasted,
    dismissNotification,
    dismissAll,
    clearDismissed,
    unreadCount,
    getNotificationsBySeverity,
  };

  return (
    <NotificationContext.Provider value={value}>
      {children}
    </NotificationContext.Provider>
  );
}

export function useNotification() {
  const context = useContext(NotificationContext);
  if (!context) {
    throw new Error('useNotification must be used within NotificationProvider');
  }
  return context;
}
