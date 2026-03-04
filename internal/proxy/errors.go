package proxy

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"syscall"
)

// MapErrorToStatus maps proxy errors to appropriate HTTP status codes
// Based on Traefik's error mapping patterns
func MapErrorToStatus(err error) int {
	if err == nil {
		return http.StatusOK
	}

	// Check for EOF (connection closed unexpectedly)
	if errors.Is(err, io.EOF) {
		return http.StatusBadGateway
	}

	// Check for context cancellation (client closed request)
	if errors.Is(err, context.Canceled) {
		return 499 // Non-standard but widely used (nginx, Traefik)
	}

	// Check for timeout errors
	if IsTimeoutError(err) {
		return http.StatusGatewayTimeout
	}

	// Check for specific network errors that should return Service Unavailable
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		// Host or network unreachable should return 503 Service Unavailable
		if errors.Is(opErr.Err, syscall.EHOSTUNREACH) || errors.Is(opErr.Err, syscall.ENETUNREACH) {
			return http.StatusServiceUnavailable
		}
	}

	// Check for other network errors
	if IsNetworkError(err) {
		return http.StatusBadGateway
	}

	// Default to Bad Gateway for unknown errors
	return http.StatusBadGateway
}

// GetErrorLabel returns a metrics-friendly label for the error
func GetErrorLabel(err error) string {
	if err == nil {
		return "success"
	}

	// EOF
	if errors.Is(err, io.EOF) {
		return "bad_gateway"
	}

	// Context cancellation
	if errors.Is(err, context.Canceled) {
		return "client_closed"
	}

	// Timeout
	if IsTimeoutError(err) {
		return "timeout"
	}

	// Network errors - check specific types
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		// Connection refused
		if errors.Is(opErr.Err, syscall.ECONNREFUSED) {
			return "connection_refused"
		}
		// Connection reset
		if errors.Is(opErr.Err, syscall.ECONNRESET) {
			return "connection_reset"
		}
		// Host unreachable
		if errors.Is(opErr.Err, syscall.EHOSTUNREACH) {
			return "host_unreachable"
		}
		// Network unreachable
		if errors.Is(opErr.Err, syscall.ENETUNREACH) {
			return "network_unreachable"
		}
		// Broken pipe
		if errors.Is(opErr.Err, syscall.EPIPE) {
			return "broken_pipe"
		}
		// Connection aborted
		if errors.Is(opErr.Err, syscall.ECONNABORTED) {
			return "connection_aborted"
		}
		// Generic network error
		return "network_error"
	}

	// DNS errors
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return "dns_error"
	}

	// Unknown error
	return "unknown"
}

// GetErrorMessage returns a user-friendly message for the status code
func GetErrorMessage(statusCode int) string {
	switch statusCode {
	case 499:
		return "Client Closed Request"
	case http.StatusBadGateway:
		return "Bad Gateway"
	case http.StatusGatewayTimeout:
		return "Gateway Timeout"
	case http.StatusServiceUnavailable:
		return "Service Unavailable"
	default:
		return http.StatusText(statusCode)
	}
}

// IsTimeoutError checks if an error is a timeout error
func IsTimeoutError(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	// Check for net.OpError with deadline exceeded
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		if errors.Is(opErr.Err, os.ErrDeadlineExceeded) {
			return true
		}
	}

	// Check if error implements Timeout() bool interface
	type timeoutError interface {
		Timeout() bool
	}
	var te timeoutError
	if errors.As(err, &te) {
		return te.Timeout()
	}

	return false
}

// IsNetworkError checks if an error is a network-related error
func IsNetworkError(err error) bool {
	// Check for net.OpError
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return true
	}

	// Check for net.DNSError
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return true
	}

	return false
}
