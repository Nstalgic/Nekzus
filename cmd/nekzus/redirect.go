package main

import (
	"net"
	"net/http"
	"time"
)

// newHTTPToHTTPSRedirectHandler creates a handler that redirects all HTTP requests to HTTPS.
// The httpsPort parameter specifies the HTTPS port to redirect to.
// If httpsPort is "443", the port is omitted from the redirect URL (standard HTTPS port).
func newHTTPToHTTPSRedirectHandler(httpsPort string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Extract host without port
		host := r.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}

		// Build HTTPS URL
		var targetURL string
		if httpsPort == "443" {
			targetURL = "https://" + host + r.URL.RequestURI()
		} else {
			targetURL = "https://" + host + ":" + httpsPort + r.URL.RequestURI()
		}

		http.Redirect(w, r, targetURL, http.StatusMovedPermanently)
	})
}

// startHTTPRedirectServer starts an HTTP server that redirects all requests to HTTPS.
// This is only started when TLS is enabled and HTTPRedirectAddr is configured.
func (app *Application) startHTTPRedirectServer() {
	if app.config.Server.HTTPRedirectAddr == "" {
		return
	}

	// Extract the HTTPS port from the main server address
	httpsPort := "443"
	if _, port, err := net.SplitHostPort(app.config.Server.Addr); err == nil && port != "" {
		httpsPort = port
	}

	app.httpRedirectServer = &http.Server{
		Addr:              app.config.Server.HTTPRedirectAddr,
		Handler:           newHTTPToHTTPSRedirectHandler(httpsPort),
		ReadTimeout:       5 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      5 * time.Second,
		IdleTimeout:       30 * time.Second,
	}

	go func() {
		log.Info("starting http to https redirect server",
			"http_addr", app.config.Server.HTTPRedirectAddr,
			"https_port", httpsPort)

		if err := app.httpRedirectServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("http redirect server error",
				"error", err)
		}
	}()
}
