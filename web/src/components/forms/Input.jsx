import React from 'react';
import PropTypes from 'prop-types';

/**
 * Input Component
 *
 * A controlled text input field with terminal styling.
 *
 * @component
 * @example
 * ```jsx
 * <Input
 *   type="text"
 *   value={value}
 *   onChange={(e) => setValue(e.target.value)}
 *   placeholder="Enter value..."
 * />
 * ```
 */
const Input = ({
  type = 'text',
  value,
  onChange,
  placeholder,
  className = '',
  disabled = false,
  id,
  name,
  ...props
}) => {
  return (
    <input
      type={type}
      value={value}
      onChange={onChange}
      placeholder={placeholder}
      className={`input ${className}`.trim()}
      disabled={disabled}
      id={id}
      name={name}
      {...props}
    />
  );
};

Input.propTypes = {
  /** Input type (text, password, email, url, etc.) */
  type: PropTypes.string,
  /** Current value (controlled component) */
  value: PropTypes.string,
  /** Change handler function */
  onChange: PropTypes.func.isRequired,
  /** Placeholder text */
  placeholder: PropTypes.string,
  /** Additional CSS classes */
  className: PropTypes.string,
  /** Disabled state */
  disabled: PropTypes.bool,
  /** Input ID attribute */
  id: PropTypes.string,
  /** Input name attribute */
  name: PropTypes.string,
};

export default Input;
