/**
 * Modal Component - Base modal wrapper
 *
 * Provides a reusable modal foundation with:
 * - Overlay with backdrop blur
 * - Escape key listener
 * - Click outside to close
 * - Body scroll lock when open
 * - Focus trap
 * - Fade/scale animation
 */

import { useEffect, useRef, useCallback } from 'react';
import PropTypes from 'prop-types';
import styles from './Modal.module.css';

/**
 * Modal Component
 *
 * @param {object} props - Component props
 * @param {boolean} props.isOpen - Whether the modal is open
 * @param {function} props.onClose - Callback when modal should close
 * @param {React.ReactNode} props.children - Modal content
 * @param {boolean} [props.closeOnEscape=true] - Close on ESC key
 * @param {boolean} [props.closeOnOverlay=true] - Close on overlay click
 * @param {string} [props.size='medium'] - Modal size (small, medium, large)
 */
export function Modal({
  isOpen,
  onClose,
  children,
  closeOnEscape = true,
  closeOnOverlay = true,
  size = 'medium'
}) {
  const overlayRef = useRef(null);
  const modalRef = useRef(null);
  const previousActiveElement = useRef(null);

  // Handle escape key
  useEffect(() => {
    if (!isOpen || !closeOnEscape) return;

    const handleEscape = (event) => {
      if (event.key === 'Escape') {
        onClose();
      }
    };

    document.addEventListener('keydown', handleEscape);
    return () => document.removeEventListener('keydown', handleEscape);
  }, [isOpen, closeOnEscape, onClose]);

  // Handle body scroll lock
  useEffect(() => {
    if (!isOpen) return;

    // Save current active element
    previousActiveElement.current = document.activeElement;

    // Lock body scroll
    const originalOverflow = document.body.style.overflow;
    document.body.style.overflow = 'hidden';

    // Focus modal
    if (modalRef.current) {
      modalRef.current.focus();
    }

    return () => {
      // Restore body scroll
      document.body.style.overflow = originalOverflow;

      // Restore focus
      if (previousActiveElement.current && previousActiveElement.current.focus) {
        previousActiveElement.current.focus();
      }
    };
  }, [isOpen]);

  // Handle overlay click
  const handleOverlayClick = useCallback((event) => {
    if (closeOnOverlay && event.target === overlayRef.current) {
      onClose();
    }
  }, [closeOnOverlay, onClose]);

  // Handle focus trap
  useEffect(() => {
    if (!isOpen || !modalRef.current) return;

    const handleTab = (event) => {
      if (event.key !== 'Tab') return;

      const focusableElements = modalRef.current.querySelectorAll(
        'a[href], button:not([disabled]), textarea:not([disabled]), input:not([disabled]), select:not([disabled]), [tabindex]:not([tabindex="-1"])'
      );

      const firstElement = focusableElements[0];
      const lastElement = focusableElements[focusableElements.length - 1];

      if (event.shiftKey && document.activeElement === firstElement) {
        event.preventDefault();
        lastElement.focus();
      } else if (!event.shiftKey && document.activeElement === lastElement) {
        event.preventDefault();
        firstElement.focus();
      }
    };

    document.addEventListener('keydown', handleTab);
    return () => document.removeEventListener('keydown', handleTab);
  }, [isOpen]);

  if (!isOpen) return null;

  return (
    <div
      ref={overlayRef}
      className={styles.modalOverlay}
      onClick={handleOverlayClick}
      role="dialog"
      aria-modal="true"
    >
      <div
        ref={modalRef}
        className={`${styles.modalContent} ${styles[`modal${size.charAt(0).toUpperCase() + size.slice(1)}`]}`}
        tabIndex={-1}
        role="document"
      >
        {children}
      </div>
    </div>
  );
}

Modal.propTypes = {
  isOpen: PropTypes.bool.isRequired,
  onClose: PropTypes.func.isRequired,
  children: PropTypes.node.isRequired,
  closeOnEscape: PropTypes.bool,
  closeOnOverlay: PropTypes.bool,
  size: PropTypes.oneOf(['small', 'medium', 'large'])
};
