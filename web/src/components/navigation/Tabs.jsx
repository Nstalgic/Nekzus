import PropTypes from 'prop-types';
import TabItem from './TabItem';

/**
 * Tabs Component - Tab navigation wrapper
 *
 * Manages tab navigation state and renders TabItem components.
 * Handles keyboard navigation and ARIA attributes for accessibility.
 *
 * @example
 * ```jsx
 * const tabs = [
 *   { id: 'routes', label: 'ROUTES' },
 *   { id: 'discovery', label: 'DISCOVERY', badge: 7, badgeSeverity: 'warning' },
 *   { id: 'devices', label: 'DEVICES' },
 * ];
 *
 * <Tabs
 *   tabs={tabs}
 *   activeTab="routes"
 *   onChange={(tabId) => setActiveTab(tabId)}
 * />
 * ```
 *
 * @param {Object} props - Component props
 * @param {Array} props.tabs - Array of tab objects { id, label, badge?, badgeSeverity? }
 * @param {string} props.activeTab - Currently active tab ID
 * @param {Function} props.onChange - Callback when tab is clicked (tabId) => void
 * @param {string} [props['aria-label']] - ARIA label for tablist
 * @returns {JSX.Element} Tab navigation
 */
const Tabs = ({ tabs, activeTab, onChange, 'aria-label': ariaLabel = 'Navigation tabs' }) => {
  const handleTabClick = (e, tabId) => {
    e.preventDefault();
    onChange(tabId);
  };

  return (
    <nav className="tabs" role="tablist" aria-label={ariaLabel}>
      {tabs.map((tab) => (
        <TabItem
          key={tab.id}
          id={tab.id}
          label={tab.label}
          badge={tab.badge}
          badgeSeverity={tab.badgeSeverity}
          active={activeTab === tab.id}
          onClick={(e) => handleTabClick(e, tab.id)}
        />
      ))}
    </nav>
  );
};

Tabs.propTypes = {
  tabs: PropTypes.arrayOf(
    PropTypes.shape({
      id: PropTypes.string.isRequired,
      label: PropTypes.string.isRequired,
      badge: PropTypes.number,
      badgeSeverity: PropTypes.oneOf(['warning', 'error', 'info']),
    })
  ).isRequired,
  activeTab: PropTypes.string.isRequired,
  onChange: PropTypes.func.isRequired,
  'aria-label': PropTypes.string,
};

export default Tabs;
