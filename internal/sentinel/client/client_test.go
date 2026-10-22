package client_test

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"errors"

	sentinelErrors "dougdomingos.com/aegis/internal/errors"
	"dougdomingos.com/aegis/internal/sentinel/client"
	"dougdomingos.com/aegis/internal/sentinel/identity"

	"dougdomingos.com/aegis/internal/auth"
)

// labKey is the 32-byte shared key used by the client tests.
var labKey = []byte("0123456789abcdef0123456789abcdef")

// testIdentity is the machine identity used by the client tests.
var testIdentity = identity.Identity{IP: "10.0.1.11", MAC: "aa:bb:cc:dd:ee:ff"}

func TestClient_Authenticate_WithValidChallenge_ReturnsSession(t *testing.T) {
	var receivedSignature string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/machines/authenticate/init":
			fmt.Fprint(w, `{"Nonce":"abc123","ExpiresAt":"2026-10-22T12:00:00Z"}`)
		case "/machines/authenticate":
			var payload struct {
				IP        string
				MAC       string
				Signature string
			}

			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Errorf("failed to decode authenticate request: %v", err)
			}

			receivedSignature = payload.Signature
			fmt.Fprint(w, `{"SessionToken":"token-xyz","ExpiresAt":"2026-10-23T12:00:00Z","Machine":{"ID":1,"Name":"lcc1-001","IP":"10.0.1.11","MAC":"aa:bb:cc:dd:ee:ff","Status":"CONNECTED"}}`)
		default:
			t.Errorf("unexpected request path %q", r.URL.Path)
		}
	}))
	defer server.Close()

	cl := newTestClient(t, server.URL, 2*time.Second)

	session, err := cl.Authenticate(t.Context(), testIdentity)
	if err != nil {
		t.Fatalf("client.Authenticate() error = %v, want nil", err)
	}

	if session.Token != "token-xyz" {
		t.Errorf("session.Token = %q, want token-xyz", session.Token)
	}

	expectedExpiry := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	if !session.ExpiresAt.Equal(expectedExpiry) {
		t.Errorf("session.ExpiresAt = %v, want %v", session.ExpiresAt, expectedExpiry)
	}

	if session.MachineName != "lcc1-001" {
		t.Errorf("session.MachineName = %q, want lcc1-001", session.MachineName)
	}

	expectedSignature := hex.EncodeToString(auth.Sign(labKey, auth.CanonicalPayload("10.0.1.11", "abc123")))
	if receivedSignature != expectedSignature {
		t.Errorf("received signature = %q, want %q", receivedSignature, expectedSignature)
	}
}

func TestClient_Authenticate_WithRejectedCredentials_ReturnsAuthenticationRejectedError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/machines/authenticate/init" {
			fmt.Fprint(w, `{"Nonce":"abc123","ExpiresAt":"2026-10-22T12:00:00Z"}`)
			return
		}

		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"error_code":401,"message":"invalid machine credentials"}`)
	}))
	defer server.Close()

	cl := newTestClient(t, server.URL, 2*time.Second)

	_, err := cl.Authenticate(t.Context(), testIdentity)
	if err == nil {
		t.Fatalf("client.Authenticate() error = nil, want error")
	}

	if !errors.Is(err, sentinelErrors.ErrAuthenticationRejected) {
		t.Errorf("client.Authenticate() error = %v, want ErrAuthenticationRejected", err)
	}

	if !strings.Contains(err.Error(), "invalid machine credentials") {
		t.Errorf("client.Authenticate() error = %v, want message invalid machine credentials", err)
	}
}

func TestClient_Authenticate_WithInitFailure_ReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"error_code":500,"message":"internal failure"}`)
	}))
	defer server.Close()

	cl := newTestClient(t, server.URL, 2*time.Second)

	if _, err := cl.Authenticate(t.Context(), testIdentity); err == nil {
		t.Errorf("client.Authenticate() error = nil, want error")
	}
}

func TestClient_Authenticate_WithMalformedResponse_ReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "not json")
	}))
	defer server.Close()

	cl := newTestClient(t, server.URL, 2*time.Second)

	if _, err := cl.Authenticate(t.Context(), testIdentity); err == nil {
		t.Errorf("client.Authenticate() error = nil, want error")
	}
}

func TestClient_Authenticate_WithUnreachableOrchestrator_ReturnsUnreachableError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request to closed server")
	}))
	server.Close()

	cl := newTestClient(t, server.URL, 2*time.Second)

	_, err := cl.Authenticate(t.Context(), testIdentity)
	if err == nil {
		t.Fatalf("client.Authenticate() error = nil, want error")
	}

	if !errors.Is(err, sentinelErrors.ErrOrchestratorUnreachable) {
		t.Errorf("client.Authenticate() error = %v, want ErrOrchestratorUnreachable", err)
	}
}

func TestClient_Authenticate_WithRequestTimeout_ReturnsUnreachableError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		fmt.Fprint(w, `{"Nonce":"abc123","ExpiresAt":"2026-10-22T12:00:00Z"}`)
	}))
	defer server.Close()

	cl := newTestClient(t, server.URL, 20*time.Millisecond)

	_, err := cl.Authenticate(t.Context(), testIdentity)
	if err == nil {
		t.Fatalf("client.Authenticate() error = nil, want error")
	}

	if !errors.Is(err, sentinelErrors.ErrOrchestratorUnreachable) {
		t.Errorf("client.Authenticate() error = %v, want ErrOrchestratorUnreachable", err)
	}
}

func TestClient_Authenticate_WithTrailingSlashBaseURL_RequestsExactPath(t *testing.T) {
	var receivedPath string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if receivedPath == "" {
			receivedPath = r.URL.Path
		}
	}))
	defer server.Close()

	cl := newTestClient(t, server.URL+"/", 2*time.Second)

	if _, err := cl.Authenticate(t.Context(), testIdentity); err == nil {
		t.Errorf("client.Authenticate() error = nil, want error")
	}

	if receivedPath != "/machines/authenticate/init" {
		t.Errorf("received path = %q, want /machines/authenticate/init", receivedPath)
	}
}

// newTestClient builds a client pointing at the provided server URL, using a
// fixed lab key and a short timeout.
func newTestClient(t *testing.T, serverURL string, timeout time.Duration) *client.Client {
	t.Helper()

	return client.New(mustParseURL(t, serverURL), labKey, timeout, nil)
}

// mustParseURL parses the provided URL, failing the test on a malformed value.
func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()

	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("failed to parse test URL %q: %v", raw, err)
	}

	return parsed
}
