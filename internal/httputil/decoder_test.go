package httputil

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	apperrors "github.com/nstalgic/nekzus/internal/errors"
)

// testRequest is a test request type with validation
type testRequest struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

func (r *testRequest) Validate() error {
	if r.Name == "" {
		return apperrors.New("INVALID_NAME", "Name is required", http.StatusBadRequest)
	}
	if r.Email == "" {
		return apperrors.New("INVALID_EMAIL", "Email is required", http.StatusBadRequest)
	}
	return nil
}

// testRequestNoValidation is a test request without validation
type testRequestNoValidation struct {
	Value string `json:"value"`
}

func TestDecodeAndValidate_ValidRequest(t *testing.T) {
	body := `{"name":"John","email":"john@example.com"}`
	r := httptest.NewRequest("POST", "/test", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	req, err := DecodeAndValidate[testRequest](r, w, 1024)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if req.Name != "John" {
		t.Errorf("Expected name 'John', got %q", req.Name)
	}

	if req.Email != "john@example.com" {
		t.Errorf("Expected email 'john@example.com', got %q", req.Email)
	}
}

func TestDecodeAndValidate_ValidationError(t *testing.T) {
	body := `{"name":"","email":"john@example.com"}`
	r := httptest.NewRequest("POST", "/test", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	req, err := DecodeAndValidate[testRequest](r, w, 1024)

	if err == nil {
		t.Fatal("Expected validation error, got nil")
	}

	if req != nil {
		t.Error("Expected nil request on validation error")
	}

	if !strings.Contains(err.Error(), "Name is required") {
		t.Errorf("Expected name validation error, got %v", err)
	}
}

func TestDecodeAndValidate_InvalidJSON(t *testing.T) {
	body := `{invalid json}`
	r := httptest.NewRequest("POST", "/test", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	req, err := DecodeAndValidate[testRequest](r, w, 1024)

	if err == nil {
		t.Fatal("Expected JSON error, got nil")
	}

	if req != nil {
		t.Error("Expected nil request on JSON error")
	}

	appErr, ok := err.(*apperrors.AppError)
	if !ok {
		t.Fatal("Expected AppError type")
	}

	if appErr.Code != "INVALID_JSON" {
		t.Errorf("Expected error code INVALID_JSON, got %q", appErr.Code)
	}
}

func TestDecodeAndValidate_BodyTooLarge(t *testing.T) {
	// Create a body that exceeds the limit with valid JSON structure
	largeValue := strings.Repeat("a", 2000)
	body := `{"name":"` + largeValue + `","email":"test@example.com"}`
	r := httptest.NewRequest("POST", "/test", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	maxSize := int64(1024) // 1KB limit
	req, err := DecodeAndValidate[testRequest](r, w, maxSize)

	if err == nil {
		t.Fatal("Expected body too large error, got nil")
	}

	if req != nil {
		t.Error("Expected nil request on body too large error")
	}

	appErr, ok := err.(*apperrors.AppError)
	if !ok {
		t.Fatalf("Expected AppError type, got %T: %v", err, err)
	}

	if appErr.Code != "PAYLOAD_TOO_LARGE" {
		t.Errorf("Expected error code PAYLOAD_TOO_LARGE, got %q", appErr.Code)
	}

	if appErr.Status != http.StatusRequestEntityTooLarge {
		t.Errorf("Expected status 413, got %d", appErr.Status)
	}
}

func TestDecodeAndValidate_NoValidation(t *testing.T) {
	body := `{"value":"test"}`
	r := httptest.NewRequest("POST", "/test", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	req, err := DecodeAndValidate[testRequestNoValidation](r, w, 1024)

	if err != nil {
		t.Fatalf("Expected no error for type without validation, got %v", err)
	}

	if req.Value != "test" {
		t.Errorf("Expected value 'test', got %q", req.Value)
	}
}

func TestDecodeAndValidate_EmptyBody(t *testing.T) {
	r := httptest.NewRequest("POST", "/test", bytes.NewBufferString(""))
	w := httptest.NewRecorder()

	req, err := DecodeAndValidate[testRequest](r, w, 1024)

	if err == nil {
		t.Fatal("Expected JSON error for empty body, got nil")
	}

	if req != nil {
		t.Error("Expected nil request on empty body")
	}
}

func TestDecodeAndValidate_NilBody(t *testing.T) {
	r := httptest.NewRequest("POST", "/test", nil)
	w := httptest.NewRecorder()

	req, err := DecodeAndValidate[testRequest](r, w, 1024)

	if err == nil {
		t.Fatal("Expected error for nil body, got nil")
	}

	if req != nil {
		t.Error("Expected nil request on nil body")
	}
}

// Benchmark DecodeAndValidate
func BenchmarkDecodeAndValidate(b *testing.B) {
	body := `{"name":"John","email":"john@example.com"}`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r := httptest.NewRequest("POST", "/test", bytes.NewBufferString(body))
		w := httptest.NewRecorder()

		_, err := DecodeAndValidate[testRequest](r, w, 1024)
		if err != nil {
			b.Fatal(err)
		}
	}
}
