import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { describe, it, expect } from 'vitest';
import NotificationBell from './NotificationBell';
import { NotificationProvider } from '../../contexts/NotificationContext';
import { SettingsProvider } from '../../contexts/SettingsContext';

const renderWithProvider = (ui) => {
  return render(
    <SettingsProvider>
      <NotificationProvider>{ui}</NotificationProvider>
    </SettingsProvider>
  );
};

describe('NotificationBell', () => {
  describe('rendering', () => {
    it('should render bell icon button', () => {
      renderWithProvider(<NotificationBell />);
      expect(screen.getByRole('button', { name: /notifications/i })).toBeInTheDocument();
    });

    it('should not show badge when count is zero', () => {
      renderWithProvider(<NotificationBell />);
      const badge = screen.queryByText('0');
      expect(badge).not.toBeInTheDocument();
    });
  });

  describe('dropdown behavior', () => {
    it('should show dropdown on mouse enter', async () => {
      renderWithProvider(<NotificationBell />);

      const button = screen.getByRole('button', { name: /notifications/i });
      fireEvent.mouseEnter(button);

      // When dropdown opens, it shows "No notifications" in empty state
      await waitFor(() => {
        expect(screen.getByText(/no notifications/i)).toBeInTheDocument();
      });
    });

    it('should show "No notifications" when empty', async () => {
      renderWithProvider(<NotificationBell />);

      const button = screen.getByRole('button', { name: /notifications/i });
      fireEvent.mouseEnter(button);

      await waitFor(() => {
        expect(screen.getByText(/no notifications/i)).toBeInTheDocument();
      });
    });

    it('should not show "Dismiss All" when no notifications', async () => {
      renderWithProvider(<NotificationBell />);

      const button = screen.getByRole('button', { name: /notifications/i });
      fireEvent.mouseEnter(button);

      await waitFor(() => {
        expect(screen.getByText(/no notifications/i)).toBeInTheDocument();
      });

      expect(screen.queryByRole('button', { name: /dismiss all/i })).not.toBeInTheDocument();
    });
  });

  describe('accessibility', () => {
    it('should have accessible button label', () => {
      renderWithProvider(<NotificationBell />);
      expect(screen.getByRole('button', { name: /notifications/i })).toBeInTheDocument();
    });

    it('should be keyboard focusable', () => {
      renderWithProvider(<NotificationBell />);

      const button = screen.getByRole('button', { name: /notifications/i });
      button.focus();
      expect(button).toHaveFocus();
    });
  });
});
