import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { renderHook, act, waitFor } from '@testing-library/react';
import { DataProvider, useData } from './DataContext';
import { NotificationProvider } from './NotificationContext';
import { SettingsProvider } from './SettingsContext';

/**
 * DataContext Resource History Test Suite
 *
 * Tests the resource history accumulation functionality:
 * - Historical data collection for CPU, RAM, and Disk
 * - Rolling window of 180 data points (15 minutes at 5-second intervals)
 * - Timestamp synchronization across metrics
 */

// Mock the API modules
vi.mock('../services/api', () => ({
  routesAPI: { list: vi.fn().mockResolvedValue([]) },
  discoveryAPI: { listProposals: vi.fn().mockResolvedValue([]) },
  devicesAPI: { list: vi.fn().mockResolvedValue([]) },
  activityAPI: { getRecent: vi.fn().mockResolvedValue([]) },
  statsAPI: { get: vi.fn().mockResolvedValue(null) },
  healthAPI: { check: vi.fn().mockResolvedValue({ status: 'ok' }) },
  containersAPI: { list: vi.fn().mockResolvedValue([]) },
  systemAPI: {
    getResources: vi.fn().mockResolvedValue({
      cpu: 50,
      ram: 60,
      disk: 70,
      storage_size: 1024
    })
  }
}));

// Mock websocket service
vi.mock('../services/websocket', () => ({
  websocketService: {
    connect: vi.fn(),
    disconnect: vi.fn(),
    on: vi.fn(),
    onConnectionChange: null
  },
  WS_MSG_TYPES: {
    DISCOVERY: 'discovery',
    CONFIG_RELOAD: 'config_reload',
    DEVICE_PAIRED: 'device_paired',
    DEVICE_REVOKED: 'device_revoked',
    APP_REGISTERED: 'app_registered',
    PROPOSAL_DISMISSED: 'proposal_dismissed',
    HEALTH_CHANGE: 'health_change',
    WEBHOOK: 'webhook'
  }
}));

describe('DataContext - Resource History', () => {
  /**
   * Helper to render useData hook with required providers
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

  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.clearAllMocks();
  });

  describe('Initialization', () => {
    it('should initialize resourceHistory with empty arrays', () => {
      const { result } = renderDataHook();

      expect(result.current.resourceHistory).toBeDefined();
      expect(result.current.resourceHistory.cpu).toEqual([]);
      expect(result.current.resourceHistory.ram).toEqual([]);
      expect(result.current.resourceHistory.disk).toEqual([]);
      expect(result.current.resourceHistory.timestamps).toEqual([]);
    });

    it('should expose resourceHistory in context value', () => {
      const { result } = renderDataHook();

      expect(typeof result.current.resourceHistory).toBe('object');
      expect(Array.isArray(result.current.resourceHistory.cpu)).toBe(true);
      expect(Array.isArray(result.current.resourceHistory.ram)).toBe(true);
      expect(Array.isArray(result.current.resourceHistory.disk)).toBe(true);
      expect(Array.isArray(result.current.resourceHistory.timestamps)).toBe(true);
    });
  });

  describe('History Accumulation', () => {
    it('should accumulate CPU history on refresh', async () => {
      const { systemAPI } = await import('../services/api');
      systemAPI.getResources.mockResolvedValue({
        cpu: 45,
        ram: 55,
        disk: 65,
        storage_size: 1024
      });

      const { result } = renderDataHook();

      // Manually trigger refresh
      await act(async () => {
        await result.current.refreshSystemResources();
      });

      expect(result.current.resourceHistory.cpu.length).toBeGreaterThan(0);
      expect(result.current.resourceHistory.cpu).toContain(45);
    });

    it('should accumulate RAM history on refresh', async () => {
      const { systemAPI } = await import('../services/api');
      systemAPI.getResources.mockResolvedValue({
        cpu: 45,
        ram: 55,
        disk: 65,
        storage_size: 1024
      });

      const { result } = renderDataHook();

      await act(async () => {
        await result.current.refreshSystemResources();
      });

      expect(result.current.resourceHistory.ram.length).toBeGreaterThan(0);
      expect(result.current.resourceHistory.ram).toContain(55);
    });

    it('should accumulate DISK history on refresh', async () => {
      const { systemAPI } = await import('../services/api');
      systemAPI.getResources.mockResolvedValue({
        cpu: 45,
        ram: 55,
        disk: 65,
        storage_size: 1024
      });

      const { result } = renderDataHook();

      await act(async () => {
        await result.current.refreshSystemResources();
      });

      expect(result.current.resourceHistory.disk.length).toBeGreaterThan(0);
      expect(result.current.resourceHistory.disk).toContain(65);
    });

    it('should store corresponding timestamps', async () => {
      const { systemAPI } = await import('../services/api');
      systemAPI.getResources.mockResolvedValue({
        cpu: 45,
        ram: 55,
        disk: 65,
        storage_size: 1024
      });

      const { result } = renderDataHook();

      await act(async () => {
        await result.current.refreshSystemResources();
      });

      expect(result.current.resourceHistory.timestamps.length).toBeGreaterThan(0);
      // Timestamps should be numbers (Unix ms)
      expect(typeof result.current.resourceHistory.timestamps[0]).toBe('number');
    });

    it('should keep all arrays synchronized in length', async () => {
      const { systemAPI } = await import('../services/api');
      systemAPI.getResources.mockResolvedValue({
        cpu: 45,
        ram: 55,
        disk: 65,
        storage_size: 1024
      });

      const { result } = renderDataHook();

      // Call refresh multiple times
      await act(async () => {
        await result.current.refreshSystemResources();
        await result.current.refreshSystemResources();
        await result.current.refreshSystemResources();
      });

      const { cpu, ram, disk, timestamps } = result.current.resourceHistory;
      expect(cpu.length).toBe(ram.length);
      expect(ram.length).toBe(disk.length);
      expect(disk.length).toBe(timestamps.length);
    });
  });

  describe('Rolling Window Limit', () => {
    it('should limit history to 180 points (15 minutes)', async () => {
      const { systemAPI } = await import('../services/api');
      let callCount = 0;
      systemAPI.getResources.mockImplementation(() => {
        callCount++;
        return Promise.resolve({
          cpu: callCount,
          ram: callCount + 10,
          disk: callCount + 20,
          storage_size: 1024
        });
      });

      const { result } = renderDataHook();

      // Simulate 200 refresh calls (more than 180 limit)
      await act(async () => {
        for (let i = 0; i < 200; i++) {
          await result.current.refreshSystemResources();
        }
      });

      // Should be capped at 180
      expect(result.current.resourceHistory.cpu.length).toBeLessThanOrEqual(180);
      expect(result.current.resourceHistory.ram.length).toBeLessThanOrEqual(180);
      expect(result.current.resourceHistory.disk.length).toBeLessThanOrEqual(180);
      expect(result.current.resourceHistory.timestamps.length).toBeLessThanOrEqual(180);
    });

    it('should trim oldest points when exceeding limit', async () => {
      const { systemAPI } = await import('../services/api');
      let callCount = 0;
      systemAPI.getResources.mockImplementation(() => {
        callCount++;
        return Promise.resolve({
          cpu: callCount,
          ram: callCount + 10,
          disk: callCount + 20,
          storage_size: 1024
        });
      });

      const { result } = renderDataHook();

      // Simulate 200 refresh calls
      await act(async () => {
        for (let i = 0; i < 200; i++) {
          await result.current.refreshSystemResources();
        }
      });

      // First values should have been trimmed (oldest removed)
      const cpuHistory = result.current.resourceHistory.cpu;
      // Should not contain the very first value (1) - oldest points removed
      expect(cpuHistory[0]).toBeGreaterThan(20); // First 20+ values should be trimmed
    });
  });

  describe('Data Integration with systemResources', () => {
    it('should maintain systemResources for current values', async () => {
      const { systemAPI } = await import('../services/api');
      systemAPI.getResources.mockResolvedValue({
        cpu: 75,
        ram: 85,
        disk: 95,
        storage_size: 2048
      });

      const { result } = renderDataHook();

      await act(async () => {
        await result.current.refreshSystemResources();
      });

      expect(result.current.systemResources.cpu).toBe(75);
      expect(result.current.systemResources.ram).toBe(85);
      expect(result.current.systemResources.disk).toBe(95);
      expect(result.current.systemResources.storage_size).toBe(2048);
    });
  });
});
