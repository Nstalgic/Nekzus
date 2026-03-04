package main

import (
	"net/http"
)

// handleQRCode generates a QR code for mobile app pairing
func (app *Application) handleQRCode(w http.ResponseWriter, r *http.Request) {
	app.handlers.Auth.HandleQRCode(w, r)
}

// handlePair authenticates a new device using a bootstrap token
func (app *Application) handlePair(w http.ResponseWriter, r *http.Request) {
	app.handlers.Auth.HandlePair(w, r)
}

// handleRefresh issues a new JWT token
func (app *Application) handleRefresh(w http.ResponseWriter, r *http.Request) {
	app.handlers.Auth.HandleRefresh(w, r)
}

// handlePairWebUI serves a web page with QR code for pairing
func (app *Application) handlePairWebUI(w http.ResponseWriter, r *http.Request) {
	app.handlers.Auth.HandlePairWebUI(w, r)
}
