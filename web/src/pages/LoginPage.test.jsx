/**
 * LoginPage Tests
 *
 * Test suite for the login page component.
 * Tests form validation, submission, error handling, and accessibility.
 */

import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { AuthProvider } from '../contexts/AuthContext';
import { SettingsProvider } from '../contexts/SettingsContext';
import LoginPage from './LoginPage';

// Mock fetch
global.fetch = vi.fn();

describe('LoginPage', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.clear();
  });

  const renderLoginPage = () => {
    return render(
      <SettingsProvider>
        <AuthProvider>
          <LoginPage />
        </AuthProvider>
      </SettingsProvider>
    );
  };

  it('should render login form with all elements', () => {
    renderLoginPage();

    // Check for form title
    expect(screen.getByText(/ADMIN AUTHENTICATION/i)).toBeInTheDocument();

    // Check for input fields
    expect(screen.getByPlaceholderText(/username/i)).toBeInTheDocument();
    expect(screen.getByPlaceholderText(/password/i)).toBeInTheDocument();

    // Check for login button
    expect(screen.getByRole('button', { name: /login/i })).toBeInTheDocument();

    // Check for footer
    expect(screen.getByText(/NEKZUS/i)).toBeInTheDocument();
  });

  it('should show validation error for empty username', async () => {
    renderLoginPage();

    const passwordInput = screen.getByPlaceholderText(/password/i);
    const loginButton = screen.getByRole('button', { name: /login/i });

    fireEvent.change(passwordInput, { target: { value: 'password123' } });
    fireEvent.click(loginButton);

    await waitFor(() => {
      expect(screen.getByText(/username is required/i)).toBeInTheDocument();
    });
  });

  it('should show validation error for empty password', async () => {
    renderLoginPage();

    const usernameInput = screen.getByPlaceholderText(/username/i);
    const loginButton = screen.getByRole('button', { name: /login/i });

    fireEvent.change(usernameInput, { target: { value: 'admin' } });
    fireEvent.click(loginButton);

    await waitFor(() => {
      expect(screen.getByText(/password is required/i)).toBeInTheDocument();
    });
  });

  it('should submit login form with valid credentials', async () => {
    global.fetch.mockResolvedValueOnce({
      ok: true,
      status: 200,
      json: async () => ({
        token: 'test-token',
        user: { id: '1', username: 'admin' },
      }),
    });

    renderLoginPage();

    const usernameInput = screen.getByPlaceholderText(/username/i);
    const passwordInput = screen.getByPlaceholderText(/password/i);
    const loginButton = screen.getByRole('button', { name: /login/i });

    fireEvent.change(usernameInput, { target: { value: 'admin' } });
    fireEvent.change(passwordInput, { target: { value: 'password123' } });
    fireEvent.click(loginButton);

    await waitFor(() => {
      expect(global.fetch).toHaveBeenCalledWith(
        '/api/v1/auth/login',
        expect.objectContaining({
          method: 'POST',
          body: JSON.stringify({ username: 'admin', password: 'password123' }),
        })
      );
    });
  });

  it('should show error message for invalid credentials', async () => {
    global.fetch.mockResolvedValueOnce({
      ok: false,
      status: 401,
      statusText: 'Unauthorized',
      headers: new Headers({ 'content-type': 'application/json' }),
      json: async () => ({ message: 'Invalid credentials' }),
    });

    renderLoginPage();

    const usernameInput = screen.getByPlaceholderText(/username/i);
    const passwordInput = screen.getByPlaceholderText(/password/i);
    const loginButton = screen.getByRole('button', { name: /login/i });

    fireEvent.change(usernameInput, { target: { value: 'admin' } });
    fireEvent.change(passwordInput, { target: { value: 'wrongpassword' } });
    fireEvent.click(loginButton);

    await waitFor(() => {
      expect(screen.getByText(/invalid credentials/i)).toBeInTheDocument();
    });
  });

  it('should show loading state during login', async () => {
    global.fetch.mockImplementationOnce(
      () =>
        new Promise((resolve) =>
          setTimeout(
            () =>
              resolve({
                ok: true,
                status: 200,
                json: async () => ({
                  token: 'test-token',
                  user: { id: '1', username: 'admin' },
                }),
              }),
            100
          )
        )
    );

    renderLoginPage();

    const usernameInput = screen.getByPlaceholderText(/username/i);
    const passwordInput = screen.getByPlaceholderText(/password/i);
    const loginButton = screen.getByRole('button', { name: /login/i });

    fireEvent.change(usernameInput, { target: { value: 'admin' } });
    fireEvent.change(passwordInput, { target: { value: 'password123' } });
    fireEvent.click(loginButton);

    // Should show loading state
    expect(screen.getByText(/authenticating/i)).toBeInTheDocument();
    expect(loginButton).toBeDisabled();
  });

  it('should disable login button when loading', async () => {
    global.fetch.mockImplementationOnce(
      () =>
        new Promise((resolve) =>
          setTimeout(
            () =>
              resolve({
                ok: true,
                status: 200,
                json: async () => ({
                  token: 'test-token',
                  user: { id: '1', username: 'admin' },
                }),
              }),
            100
          )
        )
    );

    renderLoginPage();

    const usernameInput = screen.getByPlaceholderText(/username/i);
    const passwordInput = screen.getByPlaceholderText(/password/i);
    const loginButton = screen.getByRole('button', { name: /login/i });

    fireEvent.change(usernameInput, { target: { value: 'admin' } });
    fireEvent.change(passwordInput, { target: { value: 'password123' } });
    fireEvent.click(loginButton);

    expect(loginButton).toBeDisabled();
  });

  it('should have accessible form labels', () => {
    renderLoginPage();

    const usernameInput = screen.getByPlaceholderText(/username/i);
    const passwordInput = screen.getByPlaceholderText(/password/i);

    expect(usernameInput).toHaveAttribute('type', 'text');
    expect(usernameInput).toHaveAttribute('aria-label', 'Username');

    expect(passwordInput).toHaveAttribute('type', 'password');
    expect(passwordInput).toHaveAttribute('aria-label', 'Password');
  });

  it('should allow form submission with Enter key', async () => {
    global.fetch.mockResolvedValueOnce({
      ok: true,
      status: 200,
      json: async () => ({
        token: 'test-token',
        user: { id: '1', username: 'admin' },
      }),
    });

    renderLoginPage();

    const usernameInput = screen.getByPlaceholderText(/username/i);
    const passwordInput = screen.getByPlaceholderText(/password/i);

    fireEvent.change(usernameInput, { target: { value: 'admin' } });
    fireEvent.change(passwordInput, { target: { value: 'password123' } });
    fireEvent.keyPress(passwordInput, { key: 'Enter', code: 13, charCode: 13 });

    await waitFor(() => {
      expect(global.fetch).toHaveBeenCalled();
    });
  });
});
