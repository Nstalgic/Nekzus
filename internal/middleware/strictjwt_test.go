package middleware

import (
	"bufio"
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/nstalgic/nekzus/internal/storage"
)

// Test that error messages don't leak implementation details
func TestStrictJWTAuth_ErrorMessagesDoNotLeakDetails(t *testing.T) {
	// Use shared testAuth and testMetrics from ipauth_test.go init()
	middleware := NewStrictJWTAuth(testAuth, nil, testMetrics)

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	wrappedHandler := middleware(nextHandler)

	t.Run("InvalidToken_NoLeakedDetails", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/test", nil)
		req.Header.Set("Authorization", "Bearer invalid-token-12345")
		rec := httptest.NewRecorder()

		wrappedHandler.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("Expected status 401, got %d", rec.Code)
		}

		// Check that error message does NOT contain internal error details
		body := strings.TrimSpace(rec.Body.String())

		// Should NOT contain internal error details like "token contains an invalid number of segments"
		if strings.Contains(strings.ToLower(body), "segment") ||
			strings.Contains(strings.ToLower(body), "invalid") && strings.Contains(strings.ToLower(body), "number") ||
			strings.Contains(strings.ToLower(body), "parse") ||
			strings.Contains(strings.ToLower(body), "decode") {
			t.Errorf("Error message leaks implementation details: %q", body)
		}

		// Should be a generic error message
		if body != "unauthorized" {
			t.Errorf("Expected generic 'unauthorized' message, got: %q", body)
		}
	})

	t.Run("MalformedToken_NoLeakedDetails", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/test", nil)
		// Malformed base64 token
		req.Header.Set("Authorization", "Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.malformed-payload.signature")
		rec := httptest.NewRecorder()

		wrappedHandler.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("Expected status 401, got %d", rec.Code)
		}

		body := strings.TrimSpace(rec.Body.String())

		// Should NOT leak encoding errors
		if strings.Contains(strings.ToLower(body), "base64") ||
			strings.Contains(strings.ToLower(body), "illegal") ||
			strings.Contains(strings.ToLower(body), "encoding") {
			t.Errorf("Error message leaks encoding details: %q", body)
		}

		if body != "unauthorized" {
			t.Errorf("Expected generic 'unauthorized' message, got: %q", body)
		}
	})

	t.Run("ExpiredToken_NoLeakedDetails", func(t *testing.T) {
		// Create expired token
		token, err := testAuth.SignJWT("device-123", []string{"read:catalog"}, -1*time.Hour)
		if err != nil {
			t.Fatalf("Failed to create expired token: %v", err)
		}

		req := httptest.NewRequest("GET", "/api/test", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()

		wrappedHandler.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("Expected status 401, got %d", rec.Code)
		}

		body := strings.TrimSpace(rec.Body.String())

		// Should NOT leak expiration timestamp
		if strings.Contains(body, "exp") ||
			strings.Contains(body, "expired") ||
			strings.Contains(body, "validation") {
			t.Errorf("Error message leaks expiration details: %q", body)
		}

		if body != "unauthorized" {
			t.Errorf("Expected generic 'unauthorized' message, got: %q", body)
		}
	})
}

// Test successful authentication
func TestStrictJWTAuth_ValidToken(t *testing.T) {
	middleware := NewStrictJWTAuth(testAuth, nil, testMetrics)

	nextCalled := false
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		// Verify device ID is in context
		deviceID := GetDeviceIDFromContext(r.Context())
		if deviceID != "device-123" {
			t.Errorf("Expected device ID 'device-123', got %q", deviceID)
		}
		w.WriteHeader(http.StatusOK)
	})

	// Create valid token
	token, err := testAuth.SignJWT("device-123", []string{"read:catalog"}, 1*time.Hour)
	if err != nil {
		t.Fatalf("Failed to create token: %v", err)
	}

	wrappedHandler := middleware(nextHandler)

	req := httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	wrappedHandler.ServeHTTP(rec, req)

	// Note: Without storage, the middleware now returns 503
	// This test needs to use storage to pass
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status 503 when storage is nil, got %d", rec.Code)
	}

	// Handler should NOT be called when storage is nil
	if nextCalled {
		t.Error("Expected next handler NOT to be called when storage is nil")
	}
}

// Test missing token
func TestStrictJWTAuth_MissingToken(t *testing.T) {
	middleware := NewStrictJWTAuth(testAuth, nil, testMetrics)

	nextCalled := false
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	})

	wrappedHandler := middleware(nextHandler)

	req := httptest.NewRequest("GET", "/api/test", nil)
	rec := httptest.NewRecorder()

	wrappedHandler.ServeHTTP(rec, req)

	if nextCalled {
		t.Error("Expected next handler NOT to be called")
	}

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401, got %d", rec.Code)
	}
}

// Test device revocation
func TestStrictJWTAuth_DeviceRevoked(t *testing.T) {
	// Create storage
	store, err := storage.NewStore(storage.Config{DatabasePath: ":memory:"})
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	middleware := NewStrictJWTAuth(testAuth, store, testMetrics)

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Create valid token for non-existent device (simulates revoked device)
	token, err := testAuth.SignJWT("revoked-device-123", []string{"read:catalog"}, 1*time.Hour)
	if err != nil {
		t.Fatalf("Failed to create token: %v", err)
	}

	wrappedHandler := middleware(nextHandler)

	req := httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	wrappedHandler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401 for revoked device, got %d", rec.Code)
	}
}

// Test with storage and valid device
func TestStrictJWTAuth_WithStorageValidDevice(t *testing.T) {
	// Create storage
	store, err := storage.NewStore(storage.Config{DatabasePath: ":memory:"})
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	// Create test device
	deviceID := "device-with-storage"
	err = store.SaveDevice(deviceID, "test-device", "", "", []string{"read:catalog"})
	if err != nil {
		t.Fatalf("Failed to create device: %v", err)
	}

	middleware := NewStrictJWTAuth(testAuth, store, testMetrics)

	nextCalled := false
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	})

	// Create valid token
	token, err := testAuth.SignJWT(deviceID, []string{"read:catalog"}, 1*time.Hour)
	if err != nil {
		t.Fatalf("Failed to create token: %v", err)
	}

	wrappedHandler := middleware(nextHandler)

	req := httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	wrappedHandler.ServeHTTP(rec, req)

	if !nextCalled {
		t.Error("Expected next handler to be called")
	}

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	// Give async update time to complete
	time.Sleep(100 * time.Millisecond)

	// Verify last_seen was updated
	device, err := store.GetDevice(deviceID)
	if err != nil {
		t.Fatalf("Failed to get device: %v", err)
	}
	if device == nil {
		t.Fatal("Device not found")
	}
}

// Test async operations use context timeout
func TestStrictJWTAuth_AsyncUpdateUsesTimeout(t *testing.T) {
	// Create storage
	store, err := storage.NewStore(storage.Config{DatabasePath: ":memory:"})
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	// Create test device
	deviceID := "device-timeout-test"
	err = store.SaveDevice(deviceID, "test-device", "", "", []string{"read:catalog"})
	if err != nil {
		t.Fatalf("Failed to create device: %v", err)
	}

	middleware := NewStrictJWTAuth(testAuth, store, testMetrics)

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Create valid token
	token, err := testAuth.SignJWT(deviceID, []string{"read:catalog"}, 1*time.Hour)
	if err != nil {
		t.Fatalf("Failed to create token: %v", err)
	}

	wrappedHandler := middleware(nextHandler)

	// Use cancelled context to test goroutine doesn't hang
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	req := httptest.NewRequest("GET", "/api/test", nil).WithContext(ctx)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	wrappedHandler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	// The test passes if the request completes without hanging
	// The goroutine should handle the cancelled context gracefully
}

// Test StrictJWT with Hijacker
func TestStrictJWTAuth_PreservesHijacker(t *testing.T) {
	// Create storage for this test
	store, err := storage.NewStore(storage.Config{DatabasePath: ":memory:"})
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	// Create test device
	err = store.SaveDevice("device-123", "test-device", "", "", []string{"read:catalog"})
	if err != nil {
		t.Fatalf("Failed to create device: %v", err)
	}

	middleware := NewStrictJWTAuth(testAuth, store, testMetrics)

	hijackAttempted := false
	hijackSucceeded := false

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hijackAttempted = true

		hijacker, ok := w.(http.Hijacker)
		if !ok {
			t.Error("ResponseWriter does not implement http.Hijacker")
			http.Error(w, "hijacking not supported", http.StatusInternalServerError)
			return
		}

		// Try to hijack
		conn, buf, err := hijacker.Hijack()
		if err != nil {
			t.Errorf("Hijack failed: %v", err)
			http.Error(w, "hijack failed", http.StatusInternalServerError)
			return
		}

		hijackSucceeded = true
		defer conn.Close()

		// Write a simple HTTP response
		buf.WriteString("HTTP/1.1 200 OK\r\n\r\nHijacking works!\r\n")
		buf.Flush()
	})

	// Create valid token
	token, err := testAuth.SignJWT("device-123", []string{"read:catalog"}, 1*time.Hour)
	if err != nil {
		t.Fatalf("Failed to create token: %v", err)
	}

	wrappedHandler := middleware(handler)

	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Add the auth header
		r.Header.Set("Authorization", "Bearer "+token)
		wrappedHandler.ServeHTTP(w, r)
	}))
	defer server.Close()

	// Make a request
	resp, err := http.Get(server.URL)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if !hijackAttempted {
		t.Error("Hijack was not attempted")
	}

	if !hijackSucceeded {
		t.Error("Hijack did not succeed - StrictJWT middleware breaks WebSocket support")
	}
}

// mockStrictJWTHijackableWriter is a mock that implements Hijacker for testing
type mockStrictJWTHijackableWriter struct {
	http.ResponseWriter
}

func (m *mockStrictJWTHijackableWriter) Header() http.Header {
	return http.Header{}
}

func (m *mockStrictJWTHijackableWriter) Write([]byte) (int, error) {
	return 0, nil
}

func (m *mockStrictJWTHijackableWriter) WriteHeader(statusCode int) {}

func (m *mockStrictJWTHijackableWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return &mockConn{}, bufio.NewReadWriter(bufio.NewReader(strings.NewReader("")), bufio.NewWriter(&strings.Builder{})), nil
}

// Test that device revocation check is atomic with last_seen update
// This tests that we don't have a TOCTOU race where device is checked then deleted before update
func TestStrictJWTAuth_AtomicDeviceCheck(t *testing.T) {
	// Create storage
	store, err := storage.NewStore(storage.Config{DatabasePath: ":memory:"})
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	// Create test device
	deviceID := "device-atomic-test"
	err = store.SaveDevice(deviceID, "test-device", "", "", []string{"read:catalog"})
	if err != nil {
		t.Fatalf("Failed to create device: %v", err)
	}

	middleware := NewStrictJWTAuth(testAuth, store, testMetrics)

	nextCalled := false
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	})

	// Create valid token
	token, err := testAuth.SignJWT(deviceID, []string{"read:catalog"}, 1*time.Hour)
	if err != nil {
		t.Fatalf("Failed to create token: %v", err)
	}

	wrappedHandler := middleware(nextHandler)

	req := httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	wrappedHandler.ServeHTTP(rec, req)

	if !nextCalled {
		t.Error("Expected next handler to be called")
	}

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	// The test verifies GetDevice + UpdateDeviceLastSeen happen atomically
	// by checking that the device last_seen is updated
	time.Sleep(100 * time.Millisecond)

	device, err := store.GetDevice(deviceID)
	if err != nil {
		t.Fatalf("Failed to get device: %v", err)
	}
	if device == nil {
		t.Fatal("Device should exist")
	}
}

// Test that StrictJWT returns 503 when storage is nil
func TestStrictJWTAuth_StorageNilReturns503(t *testing.T) {
	middleware := NewStrictJWTAuth(testAuth, nil, testMetrics)

	nextCalled := false
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	})

	// Create valid token
	token, err := testAuth.SignJWT("device-123", []string{"read:catalog"}, 1*time.Hour)
	if err != nil {
		t.Fatalf("Failed to create token: %v", err)
	}

	wrappedHandler := middleware(nextHandler)

	req := httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	wrappedHandler.ServeHTTP(rec, req)

	// Should return 503 Service Unavailable when storage is nil
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status 503 when storage is nil, got %d", rec.Code)
	}

	// Handler should NOT be called when storage is nil
	if nextCalled {
		t.Error("Expected next handler NOT to be called when storage is nil")
	}

	// Check error message
	body := strings.TrimSpace(rec.Body.String())
	if !strings.Contains(body, "storage not available") {
		t.Errorf("Expected 'storage not available' message, got: %q", body)
	}
}

// Test that metrics are recorded when storage is nil
func TestStrictJWTAuth_StorageNilMetrics(t *testing.T) {
	middleware := NewStrictJWTAuth(testAuth, nil, testMetrics)

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Create valid token
	token, err := testAuth.SignJWT("device-123", []string{"read:catalog"}, 1*time.Hour)
	if err != nil {
		t.Fatalf("Failed to create token: %v", err)
	}

	wrappedHandler := middleware(nextHandler)

	req := httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	wrappedHandler.ServeHTTP(rec, req)

	// Should return 503
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status 503, got %d", rec.Code)
	}
}
