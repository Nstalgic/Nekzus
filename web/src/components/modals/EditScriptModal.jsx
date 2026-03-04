/**
 * EditScriptModal Component - Edit existing script
 *
 * Features:
 * - Pre-filled form with existing script data
 * - Edit name, description, category, timeout
 * - Environment variables management
 * - Advanced settings (scopes, dry run command)
 * - Validation and error handling
 */

import { useState, useEffect } from 'react';
import PropTypes from 'prop-types';
import { FileCode, ChevronDown, ChevronRight, X } from 'lucide-react';
import { DetailsModal } from './DetailsModal';
import styles from './RegisterScriptModal.module.css';

/**
 * Format script type for display
 */
const formatScriptType = (type) => {
  const labels = {
    shell: 'Shell',
    python: 'Python',
    go_binary: 'Go'
  };
  return labels[type] || type;
};

/**
 * EditScriptModal Component
 *
 * @param {object} props - Component props
 * @param {boolean} props.isOpen - Whether the modal is open
 * @param {function} props.onClose - Callback when modal closes
 * @param {function} props.onSave - Callback when saved (receives script data)
 * @param {object} props.script - The script to edit
 */
export function EditScriptModal({ isOpen, onClose, onSave, script }) {
  // Form state
  const [formData, setFormData] = useState({
    name: '',
    category: '',
    description: '',
    timeoutSeconds: 300,
    environment: {},
    allowedScopes: [],
    dryRunCommand: ''
  });

  // UI state
  const [errors, setErrors] = useState({});
  const [showAdvanced, setShowAdvanced] = useState(false);
  const [envVars, setEnvVars] = useState([{ key: '', value: '' }]);
  const [isSaving, setIsSaving] = useState(false);

  // Category suggestions
  const categorySuggestions = ['maintenance', 'deployment', 'backup', 'monitoring', 'utility'];

  // Initialize form when modal opens or script changes
  useEffect(() => {
    if (isOpen && script) {
      initializeForm(script);
    }
  }, [isOpen, script]);

  // Initialize form with script data
  const initializeForm = (scriptData) => {
    setFormData({
      name: scriptData.name || '',
      category: scriptData.category || '',
      description: scriptData.description || '',
      timeoutSeconds: scriptData.timeout_seconds || scriptData.timeoutSeconds || 300,
      environment: scriptData.environment || {},
      allowedScopes: scriptData.allowed_scopes || scriptData.allowedScopes || [],
      dryRunCommand: scriptData.dry_run_command || scriptData.dryRunCommand || ''
    });

    // Convert environment object to array for editing
    const env = scriptData.environment || {};
    const envArray = Object.entries(env).map(([key, value]) => ({ key, value }));
    setEnvVars(envArray.length > 0 ? envArray : [{ key: '', value: '' }]);

    // Show advanced section if there are advanced settings
    const hasAdvanced = Object.keys(env).length > 0 ||
      (scriptData.allowed_scopes || scriptData.allowedScopes || []).length > 0 ||
      scriptData.dry_run_command || scriptData.dryRunCommand;
    setShowAdvanced(hasAdvanced);

    setErrors({});
  };

  // Handle input changes
  const handleChange = (field, value) => {
    setFormData(prev => ({ ...prev, [field]: value }));

    // Clear error for this field
    if (errors[field]) {
      setErrors(prev => {
        const newErrors = { ...prev };
        delete newErrors[field];
        return newErrors;
      });
    }
  };

  // Handle environment variable changes
  const handleEnvChange = (index, field, value) => {
    const newEnvVars = [...envVars];
    newEnvVars[index][field] = value;
    setEnvVars(newEnvVars);
  };

  // Add environment variable
  const handleAddEnvVar = () => {
    setEnvVars([...envVars, { key: '', value: '' }]);
  };

  // Remove environment variable
  const handleRemoveEnvVar = (index) => {
    const newEnvVars = envVars.filter((_, i) => i !== index);
    setEnvVars(newEnvVars.length > 0 ? newEnvVars : [{ key: '', value: '' }]);
  };

  // Validation
  const validate = () => {
    const newErrors = {};

    // Name is required
    if (!formData.name.trim()) {
      newErrors.name = 'Script name is required';
    }

    // Category is required
    if (!formData.category.trim()) {
      newErrors.category = 'Category is required';
    }

    // Timeout must be positive number
    if (formData.timeoutSeconds !== '' && formData.timeoutSeconds !== null) {
      const timeout = parseInt(formData.timeoutSeconds, 10);
      if (isNaN(timeout) || timeout <= 0) {
        newErrors.timeoutSeconds = 'Timeout must be a positive number';
      }
    }

    setErrors(newErrors);
    return Object.keys(newErrors).length === 0;
  };

  // Handle save
  const handleSave = async () => {
    if (!validate()) {
      return;
    }

    setIsSaving(true);

    try {
      // Build environment object from env vars
      const environment = {};
      envVars.forEach(({ key, value }) => {
        if (key.trim()) {
          environment[key.trim()] = value;
        }
      });

      // Prepare save data
      const saveData = {
        name: formData.name,
        category: formData.category,
        description: formData.description,
        timeout_seconds: parseInt(formData.timeoutSeconds, 10),
        environment,
        allowed_scopes: formData.allowedScopes,
        dry_run_command: formData.dryRunCommand
      };

      await onSave(script.id, saveData);
      onClose();
    } catch (error) {
      console.error('Error saving script:', error);
      setErrors({ submit: error.message || 'Failed to save script' });
    } finally {
      setIsSaving(false);
    }
  };

  const footer = (
    <>
      <button
        className="btn btn-secondary"
        onClick={onClose}
        type="button"
        disabled={isSaving}
      >
        CANCEL
      </button>
      <button
        className="btn btn-success"
        onClick={handleSave}
        type="button"
        disabled={isSaving}
      >
        {isSaving ? 'SAVING...' : 'SAVE CHANGES'}
      </button>
    </>
  );

  if (!script) {
    return null;
  }

  return (
    <DetailsModal
      isOpen={isOpen}
      onClose={onClose}
      icon={<FileCode size={32} />}
      title="EDIT SCRIPT"
      footer={footer}
      size="medium"
    >
      <form className="terminal-form">
        {/* Submit Error */}
        {errors.submit && (
          <div className="alert alert-error" style={{ marginBottom: 'var(--spacing-md)' }}>
            <span className="alert-icon">X</span>
            <div>
              <strong>ERROR:</strong> {errors.submit}
            </div>
          </div>
        )}

        {/* Script Path (read-only) */}
        <div className="form-group">
          <label>SCRIPT PATH</label>
          <div className={styles.scriptType}>
            <span className={styles.scriptTypeLabel}>Path:</span>
            <code className={styles.scriptTypeValue}>{script.script_path || script.scriptPath}</code>
          </div>
          <div className={styles.scriptType}>
            <span className={styles.scriptTypeLabel}>Type:</span>
            <code className={styles.scriptTypeValue}>{formatScriptType(script.script_type || script.scriptType)}</code>
          </div>
        </div>

        {/* Script Name */}
        <div className="form-group">
          <label htmlFor="edit-name">
            SCRIPT NAME <span className="text-error">*</span>
          </label>
          <input
            type="text"
            id="edit-name"
            className={`input ${errors.name ? 'input-error' : ''}`}
            value={formData.name}
            onChange={(e) => handleChange('name', e.target.value)}
            placeholder="e.g., Daily Backup"
            aria-required="true"
            aria-invalid={!!errors.name}
          />
          {errors.name && (
            <span className="form-error">{errors.name}</span>
          )}
        </div>

        {/* Category */}
        <div className="form-group">
          <label htmlFor="edit-category">
            CATEGORY <span className="text-error">*</span>
          </label>
          <input
            type="text"
            id="edit-category"
            list="edit-category-suggestions"
            className={`input ${errors.category ? 'input-error' : ''}`}
            value={formData.category}
            onChange={(e) => handleChange('category', e.target.value)}
            placeholder="e.g., maintenance"
            aria-required="true"
            aria-invalid={!!errors.category}
          />
          <datalist id="edit-category-suggestions">
            {categorySuggestions.map((cat) => (
              <option key={cat} value={cat} />
            ))}
          </datalist>
          {errors.category && (
            <span className="form-error">{errors.category}</span>
          )}
        </div>

        {/* Description */}
        <div className="form-group">
          <label htmlFor="edit-description">
            DESCRIPTION
          </label>
          <textarea
            id="edit-description"
            className="input"
            value={formData.description}
            onChange={(e) => handleChange('description', e.target.value)}
            placeholder="Optional description of what this script does"
            rows="3"
          />
        </div>

        {/* Timeout Seconds */}
        <div className="form-group">
          <label htmlFor="edit-timeoutSeconds">
            TIMEOUT SECONDS
          </label>
          <input
            type="number"
            id="edit-timeoutSeconds"
            className={`input ${errors.timeoutSeconds ? 'input-error' : ''}`}
            value={formData.timeoutSeconds}
            onChange={(e) => handleChange('timeoutSeconds', e.target.value)}
            placeholder="300"
            min="1"
            aria-invalid={!!errors.timeoutSeconds}
          />
          {errors.timeoutSeconds && (
            <span className="form-error">{errors.timeoutSeconds}</span>
          )}
          <p className={`text-secondary ${styles.helperText}`}>
            Maximum execution time in seconds (default: 300)
          </p>
        </div>

        {/* Advanced Settings Toggle */}
        <button
          type="button"
          className={styles.advancedToggle}
          onClick={() => setShowAdvanced(!showAdvanced)}
        >
          {showAdvanced ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
          Advanced Settings
        </button>

        {/* Advanced Settings */}
        {showAdvanced && (
          <div className={styles.advancedSection}>
            {/* Environment Variables */}
            <div className="form-group">
              <label>ENVIRONMENT VARIABLES</label>
              <div className={styles.envVarsList}>
                {envVars.map((env, index) => (
                  <div key={index} className={styles.envVarRow}>
                    <input
                      type="text"
                      className={`input ${styles.envVarKey}`}
                      value={env.key}
                      onChange={(e) => handleEnvChange(index, 'key', e.target.value)}
                      placeholder="KEY"
                    />
                    <input
                      type="text"
                      className={`input ${styles.envVarValue}`}
                      value={env.value}
                      onChange={(e) => handleEnvChange(index, 'value', e.target.value)}
                      placeholder="value"
                    />
                    <button
                      type="button"
                      className="btn btn-sm btn-error"
                      onClick={() => handleRemoveEnvVar(index)}
                      aria-label="Remove variable"
                    >
                      <X size={14} />
                    </button>
                  </div>
                ))}
              </div>
              <button
                type="button"
                className="btn btn-sm btn-secondary"
                onClick={handleAddEnvVar}
              >
                ADD VARIABLE
              </button>
            </div>

            {/* Allowed Scopes */}
            <div className="form-group">
              <label htmlFor="edit-allowedScopes">
                ALLOWED SCOPES
              </label>
              <input
                type="text"
                id="edit-allowedScopes"
                className="input"
                value={formData.allowedScopes.join(', ')}
                onChange={(e) => handleChange('allowedScopes', e.target.value.split(',').map(s => s.trim()).filter(s => s))}
                placeholder="read, write, admin"
              />
              <p className={`text-secondary ${styles.helperText}`}>
                Comma-separated list of required scopes
              </p>
            </div>

            {/* Dry Run Command */}
            <div className="form-group">
              <label htmlFor="edit-dryRunCommand">
                DRY RUN COMMAND
              </label>
              <input
                type="text"
                id="edit-dryRunCommand"
                className="input"
                value={formData.dryRunCommand}
                onChange={(e) => handleChange('dryRunCommand', e.target.value)}
                placeholder="--dry-run"
              />
              <p className={`text-secondary ${styles.helperText}`}>
                Optional flag/argument for validation-only mode
              </p>
            </div>
          </div>
        )}
      </form>
    </DetailsModal>
  );
}

EditScriptModal.propTypes = {
  isOpen: PropTypes.bool.isRequired,
  onClose: PropTypes.func.isRequired,
  onSave: PropTypes.func.isRequired,
  script: PropTypes.shape({
    id: PropTypes.string.isRequired,
    name: PropTypes.string,
    description: PropTypes.string,
    category: PropTypes.string,
    script_path: PropTypes.string,
    scriptPath: PropTypes.string,
    script_type: PropTypes.string,
    scriptType: PropTypes.string,
    timeout_seconds: PropTypes.number,
    timeoutSeconds: PropTypes.number,
    environment: PropTypes.object,
    allowed_scopes: PropTypes.arrayOf(PropTypes.string),
    allowedScopes: PropTypes.arrayOf(PropTypes.string),
    dry_run_command: PropTypes.string,
    dryRunCommand: PropTypes.string
  })
};
