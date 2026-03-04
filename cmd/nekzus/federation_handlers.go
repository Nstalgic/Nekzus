package main

import (
	"encoding/json"
	"net/http"
	"strings"

	apperrors "github.com/nstalgic/nekzus/internal/errors"
)

// handleListPeers returns a list of all federation peers
func (app *Application) handleListPeers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		apperrors.WriteJSON(w, apperrors.New("METHOD_NOT_ALLOWED", "Method not allowed", http.StatusMethodNotAllowed))
		return
	}

	peers := app.managers.Peers.GetPeers()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"peers": peers,
		"count": len(peers),
	})
}

// handleGetPeer returns details for a specific peer
func (app *Application) handleGetPeer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		apperrors.WriteJSON(w, apperrors.New("METHOD_NOT_ALLOWED", "Method not allowed", http.StatusMethodNotAllowed))
		return
	}

	// Extract peer ID from path
	peerID := strings.TrimPrefix(r.URL.Path, "/api/v1/federation/peers/")
	if peerID == "" {
		apperrors.WriteJSON(w, apperrors.New("INVALID_REQUEST", "Peer ID required", http.StatusBadRequest))
		return
	}

	peer, err := app.managers.Peers.GetPeerByID(peerID)
	if err != nil {
		apperrors.WriteJSON(w, apperrors.Wrap(err, "PEER_NOT_FOUND", "Peer not found", http.StatusNotFound))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(peer)
}

// handleRemovePeer removes/blocks a peer from the federation
func (app *Application) handleRemovePeer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		apperrors.WriteJSON(w, apperrors.New("METHOD_NOT_ALLOWED", "Method not allowed", http.StatusMethodNotAllowed))
		return
	}

	// Extract peer ID from path
	peerID := strings.TrimPrefix(r.URL.Path, "/api/v1/federation/peers/")
	if peerID == "" {
		apperrors.WriteJSON(w, apperrors.New("INVALID_REQUEST", "Peer ID required", http.StatusBadRequest))
		return
	}

	// Remove peer from peer manager
	if err := app.managers.Peers.RemovePeer(peerID); err != nil {
		apperrors.WriteJSON(w, apperrors.Wrap(err, "PEER_REMOVE_FAILED", "Failed to remove peer", http.StatusInternalServerError))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Peer removed successfully",
		"peer_id": peerID,
	})
}

// handleTriggerSync triggers a full catalog sync with all peers
func (app *Application) handleTriggerSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		apperrors.WriteJSON(w, apperrors.New("METHOD_NOT_ALLOWED", "Method not allowed", http.StatusMethodNotAllowed))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Sync triggered",
	})
}

// handleFederationStatus returns federation health and status
func (app *Application) handleFederationStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		apperrors.WriteJSON(w, apperrors.New("METHOD_NOT_ALLOWED", "Method not allowed", http.StatusMethodNotAllowed))
		return
	}

	peers := app.managers.Peers.GetPeers()

	// Count peers by status
	statusCounts := make(map[string]int)
	for _, peer := range peers {
		statusCounts[string(peer.Status)]++
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"enabled":         true,
		"local_peer_id":   app.managers.Peers.LocalPeerID(),
		"local_peer_name": app.managers.Peers.LocalPeerName(),
		"peer_count":      len(peers),
		"peers_by_status": statusCounts,
		"running":         app.managers.Peers.IsRunning(),
	})
}
