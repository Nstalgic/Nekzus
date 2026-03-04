import PropTypes from 'prop-types';

/**
 * Box Component - Primary WTFUtil-style container
 *
 * A bordered container with a header positioned absolutely at the top edge.
 * The header has a centered text with background "cutout" effect.
 *
 * @example
 * ```jsx
 * <Box title="OVERVIEW">
 *   <div className="overview-list">
 *     <div className="overview-item">
 *       <span className="overview-label">ACTIVE ROUTES</span>
 *       <span className="overview-value">42</span>
 *     </div>
 *   </div>
 * </Box>
 * ```
 *
 * @param {Object} props - Component props
 * @param {string} props.title - Header text displayed at top edge
 * @param {React.ReactNode} props.children - Content to display inside the box
 * @param {string} [props.className] - Additional CSS classes to apply
 * @returns {JSX.Element} Box component
 */
const Box = ({ title, children, className = '' }) => {
  return (
    <div className={`box ${className}`.trim()}>
      <div className="box-header">{title}</div>
      <div className="box-content">
        {children}
      </div>
    </div>
  );
};

Box.propTypes = {
  title: PropTypes.string.isRequired,
  children: PropTypes.node.isRequired,
  className: PropTypes.string,
};

export default Box;
