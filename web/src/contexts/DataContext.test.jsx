import { describe, it, expect, beforeEach, vi } from 'vitest';
import { renderHook, act, waitFor } from '@testing-library/react';
import { DataProvider, useData } from './DataContext';
import { NotificationProvider } from './NotificationContext';
import { SettingsProvider } from './SettingsContext';

/**
 * DataContext Test Suite
 *
 * Tests the data management context including:
 * - Provider initialization
 * - Data fetching from API
 * - Route operations (update, delete, get, search)
 * - Activity logging
 */

// Mock initial data - using actual API field names
const mockRoutes = [
  { routeId: '1', appId: 'App1', pathBase: '/app1', to: 'http://app1:8080', scopes: ['READ'], status: 'ACTIVE' },
  { routeId: '2', appId: 'App2', pathBase: '/app2', to: 'http://app2:8080', scopes: ['READ', 'WRITE'], status: 'ACTIVE' },
];

const mockActivities = [
  { id: 'a1', message: 'Route added', timestamp: Date.now(), type: 'route_added' },
];

// Mock fetch responses
const createMockResponse = (data) => ({
  ok: true,
  status: 200,
  json: async () => data,
});

describe('DataContext', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    // Mock fetch to return appropriate data for each endpoint
    global.fetch = vi.fn((url) => {
      if (url.includes('/api/v1/routes')) {
        return Promise.resolve(createMockResponse(mockRoutes));
      }
      if (url.includes('/api/v1/discovery/proposals')) {
        return Promise.resolve(createMockResponse([]));
      }
      if (url.includes('/api/v1/admin/devices')) {
        return Promise.resolve(createMockResponse([]));
      }
      if (url.includes('/api/v1/activity/recent')) {
        return Promise.resolve(createMockResponse(mockActivities));
      }
      if (url.includes('/api/v1/stats')) {
        return Promise.resolve(createMockResponse({ requests: { value: 100 } }));
      }
      if (url.includes('/api/v1/containers')) {
        return Promise.resolve(createMockResponse([]));
      }
      if (url.includes('/api/v1/system/resources')) {
        return Promise.resolve(createMockResponse({
          cpu: 10, ram: 50, ram_used: 8000000000, ram_total: 16000000000,
          disk: 60, disk_used: 300000000000, disk_total: 500000000000, storage_size: 1024000
        }));
      }
      return Promise.resolve(createMockResponse({}));
    });
  });

  /**
   * Helper function to render the useData hook wrapped in DataProvider
   */
  const renderDataHook = () => {
    return renderHook(() => useData(), {
      wrapper: ({ children }) => (
        <SettingsProvider>
          <NotificationProvider>
            <DataProvider>{children}</DataProvider>
          </NotificationProvider>
        </SettingsProvider>
      )
    });
  };

  describe('Provider Initialization', () => {
    it('should provide initial routes data after loading', async () => {
      const { result } = renderDataHook();

      await waitFor(() => {
        expect(result.current.isLoading).toBe(false);
      });

      expect(result.current.routes).toBeDefined();
      expect(Array.isArray(result.current.routes)).toBe(true);
    });

    it('should provide initial activities data after loading', async () => {
      const { result } = renderDataHook();

      await waitFor(() => {
        expect(result.current.isLoading).toBe(false);
      });

      expect(result.current.activities).toBeDefined();
      expect(Array.isArray(result.current.activities)).toBe(true);
    });

    it('should provide route operation methods', async () => {
      const { result } = renderDataHook();

      await waitFor(() => {
        expect(result.current.isLoading).toBe(false);
      });

      expect(typeof result.current.updateRoute).toBe('function');
      expect(typeof result.current.deleteRoute).toBe('function');
      expect(typeof result.current.getRoute).toBe('function');
      expect(typeof result.current.searchRoutes).toBe('function');
    });

    it('should provide refresh functions', async () => {
      const { result } = renderDataHook();

      await waitFor(() => {
        expect(result.current.isLoading).toBe(false);
      });

      expect(typeof result.current.refreshRoutes).toBe('function');
      expect(typeof result.current.refreshDiscoveries).toBe('function');
      expect(typeof result.current.refreshDevices).toBe('function');
      expect(typeof result.current.refreshActivities).toBe('function');
    });

    it('should provide system resources data', async () => {
      const { result } = renderDataHook();

      await waitFor(() => {
        expect(result.current.isLoading).toBe(false);
      });

      expect(result.current.systemResources).toBeDefined();
      expect(typeof result.current.systemResources.cpu).toBe('number');
    });
  });

  describe('getRoute', () => {
    it('should return a route by ID', async () => {
      const { result } = renderDataHook();

      await waitFor(() => {
        expect(result.current.routes.length).toBeGreaterThan(0);
      });

      const foundRoute = result.current.getRoute('1');

      expect(foundRoute).toBeDefined();
      expect(foundRoute.routeId).toBe('1');
      expect(foundRoute.appId).toBe('App1');
    });

    it('should return undefined for non-existent ID', async () => {
      const { result } = renderDataHook();

      await waitFor(() => {
        expect(result.current.routes.length).toBeGreaterThan(0);
      });

      const foundRoute = result.current.getRoute('non-existent-id');

      expect(foundRoute).toBeUndefined();
    });
  });

  describe('searchRoutes', () => {
    it('should return all routes when search is empty', async () => {
      const { result } = renderDataHook();

      await waitFor(() => {
        expect(result.current.routes.length).toBeGreaterThan(0);
      });

      const totalRoutes = result.current.routes.length;
      const results = result.current.searchRoutes('');

      expect(results.length).toBe(totalRoutes);
    });

    it('should filter routes by appId', async () => {
      const { result } = renderDataHook();

      await waitFor(() => {
        expect(result.current.routes.length).toBeGreaterThan(0);
      });

      const results = result.current.searchRoutes('App1');

      expect(results.length).toBeGreaterThan(0);
      expect(results.every(r => r.appId.includes('App1'))).toBe(true);
    });

    it('should filter routes by pathBase', async () => {
      const { result } = renderDataHook();

      await waitFor(() => {
        expect(result.current.routes.length).toBeGreaterThan(0);
      });

      const results = result.current.searchRoutes('/app1');

      expect(results.length).toBeGreaterThan(0);
      expect(results.some(r => r.pathBase.includes('/app1'))).toBe(true);
    });

    it('should be case-insensitive', async () => {
      const { result } = renderDataHook();

      await waitFor(() => {
        expect(result.current.routes.length).toBeGreaterThan(0);
      });

      const resultsLower = result.current.searchRoutes('app1');
      const resultsUpper = result.current.searchRoutes('APP1');

      expect(resultsLower.length).toBeGreaterThan(0);
      expect(resultsUpper.length).toBeGreaterThan(0);
      expect(resultsLower.length).toBe(resultsUpper.length);
    });
  });

  describe('getRouteStats', () => {
    it('should return route statistics', async () => {
      const { result } = renderDataHook();

      await waitFor(() => {
        expect(result.current.routes.length).toBeGreaterThan(0);
      });

      const stats = result.current.getRouteStats();

      expect(stats).toBeDefined();
      expect(typeof stats.total).toBe('number');
      expect(typeof stats.active).toBe('number');
      expect(stats.byScope).toBeDefined();
    });
  });

  describe('activities', () => {
    it('should provide activity helper functions', async () => {
      const { result } = renderDataHook();

      await waitFor(() => {
        expect(result.current.isLoading).toBe(false);
      });

      expect(typeof result.current.getRecentActivities).toBe('function');
      expect(typeof result.current.getActivitiesByType).toBe('function');
      expect(typeof result.current.formatActivityTime).toBe('function');
    });

    it('should format activity time correctly', async () => {
      const { result } = renderDataHook();

      await waitFor(() => {
        expect(result.current.isLoading).toBe(false);
      });

      const now = Date.now();
      const formatted = result.current.formatActivityTime(now);

      expect(typeof formatted).toBe('string');
      expect(formatted).toMatch(/ago|just now|now/i);
    });
  });

  describe('error handling', () => {
    it('should handle fetch errors gracefully', async () => {
      global.fetch = vi.fn(() => Promise.reject(new Error('Network error')));

      const { result } = renderDataHook();

      // Should not throw
      await waitFor(() => {
        expect(result.current.routes).toBeDefined();
      });
    });
  });
});
