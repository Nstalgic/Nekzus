import React, { useState, useRef, useEffect } from 'react';
import PropTypes from 'prop-types';

/**
 * CustomDropdown Component
 *
 * A custom-styled dropdown that replaces native select elements.
 * Provides better styling control and visual feedback.
 *
 * @component
 * @example
 * ```jsx
 * const options = [
 *   { value: '10', label: '10 seconds' },
 *   { value: '30', label: '30 seconds' }
 * ];
 *
 * <CustomDropdown
 *   options={options}
 *   value={selected}
 *   onChange={(value) => setSelected(value)}
 *   placeholder="Select an option..."
 * />
 * ```
 */
const CustomDropdown = ({
  options = [],
  value,
  onChange,
  id,
  className = '',
  placeholder = 'Select...',
}) => {
  const [open, setOpen] = useState(false);
  const dropdownRef = useRef(null);

  // Find selected option
  const selectedOption = options.find(opt => opt.value === value);
  const displayText = selectedOption ? selectedOption.label : placeholder;

  // Close dropdown when clicking outside
  useEffect(() => {
    const handleClickOutside = (event) => {
      if (dropdownRef.current && !dropdownRef.current.contains(event.target)) {
        setOpen(false);
      }
    };

    if (open) {
      document.addEventListener('click', handleClickOutside);
    }

    return () => {
      document.removeEventListener('click', handleClickOutside);
    };
  }, [open]);

  const handleToggle = (e) => {
    e.preventDefault();
    setOpen(!open);
  };

  const handleSelect = (optionValue) => {
    onChange(optionValue);
    setOpen(false);
  };

  return (
    <div
      ref={dropdownRef}
      id={id}
      className={`custom-dropdown ${open ? 'open' : ''} ${className}`.trim()}
    >
      <button
        type="button"
        className="custom-dropdown-toggle input"
        onClick={handleToggle}
      >
        <span className="theme-name">{displayText}</span>
        <span className="dropdown-arrow">▼</span>
      </button>
      <div className="custom-dropdown-menu">
        {options.map((option) => (
          <div
            key={option.value}
            className={`custom-dropdown-item ${value === option.value ? 'active' : ''}`.trim()}
            data-value={option.value}
            onClick={() => handleSelect(option.value)}
          >
            <span className="theme-name">{option.label}</span>
            <span className="theme-check">✓</span>
          </div>
        ))}
      </div>
    </div>
  );
};

CustomDropdown.propTypes = {
  /** Array of options with { value, label } */
  options: PropTypes.arrayOf(
    PropTypes.shape({
      value: PropTypes.string.isRequired,
      label: PropTypes.string.isRequired,
    })
  ).isRequired,
  /** Currently selected value */
  value: PropTypes.string,
  /** Change handler - receives selected value */
  onChange: PropTypes.func.isRequired,
  /** Dropdown ID attribute */
  id: PropTypes.string,
  /** Additional CSS classes */
  className: PropTypes.string,
  /** Placeholder text when no selection */
  placeholder: PropTypes.string,
};

export default CustomDropdown;
