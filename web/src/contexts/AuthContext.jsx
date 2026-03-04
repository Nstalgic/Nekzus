/**
 * AuthContext - Authentication state management for Nekzus
 *
 * Manages user authentication state, JWT tokens, and login/logout flows.
 * Provides authentication context throughout the application with automatic
 * token validation and persistence.
 *
 * @module contexts/AuthContext
 */

import { createContext, useContext, useState, useEffect, useCallback, useRef } from 'react';
import PropTypes from 'prop-types';
import { useSettings } from './SettingsContext';

/**
 * LocalStorage key for JWT token persistence
 * @constant {string}
 */
const TOKEN_STORAGE_KEY = 'nekzus-token';

/**
 * LocalStorage key for last activity timestamp
 * @constant {string}
 */
const LAST_ACTIVITY_KEY = 'nekzus-last-activity';

/**
 * API endpoints for authentication
 * @constant {Object}
 */
const AUTH_ENDPOINTS = {
  LOGIN: '/api/v1/auth/login',
  LOGOUT: '/api/v1/auth/logout',
  ME: '/api/v1/auth/me',
};

/**
 * Authentication Context
 * @type {React.Context}
 */
const AuthContext = createContext(null);

/**
 * AuthProvider Component
 *
 * Provides authentication state and methods throughout the application.
 * Handles JWT token management, user session persistence, and automatic
 * authentication validation on mount.
 *
 * Authentication Flow:
 * 1. On mount: Check for existing token in localStorage
 * 2. If token exists: Validate with backend via /api/v1/auth/me
 * 3. If valid: Set user and authenticated state
 * 4. If invalid: Clear token and remain unauthenticated
 *
 * @component
 * @param {Object} props - Component props
 * @param {React.ReactNode} props.children - Child components
 * @returns {JSX.Element} Authentication provider wrapper
 *
 * @example
 * <AuthProvider>
 *   <App />
 * </AuthProvider>
 */
export const AuthProvider = ({ children }) => {
  const { settings } = useSettings();
  const [user, setUser] = useState(null);
  const [token, setToken] = useState(null);
  const [isLoading, setIsLoading] = useState(true);
  const sessionTimeoutRef = useRef(null);
  const activityCheckRef = useRef(null);

  /**
   * Derived state: user is authenticated if token exists
   */
  const isAuthenticated = token !== null && user !== null;

  /**
   * Check authentication status with backend
   * Validates current token and fetches user data
   * @returns {Promise<boolean>} True if authenticated, false otherwise
   */
  const checkAuth = useCallback(async () => {
    const storedToken = localStorage.getItem(TOKEN_STORAGE_KEY);

    console.log('[AuthContext] checkAuth called', {
      hasStoredToken: !!storedToken,
      tokenLength: storedToken?.length,
      protocol: window.location.protocol,
      host: window.location.host,
    });

    if (!storedToken) {
      console.log('[AuthContext] no stored token, not authenticated');
      setIsLoading(false);
      return false;
    }

    try {
      console.log('[AuthContext] validating token with backend', {
        endpoint: AUTH_ENDPOINTS.ME,
      });

      const response = await fetch(AUTH_ENDPOINTS.ME, {
        headers: {
          'Content-Type': 'application/json',
          'Authorization': `Bearer ${storedToken}`,
        },
      });

      console.log('[AuthContext] token validation response', {
        status: response.status,
        statusText: response.statusText,
        ok: response.ok,
      });

      if (!response.ok) {
        // Token is invalid or expired
        throw new Error(`Token validation failed: ${response.status} ${response.statusText}`);
      }

      const data = await response.json();
      console.log('[AuthContext] token valid, user authenticated', {
        username: data.user?.username,
        userId: data.user?.id,
      });

      // Set authenticated state
      setUser(data.user);
      setToken(storedToken);
      setIsLoading(false);
      return true;
    } catch (error) {
      console.error('[AuthContext] auth check failed', {
        message: error.message,
        name: error.name,
      });
      // Clear invalid token
      localStorage.removeItem(TOKEN_STORAGE_KEY);
      setUser(null);
      setToken(null);
      setIsLoading(false);
      return false;
    }
  }, []);

  /**
   * Logout user and clear authentication state
   * Calls backend logout endpoint and clears local storage
   *
   * @returns {Promise<void>}
   *
   * @example
   * await logout();
   * // User is now logged out, UI will redirect to login page
   */
  const logout = useCallback(async () => {
    try {
      // Attempt to call logout endpoint (best effort)
      const storedToken = localStorage.getItem(TOKEN_STORAGE_KEY);
      if (storedToken) {
        await fetch(AUTH_ENDPOINTS.LOGOUT, {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
            'Authorization': `Bearer ${storedToken}`,
          },
        }).catch(err => {
          console.warn('Logout endpoint failed (continuing anyway):', err);
        });
      }
    } finally {
      // Always clear local state, even if API call fails
      localStorage.removeItem(TOKEN_STORAGE_KEY);
      localStorage.removeItem(LAST_ACTIVITY_KEY);

      // Clear session timeout
      if (sessionTimeoutRef.current) {
        clearTimeout(sessionTimeoutRef.current);
        sessionTimeoutRef.current = null;
      }
      if (activityCheckRef.current) {
        clearInterval(activityCheckRef.current);
        activityCheckRef.current = null;
      }

      setUser(null);
      setToken(null);
    }
  }, []);

  /**
   * Update last activity timestamp
   * Resets the session timeout timer
   */
  const updateActivity = useCallback(() => {
    if (!isAuthenticated) return;

    const now = Date.now();
    localStorage.setItem(LAST_ACTIVITY_KEY, now.toString());

    // Clear existing timeout
    if (sessionTimeoutRef.current) {
      clearTimeout(sessionTimeoutRef.current);
    }

    // Set new timeout based on settings (convert minutes to milliseconds)
    const timeoutMs = settings.sessionTimeout * 60 * 1000;
    sessionTimeoutRef.current = setTimeout(() => {
      console.warn('Session timed out due to inactivity');
      logout();
    }, timeoutMs);
  }, [isAuthenticated, settings.sessionTimeout, logout]);

  /**
   * Check if session has expired based on last activity
   * @returns {boolean} True if session is expired
   */
  const checkSessionExpiry = useCallback(() => {
    if (!isAuthenticated) return false;

    const lastActivity = localStorage.getItem(LAST_ACTIVITY_KEY);
    if (!lastActivity) {
      // No activity recorded, update it
      updateActivity();
      return false;
    }

    const now = Date.now();
    const lastActivityTime = parseInt(lastActivity, 10);
    const timeoutMs = settings.sessionTimeout * 60 * 1000;
    const elapsed = now - lastActivityTime;

    if (elapsed > timeoutMs) {
      console.warn('Session expired based on last activity check');
      logout();
      return true;
    }

    return false;
  }, [isAuthenticated, settings.sessionTimeout, updateActivity, logout]);

  /**
   * Login user with username and password
   * @param {string} username - Username
   * @param {string} password - Password
   * @returns {Promise<Object>} Login result with success flag and error message
   *
   * @example
   * const result = await login('admin', 'password123');
   * if (result.success) {
   *   console.log('Logged in as:', result.user);
   * } else {
   *   console.error('Login failed:', result.error);
   * }
   */
  const login = useCallback(async (username, password) => {
    console.log('[AuthContext] login attempt', {
      username,
      endpoint: AUTH_ENDPOINTS.LOGIN,
      protocol: window.location.protocol,
      host: window.location.host,
    });

    try {
      const response = await fetch(AUTH_ENDPOINTS.LOGIN, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({ username, password }),
      });

      console.log('[AuthContext] login response', {
        status: response.status,
        statusText: response.statusText,
        ok: response.ok,
        headers: Object.fromEntries(response.headers.entries()),
      });

      const data = await response.json();
      console.log('[AuthContext] login response data', {
        hasToken: !!data.token,
        hasUser: !!data.user,
        error: data.message || data.error,
        code: data.code,
      });

      if (!response.ok) {
        console.warn('[AuthContext] login failed', {
          status: response.status,
          error: data.message || data.error,
          code: data.code,
        });
        return {
          success: false,
          error: data.message || 'Login failed',
        };
      }

      // Store token
      localStorage.setItem(TOKEN_STORAGE_KEY, data.token);
      setToken(data.token);
      setUser(data.user);

      console.log('[AuthContext] login successful', {
        username: data.user?.username,
        userId: data.user?.id,
      });

      // Initialize session timeout
      updateActivity();

      return {
        success: true,
        user: data.user,
      };
    } catch (error) {
      console.error('[AuthContext] login network error', {
        message: error.message,
        name: error.name,
        stack: error.stack,
      });
      return {
        success: false,
        error: error.message || 'Network error: Unable to connect to server',
      };
    }
  }, [updateActivity]);

  /**
   * Check authentication status on mount
   */
  useEffect(() => {
    checkAuth();
  }, [checkAuth]);

  /**
   * Setup session timeout when user is authenticated
   */
  useEffect(() => {
    if (!isAuthenticated) return;

    // Initialize session timeout
    updateActivity();

    // Periodically check for session expiry (every minute)
    activityCheckRef.current = setInterval(() => {
      checkSessionExpiry();
    }, 60 * 1000);

    // Track user activity events
    const activityEvents = ['mousedown', 'keydown', 'scroll', 'touchstart'];
    const handleActivity = () => {
      updateActivity();
    };

    activityEvents.forEach(event => {
      window.addEventListener(event, handleActivity);
    });

    // Cleanup
    return () => {
      if (sessionTimeoutRef.current) {
        clearTimeout(sessionTimeoutRef.current);
      }
      if (activityCheckRef.current) {
        clearInterval(activityCheckRef.current);
      }
      activityEvents.forEach(event => {
        window.removeEventListener(event, handleActivity);
      });
    };
  }, [isAuthenticated, updateActivity, checkSessionExpiry]);

  const value = {
    user,
    token,
    isAuthenticated,
    isLoading,
    login,
    logout,
    checkAuth,
  };

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
};

AuthProvider.propTypes = {
  children: PropTypes.node.isRequired,
};

/**
 * useAuth Hook
 *
 * Custom hook to access authentication context.
 * Must be used within an AuthProvider.
 *
 * @returns {Object} Authentication context value
 * @returns {Object|null} return.user - Current user object or null
 * @returns {string|null} return.token - JWT token or null
 * @returns {boolean} return.isAuthenticated - True if user is authenticated
 * @returns {boolean} return.isLoading - True if authentication check is in progress
 * @returns {Function} return.login - Login function (username, password) => Promise
 * @returns {Function} return.logout - Logout function () => Promise
 * @returns {Function} return.checkAuth - Check authentication status () => Promise
 *
 * @throws {Error} If used outside of AuthProvider
 *
 * @example
 * const { user, isAuthenticated, login, logout } = useAuth();
 *
 * // Check if user is logged in
 * if (isAuthenticated) {
 *   console.log('Logged in as:', user.username);
 * }
 *
 * // Login
 * const result = await login('admin', 'password');
 *
 * // Logout
 * await logout();
 */
export const useAuth = () => {
  const context = useContext(AuthContext);

  if (!context) {
    throw new Error('useAuth must be used within an AuthProvider');
  }

  return context;
};

export default AuthContext;
