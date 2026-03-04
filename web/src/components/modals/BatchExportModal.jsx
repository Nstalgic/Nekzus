/**
 * BatchExportModal Component
 *
 * Modal for exporting multiple container configurations to a single Docker Compose file
 *
 * Features:
 * - Export options (sanitize secrets, include volumes/networks)
 * - Stack name input
 * - Loading state during export
 * - Display warnings for redacted sensitive data
 * - Partial success handling
 * - Automatic file download
 * - YAML preview before export
 */

import { useState } from 'react';
import PropTypes from 'prop-types';
import { Modal } from './Modal';

/**
 * BatchExportModal Component
 *
 * @param {object} props - Component props
 * @param {boolean} props.isOpen - Whether the modal is open
 * @param {function} props.onClose - Callback when modal closes
 * @param {Array} props.containers - Array of container objects to export
 * @param {function} props.onExport - Callback to perform export (returns promise with response)
 * @param {function} props.onPreview - Callback to generate preview (returns promise with response)
 */
export function BatchExportModal({
  isOpen,
  onClose,
  containers,
  onExport,
  onPreview
}) {
  const [isExporting, setIsExporting] = useState(false);
  const [exportComplete, setExportComplete] = useState(false);
  const [warnings, setWarnings] = useState([]);
  const [exportedFilename, setExportedFilename] = useState('');
  const [exportedEnvFilename, setExportedEnvFilename] = useState('');
  const [error, setError] = useState(null);
  const [stackName, setStackName] = useState('');

  // Preview state
  const [showPreview, setShowPreview] = useState(false);
  const [isLoadingPreview, setIsLoadingPreview] = useState(false);
  const [previewContent, setPreviewContent] = useState('');
  const [previewEnvContent, setPreviewEnvContent] = useState('');

  // Export options with defaults
  const [options, setOptions] = useState({
    sanitizeSecrets: true,
    includeVolumes: true,
    includeNetworks: true,
    downloadAsZip: false
  });

  // Generate default stack name from selected containers
  const getDefaultStackName = () => {
    if (containers.length === 0) return 'exported-stack';
    if (containers.length === 1) {
      return containers[0]?.name?.replace(/^\//, '') || 'container';
    }
    // Try to find common prefix
    const names = containers.map(c => c.name?.replace(/^\//, '') || 'container');
    const commonPrefix = findCommonPrefix(names);
    if (commonPrefix.length > 2) {
      return `${commonPrefix}-stack`;
    }
    return 'exported-stack';
  };

  // Find common prefix in array of strings
  const findCommonPrefix = (strings) => {
    if (strings.length === 0) return '';
    let prefix = strings[0];
    for (let i = 1; i < strings.length; i++) {
      while (strings[i].indexOf(prefix) !== 0) {
        prefix = prefix.substring(0, prefix.length - 1);
        if (prefix === '') return '';
      }
    }
    return prefix;
  };

  // Handle option change
  const handleOptionChange = (option) => {
    setOptions(prev => ({
      ...prev,
      [option]: !prev[option]
    }));
    // Clear preview when options change
    setShowPreview(false);
    setPreviewContent('');
    setPreviewEnvContent('');
  };

  // Handle preview generation
  const handlePreview = async () => {
    if (!onPreview) return;

    setIsLoadingPreview(true);
    setError(null);

    try {
      const containerIds = containers.map(c => c.id);
      const finalStackName = stackName.trim() || getDefaultStackName();

      const response = await onPreview(containerIds, {
        stack_name: finalStackName,
        sanitize_secrets: options.sanitizeSecrets,
        include_volumes: options.includeVolumes,
        include_networks: options.includeNetworks
      });

      setPreviewContent(response.content);
      setPreviewEnvContent(response.env_content || '');
      setShowPreview(true);
    } catch (err) {
      setError(err.message || 'Failed to generate preview');
    } finally {
      setIsLoadingPreview(false);
    }
  };

  // Handle export
  const handleExport = async () => {
    setIsExporting(true);
    setError(null);
    setWarnings([]);

    try {
      const containerIds = containers.map(c => c.id);
      const finalStackName = stackName.trim() || getDefaultStackName();

      const response = await onExport(containerIds, {
        stack_name: finalStackName,
        sanitize_secrets: options.sanitizeSecrets,
        include_volumes: options.includeVolumes,
        include_networks: options.includeNetworks,
        format: options.downloadAsZip ? 'zip' : 'json'
      });

      // Handle ZIP response (returned as blob)
      if (options.downloadAsZip && response instanceof Blob) {
        const url = URL.createObjectURL(response);
        const link = document.createElement('a');
        link.href = url;
        link.download = `${finalStackName}.zip`;
        link.click();
        URL.revokeObjectURL(url);
        setExportedFilename(`${finalStackName}.zip`);
        setExportComplete(true);
        return;
      }

      // Trigger compose file download
      const blob = new Blob([response.content], { type: 'text/yaml' });
      const url = URL.createObjectURL(blob);
      const link = document.createElement('a');
      link.href = url;
      link.download = response.filename;
      link.click();
      URL.revokeObjectURL(url);

      // Download env file if present
      if (response.env_content && response.env_filename) {
        const envBlob = new Blob([response.env_content], { type: 'text/plain' });
        const envUrl = URL.createObjectURL(envBlob);
        const envLink = document.createElement('a');
        envLink.href = envUrl;
        envLink.download = response.env_filename;
        // Small delay to ensure both downloads work
        setTimeout(() => {
          envLink.click();
          URL.revokeObjectURL(envUrl);
        }, 100);
        setExportedEnvFilename(response.env_filename);
      }

      // Update state with results
      setWarnings(response.warnings || []);
      setExportedFilename(response.filename);
      setExportComplete(true);

    } catch (err) {
      setError(err.message || 'Failed to export container configurations');
    } finally {
      setIsExporting(false);
    }
  };

  // Handle close and reset state
  const handleClose = () => {
    setIsExporting(false);
    setExportComplete(false);
    setWarnings([]);
    setExportedFilename('');
    setExportedEnvFilename('');
    setError(null);
    setStackName('');
    setShowPreview(false);
    setPreviewContent('');
    setPreviewEnvContent('');
    setIsLoadingPreview(false);
    setOptions({
      sanitizeSecrets: true,
      includeVolumes: true,
      includeNetworks: true,
      downloadAsZip: false
    });
    onClose();
  };

  if (!containers || containers.length === 0) return null;

  return (
    <Modal isOpen={isOpen} onClose={handleClose} size={showPreview ? 'large' : 'medium'}>
      <div className="confirmation-card">
        {/* Header */}
        <div className="confirmation-card-header">
          <span className="confirmation-icon">↗</span>
          <h3>BATCH EXPORT</h3>
        </div>

        {/* Body */}
        <div className="confirmation-card-body">
          {!exportComplete ? (
            <>
              <p className="confirmation-message">
                Export configuration for <strong>{containers.length} containers</strong> to a single Docker Compose file.
              </p>

              {/* Selected containers list */}
              <div className="batch-export-containers" style={{
                maxHeight: '120px',
                overflowY: 'auto',
                marginBottom: 'var(--spacing-md)',
                padding: 'var(--spacing-sm)',
                background: 'var(--bg-secondary)',
                border: 'var(--border-width) solid var(--border-dim)'
              }}>
                {containers.map(c => (
                  <div key={c.id} style={{
                    fontSize: '12px',
                    color: 'var(--text-secondary)',
                    padding: 'var(--spacing-xs) 0'
                  }}>
                    {c.name?.replace(/^\//, '') || c.id.substring(0, 12)}
                  </div>
                ))}
              </div>

              {/* Stack Name Input */}
              <form className="terminal-form">
                <div className="form-group">
                  <label className="form-label">STACK NAME</label>
                  <input
                    type="text"
                    className="text-input"
                    value={stackName}
                    onChange={(e) => setStackName(e.target.value)}
                    placeholder={getDefaultStackName()}
                    disabled={isExporting}
                  />
                  <p className="text-secondary" style={{ marginTop: 'var(--space-1)', fontSize: '11px' }}>
                    Used for the compose filename (e.g., {stackName || getDefaultStackName()}-compose.yml)
                  </p>
                </div>

                <div className="form-group">
                  <label className="checkbox-label">
                    <input
                      type="checkbox"
                      className="checkbox"
                      checked={options.sanitizeSecrets}
                      onChange={() => handleOptionChange('sanitizeSecrets')}
                      disabled={isExporting}
                    />
                    <span>Sanitize sensitive environment variables</span>
                  </label>
                  <p className="text-secondary" style={{ marginTop: 'var(--space-1)', marginLeft: 'var(--space-6)' }}>
                    Recommended: Replaces passwords, API keys, and tokens with placeholders
                  </p>
                </div>

                <div className="form-group">
                  <label className="checkbox-label">
                    <input
                      type="checkbox"
                      className="checkbox"
                      checked={options.includeVolumes}
                      onChange={() => handleOptionChange('includeVolumes')}
                      disabled={isExporting}
                    />
                    <span>Include volume configurations</span>
                  </label>
                </div>

                <div className="form-group">
                  <label className="checkbox-label">
                    <input
                      type="checkbox"
                      className="checkbox"
                      checked={options.includeNetworks}
                      onChange={() => handleOptionChange('includeNetworks')}
                      disabled={isExporting}
                    />
                    <span>Include network configurations</span>
                  </label>
                </div>

                <div className="form-group" style={{ borderTop: 'var(--border-width) solid var(--border-dim)', paddingTop: 'var(--space-3)', marginTop: 'var(--space-3)' }}>
                  <label className="checkbox-label">
                    <input
                      type="checkbox"
                      className="checkbox"
                      checked={options.downloadAsZip}
                      onChange={() => handleOptionChange('downloadAsZip')}
                      disabled={isExporting || isLoadingPreview}
                    />
                    <span>Download as ZIP archive</span>
                  </label>
                  <p className="text-secondary" style={{ marginTop: 'var(--space-1)', marginLeft: 'var(--space-6)' }}>
                    Bundle compose file and .env into a single ZIP download
                  </p>
                </div>
              </form>

              {/* YAML Preview */}
              {showPreview && previewContent && (
                <div style={{ marginTop: 'var(--space-4)' }}>
                  <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 'var(--space-2)' }}>
                    <label className="form-label" style={{ margin: 0 }}>COMPOSE PREVIEW</label>
                    <button
                      type="button"
                      className="btn btn-ghost"
                      style={{ padding: 'var(--space-1) var(--space-2)', fontSize: 'var(--text-xs)' }}
                      onClick={() => setShowPreview(false)}
                    >
                      HIDE
                    </button>
                  </div>
                  <pre style={{
                    background: 'var(--bg-tertiary)',
                    border: 'var(--border-width) solid var(--border-dim)',
                    padding: 'var(--space-3)',
                    maxHeight: '300px',
                    overflow: 'auto',
                    fontSize: 'var(--text-xs)',
                    lineHeight: '1.4',
                    fontFamily: 'var(--font-mono)',
                    whiteSpace: 'pre',
                    margin: 0
                  }}>
                    {previewContent}
                  </pre>
                  {previewEnvContent && (
                    <>
                      <label className="form-label" style={{ marginTop: 'var(--space-3)', display: 'block' }}>.ENV PREVIEW</label>
                      <pre style={{
                        background: 'var(--bg-tertiary)',
                        border: 'var(--border-width) solid var(--border-dim)',
                        padding: 'var(--space-3)',
                        maxHeight: '150px',
                        overflow: 'auto',
                        fontSize: 'var(--text-xs)',
                        lineHeight: '1.4',
                        fontFamily: 'var(--font-mono)',
                        whiteSpace: 'pre',
                        margin: 0
                      }}>
                        {previewEnvContent}
                      </pre>
                    </>
                  )}
                </div>
              )}

              {/* Error Display */}
              {error && (
                <div className="alert alert-error">
                  <span className="alert-icon">!</span>
                  <span>{error}</span>
                </div>
              )}
            </>
          ) : (
            <>
              {/* Success State */}
              <div className="alert alert-success" style={{ marginBottom: 'var(--space-4)' }}>
                <span className="alert-icon">✓</span>
                <div>
                  <p style={{ marginBottom: 'var(--space-2)' }}>
                    {containers.length} containers exported successfully!
                  </p>
                  <code>{exportedFilename}</code>
                  {exportedEnvFilename && (
                    <>
                      <br />
                      <code>{exportedEnvFilename}</code>
                    </>
                  )}
                </div>
              </div>

              {/* Env file notice */}
              {exportedEnvFilename && (
                <div className="alert alert-info" style={{ marginBottom: 'var(--space-4)' }}>
                  <span className="alert-icon">i</span>
                  <div>
                    <strong>ENVIRONMENT FILE</strong>
                    <p style={{ marginTop: 'var(--space-1)' }}>
                      A <code>.env.example</code> file was created with placeholders for sensitive variables.
                      Rename it to <code>.env</code> and fill in the actual values before deploying.
                    </p>
                  </div>
                </div>
              )}

              {/* Warnings Display */}
              {warnings.length > 0 && (
                <div className="alert alert-warning">
                  <span className="alert-icon">⚠</span>
                  <div>
                    <strong>NOTICES</strong>
                    <ul style={{ marginTop: 'var(--space-2)', paddingLeft: 'var(--space-4)', maxHeight: '150px', overflowY: 'auto' }}>
                      {warnings.map((warning, idx) => (
                        <li key={idx} style={{ marginBottom: 'var(--space-1)', fontSize: '12px' }}>{warning}</li>
                      ))}
                    </ul>
                  </div>
                </div>
              )}
            </>
          )}
        </div>

        {/* Footer */}
        <div className="confirmation-card-actions">
          <button
            type="button"
            className="btn btn-secondary"
            onClick={handleClose}
            disabled={isExporting || isLoadingPreview}
          >
            {exportComplete ? 'CLOSE' : 'CANCEL'}
          </button>
          {!exportComplete && onPreview && !showPreview && (
            <button
              type="button"
              className="btn btn-secondary"
              onClick={handlePreview}
              disabled={isExporting || isLoadingPreview}
            >
              {isLoadingPreview ? 'LOADING...' : 'PREVIEW'}
            </button>
          )}
          {!exportComplete && (
            <button
              type="button"
              className="btn btn-primary"
              onClick={handleExport}
              disabled={isExporting || isLoadingPreview}
              autoFocus
            >
              {isExporting ? 'EXPORTING...' : `EXPORT ${containers.length} CONTAINERS`}
            </button>
          )}
        </div>
      </div>
    </Modal>
  );
}

BatchExportModal.propTypes = {
  isOpen: PropTypes.bool.isRequired,
  onClose: PropTypes.func.isRequired,
  containers: PropTypes.arrayOf(PropTypes.shape({
    id: PropTypes.string.isRequired,
    name: PropTypes.string,
    image: PropTypes.string
  })),
  onExport: PropTypes.func.isRequired,
  onPreview: PropTypes.func
};
