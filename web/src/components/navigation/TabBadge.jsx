import PropTypes from 'prop-types';

/**
 * TabBadge Component - Notification badge for tabs
 *
 * Small circular badge displaying a count with severity-based styling.
 * Positioned at the top-right of the tab label.
 *
 * @example
 * ```jsx
 * <TabBadge count={7} severity="warning" />
 * <TabBadge count={3} severity="error" />
 * <TabBadge count={12} severity="info" />
 * ```
 *
 * @param {Object} props - Component props
 * @param {number} props.count - Badge count to display
 * @param {string} [props.severity] - Severity level for styling (warning/error/info)
 * @returns {JSX.Element} Tab badge
 */
const TabBadge = ({ count, severity = 'info' }) => {
  return (
    <span
      className="tab-badge"
      data-severity={severity}
      aria-hidden="true"
    >
      {count}
    </span>
  );
};

TabBadge.propTypes = {
  count: PropTypes.number.isRequired,
  severity: PropTypes.oneOf(['warning', 'error', 'info']),
};

export default TabBadge;
