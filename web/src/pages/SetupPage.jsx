/**
 * SetupPage Component
 *
 * First-time setup page for creating the initial admin account.
 * Only shown when no users exist in the database.
 *
 * Features:
 * - Terminal aesthetic with monospace fonts and CRT styling
 * - Username and password creation with confirmation
 * - Real-time validation for password matching
 * - Error message display for invalid inputs
 * - Loading state during account creation
 * - Keyboard navigation support (Enter to submit)
 * - Accessibility attributes (ARIA labels)
 * - Redirects to login page after successful setup
 *
 * @component
 * @returns {JSX.Element} Setup page
 *
 * @example
 * <SetupPage />
 */

import { useState } from 'react';
import Input from '../components/forms/Input';
import '../styles/pages/login-page.css'; // Reuse login page styles

function SetupPage() {
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [error, setError] = useState('');
  const [success, setSuccess] = useState('');
  const [isLoading, setIsLoading] = useState(false);

  /**
   * Validate form inputs
   * @returns {boolean} True if form is valid
   */
  const validateForm = () => {
    if (!username.trim()) {
      setError('Username is required');
      return false;
    }

    if (username.length < 3) {
      setError('Username must be at least 3 characters');
      return false;
    }

    if (!password.trim()) {
      setError('Password is required');
      return false;
    }

    if (password.length < 8) {
      setError('Password must be at least 8 characters');
      return false;
    }

    if (password !== confirmPassword) {
      setError('Passwords do not match');
      return false;
    }

    return true;
  };

  /**
   * Handle form submission
   * @param {Event} e - Form submit event
   */
  const handleSubmit = async (e) => {
    e.preventDefault();
    setError('');
    setSuccess('');

    // Validate inputs
    if (!validateForm()) {
      return;
    }

    setIsLoading(true);

    try {
      const response = await fetch('/api/v1/auth/setup', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({ username, password }),
      });

      const data = await response.json();

      if (!response.ok) {
        setError(data.message || 'Setup failed. Please try again.');
        return;
      }

      // Success! Show success message and reload page after 2 seconds
      // The page will reload and show the login page since a user now exists
      setSuccess(`Account created successfully! Redirecting to login...`);
      setTimeout(() => {
        window.location.reload();
      }, 2000);
    } catch (err) {
      console.error('Setup error:', err);
      setError('Network error: Unable to connect to server');
    } finally {
      setIsLoading(false);
    }
  };

  /**
   * Handle Enter key press in inputs
   * @param {Event} e - Keyboard event
   */
  const handleKeyPress = (e) => {
    if (e.key === 'Enter') {
      handleSubmit(e);
    }
  };

  return (
    <div className="login-page">
      <div className="login-container">
        {/* Setup Form Title */}
        <h1 className="login-title">Initial Setup</h1>

        {/* Instructions */}
        <div style={{
          marginBottom: 'var(--spacing-6)',
          textAlign: 'center',
          color: 'var(--text-secondary)',
          fontSize: 'var(--text-sm)',
          lineHeight: '1.5'
        }}>
          <p>Welcome! Let's set up your admin account to get started.</p>
        </div>

        {/* Setup Form */}
        <form className="login-form" onSubmit={handleSubmit}>
          {/* Username Input */}
          <div className="form-group">
            <Input
              type="text"
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              onKeyPress={handleKeyPress}
              placeholder="Username"
              className="login-input"
              disabled={isLoading || !!success}
              autoComplete="username"
              aria-label="Username"
              autoFocus
            />
          </div>

          {/* Password Input */}
          <div className="form-group">
            <Input
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              onKeyPress={handleKeyPress}
              placeholder="Password (min 8 characters)"
              className="login-input"
              disabled={isLoading || !!success}
              autoComplete="new-password"
              aria-label="Password"
            />
          </div>

          {/* Confirm Password Input */}
          <div className="form-group">
            <Input
              type="password"
              value={confirmPassword}
              onChange={(e) => setConfirmPassword(e.target.value)}
              onKeyPress={handleKeyPress}
              placeholder="Confirm Password"
              className="login-input"
              disabled={isLoading || !!success}
              autoComplete="new-password"
              aria-label="Confirm Password"
            />
          </div>

          {/* Error Message */}
          {error && (
            <div className="login-error" role="alert" aria-live="polite">
              {error}
            </div>
          )}

          {/* Success Message */}
          {success && (
            <div
              style={{
                padding: 'var(--spacing-3) var(--spacing-4)',
                background: 'rgba(16, 185, 129, 0.1)',
                border: '2px solid var(--success)',
                borderRadius: 'var(--radius-sm)',
                color: 'var(--success)',
                fontSize: 'var(--text-sm)',
                textAlign: 'center',
                fontWeight: 600,
                letterSpacing: '0.05em',
                textTransform: 'uppercase'
              }}
              role="alert"
              aria-live="polite"
            >
              {success}
            </div>
          )}

          {/* Submit Button */}
          <button
            type="submit"
            className="btn btn-primary login-button"
            disabled={isLoading || !!success}
            aria-label={isLoading ? 'Creating Account' : 'Create Account'}
          >
            {isLoading ? 'CREATING ACCOUNT...' : 'CREATE ACCOUNT'}
          </button>
        </form>

        {/* Footer Info */}
        <div className="login-footer">
          <p className="text-tertiary">NEKZUS ADMIN PORTAL</p>
        </div>
      </div>
    </div>
  );
}

export default SetupPage;
