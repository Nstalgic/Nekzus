import { createContext, useContext, useState, useEffect } from 'react';
import PropTypes from 'prop-types';

/**
 * LocalStorage key for settings persistence
 * @constant {string}
 */
const SETTINGS_STORAGE_KEY = 'nekzus-settings';

/**
 * Generate a random webhook ID (UUID v4)
 * @returns {string} UUID v4 string
 */
const generateWebhookId = () => {
  if (typeof crypto !== 'undefined' && crypto.randomUUID) {
    return crypto.randomUUID();
  }
  // Fallback for older browsers
  return 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, function(c) {
    const r = Math.random() * 16 | 0;
    const v = c === 'x' ? r : (r & 0x3 | 0x8);
    return v.toString(16);
  });
};

/**
 * Default settings configuration
 * @constant {Object}
 */
const DEFAULT_SETTINGS = {
  // General Settings
  refreshInterval: 10,
  timezone: 'UTC',
  showTimestamp: true,

  // Feature Toggles
  enableToolbox: false, // Toolbox tab visibility (hidden by default)
  enableScripts: false, // Scripts tab visibility (hidden by default)
  enableFederation: false, // Federation tab visibility (hidden by default)
  showOnlyRoutedContainers: false, // Show all containers by default

  // Discovery Settings
  autoApprovalThreshold: 100,
  notificationBadgeThreshold: 5,
  requireConfirmationForRejections: false,

  // Security Settings
  sessionTimeout: 30,
  requireConfirmation: true,

  // Appearance Settings
  terminalTheme: 'slate-professional',
  fontSize: 'medium',
  compactMode: false,

  // Webhook Settings
  webhookId: generateWebhookId(), // Generated once on first load
  webhookKey: '', // Generated on demand by user via backend API
  webhookKeyId: '', // Backend API key ID (for management)

  // Notification Settings
  notifyNewDiscoveries: true,
  notifyDeviceOffline: true,
  notifyCertificateExpiry: true,
  notifyRouteStatusChange: false,
  notifySystemHealth: false,

  // Developer Options
  debugMode: false,
  showErrorDetails: false,
  logWebSocketEvents: false,
};

/**
 * Settings Context
 * @type {React.Context}
 */
const SettingsContext = createContext(null);

/**
 * SettingsProvider Component
 *
 * Provides settings management functionality throughout the application.
 * Manages settings state, persists to localStorage, and provides methods
 * to update individual settings or reset to defaults.
 *
 * @component
 * @param {Object} props - Component props
 * @param {React.ReactNode} props.children - Child components
 * @returns {JSX.Element} Settings provider wrapper
 *
 * @example
 * <SettingsProvider>
 *   <App />
 * </SettingsProvider>
 */
export const SettingsProvider = ({ children }) => {
  const [settings, setSettings] = useState(() => {
    // Load settings from localStorage on initialization
    try {
      const savedSettings = localStorage.getItem(SETTINGS_STORAGE_KEY);
      if (savedSettings) {
        const parsed = JSON.parse(savedSettings);
        // Ensure webhookId exists (for existing users)
        if (!parsed.webhookId) {
          parsed.webhookId = generateWebhookId();
        }
        // Merge with defaults to ensure new settings are included
        return { ...DEFAULT_SETTINGS, ...parsed };
      }
    } catch (error) {
      console.error('Failed to load settings from localStorage:', error);
    }
    return DEFAULT_SETTINGS;
  });

  /**
   * Persist settings to localStorage
   */
  const persistSettings = (newSettings) => {
    try {
      localStorage.setItem(SETTINGS_STORAGE_KEY, JSON.stringify(newSettings));
    } catch (error) {
      console.error('Failed to save settings to localStorage:', error);
    }
  };

  /**
   * Update a single setting or multiple settings
   * @param {string|Object} keyOrObject - Setting key or object with multiple settings
   * @param {*} [value] - Setting value (if keyOrObject is a string)
   *
   * @example
   * // Update single setting
   * updateSetting('fontSize', 'large');
   *
   * // Update multiple settings
   * updateSetting({ fontSize: 'large', compactMode: true });
   */
  const updateSetting = (keyOrObject, value) => {
    setSettings((prevSettings) => {
      let newSettings;

      if (typeof keyOrObject === 'object') {
        // Multiple settings update
        newSettings = { ...prevSettings, ...keyOrObject };
      } else {
        // Single setting update
        newSettings = { ...prevSettings, [keyOrObject]: value };
      }

      persistSettings(newSettings);
      return newSettings;
    });
  };

  /**
   * Reset all settings to default values
   */
  const resetSettings = () => {
    setSettings(DEFAULT_SETTINGS);
    persistSettings(DEFAULT_SETTINGS);
  };

  /**
   * Get a specific setting value
   * @param {string} key - Setting key
   * @param {*} [defaultValue] - Default value if setting doesn't exist
   * @returns {*} Setting value
   */
  const getSetting = (key, defaultValue = null) => {
    return settings[key] !== undefined ? settings[key] : defaultValue;
  };

  // Apply certain settings to document/body when they change
  useEffect(() => {
    // Apply font size class (matches dashboard.html implementation)
    // Remove all font size classes first
    document.body.classList.remove('font-small', 'font-medium', 'font-large');

    // Add the selected font size class
    if (settings.fontSize) {
      document.body.classList.add(`font-${settings.fontSize}`);
    }

    // Apply compact mode class (matches dashboard.html implementation)
    if (settings.compactMode) {
      document.body.classList.add('compact-mode');
    } else {
      document.body.classList.remove('compact-mode');
    }
  }, [settings.fontSize, settings.compactMode]);

  const value = {
    settings,
    updateSetting,
    resetSettings,
    getSetting,
    defaults: DEFAULT_SETTINGS,
  };

  return (
    <SettingsContext.Provider value={value}>
      {children}
    </SettingsContext.Provider>
  );
};

SettingsProvider.propTypes = {
  children: PropTypes.node.isRequired,
};

/**
 * useSettings Hook
 *
 * Custom hook to access settings context.
 * Must be used within a SettingsProvider.
 *
 * @returns {Object} Settings context value
 * @returns {Object} return.settings - Current settings object
 * @returns {Function} return.updateSetting - Function to update settings
 * @returns {Function} return.resetSettings - Function to reset to defaults
 * @returns {Function} return.getSetting - Function to get a specific setting
 * @returns {Object} return.defaults - Default settings object
 *
 * @throws {Error} If used outside of SettingsProvider
 *
 * @example
 * const { settings, updateSetting, resetSettings, getSetting } = useSettings();
 *
 * // Get a setting
 * const fontSize = getSetting('fontSize', 'medium');
 *
 * // Update a setting
 * updateSetting('fontSize', 'large');
 *
 * // Update multiple settings
 * updateSetting({ fontSize: 'large', compactMode: true });
 *
 * // Reset all settings
 * resetSettings();
 */
export const useSettings = () => {
  const context = useContext(SettingsContext);

  if (!context) {
    throw new Error('useSettings must be used within a SettingsProvider');
  }

  return context;
};

export default SettingsContext;
