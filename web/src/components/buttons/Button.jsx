import PropTypes from 'prop-types';

/**
 * Button component - Terminal-style button with WTFUtil aesthetic
 *
 * @component
 * @example
 * // Primary button
 * <Button variant="primary" onClick={handleClick}>Execute</Button>
 *
 * // Loading state
 * <Button variant="success" loading>Processing</Button>
 *
 * // Small button
 * <Button variant="secondary" size="sm">Edit</Button>
 */
function Button({
  variant = 'primary',
  size = 'default',
  loading = false,
  disabled = false,
  onClick,
  children,
  className = '',
  type = 'button',
  ...props
}) {
  const classNames = [
    'btn',
    variant && `btn-${variant}`,
    size === 'sm' && 'btn-sm',
    loading && 'loading',
    className
  ].filter(Boolean).join(' ');

  const handleClick = (e) => {
    if (!loading && !disabled && onClick) {
      onClick(e);
    }
  };

  return (
    <button
      type={type}
      className={classNames}
      onClick={handleClick}
      disabled={disabled || loading}
      aria-busy={loading}
      aria-disabled={disabled || loading}
      {...props}
    >
      {children}
    </button>
  );
}

Button.propTypes = {
  /** Button style variant */
  variant: PropTypes.oneOf(['primary', 'secondary', 'success', 'error']),
  /** Button size */
  size: PropTypes.oneOf(['sm', 'default']),
  /** Loading state shows spinner */
  loading: PropTypes.bool,
  /** Disabled state */
  disabled: PropTypes.bool,
  /** Click handler */
  onClick: PropTypes.func,
  /** Button content */
  children: PropTypes.node.isRequired,
  /** Additional CSS classes */
  className: PropTypes.string,
  /** HTML button type */
  type: PropTypes.oneOf(['button', 'submit', 'reset'])
};

export default Button;
