import { createContext, useContext, useState, useEffect } from 'react';
import PropTypes from 'prop-types';

/**
 * Available theme names
 * @constant {Object} THEMES
 */
const THEMES = {
  SLATE_PROFESSIONAL: 'slate-professional',
  OBSIDIAN_DARK: 'obsidian-dark',
  NORD_FROST: 'nord-frost',
  CARBON_NEUTRAL: 'carbon-neutral',
  GREEN: 'green',
  CYAN: 'cyan',
  AMBER: 'amber',
  GRUVBOX: 'gruvbox',
  GRUVBOX_LIGHT: 'gruvbox-light',
  TOKYO_NIGHT: 'tokyo-night',
  TOKYO_NIGHT_STORM: 'tokyo-night-storm',
  CATPPUCCIN_MOCHA: 'catppuccin-mocha',
  PIPBOY: 'pipboy',
  PIPBOY_GREEN: 'pipboy-green',
  RETRO: 'retro',
};

/**
 * Display names for each theme
 * @constant {Object} THEME_NAMES
 */
const THEME_NAMES = {
  [THEMES.SLATE_PROFESSIONAL]: 'Default',
  [THEMES.OBSIDIAN_DARK]: 'Obsidian Dark',
  [THEMES.NORD_FROST]: 'Nord Frost',
  [THEMES.CARBON_NEUTRAL]: 'Carbon Neutral',
  [THEMES.GREEN]: 'Classic Green',
  [THEMES.CYAN]: 'Cyan',
  [THEMES.AMBER]: 'Amber',
  [THEMES.GRUVBOX]: 'Gruvbox',
  [THEMES.GRUVBOX_LIGHT]: 'Gruvbox Light',
  [THEMES.TOKYO_NIGHT]: 'Tokyo Night',
  [THEMES.TOKYO_NIGHT_STORM]: 'Tokyo Night Storm',
  [THEMES.CATPPUCCIN_MOCHA]: 'Catppuccin Mocha',
  [THEMES.PIPBOY]: 'Pipboy',
  [THEMES.PIPBOY_GREEN]: 'Pipboy Green',
  [THEMES.RETRO]: 'Retro',
};

/**
 * LocalStorage key for theme persistence
 * @constant {string}
 */
const THEME_STORAGE_KEY = 'nekzus-theme';

/**
 * Default theme
 * @constant {string}
 */
const DEFAULT_THEME = THEMES.SLATE_PROFESSIONAL;

/**
 * Theme Context
 * @type {React.Context}
 */
const ThemeContext = createContext(null);

/**
 * ThemeProvider Component
 *
 * Provides theme management functionality throughout the application.
 * Manages theme state, applies theme classes to the body element,
 * and persists theme preference to localStorage.
 *
 * Themes are applied via body classes:
 * - 'green' theme: no class (default)
 * - Other themes: body.theme-{name}
 *
 * @component
 * @param {Object} props - Component props
 * @param {React.ReactNode} props.children - Child components
 * @returns {JSX.Element} Theme provider wrapper
 *
 * @example
 * <ThemeProvider>
 *   <App />
 * </ThemeProvider>
 */
export const ThemeProvider = ({ children }) => {
  const [theme, setThemeState] = useState(() => {
    // Load theme from localStorage on initialization
    const savedTheme = localStorage.getItem(THEME_STORAGE_KEY);
    return savedTheme && Object.values(THEMES).includes(savedTheme)
      ? savedTheme
      : DEFAULT_THEME;
  });

  /**
   * Apply theme class to body element
   * @param {string} themeName - Theme to apply
   */
  const applyThemeClass = (themeName) => {
    // Remove all existing theme classes
    Object.values(THEMES).forEach((t) => {
      if (t !== THEMES.SLATE_PROFESSIONAL) {
        document.body.classList.remove(`theme-${t}`);
      }
    });

    // Apply new theme class (unless it's the default Slate Professional theme)
    if (themeName !== THEMES.SLATE_PROFESSIONAL) {
      document.body.classList.add(`theme-${themeName}`);
    }
  };

  /**
   * Set theme and persist to localStorage
   * @param {string} newTheme - Theme name to set
   */
  const setTheme = (newTheme) => {
    if (!Object.values(THEMES).includes(newTheme)) {
      console.warn(`Invalid theme: ${newTheme}. Using default theme.`);
      newTheme = DEFAULT_THEME;
    }

    setThemeState(newTheme);
    localStorage.setItem(THEME_STORAGE_KEY, newTheme);
    applyThemeClass(newTheme);
  };

  // Apply theme on mount and when theme changes
  useEffect(() => {
    applyThemeClass(theme);
  }, [theme]);

  const value = {
    theme,
    setTheme,
    themes: THEMES,
    themeNames: THEME_NAMES,
  };

  return (
    <ThemeContext.Provider value={value}>
      {children}
    </ThemeContext.Provider>
  );
};

ThemeProvider.propTypes = {
  children: PropTypes.node.isRequired,
};

/**
 * useTheme Hook
 *
 * Custom hook to access theme context.
 * Must be used within a ThemeProvider.
 *
 * @returns {Object} Theme context value
 * @returns {string} return.theme - Current theme name
 * @returns {Function} return.setTheme - Function to change theme
 * @returns {Object} return.themes - Available theme constants
 * @returns {Object} return.themeNames - Display names for themes
 *
 * @throws {Error} If used outside of ThemeProvider
 *
 * @example
 * const { theme, setTheme, themes, themeNames } = useTheme();
 *
 * // Change theme
 * setTheme(themes.RETRO);
 *
 * // Get current theme display name
 * const displayName = themeNames[theme];
 */
export const useTheme = () => {
  const context = useContext(ThemeContext);

  if (!context) {
    throw new Error('useTheme must be used within a ThemeProvider');
  }

  return context;
};

export default ThemeContext;
