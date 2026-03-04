import PropTypes from 'prop-types';
import Badge from './Badge';

/**
 * HealthItem component - Individual health status row
 *
 * Displays a service/system component status with a label and
 * corresponding badge indicator. Optionally renders children
 * for custom content (e.g., progress bars).
 *
 * @component
 * @example
 * // Basic health status
 * <HealthItem
 *   label="Authentication"
 *   badge={{ variant: 'success', dot: true, filled: true, children: 'ONLINE' }}
 * />
 *
 * @example
 * // Health item with custom content
 * <HealthItem
 *   label="Memory"
 *   fullWidth
 * >
 *   <ProgressBar progress={38} text="6.2GB / 16GB (38%)" />
 * </HealthItem>
 */
function HealthItem({
  label,
  badge,
  children,
  fullWidth = false,
  className = '',
  ...props
}) {
  const healthItemClass = fullWidth
    ? `health-item health-item-full ${className}`
    : `health-item ${className}`;

  return (
    <div className={healthItemClass} {...props}>
      <span className="health-label">{label}</span>
      {badge && (
        <Badge
          variant={badge.variant}
          dot={badge.dot}
          filled={badge.filled}
          size={badge.size}
          role={badge.role || 'status'}
          aria-label={badge['aria-label'] || badge.ariaLabel}
        >
          {badge.children}
        </Badge>
      )}
      {children}
    </div>
  );
}

HealthItem.propTypes = {
  /** Health status label */
  label: PropTypes.string.isRequired,
  /** Badge configuration object */
  badge: PropTypes.shape({
    /** Badge variant (success, error, warning, info, primary) */
    variant: PropTypes.oneOf(['success', 'error', 'warning', 'info', 'primary']),
    /** Show dot indicator */
    dot: PropTypes.bool,
    /** Filled background */
    filled: PropTypes.bool,
    /** Badge size */
    size: PropTypes.oneOf(['sm', 'md']),
    /** ARIA role */
    role: PropTypes.string,
    /** ARIA label for accessibility */
    ariaLabel: PropTypes.string,
    /** Badge content */
    children: PropTypes.node
  }),
  /** Custom children (e.g., progress bar) */
  children: PropTypes.node,
  /** Full width layout for complex content */
  fullWidth: PropTypes.bool,
  /** Additional CSS classes */
  className: PropTypes.string
};

export default HealthItem;
