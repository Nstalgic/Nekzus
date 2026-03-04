/**
 * Test Utilities
 *
 * Shared utilities for testing components that require context providers.
 * Provides a wrapper with all necessary providers in the correct nesting order.
 */

import { render } from '@testing-library/react';
import { renderHook } from '@testing-library/react';
import { vi } from 'vitest';
import { SettingsProvider } from '../contexts/SettingsContext';
import { NotificationProvider } from '../contexts/NotificationContext';
import { AuthProvider } from '../contexts/AuthContext';
import { DataProvider } from '../contexts/DataContext';

/**
 * All providers wrapper component.
 * Provider nesting order (outermost to innermost):
 * 1. SettingsProvider - No dependencies
 * 2. NotificationProvider - Depends on Settings
 * 3. AuthProvider - Depends on Settings
 * 4. DataProvider - Depends on Notification
 */
export const AllProviders = ({ children }) => {
  return (
    <SettingsProvider>
      <NotificationProvider>
        <AuthProvider>
          <DataProvider>
            {children}
          </DataProvider>
        </AuthProvider>
      </NotificationProvider>
    </SettingsProvider>
  );
};

/**
 * Minimal providers for components that only need Settings + Notification
 */
export const MinimalProviders = ({ children }) => {
  return (
    <SettingsProvider>
      <NotificationProvider>
        {children}
      </NotificationProvider>
    </SettingsProvider>
  );
};

/**
 * Auth providers for components that need authentication context
 */
export const AuthProviders = ({ children }) => {
  return (
    <SettingsProvider>
      <AuthProvider>
        {children}
      </AuthProvider>
    </SettingsProvider>
  );
};

/**
 * Custom render function that wraps components with all providers
 * @param {React.ReactElement} ui - Component to render
 * @param {object} options - Additional render options
 * @returns {object} Render result with all testing-library utilities
 */
export const renderWithProviders = (ui, options = {}) => {
  const { wrapper: Wrapper = AllProviders, ...renderOptions } = options;
  return render(ui, { wrapper: Wrapper, ...renderOptions });
};

/**
 * Custom render function with minimal providers (Settings + Notification only)
 */
export const renderWithMinimalProviders = (ui, options = {}) => {
  return render(ui, { wrapper: MinimalProviders, ...options });
};

/**
 * Custom render function with auth providers
 */
export const renderWithAuthProviders = (ui, options = {}) => {
  return render(ui, { wrapper: AuthProviders, ...options });
};

/**
 * Custom renderHook function that wraps hooks with all providers
 */
export const renderHookWithProviders = (hook, options = {}) => {
  const { wrapper: Wrapper = AllProviders, ...hookOptions } = options;
  return renderHook(hook, { wrapper: Wrapper, ...hookOptions });
};

/**
 * Setup mock fetch for tests
 * Returns the mock function for assertions
 */
export const setupMockFetch = () => {
  const mockFetch = vi.fn();
  global.fetch = mockFetch;
  return mockFetch;
};

/**
 * Create a mock API response
 */
export const createMockResponse = (data, options = {}) => {
  const { ok = true, status = 200 } = options;
  return {
    ok,
    status,
    json: async () => data,
    text: async () => JSON.stringify(data),
  };
};

/**
 * Mock successful API responses for DataProvider initialization
 */
export const mockDataProviderAPIs = (mockFetch) => {
  mockFetch.mockImplementation((url) => {
    if (url.includes('/api/v1/routes')) {
      return Promise.resolve(createMockResponse([]));
    }
    if (url.includes('/api/v1/discovery/proposals')) {
      return Promise.resolve(createMockResponse([]));
    }
    if (url.includes('/api/v1/admin/devices')) {
      return Promise.resolve(createMockResponse([]));
    }
    if (url.includes('/api/v1/activity/recent')) {
      return Promise.resolve(createMockResponse([]));
    }
    if (url.includes('/api/v1/stats')) {
      return Promise.resolve(createMockResponse({ requests: { value: 0 } }));
    }
    if (url.includes('/api/v1/containers')) {
      return Promise.resolve(createMockResponse([]));
    }
    if (url.includes('/api/v1/system/resources')) {
      return Promise.resolve(createMockResponse({
        cpu: 0,
        ram: 0,
        ram_used: 0,
        ram_total: 0,
        disk: 0,
        disk_used: 0,
        disk_total: 0,
        storage_size: 0,
      }));
    }
    // Default response
    return Promise.resolve(createMockResponse({}));
  });
};

// Re-export everything from @testing-library/react for convenience
export * from '@testing-library/react';
