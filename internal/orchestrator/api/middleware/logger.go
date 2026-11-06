package middleware

import (
	"log"
	"net/http"
)

// responseWriterWrapper intercepts the ResponseWriter to capture its HTTP
// status code.
type responseWriterWrapper struct {
	http.ResponseWriter
	statusCode int
}

// newResponseWriterWrapper creates a new responseWriterWrapper instance.
func newResponseWriterWrapper(w http.ResponseWriter) *responseWriterWrapper {
	return &responseWriterWrapper{
		ResponseWriter: w,
		statusCode:     http.StatusOK,
	}
}

// WriteHeader records the HTTP status code and calls the underlying
// ResponseWriter.WriteHeader.
func (rw *responseWriterWrapper) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// Logger returns an HTTP middleware that logs the request method, URI, and
// resulting status code.
func Logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rw := newResponseWriterWrapper(w)
		next.ServeHTTP(rw, r)

		log.Printf("[%s] %s - %d", r.Method, r.RequestURI, rw.statusCode)
	})
}
