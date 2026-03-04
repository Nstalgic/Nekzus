package federation

// ResolveConflict determines which version of a federated service to keep when conflicts occur.
//
// Conflict resolution algorithm (in priority order):
//  1. Local-first precedence: If service originates from local peer, prefer local version (when same origin)
//  2. Remote-first precedence: If service originates from remote peer, prefer remote version (when same origin)
//  3. Vector clock comparison: If same origin, use causality ordering
//     - If local happened before remote → keep remote (newer)
//     - If local happened after remote → keep local (newer)
//  4. Concurrent tie-breaker: If clocks are concurrent, use LastSeen timestamp (most recent wins)
//
// Parameters:
//   - local: The local version of the service (can be nil if doesn't exist locally)
//   - remote: The remote version of the service (can be nil)
//   - localPeerID: The ID of the local peer
//
// Returns:
//   - The winning FederatedService (either local or remote)
func ResolveConflict(local, remote *FederatedService, localPeerID string) *FederatedService {
	// Handle nil cases
	if local == nil && remote == nil {
		return nil // Both nil
	}
	if local == nil {
		return remote // No local version, accept remote
	}
	if remote == nil {
		return local // No remote version, keep local
	}

	// Determine who owns the service
	localOwnsService := local.OriginPeerID == localPeerID

	// Rule 1: Both versions have same origin peer
	if local.OriginPeerID == remote.OriginPeerID {
		// Same origin → check if local or remote owns it
		if localOwnsService {
			// Local peer is the owner → use vector clock (local-first applies to causality, not timestamp)
			return resolveByVectorClock(local, remote)
		} else {
			// Remote peer is the owner → prefer remote version (remote-first)
			return remote
		}
	}

	// Rule 2: Different origin peers (shouldn't happen often, but handle it)
	// Prefer whichever has the higher vector clock
	return resolveByVectorClock(local, remote)
}

// resolveByVectorClock uses vector clock comparison to determine which version to keep
func resolveByVectorClock(local, remote *FederatedService) *FederatedService {
	// Compare vector clocks
	comparison := local.VectorClock.Compare(remote.VectorClock)

	switch comparison {
	case -1:
		// Local happened before remote → remote is newer
		return remote

	case 1:
		// Local happened after remote → local is newer
		return local

	case 0:
		// Concurrent or equal → use tie-breaker
		return resolveTieBreaker(local, remote)
	}

	// Default (should not reach here)
	return local
}

// resolveTieBreaker resolves concurrent updates using LastSeen timestamp
// Most recently seen version wins
func resolveTieBreaker(local, remote *FederatedService) *FederatedService {
	// Compare LastSeen timestamps
	if local.LastSeen.After(remote.LastSeen) {
		return local
	}

	if remote.LastSeen.After(local.LastSeen) {
		return remote
	}

	// Exact same timestamp (very rare) → default to local
	return local
}
