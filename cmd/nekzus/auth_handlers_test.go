package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/nstalgic/nekzus/internal/activity"
	"github.com/nstalgic/nekzus/internal/auth"
	"github.com/nstalgic/nekzus/internal/handlers"
	"github.com/nstalgic/nekzus/internal/proxy"
	"github.com/nstalgic/nekzus/internal/ratelimit"
	"github.com/nstalgic/nekzus/internal/router"
	"github.com/nstalgic/nekzus/internal/storage"
	"github.com/nstalgic/nekzus/internal/websocket"
)

// setupTestApp creates a test application with storage and auth
func setupAuthTestApp(t *testing.T) *Application {
	t.Helper()

	// Create temp directory database (in-memory has issues with multiple connections)
	tmpDir := t.TempDir()
	cfg := storage.Config{
		DatabasePath: filepath.Join(tmpDir, "test.db"),
	}
	store, err := storage.NewStore(cfg)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	// Initialize user schema
	if err := store.InitializeUserSchema(); err != nil {
		t.Fatalf("Failed to initialize user schema: %v", err)
	}

	// Create auth manager
	jwtSecret := "random-jwt-hmac-key-f8e7d6c5b4a39281-only"
	authMgr, err := auth.NewManager(
		[]byte(jwtSecret),
		"nekzus",
		"nekzus-web",
		[]string{},
	)
	if err != nil {
		t.Fatalf("Failed to create auth manager: %v", err)
	}

	// Create managers first
	activityTracker := activity.NewTracker(store)
	wsManager := websocket.NewManager(testMetrics, store)
	qrLimiter := ratelimit.NewLimiter(1.0, 5)

	// Create auth handler
	authHandler := handlers.NewAuthHandler(
		authMgr,
		store,
		testMetrics,
		wsManager,
		activityTracker,
		qrLimiter,
		nil, // no cert manager for tests
		"http://localhost:8080",
		"",
		"test-nexus-id",
		"1.0.0-test",
		[]string{"catalog", "events", "proxy", "discovery"},
	)

	// Create test app - use shared test metrics to avoid duplicate registration
	app := &Application{
		storage: store,
		services: &ServiceRegistry{
			Auth: authMgr,
		},
		limiters: &RateLimiterRegistry{
			QR: qrLimiter,
		},
		managers: &ManagerRegistry{
			Router:    router.NewRegistry(store),
			WebSocket: wsManager,
			Activity:  activityTracker,
		},
		handlers: &HandlerRegistry{
			Auth: authHandler,
		},
		jobs:         &JobRegistry{}, // Empty jobs registry for tests
		metrics:      testMetrics,    // Use shared metrics from main_test.go
		proxyCache:   proxy.NewCache(),
		version:      "1.0.0-test",
		nexusID:      "test-nexus-id",
		baseURL:      "http://localhost:8080",
		capabilities: []string{"catalog", "events", "proxy", "discovery"},
	}

	t.Cleanup(func() {
		store.Close()
	})

	return app
}

func TestHandleLogin(t *testing.T) {
	app := setupAuthTestApp(t)

	// Create test user
	testUsername := "testuser"
	testPassword := "TestPassword123!"
	if err := app.storage.CreateUser(testUsername, testPassword); err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}

	tests := []struct {
		name           string
		requestBody    interface{}
		wantStatus     int
		wantToken      bool
		wantUser       bool
		checkLastLogin bool
	}{
		{
			name: "valid login",
			requestBody: map[string]string{
				"username": testUsername,
				"password": testPassword,
			},
			wantStatus:     http.StatusOK,
			wantToken:      true,
			wantUser:       true,
			checkLastLogin: true,
		},
		{
			name: "invalid password",
			requestBody: map[string]string{
				"username": testUsername,
				"password": "WrongPassword123!",
			},
			wantStatus: http.StatusUnauthorized,
			wantToken:  false,
			wantUser:   false,
		},
		{
			name: "non-existent user",
			requestBody: map[string]string{
				"username": "nonexistent",
				"password": "SomePassword123!",
			},
			wantStatus: http.StatusUnauthorized,
			wantToken:  false,
			wantUser:   false,
		},
		{
			name: "missing username",
			requestBody: map[string]string{
				"password": testPassword,
			},
			wantStatus: http.StatusBadRequest,
			wantToken:  false,
			wantUser:   false,
		},
		{
			name: "missing password",
			requestBody: map[string]string{
				"username": testUsername,
			},
			wantStatus: http.StatusBadRequest,
			wantToken:  false,
			wantUser:   false,
		},
		{
			name:        "invalid json",
			requestBody: "not valid json",
			wantStatus:  http.StatusBadRequest,
			wantToken:   false,
			wantUser:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create request
			var body []byte
			var err error
			if str, ok := tt.requestBody.(string); ok {
				body = []byte(str)
			} else {
				body, err = json.Marshal(tt.requestBody)
				if err != nil {
					t.Fatalf("Failed to marshal request: %v", err)
				}
			}

			req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			// Call handler
			app.handleLogin(w, req)

			// Check status code
			if w.Code != tt.wantStatus {
				t.Errorf("Status = %d, want %d. Body: %s", w.Code, tt.wantStatus, w.Body.String())
			}

			if w.Code == http.StatusOK {
				// Parse response
				var response map[string]interface{}
				if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
					t.Fatalf("Failed to decode response: %v", err)
				}

				// Check token
				if tt.wantToken {
					token, ok := response["token"].(string)
					if !ok || token == "" {
						t.Error("Expected token in response")
					}
				}

				// Check user object
				if tt.wantUser {
					user, ok := response["user"].(map[string]interface{})
					if !ok {
						t.Error("Expected user object in response")
					} else {
						if user["username"] != testUsername {
							t.Errorf("Username = %v, want %v", user["username"], testUsername)
						}
						// Password hash should never be returned
						if _, exists := user["passwordHash"]; exists {
							t.Error("Password hash should not be in response")
						}
					}
				}

				// Check last_login was updated
				if tt.checkLastLogin {
					// Give the async goroutine time to complete
					time.Sleep(100 * time.Millisecond)
					user, err := app.storage.GetUserByUsername(testUsername)
					if err != nil {
						t.Errorf("Failed to get user: %v", err)
					}
					if user.LastLogin == nil {
						t.Error("LastLogin should be set after login")
					} else if time.Since(*user.LastLogin) > 5*time.Second {
						t.Errorf("LastLogin timestamp is not recent: %v", user.LastLogin)
					}
				}
			}
		})
	}
}

func TestHandleAuthMe(t *testing.T) {
	app := setupAuthTestApp(t)

	// Create test user
	testUsername := "testuser"
	testPassword := "TestPassword123!"
	if err := app.storage.CreateUser(testUsername, testPassword); err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}

	user, err := app.storage.GetUserByUsername(testUsername)
	if err != nil || user == nil {
		t.Fatalf("Failed to get test user: %v", err)
	}

	// Generate valid token
	validToken, err := app.services.Auth.SignJWT("user-"+testUsername, []string{"admin"}, 12*time.Hour)
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	tests := []struct {
		name       string
		authHeader string
		wantStatus int
		wantUser   bool
	}{
		{
			name:       "valid token",
			authHeader: "Bearer " + validToken,
			wantStatus: http.StatusOK,
			wantUser:   true,
		},
		{
			name:       "missing token",
			authHeader: "",
			wantStatus: http.StatusUnauthorized,
			wantUser:   false,
		},
		{
			name:       "invalid token",
			authHeader: "Bearer invalid.token.here",
			wantStatus: http.StatusUnauthorized,
			wantUser:   false,
		},
		{
			name:       "malformed header",
			authHeader: "InvalidHeader",
			wantStatus: http.StatusUnauthorized,
			wantUser:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			w := httptest.NewRecorder()

			app.handleAuthMe(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("Status = %d, want %d. Body: %s", w.Code, tt.wantStatus, w.Body.String())
			}

			if tt.wantUser && w.Code == http.StatusOK {
				var response map[string]interface{}
				if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
					t.Fatalf("Failed to decode response: %v", err)
				}

				user, ok := response["user"].(map[string]interface{})
				if !ok {
					t.Error("Expected user object in response")
				} else {
					// Password hash should never be returned
					if _, exists := user["passwordHash"]; exists {
						t.Error("Password hash should not be in response")
					}
				}
			}
		})
	}
}

func TestHandleLogout(t *testing.T) {
	app := setupAuthTestApp(t)

	// Create test user
	testUsername := "testuser"
	testPassword := "TestPassword123!"
	if err := app.storage.CreateUser(testUsername, testPassword); err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}

	// Generate valid token
	validToken, err := app.services.Auth.SignJWT("user-"+testUsername, []string{"admin"}, 12*time.Hour)
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	tests := []struct {
		name       string
		authHeader string
		wantStatus int
	}{
		{
			name:       "valid logout",
			authHeader: "Bearer " + validToken,
			wantStatus: http.StatusOK,
		},
		{
			name:       "logout without token",
			authHeader: "",
			wantStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			w := httptest.NewRecorder()

			app.handleLogout(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("Status = %d, want %d. Body: %s", w.Code, tt.wantStatus, w.Body.String())
			}

			if w.Code == http.StatusOK {
				var response map[string]interface{}
				if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
					t.Fatalf("Failed to decode response: %v", err)
				}

				message, ok := response["message"].(string)
				if !ok || message == "" {
					t.Error("Expected message in response")
				}
			}
		})
	}
}

func TestLoginRateLimiting(t *testing.T) {
	app := setupAuthTestApp(t)

	// Create test user
	testUsername := "testuser"
	testPassword := "TestPassword123!"
	if err := app.storage.CreateUser(testUsername, testPassword); err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}

	// Test that multiple login attempts work within limits
	// This is a basic test - full rate limiting would be tested via middleware
	for i := 0; i < 3; i++ {
		body, _ := json.Marshal(map[string]string{
			"username": testUsername,
			"password": testPassword,
		})

		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		app.handleLogin(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Request %d: Status = %d, want %d", i+1, w.Code, http.StatusOK)
		}
	}
}
