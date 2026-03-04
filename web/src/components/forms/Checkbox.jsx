import React from 'react';
import PropTypes from 'prop-types';

/**
 * Checkbox Component
 *
 * A custom-styled checkbox with label.
 *
 * @component
 * @example
 * ```jsx
 * <Checkbox
 *   label="Enable notifications"
 *   checked={enabled}
 *   onChange={(e) => setEnabled(e.target.checked)}
 * />
 * ```
 */
const Checkbox = ({
  label,
  checked,
  onChange,
  id,
  disabled = false,
  ariaLabel,
}) => {
  return (
    <label className="checkbox-label">
      <input
        type="checkbox"
        className="checkbox"
        checked={checked}
        onChange={onChange}
        id={id}
        disabled={disabled}
        aria-label={ariaLabel || label}
      />
      {label && <span>{label}</span>}
    </label>
  );
};

Checkbox.propTypes = {
  /** Label text displayed next to checkbox */
  label: PropTypes.string,
  /** Checked state */
  checked: PropTypes.bool.isRequired,
  /** Change handler function */
  onChange: PropTypes.func.isRequired,
  /** Checkbox ID attribute */
  id: PropTypes.string,
  /** Disabled state */
  disabled: PropTypes.bool,
  /** ARIA label for accessibility */
  ariaLabel: PropTypes.string,
};

export default Checkbox;
