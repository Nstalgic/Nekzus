import React from 'react';
import PropTypes from 'prop-types';

/**
 * Label Component
 *
 * A form label with optional required indicator.
 *
 * @component
 * @example
 * ```jsx
 * <Label htmlFor="email" required>
 *   Email Address
 * </Label>
 * <Input id="email" ... />
 * ```
 */
const Label = ({
  children,
  htmlFor,
  required = false,
}) => {
  return (
    <label htmlFor={htmlFor}>
      {children}
      {required && <span style={{ color: 'var(--color-error)', marginLeft: '2px' }}>*</span>}
    </label>
  );
};

Label.propTypes = {
  /** Label content */
  children: PropTypes.node.isRequired,
  /** ID of the associated form element */
  htmlFor: PropTypes.string,
  /** Shows required asterisk */
  required: PropTypes.bool,
};

export default Label;
