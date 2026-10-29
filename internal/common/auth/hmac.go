// Package auth provides the shared authentication primitives used by the
// Orchestrator to verify requests and by the Sentinel to sign them.
package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
)

// CanonicalPayload builds the deterministic byte representation of an
// authentication challenge. It concatenates the ip and nonce fields, prefixing
// each with a 4-byte big-endian length.
func CanonicalPayload(ip, nonce string) []byte {
	fields := []string{ip, nonce}

	payload := make([]byte, 0, 8+len(ip)+len(nonce))
	for _, field := range fields {
		length := make([]byte, 4)
		binary.BigEndian.PutUint32(length, uint32(len(field)))
		payload = append(payload, length...)
		payload = append(payload, field...)
	}

	return payload
}

// Sign returns the HMAC-SHA256 signature of payload using secret.
func Sign(secret, payload []byte) []byte {
	mac := hmac.New(sha256.New, secret)
	mac.Write(payload)

	return mac.Sum(nil)
}

// Verify reports whether signature is a valid HMAC-SHA256 of payload for the
// given secret, using a constant-time comparison.
func Verify(secret, payload, signature []byte) bool {
	return hmac.Equal(Sign(secret, payload), signature)
}
