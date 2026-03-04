package main

import (
	"net/http"
	"time"
)

// LoginRequest represents a login request payload
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// LoginResponse represents a successful login response
type LoginResponse struct {
	Token string      `json:"token"`
	User  interface{} `json:"user"`
}

// UserResponse represents a user object in API responses
type UserResponse struct {
	ID        int        `json:"id"`
	Username  string     `json:"username"`
	CreatedAt time.Time  `json:"createdAt"`
	LastLogin *time.Time `json:"lastLogin,omitempty"`
	IsActive  bool       `json:"isActive"`
}

// handleLogin handles POST /api/v1/auth/login
func (app *Application) handleLogin(w http.ResponseWriter, r *http.Request) {
	app.handlers.Auth.HandleLogin(w, r)
}

// handleAuthMe handles GET /api/v1/auth/me
func (app *Application) handleAuthMe(w http.ResponseWriter, r *http.Request) {
	app.handlers.Auth.HandleAuthMe(w, r)
}

// handleLogout handles POST /api/v1/auth/logout
func (app *Application) handleLogout(w http.ResponseWriter, r *http.Request) {
	app.handlers.Auth.HandleLogout(w, r)
}

// handleSetupStatus handles GET /api/v1/auth/setup-status
// Returns whether initial setup is required (no users exist)
func (app *Application) handleSetupStatus(w http.ResponseWriter, r *http.Request) {
	app.handlers.Auth.HandleSetupStatus(w, r)
}

// SetupRequest represents an initial setup request payload
type SetupRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// handleSetup handles POST /api/v1/auth/setup
// Creates the first admin user (only works if no users exist)
func (app *Application) handleSetup(w http.ResponseWriter, r *http.Request) {
	app.handlers.Auth.HandleSetup(w, r)
}
