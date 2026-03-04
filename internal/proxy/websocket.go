package proxy

import (
	"bytes"
	"context"
	"crypto/sha1"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"
)

// WebSocketBufferSize is the size of pooled buffers for WebSocket connections
const WebSocketBufferSize = 32 * 1024 // 32KB

// webSocketBufferPool is a sync.Pool for reusing WebSocket copy buffers
var webSocketBufferPool = sync.Pool{
	New: func() interface{} {
		buf := make([]byte, WebSocketBufferSize)
		return &buf
	},
}

// GetWebSocketBuffer gets a buffer from the pool
func GetWebSocketBuffer() *[]byte {
	return webSocketBufferPool.Get().(*[]byte)
}

// PutWebSocketBuffer returns a buffer to the pool
func PutWebSocketBuffer(buf *[]byte) {
	webSocketBufferPool.Put(buf)
}

// CreateTLSDialer creates a TLS dialer that supports context cancellation
// This allows the TLS handshake to be cancelled when the context is cancelled
func CreateTLSDialer(timeout time.Duration) *tls.Dialer {
	return &tls.Dialer{
		NetDialer: &net.Dialer{
			Timeout:   timeout,
			KeepAlive: 30 * time.Second,
		},
	}
}

// IsWebSocketUpgrade checks if the request is a WebSocket upgrade request
func IsWebSocketUpgrade(r *http.Request) bool {
	return strings.ToLower(r.Header.Get("Upgrade")) == "websocket" &&
		strings.Contains(strings.ToLower(r.Header.Get("Connection")), "upgrade")
}

// ImportantHeaders lists headers that should be forwarded to upstream WebSocket servers
// These headers are important for proper WebSocket functionality and user identification
var ImportantHeaders = []string{
	"User-Agent",
	"Accept-Language",
	"Referer",
}

// DefaultMaxMessageSize is the default maximum WebSocket message size
const DefaultMaxMessageSize int64 = 100 * 1024 * 1024 // 100MB

// hasPort checks if a host string has a port number, handling IPv6 addresses correctly.
// IPv6 addresses are expected in bracket notation: [::1]:8080
func hasPort(host string) bool {
	// IPv6 addresses are enclosed in brackets
	if strings.HasPrefix(host, "[") {
		// For IPv6 in brackets, port comes after the closing bracket
		return strings.Contains(host, "]:")
	}
	// For IPv4/hostname, just check for colon
	return strings.Contains(host, ":")
}

// extractServerName extracts the server name from a host string for TLS SNI.
// Handles both IPv4 and IPv6 addresses.
func extractServerName(host string) string {
	// IPv6 addresses are enclosed in brackets
	if strings.HasPrefix(host, "[") {
		// Find the closing bracket
		if idx := strings.Index(host, "]"); idx != -1 {
			// Return the address without brackets
			return host[1:idx]
		}
		// Malformed, return as-is without the opening bracket
		return host[1:]
	}

	// For IPv4/hostname, strip port if present
	if colonIdx := strings.LastIndex(host, ":"); colonIdx != -1 {
		return host[:colonIdx]
	}
	return host
}

// WebSocketProxy handles WebSocket connections by establishing a bidirectional
// tunnel between the client and upstream server
type WebSocketProxy struct {
	// Target is the upstream WebSocket server URL (scheme should be ws:// or wss://)
	Target string

	// OnConnect is called when a WebSocket connection is established
	OnConnect func(clientAddr, upstreamAddr string)

	// OnDisconnect is called when a WebSocket connection is closed
	OnDisconnect func(clientAddr, upstreamAddr string, duration time.Duration)

	// OnError is called when an error occurs during proxying
	OnError func(err error)

	// BufferSize is the size of the copy buffer (default: 32KB)
	BufferSize int

	// DialTimeout is the timeout for establishing upstream connection (default: 10s)
	DialTimeout time.Duration

	// ReadTimeout is the timeout for read operations (default: 0 = no timeout)
	ReadTimeout time.Duration

	// WriteTimeout is the timeout for write operations (default: 0 = no timeout)
	WriteTimeout time.Duration

	// IdleTimeout closes connections after this duration of inactivity (default: 0 = no timeout)
	IdleTimeout time.Duration

	// TLSConfig is the TLS configuration for wss:// connections (optional)
	TLSConfig *tls.Config

	// InsecureSkipVerify skips TLS certificate verification (not recommended for production)
	InsecureSkipVerify bool

	// MaxMessageSize is the maximum size of a single WebSocket message
	// Default: 0 (uses DefaultMaxMessageSize of 100MB)
	MaxMessageSize int64

	// StripCookies removes Cookie headers before forwarding to upstream
	// Use this for untrusted upstreams
	StripCookies bool
}

// NewWebSocketProxy creates a new WebSocket proxy
func NewWebSocketProxy(target string) *WebSocketProxy {
	return &WebSocketProxy{
		Target:      target,
		BufferSize:  32 * 1024, // 32KB
		DialTimeout: 10 * time.Second,
	}
}

// ServeHTTP handles the WebSocket proxy request
func (p *WebSocketProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Verify this is a WebSocket upgrade request
	if !IsWebSocketUpgrade(r) {
		http.Error(w, "Expected WebSocket upgrade", http.StatusBadRequest)
		return
	}

	// Hijack the connection to take control of the underlying TCP connection
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "WebSocket hijacking not supported", http.StatusInternalServerError)
		return
	}

	clientConn, clientBuf, err := hijacker.Hijack()
	if err != nil {
		p.logError(err)
		http.Error(w, "Failed to hijack connection", http.StatusInternalServerError)
		return
	}

	// Create cancellable context for graceful shutdown
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	// Convert target URL scheme (http -> ws, https -> wss)
	target := p.Target
	target = strings.Replace(target, "http://", "ws://", 1)
	target = strings.Replace(target, "https://", "wss://", 1)

	// If the target doesn't have a scheme, add ws://
	if !strings.HasPrefix(target, "ws://") && !strings.HasPrefix(target, "wss://") {
		target = "ws://" + target
	}

	// Get the WebSocket key for validation
	wsKey := r.Header.Get("Sec-WebSocket-Key")

	// Connect to upstream WebSocket server
	upstreamConn, upgradeResponse, extraData, err := p.dialUpstream(ctx, target, r)
	if err != nil {
		p.logError(err)
		// Send error response to client
		clientConn.Write([]byte("HTTP/1.1 502 Bad Gateway\r\n\r\n"))
		clientConn.Close()
		return
	}

	// Validate the upgrade response per RFC 6455
	if err := p.validateUpgradeResponse(upgradeResponse, wsKey); err != nil {
		p.logError(fmt.Errorf("invalid upgrade response: %w", err))
		clientConn.Write([]byte("HTTP/1.1 502 Bad Gateway\r\n\r\n"))
		clientConn.Close()
		upstreamConn.Close()
		return
	}

	// Forward the 101 Switching Protocols response to the client
	if _, err := clientConn.Write(upgradeResponse); err != nil {
		p.logError(fmt.Errorf("failed to send upgrade response to client: %w", err))
		clientConn.Close()
		upstreamConn.Close()
		return
	}

	// Forward any extra data that came after the headers
	if len(extraData) > 0 {
		if _, err := clientConn.Write(extraData); err != nil {
			p.logError(fmt.Errorf("failed to send initial data to client: %w", err))
			clientConn.Close()
			upstreamConn.Close()
			return
		}
	}

	// Call connect callback
	startTime := time.Now()
	if p.OnConnect != nil {
		p.OnConnect(clientConn.RemoteAddr().String(), upstreamConn.RemoteAddr().String())
	}

	// Set up idle timeout if configured
	if p.IdleTimeout > 0 {
		idleTimer := time.AfterFunc(p.IdleTimeout, func() {
			cancel() // Cancel context to stop copy goroutines
		})
		defer idleTimer.Stop()
	}

	// Bidirectional copy between client and upstream with graceful shutdown
	errChan := make(chan error, 2)

	// Client -> Upstream
	go func() {
		_, err := p.copyWithContext(ctx, upstreamConn, clientBuf, p.ReadTimeout, p.WriteTimeout)
		errChan <- err
	}()

	// Upstream -> Client
	go func() {
		_, err := p.copyWithContext(ctx, clientConn, upstreamConn, p.ReadTimeout, p.WriteTimeout)
		errChan <- err
	}()

	// Wait for first error/completion
	err = <-errChan

	// Cancel context to signal other goroutine to stop
	cancel()

	// Set deadlines to force connections to close
	deadline := time.Now().Add(100 * time.Millisecond)
	clientConn.SetDeadline(deadline)
	upstreamConn.SetDeadline(deadline)

	// Wait for second goroutine with timeout
	select {
	case <-errChan:
	case <-time.After(200 * time.Millisecond):
	}

	// Close connections
	clientConn.Close()
	upstreamConn.Close()

	// Call disconnect callback
	duration := time.Since(startTime)
	if p.OnDisconnect != nil {
		p.OnDisconnect(clientConn.RemoteAddr().String(), upstreamConn.RemoteAddr().String(), duration)
	}

	// Log error if not a normal closure
	if err != nil && !isNormalClose(err) {
		p.logError(err)
	}
}

// dialUpstream establishes a connection to the upstream WebSocket server
// Returns the connection, the upgrade response headers, any extra data after headers, and error
func (p *WebSocketProxy) dialUpstream(ctx context.Context, target string, originalReq *http.Request) (net.Conn, []byte, []byte, error) {
	// Determine if we need TLS
	useTLS := strings.HasPrefix(target, "wss://")

	// Remove scheme prefix
	target = strings.TrimPrefix(target, "wss://")
	target = strings.TrimPrefix(target, "ws://")

	// Split host and path
	parts := strings.SplitN(target, "/", 2)
	host := parts[0]
	basePath := "/"
	if len(parts) > 1 {
		basePath = "/" + parts[1]
	}

	// Join target path with request path (handles double slashes correctly)
	requestPath := originalReq.URL.Path
	if requestPath == "" {
		requestPath = "/"
	}
	finalPath := path.Join(basePath, requestPath)
	if !strings.HasPrefix(finalPath, "/") {
		finalPath = "/" + finalPath
	}

	// Add query string if present
	if originalReq.URL.RawQuery != "" {
		finalPath = finalPath + "?" + originalReq.URL.RawQuery
	}

	// Dial with timeout
	dialer := &net.Dialer{
		Timeout:   p.DialTimeout,
		KeepAlive: 30 * time.Second,
	}

	var conn net.Conn
	var err error

	if useTLS {
		// Extract hostname for SNI (handles IPv6 addresses correctly)
		serverName := extractServerName(host)

		tlsConfig := p.TLSConfig
		if tlsConfig == nil {
			tlsConfig = &tls.Config{
				ServerName:         serverName,
				MinVersion:         tls.VersionTLS12, // Secure default
				InsecureSkipVerify: p.InsecureSkipVerify,
			}
		} else {
			// Clone config and set ServerName if not set
			tlsConfig = tlsConfig.Clone()
			if tlsConfig.ServerName == "" {
				tlsConfig.ServerName = serverName
			}
		}

		// Add default port for TLS if not specified (handles IPv6)
		if !hasPort(host) {
			host = host + ":443"
		}

		conn, err = tls.DialWithDialer(dialer, "tcp", host, tlsConfig)
	} else {
		// Add default port for non-TLS if not specified (handles IPv6)
		if !hasPort(host) {
			host = host + ":80"
		}

		conn, err = dialer.DialContext(ctx, "tcp", host)
	}

	if err != nil {
		return nil, nil, nil, err
	}

	// Enable TCP keepalive
	if tcpConn, ok := conn.(*net.TCPConn); ok {
		tcpConn.SetKeepAlive(true)
		tcpConn.SetKeepAlivePeriod(30 * time.Second)
	}

	// Build HTTP upgrade request
	var upgradeReq strings.Builder
	upgradeReq.WriteString("GET ")
	upgradeReq.WriteString(finalPath)
	upgradeReq.WriteString(" HTTP/1.1\r\n")

	// Use original host for Host header to preserve virtual hosting
	upgradeReq.WriteString("Host: ")
	upgradeReq.WriteString(host)
	upgradeReq.WriteString("\r\n")

	upgradeReq.WriteString("Upgrade: websocket\r\n")
	upgradeReq.WriteString("Connection: Upgrade\r\n")

	// Copy WebSocket-specific headers
	if val := originalReq.Header.Get("Sec-WebSocket-Key"); val != "" {
		upgradeReq.WriteString("Sec-WebSocket-Key: ")
		upgradeReq.WriteString(val)
		upgradeReq.WriteString("\r\n")
	}
	if val := originalReq.Header.Get("Sec-WebSocket-Version"); val != "" {
		upgradeReq.WriteString("Sec-WebSocket-Version: ")
		upgradeReq.WriteString(val)
		upgradeReq.WriteString("\r\n")
	}
	if val := originalReq.Header.Get("Sec-WebSocket-Protocol"); val != "" {
		upgradeReq.WriteString("Sec-WebSocket-Protocol: ")
		upgradeReq.WriteString(val)
		upgradeReq.WriteString("\r\n")
	}
	if val := originalReq.Header.Get("Sec-WebSocket-Extensions"); val != "" {
		upgradeReq.WriteString("Sec-WebSocket-Extensions: ")
		upgradeReq.WriteString(val)
		upgradeReq.WriteString("\r\n")
	}

	// Copy Origin header (important for WebSocket servers that check origin)
	if val := originalReq.Header.Get("Origin"); val != "" {
		upgradeReq.WriteString("Origin: ")
		upgradeReq.WriteString(val)
		upgradeReq.WriteString("\r\n")
	}

	// Copy Cookie header (important for session-based auth) unless StripCookies is set
	if !p.StripCookies {
		if val := originalReq.Header.Get("Cookie"); val != "" {
			upgradeReq.WriteString("Cookie: ")
			upgradeReq.WriteString(val)
			upgradeReq.WriteString("\r\n")
		}
	}

	// Copy forwarded headers
	if val := originalReq.Header.Get("X-Forwarded-For"); val != "" {
		upgradeReq.WriteString("X-Forwarded-For: ")
		upgradeReq.WriteString(val)
		upgradeReq.WriteString("\r\n")
	}
	if val := originalReq.Header.Get("X-Forwarded-Host"); val != "" {
		upgradeReq.WriteString("X-Forwarded-Host: ")
		upgradeReq.WriteString(val)
		upgradeReq.WriteString("\r\n")
	}
	if val := originalReq.Header.Get("X-Forwarded-Proto"); val != "" {
		upgradeReq.WriteString("X-Forwarded-Proto: ")
		upgradeReq.WriteString(val)
		upgradeReq.WriteString("\r\n")
	}
	if val := originalReq.Header.Get("X-Real-IP"); val != "" {
		upgradeReq.WriteString("X-Real-IP: ")
		upgradeReq.WriteString(val)
		upgradeReq.WriteString("\r\n")
	}

	// Copy important headers
	for _, h := range ImportantHeaders {
		if val := originalReq.Header.Get(h); val != "" {
			upgradeReq.WriteString(h)
			upgradeReq.WriteString(": ")
			upgradeReq.WriteString(val)
			upgradeReq.WriteString("\r\n")
		}
	}

	upgradeReq.WriteString("\r\n")

	// Send upgrade request
	if _, err := conn.Write([]byte(upgradeReq.String())); err != nil {
		conn.Close()
		return nil, nil, nil, err
	}

	// Read upgrade response with timeout
	if p.DialTimeout > 0 {
		conn.SetReadDeadline(time.Now().Add(p.DialTimeout))
	}

	// Read response headers
	buf := make([]byte, 8192) // Increased buffer for larger headers
	totalRead := 0

	for totalRead < len(buf) {
		n, err := conn.Read(buf[totalRead:])
		if err != nil {
			conn.Close()
			return nil, nil, nil, err
		}
		totalRead += n

		// Find end of headers
		response := buf[:totalRead]
		if idx := bytes.Index(response, []byte("\r\n\r\n")); idx != -1 {
			headerEnd := idx + 4

			// Clear the read deadline
			conn.SetReadDeadline(time.Time{})

			// Return headers and any extra data
			headers := response[:headerEnd]
			extraData := response[headerEnd:]

			return conn, headers, extraData, nil
		}
	}

	conn.Close()
	return nil, nil, nil, fmt.Errorf("websocket upgrade response headers too large (>8KB)")
}

// validateUpgradeResponse validates the WebSocket upgrade response per RFC 6455
func (p *WebSocketProxy) validateUpgradeResponse(response []byte, clientKey string) error {
	// Parse status line
	lines := bytes.Split(response, []byte("\r\n"))
	if len(lines) < 1 {
		return errors.New("empty response")
	}

	// Check status line: must start with "HTTP/1.1 101"
	statusLine := string(lines[0])
	if !strings.HasPrefix(statusLine, "HTTP/1.1 101") && !strings.HasPrefix(statusLine, "HTTP/1.0 101") {
		return fmt.Errorf("unexpected status: %s", statusLine)
	}

	// Parse headers into map
	headers := make(map[string]string)
	for _, line := range lines[1:] {
		if len(line) == 0 {
			break
		}
		if idx := bytes.IndexByte(line, ':'); idx > 0 {
			key := strings.ToLower(strings.TrimSpace(string(line[:idx])))
			val := strings.TrimSpace(string(line[idx+1:]))
			headers[key] = val
		}
	}

	// Validate Upgrade header (RFC 6455 Section 4.2.1)
	upgrade := strings.ToLower(headers["upgrade"])
	if !strings.Contains(upgrade, "websocket") {
		return errors.New("missing or invalid Upgrade header")
	}

	// Validate Connection header (RFC 6455 Section 4.2.1)
	connection := strings.ToLower(headers["connection"])
	if !strings.Contains(connection, "upgrade") {
		return errors.New("missing or invalid Connection header")
	}

	// Validate Sec-WebSocket-Accept (RFC 6455 Section 4.2.2)
	if clientKey != "" {
		expectedAccept := computeAcceptKey(clientKey)
		actualAccept := headers["sec-websocket-accept"]
		if actualAccept != expectedAccept {
			return fmt.Errorf("invalid Sec-WebSocket-Accept: got %q, expected %q", actualAccept, expectedAccept)
		}
	}

	return nil
}

// computeAcceptKey computes the Sec-WebSocket-Accept value per RFC 6455
func computeAcceptKey(key string) string {
	const websocketGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"
	h := sha1.New()
	h.Write([]byte(key))
	h.Write([]byte(websocketGUID))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

// copyWithContext copies data from src to dst with context cancellation support
func (p *WebSocketProxy) copyWithContext(ctx context.Context, dst io.Writer, src io.Reader, readTimeout, writeTimeout time.Duration) (int64, error) {
	bufSize := p.BufferSize
	if bufSize == 0 {
		bufSize = 32 * 1024
	}

	buf := make([]byte, bufSize)
	written := int64(0)

	for {
		// Check context cancellation
		select {
		case <-ctx.Done():
			return written, ctx.Err()
		default:
		}

		// Set read timeout if configured
		if readTimeout > 0 {
			if conn, ok := src.(net.Conn); ok {
				conn.SetReadDeadline(time.Now().Add(readTimeout))
			}
		}

		nr, readErr := src.Read(buf)
		if nr > 0 {
			// Set write timeout if configured
			if writeTimeout > 0 {
				if conn, ok := dst.(net.Conn); ok {
					conn.SetWriteDeadline(time.Now().Add(writeTimeout))
				}
			}

			nw, writeErr := dst.Write(buf[:nr])
			if nw > 0 {
				written += int64(nw)
			}
			if writeErr != nil {
				return written, writeErr
			}
			if nr != nw {
				return written, io.ErrShortWrite
			}
		}
		if readErr != nil {
			// Check if it's a timeout and context is still active
			if isTimeout(readErr) {
				select {
				case <-ctx.Done():
					return written, ctx.Err()
				default:
					continue
				}
			}
			return written, readErr
		}
	}
}

// logError logs an error if OnError callback is set
func (p *WebSocketProxy) logError(err error) {
	if p.OnError != nil {
		p.OnError(err)
	} else {
		log.Error("websocket proxy error",
			"error", err)
	}
}

// isNormalClose checks if the error is a normal connection closure
func isNormalClose(err error) bool {
	if err == nil || err == io.EOF {
		return true
	}

	// Check for context cancellation
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	// Check for closed network connection
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		if errors.Is(opErr.Err, net.ErrClosed) {
			return true
		}
		// Also check the error string for compatibility
		if opErr.Err != nil && strings.Contains(opErr.Err.Error(), "use of closed network connection") {
			return true
		}
	}

	return false
}

// isTimeout checks if an error is a timeout error
func isTimeout(err error) bool {
	var netErr net.Error
	if errors.As(err, &netErr) {
		return netErr.Timeout()
	}
	return false
}

// ParseHTTPStatus parses the HTTP status code from a response header
// Returns the status code and true if successful, or 0 and false if parsing fails
func ParseHTTPStatus(response []byte) (int, bool) {
	if len(response) == 0 {
		return 0, false
	}

	// Find the end of the first line
	lineEnd := bytes.Index(response, []byte("\r\n"))
	if lineEnd == -1 {
		lineEnd = len(response)
	}

	statusLine := string(response[:lineEnd])

	// Parse "HTTP/x.x NNN ..."
	if !strings.HasPrefix(statusLine, "HTTP/") {
		return 0, false
	}

	// Find the status code after "HTTP/x.x "
	parts := strings.SplitN(statusLine, " ", 3)
	if len(parts) < 2 {
		return 0, false
	}

	// Parse status code
	var status int
	_, err := fmt.Sscanf(parts[1], "%d", &status)
	if err != nil {
		return 0, false
	}

	return status, true
}
