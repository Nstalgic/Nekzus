import { useRef, useEffect, useCallback } from 'react';
import PropTypes from 'prop-types';
import { useTheme } from '../../contexts';

/**
 * ResourceLineChart - Canvas-based retro line chart for resource monitoring
 *
 * Renders a line graph with phosphor green aesthetics matching the terminal UI.
 * Uses Canvas for efficient rendering and supports responsive sizing.
 *
 * @param {Object} props
 * @param {string} props.label - Chart title (e.g., "CPU", "RAM", "DISK")
 * @param {number[]} props.data - Array of values (0-100)
 * @param {number[]} props.timestamps - Array of timestamps for x-axis
 * @param {number} [props.currentValue=0] - Current value to display prominently
 * @param {number} [props.height=100] - Chart height in pixels
 * @param {string} [props.className=''] - Additional CSS classes
 */
function ResourceLineChart({
  label,
  data = [],
  timestamps = [],
  currentValue = 0,
  height = 100,
  className = ''
}) {
  const canvasRef = useRef(null);
  const containerRef = useRef(null);
  const { theme } = useTheme();

  /**
   * Get CSS variable values for theming
   * Reads from document.body since themes are applied via body.theme-{name} classes
   */
  const getColors = useCallback(() => {
    const styles = getComputedStyle(document.body);
    return {
      line: styles.getPropertyValue('--text-primary').trim() || '#33ff66',
      grid: styles.getPropertyValue('--border-dim').trim() || '#1f9940',
      bg: styles.getPropertyValue('--bg-secondary').trim() || '#0d2410',
      text: styles.getPropertyValue('--text-secondary').trim() || '#29cc52',
    };
  }, []);

  /**
   * Draw the chart on canvas
   */
  const draw = useCallback(() => {
    const canvas = canvasRef.current;
    const container = containerRef.current;
    if (!canvas || !container) return;

    const ctx = canvas.getContext('2d');
    if (!ctx) return;

    // Get actual container dimensions
    const rect = container.getBoundingClientRect();
    const dpr = window.devicePixelRatio || 1;
    const width = rect.width;
    const chartHeight = height;

    // Set canvas size with device pixel ratio for crisp rendering
    canvas.width = width * dpr;
    canvas.height = chartHeight * dpr;
    canvas.style.width = `${width}px`;
    canvas.style.height = `${chartHeight}px`;

    // Scale context for retina displays
    ctx.scale(dpr, dpr);

    const colors = getColors();
    const padding = { top: 8, right: 8, bottom: 20, left: 32 };
    const graphWidth = width - padding.left - padding.right;
    const graphHeight = chartHeight - padding.top - padding.bottom;

    // Clear canvas
    ctx.clearRect(0, 0, width, chartHeight);

    // Fill background
    ctx.fillStyle = colors.bg;
    ctx.fillRect(0, 0, width, chartHeight);

    // Draw grid lines (horizontal at 0%, 25%, 50%, 75%, 100%)
    ctx.strokeStyle = colors.grid;
    ctx.lineWidth = 1;
    ctx.setLineDash([2, 4]);

    const gridLevels = [0, 0.25, 0.5, 0.75, 1];
    gridLevels.forEach(pct => {
      const y = padding.top + (1 - pct) * graphHeight;
      ctx.beginPath();
      ctx.moveTo(padding.left, y);
      ctx.lineTo(width - padding.right, y);
      ctx.stroke();
    });

    ctx.setLineDash([]);

    // Draw Y-axis labels
    ctx.fillStyle = colors.text;
    ctx.font = '10px monospace';
    ctx.textAlign = 'right';
    ctx.textBaseline = 'middle';

    [0, 50, 100].forEach(val => {
      const y = padding.top + (1 - val / 100) * graphHeight;
      ctx.fillText(`${val}`, padding.left - 4, y);
    });

    // Draw data line
    if (data.length >= 2) {
      ctx.beginPath();
      ctx.strokeStyle = colors.line;
      ctx.lineWidth = 2;
      ctx.lineJoin = 'round';
      ctx.lineCap = 'round';

      // Add glow effect
      ctx.shadowColor = colors.line;
      ctx.shadowBlur = 6;

      data.forEach((value, i) => {
        const x = padding.left + (i / (data.length - 1)) * graphWidth;
        const y = padding.top + (1 - Math.min(100, Math.max(0, value)) / 100) * graphHeight;

        if (i === 0) {
          ctx.moveTo(x, y);
        } else {
          ctx.lineTo(x, y);
        }
      });

      ctx.stroke();

      // Draw filled area under the line (subtle)
      ctx.beginPath();
      ctx.shadowBlur = 0;

      data.forEach((value, i) => {
        const x = padding.left + (i / (data.length - 1)) * graphWidth;
        const y = padding.top + (1 - Math.min(100, Math.max(0, value)) / 100) * graphHeight;

        if (i === 0) {
          ctx.moveTo(x, y);
        } else {
          ctx.lineTo(x, y);
        }
      });

      // Close the path to bottom
      ctx.lineTo(padding.left + graphWidth, padding.top + graphHeight);
      ctx.lineTo(padding.left, padding.top + graphHeight);
      ctx.closePath();

      // Subtle fill
      ctx.fillStyle = `${colors.line}15`; // Very transparent
      ctx.fill();

      ctx.shadowBlur = 0;
    } else if (data.length === 1) {
      // Single point - draw a dot
      const x = padding.left + graphWidth / 2;
      const y = padding.top + (1 - Math.min(100, Math.max(0, data[0])) / 100) * graphHeight;

      ctx.beginPath();
      ctx.arc(x, y, 3, 0, Math.PI * 2);
      ctx.fillStyle = colors.line;
      ctx.shadowColor = colors.line;
      ctx.shadowBlur = 6;
      ctx.fill();
      ctx.shadowBlur = 0;
    }

    // Draw time axis labels (start and end)
    if (timestamps.length >= 2) {
      ctx.fillStyle = colors.text;
      ctx.font = '9px monospace';
      ctx.textAlign = 'left';
      ctx.textBaseline = 'top';

      const formatTime = (ts) => {
        const date = new Date(ts);
        const mins = date.getMinutes().toString().padStart(2, '0');
        const secs = date.getSeconds().toString().padStart(2, '0');
        return `${mins}:${secs}`;
      };

      // Start time
      ctx.textAlign = 'left';
      ctx.fillText(formatTime(timestamps[0]), padding.left, chartHeight - 12);

      // End time
      ctx.textAlign = 'right';
      ctx.fillText(formatTime(timestamps[timestamps.length - 1]), width - padding.right, chartHeight - 12);
    }
  }, [data, timestamps, height, getColors, theme]);

  /**
   * Setup ResizeObserver for responsive canvas
   */
  useEffect(() => {
    const container = containerRef.current;
    if (!container) return;

    const resizeObserver = new ResizeObserver(() => {
      draw();
    });

    resizeObserver.observe(container);

    // Initial draw
    draw();

    return () => {
      resizeObserver.disconnect();
    };
  }, [draw]);

  /**
   * Redraw when data changes
   */
  useEffect(() => {
    draw();
  }, [data, timestamps, draw]);

  /**
   * Redraw when theme changes (with slight delay for CSS to apply)
   */
  useEffect(() => {
    // Small delay to ensure CSS variables are updated after theme class change
    const timeoutId = setTimeout(() => {
      draw();
    }, 50);
    return () => clearTimeout(timeoutId);
  }, [theme, draw]);

  const displayValue = Math.round(currentValue || 0);
  const ariaLabel = `${label} usage chart showing ${displayValue}% current utilization`;

  return (
    <div
      className={`resource-chart ${className}`}
      data-testid="resource-line-chart"
      aria-label={ariaLabel}
    >
      <div className="resource-chart-header">
        <span className="resource-chart-label">{label}</span>
        <span className="resource-chart-value">{displayValue}%</span>
      </div>
      <div className="resource-chart-canvas-container" ref={containerRef}>
        <canvas
          ref={canvasRef}
          className="resource-chart-canvas"
          role="img"
          aria-label={`Line chart for ${label}`}
        />
      </div>
    </div>
  );
}

ResourceLineChart.propTypes = {
  label: PropTypes.string.isRequired,
  data: PropTypes.arrayOf(PropTypes.number),
  timestamps: PropTypes.arrayOf(PropTypes.number),
  currentValue: PropTypes.number,
  height: PropTypes.number,
  className: PropTypes.string,
};

export default ResourceLineChart;
