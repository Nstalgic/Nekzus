import PropTypes from 'prop-types';
import TabBadge from './TabBadge';

/**
 * TabItem Component - Individual tab button
 *
 * Renders a single tab with optional notification badge.
 * Includes ARIA attributes for screen reader accessibility.
 *
 * @example
 * ```jsx
 * <TabItem
 *   id="discovery"
 *   label="DISCOVERY"
 *   badge={7}
 *   badgeSeverity="warning"
 *   active={true}
 *   onClick={(e) => handleClick(e)}
 * />
 * ```
 *
 * @param {Object} props - Component props
 * @param {string} props.id - Tab ID (used for aria-controls)
 * @param {string} props.label - Tab label text
 * @param {number} [props.badge] - Optional badge count
 * @param {string} [props.badgeSeverity] - Badge severity level (warning/error/info)
 * @param {boolean} props.active - Whether tab is currently active
 * @param {Function} props.onClick - Click handler
 * @returns {JSX.Element} Tab item button
 */
const TabItem = ({ id, label, badge, badgeSeverity, active, onClick }) => {
  const ariaLabel = badge
    ? `${label} tab, ${badge} items ${badgeSeverity ? `(${badgeSeverity})` : ''}`
    : `${label} tab`;

  return (
    <a
      href="#"
      className={`tab-item ${active ? 'active' : ''}`.trim()}
      role="tab"
      aria-selected={active}
      aria-controls={id}
      aria-label={ariaLabel}
      data-tab={id}
      onClick={onClick}
    >
      <span className="tab-label">{label}</span>
      {badge && <TabBadge count={badge} severity={badgeSeverity} />}
    </a>
  );
};

TabItem.propTypes = {
  id: PropTypes.string.isRequired,
  label: PropTypes.string.isRequired,
  badge: PropTypes.number,
  badgeSeverity: PropTypes.oneOf(['warning', 'error', 'info']),
  active: PropTypes.bool.isRequired,
  onClick: PropTypes.func.isRequired,
};

export default TabItem;
