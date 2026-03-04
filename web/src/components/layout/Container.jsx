import PropTypes from 'prop-types';

/**
 * Container Component
 *
 * Main application wrapper that provides the foundational layout structure.
 * This component wraps the entire terminal dashboard UI.
 *
 * @component
 * @param {Object} props - Component props
 * @param {React.ReactNode} props.children - Child elements to render inside container
 * @param {string} [props.className] - Additional CSS classes to apply
 * @returns {JSX.Element} Container wrapper
 *
 * @example
 * <Container>
 *   <TerminalHeader />
 *   <TerminalContent>...</TerminalContent>
 *   <TerminalFooter />
 * </Container>
 */
const Container = ({ children, className = '' }) => {
  return (
    <div className={`terminal-container ${className}`.trim()}>
      {children}
    </div>
  );
};

Container.propTypes = {
  children: PropTypes.node.isRequired,
  className: PropTypes.string,
};

export default Container;
