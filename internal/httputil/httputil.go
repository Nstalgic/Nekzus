package httputil

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
)

var log = slog.With("package", "httputil")

// ExtractBearerToken extracts the bearer token from the Authorization header
func ExtractBearerToken(r *http.Request) string {
	authHeader := r.Header.Get("Authorization")
	if len(authHeader) < 7 || authHeader[:7] != "Bearer " {
		return ""
	}
	return authHeader[7:]
}

// ExtractClientIP extracts the client IP address from the request
// Checks X-Real-IP and X-Forwarded-For headers for proxy deployments
func ExtractClientIP(r *http.Request) string {
	// Check X-Real-IP first (single IP from reverse proxy)
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return ip
	}

	// Check X-Forwarded-For (may contain chain of IPs)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Take the first IP in the chain (original client)
		for idx := 0; idx < len(xff); idx++ {
			if xff[idx] == ',' {
				return xff[:idx]
			}
		}
		return xff
	}

	// Fall back to RemoteAddr
	return r.RemoteAddr
}

// GenerateRandomID generates a cryptographically secure random ID
// Returns a base64-encoded string of the specified byte length
func GenerateRandomID(byteLength int) string {
	b := make([]byte, byteLength)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// GenerateRandomToken generates a cryptographically secure random token
// Alias for GenerateRandomID for backwards compatibility
func GenerateRandomToken(byteLength int) string {
	return GenerateRandomID(byteLength)
}

// WriteJSON writes a JSON response with the given status code
// If encoding fails, it logs the error and sends a 500 Internal Server Error
func WriteJSON(w http.ResponseWriter, code int, v interface{}) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)

	if err := json.NewEncoder(w).Encode(v); err != nil {
		// Log encoding error but don't try to write another response
		// as headers have already been sent
		return err
	}
	return nil
}

// IsLocalRequest checks if the request originates from localhost or a private IP range
func IsLocalRequest(r *http.Request) bool {
	// Extract IP from RemoteAddr
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		// If no port, try parsing as-is
		host = r.RemoteAddr
	}

	ip := net.ParseIP(host)
	if ip == nil {
		// Failed to parse IP, treat as external for security
		log.Warn("Failed to parse IP from RemoteAddr", "remoteAddr", r.RemoteAddr)
		return false
	}

	// Check if IP is loopback (127.0.0.0/8 for IPv4, ::1 for IPv6)
	if ip.IsLoopback() {
		return true
	}

	// Check if IP is in private ranges
	// IPv4: 10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16
	// IPv6: fc00::/7 (which includes fd00::/8)
	if ip.IsPrivate() {
		return true
	}

	// Explicitly check for Docker bridge networks (172.17.0.0/16 to 172.31.0.0/16)
	// Docker typically uses 172.17-172.31 ranges for bridge networks
	if ip.To4() != nil {
		ip4 := ip.To4()
		// Check if in 172.16.0.0/12 range (172.16.0.0 - 172.31.255.255)
		if ip4[0] == 172 && ip4[1] >= 16 && ip4[1] <= 31 {
			return true
		}
	}

	return false
}
