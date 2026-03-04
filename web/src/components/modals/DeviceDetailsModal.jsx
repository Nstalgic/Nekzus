/**
 * DeviceDetailsModal Component
 *
 * Modal for viewing detailed device information
 *
 * Features:
 * - Complete device information
 * - Request statistics
 * - Last active timestamp
 * - Platform and version info
 * - IP address and user agent
 * - Revoke access action
 */

import PropTypes from 'prop-types';
import { Smartphone, Monitor, Tablet, Globe, Server, Info, Activity, Network } from 'lucide-react';
import { DetailsModal } from './DetailsModal';
import { Badge } from '../data-display';

/**
 * Get platform icon based on platform type
 */
const getPlatformIcon = (platform, size = 48) => {
  const iconProps = { size };

  switch (platform?.toLowerCase()) {
    case 'ios':
      return <Smartphone {...iconProps} />;
    case 'android':
      return <Smartphone {...iconProps} />;
    case 'macos':
      return <Monitor {...iconProps} />;
    case 'windows':
      return <Monitor {...iconProps} />;
    case 'linux':
      return <Server {...iconProps} />;
    case 'web':
      return <Globe {...iconProps} />;
    default:
      return <Tablet {...iconProps} />;
  }
};

/**
 * Format full timestamp
 */
const formatFullTimestamp = (timestamp) => {
  return new Date(timestamp).toLocaleString('en-US', {
    year: 'numeric',
    month: 'long',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  });
};

/**
 * DeviceDetailsModal Component
 *
 * @param {object} props - Component props
 * @param {boolean} props.isOpen - Modal open state
 * @param {function} props.onClose - Close callback
 * @param {object} props.device - Device object
 * @param {function} props.onRevoke - Revoke callback
 */
export function DeviceDetailsModal({ isOpen, onClose, device, onRevoke }) {
  if (!device) return null;

  const isOnline = device.status === 'online';

  const handleRevoke = () => {
    if (onRevoke) {
      onRevoke(device);
    }
    onClose();
  };

  const footer = (
    <>
      <button className="btn btn-secondary" onClick={onClose}>
        CLOSE
      </button>
      <button className="btn btn-error" onClick={handleRevoke}>
        REVOKE ACCESS
      </button>
    </>
  );

  return (
    <DetailsModal
      isOpen={isOpen}
      onClose={onClose}
      icon={getPlatformIcon(device.platform)}
      title={device.name}
      badge={
        <Badge
          variant={isOnline ? 'success' : 'default'}
          size="sm"
          dot={true}
          filled={true}
          role="status"
        >
          <span className="sr-only">Status: </span>
          {(device.status || 'offline').toUpperCase()}
        </Badge>
      }
      footer={footer}
      size="medium"
    >
      {/* Device Information Section */}
      <section className="details-section">
        <h3 className="section-title">
          <Info size={16} />
          DEVICE INFORMATION
        </h3>
        <div className="details-grid">
          <div className="detail-item">
            <span className="detail-item-label">Device ID:</span>
            <span className="detail-item-value">
              <code>{device.id}</code>
            </span>
          </div>
          <div className="detail-item">
            <span className="detail-item-label">Platform:</span>
            <span className="detail-item-value">
              {device.platform} {device.platformVersion}
            </span>
          </div>
          <div className="detail-item">
            <span className="detail-item-label">Status:</span>
            <span className="detail-item-value">
              <Badge variant={isOnline ? 'success' : 'default'} size="sm">
                {device.status}
              </Badge>
            </span>
          </div>
          {device.ipAddress && (
            <div className="detail-item">
              <span className="detail-item-label">IP Address:</span>
              <span className="detail-item-value">
                <code>{device.ipAddress}</code>
              </span>
            </div>
          )}
          {device.userAgent && (
            <div className="detail-item full-width">
              <span className="detail-item-label">User Agent:</span>
              <span className="detail-item-value">
                <code>{device.userAgent}</code>
              </span>
            </div>
          )}
        </div>
      </section>

      {/* Activity Section */}
      <section className="details-section">
        <h3 className="section-title">
          <Activity size={16} />
          ACTIVITY
        </h3>
        <div className="details-grid">
          <div className="detail-item">
            <span className="detail-item-label">Requests Today:</span>
            <span className="detail-item-value stat-value">
              {device.requestsToday || 0}
            </span>
          </div>
          {device.totalRequests !== undefined && (
            <div className="detail-item">
              <span className="detail-item-label">Total Requests:</span>
              <span className="detail-item-value stat-value">
                {device.totalRequests}
              </span>
            </div>
          )}
          <div className="detail-item">
            <span className="detail-item-label">Last Seen:</span>
            <span className="detail-item-value">
              {formatFullTimestamp(device.lastSeen)}
            </span>
          </div>
          <div className="detail-item">
            <span className="detail-item-label">Paired At:</span>
            <span className="detail-item-value">
              {formatFullTimestamp(device.pairedAt)}
            </span>
          </div>
        </div>
      </section>

      {/* Network Section */}
      {(device.ipAddress || device.connectionType) && (
        <section className="details-section">
          <h3 className="section-title">
            <Network size={16} />
            NETWORK INFORMATION
          </h3>
          <div className="details-grid">
            {device.ipAddress && (
              <div className="detail-item">
                <span className="detail-item-label">Current IP:</span>
                <span className="detail-item-value">
                  <code>{device.ipAddress}</code>
                </span>
              </div>
            )}
            {device.connectionType && (
              <div className="detail-item">
                <span className="detail-item-label">Connection:</span>
                <span className="detail-item-value">
                  {device.connectionType}
                </span>
              </div>
            )}
          </div>
        </section>
      )}

      {/* Security Warning */}
      <div className="security-notice">
        <span className="security-notice-icon" aria-hidden="true">
          ⚠
        </span>
        <div className="security-notice-text">
          <strong>Security Notice:</strong> Revoking this device will immediately
          terminate all active sessions and prevent future access to all routes
          and services.
        </div>
      </div>
    </DetailsModal>
  );
}

DeviceDetailsModal.propTypes = {
  isOpen: PropTypes.bool.isRequired,
  onClose: PropTypes.func.isRequired,
  device: PropTypes.shape({
    id: PropTypes.string.isRequired,
    name: PropTypes.string.isRequired,
    platform: PropTypes.string.isRequired,
    platformVersion: PropTypes.string,
    status: PropTypes.oneOf(['online', 'offline']).isRequired,
    lastSeen: PropTypes.string.isRequired,
    pairedAt: PropTypes.string.isRequired,
    requestsToday: PropTypes.number.isRequired,
    totalRequests: PropTypes.number,
    ipAddress: PropTypes.string,
    userAgent: PropTypes.string,
    connectionType: PropTypes.string,
  }),
  onRevoke: PropTypes.func,
};
