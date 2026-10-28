package handler_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// buildTestRequest creates a response recorder and an HTTP request for the
// provided method, endpoint, and JSON payload.
func buildTestRequest(method, endpoint string, payload any) (*httptest.ResponseRecorder, *http.Request) {
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(method, endpoint, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	return httptest.NewRecorder(), req
}

// performRequest builds a test request for the provided payload and serves it
// against the given router, returning the recorded response.
func performRequest(t *testing.T, router http.Handler, method, endpoint string, payload any) *httptest.ResponseRecorder {
	t.Helper()

	rec, req := buildTestRequest(method, endpoint, payload)
	router.ServeHTTP(rec, req)

	return rec
}

// assertStatusCode fails the test when the response does not carry the
// expected HTTP status code.
func assertStatusCode(t *testing.T, rec *httptest.ResponseRecorder, want int) {
	t.Helper()

	if rec.Code != want {
		t.Errorf("expected status code %d, got %d", want, rec.Code)
	}
}

// performBearerRequest sends a POST to /machines/heartbeat with the provided
// bearer token and returns the recorded response.
func performBearerRequest(t *testing.T, router http.Handler, token string) *httptest.ResponseRecorder {
	t.Helper()

	rec, req := buildTestRequest(http.MethodPost, "/machines/heartbeat", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(rec, req)

	return rec
}
