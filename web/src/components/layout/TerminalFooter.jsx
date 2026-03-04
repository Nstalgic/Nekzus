import { useState, useEffect } from 'react';
import PropTypes from 'prop-types';
import { useSettings } from '../../contexts/SettingsContext';

/**
 * TerminalFooter Component
 *
 * Fixed footer displaying version information, keyboard shortcuts, and a live timestamp.
 * The timestamp updates every second and respects user timezone and display preferences.
 *
 * @component
 * @param {Object} props - Component props
 * @param {string} [props.version='1.0.0'] - Application version to display
 * @returns {JSX.Element} Terminal footer with version, shortcuts, and timestamp
 *
 * @example
 * <TerminalFooter version="1.0.0" />
 */
const TerminalFooter = ({ version = '1.0.0' }) => {
  const { settings } = useSettings();
  const [timestamp, setTimestamp] = useState('');

  useEffect(() => {
    /**
     * Updates the timestamp based on user timezone preference
     * Supports 'UTC', 'Local', and IANA timezone strings
     */
    const updateTimestamp = () => {
      const now = new Date();
      let timeString;

      if (settings.timezone === 'UTC') {
        // UTC time format
        const year = now.getUTCFullYear();
        const month = String(now.getUTCMonth() + 1).padStart(2, '0');
        const day = String(now.getUTCDate()).padStart(2, '0');
        const hours = String(now.getUTCHours()).padStart(2, '0');
        const minutes = String(now.getUTCMinutes()).padStart(2, '0');
        const seconds = String(now.getUTCSeconds()).padStart(2, '0');
        timeString = `${year}-${month}-${day} ${hours}:${minutes}:${seconds} UTC`;
      } else if (settings.timezone === 'Local') {
        // Local time format
        const year = now.getFullYear();
        const month = String(now.getMonth() + 1).padStart(2, '0');
        const day = String(now.getDate()).padStart(2, '0');
        const hours = String(now.getHours()).padStart(2, '0');
        const minutes = String(now.getMinutes()).padStart(2, '0');
        const seconds = String(now.getSeconds()).padStart(2, '0');
        timeString = `${year}-${month}-${day} ${hours}:${minutes}:${seconds} Local`;
      } else {
        // IANA timezone (e.g., 'America/New_York', 'Europe/London')
        try {
          timeString = now.toLocaleString('en-US', {
            timeZone: settings.timezone,
            year: 'numeric',
            month: '2-digit',
            day: '2-digit',
            hour: '2-digit',
            minute: '2-digit',
            second: '2-digit',
            hour12: false,
          }).replace(/(\d+)\/(\d+)\/(\d+),\s(\d+):(\d+):(\d+)/, '$3-$1-$2 $4:$5:$6') + ` ${settings.timezone}`;
        } catch (error) {
          // Fallback to UTC if timezone is invalid
          console.error(`Invalid timezone: ${settings.timezone}`, error);
          const year = now.getUTCFullYear();
          const month = String(now.getUTCMonth() + 1).padStart(2, '0');
          const day = String(now.getUTCDate()).padStart(2, '0');
          const hours = String(now.getUTCHours()).padStart(2, '0');
          const minutes = String(now.getUTCMinutes()).padStart(2, '0');
          const seconds = String(now.getUTCSeconds()).padStart(2, '0');
          timeString = `${year}-${month}-${day} ${hours}:${minutes}:${seconds} UTC`;
        }
      }

      setTimestamp(timeString);
    };

    // Initial update
    updateTimestamp();

    // Update every second
    const intervalId = setInterval(updateTimestamp, 1000);

    // Cleanup interval on unmount
    return () => clearInterval(intervalId);
  }, [settings.timezone]);

  return (
    <footer className="terminal-footer">
      <span>DASHBOARD v{version}</span>
      {settings.showTimestamp && (
        <>
          <span>|</span>
          <span className="timestamp" id="timestamp">{timestamp}</span>
        </>
      )}
    </footer>
  );
};

TerminalFooter.propTypes = {
  version: PropTypes.string,
};

export default TerminalFooter;
