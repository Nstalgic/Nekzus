/**
 * Date Formatting Utilities
 *
 * Provides timezone-aware date formatting for Nekzus.
 * Uses Intl.DateTimeFormat for robust timezone support.
 *
 * @module utils/dateFormat
 */

/**
 * Format a timestamp with timezone support
 *
 * @param {Date|string|number} date - Date to format
 * @param {string} [timezone='UTC'] - IANA timezone name (e.g., 'America/New_York', 'UTC')
 * @param {Object} [options] - Additional formatting options
 * @param {boolean} [options.includeSeconds=true] - Include seconds in output
 * @param {boolean} [options.includeDate=true] - Include date in output
 * @param {boolean} [options.includeTime=true] - Include time in output
 * @returns {string} Formatted timestamp
 *
 * @example
 * formatTimestamp(new Date(), 'America/New_York')
 * // => "2025-11-10 14:30:45 EST"
 *
 * formatTimestamp(Date.now(), 'UTC', { includeSeconds: false })
 * // => "2025-11-10 19:30 UTC"
 */
export function formatTimestamp(date, timezone = 'UTC', options = {}) {
  const {
    includeSeconds = true,
    includeDate = true,
    includeTime = true,
  } = options;

  try {
    const dateObj = date instanceof Date ? date : new Date(date);

    // Check for invalid date
    if (isNaN(dateObj.getTime())) {
      console.error('Invalid date provided to formatTimestamp:', date);
      return 'Invalid Date';
    }

    const parts = [];

    // Format date part: YYYY-MM-DD
    if (includeDate) {
      const dateFormat = new Intl.DateTimeFormat('en-CA', {
        timeZone: timezone,
        year: 'numeric',
        month: '2-digit',
        day: '2-digit',
      });
      parts.push(dateFormat.format(dateObj));
    }

    // Format time part: HH:MM:SS or HH:MM
    if (includeTime) {
      const timeFormatOptions = {
        timeZone: timezone,
        hour: '2-digit',
        minute: '2-digit',
        hour12: false,
      };

      if (includeSeconds) {
        timeFormatOptions.second = '2-digit';
      }

      const timeFormat = new Intl.DateTimeFormat('en-GB', timeFormatOptions);
      parts.push(timeFormat.format(dateObj));
    }

    // Get timezone abbreviation
    const tzFormat = new Intl.DateTimeFormat('en-US', {
      timeZone: timezone,
      timeZoneName: 'short',
    });
    const tzParts = tzFormat.formatToParts(dateObj);
    const tzName = tzParts.find(part => part.type === 'timeZoneName')?.value || timezone;

    return `${parts.join(' ')} ${tzName}`;
  } catch (error) {
    console.error('Error formatting timestamp:', error);
    // Fallback to ISO string
    const dateObj = date instanceof Date ? date : new Date(date);
    return dateObj.toISOString();
  }
}

/**
 * Format a relative time (e.g., "2m ago", "1h ago", "3d ago")
 *
 * @param {Date|string|number} date - Date to format
 * @param {string} [timezone='UTC'] - IANA timezone name (used for "now" comparison)
 * @returns {string} Relative time string
 *
 * @example
 * formatRelativeTime(Date.now() - 120000) // 2 minutes ago
 * // => "2m ago"
 *
 * formatRelativeTime(Date.now() - 7200000) // 2 hours ago
 * // => "2h ago"
 */
export function formatRelativeTime(date, timezone = 'UTC') {
  try {
    const dateObj = date instanceof Date ? date : new Date(date);

    // Check for invalid date
    if (isNaN(dateObj.getTime())) {
      console.error('Invalid date provided to formatRelativeTime:', date);
      return 'Invalid Date';
    }

    const now = Date.now();
    const past = dateObj.getTime();
    const diffMs = now - past;

    // Handle future dates
    if (diffMs < 0) {
      return 'Just now';
    }

    const diffSec = Math.floor(diffMs / 1000);
    const diffMin = Math.floor(diffSec / 60);
    const diffHour = Math.floor(diffMin / 60);
    const diffDay = Math.floor(diffHour / 24);
    const diffWeek = Math.floor(diffDay / 7);
    const diffMonth = Math.floor(diffDay / 30);
    const diffYear = Math.floor(diffDay / 365);

    // Less than 1 minute
    if (diffSec < 60) {
      return 'Just now';
    }

    // Less than 1 hour
    if (diffMin < 60) {
      return `${diffMin}m ago`;
    }

    // Less than 1 day
    if (diffHour < 24) {
      return `${diffHour}h ago`;
    }

    // Less than 1 week
    if (diffDay < 7) {
      return `${diffDay}d ago`;
    }

    // Less than 1 month
    if (diffWeek < 4) {
      return `${diffWeek}w ago`;
    }

    // Less than 1 year
    if (diffMonth < 12) {
      return `${diffMonth}mo ago`;
    }

    // 1 year or more
    return `${diffYear}y ago`;
  } catch (error) {
    console.error('Error formatting relative time:', error);
    return 'Unknown';
  }
}

/**
 * Format a date for display with locale and timezone support
 *
 * @param {Date|string|number} date - Date to format
 * @param {string} [timezone='UTC'] - IANA timezone name
 * @param {Object} [options] - Additional formatting options
 * @returns {string} Formatted date string
 *
 * @example
 * formatDate(new Date(), 'America/New_York')
 * // => "November 10, 2025"
 */
export function formatDate(date, timezone = 'UTC', options = {}) {
  try {
    const dateObj = date instanceof Date ? date : new Date(date);

    // Check for invalid date
    if (isNaN(dateObj.getTime())) {
      console.error('Invalid date provided to formatDate:', date);
      return 'Invalid Date';
    }

    const formatter = new Intl.DateTimeFormat('en-US', {
      timeZone: timezone,
      year: 'numeric',
      month: 'long',
      day: 'numeric',
      ...options,
    });

    return formatter.format(dateObj);
  } catch (error) {
    console.error('Error formatting date:', error);
    const dateObj = date instanceof Date ? date : new Date(date);
    return dateObj.toLocaleDateString();
  }
}

/**
 * Validate if a timezone string is valid
 *
 * @param {string} timezone - IANA timezone name to validate
 * @returns {boolean} True if timezone is valid
 *
 * @example
 * isValidTimezone('America/New_York') // => true
 * isValidTimezone('Invalid/Zone') // => false
 */
export function isValidTimezone(timezone) {
  try {
    // Try to create a date formatter with the timezone
    new Intl.DateTimeFormat('en-US', { timeZone: timezone });
    return true;
  } catch (error) {
    return false;
  }
}

/**
 * Get list of common timezones for timezone picker
 *
 * @returns {Array<{value: string, label: string}>} Array of timezone options
 */
export function getCommonTimezones() {
  return [
    { value: 'UTC', label: 'UTC (Coordinated Universal Time)' },
    { value: 'America/New_York', label: 'Eastern Time (US & Canada)' },
    { value: 'America/Chicago', label: 'Central Time (US & Canada)' },
    { value: 'America/Denver', label: 'Mountain Time (US & Canada)' },
    { value: 'America/Los_Angeles', label: 'Pacific Time (US & Canada)' },
    { value: 'America/Anchorage', label: 'Alaska' },
    { value: 'Pacific/Honolulu', label: 'Hawaii' },
    { value: 'Europe/London', label: 'London' },
    { value: 'Europe/Paris', label: 'Paris' },
    { value: 'Europe/Berlin', label: 'Berlin' },
    { value: 'Asia/Tokyo', label: 'Tokyo' },
    { value: 'Asia/Shanghai', label: 'Shanghai' },
    { value: 'Asia/Hong_Kong', label: 'Hong Kong' },
    { value: 'Asia/Singapore', label: 'Singapore' },
    { value: 'Australia/Sydney', label: 'Sydney' },
    { value: 'Pacific/Auckland', label: 'Auckland' },
  ];
}
