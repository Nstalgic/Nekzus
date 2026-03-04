import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import ResourceLineChart from './ResourceLineChart';
import { ThemeProvider } from '../../contexts';

/**
 * ResourceLineChart Test Suite
 *
 * Tests the Canvas-based line chart component:
 * - Rendering and structure
 * - Canvas initialization
 * - Props handling
 * - Accessibility
 */

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

// Mock HTMLCanvasElement.getContext
HTMLCanvasElement.prototype.getContext = vi.fn(() => mockCanvasContext);

// Helper to render with ThemeProvider
const renderWithTheme = (ui) => {
  return render(<ThemeProvider>{ui}</ThemeProvider>);
};

describe('ResourceLineChart', () => {
  const defaultProps = {
    label: 'CPU',
    data: [10, 20, 30, 40, 50],
    timestamps: [1000, 2000, 3000, 4000, 5000],
    currentValue: 50,
  };

  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.clearAllMocks();
  });

  describe('Rendering', () => {
    it('should render without crashing', () => {
      renderWithTheme(<ResourceLineChart {...defaultProps} />);
      expect(screen.getByTestId('resource-line-chart')).toBeInTheDocument();
    });

    it('should render canvas element', () => {
      renderWithTheme(<ResourceLineChart {...defaultProps} />);
      const canvas = document.querySelector('canvas');
      expect(canvas).toBeInTheDocument();
    });

    it('should render label', () => {
      renderWithTheme(<ResourceLineChart {...defaultProps} />);
      expect(screen.getByText('CPU')).toBeInTheDocument();
    });

    it('should render current value with percentage', () => {
      renderWithTheme(<ResourceLineChart {...defaultProps} />);
      expect(screen.getByText('50%')).toBeInTheDocument();
    });

    it('should apply custom className', () => {
      renderWithTheme(<ResourceLineChart {...defaultProps} className="custom-class" />);
      const container = screen.getByTestId('resource-line-chart');
      expect(container).toHaveClass('custom-class');
    });

    it('should render with custom height', () => {
      renderWithTheme(<ResourceLineChart {...defaultProps} height={200} />);
      const canvas = document.querySelector('canvas');
      expect(canvas).toBeInTheDocument();
    });
  });

  describe('Props Handling', () => {
    it('should handle empty data gracefully', () => {
      renderWithTheme(<ResourceLineChart {...defaultProps} data={[]} timestamps={[]} />);
      expect(screen.getByTestId('resource-line-chart')).toBeInTheDocument();
    });

    it('should handle single data point', () => {
      renderWithTheme(<ResourceLineChart {...defaultProps} data={[50]} timestamps={[1000]} />);
      expect(screen.getByTestId('resource-line-chart')).toBeInTheDocument();
    });

    it('should handle undefined currentValue', () => {
      renderWithTheme(<ResourceLineChart label="CPU" data={[10, 20]} timestamps={[1000, 2000]} />);
      expect(screen.getByText('0%')).toBeInTheDocument();
    });

    it('should format large values correctly', () => {
      renderWithTheme(<ResourceLineChart {...defaultProps} currentValue={99.4} />);
      // Should round to nearest integer for display
      expect(screen.getByText(/99/)).toBeInTheDocument();
    });

    it('should accept different labels', () => {
      renderWithTheme(<ResourceLineChart {...defaultProps} label="RAM" />);
      expect(screen.getByText('RAM')).toBeInTheDocument();
    });

    it('should accept different labels for disk', () => {
      renderWithTheme(<ResourceLineChart {...defaultProps} label="DISK" />);
      expect(screen.getByText('DISK')).toBeInTheDocument();
    });
  });

  describe('Canvas Initialization', () => {
    it('should get 2d context on mount', () => {
      renderWithTheme(<ResourceLineChart {...defaultProps} />);
      expect(HTMLCanvasElement.prototype.getContext).toHaveBeenCalledWith('2d');
    });

    it('should call clearRect to clear canvas before drawing', () => {
      renderWithTheme(<ResourceLineChart {...defaultProps} />);
      expect(mockCanvasContext.clearRect).toHaveBeenCalled();
    });
  });

  describe('Accessibility', () => {
    it('should have aria-label on container', () => {
      renderWithTheme(<ResourceLineChart {...defaultProps} />);
      const container = screen.getByTestId('resource-line-chart');
      expect(container).toHaveAttribute('aria-label');
    });

    it('should include metric name in aria-label', () => {
      renderWithTheme(<ResourceLineChart {...defaultProps} />);
      const container = screen.getByTestId('resource-line-chart');
      expect(container.getAttribute('aria-label')).toContain('CPU');
    });

    it('should include current value in aria-label', () => {
      renderWithTheme(<ResourceLineChart {...defaultProps} />);
      const container = screen.getByTestId('resource-line-chart');
      expect(container.getAttribute('aria-label')).toContain('50');
    });

    it('should have role="img" for the chart', () => {
      renderWithTheme(<ResourceLineChart {...defaultProps} />);
      const canvas = document.querySelector('canvas');
      expect(canvas).toHaveAttribute('role', 'img');
    });
  });

  describe('Responsive Behavior', () => {
    it('should use ResizeObserver for responsive canvas', () => {
      const observeSpy = vi.spyOn(ResizeObserverMock.prototype, 'observe');
      renderWithTheme(<ResourceLineChart {...defaultProps} />);
      expect(observeSpy).toHaveBeenCalled();
    });

    it('should disconnect ResizeObserver on unmount', () => {
      const disconnectSpy = vi.spyOn(ResizeObserverMock.prototype, 'disconnect');
      const { unmount } = renderWithTheme(<ResourceLineChart {...defaultProps} />);
      unmount();
      expect(disconnectSpy).toHaveBeenCalled();
    });
  });
});
