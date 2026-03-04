package federation

// VectorClock tracks causality for conflict resolution in federated services
// Maps peer_id -> counter to track events from each peer
type VectorClock map[string]uint64

// NewVectorClock creates a new empty vector clock
func NewVectorClock() VectorClock {
	return make(VectorClock)
}

// Increment increments the clock for the given peer ID
func (vc VectorClock) Increment(peerID string) {
	// Handle nil map by initializing
	if vc == nil {
		vc = make(VectorClock)
	}
	vc[peerID]++
}

// Merge combines two vector clocks by taking the maximum value for each peer
// This is used when merging state from different peers
func (vc VectorClock) Merge(other VectorClock) {
	if other == nil {
		return
	}

	for peer, count := range other {
		if count > vc[peer] {
			vc[peer] = count
		}
	}
}

// Compare compares two vector clocks to determine causality ordering
// Returns:
//
//	-1 if vc happened before other (vc < other)
//	 0 if concurrent (neither happened before the other)
//	 1 if vc happened after other (vc > other)
func (vc VectorClock) Compare(other VectorClock) int {
	// Collect all peer IDs from both clocks
	allPeers := make(map[string]bool)
	for peer := range vc {
		allPeers[peer] = true
	}
	for peer := range other {
		allPeers[peer] = true
	}

	// Track if vc is less than or greater than other for any peer
	lessThan := false
	greaterThan := false

	for peer := range allPeers {
		myCount := uint64(0)
		if vc != nil {
			myCount = vc[peer]
		}

		otherCount := uint64(0)
		if other != nil {
			otherCount = other[peer]
		}

		if myCount < otherCount {
			lessThan = true
		} else if myCount > otherCount {
			greaterThan = true
		}
	}

	// Determine ordering
	if lessThan && !greaterThan {
		return -1 // vc happened before other
	} else if greaterThan && !lessThan {
		return 1 // vc happened after other
	}

	// Either equal or concurrent
	return 0
}

// Copy creates a deep copy of the vector clock
func (vc VectorClock) Copy() VectorClock {
	if vc == nil {
		return NewVectorClock()
	}

	copy := make(VectorClock, len(vc))
	for k, v := range vc {
		copy[k] = v
	}
	return copy
}
