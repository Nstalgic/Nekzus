import React from 'react';
import PropTypes from 'prop-types';

/**
 * Radio Component
 *
 * A custom-styled radio button with label.
 *
 * @component
 * @example
 * ```jsx
 * <Radio
 *   label="Option 1"
 *   name="choices"
 *   value="option1"
 *   checked={selected === 'option1'}
 *   onChange={(e) => setSelected(e.target.value)}
 * />
 * ```
 */
const Radio = ({
  label,
  name,
  checked,
  onChange,
  value,
  disabled = false,
  id,
}) => {
  return (
    <label className="radio-label">
      <input
        type="radio"
        className="radio"
        name={name}
        value={value}
        checked={checked}
        onChange={onChange}
        disabled={disabled}
        id={id}
      />
      {label && <span>{label}</span>}
    </label>
  );
};

Radio.propTypes = {
  /** Label text displayed next to radio button */
  label: PropTypes.string,
  /** Radio group name (all radios in a group should have same name) */
  name: PropTypes.string.isRequired,
  /** Checked state */
  checked: PropTypes.bool.isRequired,
  /** Change handler function */
  onChange: PropTypes.func.isRequired,
  /** Radio button value */
  value: PropTypes.string.isRequired,
  /** Disabled state */
  disabled: PropTypes.bool,
  /** Radio button ID attribute */
  id: PropTypes.string,
};

export default Radio;
