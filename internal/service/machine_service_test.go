package service_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"dougdomingos.com/aegis/internal/auth"
	"dougdomingos.com/aegis/internal/domain"
	machineErrors "dougdomingos.com/aegis/internal/errors"
	"dougdomingos.com/aegis/internal/schemas"
	"dougdomingos.com/aegis/internal/service"
)

// labSecretKey is the 32-byte shared lab key used across the service tests.
const labSecretKey = "0123456789abcdef0123456789abcdef"

// ============================================================================
// SyncMachines
// ============================================================================

func TestMachineService_SyncMachines_WithNewSources_InsertsMachines(t *testing.T) {
	ctx, store, svc, _ := arrangeMachineServiceTest(t)
	machines := []domain.Machine{
		{IP: "10.0.1.11", Name: "lcc1-011"},
		{IP: "10.0.1.12", Name: "lcc1-012"},
	}

	if err := svc.SyncMachines(ctx, machines); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	listed, err := store.List(ctx)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if len(listed) != 2 {
		t.Errorf("expected 2 machines, got %d", len(listed))
	}
}

func TestMachineService_SyncMachines_AfterSourceRemoved_PrunesMachine(t *testing.T) {
	ctx, store, svc, _ := arrangeMachineServiceTest(t)

	if err := svc.SyncMachines(ctx, []domain.Machine{
		{IP: "10.0.1.11", Name: "lcc1-011"},
		{IP: "10.0.1.12", Name: "lcc1-012"},
	}); err != nil {
		t.Fatalf("failed to seed machines: %v", err)
	}

	if err := svc.SyncMachines(ctx, []domain.Machine{
		{IP: "10.0.1.11", Name: "lcc1-011"},
	}); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	removed, err := store.GetByIP(ctx, "10.0.1.12")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if removed != nil {
		t.Errorf("expected pruned machine to be nil, got %+v", removed)
	}
}

// ============================================================================
// ListMachines
// ============================================================================

func TestMachineService_ListMachines_WithMachines_ReturnsSchemas(t *testing.T) {
	ctx, store, svc, _ := arrangeMachineServiceTest(t)

	if _, err := store.Upsert(ctx, domain.Machine{IP: "10.0.1.11", Name: "lcc1-011"}); err != nil {
		t.Fatalf("failed to seed machine: %v", err)
	}

	listed, err := svc.ListMachines(ctx)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if len(listed) != 1 {
		t.Fatalf("expected 1 machine, got %d", len(listed))
	}

	if listed[0].Name != "lcc1-011" {
		t.Errorf("expected name %q, got %q", "lcc1-011", listed[0].Name)
	}

	if listed[0].Status != string(domain.DisconnectedStatus) {
		t.Errorf("expected status %q, got %q", domain.DisconnectedStatus, listed[0].Status)
	}
}

func TestMachineService_ListMachines_WithNoMachines_ReturnsEmptyList(t *testing.T) {
	_, _, svc, _ := arrangeMachineServiceTest(t)

	listed, err := svc.ListMachines(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if len(listed) != 0 {
		t.Errorf("expected 0 machines, got %d", len(listed))
	}
}

// ============================================================================
// InitiateAuthentication
// ============================================================================

func TestMachineService_InitiateAuthentication_WithValidIP_ReturnsNonce(t *testing.T) {
	ctx, _, svc, clock := arrangeMachineServiceTest(t)

	result, err := svc.InitiateAuthentication(ctx, schemas.InitiateAuthenticationSchema{IP: "10.0.1.11"})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if result == nil {
		t.Fatal("expected result, got nil")
	}

	if len(result.Nonce) != 32 {
		t.Errorf("expected 32-character nonce, got %d characters", len(result.Nonce))
	}

	expectedExpiry := clock.Now().Add(60 * time.Second)
	if !result.ExpiresAt.Equal(expectedExpiry) {
		t.Errorf("expected expiry %v, got %v", expectedExpiry, result.ExpiresAt)
	}
}

func TestMachineService_InitiateAuthentication_WithUnknownIP_ReturnsNonce(t *testing.T) {
	ctx, _, svc, _ := arrangeMachineServiceTest(t)

	result, err := svc.InitiateAuthentication(ctx, schemas.InitiateAuthenticationSchema{IP: "10.0.1.99"})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if result == nil || result.Nonce == "" {
		t.Error("expected a nonce to be issued even for an unregistered IP")
	}
}

func TestMachineService_InitiateAuthentication_WithEmptyIP_ReturnsIPRequired(t *testing.T) {
	_, _, svc, _ := arrangeMachineServiceTest(t)

	result, err := svc.InitiateAuthentication(context.Background(), schemas.InitiateAuthenticationSchema{})

	if !errors.Is(err, machineErrors.ErrMachineIPRequired) {
		t.Errorf("expected error %q, got %q", machineErrors.ErrMachineIPRequired, err)
	}

	if result != nil {
		t.Errorf("expected nil result, got %+v", result)
	}
}

// ============================================================================
// Authenticate
// ============================================================================

func TestMachineService_Authenticate_WithValidChallenge_ReturnsSession(t *testing.T) {
	ctx, store, svc, clock := arrangeMachineServiceTest(t)

	if err := svc.SyncMachines(ctx, []domain.Machine{
		{IP: "10.0.1.11", Name: "lcc1-011"},
	}); err != nil {
		t.Fatalf("failed to seed machines: %v", err)
	}

	challenge := initiateChallenge(t, svc, "10.0.1.11")

	result, err := svc.Authenticate(ctx, schemas.AuthenticateMachineSchema{
		IP:        "10.0.1.11",
		Signature: signChallenge(t, challenge),
	})

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if result == nil {
		t.Fatal("expected result, got nil")
	}

	if result.Machine.Status != string(domain.ConnectedStatus) {
		t.Errorf("expected status %q, got %q", domain.ConnectedStatus, result.Machine.Status)
	}

	if result.Machine.LastSeenAt == nil {
		t.Error("expected last_seen_at to be populated")
	}

	if result.SessionToken == "" {
		t.Error("expected a session token to be issued")
	}

	expectedExpiry := clock.Now().Add(24 * time.Hour)
	if result.ExpiresAt.Before(expectedExpiry.Add(-time.Minute)) || result.ExpiresAt.After(expectedExpiry.Add(time.Minute)) {
		t.Errorf("expected expiry near %v, got %v", expectedExpiry, result.ExpiresAt)
	}

	if !validSessionToken(result.SessionToken, "sign-secret", 1) {
		t.Errorf("expected a well-formed signed session token, got %q", result.SessionToken)
	}

	touched, err := store.GetByIP(ctx, "10.0.1.11")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if touched.Status != domain.ConnectedStatus {
		t.Errorf("expected stored status %q, got %q", domain.ConnectedStatus, touched.Status)
	}
}

func TestMachineService_Authenticate_WithUnknownIP_ReturnsInvalidCredentials(t *testing.T) {
	ctx, _, svc, _ := arrangeMachineServiceTest(t)

	if err := svc.SyncMachines(ctx, []domain.Machine{
		{IP: "10.0.1.11", Name: "lcc1-011"},
	}); err != nil {
		t.Fatalf("failed to seed machines: %v", err)
	}

	result, err := svc.Authenticate(ctx, schemas.AuthenticateMachineSchema{
		IP:        "10.0.1.99",
		Signature: signChallengeFor(t, "10.0.1.99", "any-nonce"),
	})

	if !errors.Is(err, machineErrors.ErrInvalidCredentials) {
		t.Errorf("expected error %q, got %q", machineErrors.ErrInvalidCredentials, err)
	}

	if result != nil {
		t.Errorf("expected nil result, got %+v", result)
	}
}

func TestMachineService_Authenticate_WithUnknownIPAndValidSignature_ReturnsInvalidCredentials(t *testing.T) {
	ctx, _, svc, _ := arrangeMachineServiceTest(t)

	if err := svc.SyncMachines(ctx, []domain.Machine{
		{IP: "10.0.1.11", Name: "lcc1-011"},
	}); err != nil {
		t.Fatalf("failed to seed machines: %v", err)
	}

	challenge := initiateChallenge(t, svc, "10.0.1.11")

	result, err := svc.Authenticate(ctx, schemas.AuthenticateMachineSchema{
		IP:        "10.0.1.99",
		Signature: signChallenge(t, challenge),
	})

	if !errors.Is(err, machineErrors.ErrInvalidCredentials) {
		t.Errorf("expected error %q, got %q", machineErrors.ErrInvalidCredentials, err)
	}

	if result != nil {
		t.Errorf("expected nil result, got %+v", result)
	}
}

func TestMachineService_Authenticate_WithWrongSignature_ReturnsInvalidCredentials(t *testing.T) {
	ctx, _, svc, _ := arrangeMachineServiceTest(t)

	if err := svc.SyncMachines(ctx, []domain.Machine{
		{IP: "10.0.1.11", Name: "lcc1-011"},
	}); err != nil {
		t.Fatalf("failed to seed machines: %v", err)
	}

	initiateChallenge(t, svc, "10.0.1.11")

	result, err := svc.Authenticate(ctx, schemas.AuthenticateMachineSchema{
		IP:        "10.0.1.11",
		Signature: signChallengeWithKey(t, []byte("ffffffffffffffffffffffffffffffff"), "10.0.1.11", "any-nonce"),
	})

	if !errors.Is(err, machineErrors.ErrInvalidCredentials) {
		t.Errorf("expected error %q, got %q", machineErrors.ErrInvalidCredentials, err)
	}

	if result != nil {
		t.Errorf("expected nil result, got %+v", result)
	}
}

func TestMachineService_Authenticate_WithSignatureForDifferentIP_ReturnsInvalidCredentials(t *testing.T) {
	ctx, _, svc, _ := arrangeMachineServiceTest(t)

	if err := svc.SyncMachines(ctx, []domain.Machine{
		{IP: "10.0.1.11", Name: "lcc1-011"},
	}); err != nil {
		t.Fatalf("failed to seed machines: %v", err)
	}

	challenge := initiateChallenge(t, svc, "10.0.1.11")

	mismatched := signChallengeFor(t, "10.0.1.12", challenge.Nonce)

	result, err := svc.Authenticate(ctx, schemas.AuthenticateMachineSchema{
		IP:        "10.0.1.11",
		Signature: mismatched,
	})

	if !errors.Is(err, machineErrors.ErrInvalidCredentials) {
		t.Errorf("expected error %q, got %q", machineErrors.ErrInvalidCredentials, err)
	}

	if result != nil {
		t.Errorf("expected nil result, got %+v", result)
	}
}

func TestMachineService_Authenticate_WithReplayedNonce_ReturnsInvalidCredentials(t *testing.T) {
	ctx, _, svc, _ := arrangeMachineServiceTest(t)

	if err := svc.SyncMachines(ctx, []domain.Machine{
		{IP: "10.0.1.11", Name: "lcc1-011"},
	}); err != nil {
		t.Fatalf("failed to seed machines: %v", err)
	}

	challenge := initiateChallenge(t, svc, "10.0.1.11")
	signature := signChallenge(t, challenge)

	if _, err := svc.Authenticate(ctx, schemas.AuthenticateMachineSchema{
		IP:        "10.0.1.11",
		Signature: signature,
	}); err != nil {
		t.Fatalf("expected the first authentication to succeed, got: %v", err)
	}

	result, err := svc.Authenticate(ctx, schemas.AuthenticateMachineSchema{
		IP:        "10.0.1.11",
		Signature: signature,
	})

	if !errors.Is(err, machineErrors.ErrInvalidCredentials) {
		t.Errorf("expected error %q, got %q", machineErrors.ErrInvalidCredentials, err)
	}

	if result != nil {
		t.Errorf("expected nil result, got %+v", result)
	}
}

func TestMachineService_Authenticate_WithExpiredNonce_ReturnsInvalidCredentials(t *testing.T) {
	ctx, _, svc, clock := arrangeMachineServiceTest(t)

	if err := svc.SyncMachines(ctx, []domain.Machine{
		{IP: "10.0.1.11", Name: "lcc1-011"},
	}); err != nil {
		t.Fatalf("failed to seed machines: %v", err)
	}

	challenge := initiateChallenge(t, svc, "10.0.1.11")
	clock.Advance(2 * time.Minute)

	result, err := svc.Authenticate(ctx, schemas.AuthenticateMachineSchema{
		IP:        "10.0.1.11",
		Signature: signChallenge(t, challenge),
	})

	if !errors.Is(err, machineErrors.ErrInvalidCredentials) {
		t.Errorf("expected error %q, got %q", machineErrors.ErrInvalidCredentials, err)
	}

	if result != nil {
		t.Errorf("expected nil result, got %+v", result)
	}
}

func TestMachineService_Authenticate_WithEmptySignature_ReturnsInvalidCredentials(t *testing.T) {
	ctx, _, svc, _ := arrangeMachineServiceTest(t)

	if err := svc.SyncMachines(ctx, []domain.Machine{
		{IP: "10.0.1.11", Name: "lcc1-011"},
	}); err != nil {
		t.Fatalf("failed to seed machines: %v", err)
	}

	initiateChallenge(t, svc, "10.0.1.11")

	result, err := svc.Authenticate(ctx, schemas.AuthenticateMachineSchema{
		IP:        "10.0.1.11",
		Signature: "",
	})

	if !errors.Is(err, machineErrors.ErrInvalidCredentials) {
		t.Errorf("expected error %q, got %q", machineErrors.ErrInvalidCredentials, err)
	}

	if result != nil {
		t.Errorf("expected nil result, got %+v", result)
	}
}

func TestMachineService_Authenticate_WithMalformedSignature_ReturnsInvalidCredentials(t *testing.T) {
	ctx, _, svc, _ := arrangeMachineServiceTest(t)

	if err := svc.SyncMachines(ctx, []domain.Machine{
		{IP: "10.0.1.11", Name: "lcc1-011"},
	}); err != nil {
		t.Fatalf("failed to seed machines: %v", err)
	}

	initiateChallenge(t, svc, "10.0.1.11")

	result, err := svc.Authenticate(ctx, schemas.AuthenticateMachineSchema{
		IP:        "10.0.1.11",
		Signature: "not-hex",
	})

	if !errors.Is(err, machineErrors.ErrInvalidCredentials) {
		t.Errorf("expected error %q, got %q", machineErrors.ErrInvalidCredentials, err)
	}

	if result != nil {
		t.Errorf("expected nil result, got %+v", result)
	}
}

func TestMachineService_Authenticate_WithEmptyIP_ReturnsIPRequired(t *testing.T) {
	_, _, svc, _ := arrangeMachineServiceTest(t)

	result, err := svc.Authenticate(context.Background(), schemas.AuthenticateMachineSchema{
		Signature: signChallengeFor(t, "10.0.1.11", "any-nonce"),
	})

	if !errors.Is(err, machineErrors.ErrMachineIPRequired) {
		t.Errorf("expected error %q, got %q", machineErrors.ErrMachineIPRequired, err)
	}

	if result != nil {
		t.Errorf("expected nil result, got %+v", result)
	}
}

func TestMachineService_Authenticate_WhenStoreFails_ReturnsError(t *testing.T) {
	ctx, store, svc, _ := arrangeMachineServiceTest(t)
	expectedErr := errors.New("failed to query database for machines")

	if err := svc.SyncMachines(ctx, []domain.Machine{
		{IP: "10.0.1.11", Name: "lcc1-011"},
	}); err != nil {
		t.Fatalf("failed to seed machines: %v", err)
	}

	store.StoreErr = expectedErr

	challenge := initiateChallenge(t, svc, "10.0.1.11")

	result, err := svc.Authenticate(ctx, schemas.AuthenticateMachineSchema{
		IP:        "10.0.1.11",
		Signature: signChallenge(t, challenge),
	})

	if !errors.Is(err, expectedErr) {
		t.Errorf("expected store error %q, got %q", expectedErr, err)
	}

	if result != nil {
		t.Errorf("expected nil result, got %+v", result)
	}
}

// ============================================================================
// Helpers
// ============================================================================

// challenge holds the pending nonce issued for a machine IP.
type challenge struct {
	IP    string
	Nonce string
}

// arrangeMachineServiceTest initializes a new service instance with a mocked
// store provider and a controllable clock.
func arrangeMachineServiceTest(t *testing.T) (context.Context, *MockMachineStore, *service.MachineService, *testClock) {
	t.Helper()

	mockStore := NewMockMachineStore()
	clock := newTestClock()
	ctx := context.Background()
	svc := service.NewMachineService(mockStore, []byte(labSecretKey), "sign-secret", 24*time.Hour, clock.Now)

	return ctx, mockStore, svc, clock
}

// initiateChallenge starts an authentication challenge for the provided IP and
// returns the issued challenge.
func initiateChallenge(t *testing.T, svc *service.MachineService, ip string) challenge {
	t.Helper()

	result, err := svc.InitiateAuthentication(context.Background(), schemas.InitiateAuthenticationSchema{IP: ip})
	if err != nil {
		t.Fatalf("failed to initiate challenge: %v", err)
	}

	return challenge{IP: ip, Nonce: result.Nonce}
}

// signChallenge returns the hex-encoded signature of the provided challenge
// using the shared lab key.
func signChallenge(t *testing.T, challenge challenge) string {
	t.Helper()

	signature := auth.Sign([]byte(labSecretKey), auth.CanonicalPayload(challenge.IP, challenge.Nonce))

	return hex.EncodeToString(signature)
}

// signChallengeFor returns the hex-encoded signature over the canonical
// payload for the provided IP and nonce using the shared lab key.
func signChallengeFor(t *testing.T, ip, nonce string) string {
	t.Helper()

	signature := auth.Sign([]byte(labSecretKey), auth.CanonicalPayload(ip, nonce))

	return hex.EncodeToString(signature)
}

// signChallengeWithKey returns the hex-encoded signature over the canonical
// payload for the provided IP and nonce using the provided key.
func signChallengeWithKey(t *testing.T, key []byte, ip, nonce string) string {
	t.Helper()

	signature := auth.Sign(key, auth.CanonicalPayload(ip, nonce))

	return hex.EncodeToString(signature)
}

// validSessionToken verifies that the token is a payload signed with the given
// HMAC secret and that the payload encodes the expected machine ID.
func validSessionToken(token, secret string, expectedMachineID int64) bool {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return false
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return false
	}

	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return false
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)

	if !hmac.Equal(mac.Sum(nil), signature) {
		return false
	}

	return strings.HasPrefix(string(payload), fmt.Sprintf("%d:", expectedMachineID))
}

// testClock is a controllable clock for time-dependent tests.
type testClock struct {
	mu  sync.Mutex
	now time.Time
}

// newTestClock creates a testClock initialized to the current time.
func newTestClock() *testClock {
	return &testClock{now: time.Now()}
}

// Now returns the current time of the clock.
func (clock *testClock) Now() time.Time {
	clock.mu.Lock()
	defer clock.mu.Unlock()

	return clock.now
}

// Advance moves the clock forward by the provided duration.
func (clock *testClock) Advance(duration time.Duration) {
	clock.mu.Lock()
	clock.now = clock.now.Add(duration)
	clock.mu.Unlock()
}
