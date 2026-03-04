import PropTypes from 'prop-types';

/**
 * TerminalContent Component
 *
 * Main scrollable content area wrapper. This component provides the primary
 * content container that sits between the header and footer, with appropriate
 * padding and overflow handling.
 *
 * @component
 * @param {Object} props - Component props
 * @param {React.ReactNode} props.children - Content to render inside the main area
 * @param {string} [props.className] - Additional CSS classes to apply
 * @returns {JSX.Element} Main content wrapper
 *
 * @example
 * <TerminalContent>
 *   <AlertSection alerts={alerts} />
 *   <div>Dashboard content...</div>
 * </TerminalContent>
 */
const TerminalContent = ({ children, className = '' }) => {
  return (
    <main className={`terminal-content ${className}`.trim()}>
      {children}
    </main>
  );
};

TerminalContent.propTypes = {
  children: PropTypes.node.isRequired,
  className: PropTypes.string,
};

export default TerminalContent;
