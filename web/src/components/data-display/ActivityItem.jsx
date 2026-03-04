import PropTypes from 'prop-types';

/**
 * ActivityItem component for displaying a single activity entry
 *
 * @component
 * @example
 * <ActivityItem text="New route registered" time="2m ago" />
 */
function ActivityItem({
  text,
  time,
  className = '',
  ...props
}) {
  return (
    <div className={`activity-item ${className}`} {...props}>
      <span className="activity-text">{text}</span>
      <span className="activity-time">{time}</span>
    </div>
  );
}

ActivityItem.propTypes = {
  /** Activity description text */
  text: PropTypes.string.isRequired,
  /** Timestamp or relative time */
  time: PropTypes.string.isRequired,
  /** Additional CSS classes */
  className: PropTypes.string
};

export default ActivityItem;
