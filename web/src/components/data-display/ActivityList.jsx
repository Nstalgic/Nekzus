import PropTypes from 'prop-types';
import ActivityItem from './ActivityItem';

/**
 * ActivityList component - Scrollable activity feed container
 *
 * @component
 * @example
 * const activities = [
 *   { id: '1', text: 'New route registered', time: '2m ago' },
 *   { id: '2', text: 'Device paired', time: '5m ago' }
 * ];
 * <ActivityList activities={activities} />
 */
function ActivityList({
  activities,
  className = '',
  style = {},
  maxHeight = '300px',
  ...props
}) {
  if (!activities || activities.length === 0) {
    return null;
  }

  const containerStyle = {
    ...style,
    maxHeight,
    overflowY: 'auto'
  };

  return (
    <div
      className={`activity-list ${className}`}
      style={containerStyle}
      {...props}
    >
      {activities.map((activity) => (
        <ActivityItem
          key={activity.id}
          text={activity.text}
          time={activity.time}
        />
      ))}
    </div>
  );
}

ActivityList.propTypes = {
  /** Array of activity objects */
  activities: PropTypes.arrayOf(
    PropTypes.shape({
      /** Unique identifier */
      id: PropTypes.string.isRequired,
      /** Activity text */
      text: PropTypes.string.isRequired,
      /** Timestamp */
      time: PropTypes.string.isRequired
    })
  ).isRequired,
  /** Additional CSS classes */
  className: PropTypes.string,
  /** Inline styles */
  style: PropTypes.object,
  /** Maximum height before scrolling */
  maxHeight: PropTypes.string
};

export default ActivityList;
