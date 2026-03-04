package websocket

import (
	"errors"
	"strings"
)

// Topic matching errors
var (
	ErrEmptyTopic         = errors.New("topic cannot be empty")
	ErrEmptyPattern       = errors.New("pattern cannot be empty")
	ErrInvalidTopic       = errors.New("invalid topic format")
	ErrInvalidPattern     = errors.New("invalid pattern format")
	ErrHashNotAtEnd       = errors.New("multi-level wildcard (#) must be at end of pattern")
	ErrInvalidWildcard    = errors.New("invalid wildcard usage")
	ErrInvalidCharInTopic = errors.New("topic contains invalid characters")
)

// MatchTopic checks if a topic matches a pattern.
// Patterns support:
//   - Exact match: "health_change" matches "health_change"
//   - Single-level wildcard (+): "container.+" matches "container.logs" but not "container.logs.data"
//   - Multi-level wildcard (#): "container.#" matches "container", "container.logs", "container.logs.data"
//
// The multi-level wildcard (#) must be at the end of the pattern.
func MatchTopic(pattern, topic string) bool {
	if pattern == "" || topic == "" {
		return false
	}

	// Special case: # matches everything
	if pattern == "#" {
		return true
	}

	patternParts := strings.Split(pattern, ".")
	topicParts := strings.Split(topic, ".")

	return matchParts(patternParts, topicParts, 0, 0)
}

// matchParts recursively matches pattern parts against topic parts.
func matchParts(pattern, topic []string, pi, ti int) bool {
	// If we've consumed all pattern parts
	if pi >= len(pattern) {
		// Match if we've also consumed all topic parts
		return ti >= len(topic)
	}

	// Get current pattern part
	pp := pattern[pi]

	// Multi-level wildcard (#) - matches zero or more levels
	if pp == "#" {
		// # must be the last part of the pattern
		if pi != len(pattern)-1 {
			return false
		}
		// Matches remaining topic parts (zero or more)
		return true
	}

	// If we've run out of topic parts but still have pattern parts
	if ti >= len(topic) {
		return false
	}

	// Single-level wildcard (+) - matches exactly one level
	if pp == "+" {
		// Match this level and continue
		return matchParts(pattern, topic, pi+1, ti+1)
	}

	// Exact match required
	if pp != topic[ti] {
		return false
	}

	// Continue matching
	return matchParts(pattern, topic, pi+1, ti+1)
}

// MatchesAnyTopic checks if a topic matches any of the given patterns.
func MatchesAnyTopic(patterns []string, topic string) bool {
	for _, pattern := range patterns {
		if MatchTopic(pattern, topic) {
			return true
		}
	}
	return false
}

// ValidateTopic validates a topic string.
// Topics cannot contain wildcards, must not be empty, and must have valid format.
func ValidateTopic(topic string) error {
	if topic == "" {
		return ErrEmptyTopic
	}

	// Check for invalid characters
	if strings.ContainsAny(topic, "+#") {
		return ErrInvalidCharInTopic
	}

	// Check for spaces
	if strings.ContainsAny(topic, " \t\n\r") {
		return ErrInvalidCharInTopic
	}

	// Check for leading/trailing dots
	if strings.HasPrefix(topic, ".") || strings.HasSuffix(topic, ".") {
		return ErrInvalidTopic
	}

	// Check for empty segments (double dots)
	if strings.Contains(topic, "..") {
		return ErrInvalidTopic
	}

	// Check that it's not just a dot
	if topic == "." {
		return ErrInvalidTopic
	}

	return nil
}

// ValidatePattern validates a subscription pattern.
// Patterns can contain wildcards but must follow rules:
// - + can appear anywhere as a complete segment
// - # must be at the end as the last segment
func ValidatePattern(pattern string) error {
	if pattern == "" {
		return ErrEmptyPattern
	}

	// Check for leading/trailing dots
	if strings.HasPrefix(pattern, ".") || strings.HasSuffix(pattern, ".") {
		return ErrInvalidPattern
	}

	// Check for empty segments (double dots)
	if strings.Contains(pattern, "..") {
		return ErrInvalidPattern
	}

	parts := strings.Split(pattern, ".")

	for i, part := range parts {
		if part == "" {
			return ErrInvalidPattern
		}

		// # must be at the end
		if part == "#" && i != len(parts)-1 {
			return ErrHashNotAtEnd
		}

		// Check for invalid wildcard usage (e.g., "container#" or "a+b")
		if part != "+" && part != "#" {
			if strings.ContainsAny(part, "+#") {
				return ErrInvalidWildcard
			}
		}
	}

	return nil
}

// ExtractTopicFromType converts a WebSocket message type to a topic.
// This allows treating message types as topics for subscription.
func ExtractTopicFromType(msgType string) string {
	// Message types like "container.logs.data" are already in topic format
	// Types like "health_change" can be used as-is
	return msgType
}

// ParseTopics parses a topic list, validating each one.
func ParseTopics(topics []string) ([]string, error) {
	if len(topics) == 0 {
		return nil, errors.New("at least one topic is required")
	}

	validated := make([]string, 0, len(topics))
	for _, topic := range topics {
		topic = strings.TrimSpace(topic)
		if topic == "" {
			continue
		}
		validated = append(validated, topic)
	}

	if len(validated) == 0 {
		return nil, errors.New("at least one valid topic is required")
	}

	return validated, nil
}

// ParsePatterns parses and validates subscription patterns.
func ParsePatterns(patterns []string) ([]string, error) {
	if len(patterns) == 0 {
		return nil, errors.New("at least one pattern is required")
	}

	validated := make([]string, 0, len(patterns))
	for _, pattern := range patterns {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}
		if err := ValidatePattern(pattern); err != nil {
			return nil, err
		}
		validated = append(validated, pattern)
	}

	if len(validated) == 0 {
		return nil, errors.New("at least one valid pattern is required")
	}

	return validated, nil
}
