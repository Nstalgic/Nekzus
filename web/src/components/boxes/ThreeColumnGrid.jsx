import PropTypes from 'prop-types';

/**
 * ThreeColumnGrid Component - 3-column responsive grid layout
 *
 * Uses CSS Grid with 3 equal columns (1fr 1fr 1fr) with gap spacing.
 * Automatically stacks to 1 column on mobile devices (<1024px).
 *
 * @example
 * ```jsx
 * <ThreeColumnGrid>
 *   <Box title="OVERVIEW">...</Box>
 *   <Box title="RECENT ACTIVITY">...</Box>
 *   <Box title="SYSTEM HEALTH">...</Box>
 * </ThreeColumnGrid>
 * ```
 *
 * @param {Object} props - Component props
 * @param {React.ReactNode} props.children - Grid items (typically Box components)
 * @param {string} [props.className] - Additional CSS classes to apply
 * @returns {JSX.Element} Three column grid layout
 */
const ThreeColumnGrid = ({ children, className = '' }) => {
  return (
    <div className={`three-column ${className}`.trim()}>
      {children}
    </div>
  );
};

ThreeColumnGrid.propTypes = {
  children: PropTypes.node.isRequired,
  className: PropTypes.string,
};

export default ThreeColumnGrid;
