/**
 * PeerCard Component
 *
 * Displays a federation peer with status, services count, and last seen time.
 *
 * Features:
 * - Online/offline status badge
 * - Service count from peer
 * - Last seen timestamp
 * - Click to view details
 */

import { Badge } from '../data-display';
import PropTypes from 'prop-types';

/**
 * PeerCard Component
 * @param {Object} props
 * @param {Object} props.peer - Peer data
 * @param {Function} props.onViewDetails - Callback when view details clicked
 */
export function PeerCard({ peer, onViewDetails }) {
  const isOnline = peer.status === 'online';

  // Format last seen time
  const formatLastSeen = (timestamp) => {
    if (!timestamp) return 'Never';
    const date = new Date(timestamp);
    const now = new Date();
    const diff = now - date;
    const minutes = Math.floor(diff / 60000);
    const hours = Math.floor(diff / 3600000);

    if (minutes < 1) return 'Just now';
    if (minutes < 60) return `${minutes}m ago`;
    if (hours < 24) return `${hours}h ago`;
    return date.toLocaleDateString();
  };

  return (
    <div
      className={`peer-card ${isOnline ? 'online' : 'offline'}`}
      onClick={() => onViewDetails?.(peer)}
      role="button"
      tabIndex={0}
      onKeyDown={(e) => {
        if (e.key === 'Enter' || e.key === ' ') {
          onViewDetails?.(peer);
        }
      }}
    >
      <div className="peer-card-header">
        <div className="peer-identity">
          <span className="peer-icon">
            {isOnline ? '\u25CF' : '\u25CB'}
          </span>
          <div className="peer-info">
            <h4 className="peer-name">{peer.name || peer.id}</h4>
            <span className="peer-id">{peer.id}</span>
          </div>
        </div>
        <Badge variant={isOnline ? 'success' : 'warning'} filled>
          {isOnline ? 'ONLINE' : 'OFFLINE'}
        </Badge>
      </div>

      <div className="peer-card-body">
        <div className="peer-stat">
          <span className="stat-label">Address</span>
          <span className="stat-value">{peer.gossip_addr || peer.address}</span>
        </div>
        <div className="peer-stat">
          <span className="stat-label">Services</span>
          <span className="stat-value">{peer.service_count || 0}</span>
        </div>
        <div className="peer-stat">
          <span className="stat-label">Last Seen</span>
          <span className="stat-value">{formatLastSeen(peer.last_seen)}</span>
        </div>
      </div>

      <div className="peer-card-footer">
        <button
          className="btn btn-sm btn-secondary"
          onClick={(e) => {
            e.stopPropagation();
            onViewDetails?.(peer);
          }}
        >
          VIEW DETAILS
        </button>
      </div>
    </div>
  );
}

PeerCard.propTypes = {
  peer: PropTypes.shape({
    id: PropTypes.string.isRequired,
    name: PropTypes.string,
    status: PropTypes.oneOf(['online', 'offline']),
    address: PropTypes.string,
    gossip_addr: PropTypes.string,
    service_count: PropTypes.number,
    last_seen: PropTypes.string,
  }).isRequired,
  onViewDetails: PropTypes.func,
};

PeerCard.defaultProps = {
  onViewDetails: null,
};
