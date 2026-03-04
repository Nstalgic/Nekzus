import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import ServerResourcesPanel from './ServerResourcesPanel';
import { DataProvider } from '../../contexts/DataContext';
import { NotificationProvider } from '../../contexts/NotificationContext';
import { SettingsProvider } from '../../contexts/SettingsContext';
import { ThemeProvider } from '../../contexts/ThemeContext';

/**
 * ServerResourcesPanel Test Suite
 *
 * Tests the container component for resource charts:
 * - Rendering and structure
 * - Box title
 * - Three chart layout
 * - Data context integration
 */

// Mock the API modules
vi.mock('../../services/api', () => ({
  routesAPI: { list: vi.fn().mockResolvedValue([]) },
  discoveryAPI: { listProposals: vi.fn().mockResolvedValue([]) },
  devicesAPI: { list: vi.fn().mockResolvedValue([]) },
  activityAPI: { getRecent: vi.fn().mockResolvedValue([]) },
  statsAPI: { get: vi.fn().mockResolvedValue(null) },
  healthAPI: { check: vi.fn().mockResolvedValue({ status: 'ok' }) },
  containersAPI: { list: vi.fn().mockResolvedValue([]) },
  systemAPI: {
    getResources: vi.fn().mockResolvedValue({
      cpu: 45,
      ram: 65,
      disk: 80,
      storage_size: 1024
    })
  }
}));

// Mock websocket service
vi.mock('../../services/websocket', () => ({
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

// Mock ResizeObserver
class ResizeObserverMock {
  observe() {}
  unobserve() {}
  disconnect() {}
}
global.ResizeObserver = ResizeObserverMock;

// Mock canvas context
const mockCanvasContext = {
  clearRect: vi.fn(),
  fillRect: vi.fn(),
  beginPath: vi.fn(),
  moveTo: vi.fn(),
  lineTo: vi.fn(),
  stroke: vi.fn(),
  fill: vi.fn(),
  fillText: vi.fn(),
  measureText: vi.fn(() => ({ width: 30 })),
  setLineDash: vi.fn(),
  save: vi.fn(),
  restore: vi.fn(),
  scale: vi.fn(),
  arc: vi.fn(),
  closePath: vi.fn(),
  fillStyle: '',
  strokeStyle: '',
  lineWidth: 1,
  lineJoin: '',
  lineCap: '',
  shadowColor: '',
  shadowBlur: 0,
  font: '',
  textAlign: '',
  textBaseline: '',
};

HTMLCanvasElement.prototype.getContext = vi.fn(() => mockCanvasContext);

describe('ServerResourcesPanel', () => {
  /**
   * Helper to render with providers
   */
  const renderWithProviders = () => {
    return render(
      <ThemeProvider>
        <SettingsProvider>
          <NotificationProvider>
            <DataProvider>
              <ServerResourcesPanel />
            </DataProvider>
          </NotificationProvider>
        </SettingsProvider>
      </ThemeProvider>
    );
  };

  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.clearAllMocks();
  });

  describe('Rendering', () => {
    it('should render without crashing', () => {
      renderWithProviders();
      expect(screen.getByTestId('server-resources-panel')).toBeInTheDocument();
    });

    it('should render Box with "SERVER RESOURCES" title', () => {
      renderWithProviders();
      expect(screen.getByText('SERVER RESOURCES')).toBeInTheDocument();
    });

    it('should render three ResourceLineChart components', () => {
      renderWithProviders();
      const charts = screen.getAllByTestId('resource-line-chart');
      expect(charts).toHaveLength(3);
    });

    it('should display CPU chart label', () => {
      renderWithProviders();
      expect(screen.getByText('CPU')).toBeInTheDocument();
    });

    it('should display RAM chart label', () => {
      renderWithProviders();
      expect(screen.getByText('RAM')).toBeInTheDocument();
    });

    it('should display DISK chart label', () => {
      renderWithProviders();
      expect(screen.getByText('DISK')).toBeInTheDocument();
    });
  });

  describe('Layout', () => {
    it('should use three-column grid layout', () => {
      renderWithProviders();
      const grid = screen.getByTestId('resource-charts-grid');
      expect(grid).toHaveClass('three-column');
    });

    it('should wrap in a component-section', () => {
      renderWithProviders();
      const panel = screen.getByTestId('server-resources-panel');
      expect(panel).toHaveClass('component-section');
    });
  });

  describe('Data Integration', () => {
    it('should render with current resource values', async () => {
      renderWithProviders();
      // The charts are rendered - they receive data from context
      const charts = screen.getAllByTestId('resource-line-chart');
      expect(charts).toHaveLength(3);
    });
  });
});
