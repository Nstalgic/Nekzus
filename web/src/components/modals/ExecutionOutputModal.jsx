/**
 * ExecutionOutputModal Component
 *
 * Modal for viewing script execution output with real-time updates
 *
 * Features:
 * - Display execution metadata (status, duration, exit code, etc.)
 * - Terminal-like output display with monospace font
 * - Auto-refresh for running executions
 * - Copy output to clipboard
 * - Colored status badges
 * - Scrollable output area
 */

import { useState, useEffect, useCallback } from 'react';
import PropTypes from 'prop-types';
import { Terminal, Copy, Check, RefreshCw, Clock, User, Calendar } from 'lucide-react';
import { DetailsModal } from './DetailsModal';
import Badge from '../data-display/Badge';
import styles from './ExecutionOutputModal.module.css';

/**
 * Get status badge variant based on execution status
 * @param {string} status - Execution status
 * @returns {string} Badge variant
 */
function getStatusVariant(status) {
  switch (status?.toLowerCase()) {
    case 'completed':
      return 'success';
    case 'failed':
      return 'error';
    case 'running':
      return 'info';
    case 'timeout':
      return 'warning';
    case 'cancelled':
      return 'secondary';
    default:
      return 'secondary';
  }
}

/**
 * Format timestamp to readable date/time
 * @param {string|number} timestamp - Unix timestamp or ISO string
 * @returns {string} Formatted date/time
 */
function formatTimestamp(timestamp) {
  if (!timestamp) return 'N/A';
  const date = new Date(typeof timestamp === 'number' ? timestamp * 1000 : timestamp);
  return date.toLocaleString('en-US', {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  });
}

/**
 * Calculate duration between two timestamps
 * @param {string|number} startedAt - Start timestamp
 * @param {string|number} completedAt - End timestamp
 * @returns {string} Formatted duration
 */
function calculateDuration(startedAt, completedAt) {
  if (!startedAt) return 'N/A';

  const start = new Date(typeof startedAt === 'number' ? startedAt * 1000 : startedAt);
  const end = completedAt
    ? new Date(typeof completedAt === 'number' ? completedAt * 1000 : completedAt)
    : new Date();

  const durationMs = end - start;
  const seconds = Math.floor(durationMs / 1000);
  const minutes = Math.floor(seconds / 60);
  const hours = Math.floor(minutes / 60);

  if (hours > 0) {
    return `${hours}h ${minutes % 60}m ${seconds % 60}s`;
  } else if (minutes > 0) {
    return `${minutes}m ${seconds % 60}s`;
  } else {
    return `${seconds}s`;
  }
}

/**
 * ExecutionOutputModal Component
 *
 * @param {object} props - Component props
 * @param {boolean} props.isOpen - Modal open state
 * @param {function} props.onClose - Close callback
 * @param {object} props.execution - Execution object
 * @param {function} props.onRefresh - Refresh callback for running executions
 */
export function ExecutionOutputModal({ isOpen, onClose, execution, onRefresh }) {
  const [copied, setCopied] = useState(false);
  const [refreshing, setRefreshing] = useState(false);

  // Auto-refresh for running executions
  useEffect(() => {
    if (!isOpen || !execution || execution.status?.toLowerCase() !== 'running') {
      return;
    }

    // Refresh every 2 seconds while running
    const interval = setInterval(() => {
      if (onRefresh) {
        onRefresh();
      }
    }, 2000);

    return () => clearInterval(interval);
  }, [isOpen, execution, onRefresh]);

  // Copy output to clipboard
  const handleCopyOutput = useCallback(async () => {
    if (!execution?.output) return;

    try {
      await navigator.clipboard.writeText(execution.output);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch (err) {
      console.error('Failed to copy output:', err);
    }
  }, [execution?.output]);

  // Manual refresh
  const handleRefresh = useCallback(async () => {
    if (!onRefresh || refreshing) return;

    setRefreshing(true);
    try {
      await onRefresh();
    } finally {
      setTimeout(() => setRefreshing(false), 500);
    }
  }, [onRefresh, refreshing]);

  if (!execution) return null;

  const isRunning = execution.status?.toLowerCase() === 'running';
  const statusVariant = getStatusVariant(execution.status);
  const duration = calculateDuration(execution.startedAt, execution.completedAt);

  return (
    <DetailsModal
      isOpen={isOpen}
      onClose={onClose}
      icon={<Terminal size={24} />}
      title="Execution Output"
      subtitle={execution.scriptName || 'Script Execution'}
      badge={
        <Badge variant={statusVariant} dot filled size="sm">
          {execution.status?.toUpperCase() || 'UNKNOWN'}
        </Badge>
      }
      size="large"
      className={styles.executionOutputModal}
      footer={
        <div className={styles.footer}>
          <button
            type="button"
            className="btn btn-secondary"
            onClick={handleCopyOutput}
            disabled={!execution.output || execution.output.length === 0}
            title="Copy output to clipboard"
            aria-label="Copy output to clipboard"
          >
            {copied ? (
              <>
                <Check size={16} />
                COPIED
              </>
            ) : (
              <>
                <Copy size={16} />
                COPY OUTPUT
              </>
            )}
          </button>
          {isRunning && onRefresh && (
            <button
              type="button"
              className="btn btn-primary"
              onClick={handleRefresh}
              disabled={refreshing}
              title="Refresh output"
              aria-label="Refresh output"
            >
              <RefreshCw size={16} className={refreshing ? styles.spinning : ''} />
              REFRESH
            </button>
          )}
          <button
            type="button"
            className="btn btn-secondary"
            onClick={onClose}
          >
            CLOSE
          </button>
        </div>
      }
    >
      {/* Execution Metadata */}
      <div className={styles.metadata}>
        <div className={styles.metadataGrid}>
          <div className={styles.metadataItem}>
            <div className={styles.metadataLabel}>
              <User size={14} />
              Triggered By
            </div>
            <div className={styles.metadataValue}>
              {execution.triggeredBy || 'Unknown'}
            </div>
          </div>

          <div className={styles.metadataItem}>
            <div className={styles.metadataLabel}>
              <Calendar size={14} />
              Started At
            </div>
            <div className={styles.metadataValue}>
              {formatTimestamp(execution.startedAt)}
            </div>
          </div>

          {execution.completedAt && (
            <div className={styles.metadataItem}>
              <div className={styles.metadataLabel}>
                <Calendar size={14} />
                Completed At
              </div>
              <div className={styles.metadataValue}>
                {formatTimestamp(execution.completedAt)}
              </div>
            </div>
          )}

          <div className={styles.metadataItem}>
            <div className={styles.metadataLabel}>
              <Clock size={14} />
              Duration
            </div>
            <div className={styles.metadataValue}>
              {duration}
            </div>
          </div>

          {execution.exitCode !== null && execution.exitCode !== undefined && (
            <div className={styles.metadataItem}>
              <div className={styles.metadataLabel}>
                <Terminal size={14} />
                Exit Code
              </div>
              <div className={styles.metadataValue}>
                <code className={execution.exitCode === 0 ? styles.exitCodeSuccess : styles.exitCodeError}>
                  {execution.exitCode}
                </code>
              </div>
            </div>
          )}
        </div>
      </div>

      {/* Output Display */}
      <div className={styles.outputSection}>
        <div className={styles.outputHeader}>
          <span className={styles.outputTitle}>
            <Terminal size={14} />
            OUTPUT
          </span>
          {isRunning && (
            <span className={styles.runningIndicator}>
              <span className={styles.pulsingDot} />
              Execution in progress...
            </span>
          )}
        </div>

        <div className={styles.outputContainer}>
          {execution.output && execution.output.length > 0 ? (
            <pre className="output-terminal">
              <code>{execution.output}</code>
            </pre>
          ) : isRunning ? (
            <div className={styles.emptyState}>
              <Terminal size={32} />
              <p>Waiting for output...</p>
            </div>
          ) : (
            <div className={styles.emptyState}>
              <Terminal size={32} />
              <p>No output generated</p>
            </div>
          )}
        </div>
      </div>

      {/* Error message if execution failed */}
      {execution.status?.toLowerCase() === 'failed' && execution.error && (
        <div className={styles.errorSection}>
          <div className={styles.errorHeader}>
            ERROR DETAILS
          </div>
          <div className={styles.errorContent}>
            {execution.error}
          </div>
        </div>
      )}
    </DetailsModal>
  );
}

ExecutionOutputModal.propTypes = {
  isOpen: PropTypes.bool.isRequired,
  onClose: PropTypes.func.isRequired,
  execution: PropTypes.shape({
    id: PropTypes.string,
    scriptName: PropTypes.string,
    status: PropTypes.string,
    output: PropTypes.string,
    error: PropTypes.string,
    exitCode: PropTypes.number,
    triggeredBy: PropTypes.string,
    startedAt: PropTypes.oneOfType([PropTypes.string, PropTypes.number]),
    completedAt: PropTypes.oneOfType([PropTypes.string, PropTypes.number]),
  }),
  onRefresh: PropTypes.func,
};
