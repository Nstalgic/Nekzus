package websocket

import "testing"

func TestTopicMatcher_ExactMatch(t *testing.T) {
	tests := []struct {
		name     string
		pattern  string
		topic    string
		expected bool
	}{
		{"exact match", "health_change", "health_change", true},
		{"no match different topic", "health_change", "discovery", false},
		{"no match prefix", "health", "health_change", false},
		{"no match suffix", "health_change", "health", false},
		{"empty pattern", "", "health_change", false},
		{"empty topic", "health_change", "", false},
		{"both empty", "", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MatchTopic(tt.pattern, tt.topic)
			if result != tt.expected {
				t.Errorf("MatchTopic(%q, %q) = %v, want %v", tt.pattern, tt.topic, result, tt.expected)
			}
		})
	}
}

func TestTopicMatcher_SingleLevelWildcard(t *testing.T) {
	// + matches exactly one level
	tests := []struct {
		name     string
		pattern  string
		topic    string
		expected bool
	}{
		{"single level at end", "container.+", "container.logs", true},
		{"single level at end no match multi", "container.+", "container.logs.data", false},
		{"single level in middle", "container.+.data", "container.logs.data", true},
		{"single level in middle no match", "container.+.data", "container.logs.extra.data", false},
		{"single level at start", "+.logs", "container.logs", true},
		{"single level at start no match", "+.logs", "container.extra.logs", false},
		{"multiple single wildcards", "+.+.data", "container.logs.data", true},
		{"multiple single wildcards no match", "+.+.data", "container.data", false},
		{"just plus", "+", "health_change", true},
		{"just plus no match multi", "+", "container.logs", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MatchTopic(tt.pattern, tt.topic)
			if result != tt.expected {
				t.Errorf("MatchTopic(%q, %q) = %v, want %v", tt.pattern, tt.topic, result, tt.expected)
			}
		})
	}
}

func TestTopicMatcher_MultiLevelWildcard(t *testing.T) {
	// # matches zero or more levels, must be at end
	tests := []struct {
		name     string
		pattern  string
		topic    string
		expected bool
	}{
		{"multi level at end zero", "container.#", "container", true},
		{"multi level at end one", "container.#", "container.logs", true},
		{"multi level at end two", "container.#", "container.logs.data", true},
		{"multi level at end many", "container.#", "container.logs.data.extra.more", true},
		{"just hash matches all", "#", "anything", true},
		{"just hash matches multi", "#", "container.logs.data", true},
		{"just hash matches empty", "#", "", false}, // empty topic is invalid
		{"hash not at end invalid", "container.#.data", "container.logs.data", false},
		{"hash with prefix", "a.b.#", "a.b", true},
		{"hash with prefix match one", "a.b.#", "a.b.c", true},
		{"hash with prefix no match", "a.b.#", "a.c.d", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MatchTopic(tt.pattern, tt.topic)
			if result != tt.expected {
				t.Errorf("MatchTopic(%q, %q) = %v, want %v", tt.pattern, tt.topic, result, tt.expected)
			}
		})
	}
}

func TestTopicMatcher_MixedWildcards(t *testing.T) {
	tests := []struct {
		name     string
		pattern  string
		topic    string
		expected bool
	}{
		{"plus then hash", "+.#", "container", true},
		{"plus then hash multi", "+.#", "container.logs.data", true},
		{"prefix plus hash", "a.+.#", "a.b", true},
		{"prefix plus hash multi", "a.+.#", "a.b.c.d", true},
		{"prefix plus hash no match", "a.+.#", "a", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MatchTopic(tt.pattern, tt.topic)
			if result != tt.expected {
				t.Errorf("MatchTopic(%q, %q) = %v, want %v", tt.pattern, tt.topic, result, tt.expected)
			}
		})
	}
}

func TestTopicMatcher_RealWorldPatterns(t *testing.T) {
	tests := []struct {
		name     string
		pattern  string
		topic    string
		expected bool
	}{
		// Container log patterns
		{"container logs all", "container.#", "container.logs.started", true},
		{"container logs all", "container.#", "container.logs.data", true},
		{"container logs all", "container.#", "container.logs.ended", true},
		{"container logs specific", "container.logs.+", "container.logs.started", true},
		{"container logs specific no match", "container.logs.+", "container.stats", false},

		// Health monitoring
		{"all health", "health_change", "health_change", true},
		{"health no match", "health_change", "discovery", false},

		// Federation events (underscore is part of name, not separator)
		{"peer joined exact", "peer_joined", "peer_joined", true},
		{"peer left exact", "peer_left", "peer_left", true},
		{"federation all", "federation.#", "federation.sync", true},

		// Catch all
		{"subscribe to everything", "#", "health_change", true},
		{"subscribe to everything", "#", "container.logs.data", true},
		{"subscribe to everything", "#", "peer_joined", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MatchTopic(tt.pattern, tt.topic)
			if result != tt.expected {
				t.Errorf("MatchTopic(%q, %q) = %v, want %v", tt.pattern, tt.topic, result, tt.expected)
			}
		})
	}
}

func TestMatchesAnyTopic(t *testing.T) {
	tests := []struct {
		name     string
		patterns []string
		topic    string
		expected bool
	}{
		{"matches first", []string{"health_change", "discovery"}, "health_change", true},
		{"matches second", []string{"health_change", "discovery"}, "discovery", true},
		{"matches wildcard", []string{"container.#", "health_change"}, "container.logs.data", true},
		{"no match", []string{"health_change", "discovery"}, "config_reload", false},
		{"empty patterns", []string{}, "health_change", false},
		{"nil patterns", nil, "health_change", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MatchesAnyTopic(tt.patterns, tt.topic)
			if result != tt.expected {
				t.Errorf("MatchesAnyTopic(%v, %q) = %v, want %v", tt.patterns, tt.topic, result, tt.expected)
			}
		})
	}
}

func TestValidateTopic(t *testing.T) {
	tests := []struct {
		name    string
		topic   string
		isValid bool
	}{
		{"simple topic", "health_change", true},
		{"dotted topic", "container.logs", true},
		{"multi dotted", "a.b.c.d.e", true},
		{"with underscore", "health_change", true},
		{"with dash", "my-topic", true},
		{"empty", "", false},
		{"just dot", ".", false},
		{"leading dot", ".topic", false},
		{"trailing dot", "topic.", false},
		{"double dot", "topic..other", false},
		{"with space", "topic name", false},
		{"with hash", "topic#name", false},
		{"with plus", "topic+name", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTopic(tt.topic)
			if tt.isValid && err != nil {
				t.Errorf("ValidateTopic(%q) returned error %v, expected valid", tt.topic, err)
			}
			if !tt.isValid && err == nil {
				t.Errorf("ValidateTopic(%q) returned nil, expected error", tt.topic)
			}
		})
	}
}

func TestValidatePattern(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		isValid bool
	}{
		{"simple pattern", "health_change", true},
		{"single wildcard", "container.+", true},
		{"multi wildcard", "container.#", true},
		{"mixed wildcards", "+.#", true},
		{"prefix plus hash", "a.+.#", true},
		{"multiple plus", "+.+.+", true},
		{"empty", "", false},
		{"hash not at end", "container.#.data", false},
		{"hash in middle", "a.#.b", false},
		{"double hash", "a.#.#", false},
		{"leading dot", ".+", false},
		{"trailing dot", "+.", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePattern(tt.pattern)
			if tt.isValid && err != nil {
				t.Errorf("ValidatePattern(%q) returned error %v, expected valid", tt.pattern, err)
			}
			if !tt.isValid && err == nil {
				t.Errorf("ValidatePattern(%q) returned nil, expected error", tt.pattern)
			}
		})
	}
}

func BenchmarkMatchTopic_ExactMatch(b *testing.B) {
	for i := 0; i < b.N; i++ {
		MatchTopic("health_change", "health_change")
	}
}

func BenchmarkMatchTopic_SingleWildcard(b *testing.B) {
	for i := 0; i < b.N; i++ {
		MatchTopic("container.+", "container.logs")
	}
}

func BenchmarkMatchTopic_MultiWildcard(b *testing.B) {
	for i := 0; i < b.N; i++ {
		MatchTopic("container.#", "container.logs.data.extra")
	}
}

func BenchmarkMatchesAnyTopic_FivePatterns(b *testing.B) {
	patterns := []string{"health_change", "discovery", "container.#", "peer_+", "config_reload"}
	for i := 0; i < b.N; i++ {
		MatchesAnyTopic(patterns, "container.logs.data")
	}
}
