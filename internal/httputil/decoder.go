package httputil

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	apperrors "github.com/nstalgic/nekzus/internal/errors"
)

// DecodeAndValidate reads a JSON request body, decodes it into type T, and validates it.
// This eliminates code duplication across handlers that all do the same error handling.
//
// The function:
// 1. Limits the request body size to prevent DoS attacks
// 2. Decodes JSON into the provided type T
// 3. Validates the decoded struct if it implements Validate() error
//
// Usage example:
//
//	req, err := httputil.DecodeAndValidate[PairRequest](r, w, constants.MaxAuthRequestBodySize)
//	if err != nil {
//	    apperrors.WriteJSON(w, err)
//	    return
//	}
//	// Use req...
func DecodeAndValidate[T any](r *http.Request, w http.ResponseWriter, maxSize int64) (*T, error) {
	// Limit request body size to prevent DoS attacks
	r.Body = http.MaxBytesReader(w, r.Body, maxSize)

	// Decode JSON request body
	var req T
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		// Check if the error is due to body too large
		// MaxBytesReader returns http.MaxBytesError or a generic error with "too large" message
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			return nil, apperrors.New("PAYLOAD_TOO_LARGE",
				"Request body too large", http.StatusRequestEntityTooLarge)
		}

		// Also check error message as fallback
		errMsg := err.Error()
		if strings.Contains(errMsg, "request body too large") ||
			strings.Contains(errMsg, "http: request body too large") ||
			strings.Contains(errMsg, "body too large") {
			return nil, apperrors.New("PAYLOAD_TOO_LARGE",
				"Request body too large", http.StatusRequestEntityTooLarge)
		}

		// Check for unexpected EOF which can happen with very large bodies
		if errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF) {
			return nil, apperrors.New("PAYLOAD_TOO_LARGE",
				"Request body too large", http.StatusRequestEntityTooLarge)
		}

		// Generic JSON decode error
		return nil, apperrors.Wrap(err, "INVALID_JSON",
			"Invalid request body", http.StatusBadRequest)
	}

	// Validate the decoded struct if it implements Validate() error
	// Use type assertion to check for Validator interface
	if validator, ok := any(&req).(interface{ Validate() error }); ok {
		if err := validator.Validate(); err != nil {
			return nil, err
		}
	}

	return &req, nil
}
