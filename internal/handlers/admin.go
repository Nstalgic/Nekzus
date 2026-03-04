package handlers

import (
	"log/slog"
	"net/http"

	"github.com/nstalgic/nekzus/internal/httputil"
)

var adminlog = slog.With("package", "handlers")

// AdminHandler handles admin endpoints
type AdminHandler struct {
	version      string
	nexusID      string
	capabilities []string
	buildDate    string
}

// NewAdminHandler creates a new admin handler
func NewAdminHandler(version, nexusID string, capabilities []string) *AdminHandler {
	return &AdminHandler{
		version:      version,
		nexusID:      nexusID,
		capabilities: capabilities,
		buildDate:    "2025-10-13",
	}
}

// HandleInfo returns Nexus instance information and capabilities
func (h *AdminHandler) HandleInfo(w http.ResponseWriter, r *http.Request) {
	info := map[string]interface{}{
		"version":      h.version,
		"nexusId":      h.nexusID,
		"capabilities": h.capabilities,
		"buildDate":    h.buildDate,
	}
	if err := httputil.WriteJSON(w, http.StatusOK, info); err != nil {
		adminlog.Error("Error encoding JSON response", "error", err)
	}
}
