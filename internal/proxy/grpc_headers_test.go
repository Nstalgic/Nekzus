package proxy

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGRPCHeaderResponseWriter_AddsHeaders(t *testing.T) {
	recorder := httptest.NewRecorder()
	writer := NewGRPCHeaderResponseWriter(recorder)

	// Write response
	writer.WriteHeader(http.StatusOK)

	// Check gRPC CORS headers
	exposeHeaders := recorder.Header().Get("Access-Control-Expose-Headers")
	assert.Contains(t, exposeHeaders, "grpc-status", "Should expose grpc-status header")
	assert.Contains(t, exposeHeaders, "grpc-message", "Should expose grpc-message header")
	assert.Contains(t, exposeHeaders, "grpc-status-details-bin", "Should expose grpc-status-details-bin header")
}

func TestGRPCHeaderResponseWriter_AddsAllowOrigin(t *testing.T) {
	recorder := httptest.NewRecorder()
	writer := NewGRPCHeaderResponseWriter(recorder)

	writer.WriteHeader(http.StatusOK)

	allowOrigin := recorder.Header().Get("Access-Control-Allow-Origin")
	assert.Equal(t, "*", allowOrigin, "Should allow all origins for gRPC-Web")
}

func TestGRPCHeaderResponseWriter_AddsAllowMethods(t *testing.T) {
	recorder := httptest.NewRecorder()
	writer := NewGRPCHeaderResponseWriter(recorder)

	writer.WriteHeader(http.StatusOK)

	allowMethods := recorder.Header().Get("Access-Control-Allow-Methods")
	assert.Contains(t, allowMethods, "POST", "Should allow POST method")
	assert.Contains(t, allowMethods, "OPTIONS", "Should allow OPTIONS method")
}

func TestGRPCHeaderResponseWriter_AddsAllowHeaders(t *testing.T) {
	recorder := httptest.NewRecorder()
	writer := NewGRPCHeaderResponseWriter(recorder)

	writer.WriteHeader(http.StatusOK)

	allowHeaders := recorder.Header().Get("Access-Control-Allow-Headers")
	assert.Contains(t, allowHeaders, "Content-Type", "Should allow Content-Type header")
	assert.Contains(t, allowHeaders, "X-Grpc-Web", "Should allow X-Grpc-Web header")
	assert.Contains(t, allowHeaders, "grpc-timeout", "Should allow grpc-timeout header")
}

func TestGRPCHeaderResponseWriter_AppendsToExistingExposeHeaders(t *testing.T) {
	recorder := httptest.NewRecorder()
	// Pre-set an existing Expose-Headers value
	recorder.Header().Set("Access-Control-Expose-Headers", "X-Custom-Header")

	writer := NewGRPCHeaderResponseWriter(recorder)
	writer.WriteHeader(http.StatusOK)

	exposeHeaders := recorder.Header().Get("Access-Control-Expose-Headers")
	assert.Contains(t, exposeHeaders, "X-Custom-Header", "Should preserve existing exposed headers")
	assert.Contains(t, exposeHeaders, "grpc-status", "Should append grpc-status header")
	assert.Contains(t, exposeHeaders, "grpc-message", "Should append grpc-message header")
}

func TestGRPCHeaderResponseWriter_DoesNotOverrideExistingCORS(t *testing.T) {
	recorder := httptest.NewRecorder()
	// Pre-set existing CORS headers
	recorder.Header().Set("Access-Control-Allow-Origin", "https://example.com")
	recorder.Header().Set("Access-Control-Allow-Methods", "GET, POST")

	writer := NewGRPCHeaderResponseWriter(recorder)
	writer.WriteHeader(http.StatusOK)

	// Should not override existing headers
	allowOrigin := recorder.Header().Get("Access-Control-Allow-Origin")
	assert.Equal(t, "https://example.com", allowOrigin, "Should not override existing Allow-Origin")

	allowMethods := recorder.Header().Get("Access-Control-Allow-Methods")
	assert.Equal(t, "GET, POST", allowMethods, "Should not override existing Allow-Methods")
}

func TestGRPCHeaderResponseWriter_WriteAddsHeaders(t *testing.T) {
	recorder := httptest.NewRecorder()
	writer := NewGRPCHeaderResponseWriter(recorder)

	// Write body directly (without explicit WriteHeader)
	_, err := writer.Write([]byte("test body"))
	assert.NoError(t, err)

	// Headers should still be added
	exposeHeaders := recorder.Header().Get("Access-Control-Expose-Headers")
	assert.Contains(t, exposeHeaders, "grpc-status", "Should add headers on Write")
}

func TestGRPCHeaderResponseWriter_OnlyAddsHeadersOnce(t *testing.T) {
	recorder := httptest.NewRecorder()
	writer := NewGRPCHeaderResponseWriter(recorder)

	// Write multiple times
	writer.WriteHeader(http.StatusOK)
	_, _ = writer.Write([]byte("part 1"))
	_, _ = writer.Write([]byte("part 2"))

	// Count occurrences of grpc-message in exposed headers (unique to our headers)
	exposeHeaders := recorder.Header().Get("Access-Control-Expose-Headers")
	count := strings.Count(exposeHeaders, "grpc-message")
	assert.Equal(t, 1, count, "Should only add grpc-message once")
}

func TestGRPCHeaderResponseWriter_PreservesStatusCode(t *testing.T) {
	testCases := []int{
		http.StatusOK,
		http.StatusCreated,
		http.StatusBadRequest,
		http.StatusInternalServerError,
	}

	for _, statusCode := range testCases {
		t.Run(http.StatusText(statusCode), func(t *testing.T) {
			recorder := httptest.NewRecorder()
			writer := NewGRPCHeaderResponseWriter(recorder)

			writer.WriteHeader(statusCode)

			assert.Equal(t, statusCode, recorder.Code, "Should preserve status code")
		})
	}
}

func TestGRPCHeaderResponseWriter_PreservesBody(t *testing.T) {
	recorder := httptest.NewRecorder()
	writer := NewGRPCHeaderResponseWriter(recorder)

	testBody := []byte("test response body")
	n, err := writer.Write(testBody)

	assert.NoError(t, err)
	assert.Equal(t, len(testBody), n, "Should return correct byte count")
	assert.Equal(t, testBody, recorder.Body.Bytes(), "Should preserve response body")
}

// TestGRPCHeaderResponseWriter_SkipsIfAllHeadersPresent tests duplicate check
func TestGRPCHeaderResponseWriter_SkipsIfAllHeadersPresent(t *testing.T) {
	recorder := httptest.NewRecorder()
	// Pre-set all gRPC headers
	originalHeaders := "grpc-status, grpc-message, grpc-status-details-bin"
	recorder.Header().Set("Access-Control-Expose-Headers", originalHeaders)

	writer := NewGRPCHeaderResponseWriter(recorder)
	writer.WriteHeader(http.StatusOK)

	exposeHeaders := recorder.Header().Get("Access-Control-Expose-Headers")
	// Should not modify the headers when all are present
	assert.Equal(t, originalHeaders, exposeHeaders, "Should not duplicate headers when all gRPC headers are already present")
}
