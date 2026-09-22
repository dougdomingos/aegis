package middleware_test

import (
	"bytes"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"dougdomingos.com/aegis/internal/api/middleware"
)

// ============================================================================
// Logger
// ============================================================================

func TestMiddleware_Logger_WhenResponseWritten_PropagatesBodyAndStatus(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("created"))
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/rules", nil)

	middleware.Logger(next).ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected status %d, got %d", http.StatusCreated, w.Code)
	}

	if w.Body.String() != "created" {
		t.Errorf("expected body %q, got %q", "created", w.Body.String())
	}
}

func TestMiddleware_Logger_WhenNoStatusWritten_DefaultsToOK(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/groups", nil)

	middleware.Logger(next).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected default status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestMiddleware_Logger_WhenRequestHandled_LogsMethodURIAndStatus(t *testing.T) {
	var logBuf bytes.Buffer
	log.SetOutput(&logBuf)
	defer log.SetOutput(nil)

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/rules/42", nil)

	middleware.Logger(next).ServeHTTP(w, req)

	logged := logBuf.String()

	if !strings.Contains(logged, http.MethodDelete) {
		t.Errorf("expected log to contain method %q, got %q", http.MethodDelete, logged)
	}

	if !strings.Contains(logged, "/rules/42") {
		t.Errorf("expected log to contain URI %q, got %q", "/rules/42", logged)
	}

	if !strings.Contains(logged, "404") {
		t.Errorf("expected log to contain status %d, got %q", http.StatusNotFound, logged)
	}
}
