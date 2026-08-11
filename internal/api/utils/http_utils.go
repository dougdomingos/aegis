package utils

import (
	"encoding/json"
	"errors"
	"net/http"
)

// ErrorResponse is used to write JSON responses for API errors, including the
// HTTP status code and error message.
type ErrorResponse struct {
	ErrorCode int    `json:"error_code"`
	Message   string `json:"message"`
}

// DecodePayload decodes the JSON request body into dst. If decoding fails, it
// writes an HTTP 400 Bad Request error and returns false.
func DecodePayload(w http.ResponseWriter, r *http.Request, dst any) bool {
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		EmitError(w, "invalid request body", http.StatusBadRequest)
		return false
	}

	return true
}

// EncodeToJSON writes a JSON response with the given HTTP status code and
// body.
func EncodeToJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// EmitError writes a JSON response for errors, containing the given HTTP
// status code and the message.
func EmitError(w http.ResponseWriter, message string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(ErrorResponse{
		ErrorCode: status,
		Message:   message,
	})
}

// MapByError maps a given error to an HTTP status code based on errMap and
// writes the error response. If no matching error is found, it defaults to an
// HTTP 500 Internal Server MapByError.
func MapByError(w http.ResponseWriter, err error, errMap map[error]int) {
	status := http.StatusInternalServerError
	message := "internal server error"

	for sentinel, code := range errMap {
		if errors.Is(err, sentinel) {
			status = code
			message = err.Error()
			break
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(ErrorResponse{
		ErrorCode: status,
		Message:   message,
	})
}
