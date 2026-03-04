/**
 * CreateScheduleModal Component
 *
 * Modal for creating scheduled script/workflow executions using cron expressions.
 *
 * Features:
 * - Script or workflow selection
 * - Cron expression input with presets
 * - Parameter configuration
 * - Enable/disable toggle
 */

import { useState, useEffect } from 'react';
import PropTypes from 'prop-types';
import { Clock, Info } from 'lucide-react';
import { DetailsModal } from './DetailsModal';
import CustomDropdown from '../forms/CustomDropdown';
import styles from './CreateScheduleModal.module.css';

// Common cron presets
const CRON_PRESETS = [
  { value: '', label: 'Custom...' },
  { value: '0 * * * *', label: 'Every hour' },
  { value: '0 */6 * * *', label: 'Every 6 hours' },
  { value: '0 0 * * *', label: 'Daily at midnight' },
  { value: '0 6 * * *', label: 'Daily at 6 AM' },
  { value: '0 0 * * 0', label: 'Weekly (Sunday midnight)' },
  { value: '0 0 1 * *', label: 'Monthly (1st at midnight)' },
  { value: '*/5 * * * *', label: 'Every 5 minutes' },
  { value: '*/15 * * * *', label: 'Every 15 minutes' },
  { value: '*/30 * * * *', label: 'Every 30 minutes' }
];

/**
 * CreateScheduleModal Component
 *
 * @param {object} props - Component props
 * @param {boolean} props.isOpen - Whether the modal is open
 * @param {function} props.onClose - Callback when modal closes
 * @param {function} props.onSave - Callback when saved (receives schedule data)
 * @param {Array} props.scripts - Available scripts to choose from
 * @param {Array} props.workflows - Available workflows to choose from
 */
export function CreateScheduleModal({ isOpen, onClose, onSave, scripts = [], workflows = [] }) {
  // Form state
  const [targetType, setTargetType] = useState('script'); // 'script' or 'workflow'
  const [formData, setFormData] = useState({
    scriptId: '',
    workflowId: '',
    cronExpression: '',
    enabled: true
  });

  // UI state
  const [selectedPreset, setSelectedPreset] = useState('');
  const [errors, setErrors] = useState({});
  const [isSaving, setIsSaving] = useState(false);

  // Reset form when modal opens
  useEffect(() => {
    if (isOpen) {
      resetForm();
    }
  }, [isOpen]);

  const resetForm = () => {
    setTargetType('script');
    setFormData({
      scriptId: '',
      workflowId: '',
      cronExpression: '',
      enabled: true
    });
    setSelectedPreset('');
    setErrors({});
    setIsSaving(false);
  };

  // Handle input changes
  const handleChange = (field, value) => {
    setFormData(prev => ({ ...prev, [field]: value }));
    if (errors[field]) {
      setErrors(prev => {
        const newErrors = { ...prev };
        delete newErrors[field];
        return newErrors;
      });
    }
  };

  // Handle preset selection
  const handlePresetChange = (value) => {
    setSelectedPreset(value);
    if (value) {
      handleChange('cronExpression', value);
    }
  };

  // Handle target type change
  const handleTargetTypeChange = (type) => {
    setTargetType(type);
    setFormData(prev => ({
      ...prev,
      scriptId: '',
      workflowId: ''
    }));
  };

  // Validation
  const validate = () => {
    const newErrors = {};

    if (targetType === 'script' && !formData.scriptId) {
      newErrors.target = 'Please select a script';
    }
    if (targetType === 'workflow' && !formData.workflowId) {
      newErrors.target = 'Please select a workflow';
    }

    if (!formData.cronExpression.trim()) {
      newErrors.cronExpression = 'Cron expression is required';
    } else {
      // Basic cron validation (5 or 6 fields)
      const parts = formData.cronExpression.trim().split(/\s+/);
      if (parts.length < 5 || parts.length > 6) {
        newErrors.cronExpression = 'Invalid cron expression (expected 5-6 fields)';
      }
    }

    setErrors(newErrors);
    return Object.keys(newErrors).length === 0;
  };

  // Handle save
  const handleSave = async () => {
    if (!validate()) {
      return;
    }

    setIsSaving(true);

    try {
      const scheduleData = {
        cronExpression: formData.cronExpression.trim(),
        enabled: formData.enabled,
        parameters: {}
      };

      if (targetType === 'script') {
        scheduleData.scriptId = formData.scriptId;
      } else {
        scheduleData.workflowId = formData.workflowId;
      }

      await onSave(scheduleData);
      onClose();
    } catch (error) {
      console.error('Error saving schedule:', error);
      setErrors({ submit: error.message || 'Failed to save schedule' });
    } finally {
      setIsSaving(false);
    }
  };

  // Build options
  const scriptOptions = scripts.map(s => ({
    value: s.id,
    label: s.name
  }));

  const workflowOptions = workflows.map(w => ({
    value: w.id,
    label: w.name
  }));

  const hasTargets = scripts.length > 0 || workflows.length > 0;

  const footer = (
    <>
      <button
        className="btn btn-secondary"
        onClick={onClose}
        type="button"
        disabled={isSaving}
      >
        CANCEL
      </button>
      <button
        className="btn btn-success"
        onClick={handleSave}
        type="button"
        disabled={isSaving || !hasTargets}
      >
        {isSaving ? 'CREATING...' : 'CREATE SCHEDULE'}
      </button>
    </>
  );

  return (
    <DetailsModal
      isOpen={isOpen}
      onClose={onClose}
      icon={<Clock size={32} />}
      title="CREATE SCHEDULE"
      footer={footer}
      size="medium"
    >
      <form className="terminal-form">
        {/* Submit Error */}
        {errors.submit && (
          <div className="alert alert-error" style={{ marginBottom: 'var(--spacing-md)' }}>
            <span className="alert-icon">X</span>
            <div>
              <strong>ERROR:</strong> {errors.submit}
            </div>
          </div>
        )}

        {/* No Targets Warning */}
        {!hasTargets && (
          <div className="alert alert-warning" style={{ marginBottom: 'var(--spacing-md)' }}>
            <span className="alert-icon">!</span>
            <div>
              No scripts or workflows available. Create some first.
            </div>
          </div>
        )}

        {/* Target Type Toggle */}
        <div className="form-group">
          <label>SCHEDULE TYPE</label>
          <div className={styles.typeToggle}>
            <button
              type="button"
              className={`btn btn-sm ${targetType === 'script' ? 'btn-primary' : 'btn-secondary'}`}
              onClick={() => handleTargetTypeChange('script')}
            >
              SCRIPT
            </button>
            <button
              type="button"
              className={`btn btn-sm ${targetType === 'workflow' ? 'btn-primary' : 'btn-secondary'}`}
              onClick={() => handleTargetTypeChange('workflow')}
              disabled={workflows.length === 0}
            >
              WORKFLOW
            </button>
          </div>
        </div>

        {/* Target Selection */}
        <div className="form-group">
          <label>
            {targetType === 'script' ? 'SCRIPT' : 'WORKFLOW'} <span className="text-error">*</span>
          </label>
          {targetType === 'script' ? (
            <CustomDropdown
              options={scriptOptions}
              value={formData.scriptId}
              onChange={(value) => handleChange('scriptId', value)}
              placeholder="Select a script..."
              className={errors.target ? 'input-error' : ''}
            />
          ) : (
            <CustomDropdown
              options={workflowOptions}
              value={formData.workflowId}
              onChange={(value) => handleChange('workflowId', value)}
              placeholder="Select a workflow..."
              className={errors.target ? 'input-error' : ''}
            />
          )}
          {errors.target && (
            <span className="form-error">{errors.target}</span>
          )}
        </div>

        {/* Cron Preset */}
        <div className="form-group">
          <label>SCHEDULE PRESET</label>
          <CustomDropdown
            options={CRON_PRESETS}
            value={selectedPreset}
            onChange={handlePresetChange}
            placeholder="Select a preset or enter custom..."
          />
        </div>

        {/* Cron Expression */}
        <div className="form-group">
          <label htmlFor="cron-expression">
            CRON EXPRESSION <span className="text-error">*</span>
          </label>
          <input
            type="text"
            id="cron-expression"
            className={`input ${errors.cronExpression ? 'input-error' : ''}`}
            value={formData.cronExpression}
            onChange={(e) => {
              handleChange('cronExpression', e.target.value);
              setSelectedPreset(''); // Clear preset when manually editing
            }}
            placeholder="* * * * *"
            aria-required="true"
          />
          {errors.cronExpression && (
            <span className="form-error">{errors.cronExpression}</span>
          )}
          <div className={styles.cronHelp}>
            <Info size={12} />
            <span>Format: minute hour day-of-month month day-of-week</span>
          </div>
        </div>

        {/* Enabled Toggle */}
        <div className="form-group">
          <label className={styles.checkboxLabel}>
            <input
              type="checkbox"
              checked={formData.enabled}
              onChange={(e) => handleChange('enabled', e.target.checked)}
            />
            <span>Enable schedule immediately</span>
          </label>
        </div>
      </form>
    </DetailsModal>
  );
}

CreateScheduleModal.propTypes = {
  isOpen: PropTypes.bool.isRequired,
  onClose: PropTypes.func.isRequired,
  onSave: PropTypes.func.isRequired,
  scripts: PropTypes.arrayOf(
    PropTypes.shape({
      id: PropTypes.string.isRequired,
      name: PropTypes.string.isRequired
    })
  ),
  workflows: PropTypes.arrayOf(
    PropTypes.shape({
      id: PropTypes.string.isRequired,
      name: PropTypes.string.isRequired
    })
  )
};
