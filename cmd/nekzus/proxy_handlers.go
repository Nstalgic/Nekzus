package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/nstalgic/nekzus/internal/auth"
	apperrors "github.com/nstalgic/nekzus/internal/errors"
	"github.com/nstalgic/nekzus/internal/httputil"
	"github.com/nstalgic/nekzus/internal/logger"
	"github.com/nstalgic/nekzus/internal/middleware"
	"github.com/nstalgic/nekzus/internal/proxy"
	"github.com/nstalgic/nekzus/internal/router"
	"github.com/nstalgic/nekzus/internal/types"
)

var proxyLog = slog.With("package", "proxy_handlers")

// handleProxy handles reverse proxy requests (HTTP and WebSocket)
func (app *Application) handleProxy(w http.ResponseWriter, r *http.Request) {
	// Validate Host header for CRLF injection
	host := r.Host
	if strings.ContainsAny(host, "\r\n\x00") {
		apperrors.WriteJSON(w, apperrors.New("INVALID_REQUEST", "Invalid host header", http.StatusBadRequest))
		return
	}

	// Normalize path to prevent traversal attacks (/../, //, etc.)
	normalizedPath := router.NormalizePath(r.URL.Path)

	// Find matching route first
	route, ok := app.managers.Router.GetRouteByPath(normalizedPath)
	if !ok {
		// Try adding trailing slash - many routes are defined with trailing slash
		// Redirect to canonical URL with trailing slash if that route exists
		if !strings.HasSuffix(normalizedPath, "/") {
			pathWithSlash := normalizedPath + "/"
			if _, found := app.managers.Router.GetRouteByPath(pathWithSlash); found {
				// Redirect to path with trailing slash
				http.Redirect(w, r, pathWithSlash, http.StatusMovedPermanently)
				return
			}
		}
		http.NotFound(w, r)
		return
	}

	// Extract JWT if present (optional - IP-based auth middleware handles authentication)
	// Use effective scopes (route scopes or default scopes from config)
	token := httputil.ExtractBearerToken(r)
	var claims map[string]interface{}
	effectiveScopes := route.GetEffectiveScopes(app.config.Auth.DefaultScopes)

	// If JWT is present and route has scope requirements, validate it
	if token != "" && len(effectiveScopes) > 0 {
		_, parsedClaims, err := app.services.Auth.ParseJWT(token)
		if err != nil {
			apperrors.WriteJSON(w, apperrors.ErrInvalidToken)
			return
		}
		claims = parsedClaims
	}

	// Check if this is a federated service on a remote peer
	if app.managers.Peers != nil && app.config.Federation.AllowRemoteRoutes {
		if remotePeerAddr, isRemote := app.getRemoteServiceAddress(route.AppID); isRemote {
			// Proxy to remote peer
			app.handleRemoteProxy(w, r, route, remotePeerAddr, claims)
			return
		}
	}

	// Check scopes only if JWT was provided and route has scope requirements
	if claims != nil && len(effectiveScopes) > 0 {
		if !auth.HasAllScopesFromAny(claims["scopes"], effectiveScopes) {
			apperrors.WriteJSON(w, apperrors.ErrForbidden)
			return
		}
	}

	// Check service health (if health checker is configured)
	if app.jobs.ServiceHealth != nil && !app.jobs.ServiceHealth.IsServiceHealthy(route.AppID) {
		proxyLog.Debug("service unhealthy, returning 503",
			"app_id", route.AppID)
		apperrors.WriteJSON(w, apperrors.New("SERVICE_UNHEALTHY", "Service is currently unhealthy", http.StatusServiceUnavailable))
		if app.metrics != nil {
			app.metrics.RecordProxyError(route.AppID, "service_unhealthy")
		}
		return
	}

	// Rewrite path if StripPrefix is enabled
	// Use normalized path to prevent bypassing strip logic with path traversal
	if route.StripPrefix && strings.HasPrefix(normalizedPath, route.PathBase) {
		// Strip prefix from both Path and RawPath (for URL-encoded paths)
		newPath := strings.TrimPrefix(normalizedPath, route.PathBase)
		if newPath == "" {
			r.URL.Path = "/"
		} else {
			if !strings.HasPrefix(newPath, "/") {
				newPath = "/" + newPath
			}
			r.URL.Path = newPath
		}

		// Handle RawPath for URL-encoded paths
		if r.URL.RawPath != "" {
			newRawPath := strings.TrimPrefix(r.URL.RawPath, route.PathBase)
			if newRawPath == "" {
				r.URL.RawPath = "/"
			} else {
				if !strings.HasPrefix(newRawPath, "/") {
					newRawPath = "/" + newRawPath
				}
				r.URL.RawPath = newRawPath
			}
		}

		// Add X-Forwarded-Prefix header to communicate stripped prefix to upstream
		r.Header.Set("X-Forwarded-Prefix", route.PathBase)

		// Update RequestURI to reflect the rewritten path
		r.RequestURI = r.URL.RequestURI()
	}

	// Parse target URL
	target, err := url.Parse(route.To)
	if err != nil {
		apperrors.WriteJSON(w, apperrors.ErrBadGateway)
		return
	}

	// Set forwarding headers
	r.Header.Del("Authorization") // Remove client auth

	// Extract client IP from RemoteAddr (strip port)
	clientIP := r.RemoteAddr
	if host, _, err := net.SplitHostPort(clientIP); err == nil {
		clientIP = host
	}

	// Set X-Real-IP if not already set
	if r.Header.Get("X-Real-IP") == "" {
		r.Header.Set("X-Real-IP", clientIP)
	}

	// Append to X-Forwarded-For (don't replace existing chain)
	if existing := r.Header.Get("X-Forwarded-For"); existing != "" {
		r.Header.Set("X-Forwarded-For", existing+", "+clientIP)
	} else {
		r.Header.Set("X-Forwarded-For", clientIP)
	}

	// Set X-Forwarded-Host
	r.Header.Set("X-Forwarded-Host", r.Host)

	// Check if this is a WebSocket upgrade request first
	// to set the correct X-Forwarded-Proto (ws/wss instead of http/https)
	isWebSocket := route.Websocket && proxy.IsWebSocketUpgrade(r)

	// Set X-Forwarded-Proto and X-Forwarded-Port
	// Extract actual port from Host header, or use default based on protocol
	var defaultPort string
	if r.TLS != nil {
		defaultPort = "443"
	} else {
		defaultPort = "80"
	}

	actualPort := defaultPort
	if _, port, err := net.SplitHostPort(r.Host); err == nil {
		actualPort = port
	}

	if isWebSocket {
		// For WebSocket connections, use ws/wss protocol
		if r.TLS != nil {
			r.Header.Set("X-Forwarded-Proto", "wss")
		} else {
			r.Header.Set("X-Forwarded-Proto", "ws")
		}
	} else {
		// For regular HTTP requests
		if r.TLS != nil {
			r.Header.Set("X-Forwarded-Proto", "https")
		} else {
			r.Header.Set("X-Forwarded-Proto", "http")
		}
	}

	if r.Header.Get("X-Forwarded-Port") == "" {
		r.Header.Set("X-Forwarded-Port", actualPort)
	}

	// Handle WebSocket proxy if needed
	if isWebSocket {
		app.handleWebSocketProxy(w, r, route, target)
		return
	}

	// Handle regular HTTP proxy
	httpProxy := app.proxyCache.GetOrCreate(target)
	// Note: We preserve the original Host header by default (standard reverse proxy behavior)
	// X-Forwarded-Host is also set for backends that need the original host info

	// Get device ID from context for session cookie persistence
	deviceID := middleware.GetDeviceIDFromContext(r.Context())

	// Inject stored session cookies if persistence is enabled and device is authenticated
	if route.PersistCookies && deviceID != "" && app.services.SessionCookies != nil {
		if err := app.services.SessionCookies.InjectRequestCookies(deviceID, route.AppID, r); err != nil {
			proxyLog.Debug("failed to inject session cookies",
				"device_id", deviceID,
				"app_id", route.AppID,
				"error", err)
		}
	}

	// Wrap ResponseWriter with appropriate middlewares
	var responseWriter http.ResponseWriter = w

	// Layer 0: gRPC-Web CORS headers (always enabled for gRPC compatibility)
	responseWriter = proxy.NewGRPCHeaderResponseWriter(responseWriter)

	// Layer 1: Cookie handling (with optional persistence)
	var cookieWriter *proxy.CookieResponseWriter
	if route.PersistCookies && deviceID != "" && app.services.SessionCookies != nil {
		// Use persistence-enabled cookie writer
		cookieWriter = proxy.NewCookieResponseWriterWithPersistence(
			responseWriter,
			route.StripResponseCookies,
			route.RewriteCookiePaths && route.StripPrefix,
			route.PathBase,
			deviceID,
			route.AppID,
			app.services.SessionCookies,
		)
		responseWriter = cookieWriter
	} else if route.StripResponseCookies || (route.RewriteCookiePaths && route.StripPrefix) {
		responseWriter = proxy.NewCookieResponseWriter(
			responseWriter,
			route.StripResponseCookies,
			route.RewriteCookiePaths && route.StripPrefix, // Only rewrite if strip_prefix is also enabled
			route.PathBase,
		)
	}

	// Layer 2: Header and HTML rewriting (if StripPrefix is enabled)
	// Always rewrite headers (Location, Refresh, etc.) when stripping prefix
	// Only rewrite HTML body content if RewriteHTML is also enabled
	var htmlWriter *proxy.HTMLRewritingResponseWriter
	if route.StripPrefix {
		requestScheme := "http"
		if r.TLS != nil {
			requestScheme = "https"
		}

		if route.RewriteHTML {
			// Full rewriting: headers + HTML body
			// Pass both route.PathBase (for JS interceptor) and r.URL.Path (for base href)
			// This ensures relative paths like "./app.js" resolve correctly when the app
			// internally redirects to subpaths (e.g., Transmission redirecting to /transmission/web/)
			htmlWriter = proxy.NewHTMLRewritingResponseWriterWithHost(responseWriter, route.PathBase, r.URL.Path, r.Host, requestScheme)
			responseWriter = htmlWriter

			// Remove Accept-Encoding to get uncompressed responses from backend
			// This avoids needing to decompress brotli, zstd, etc. before rewriting
			// We still handle gzip/deflate as fallback for backends that ignore this
			r.Header.Del("Accept-Encoding")
		} else {
			// Header-only rewriting: Location, Refresh, CSP, etc. but not HTML body
			htmlWriter = proxy.NewHeaderRewritingResponseWriter(responseWriter, route.PathBase, r.Host, requestScheme)
			responseWriter = htmlWriter
		}
	}

	// Propagate context with timeout to cancel upstream on client disconnect
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	r = r.WithContext(ctx)

	// Proxy the request
	httpProxy.ServeHTTP(responseWriter, r)

	// Flush HTML rewriter if it was used
	if htmlWriter != nil {
		if err := htmlWriter.FlushHTML(); err != nil {
			proxyLog.Error("failed to flush html rewriter",
				"error", err)
		}
	}

	// Persist captured cookies asynchronously
	if cookieWriter != nil && cookieWriter.HasCapturedCookies() {
		go func() {
			if err := cookieWriter.PersistCapturedCookies(); err != nil {
				proxyLog.Warn("failed to persist session cookies",
					"device_id", deviceID,
					"app_id", route.AppID,
					"error", err)
			}
		}()
	}
}

// handleWebSocketProxy handles WebSocket connections
func (app *Application) handleWebSocketProxy(w http.ResponseWriter, r *http.Request, route types.Route, target *url.URL) {
	// Create WebSocket proxy
	wsProxy := proxy.NewWebSocketProxy(target.String())

	// Set up callbacks for metrics
	wsProxy.OnConnect = func(clientAddr, upstreamAddr string) {
		proxyLog.Debug("websocket connected",
			"client_addr", clientAddr,
			"upstream_addr", upstreamAddr,
			"app_id", route.AppID,
			"component", logger.CompWebSocket)
		if app.metrics != nil {
			app.metrics.WebSocketConnectionsActive.Inc()
			app.metrics.WebSocketConnectionsTotal.WithLabelValues(route.AppID, "connected").Inc()
		}
	}

	wsProxy.OnDisconnect = func(clientAddr, upstreamAddr string, duration time.Duration) {
		proxyLog.Debug("websocket disconnected",
			"client_addr", clientAddr,
			"upstream_addr", upstreamAddr,
			"app_id", route.AppID,
			"duration", duration,
			"component", logger.CompWebSocket)
		if app.metrics != nil {
			app.metrics.WebSocketConnectionsActive.Dec()
			app.metrics.WebSocketConnectionDuration.Observe(duration.Seconds())
		}
	}

	wsProxy.OnError = func(err error) {
		proxyLog.Error("websocket error",
			"app_id", route.AppID,
			"error", err,
			"component", logger.CompWebSocket)
		if app.metrics != nil {
			app.metrics.WebSocketConnectionsTotal.WithLabelValues(route.AppID, "error").Inc()
		}
	}

	// Proxy the WebSocket connection
	wsProxy.ServeHTTP(w, r)
}

// requireJWT is middleware that validates JWT authentication and tracks device activity
func (app *Application) requireJWT(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := httputil.ExtractBearerToken(r)
		if token == "" {
			if app.metrics != nil {
				app.metrics.RecordJWTValidation("error_no_token")
			}
			apperrors.WriteJSON(w, apperrors.ErrUnauthorized)
			return
		}

		_, claims, err := app.services.Auth.ParseJWT(token)
		if err != nil {
			if app.metrics != nil {
				app.metrics.RecordJWTValidation("error_invalid")
			}
			apperrors.WriteJSON(w, apperrors.Wrap(err, "INVALID_TOKEN", "Invalid authentication token", http.StatusUnauthorized))
			return
		}

		// Record successful validation
		if app.metrics != nil {
			app.metrics.RecordJWTValidation("success")
		}

		// Track device activity if storage is available
		if app.storage != nil {
			if deviceID, ok := claims["sub"].(string); ok && deviceID != "" {
				// Update last_seen asynchronously to avoid blocking request
				go func() {
					if err := app.storage.UpdateDeviceLastSeen(deviceID); err != nil {
						proxyLog.Warn("failed to update device last_seen",
							"device_id", deviceID,
							"error", err)
					}
				}()
			}
		}

		next.ServeHTTP(w, r)
	})
}

// Federation Remote Proxy

// getRemoteServiceAddress checks if a service is on a remote peer and returns the peer's address
// Returns (peerAddress, true) if remote, ("", false) if local or not found
func (app *Application) getRemoteServiceAddress(appID string) (string, bool) {
	if app.managers.Peers == nil {
		return "", false
	}

	catalogSyncer := app.managers.Peers.GetCatalogSyncer()
	if catalogSyncer == nil {
		return "", false
	}

	// Get all federated services
	federatedServices, err := catalogSyncer.GetFederatedCatalog()
	if err != nil {
		proxyLog.Error("failed to get federated catalog",
			"error", err,
			"component", logger.CompFederation)
		return "", false
	}

	// Find the service
	for _, service := range federatedServices {
		if service.ServiceID == appID {
			// Check if service is remote (not originated from local peer)
			if service.OriginPeerID != app.managers.Peers.LocalPeerID() {
				// Service is remote - get the peer's address
				peer, err := app.managers.Peers.GetPeerByID(service.OriginPeerID)
				if err != nil {
					proxyLog.Error("failed to get peer",
						"peer_id", service.OriginPeerID,
						"error", err,
						"component", logger.CompFederation)
					return "", false
				}

				// Construct peer's HTTP address
				// Assuming peer is listening on same protocol as us
				peerAddr := fmt.Sprintf("http://%s", peer.Address)
				return peerAddr, true
			}
			// Service is local
			return "", false
		}
	}

	// Service not found in federated catalog
	return "", false
}

// handleRemoteProxy proxies a request to a remote peer's Nexus instance
func (app *Application) handleRemoteProxy(w http.ResponseWriter, r *http.Request, route types.Route, remotePeerAddr string, claims map[string]interface{}) {
	// Check scopes if JWT was provided and route has scope requirements
	effectiveScopes := route.GetEffectiveScopes(app.config.Auth.DefaultScopes)
	if claims != nil && len(effectiveScopes) > 0 {
		if !auth.HasAllScopesFromAny(claims["scopes"], effectiveScopes) {
			apperrors.WriteJSON(w, apperrors.ErrForbidden)
			return
		}
	}

	// Parse remote peer address as target (base URL only)
	// The proxy's SetURL will append r.URL.Path automatically
	target, err := url.Parse(remotePeerAddr)
	if err != nil {
		proxyLog.Error("invalid remote proxy url",
			"error", err,
			"component", logger.CompFederation)
		apperrors.WriteJSON(w, apperrors.ErrBadGateway)
		return
	}

	// Set forwarding headers
	clientIP := httputil.ExtractClientIP(r)
	r.Header.Del("Authorization") // Remove client auth
	r.Header.Set("X-Forwarded-For", clientIP)
	r.Header.Set("X-Forwarded-Host", r.Host)
	r.Header.Set("X-Federated-From", app.nekzusID) // Indicate federation hop
	if r.TLS != nil {
		r.Header.Set("X-Forwarded-Proto", "https")
	} else {
		r.Header.Set("X-Forwarded-Proto", "http")
	}

	proxyLog.Debug("proxying to remote peer",
		"path", r.URL.Path,
		"target_host", target.Host,
		"component", logger.CompFederation)

	// Use HTTP proxy to forward request
	httpProxy := app.proxyCache.GetOrCreate(target)
	r.Host = target.Host
	httpProxy.ServeHTTP(w, r)
}
