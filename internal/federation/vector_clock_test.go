package federation

import (
	"encoding/json"
	"testing"
)

// TestVectorClock_NewVectorClock tests creating a new vector clock
func TestVectorClock_NewVectorClock(t *testing.T) {
	vc := NewVectorClock()

	if vc == nil {
		t.Fatal("NewVectorClock() returned nil")
	}

	if len(vc) != 0 {
		t.Errorf("New vector clock should be empty, got %d entries", len(vc))
	}
}

// TestVectorClock_Increment tests incrementing a vector clock
func TestVectorClock_Increment(t *testing.T) {
	vc := NewVectorClock()
	peerID := "peer1"

	// First increment
	vc.Increment(peerID)
	if vc[peerID] != 1 {
		t.Errorf("Expected clock[%s] = 1, got %d", peerID, vc[peerID])
	}

	// Second increment
	vc.Increment(peerID)
	if vc[peerID] != 2 {
		t.Errorf("Expected clock[%s] = 2, got %d", peerID, vc[peerID])
	}

	// Different peer
	vc.Increment("peer2")
	if vc["peer2"] != 1 {
		t.Errorf("Expected clock[peer2] = 1, got %d", vc["peer2"])
	}

	// Original peer unchanged
	if vc[peerID] != 2 {
		t.Errorf("Expected clock[%s] still = 2, got %d", peerID, vc[peerID])
	}
}

// TestVectorClock_Merge tests merging two vector clocks
func TestVectorClock_Merge(t *testing.T) {
	tests := []struct {
		name     string
		vc1      VectorClock
		vc2      VectorClock
		expected VectorClock
	}{
		{
			name:     "empty clocks",
			vc1:      VectorClock{},
			vc2:      VectorClock{},
			expected: VectorClock{},
		},
		{
			name:     "merge with empty",
			vc1:      VectorClock{"peer1": 5},
			vc2:      VectorClock{},
			expected: VectorClock{"peer1": 5},
		},
		{
			name:     "merge disjoint clocks",
			vc1:      VectorClock{"peer1": 3},
			vc2:      VectorClock{"peer2": 5},
			expected: VectorClock{"peer1": 3, "peer2": 5},
		},
		{
			name:     "merge overlapping clocks - take max",
			vc1:      VectorClock{"peer1": 3, "peer2": 7},
			vc2:      VectorClock{"peer1": 5, "peer2": 4},
			expected: VectorClock{"peer1": 5, "peer2": 7},
		},
		{
			name:     "merge with partial overlap",
			vc1:      VectorClock{"peer1": 3, "peer2": 7},
			vc2:      VectorClock{"peer2": 4, "peer3": 9},
			expected: VectorClock{"peer1": 3, "peer2": 7, "peer3": 9},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create copies to avoid mutation
			vc1 := make(VectorClock)
			for k, v := range tt.vc1 {
				vc1[k] = v
			}

			vc1.Merge(tt.vc2)

			// Check all expected keys
			for peer, expectedCount := range tt.expected {
				if vc1[peer] != expectedCount {
					t.Errorf("Expected clock[%s] = %d, got %d", peer, expectedCount, vc1[peer])
				}
			}

			// Check no extra keys
			if len(vc1) != len(tt.expected) {
				t.Errorf("Expected %d entries, got %d", len(tt.expected), len(vc1))
			}
		})
	}
}

// TestVectorClock_Compare tests comparing vector clocks
func TestVectorClock_Compare(t *testing.T) {
	tests := []struct {
		name     string
		vc1      VectorClock
		vc2      VectorClock
		expected int // -1 = vc1 < vc2, 0 = concurrent, 1 = vc1 > vc2
	}{
		{
			name:     "equal clocks",
			vc1:      VectorClock{"peer1": 5, "peer2": 3},
			vc2:      VectorClock{"peer1": 5, "peer2": 3},
			expected: 0, // Equal counts as concurrent
		},
		{
			name:     "vc1 happened before vc2",
			vc1:      VectorClock{"peer1": 3, "peer2": 2},
			vc2:      VectorClock{"peer1": 5, "peer2": 4},
			expected: -1,
		},
		{
			name:     "vc1 happened after vc2",
			vc1:      VectorClock{"peer1": 7, "peer2": 6},
			vc2:      VectorClock{"peer1": 5, "peer2": 4},
			expected: 1,
		},
		{
			name:     "concurrent updates - incomparable",
			vc1:      VectorClock{"peer1": 7, "peer2": 2},
			vc2:      VectorClock{"peer1": 3, "peer2": 9},
			expected: 0,
		},
		{
			name:     "empty vs non-empty - empty is less",
			vc1:      VectorClock{},
			vc2:      VectorClock{"peer1": 1},
			expected: -1,
		},
		{
			name:     "non-empty vs empty - non-empty is greater",
			vc1:      VectorClock{"peer1": 1},
			vc2:      VectorClock{},
			expected: 1,
		},
		{
			name:     "both empty - concurrent",
			vc1:      VectorClock{},
			vc2:      VectorClock{},
			expected: 0,
		},
		{
			name:     "disjoint peers - concurrent",
			vc1:      VectorClock{"peer1": 5},
			vc2:      VectorClock{"peer2": 3},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.vc1.Compare(tt.vc2)
			if result != tt.expected {
				t.Errorf("Expected Compare() = %d, got %d", tt.expected, result)
			}
		})
	}
}

// TestVectorClock_Copy tests deep copying a vector clock
func TestVectorClock_Copy(t *testing.T) {
	original := VectorClock{"peer1": 5, "peer2": 3}

	copy := original.Copy()

	// Should be equal initially
	if copy.Compare(original) != 0 {
		t.Error("Copy should be equal to original")
	}

	// Modify copy
	copy.Increment("peer1")

	// Original should be unchanged
	if original["peer1"] != 5 {
		t.Errorf("Original should be unchanged, got clock[peer1] = %d", original["peer1"])
	}

	// Copy should be different
	if copy["peer1"] != 6 {
		t.Errorf("Copy should be incremented, got clock[peer1] = %d", copy["peer1"])
	}
}

// TestVectorClock_JSON tests JSON marshaling/unmarshaling
func TestVectorClock_JSON(t *testing.T) {
	original := VectorClock{
		"peer1": 10,
		"peer2": 20,
		"peer3": 5,
	}

	// Marshal to JSON
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	// Unmarshal from JSON
	var decoded VectorClock
	err = json.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	// Should be equal
	if decoded.Compare(original) != 0 {
		t.Error("Decoded clock should equal original")
	}

	// Check all values
	for peer, count := range original {
		if decoded[peer] != count {
			t.Errorf("Mismatch for %s: expected %d, got %d", peer, count, decoded[peer])
		}
	}
}

// TestVectorClock_JSONEmpty tests marshaling empty clock
func TestVectorClock_JSONEmpty(t *testing.T) {
	empty := NewVectorClock()

	data, err := json.Marshal(empty)
	if err != nil {
		t.Fatalf("Failed to marshal empty clock: %v", err)
	}

	// Should be valid JSON (either {} or null)
	var decoded VectorClock
	err = json.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Failed to unmarshal empty clock: %v", err)
	}
}

// TestVectorClock_EdgeCases tests edge cases
func TestVectorClock_EdgeCases(t *testing.T) {
	t.Run("increment on empty map", func(t *testing.T) {
		vc := NewVectorClock() // Use NewVectorClock() to get initialized map
		vc.Increment("peer1")
		// Should work
		if vc["peer1"] != 1 {
			t.Error("Increment on empty map should work")
		}
	})

	t.Run("merge with nil", func(t *testing.T) {
		vc := VectorClock{"peer1": 5}
		var nilVC VectorClock
		vc.Merge(nilVC)
		// Should not panic
		if vc["peer1"] != 5 {
			t.Error("Merge with nil should not change clock")
		}
	})

	t.Run("compare with nil", func(t *testing.T) {
		vc := VectorClock{"peer1": 5}
		var nilVC VectorClock
		result := vc.Compare(nilVC)
		// Non-empty should be greater than nil
		if result != 1 {
			t.Errorf("Non-empty should be > nil, got %d", result)
		}
	})
}
