package errors

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// Numeric error codes for Portal compatibility
// These codes help Portal distinguish between different error conditions
const (
	// Authentication errors (1xxx) - 401 Unauthorized
	CodeTokenExpired  = 1001 // JWT has expired - Portal should attempt refresh
	CodeTokenInvalid  = 1002 // JWT signature invalid or malformed
	CodeTokenRevoked  = 1003 // Token explicitly revoked
	CodeDeviceRevoked = 1004 // Device has been revoked - Portal should logout
	CodeAuthRequired  = 1005 // No token provided

	// Authorization errors (2xxx) - 403 Forbidden
	CodePermissionDenied  = 2001 // Valid token, insufficient permissions
	CodeResourceForbidden = 2002 // Cannot access this specific resource

	// Resource errors (3xxx) - 404 Not Found
	CodeResourceNotFound = 3001 // Resource does not exist

	// Rate limiting errors (4xxx) - 429 Too Many Requests
	CodeRateLimited = 4001 // Too many requests

	// Client errors (5xxx) - 400 Bad Request
	CodeBadRequest       = 5001 // Generic bad request
	CodeInvalidBootstrap = 5002 // Invalid bootstrap token
	CodeValidationFailed = 5003 // Request validation failed

	// Server errors (6xxx) - 500/502/503
	CodeInternalError = 6001 // Internal server error
	CodeBadGateway    = 6002 // Service temporarily unavailable
)

// AppError represents an application error with HTTP status, error code, and numeric code
type AppError struct {
	Code        string // String error code (e.g., "TOKEN_EXPIRED")
	NumericCode int    // Numeric error code for Portal (e.g., 1001)
	Message     string
	Cause       error
	Status      int // HTTP status code
}

func (e *AppError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Cause)
	}
	return e.Message
}

func (e *AppError) Unwrap() error {
	return e.Cause
}

// New creates a new AppError with string code only (numeric code defaults to 0)
func New(code, message string, status int) *AppError {
	return &AppError{
		Code:    code,
		Message: message,
		Status:  status,
	}
}

// NewWithCode creates a new AppError with both string and numeric codes
func NewWithCode(code string, numericCode int, message string, status int) *AppError {
	return &AppError{
		Code:        code,
		NumericCode: numericCode,
		Message:     message,
		Status:      status,
	}
}

// Wrap wraps an existing error with context
func Wrap(err error, code, message string, status int) *AppError {
	return &AppError{
		Code:    code,
		Message: message,
		Cause:   err,
		Status:  status,
	}
}

// WrapWithCode wraps an existing error with both string and numeric codes
func WrapWithCode(err error, code string, numericCode int, message string, status int) *AppError {
	return &AppError{
		Code:        code,
		NumericCode: numericCode,
		Message:     message,
		Cause:       err,
		Status:      status,
	}
}

// Authentication errors (401) - require different Portal handling
var (
	// ErrTokenExpired - Portal should attempt token refresh
	ErrTokenExpired = &AppError{
		Code:        "TOKEN_EXPIRED",
		NumericCode: CodeTokenExpired,
		Message:     "JWT token has expired",
		Status:      http.StatusUnauthorized,
	}

	// ErrTokenInvalid - Portal should logout (token is corrupt)
	ErrTokenInvalid = &AppError{
		Code:        "TOKEN_INVALID",
		NumericCode: CodeTokenInvalid,
		Message:     "Token is invalid or malformed",
		Status:      http.StatusUnauthorized,
	}

	// ErrTokenRevoked - Portal should logout immediately
	ErrTokenRevoked = &AppError{
		Code:        "TOKEN_REVOKED",
		NumericCode: CodeTokenRevoked,
		Message:     "This token has been revoked",
		Status:      http.StatusUnauthorized,
	}

	// ErrDeviceRevoked - Portal should logout immediately and clear pairing
	ErrDeviceRevoked = &AppError{
		Code:        "DEVICE_REVOKED",
		NumericCode: CodeDeviceRevoked,
		Message:     "This device has been revoked",
		Status:      http.StatusUnauthorized,
	}

	// ErrAuthRequired - Portal should prompt for authentication
	ErrAuthRequired = &AppError{
		Code:        "AUTH_REQUIRED",
		NumericCode: CodeAuthRequired,
		Message:     "Authentication required",
		Status:      http.StatusUnauthorized,
	}

	// Legacy aliases for backward compatibility
	ErrUnauthorized = ErrAuthRequired
	ErrInvalidToken = ErrTokenInvalid
)

// Authorization errors (403) - Portal should NOT logout
var (
	// ErrPermissionDenied - valid token but insufficient permissions
	ErrPermissionDenied = &AppError{
		Code:        "PERMISSION_DENIED",
		NumericCode: CodePermissionDenied,
		Message:     "You do not have permission to perform this action",
		Status:      http.StatusForbidden,
	}

	// ErrResourceForbidden - cannot access this specific resource
	ErrResourceForbidden = &AppError{
		Code:        "RESOURCE_FORBIDDEN",
		NumericCode: CodeResourceForbidden,
		Message:     "Access to this resource is forbidden",
		Status:      http.StatusForbidden,
	}

	// Legacy alias
	ErrForbidden = ErrPermissionDenied
)

// Resource errors (404)
var (
	// ErrResourceNotFound - resource does not exist
	ErrResourceNotFound = &AppError{
		Code:        "RESOURCE_NOT_FOUND",
		NumericCode: CodeResourceNotFound,
		Message:     "Resource not found",
		Status:      http.StatusNotFound,
	}

	// Legacy alias
	ErrNotFound = ErrResourceNotFound
)

// Rate limiting errors (429)
var (
	// ErrRateLimited - too many requests
	ErrRateLimited = &AppError{
		Code:        "RATE_LIMITED",
		NumericCode: CodeRateLimited,
		Message:     "Too many requests",
		Status:      http.StatusTooManyRequests,
	}

	// Legacy alias
	ErrRateLimitExceeded = ErrRateLimited
)

// Client errors (400)
var (
	// ErrBadRequest - generic bad request
	ErrBadRequest = &AppError{
		Code:        "BAD_REQUEST",
		NumericCode: CodeBadRequest,
		Message:     "Invalid request",
		Status:      http.StatusBadRequest,
	}

	// ErrInvalidBootstrap - invalid bootstrap token
	ErrInvalidBootstrap = &AppError{
		Code:        "INVALID_BOOTSTRAP",
		NumericCode: CodeInvalidBootstrap,
		Message:     "Invalid or expired bootstrap token",
		Status:      http.StatusUnauthorized,
	}

	// ErrValidationFailed - request validation failed
	ErrValidationFailed = &AppError{
		Code:        "VALIDATION_FAILED",
		NumericCode: CodeValidationFailed,
		Message:     "Request validation failed",
		Status:      http.StatusBadRequest,
	}
)

// Server errors (500/502)
var (
	// ErrInternalServer - internal server error
	ErrInternalServer = &AppError{
		Code:        "INTERNAL_ERROR",
		NumericCode: CodeInternalError,
		Message:     "Internal server error",
		Status:      http.StatusInternalServerError,
	}

	// ErrBadGateway - service temporarily unavailable
	ErrBadGateway = &AppError{
		Code:        "BAD_GATEWAY",
		NumericCode: CodeBadGateway,
		Message:     "Service temporarily unavailable",
		Status:      http.StatusBadGateway,
	}
)

// WriteJSON writes an error response as JSON in the spec format:
// {"error": "ERROR_CODE", "message": "Human readable description", "code": 1001}
func WriteJSON(w http.ResponseWriter, err error) {
	var appErr *AppError
	if e, ok := err.(*AppError); ok {
		appErr = e
	} else {
		appErr = ErrInternalServer
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(appErr.Status)

	// New flat format as per security spec
	response := struct {
		Error   string `json:"error"`
		Message string `json:"message"`
		Code    int    `json:"code,omitempty"`
	}{
		Error:   appErr.Code,
		Message: appErr.Message,
		Code:    appErr.NumericCode,
	}

	data, _ := json.Marshal(response)
	w.Write(data)
}

// IsAuthError returns true if the error is an authentication error (requires logout/refresh)
func IsAuthError(err error) bool {
	if appErr, ok := err.(*AppError); ok {
		return appErr.NumericCode >= 1001 && appErr.NumericCode <= 1005
	}
	return false
}

// RequiresLogout returns true if the error should trigger a Portal logout
func RequiresLogout(err error) bool {
	if appErr, ok := err.(*AppError); ok {
		// Token invalid, revoked, or device revoked require logout
		return appErr.NumericCode == CodeTokenInvalid ||
			appErr.NumericCode == CodeTokenRevoked ||
			appErr.NumericCode == CodeDeviceRevoked
	}
	return false
}

// RequiresRefresh returns true if the error indicates token refresh should be attempted
func RequiresRefresh(err error) bool {
	if appErr, ok := err.(*AppError); ok {
		return appErr.NumericCode == CodeTokenExpired
	}
	return false
}
