package main

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"time"

	"github.com/nstalgic/nekzus/internal/httputil"
)

// Core Handler methods

// handleHealthz returns a simple health check response
func (app *Application) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

// Helper functions

// randToken generates a random base64-encoded token
func randToken(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// logMiddleware logs HTTP requests
func logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		duration := time.Since(start)
		log.Info("http request", "method", r.Method, "path", r.URL.Path, "client_ip", httputil.ExtractClientIP(r), "duration_ms", duration.Milliseconds())
	})
}
