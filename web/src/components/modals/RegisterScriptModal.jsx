/**
 * RegisterScriptModal Component - Register new script
 *
 * Features:
 * - Fetch available (unregistered) scripts from API
 * - Form fields for script metadata
 * - Auto-detect script type from path
 * - Category suggestions
 * - Environment variables management
 * - Advanced settings (scopes, dry run command)
 * - Validation and error handling
 */

import { useState, useEffect } from 'react';
import PropTypes from 'prop-types';
import { FileCode, ChevronDown, ChevronRight, X } from 'lucide-react';
import { DetailsModal } from './DetailsModal';
import CustomDropdown from '../forms/CustomDropdown';
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
 * RegisterScriptModal Component
 *
 * @param {object} props - Component props
 * @param {boolean} props.isOpen - Whether the modal is open
 * @param {function} props.onClose - Callback when modal closes
 * @param {function} props.onSave - Callback when saved (receives script data)
 */
export function RegisterScriptModal({ isOpen, onClose, onSave }) {
  // Form state
  const [formData, setFormData] = useState({
    name: '',
    scriptPath: '',
    category: '',
    description: '',
    timeoutSeconds: 300,
    environment: {},
    allowedScopes: [],
    dryRunCommand: ''
  });

  // Available scripts state
  const [availableScripts, setAvailableScripts] = useState([]);
  const [scriptsLoading, setScriptsLoading] = useState(true);
  const [scriptsError, setScriptsError] = useState(null);

  // UI state
  const [errors, setErrors] = useState({});
  const [detectedType, setDetectedType] = useState('');
  const [showAdvanced, setShowAdvanced] = useState(false);
  const [envVars, setEnvVars] = useState([{ key: '', value: '' }]);

  // Category suggestions
  const categorySuggestions = ['maintenance', 'deployment', 'backup', 'monitoring', 'utility'];

  // Fetch available scripts when modal opens
  useEffect(() => {
    if (isOpen) {
      fetchAvailableScripts();
      resetForm();
    }
  }, [isOpen]);

  // Reset form when modal closes
  const resetForm = () => {
    setFormData({
      name: '',
      scriptPath: '',
      category: '',
      description: '',
      timeoutSeconds: 300,
      environment: {},
      allowedScopes: [],
      dryRunCommand: ''
    });
    setErrors({});
    setDetectedType('');
    setShowAdvanced(false);
    setEnvVars([{ key: '', value: '' }]);
  };

  // Fetch available scripts from API
  const fetchAvailableScripts = async () => {
    try {
      setScriptsLoading(true);
      setScriptsError(null);

      const response = await fetch('/api/v1/scripts/available');
      if (!response.ok) {
        throw new Error('Failed to fetch available scripts');
      }

      const data = await response.json();
      setAvailableScripts(data.available || []);
    } catch (error) {
      console.error('Error fetching available scripts:', error);
      setScriptsError(error.message);
    } finally {
      setScriptsLoading(false);
    }
  };

  // Detect script type from path
  const detectScriptType = (path) => {
    if (!path) return '';

    if (path.endsWith('.sh')) return 'shell';
    if (path.endsWith('.py')) return 'python';
    // Go binaries typically don't have extension
    if (!path.includes('.')) return 'go_binary';

    return 'shell'; // Default
  };

  // Handle input changes
  const handleChange = (field, value) => {
    setFormData(prev => ({ ...prev, [field]: value }));

    // Auto-detect script type when path changes
    if (field === 'scriptPath') {
      const type = detectScriptType(value);
      setDetectedType(type);
    }

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
    setEnvVars(newEnvVars);
  };

  // Validation
  const validate = () => {
    const newErrors = {};

    // Name is required
    if (!formData.name.trim()) {
      newErrors.name = 'Script name is required';
    }

    // Path is required
    if (!formData.scriptPath.trim()) {
      newErrors.scriptPath = 'Script path is required';
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
  const handleSave = () => {
    if (!validate()) {
      return;
    }

    // Build environment object from env vars
    const environment = {};
    envVars.forEach(({ key, value }) => {
      if (key.trim()) {
        environment[key.trim()] = value;
      }
    });

    // Prepare save data
    const saveData = {
      ...formData,
      scriptType: detectedType,
      environment,
      timeoutSeconds: parseInt(formData.timeoutSeconds, 10)
    };

    onSave(saveData);
    onClose();
  };

  const footer = (
    <>
      <button
        className="btn btn-secondary"
        onClick={onClose}
        type="button"
      >
        CANCEL
      </button>
      <button
        className="btn btn-success"
        onClick={handleSave}
        type="button"
        disabled={scriptsLoading || scriptsError}
      >
        REGISTER
      </button>
    </>
  );

  return (
    <DetailsModal
      isOpen={isOpen}
      onClose={onClose}
      icon={<FileCode size={32} />}
      title="REGISTER SCRIPT"
      footer={footer}
      size="medium"
    >
      <form className="terminal-form">
        {/* Loading State */}
        {scriptsLoading && (
          <div className={styles.loadingState}>
            <p>Loading available scripts...</p>
          </div>
        )}

        {/* Error State */}
        {scriptsError && (
          <div className={styles.errorState}>
            <div className="alert alert-error">
              <span className="alert-icon">✕</span>
              <div>
                <strong>ERROR:</strong> Failed to load available scripts. {scriptsError}
              </div>
            </div>
            <button className="btn btn-primary" onClick={fetchAvailableScripts}>
              TRY AGAIN
            </button>
          </div>
        )}

        {/* Form Fields */}
        {!scriptsLoading && !scriptsError && (
          <>
            {/* No Scripts Available */}
            {availableScripts.length === 0 && (
              <div className={styles.emptyState}>
                <p className="text-secondary">
                  No unregistered scripts found. All available scripts have been registered.
                </p>
              </div>
            )}

            {/* Form */}
            {availableScripts.length > 0 && (
              <>
                {/* Script Name */}
                <div className="form-group">
                  <label htmlFor="name">
                    SCRIPT NAME <span className="text-error">*</span>
                  </label>
                  <input
                    type="text"
                    id="name"
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

                {/* Script Path */}
                <div className="form-group">
                  <label htmlFor="scriptPath">
                    SCRIPT PATH <span className="text-error">*</span>
                  </label>
                  <CustomDropdown
                    id="scriptPath"
                    options={availableScripts.map((script) => ({
                      value: script.path,
                      label: script.path
                    }))}
                    value={formData.scriptPath}
                    onChange={(value) => handleChange('scriptPath', value)}
                    placeholder="Select a script..."
                    className={errors.scriptPath ? 'input-error' : ''}
                  />
                  {errors.scriptPath && (
                    <span className="form-error">{errors.scriptPath}</span>
                  )}
                  {detectedType && (
                    <div className={styles.scriptType}>
                      <span className={styles.scriptTypeLabel}>Detected Type:</span>
                      <code className={styles.scriptTypeValue}>{formatScriptType(detectedType)}</code>
                    </div>
                  )}
                </div>

                {/* Category */}
                <div className="form-group">
                  <label htmlFor="category">
                    CATEGORY <span className="text-error">*</span>
                  </label>
                  <input
                    type="text"
                    id="category"
                    list="category-suggestions"
                    className={`input ${errors.category ? 'input-error' : ''}`}
                    value={formData.category}
                    onChange={(e) => handleChange('category', e.target.value)}
                    placeholder="e.g., maintenance"
                    aria-required="true"
                    aria-invalid={!!errors.category}
                  />
                  <datalist id="category-suggestions">
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
                  <label htmlFor="description">
                    DESCRIPTION
                  </label>
                  <textarea
                    id="description"
                    className="input"
                    value={formData.description}
                    onChange={(e) => handleChange('description', e.target.value)}
                    placeholder="Optional description of what this script does"
                    rows="3"
                  />
                </div>

                {/* Timeout Seconds */}
                <div className="form-group">
                  <label htmlFor="timeoutSeconds">
                    TIMEOUT SECONDS
                  </label>
                  <input
                    type="number"
                    id="timeoutSeconds"
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
                              disabled={envVars.length === 1}
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
                      <label htmlFor="allowedScopes">
                        ALLOWED SCOPES
                      </label>
                      <input
                        type="text"
                        id="allowedScopes"
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
                      <label htmlFor="dryRunCommand">
                        DRY RUN COMMAND
                      </label>
                      <input
                        type="text"
                        id="dryRunCommand"
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
              </>
            )}
          </>
        )}
      </form>
    </DetailsModal>
  );
}

RegisterScriptModal.propTypes = {
  isOpen: PropTypes.bool.isRequired,
  onClose: PropTypes.func.isRequired,
  onSave: PropTypes.func.isRequired
};
