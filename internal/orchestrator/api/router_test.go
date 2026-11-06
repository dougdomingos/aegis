package api_test

import (
	"bytes"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"dougdomingos.com/aegis/internal/common/auth"
	"dougdomingos.com/aegis/internal/common/schemas"
	"dougdomingos.com/aegis/internal/orchestrator/api"
	"dougdomingos.com/aegis/internal/orchestrator/config"
	"dougdomingos.com/aegis/internal/orchestrator/infra/migrations"
	_ "modernc.org/sqlite"
)

// labKey is the 32-byte shared lab key used across the router tests.
var labKey = []byte("0123456789abcdef0123456789abcdef")

func TestNewRouter_WithValidConfig_RegistersMachineRoutes(t *testing.T) {
	db, err := sql.Open("sqlite", "file::memory:?mode=memory&cache=shared&_pragma=foreign_keys(ON)")
	if err != nil {
		t.Fatalf("failed to open in-memory database: %v", err)
	}

	t.Cleanup(func() {
		db.Close()
	})

	if err := migrations.ApplyMigrations(db); err != nil {
		t.Fatalf("failed to apply migrations: %v", err)
	}

	cfg := &config.Config{
		LabKey:         labKey,
		SessionSecret:  "sign-secret",
		SessionTTL:     24 * time.Hour,
		StalenessGrace: 90 * time.Second,
		Machines: []config.Machine{
			{IP: "10.0.1.11", Name: "lcc1-011"},
		},
	}

	router, err := api.NewRouter(db, cfg)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/machines", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status code %d, got %d", http.StatusOK, rec.Code)
	}

	var machines []schemas.MachineOutputSchema
	if err := json.NewDecoder(rec.Body).Decode(&machines); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}

	if len(machines) != 1 {
		t.Fatalf("expected 1 machine, got %d", len(machines))
	}

	if machines[0].Name != "lcc1-011" {
		t.Errorf("expected machine name %q, got %q", "lcc1-011", machines[0].Name)
	}
}

func TestNewRouter_WithValidConfig_AuthenticatesConfiguredMachine(t *testing.T) {
	db, err := sql.Open("sqlite", "file::memory:?mode=memory&cache=shared&_pragma=foreign_keys(ON)")
	if err != nil {
		t.Fatalf("failed to open in-memory database: %v", err)
	}

	t.Cleanup(func() {
		db.Close()
	})

	if err := migrations.ApplyMigrations(db); err != nil {
		t.Fatalf("failed to apply migrations: %v", err)
	}

	cfg := &config.Config{
		LabKey:         labKey,
		SessionSecret:  "sign-secret",
		SessionTTL:     24 * time.Hour,
		StalenessGrace: 90 * time.Second,
		Machines: []config.Machine{
			{IP: "10.0.1.11", Name: "lcc1-011"},
		},
	}

	router, err := api.NewRouter(db, cfg)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	nonce := initiateChallenge(t, router, "10.0.1.11")

	rec := httptest.NewRecorder()
	body, _ := json.Marshal(map[string]any{"ip": "10.0.1.11", "signature": signChallenge(t, "10.0.1.11", nonce)})
	req := httptest.NewRequest(http.MethodPost, "/machines/authenticate", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status code %d, got %d", http.StatusOK, rec.Code)
	}

	var result schemas.AuthenticateOutputSchema
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}

	if result.SessionToken == "" {
		t.Error("expected a session token to be issued")
	}

	if result.Machine.Status != "CONNECTED" {
		t.Errorf("expected status %q, got %q", "CONNECTED", result.Machine.Status)
	}
}

// initiateChallenge starts an authentication challenge for the provided IP and
// returns the issued nonce.
func initiateChallenge(t *testing.T, router http.Handler, ip string) string {
	t.Helper()

	body, _ := json.Marshal(map[string]any{"ip": ip})
	req := httptest.NewRequest(http.MethodPost, "/machines/authenticate/init", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status code %d, got %d", http.StatusOK, rec.Code)
	}

	var out schemas.InitiateOutputSchema
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}

	return out.Nonce
}

// signChallenge returns the hex-encoded signature over the canonical payload
// for the provided IP and nonce using the shared lab key.
func signChallenge(t *testing.T, ip, nonce string) string {
	t.Helper()

	signature := auth.Sign(labKey, auth.CanonicalPayload(ip, nonce))

	return hex.EncodeToString(signature)
}
