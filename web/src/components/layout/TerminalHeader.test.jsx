import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import TerminalHeader from './TerminalHeader';

/**
 * TerminalHeader Component Test Suite
 *
 * Tests the TerminalHeader component including:
 * - Rendering NEKZUS title in center
 * - User badge display when authenticated
 * - Logout button functionality
 * - Layout structure (left, center, right sections)
 * - Matching footer styling pattern
 */

// Mock the useAuth hook
vi.mock('../../contexts/AuthContext', async () => {
  const actual = await vi.importActual('../../contexts/AuthContext');
  return {
    ...actual,
    useAuth: vi.fn(),
  };
});

// Mock NotificationBell to avoid context dependencies
vi.mock('../notifications/NotificationBell', () => ({
  default: () => <div data-testid="notification-bell-mock">NotificationBell</div>,
}));

import { useAuth } from '../../contexts/AuthContext';

describe('TerminalHeader', () => {
  describe('Layout Structure', () => {
    it('should render three-column layout (left, center, right)', () => {
      useAuth.mockReturnValue({
        user: null,
        token: null,
        isAuthenticated: false,
        isLoading: false,
        login: vi.fn(),
        logout: vi.fn(),
        checkAuth: vi.fn(),
      });

      const { container } = render(<TerminalHeader />);

      expect(container.querySelector('.terminal-header-left')).toBeInTheDocument();
      expect(container.querySelector('.terminal-header-center')).toBeInTheDocument();
      expect(container.querySelector('.terminal-header-right')).toBeInTheDocument();
    });

    it('should always render centered NEKZUS title', () => {
      useAuth.mockReturnValue({
        user: null,
        token: null,
        isAuthenticated: false,
        isLoading: false,
        login: vi.fn(),
        logout: vi.fn(),
        checkAuth: vi.fn(),
      });

      render(<TerminalHeader />);

      expect(screen.getByText('=== NEKZUS ===')).toBeInTheDocument();
    });
  });

  describe('Unauthenticated State', () => {
    it('should not show user badge when not authenticated', () => {
      useAuth.mockReturnValue({
        user: null,
        token: null,
        isAuthenticated: false,
        isLoading: false,
        login: vi.fn(),
        logout: vi.fn(),
        checkAuth: vi.fn(),
      });

      render(<TerminalHeader />);

      expect(screen.queryByText(/user:/)).not.toBeInTheDocument();
    });

    it('should not show logout button when not authenticated', () => {
      useAuth.mockReturnValue({
        user: null,
        token: null,
        isAuthenticated: false,
        isLoading: false,
        login: vi.fn(),
        logout: vi.fn(),
        checkAuth: vi.fn(),
      });

      render(<TerminalHeader />);

      expect(screen.queryByRole('button', { name: /logout/i })).not.toBeInTheDocument();
    });
  });

  describe('Authenticated State', () => {
    it('should display user badge with username when authenticated', () => {
      useAuth.mockReturnValue({
        user: { username: 'testuser' },
        token: 'fake-token',
        isAuthenticated: true,
        isLoading: false,
        login: vi.fn(),
        logout: vi.fn(),
        checkAuth: vi.fn(),
      });

      render(<TerminalHeader />);

      expect(screen.getByText('user: testuser')).toBeInTheDocument();
    });

    it('should display user badge with "admin" as fallback when username is missing', () => {
      useAuth.mockReturnValue({
        user: {},
        token: 'fake-token',
        isAuthenticated: true,
        isLoading: false,
        login: vi.fn(),
        logout: vi.fn(),
        checkAuth: vi.fn(),
      });

      render(<TerminalHeader />);

      expect(screen.getByText('user: admin')).toBeInTheDocument();
    });

    it('should display username in lowercase', () => {
      useAuth.mockReturnValue({
        user: { username: 'TESTUSER' },
        token: 'fake-token',
        isAuthenticated: true,
        isLoading: false,
        login: vi.fn(),
        logout: vi.fn(),
        checkAuth: vi.fn(),
      });

      render(<TerminalHeader />);

      expect(screen.getByText('user: testuser')).toBeInTheDocument();
    });

    it('should display logout button when authenticated', () => {
      useAuth.mockReturnValue({
        user: { username: 'testuser' },
        token: 'fake-token',
        isAuthenticated: true,
        isLoading: false,
        login: vi.fn(),
        logout: vi.fn(),
        checkAuth: vi.fn(),
      });

      render(<TerminalHeader />);

      expect(screen.getByRole('button', { name: /logout/i })).toBeInTheDocument();
    });

    it('should display separator between user badge and logout button', () => {
      useAuth.mockReturnValue({
        user: { username: 'testuser' },
        token: 'fake-token',
        isAuthenticated: true,
        isLoading: false,
        login: vi.fn(),
        logout: vi.fn(),
        checkAuth: vi.fn(),
      });

      render(<TerminalHeader />);

      // Find separator elements - there may be multiple
      const separators = screen.getAllByText('|');
      expect(separators.length).toBeGreaterThan(0);
      expect(separators[0]).toHaveClass('terminal-separator');
    });
  });

  describe('Logout Functionality', () => {
    it('should call logout function when logout button is clicked', async () => {
      const user = userEvent.setup();
      const mockLogout = vi.fn().mockResolvedValue(undefined);

      useAuth.mockReturnValue({
        user: { username: 'testuser' },
        token: 'fake-token',
        isAuthenticated: true,
        isLoading: false,
        login: vi.fn(),
        logout: mockLogout,
        checkAuth: vi.fn(),
      });

      render(<TerminalHeader />);

      const logoutButton = screen.getByRole('button', { name: /logout/i });
      await user.click(logoutButton);

      expect(mockLogout).toHaveBeenCalledTimes(1);
    });

    it('should handle logout errors gracefully', async () => {
      const user = userEvent.setup();
      const consoleErrorSpy = vi.spyOn(console, 'error').mockImplementation(() => {});
      const mockLogout = vi.fn().mockRejectedValue(new Error('Logout failed'));

      useAuth.mockReturnValue({
        user: { username: 'testuser' },
        token: 'fake-token',
        isAuthenticated: true,
        isLoading: false,
        login: vi.fn(),
        logout: mockLogout,
        checkAuth: vi.fn(),
      });

      render(<TerminalHeader />);

      const logoutButton = screen.getByRole('button', { name: /logout/i });
      await user.click(logoutButton);

      expect(mockLogout).toHaveBeenCalledTimes(1);
      expect(consoleErrorSpy).toHaveBeenCalledWith(
        'Logout error:',
        expect.any(Error)
      );

      consoleErrorSpy.mockRestore();
    });
  });

  describe('Styling and CSS Classes', () => {
    it('should apply terminal-header class to main container', () => {
      useAuth.mockReturnValue({
        user: null,
        token: null,
        isAuthenticated: false,
        isLoading: false,
        login: vi.fn(),
        logout: vi.fn(),
        checkAuth: vi.fn(),
      });

      const { container } = render(<TerminalHeader />);

      const header = container.querySelector('.terminal-header');
      expect(header).toBeInTheDocument();
      expect(header.tagName).toBe('HEADER');
    });

    it('should apply terminal-user-badge class to user badge', () => {
      useAuth.mockReturnValue({
        user: { username: 'testuser' },
        token: 'fake-token',
        isAuthenticated: true,
        isLoading: false,
        login: vi.fn(),
        logout: vi.fn(),
        checkAuth: vi.fn(),
      });

      const { container } = render(<TerminalHeader />);

      const userBadge = container.querySelector('.terminal-user-badge');
      expect(userBadge).toBeInTheDocument();
      expect(userBadge).toHaveTextContent('user: testuser');
    });

    it('should apply terminal-logout-btn class to logout button', () => {
      useAuth.mockReturnValue({
        user: { username: 'testuser' },
        token: 'fake-token',
        isAuthenticated: true,
        isLoading: false,
        login: vi.fn(),
        logout: vi.fn(),
        checkAuth: vi.fn(),
      });

      const { container } = render(<TerminalHeader />);

      const logoutButton = container.querySelector('.terminal-logout-btn');
      expect(logoutButton).toBeInTheDocument();
      expect(logoutButton.tagName).toBe('BUTTON');
    });
  });

  describe('Accessibility', () => {
    it('should have proper aria-label on logout button', () => {
      useAuth.mockReturnValue({
        user: { username: 'testuser' },
        token: 'fake-token',
        isAuthenticated: true,
        isLoading: false,
        login: vi.fn(),
        logout: vi.fn(),
        checkAuth: vi.fn(),
      });

      render(<TerminalHeader />);

      const logoutButton = screen.getByRole('button', { name: /logout/i });
      expect(logoutButton).toHaveAttribute('aria-label', 'Logout');
    });

    it('should render header as semantic HTML element', () => {
      useAuth.mockReturnValue({
        user: null,
        token: null,
        isAuthenticated: false,
        isLoading: false,
        login: vi.fn(),
        logout: vi.fn(),
        checkAuth: vi.fn(),
      });

      const { container } = render(<TerminalHeader />);

      const header = container.querySelector('header');
      expect(header).toBeInTheDocument();
    });
  });
});
