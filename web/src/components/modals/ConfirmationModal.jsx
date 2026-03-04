/**
 * ConfirmationModal Component - Delete/Action confirmation dialog
 *
 * Features:
 * - Warning icon for danger mode
 * - Details section (shows route/item info)
 * - Cancel/Confirm buttons
 * - Red styling when danger={true}
 */

import PropTypes from 'prop-types';
import { AlertTriangle } from 'lucide-react';
import { Modal } from './Modal';

/**
 * ConfirmationModal Component
 *
 * @param {object} props - Component props
 * @param {boolean} props.isOpen - Whether the modal is open
 * @param {function} props.onClose - Callback when modal closes
 * @param {function} props.onConfirm - Callback when confirmed
 * @param {string} props.title - Modal title
 * @param {string} props.message - Confirmation message
 * @param {React.ReactNode} [props.details] - Additional details to display
 * @param {string} [props.confirmText='Confirm'] - Text for confirm button
 * @param {string} [props.cancelText='Cancel'] - Text for cancel button
 * @param {boolean} [props.danger=false] - Use danger styling
 */
export function ConfirmationModal({
  isOpen,
  onClose,
  onConfirm,
  title,
  message,
  details,
  confirmText = 'Confirm',
  cancelText = 'Cancel',
  danger = false
}) {
  const handleConfirm = () => {
    onConfirm();
    onClose();
  };

  return (
    <Modal isOpen={isOpen} onClose={onClose} size="small">
      <div className={`confirmation-card ${danger ? 'danger' : ''}`}>
        {/* Header */}
        <div className="confirmation-card-header">
          {danger && (
            <span className="confirmation-icon">⚠</span>
          )}
          <h3>{title}</h3>
        </div>

        {/* Body */}
        <div className="confirmation-card-body">
          <p className="confirmation-message">{message}</p>

          {details && details}
        </div>

        {/* Footer */}
        <div className="confirmation-card-actions">
          <button
            className="btn btn-secondary"
            onClick={onClose}
            type="button"
          >
            {cancelText}
          </button>
          <button
            className={`btn ${danger ? 'btn-error' : 'btn-primary'}`}
            onClick={handleConfirm}
            type="button"
            autoFocus
          >
            {confirmText}
          </button>
        </div>
      </div>
    </Modal>
  );
}

ConfirmationModal.propTypes = {
  isOpen: PropTypes.bool.isRequired,
  onClose: PropTypes.func.isRequired,
  onConfirm: PropTypes.func.isRequired,
  title: PropTypes.string.isRequired,
  message: PropTypes.string.isRequired,
  details: PropTypes.node,
  confirmText: PropTypes.string,
  cancelText: PropTypes.string,
  danger: PropTypes.bool
};
