import React from 'react';
import PropTypes from 'prop-types';

/**
 * TextArea Component
 *
 * A multi-line text input field.
 *
 * @component
 * @example
 * ```jsx
 * <TextArea
 *   value={description}
 *   onChange={(e) => setDescription(e.target.value)}
 *   placeholder="Enter description..."
 *   rows={5}
 * />
 * ```
 */
const TextArea = ({
  value,
  onChange,
  placeholder,
  rows = 4,
  className = '',
  disabled = false,
  id,
  name,
  ...props
}) => {
  return (
    <textarea
      value={value}
      onChange={onChange}
      placeholder={placeholder}
      rows={rows}
      className={`input ${className}`.trim()}
      disabled={disabled}
      id={id}
      name={name}
      {...props}
    />
  );
};

TextArea.propTypes = {
  /** Current value (controlled component) */
  value: PropTypes.string,
  /** Change handler function */
  onChange: PropTypes.func.isRequired,
  /** Placeholder text */
  placeholder: PropTypes.string,
  /** Number of visible rows */
  rows: PropTypes.number,
  /** Additional CSS classes */
  className: PropTypes.string,
  /** Disabled state */
  disabled: PropTypes.bool,
  /** TextArea ID attribute */
  id: PropTypes.string,
  /** TextArea name attribute */
  name: PropTypes.string,
};

export default TextArea;
