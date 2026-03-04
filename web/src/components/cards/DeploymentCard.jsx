/**
 * DeploymentCard Component
 *
 * Card component for displaying active toolbox deployments
 *
 * Features:
 * - Deployment status badge
 * - Service name and container ID
 * - Deployed timestamp
 * - Environment variables display
 * - Remove action
 */

import { useMemo } from 'react';
import PropTypes from 'prop-types';
import { Package, Trash2 } from 'lucide-react';
import { Badge } from '../data-display';

/**
 * Get status badge variant
 */
const getStatusVariant = (status) => {
  switch (status?.toLowerCase()) {
    case 'running':
      return 'success';
    case 'deploying':
      return 'warning';
    case 'stopped':
      return 'default';
    case 'error':
      return 'error';
    default:
      return 'default';
  }
};

/**
 * Format timestamp for display
 */
const formatTimestamp = (timestamp) => {
  if (!timestamp) return 'N/A';
  const date = new Date(timestamp);
  return date.toLocaleString();
};

/**
 * Truncate container ID for display
 */
const truncateId = (id) => {
  return id ? id.substring(0, 12) : 'N/A';
};

/**
 * DeploymentCard Component
 *
 * @param {object} props - Component props
 * @param {object} props.deployment - Deployment object
 * @param {string} props.deployment.id - Deployment ID
 * @param {string} props.deployment.service_id - Service ID
 * @param {string} props.deployment.service_name - Service name
 * @param {string} props.deployment.container_id - Container ID
 * @param {string} props.deployment.status - Deployment status
 * @param {string} props.deployment.deployed_at - Deployment timestamp
 * @param {object} props.deployment.env_vars - Environment variables
 * @param {function} props.onRemove - Remove deployment callback
 */
export function DeploymentCard({
  deployment,
  onRemove
}) {
  // Format display name
  const displayName = useMemo(() => {
    return deployment.service_name || 'Unnamed Deployment';
  }, [deployment.service_name]);

  // Get status variant
  const statusVariant = useMemo(() => {
    return getStatusVariant(deployment.status);
  }, [deployment.status]);

  // Format deployed timestamp
  const deployedAt = useMemo(() => {
    return formatTimestamp(deployment.deployed_at);
  }, [deployment.deployed_at]);

  return (
    <div className="deployment-card">
      {/* Header: Status Badge */}
      <div className="deployment-card-header">
        <Badge
          variant={statusVariant}
          size="sm"
          dot={true}
          filled={true}
          role="status"
          className="deployment-status-badge"
        >
          <span className="sr-only">Status: </span>
          {deployment.status?.toUpperCase() || 'UNKNOWN'}
        </Badge>
      </div>

      {/* Body: Deployment Info */}
      <div className="deployment-card-body">
        <div className="deployment-card-title">
          <div className="deployment-icon">
            <Package size={24} strokeWidth={2} />
          </div>
          <div>
            <h3 className="deployment-name">{displayName}</h3>
            <p className="deployment-service-id">Service ID: {deployment.service_id || 'N/A'}</p>
          </div>
        </div>

        <div className="deployment-info">
          <div className="deployment-info-item">
            <span className="deployment-info-label">Container ID:</span>
            <span className="deployment-info-value">
              <code>{truncateId(deployment.container_id)}</code>
            </span>
          </div>
          <div className="deployment-info-item">
            <span className="deployment-info-label">Deployed:</span>
            <span className="deployment-info-value">{deployedAt}</span>
          </div>
          {deployment.port && (
            <div className="deployment-info-item">
              <span className="deployment-info-label">Port:</span>
              <span className="deployment-info-value">
                <code>{deployment.port}</code>
              </span>
            </div>
          )}
        </div>

        {/* Environment Variables (if any) */}
        {deployment.env_vars && Object.keys(deployment.env_vars).length > 0 && (
          <div className="deployment-env-vars">
            <div className="deployment-env-vars-header">
              <span className="deployment-info-label">Environment Variables:</span>
            </div>
            <div className="deployment-env-vars-list">
              {Object.entries(deployment.env_vars).slice(0, 3).map(([key, value]) => (
                <div key={key} className="deployment-env-var-item">
                  <code className="env-var-key">{key}</code>
                  <code className="env-var-value">{value}</code>
                </div>
              ))}
              {Object.keys(deployment.env_vars).length > 3 && (
                <p className="env-vars-more">
                  +{Object.keys(deployment.env_vars).length - 3} more...
                </p>
              )}
            </div>
          </div>
        )}
      </div>

      {/* Actions */}
      <div className="deployment-card-actions">
        <button
          type="button"
          className="btn btn-error btn-sm"
          onClick={() => onRemove(deployment)}
          aria-label={`Remove deployment ${displayName}`}
        >
          <Trash2 size={16} />
          REMOVE
        </button>
      </div>
    </div>
  );
}

DeploymentCard.propTypes = {
  deployment: PropTypes.shape({
    id: PropTypes.string.isRequired,
    service_id: PropTypes.string.isRequired,
    service_name: PropTypes.string.isRequired,
    container_id: PropTypes.string,
    status: PropTypes.string.isRequired,
    deployed_at: PropTypes.string,
    port: PropTypes.number,
    env_vars: PropTypes.object
  }).isRequired,
  onRemove: PropTypes.func.isRequired
};
