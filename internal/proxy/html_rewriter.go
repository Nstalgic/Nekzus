package proxy

import (
	"bufio"
	"bytes"
	"compress/flate"
	"compress/gzip"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// MaxHTMLBufferSize is the maximum size of HTML content that will be buffered for rewriting
// HTML content larger than this will be passed through without rewriting to prevent OOM
const MaxHTMLBufferSize = 10 * 1024 * 1024 // 10MB

// Ensure HTMLRewritingResponseWriter implements http.Flusher and http.Hijacker
var _ http.Flusher = (*HTMLRewritingResponseWriter)(nil)
var _ http.Hijacker = (*HTMLRewritingResponseWriter)(nil)

// HTMLRewritingResponseWriter wraps http.ResponseWriter to rewrite HTML responses
type HTMLRewritingResponseWriter struct {
	http.ResponseWriter
	pathPrefix      string // Route's pathBase (for JS interceptor)
	requestPath     string // Actual request path (for base href)
	requestHost     string // Original request's Host header (for absolute URL rewriting)
	requestScheme   string // Original request's scheme (http/https)
	rewriteBody     bool   // Whether to rewrite HTML body content (headers are always rewritten)
	buffer          *bytes.Buffer
	statusCode      int
	isHTML          bool
	isCSS           bool
	isJSON          bool
	headerSent      bool
	contentEncoding string // gzip, deflate, or empty
}

// NewHTMLRewritingResponseWriter creates a new HTML rewriting response writer
// pathPrefix is the route's pathBase (used for JS interceptor to rewrite absolute paths)
// requestPath is the actual request URL path (used for base href to resolve relative paths)
func NewHTMLRewritingResponseWriter(w http.ResponseWriter, pathPrefix string, requestPath string) *HTMLRewritingResponseWriter {
	return &HTMLRewritingResponseWriter{
		ResponseWriter: w,
		pathPrefix:     pathPrefix,
		requestPath:    requestPath,
		rewriteBody:    true,
		buffer:         &bytes.Buffer{},
		statusCode:     http.StatusOK,
		isHTML:         false,
		headerSent:     false,
	}
}

// NewHTMLRewritingResponseWriterWithHost creates a new HTML rewriting response writer with host info
// This version also stores the original request's host and scheme for absolute URL rewriting in redirects
func NewHTMLRewritingResponseWriterWithHost(w http.ResponseWriter, pathPrefix, requestPath, requestHost, requestScheme string) *HTMLRewritingResponseWriter {
	return &HTMLRewritingResponseWriter{
		ResponseWriter: w,
		pathPrefix:     pathPrefix,
		requestPath:    requestPath,
		requestHost:    requestHost,
		requestScheme:  requestScheme,
		rewriteBody:    true,
		buffer:         &bytes.Buffer{},
		statusCode:     http.StatusOK,
		isHTML:         false,
		headerSent:     false,
	}
}

// NewHeaderRewritingResponseWriter creates a response writer that only rewrites headers (Location, Refresh, etc.)
// but does NOT rewrite HTML body content. Use this when StripPrefix is enabled but RewriteHTML is disabled.
func NewHeaderRewritingResponseWriter(w http.ResponseWriter, pathPrefix, requestHost, requestScheme string) *HTMLRewritingResponseWriter {
	return &HTMLRewritingResponseWriter{
		ResponseWriter: w,
		pathPrefix:     pathPrefix,
		requestPath:    "",
		requestHost:    requestHost,
		requestScheme:  requestScheme,
		rewriteBody:    false,
		buffer:         &bytes.Buffer{},
		statusCode:     http.StatusOK,
		isHTML:         false,
		headerSent:     false,
	}
}

// WriteHeader captures the status code and checks Content-Type
func (rw *HTMLRewritingResponseWriter) WriteHeader(statusCode int) {
	if rw.headerSent {
		return
	}
	rw.statusCode = statusCode

	// Rewrite Location header for redirects (3xx status codes)
	if statusCode >= 300 && statusCode < 400 {
		if location := rw.ResponseWriter.Header().Get("Location"); location != "" {
			newLocation := rw.rewriteLocationHeader(location)
			if newLocation != location {
				rw.ResponseWriter.Header().Set("Location", newLocation)
			}
		}
	}

	// Rewrite Refresh header (e.g., "5; url=/path")
	if refresh := rw.ResponseWriter.Header().Get("Refresh"); refresh != "" {
		newRefresh := rw.rewriteRefreshHeader(refresh)
		if newRefresh != refresh {
			rw.ResponseWriter.Header().Set("Refresh", newRefresh)
		}
	}

	// Rewrite Content-Location header
	if contentLoc := rw.ResponseWriter.Header().Get("Content-Location"); contentLoc != "" {
		newContentLoc := rw.rewriteLocationHeader(contentLoc)
		if newContentLoc != contentLoc {
			rw.ResponseWriter.Header().Set("Content-Location", newContentLoc)
		}
	}

	// Rewrite Link headers (RFC 8288) - can have multiple values
	if linkHeaders := rw.ResponseWriter.Header().Values("Link"); len(linkHeaders) > 0 {
		rw.ResponseWriter.Header().Del("Link")
		for _, link := range linkHeaders {
			newLink := rw.rewriteLinkHeader(link)
			rw.ResponseWriter.Header().Add("Link", newLink)
		}
	}

	// Rewrite Content-Security-Policy headers
	if csp := rw.ResponseWriter.Header().Get("Content-Security-Policy"); csp != "" {
		newCSP := rw.rewriteCSPHeader(csp)
		if newCSP != csp {
			rw.ResponseWriter.Header().Set("Content-Security-Policy", newCSP)
		}
	}
	if cspRO := rw.ResponseWriter.Header().Get("Content-Security-Policy-Report-Only"); cspRO != "" {
		newCSPRO := rw.rewriteCSPHeader(cspRO)
		if newCSPRO != cspRO {
			rw.ResponseWriter.Header().Set("Content-Security-Policy-Report-Only", newCSPRO)
		}
	}

	// Check if this is HTML, CSS, or JSON/JS content that should be rewritten
	// Only buffer if rewriteBody is enabled
	contentType := rw.ResponseWriter.Header().Get("Content-Type")
	rw.isHTML = rw.rewriteBody && strings.HasPrefix(contentType, "text/html")
	rw.isCSS = rw.rewriteBody && strings.HasPrefix(contentType, "text/css")
	// Buffer JSON and small JS responses for urlBase rewriting.
	// JS config files like initialize.js contain urlBase that needs patching.
	// Large JS bundles are skipped (Content-Length > 50KB) to avoid buffering overhead.
	isJSONType := strings.HasPrefix(contentType, "application/json")
	isJSType := strings.HasPrefix(contentType, "application/javascript") || strings.HasPrefix(contentType, "text/javascript")
	if isJSType {
		// Only buffer small JS files (config files are typically < 1KB).
		// Skip if Content-Length is missing (chunked = likely a large bundle) or > 50KB.
		cl := rw.ResponseWriter.Header().Get("Content-Length")
		if cl == "" {
			isJSType = false
		} else if size, err := strconv.Atoi(cl); err != nil || size > 50*1024 {
			isJSType = false
		}
	}
	rw.isJSON = rw.rewriteBody && (isJSONType || isJSType)

	// Capture content encoding for decompression during rewrite
	rw.contentEncoding = strings.ToLower(rw.ResponseWriter.Header().Get("Content-Encoding"))

	// If not a rewritable content type, write headers immediately and don't buffer
	if !rw.isHTML && !rw.isCSS && !rw.isJSON {
		rw.ResponseWriter.WriteHeader(statusCode)
		rw.headerSent = true
	}
}

// rewriteLocationHeader rewrites a Location header value to include the path prefix
// Handles both relative paths (/path) and absolute URLs (http://host/path)
func (rw *HTMLRewritingResponseWriter) rewriteLocationHeader(location string) string {
	// Handle relative paths starting with /
	if strings.HasPrefix(location, "/") && !strings.HasPrefix(location, "//") {
		// Don't rewrite if already has the prefix
		if !strings.HasPrefix(location, rw.pathPrefix) {
			return rw.pathPrefix + strings.TrimPrefix(location, "/")
		}
		return location
	}

	// Handle absolute URLs (http:// or https://)
	if strings.HasPrefix(location, "http://") || strings.HasPrefix(location, "https://") {
		parsedURL, err := url.Parse(location)
		if err != nil {
			// Can't parse, return as-is
			return location
		}

		// Check if this URL points to the same host as the original request
		// This handles cases where the backend returns absolute URLs using X-Forwarded-Host
		if rw.requestHost != "" && parsedURL.Host == rw.requestHost {
			// Same host - rewrite the path
			if !strings.HasPrefix(parsedURL.Path, rw.pathPrefix) {
				parsedURL.Path = rw.pathPrefix + strings.TrimPrefix(parsedURL.Path, "/")
				return parsedURL.String()
			}
		}

		// Also check if the URL's host matches common proxy patterns
		// Some apps construct URLs using the port from X-Forwarded-Port
		if rw.requestHost != "" {
			// Extract host without port from both
			reqHostOnly, _, _ := net.SplitHostPort(rw.requestHost)
			if reqHostOnly == "" {
				reqHostOnly = rw.requestHost
			}
			urlHostOnly, _, _ := net.SplitHostPort(parsedURL.Host)
			if urlHostOnly == "" {
				urlHostOnly = parsedURL.Host
			}

			// If hosts match (ignoring port), rewrite the path
			if reqHostOnly == urlHostOnly {
				if !strings.HasPrefix(parsedURL.Path, rw.pathPrefix) {
					parsedURL.Path = rw.pathPrefix + strings.TrimPrefix(parsedURL.Path, "/")
					return parsedURL.String()
				}
			}
		}
	}

	// No rewriting needed
	return location
}

// rewriteRefreshHeader rewrites a Refresh header value (e.g., "5; url=/path")
func (rw *HTMLRewritingResponseWriter) rewriteRefreshHeader(refresh string) string {
	// Refresh header format: "seconds; url=path" or just "seconds"
	// The url= part is case-insensitive

	// Find url= (case-insensitive)
	lowerRefresh := strings.ToLower(refresh)
	urlIdx := strings.Index(lowerRefresh, "url=")
	if urlIdx == -1 {
		// No URL component, return as-is
		return refresh
	}

	// Extract the URL part (preserving original case)
	prefix := refresh[:urlIdx+4] // "5; url=" or "5; URL="
	urlPart := refresh[urlIdx+4:]

	// Rewrite the URL using the same logic as Location header
	newURL := rw.rewriteLocationHeader(urlPart)

	return prefix + newURL
}

// rewriteLinkHeader rewrites RFC 8288 Link header URLs
// Format: <url>; rel="relation", <url2>; rel="relation2"
func (rw *HTMLRewritingResponseWriter) rewriteLinkHeader(link string) string {
	// Link header can contain multiple links separated by commas
	// Each link is: <url>; param1; param2
	// We need to be careful not to split on commas inside angle brackets

	var result strings.Builder
	remaining := link

	for len(remaining) > 0 {
		// Find the URL part between < and >
		startIdx := strings.Index(remaining, "<")
		if startIdx == -1 {
			result.WriteString(remaining)
			break
		}

		endIdx := strings.Index(remaining, ">")
		if endIdx == -1 || endIdx < startIdx {
			result.WriteString(remaining)
			break
		}

		// Write everything before <
		result.WriteString(remaining[:startIdx])

		// Extract and rewrite the URL
		urlPart := remaining[startIdx+1 : endIdx]
		newURL := rw.rewriteLocationHeader(urlPart)

		// Write the rewritten URL with angle brackets
		result.WriteString("<")
		result.WriteString(newURL)
		result.WriteString(">")

		// Move past the >
		remaining = remaining[endIdx+1:]

		// Find the next link (after comma) or end
		commaIdx := strings.Index(remaining, ",")
		if commaIdx == -1 {
			// No more links, write remaining params
			result.WriteString(remaining)
			break
		}

		// Check if there's another < before the comma (meaning comma is inside params)
		nextLinkIdx := strings.Index(remaining, "<")
		if nextLinkIdx != -1 && nextLinkIdx < commaIdx {
			// The < comes before comma, which means we haven't found a separator yet
			// Write up to the next < and continue
			result.WriteString(remaining[:nextLinkIdx])
			remaining = remaining[nextLinkIdx:]
		} else {
			// Write params including the comma and space after it
			result.WriteString(remaining[:commaIdx+1])
			remaining = remaining[commaIdx+1:]
			// Preserve space after comma if present, otherwise add one
			if len(remaining) > 0 && remaining[0] == ' ' {
				result.WriteString(" ")
				remaining = remaining[1:]
			} else if len(remaining) > 0 {
				result.WriteString(" ")
			}
		}
	}

	return result.String()
}

// rewriteCSPHeader rewrites paths in Content-Security-Policy headers
func (rw *HTMLRewritingResponseWriter) rewriteCSPHeader(csp string) string {
	// CSP format: directive1 value1 value2; directive2 value1 value2
	// We need to rewrite absolute paths (/path) but preserve:
	// - 'self', 'unsafe-inline', 'unsafe-eval', 'none', 'strict-dynamic'
	// - 'nonce-xxx', 'sha256-xxx', 'sha384-xxx', 'sha512-xxx'
	// - https://, http://, data:, blob:, ws:, wss:
	// - *.domain.com patterns

	// Split by semicolon to get directives
	directives := strings.Split(csp, ";")
	var result []string

	for _, directive := range directives {
		directive = strings.TrimSpace(directive)
		if directive == "" {
			continue
		}

		// Split directive into parts
		parts := strings.Fields(directive)
		if len(parts) == 0 {
			continue
		}

		// First part is the directive name
		directiveName := parts[0]
		var newParts []string
		newParts = append(newParts, directiveName)

		// Process remaining parts (values)
		for _, part := range parts[1:] {
			newPart := rw.rewriteCSPValue(part)
			newParts = append(newParts, newPart)
		}

		result = append(result, strings.Join(newParts, " "))
	}

	return strings.Join(result, "; ")
}

// rewriteCSPValue rewrites a single CSP value if it's a path
func (rw *HTMLRewritingResponseWriter) rewriteCSPValue(value string) string {
	// Skip special CSP keywords (quoted)
	if strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'") {
		return value
	}

	// Skip URLs with schemes
	if strings.Contains(value, "://") || strings.HasPrefix(value, "data:") ||
		strings.HasPrefix(value, "blob:") || strings.HasPrefix(value, "ws:") ||
		strings.HasPrefix(value, "wss:") {
		return value
	}

	// Skip wildcard domain patterns
	if strings.HasPrefix(value, "*.") || strings.Contains(value, ".") && !strings.HasPrefix(value, "/") {
		return value
	}

	// Rewrite absolute paths
	if strings.HasPrefix(value, "/") && !strings.HasPrefix(value, "//") {
		if !strings.HasPrefix(value, rw.pathPrefix) {
			return rw.pathPrefix + strings.TrimPrefix(value, "/")
		}
	}

	return value
}

// Write buffers HTML content or writes directly for non-HTML
func (rw *HTMLRewritingResponseWriter) Write(data []byte) (int, error) {
	// If headers weren't sent yet, send them now (with default 200)
	if !rw.headerSent && rw.statusCode == 0 {
		rw.WriteHeader(http.StatusOK)
	}

	// If not a rewritable content type, write directly
	if !rw.isHTML && !rw.isCSS && !rw.isJSON {
		return rw.ResponseWriter.Write(data)
	}

	// Buffer content for rewriting
	return rw.buffer.Write(data)
}

// FlushHTML rewrites HTML/CSS/JSON and sends the final response
func (rw *HTMLRewritingResponseWriter) FlushHTML() error {
	// If not a rewritable type or already sent, nothing to do
	if (!rw.isHTML && !rw.isCSS && !rw.isJSON) || rw.headerSent {
		return nil
	}

	// Get the buffered content
	bufferedData := rw.buffer.Bytes()
	var content string

	// Decompress if needed
	if rw.contentEncoding == "gzip" {
		reader, err := gzip.NewReader(bytes.NewReader(bufferedData))
		if err != nil {
			log.Warn("Rewriter failed to decompress gzip, passing through", "error", err)
			rw.ResponseWriter.WriteHeader(rw.statusCode)
			rw.headerSent = true
			_, writeErr := rw.ResponseWriter.Write(bufferedData)
			return writeErr
		}
		defer reader.Close()

		decompressed, err := io.ReadAll(reader)
		if err != nil {
			log.Warn("Rewriter gzip read failed, passing through", "error", err)
			rw.ResponseWriter.WriteHeader(rw.statusCode)
			rw.headerSent = true
			_, writeErr := rw.ResponseWriter.Write(bufferedData)
			return writeErr
		}
		content = string(decompressed)
	} else if rw.contentEncoding == "deflate" {
		reader := flate.NewReader(bytes.NewReader(bufferedData))
		defer reader.Close()

		decompressed, err := io.ReadAll(reader)
		if err != nil {
			log.Warn("Rewriter deflate read failed, passing through", "error", err)
			rw.ResponseWriter.WriteHeader(rw.statusCode)
			rw.headerSent = true
			_, writeErr := rw.ResponseWriter.Write(bufferedData)
			return writeErr
		}
		content = string(decompressed)
	} else {
		content = string(bufferedData)
	}

	// Apply content-type-specific rewriting
	var rewritten string
	if rw.isCSS {
		rewritten = rewriteCSSContent(content, rw.pathPrefix)
	} else if rw.isJSON {
		rewritten = rewriteURLBase(content, rw.pathPrefix)
	} else {
		rewritten = rewriteHTMLPaths(content, rw.pathPrefix, rw.requestPath)
	}

	// Remove compression headers since we're sending uncompressed
	// This is simpler and more reliable than re-compressing
	rw.ResponseWriter.Header().Del("Content-Encoding")
	rw.ResponseWriter.Header().Del("Content-Length")
	rw.ResponseWriter.Header().Set("Content-Length", strconv.Itoa(len(rewritten)))

	// Send headers
	rw.ResponseWriter.WriteHeader(rw.statusCode)
	rw.headerSent = true

	// Write rewritten content
	_, err := rw.ResponseWriter.Write([]byte(rewritten))
	if err != nil {
		return fmt.Errorf("failed to write rewritten response: %w", err)
	}
	return nil
}

// Flush implements http.Flusher to support streaming responses
func (rw *HTMLRewritingResponseWriter) Flush() {
	// IMPORTANT: Do NOT call FlushHTML() here!
	// ReverseProxy calls Flush() after WriteHeader but BEFORE Write(),
	// which would cause us to flush an empty buffer.
	// FlushHTML() should only be called manually after ServeHTTP() returns.

	// For non-buffered content, flush the underlying writer
	if !rw.isHTML && !rw.isCSS && !rw.isJSON {
		if f, ok := rw.ResponseWriter.(http.Flusher); ok {
			f.Flush()
		}
	}
	// For HTML/CSS content, do nothing - we're buffering until FlushHTML() is called manually
}

// Hijack implements http.Hijacker for WebSocket upgrade support
func (rw *HTMLRewritingResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hj, ok := rw.ResponseWriter.(http.Hijacker); ok {
		return hj.Hijack()
	}
	return nil, nil, http.ErrNotSupported
}

// Regular expressions for matching CSS and config patterns
var (
	// Match CSS url() patterns in style attributes and style tags
	cssURLRegex = regexp.MustCompile(`url\(\s*(['"]?)(/[^'")]+)(['"]?)\s*\)`)
	// Match @import "path" or @import 'path' in CSS (without url())
	cssImportRegex = regexp.MustCompile(`@import\s+(['"])(/[^'"]+)(['"])`)
	// Match urlBase config in both JS (urlBase: '') and JSON ("urlBase": "") formats
	// The optional leading quote handles JSON keys: "urlBase": "" vs JS keys: urlBase: ''
	urlBaseRegex = regexp.MustCompile(`("?urlBase"?\s*:\s*(['"]))(\/?)(['"])`)
)

// rewriteHTMLPaths rewrites absolute paths in HTML to include the path prefix
// pathPrefix is the route's pathBase (for JS interceptor and absolute path rewriting)
// requestPath is the actual request URL path (for base href to resolve relative paths)
func rewriteHTMLPaths(htmlContent string, pathPrefix string, requestPath string) string {
	if !strings.HasSuffix(pathPrefix, "/") {
		pathPrefix = pathPrefix + "/"
	}

	tokenizer := html.NewTokenizer(strings.NewReader(htmlContent))
	var buf bytes.Buffer
	var insideScript, insideStyle bool
	headInjected := false

	interceptor := generateFetchInterceptor(pathPrefix)
	baseHrefPath := computeBaseHrefPath(pathPrefix, requestPath)
	hasExistingBase := strings.Contains(strings.ToLower(htmlContent), "<base")

	for {
		tt := tokenizer.Next()
		switch tt {
		case html.ErrorToken:
			return buf.String()

		case html.StartTagToken, html.SelfClosingTagToken:
			token := tokenizer.Token()
			tagName := token.DataAtom

			if tagName == atom.Script && tt == html.StartTagToken {
				insideScript = true
			}
			if tagName == atom.Style && tt == html.StartTagToken {
				insideStyle = true
			}

			// Inject after <head>
			if tagName == atom.Head && !headInjected && tt == html.StartTagToken {
				buf.WriteString(token.String())
				headInjected = true
				if !hasExistingBase {
					buf.WriteString(`<base href="` + baseHrefPath + `">`)
				}
				buf.WriteString(interceptor)
				continue
			}

			// Rewrite <base> tag
			if tagName == atom.Base {
				rewriteBaseTag(&token, pathPrefix, baseHrefPath)
				buf.WriteString(token.String())
				continue
			}

			// Rewrite <meta> tags
			if tagName == atom.Meta {
				rewriteMetaTag(&token, pathPrefix)
				buf.WriteString(token.String())
				continue
			}

			// Rewrite resource attributes
			rewriteTokenAttributes(&token, pathPrefix)
			buf.WriteString(token.String())

		case html.EndTagToken:
			token := tokenizer.Token()
			if token.DataAtom == atom.Script {
				insideScript = false
			}
			if token.DataAtom == atom.Style {
				insideStyle = false
			}
			buf.WriteString(token.String())

		case html.TextToken:
			text := string(tokenizer.Raw())
			if insideStyle {
				text = rewriteCSSContent(text, pathPrefix)
			} else if insideScript {
				text = rewriteURLBase(text, pathPrefix)
			}
			buf.WriteString(text)

		default:
			buf.Write(tokenizer.Raw())
		}
	}
}

// computeBaseHrefPath computes the base href path from pathPrefix and requestPath
func computeBaseHrefPath(pathPrefix, requestPath string) string {
	baseHrefPath := requestPath
	if baseHrefPath == "" {
		baseHrefPath = pathPrefix
	}
	if !strings.HasSuffix(baseHrefPath, "/") {
		if idx := strings.LastIndex(baseHrefPath, "/"); idx >= 0 {
			baseHrefPath = baseHrefPath[:idx+1]
		} else {
			baseHrefPath = "/"
		}
	}
	return baseHrefPath
}

// rewritableAttrs lists HTML attributes whose values should be rewritten
var rewritableAttrs = map[string]bool{
	"src": true, "href": true, "action": true, "formaction": true,
	"poster": true, "data": true,
}

// rewriteTokenAttributes rewrites resource attributes on an HTML token
func rewriteTokenAttributes(token *html.Token, pathPrefix string) {
	for i, attr := range token.Attr {
		key := attr.Key
		if attr.Namespace != "" {
			key = attr.Namespace + ":" + attr.Key
		}

		if key == "srcset" {
			token.Attr[i].Val = rewriteSrcsetValue(attr.Val, pathPrefix)
			continue
		}

		if key == "style" && strings.Contains(attr.Val, "url(") {
			token.Attr[i].Val = rewriteCSSURLsInString(attr.Val, pathPrefix)
			continue
		}

		if rewritableAttrs[key] && shouldRewritePath(attr.Val, pathPrefix) {
			token.Attr[i].Val = pathPrefix + strings.TrimPrefix(attr.Val, "/")
		}

		// Handle xlink:href for SVG
		if key == "xlink:href" && shouldRewritePath(attr.Val, pathPrefix) {
			token.Attr[i].Val = pathPrefix + strings.TrimPrefix(attr.Val, "/")
		}
	}
}

// rewriteSrcsetValue rewrites paths in a srcset attribute value
func rewriteSrcsetValue(srcset, pathPrefix string) string {
	entries := strings.Split(srcset, ",")
	for i, entry := range entries {
		entry = strings.TrimSpace(entry)
		parts := strings.Fields(entry)
		if len(parts) > 0 && shouldRewritePath(parts[0], pathPrefix) {
			parts[0] = pathPrefix + strings.TrimPrefix(parts[0], "/")
			entries[i] = strings.Join(parts, " ")
		}
	}
	return strings.Join(entries, ", ")
}

// rewriteCSSURLsInString rewrites url() paths in a CSS string (e.g., from a style attribute)
func rewriteCSSURLsInString(css, pathPrefix string) string {
	return cssURLRegex.ReplaceAllStringFunc(css, func(match string) string {
		submatches := cssURLRegex.FindStringSubmatch(match)
		if len(submatches) < 4 {
			return match
		}
		openQuote := submatches[1]
		urlVal := submatches[2]
		closeQuote := submatches[3]
		if !shouldRewritePath(urlVal, pathPrefix) {
			return match
		}
		return "url(" + openQuote + pathPrefix + strings.TrimPrefix(urlVal, "/") + closeQuote + ")"
	})
}

// rewriteBaseTag rewrites the href attribute of a <base> tag
func rewriteBaseTag(token *html.Token, pathPrefix, baseHrefPath string) {
	for i, attr := range token.Attr {
		if attr.Key == "href" {
			if strings.HasPrefix(attr.Val, pathPrefix) {
				return
			}
			if attr.Val == "/" || attr.Val == "" {
				token.Attr[i].Val = baseHrefPath
			} else {
				token.Attr[i].Val = pathPrefix + strings.TrimPrefix(attr.Val, "/")
			}
			return
		}
	}
}

// rewriteMetaTag rewrites content attributes on <meta> tags for refresh and CSP
func rewriteMetaTag(token *html.Token, pathPrefix string) {
	var httpEquiv string
	for _, attr := range token.Attr {
		if strings.EqualFold(attr.Key, "http-equiv") {
			httpEquiv = strings.ToLower(attr.Val)
			break
		}
	}
	if httpEquiv == "" {
		return
	}
	for i, attr := range token.Attr {
		if strings.EqualFold(attr.Key, "content") {
			switch httpEquiv {
			case "refresh":
				lowerContent := strings.ToLower(attr.Val)
				urlIdx := strings.Index(lowerContent, "url=")
				if urlIdx == -1 {
					return
				}
				prefix := attr.Val[:urlIdx+4]
				urlPart := attr.Val[urlIdx+4:]
				if strings.HasPrefix(urlPart, "/") && !strings.HasPrefix(urlPart, "//") && !strings.HasPrefix(urlPart, pathPrefix) {
					token.Attr[i].Val = prefix + pathPrefix + strings.TrimPrefix(urlPart, "/")
				}
			case "content-security-policy", "content-security-policy-report-only":
				rw := &HTMLRewritingResponseWriter{pathPrefix: pathPrefix}
				token.Attr[i].Val = rw.rewriteCSPHeader(attr.Val)
			}
			return
		}
	}
}


// rewriteCSSURLs rewrites url() paths in CSS (inline styles and style tags)
func rewriteCSSURLs(html string, pathPrefix string) string {
	return cssURLRegex.ReplaceAllStringFunc(html, func(match string) string {
		// Extract the URL from url(...)
		submatches := cssURLRegex.FindStringSubmatch(match)
		if len(submatches) < 4 {
			return match
		}

		openQuote := submatches[1]
		url := submatches[2]
		closeQuote := submatches[3]

		// Only rewrite absolute paths
		if !shouldRewritePath(url, pathPrefix) {
			return match
		}

		newURL := pathPrefix + strings.TrimPrefix(url, "/")
		return "url(" + openQuote + newURL + closeQuote + ")"
	})
}

// rewriteCSSContent rewrites absolute paths in standalone CSS file responses
func rewriteCSSContent(css string, pathPrefix string) string {
	if !strings.HasSuffix(pathPrefix, "/") {
		pathPrefix = pathPrefix + "/"
	}

	// Rewrite url() patterns (reuse existing regex)
	css = rewriteCSSURLs(css, pathPrefix)

	// Rewrite @import "/path" patterns (without url())
	css = cssImportRegex.ReplaceAllStringFunc(css, func(match string) string {
		submatches := cssImportRegex.FindStringSubmatch(match)
		if len(submatches) < 4 {
			return match
		}
		openQuote := submatches[1]
		importPath := submatches[2]
		closeQuote := submatches[3]

		if !shouldRewritePath(importPath, pathPrefix) {
			return match
		}

		newPath := pathPrefix + strings.TrimPrefix(importPath, "/")
		return `@import ` + openQuote + newPath + closeQuote
	})

	return css
}

// rewriteURLBase rewrites urlBase config values in inline scripts.
// Apps like Sonarr/Radarr/Prowlarr use `urlBase: ”` to configure their SPA router basename.
// The backend sets it to empty since it doesn't know about the proxy prefix.
// We rewrite it to include the prefix so the router correctly strips it from the pathname.
func rewriteURLBase(html string, pathPrefix string) string {
	// pathPrefix has trailing slash (e.g. "/apps/sonarr/"), urlBase needs it without
	urlBaseValue := strings.TrimSuffix(pathPrefix, "/")

	return urlBaseRegex.ReplaceAllStringFunc(html, func(match string) string {
		submatches := urlBaseRegex.FindStringSubmatch(match)
		if len(submatches) < 5 {
			return match
		}
		value := submatches[3] // "" or "/"

		// Don't rewrite if already has a non-trivial urlBase (app has its own prefix configured)
		if value != "" && value != "/" {
			return match
		}

		return submatches[1] + urlBaseValue + submatches[4]
	})
}

// shouldRewritePath returns true if the path should be rewritten
func shouldRewritePath(path string, pathPrefix string) bool {
	// Don't rewrite empty paths
	if path == "" {
		return false
	}

	// Don't rewrite external URLs
	if strings.HasPrefix(path, "http://") ||
		strings.HasPrefix(path, "https://") ||
		strings.HasPrefix(path, "//") {
		return false
	}

	// Don't rewrite data URIs
	if strings.HasPrefix(path, "data:") {
		return false
	}

	// Don't rewrite relative paths (don't start with /)
	if !strings.HasPrefix(path, "/") {
		return false
	}

	// Don't rewrite if already has the prefix
	if strings.HasPrefix(path, pathPrefix) {
		return false
	}

	// Only exclude Nexus's own API paths, not app API paths
	// /api/v1/ is Nexus's API, but /api/v2/, /api/v3/, /api/config are app paths
	if strings.HasPrefix(path, "/api/v1/") {
		return false
	}

	// Don't rewrite Nexus health endpoint
	if path == "/healthz" || strings.HasPrefix(path, "/healthz/") {
		return false
	}

	return true
}

// generateFetchInterceptor generates a JavaScript interceptor that rewrites absolute paths
// in fetch() and XMLHttpRequest calls to include the path prefix
// This is critical for SPAs like Memos that make API calls with absolute paths starting with "/"
func generateFetchInterceptor(pathPrefix string) string {
	// Ensure pathPrefix has trailing slash
	if !strings.HasSuffix(pathPrefix, "/") {
		pathPrefix = pathPrefix + "/"
	}

	// Generate the interceptor script
	// This intercepts both fetch() and XMLHttpRequest.open() to rewrite absolute paths
	return `<script>
(function() {
  'use strict';

  const basePath = '` + pathPrefix + `';
  const OriginalURL = typeof URL !== 'undefined' ? URL : undefined;

  // Helper function to rewrite URL
  // Build same-origin prefixes for http(s) and ws(s) schemes
  var sameOriginPrefixes = [window.location.origin + '/'];
  // Also match ws:// and wss:// URLs with the same host (for WebSocket/SignalR)
  if (window.location.protocol === 'https:') {
    sameOriginPrefixes.push('wss://' + window.location.host + '/');
  } else {
    sameOriginPrefixes.push('ws://' + window.location.host + '/');
  }

  function rewriteUrl(url) {
    // Handle string URLs
    if (typeof url === 'string') {
      // Get basePath without trailing slash for comparison
      const basePathNoSlash = basePath.endsWith('/') ? basePath.slice(0, -1) : basePath;

      // Absolute path starting with /
      if (url.startsWith('/') && !url.startsWith('//')) {
        // Skip if already has prefix (with or without trailing slash)
        if (url.startsWith(basePath) || url === basePathNoSlash || url.startsWith(basePathNoSlash + '/')) {
          return url;
        }
        return basePath + url.substring(1);
      }
      // Full URL with same origin (http/https and ws/wss)
      for (var i = 0; i < sameOriginPrefixes.length; i++) {
        var prefix = sameOriginPrefixes[i];
        if (url.startsWith(prefix)) {
          var path = url.substring(prefix.length - 1); // include the leading /
          if (path.startsWith('/')) {
            // Skip if already has prefix (with or without trailing slash)
            if (path.startsWith(basePath) || path === basePathNoSlash || path.startsWith(basePathNoSlash + '/')) {
              return url;
            }
            return url.substring(0, prefix.length - 1) + basePath + path.substring(1);
          }
        }
      }
    }
    return url;
  }

  // Intercept window.fetch
  const originalFetch = window.fetch;
  window.fetch = function(resource, options) {
    let finalResource = resource;

    // Handle string URLs
    if (typeof resource === 'string') {
      const rewritten = rewriteUrl(resource);
      if (rewritten !== resource) {
        finalResource = rewritten;
      }
    }
    // Handle Request objects
    else if (resource instanceof Request) {
      const rewritten = rewriteUrl(resource.url);
      if (rewritten !== resource.url) {
        // Create new Request with rewritten URL and copy all options
        const init = {
          method: resource.method,
          headers: resource.headers,
          body: resource.body,
          mode: resource.mode,
          credentials: resource.credentials,
          cache: resource.cache,
          redirect: resource.redirect,
          referrer: resource.referrer,
          integrity: resource.integrity,
        };
        finalResource = new Request(rewritten, init);
      }
    }

    return originalFetch.call(this, finalResource, options);
  };

  // Intercept XMLHttpRequest.open
  const originalOpen = XMLHttpRequest.prototype.open;
  XMLHttpRequest.prototype.open = function(method, url, ...rest) {
    const rewritten = rewriteUrl(url);

    if (rewritten !== url) {
      url = rewritten;
    }

    return originalOpen.call(this, method, url, ...rest);
  };

  // Intercept setAttribute to rewrite paths as they're set
  const resourceAttrs = ['src', 'href', 'data', 'poster', 'action', 'formaction'];
  const originalSetAttribute = Element.prototype.setAttribute;
  Element.prototype.setAttribute = function(name, value) {
    // Only rewrite resource attributes with absolute paths
    if (resourceAttrs.includes(name.toLowerCase()) &&
        typeof value === 'string' &&
        value.startsWith('/') &&
        !value.startsWith('//') &&
        !value.startsWith(basePath)) {
      value = basePath + value.substring(1);
    }
    return originalSetAttribute.call(this, name, value);
  };

  // Intercept getAttribute to strip the prefix when JS reads resource attributes.
  // The DOM retains the prefixed value (so the browser loads resources from the
  // correct proxy URL), but JS sees the unprefixed "native" path. This prevents
  // the proxy prefix from leaking into SPA hash routes when apps read href
  // attributes and use them for client-side routing.
  const originalGetAttribute = Element.prototype.getAttribute;
  Element.prototype.getAttribute = function(name) {
    const value = originalGetAttribute.call(this, name);
    if (value && resourceAttrs.includes(name.toLowerCase()) &&
        typeof value === 'string' &&
        value.startsWith(basePath)) {
      return '/' + value.substring(basePath.length);
    }
    return value;
  };

  // Intercept property setters AND getters for direct property assignments.
  // Setter adds the prefix (so the DOM/browser uses the correct proxy URL).
  // Getter strips the prefix (so JS sees the native path for SPA routing).
  // The property getter returns the resolved absolute URL, so we strip the
  // prefix from the pathname portion.
  function interceptProperty(prototype, propertyName) {
    const descriptor = Object.getOwnPropertyDescriptor(prototype, propertyName);
    if (!descriptor || !descriptor.set) return;

    const originalSetter = descriptor.set;
    const originalGetter = descriptor.get;
    Object.defineProperty(prototype, propertyName, {
      set: function(value) {
        // Only rewrite absolute paths that don't already have the prefix
        if (typeof value === 'string' &&
            value.startsWith('/') &&
            !value.startsWith('//') &&
            !value.startsWith(basePath)) {
          value = basePath + value.substring(1);
        }
        return originalSetter.call(this, value);
      },
      get: originalGetter ? function() {
        var value = originalGetter.call(this);
        // Property getters return resolved absolute URLs (e.g. http://host/apps/x/path)
        // Strip the basePath from the pathname so SPAs see the native path
        if (typeof value === 'string' && OriginalURL) {
          try {
            var u = new OriginalURL(value);
            if (u.pathname.startsWith(basePath)) {
              u.pathname = '/' + u.pathname.substring(basePath.length);
              return u.toString();
            }
          } catch(e) {}
        }
        return value;
      } : undefined,
      enumerable: descriptor.enumerable,
      configurable: descriptor.configurable
    });
  }

  // Intercept all resource-loading properties on their respective prototypes
  interceptProperty(HTMLScriptElement.prototype, 'src');
  interceptProperty(HTMLImageElement.prototype, 'src');
  interceptProperty(HTMLAnchorElement.prototype, 'href');
  interceptProperty(HTMLLinkElement.prototype, 'href');
  interceptProperty(HTMLObjectElement.prototype, 'data');
  interceptProperty(HTMLEmbedElement.prototype, 'src');
  interceptProperty(HTMLSourceElement.prototype, 'src');
  interceptProperty(HTMLTrackElement.prototype, 'src');
  interceptProperty(HTMLIFrameElement.prototype, 'src');
  interceptProperty(HTMLFrameElement.prototype, 'src');
  interceptProperty(HTMLVideoElement.prototype, 'src');
  interceptProperty(HTMLVideoElement.prototype, 'poster');
  interceptProperty(HTMLAudioElement.prototype, 'src');
  interceptProperty(HTMLFormElement.prototype, 'action');
  interceptProperty(HTMLButtonElement.prototype, 'formAction');
  interceptProperty(HTMLInputElement.prototype, 'formAction');

  // Intercept EventSource for Server-Sent Events (SSE)
  // Used by SignalR and other real-time frameworks
  if (typeof EventSource !== 'undefined') {
    const OriginalEventSource = EventSource;
    window.EventSource = function(url, config) {
      const rewritten = rewriteUrl(url);
      return new OriginalEventSource(rewritten, config);
    };
    window.EventSource.prototype = OriginalEventSource.prototype;
    window.EventSource.CONNECTING = OriginalEventSource.CONNECTING;
    window.EventSource.OPEN = OriginalEventSource.OPEN;
    window.EventSource.CLOSED = OriginalEventSource.CLOSED;
  }

  // Intercept WebSocket constructor
  if (typeof WebSocket !== 'undefined') {
    const OriginalWebSocket = WebSocket;
    window.WebSocket = function(url, protocols) {
      const rewritten = rewriteUrl(url);
      return protocols !== undefined
        ? new OriginalWebSocket(rewritten, protocols)
        : new OriginalWebSocket(rewritten);
    };
    window.WebSocket.prototype = OriginalWebSocket.prototype;
    window.WebSocket.CONNECTING = OriginalWebSocket.CONNECTING;
    window.WebSocket.OPEN = OriginalWebSocket.OPEN;
    window.WebSocket.CLOSING = OriginalWebSocket.CLOSING;
    window.WebSocket.CLOSED = OriginalWebSocket.CLOSED;
  }

  // Intercept URL constructor
  // Only rewrite when no base is provided (single-argument form).
  // When a base IS provided, the caller is explicitly controlling resolution,
  // and the fetch/XHR interceptors will add the prefix at request time.
  // Rewriting here with a base causes double-prefixing when libraries like
  // axios prepend their own baseURL to the already-rewritten path.
  if (OriginalURL) {
    window.URL = function(url, base) {
      if (typeof url === 'string' && base === undefined) {
        url = rewriteUrl(url);
      }
      return base !== undefined
        ? new OriginalURL(url, base)
        : new OriginalURL(url);
    };
    window.URL.prototype = OriginalURL.prototype;
    window.URL.createObjectURL = OriginalURL.createObjectURL;
    window.URL.revokeObjectURL = OriginalURL.revokeObjectURL;
  }

  // Helper: strip the proxy prefix from the hash portion of a URL.
  // e.g. "http://host/apps/x/web/#/apps/x/login" → "http://host/apps/x/web/#/login"
  function cleanHash(url) {
    if (typeof url !== 'string') return url;
    var hashIdx = url.indexOf('#');
    if (hashIdx === -1) return url;
    var base = url.substring(0, hashIdx);
    var hash = url.substring(hashIdx + 1);
    var basePathNoSlash = basePath.endsWith('/') ? basePath.slice(0, -1) : basePath;
    if (hash.startsWith(basePathNoSlash + '/') || hash === basePathNoSlash) {
      var stripped = hash.substring(basePathNoSlash.length);
      hash = stripped === '' ? '/' : stripped;
    }
    return base + '#' + hash;
  }

  // Intercept history.pushState and history.replaceState
  // This handles SPA navigation that uses the History API
  if (typeof history !== 'undefined') {
    const originalPushState = history.pushState;
    history.pushState = function(state, title, url) {
      if (typeof url === 'string') {
        url = cleanHash(rewriteUrl(url));
      }
      return originalPushState.call(this, state, title, url);
    };

    const originalReplaceState = history.replaceState;
    history.replaceState = function(state, title, url) {
      if (typeof url === 'string') {
        url = cleanHash(rewriteUrl(url));
      }
      return originalReplaceState.call(this, state, title, url);
    };
  }

  // Intercept location.assign() and location.replace()
  // These are commonly used for programmatic navigation
  // Use try-catch because some browsers (Firefox) have read-only location methods
  try {
    if (typeof location !== 'undefined') {
      const originalAssign = location.assign.bind(location);
      Object.defineProperty(location, 'assign', {
        value: function(url) { return originalAssign(rewriteUrl(url)); },
        writable: true,
        configurable: true
      });

      const originalReplace = location.replace.bind(location);
      Object.defineProperty(location, 'replace', {
        value: function(url) { return originalReplace(rewriteUrl(url)); },
        writable: true,
        configurable: true
      });
    }
  } catch(e) {
    // Firefox and some browsers don't allow modifying location methods
  }

  // Intercept location.href setter
  // This catches window.location.href = '/path' and location.href = '/path'
  try {
    const locationDescriptor = Object.getOwnPropertyDescriptor(window, 'location');
    if (locationDescriptor && locationDescriptor.set) {
      // Some browsers allow overriding window.location
      const originalLocationSetter = locationDescriptor.set;
      Object.defineProperty(window, 'location', {
        get: locationDescriptor.get,
        set: function(url) {
          return originalLocationSetter.call(this, rewriteUrl(url));
        },
        configurable: true
      });
    }
  } catch(e) {
    // Some browsers don't allow modifying window.location, ignore
  }

  // Intercept Worker and SharedWorker constructors
  // Workers load scripts from absolute paths
  if (typeof Worker !== 'undefined') {
    const OriginalWorker = window.Worker;
    window.Worker = function(url, options) {
      const rewritten = rewriteUrl(url);
      return options !== undefined
        ? new OriginalWorker(rewritten, options)
        : new OriginalWorker(rewritten);
    };
    window.Worker.prototype = OriginalWorker.prototype;
  }

  if (typeof SharedWorker !== 'undefined') {
    const OriginalSharedWorker = window.SharedWorker;
    window.SharedWorker = function(url, options) {
      const rewritten = rewriteUrl(url);
      return options !== undefined
        ? new OriginalSharedWorker(rewritten, options)
        : new OriginalSharedWorker(rewritten);
    };
    window.SharedWorker.prototype = OriginalSharedWorker.prototype;
  }

  // Intercept navigator.sendBeacon
  if (typeof navigator !== 'undefined' && navigator.sendBeacon) {
    const originalSendBeacon = navigator.sendBeacon.bind(navigator);
    navigator.sendBeacon = function(url, data) {
      return originalSendBeacon(rewriteUrl(url), data);
    };
  }

  // Intercept ServiceWorker registration
  if (typeof navigator !== 'undefined' && navigator.serviceWorker) {
    const origRegister = navigator.serviceWorker.register.bind(navigator.serviceWorker);
    navigator.serviceWorker.register = function(scriptURL, opts) {
      opts = opts || {};
      if (typeof opts.scope === 'string') {
        opts.scope = rewriteUrl(opts.scope);
      }
      return origRegister(rewriteUrl(scriptURL), opts);
    };
  }

  // Intercept window.open
  if (typeof window.open === 'function') {
    const origWindowOpen = window.open;
    window.open = function(url, target, features) {
      if (typeof url === 'string') {
        url = rewriteUrl(url);
      }
      return origWindowOpen.call(this, url, target, features);
    };
  }

  // Intercept importScripts (Web Workers)
  if (typeof importScripts === 'function') {
    const origImportScripts = importScripts;
    importScripts = function() {
      var rewritten = [];
      for (var i = 0; i < arguments.length; i++) {
        rewritten.push(rewriteUrl(arguments[i]));
      }
      return origImportScripts.apply(this, rewritten);
    };
  }

  // Override Location.prototype getters to strip the base path prefix.
  // This fixes SPA routers that read window.location for route matching.
  // We must intercept pathname, href, and toString consistently so that
  // code using new URL(location.href) agrees with location.pathname.
  try {
    const locProto = Object.getPrototypeOf(window.location);
    const basePathNoSlash = basePath.endsWith('/') ? basePath.slice(0, -1) : basePath;

    function stripPrefix(p) {
      if (p === basePathNoSlash || p.startsWith(basePathNoSlash + '/')) {
        const s = p.substring(basePathNoSlash.length);
        return s === '' ? '/' : s;
      }
      return p;
    }

    // Intercept pathname getter
    const pathDesc = Object.getOwnPropertyDescriptor(locProto, 'pathname');
    if (pathDesc && pathDesc.get) {
      const origPathGetter = pathDesc.get;
      Object.defineProperty(locProto, 'pathname', {
        get: function() { return stripPrefix(origPathGetter.call(this)); },
        set: pathDesc.set,
        enumerable: pathDesc.enumerable,
        configurable: pathDesc.configurable
      });
    }

    // Intercept href getter so new URL(location.href) is consistent with pathname
    const hrefDesc = Object.getOwnPropertyDescriptor(locProto, 'href');
    if (hrefDesc && hrefDesc.get) {
      const origHrefGetter = hrefDesc.get;
      Object.defineProperty(locProto, 'href', {
        get: function() {
          const h = origHrefGetter.call(this);
          try {
            const u = new OriginalURL(h);
            const stripped = stripPrefix(u.pathname);
            if (stripped !== u.pathname) {
              u.pathname = stripped;
              return u.toString();
            }
          } catch(e) {}
          return h;
        },
        set: hrefDesc.set,
        enumerable: hrefDesc.enumerable,
        configurable: hrefDesc.configurable
      });
    }

    // Intercept hash getter/setter to strip the proxy prefix from hash routes.
    // SPAs using hash-based routing (e.g. /#/login) can accidentally get the
    // proxy prefix into the hash fragment (e.g. /#/apps/myapp/login) from
    // various sources. This interceptor acts as a safety net.
    const hashDesc = Object.getOwnPropertyDescriptor(locProto, 'hash');
    if (hashDesc && hashDesc.get) {
      const origHashGetter = hashDesc.get;
      const origHashSetter = hashDesc.set;
      Object.defineProperty(locProto, 'hash', {
        get: function() {
          const h = origHashGetter.call(this);
          // Strip prefix from hash path: #/apps/myapp/login → #/login
          if (h.startsWith('#' + basePathNoSlash + '/') || h === '#' + basePathNoSlash) {
            const stripped = h.substring(1 + basePathNoSlash.length);
            return '#' + (stripped === '' ? '/' : stripped);
          }
          return h;
        },
        set: origHashSetter ? function(v) {
          // Strip prefix when setting hash too
          if (typeof v === 'string') {
            var val = v.startsWith('#') ? v.substring(1) : v;
            val = stripPrefix(val);
            return origHashSetter.call(this, '#' + val);
          }
          return origHashSetter.call(this, v);
        } : undefined,
        enumerable: hashDesc.enumerable,
        configurable: hashDesc.configurable
      });
    }

    // Intercept toString (used when location is coerced to string)
    const toStrDesc = Object.getOwnPropertyDescriptor(locProto, 'toString');
    if (toStrDesc) {
      Object.defineProperty(locProto, 'toString', {
        value: function() {
          return this.href;
        },
        writable: toStrDesc.writable,
        enumerable: toStrDesc.enumerable,
        configurable: toStrDesc.configurable
      });
    }
  } catch(e) {
    // Some browsers may not allow overriding Location.prototype
  }
})();
</script>`
}

