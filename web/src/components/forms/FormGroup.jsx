import React from 'react';
import PropTypes from 'prop-types';

/**
 * FormGroup Component
 *
 * A wrapper component for form fields that includes label, input, and helper text.
 *
 * @component
 * @example
 * ```jsx
 * <FormGroup
 *   label="Email Address"
 *   helperText="Enter your email"
 *   required
 * >
 *   <Input value={email} onChange={setEmail} />
 * </FormGroup>
 * ```
 */
const FormGroup = ({
  label,
  children,
  helperText,
  error,
  required = false,
}) => {
  return (
    <div className="form-group">
      {label && (
        <label>
          {label}
          {required && <span style={{ color: 'var(--color-error)', marginLeft: '2px' }}>*</span>}
        </label>
      )}
      {children}
      {helperText && !error && (
        <p className="text-secondary" style={{ fontSize: '11px', marginTop: 'var(--spacing-xs)' }}>
          {helperText}
        </p>
      )}
      {error && (
        <p className="text-error" style={{ fontSize: '11px', marginTop: 'var(--spacing-xs)' }}>
          {error}
        </p>
      )}
    </div>
  );
};

FormGroup.propTypes = {
  /** Label text */
  label: PropTypes.string,
  /** Form field element(s) */
  children: PropTypes.node.isRequired,
  /** Helper text displayed below field */
  helperText: PropTypes.string,
  /** Error message (replaces helper text) */
  error: PropTypes.string,
  /** Shows required asterisk */
  required: PropTypes.bool,
};

export default FormGroup;
