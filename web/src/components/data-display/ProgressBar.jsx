import PropTypes from 'prop-types';

/**
 * ProgressBar component - ASCII-style progress indicator
 * Uses block characters: █ (filled) and ░ (empty)
 *
 * @component
 * @example
 * <ProgressBar current={38} max={100} label="Sync Progress" />
 */
function ProgressBar({
  current,
  max,
  label,
  blocks = 20,
  showPercentage = true,
  className = '',
  ...props
}) {
  // Calculate percentage
  const percentage = Math.min(100, Math.max(0, (current / max) * 100));

  // Calculate filled blocks
  const filledCount = Math.round((percentage / 100) * blocks);
  const emptyCount = blocks - filledCount;

  // Generate block characters
  const filledBlocks = '█'.repeat(filledCount);
  const emptyBlocks = '░'.repeat(emptyCount);
  const blockString = filledBlocks + emptyBlocks;

  return (
    <div className={`ascii-progress-bar ${className}`} {...props}>
      {label && (
        <div className="ascii-progress-text" style={{ marginBottom: 'var(--spacing-xs)' }}>
          {label}
        </div>
      )}
      <div className="ascii-progress-fill" style={{ '--progress': `${percentage}%` }}>
        <span className="ascii-progress-blocks" aria-hidden="true">
          {blockString}
        </span>
      </div>
      {showPercentage && (
        <div className="ascii-progress-text">
          {current} / {max} ({percentage.toFixed(1)}%)
        </div>
      )}
      {/* Screen reader only */}
      <span className="sr-only">
        {label ? `${label}: ` : ''}Progress: {current} of {max} ({percentage.toFixed(1)}%)
      </span>
    </div>
  );
}

ProgressBar.propTypes = {
  /** Current progress value */
  current: PropTypes.number.isRequired,
  /** Maximum value */
  max: PropTypes.number.isRequired,
  /** Progress label */
  label: PropTypes.string,
  /** Number of block characters to display */
  blocks: PropTypes.number,
  /** Show percentage text */
  showPercentage: PropTypes.bool,
  /** Additional CSS classes */
  className: PropTypes.string
};

export default ProgressBar;
