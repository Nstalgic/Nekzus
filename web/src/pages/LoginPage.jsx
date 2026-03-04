/**
 * LoginPage Component
 *
 * Full-page terminal-themed login screen for admin authentication.
 * Displays centered login form with ASCII logo, username/password inputs,
 * and handles authentication errors and loading states.
 *
 * Features:
 * - Terminal aesthetic with monospace fonts and CRT styling
 * - Real-time validation for username and password fields
 * - Error message display for invalid credentials
 * - Loading state during authentication
 * - Keyboard navigation support (Enter to submit)
 * - Accessibility attributes (ARIA labels)
 *
 * @component
 * @returns {JSX.Element} Login page
 *
 * @example
 * <LoginPage />
 */

import { useState } from 'react';
import PropTypes from 'prop-types';
import { useAuth } from '../contexts/AuthContext';
import Input from '../components/forms/Input';
import '../styles/pages/login-page.css';

function LoginPage() {
  const { login } = useAuth();
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
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
    if (!password.trim()) {
      setError('Password is required');
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

    // Validate inputs
    if (!validateForm()) {
      return;
    }

    setIsLoading(true);

    try {
      const result = await login(username, password);

      if (!result.success) {
        setError(result.error);
      }
      // If successful, AuthProvider will update state and App.jsx will redirect
    } catch (err) {
      console.error('Login error:', err);
      setError('An unexpected error occurred. Please try again.');
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
        {/* Login Form Title */}
        <h1 className="login-title">ADMIN AUTHENTICATION</h1>

        {/* Login Form */}
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
              disabled={isLoading}
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
              placeholder="Password"
              className="login-input"
              disabled={isLoading}
              autoComplete="current-password"
              aria-label="Password"
            />
          </div>

          {/* Error Message */}
          {error && (
            <div className="login-error" role="alert" aria-live="polite">
              {error}
            </div>
          )}

          {/* Submit Button */}
          <button
            type="submit"
            className="btn btn-primary login-button"
            disabled={isLoading}
            aria-label={isLoading ? 'Authenticating' : 'Login'}
          >
            {isLoading ? 'AUTHENTICATING...' : 'LOGIN'}
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

LoginPage.propTypes = {};

export default LoginPage;
