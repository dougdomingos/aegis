package store

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

// NonceTTL is the lifetime of an issued authentication nonce.
const NonceTTL = 60 * time.Second

// NonceLength is the number of random bytes contained in an issued nonce.
const NonceLength = 16

// nonceEntry holds a pending authentication nonce and its expiry.
type nonceEntry struct {
	nonce     string
	expiresAt time.Time
}

// NonceStore tracks the pending authentication nonces issued per machine IP.
// It is safe for concurrent use.
type NonceStore struct {
	mu      sync.Mutex
	pending map[string]nonceEntry
	now     func() time.Time
}

// NewNonceStore creates a nonceStore using now as its clock.
func NewNonceStore(now func() time.Time) *NonceStore {
	return &NonceStore{
		pending: make(map[string]nonceEntry),
		now:     now,
	}
}

// Issue generates a fresh nonce for the provided IP, replacing any pending
// nonce, and returns it along with its expiry.
func (store *NonceStore) Issue(ip string) (string, time.Time) {
	nonce := randomNonce()
	expiresAt := store.now().Add(NonceTTL)

	store.mu.Lock()
	store.pending[ip] = nonceEntry{nonce: nonce, expiresAt: expiresAt}
	store.mu.Unlock()

	return nonce, expiresAt
}

// Lookup returns the pending, unexpired nonce for the provided IP without
// removing it. The second return value is false when no live nonce exists.
func (store *NonceStore) Lookup(ip string) (string, bool) {
	store.mu.Lock()
	defer store.mu.Unlock()

	entry, ok := store.pending[ip]
	if !ok {
		return "", false
	}

	if store.now().After(entry.expiresAt) {
		delete(store.pending, ip)
		return "", false
	}

	return entry.nonce, true
}

// Consume removes the pending nonce for the provided IP so it cannot be reused.
func (store *NonceStore) Consume(ip string) {
	store.mu.Lock()
	delete(store.pending, ip)
	store.mu.Unlock()
}

// randomNonce returns a cryptographically random nonce encoded as
// hexadecimal.
func randomNonce() string {
	bytes := make([]byte, NonceLength)
	if _, err := rand.Read(bytes); err != nil {
		panic("failed to generate nonce: " + err.Error())
	}

	return hex.EncodeToString(bytes)
}
