/**
 * DeployServiceModal Component
 *
 * Modal for deploying a service with configuration
 *
 * Features:
 * - Service name input
 * - Environment variables form
 * - Route creation option
 * - Auto-start option
 * - Form validation
 * - Deployment progress
 */

import { useState, useEffect } from 'react';
import PropTypes from 'prop-types';
import { X, AlertCircle, Package } from 'lucide-react';
import { Modal } from './Modal';
import { Input, FormGroup, Label, Checkbox } from '../forms';
import styles from './Modal.module.css';

/**
 * Check if a string is a URL
 */
const isUrl = (str) => {
  if (!str) return false;
  return str.startsWith('http://') || str.startsWith('https://');
};

/**
 * DeployServiceModal Component
 *
 * @param {object} props - Component props
 * @param {boolean} props.isOpen - Whether the modal is open
 * @param {function} props.onClose - Close callback
 * @param {object} props.service - Service object
 * @param {function} props.onDeploy - Deploy callback
 */
export function DeployServiceModal({
  isOpen,
  onClose,
  service,
  onDeploy
}) {
  const [serviceName, setServiceName] = useState('');
  const [imageTag, setImageTag] = useState('');
  const [customPort, setCustomPort] = useState('');
  const [envVars, setEnvVars] = useState({});
  const [autoStart, setAutoStart] = useState(true);
  const [isDeploying, setIsDeploying] = useState(false);
  const [deployError, setDeployError] = useState(null);
  const [validationErrors, setValidationErrors] = useState({});

  // Initialize form when service changes
  useEffect(() => {
    if (service) {
      setServiceName(service.name || '');

      // Initialize env vars with defaults
      const initialEnvVars = {};
      if (service.env_vars) {
        service.env_vars.forEach(envVar => {
          if (envVar.default) {
            initialEnvVars[envVar.name] = envVar.default;
          } else {
            initialEnvVars[envVar.name] = '';
          }
        });
      }
      setEnvVars(initialEnvVars);
      // Extract tag from image (e.g., "nginx:latest" -> "latest")
      const tag = service.image?.includes(':')
        ? service.image.split(':').pop()
        : 'latest';
      setImageTag(tag);
      setCustomPort(service.default_port ? String(service.default_port) : '');
      setAutoStart(true);
      setDeployError(null);
      setValidationErrors({});
    }
  }, [service]);

  // Reset form when modal closes
  useEffect(() => {
    if (!isOpen) {
      setIsDeploying(false);
      setDeployError(null);
      setValidationErrors({});
    }
  }, [isOpen]);

  // Validate form
  const validateForm = () => {
    const errors = {};

    // Validate service name
    if (!serviceName.trim()) {
      errors.serviceName = 'Service name is required';
    }

    // Validate required env vars
    if (service?.env_vars) {
      service.env_vars.forEach(envVar => {
        if (envVar.required && !envVars[envVar.name]?.trim()) {
          errors[envVar.name] = `${envVar.name} is required`;
        }
      });
    }

    setValidationErrors(errors);
    return Object.keys(errors).length === 0;
  };

  // Handle env var change
  const handleEnvVarChange = (name, value) => {
    setEnvVars(prev => ({
      ...prev,
      [name]: value
    }));
    // Clear validation error for this field
    if (validationErrors[name]) {
      setValidationErrors(prev => {
        const newErrors = { ...prev };
        delete newErrors[name];
        return newErrors;
      });
    }
  };

  // Handle deploy
  const handleDeploy = async () => {
    if (!validateForm()) {
      return;
    }

    setIsDeploying(true);
    setDeployError(null);

    try {
      // Filter out empty env vars
      const filteredEnvVars = Object.entries(envVars).reduce((acc, [key, value]) => {
        if (value && value.trim()) {
          acc[key] = value;
        }
        return acc;
      }, {});

      const deploymentConfig = {
        service_id: service.id,
        service_name: serviceName,
        env_vars: filteredEnvVars,
        auto_start: autoStart
      };

      // Build custom image if tag differs from default
      const defaultTag = service.image?.includes(':')
        ? service.image.split(':').pop()
        : 'latest';
      if (imageTag && imageTag !== defaultTag) {
        const imageBase = service.image?.includes(':')
          ? service.image.split(':').slice(0, -1).join(':')
          : service.image;
        deploymentConfig.custom_image = `${imageBase}:${imageTag}`;
      }

      // Only include custom_port if it differs from the default
      const portNum = parseInt(customPort, 10);
      if (portNum && portNum !== service.default_port) {
        deploymentConfig.custom_port = portNum;
      }

      await onDeploy(deploymentConfig);

      // Success - modal will be closed by parent
    } catch (error) {
      console.error('Deployment error:', error);
      setDeployError(error.message || 'Failed to deploy service');
      setIsDeploying(false);
    }
  };

  if (!service) return null;

  const iconIsUrl = isUrl(service.icon);

  return (
    <Modal isOpen={isOpen} onClose={onClose} size="large" closeOnOverlay={!isDeploying}>
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
          <h2>Deploy {service.name}</h2>
        </div>
        <button
          className={styles.modalCloseButton}
          onClick={onClose}
          aria-label="Close modal"
          disabled={isDeploying}
        >
          <X size={20} />
        </button>
      </div>

      <div className={styles.modalBody}>
        {/* Error Alert */}
        {deployError && (
          <div className="alert alert-error" style={{ marginBottom: 'var(--spacing-md)' }}>
            <AlertCircle className="alert-icon" size={16} />
            <div>
              <strong>Deployment Failed</strong>
              <p>{deployError}</p>
            </div>
          </div>
        )}

        {/* Service Name */}
        <FormGroup>
          <Label htmlFor="serviceName">Service Name</Label>
          <Input
            id="serviceName"
            type="text"
            value={serviceName}
            onChange={(e) => {
              setServiceName(e.target.value);
              if (validationErrors.serviceName) {
                setValidationErrors(prev => {
                  const newErrors = { ...prev };
                  delete newErrors.serviceName;
                  return newErrors;
                });
              }
            }}
            placeholder="Enter a name for this deployment"
            disabled={isDeploying}
            error={validationErrors.serviceName}
          />
          {validationErrors.serviceName && (
            <p className="text-error" style={{ fontSize: '12px', marginTop: 'var(--spacing-xs)' }}>
              {validationErrors.serviceName}
            </p>
          )}
        </FormGroup>

        {/* Docker Image & Host Port - side by side for compactness */}
        <div style={{ display: 'grid', gridTemplateColumns: '1fr 120px', gap: 'var(--spacing-md)' }}>
          <FormGroup>
            <Label htmlFor="imageTag">Docker Image</Label>
            <div className="image-tag-input">
              <span className="image-name-readonly">
                {service.image?.includes(':')
                  ? service.image.split(':').slice(0, -1).join(':')
                  : service.image}:
              </span>
              <Input
                id="imageTag"
                type="text"
                value={imageTag}
                onChange={(e) => setImageTag(e.target.value)}
                placeholder="latest"
                disabled={isDeploying}
                className="image-tag-field"
              />
            </div>
          </FormGroup>

          <FormGroup>
            <Label htmlFor="customPort">Host Port</Label>
            <Input
              id="customPort"
              type="number"
              value={customPort}
              onChange={(e) => setCustomPort(e.target.value)}
              placeholder={service.default_port ? String(service.default_port) : '8080'}
              disabled={isDeploying}
              min="1"
              max="65535"
            />
          </FormGroup>
        </div>

        {/* Environment Variables - filter out tag/port vars that are shown above */}
        {(() => {
          const filteredEnvVars = service.env_vars?.filter(envVar => {
            const upperName = envVar.name.toUpperCase();
            // Skip tag variables (already shown in Docker Image field)
            if (upperName.endsWith('_TAG') || upperName === 'TAG') return false;
            // Skip port variables (already shown in Host Port field)
            if (upperName.endsWith('_PORT') || upperName === 'PORT') return false;
            // Skip BASE_URL (auto-injected by server)
            if (upperName === 'BASE_URL') return false;
            return true;
          }) || [];

          if (filteredEnvVars.length === 0) return null;

          return (
            <div className="deploy-env-vars-section">
              <h3 style={{ marginBottom: 'var(--spacing-sm)' }}>Environment Variables</h3>
              {filteredEnvVars.map((envVar) => (
                <FormGroup key={envVar.name}>
                  <Label htmlFor={envVar.name}>
                    {envVar.label || envVar.name}
                    {envVar.required && <span className="text-error"> *</span>}
                  </Label>
                  <Input
                    id={envVar.name}
                    type={envVar.type === 'password' ? 'password' : 'text'}
                    value={envVars[envVar.name] || ''}
                    onChange={(e) => handleEnvVarChange(envVar.name, e.target.value)}
                    placeholder={envVar.default || ''}
                    disabled={isDeploying}
                    error={validationErrors[envVar.name]}
                  />
                  {validationErrors[envVar.name] && (
                    <p className="text-error" style={{ fontSize: '12px', marginTop: 'var(--spacing-xs)' }}>
                      {validationErrors[envVar.name]}
                    </p>
                  )}
                </FormGroup>
              ))}
            </div>
          );
        })()}

        {/* Options - inline checkbox */}
        <Checkbox
          id="autoStart"
          label="Start container after deployment"
          checked={autoStart}
          onChange={(e) => setAutoStart(e.target.checked)}
          disabled={isDeploying}
        />
      </div>

      <div className={styles.modalFooter}>
        <button
          type="button"
          className="btn btn-secondary"
          onClick={onClose}
          disabled={isDeploying}
        >
          CANCEL
        </button>
        <button
          type="button"
          className="btn btn-success"
          onClick={handleDeploy}
          disabled={isDeploying}
        >
          {isDeploying ? 'DEPLOYING...' : 'DEPLOY'}
        </button>
      </div>
    </Modal>
  );
}

DeployServiceModal.propTypes = {
  isOpen: PropTypes.bool.isRequired,
  onClose: PropTypes.func.isRequired,
  service: PropTypes.shape({
    id: PropTypes.string.isRequired,
    name: PropTypes.string.isRequired,
    icon: PropTypes.string,
    image: PropTypes.string,
    default_port: PropTypes.number,
    env_vars: PropTypes.arrayOf(PropTypes.shape({
      name: PropTypes.string.isRequired,
      description: PropTypes.string,
      required: PropTypes.bool,
      default: PropTypes.string
    }))
  }),
  onDeploy: PropTypes.func.isRequired
};
