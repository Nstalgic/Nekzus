package handlers

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/nstalgic/nekzus/internal/auth"
	apperrors "github.com/nstalgic/nekzus/internal/errors"
	"github.com/nstalgic/nekzus/internal/httputil"
	"github.com/nstalgic/nekzus/internal/ratelimit"
	qrcode "github.com/skip2/go-qrcode"
)

var qrlog = slog.With("package", "handlers")

// SPKIProvider interface for getting SPKI pins
type SPKIProvider interface {
	GetSPKIPins() []string
}

// QRHandler handles QR code generation for mobile pairing
type QRHandler struct {
	authManager    *auth.Manager
	pairingManager *auth.PairingManager
	rateLimiter    *ratelimit.Limiter
	spkiProvider   SPKIProvider
	baseURL        string
	nekzusID       string
	capabilities   []string
}

// NewQRHandler creates a new QR handler
func NewQRHandler(authMgr *auth.Manager, limiter *ratelimit.Limiter, baseURL, tlsCertPath, nekzusID string, capabilities []string) *QRHandler {
	return &QRHandler{
		authManager:  authMgr,
		rateLimiter:  limiter,
		baseURL:      baseURL,
		nekzusID:     nekzusID,
		capabilities: capabilities,
	}
}

// SetPairingManager sets the pairing manager for short code flow
func (h *QRHandler) SetPairingManager(pm *auth.PairingManager) {
	h.pairingManager = pm
}

// SetSPKIProvider sets the SPKI provider for 2-pin support
func (h *QRHandler) SetSPKIProvider(provider SPKIProvider) {
	h.spkiProvider = provider
}

// SetBaseURL updates the base URL (called after TLS upgrade)
func (h *QRHandler) SetBaseURL(baseURL string) {
	h.baseURL = baseURL
}

// HandleQRCode generates a minimal QR code for mobile app pairing.
// The QR contains only the base URL and a short pairing code.
// Mobile app fetches full config via GET /api/v1/pair/{code}.
// Query params:
//   - format=png: Return PNG image instead of JSON
func (h *QRHandler) HandleQRCode(w http.ResponseWriter, r *http.Request) {
	// Rate limit check
	clientIP := getClientIP(r)
	if !h.rateLimiter.Allow(clientIP) {
		apperrors.WriteJSON(w, apperrors.ErrRateLimitExceeded)
		return
	}

	if h.pairingManager == nil {
		apperrors.WriteJSON(w, apperrors.New("PAIRING_UNAVAILABLE", "Pairing manager not initialized", http.StatusServiceUnavailable))
		return
	}

	// Generate short-lived bootstrap token
	bootstrapToken, err := h.authManager.GenerateShortLivedToken(5 * time.Minute)
	if err != nil {
		qrlog.Error("Failed to generate bootstrap token", "error", err)
		apperrors.WriteJSON(w, apperrors.Wrap(err, "TOKEN_GENERATION_FAILED", "Failed to generate bootstrap token", http.StatusInternalServerError))
		return
	}

	// Get SPKI pins (primary + backup for TrustKit iOS)
	spkiPins := h.getSPKIPins()

	// Determine hostname
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "Nexus"
	}

	// Create full pairing config (stored server-side, retrieved via short code)
	config := auth.PairingConfig{
		BaseURL:        h.baseURL,
		Name:           "Nekzus @ " + hostname,
		SPKIPins:       spkiPins,
		BootstrapToken: bootstrapToken,
		Capabilities:   h.capabilities,
		NekzusID:       h.nekzusID,
	}

	// Generate pairing code
	code, err := h.pairingManager.GenerateCode(config)
	if err != nil {
		qrlog.Error("Failed to generate pairing code", "error", err)
		apperrors.WriteJSON(w, apperrors.Wrap(err, "CODE_GENERATION_FAILED", "Failed to generate pairing code", http.StatusInternalServerError))
		return
	}

	// Build minimal QR payload
	payload := map[string]interface{}{
		"u": h.baseURL, // base URL
		"c": code,      // pairing code
	}

	qrlog.Info("generated QR code",
		"code", code,
		"base_url", h.baseURL,
		"pins_count", len(spkiPins))

	qrlog.Debug("QR pairing details",
		"code_hash", hashCode(code),
		"code_length", len(code),
		"bootstrap_token_prefix", bootstrapToken[:min(10, len(bootstrapToken))]+"...",
		"nekzus_id", h.nekzusID,
		"capabilities", fmt.Sprintf("%v", h.capabilities),
		"hostname", hostname)

	// Check if client wants QR code image
	if r.URL.Query().Get("format") == "png" {
		h.servePNGQRCode(w, payload)
		return
	}

	// Return JSON payload (includes code for display)
	response := map[string]interface{}{
		"qr":   payload,
		"code": code, // Also include code separately for manual entry
	}
	if err := httputil.WriteJSON(w, http.StatusOK, response); err != nil {
		qrlog.Error("Error encoding JSON response", "error", err)
	}
}

// getSPKIPins returns the SPKI pins from the provider
func (h *QRHandler) getSPKIPins() []string {
	if h.spkiProvider != nil {
		return h.spkiProvider.GetSPKIPins()
	}
	return []string{}
}

// servePNGQRCode generates and serves a PNG QR code
func (h *QRHandler) servePNGQRCode(w http.ResponseWriter, payload map[string]interface{}) {
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		apperrors.WriteJSON(w, apperrors.Wrap(err, "JSON_ENCODING_FAILED", "Failed to encode payload", http.StatusInternalServerError))
		return
	}

	qrCode, err := qrcode.Encode(string(payloadJSON), qrcode.Medium, 256)
	if err != nil {
		apperrors.WriteJSON(w, apperrors.Wrap(err, "QR_GENERATION_FAILED", "Failed to generate QR code", http.StatusInternalServerError))
		return
	}

	w.Header().Set("Content-Type", "image/png")
	w.WriteHeader(http.StatusOK)
	w.Write(qrCode)
}

// getClientIP extracts the client IP address from the request
func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header first
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return xff
	}

	// Check X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// Fall back to RemoteAddr
	host := r.RemoteAddr
	if colon := len(host) - 1; colon >= 0 {
		for i := colon; i >= 0; i-- {
			if host[i] == ':' {
				return host[:i]
			}
		}
	}

	return host
}
