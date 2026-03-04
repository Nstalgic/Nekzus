/**
 * DetailsModal Component - Reusable base modal for detail views
 *
 * Provides consistent structure for modals:
 * - Header with icon, title, subtitle, and close button
 * - Scrollable body
 * - Optional footer for actions
 *
 * Used by: DeviceDetailsModal, ContainerDetailsModal, EditRouteModal, etc.
 */

import PropTypes from 'prop-types';
import { X } from 'lucide-react';
import { Card } from '../boxes';
import styles from './DetailsModal.module.css';

/**
 * DetailsModal Component
 *
 * @param {object} props - Component props
 * @param {boolean} props.isOpen - Whether the modal is open
 * @param {function} props.onClose - Close callback
 * @param {React.ReactNode} props.icon - Icon element to display in header
 * @param {string} props.title - Modal title
 * @param {string} [props.subtitle] - Optional subtitle below title
 * @param {React.ReactNode} [props.badge] - Optional badge element (e.g., status badge)
 * @param {React.ReactNode} props.children - Modal body content
 * @param {React.ReactNode} [props.footer] - Optional footer content (buttons)
 * @param {string} [props.size='medium'] - Modal size: 'small', 'medium', 'large'
 * @param {string} [props.className] - Additional class for the card
 */
export function DetailsModal({
  isOpen,
  onClose,
  icon,
  title,
  subtitle,
  badge,
  children,
  footer,
  size = 'medium',
  className = ''
}) {
  if (!isOpen) return null;

  const sizeClass = {
    small: styles.modalSmall,
    medium: styles.modalMedium,
    large: styles.modalLarge
  }[size] || styles.modalMedium;

  return (
    <div className={styles.modalOverlay} onClick={onClose}>
      <div
        className={`${styles.modalContent} ${sizeClass}`}
        onClick={(e) => e.stopPropagation()}
      >
        <Card className={`${styles.detailsCard} ${className}`}>
          {/* Header */}
          <div className={styles.header}>
            <div className={styles.headerTitleSection}>
              {icon && (
                <div className={styles.headerIcon}>
                  {icon}
                </div>
              )}
              <div className={styles.headerInfo}>
                <h2 className={styles.headerTitle}>{title}</h2>
                {subtitle && (
                  <p className={styles.headerSubtitle}>{subtitle}</p>
                )}
                {badge}
              </div>
            </div>
            <button
              className={styles.closeButton}
              onClick={onClose}
              aria-label="Close modal"
            >
              <X size={20} />
            </button>
          </div>

          {/* Body */}
          <div className={styles.body}>
            {children}
          </div>

          {/* Footer */}
          {footer && (
            <div className={styles.footer}>
              {footer}
            </div>
          )}
        </Card>
      </div>
    </div>
  );
}

DetailsModal.propTypes = {
  isOpen: PropTypes.bool.isRequired,
  onClose: PropTypes.func.isRequired,
  icon: PropTypes.node,
  title: PropTypes.string.isRequired,
  subtitle: PropTypes.string,
  badge: PropTypes.node,
  children: PropTypes.node.isRequired,
  footer: PropTypes.node,
  size: PropTypes.oneOf(['small', 'medium', 'large']),
  className: PropTypes.string
};
