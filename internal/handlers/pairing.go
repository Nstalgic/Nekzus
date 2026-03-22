package handlers

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/nstalgic/nekzus/internal/auth"
	apperrors "github.com/nstalgic/nekzus/internal/errors"
	"github.com/nstalgic/nekzus/internal/httputil"
	"github.com/nstalgic/nekzus/internal/ratelimit"
)

var pairLog = slog.With("package", "handlers", "handler", "pairing")

// PairingHandler handles the v2 pairing flow with short codes
type PairingHandler struct {
	pairingManager *auth.PairingManager
	rateLimiter    *ratelimit.Limiter
}

// NewPairingHandler creates a new pairing handler
func NewPairingHandler(pm *auth.PairingManager, limiter *ratelimit.Limiter) *PairingHandler {
	return &PairingHandler{
		pairingManager: pm,
		rateLimiter:    limiter,
	}
}

// PairingRequest represents the request body for code redemption
type PairingRequest struct {
	Code string `json:"code"`
}

// HandleRedeemPairingCode redeems a pairing code and returns the config
// POST /api/v1/pair
// Request body: {"code": "ABCD1234"}
// Requires header: X-Pairing-Request: true
func (h *PairingHandler) HandleRedeemPairingCode(w http.ResponseWriter, r *http.Request) {
	clientIP := getClientIP(r)

	pairLog.Debug("pairing redemption request received",
		"method", r.Method,
		"ip", clientIP,
		"content_type", r.Header.Get("Content-Type"),
		"has_pairing_header", r.Header.Get("X-Pairing-Request") == "true",
		"user_agent", r.UserAgent())

	// Only accept POST requests
	if r.Method != http.MethodPost {
		pairLog.Debug("pairing rejected: wrong method", "method", r.Method, "ip", clientIP)
		apperrors.WriteJSON(w, apperrors.New(
			"METHOD_NOT_ALLOWED",
			"Use POST to redeem pairing codes",
			http.StatusMethodNotAllowed,
		))
		return
	}

	// Require explicit pairing header to prevent CSRF
	if r.Header.Get("X-Pairing-Request") != "true" {
		pairLog.Debug("pairing rejected: missing X-Pairing-Request header",
			"ip", clientIP,
			"header_value", r.Header.Get("X-Pairing-Request"))
		apperrors.WriteJSON(w, apperrors.New(
			"MISSING_HEADER",
			"X-Pairing-Request header required",
			http.StatusBadRequest,
		))
		return
	}

	// Rate limit check
	if !h.rateLimiter.Allow(clientIP) {
		pairLog.Warn("rate limit exceeded for pairing", "ip", clientIP)
		apperrors.WriteJSON(w, apperrors.ErrRateLimitExceeded)
		return
	}

	// Parse request body
	var req PairingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		pairLog.Debug("pairing rejected: invalid request body", "ip", clientIP, "error", err)
		apperrors.WriteJSON(w, apperrors.New(
			"INVALID_REQUEST",
			"Invalid request body",
			http.StatusBadRequest,
		))
		return
	}

	code := strings.TrimSpace(req.Code)
	if code == "" {
		pairLog.Debug("pairing rejected: empty code", "ip", clientIP)
		apperrors.WriteJSON(w, apperrors.New(
			"INVALID_CODE",
			"Pairing code required",
			http.StatusBadRequest,
		))
		return
	}

	pairLog.Debug("attempting code redemption",
		"code_hash", hashCode(code),
		"code_length", len(code),
		"ip", clientIP,
		"active_codes", h.pairingManager.ActiveCodeCount())

	// Redeem the code (single-use)
	config, err := h.pairingManager.RedeemCode(code)
	if err != nil {
		pairLog.Warn("pairing code redemption failed",
			"code_hash", hashCode(code),
			"error", err,
			"ip", clientIP)

		// Check if it's a rate limit error from the pairing manager
		if strings.Contains(err.Error(), "too many pairing attempts") {
			apperrors.WriteJSON(w, apperrors.New(
				"RATE_LIMITED",
				"Too many pairing attempts, please try again later",
				http.StatusTooManyRequests,
			))
			return
		}

		// Use generic error to prevent code enumeration
		apperrors.WriteJSON(w, apperrors.New(
			"INVALID_CODE",
			"Invalid or expired pairing code",
			http.StatusNotFound,
		))
		return
	}

	pairLog.Info("pairing config retrieved",
		"code_hash", hashCode(code),
		"ip", clientIP,
		"base_url", config.BaseURL)

	// Return the full pairing config
	if err := httputil.WriteJSON(w, http.StatusOK, config); err != nil {
		pairLog.Error("failed to write pairing config response", "error", err)
	}
}

// hashCode returns a truncated hash of the code for safe logging
func hashCode(code string) string {
	hash := sha256.Sum256([]byte(code))
	return fmt.Sprintf("%x", hash[:4])
}

// HandleCodeStatus returns the status of a pairing code
// GET /api/v1/auth/qr/status?code=ABCD1234
func (h *PairingHandler) HandleCodeStatus(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	if code == "" {
		apperrors.WriteJSON(w, apperrors.New(
			"MISSING_CODE",
			"Code parameter required",
			http.StatusBadRequest,
		))
		return
	}

	status := h.pairingManager.GetCodeStatus(code)

	if err := httputil.WriteJSON(w, http.StatusOK, status); err != nil {
		pairLog.Error("failed to write status response", "error", err)
	}
}
