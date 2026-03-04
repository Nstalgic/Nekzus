import PropTypes from 'prop-types';

/**
 * ButtonGroup component - Groups multiple buttons together
 *
 * @component
 * @example
 * <ButtonGroup>
 *   <Button variant="primary">Execute</Button>
 *   <Button variant="secondary">Cancel</Button>
 * </ButtonGroup>
 */
function ButtonGroup({
  children,
  className = '',
  ...props
}) {
  return (
    <div className={`button-group ${className}`} {...props}>
      {children}
    </div>
  );
}

ButtonGroup.propTypes = {
  /** Button elements to group */
  children: PropTypes.node.isRequired,
  /** Additional CSS classes */
  className: PropTypes.string
};

export default ButtonGroup;
