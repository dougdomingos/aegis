package store_test

import (
	"testing"
	"time"

	"dougdomingos.com/aegis/internal/orchestrator/store"
)

func TestNonceStore_Issue_ReturnsNonceAndExpiry(t *testing.T) {
	now := time.Now()
	nonceStore := store.NewNonceStore(func() time.Time { return now })

	nonce, expiresAt := nonceStore.Issue("10.0.1.11")

	if len(nonce) != 32 {
		t.Errorf("expected 32-character nonce, got %d characters", len(nonce))
	}

	if !expiresAt.Equal(now.Add(store.NonceTTL)) {
		t.Errorf("expected expiry %v, got %v", now.Add(store.NonceTTL), expiresAt)
	}
}

func TestNonceStore_Issue_WithExistingNonce_ReplacesPrevious(t *testing.T) {
	now := time.Now()
	nonceStore := store.NewNonceStore(func() time.Time { return now })

	first, _ := nonceStore.Issue("10.0.1.11")
	second, _ := nonceStore.Issue("10.0.1.11")

	if first == second {
		t.Errorf("expected a fresh nonce, got the same value %q twice", first)
	}

	if got, ok := nonceStore.Lookup("10.0.1.11"); !ok || got != second {
		t.Errorf("expected the current nonce %q to be pending, got %q (ok=%v)", second, got, ok)
	}
}

func TestNonceStore_Lookup_WithValidNonce_ReturnsTrue(t *testing.T) {
	now := time.Now()
	nonceStore := store.NewNonceStore(func() time.Time { return now })

	nonce, _ := nonceStore.Issue("10.0.1.11")

	got, ok := nonceStore.Lookup("10.0.1.11")

	if !ok {
		t.Error("expected a valid nonce to be found")
	}

	if got != nonce {
		t.Errorf("expected nonce %q, got %q", nonce, got)
	}
}

func TestNonceStore_Lookup_WithUnknownIP_ReturnsFalse(t *testing.T) {
	now := time.Now()
	nonceStore := store.NewNonceStore(func() time.Time { return now })

	if _, ok := nonceStore.Lookup("10.0.1.99"); ok {
		t.Error("expected an unknown IP to have no pending nonce")
	}
}

func TestNonceStore_Lookup_AfterExpiry_ReturnsFalse(t *testing.T) {
	now := time.Now()
	nonceStore := store.NewNonceStore(func() time.Time { return now })

	nonceStore.Issue("10.0.1.11")
	now = now.Add(2 * store.NonceTTL)

	if _, ok := nonceStore.Lookup("10.0.1.11"); ok {
		t.Error("expected an expired nonce to be rejected")
	}
}

func TestNonceStore_Consume_RemovesPendingNonce(t *testing.T) {
	now := time.Now()
	nonceStore := store.NewNonceStore(func() time.Time { return now })

	nonceStore.Issue("10.0.1.11")
	nonceStore.Consume("10.0.1.11")

	if _, ok := nonceStore.Lookup("10.0.1.11"); ok {
		t.Error("expected the nonce to be removed after consumption")
	}
}
