import PropTypes from 'prop-types';

/**
 * Card Component - Generic card container
 *
 * A simple bordered container for content. Unlike Box, it doesn't have
 * a positioned header at the top edge.
 *
 * @example
 * ```jsx
 * <Card className="discovery-card">
 *   <div className="discovery-card-header">
 *     <h3>Service Name</h3>
 *   </div>
 *   <div className="discovery-card-body">
 *     <p>Service details...</p>
 *   </div>
 * </Card>
 * ```
 *
 * @param {Object} props - Component props
 * @param {React.ReactNode} props.children - Content to display inside the card
 * @param {string} [props.className] - Additional CSS classes to apply
 * @returns {JSX.Element} Card component
 */
const Card = ({ children, className = '' }) => {
  return (
    <div className={`card ${className}`.trim()}>
      {children}
    </div>
  );
};

Card.propTypes = {
  children: PropTypes.node.isRequired,
  className: PropTypes.string,
};

export default Card;
