package errors

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAppError_Error(t *testing.T) {
	tests := []struct {
		name     string
		appErr   *AppError
		expected string
	}{
		{
			name: "error without cause",
			appErr: &AppError{
				Code:    "TEST_ERROR",
				Message: "test error message",
				Status:  http.StatusBadRequest,
			},
			expected: "test error message",
		},
		{
			name: "error with cause",
			appErr: &AppError{
				Code:    "WRAPPED_ERROR",
				Message: "wrapped error",
				Cause:   fmt.Errorf("original error"),
				Status:  http.StatusInternalServerError,
			},
			expected: "wrapped error: original error",
		},
		{
			name: "error with nil cause",
			appErr: &AppError{
				Code:    "NIL_CAUSE",
				Message: "error with nil cause",
				Cause:   nil,
				Status:  http.StatusBadRequest,
			},
			expected: "error with nil cause",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.appErr.Error()
			if got != tt.expected {
				t.Errorf("Error() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestAppError_Unwrap(t *testing.T) {
	t.Run("unwraps cause error", func(t *testing.T) {
		cause := fmt.Errorf("underlying error")
		appErr := &AppError{
			Code:    "WRAPPED",
			Message: "wrapper",
			Cause:   cause,
			Status:  http.StatusInternalServerError,
		}

		unwrapped := appErr.Unwrap()
		if unwrapped != cause {
			t.Errorf("Unwrap() = %v, want %v", unwrapped, cause)
		}
	})

	t.Run("returns nil when no cause", func(t *testing.T) {
		appErr := &AppError{
			Code:    "NO_CAUSE",
			Message: "no underlying error",
			Status:  http.StatusBadRequest,
		}

		unwrapped := appErr.Unwrap()
		if unwrapped != nil {
			t.Errorf("Unwrap() = %v, want nil", unwrapped)
		}
	})
}

func TestNew(t *testing.T) {
	code := "TEST_CODE"
	message := "test message"
	status := http.StatusNotFound

	appErr := New(code, message, status)

	if appErr.Code != code {
		t.Errorf("Code = %q, want %q", appErr.Code, code)
	}
	if appErr.Message != message {
		t.Errorf("Message = %q, want %q", appErr.Message, message)
	}
	if appErr.Status != status {
		t.Errorf("Status = %d, want %d", appErr.Status, status)
	}
	if appErr.Cause != nil {
		t.Errorf("Cause = %v, want nil", appErr.Cause)
	}
}

func TestNewWithCode(t *testing.T) {
	code := "TEST_CODE"
	numericCode := 1234
	message := "test message"
	status := http.StatusNotFound

	appErr := NewWithCode(code, numericCode, message, status)

	if appErr.Code != code {
		t.Errorf("Code = %q, want %q", appErr.Code, code)
	}
	if appErr.NumericCode != numericCode {
		t.Errorf("NumericCode = %d, want %d", appErr.NumericCode, numericCode)
	}
	if appErr.Message != message {
		t.Errorf("Message = %q, want %q", appErr.Message, message)
	}
	if appErr.Status != status {
		t.Errorf("Status = %d, want %d", appErr.Status, status)
	}
}

func TestWrap(t *testing.T) {
	originalErr := fmt.Errorf("database connection failed")
	code := "DB_ERROR"
	message := "failed to connect to database"
	status := http.StatusInternalServerError

	appErr := Wrap(originalErr, code, message, status)

	if appErr.Code != code {
		t.Errorf("Code = %q, want %q", appErr.Code, code)
	}
	if appErr.Message != message {
		t.Errorf("Message = %q, want %q", appErr.Message, message)
	}
	if appErr.Status != status {
		t.Errorf("Status = %d, want %d", appErr.Status, status)
	}
	if appErr.Cause != originalErr {
		t.Errorf("Cause = %v, want %v", appErr.Cause, originalErr)
	}

	// Verify unwrap works
	if unwrapped := appErr.Unwrap(); unwrapped != originalErr {
		t.Errorf("Unwrap() = %v, want %v", unwrapped, originalErr)
	}

	// Verify errors.Is works
	if !errors.Is(appErr, originalErr) {
		t.Error("errors.Is() should return true for wrapped error")
	}
}

func TestWrapWithCode(t *testing.T) {
	originalErr := fmt.Errorf("database connection failed")
	code := "DB_ERROR"
	numericCode := 6001
	message := "failed to connect to database"
	status := http.StatusInternalServerError

	appErr := WrapWithCode(originalErr, code, numericCode, message, status)

	if appErr.Code != code {
		t.Errorf("Code = %q, want %q", appErr.Code, code)
	}
	if appErr.NumericCode != numericCode {
		t.Errorf("NumericCode = %d, want %d", appErr.NumericCode, numericCode)
	}
	if appErr.Cause != originalErr {
		t.Errorf("Cause = %v, want %v", appErr.Cause, originalErr)
	}
}

func TestCommonErrors(t *testing.T) {
	tests := []struct {
		name            string
		err             *AppError
		wantCode        string
		wantNumericCode int
		wantStatus      int
	}{
		{"ErrTokenExpired", ErrTokenExpired, "TOKEN_EXPIRED", CodeTokenExpired, http.StatusUnauthorized},
		{"ErrTokenInvalid", ErrTokenInvalid, "TOKEN_INVALID", CodeTokenInvalid, http.StatusUnauthorized},
		{"ErrTokenRevoked", ErrTokenRevoked, "TOKEN_REVOKED", CodeTokenRevoked, http.StatusUnauthorized},
		{"ErrDeviceRevoked", ErrDeviceRevoked, "DEVICE_REVOKED", CodeDeviceRevoked, http.StatusUnauthorized},
		{"ErrAuthRequired", ErrAuthRequired, "AUTH_REQUIRED", CodeAuthRequired, http.StatusUnauthorized},
		{"ErrPermissionDenied", ErrPermissionDenied, "PERMISSION_DENIED", CodePermissionDenied, http.StatusForbidden},
		{"ErrResourceForbidden", ErrResourceForbidden, "RESOURCE_FORBIDDEN", CodeResourceForbidden, http.StatusForbidden},
		{"ErrResourceNotFound", ErrResourceNotFound, "RESOURCE_NOT_FOUND", CodeResourceNotFound, http.StatusNotFound},
		{"ErrRateLimited", ErrRateLimited, "RATE_LIMITED", CodeRateLimited, http.StatusTooManyRequests},
		{"ErrBadRequest", ErrBadRequest, "BAD_REQUEST", CodeBadRequest, http.StatusBadRequest},
		{"ErrInvalidBootstrap", ErrInvalidBootstrap, "INVALID_BOOTSTRAP", CodeInvalidBootstrap, http.StatusUnauthorized},
		{"ErrInternalServer", ErrInternalServer, "INTERNAL_ERROR", CodeInternalError, http.StatusInternalServerError},
		{"ErrBadGateway", ErrBadGateway, "BAD_GATEWAY", CodeBadGateway, http.StatusBadGateway},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err.Code != tt.wantCode {
				t.Errorf("Code = %q, want %q", tt.err.Code, tt.wantCode)
			}
			if tt.err.NumericCode != tt.wantNumericCode {
				t.Errorf("NumericCode = %d, want %d", tt.err.NumericCode, tt.wantNumericCode)
			}
			if tt.err.Status != tt.wantStatus {
				t.Errorf("Status = %d, want %d", tt.err.Status, tt.wantStatus)
			}
			if tt.err.Message == "" {
				t.Error("Message should not be empty")
			}
		})
	}
}

func TestLegacyAliases(t *testing.T) {
	// Verify legacy aliases point to new errors
	if ErrUnauthorized != ErrAuthRequired {
		t.Error("ErrUnauthorized should be alias of ErrAuthRequired")
	}
	if ErrInvalidToken != ErrTokenInvalid {
		t.Error("ErrInvalidToken should be alias of ErrTokenInvalid")
	}
	if ErrForbidden != ErrPermissionDenied {
		t.Error("ErrForbidden should be alias of ErrPermissionDenied")
	}
	if ErrNotFound != ErrResourceNotFound {
		t.Error("ErrNotFound should be alias of ErrResourceNotFound")
	}
	if ErrRateLimitExceeded != ErrRateLimited {
		t.Error("ErrRateLimitExceeded should be alias of ErrRateLimited")
	}
}

func TestWriteJSON(t *testing.T) {
	tests := []struct {
		name           string
		err            error
		expectedStatus int
		expectedCode   string
		expectedMsg    string
		expectedNum    int
	}{
		{
			name:           "AppError with numeric code",
			err:            NewWithCode("TEST_ERROR", 9999, "test error message", http.StatusBadRequest),
			expectedStatus: http.StatusBadRequest,
			expectedCode:   "TEST_ERROR",
			expectedMsg:    "test error message",
			expectedNum:    9999,
		},
		{
			name:           "AppError without numeric code",
			err:            New("TEST_ERROR", "test error message", http.StatusBadRequest),
			expectedStatus: http.StatusBadRequest,
			expectedCode:   "TEST_ERROR",
			expectedMsg:    "test error message",
			expectedNum:    -1, // Special value: -1 means omit code field from JSON
		},
		{
			name:           "ErrTokenExpired",
			err:            ErrTokenExpired,
			expectedStatus: http.StatusUnauthorized,
			expectedCode:   "TOKEN_EXPIRED",
			expectedMsg:    "JWT token has expired",
			expectedNum:    CodeTokenExpired,
		},
		{
			name:           "ErrDeviceRevoked",
			err:            ErrDeviceRevoked,
			expectedStatus: http.StatusUnauthorized,
			expectedCode:   "DEVICE_REVOKED",
			expectedMsg:    "This device has been revoked",
			expectedNum:    CodeDeviceRevoked,
		},
		{
			name:           "non-AppError (standard error)",
			err:            fmt.Errorf("some random error"),
			expectedStatus: http.StatusInternalServerError,
			expectedCode:   "INTERNAL_ERROR",
			expectedMsg:    "Internal server error",
			expectedNum:    CodeInternalError,
		},
		{
			name:           "ErrResourceNotFound",
			err:            ErrResourceNotFound,
			expectedStatus: http.StatusNotFound,
			expectedCode:   "RESOURCE_NOT_FOUND",
			expectedMsg:    "Resource not found",
			expectedNum:    CodeResourceNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()

			WriteJSON(w, tt.err)

			// Check status code
			if w.Code != tt.expectedStatus {
				t.Errorf("Status = %d, want %d", w.Code, tt.expectedStatus)
			}

			// Check Content-Type header
			contentType := w.Header().Get("Content-Type")
			if contentType != "application/json" {
				t.Errorf("Content-Type = %q, want %q", contentType, "application/json")
			}

			// Check response body - new flat format
			body := w.Body.String()
			var expectedBody string
			if tt.expectedNum == -1 {
				// Code field should be omitted when NumericCode is 0
				expectedBody = fmt.Sprintf(`{"error":"%s","message":"%s"}`, tt.expectedCode, tt.expectedMsg)
			} else {
				expectedBody = fmt.Sprintf(`{"error":"%s","message":"%s","code":%d}`, tt.expectedCode, tt.expectedMsg, tt.expectedNum)
			}
			if body != expectedBody {
				t.Errorf("Body = %q, want %q", body, expectedBody)
			}
		})
	}
}

func TestWriteJSON_ContentType(t *testing.T) {
	w := httptest.NewRecorder()
	err := New("TEST", "test", http.StatusBadRequest)

	WriteJSON(w, err)

	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/json")
	}
}

func TestWriteJSON_ValidJSON(t *testing.T) {
	w := httptest.NewRecorder()
	err := NewWithCode("CODE", 1234, "message", http.StatusBadRequest)

	WriteJSON(w, err)

	body := w.Body.String()
	// New flat format
	if !strings.HasPrefix(body, `{"error":`) {
		t.Errorf("Response doesn't look like JSON: %q", body)
	}
	if !strings.Contains(body, `"error":"CODE"`) {
		t.Errorf("Response missing error field: %q", body)
	}
	if !strings.Contains(body, `"message":"message"`) {
		t.Errorf("Response missing message field: %q", body)
	}
	if !strings.Contains(body, `"code":1234`) {
		t.Errorf("Response missing code field: %q", body)
	}
}

func TestIsAuthError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{"ErrTokenExpired", ErrTokenExpired, true},
		{"ErrTokenInvalid", ErrTokenInvalid, true},
		{"ErrTokenRevoked", ErrTokenRevoked, true},
		{"ErrDeviceRevoked", ErrDeviceRevoked, true},
		{"ErrAuthRequired", ErrAuthRequired, true},
		{"ErrPermissionDenied", ErrPermissionDenied, false},
		{"ErrResourceNotFound", ErrResourceNotFound, false},
		{"ErrBadRequest", ErrBadRequest, false},
		{"standard error", fmt.Errorf("error"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsAuthError(tt.err); got != tt.expected {
				t.Errorf("IsAuthError() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestRequiresLogout(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{"ErrTokenExpired", ErrTokenExpired, false},  // Should refresh, not logout
		{"ErrTokenInvalid", ErrTokenInvalid, true},   // Should logout
		{"ErrTokenRevoked", ErrTokenRevoked, true},   // Should logout
		{"ErrDeviceRevoked", ErrDeviceRevoked, true}, // Should logout
		{"ErrAuthRequired", ErrAuthRequired, false},  // Should authenticate, not logout
		{"ErrPermissionDenied", ErrPermissionDenied, false},
		{"standard error", fmt.Errorf("error"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RequiresLogout(tt.err); got != tt.expected {
				t.Errorf("RequiresLogout() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestRequiresRefresh(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{"ErrTokenExpired", ErrTokenExpired, true},
		{"ErrTokenInvalid", ErrTokenInvalid, false},
		{"ErrTokenRevoked", ErrTokenRevoked, false},
		{"ErrDeviceRevoked", ErrDeviceRevoked, false},
		{"ErrAuthRequired", ErrAuthRequired, false},
		{"standard error", fmt.Errorf("error"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RequiresRefresh(tt.err); got != tt.expected {
				t.Errorf("RequiresRefresh() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestNumericCodes(t *testing.T) {
	// Verify numeric code ranges are correct
	if CodeTokenExpired != 1001 {
		t.Errorf("CodeTokenExpired = %d, want 1001", CodeTokenExpired)
	}
	if CodePermissionDenied != 2001 {
		t.Errorf("CodePermissionDenied = %d, want 2001", CodePermissionDenied)
	}
	if CodeResourceNotFound != 3001 {
		t.Errorf("CodeResourceNotFound = %d, want 3001", CodeResourceNotFound)
	}
	if CodeRateLimited != 4001 {
		t.Errorf("CodeRateLimited = %d, want 4001", CodeRateLimited)
	}
	if CodeBadRequest != 5001 {
		t.Errorf("CodeBadRequest = %d, want 5001", CodeBadRequest)
	}
	if CodeInternalError != 6001 {
		t.Errorf("CodeInternalError = %d, want 6001", CodeInternalError)
	}
}
