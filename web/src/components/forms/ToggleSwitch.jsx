import React from 'react';
import PropTypes from 'prop-types';

/**
 * ToggleSwitch Component
 *
 * An animated on/off toggle switch.
 *
 * @component
 * @example
 * ```jsx
 * <ToggleSwitch
 *   checked={enabled}
 *   onChange={(e) => setEnabled(e.target.checked)}
 *   label="Enable feature"
 * />
 * ```
 */
const ToggleSwitch = ({
  checked,
  onChange,
  id,
  label,
  disabled = false,
}) => {
  return (
    <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--spacing-sm)' }}>
      <label className="toggle-switch">
        <input
          type="checkbox"
          checked={checked}
          onChange={onChange}
          id={id}
          disabled={disabled}
        />
        <span className="toggle-slider"></span>
      </label>
      {label && (
        <label htmlFor={id} style={{ cursor: disabled ? 'not-allowed' : 'pointer' }}>
          {label}
        </label>
      )}
    </div>
  );
};

ToggleSwitch.propTypes = {
  /** Checked state */
  checked: PropTypes.bool.isRequired,
  /** Change handler function */
  onChange: PropTypes.func.isRequired,
  /** Toggle ID attribute */
  id: PropTypes.string,
  /** Optional label text */
  label: PropTypes.string,
  /** Disabled state */
  disabled: PropTypes.bool,
};

export default ToggleSwitch;
