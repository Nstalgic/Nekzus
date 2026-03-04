/**
 * DeviceCard Component
 *
 * Card component for displaying paired devices
 *
 * Features:
 * - Device icon (platform-specific)
 * - Status indicator dot (online/offline)
 * - Status badge
 * - Device name, ID, and platform
 * - Last seen, paired date, and requests today
 * - View Details and Revoke actions
 */

import { useMemo } from 'react';
import PropTypes from 'prop-types';
import { Badge } from '../data-display';

/**
 * Format relative time (e.g., "2m ago", "Just now")
 */
const formatRelativeTime = (timestamp) => {
  const now = Date.now();
  const past = new Date(timestamp).getTime();
  const diffMs = now - past;
  const diffSec = Math.floor(diffMs / 1000);
  const diffMin = Math.floor(diffSec / 60);
  const diffHour = Math.floor(diffMin / 60);
  const diffDay = Math.floor(diffHour / 24);

  if (diffSec < 60) return 'Just now';
  if (diffMin < 60) return `${diffMin}m ago`;
  if (diffHour < 24) return `${diffHour}h ago`;
  return `${diffDay}d ago`;
};

/**
 * DeviceCard Component
 *
 * @param {object} props - Component props
 * @param {object} props.device - Device object
 * @param {string} props.device.id - Unique device ID
 * @param {string} props.device.name - Device name
 * @param {string} props.device.platform - Platform (iOS, Android, etc.)
 * @param {string} props.device.platformVersion - Platform version
 * @param {string} props.device.status - Status (online/offline)
 * @param {string} props.device.lastSeen - Last seen timestamp
 * @param {string} props.device.pairedAt - Paired timestamp
 * @param {number} props.device.requestsToday - Requests count today
 * @param {function} props.onViewDetails - View details callback
 * @param {function} props.onRevoke - Revoke callback
 */
export function DeviceCard({
  device,
  onViewDetails,
  onRevoke
}) {
  const isOnline = device.status === 'online';

  const lastSeenFormatted = useMemo(() => {
    return formatRelativeTime(device.lastSeen);
  }, [device.lastSeen]);

  const pairedAtFormatted = useMemo(() => {
    return formatRelativeTime(device.pairedAt);
  }, [device.pairedAt]);

  return (
    <div className="device-card">
      {/* Header: Status Badge */}
      <div className="device-card-header">
        <Badge
          variant={isOnline ? 'success' : 'default'}
          size="sm"
          dot={true}
          filled={true}
          role="status"
          className="device-status-badge"
        >
          <span className="sr-only">Status: </span>
          {(device.status || 'offline').toUpperCase()}
        </Badge>
      </div>

      {/* Body: Device Info */}
      <div className="device-card-body">
        <h3 className="device-name">{device.name}</h3>
        <p className="device-id">ID: {device.id}</p>
        <p className="device-type">
          {device.platform} {device.platformVersion} • {device.status}
        </p>

        <div className="device-info">
          <div className="device-info-item">
            <span className="device-info-label">Last seen:</span>
            <span className="device-info-value">{lastSeenFormatted}</span>
          </div>
          <div className="device-info-item">
            <span className="device-info-label">Paired:</span>
            <span className="device-info-value">{pairedAtFormatted}</span>
          </div>
          <div className="device-info-item">
            <span className="device-info-label">Requests:</span>
            <span className="device-info-value">{device.requestsToday} today</span>
          </div>
        </div>
      </div>

      {/* Actions: View Details and Revoke */}
      <div className="device-card-actions">
        <button
          className="btn btn-secondary"
          onClick={() => onViewDetails(device)}
          aria-label={`View details for ${device.name}`}
        >
          VIEW DETAILS
        </button>
        <button
          className="btn btn-error"
          onClick={() => onRevoke(device)}
          aria-label={`Revoke ${device.name}`}
        >
          REVOKE
        </button>
      </div>
    </div>
  );
}

DeviceCard.propTypes = {
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
    userAgent: PropTypes.string
  }).isRequired,
  onViewDetails: PropTypes.func.isRequired,
  onRevoke: PropTypes.func.isRequired
};
