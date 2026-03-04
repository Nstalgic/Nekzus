/**
 * ContainerDetailsModal Component
 *
 * Modal for viewing comprehensive Docker container information
 *
 * Features:
 * - Complete container configuration details
 * - Networking information (IPs, networks, ports)
 * - Volume mounts and binds
 * - Environment variables
 * - Labels and metadata
 * - Runtime configuration
 * - Resource limits and stats
 */

import { useState, useEffect } from 'react';
import PropTypes from 'prop-types';
import {
  X,
  Container,
  Info,
  Network,
  HardDrive,
  Settings,
  Tag,
  Activity,
  Database,
  Globe
} from 'lucide-react';
import { Card } from '../boxes';
import { Badge } from '../data-display';
import { containersAPI } from '../../services/api';

/**
 * Format bytes to human-readable size
 */
const formatBytes = (bytes) => {
  if (!bytes || bytes === 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return `${(bytes / Math.pow(k, i)).toFixed(2)} ${sizes[i]}`;
};

/**
 * Format timestamp (handles both Unix seconds and ISO 8601 strings)
 */
const formatTimestamp = (timestamp) => {
  if (!timestamp) return 'N/A';

  let date;
  if (typeof timestamp === 'string') {
    // Docker inspect returns ISO 8601 format
    date = new Date(timestamp);
  } else {
    // Unix timestamp in seconds
    date = new Date(timestamp * 1000);
  }

  if (isNaN(date.getTime())) return 'N/A';

  return date.toLocaleString('en-US', {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  });
};

/**
 * Normalize Docker API response to consistent format
 * Docker API returns PascalCase, we normalize to lowercase keys
 */
const normalizeDockerResponse = (data) => {
  if (!data) return null;

  return {
    id: data.Id || data.id,
    name: data.Name || data.name,
    image: data.Image || data.image,
    created: data.Created || data.created,
    state: data.State?.Status || data.state,
    status: data.State?.Status || data.status,
    config: data.Config ? {
      hostname: data.Config.Hostname,
      env: data.Config.Env,
      cmd: data.Config.Cmd,
      labels: data.Config.Labels,
    } : data.config,
    restart_policy: data.HostConfig?.RestartPolicy ? {
      name: data.HostConfig.RestartPolicy.Name,
    } : data.restart_policy,
    network_settings: data.NetworkSettings ? {
      ip_address: data.NetworkSettings.IPAddress,
      gateway: data.NetworkSettings.Gateway,
      networks: data.NetworkSettings.Networks ?
        Object.fromEntries(
          Object.entries(data.NetworkSettings.Networks).map(([name, net]) => [
            name,
            { ip_address: net.IPAddress, gateway: net.Gateway }
          ])
        ) : null,
    } : data.network_settings,
    mounts: data.Mounts?.map(m => ({
      type: m.Type,
      source: m.Source,
      destination: m.Destination,
      mode: m.Mode,
    })) || data.mounts,
  };
};

/**
 * ContainerDetailsModal Component
 *
 * @param {object} props - Component props
 * @param {boolean} props.isOpen - Modal open state
 * @param {function} props.onClose - Close callback
 * @param {object} props.container - Basic container object
 */
export function ContainerDetailsModal({ isOpen, onClose, container }) {
  const [details, setDetails] = useState(null);
  const [stats, setStats] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);

  useEffect(() => {
    if (!isOpen || !container) return;

    const fetchDetails = async () => {
      setLoading(true);
      setError(null);

      try {
        // Fetch detailed container info
        const detailsData = await containersAPI.get(container.id);
        // Normalize Docker API response to consistent format
        setDetails(normalizeDockerResponse(detailsData));

        // Fetch stats if container is running
        if (container.state === 'running') {
          try {
            const statsData = await containersAPI.stats(container.id);
            setStats(statsData);
          } catch (err) {
            console.warn('Failed to fetch container stats:', err);
            // Non-critical, continue without stats
          }
        }
      } catch (err) {
        console.error('Failed to fetch container details:', err);
        setError(err.message || 'Failed to load container details');
      } finally {
        setLoading(false);
      }
    };

    fetchDetails();
  }, [isOpen, container]);

  if (!isOpen || !container) return null;

  const isRunning = container.state === 'running';
  const displayName = container.name?.replace(/^\//, '') || 'Unnamed Container';

  // Get status variant for badge
  const getStatusVariant = () => {
    if (container.state === 'running') return 'success';
    if (container.state === 'paused') return 'warning';
    return 'default';
  };

  return (
    <div className="modal-overlay" onClick={onClose}>
      <div className="modal-content modal-content-large" onClick={(e) => e.stopPropagation()}>
        <Card className="container-details-modal-card">
          {/* Header */}
          <div className="container-details-modal-header">
            <div className="header-title-section">
              <div className="container-details-icon">
                <Container size={48} />
              </div>
              <div>
                <h2 className="container-details-modal-title">{displayName}</h2>
                <p className="container-image-name">{container.image}</p>
                <Badge
                  variant={getStatusVariant()}
                  size="sm"
                  dot={true}
                  filled={true}
                  role="status"
                >
                  <span className="sr-only">Status: </span>
                  {container.state?.toUpperCase() || 'UNKNOWN'}
                </Badge>
              </div>
            </div>
            <button
              className="modal-close-btn"
              onClick={onClose}
              aria-label="Close container details"
            >
              <X size={20} />
            </button>
          </div>

          {/* Body */}
          <div className="container-details-modal-body">
            {loading && (
              <div className="loading-state">
                <p>Loading container details...</p>
              </div>
            )}

            {error && (
              <div className="error-state">
                <p>Error: {error}</p>
              </div>
            )}

            {!loading && !error && details && (
              <>
                {/* Basic Information Section */}
                <section className="details-section">
                  <h3 className="section-title">
                    <Info size={16} />
                    BASIC INFORMATION
                  </h3>
                  <div className="details-grid">
                    <div className="detail-item">
                      <span className="detail-item-label">Container ID:</span>
                      <span className="detail-item-value">
                        <code>{details.id || container.id}</code>
                      </span>
                    </div>
                    <div className="detail-item">
                      <span className="detail-item-label">Image:</span>
                      <span className="detail-item-value">
                        <code>{details.image || container.image}</code>
                      </span>
                    </div>
                    <div className="detail-item">
                      <span className="detail-item-label">Created:</span>
                      <span className="detail-item-value">
                        {formatTimestamp(details.created || container.created)}
                      </span>
                    </div>
                    <div className="detail-item">
                      <span className="detail-item-label">Status:</span>
                      <span className="detail-item-value">
                        {details.status || container.status || 'N/A'}
                      </span>
                    </div>
                    {details.config?.hostname && (
                      <div className="detail-item">
                        <span className="detail-item-label">Hostname:</span>
                        <span className="detail-item-value">
                          <code>{details.config.hostname}</code>
                        </span>
                      </div>
                    )}
                    {details.restart_policy && (
                      <div className="detail-item">
                        <span className="detail-item-label">Restart Policy:</span>
                        <span className="detail-item-value">
                          {details.restart_policy.name || 'none'}
                        </span>
                      </div>
                    )}
                  </div>
                </section>

                {/* Resource Stats (if running) */}
                {stats && isRunning && (
                  <section className="details-section">
                    <h3 className="section-title">
                      <Activity size={16} />
                      RESOURCE USAGE
                    </h3>
                    <div className="details-grid">
                      <div className="detail-item">
                        <span className="detail-item-label">CPU Usage:</span>
                        <span className="detail-item-value stat-value">
                          {stats.cpu?.usage?.toFixed(2) || '0.00'}%
                        </span>
                      </div>
                      <div className="detail-item">
                        <span className="detail-item-label">Memory Usage:</span>
                        <span className="detail-item-value stat-value">
                          {formatBytes(stats.memory?.used)} / {formatBytes(stats.memory?.limit)}
                          {stats.memory?.usage && ` (${stats.memory.usage.toFixed(1)}%)`}
                        </span>
                      </div>
                      <div className="detail-item">
                        <span className="detail-item-label">Network RX:</span>
                        <span className="detail-item-value stat-value">
                          {formatBytes(stats.network?.rx)}
                        </span>
                      </div>
                      <div className="detail-item">
                        <span className="detail-item-label">Network TX:</span>
                        <span className="detail-item-value stat-value">
                          {formatBytes(stats.network?.tx)}
                        </span>
                      </div>
                    </div>
                  </section>
                )}

                {/* Networking Section */}
                {details.network_settings && (
                  <section className="details-section">
                    <h3 className="section-title">
                      <Network size={16} />
                      NETWORKING
                    </h3>
                    <div className="details-grid">
                      {details.network_settings.ip_address && (
                        <div className="detail-item">
                          <span className="detail-item-label">IP Address:</span>
                          <span className="detail-item-value">
                            <code>{details.network_settings.ip_address}</code>
                          </span>
                        </div>
                      )}
                      {details.network_settings.gateway && (
                        <div className="detail-item">
                          <span className="detail-item-label">Gateway:</span>
                          <span className="detail-item-value">
                            <code>{details.network_settings.gateway}</code>
                          </span>
                        </div>
                      )}
                      {details.network_settings.networks && Object.keys(details.network_settings.networks).length > 0 && (
                        <div className="detail-item full-width">
                          <span className="detail-item-label">Networks:</span>
                          <div className="detail-item-value">
                            {Object.entries(details.network_settings.networks).map(([name, network]) => (
                              <div key={name} className="network-item">
                                <code>{name}</code>: {network.ip_address || 'N/A'}
                              </div>
                            ))}
                          </div>
                        </div>
                      )}
                      {container.ports && container.ports.length > 0 && (
                        <div className="detail-item full-width">
                          <span className="detail-item-label">Port Bindings:</span>
                          <div className="detail-item-value">
                            {container.ports.map((port, idx) => (
                              <div key={idx} className="port-item">
                                <code>
                                  {port.ip || '0.0.0.0'}:{port.publicPort} → {port.privatePort}/{port.type}
                                </code>
                              </div>
                            ))}
                          </div>
                        </div>
                      )}
                    </div>
                  </section>
                )}

                {/* Volumes Section */}
                {details.mounts && details.mounts.length > 0 && (
                  <section className="details-section">
                    <h3 className="section-title">
                      <HardDrive size={16} />
                      VOLUMES & MOUNTS
                    </h3>
                    <div className="mounts-list">
                      {details.mounts.map((mount, idx) => (
                        <div key={idx} className="mount-item">
                          <div className="mount-type">
                            <Badge variant="default" size="sm">
                              {mount.type || 'bind'}
                            </Badge>
                          </div>
                          <div className="mount-details">
                            <div className="mount-source">
                              <span className="mount-label">Source:</span>
                              <code>{mount.source || 'N/A'}</code>
                            </div>
                            <div className="mount-destination">
                              <span className="mount-label">Destination:</span>
                              <code>{mount.destination || mount.target || 'N/A'}</code>
                            </div>
                            {mount.mode && (
                              <div className="mount-mode">
                                <span className="mount-label">Mode:</span>
                                <code>{mount.mode}</code>
                              </div>
                            )}
                          </div>
                        </div>
                      ))}
                    </div>
                  </section>
                )}

                {/* Environment Variables Section */}
                {details.config?.env && details.config.env.length > 0 && (
                  <section className="details-section">
                    <h3 className="section-title">
                      <Settings size={16} />
                      ENVIRONMENT VARIABLES
                    </h3>
                    <div className="env-list">
                      {details.config.env.map((envVar, idx) => {
                        const [key, ...valueParts] = envVar.split('=');
                        const value = valueParts.join('=');
                        return (
                          <div key={idx} className="env-item">
                            <code className="env-key">{key}</code>
                            <code className="env-value">{value || '(empty)'}</code>
                          </div>
                        );
                      })}
                    </div>
                  </section>
                )}

                {/* Labels Section */}
                {details.config?.labels && Object.keys(details.config.labels).length > 0 && (
                  <section className="details-section">
                    <h3 className="section-title">
                      <Tag size={16} />
                      LABELS
                    </h3>
                    <div className="labels-list">
                      {Object.entries(details.config.labels).map(([key, value]) => (
                        <div key={key} className="label-item">
                          <code className="label-key">{key}</code>
                          <code className="label-value">{value}</code>
                        </div>
                      ))}
                    </div>
                  </section>
                )}

                {/* Command Section */}
                {details.config?.cmd && details.config.cmd.length > 0 && (
                  <section className="details-section">
                    <h3 className="section-title">
                      <Database size={16} />
                      COMMAND
                    </h3>
                    <div className="command-display">
                      <code>{details.config.cmd.join(' ')}</code>
                    </div>
                  </section>
                )}
              </>
            )}
          </div>

          {/* Footer */}
          <div className="container-details-modal-footer">
            <button className="btn btn-secondary" onClick={onClose}>
              CLOSE
            </button>
          </div>
        </Card>
      </div>
    </div>
  );
}

ContainerDetailsModal.propTypes = {
  isOpen: PropTypes.bool.isRequired,
  onClose: PropTypes.func.isRequired,
  container: PropTypes.shape({
    id: PropTypes.string.isRequired,
    name: PropTypes.string,
    image: PropTypes.string,
    state: PropTypes.string.isRequired,
    status: PropTypes.string,
    created: PropTypes.string,
    ports: PropTypes.arrayOf(
      PropTypes.shape({
        ip: PropTypes.string,
        privatePort: PropTypes.number,
        publicPort: PropTypes.number,
        type: PropTypes.string
      })
    )
  }),
};
