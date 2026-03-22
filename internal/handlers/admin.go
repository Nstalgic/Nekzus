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
	nekzusID     string
	capabilities []string
	buildDate    string
}

// NewAdminHandler creates a new admin handler
func NewAdminHandler(version, nekzusID string, capabilities []string) *AdminHandler {
	return &AdminHandler{
		version:      version,
		nekzusID:     nekzusID,
		capabilities: capabilities,
		buildDate:    "2025-10-13",
	}
}

// HandleInfo returns Nexus instance information and capabilities
func (h *AdminHandler) HandleInfo(w http.ResponseWriter, r *http.Request) {
	info := map[string]interface{}{
		"version":      h.version,
		"nekzusId":     h.nekzusID,
		"capabilities": h.capabilities,
		"buildDate":    h.buildDate,
	}
	if err := httputil.WriteJSON(w, http.StatusOK, info); err != nil {
		adminlog.Error("Error encoding JSON response", "error", err)
	}
}
