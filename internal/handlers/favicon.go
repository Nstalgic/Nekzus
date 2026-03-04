package handlers

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	apperrors "github.com/nstalgic/nekzus/internal/errors"
	"github.com/nstalgic/nekzus/internal/router"
)

var faviconLog = slog.With("package", "handlers.favicon")

// cachedFavicon stores a fetched favicon with metadata
type cachedFavicon struct {
	data        []byte
	contentType string
	fetchedAt   time.Time
	notFound    bool // True if we tried and failed to fetch
}

// AppResolver resolves an app ID to its base URL
type AppResolver func(appID string) (baseURL string, found bool)

// FaviconHandler handles favicon requests for apps
type FaviconHandler struct {
	cache           map[string]*cachedFavicon
	mu              sync.RWMutex
	cacheTTL        time.Duration
	failureCacheTTL time.Duration // Shorter TTL for failed fetches (retry sooner)
	appResolver     AppResolver
	router          *router.Registry
	httpClient      *http.Client
}

// NewFaviconHandler creates a new favicon handler
func NewFaviconHandler(routerRegistry *router.Registry, cacheTTL time.Duration) *FaviconHandler {
	return &FaviconHandler{
		cache:           make(map[string]*cachedFavicon),
		cacheTTL:        cacheTTL,
		failureCacheTTL: 5 * time.Minute, // Retry failed fetches after 5 minutes
		router:          routerRegistry,
		httpClient: &http.Client{
			Timeout: 10 * time.Second, // Increased timeout for mobile networks
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 3 {
					return http.ErrUseLastResponse
				}
				return nil
			},
		},
	}
}

// SetAppResolver sets a custom app resolver (for testing)
func (h *FaviconHandler) SetAppResolver(resolver AppResolver) {
	h.appResolver = resolver
}

// HandleFavicon serves the favicon for an app
// GET /api/v1/apps/{appId}/favicon
func (h *FaviconHandler) HandleFavicon(w http.ResponseWriter, r *http.Request, appID string) {
	if r.Method != http.MethodGet {
		apperrors.WriteJSON(w, apperrors.New("METHOD_NOT_ALLOWED", "Method not allowed", http.StatusMethodNotAllowed))
		return
	}

	// Resolve app URL and path prefix
	baseURL, pathPrefix, found := h.resolveAppURLWithPath(appID)
	if !found {
		apperrors.WriteJSON(w, apperrors.New("APP_NOT_FOUND", "Application not found", http.StatusNotFound))
		return
	}

	// Check cache
	h.mu.RLock()
	cached, exists := h.cache[appID]
	h.mu.RUnlock()

	if exists {
		// Use shorter TTL for failures to allow retries sooner
		ttl := h.cacheTTL
		if cached.notFound {
			ttl = h.failureCacheTTL
		}

		if time.Since(cached.fetchedAt) < ttl {
			if cached.notFound {
				apperrors.WriteJSON(w, apperrors.New("FAVICON_NOT_FOUND", "Favicon not available", http.StatusNotFound))
				return
			}
			h.serveFavicon(w, cached)
			return
		}
	}

	// Fetch favicon
	favicon, err := h.fetchFavicon(baseURL, pathPrefix)
	if err != nil {
		faviconLog.Debug("Failed to fetch favicon", "app_id", appID, "error", err)
		// Cache the failure to avoid repeated attempts
		h.mu.Lock()
		h.cache[appID] = &cachedFavicon{
			notFound:  true,
			fetchedAt: time.Now(),
		}
		h.mu.Unlock()
		apperrors.WriteJSON(w, apperrors.New("FAVICON_NOT_FOUND", "Favicon not available", http.StatusNotFound))
		return
	}

	// Cache and serve
	h.mu.Lock()
	h.cache[appID] = favicon
	h.mu.Unlock()

	h.serveFavicon(w, favicon)
}

// resolveAppURL resolves an app ID to its upstream URL (for testing compatibility)
func (h *FaviconHandler) resolveAppURL(appID string) (string, bool) {
	baseURL, _, found := h.resolveAppURLWithPath(appID)
	return baseURL, found
}

// resolveAppURLWithPath resolves an app ID to its upstream URL and proxy path prefix
func (h *FaviconHandler) resolveAppURLWithPath(appID string) (baseURL string, pathPrefix string, found bool) {
	// Use custom resolver if set (for testing)
	if h.appResolver != nil {
		url, ok := h.appResolver(appID)
		return url, "", ok
	}

	// Use router to find the app's route
	if h.router == nil {
		return "", "", false
	}

	route, ok := h.router.GetRouteByAppID(appID)
	if !ok || route == nil {
		return "", "", false
	}

	return route.To, route.PathBase, true
}

// fetchFavicon attempts to fetch the favicon from multiple sources
// pathPrefix is the proxy path (e.g., "/apps/gitea/") used to strip from icon URLs
func (h *FaviconHandler) fetchFavicon(baseURL, pathPrefix string) (*cachedFavicon, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Strategy 1: Try /favicon.ico
	if favicon, err := h.tryFetch(ctx, baseURL+"/favicon.ico"); err == nil {
		return favicon, nil
	}

	// Strategy 2: Try /favicon.png
	if favicon, err := h.tryFetch(ctx, baseURL+"/favicon.png"); err == nil {
		return favicon, nil
	}

	// Strategy 3: Parse HTML for icon link
	// parseHTMLForIcon returns both the icon URL and the final base URL after redirects
	// This is important for apps like Transmission that redirect to a subpath
	iconURL, finalBaseURL := h.parseHTMLForIcon(ctx, baseURL)
	if iconURL != "" {
		// Strip proxy path prefix if present (e.g., "/apps/gitea/assets/..." -> "/assets/...")
		// This handles apps behind reverse proxy that return URLs with the proxy prefix
		if pathPrefix != "" && strings.HasPrefix(iconURL, pathPrefix) {
			iconURL = "/" + strings.TrimPrefix(iconURL, pathPrefix)
		}

		// Make absolute URL if relative
		// Use finalBaseURL (after redirects) for proper resolution
		if strings.HasPrefix(iconURL, "./") {
			// Relative to current path: "./images/favicon.ico"
			iconURL = finalBaseURL + strings.TrimPrefix(iconURL, "./")
		} else if strings.HasPrefix(iconURL, "/") {
			// Absolute path: "/images/favicon.ico" - use original baseURL
			iconURL = baseURL + iconURL
		} else if !strings.HasPrefix(iconURL, "http") {
			// Other relative path: "images/favicon.ico"
			iconURL = finalBaseURL + iconURL
		}

		if favicon, err := h.tryFetch(ctx, iconURL); err == nil {
			return favicon, nil
		}
	}

	return nil, http.ErrNotSupported
}

// tryFetch attempts to fetch a favicon from a URL
func (h *FaviconHandler) tryFetch(ctx context.Context, url string) (*cachedFavicon, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", "Nekzus/1.0")
	req.Header.Set("Accept", "image/*")

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, http.ErrNotSupported
	}

	// Check content type is an image
	contentType := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "image/") {
		return nil, http.ErrNotSupported
	}

	// Read body (limit to 1MB)
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if err != nil {
		return nil, err
	}

	return &cachedFavicon{
		data:        data,
		contentType: contentType,
		fetchedAt:   time.Now(),
	}, nil
}

// parseHTMLForIcon fetches the HTML and parses for icon links
// Returns the icon URL and the final URL after redirects (for resolving relative paths)
func (h *FaviconHandler) parseHTMLForIcon(ctx context.Context, baseURL string) (iconURL string, finalBaseURL string) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL, nil)
	if err != nil {
		return "", baseURL
	}

	req.Header.Set("User-Agent", "Nekzus/1.0")
	req.Header.Set("Accept", "text/html")

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return "", baseURL
	}
	defer resp.Body.Close()

	// Get the final URL after redirects (important for apps like Transmission that redirect)
	finalURL := resp.Request.URL.String()
	// Extract base URL (scheme + host + path without file)
	if idx := strings.LastIndex(finalURL, "/"); idx > 8 { // After "https://"
		finalBaseURL = finalURL[:idx+1]
	} else {
		finalBaseURL = finalURL
		if !strings.HasSuffix(finalBaseURL, "/") {
			finalBaseURL += "/"
		}
	}

	if resp.StatusCode != http.StatusOK {
		return "", finalBaseURL
	}

	// Read limited HTML (first 64KB should contain head)
	data, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return "", finalBaseURL
	}

	return parseFaviconFromHTML(string(data)), finalBaseURL
}

// serveFavicon writes the cached favicon to the response
func (h *FaviconHandler) serveFavicon(w http.ResponseWriter, favicon *cachedFavicon) {
	w.Header().Set("Content-Type", favicon.contentType)
	w.Header().Set("Cache-Control", "public, max-age=86400") // 24 hours
	w.Header().Set("X-Content-Type-Options", "nosniff")
	// CORS headers for mobile app access
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET")
	w.Write(favicon.data)
}

// parseFaviconFromHTML extracts the favicon URL from HTML
func parseFaviconFromHTML(html string) string {
	// Pattern to match link tags with rel containing "icon"
	// Matches: rel="icon", rel="shortcut icon", rel="apple-touch-icon"
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`<link[^>]+rel=["']([^"']*icon[^"']*)["'][^>]+href=["']([^"']+)["']`),
		regexp.MustCompile(`<link[^>]+href=["']([^"']+)["'][^>]+rel=["']([^"']*icon[^"']*)["']`),
	}

	for _, pattern := range patterns {
		matches := pattern.FindStringSubmatch(html)
		if len(matches) >= 3 {
			// First pattern: rel is match[1], href is match[2]
			// Second pattern: href is match[1], rel is match[2]
			if strings.Contains(matches[1], "icon") {
				return matches[2]
			}
			if strings.Contains(matches[2], "icon") {
				return matches[1]
			}
		}
	}

	return ""
}

// ClearCache clears the favicon cache (for testing or manual refresh)
func (h *FaviconHandler) ClearCache() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.cache = make(map[string]*cachedFavicon)
}

// ClearCacheForApp clears the cached favicon for a specific app
func (h *FaviconHandler) ClearCacheForApp(appID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.cache, appID)
}
