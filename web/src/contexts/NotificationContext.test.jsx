import { render, renderHook, act, screen } from '@testing-library/react';
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { NotificationProvider, useNotification } from './NotificationContext';
import { SettingsProvider } from './SettingsContext';

// Wrapper that provides all required context providers
const TestWrapper = ({ children }) => (
  <SettingsProvider>
    <NotificationProvider>
      {children}
    </NotificationProvider>
  </SettingsProvider>
);

describe('NotificationContext', () => {
  beforeEach(() => {
    localStorage.clear();
    vi.clearAllTimers();
  });

  describe('Provider', () => {
    it('should render children', () => {
      render(
        <TestWrapper>
          <div>Test Child</div>
        </TestWrapper>
      );
      expect(screen.getByText('Test Child')).toBeInTheDocument();
    });

    it('should throw error when useNotification is used outside provider', () => {
      const spy = vi.spyOn(console, 'error').mockImplementation(() => {});

      expect(() => {
        renderHook(() => useNotification());
      }).toThrow('useNotification must be used within NotificationProvider');

      spy.mockRestore();
    });
  });

  describe('addNotification', () => {
    it('should add a notification with all properties', () => {
      const { result } = renderHook(() => useNotification(), {
        wrapper: TestWrapper,
      });

      act(() => {
        result.current.addNotification({
          severity: 'warning',
          message: 'Test warning message',
          strongText: 'WARNING:',
        });
      });

      expect(result.current.notifications).toHaveLength(1);
      expect(result.current.notifications[0]).toMatchObject({
        severity: 'warning',
        message: 'Test warning message',
        strongText: 'WARNING:',
        dismissed: false,
      });
      expect(result.current.notifications[0].id).toBeDefined();
      expect(result.current.notifications[0].timestamp).toBeDefined();
    });

    it('should generate unique IDs for notifications', () => {
      const { result } = renderHook(() => useNotification(), {
        wrapper: TestWrapper,
      });

      act(() => {
        result.current.addNotification({ severity: 'info', message: 'First' });
        result.current.addNotification({ severity: 'info', message: 'Second' });
      });

      expect(result.current.notifications).toHaveLength(2);
      expect(result.current.notifications[0].id).not.toBe(
        result.current.notifications[1].id
      );
    });

    it('should add multiple notifications', () => {
      const { result } = renderHook(() => useNotification(), {
        wrapper: TestWrapper,
      });

      act(() => {
        result.current.addNotification({ severity: 'info', message: 'First' });
        result.current.addNotification({ severity: 'warning', message: 'Second' });
        result.current.addNotification({ severity: 'error', message: 'Third' });
      });

      expect(result.current.notifications).toHaveLength(3);
    });

    it('should add notifications with optional link', () => {
      const { result } = renderHook(() => useNotification(), {
        wrapper: TestWrapper,
      });

      act(() => {
        result.current.addNotification({
          severity: 'info',
          message: 'Check details',
          link: {
            text: 'View',
            href: '#details',
          },
        });
      });

      expect(result.current.notifications[0].link).toEqual({
        text: 'View',
        href: '#details',
      });
    });
  });

  describe('dismissNotification', () => {
    it('should mark notification as dismissed', () => {
      const { result } = renderHook(() => useNotification(), {
        wrapper: TestWrapper,
      });

      act(() => {
        result.current.addNotification({ severity: 'info', message: 'Test' });
      });

      const notificationId = result.current.notifications[0].id;

      act(() => {
        result.current.dismissNotification(notificationId);
      });

      const notification = result.current.notifications.find(
        (n) => n.id === notificationId
      );
      expect(notification.dismissed).toBe(true);
    });

    it('should not remove dismissed notifications immediately', () => {
      const { result } = renderHook(() => useNotification(), {
        wrapper: TestWrapper,
      });

      act(() => {
        result.current.addNotification({ severity: 'info', message: 'Test' });
      });

      const notificationId = result.current.notifications[0].id;

      act(() => {
        result.current.dismissNotification(notificationId);
      });

      expect(result.current.notifications).toHaveLength(1);
    });

    it('should handle dismissing non-existent notification', () => {
      const { result } = renderHook(() => useNotification(), {
        wrapper: TestWrapper,
      });

      act(() => {
        result.current.addNotification({ severity: 'info', message: 'Test' });
      });

      expect(() => {
        act(() => {
          result.current.dismissNotification('non-existent-id');
        });
      }).not.toThrow();

      expect(result.current.notifications).toHaveLength(1);
      expect(result.current.notifications[0].dismissed).toBe(false);
    });
  });

  describe('dismissAll', () => {
    it('should dismiss all notifications', () => {
      const { result } = renderHook(() => useNotification(), {
        wrapper: TestWrapper,
      });

      act(() => {
        result.current.addNotification({ severity: 'info', message: 'First' });
        result.current.addNotification({ severity: 'warning', message: 'Second' });
        result.current.addNotification({ severity: 'error', message: 'Third' });
      });

      act(() => {
        result.current.dismissAll();
      });

      expect(result.current.notifications.every((n) => n.dismissed)).toBe(true);
    });

    it('should handle dismissAll with no notifications', () => {
      const { result } = renderHook(() => useNotification(), {
        wrapper: TestWrapper,
      });

      expect(() => {
        act(() => {
          result.current.dismissAll();
        });
      }).not.toThrow();

      expect(result.current.notifications).toHaveLength(0);
    });
  });

  describe('clearDismissed', () => {
    it('should remove dismissed notifications', () => {
      const { result } = renderHook(() => useNotification(), {
        wrapper: TestWrapper,
      });

      act(() => {
        result.current.addNotification({ severity: 'info', message: 'First' });
        result.current.addNotification({ severity: 'warning', message: 'Second' });
        result.current.addNotification({ severity: 'error', message: 'Third' });
      });

      // Notifications are stored newest-first (Third at index 0, First at index 2)
      const firstId = result.current.notifications[2].id;  // 'First'
      const secondId = result.current.notifications[1].id; // 'Second'

      act(() => {
        result.current.dismissNotification(firstId);
        result.current.dismissNotification(secondId);
      });

      act(() => {
        result.current.clearDismissed();
      });

      expect(result.current.notifications).toHaveLength(1);
      expect(result.current.notifications[0].message).toBe('Third');
    });

    it('should not affect active notifications', () => {
      const { result } = renderHook(() => useNotification(), {
        wrapper: TestWrapper,
      });

      act(() => {
        result.current.addNotification({ severity: 'info', message: 'Active' });
      });

      act(() => {
        result.current.clearDismissed();
      });

      expect(result.current.notifications).toHaveLength(1);
      expect(result.current.notifications[0].dismissed).toBe(false);
    });
  });

  describe('unreadCount', () => {
    it('should return count of undismissed notifications', () => {
      const { result } = renderHook(() => useNotification(), {
        wrapper: TestWrapper,
      });

      expect(result.current.unreadCount).toBe(0);

      act(() => {
        result.current.addNotification({ severity: 'info', message: 'First' });
        result.current.addNotification({ severity: 'warning', message: 'Second' });
      });

      expect(result.current.unreadCount).toBe(2);

      act(() => {
        result.current.dismissNotification(result.current.notifications[0].id);
      });

      expect(result.current.unreadCount).toBe(1);
    });
  });

  describe('getNotificationsBySeverity', () => {
    it('should filter notifications by severity', () => {
      const { result } = renderHook(() => useNotification(), {
        wrapper: TestWrapper,
      });

      act(() => {
        result.current.addNotification({ severity: 'info', message: 'Info 1' });
        result.current.addNotification({ severity: 'warning', message: 'Warning 1' });
        result.current.addNotification({ severity: 'error', message: 'Error 1' });
        result.current.addNotification({ severity: 'warning', message: 'Warning 2' });
      });

      const warnings = result.current.getNotificationsBySeverity('warning');
      expect(warnings).toHaveLength(2);
      expect(warnings.every((n) => n.severity === 'warning')).toBe(true);
    });

    it('should return empty array for severity with no notifications', () => {
      const { result } = renderHook(() => useNotification(), {
        wrapper: TestWrapper,
      });

      act(() => {
        result.current.addNotification({ severity: 'info', message: 'Info' });
      });

      const errors = result.current.getNotificationsBySeverity('error');
      expect(errors).toHaveLength(0);
    });
  });
});
