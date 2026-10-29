package auth_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"

	"dougdomingos.com/aegis/internal/common/auth"
	machineErrors "dougdomingos.com/aegis/internal/common/errors"
)

const (
	sessionSecret     = "test-secret"
	alternativeSecret = "other-secret"
)

// sessionNow is the fixed clock used across the session token tests.
var sessionNow = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

func TestSignSessionToken_RoundTrip_ReturnsValidToken(t *testing.T) {
	token, expiresAt, err := auth.SignSessionToken(42, sessionSecret, 24*time.Hour, sessionNow)
	if err != nil {
		t.Fatalf("expected no signing error, got: %v", err)
	}

	if want := sessionNow.Add(24 * time.Hour); !expiresAt.Equal(want) {
		t.Errorf("expected expiry %v, got %v", want, expiresAt)
	}

	machineID, err := auth.ParseSessionToken(token, sessionSecret, sessionNow.Add(time.Second))
	if err != nil {
		t.Fatalf("expected a parseable token, got error: %v", err)
	}

	if machineID != 42 {
		t.Errorf("expected machine id 42, got %d", machineID)
	}
}

func TestParseSessionToken_ExpiredToken_ReturnsInvalidSessionToken(t *testing.T) {
	token, _, err := auth.SignSessionToken(42, sessionSecret, 24*time.Hour, sessionNow)
	if err != nil {
		t.Fatalf("expected no signing error, got: %v", err)
	}

	_, err = auth.ParseSessionToken(token, sessionSecret, sessionNow.Add(25*time.Hour))
	if !errors.Is(err, machineErrors.ErrSessionTokenInvalid) {
		t.Errorf("expected ErrSessionTokenInvalid for an expired token, got: %v", err)
	}
}

func TestParseSessionToken_TamperedToken_ReturnsInvalidSessionToken(t *testing.T) {
	token, _, err := auth.SignSessionToken(42, sessionSecret, 24*time.Hour, sessionNow)
	if err != nil {
		t.Fatalf("expected no signing error, got: %v", err)
	}

	parts := strings.SplitN(token, ".", 2)

	tamperedPayload := flipByte(parts[0]) + "." + parts[1]
	if _, err := auth.ParseSessionToken(tamperedPayload, sessionSecret, sessionNow); !errors.Is(err, machineErrors.ErrSessionTokenInvalid) {
		t.Errorf("expected ErrSessionTokenInvalid for a tampered payload, got: %v", err)
	}

	tamperedSignature := parts[0] + "." + flipByte(parts[1])
	if _, err := auth.ParseSessionToken(tamperedSignature, sessionSecret, sessionNow); !errors.Is(err, machineErrors.ErrSessionTokenInvalid) {
		t.Errorf("expected ErrSessionTokenInvalid for a tampered signature, got: %v", err)
	}
}

func TestParseSessionToken_MalformedToken_ReturnsInvalidSessionToken(t *testing.T) {
	nonNumericPayload := base64.RawURLEncoding.EncodeToString([]byte("x:9999999999")) + "." +
		base64.RawURLEncoding.EncodeToString(signSegment([]byte("x:9999999999")))

	scenarios := map[string]struct {
		token string
	}{
		"empty token":            {token: ""},
		"missing separator":      {token: "abc"},
		"undecodable segments":   {token: "a.b"},
		"non-numeric machine id": {token: nonNumericPayload},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			_, err := auth.ParseSessionToken(scenario.token, sessionSecret, sessionNow)
			if !errors.Is(err, machineErrors.ErrSessionTokenInvalid) {
				t.Errorf("expected ErrSessionTokenInvalid, got: %v", err)
			}
		})
	}
}

func TestParseSessionToken_WrongSecret_ReturnsInvalidSessionToken(t *testing.T) {
	token, _, err := auth.SignSessionToken(42, sessionSecret, 24*time.Hour, sessionNow)
	if err != nil {
		t.Fatalf("expected no signing error, got: %v", err)
	}

	_, err = auth.ParseSessionToken(token, alternativeSecret, sessionNow)
	if !errors.Is(err, machineErrors.ErrSessionTokenInvalid) {
		t.Errorf("expected ErrSessionTokenInvalid for a foreign secret, got: %v", err)
	}
}

// flipByte changes the first byte of the segment to a different printable
// character, guaranteeing a different (but still decodable-looking) value.
func flipByte(segment string) string {
	if segment == "" {
		return "A"
	}

	flipped := 'A'
	if segment[0] == 'A' {
		flipped = 'B'
	}

	return string(flipped) + segment[1:]
}

// signSegment returns the raw HMAC-SHA256 of the payload with the test secret.
func signSegment(payload []byte) []byte {
	mac := hmac.New(sha256.New, []byte(sessionSecret))
	mac.Write(payload)

	return mac.Sum(nil)
}
