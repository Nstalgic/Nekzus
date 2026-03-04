/**
 * ExecuteScriptModal Component
 *
 * Modal for executing scripts with parameter input
 *
 * Features:
 * - Dynamic parameter form based on script definition
 * - Support for text, password, number, boolean, and select inputs
 * - Dry run option for validation without execution
 * - Form validation for required fields
 * - Loading state during execution
 * - Success/error feedback
 */

import { useState, useEffect } from 'react';
import PropTypes from 'prop-types';
import { X, AlertCircle, Play } from 'lucide-react';
import { Modal } from './Modal';
import { Input, Select, FormGroup, Label, Checkbox } from '../forms';
import { scriptsAPI } from '../../services/api';
import styles from './Modal.module.css';

/**
 * ExecuteScriptModal Component
 *
 * @param {object} props - Component props
 * @param {boolean} props.isOpen - Whether the modal is open
 * @param {function} props.onClose - Close callback
 * @param {object} props.script - Script object
 * @param {function} props.onExecutionComplete - Callback when execution completes
 */
export function ExecuteScriptModal({
  isOpen,
  onClose,
  script,
  onExecutionComplete
}) {
  const [parameters, setParameters] = useState({});
  const [isDryRun, setIsDryRun] = useState(false);
  const [isExecuting, setIsExecuting] = useState(false);
  const [executeError, setExecuteError] = useState(null);
  const [validationErrors, setValidationErrors] = useState({});

  // Initialize form when script changes
  useEffect(() => {
    if (script && script.parameters) {
      const initialParams = {};
      script.parameters.forEach(param => {
        if (param.default !== undefined && param.default !== null) {
          initialParams[param.name] = param.default;
        } else if (param.type === 'boolean') {
          initialParams[param.name] = false;
        } else {
          initialParams[param.name] = '';
        }
      });
      setParameters(initialParams);
      setValidationErrors({});
      setExecuteError(null);
    }
  }, [script]);

  // Reset form when modal closes
  useEffect(() => {
    if (!isOpen) {
      setIsExecuting(false);
      setExecuteError(null);
      setValidationErrors({});
      setIsDryRun(false);
    }
  }, [isOpen]);

  // Validate form
  const validateForm = () => {
    const errors = {};

    if (script?.parameters) {
      script.parameters.forEach(param => {
        const value = parameters[param.name];

        // Check required fields
        if (param.required) {
          if (param.type === 'boolean') {
            // Boolean fields are always valid (true/false)
          } else if (!value || (typeof value === 'string' && !value.trim())) {
            errors[param.name] = `${param.label || param.name} is required`;
          }
        }

        // Type-specific validation
        if (value && typeof value === 'string' && value.trim()) {
          if (param.type === 'number') {
            const num = parseFloat(value);
            if (isNaN(num)) {
              errors[param.name] = `${param.label || param.name} must be a number`;
            }
          }

          // Custom validation regex
          if (param.validation) {
            try {
              const regex = new RegExp(param.validation);
              if (!regex.test(value)) {
                errors[param.name] = `Invalid format for ${param.label || param.name}`;
              }
            } catch (e) {
              console.error('Invalid validation regex:', param.validation);
            }
          }
        }
      });
    }

    setValidationErrors(errors);
    return Object.keys(errors).length === 0;
  };

  // Handle parameter change
  const handleParameterChange = (name, value) => {
    setParameters(prev => ({
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

  // Handle execute
  const handleExecute = async () => {
    if (!validateForm()) {
      return;
    }

    setIsExecuting(true);
    setExecuteError(null);

    try {
      // Convert parameters to appropriate types
      const formattedParams = {};
      if (script?.parameters) {
        script.parameters.forEach(param => {
          const value = parameters[param.name];

          if (param.type === 'number') {
            formattedParams[param.name] = value ? parseFloat(value) : undefined;
          } else if (param.type === 'boolean') {
            formattedParams[param.name] = Boolean(value);
          } else {
            // text, password, select
            formattedParams[param.name] = value || undefined;
          }
        });
      }

      // Execute or dry-run
      let result;
      if (isDryRun) {
        result = await scriptsAPI.dryRun(script.id, formattedParams);
      } else {
        result = await scriptsAPI.execute(script.id, formattedParams);
      }

      // Success - notify parent
      if (onExecutionComplete) {
        onExecutionComplete(result);
      }

      // Close modal
      onClose();
    } catch (error) {
      console.error('Execution error:', error);
      setExecuteError(error.message || 'Failed to execute script');
      setIsExecuting(false);
    }
  };

  if (!script) return null;

  return (
    <Modal isOpen={isOpen} onClose={onClose} size="large" closeOnOverlay={!isExecuting}>
      <div className={styles.modalHeader}>
        <div className={styles.modalTitleSection}>
          <Play size={48} strokeWidth={2} />
          <h2>Execute Script</h2>
        </div>
        <button
          className={styles.modalCloseButton}
          onClick={onClose}
          aria-label="Close modal"
          disabled={isExecuting}
        >
          <X size={20} />
        </button>
      </div>

      <div className={styles.modalBody}>
        {/* Script Info */}
        <div style={{ marginBottom: 'var(--space-4)' }}>
          <h3 style={{ marginBottom: 'var(--space-2)' }}>{script.name}</h3>
          {script.description && (
            <p className="text-secondary" style={{ fontSize: '14px' }}>
              {script.description}
            </p>
          )}
        </div>

        {/* Error Alert */}
        {executeError && (
          <div className="alert alert-error" style={{ marginBottom: 'var(--space-4)' }}>
            <AlertCircle className="alert-icon" size={16} />
            <div>
              <strong>Execution Failed</strong>
              <p>{executeError}</p>
            </div>
          </div>
        )}

        {/* Parameters Form */}
        {script.parameters && script.parameters.length > 0 ? (
          <div style={{ marginBottom: 'var(--space-4)' }}>
            <h3 style={{ marginBottom: 'var(--space-3)' }}>Parameters</h3>
            {script.parameters.map((param) => (
              <FormGroup
                key={param.name}
                error={validationErrors[param.name]}
              >
                <Label htmlFor={param.name} required={param.required}>
                  {param.label || param.name}
                </Label>

                {/* Text input */}
                {param.type === 'text' && (
                  <Input
                    id={param.name}
                    type="text"
                    value={parameters[param.name] || ''}
                    onChange={(e) => handleParameterChange(param.name, e.target.value)}
                    placeholder={param.description || ''}
                    disabled={isExecuting}
                    error={validationErrors[param.name]}
                  />
                )}

                {/* Password input */}
                {param.type === 'password' && (
                  <Input
                    id={param.name}
                    type="password"
                    value={parameters[param.name] || ''}
                    onChange={(e) => handleParameterChange(param.name, e.target.value)}
                    placeholder={param.description || ''}
                    disabled={isExecuting}
                    error={validationErrors[param.name]}
                  />
                )}

                {/* Number input */}
                {param.type === 'number' && (
                  <Input
                    id={param.name}
                    type="number"
                    value={parameters[param.name] || ''}
                    onChange={(e) => handleParameterChange(param.name, e.target.value)}
                    placeholder={param.description || ''}
                    disabled={isExecuting}
                    error={validationErrors[param.name]}
                  />
                )}

                {/* Boolean input */}
                {param.type === 'boolean' && (
                  <Checkbox
                    id={param.name}
                    label={param.description || ''}
                    checked={Boolean(parameters[param.name])}
                    onChange={(e) => handleParameterChange(param.name, e.target.checked)}
                    disabled={isExecuting}
                  />
                )}

                {/* Select input */}
                {param.type === 'select' && param.options && (
                  <Select
                    id={param.name}
                    value={parameters[param.name] || ''}
                    onChange={(e) => handleParameterChange(param.name, e.target.value)}
                    options={param.options.map(opt => ({
                      value: typeof opt === 'string' ? opt : opt.value,
                      label: typeof opt === 'string' ? opt : opt.label
                    }))}
                    disabled={isExecuting}
                  />
                )}

                {param.description && param.type !== 'boolean' && (
                  <p className="text-secondary" style={{ fontSize: '12px', marginTop: 'var(--space-1)' }}>
                    {param.description}
                  </p>
                )}
              </FormGroup>
            ))}
          </div>
        ) : (
          <p className="text-secondary" style={{ marginBottom: 'var(--space-4)' }}>
            This script has no parameters.
          </p>
        )}

        {/* Execution Options */}
        <div style={{ marginBottom: 'var(--space-2)' }}>
          <h3 style={{ marginBottom: 'var(--space-3)' }}>Execution Options</h3>
          <Checkbox
            id="dryRun"
            label="Dry Run (validate parameters without executing)"
            checked={isDryRun}
            onChange={(e) => setIsDryRun(e.target.checked)}
            disabled={isExecuting}
          />
        </div>
      </div>

      <div className={styles.modalFooter}>
        <button
          type="button"
          className="btn btn-secondary"
          onClick={onClose}
          disabled={isExecuting}
        >
          CANCEL
        </button>
        <button
          type="button"
          className="btn btn-primary"
          onClick={handleExecute}
          disabled={isExecuting}
        >
          {isExecuting ? 'EXECUTING...' : isDryRun ? 'DRY RUN' : 'EXECUTE'}
        </button>
      </div>
    </Modal>
  );
}

ExecuteScriptModal.propTypes = {
  isOpen: PropTypes.bool.isRequired,
  onClose: PropTypes.func.isRequired,
  script: PropTypes.shape({
    id: PropTypes.string.isRequired,
    name: PropTypes.string.isRequired,
    description: PropTypes.string,
    parameters: PropTypes.arrayOf(PropTypes.shape({
      name: PropTypes.string.isRequired,
      label: PropTypes.string,
      type: PropTypes.oneOf(['text', 'password', 'number', 'boolean', 'select']).isRequired,
      required: PropTypes.bool,
      default: PropTypes.any,
      validation: PropTypes.string,
      description: PropTypes.string,
      options: PropTypes.arrayOf(
        PropTypes.oneOfType([
          PropTypes.string,
          PropTypes.shape({
            value: PropTypes.string.isRequired,
            label: PropTypes.string.isRequired
          })
        ])
      )
    }))
  }),
  onExecutionComplete: PropTypes.func.isRequired
};
