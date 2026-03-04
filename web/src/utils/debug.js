/**
 * Debug Utility Module
 *
 * Provides conditional logging based on Developer Options settings.
 * Settings are read from localStorage to work in non-React contexts (like WebSocket service).
 */

const SETTINGS_STORAGE_KEY = 'nekzus-settings';

/**
 * Get current debug settings from localStorage
 * @returns {Object} Debug settings
 */
function getDebugSettings() {
  try {
    const saved = localStorage.getItem(SETTINGS_STORAGE_KEY);
    if (saved) {
      const settings = JSON.parse(saved);
      return {
        debugMode: settings.debugMode ?? false,
        showErrorDetails: settings.showErrorDetails ?? false,
        logWebSocketEvents: settings.logWebSocketEvents ?? false,
      };
    }
  } catch (error) {
    // Silently fail - don't log errors about logging
  }
  return {
    debugMode: false,
    showErrorDetails: false,
    logWebSocketEvents: false,
  };
}

/**
 * Debug logger - only logs when debugMode is enabled
 */
export const debug = {
  /**
   * Log a debug message (only when debugMode is enabled)
   * @param {string} category - Category/component name
   * @param {string} message - Log message
   * @param {...any} args - Additional arguments
   */
  log(category, message, ...args) {
    if (getDebugSettings().debugMode) {
      console.log(`[DEBUG:${category}]`, message, ...args);
    }
  },

  /**
   * Log a warning (only when debugMode is enabled)
   * @param {string} category - Category/component name
   * @param {string} message - Warning message
   * @param {...any} args - Additional arguments
   */
  warn(category, message, ...args) {
    if (getDebugSettings().debugMode) {
      console.warn(`[DEBUG:${category}]`, message, ...args);
    }
  },

  /**
   * Log an error with optional details (always logs, but details only when showErrorDetails is enabled)
   * @param {string} category - Category/component name
   * @param {string} message - Error message
   * @param {Error|any} error - Error object or details
   */
  error(category, message, error) {
    const settings = getDebugSettings();
    if (settings.showErrorDetails && error) {
      console.error(`[ERROR:${category}]`, message, {
        error,
        stack: error?.stack,
        details: error?.response?.data || error?.message,
      });
    } else {
      console.error(`[ERROR:${category}]`, message);
    }
  },

  /**
   * Log state changes (only when debugMode is enabled)
   * @param {string} component - Component name
   * @param {string} stateName - State variable name
   * @param {any} oldValue - Previous value
   * @param {any} newValue - New value
   */
  stateChange(component, stateName, oldValue, newValue) {
    if (getDebugSettings().debugMode) {
      console.log(`[STATE:${component}]`, stateName, { from: oldValue, to: newValue });
    }
  },

  /**
   * Log API calls (only when debugMode is enabled)
   * @param {string} method - HTTP method
   * @param {string} url - Request URL
   * @param {Object} [data] - Request data
   */
  api(method, url, data) {
    if (getDebugSettings().debugMode) {
      console.log(`[API]`, method.toUpperCase(), url, data ? { data } : '');
    }
  },

  /**
   * Log API response (only when debugMode is enabled)
   * @param {string} method - HTTP method
   * @param {string} url - Request URL
   * @param {number} status - Response status
   * @param {any} [data] - Response data
   */
  apiResponse(method, url, status, data) {
    if (getDebugSettings().debugMode) {
      console.log(`[API:${status}]`, method.toUpperCase(), url, data ? { data } : '');
    }
  },

  /**
   * Check if debug mode is enabled
   * @returns {boolean}
   */
  isEnabled() {
    return getDebugSettings().debugMode;
  },
};

/**
 * WebSocket event logger - only logs when logWebSocketEvents is enabled
 */
export const wsDebug = {
  /**
   * Log WebSocket connection event
   * @param {string} url - WebSocket URL
   */
  connect(url) {
    if (getDebugSettings().logWebSocketEvents) {
      console.log('%c[WS] Connecting', 'color: #4CAF50; font-weight: bold', url);
    }
  },

  /**
   * Log WebSocket open event
   */
  open() {
    if (getDebugSettings().logWebSocketEvents) {
      console.log('%c[WS] Connected', 'color: #4CAF50; font-weight: bold');
    }
  },

  /**
   * Log WebSocket close event
   * @param {number} code - Close code
   * @param {string} reason - Close reason
   */
  close(code, reason) {
    if (getDebugSettings().logWebSocketEvents) {
      console.log('%c[WS] Disconnected', 'color: #f44336; font-weight: bold', { code, reason });
    }
  },

  /**
   * Log WebSocket error
   * @param {Error} error - Error object
   */
  error(error) {
    if (getDebugSettings().logWebSocketEvents) {
      console.error('%c[WS] Error', 'color: #f44336; font-weight: bold', error);
    }
  },

  /**
   * Log WebSocket message sent
   * @param {string} type - Message type
   * @param {any} data - Message data
   */
  send(type, data) {
    if (getDebugSettings().logWebSocketEvents) {
      console.log('%c[WS] >>>', 'color: #2196F3; font-weight: bold', type, data);
    }
  },

  /**
   * Log WebSocket message received
   * @param {string} type - Message type
   * @param {any} data - Message data
   */
  receive(type, data) {
    if (getDebugSettings().logWebSocketEvents) {
      console.log('%c[WS] <<<', 'color: #9C27B0; font-weight: bold', type, data);
    }
  },

  /**
   * Log WebSocket reconnection attempt
   * @param {number} attempt - Attempt number
   * @param {number} maxAttempts - Maximum attempts
   * @param {number} delay - Delay in ms
   */
  reconnect(attempt, maxAttempts, delay) {
    if (getDebugSettings().logWebSocketEvents) {
      console.log(
        '%c[WS] Reconnecting',
        'color: #FF9800; font-weight: bold',
        `Attempt ${attempt}/${maxAttempts} in ${Math.round(delay / 1000)}s`
      );
    }
  },

  /**
   * Check if WebSocket logging is enabled
   * @returns {boolean}
   */
  isEnabled() {
    return getDebugSettings().logWebSocketEvents;
  },
};

/**
 * Error details helper - formats error for display based on showErrorDetails setting
 */
export const errorDetails = {
  /**
   * Get formatted error message
   * @param {Error|any} error - Error object
   * @param {string} fallbackMessage - Fallback message if details are hidden
   * @returns {string} Formatted error message
   */
  getMessage(error, fallbackMessage = 'An error occurred') {
    const settings = getDebugSettings();
    if (settings.showErrorDetails && error) {
      if (error.response?.data?.message) {
        return error.response.data.message;
      }
      if (error.message) {
        return error.message;
      }
      if (typeof error === 'string') {
        return error;
      }
    }
    return fallbackMessage;
  },

  /**
   * Get full error details object for display
   * @param {Error|any} error - Error object
   * @returns {Object|null} Error details or null if showErrorDetails is disabled
   */
  getDetails(error) {
    const settings = getDebugSettings();
    if (!settings.showErrorDetails || !error) {
      return null;
    }

    return {
      message: error.message || 'Unknown error',
      code: error.code || error.response?.status,
      stack: error.stack,
      response: error.response?.data,
    };
  },

  /**
   * Check if error details should be shown
   * @returns {boolean}
   */
  isEnabled() {
    return getDebugSettings().showErrorDetails;
  },
};

/**
 * React hook for debug logging within components
 * @param {string} componentName - Name of the component
 * @returns {Object} Debug logging functions scoped to the component
 */
export function useDebug(componentName) {
  return {
    log: (message, ...args) => debug.log(componentName, message, ...args),
    warn: (message, ...args) => debug.warn(componentName, message, ...args),
    error: (message, error) => debug.error(componentName, message, error),
    stateChange: (stateName, oldValue, newValue) =>
      debug.stateChange(componentName, stateName, oldValue, newValue),
    render: () => {
      if (getDebugSettings().debugMode) {
        console.log(`%c[RENDER:${componentName}]`, 'color: #888; font-style: italic');
      }
    },
    mount: () => debug.log(componentName, 'Mounted'),
    unmount: () => debug.log(componentName, 'Unmounted'),
    effect: (effectName) => debug.log(componentName, `Effect: ${effectName}`),
  };
}

export default { debug, wsDebug, errorDetails, useDebug };
