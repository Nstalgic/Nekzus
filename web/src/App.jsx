import {
  Container,
  TerminalHeader,
  TerminalFooter,
  TerminalContent,
} from './components/layout';
import { ThemeProvider, SettingsProvider, DataProvider, AuthProvider, useAuth } from './contexts';
import { NotificationProvider } from './contexts/NotificationContext';
import ToastContainer from './components/notifications/ToastContainer';
import DashboardPage from './pages/DashboardPage';
import LoginPage from './pages/LoginPage';
import SetupPage from './pages/SetupPage';
import { useState, useEffect } from 'react';

/**
 * LoadingScreen Component
 *
 * Displays a loading screen while authentication state is being checked.
 * Uses terminal aesthetic to match the rest of the application.
 *
 * @component
 * @returns {JSX.Element} Loading screen
 */
function LoadingScreen() {
  return (
    <div className="login-page">
      <div className="login-container">
        <div style={{ textAlign: 'center' }}>
          <h1 style={{ color: 'var(--accent-primary)', fontSize: '2rem', letterSpacing: '0.2em', marginBottom: '2rem' }}>
            NEKZUS
          </h1>
          <p style={{ color: 'var(--accent-primary)', textTransform: 'uppercase', letterSpacing: '0.1em' }}>
            INITIALIZING...
          </p>
        </div>
      </div>
    </div>
  );
}

/**
 * AuthGate Component
 *
 * Controls access to the dashboard based on authentication and setup status.
 * Displays loading screen, setup page, login page, or dashboard depending on state.
 *
 * Authentication Flow:
 * 1. isLoading === true OR checkingSetup === true: Show loading screen
 * 2. setupRequired === true: Show setup page (first-time setup)
 * 3. isAuthenticated === false: Show login page
 * 4. isAuthenticated === true: Show dashboard
 *
 * @component
 * @returns {JSX.Element} Appropriate screen based on auth and setup state
 */
function AuthGate() {
  const { isAuthenticated, isLoading } = useAuth();
  const [setupRequired, setSetupRequired] = useState(false);
  const [checkingSetup, setCheckingSetup] = useState(true);

  // Check if initial setup is required (no users exist)
  useEffect(() => {
    const checkSetupStatus = async () => {
      console.log('[App] checking setup status', {
        protocol: window.location.protocol,
        host: window.location.host,
        pathname: window.location.pathname,
      });

      try {
        const response = await fetch('/api/v1/auth/setup-status');

        console.log('[App] setup-status response', {
          status: response.status,
          statusText: response.statusText,
          ok: response.ok,
        });

        if (response.ok) {
          const data = await response.json();
          console.log('[App] setup-status data', {
            setupRequired: data.setupRequired,
            hasUsers: data.hasUsers,
          });
          setSetupRequired(data.setupRequired);
        } else {
          console.warn('[App] setup-status request failed', {
            status: response.status,
            statusText: response.statusText,
          });
          // Assume setup is not required if API fails
          setSetupRequired(false);
        }
      } catch (error) {
        console.error('[App] setup-status network error', {
          message: error.message,
          name: error.name,
        });
        // Assume setup is not required if we can't check
        setSetupRequired(false);
      } finally {
        setCheckingSetup(false);
      }
    };

    checkSetupStatus();
  }, []);

  // Log routing decision
  console.log('[App] AuthGate routing decision', {
    isLoading,
    checkingSetup,
    setupRequired,
    isAuthenticated,
    willShow: isLoading || checkingSetup ? 'loading' : setupRequired ? 'setup' : !isAuthenticated ? 'login' : 'dashboard',
  });

  // Show loading screen while checking authentication or setup status
  if (isLoading || checkingSetup) {
    return <LoadingScreen />;
  }

  // Show setup page if initial setup is required
  if (setupRequired) {
    return <SetupPage />;
  }

  // Show login page if not authenticated
  if (!isAuthenticated) {
    return <LoginPage />;
  }

  // Show dashboard if authenticated
  return (
    <DataProvider>
      <Container>
        <TerminalHeader />

        <TerminalContent>
          {/* Main Dashboard */}
          <DashboardPage />
        </TerminalContent>

        <TerminalFooter version="1.0.0" />
      </Container>

      {/* Toast notifications */}
      <ToastContainer />
    </DataProvider>
  );
}

/**
 * App Component
 *
 * Root application component that sets up context providers and
 * renders the complete terminal dashboard interface with authentication.
 *
 * Provider Hierarchy:
 * - ThemeProvider: Manages theme state and persistence (outermost)
 * - SettingsProvider: Manages application settings (required by AuthProvider)
 * - NotificationProvider: Manages notification state and toast system
 * - AuthProvider: Manages authentication state and JWT tokens
 * - AuthGate: Controls access to dashboard based on auth state
 *   - DataProvider: Loads data only when authenticated (inside AuthGate)
 *
 * @component
 * @returns {JSX.Element} Application root
 */
function App() {
  return (
    <ThemeProvider>
      <SettingsProvider>
        <NotificationProvider>
          <AuthProvider>
            <AuthGate />
          </AuthProvider>
        </NotificationProvider>
      </SettingsProvider>
    </ThemeProvider>
  );
}

export default App;
