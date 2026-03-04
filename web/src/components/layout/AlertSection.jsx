import PropTypes from 'prop-types';

/**
 * Alert Type Definition
 * @typedef {Object} Alert
 * @property {string} id - Unique identifier for the alert
 * @property {'warning'|'error'|'success'|'info'} severity - Alert severity level
 * @property {string} message - Main alert message
 * @property {string} [strongText] - Bold text to emphasize (e.g., "ACTION REQUIRED:")
 * @property {Object} [link] - Optional link object
 * @property {string} link.text - Link text
 * @property {string} link.href - Link URL or route
 */

/**
 * AlertSection Component
 *
 * Container for critical system alerts and notifications. Displays alerts with
 * appropriate severity styling and optional action links. Alerts are shown in
 * terminal-style boxes with icons and formatted text.
 *
 * @component
 * @param {Object} props - Component props
 * @param {Alert[]} props.alerts - Array of alert objects to display
 * @returns {JSX.Element|null} Alert section or null if no alerts
 *
 * @example
 * const alerts = [
 *   {
 *     id: 'cert-expiry',
 *     severity: 'warning',
 *     strongText: 'ACTION REQUIRED:',
 *     message: '3 certificates expiring within 7 days.',
 *     link: { text: 'View details', href: '#certificates' }
 *   }
 * ];
 *
 * <AlertSection alerts={alerts} />
 */
const AlertSection = ({ alerts = [] }) => {
  // Don't render section if no alerts
  if (!alerts || alerts.length === 0) {
    return null;
  }

  /**
   * Get icon for alert severity
   * @param {string} severity - Alert severity level
   * @returns {string} Unicode icon character
   */
  const getAlertIcon = (severity) => {
    const icons = {
      warning: '⚠',
      error: '✕',
      success: '✓',
      info: 'ℹ',
    };
    return icons[severity] || icons.info;
  };

  return (
    <section className="alert-section">
      {alerts.map((alert) => (
        <div
          key={alert.id}
          className={`alert alert-${alert.severity}`}
          role="alert"
        >
          <span className="alert-icon" aria-hidden="true">
            {getAlertIcon(alert.severity)}
          </span>
          <div>
            {alert.strongText && <strong>{alert.strongText}</strong>}
            {alert.strongText ? ' ' : ''}
            {alert.message}
            {alert.link && (
              <a
                href={alert.link.href}
                className="link"
                style={{ marginLeft: 'var(--spacing-xs)' }}
              >
                {alert.link.text}
              </a>
            )}
          </div>
        </div>
      ))}
    </section>
  );
};

AlertSection.propTypes = {
  alerts: PropTypes.arrayOf(
    PropTypes.shape({
      id: PropTypes.string.isRequired,
      severity: PropTypes.oneOf(['warning', 'error', 'success', 'info']).isRequired,
      message: PropTypes.string.isRequired,
      strongText: PropTypes.string,
      link: PropTypes.shape({
        text: PropTypes.string.isRequired,
        href: PropTypes.string.isRequired,
      }),
    })
  ),
};

export default AlertSection;
