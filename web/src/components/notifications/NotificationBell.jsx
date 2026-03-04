import { useState, useRef, useEffect } from 'react';
import { createPortal } from 'react-dom';
import { Bell } from 'lucide-react';
import { useNotification } from '../../contexts/NotificationContext';

const MAX_DISPLAY = 10;

function formatRelativeTime(timestamp) {
  const seconds = Math.floor((Date.now() - timestamp) / 1000);

  if (seconds < 60) return 'just now';
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m ago`;
  if (seconds < 86400) return `${Math.floor(seconds / 3600)}h ago`;
  return `${Math.floor(seconds / 86400)}d ago`;
}

export default function NotificationBell() {
  const {
    notifications,
    unreadCount,
    dismissNotification,
    dismissAll,
  } = useNotification();

  const [isOpen, setIsOpen] = useState(false);
  const [dropdownPosition, setDropdownPosition] = useState({ top: 0, right: 0 });
  const buttonRef = useRef(null);
  const closeTimeoutRef = useRef(null);

  // Get undismissed notifications
  const activeNotifications = notifications
    .filter((n) => !n.dismissed)
    .slice(0, MAX_DISPLAY);

  // Update dropdown position when opened
  useEffect(() => {
    if (isOpen && buttonRef.current) {
      const rect = buttonRef.current.getBoundingClientRect();
      setDropdownPosition({
        top: rect.bottom,
        right: window.innerWidth - rect.right,
      });
    }
  }, [isOpen]);

  // Clean up timeout on unmount
  useEffect(() => {
    return () => {
      if (closeTimeoutRef.current) {
        clearTimeout(closeTimeoutRef.current);
      }
    };
  }, []);

  const handleMouseEnter = () => {
    // Cancel any pending close
    if (closeTimeoutRef.current) {
      clearTimeout(closeTimeoutRef.current);
      closeTimeoutRef.current = null;
    }
    setIsOpen(true);
  };

  const handleMouseLeave = () => {
    // Delay closing to allow diagonal mouse movement
    closeTimeoutRef.current = setTimeout(() => {
      setIsOpen(false);
    }, 150);
  };

  const handleToggle = () => {
    setIsOpen(!isOpen);
  };

  const handleKeyDown = (e) => {
    if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault();
      setIsOpen(!isOpen);
    } else if (e.key === 'Escape' && isOpen) {
      e.preventDefault();
      setIsOpen(false);
    }
  };

  const handleDismiss = (id, e) => {
    e.preventDefault();
    e.stopPropagation();
    dismissNotification(id);
  };

  const handleDismissAll = (e) => {
    e.preventDefault();
    e.stopPropagation();
    dismissAll();
  };

  // Render dropdown
  const dropdownContent = isOpen && (
    <div
      className="notification-dropdown"
      role="region"
      aria-label="Notification dropdown"
      style={{
        position: 'fixed',
        top: `${dropdownPosition.top}px`,
        right: `${dropdownPosition.right}px`,
      }}
      onMouseEnter={handleMouseEnter}
      onMouseLeave={handleMouseLeave}
    >
          <div className="notification-dropdown-header">
            <h3>Notifications</h3>
            {activeNotifications.length > 0 && (
              <button
                className="btn btn-secondary btn-sm"
                onClick={handleDismissAll}
                type="button"
              >
                DISMISS ALL
              </button>
            )}
          </div>

          <div className="notification-list">
            {activeNotifications.length === 0 ? (
              <div className="notification-empty">
                <p>No notifications</p>
              </div>
            ) : (
              activeNotifications.map((notification) => (
                <div
                  key={notification.id}
                  className={`notification-item notification-${notification.severity}`}
                >
                  <div className="notification-item-content">
                    <div className="notification-item-message">
                      {notification.strongText && (
                        <strong>{notification.strongText}</strong>
                      )}{' '}
                      {notification.message}
                    </div>

                    {notification.link && (
                      <a
                        href={notification.link.href}
                        className="notification-item-link"
                        onClick={() => setIsOpen(false)}
                      >
                        {notification.link.text}
                      </a>
                    )}

                    <div className="notification-item-time">
                      {formatRelativeTime(notification.timestamp)}
                    </div>
                  </div>

                  <button
                    className="notification-item-dismiss"
                    onClick={(e) => handleDismiss(notification.id, e)}
                    aria-label="Dismiss notification"
                    type="button"
                  >
                    ×
                  </button>
                </div>
              ))
            )}
          </div>
    </div>
  );

  return (
    <>
      <div
        className="notification-bell-container"
        onMouseEnter={handleMouseEnter}
        onMouseLeave={handleMouseLeave}
      >
        <button
          ref={buttonRef}
          className="notification-bell-button"
          aria-label={`Notifications ${unreadCount > 0 ? `(${unreadCount} unread)` : ''}`}
          aria-expanded={isOpen}
          aria-haspopup="true"
          onClick={handleToggle}
          onKeyDown={handleKeyDown}
          type="button"
        >
          <Bell size={18} />
          {unreadCount > 0 && (
            <span className="notification-badge">{unreadCount}</span>
          )}
        </button>
      </div>

      {/* Render dropdown via portal to escape header stacking context */}
      {dropdownContent && createPortal(dropdownContent, document.body)}
    </>
  );
}
