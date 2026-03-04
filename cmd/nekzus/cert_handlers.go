package main

import (
	"encoding/json"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/nstalgic/nekzus/internal/certmanager"
	apperrors "github.com/nstalgic/nekzus/internal/errors"
	"github.com/nstalgic/nekzus/internal/types"
)

// handleGenerateCertificate generates a new certificate
func (app *Application) handleGenerateCertificate(w http.ResponseWriter, r *http.Request) {
	// Require JWT authentication
	_, claims, err := app.services.Auth.ParseJWT(extractToken(r))
	if err != nil || claims == nil {
		apperrors.WriteJSON(w, apperrors.New(
			"UNAUTHORIZED",
			"Valid JWT token required",
			http.StatusUnauthorized,
		))
		return
	}

	// Parse request
	var req struct {
		Domains  []string `json:"domains"`
		Provider string   `json:"provider"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apperrors.WriteJSON(w, apperrors.New(
			"INVALID_REQUEST",
			"Invalid JSON",
			http.StatusBadRequest,
		))
		return
	}

	// Validate input
	if len(req.Domains) == 0 {
		apperrors.WriteJSON(w, apperrors.New(
			"INVALID_DOMAINS",
			"At least one domain required",
			http.StatusBadRequest,
		))
		return
	}

	// Validate domains (block public domains by default, allow only local network)
	validator := certmanager.NewDomainValidator(false, nil)
	if err := validator.Validate(req.Domains); err != nil {
		apperrors.WriteJSON(w, apperrors.New(
			"INVALID_DOMAIN",
			err.Error(),
			http.StatusBadRequest,
		))
		return
	}

	// Generate certificate
	cert, err := app.services.Certs.Generate(certmanager.GenerateRequest{
		Domains:     req.Domains,
		Provider:    req.Provider,
		RequestedBy: claims["sub"].(string),
	})

	if err != nil {
		apperrors.WriteJSON(w, apperrors.New(
			"CERT_GENERATION_FAILED",
			err.Error(),
			http.StatusBadRequest,
		))
		return
	}

	// Check if we should trigger TLS upgrade (hot-reload)
	tlsUpgraded := false
	if !app.IsTLSEnabled() {
		if err := app.UpgradeToTLS(); err != nil {
			// Log the error but don't fail the request - cert was still generated
			log.Warn("TLS upgrade failed", "error", err)
		} else {
			tlsUpgraded = true

			// Notify all devices (including offline) that they need to re-pair
			// Devices paired before TLS was enabled don't have SPKI for cert pinning
			if app.notificationService != nil {
				// Calculate the new HTTPS base URL (upgrade happens async, so baseURL isn't updated yet)
				newBaseURL := strings.Replace(app.baseURL, "http://", "https://", 1)
				if err := app.notificationService.SendToAll(types.WSMsgTypeRepairRequired, map[string]interface{}{
					"reason":     "tls_enabled",
					"newBaseUrl": newBaseURL,
					"message":    "Server TLS configuration has changed. Please re-pair your device to establish a secure connection with certificate pinning.",
					"timestamp":  time.Now().Unix(),
				}); err != nil {
					log.Warn("failed to queue repair_required notifications", "error", err)
				}
			}
		}
	}

	// Return success response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"certificate": map[string]interface{}{
			"domain":      cert.Domain,
			"issuer":      cert.Metadata.Issuer,
			"not_before":  cert.Metadata.NotBefore,
			"not_after":   cert.Metadata.NotAfter,
			"sans":        cert.Metadata.SANs,
			"fingerprint": cert.Metadata.Fingerprint,
		},
		"tls_upgraded": tlsUpgraded,
	})
}

// handleListCertificates lists all certificates
func (app *Application) handleListCertificates(w http.ResponseWriter, r *http.Request) {
	// Require JWT authentication
	_, claims, err := app.services.Auth.ParseJWT(extractToken(r))
	if err != nil || claims == nil {
		apperrors.WriteJSON(w, apperrors.New(
			"UNAUTHORIZED",
			"Valid JWT token required",
			http.StatusUnauthorized,
		))
		return
	}

	// Get all certificates
	certs := app.services.Certs.List()

	// Build response
	certList := make([]map[string]interface{}, len(certs))
	for i, cert := range certs {
		certList[i] = map[string]interface{}{
			"domain":          cert.Domain,
			"issuer":          cert.Metadata.Issuer,
			"not_before":      cert.Metadata.NotBefore,
			"not_after":       cert.Metadata.NotAfter,
			"sans":            cert.Metadata.SANs,
			"fingerprint":     cert.Metadata.Fingerprint,
			"expires_in_days": cert.DaysUntilExpiry(),
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"certificates": certList,
		"count":        len(certList),
	})
}

// handleGetCertificate gets a specific certificate by domain
func (app *Application) handleGetCertificate(w http.ResponseWriter, r *http.Request) {
	// Require JWT authentication
	_, claims, err := app.services.Auth.ParseJWT(extractToken(r))
	if err != nil || claims == nil {
		apperrors.WriteJSON(w, apperrors.New(
			"UNAUTHORIZED",
			"Valid JWT token required",
			http.StatusUnauthorized,
		))
		return
	}

	// Extract domain from URL path
	domain := strings.TrimPrefix(r.URL.Path, "/api/certificates/")
	if domain == "" {
		apperrors.WriteJSON(w, apperrors.New(
			"INVALID_DOMAIN",
			"Domain parameter required",
			http.StatusBadRequest,
		))
		return
	}

	// Get certificate
	cert, err := app.services.Certs.Get(domain)
	if err != nil {
		apperrors.WriteJSON(w, apperrors.New(
			"CERT_NOT_FOUND",
			"Certificate not found",
			http.StatusNotFound,
		))
		return
	}

	// Return certificate details
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"certificate": map[string]interface{}{
			"domain":          cert.Domain,
			"issuer":          cert.Metadata.Issuer,
			"not_before":      cert.Metadata.NotBefore,
			"not_after":       cert.Metadata.NotAfter,
			"sans":            cert.Metadata.SANs,
			"fingerprint":     cert.Metadata.Fingerprint,
			"expires_in_days": cert.DaysUntilExpiry(),
		},
	})
}

// handleDeleteCertificate deletes a certificate
func (app *Application) handleDeleteCertificate(w http.ResponseWriter, r *http.Request) {
	// Require JWT authentication
	_, claims, err := app.services.Auth.ParseJWT(extractToken(r))
	if err != nil || claims == nil {
		apperrors.WriteJSON(w, apperrors.New(
			"UNAUTHORIZED",
			"Valid JWT token required",
			http.StatusUnauthorized,
		))
		return
	}

	// Extract domain from URL path
	domain := strings.TrimPrefix(r.URL.Path, "/api/certificates/")
	if domain == "" {
		apperrors.WriteJSON(w, apperrors.New(
			"INVALID_DOMAIN",
			"Domain parameter required",
			http.StatusBadRequest,
		))
		return
	}

	// Check if this is the active certificate
	if app.services.Certs.IsActiveCertificate(domain) {
		// Check if there are other certificates available
		certs := app.services.Certs.List()
		if len(certs) <= 1 {
			apperrors.WriteJSON(w, apperrors.New(
				"CERT_IN_USE",
				"Cannot delete the only active TLS certificate. Generate or import a new certificate first, or use the renew endpoint to replace it.",
				http.StatusConflict,
			))
			return
		}
	}

	// Get the certificate info before deletion for notification
	cert, _ := app.services.Certs.Get(domain)
	var oldSPKI string
	if cert != nil {
		oldSPKI = cert.Metadata.Fingerprint
	}

	// Delete certificate
	err = app.services.Certs.Delete(domain)
	if err != nil {
		apperrors.WriteJSON(w, apperrors.New(
			"CERT_DELETE_FAILED",
			err.Error(),
			http.StatusInternalServerError,
		))
		return
	}

	// Notify devices that the certificate was deleted
	if app.notificationService != nil && oldSPKI != "" {
		if err := app.notificationService.SendToAll(types.WSMsgTypeRepairRequired, map[string]interface{}{
			"reason":    "certificate_deleted",
			"domain":    domain,
			"oldSpki":   oldSPKI,
			"message":   "A certificate was deleted. Re-pair if you experience connection issues.",
			"timestamp": time.Now().Unix(),
		}); err != nil {
			log.Warn("failed to send certificate deletion notification", "error", err)
		}
	}

	// Return success with warning
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Certificate deleted successfully",
		"warning": "Devices pinned to this certificate may need to re-pair",
	})
}

// handleRenewCertificate renews a Nexus-generated certificate
// POST /api/v1/certificates/renew
func (app *Application) handleRenewCertificate(w http.ResponseWriter, r *http.Request) {
	// Require JWT authentication
	_, claims, err := app.services.Auth.ParseJWT(extractToken(r))
	if err != nil || claims == nil {
		apperrors.WriteJSON(w, apperrors.New(
			"UNAUTHORIZED",
			"Valid JWT token required",
			http.StatusUnauthorized,
		))
		return
	}

	// Parse request
	var req struct {
		Domain string `json:"domain"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apperrors.WriteJSON(w, apperrors.New(
			"INVALID_REQUEST",
			"Invalid JSON",
			http.StatusBadRequest,
		))
		return
	}

	if req.Domain == "" {
		apperrors.WriteJSON(w, apperrors.New(
			"INVALID_DOMAIN",
			"Domain parameter required",
			http.StatusBadRequest,
		))
		return
	}

	// Renew certificate
	result, err := app.services.Certs.Renew(certmanager.RenewRequest{
		Domain:      req.Domain,
		RequestedBy: claims["sub"].(string),
	})
	if err != nil {
		apperrors.WriteJSON(w, apperrors.New(
			"CERT_RENEWAL_FAILED",
			err.Error(),
			http.StatusBadRequest,
		))
		return
	}

	// Notify devices about the certificate rotation
	devicesNotified := 0
	if app.notificationService != nil && result.SPKIChanged {
		if err := app.notificationService.SendToAll(types.WSMsgTypeRepairRequired, map[string]interface{}{
			"reason":    "certificate_rotated",
			"domain":    req.Domain,
			"oldSpki":   result.OldSPKI,
			"newSpki":   result.NewSPKI,
			"message":   "Certificate has been renewed. Please re-pair your device to update certificate pinning.",
			"timestamp": time.Now().Unix(),
		}); err != nil {
			log.Warn("failed to send certificate rotation notification", "error", err)
		} else {
			// Approximate count - in production you'd track this
			devicesNotified = -1 // -1 indicates "all devices"
		}
	}

	// Return success response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"certificate": map[string]interface{}{
			"domain":      result.NewCert.Domain,
			"issuer":      result.NewCert.Metadata.Issuer,
			"not_before":  result.NewCert.Metadata.NotBefore,
			"not_after":   result.NewCert.Metadata.NotAfter,
			"sans":        result.NewCert.Metadata.SANs,
			"fingerprint": result.NewCert.Metadata.Fingerprint,
		},
		"rotation": map[string]interface{}{
			"old_spki":      result.OldSPKI,
			"new_spki":      result.NewSPKI,
			"spki_changed":  result.SPKIChanged,
			"devices_notified": devicesNotified,
		},
	})
}

// handleReplaceCertificate replaces a certificate with a user-provided one
// POST /api/v1/certificates/replace
func (app *Application) handleReplaceCertificate(w http.ResponseWriter, r *http.Request) {
	// Require JWT authentication
	_, claims, err := app.services.Auth.ParseJWT(extractToken(r))
	if err != nil || claims == nil {
		apperrors.WriteJSON(w, apperrors.New(
			"UNAUTHORIZED",
			"Valid JWT token required",
			http.StatusUnauthorized,
		))
		return
	}

	// Parse request
	var req struct {
		Domain      string `json:"domain"`
		Certificate string `json:"certificate"` // PEM-encoded certificate
		PrivateKey  string `json:"privateKey"`  // PEM-encoded private key
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apperrors.WriteJSON(w, apperrors.New(
			"INVALID_REQUEST",
			"Invalid JSON",
			http.StatusBadRequest,
		))
		return
	}

	// Validate input
	if req.Domain == "" {
		apperrors.WriteJSON(w, apperrors.New(
			"INVALID_DOMAIN",
			"Domain parameter required",
			http.StatusBadRequest,
		))
		return
	}

	if req.Certificate == "" {
		apperrors.WriteJSON(w, apperrors.New(
			"INVALID_CERTIFICATE",
			"Certificate PEM required",
			http.StatusBadRequest,
		))
		return
	}

	if req.PrivateKey == "" {
		apperrors.WriteJSON(w, apperrors.New(
			"INVALID_KEY",
			"Private key PEM required",
			http.StatusBadRequest,
		))
		return
	}

	// Replace certificate
	result, err := app.services.Certs.Replace(certmanager.ReplaceRequest{
		Domain:         req.Domain,
		CertificatePEM: []byte(req.Certificate),
		PrivateKeyPEM:  []byte(req.PrivateKey),
		RequestedBy:    claims["sub"].(string),
	})
	if err != nil {
		apperrors.WriteJSON(w, apperrors.New(
			"CERT_REPLACE_FAILED",
			err.Error(),
			http.StatusBadRequest,
		))
		return
	}

	// Check if we should trigger TLS upgrade (hot-reload)
	tlsUpgraded := false
	if !app.IsTLSEnabled() {
		if err := app.UpgradeToTLS(); err != nil {
			log.Warn("TLS upgrade failed after certificate replacement", "error", err)
		} else {
			tlsUpgraded = true
		}
	}

	// Notify devices about the certificate change
	devicesNotified := 0
	if app.notificationService != nil {
		reason := "certificate_replaced"
		message := "Certificate has been replaced. Please re-pair your device to update certificate pinning."

		if tlsUpgraded {
			reason = "tls_enabled"
			message = "TLS has been enabled with the new certificate. Please re-pair your device."
		}

		if result.SPKIChanged || tlsUpgraded {
			if err := app.notificationService.SendToAll(types.WSMsgTypeRepairRequired, map[string]interface{}{
				"reason":     reason,
				"domain":     req.Domain,
				"oldSpki":    result.OldSPKI,
				"newSpki":    result.NewSPKI,
				"newBaseUrl": app.baseURL,
				"message":    message,
				"timestamp":  time.Now().Unix(),
			}); err != nil {
				log.Warn("failed to send certificate replacement notification", "error", err)
			} else {
				devicesNotified = -1
			}
		}
	}

	// Return success response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"certificate": map[string]interface{}{
			"domain":      result.NewCert.Domain,
			"issuer":      result.NewCert.Metadata.Issuer,
			"not_before":  result.NewCert.Metadata.NotBefore,
			"not_after":   result.NewCert.Metadata.NotAfter,
			"sans":        result.NewCert.Metadata.SANs,
			"fingerprint": result.NewCert.Metadata.Fingerprint,
		},
		"rotation": map[string]interface{}{
			"old_spki":         result.OldSPKI,
			"new_spki":         result.NewSPKI,
			"spki_changed":     result.SPKIChanged,
			"devices_notified": devicesNotified,
			"was_new_import":   result.OldCert == nil,
		},
		"tls_upgraded": tlsUpgraded,
	})
}

// handleSuggestCertDomains suggests domains for certificate generation based on local network
func (app *Application) handleSuggestCertDomains(w http.ResponseWriter, r *http.Request) {
	// Require JWT authentication
	_, claims, err := app.services.Auth.ParseJWT(extractToken(r))
	if err != nil || claims == nil {
		apperrors.WriteJSON(w, apperrors.New(
			"UNAUTHORIZED",
			"Valid JWT token required",
			http.StatusUnauthorized,
		))
		return
	}

	// Collect suggested domains
	suggestions := getSuggestedDomains()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"suggestions": suggestions,
		"count":       len(suggestions),
	})
}

// getSuggestedDomains returns a list of suggested domains for certificate generation
func getSuggestedDomains() []string {
	domains := []string{"localhost"}

	// Add system hostname with .local suffix
	if hostname, err := os.Hostname(); err == nil && hostname != "" {
		// Only add hostname if it looks like a valid local domain
		// Skip container IDs (hex strings) and other non-domain hostnames
		if isValidLocalHostname(hostname) {
			domains = append(domains, hostname)
			// Add hostname.local for mDNS compatibility
			if !strings.HasSuffix(strings.ToLower(hostname), ".local") {
				domains = append(domains, hostname+".local")
			}
		}
	}

	// Add local IP addresses
	addrs, err := net.InterfaceAddrs()
	if err == nil {
		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
				// Only include IPv4 addresses for simplicity
				if ipnet.IP.To4() != nil {
					domains = append(domains, ipnet.IP.String())
				}
			}
		}
	}

	return domains
}

// isValidLocalHostname checks if a hostname is valid for certificate generation
// Filters out container IDs (hex strings) and other invalid hostnames
func isValidLocalHostname(hostname string) bool {
	// Skip empty hostnames
	if hostname == "" {
		return false
	}

	// If it ends with .local, .lan, .home, .internal, it's valid
	lower := strings.ToLower(hostname)
	localSuffixes := []string{".local", ".lan", ".home", ".internal"}
	for _, suffix := range localSuffixes {
		if strings.HasSuffix(lower, suffix) {
			return true
		}
	}

	// Check if it looks like a container ID (12+ hex characters)
	// Docker container IDs are 12 or 64 hex characters
	if len(hostname) >= 12 && isHexString(hostname) {
		return false
	}

	// Check if it contains at least one letter (not just numbers/hex)
	hasLetter := false
	for _, c := range hostname {
		if (c >= 'g' && c <= 'z') || (c >= 'G' && c <= 'Z') {
			hasLetter = true
			break
		}
	}

	// If it's all hex characters (a-f, 0-9), likely a container ID
	if !hasLetter && len(hostname) >= 8 {
		return false
	}

	return true
}

// isHexString checks if a string contains only hexadecimal characters
func isHexString(s string) bool {
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

// extractToken extracts JWT token from Authorization header
func extractToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return ""
	}

	// Bearer token format: "Bearer <token>"
	parts := strings.Split(auth, " ")
	if len(parts) != 2 || parts[0] != "Bearer" {
		return ""
	}

	return parts[1]
}
