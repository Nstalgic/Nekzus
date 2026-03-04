import PropTypes from 'prop-types';

/**
 * TwoColumnGrid Component - 2-column responsive grid layout
 *
 * Uses CSS Grid with 2 equal columns (1fr 1fr) with gap spacing.
 * Automatically stacks to 1 column on mobile devices.
 *
 * @example
 * ```jsx
 * <TwoColumnGrid>
 *   <Box title="REQUEST">
 *     <form>...</form>
 *   </Box>
 *   <Box title="RESPONSE">
 *     <pre>...</pre>
 *   </Box>
 * </TwoColumnGrid>
 * ```
 *
 * @param {Object} props - Component props
 * @param {React.ReactNode} props.children - Grid items
 * @param {string} [props.className] - Additional CSS classes to apply
 * @returns {JSX.Element} Two column grid layout
 */
const TwoColumnGrid = ({ children, className = '' }) => {
  return (
    <div className={`two-column ${className}`.trim()}>
      {children}
    </div>
  );
};

TwoColumnGrid.propTypes = {
  children: PropTypes.node.isRequired,
  className: PropTypes.string,
};

export default TwoColumnGrid;
