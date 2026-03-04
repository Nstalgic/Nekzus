import React from 'react';
import PropTypes from 'prop-types';

/**
 * Select Component
 *
 * A wrapper for native select element with consistent styling.
 * Use CustomDropdown for better visual control and UX.
 *
 * @component
 * @example
 * ```jsx
 * const options = [
 *   { value: 'option1', label: 'Option 1' },
 *   { value: 'option2', label: 'Option 2' }
 * ];
 *
 * <Select
 *   options={options}
 *   value={selected}
 *   onChange={(e) => setSelected(e.target.value)}
 * />
 * ```
 */
const Select = ({
  options = [],
  value,
  onChange,
  className = '',
  disabled = false,
  id,
  name,
  ...props
}) => {
  return (
    <div className="select-wrapper">
      <select
        value={value}
        onChange={onChange}
        className={`input ${className}`.trim()}
        disabled={disabled}
        id={id}
        name={name}
        {...props}
      >
        {options.map((option) => (
          <option key={option.value} value={option.value}>
            {option.label}
          </option>
        ))}
      </select>
    </div>
  );
};

Select.propTypes = {
  /** Array of options with { value, label } */
  options: PropTypes.arrayOf(
    PropTypes.shape({
      value: PropTypes.string.isRequired,
      label: PropTypes.string.isRequired,
    })
  ).isRequired,
  /** Currently selected value */
  value: PropTypes.string,
  /** Change handler function */
  onChange: PropTypes.func.isRequired,
  /** Additional CSS classes */
  className: PropTypes.string,
  /** Disabled state */
  disabled: PropTypes.bool,
  /** Select ID attribute */
  id: PropTypes.string,
  /** Select name attribute */
  name: PropTypes.string,
};

export default Select;
