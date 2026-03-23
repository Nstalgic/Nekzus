package handlers

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/nstalgic/nekzus/internal/auth"
	apperrors "github.com/nstalgic/nekzus/internal/errors"
	"github.com/nstalgic/nekzus/internal/httputil"
	"github.com/nstalgic/nekzus/internal/metrics"
	"github.com/nstalgic/nekzus/internal/ratelimit"
	"github.com/nstalgic/nekzus/internal/storage"
	"github.com/nstalgic/nekzus/internal/types"
	qrcode "github.com/skip2/go-qrcode"
)

var authlog = slog.With("package", "handlers")

// Body size limits for auth endpoints
const (
	// MaxAuthRequestBodySize is the maximum size for auth API request bodies (10KB)
	MaxAuthRequestBodySize = 10 * 1024
)

// MetricsRecorder is an interface for recording metrics
type MetricsRecorder interface {
	RecordAuthPairing(status, platform string, duration time.Duration)
	RecordAuthRefresh(status string)
}

// Ensure Metrics implements MetricsRecorder at compile time
var _ MetricsRecorder = (*metrics.Metrics)(nil)

// EventPublisher is an interface for publishing SSE events
type EventPublisher interface {
	PublishDevicePaired(deviceID, deviceName, platform string)
	PublishDeviceRevoked(deviceID string)
}

// ActivityTracker is an interface for tracking activity events
type ActivityTracker interface {
	Add(event types.ActivityEvent) error
}

// WebSocketDisconnecter is an interface for disconnecting WebSocket clients
type WebSocketDisconnecter interface {
	DisconnectDevice(deviceID string) int
}

// CertificateManager provides access to active certificates for SPKI calculation
type CertificateManager interface {
	GetFirstAvailableCertificate() *tls.Certificate
	GetFallbackCertificate() *tls.Certificate
}

// CertificateManagerWithBackup extends CertificateManager with backup certificate support
// for SPKI pin rotation during certificate renewal
type CertificateManagerWithBackup interface {
	CertificateManager
	GetBackupCertificate() *tls.Certificate
}

// AuthHandler handles authentication endpoints
type AuthHandler struct {
	authManager    *auth.Manager
	storage        *storage.Store
	metrics        MetricsRecorder
	events         EventPublisher
	activity       ActivityTracker
	qrLimiter      *ratelimit.Limiter
	certManager    CertificateManager
	wsDisconnecter WebSocketDisconnecter
	pairingManager *auth.PairingManager
	baseURL        string
	tlsCertPath    string
	nekzusID       string
	capabilities   []string
	version        string
}

// NewAuthHandler creates a new auth handler
func NewAuthHandler(authMgr *auth.Manager, store *storage.Store, m MetricsRecorder, events EventPublisher, activity ActivityTracker, qrLimiter *ratelimit.Limiter, certMgr CertificateManager, baseURL, tlsCertPath, nekzusID, version string, capabilities []string) *AuthHandler {
	return &AuthHandler{
		authManager:  authMgr,
		storage:      store,
		metrics:      m,
		events:       events,
		activity:     activity,
		qrLimiter:    qrLimiter,
		certManager:  certMgr,
		baseURL:      baseURL,
		tlsCertPath:  tlsCertPath,
		nekzusID:     nekzusID,
		capabilities: capabilities,
		version:      version,
	}
}

// NewAuthHandlerWithBackupCert creates a new auth handler with backup certificate support
// for SPKI pin rotation. The certMgr should implement CertificateManagerWithBackup.
func NewAuthHandlerWithBackupCert(authMgr *auth.Manager, store *storage.Store, m MetricsRecorder, events EventPublisher, activity ActivityTracker, qrLimiter *ratelimit.Limiter, certMgr CertificateManagerWithBackup, baseURL, tlsCertPath, nekzusID, version string, capabilities []string) *AuthHandler {
	return &AuthHandler{
		authManager:  authMgr,
		storage:      store,
		metrics:      m,
		events:       events,
		activity:     activity,
		qrLimiter:    qrLimiter,
		certManager:  certMgr,
		baseURL:      baseURL,
		tlsCertPath:  tlsCertPath,
		nekzusID:     nekzusID,
		capabilities: capabilities,
		version:      version,
	}
}

// SetBaseURL updates the base URL used for QR code generation.
// This is called when the server upgrades from HTTP to HTTPS.
func (h *AuthHandler) SetBaseURL(baseURL string) {
	h.baseURL = baseURL
}

// SetWebSocketDisconnecter sets the WebSocket disconnecter for cleaning up
// stale connections during re-pairing.
func (h *AuthHandler) SetWebSocketDisconnecter(d WebSocketDisconnecter) {
	h.wsDisconnecter = d
}

// SetPairingManager sets the pairing manager for consuming codes after successful pairing
func (h *AuthHandler) SetPairingManager(pm *auth.PairingManager) {
	h.pairingManager = pm
}

// PairRequest represents a device pairing request
type PairRequest struct {
	Device DeviceInfo `json:"device"`
}

// DeviceInfo contains device information
type DeviceInfo struct {
	ID        string  `json:"id"`
	Model     string  `json:"model"`
	Platform  string  `json:"platform"`
	PushToken *string `json:"pushToken,omitempty"`
}

// Validation constants
const (
	maxDeviceIDLength    = 255
	maxDeviceModelLength = 255
	maxPushTokenLength   = 512
)

// Allowed platforms
var validPlatforms = map[string]bool{
	"ios":     true,
	"android": true,
	"web":     true,
	"desktop": true,
	"linux":   true,
	"macos":   true,
	"windows": true,
}

// isValidPlatform checks if a platform string is in the allowed list
func isValidPlatform(platform string) bool {
	return validPlatforms[platform]
}

// Validate validates the pair request
func (r *PairRequest) Validate() error {
	// Platform is required
	if r.Device.Platform == "" {
		return apperrors.New("VALIDATION_ERROR", "device.platform is required", http.StatusBadRequest)
	}

	// Validate platform is from allowed set
	if !isValidPlatform(r.Device.Platform) {
		return apperrors.New("VALIDATION_ERROR", "device.platform must be one of: ios, android, web, desktop, linux, macos, windows", http.StatusBadRequest)
	}

	// Validate device ID length (if provided)
	if len(r.Device.ID) > maxDeviceIDLength {
		return apperrors.New("VALIDATION_ERROR", "device ID too long (max 255 characters)", http.StatusBadRequest)
	}

	// Validate device model length
	if len(r.Device.Model) > maxDeviceModelLength {
		return apperrors.New("VALIDATION_ERROR", "device model too long (max 255 characters)", http.StatusBadRequest)
	}

	// Validate push token length (if provided)
	if r.Device.PushToken != nil && len(*r.Device.PushToken) > maxPushTokenLength {
		return apperrors.New("VALIDATION_ERROR", "push token too long (max 512 characters)", http.StatusBadRequest)
	}

	return nil
}

// HandlePair authenticates a new device using a bootstrap token
func (h *AuthHandler) HandlePair(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	var platform string

	clientIP := getClientIP(r)
	authlog.Debug("pair request received",
		"method", r.Method,
		"ip", clientIP,
		"has_auth_header", r.Header.Get("Authorization") != "",
		"content_type", r.Header.Get("Content-Type"),
		"user_agent", r.UserAgent())

	// Extract bootstrap token
	bootstrap := httputil.ExtractBearerToken(r)
	if bootstrap == "" {
		authlog.Debug("pair rejected: no bearer token", "ip", clientIP)
		if h.metrics != nil {
			h.metrics.RecordAuthPairing("error_no_token", "", time.Since(start))
		}
		apperrors.WriteJSON(w, apperrors.ErrUnauthorized)
		return
	}

	authlog.Debug("pair: bootstrap token extracted",
		"ip", clientIP,
		"token_prefix", bootstrap[:min(10, len(bootstrap))]+"...")

	// Check if token is rate limited (too many failed attempts)
	if h.authManager.IsBootstrapRateLimited(bootstrap) {
		authlog.Debug("pair rejected: bootstrap token rate limited", "ip", clientIP)
		if h.metrics != nil {
			h.metrics.RecordAuthPairing("error_rate_limited", "", time.Since(start))
		}
		apperrors.WriteJSON(w, apperrors.NewWithCode(
			"RATE_LIMITED",
			apperrors.CodeRateLimited,
			"Too many pairing attempts. Generate a new QR code.",
			http.StatusTooManyRequests,
		))
		return
	}

	// Validate bootstrap token
	if !h.authManager.ValidateBootstrap(bootstrap) {
		authlog.Debug("pair rejected: invalid bootstrap token", "ip", clientIP)
		// Record failed attempt
		h.authManager.RecordFailedPairing(bootstrap)
		if h.metrics != nil {
			h.metrics.RecordAuthPairing("error_invalid_token", "", time.Since(start))
		}
		apperrors.WriteJSON(w, apperrors.ErrInvalidBootstrap)
		return
	}

	authlog.Debug("pair: bootstrap token valid, decoding request body", "ip", clientIP)

	// Decode and validate request
	req, err := httputil.DecodeAndValidate[PairRequest](r, w, MaxAuthRequestBodySize)
	if err != nil {
		authlog.Debug("pair rejected: request decode/validation failed", "ip", clientIP, "error", err)
		// Determine error type for metrics
		appErr, ok := err.(*apperrors.AppError)
		if ok {
			switch appErr.Code {
			case "PAYLOAD_TOO_LARGE":
				if h.metrics != nil {
					h.metrics.RecordAuthPairing("error_payload_too_large", "", time.Since(start))
				}
			case "INVALID_JSON":
				if h.metrics != nil {
					h.metrics.RecordAuthPairing("error_invalid_json", "", time.Since(start))
				}
			default:
				// Validation error
				if h.metrics != nil {
					platform := ""
					if req != nil {
						platform = req.Device.Platform
					}
					h.metrics.RecordAuthPairing("error_validation", platform, time.Since(start))
				}
			}
		}
		apperrors.WriteJSON(w, err)
		return
	}

	platform = req.Device.Platform

	// Generate device ID if not provided
	if req.Device.ID == "" {
		req.Device.ID = "dev-" + httputil.GenerateRandomID(8)
	}

	// Determine scopes based on device platform
	scopes := auth.DetermineScopes(req.Device.Platform)

	// Sign JWT
	token, err := h.authManager.SignJWT(req.Device.ID, scopes, 12*time.Hour)
	if err != nil {
		if h.metrics != nil {
			h.metrics.RecordAuthPairing("error_token_generation", platform, time.Since(start))
		}
		apperrors.WriteJSON(w, apperrors.Wrap(err, "TOKEN_GENERATION_FAILED", "Failed to generate access token", http.StatusInternalServerError))
		return
	}

	// Disconnect any existing WebSocket connections for this device ID
	// This handles the case where a device re-pairs with an expired JWT but
	// still has a stale WebSocket connection from the previous session
	if h.wsDisconnecter != nil {
		if disconnected := h.wsDisconnecter.DisconnectDevice(req.Device.ID); disconnected > 0 {
			authlog.Info("Disconnected stale WebSocket connections during re-pair",
				"device_id", req.Device.ID,
				"connections_closed", disconnected)
		}
	}

	// Save device to storage if available
	if h.storage != nil {
		deviceName := req.Device.Model
		if deviceName == "" {
			deviceName = req.Device.Platform + " Device"
		}

		// Extract platform version (if provided)
		platformVersion := ""
		// Note: req.Device doesn't currently have PlatformVersion in the struct
		// Add it to DeviceInfo if clients send it in the future

		if err := h.storage.SaveDevice(req.Device.ID, deviceName, req.Device.Platform, platformVersion, scopes); err != nil {
			authlog.Warn("Failed to save device to storage", "device_id", req.Device.ID, "error", err)
			// Don't fail the pairing, just log the error
		}
	}

	// Record successful pairing (consumes the token for short-lived tokens)
	h.authManager.RecordSuccessfulPairing(bootstrap)

	// Consume the pairing code so it can't be re-redeemed
	if h.pairingManager != nil {
		h.pairingManager.ConsumeByBootstrapToken(bootstrap)
	}

	// Record success metric
	if h.metrics != nil {
		h.metrics.RecordAuthPairing("success", platform, time.Since(start))
	}

	// Publish device.paired event for SSE
	deviceName := req.Device.Model
	if deviceName == "" {
		deviceName = req.Device.Platform + " Device"
	}
	if h.events != nil {
		h.events.PublishDevicePaired(req.Device.ID, deviceName, platform)
	}

	// Add to activity feed
	if h.activity != nil {
		h.activity.Add(types.ActivityEvent{
			ID:        "device_paired_" + req.Device.ID,
			Type:      "device_paired",
			Icon:      "Smartphone",
			IconClass: "success",
			Message:   "Device paired: " + deviceName,
			Timestamp: time.Now().UnixMilli(),
		})
	}

	// Return response
	if err := httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"accessToken": token,
		"expiresIn":   int((12 * time.Hour).Seconds()),
		"scopes":      scopes,
		"deviceId":    req.Device.ID,
	}); err != nil {
		authlog.Error("Error encoding JSON response", "error", err)
	}
}

// HandleRefresh issues a new JWT token
func (h *AuthHandler) HandleRefresh(w http.ResponseWriter, r *http.Request) {
	// Extract existing token
	oldToken := httputil.ExtractBearerToken(r)
	if oldToken == "" {
		if h.metrics != nil {
			h.metrics.RecordAuthRefresh("error_no_token")
		}
		apperrors.WriteJSON(w, apperrors.ErrUnauthorized)
		return
	}

	// Parse and validate old token
	_, claims, err := h.authManager.ParseJWT(oldToken)
	if err != nil {
		if h.metrics != nil {
			h.metrics.RecordAuthRefresh("error_invalid_token")
		}
		apperrors.WriteJSON(w, err)
		return
	}

	// Extract device ID from claims
	deviceID, ok := claims["sub"].(string)
	if !ok || deviceID == "" {
		if h.metrics != nil {
			h.metrics.RecordAuthRefresh("error_invalid_claims")
		}
		apperrors.WriteJSON(w, apperrors.New("INVALID_CLAIMS", "Token missing device ID", http.StatusUnauthorized))
		return
	}

	// Extract existing scopes
	scopes := auth.ExtractScopes(claims)

	// Issue new token with same device ID and scopes
	newToken, err := h.authManager.SignJWT(deviceID, scopes, 12*time.Hour)
	if err != nil {
		if h.metrics != nil {
			h.metrics.RecordAuthRefresh("error_token_generation")
		}
		apperrors.WriteJSON(w, apperrors.Wrap(err, "TOKEN_GENERATION_FAILED", "Failed to generate access token", http.StatusInternalServerError))
		return
	}

	// Record success metric
	if h.metrics != nil {
		h.metrics.RecordAuthRefresh("success")
	}

	// Return response
	if err := httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"accessToken": newToken,
		"expiresIn":   int((12 * time.Hour).Seconds()),
		"scopes":      scopes,
		"deviceId":    deviceID,
	}); err != nil {
		authlog.Error("Error encoding JSON response", "error", err)
	}
}

// LoginRequest represents a login request payload
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// LoginResponse represents a successful login response
type LoginResponse struct {
	Token string      `json:"token"`
	User  interface{} `json:"user"`
}

// UserResponse represents a user object in API responses
type UserResponse struct {
	ID        int        `json:"id"`
	Username  string     `json:"username"`
	CreatedAt time.Time  `json:"createdAt"`
	LastLogin *time.Time `json:"lastLogin,omitempty"`
	IsActive  bool       `json:"isActive"`
}

// HandleLogin handles POST /api/v1/auth/login
func (h *AuthHandler) HandleLogin(w http.ResponseWriter, r *http.Request) {
	clientIP := httputil.ExtractClientIP(r)
	authlog.Info("login attempt",
		"client_ip", clientIP,
		"tls", r.TLS != nil,
		"proto", r.Proto,
		"host", r.Host)

	if r.Method != http.MethodPost {
		authlog.Warn("login rejected: wrong method", "method", r.Method, "client_ip", clientIP)
		apperrors.WriteJSON(w, apperrors.New("METHOD_NOT_ALLOWED", "Method not allowed", http.StatusMethodNotAllowed))
		return
	}

	// Parse request body
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		authlog.Warn("login rejected: invalid request body", "error", err, "client_ip", clientIP)
		apperrors.WriteJSON(w, apperrors.New(
			"INVALID_REQUEST",
			"Invalid request body",
			http.StatusBadRequest,
		))
		return
	}

	authlog.Debug("login request parsed", "username", req.Username, "client_ip", clientIP)

	// Validate required fields
	if req.Username == "" || req.Password == "" {
		authlog.Warn("login rejected: missing fields",
			"has_username", req.Username != "",
			"has_password", req.Password != "",
			"client_ip", clientIP)
		apperrors.WriteJSON(w, apperrors.New(
			"MISSING_FIELDS",
			"Username and password are required",
			http.StatusBadRequest,
		))
		return
	}

	// Validate credentials
	user, err := h.storage.ValidateCredentials(req.Username, req.Password)
	if err != nil {
		authlog.Warn("login rejected: invalid credentials",
			"username", req.Username,
			"error", err.Error(),
			"client_ip", clientIP)
		apperrors.WriteJSON(w, apperrors.New(
			"INVALID_CREDENTIALS",
			"Invalid username or password",
			http.StatusUnauthorized,
		))
		return
	}

	// Generate JWT token (12 hour expiry)
	// Use "user-{username}" as device ID for web sessions
	deviceID := "user-" + user.Username
	scopes := auth.DetermineScopes("web")
	token, err := h.authManager.SignJWT(deviceID, scopes, 12*time.Hour)
	if err != nil {
		authlog.Error("login failed: JWT generation error",
			"username", req.Username,
			"error", err,
			"client_ip", clientIP)
		apperrors.WriteJSON(w, apperrors.New(
			"TOKEN_GENERATION_FAILED",
			"Failed to generate authentication token",
			http.StatusInternalServerError,
		))
		return
	}

	authlog.Info("login successful",
		"username", user.Username,
		"user_id", user.ID,
		"client_ip", clientIP,
		"tls", r.TLS != nil)

	// Update last_login timestamp asynchronously
	go func(userID int) {
		if err := h.storage.UpdateLastLogin(userID); err != nil {
			authlog.Warn("Failed to update last login for user", "user_id", userID, "error", err)
		}
	}(user.ID)

	// Return token and user info (without password hash)
	response := LoginResponse{
		Token: token,
		User: UserResponse{
			ID:        user.ID,
			Username:  user.Username,
			CreatedAt: user.CreatedAt,
			LastLogin: user.LastLogin,
			IsActive:  user.IsActive,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// HandleAuthMe handles GET /api/v1/auth/me
func (h *AuthHandler) HandleAuthMe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		apperrors.WriteJSON(w, apperrors.New("METHOD_NOT_ALLOWED", "Method not allowed", http.StatusMethodNotAllowed))
		return
	}

	// Extract and validate JWT token
	tokenString := httputil.ExtractBearerToken(r)
	if tokenString == "" {
		apperrors.WriteJSON(w, apperrors.New(
			"MISSING_TOKEN",
			"Authorization token is required",
			http.StatusUnauthorized,
		))
		return
	}

	// Parse and validate JWT
	_, claims, err := h.authManager.ParseJWT(tokenString)
	if err != nil {
		apperrors.WriteJSON(w, apperrors.New(
			"INVALID_TOKEN",
			"Invalid or expired token",
			http.StatusUnauthorized,
		))
		return
	}

	// Extract device ID (which is "user-{username}" for web sessions)
	deviceID, ok := claims["sub"].(string)
	if !ok || deviceID == "" {
		apperrors.WriteJSON(w, apperrors.New(
			"INVALID_TOKEN",
			"Invalid token claims",
			http.StatusUnauthorized,
		))
		return
	}

	// Extract username from device ID
	username := strings.TrimPrefix(deviceID, "user-")
	if username == deviceID {
		// Not a user session token
		apperrors.WriteJSON(w, apperrors.New(
			"INVALID_TOKEN",
			"Not a user session token",
			http.StatusUnauthorized,
		))
		return
	}

	// Get user from database
	user, err := h.storage.GetUserByUsername(username)
	if err != nil {
		authlog.Error("Error getting user", "error", err)
		apperrors.WriteJSON(w, apperrors.New(
			"USER_NOT_FOUND",
			"User not found",
			http.StatusUnauthorized,
		))
		return
	}

	if user == nil {
		apperrors.WriteJSON(w, apperrors.New(
			"USER_NOT_FOUND",
			"User not found",
			http.StatusUnauthorized,
		))
		return
	}

	// Check if user is still active
	if !user.IsActive {
		apperrors.WriteJSON(w, apperrors.New(
			"USER_DISABLED",
			"User account is disabled",
			http.StatusUnauthorized,
		))
		return
	}

	// Return user info (without password hash)
	response := map[string]interface{}{
		"user": UserResponse{
			ID:        user.ID,
			Username:  user.Username,
			CreatedAt: user.CreatedAt,
			LastLogin: user.LastLogin,
			IsActive:  user.IsActive,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// HandleLogout handles POST /api/v1/auth/logout
func (h *AuthHandler) HandleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		apperrors.WriteJSON(w, apperrors.New("METHOD_NOT_ALLOWED", "Method not allowed", http.StatusMethodNotAllowed))
		return
	}

	// Extract and validate JWT token
	tokenString := httputil.ExtractBearerToken(r)
	if tokenString == "" {
		apperrors.WriteJSON(w, apperrors.New(
			"MISSING_TOKEN",
			"Authorization token is required",
			http.StatusUnauthorized,
		))
		return
	}

	// Validate token
	_, _, err := h.authManager.ParseJWT(tokenString)
	if err != nil {
		apperrors.WriteJSON(w, apperrors.New(
			"INVALID_TOKEN",
			"Invalid or expired token",
			http.StatusUnauthorized,
		))
		return
	}

	// For now, logout is client-side only (client discards token)
	// In the future, we could add token revocation list here

	response := map[string]string{
		"message": "Logged out successfully",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// HandleSetupStatus handles GET /api/v1/auth/setup-status
// Returns whether initial setup is required (no users exist)
func (h *AuthHandler) HandleSetupStatus(w http.ResponseWriter, r *http.Request) {
	clientIP := httputil.ExtractClientIP(r)
	authlog.Info("setup-status check",
		"client_ip", clientIP,
		"tls", r.TLS != nil,
		"proto", r.Proto,
		"host", r.Host)

	if r.Method != http.MethodGet {
		apperrors.WriteJSON(w, apperrors.New("METHOD_NOT_ALLOWED", "Method not allowed", http.StatusMethodNotAllowed))
		return
	}

	// Check if any users exist
	users, err := h.storage.ListUsers()
	if err != nil {
		authlog.Error("setup-status check failed: database error",
			"error", err,
			"client_ip", clientIP)
		apperrors.WriteJSON(w, apperrors.New(
			"DATABASE_ERROR",
			"Failed to check setup status",
			http.StatusInternalServerError,
		))
		return
	}

	setupRequired := len(users) == 0
	authlog.Info("setup-status result",
		"setup_required", setupRequired,
		"user_count", len(users),
		"client_ip", clientIP,
		"tls", r.TLS != nil)

	response := map[string]interface{}{
		"setupRequired": setupRequired,
		"hasUsers":      len(users) > 0,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// SetupRequest represents an initial setup request payload
type SetupRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// HandleSetup handles POST /api/v1/auth/setup
// Creates the first admin user (only works if no users exist)
func (h *AuthHandler) HandleSetup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		apperrors.WriteJSON(w, apperrors.New("METHOD_NOT_ALLOWED", "Method not allowed", http.StatusMethodNotAllowed))
		return
	}

	// Check if any users already exist
	users, err := h.storage.ListUsers()
	if err != nil {
		authlog.Error("Error checking users", "error", err)
		apperrors.WriteJSON(w, apperrors.New(
			"DATABASE_ERROR",
			"Failed to check setup status",
			http.StatusInternalServerError,
		))
		return
	}

	// Only allow setup if no users exist
	if len(users) > 0 {
		apperrors.WriteJSON(w, apperrors.New(
			"SETUP_ALREADY_COMPLETED",
			"Setup has already been completed. Please use the login page.",
			http.StatusBadRequest,
		))
		return
	}

	// Parse request body
	var req SetupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apperrors.WriteJSON(w, apperrors.New(
			"INVALID_REQUEST",
			"Invalid request body",
			http.StatusBadRequest,
		))
		return
	}

	// Validate required fields
	if req.Username == "" || req.Password == "" {
		apperrors.WriteJSON(w, apperrors.New(
			"MISSING_FIELDS",
			"Username and password are required",
			http.StatusBadRequest,
		))
		return
	}

	// Create the first user
	if err := h.storage.CreateUser(req.Username, req.Password); err != nil {
		authlog.Error("Error creating user", "error", err)
		apperrors.WriteJSON(w, apperrors.New(
			"USER_CREATION_FAILED",
			err.Error(),
			http.StatusBadRequest,
		))
		return
	}

	authlog.Info("Initial setup completed", "username", req.Username)

	// Return success response (don't auto-login, require explicit login)
	response := map[string]interface{}{
		"message":  "Setup completed successfully. Please log in.",
		"username": req.Username,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(response)
}

// HandleQRCode generates a QR code for mobile app pairing
func (h *AuthHandler) HandleQRCode(w http.ResponseWriter, r *http.Request) {
	// Rate limit check
	clientIP := httputil.ExtractClientIP(r)
	if h.qrLimiter != nil && !h.qrLimiter.Allow(clientIP) {
		apperrors.WriteJSON(w, apperrors.ErrRateLimitExceeded)
		return
	}

	// Generate short-lived bootstrap token
	bootstrapToken, err := h.authManager.GenerateShortLivedToken(5 * time.Minute)
	if err != nil {
		apperrors.WriteJSON(w, apperrors.Wrap(err, "TOKEN_GENERATION_FAILED", "Failed to generate pairing token", http.StatusInternalServerError))
		return
	}

	// Calculate SPKI for TLS pinning
	spki, err := h.calculateSPKI()
	if err != nil {
		authlog.Warn("Failed to calculate SPKI", "error", err)
		spki = ""
	}

	// SECURITY: SPKI is MANDATORY for HTTPS connections
	// Without SPKI pinning, MITM attacks are possible
	if strings.HasPrefix(h.baseURL, "https://") && spki == "" {
		authlog.Error("SPKI required for HTTPS but no certificate available",
			"baseURL", h.baseURL)
		apperrors.WriteJSON(w, apperrors.NewWithCode(
			"SPKI_REQUIRED",
			apperrors.CodeBadGateway,
			"Certificate pinning (SPKI) is required for HTTPS but no certificate is available",
			http.StatusInternalServerError,
		))
		return
	}

	// Determine hostname
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "Nexus"
	}

	// Calculate backup SPKI for pin rotation (if available)
	spkiBackup := h.calculateBackupSPKI()

	// Build QR payload
	payload := map[string]interface{}{
		"name":           "Nekzus @ " + hostname,
		"baseUrl":        h.baseURL,
		"spki":           spki,
		"spkiBackup":     spkiBackup,
		"bootstrapToken": bootstrapToken,
		"capabilities":   h.capabilities,
		"nekzusId":       h.nekzusID,
	}

	// Check if client wants QR code image
	if r.URL.Query().Get("format") == "png" {
		payloadJSON, _ := json.Marshal(payload)
		qrCode, err := qrcode.Encode(string(payloadJSON), qrcode.Medium, 256)
		if err != nil {
			apperrors.WriteJSON(w, apperrors.Wrap(err, "QR_CODE_GENERATION_FAILED", "Failed to generate QR code image", http.StatusInternalServerError))
			return
		}

		w.Header().Set("Content-Type", "image/png")
		w.Write(qrCode)
		return
	}

	// Return JSON payload
	if err := httputil.WriteJSON(w, http.StatusOK, payload); err != nil {
		authlog.Error("Error encoding JSON response", "error", err)
	}
}

// HandlePairWebUI serves a web page with QR code for pairing
func (h *AuthHandler) HandlePairWebUI(w http.ResponseWriter, r *http.Request) {
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "Nexus"
	}

	html := `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Nekzus - Mobile Pairing</title>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Oxygen, Ubuntu, Cantarell, sans-serif;
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            min-height: 100vh;
            display: flex;
            align-items: center;
            justify-content: center;
            padding: 20px;
        }
        .container {
            background: white;
            border-radius: 20px;
            box-shadow: 0 20px 60px rgba(0,0,0,0.3);
            max-width: 500px;
            width: 100%;
            padding: 40px;
            text-align: center;
        }
        h1 {
            color: #333;
            margin-bottom: 10px;
            font-size: 28px;
        }
        .subtitle {
            color: #666;
            margin-bottom: 30px;
            font-size: 16px;
        }
        .qr-container {
            background: #f8f9fa;
            border-radius: 15px;
            padding: 30px;
            margin: 30px 0;
        }
        .qr-code {
            width: 100%;
            max-width: 300px;
            height: auto;
            margin: 0 auto;
            display: block;
        }
        .info {
            background: #e3f2fd;
            border-left: 4px solid #2196f3;
            padding: 15px;
            margin: 20px 0;
            text-align: left;
            border-radius: 5px;
        }
        .info strong {
            color: #1976d2;
        }
        .info p {
            margin: 8px 0;
            color: #555;
            font-size: 14px;
        }
        .steps {
            text-align: left;
            margin-top: 30px;
        }
        .steps h3 {
            color: #333;
            margin-bottom: 15px;
            font-size: 18px;
        }
        .steps ol {
            padding-left: 25px;
        }
        .steps li {
            margin: 10px 0;
            color: #555;
            line-height: 1.6;
        }
        .warning {
            background: #fff3cd;
            border-left: 4px solid #ffc107;
            padding: 15px;
            margin: 20px 0;
            border-radius: 5px;
        }
        .warning p {
            color: #856404;
            font-size: 14px;
            margin: 0;
        }
        .footer {
            margin-top: 30px;
            padding-top: 20px;
            border-top: 1px solid #eee;
            color: #999;
            font-size: 12px;
        }
        @media (max-width: 600px) {
            .container { padding: 30px 20px; }
            h1 { font-size: 24px; }
        }
    </style>
</head>
<body>
    <div class="container">
        <h1>🔐 Nekzus</h1>
        <p class="subtitle">Mobile App Pairing</p>

        <div class="qr-container">
            <img src="/api/v1/auth/qr?format=png" alt="Pairing QR Code" class="qr-code">
        </div>

        <div class="info">
            <p><strong>Instance:</strong> ` + hostname + `</p>
            <p><strong>Nekzus ID:</strong> ` + h.nekzusID + `</p>
            <p><strong>Base URL:</strong> ` + h.baseURL + `</p>
        </div>

        <div class="warning">
            <p>⏱️ This QR code expires in <strong>5 minutes</strong>. Refresh the page to generate a new one.</p>
        </div>

        <div class="steps">
            <h3>How to Pair:</h3>
            <ol>
                <li>Open the <strong>Nekzus Mobile App</strong> on your phone</li>
                <li>Tap <strong>"Add Nexus"</strong> or the <strong>+</strong> button</li>
                <li>Select <strong>"Scan QR Code"</strong></li>
                <li>Point your camera at the QR code above</li>
                <li>The app will automatically pair and connect!</li>
            </ol>
        </div>

        <div class="footer">
            <p>Nekzus v` + h.version + ` • Secure Local Gateway</p>
        </div>
    </div>
</body>
</html>`

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(html))
}

// calculateSPKI calculates the SHA-256 hash of the certificate's SPKI.
// It tries sources in order: file path -> managed certs -> fallback cert.
func (h *AuthHandler) calculateSPKI() (string, error) {
	// Try file-based cert first (if path is configured)
	if h.tlsCertPath != "" {
		spki, err := calculateSPKIFromFile(h.tlsCertPath)
		if err == nil {
			return spki, nil
		}
		authlog.Warn("Failed to calculate SPKI from file, trying managed certs",
			"path", h.tlsCertPath, "error", err)
	}

	// Try managed certificates
	if h.certManager != nil {
		// Try first available (prefers localhost)
		if cert := h.certManager.GetFirstAvailableCertificate(); cert != nil {
			spki, err := calculateSPKIFromTLSCert(cert)
			if err == nil {
				return spki, nil
			}
			authlog.Warn("Failed to calculate SPKI from managed cert", "error", err)
		}

		// Try fallback cert
		if cert := h.certManager.GetFallbackCertificate(); cert != nil {
			spki, err := calculateSPKIFromTLSCert(cert)
			if err == nil {
				return spki, nil
			}
			authlog.Warn("Failed to calculate SPKI from fallback cert", "error", err)
		}
	}

	return "", nil // No cert available, return empty (not an error)
}

// calculateBackupSPKI calculates the SPKI hash for the backup certificate.
// This enables pin rotation: clients can accept either the primary or backup SPKI.
func (h *AuthHandler) calculateBackupSPKI() string {
	// Check if cert manager supports backup certificates
	backupManager, ok := h.certManager.(CertificateManagerWithBackup)
	if !ok || backupManager == nil {
		return ""
	}

	cert := backupManager.GetBackupCertificate()
	if cert == nil {
		return ""
	}

	spki, err := calculateSPKIFromTLSCert(cert)
	if err != nil {
		authlog.Warn("Failed to calculate backup SPKI", "error", err)
		return ""
	}

	return spki
}

// calculateSPKIFromFile reads cert from file and calculates SPKI
func calculateSPKIFromFile(certPath string) (string, error) {
	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		return "", fmt.Errorf("failed to read cert: %w", err)
	}

	block, _ := pem.Decode(certPEM)
	if block == nil {
		return "", fmt.Errorf("failed to decode PEM block")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return "", fmt.Errorf("failed to parse certificate: %w", err)
	}

	return calculateSPKIFromX509Cert(cert)
}

// calculateSPKIFromTLSCert extracts SPKI from tls.Certificate
func calculateSPKIFromTLSCert(tlsCert *tls.Certificate) (string, error) {
	if len(tlsCert.Certificate) == 0 {
		return "", fmt.Errorf("no certificates in chain")
	}

	cert, err := x509.ParseCertificate(tlsCert.Certificate[0])
	if err != nil {
		return "", fmt.Errorf("failed to parse certificate: %w", err)
	}

	return calculateSPKIFromX509Cert(cert)
}

// calculateSPKIFromX509Cert computes SPKI hash from x509 certificate
func calculateSPKIFromX509Cert(cert *x509.Certificate) (string, error) {
	spkiDER, err := x509.MarshalPKIXPublicKey(cert.PublicKey)
	if err != nil {
		return "", fmt.Errorf("failed to marshal public key: %w", err)
	}

	hash := sha256.Sum256(spkiDER)
	spki := "sha256/" + base64.StdEncoding.EncodeToString(hash[:])

	return spki, nil
}

// HandleVerifyToken verifies a JWT token and returns its validity status.
// GET /api/v1/auth/verify
// This allows clients to check token validity without making a functional API call.
func (h *AuthHandler) HandleVerifyToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		apperrors.WriteJSON(w, apperrors.New("METHOD_NOT_ALLOWED", "Method not allowed", http.StatusMethodNotAllowed))
		return
	}

	// Extract token from Authorization header
	tokenString := httputil.ExtractBearerToken(r)
	if tokenString == "" {
		apperrors.WriteJSON(w, apperrors.NewWithCode(
			"AUTH_REQUIRED",
			apperrors.CodeAuthRequired,
			"Authorization token is required",
			http.StatusUnauthorized,
		))
		return
	}

	// Parse and validate the JWT
	_, claims, err := h.authManager.ParseJWT(tokenString)
	if err != nil {
		// Return the error directly - it already has structured error codes
		apperrors.WriteJSON(w, err)
		return
	}

	// Extract device ID from claims
	deviceID, _ := claims["sub"].(string)

	// Extract expiration time
	var expiresAt string
	if exp, ok := claims["exp"].(float64); ok {
		expiresAt = time.Unix(int64(exp), 0).UTC().Format(time.RFC3339)
	}

	// Return valid token response
	if err := httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"valid":     true,
		"expiresAt": expiresAt,
		"deviceId":  deviceID,
	}); err != nil {
		authlog.Error("Error encoding JSON response", "error", err)
	}
}
