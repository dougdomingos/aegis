package utils

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ============================================================================
// DecodePayload
// ============================================================================

func TestHttpUtils_DecodePayload_WhenValidJSON_ReturnsTrue(t *testing.T) {
	type User struct {
		Name  string `json:"name"`
		Email string `json:"email"`
		Age   int    `json:"age"`
	}

	tests := []struct {
		name           string
		payload        string
		expectedResult User
	}{
		{
			name:    "Simple user payload",
			payload: `{"name":"John","email":"john@example.com","age":30}`,
			expectedResult: User{
				Name:  "John",
				Email: "john@example.com",
				Age:   30,
			},
		},
		{
			name:    "User with special characters",
			payload: `{"name":"José García","email":"jose@example.com","age":25}`,
			expectedResult: User{
				Name:  "José García",
				Email: "jose@example.com",
				Age:   25,
			},
		},
		{
			name:    "Minimal user payload",
			payload: `{"name":"","email":"","age":0}`,
			expectedResult: User{
				Name:  "",
				Email: "",
				Age:   0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := newJSONRequest(tt.payload)
			w := httptest.NewRecorder()

			var result User
			success := DecodePayload(w, req, &result)

			if !success {
				t.Error("Expected DecodePayload to return true, got false")
			}

			if result != tt.expectedResult {
				t.Errorf("Expected %+v, got %+v", tt.expectedResult, result)
			}

			if w.Code != 200 {
				t.Errorf("Expected no response to be written on success, got code %d", w.Code)
			}
		})
	}
}

func TestHttpUtils_DecodePayload_WhenInvalidJSON_ReturnsFalseAndBadRequest(t *testing.T) {
	tests := []struct {
		name    string
		payload string
	}{
		{name: "Missing closing brace", payload: `{"name":"John"`},
		{name: "Unquoted string value", payload: `{"name": John}`},
		{name: "Empty body", payload: ``},
		{name: "Plain text", payload: `this is not json`},
		{name: "Trailing comma", payload: `{"name":"John",}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := newJSONRequest(tt.payload)
			w := httptest.NewRecorder()

			var result any
			success := DecodePayload(w, req, &result)

			if success {
				t.Error("Expected DecodePayload to return false for invalid JSON, got true")
			}

			errResp := decodeErrorResponse(t, w)

			if w.Code != http.StatusBadRequest {
				t.Errorf("Expected status code %d, got %d", http.StatusBadRequest, w.Code)
			}

			if errResp.ErrorCode != http.StatusBadRequest {
				t.Errorf("Expected error code %d, got %d", http.StatusBadRequest, errResp.ErrorCode)
			}

			if !strings.Contains(errResp.Message, "invalid request body") {
				t.Errorf("Expected error message to contain 'invalid request body', got '%s'", errResp.Message)
			}
		})
	}
}

func TestHttpUtils_DecodePayload_WhenComplexTypes_DecodesCorrectly(t *testing.T) {
	type Data struct {
		Flag  bool     `json:"flag"`
		Count int      `json:"count"`
		Price float64  `json:"price"`
		Tags  []string `json:"tags"`
	}

	req := newJSONRequest(`{"flag":true,"count":42,"price":19.99,"tags":["a","b","c"]}`)
	w := httptest.NewRecorder()

	var result Data
	success := DecodePayload(w, req, &result)

	if !success {
		t.Error("Expected DecodePayload to return true, got false")
	}

	if result.Flag != true {
		t.Errorf("Expected Flag=true, got %v", result.Flag)
	}

	if result.Count != 42 {
		t.Errorf("Expected Count=42, got %d", result.Count)
	}

	if result.Price != 19.99 {
		t.Errorf("Expected Price=19.99, got %f", result.Price)
	}

	if len(result.Tags) != 3 || result.Tags[0] != "a" {
		t.Errorf("Expected Tags=[a,b,c], got %v", result.Tags)
	}
}

func TestHttpUtils_DecodePayload_WhenNestedJSON_DecodesCorrectly(t *testing.T) {
	type Address struct {
		Street string `json:"street"`
		City   string `json:"city"`
	}

	type User struct {
		Name    string  `json:"name"`
		Address Address `json:"address"`
	}

	req := newJSONRequest(`{"name":"John","address":{"street":"Main St","city":"New York"}}`)
	w := httptest.NewRecorder()

	var result User
	success := DecodePayload(w, req, &result)

	if !success {
		t.Error("Expected DecodePayload to return true, got false")
	}

	if result.Name != "John" {
		t.Errorf("Expected Name='John', got '%s'", result.Name)
	}

	if result.Address.City != "New York" {
		t.Errorf("Expected City='New York', got '%s'", result.Address.City)
	}
}

// ============================================================================
// EncodeToJSON
// ============================================================================

func TestHttpUtils_EncodeToJSON_WhenCalled_WritesStatusAndJSONBody(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   interface{}
	}{
		{
			name:   "Map response",
			status: http.StatusOK,
			body:   map[string]interface{}{"id": 1, "name": "Test"},
		},
		{
			name:   "Struct response",
			status: http.StatusCreated,
			body:   struct{ ID int }{ID: 123},
		},
		{
			name:   "Slice response",
			status: http.StatusOK,
			body:   []int{1, 2, 3},
		},
		{
			name:   "String response",
			status: http.StatusOK,
			body:   "simple string",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()

			EncodeToJSON(w, tt.status, tt.body)

			if w.Code != tt.status {
				t.Errorf("Expected status %d, got %d", tt.status, w.Code)
			}

			var result interface{}
			if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
				t.Errorf("Response body is not valid JSON: %v", err)
			}
		})
	}
}

func TestHttpUtils_EncodeToJSON_WhenCalled_SetsContentTypeHeader(t *testing.T) {
	w := httptest.NewRecorder()

	EncodeToJSON(w, http.StatusOK, map[string]string{"key": "value"})

	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Expected Content-Type 'application/json', got '%s'", ct)
	}
}

// ============================================================================
// EmitError
// ============================================================================

func TestHttpUtils_EmitError_WhenCalled_WritesErrorResponse(t *testing.T) {
	tests := []struct {
		name    string
		message string
		status  int
	}{
		{name: "Bad Request", message: "Invalid input", status: http.StatusBadRequest},
		{name: "Not Found", message: "Resource not found", status: http.StatusNotFound},
		{name: "Internal Server Error", message: "Something went wrong", status: http.StatusInternalServerError},
		{name: "Unauthorized", message: "Authentication required", status: http.StatusUnauthorized},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()

			EmitError(w, tt.message, tt.status)

			assertErrorResponse(t, w, tt.status, tt.message)
		})
	}
}

func TestHttpUtils_EmitError_WhenCalled_SetsContentTypeHeader(t *testing.T) {
	w := httptest.NewRecorder()

	EmitError(w, "test error", http.StatusBadRequest)

	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Expected Content-Type 'application/json', got '%s'", ct)
	}
}

// ============================================================================
// MapByError
// ============================================================================

func TestHttpUtils_MapByError_WhenErrorMatches_WritesExpectedStatusAndMessage(t *testing.T) {
	var ErrNotFound = errors.New("not found")
	var ErrInvalidInput = errors.New("invalid input")
	var ErrUnauthorized = errors.New("unauthorized")

	errMap := map[error]int{
		ErrNotFound:     http.StatusNotFound,
		ErrInvalidInput: http.StatusBadRequest,
		ErrUnauthorized: http.StatusUnauthorized,
	}

	tests := []struct {
		name           string
		err            error
		expectedStatus int
		expectedMsg    string
	}{
		{
			name:           "NotFound sentinel",
			err:            ErrNotFound,
			expectedStatus: http.StatusNotFound,
			expectedMsg:    "not found",
		},
		{
			name:           "InvalidInput sentinel",
			err:            ErrInvalidInput,
			expectedStatus: http.StatusBadRequest,
			expectedMsg:    "invalid input",
		},
		{
			name:           "Unauthorized sentinel",
			err:            ErrUnauthorized,
			expectedStatus: http.StatusUnauthorized,
			expectedMsg:    "unauthorized",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()

			MapByError(w, tt.err, errMap)

			assertErrorResponse(t, w, tt.expectedStatus, tt.expectedMsg)
		})
	}
}

func TestHttpUtils_MapByError_WhenErrorIsWrapped_MatchesSentinel(t *testing.T) {
	var sentinelErr = errors.New("sentinel")

	chainedErr := errors.Join(errors.New("additional context"), sentinelErr)

	errMap := map[error]int{
		sentinelErr: http.StatusNotFound,
	}

	w := httptest.NewRecorder()
	MapByError(w, chainedErr, errMap)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status %d, got %d", http.StatusNotFound, w.Code)
	}

	errResp := decodeErrorResponse(t, w)
	if errResp.ErrorCode != http.StatusNotFound {
		t.Errorf("Expected ErrorCode %d, got %d", http.StatusNotFound, errResp.ErrorCode)
	}
}

func TestHttpUtils_MapByError_WhenErrorNotMapped_WritesInternalServerError(t *testing.T) {
	errMap := map[error]int{
		errors.New("mapped error"): http.StatusBadRequest,
	}

	w := httptest.NewRecorder()
	MapByError(w, errors.New("unknown error"), errMap)

	assertErrorResponse(t, w, http.StatusInternalServerError, "internal server error")
}

func TestHttpUtils_MapByError_WhenMapIsEmpty_WritesInternalServerError(t *testing.T) {
	w := httptest.NewRecorder()
	MapByError(w, errors.New("some error"), map[error]int{})

	assertErrorResponse(t, w, http.StatusInternalServerError, "internal server error")
}

// ============================================================================
// Test Helpers
// ============================================================================

func newJSONRequest(payload string) *http.Request {
	return &http.Request{
		Body: io.NopCloser(strings.NewReader(payload)),
	}
}

func decodeErrorResponse(t *testing.T, w *httptest.ResponseRecorder) ErrorResponse {
	t.Helper()

	var errResp ErrorResponse
	if err := json.NewDecoder(w.Body).Decode(&errResp); err != nil {
		t.Fatalf("Failed to decode error response: %v", err)
	}

	return errResp
}

func assertErrorResponse(t *testing.T, w *httptest.ResponseRecorder, expectedStatus int, expectedMsg string) {
	t.Helper()

	if w.Code != expectedStatus {
		t.Errorf("Expected status %d, got %d", expectedStatus, w.Code)
	}

	errResp := decodeErrorResponse(t, w)

	if errResp.ErrorCode != expectedStatus {
		t.Errorf("Expected ErrorCode %d, got %d", expectedStatus, errResp.ErrorCode)
	}

	if errResp.Message != expectedMsg {
		t.Errorf("Expected message '%s', got '%s'", expectedMsg, errResp.Message)
	}
}
