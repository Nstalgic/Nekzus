/**
 * AuthContext Tests
 *
 * Test suite for authentication context provider and hooks.
 * Tests authentication flow, token management, and state management.
 */

import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { renderHook, act, waitFor } from '@testing-library/react';
import { AuthProvider, useAuth } from './AuthContext';
import { SettingsProvider } from './SettingsContext';

// Mock localStorage
const localStorageMock = (() => {
  let store = {};
  return {
    getItem: vi.fn((key) => store[key] || null),
    setItem: vi.fn((key, value) => {
      store[key] = value.toString();
    }),
    removeItem: vi.fn((key) => {
      delete store[key];
    }),
    clear: vi.fn(() => {
      store = {};
    }),
  };
})();

Object.defineProperty(window, 'localStorage', {
  value: localStorageMock,
});

// Mock fetch
global.fetch = vi.fn();

describe('AuthContext', () => {
  beforeEach(() => {
    localStorageMock.clear();
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  describe('useAuth hook', () => {
    it('should throw error when used outside AuthProvider', () => {
      // Suppress console.error for this test
      const consoleSpy = vi.spyOn(console, 'error').mockImplementation(() => {});

      expect(() => {
        renderHook(() => useAuth());
      }).toThrow('useAuth must be used within an AuthProvider');

      consoleSpy.mockRestore();
    });

    it('should return auth context when used within AuthProvider', () => {
      const wrapper = ({ children }) => <SettingsProvider><AuthProvider>{children}</AuthProvider></SettingsProvider>;
      const { result } = renderHook(() => useAuth(), { wrapper });

      expect(result.current).toHaveProperty('user');
      expect(result.current).toHaveProperty('token');
      expect(result.current).toHaveProperty('isAuthenticated');
      expect(result.current).toHaveProperty('isLoading');
      expect(result.current).toHaveProperty('login');
      expect(result.current).toHaveProperty('logout');
      expect(result.current).toHaveProperty('checkAuth');
    });
  });

  describe('Initial state', () => {
    it('should start with no user and no token', async () => {
      const wrapper = ({ children }) => <SettingsProvider><AuthProvider>{children}</AuthProvider></SettingsProvider>;
      const { result } = renderHook(() => useAuth(), { wrapper });

      await waitFor(() => {
        expect(result.current.isLoading).toBe(false);
      });

      expect(result.current.user).toBeNull();
      expect(result.current.token).toBeNull();
      expect(result.current.isAuthenticated).toBe(false);
    });

    it('should check for existing token on mount', async () => {
      // Set a token in localStorage
      localStorageMock.setItem('nekzus-token', 'test-token');

      // Mock successful /me response
      global.fetch.mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: async () => ({
          user: { id: '1', username: 'admin' },
        }),
      });

      const wrapper = ({ children }) => <SettingsProvider><AuthProvider>{children}</AuthProvider></SettingsProvider>;
      const { result } = renderHook(() => useAuth(), { wrapper });

      // Should be loading initially
      expect(result.current.isLoading).toBe(true);

      // Wait for auth check to complete
      await waitFor(() => {
        expect(result.current.isLoading).toBe(false);
      });

      expect(result.current.isAuthenticated).toBe(true);
      expect(result.current.user).toEqual({ id: '1', username: 'admin' });
      expect(result.current.token).toBe('test-token');
    });

    it('should clear invalid token on mount', async () => {
      localStorageMock.setItem('nekzus-token', 'invalid-token');

      // Mock 401 response
      global.fetch.mockResolvedValueOnce({
        ok: false,
        status: 401,
        json: async () => ({ message: 'Unauthorized' }),
      });

      const wrapper = ({ children }) => <SettingsProvider><AuthProvider>{children}</AuthProvider></SettingsProvider>;
      const { result } = renderHook(() => useAuth(), { wrapper });

      await waitFor(() => {
        expect(result.current.isLoading).toBe(false);
      });

      expect(result.current.isAuthenticated).toBe(false);
      expect(result.current.user).toBeNull();
      expect(result.current.token).toBeNull();
      expect(localStorageMock.removeItem).toHaveBeenCalledWith('nekzus-token');
    });
  });

  describe('login', () => {
    it('should successfully login with valid credentials', async () => {
      global.fetch.mockResolvedValueOnce({
        ok: true,
        status: 200,
        statusText: 'OK',
        headers: new Headers({ 'content-type': 'application/json' }),
        json: async () => ({
          token: 'new-jwt-token',
          user: { id: '1', username: 'admin' },
        }),
      });

      const wrapper = ({ children }) => <SettingsProvider><AuthProvider>{children}</AuthProvider></SettingsProvider>;
      const { result } = renderHook(() => useAuth(), { wrapper });

      await waitFor(() => {
        expect(result.current.isLoading).toBe(false);
      });

      let loginResult;
      await act(async () => {
        loginResult = await result.current.login('admin', 'password123');
      });

      expect(loginResult.success).toBe(true);
      expect(result.current.isAuthenticated).toBe(true);
      expect(result.current.user).toEqual({ id: '1', username: 'admin' });
      expect(result.current.token).toBe('new-jwt-token');
      expect(localStorageMock.setItem).toHaveBeenCalledWith('nekzus-token', 'new-jwt-token');
    });

    it('should fail login with invalid credentials', async () => {
      global.fetch.mockResolvedValueOnce({
        ok: false,
        status: 401,
        statusText: 'Unauthorized',
        headers: new Headers({ 'content-type': 'application/json' }),
        json: async () => ({ message: 'Invalid credentials' }),
      });

      const wrapper = ({ children }) => <SettingsProvider><AuthProvider>{children}</AuthProvider></SettingsProvider>;
      const { result } = renderHook(() => useAuth(), { wrapper });

      await waitFor(() => {
        expect(result.current.isLoading).toBe(false);
      });

      let loginResult;
      await act(async () => {
        loginResult = await result.current.login('admin', 'wrongpassword');
      });

      expect(loginResult.success).toBe(false);
      expect(loginResult.error).toBe('Invalid credentials');
      expect(result.current.isAuthenticated).toBe(false);
      expect(result.current.user).toBeNull();
      expect(result.current.token).toBeNull();
    });

    it('should handle network errors during login', async () => {
      global.fetch.mockRejectedValueOnce(new Error('Network error'));

      const wrapper = ({ children }) => <SettingsProvider><AuthProvider>{children}</AuthProvider></SettingsProvider>;
      const { result } = renderHook(() => useAuth(), { wrapper });

      await waitFor(() => {
        expect(result.current.isLoading).toBe(false);
      });

      let loginResult;
      await act(async () => {
        loginResult = await result.current.login('admin', 'password123');
      });

      expect(loginResult.success).toBe(false);
      expect(loginResult.error).toContain('Network error');
    });
  });

  describe('logout', () => {
    it('should clear user data and token', async () => {
      // Setup authenticated state
      localStorageMock.setItem('nekzus-token', 'test-token');
      global.fetch.mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: async () => ({
          user: { id: '1', username: 'admin' },
        }),
      });

      const wrapper = ({ children }) => <SettingsProvider><AuthProvider>{children}</AuthProvider></SettingsProvider>;
      const { result } = renderHook(() => useAuth(), { wrapper });

      await waitFor(() => {
        expect(result.current.isAuthenticated).toBe(true);
      });

      // Mock logout endpoint
      global.fetch.mockResolvedValueOnce({
        ok: true,
        status: 200,
      });

      // Logout
      await act(async () => {
        await result.current.logout();
      });

      expect(result.current.isAuthenticated).toBe(false);
      expect(result.current.user).toBeNull();
      expect(result.current.token).toBeNull();
      expect(localStorageMock.removeItem).toHaveBeenCalledWith('nekzus-token');
    });

    it('should still clear local state even if logout API fails', async () => {
      localStorageMock.setItem('nekzus-token', 'test-token');
      global.fetch.mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: async () => ({
          user: { id: '1', username: 'admin' },
        }),
      });

      const wrapper = ({ children }) => <SettingsProvider><AuthProvider>{children}</AuthProvider></SettingsProvider>;
      const { result } = renderHook(() => useAuth(), { wrapper });

      await waitFor(() => {
        expect(result.current.isAuthenticated).toBe(true);
      });

      // Mock failed logout
      global.fetch.mockRejectedValueOnce(new Error('Network error'));

      await act(async () => {
        await result.current.logout();
      });

      expect(result.current.isAuthenticated).toBe(false);
      expect(result.current.user).toBeNull();
      expect(result.current.token).toBeNull();
    });
  });

  describe('checkAuth', () => {
    it('should validate existing token', async () => {
      const wrapper = ({ children }) => <SettingsProvider><AuthProvider>{children}</AuthProvider></SettingsProvider>;
      const { result } = renderHook(() => useAuth(), { wrapper });

      await waitFor(() => {
        expect(result.current.isLoading).toBe(false);
      });

      // Login first
      global.fetch.mockResolvedValueOnce({
        ok: true,
        status: 200,
        statusText: 'OK',
        headers: new Headers({ 'content-type': 'application/json' }),
        json: async () => ({
          token: 'test-token',
          user: { id: '1', username: 'admin' },
        }),
      });

      await act(async () => {
        await result.current.login('admin', 'password');
      });

      // Mock /me endpoint
      global.fetch.mockResolvedValueOnce({
        ok: true,
        status: 200,
        statusText: 'OK',
        headers: new Headers({ 'content-type': 'application/json' }),
        json: async () => ({
          user: { id: '1', username: 'admin' },
        }),
      });

      // Check auth
      await act(async () => {
        await result.current.checkAuth();
      });

      expect(result.current.isAuthenticated).toBe(true);
    });

    it('should logout if token is invalid', async () => {
      localStorageMock.setItem('nekzus-token', 'invalid-token');

      const wrapper = ({ children }) => <SettingsProvider><AuthProvider>{children}</AuthProvider></SettingsProvider>;
      const { result } = renderHook(() => useAuth(), { wrapper });

      // Mock 401 response
      global.fetch.mockResolvedValueOnce({
        ok: false,
        status: 401,
        statusText: 'Unauthorized',
        headers: new Headers({ 'content-type': 'application/json' }),
        json: async () => ({ message: 'Unauthorized' }),
      });

      await waitFor(() => {
        expect(result.current.isLoading).toBe(false);
      });

      expect(result.current.isAuthenticated).toBe(false);
      expect(localStorageMock.removeItem).toHaveBeenCalledWith('nekzus-token');
    });
  });
});
