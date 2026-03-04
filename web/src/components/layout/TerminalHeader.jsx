/**
 * TerminalHeader Component
 *
 * Ultra-slim header with minimal chrome. Left-aligned branding,
 * right-aligned user controls (notifications, logout).
 *
 * @component
 * @returns {JSX.Element} Minimal terminal header
 *
 * @example
 * <TerminalHeader />
 */

import { useAuth } from '../../contexts/AuthContext';
import NotificationBell from '../notifications/NotificationBell';

const TerminalHeader = () => {
  const { user, logout } = useAuth();

  /**
   * Handle logout button click
   */
  const handleLogout = async () => {
    try {
      await logout();
    } catch (error) {
      console.error('Logout error:', error);
    }
  };

  return (
    <header className="terminal-header">
      <span className="terminal-header-brand">NEKZUS</span>

      {user && (
        <div className="terminal-header-controls">
          <span className="terminal-user-badge">{user.username?.toLowerCase() || 'admin'}</span>
          <span className="terminal-separator">|</span>
          <NotificationBell />
          <span className="terminal-separator">|</span>
          <button
            onClick={handleLogout}
            className="terminal-logout-btn"
            aria-label="Logout"
          >
            exit
          </button>
        </div>
      )}
    </header>
  );
};

export default TerminalHeader;
