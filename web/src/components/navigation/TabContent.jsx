import PropTypes from 'prop-types';

/**
 * TabContent Component - Tab panel wrapper
 *
 * Container for tab panel content. Shows/hides based on active state.
 * Includes ARIA attributes for accessibility.
 *
 * @example
 * ```jsx
 * <TabContent id="routes" active={activeTab === 'routes'}>
 *   <div className="table-controls">
 *     <input type="text" className="input" placeholder="Filter routes..." />
 *   </div>
 *   <table className="table">
 *     // ... table content
 *   </table>
 * </TabContent>
 * ```
 *
 * @param {Object} props - Component props
 * @param {string} props.id - Panel ID (matches tab's aria-controls)
 * @param {boolean} props.active - Whether panel is currently visible
 * @param {React.ReactNode} props.children - Panel content
 * @param {string} [props.className] - Additional CSS classes to apply
 * @returns {JSX.Element} Tab panel content
 */
const TabContent = ({ id, active, children, className = '' }) => {
  return (
    <div
      className={`tab-content ${active ? 'active' : ''} ${className}`.trim()}
      id={id}
      role="tabpanel"
      aria-labelledby={`${id}-tab`}
    >
      {children}
    </div>
  );
};

TabContent.propTypes = {
  id: PropTypes.string.isRequired,
  active: PropTypes.bool.isRequired,
  children: PropTypes.node.isRequired,
  className: PropTypes.string,
};

export default TabContent;
