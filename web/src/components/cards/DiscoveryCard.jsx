/**
 * DiscoveryCard Component
 *
 * Card component for displaying discovered services pending approval
 *
 * Features:
 * - Checkbox for bulk selection
 * - Service icon and name
 * - Source badge (docker/mdns/k8s)
 * - Confidence category badge
 * - Security alerts
 * - Service details
 * - Approve/Reject actions
 * - Tags display
 */

import { useState } from 'react';
import PropTypes from 'prop-types';
import { Target } from 'lucide-react';
import { Badge } from '../data-display';
import { getConfidenceCategory } from '../../utils/confidence';

/**
 * DiscoveryCard Component
 *
 * @param {object} props - Component props
 * @param {object} props.discovery - Discovery service object (Proposal)
 * @param {string} props.discovery.id - Unique discovery ID
 * @param {string} props.discovery.source - Discovery source (docker/mdns/kubernetes)
 * @param {object} props.discovery.suggestedApp - Suggested app details
 * @param {string} props.discovery.suggestedApp.name - Service name
 * @param {object} props.discovery.suggestedRoute - Suggested route details
 * @param {string} props.discovery.suggestedRoute.to - Target URL
 * @param {string} props.discovery.suggestedRoute.pathBase - Suggested path
 * @param {number} props.discovery.confidence - Confidence percentage (0-100)
 * @param {string[]} props.discovery.tags - Service tags
 * @param {object} props.discovery.security - Security info
 * @param {boolean} props.discovery.selected - Selection state
 * @param {function} props.onSelect - Selection toggle callback
 * @param {function} props.onApprove - Approve callback
 * @param {function} props.onReject - Reject callback
 */
export function DiscoveryCard({
  discovery,
  onSelect,
  onApprove,
  onReject
}) {
  const [isProcessing, setIsProcessing] = useState(false);
  // Initialize with the first available port (the default)
  const availablePorts = discovery.availablePorts || [];
  const hasMultiplePorts = availablePorts.length > 1;
  const [selectedPort, setSelectedPort] = useState(
    availablePorts.length > 0 ? availablePorts[0].port : null
  );

  const handleApprove = async () => {
    setIsProcessing(true);
    try {
      // Pass selected port if multiple ports are available
      await onApprove(discovery, hasMultiplePorts ? selectedPort : null);
    } finally {
      setIsProcessing(false);
    }
  };

  const handleReject = async () => {
    setIsProcessing(true);
    try {
      await onReject(discovery);
    } finally {
      setIsProcessing(false);
    }
  };

  const getSourceBadgeVariant = (source) => {
    switch (source) {
      case 'docker':
        return 'primary';
      case 'kubernetes':
        return 'success';
      case 'mdns':
        return 'warning';
      default:
        return 'default';
    }
  };

  const confidenceCategory = getConfidenceCategory(discovery.confidence);

  return (
    <div className={`discovery-card ${discovery.selected ? 'selected' : ''}`}>
      {/* Selection Checkbox */}
      <div className="discovery-card-select">
        <input
          type="checkbox"
          className="checkbox discovery-checkbox"
          checked={discovery.selected}
          onChange={() => onSelect(discovery.id)}
          aria-label={`Select ${discovery.suggestedApp?.name || 'service'}`}
          disabled={isProcessing}
        />
      </div>

      {/* Header: Name, Tags, Confidence */}
      <div className="discovery-card-header">
        <div className="discovery-card-title">
          <div className="discovery-icon">
            <Target size={24} strokeWidth={2} />
          </div>
          <div>
            <h3>{discovery.suggestedApp?.name || discovery.suggestedApp?.id || 'Unknown Service'}</h3>
            {discovery.tags && discovery.tags.length > 0 && (
              <div className="discovery-tags">
                {discovery.tags.map((tag) => (
                  <span key={tag} className="tag">
                    {tag}
                  </span>
                ))}
              </div>
            )}
          </div>
        </div>
        <div className="discovery-confidence">
          <Badge variant={confidenceCategory.variant} size="sm">
            {confidenceCategory.label}
          </Badge>
        </div>
      </div>

      {/* Body: Service Details */}
      <div className="discovery-card-body">
        <div className="discovery-detail">
          <span className="detail-label">Source:</span>
          <span className="detail-value">
            <Badge variant={getSourceBadgeVariant(discovery.source)} size="sm">
              {discovery.source}
            </Badge>
          </span>
        </div>
        <div className="discovery-detail">
          <span className="detail-label">Target:</span>
          <span className="detail-value">{discovery.suggestedRoute?.to || 'N/A'}</span>
        </div>
        <div className="discovery-detail">
          <span className="detail-label">Path:</span>
          <span className="detail-value">
            <code>{discovery.suggestedRoute?.pathBase || '/'}</code>
          </span>
        </div>
        {discovery.details && discovery.details.security && (
          <div className="discovery-detail">
            <span className="detail-label">Security:</span>
            <span className="detail-value">{discovery.details.security}</span>
          </div>
        )}
        {hasMultiplePorts && (
          <div className="discovery-detail">
            <span className="detail-label">Port:</span>
            <span className="detail-value">
              <select
                className="port-select"
                value={selectedPort || ''}
                onChange={(e) => setSelectedPort(parseInt(e.target.value, 10))}
                disabled={isProcessing}
                aria-label="Select port"
              >
                {availablePorts.map((portOption) => (
                  <option key={portOption.port} value={portOption.port}>
                    {portOption.port} ({portOption.scheme})
                  </option>
                ))}
              </select>
            </span>
          </div>
        )}
      </div>

      {/* Security Alert */}
      {discovery.security?.hasWarning && discovery.security?.message && (
        <div className="security-alert">
          <span>
            <strong>Security Risk:</strong> {discovery.security.message}
          </span>
        </div>
      )}

      {/* Actions: Reject and Approve */}
      <div className="discovery-card-actions">
        <button
          type="button"
          className="btn btn-secondary"
          onClick={handleReject}
          disabled={isProcessing}
          aria-label={`Reject ${discovery.suggestedApp?.name || 'service'}`}
        >
          REJECT
        </button>
        <button
          type="button"
          className="btn btn-success"
          onClick={handleApprove}
          disabled={isProcessing}
          aria-label={`Approve ${discovery.suggestedApp?.name || 'service'}`}
        >
          APPROVE
        </button>
      </div>
    </div>
  );
}

DiscoveryCard.propTypes = {
  discovery: PropTypes.shape({
    id: PropTypes.string.isRequired,
    source: PropTypes.string.isRequired,
    confidence: PropTypes.number.isRequired,
    suggestedApp: PropTypes.shape({
      id: PropTypes.string,
      name: PropTypes.string,
      icon: PropTypes.string,
      tags: PropTypes.arrayOf(PropTypes.string)
    }),
    suggestedRoute: PropTypes.shape({
      routeId: PropTypes.string,
      appId: PropTypes.string,
      pathBase: PropTypes.string,
      to: PropTypes.string,
      scopes: PropTypes.arrayOf(PropTypes.string)
    }),
    tags: PropTypes.arrayOf(PropTypes.string),
    security: PropTypes.shape({
      hasWarning: PropTypes.bool,
      message: PropTypes.string
    }),
    securityNotes: PropTypes.arrayOf(PropTypes.string),
    details: PropTypes.shape({
      security: PropTypes.string
    }),
    availablePorts: PropTypes.arrayOf(PropTypes.shape({
      port: PropTypes.number.isRequired,
      scheme: PropTypes.string.isRequired
    })),
    selected: PropTypes.bool,
    lastSeen: PropTypes.string
  }).isRequired,
  onSelect: PropTypes.func.isRequired,
  onApprove: PropTypes.func.isRequired,
  onReject: PropTypes.func.isRequired
};
