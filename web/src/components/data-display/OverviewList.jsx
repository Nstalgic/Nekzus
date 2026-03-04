import PropTypes from 'prop-types';
import OverviewItem from './OverviewItem';

/**
 * OverviewList component - Container for overview metrics
 *
 * @component
 * @example
 * const metrics = [
 *   { label: 'Active Routes', value: '23' },
 *   { label: 'Pending', value: '5', link: true, urgent: true, onClick: handleClick }
 * ];
 * <OverviewList metrics={metrics} />
 */
function OverviewList({
  metrics,
  className = '',
  ...props
}) {
  if (!metrics || metrics.length === 0) {
    return null;
  }

  return (
    <div className={`overview-list ${className}`} {...props}>
      {metrics.map((metric, index) => (
        <OverviewItem
          key={metric.id || `metric-${index}`}
          label={metric.label}
          value={metric.value}
          link={metric.link}
          urgent={metric.urgent}
          onClick={metric.onClick}
        />
      ))}
    </div>
  );
}

OverviewList.propTypes = {
  /** Array of metric objects */
  metrics: PropTypes.arrayOf(
    PropTypes.shape({
      /** Optional unique identifier */
      id: PropTypes.string,
      /** Metric label */
      label: PropTypes.string.isRequired,
      /** Metric value */
      value: PropTypes.oneOfType([PropTypes.string, PropTypes.number]).isRequired,
      /** Make value clickable */
      link: PropTypes.bool,
      /** Urgent state */
      urgent: PropTypes.bool,
      /** Click handler */
      onClick: PropTypes.func
    })
  ).isRequired,
  /** Additional CSS classes */
  className: PropTypes.string
};

export default OverviewList;
