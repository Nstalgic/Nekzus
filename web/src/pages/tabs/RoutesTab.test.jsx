import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { SettingsProvider } from '../../contexts/SettingsContext';
import { NotificationProvider } from '../../contexts/NotificationContext';
import { DataProvider } from '../../contexts/DataContext';
import { RoutesTab } from './RoutesTab';

/**
 * RoutesTab Test Suite
 *
 * Tests basic rendering and interaction of the Routes management interface.
 * Note: Full integration tests require mocking WebSocket and extensive API responses.
 */

// Mock routes data matching the actual API response structure
const mockRoutes = [
  { routeId: '1', appId: 'grafana', pathBase: '/grafana', to: 'http://localhost:3000', scopes: ['READ', 'WRITE'], status: 'ACTIVE' },
  { routeId: '2', appId: 'prometheus', pathBase: '/prometheus', to: 'http://localhost:9090', scopes: ['READ'], status: 'ACTIVE' },
];

const createMockResponse = (data) => ({
  ok: true,
  status: 200,
  json: async () => data,
});

describe('RoutesTab', () => {
  beforeEach(() => {
    vi.clearAllMocks();
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
        return Promise.resolve(createMockResponse([]));
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

  const renderRoutesTab = () => {
    return render(
      <SettingsProvider>
        <NotificationProvider>
          <DataProvider>
            <RoutesTab />
          </DataProvider>
        </NotificationProvider>
      </SettingsProvider>
    );
  };

  describe('Rendering', () => {
    it('should render the Add Route button', async () => {
      renderRoutesTab();

      await waitFor(() => {
        expect(screen.getByRole('button', { name: /add route/i })).toBeInTheDocument();
      });
    });

    it('should render table headers', async () => {
      renderRoutesTab();

      await waitFor(() => {
        expect(screen.getByText('Application')).toBeInTheDocument();
      });

      expect(screen.getByText('Path')).toBeInTheDocument();
      expect(screen.getByText('Target')).toBeInTheDocument();
    });
  });

  describe('Add Route Modal', () => {
    it('should open add route modal when Add Route button is clicked', async () => {
      const user = userEvent.setup();
      renderRoutesTab();

      await waitFor(() => {
        expect(screen.getByRole('button', { name: /add route/i })).toBeInTheDocument();
      });

      await user.click(screen.getByRole('button', { name: /add route/i }));

      // Modal should open - look for modal-specific content
      await waitFor(() => {
        expect(screen.getByText(/APPLICATION/)).toBeInTheDocument();
      });
    });
  });
});
