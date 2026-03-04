/**
 * EditRouteModal Component - Add/Edit route form
 *
 * Features:
 * - Form fields for all route properties
 * - Validation (path, target URL, required fields)
 * - Save/Cancel buttons
 * - Test Connection button (simulated)
 * - Health check configuration with path, timeout, interval, and expected status codes
 * - Works for both adding new routes and editing existing ones
 */

import { useState, useEffect } from 'react';
import PropTypes from 'prop-types';
import { Check, X, Loader, Route, ChevronDown, ChevronRight, Activity } from 'lucide-react';
import { DetailsModal } from './DetailsModal';
import styles from './EditRouteModal.module.css';

/**
 * EditRouteModal Component
 *
 * @param {object} props - Component props
 * @param {boolean} props.isOpen - Whether the modal is open
 * @param {function} props.onClose - Callback when modal closes
 * @param {function} props.onSave - Callback when saved (receives route data)
 * @param {object} [props.route] - Route to edit (null for new route)
 */
export function EditRouteModal({
  isOpen,
  onClose,
  onSave,
  route = null
}) {
  const isEditing = route !== null;

  // Form state
  const [formData, setFormData] = useState({
    routeId: '',
    appId: '',
    pathBase: '',
    to: '',
    scopes: [],
    healthCheckPath: '',
    healthCheckTimeout: '',
    healthCheckInterval: '',
    expectedStatusCodes: '',
    persistCookies: false
  });

  const [errors, setErrors] = useState({});
  const [isTestingConnection, setIsTestingConnection] = useState(false);
  const [testResult, setTestResult] = useState(null);
  const [isTestingHealth, setIsTestingHealth] = useState(false);
  const [healthTestResult, setHealthTestResult] = useState(null);
  const [showAdvancedHealth, setShowAdvancedHealth] = useState(false);

  // Initialize form data when route changes
  useEffect(() => {
    if (route) {
      setFormData({
        routeId: route.routeId || '',
        appId: route.appId || '',
        pathBase: route.pathBase || '',
        to: route.to || '',
        scopes: route.scopes || [],
        healthCheckPath: route.healthCheckPath || '',
        healthCheckTimeout: route.healthCheckTimeout || '',
        healthCheckInterval: route.healthCheckInterval || '',
        expectedStatusCodes: route.expectedStatusCodes?.join(', ') || '',
        persistCookies: route.persistCookies || false
      });
      // Show advanced section if any advanced fields are set
      if (route.healthCheckTimeout || route.healthCheckInterval || route.expectedStatusCodes?.length) {
        setShowAdvancedHealth(true);
      }
    } else {
      // Reset form for new route
      setFormData({
        routeId: '',
        appId: '',
        pathBase: '',
        to: '',
        scopes: [],
        healthCheckPath: '',
        healthCheckTimeout: '',
        healthCheckInterval: '',
        expectedStatusCodes: '',
        persistCookies: false
      });
      setShowAdvancedHealth(false);
    }
    setErrors({});
    setTestResult(null);
    setHealthTestResult(null);
  }, [route, isOpen]);

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

  // Validation
  const validate = () => {
    const newErrors = {};

    // Application name is required
    if (!formData.appId.trim()) {
      newErrors.appId = 'Application name is required';
    }

    // Path is required and must start with /
    if (!formData.pathBase.trim()) {
      newErrors.pathBase = 'Path is required';
    } else if (!formData.pathBase.startsWith('/')) {
      newErrors.pathBase = 'Path must start with /';
    }

    // Target is required and must be valid URL
    if (!formData.to.trim()) {
      newErrors.to = 'Target URL is required';
    } else {
      try {
        new URL(formData.to);
      } catch {
        newErrors.to = 'Target must be a valid URL (e.g., http://localhost:3000)';
      }
    }

    // Health check path must start with / if provided
    if (formData.healthCheckPath && formData.healthCheckPath.trim()) {
      if (!formData.healthCheckPath.startsWith('/')) {
        newErrors.healthCheckPath = 'Health check path must start with /';
      }
    }

    // Validate timeout format if provided
    if (formData.healthCheckTimeout && formData.healthCheckTimeout.trim()) {
      if (!/^\d+[smh]?$/.test(formData.healthCheckTimeout.trim())) {
        newErrors.healthCheckTimeout = 'Invalid format (e.g., 5s, 10s, 1m)';
      }
    }

    // Validate interval format if provided
    if (formData.healthCheckInterval && formData.healthCheckInterval.trim()) {
      if (!/^\d+[smh]?$/.test(formData.healthCheckInterval.trim())) {
        newErrors.healthCheckInterval = 'Invalid format (e.g., 30s, 1m)';
      }
    }

    // Validate expected status codes if provided
    if (formData.expectedStatusCodes && formData.expectedStatusCodes.trim()) {
      const codes = formData.expectedStatusCodes.split(',').map(s => s.trim());
      const invalidCodes = codes.filter(c => isNaN(parseInt(c, 10)) || parseInt(c, 10) < 100 || parseInt(c, 10) > 599);
      if (invalidCodes.length > 0) {
        newErrors.expectedStatusCodes = 'Invalid status codes (must be 100-599)';
      }
    }

    setErrors(newErrors);
    return Object.keys(newErrors).length === 0;
  };

  // Handle test connection (simulated)
  const handleTestConnection = async () => {
    if (!formData.to.trim()) {
      setErrors(prev => ({ ...prev, to: 'Enter a target URL to test' }));
      return;
    }

    setIsTestingConnection(true);
    setTestResult(null);

    // Simulate connection test (2 second delay)
    await new Promise(resolve => setTimeout(resolve, 2000));

    // Simulate random success/failure
    const success = Math.random() > 0.3;
    setTestResult({
      success,
      message: success
        ? 'Connection successful'
        : 'Connection failed: Unable to reach target'
    });

    setIsTestingConnection(false);
  };

  // Handle test health check
  const handleTestHealthCheck = async () => {
    if (!formData.to.trim()) {
      setErrors(prev => ({ ...prev, to: 'Enter a target URL first' }));
      return;
    }

    setIsTestingHealth(true);
    setHealthTestResult(null);

    // Build the probe URL from target + path
    const path = formData.healthCheckPath || '/';
    let probeUrl;
    try {
      const targetUrl = new URL(formData.to);
      targetUrl.pathname = path;
      probeUrl = targetUrl.toString();
    } catch {
      setHealthTestResult({ success: false, message: 'Invalid target URL' });
      setIsTestingHealth(false);
      return;
    }

    // Simulate connection test (2 second delay)
    await new Promise(resolve => setTimeout(resolve, 2000));

    // Simulate random success/failure
    const success = Math.random() > 0.3;
    setHealthTestResult({
      success,
      message: success
        ? `Health check successful (${probeUrl})`
        : `Health check failed: Unable to reach ${probeUrl}`
    });

    setIsTestingHealth(false);
  };

  // Get computed probe URL
  const getProbeUrl = () => {
    if (!formData.to) return null;
    try {
      const targetUrl = new URL(formData.to);
      targetUrl.pathname = formData.healthCheckPath || '/';
      return targetUrl.toString();
    } catch {
      return null;
    }
  };

  // Handle save
  const handleSave = () => {
    if (!validate()) {
      return;
    }

    // Convert form data to API format
    const saveData = {
      ...formData,
      // Parse expected status codes from comma-separated string to array of integers
      expectedStatusCodes: formData.expectedStatusCodes
        ? formData.expectedStatusCodes.split(',').map(s => parseInt(s.trim(), 10)).filter(n => !isNaN(n))
        : []
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
      >
        {isEditing ? 'UPDATE ROUTE' : 'CREATE ROUTE'}
      </button>
    </>
  );

  return (
    <DetailsModal
      isOpen={isOpen}
      onClose={onClose}
      icon={<Route size={32} />}
      title={isEditing ? 'EDIT ROUTE' : 'ADD NEW ROUTE'}
      subtitle={isEditing && formData.routeId ? formData.routeId : undefined}
      footer={footer}
      size="medium"
    >
      <form className="terminal-form">
        {/* Application Name */}
        <div className="form-group">
          <label htmlFor="appId">
            APPLICATION <span className="text-error">*</span>
          </label>
          <input
            type="text"
            id="appId"
            className={`input ${errors.appId ? 'input-error' : ''}`}
            value={formData.appId}
            onChange={(e) => handleChange('appId', e.target.value)}
            placeholder="e.g., Grafana"
            aria-required="true"
            aria-invalid={!!errors.appId}
          />
          {errors.appId && (
            <span className="form-error">{errors.appId}</span>
          )}
        </div>

        {/* Path */}
        <div className="form-group">
          <label htmlFor="pathBase">
            PATH BASE <span className="text-error">*</span>
          </label>
          <input
            type="text"
            id="pathBase"
            className={`input ${errors.pathBase ? 'input-error' : ''}`}
            value={formData.pathBase}
            onChange={(e) => handleChange('pathBase', e.target.value)}
            placeholder="/grafana"
            aria-required="true"
            aria-invalid={!!errors.pathBase}
          />
          {errors.pathBase && (
            <span className="form-error">{errors.pathBase}</span>
          )}
        </div>

        {/* Target URL */}
        <div className="form-group">
          <label htmlFor="to">
            TARGET URL <span className="text-error">*</span>
          </label>
          <div className={styles.inputGroup}>
            <input
              type="url"
              id="to"
              className={`input ${styles.inputGroupInput} ${errors.to ? 'input-error' : ''}`}
              value={formData.to}
              onChange={(e) => handleChange('to', e.target.value)}
              placeholder="http://localhost:3000"
              aria-required="true"
              aria-invalid={!!errors.to}
            />
            <button
              type="button"
              className="btn btn-secondary"
              onClick={handleTestConnection}
              disabled={isTestingConnection}
            >
              {isTestingConnection ? (
                <>
                  <Loader size={14} className={styles.spin} />
                  TESTING...
                </>
              ) : (
                'TEST'
              )}
            </button>
          </div>
          {errors.to && (
            <span className="form-error">{errors.to}</span>
          )}
          <div className={`${styles.testResultContainer} ${!testResult ? styles.hidden : ''}`}>
            {testResult && (
              <div className={`${styles.testResult} ${testResult.success ? styles.success : styles.error}`}>
                {testResult.success ? (
                  <Check size={14} />
                ) : (
                  <X size={14} />
                )}
                <span>{testResult.message}</span>
              </div>
            )}
          </div>
        </div>

        {/* Health Check Configuration */}
        <div className={`form-group ${styles.healthCheckSection}`}>
          <label htmlFor="healthCheckPath">
            <Activity size={14} className={styles.sectionIcon} />
            HEALTH CHECK PATH
          </label>
          <div className={styles.inputGroup}>
            <input
              type="text"
              id="healthCheckPath"
              className={`input ${styles.inputGroupInput} ${errors.healthCheckPath ? 'input-error' : ''}`}
              value={formData.healthCheckPath}
              onChange={(e) => handleChange('healthCheckPath', e.target.value)}
              placeholder="/health"
              aria-invalid={!!errors.healthCheckPath}
            />
            <button
              type="button"
              className="btn btn-secondary"
              onClick={handleTestHealthCheck}
              disabled={isTestingHealth || !formData.to.trim()}
            >
              {isTestingHealth ? (
                <>
                  <Loader size={14} className={styles.spin} />
                  TESTING...
                </>
              ) : (
                'TEST'
              )}
            </button>
          </div>
          {errors.healthCheckPath && (
            <span className="form-error">{errors.healthCheckPath}</span>
          )}

          {/* Probe URL Display */}
          {getProbeUrl() && (
            <div className={styles.probeUrlDisplay}>
              <span className={styles.probeUrlLabel}>Probe URL:</span>
              <code className={styles.probeUrl}>{getProbeUrl()}</code>
              {route?.healthInfo && (
                <span className={`${styles.healthBadge} ${styles[route.healthInfo.status]}`}>
                  {route.healthInfo.status?.toUpperCase() || 'UNKNOWN'}
                </span>
              )}
            </div>
          )}

          {/* Test Result */}
          <div className={`${styles.testResultContainer} ${!healthTestResult ? styles.hidden : ''}`}>
            {healthTestResult && (
              <div className={`${styles.testResult} ${healthTestResult.success ? styles.success : styles.error}`}>
                {healthTestResult.success ? (
                  <Check size={14} />
                ) : (
                  <X size={14} />
                )}
                <span>{healthTestResult.message}</span>
              </div>
            )}
          </div>

          {/* Advanced Health Check Toggle */}
          <button
            type="button"
            className={styles.advancedToggle}
            onClick={() => setShowAdvancedHealth(!showAdvancedHealth)}
          >
            {showAdvancedHealth ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
            Advanced Health Check Settings
          </button>

          {/* Advanced Health Check Settings */}
          {showAdvancedHealth && (
            <div className={styles.advancedSection}>
              {/* Expected Status Codes */}
              <div className="form-group">
                <label htmlFor="expectedStatusCodes">
                  EXPECTED STATUS CODES
                </label>
                <input
                  type="text"
                  id="expectedStatusCodes"
                  className={`input ${errors.expectedStatusCodes ? 'input-error' : ''}`}
                  value={formData.expectedStatusCodes}
                  onChange={(e) => handleChange('expectedStatusCodes', e.target.value)}
                  placeholder="200, 204, 301"
                />
                {errors.expectedStatusCodes && (
                  <span className="form-error">{errors.expectedStatusCodes}</span>
                )}
                <p className={`text-secondary ${styles.helperText}`}>
                  Comma-separated list of valid HTTP status codes (default: 200-299)
                </p>
              </div>

              {/* Timeout */}
              <div className="form-group">
                <label htmlFor="healthCheckTimeout">
                  TIMEOUT
                </label>
                <input
                  type="text"
                  id="healthCheckTimeout"
                  className={`input ${errors.healthCheckTimeout ? 'input-error' : ''}`}
                  value={formData.healthCheckTimeout}
                  onChange={(e) => handleChange('healthCheckTimeout', e.target.value)}
                  placeholder="5s"
                />
                {errors.healthCheckTimeout && (
                  <span className="form-error">{errors.healthCheckTimeout}</span>
                )}
                <p className={`text-secondary ${styles.helperText}`}>
                  Request timeout (e.g., 5s, 10s, 1m)
                </p>
              </div>

              {/* Check Interval */}
              <div className="form-group">
                <label htmlFor="healthCheckInterval">
                  CHECK INTERVAL
                </label>
                <input
                  type="text"
                  id="healthCheckInterval"
                  className={`input ${errors.healthCheckInterval ? 'input-error' : ''}`}
                  value={formData.healthCheckInterval}
                  onChange={(e) => handleChange('healthCheckInterval', e.target.value)}
                  placeholder="30s"
                />
                {errors.healthCheckInterval && (
                  <span className="form-error">{errors.healthCheckInterval}</span>
                )}
                <p className={`text-secondary ${styles.helperText}`}>
                  How often to check health (e.g., 30s, 1m, 5m)
                </p>
              </div>
            </div>
          )}
        </div>

        {/* Scopes */}
        <div className="form-group">
          <label>
            ACCESS SCOPES
          </label>
          <input
            type="text"
            className="input"
            id="scopes"
            value={formData.scopes.join(', ')}
            onChange={(e) => handleChange('scopes', e.target.value.split(',').map(s => s.trim()).filter(s => s))}
            placeholder="read, write, admin"
          />
          <p className={`text-secondary ${styles.helperText}`}>
            Comma-separated list of scopes
          </p>
        </div>

        {/* Session Persistence Toggle */}
        <div className="form-group">
          <div className={styles.toggleRow}>
            <label className="toggle" htmlFor="persistCookies">
              <input
                type="checkbox"
                id="persistCookies"
                checked={formData.persistCookies}
                onChange={(e) => handleChange('persistCookies', e.target.checked)}
              />
              <span className="toggle-slider"></span>
            </label>
            <div className={styles.toggleLabel}>
              <span>SESSION PERSISTENCE</span>
              <p className={`text-secondary ${styles.helperText}`}>
                Store cookies for mobile app webview sessions
              </p>
            </div>
          </div>
        </div>

        {/* Warning Box */}
        {isEditing && (
          <div className={`alert alert-warning ${styles.alert}`}>
            <span className="alert-icon">⚠</span>
            <div>
              <strong>WARNING:</strong> Changing the path base or target URL may affect active connections.
            </div>
          </div>
        )}
      </form>
    </DetailsModal>
  );
}

EditRouteModal.propTypes = {
  isOpen: PropTypes.bool.isRequired,
  onClose: PropTypes.func.isRequired,
  onSave: PropTypes.func.isRequired,
  route: PropTypes.shape({
    id: PropTypes.string,
    routeId: PropTypes.string,
    appId: PropTypes.string,
    pathBase: PropTypes.string,
    to: PropTypes.string,
    scopes: PropTypes.arrayOf(PropTypes.string),
    status: PropTypes.string,
    requiresAuth: PropTypes.bool,
    healthCheckPath: PropTypes.string,
    healthCheckTimeout: PropTypes.string,
    healthCheckInterval: PropTypes.string,
    expectedStatusCodes: PropTypes.arrayOf(PropTypes.number),
    persistCookies: PropTypes.bool,
    healthInfo: PropTypes.shape({
      probeUrl: PropTypes.string,
      status: PropTypes.string,
      lastCheck: PropTypes.string,
      effectivePath: PropTypes.string,
      effectiveTimeout: PropTypes.string,
      effectiveInterval: PropTypes.string,
      effectiveCodes: PropTypes.arrayOf(PropTypes.number),
      configSource: PropTypes.string,
      lastError: PropTypes.string
    })
  })
};
