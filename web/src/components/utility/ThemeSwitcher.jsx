import { useTheme } from '../../contexts/ThemeContext';
import CustomDropdown from '../forms/CustomDropdown';

/**
 * ThemeSwitcher Component
 *
 * Provides a dropdown interface for selecting application themes.
 * Integrates with ThemeContext to apply theme changes globally.
 * The current theme selection persists to localStorage.
 *
 * Available themes:
 * - Classic Green (default)
 * - Cyan
 * - Amber
 * - Gruvbox
 * - Gruvbox Light
 * - Tokyo Night
 * - Tokyo Night Storm
 * - Catppuccin Mocha
 * - Pipboy
 * - Pipboy Green
 * - Retro
 *
 * @component
 * @returns {JSX.Element} Theme selection dropdown
 *
 * @example
 * // Basic usage in settings panel
 * <div className="form-group">
 *   <label>THEME</label>
 *   <ThemeSwitcher />
 * </div>
 */
function ThemeSwitcher() {
  const { theme, setTheme, themes, themeNames } = useTheme();

  // Convert themes object to dropdown options format
  const themeOptions = Object.values(themes).map(themeValue => ({
    value: themeValue,
    label: themeNames[themeValue]
  }));

  /**
   * Handle theme selection change
   * @param {string} selectedTheme - The theme value selected
   */
  const handleThemeChange = (selectedTheme) => {
    setTheme(selectedTheme);
  };

  return (
    <CustomDropdown
      options={themeOptions}
      value={theme}
      onChange={handleThemeChange}
      placeholder="Select Theme"
      className="theme-dropdown"
    />
  );
}

export default ThemeSwitcher;
