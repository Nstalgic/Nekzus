/**
 * CreateWorkflowModal Component
 *
 * Modal for creating multi-step workflows that chain scripts together.
 *
 * Features:
 * - Name and description inputs
 * - Dynamic step list with add/remove
 * - Script selection per step
 * - Failure action selection (stop/continue)
 * - Parameter configuration per step
 */

import { useState, useEffect } from 'react';
import PropTypes from 'prop-types';
import { GitBranch, Plus, X, ChevronUp, ChevronDown } from 'lucide-react';
import { DetailsModal } from './DetailsModal';
import CustomDropdown from '../forms/CustomDropdown';
import styles from './CreateWorkflowModal.module.css';

/**
 * CreateWorkflowModal Component
 *
 * @param {object} props - Component props
 * @param {boolean} props.isOpen - Whether the modal is open
 * @param {function} props.onClose - Callback when modal closes
 * @param {function} props.onSave - Callback when saved (receives workflow data)
 * @param {Array} props.scripts - Available scripts to choose from
 */
export function CreateWorkflowModal({ isOpen, onClose, onSave, scripts = [] }) {
  // Form state
  const [formData, setFormData] = useState({
    name: '',
    description: ''
  });

  // Steps state
  const [steps, setSteps] = useState([
    { scriptId: '', parameters: {}, onFailure: 'stop' }
  ]);

  // UI state
  const [errors, setErrors] = useState({});
  const [isSaving, setIsSaving] = useState(false);

  // Reset form when modal opens
  useEffect(() => {
    if (isOpen) {
      resetForm();
    }
  }, [isOpen]);

  const resetForm = () => {
    setFormData({ name: '', description: '' });
    setSteps([{ scriptId: '', parameters: {}, onFailure: 'stop' }]);
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

  // Handle step changes
  const handleStepChange = (index, field, value) => {
    const newSteps = [...steps];
    newSteps[index] = { ...newSteps[index], [field]: value };
    setSteps(newSteps);

    // Clear step errors
    if (errors.steps) {
      setErrors(prev => {
        const newErrors = { ...prev };
        delete newErrors.steps;
        return newErrors;
      });
    }
  };

  // Add step
  const handleAddStep = () => {
    setSteps([...steps, { scriptId: '', parameters: {}, onFailure: 'stop' }]);
  };

  // Remove step
  const handleRemoveStep = (index) => {
    if (steps.length > 1) {
      setSteps(steps.filter((_, i) => i !== index));
    }
  };

  // Move step up
  const handleMoveUp = (index) => {
    if (index > 0) {
      const newSteps = [...steps];
      [newSteps[index - 1], newSteps[index]] = [newSteps[index], newSteps[index - 1]];
      setSteps(newSteps);
    }
  };

  // Move step down
  const handleMoveDown = (index) => {
    if (index < steps.length - 1) {
      const newSteps = [...steps];
      [newSteps[index], newSteps[index + 1]] = [newSteps[index + 1], newSteps[index]];
      setSteps(newSteps);
    }
  };

  // Validation
  const validate = () => {
    const newErrors = {};

    if (!formData.name.trim()) {
      newErrors.name = 'Workflow name is required';
    }

    const validSteps = steps.filter(s => s.scriptId);
    if (validSteps.length === 0) {
      newErrors.steps = 'At least one step with a script is required';
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
      // Filter out empty steps and prepare data
      const validSteps = steps
        .filter(s => s.scriptId)
        .map(s => ({
          scriptId: s.scriptId,
          parameters: s.parameters || {},
          onFailure: s.onFailure || 'stop'
        }));

      const workflowData = {
        name: formData.name.trim(),
        description: formData.description.trim(),
        steps: validSteps
      };

      await onSave(workflowData);
      onClose();
    } catch (error) {
      console.error('Error saving workflow:', error);
      setErrors({ submit: error.message || 'Failed to save workflow' });
    } finally {
      setIsSaving(false);
    }
  };

  // Get script name by ID
  const getScriptName = (scriptId) => {
    const script = scripts.find(s => s.id === scriptId);
    return script?.name || scriptId;
  };

  // Build script options for dropdown
  const scriptOptions = scripts.map(s => ({
    value: s.id,
    label: s.name
  }));

  const failureOptions = [
    { value: 'stop', label: 'Stop workflow' },
    { value: 'continue', label: 'Continue to next step' }
  ];

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
        disabled={isSaving || scripts.length === 0}
      >
        {isSaving ? 'CREATING...' : 'CREATE WORKFLOW'}
      </button>
    </>
  );

  return (
    <DetailsModal
      isOpen={isOpen}
      onClose={onClose}
      icon={<GitBranch size={32} />}
      title="CREATE WORKFLOW"
      footer={footer}
      size="large"
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

        {/* No Scripts Warning */}
        {scripts.length === 0 && (
          <div className="alert alert-warning" style={{ marginBottom: 'var(--spacing-md)' }}>
            <span className="alert-icon">!</span>
            <div>
              No scripts registered. Register scripts first before creating workflows.
            </div>
          </div>
        )}

        {/* Workflow Name */}
        <div className="form-group">
          <label htmlFor="workflow-name">
            WORKFLOW NAME <span className="text-error">*</span>
          </label>
          <input
            type="text"
            id="workflow-name"
            className={`input ${errors.name ? 'input-error' : ''}`}
            value={formData.name}
            onChange={(e) => handleChange('name', e.target.value)}
            placeholder="e.g., Daily Maintenance"
            aria-required="true"
          />
          {errors.name && (
            <span className="form-error">{errors.name}</span>
          )}
        </div>

        {/* Description */}
        <div className="form-group">
          <label htmlFor="workflow-description">
            DESCRIPTION
          </label>
          <textarea
            id="workflow-description"
            className="input"
            value={formData.description}
            onChange={(e) => handleChange('description', e.target.value)}
            placeholder="Optional description of what this workflow does"
            rows="2"
          />
        </div>

        {/* Steps */}
        <div className="form-group">
          <label>
            WORKFLOW STEPS <span className="text-error">*</span>
          </label>
          {errors.steps && (
            <span className="form-error" style={{ display: 'block', marginBottom: 'var(--spacing-sm)' }}>
              {errors.steps}
            </span>
          )}

          <div className={styles.stepsList}>
            {steps.map((step, index) => (
              <div key={index} className={styles.stepItem}>
                <div className={styles.stepHeader}>
                  <span className={styles.stepNumber}>Step {index + 1}</span>
                  <div className={styles.stepActions}>
                    <button
                      type="button"
                      className="btn btn-sm btn-secondary"
                      onClick={() => handleMoveUp(index)}
                      disabled={index === 0}
                      aria-label="Move up"
                    >
                      <ChevronUp size={14} />
                    </button>
                    <button
                      type="button"
                      className="btn btn-sm btn-secondary"
                      onClick={() => handleMoveDown(index)}
                      disabled={index === steps.length - 1}
                      aria-label="Move down"
                    >
                      <ChevronDown size={14} />
                    </button>
                    <button
                      type="button"
                      className="btn btn-sm btn-error"
                      onClick={() => handleRemoveStep(index)}
                      disabled={steps.length === 1}
                      aria-label="Remove step"
                    >
                      <X size={14} />
                    </button>
                  </div>
                </div>

                <div className={styles.stepContent}>
                  <div className={styles.stepField}>
                    <label>Script</label>
                    <CustomDropdown
                      options={scriptOptions}
                      value={step.scriptId}
                      onChange={(value) => handleStepChange(index, 'scriptId', value)}
                      placeholder="Select a script..."
                    />
                  </div>

                  <div className={styles.stepField}>
                    <label>On Failure</label>
                    <CustomDropdown
                      options={failureOptions}
                      value={step.onFailure}
                      onChange={(value) => handleStepChange(index, 'onFailure', value)}
                      placeholder="Select action..."
                    />
                  </div>
                </div>
              </div>
            ))}
          </div>

          <button
            type="button"
            className="btn btn-secondary"
            onClick={handleAddStep}
            style={{ marginTop: 'var(--spacing-sm)' }}
          >
            <Plus size={14} />
            ADD STEP
          </button>
        </div>
      </form>
    </DetailsModal>
  );
}

CreateWorkflowModal.propTypes = {
  isOpen: PropTypes.bool.isRequired,
  onClose: PropTypes.func.isRequired,
  onSave: PropTypes.func.isRequired,
  scripts: PropTypes.arrayOf(
    PropTypes.shape({
      id: PropTypes.string.isRequired,
      name: PropTypes.string.isRequired
    })
  )
};
