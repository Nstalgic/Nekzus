import PropTypes from 'prop-types';

/**
 * Badge component for status/tag display with terminal aesthetic
 *
 * @component
 * @example
 * // Basic badge
 * <Badge variant="success">ONLINE</Badge>
 *
 * // Filled badge with dot
 * <Badge variant="success" dot filled>ACTIVE</Badge>
 *
 * // Small badge
 * <Badge variant="primary" size="sm">READ</Badge>
 */
function Badge({
  variant = 'primary',
  dot = false,
  filled = false,
  size = 'default',
  children,
  className = '',
  ...props
}) {
  const classNames = [
    'badge',
    `badge-${variant}`,
    dot && 'badge-dot',
    filled && 'badge-filled',
    size === 'sm' && 'badge-sm',
    className
  ].filter(Boolean).join(' ');

  return (
    <span className={classNames} {...props}>
      {children}
    </span>
  );
}

Badge.propTypes = {
  /** Badge color variant */
  variant: PropTypes.oneOf(['primary', 'secondary', 'success', 'error', 'warning', 'info']),
  /** Show dot indicator before text */
  dot: PropTypes.bool,
  /** Filled background style */
  filled: PropTypes.bool,
  /** Badge size */
  size: PropTypes.oneOf(['sm', 'default']),
  /** Badge content */
  children: PropTypes.node.isRequired,
  /** Additional CSS classes */
  className: PropTypes.string
};

export default Badge;
