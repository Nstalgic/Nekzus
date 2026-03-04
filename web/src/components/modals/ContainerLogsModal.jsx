/**
 * ContainerLogsModal Component
 *
 * Modal for viewing Docker container logs with WebSocket streaming support
 *
 * Features:
 * - Real-time log streaming via WebSocket
 * - Auto-scroll with pause/resume
 * - Search/filter text
 * - Copy logs to clipboard
 * - Timestamp toggle
 * - Tail line count adjustment
 * - stdout/stderr differentiation
 */

import { useState, useEffect, useRef, useCallback } from 'react';
import PropTypes from 'prop-types';
import { X, Copy, Search, Play, Pause, Check, Wifi, WifiOff } from 'lucide-react';
import { Card } from '../boxes';
import { websocketService, WS_MSG_TYPES } from '../../services/websocket';

/**
 * ContainerLogsModal Component
 *
 * @param {object} props - Component props
 * @param {boolean} props.isOpen - Modal open state
 * @param {function} props.onClose - Close callback
 * @param {object} props.container - Container object
 */
export function ContainerLogsModal({ isOpen, onClose, container }) {
  const [logs, setLogs] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [autoScroll, setAutoScroll] = useState(true);
  const [searchTerm, setSearchTerm] = useState('');
  const [showTimestamps, setShowTimestamps] = useState(false);
  const [tailLines, setTailLines] = useState(100);
  const [copied, setCopied] = useState(false);
  const [streaming, setStreaming] = useState(false);

  const logsEndRef = useRef(null);
  const logsContainerRef = useRef(null);
  const isSubscribedRef = useRef(false);

  // Subscribe to log stream
  const subscribeToLogs = useCallback(() => {
    if (!container?.id || isSubscribedRef.current) return;

    setLoading(true);
    setError(null);
    setLogs([]);

    websocketService.send({
      type: WS_MSG_TYPES.CONTAINER_LOGS_SUBSCRIBE,
      data: {
        containerId: container.id,
        tail: tailLines,
        timestamps: showTimestamps,
      },
    });

    isSubscribedRef.current = true;
  }, [container?.id, tailLines, showTimestamps]);

  // Unsubscribe from log stream
  const unsubscribeFromLogs = useCallback(() => {
    if (!container?.id || !isSubscribedRef.current) return;

    websocketService.send({
      type: WS_MSG_TYPES.CONTAINER_LOGS_UNSUBSCRIBE,
      data: {
        containerId: container.id,
      },
    });

    isSubscribedRef.current = false;
    setStreaming(false);
  }, [container?.id]);

  // Handle log data messages
  const handleLogData = useCallback((data) => {
    if (data.containerId !== container?.id) return;

    setLogs((prev) => {
      const newLog = {
        message: data.message,
        stream: data.stream || 'stdout',
        timestamp: data.timestamp,
      };

      // Limit logs to prevent memory issues (keep last 2000 lines)
      const updated = [...prev, newLog];
      if (updated.length > 2000) {
        return updated.slice(-2000);
      }
      return updated;
    });
  }, [container?.id]);

  // Handle stream started
  const handleLogsStarted = useCallback((data) => {
    if (data.containerId !== container?.id) return;
    setLoading(false);
    setStreaming(true);
  }, [container?.id]);

  // Handle stream ended
  const handleLogsEnded = useCallback((data) => {
    if (data.containerId !== container?.id) return;
    setStreaming(false);
    isSubscribedRef.current = false;

    if (data.reason === 'error') {
      setError(data.message || 'Stream ended with error');
    }
  }, [container?.id]);

  // Handle stream error
  const handleLogsError = useCallback((data) => {
    if (data.containerId !== container?.id) return;
    setLoading(false);
    setStreaming(false);
    setError(data.message || 'Failed to stream logs');
    isSubscribedRef.current = false;
  }, [container?.id]);

  // Setup WebSocket listeners
  useEffect(() => {
    if (!isOpen || !container) return;

    // Register event listeners
    websocketService.on(WS_MSG_TYPES.CONTAINER_LOGS, handleLogData);
    websocketService.on(WS_MSG_TYPES.CONTAINER_LOGS_STARTED, handleLogsStarted);
    websocketService.on(WS_MSG_TYPES.CONTAINER_LOGS_ENDED, handleLogsEnded);
    websocketService.on(WS_MSG_TYPES.CONTAINER_LOGS_ERROR, handleLogsError);

    // Subscribe to logs
    subscribeToLogs();

    // Cleanup
    return () => {
      websocketService.off(WS_MSG_TYPES.CONTAINER_LOGS, handleLogData);
      websocketService.off(WS_MSG_TYPES.CONTAINER_LOGS_STARTED, handleLogsStarted);
      websocketService.off(WS_MSG_TYPES.CONTAINER_LOGS_ENDED, handleLogsEnded);
      websocketService.off(WS_MSG_TYPES.CONTAINER_LOGS_ERROR, handleLogsError);
      unsubscribeFromLogs();
    };
  }, [isOpen, container, subscribeToLogs, unsubscribeFromLogs, handleLogData, handleLogsStarted, handleLogsEnded, handleLogsError]);

  // Resubscribe when settings change
  useEffect(() => {
    if (!isOpen || !container) return;

    // Unsubscribe and resubscribe with new settings
    if (isSubscribedRef.current) {
      unsubscribeFromLogs();
      // Small delay to ensure unsubscribe is processed
      const timer = setTimeout(() => {
        subscribeToLogs();
      }, 100);
      return () => clearTimeout(timer);
    }
  }, [tailLines, showTimestamps]);

  // Auto-scroll to bottom when logs update
  useEffect(() => {
    if (autoScroll && logsEndRef.current) {
      logsEndRef.current.scrollIntoView({ behavior: 'smooth' });
    }
  }, [logs, autoScroll]);

  // Handle manual scroll detection
  const handleScroll = () => {
    if (!logsContainerRef.current) return;

    const { scrollTop, scrollHeight, clientHeight } = logsContainerRef.current;
    const isAtBottom = scrollHeight - scrollTop - clientHeight < 50;

    // Auto-enable scroll if user scrolls to bottom manually
    if (isAtBottom && !autoScroll) {
      setAutoScroll(true);
    }
    // Disable auto-scroll if user scrolls up
    else if (!isAtBottom && autoScroll) {
      setAutoScroll(false);
    }
  };

  // Format logs as text for copying/filtering
  const getLogsAsText = useCallback(() => {
    return logs.map((log) => {
      if (showTimestamps && log.timestamp) {
        const date = new Date(log.timestamp * 1000);
        return `[${date.toISOString()}] ${log.message}`;
      }
      return log.message;
    }).join('\n');
  }, [logs, showTimestamps]);

  // Copy logs to clipboard
  const handleCopyLogs = async () => {
    try {
      await navigator.clipboard.writeText(getLogsAsText());
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch (err) {
      console.error('Failed to copy logs:', err);
    }
  };

  // Filter logs by search term
  const filteredLogs = searchTerm
    ? logs.filter((log) => log.message.toLowerCase().includes(searchTerm.toLowerCase()))
    : logs;

  if (!isOpen || !container) return null;

  const displayName = container.name?.replace(/^\//, '') || 'Unnamed Container';

  return (
    <div className="modal-overlay" onClick={onClose}>
      <div className="modal-content modal-content-large" onClick={(e) => e.stopPropagation()}>
        <Card className="container-logs-modal-card">
          {/* Header */}
          <div className="container-logs-modal-header">
            <div className="header-title-section">
              <h2 className="container-logs-modal-title">Container Logs</h2>
              <p className="container-logs-subtitle">
                {displayName}
                {streaming && (
                  <span className="streaming-indicator" title="Streaming logs">
                    <Wifi size={14} />
                  </span>
                )}
                {!streaming && !loading && (
                  <span className="streaming-indicator offline" title="Stream ended">
                    <WifiOff size={14} />
                  </span>
                )}
              </p>
            </div>
            <button
              className="modal-close-btn"
              onClick={onClose}
              aria-label="Close logs viewer"
            >
              <X size={20} />
            </button>
          </div>

          {/* Controls */}
          <div className="logs-controls">
            <div className="logs-controls-left">
              {/* Search Input */}
              <div className="search-input-wrapper">
                <Search size={16} className="search-icon" />
                <input
                  type="text"
                  className="logs-search-input"
                  placeholder="Filter logs..."
                  value={searchTerm}
                  onChange={(e) => setSearchTerm(e.target.value)}
                  aria-label="Filter logs"
                />
              </div>

              {/* Tail Lines Select */}
              <div className="tail-select-wrapper">
                <label htmlFor="tail-lines" className="tail-label">
                  Lines:
                </label>
                <select
                  id="tail-lines"
                  className="tail-select"
                  value={tailLines}
                  onChange={(e) => setTailLines(Number(e.target.value))}
                >
                  <option value={50}>50</option>
                  <option value={100}>100</option>
                  <option value={200}>200</option>
                  <option value={500}>500</option>
                  <option value={1000}>1000</option>
                </select>
              </div>

              {/* Timestamps Toggle */}
              <label className="checkbox-label">
                <input
                  type="checkbox"
                  className="checkbox"
                  checked={showTimestamps}
                  onChange={(e) => setShowTimestamps(e.target.checked)}
                />
                <span>Timestamps</span>
              </label>
            </div>

            <div className="logs-controls-right">
              {/* Auto-scroll Toggle */}
              <button
                type="button"
                className={`btn btn-sm ${autoScroll ? 'btn-primary' : 'btn-secondary'}`}
                onClick={() => setAutoScroll(!autoScroll)}
                title={autoScroll ? 'Pause auto-scroll' : 'Resume auto-scroll'}
                aria-label={autoScroll ? 'Pause auto-scroll' : 'Resume auto-scroll'}
              >
                {autoScroll ? <Pause size={16} /> : <Play size={16} />}
              </button>

              {/* Copy Button */}
              <button
                type="button"
                className="btn btn-sm btn-secondary"
                onClick={handleCopyLogs}
                disabled={logs.length === 0 || loading}
                title="Copy logs to clipboard"
                aria-label="Copy logs to clipboard"
              >
                {copied ? <Check size={16} /> : <Copy size={16} />}
              </button>
            </div>
          </div>

          {/* Body - Logs Display */}
          <div
            className="container-logs-modal-body"
            ref={logsContainerRef}
            onScroll={handleScroll}
          >
            {loading && (
              <div className="logs-loading-state">
                <p>Connecting to log stream...</p>
              </div>
            )}

            {error && (
              <div className="logs-error-state">
                <p>Error: {error}</p>
              </div>
            )}

            {!loading && !error && (
              <pre className="logs-content">
                <code>
                  {filteredLogs.length > 0 ? (
                    filteredLogs.map((log, index) => (
                      <div
                        key={index}
                        className={`log-line ${log.stream === 'stderr' ? 'log-stderr' : 'log-stdout'}`}
                      >
                        {showTimestamps && log.timestamp && (
                          <span className="log-timestamp">
                            [{new Date(log.timestamp * 1000).toISOString()}]{' '}
                          </span>
                        )}
                        {log.message}
                      </div>
                    ))
                  ) : (
                    'No logs available'
                  )}
                </code>
                <div ref={logsEndRef} />
              </pre>
            )}
          </div>

          {/* Footer */}
          <div className="container-logs-modal-footer">
            <div className="logs-info">
              {searchTerm && (
                <span className="logs-filter-info">
                  Filtered: showing {filteredLogs.length} of {logs.length} lines
                </span>
              )}
              {!searchTerm && logs.length > 0 && (
                <span className="logs-count-info">
                  {logs.length} lines
                </span>
              )}
            </div>
            <button className="btn btn-secondary" onClick={onClose}>
              CLOSE
            </button>
          </div>
        </Card>
      </div>
    </div>
  );
}

ContainerLogsModal.propTypes = {
  isOpen: PropTypes.bool.isRequired,
  onClose: PropTypes.func.isRequired,
  container: PropTypes.shape({
    id: PropTypes.string.isRequired,
    name: PropTypes.string,
    state: PropTypes.string,
  }),
};
