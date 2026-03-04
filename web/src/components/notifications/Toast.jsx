import { useEffect, useState } from 'react';
import { errorDetails } from '../../utils/debug';

export default function Toast({ notification, onDismiss, duration = 5000 }) {
  const { id, severity, message, strongText, link, error } = notification;
  const [showDetails, setShowDetails] = useState(false);

  // Auto-dismiss after duration
  useEffect(() => {
    if (duration === null) return;

    const timer = setTimeout(() => {
      onDismiss(id);
    }, duration);

    return () => clearTimeout(timer);
  }, [id, duration, onDismiss]);

  const handleClose = () => {
    onDismiss(id);
  };

  const ariaLive = severity === 'error' || severity === 'warning' ? 'assertive' : 'polite';

  // Get error details if available and setting is enabled
  const details = error ? errorDetails.getDetails(error) : null;
  const hasDetails = details && (details.code || details.stack || details.response);

  return (
    <div
      className={`toast toast-${severity} toast-enter`}
      role="alert"
      aria-live={ariaLive}
    >
      <div className="toast-content">
        <div className="toast-message">
          {strongText && <strong>{strongText}</strong>}{' '}
          {message}
          {hasDetails && (
            <button
              className="toast-details-toggle"
              onClick={() => setShowDetails(!showDetails)}
              type="button"
              style={{
                marginLeft: '8px',
                background: 'none',
                border: 'none',
                color: 'inherit',
                textDecoration: 'underline',
                cursor: 'pointer',
                fontSize: 'inherit',
                fontFamily: 'inherit',
                opacity: 0.8,
              }}
            >
              {showDetails ? '[hide details]' : '[show details]'}
            </button>
          )}
        </div>

        {showDetails && details && (
          <div
            className="toast-error-details"
            style={{
              marginTop: '8px',
              padding: '8px',
              background: 'rgba(0, 0, 0, 0.2)',
              borderRadius: '4px',
              fontSize: '11px',
              fontFamily: 'var(--font-mono)',
              whiteSpace: 'pre-wrap',
              wordBreak: 'break-word',
              maxHeight: '150px',
              overflow: 'auto',
            }}
          >
            {details.code && <div>Code: {details.code}</div>}
            {details.message && <div>Message: {details.message}</div>}
            {details.response && (
              <div>Response: {JSON.stringify(details.response, null, 2)}</div>
            )}
            {details.stack && (
              <details style={{ marginTop: '4px' }}>
                <summary style={{ cursor: 'pointer' }}>Stack Trace</summary>
                <pre style={{ margin: '4px 0 0 0', fontSize: '10px' }}>
                  {details.stack}
                </pre>
              </details>
            )}
          </div>
        )}

        {link && (
          <a href={link.href} className="toast-link">
            {link.text}
          </a>
        )}
      </div>

      <button
        className="toast-close"
        onClick={handleClose}
        aria-label="Close notification"
        type="button"
      >
        ×
      </button>
    </div>
  );
}
