import { render, screen, waitFor } from '@testing-library/react';
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import Toast from './Toast';

describe('Toast', () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  describe('rendering', () => {
    it('should render toast with message', () => {
      render(
        <Toast
          notification={{
            id: 'test-1',
            severity: 'info',
            message: 'Test message',
            timestamp: Date.now(),
            dismissed: false,
          }}
          onDismiss={() => {}}
        />
      );

      expect(screen.getByText('Test message')).toBeInTheDocument();
    });

    it('should render toast with strongText prefix', () => {
      render(
        <Toast
          notification={{
            id: 'test-1',
            severity: 'warning',
            message: 'Certificate expiring',
            strongText: 'WARNING:',
            timestamp: Date.now(),
            dismissed: false,
          }}
          onDismiss={() => {}}
        />
      );

      expect(screen.getByText('WARNING:')).toBeInTheDocument();
      expect(screen.getByText('Certificate expiring')).toBeInTheDocument();
    });

    it('should render toast with link', () => {
      render(
        <Toast
          notification={{
            id: 'test-1',
            severity: 'info',
            message: 'Check details',
            link: {
              text: 'View',
              href: '#details',
            },
            timestamp: Date.now(),
            dismissed: false,
          }}
          onDismiss={() => {}}
        />
      );

      const link = screen.getByText('View');
      expect(link).toBeInTheDocument();
      expect(link).toHaveAttribute('href', '#details');
    });

    it('should apply correct severity class', () => {
      const { rerender } = render(
        <Toast
          notification={{
            id: 'test-1',
            severity: 'error',
            message: 'Error message',
            timestamp: Date.now(),
            dismissed: false,
          }}
          onDismiss={() => {}}
        />
      );

      const toast = screen.getByRole('alert');
      expect(toast).toHaveClass('toast-error');

      rerender(
        <Toast
          notification={{
            id: 'test-2',
            severity: 'success',
            message: 'Success message',
            timestamp: Date.now(),
            dismissed: false,
          }}
          onDismiss={() => {}}
        />
      );

      const successToast = screen.getByRole('alert');
      expect(successToast).toHaveClass('toast-success');
    });

    it('should apply severity class based on notification severity', () => {
      render(
        <Toast
          notification={{
            id: 'test-1',
            severity: 'warning',
            message: 'Warning message',
            timestamp: Date.now(),
            dismissed: false,
          }}
          onDismiss={() => {}}
        />
      );

      const toast = screen.getByRole('alert');
      expect(toast).toHaveClass('toast-warning');
    });
  });

  describe('auto-dismiss', () => {
    it('should auto-dismiss after default duration (5s)', () => {
      const onDismiss = vi.fn();

      render(
        <Toast
          notification={{
            id: 'test-1',
            severity: 'info',
            message: 'Auto dismiss test',
            timestamp: Date.now(),
            dismissed: false,
          }}
          onDismiss={onDismiss}
        />
      );

      expect(onDismiss).not.toHaveBeenCalled();

      // Fast-forward 5 seconds
      vi.advanceTimersByTime(5000);

      expect(onDismiss).toHaveBeenCalledWith('test-1');
    });

    it('should auto-dismiss after custom duration', () => {
      const onDismiss = vi.fn();

      render(
        <Toast
          notification={{
            id: 'test-1',
            severity: 'info',
            message: 'Custom duration test',
            timestamp: Date.now(),
            dismissed: false,
          }}
          onDismiss={onDismiss}
          duration={3000}
        />
      );

      vi.advanceTimersByTime(3000);

      expect(onDismiss).toHaveBeenCalledWith('test-1');
    });

    it('should not auto-dismiss when duration is null', () => {
      const onDismiss = vi.fn();

      render(
        <Toast
          notification={{
            id: 'test-1',
            severity: 'error',
            message: 'Persistent error',
            timestamp: Date.now(),
            dismissed: false,
          }}
          onDismiss={onDismiss}
          duration={null}
        />
      );

      vi.advanceTimersByTime(10000);

      expect(onDismiss).not.toHaveBeenCalled();
    });

    it('should clear timeout on unmount', () => {
      const onDismiss = vi.fn();

      const { unmount } = render(
        <Toast
          notification={{
            id: 'test-1',
            severity: 'info',
            message: 'Unmount test',
            timestamp: Date.now(),
            dismissed: false,
          }}
          onDismiss={onDismiss}
        />
      );

      // Unmount before timer completes
      unmount();

      vi.advanceTimersByTime(5000);

      expect(onDismiss).not.toHaveBeenCalled();
    });
  });

  describe('manual dismiss', () => {
    it('should call onDismiss when close button clicked', async () => {
      const onDismiss = vi.fn();

      render(
        <Toast
          notification={{
            id: 'test-1',
            severity: 'info',
            message: 'Manual dismiss test',
            timestamp: Date.now(),
            dismissed: false,
          }}
          onDismiss={onDismiss}
        />
      );

      const closeButton = screen.getByRole('button', { name: /close/i });
      closeButton.click();

      expect(onDismiss).toHaveBeenCalledWith('test-1');
    });
  });

  describe('animations', () => {
    it('should have slide-in animation class', () => {
      render(
        <Toast
          notification={{
            id: 'test-1',
            severity: 'info',
            message: 'Animation test',
            timestamp: Date.now(),
            dismissed: false,
          }}
          onDismiss={() => {}}
        />
      );

      const toast = screen.getByRole('alert');
      expect(toast).toHaveClass('toast-enter');
    });
  });

  describe('accessibility', () => {
    it('should have role="alert"', () => {
      render(
        <Toast
          notification={{
            id: 'test-1',
            severity: 'info',
            message: 'Accessibility test',
            timestamp: Date.now(),
            dismissed: false,
          }}
          onDismiss={() => {}}
        />
      );

      expect(screen.getByRole('alert')).toBeInTheDocument();
    });

    it('should have aria-live="polite" for info/success', () => {
      render(
        <Toast
          notification={{
            id: 'test-1',
            severity: 'info',
            message: 'Info message',
            timestamp: Date.now(),
            dismissed: false,
          }}
          onDismiss={() => {}}
        />
      );

      const toast = screen.getByRole('alert');
      expect(toast).toHaveAttribute('aria-live', 'polite');
    });

    it('should have aria-live="assertive" for warning/error', () => {
      render(
        <Toast
          notification={{
            id: 'test-1',
            severity: 'error',
            message: 'Error message',
            timestamp: Date.now(),
            dismissed: false,
          }}
          onDismiss={() => {}}
        />
      );

      const toast = screen.getByRole('alert');
      expect(toast).toHaveAttribute('aria-live', 'assertive');
    });

    it('should have accessible close button label', () => {
      render(
        <Toast
          notification={{
            id: 'test-1',
            severity: 'info',
            message: 'Test',
            timestamp: Date.now(),
            dismissed: false,
          }}
          onDismiss={() => {}}
        />
      );

      expect(
        screen.getByRole('button', { name: /close notification/i })
      ).toBeInTheDocument();
    });
  });
});
