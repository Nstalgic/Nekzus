package federation

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/nstalgic/nekzus/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockPeerManagerForDelegate creates a minimal PeerManager for testing catalogDelegate
func mockPeerManagerForDelegate(localPeerID string, syncer *CatalogSyncer) *PeerManager {
	return &PeerManager{
		config: Config{
			LocalPeerID: localPeerID,
		},
		catalogSyncer: syncer,
	}
}

// TestAntiEntropyState_JSONSerialization tests that AntiEntropyState serializes correctly
func TestAntiEntropyState_JSONSerialization(t *testing.T) {
	now := time.Now().Truncate(time.Second)

	state := AntiEntropyState{
		SenderID:  "nxs_peer123",
		Timestamp: now,
		Services: []FederatedService{
			{
				ServiceID:    "grafana",
				OriginPeerID: "nxs_peer123",
				App: &types.App{
					ID:   "grafana",
					Name: "Grafana",
				},
				Confidence:  1.0,
				LastSeen:    now,
				Tombstone:   false,
				VectorClock: VectorClock{"nxs_peer123": 5},
			},
		},
		VectorClock: VectorClock{"nxs_peer123": 5},
	}

	// Serialize
	data, err := json.Marshal(state)
	require.NoError(t, err, "should marshal without error")
	require.NotEmpty(t, data, "marshaled data should not be empty")

	// Deserialize
	var decoded AntiEntropyState
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err, "should unmarshal without error")

	// Verify fields
	assert.Equal(t, state.SenderID, decoded.SenderID)
	assert.Equal(t, state.Timestamp.Unix(), decoded.Timestamp.Unix())
	assert.Len(t, decoded.Services, 1)
	assert.Equal(t, "grafana", decoded.Services[0].ServiceID)
	assert.Equal(t, uint64(5), decoded.VectorClock["nxs_peer123"])
}

// TestAntiEntropyState_EmptyServices tests serialization with empty services
func TestAntiEntropyState_EmptyServices(t *testing.T) {
	state := AntiEntropyState{
		SenderID:    "nxs_peer123",
		Timestamp:   time.Now(),
		Services:    []FederatedService{},
		VectorClock: VectorClock{},
	}

	data, err := json.Marshal(state)
	require.NoError(t, err)

	var decoded AntiEntropyState
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, state.SenderID, decoded.SenderID)
	assert.Empty(t, decoded.Services)
}

// TestAntiEntropyState_WithTombstones tests that tombstoned services are included
func TestAntiEntropyState_WithTombstones(t *testing.T) {
	now := time.Now()

	state := AntiEntropyState{
		SenderID:  "nxs_peer123",
		Timestamp: now,
		Services: []FederatedService{
			{
				ServiceID:    "active-service",
				OriginPeerID: "nxs_peer123",
				Tombstone:    false,
				VectorClock:  VectorClock{"nxs_peer123": 1},
			},
			{
				ServiceID:    "deleted-service",
				OriginPeerID: "nxs_peer123",
				Tombstone:    true,
				VectorClock:  VectorClock{"nxs_peer123": 2},
			},
		},
		VectorClock: VectorClock{"nxs_peer123": 2},
	}

	data, err := json.Marshal(state)
	require.NoError(t, err)

	var decoded AntiEntropyState
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Len(t, decoded.Services, 2)

	// Find the tombstoned service
	var foundTombstone bool
	for _, svc := range decoded.Services {
		if svc.ServiceID == "deleted-service" {
			assert.True(t, svc.Tombstone, "tombstone flag should be preserved")
			foundTombstone = true
		}
	}
	assert.True(t, foundTombstone, "tombstoned service should be in the list")
}

// TestCatalogDelegate_LocalState_NilSyncer tests LocalState with no catalog syncer
func TestCatalogDelegate_LocalState_NilSyncer(t *testing.T) {
	pm := mockPeerManagerForDelegate("nxs_peer1", nil)
	cd := &catalogDelegate{pm: pm}

	result := cd.LocalState(false)

	// Should return empty bytes when no syncer
	assert.Empty(t, result)
}

// TestCatalogDelegate_LocalState_Empty tests LocalState with empty catalog
func TestCatalogDelegate_LocalState_Empty(t *testing.T) {
	syncer := NewCatalogSyncer("nxs_peer1", nil)
	pm := mockPeerManagerForDelegate("nxs_peer1", syncer)
	cd := &catalogDelegate{pm: pm}

	result := cd.LocalState(false)

	// Should return valid JSON with empty services
	require.NotEmpty(t, result, "should return valid JSON even with empty catalog")

	var state AntiEntropyState
	err := json.Unmarshal(result, &state)
	require.NoError(t, err, "should be valid JSON")

	assert.Equal(t, "nxs_peer1", state.SenderID)
	assert.Empty(t, state.Services)
}

// TestCatalogDelegate_LocalState_IncludesVectorClock tests that vector clock is included
func TestCatalogDelegate_LocalState_IncludesVectorClock(t *testing.T) {
	syncer := NewCatalogSyncer("nxs_peer1", nil)
	syncer.SetBroadcastFunc(func(msg []byte) {})

	// Add services to build up vector clock
	syncer.OnLocalServiceAdded(&types.App{ID: "app1", Name: "App 1"})
	syncer.OnLocalServiceAdded(&types.App{ID: "app2", Name: "App 2"})

	pm := mockPeerManagerForDelegate("nxs_peer1", syncer)
	cd := &catalogDelegate{pm: pm}

	result := cd.LocalState(false)
	require.NotEmpty(t, result)

	var state AntiEntropyState
	err := json.Unmarshal(result, &state)
	require.NoError(t, err)

	// Vector clock should reflect the 2 additions
	assert.Equal(t, uint64(2), state.VectorClock["nxs_peer1"])
}

// TestCatalogDelegate_LocalState_JoinFlag tests behavior with join flag
func TestCatalogDelegate_LocalState_JoinFlag(t *testing.T) {
	syncer := NewCatalogSyncer("nxs_peer1", nil)
	pm := mockPeerManagerForDelegate("nxs_peer1", syncer)
	cd := &catalogDelegate{pm: pm}

	// Both join=true and join=false should work
	resultJoin := cd.LocalState(true)
	resultPeriodic := cd.LocalState(false)

	// Both should return valid JSON
	require.NotEmpty(t, resultJoin)
	require.NotEmpty(t, resultPeriodic)

	var stateJoin, statePeriodic AntiEntropyState
	require.NoError(t, json.Unmarshal(resultJoin, &stateJoin))
	require.NoError(t, json.Unmarshal(resultPeriodic, &statePeriodic))

	assert.Equal(t, "nxs_peer1", stateJoin.SenderID)
	assert.Equal(t, "nxs_peer1", statePeriodic.SenderID)
}

// TestCatalogDelegate_MergeRemoteState_EmptyBuffer tests with empty buffer
func TestCatalogDelegate_MergeRemoteState_EmptyBuffer(t *testing.T) {
	syncer := NewCatalogSyncer("nxs_peer1", nil)
	pm := mockPeerManagerForDelegate("nxs_peer1", syncer)
	cd := &catalogDelegate{pm: pm}

	// Should not panic with empty buffer
	cd.MergeRemoteState([]byte{}, false)
	cd.MergeRemoteState(nil, false)
}

// TestCatalogDelegate_MergeRemoteState_InvalidJSON tests with invalid JSON
func TestCatalogDelegate_MergeRemoteState_InvalidJSON(t *testing.T) {
	syncer := NewCatalogSyncer("nxs_peer1", nil)
	pm := mockPeerManagerForDelegate("nxs_peer1", syncer)
	cd := &catalogDelegate{pm: pm}

	// Should not panic with invalid JSON
	cd.MergeRemoteState([]byte("not valid json"), false)
	cd.MergeRemoteState([]byte("{incomplete"), false)
}

// TestCatalogDelegate_MergeRemoteState_SkipsSelf tests that own state is skipped
func TestCatalogDelegate_MergeRemoteState_SkipsSelf(t *testing.T) {
	syncer := NewCatalogSyncer("nxs_peer1", nil)
	pm := mockPeerManagerForDelegate("nxs_peer1", syncer)
	cd := &catalogDelegate{pm: pm}

	// Create state from ourselves
	selfState := AntiEntropyState{
		SenderID:  "nxs_peer1", // Same as local peer
		Timestamp: time.Now(),
		Services: []FederatedService{
			{
				ServiceID:    "app1",
				OriginPeerID: "nxs_peer1",
				VectorClock:  VectorClock{"nxs_peer1": 100},
			},
		},
		VectorClock: VectorClock{"nxs_peer1": 100},
	}

	data, _ := json.Marshal(selfState)
	cd.MergeRemoteState(data, false)

	// Clock should not have been merged
	clock := syncer.GetLocalVectorClock()
	assert.NotEqual(t, uint64(100), clock["nxs_peer1"], "should skip state from self")
}

// TestCatalogDelegate_MergeRemoteState_NewServices tests merging new services
func TestCatalogDelegate_MergeRemoteState_NewServices(t *testing.T) {
	syncer := NewCatalogSyncer("nxs_peer1", nil)
	pm := mockPeerManagerForDelegate("nxs_peer1", syncer)
	cd := &catalogDelegate{pm: pm}

	// Create state from a remote peer
	remoteState := AntiEntropyState{
		SenderID:  "nxs_peer2",
		Timestamp: time.Now(),
		Services: []FederatedService{
			{
				ServiceID:    "remote-app1",
				OriginPeerID: "nxs_peer2",
				App:          &types.App{ID: "remote-app1", Name: "Remote App 1"},
				VectorClock:  VectorClock{"nxs_peer2": 5},
				LastSeen:     time.Now(),
			},
		},
		VectorClock: VectorClock{"nxs_peer2": 5},
	}

	data, _ := json.Marshal(remoteState)
	cd.MergeRemoteState(data, false)

	// Clock should have been merged
	clock := syncer.GetLocalVectorClock()
	assert.Equal(t, uint64(5), clock["nxs_peer2"], "should merge remote vector clock")
}

// TestCatalogDelegate_MergeRemoteState_NilSyncer tests with nil catalog syncer
func TestCatalogDelegate_MergeRemoteState_NilSyncer(t *testing.T) {
	pm := mockPeerManagerForDelegate("nxs_peer1", nil)
	cd := &catalogDelegate{pm: pm}

	remoteState := AntiEntropyState{
		SenderID:  "nxs_peer2",
		Timestamp: time.Now(),
		Services:  []FederatedService{},
	}

	data, _ := json.Marshal(remoteState)

	// Should not panic with nil syncer
	cd.MergeRemoteState(data, false)
}

// TestCatalogDelegate_MergeRemoteState_JoinFlag tests behavior with join flag
func TestCatalogDelegate_MergeRemoteState_JoinFlag(t *testing.T) {
	syncer := NewCatalogSyncer("nxs_peer1", nil)
	pm := mockPeerManagerForDelegate("nxs_peer1", syncer)
	cd := &catalogDelegate{pm: pm}

	remoteState := AntiEntropyState{
		SenderID:  "nxs_peer2",
		Timestamp: time.Now(),
		Services: []FederatedService{
			{
				ServiceID:    "app1",
				OriginPeerID: "nxs_peer2",
				VectorClock:  VectorClock{"nxs_peer2": 3},
			},
		},
		VectorClock: VectorClock{"nxs_peer2": 3},
	}

	data, _ := json.Marshal(remoteState)

	// Both join=true and join=false should work
	cd.MergeRemoteState(data, true)

	clock := syncer.GetLocalVectorClock()
	assert.Equal(t, uint64(3), clock["nxs_peer2"])
}
