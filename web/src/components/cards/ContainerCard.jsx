/**
 * ContainerCard Component
 *
 * Card component for displaying containers with management controls
 * Supports both Docker containers and Kubernetes pods
 *
 * Features:
 * - Container status badge (running/stopped/paused)
 * - Runtime badge (Docker/Kubernetes)
 * - Namespace display for Kubernetes pods
 * - Container name, image, and ID
 * - Resource metrics (CPU, Memory, Network I/O)
 * - Port mappings display
 * - Management actions (Start/Stop, Restart, Inspect, Logs)
 */

import { useState, useMemo } from 'react';
import PropTypes from 'prop-types';
import { Container, Activity, Cpu, HardDrive, Network } from 'lucide-react';
import { Badge } from '../data-display';

/**
 * Format bytes to human-readable size
 */
const formatBytes = (bytes) => {
  if (!bytes || bytes === 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return `${(bytes / Math.pow(k, i)).toFixed(1)} ${sizes[i]}`;
};

/**
 * Format port mappings for display
 */
const formatPorts = (ports) => {
  if (!ports || ports.length === 0) return 'No exposed ports';

  return ports
    .map((port) => {
      if (port.publicPort) {
        return `${port.ip || '0.0.0.0'}:${port.publicPort}→${port.privatePort}/${port.type}`;
      }
      return `${port.privatePort}/${port.type}`;
    })
    .join(', ');
};

/**
 * Truncate container ID for display
 */
const truncateId = (id) => {
  return id ? id.substring(0, 12) : 'Unknown';
};

/**
 * ContainerCard Component
 *
 * @param {object} props - Component props
 * @param {object} props.container - Container object
 * @param {string} props.container.id - Container ID
 * @param {string} props.container.name - Container name
 * @param {string} props.container.image - Container image
 * @param {string} props.container.state - Container state (running/stopped/paused)
 * @param {string} props.container.status - Container status text
 * @param {array} props.container.ports - Port mappings
 * @param {string} props.container.runtime - Container runtime (docker/kubernetes)
 * @param {string} props.container.namespace - Kubernetes namespace (if applicable)
 * @param {object} props.stats - Container statistics
 * @param {number} props.stats.cpu_percent - CPU usage percentage
 * @param {number} props.stats.memory_usage - Memory usage in bytes
 * @param {number} props.stats.network_rx - Network received in bytes
 * @param {number} props.stats.network_tx - Network transmitted in bytes
 * @param {function} props.onStart - Start container callback
 * @param {function} props.onStop - Stop container callback
 * @param {function} props.onRestart - Restart container callback
 * @param {function} props.onInspect - Inspect container callback
 * @param {function} props.onLogs - View logs callback
 * @param {function} props.onExport - Export container config callback
 * @param {boolean} props.selectable - Whether the card is selectable
 * @param {boolean} props.selected - Whether the card is selected
 * @param {function} props.onSelect - Callback when selection changes
 */
export function ContainerCard({
  container,
  stats = null,
  onStart,
  onStop,
  onRestart,
  onInspect,
  onLogs,
  onExport,
  selectable = false,
  selected = false,
  onSelect
}) {
  const [isProcessing, setIsProcessing] = useState(false);

  const isRunning = container.state === 'running';
  const isStopped = container.state === 'exited' || container.state === 'stopped';
  const isPaused = container.state === 'paused';
  const isKubernetes = container.runtime === 'kubernetes';

  // Get status badge variant
  const getStatusVariant = () => {
    if (isRunning) return 'success';
    if (isPaused) return 'warning';
    return 'default';
  };

  // Get runtime badge variant
  const getRuntimeVariant = () => {
    return isKubernetes ? 'info' : 'secondary';
  };

  // Get runtime display name
  const runtimeDisplay = isKubernetes ? 'K8s' : 'Docker';

  // Format container name (remove leading slash if present)
  const displayName = useMemo(() => {
    return container.name?.replace(/^\//, '') || 'Unnamed Container';
  }, [container.name]);

  // Format ports display
  const portsDisplay = useMemo(() => {
    return formatPorts(container.ports);
  }, [container.ports]);

  // Handle action buttons
  const handleAction = async (action, callback) => {
    setIsProcessing(true);
    try {
      await callback(container);
    } finally {
      setIsProcessing(false);
    }
  };

  // Handle selection toggle
  const handleSelectionChange = () => {
    if (selectable && onSelect) {
      onSelect(container, !selected);
    }
  };

  return (
    <div className={`container-card ${selected ? 'container-card-selected' : ''}`}>
      {/* Header: Status Badge, Runtime Badge, and Selection */}
      <div className="container-card-header">
        {selectable && (
          <label className="container-select-checkbox" onClick={(e) => e.stopPropagation()}>
            <input
              type="checkbox"
              className="checkbox"
              checked={selected}
              onChange={handleSelectionChange}
              aria-label={`Select ${displayName}`}
            />
          </label>
        )}
        <div className="container-badges">
          <Badge
            variant={getStatusVariant()}
            size="sm"
            dot={true}
            filled={true}
            role="status"
            className="container-status-badge"
          >
            <span className="sr-only">Status: </span>
            {container.state?.toUpperCase() || 'UNKNOWN'}
          </Badge>
          {container.runtime && (
            <Badge
              variant={getRuntimeVariant()}
              size="sm"
              filled={false}
              className="container-runtime-badge"
            >
              {runtimeDisplay}
            </Badge>
          )}
        </div>
      </div>

      {/* Body: Container Info */}
      <div className="container-card-body">
        <div className="container-card-title">
          <div className="container-icon">
            <Container size={24} strokeWidth={2} />
          </div>
          <div>
            <h3 className="container-name">{displayName}</h3>
            <p className="container-image">{container.image || 'Unknown image'}</p>
          </div>
        </div>

        <div className="container-info">
          <div className="container-info-item">
            <span className="container-info-label">ID:</span>
            <span className="container-info-value">
              <code>{truncateId(container.id)}</code>
            </span>
          </div>
          {isKubernetes && container.namespace && (
            <div className="container-info-item">
              <span className="container-info-label">Namespace:</span>
              <span className="container-info-value">
                <code>{container.namespace}</code>
              </span>
            </div>
          )}
          <div className="container-info-item">
            <span className="container-info-label">Status:</span>
            <span className="container-info-value">{container.status || 'N/A'}</span>
          </div>
          <div className="container-info-item">
            <span className="container-info-label">Ports:</span>
            <span className="container-info-value">
              <code className="container-ports">{portsDisplay}</code>
            </span>
          </div>
        </div>

        {/* Resource Metrics (if available and container is running) */}
        {stats && isRunning && (
          <div className="container-metrics">
            <div className="metric-item">
              <Cpu size={14} className="metric-icon" />
              <span className="metric-label">CPU:</span>
              <span className="metric-value">{stats.cpu?.usage?.toFixed(1) || '0.0'}%</span>
            </div>
            <div className="metric-item">
              <HardDrive size={14} className="metric-icon" />
              <span className="metric-label">MEM:</span>
              <span className="metric-value">{formatBytes(stats.memory?.used)}</span>
            </div>
            <div className="metric-item">
              <Network size={14} className="metric-icon" />
              <span className="metric-label">NET:</span>
              <span className="metric-value">
                ↑{formatBytes(stats.network?.tx)} ↓{formatBytes(stats.network?.rx)}
              </span>
            </div>
          </div>
        )}
      </div>

      {/* Actions: Primary and Secondary Controls */}
      <div className="container-card-actions">
        <div className="container-actions-primary">
          <button
            type="button"
            className="btn btn-secondary btn-sm"
            onClick={() => handleAction('inspect', onInspect)}
            disabled={isProcessing}
            aria-label={`Inspect ${displayName}`}
          >
            INSPECT
          </button>
          <button
            type="button"
            className="btn btn-secondary btn-sm"
            onClick={() => handleAction('logs', onLogs)}
            disabled={isProcessing}
            aria-label={`View logs for ${displayName}`}
          >
            LOGS
          </button>
          <button
            type="button"
            className="btn btn-secondary btn-sm"
            onClick={() => handleAction('export', onExport)}
            disabled={isProcessing}
            aria-label={`Export ${displayName} configuration`}
          >
            EXPORT
          </button>
        </div>
        <div className="container-actions-secondary">
          {isRunning && (
            <>
              <button
                type="button"
                className="btn btn-warning btn-sm"
                onClick={() => handleAction('stop', onStop)}
                disabled={isProcessing}
                aria-label={`Stop ${displayName}`}
              >
                STOP
              </button>
              <button
                type="button"
                className="btn btn-primary btn-sm"
                onClick={() => handleAction('restart', onRestart)}
                disabled={isProcessing}
                aria-label={`Restart ${displayName}`}
              >
                RESTART
              </button>
            </>
          )}
          {isStopped && (
            <button
              type="button"
              className="btn btn-success btn-sm"
              onClick={() => handleAction('start', onStart)}
              disabled={isProcessing}
              aria-label={`Start ${displayName}`}
            >
              START
            </button>
          )}
          {isPaused && (
            <button
              type="button"
              className="btn btn-success btn-sm"
              onClick={() => handleAction('start', onStart)}
              disabled={isProcessing}
              aria-label={`Unpause ${displayName}`}
            >
              UNPAUSE
            </button>
          )}
        </div>
      </div>
    </div>
  );
}

ContainerCard.propTypes = {
  container: PropTypes.shape({
    id: PropTypes.string.isRequired,
    name: PropTypes.string,
    image: PropTypes.string,
    state: PropTypes.string.isRequired,
    status: PropTypes.string,
    ports: PropTypes.arrayOf(
      PropTypes.shape({
        ip: PropTypes.string,
        privatePort: PropTypes.number.isRequired,
        publicPort: PropTypes.number,
        type: PropTypes.string.isRequired
      })
    ),
    runtime: PropTypes.oneOf(['docker', 'kubernetes']),
    namespace: PropTypes.string
  }).isRequired,
  stats: PropTypes.shape({
    cpu_percent: PropTypes.number,
    memory_usage: PropTypes.number,
    memory_limit: PropTypes.number,
    memory_percent: PropTypes.number,
    network_rx: PropTypes.number,
    network_tx: PropTypes.number
  }),
  onStart: PropTypes.func.isRequired,
  onStop: PropTypes.func.isRequired,
  onRestart: PropTypes.func.isRequired,
  onInspect: PropTypes.func.isRequired,
  onLogs: PropTypes.func.isRequired,
  onExport: PropTypes.func.isRequired,
  selectable: PropTypes.bool,
  selected: PropTypes.bool,
  onSelect: PropTypes.func
};
