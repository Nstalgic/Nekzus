/**
 * ServiceDetailsModal Component
 *
 * Modal for displaying detailed service information
 *
 * Features:
 * - Service overview
 * - Full description
 * - Environment variables list
 * - Port mappings
 * - Volume mounts
 * - Deploy button
 */

import PropTypes from 'prop-types';
import { X, Package } from 'lucide-react';
import { Modal } from './Modal';
import { Badge } from '../data-display';
import styles from './Modal.module.css';

/**
 * Check if a string is a URL
 */
const isUrl = (str) => {
  if (!str) return false;
  return str.startsWith('http://') || str.startsWith('https://');
};

/**
 * ServiceDetailsModal Component
 *
 * @param {object} props - Component props
 * @param {boolean} props.isOpen - Whether the modal is open
 * @param {function} props.onClose - Close callback
 * @param {object} props.service - Service object
 * @param {function} props.onDeploy - Deploy callback
 */
export function ServiceDetailsModal({
  isOpen,
  onClose,
  service,
  onDeploy
}) {
  if (!service) return null;

  const handleDeploy = () => {
    onClose();
    onDeploy(service);
  };

  const iconIsUrl = isUrl(service.icon);

  return (
    <Modal isOpen={isOpen} onClose={onClose} size="large">
      <div className={styles.modalHeader}>
        <div className={styles.modalTitleSection}>
          {service.icon ? (
            iconIsUrl ? (
              <img
                src={service.icon}
                alt={`${service.name} icon`}
                className="service-icon-image-large"
                onError={(e) => {
                  e.target.style.display = 'none';
                }}
              />
            ) : (
              <span className="service-icon-emoji-large">{service.icon}</span>
            )
          ) : (
            <Package size={48} strokeWidth={2} />
          )}
          <h2>{service.name}</h2>
        </div>
        <button
          className={styles.modalCloseButton}
          onClick={onClose}
          aria-label="Close modal"
        >
          <X size={20} />
        </button>
      </div>

      <div className={styles.modalBody}>
        {/* Metadata */}
        <div className="service-details-metadata">
          <Badge variant="info" filled={true}>
            {service.category?.toUpperCase() || 'GENERAL'}
          </Badge>
        </div>

        {/* Description */}
        <div className="service-details-section">
          <h3>Description</h3>
          <p className="text-secondary">{service.description || 'No description available'}</p>
        </div>

        {/* Tags */}
        {service.tags && service.tags.length > 0 && (
          <div className="service-details-section">
            <h3>Tags</h3>
            <div className="service-tags-list">
              {service.tags.map((tag, index) => (
                <span key={index} className="service-tag">{tag}</span>
              ))}
            </div>
          </div>
        )}

        {/* Docker Image */}
        <div className="service-details-section">
          <h3>Docker Image</h3>
          <code className="service-details-code">{service.image}</code>
        </div>

        {/* Default Port */}
        {service.default_port && (
          <div className="service-details-section">
            <h3>Default Port</h3>
            <code className="service-details-code">{service.default_port}</code>
          </div>
        )}

        {/* Environment Variables */}
        {service.env_vars && service.env_vars.length > 0 && (
          <div className="service-details-section">
            <h3>Environment Variables</h3>
            <div className="service-env-vars-list">
              {service.env_vars.map((envVar, index) => (
                <div key={index} className="service-env-var-item">
                  <div className="env-var-name">
                    <code>{envVar.name}</code>
                    {envVar.required && (
                      <Badge variant="warning" size="sm">REQUIRED</Badge>
                    )}
                  </div>
                  <div className="env-var-description">
                    {envVar.description || 'No description'}
                  </div>
                  {envVar.default && (
                    <div className="env-var-default">
                      Default: <code>{envVar.default}</code>
                    </div>
                  )}
                </div>
              ))}
            </div>
          </div>
        )}

        {/* Volumes */}
        {service.volumes && service.volumes.length > 0 && (
          <div className="service-details-section">
            <h3>Volume Mounts</h3>
            <div className="service-volumes-list">
              {service.volumes.map((volume, index) => (
                <div key={index} className="service-volume-item">
                  <code>{volume}</code>
                </div>
              ))}
            </div>
          </div>
        )}
      </div>

      <div className={styles.modalFooter}>
        <button
          type="button"
          className="btn btn-secondary"
          onClick={onClose}
        >
          CLOSE
        </button>
        <button
          type="button"
          className="btn btn-success"
          onClick={handleDeploy}
        >
          DEPLOY SERVICE
        </button>
      </div>
    </Modal>
  );
}

ServiceDetailsModal.propTypes = {
  isOpen: PropTypes.bool.isRequired,
  onClose: PropTypes.func.isRequired,
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
    repository_url: PropTypes.string,
    env_vars: PropTypes.arrayOf(PropTypes.shape({
      name: PropTypes.string.isRequired,
      description: PropTypes.string,
      required: PropTypes.bool,
      default: PropTypes.string
    })),
    volumes: PropTypes.arrayOf(PropTypes.string)
  }),
  onDeploy: PropTypes.func.isRequired
};
