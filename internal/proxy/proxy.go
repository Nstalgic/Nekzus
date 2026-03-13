package proxy

import (
	"container/list"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"
)

var log = slog.With("package", "proxy")

// cacheEntry wraps a reverse proxy with its LRU list element for O(1) access
type cacheEntry struct {
	proxy       *httputil.ReverseProxy
	listElement *list.Element
}

// Cache manages reverse proxy instances with optional LRU eviction
// Uses container/list for O(1) LRU operations instead of O(n) slice operations
type Cache struct {
	mu        sync.RWMutex
	proxies   map[string]*cacheEntry
	lruList   *list.List // Doubly-linked list for O(1) LRU operations
	maxSize   int        // 0 means no limit
	evictions int        // Track number of evictions
}

// NewCache creates a new proxy cache with no size limit
func NewCache() *Cache {
	return &Cache{
		proxies: make(map[string]*cacheEntry),
		lruList: list.New(),
	}
}

// NewCacheWithMaxSize creates a proxy cache with LRU eviction
func NewCacheWithMaxSize(maxSize int) *Cache {
	return &Cache{
		proxies: make(map[string]*cacheEntry, maxSize),
		lruList: list.New(),
		maxSize: maxSize,
	}
}

// GetOrCreate returns a cached proxy or creates a new one
func (c *Cache) GetOrCreate(target *url.URL) *httputil.ReverseProxy {
	key := target.String()

	// Fast path: check with read lock
	c.mu.RLock()
	if entry, ok := c.proxies[key]; ok {
		c.mu.RUnlock()
		// Update access order for LRU (requires write lock for O(1) operation)
		if c.maxSize > 0 {
			c.mu.Lock()
			c.lruList.MoveToFront(entry.listElement)
			c.mu.Unlock()
		}
		return entry.proxy
	}
	c.mu.RUnlock()

	// Slow path: create proxy with write lock
	c.mu.Lock()
	defer c.mu.Unlock()

	// Double-check in case another goroutine created it
	if entry, ok := c.proxies[key]; ok {
		if c.maxSize > 0 {
			c.lruList.MoveToFront(entry.listElement)
		}
		return entry.proxy
	}

	// LRU eviction if at capacity - O(1) operation with list
	if c.maxSize > 0 && len(c.proxies) >= c.maxSize {
		// Evict least recently used (back of list)
		oldest := c.lruList.Back()
		if oldest != nil {
			oldestKey := oldest.Value.(string)
			delete(c.proxies, oldestKey)
			c.lruList.Remove(oldest)
			c.evictions++
		}
	}

	// Configure transport with proper timeouts and connection pool limits
	// HTTP client timeouts
	// TLS MinVersion
	// Connection pool limits
	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
		},
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   10,
		MaxConnsPerHost:       100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
	}

	// Create new proxy using Rewrite instead of Director
	// Rewrite gives us full control over headers without automatic X-Forwarded-For appending
	// Note: Rewrite automatically removes X-Forwarded-* headers, so we need to restore them
	proxy := &httputil.ReverseProxy{
		Transport: transport,
		Rewrite: func(pr *httputil.ProxyRequest) {
			// Save our X-Forwarded-* headers that we set in the handler
			// (Rewrite removes them automatically before calling this function)
			xForwardedFor := pr.In.Header.Get("X-Forwarded-For")
			xForwardedProto := pr.In.Header.Get("X-Forwarded-Proto")
			xForwardedHost := pr.In.Header.Get("X-Forwarded-Host")
			xForwardedPort := pr.In.Header.Get("X-Forwarded-Port")
			xRealIP := pr.In.Header.Get("X-Real-IP")
			xForwardedPrefix := pr.In.Header.Get("X-Forwarded-Prefix")

			// Set target URL
			pr.SetURL(target)

			// Preserve original Host header (standard reverse proxy behavior)
			// This allows origin-validating apps to see the proxy's host, not the backend's
			pr.Out.Host = pr.In.Host

			// Restore our X-Forwarded-* headers
			if xForwardedFor != "" {
				pr.Out.Header.Set("X-Forwarded-For", xForwardedFor)
			}
			if xForwardedProto != "" {
				pr.Out.Header.Set("X-Forwarded-Proto", xForwardedProto)
			}
			if xForwardedHost != "" {
				pr.Out.Header.Set("X-Forwarded-Host", xForwardedHost)
			}
			if xForwardedPort != "" {
				pr.Out.Header.Set("X-Forwarded-Port", xForwardedPort)
			}
			if xRealIP != "" {
				pr.Out.Header.Set("X-Real-IP", xRealIP)
			}
			if xForwardedPrefix != "" {
				pr.Out.Header.Set("X-Forwarded-Prefix", xForwardedPrefix)
			}
		},
		FlushInterval: -1, // Immediate flush for SSE support
	}

	// Custom error handler with granular error mapping
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		// Map error to appropriate status code
		statusCode := MapErrorToStatus(err)
		errorLabel := GetErrorLabel(err)
		errorMessage := GetErrorMessage(statusCode)

		// Log internally with details (including error type)
		log.Error("proxy error",
			"target_host", target.Host,
			"error_type", errorLabel,
			"status", statusCode,
			"error", err)

		// Return user-friendly error to client (no upstream details leaked)
		http.Error(w, errorMessage, statusCode)
	}

	// Create cache entry with LRU tracking - O(1) operation
	entry := &cacheEntry{
		proxy: proxy,
	}

	// Track access order for LRU (most recently used at front)
	if c.maxSize > 0 {
		entry.listElement = c.lruList.PushFront(key)
	}

	c.proxies[key] = entry

	return proxy
}

// Clear removes all cached proxies
func (c *Cache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.proxies = make(map[string]*cacheEntry)
	c.lruList = list.New()
}

// Remove removes a specific proxy from cache
func (c *Cache) Remove(targetURL string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if entry, ok := c.proxies[targetURL]; ok {
		// Remove from LRU list if tracking
		if c.maxSize > 0 && entry.listElement != nil {
			c.lruList.Remove(entry.listElement)
		}
		delete(c.proxies, targetURL)
	}
}

// Stats returns cache statistics
func (c *Cache) Stats() map[string]int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	stats := map[string]int{
		"cached_proxies": len(c.proxies),
	}
	if c.maxSize > 0 {
		stats["evictions"] = c.evictions
	}
	return stats
}

// Delete removes a proxy from the cache by target URL
func (c *Cache) Delete(target *url.URL) {
	key := target.String()

	c.mu.Lock()
	defer c.mu.Unlock()

	if entry, ok := c.proxies[key]; ok {
		delete(c.proxies, key)
		if c.maxSize > 0 && entry.listElement != nil {
			c.lruList.Remove(entry.listElement)
		}
	}
}

// CacheWithCA is a proxy cache that includes custom Certificate Authority support
// for trusting self-signed certificates from Nekzus CA
type CacheWithCA struct {
	*Cache
	mu      sync.RWMutex
	rootCAs *x509.CertPool
}

// NewCacheWithCA creates a new proxy cache with CA support
func NewCacheWithCA() *CacheWithCA {
	// Start with system CAs
	rootCAs, err := x509.SystemCertPool()
	if err != nil {
		log.Warn("failed to load system cert pool, creating empty pool", "error", err)
		rootCAs = x509.NewCertPool()
	}

	return &CacheWithCA{
		Cache:   NewCache(),
		rootCAs: rootCAs,
	}
}

// AddCA adds a CA certificate to the trusted pool
func (c *CacheWithCA) AddCA(caCertPEM []byte) error {
	if len(caCertPEM) == 0 {
		return fmt.Errorf("CA certificate is empty")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.rootCAs.AppendCertsFromPEM(caCertPEM) {
		return fmt.Errorf("failed to parse CA certificate")
	}

	log.Info("added CA certificate to proxy trust pool")
	return nil
}

// RootCAs returns the current root CA pool
func (c *CacheWithCA) RootCAs() *x509.CertPool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.rootCAs
}

// GetOrCreate returns a cached proxy or creates a new one with custom CA trust
func (c *CacheWithCA) GetOrCreate(target *url.URL) *httputil.ReverseProxy {
	key := target.String()

	// Fast path: check with read lock
	c.Cache.mu.RLock()
	if entry, ok := c.Cache.proxies[key]; ok {
		c.Cache.mu.RUnlock()
		return entry.proxy
	}
	c.Cache.mu.RUnlock()

	// Slow path: create proxy with write lock
	c.Cache.mu.Lock()
	defer c.Cache.mu.Unlock()

	// Double-check
	if entry, ok := c.Cache.proxies[key]; ok {
		return entry.proxy
	}

	// Get current RootCAs
	c.mu.RLock()
	rootCAs := c.rootCAs
	c.mu.RUnlock()

	// Configure transport with custom CA trust
	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
			RootCAs:    rootCAs,
		},
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   10,
		MaxConnsPerHost:       100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
	}

	// Create proxy
	proxy := &httputil.ReverseProxy{
		Transport: transport,
		Rewrite: func(pr *httputil.ProxyRequest) {
			// Save X-Forwarded-* headers
			xForwardedFor := pr.In.Header.Get("X-Forwarded-For")
			xForwardedProto := pr.In.Header.Get("X-Forwarded-Proto")
			xForwardedHost := pr.In.Header.Get("X-Forwarded-Host")
			xForwardedPort := pr.In.Header.Get("X-Forwarded-Port")
			xRealIP := pr.In.Header.Get("X-Real-IP")
			xForwardedPrefix := pr.In.Header.Get("X-Forwarded-Prefix")

			pr.SetURL(target)

			// Preserve original Host header (standard reverse proxy behavior)
			pr.Out.Host = pr.In.Host

			// Restore headers
			if xForwardedFor != "" {
				pr.Out.Header.Set("X-Forwarded-For", xForwardedFor)
			}
			if xForwardedProto != "" {
				pr.Out.Header.Set("X-Forwarded-Proto", xForwardedProto)
			}
			if xForwardedHost != "" {
				pr.Out.Header.Set("X-Forwarded-Host", xForwardedHost)
			}
			if xForwardedPort != "" {
				pr.Out.Header.Set("X-Forwarded-Port", xForwardedPort)
			}
			if xRealIP != "" {
				pr.Out.Header.Set("X-Real-IP", xRealIP)
			}
			if xForwardedPrefix != "" {
				pr.Out.Header.Set("X-Forwarded-Prefix", xForwardedPrefix)
			}
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			log.Error("proxy error", "target", target.String(), "error", err)
			statusCode := http.StatusBadGateway
			errorMessage := "Bad Gateway"

			if err.Error() == "context canceled" || strings.Contains(err.Error(), "context deadline exceeded") {
				statusCode = http.StatusGatewayTimeout
				errorMessage = "Gateway Timeout"
			}

			http.Error(w, errorMessage, statusCode)
		},
	}

	// Cache entry
	c.Cache.proxies[key] = &cacheEntry{proxy: proxy}

	return proxy
}

// SanitizeHeaders removes sensitive headers before proxying
func SanitizeHeaders(r *http.Request) {
	// Always remove authentication headers
	r.Header.Del("Authorization")
	r.Header.Del("Cookie")
	r.Header.Del("X-Api-Key")
	r.Header.Del("X-Auth-Token")
}

// SetForwardedHeaders sets standard forwarded headers
func SetForwardedHeaders(r *http.Request, clientIP string) {
	r.Header.Set("X-Forwarded-For", clientIP)
	r.Header.Set("X-Forwarded-Host", r.Host)

	// Determine protocol and default port
	var proto, defaultPort string
	if r.TLS != nil {
		proto = "https"
		defaultPort = "443"
	} else {
		proto = "http"
		defaultPort = "80"
	}
	r.Header.Set("X-Forwarded-Proto", proto)

	// Set X-Forwarded-Port
	// Extract port from Host header, or use default
	_, port, err := net.SplitHostPort(r.Host)
	if err != nil {
		// No port in Host header, use default based on protocol
		port = defaultPort
	}
	r.Header.Set("X-Forwarded-Port", port)
}

// hopByHopHeaders lists headers that should not be forwarded to the backend (RFC 2616)
var hopByHopHeaders = []string{
	"Connection",
	"Keep-Alive",
	"Proxy-Authenticate",
	"Proxy-Authorization",
	"Proxy-Connection",
	"Te",
	"Trailer",
	"Transfer-Encoding",
	"Upgrade",
}

// FormatProxyError formats a proxy error message with optional request ID
func FormatProxyError(requestID, host string, err error) string {
	if requestID != "" {
		return fmt.Sprintf("Proxy error [%s] for %s: %v", requestID, host, err)
	}
	return fmt.Sprintf("Proxy error for %s: %v", host, err)
}

// RemoveHopByHopHeaders removes hop-by-hop headers before proxying
// These headers are connection-specific and should not be forwarded.
func RemoveHopByHopHeaders(header http.Header) {
	// First, get headers listed in Connection header (these are also hop-by-hop)
	connectionHeaders := header.Get("Connection")
	if connectionHeaders != "" {
		for _, h := range strings.Split(connectionHeaders, ",") {
			header.Del(strings.TrimSpace(h))
		}
	}

	// Remove standard hop-by-hop headers
	for _, h := range hopByHopHeaders {
		header.Del(h)
	}
}
