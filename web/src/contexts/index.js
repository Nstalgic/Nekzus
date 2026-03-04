/**
 * Context Providers
 *
 * Barrel export file for all context providers and hooks.
 * Import contexts from this file for cleaner imports.
 *
 * @example
 * import { ThemeProvider, useTheme, SettingsProvider, useSettings } from '@/contexts';
 */

export {
  ThemeProvider,
  useTheme,
  default as ThemeContext,
} from './ThemeContext.jsx';

export {
  SettingsProvider,
  useSettings,
  default as SettingsContext,
} from './SettingsContext.jsx';

export {
  DataProvider,
  useData,
  default as DataContext,
} from './DataContext.jsx';

export {
  AuthProvider,
  useAuth,
  default as AuthContext,
} from './AuthContext.jsx';
