/**
 * Confidence score utilities
 *
 * Converts numeric confidence scores (0-1 or 0-100) to human-readable categories.
 */

/**
 * Confidence categories with their thresholds and display properties
 */
export const CONFIDENCE_CATEGORIES = {
  VERIFIED: {
    label: 'Verified',
    color: 'var(--color-success)',
    variant: 'success',
    minScore: 0.90,
  },
  DETECTED: {
    label: 'Detected',
    color: 'var(--color-warning)',
    variant: 'warning',
    minScore: 0.70,
  },
  UNCERTAIN: {
    label: 'Uncertain',
    color: 'var(--color-error)',
    variant: 'error',
    minScore: 0,
  },
};

/**
 * Normalizes confidence score to 0-1 range
 * @param {number} score - Confidence score (0-1 or 0-100)
 * @returns {number} Normalized score (0-1)
 */
function normalizeScore(score) {
  if (score > 1) {
    return score / 100;
  }
  return score;
}

/**
 * Gets the confidence category for a given score
 * @param {number} score - Confidence score (0-1 or 0-100)
 * @returns {object} Category object with label, color, and variant
 */
export function getConfidenceCategory(score) {
  const normalized = normalizeScore(score);

  if (normalized >= CONFIDENCE_CATEGORIES.VERIFIED.minScore) {
    return CONFIDENCE_CATEGORIES.VERIFIED;
  }
  if (normalized >= CONFIDENCE_CATEGORIES.DETECTED.minScore) {
    return CONFIDENCE_CATEGORIES.DETECTED;
  }
  return CONFIDENCE_CATEGORIES.UNCERTAIN;
}

/**
 * Gets the confidence label for a given score
 * @param {number} score - Confidence score (0-1 or 0-100)
 * @returns {string} Human-readable label
 */
export function getConfidenceLabel(score) {
  return getConfidenceCategory(score).label;
}

/**
 * Gets the confidence color for a given score
 * @param {number} score - Confidence score (0-1 or 0-100)
 * @returns {string} CSS color variable
 */
export function getConfidenceColor(score) {
  return getConfidenceCategory(score).color;
}

/**
 * Gets the badge variant for a given score
 * @param {number} score - Confidence score (0-1 or 0-100)
 * @returns {string} Badge variant name
 */
export function getConfidenceVariant(score) {
  return getConfidenceCategory(score).variant;
}
