package auth_test

import (
	"bytes"
	"encoding/hex"
	"testing"

	"dougdomingos.com/aegis/internal/auth"
)

func TestAuth_CanonicalPayload_WithGoldenInput_ReturnsKnownEncoding(t *testing.T) {
	got := auth.CanonicalPayload("192.168.0.10", "abc123")
	want, _ := hex.DecodeString("0000000c3139322e3136382e302e313000000006616263313233")
	if !bytes.Equal(got, want) {
		t.Errorf("payload = %x, want %x", got, want)
	}
}

func TestAuth_Sign_WithGoldenInput_ReturnsKnownSignature(t *testing.T) {
	secret := []byte("0123456789abcdef0123456789abcdef")
	sig := auth.Sign(secret, auth.CanonicalPayload("192.168.0.10", "abc123"))
	want, _ := hex.DecodeString("74cd02b7ad2fb3d3ba018ba811db2e2ba947f96014e8fd7ed628b70d10caae02")
	if !bytes.Equal(sig, want) {
		t.Errorf("signature = %x, want %x", sig, want)
	}
}

func TestAuth_Verify_WithValidSignature_ReturnsTrue(t *testing.T) {
	secret := []byte("0123456789abcdef0123456789abcdef")
	payload := auth.CanonicalPayload("192.168.0.10", "abc123")
	sig := auth.Sign(secret, payload)

	if !auth.Verify(secret, payload, sig) {
		t.Error("expected verification to succeed for a valid signature")
	}
}

func TestAuth_Verify_WithTamperedPayload_ReturnsFalse(t *testing.T) {
	secret := []byte("0123456789abcdef0123456789abcdef")
	sig := auth.Sign(secret, auth.CanonicalPayload("192.168.0.10", "abc123"))

	if auth.Verify(secret, auth.CanonicalPayload("192.168.0.11", "abc123"), sig) {
		t.Error("expected verification to fail for a tampered ip")
	}
}

func TestAuth_Verify_WithWrongSecret_ReturnsFalse(t *testing.T) {
	payload := auth.CanonicalPayload("192.168.0.10", "abc123")
	sig := auth.Sign([]byte("0123456789abcdef0123456789abcdef"), payload)

	if auth.Verify([]byte("ffffffffffffffffffffffffffffffff"), payload, sig) {
		t.Error("expected verification to fail for a wrong secret")
	}
}
