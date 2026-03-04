/**
 * ServiceCard Component
 *
 * Card component for displaying toolbox services
 *
 * Features:
 * - Service icon and name
 * - Description and category badge
 * - Tags display
 * - Image/port info with links
 * - Deploy and details actions
 */

import { useMemo } from 'react';
import PropTypes from 'prop-types';
import { Package } from 'lucide-react';
import { Badge } from '../data-display';

/**
 * Check if a string is a URL
 */
const isUrl = (str) => {
  if (!str) return false;
  return str.startsWith('http://') || str.startsWith('https://');
};

/**
 * ServiceCard Component
 *
 * @param {object} props - Component props
 * @param {object} props.service - Service object
 * @param {string} props.service.id - Service ID
 * @param {string} props.service.name - Service name
 * @param {string} props.service.description - Service description
 * @param {string} props.service.icon - Service icon (emoji or URL)
 * @param {string} props.service.category - Service category
 * @param {array} props.service.tags - Service tags
 * @param {string} props.service.image_url - Docker Hub URL
 * @param {string} props.service.repository_url - Source repository URL
 * @param {function} props.onViewDetails - View details callback
 * @param {function} props.onDeploy - Deploy service callback
 */
export function ServiceCard({
  service,
  onViewDetails,
  onDeploy
}) {
  // Format display name
  const displayName = useMemo(() => {
    return service.name || 'Unnamed Service';
  }, [service.name]);

  // Check if icon is a URL or emoji
  const iconIsUrl = useMemo(() => isUrl(service.icon), [service.icon]);

  return (
    <div className="service-card">
      {/* Header: Category Badge */}
      <div className="service-card-header">
        <Badge
          variant="info"
          size="sm"
          filled={true}
          className="service-category-badge"
        >
          {service.category?.toUpperCase() || 'GENERAL'}
        </Badge>
      </div>

      {/* Body: Service Info */}
      <div className="service-card-body">
        <div className="service-card-title">
          <div className="service-icon">
            {service.icon ? (
              iconIsUrl ? (
                <img
                  src={service.icon}
                  alt={`${displayName} icon`}
                  className="service-icon-image"
                  onError={(e) => {
                    e.target.style.display = 'none';
                    e.target.nextSibling?.classList.remove('hidden');
                  }}
                />
              ) : (
                <span className="service-icon-emoji">{service.icon}</span>
              )
            ) : (
              <Package size={32} strokeWidth={2} />
            )}
            {iconIsUrl && <Package size={32} strokeWidth={2} className="hidden service-icon-fallback" />}
          </div>
          <div>
            <h3 className="service-name">{displayName}</h3>
            <p className="service-description">{service.description || 'No description available'}</p>
          </div>
        </div>

        {/* Tags */}
        {service.tags && service.tags.length > 0 && (
          <div className="service-tags">
            {service.tags.map((tag, index) => (
              <span key={index} className="service-tag">
                {tag}
              </span>
            ))}
          </div>
        )}

        {/* Service Info */}
        <div className="service-info">
          {service.image && (
            <div className="service-info-item">
              <span className="service-info-label">Image:</span>
              <span className="service-info-value">
                {service.image_url ? (
                  <a
                    href={service.image_url}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="service-info-link"
                  >
                    <code>{service.image}</code>
                  </a>
                ) : (
                  <code>{service.image}</code>
                )}
              </span>
            </div>
          )}
          {service.default_port && (
            <div className="service-info-item">
              <span className="service-info-label">Port:</span>
              <span className="service-info-value">
                <code>{service.default_port}</code>
              </span>
            </div>
          )}
          {service.repository_url && (
            <div className="service-info-item">
              <span className="service-info-label">Source:</span>
              <span className="service-info-value">
                <a
                  href={service.repository_url}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="service-info-link"
                >
                  <code>GitHub</code>
                </a>
              </span>
            </div>
          )}
        </div>
      </div>

      {/* Actions */}
      <div className="service-card-actions">
        <button
          type="button"
          className="btn btn-secondary btn-sm"
          onClick={() => onViewDetails(service)}
          aria-label={`View details for ${displayName}`}
        >
          DETAILS
        </button>
        <button
          type="button"
          className="btn btn-success btn-sm"
          onClick={() => onDeploy(service)}
          aria-label={`Deploy ${displayName}`}
        >
          DEPLOY
        </button>
      </div>
    </div>
  );
}

ServiceCard.propTypes = {
  service: PropTypes.shape({
    id: PropTypes.string.isRequired,
    name: PropTypes.string.isRequired,
    description: PropTypes.string,
    icon: PropTypes.string,
    category: PropTypes.string,
    tags: PropTypes.arrayOf(PropTypes.string),
    image: PropTypes.string,
    default_port: PropTypes.number,
    image_url: PropTypes.string,
    repository_url: PropTypes.string
  }).isRequired,
  onViewDetails: PropTypes.func.isRequired,
  onDeploy: PropTypes.func.isRequired
};
