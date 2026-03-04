import PropTypes from 'prop-types';
import HealthItem from './HealthItem';

/**
 * HealthList component - Container for health status items
 *
 * Displays a list of system/service health indicators with consistent
 * formatting. Supports both simple badge indicators and complex content
 * like progress bars.
 *
 * @component
 * @example
 * const healthItems = [
 *   {
 *     id: 'auth',
 *     label: 'Authentication',
 *     badge: { variant: 'success', dot: true, filled: true, children: 'ONLINE' }
 *   },
 *   {
 *     id: 'certs',
 *     label: 'Certificates',
 *     badge: { variant: 'warning', filled: true, children: '⚠ 3 EXPIRING' }
 *   }
 * ];
 * <HealthList items={healthItems} />
 */
function HealthList({
  items,
  className = '',
  ...props
}) {
  if (!items || items.length === 0) {
    return null;
  }

  return (
    <div className={`health-list ${className}`} {...props}>
      {items.map((item) => (
        <HealthItem
          key={item.id}
          label={item.label}
          badge={item.badge}
          fullWidth={item.fullWidth}
        >
          {item.children}
        </HealthItem>
      ))}
    </div>
  );
}

HealthList.propTypes = {
  /** Array of health status items */
  items: PropTypes.arrayOf(
    PropTypes.shape({
      /** Unique identifier */
      id: PropTypes.string.isRequired,
      /** Health status label */
      label: PropTypes.string.isRequired,
      /** Badge configuration */
      badge: PropTypes.object,
      /** Custom children (e.g., progress bar) */
      children: PropTypes.node,
      /** Full width layout */
      fullWidth: PropTypes.bool
    })
  ).isRequired,
  /** Additional CSS classes */
  className: PropTypes.string
};

export default HealthList;
