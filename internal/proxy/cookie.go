package proxy

import (
	"bufio"
	"net"
	"net/http"
	"strings"
)

// Ensure CookieResponseWriter implements http.Flusher and http.Hijacker
var _ http.Flusher = (*CookieResponseWriter)(nil)
var _ http.Hijacker = (*CookieResponseWriter)(nil)

// CookieResponseWriter wraps http.ResponseWriter to intercept and modify Set-Cookie headers
type CookieResponseWriter struct {
	http.ResponseWriter
	stripCookies bool
	rewritePaths bool
	pathPrefix   string
	proxyHost    string // The proxy's host for domain rewriting
	wroteHeader  bool
	statusCode   int

	// Session persistence fields
	persistCookies  bool
	deviceID        string
	appID           string
	cookieManager   *SessionCookieManager
	capturedCookies []*http.Cookie
}

// NewCookieResponseWriter creates a new response writer that handles cookies
func NewCookieResponseWriter(w http.ResponseWriter, stripCookies, rewritePaths bool, pathPrefix string) *CookieResponseWriter {
	return &CookieResponseWriter{
		ResponseWriter: w,
		stripCookies:   stripCookies,
		rewritePaths:   rewritePaths,
		pathPrefix:     pathPrefix,
	}
}

// NewCookieResponseWriterWithPersistence creates a response writer with cookie capture for session persistence
func NewCookieResponseWriterWithPersistence(
	w http.ResponseWriter,
	stripCookies, rewritePaths bool,
	pathPrefix string,
	deviceID, appID string,
	cookieManager *SessionCookieManager,
) *CookieResponseWriter {
	return &CookieResponseWriter{
		ResponseWriter:  w,
		stripCookies:    stripCookies,
		rewritePaths:    rewritePaths,
		pathPrefix:      pathPrefix,
		persistCookies:  true,
		deviceID:        deviceID,
		appID:           appID,
		cookieManager:   cookieManager,
		capturedCookies: make([]*http.Cookie, 0),
	}
}

// NewCookieResponseWriterWithDomain creates a response writer with domain rewriting support
func NewCookieResponseWriterWithDomain(w http.ResponseWriter, stripCookies, rewritePaths bool, pathPrefix, proxyHost string) *CookieResponseWriter {
	return &CookieResponseWriter{
		ResponseWriter: w,
		stripCookies:   stripCookies,
		rewritePaths:   rewritePaths,
		pathPrefix:     pathPrefix,
		proxyHost:      proxyHost,
	}
}

// Header returns the header map
func (w *CookieResponseWriter) Header() http.Header {
	return w.ResponseWriter.Header()
}

// WriteHeader writes the status code and processes cookies before sending headers
func (w *CookieResponseWriter) WriteHeader(statusCode int) {
	if w.wroteHeader {
		return
	}
	w.wroteHeader = true
	w.statusCode = statusCode

	// Process cookies based on configuration
	w.processCookies()

	// Write the actual header
	w.ResponseWriter.WriteHeader(statusCode)
}

// Write writes the response body
func (w *CookieResponseWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(b)
}

// processCookies handles cookie modification based on configuration
func (w *CookieResponseWriter) processCookies() {
	// Get all Set-Cookie headers
	cookieHeaders := w.Header().Values("Set-Cookie")

	// Capture cookies for persistence before any modifications
	if w.persistCookies && len(cookieHeaders) > 0 {
		w.captureCookies(cookieHeaders)
	}

	if w.stripCookies {
		// Remove all Set-Cookie headers
		w.Header().Del("Set-Cookie")
		return
	}

	// Check if we need any rewriting
	needsRewrite := (w.rewritePaths && w.pathPrefix != "") || w.proxyHost != ""
	if needsRewrite {
		w.Header().Del("Set-Cookie")
		for _, cookie := range cookieHeaders {
			newCookie := cookie
			// Rewrite path if needed
			if w.rewritePaths && w.pathPrefix != "" {
				newCookie = rewriteCookiePath(newCookie, w.pathPrefix)
			}
			// Rewrite domain if proxy host is set
			if w.proxyHost != "" {
				newCookie = rewriteCookieDomain(newCookie, w.proxyHost)
			}
			w.Header().Add("Set-Cookie", newCookie)
		}
	}
}

// captureCookies parses Set-Cookie headers and stores them for persistence
func (w *CookieResponseWriter) captureCookies(cookieHeaders []string) {
	for _, header := range cookieHeaders {
		// Parse the Set-Cookie header into an http.Cookie
		// We need to create a fake response to use http.ReadSetCookies
		cookie := parseSetCookieHeader(header)
		if cookie != nil {
			w.capturedCookies = append(w.capturedCookies, cookie)
		}
	}
}

// parseSetCookieHeader parses a single Set-Cookie header into an http.Cookie
func parseSetCookieHeader(header string) *http.Cookie {
	// Create a minimal HTTP response to parse the cookie
	resp := &http.Response{Header: http.Header{"Set-Cookie": {header}}}
	cookies := resp.Cookies()
	if len(cookies) > 0 {
		return cookies[0]
	}
	return nil
}

// HasCapturedCookies returns true if any cookies were captured for persistence
func (w *CookieResponseWriter) HasCapturedCookies() bool {
	return len(w.capturedCookies) > 0
}

// PersistCapturedCookies saves the captured cookies using the session cookie manager
func (w *CookieResponseWriter) PersistCapturedCookies() error {
	if w.cookieManager == nil || len(w.capturedCookies) == 0 {
		return nil
	}
	return w.cookieManager.CaptureResponseCookies(w.deviceID, w.appID, w.capturedCookies)
}

// GetCapturedCookies returns the cookies that were captured (for testing)
func (w *CookieResponseWriter) GetCapturedCookies() []*http.Cookie {
	return w.capturedCookies
}

// rewriteCookiePath rewrites the Path attribute of a Set-Cookie header
func rewriteCookiePath(cookieHeader, pathPrefix string) string {
	// Parse the cookie to find the Path attribute
	parts := strings.Split(cookieHeader, ";")
	if len(parts) == 0 {
		return cookieHeader
	}

	var result []string
	result = append(result, parts[0]) // Cookie name=value

	for _, part := range parts[1:] {
		trimmed := strings.TrimSpace(part)

		// Check if this is the Path attribute (case-insensitive)
		if strings.HasPrefix(strings.ToLower(trimmed), "path=") {
			// Extract the path value - use the length of "path=" which is always 5
			pathValue := trimmed[5:]

			// Rewrite the path
			newPath := rewritePath(pathValue, pathPrefix)
			result = append(result, " Path="+newPath)
		} else {
			result = append(result, " "+trimmed)
		}
	}

	return strings.Join(result, ";")
}

// rewritePath combines the prefix with the cookie path
func rewritePath(cookiePath, pathPrefix string) string {
	// Handle root path - use prefix as-is
	if cookiePath == "/" {
		return pathPrefix
	}

	// Ensure prefix ends with /
	if !strings.HasSuffix(pathPrefix, "/") {
		pathPrefix += "/"
	}

	// Remove leading slash from cookie path if present
	cookiePath = strings.TrimPrefix(cookiePath, "/")

	// Combine paths
	return pathPrefix + cookiePath
}

// rewriteCookieDomain rewrites or removes the Domain attribute of a Set-Cookie header
// to match the proxy's host, preventing cookies from being rejected by browsers
func rewriteCookieDomain(cookieHeader, proxyHost string) string {
	parts := strings.Split(cookieHeader, ";")
	if len(parts) == 0 {
		return cookieHeader
	}

	var result []string
	result = append(result, parts[0]) // Cookie name=value

	foundDomain := false
	for _, part := range parts[1:] {
		trimmed := strings.TrimSpace(part)

		// Check if this is the Domain attribute (case-insensitive)
		if strings.HasPrefix(strings.ToLower(trimmed), "domain=") {
			foundDomain = true
			// Extract the domain value - use the length of "domain=" which is always 7
			domainValue := trimmed[7:]

			// Strip leading dot if present (e.g., ".backend.local" -> "backend.local")
			domainValue = strings.TrimPrefix(domainValue, ".")

			// If the domain matches the proxy host, keep it as-is
			if strings.EqualFold(domainValue, proxyHost) {
				result = append(result, " Domain="+proxyHost)
			} else {
				// Rewrite to proxy host
				result = append(result, " Domain="+proxyHost)
			}
		} else {
			result = append(result, " "+trimmed)
		}
	}

	// If no domain was found, we don't add one - let the browser use the request host
	_ = foundDomain

	return strings.Join(result, ";")
}

// StatusCode returns the status code that was written
func (w *CookieResponseWriter) StatusCode() int {
	if w.statusCode == 0 {
		return http.StatusOK
	}
	return w.statusCode
}

// Flush implements http.Flusher to support streaming responses
func (w *CookieResponseWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Hijack implements http.Hijacker for WebSocket upgrade support
func (w *CookieResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hj, ok := w.ResponseWriter.(http.Hijacker); ok {
		return hj.Hijack()
	}
	return nil, nil, http.ErrNotSupported
}
