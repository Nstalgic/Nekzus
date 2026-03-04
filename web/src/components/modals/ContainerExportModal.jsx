/**
 * ContainerExportModal Component
 *
 * Modal for exporting container configuration to Docker Compose format
 *
 * Features:
 * - Export options (sanitize secrets, include volumes/networks)
 * - Loading state during export
 * - Display warnings for redacted sensitive data
 * - Automatic file download
 * - YAML preview before export
 */

import { useState } from 'react';
import PropTypes from 'prop-types';
import { Modal } from './Modal';

/**
 * ContainerExportModal Component
 *
 * @param {object} props - Component props
 * @param {boolean} props.isOpen - Whether the modal is open
 * @param {function} props.onClose - Callback when modal closes
 * @param {object} props.container - Container object to export
 * @param {function} props.onExport - Callback to perform export (returns promise with response)
 * @param {function} props.onPreview - Callback to generate preview (returns promise with response)
 */
export function ContainerExportModal({
  isOpen,
  onClose,
  container,
  onExport,
  onPreview
}) {
  const [isExporting, setIsExporting] = useState(false);
  const [exportComplete, setExportComplete] = useState(false);
  const [warnings, setWarnings] = useState([]);
  const [exportedFilename, setExportedFilename] = useState('');
  const [exportedEnvFilename, setExportedEnvFilename] = useState('');
  const [error, setError] = useState(null);

  // Preview state
  const [showPreview, setShowPreview] = useState(false);
  const [isLoadingPreview, setIsLoadingPreview] = useState(false);
  const [previewContent, setPreviewContent] = useState('');
  const [previewEnvContent, setPreviewEnvContent] = useState('');

  // Export options with defaults
  const [options, setOptions] = useState({
    sanitizeSecrets: true,
    includeVolumes: true,
    includeNetworks: true
  });

  const displayName = container?.name?.replace(/^\//, '') || 'Unnamed Container';

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
      const response = await onPreview(container.id, {
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
      const response = await onExport(container.id, {
        sanitize_secrets: options.sanitizeSecrets,
        include_volumes: options.includeVolumes,
        include_networks: options.includeNetworks
      });

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
      setError(err.message || 'Failed to export container configuration');
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
    setShowPreview(false);
    setPreviewContent('');
    setPreviewEnvContent('');
    setIsLoadingPreview(false);
    setOptions({
      sanitizeSecrets: true,
      includeVolumes: true,
      includeNetworks: true
    });
    onClose();
  };

  if (!container) return null;

  return (
    <Modal isOpen={isOpen} onClose={handleClose} size={showPreview ? 'large' : 'medium'}>
      <div className="confirmation-card">
        {/* Header */}
        <div className="confirmation-card-header">
          <span className="confirmation-icon">↗</span>
          <h3>EXPORT CONTAINER</h3>
        </div>

        {/* Body */}
        <div className="confirmation-card-body">
          {!exportComplete ? (
            <>
              <p className="confirmation-message">
                Export configuration for <strong>{displayName}</strong> to a Docker Compose file.
              </p>

              {/* Export Options */}
              <form className="terminal-form">
                <div className="form-group">
                  <label className="checkbox-label">
                    <input
                      type="checkbox"
                      className="checkbox"
                      checked={options.sanitizeSecrets}
                      onChange={() => handleOptionChange('sanitizeSecrets')}
                      disabled={isExporting || isLoadingPreview}
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
                      disabled={isExporting || isLoadingPreview}
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
                      disabled={isExporting || isLoadingPreview}
                    />
                    <span>Include network configurations</span>
                  </label>
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
                  <p style={{ marginBottom: 'var(--space-2)' }}>Configuration exported successfully!</p>
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
                    <ul style={{ marginTop: 'var(--space-2)', paddingLeft: 'var(--space-4)' }}>
                      {warnings.map((warning, idx) => (
                        <li key={idx} style={{ marginBottom: 'var(--space-1)' }}>{warning}</li>
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
              {isExporting ? 'EXPORTING...' : 'EXPORT'}
            </button>
          )}
        </div>
      </div>
    </Modal>
  );
}

ContainerExportModal.propTypes = {
  isOpen: PropTypes.bool.isRequired,
  onClose: PropTypes.func.isRequired,
  container: PropTypes.shape({
    id: PropTypes.string.isRequired,
    name: PropTypes.string,
    image: PropTypes.string
  }),
  onExport: PropTypes.func.isRequired,
  onPreview: PropTypes.func
};
