import PropTypes from 'prop-types';

/**
 * OverviewItem component for displaying a single metric
 *
 * @component
 * @example
 * // Basic metric
 * <OverviewItem label="Active Routes" value="23" />
 *
 * // Clickable metric
 * <OverviewItem label="Pending" value="5" link onClick={handleClick} />
 *
 * // Urgent metric
 * <OverviewItem label="Alerts" value="3" link urgent onClick={handleClick} />
 */
function OverviewItem({
  label,
  value,
  link = false,
  urgent = false,
  onClick,
  className = '',
  ...props
}) {
  const valueClassNames = [
    'overview-value',
    link && 'overview-link',
    urgent && 'urgent'
  ].filter(Boolean).join(' ');

  const handleClick = (e) => {
    if (onClick) {
      e.preventDefault();
      onClick(e);
    }
  };

  const ValueElement = link ? 'a' : 'span';
  const valueProps = link ? {
    href: '#',
    onClick: handleClick,
    role: 'button',
    tabIndex: 0,
    onKeyDown: (e) => {
      if ((e.key === 'Enter' || e.key === ' ') && onClick) {
        e.preventDefault();
        onClick(e);
      }
    }
  } : {};

  return (
    <div className={`overview-item ${className}`} {...props}>
      <span className="overview-label">{label}</span>
      <ValueElement className={valueClassNames} {...valueProps}>
        {value}
      </ValueElement>
    </div>
  );
}

OverviewItem.propTypes = {
  /** Metric label */
  label: PropTypes.string.isRequired,
  /** Metric value */
  value: PropTypes.oneOfType([PropTypes.string, PropTypes.number]).isRequired,
  /** Make value clickable */
  link: PropTypes.bool,
  /** Urgent state with pulse animation */
  urgent: PropTypes.bool,
  /** Click handler for link */
  onClick: PropTypes.func,
  /** Additional CSS classes */
  className: PropTypes.string
};

export default OverviewItem;
