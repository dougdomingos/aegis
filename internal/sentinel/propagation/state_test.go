package propagation_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dougdomingos.com/aegis/internal/common/propagation"
	sentinelPropagation "dougdomingos.com/aegis/internal/sentinel/propagation"
)

func TestFileStore_NewFileStore_WithEmptyPath_ReturnsError(t *testing.T) {
	if _, err := sentinelPropagation.NewFileStore("  "); err == nil {
		t.Error("NewFileStore() = nil, want an error for an empty path")
	}
}

func TestFileStore_Load_WhenFileMissing_ReturnsZeroState(t *testing.T) {
	store, _ := newFileStoreAt(t, "rules.json")

	state, err := store.Load()
	if err != nil {
		t.Fatalf("Load() = %v, want nil", err)
	}

	if state.Epoch != "" || state.LastSequence != 0 || state.PolicyID != nil {
		t.Errorf("state = %+v, want the zero state", state)
	}
	if state.Rules == nil || len(state.Rules) != 0 {
		t.Errorf("Rules = %v, want an empty slice", state.Rules)
	}
}

func TestFileStore_SaveAndLoad_WithPackage_RoundTrips(t *testing.T) {
	store, _ := newFileStoreAt(t, "rules.json")
	policyID := int64(7)
	protocol := "TCP"
	port := 443

	want := sentinelPropagation.State{
		Epoch:         "abcdef0123456789abcdef0123456789",
		LastSequence:  9,
		PolicyID:      &policyID,
		PolicyName:    "lab-whitelist",
		PolicyVersion: 3,
		PolicyType:    propagation.PolicyTypeWhitelist,
		Rules: []propagation.Rule{
			{Type: "IP", Value: "10.0.0.1", Protocol: &protocol, Port: &port},
			{Type: "DOMAIN", Value: "example.com"},
		},
	}

	if err := store.Save(want); err != nil {
		t.Fatalf("Save() = %v, want nil", err)
	}

	got, err := store.Load()
	if err != nil {
		t.Fatalf("Load() = %v, want nil", err)
	}

	if got.Epoch != want.Epoch || got.LastSequence != want.LastSequence {
		t.Errorf("envelope = %q/%d, want %q/%d", got.Epoch, got.LastSequence, want.Epoch, want.LastSequence)
	}
	if got.PolicyID == nil || *got.PolicyID != policyID {
		t.Errorf("PolicyID = %v, want %d", got.PolicyID, policyID)
	}
	if got.PolicyName != want.PolicyName || got.PolicyVersion != want.PolicyVersion || got.PolicyType != want.PolicyType {
		t.Errorf("policy metadata = %+v, want %+v", got, want)
	}
	if len(got.Rules) != 2 {
		t.Fatalf("Rules = %d entries, want 2", len(got.Rules))
	}
	if got.Rules[0].Protocol == nil || *got.Rules[0].Protocol != "TCP" {
		t.Errorf("IP rule protocol = %v, want TCP", got.Rules[0].Protocol)
	}
	if got.Rules[0].Port == nil || *got.Rules[0].Port != 443 {
		t.Errorf("IP rule port = %v, want 443", got.Rules[0].Port)
	}
	if got.Rules[1].Protocol != nil || got.Rules[1].Port != nil {
		t.Errorf("domain rule optional fields = %v/%v, want nil/nil", got.Rules[1].Protocol, got.Rules[1].Port)
	}
}

func TestFileStore_SaveAndLoad_WithRevoke_PreservesSequenceAndClearsRules(t *testing.T) {
	store, _ := newFileStoreAt(t, "rules.json")

	applied := sentinelPropagation.State{
		Epoch:         "abcdef0123456789abcdef0123456789",
		LastSequence:  9,
		PolicyID:      ptr(int64(7)),
		PolicyName:    "lab-whitelist",
		PolicyVersion: 3,
		PolicyType:    propagation.PolicyTypeWhitelist,
		Rules:         []propagation.Rule{{Type: "DOMAIN", Value: "example.com"}},
	}
	if err := store.Save(applied); err != nil {
		t.Fatalf("Save() = %v, want nil", err)
	}

	revoked := sentinelPropagation.State{
		Epoch:        applied.Epoch,
		LastSequence: 10,
		Rules:        []propagation.Rule{},
	}
	if err := store.Save(revoked); err != nil {
		t.Fatalf("Save() = %v, want nil", err)
	}

	got, err := store.Load()
	if err != nil {
		t.Fatalf("Load() = %v, want nil", err)
	}

	if got.LastSequence != 10 {
		t.Errorf("LastSequence = %d, want 10 (the sequence must survive a revoke)", got.LastSequence)
	}
	if got.PolicyID != nil || got.PolicyName != "" || got.PolicyVersion != 0 || got.PolicyType != "" {
		t.Errorf("policy metadata = %+v, want cleared", got)
	}
	if len(got.Rules) != 0 {
		t.Errorf("Rules = %d entries, want 0", len(got.Rules))
	}
}

func TestFileStore_Load_WithCorruptFile_ReturnsError(t *testing.T) {
	store, path := newFileStoreAt(t, "rules.json")

	if err := os.WriteFile(path, []byte("{not-json"), 0o600); err != nil {
		t.Fatalf("failed to write corrupt state: %v", err)
	}

	if _, err := store.Load(); err == nil {
		t.Error("Load() = nil, want an error for a corrupt file")
	}
}

func TestFileStore_Save_WhenParentMissing_CreatesDirectory(t *testing.T) {
	nested := filepath.Join(t.TempDir(), "missing", "deeper", "rules.json")

	store, err := sentinelPropagation.NewFileStore(nested)
	if err != nil {
		t.Fatalf("NewFileStore(%q) = %v, want nil", nested, err)
	}

	if err := store.Save(sentinelPropagation.State{Epoch: "abc", LastSequence: 1, Rules: []propagation.Rule{}}); err != nil {
		t.Fatalf("Save() = %v, want nil", err)
	}

	if _, err := os.Stat(nested); err != nil {
		t.Errorf("state file = missing, want %v", err)
	}
}

func TestFileStore_Save_WritesFileWithMode0600(t *testing.T) {
	store, path := newFileStoreAt(t, "rules.json")

	if err := store.Save(sentinelPropagation.State{Epoch: "abc", LastSequence: 1, Rules: []propagation.Rule{}}); err != nil {
		t.Fatalf("Save() = %v, want nil", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("failed to stat the state file: %v", err)
	}

	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Errorf("file mode = %O, want 0600", mode)
	}
}

func TestFileStore_Save_LeavesNoTemporaryFile(t *testing.T) {
	dir := t.TempDir()
	store, err := sentinelPropagation.NewFileStore(filepath.Join(dir, "rules.json"))
	if err != nil {
		t.Fatalf("NewFileStore() = %v, want nil", err)
	}

	if err := store.Save(sentinelPropagation.State{Epoch: "abc", LastSequence: 2, Rules: []propagation.Rule{}}); err != nil {
		t.Fatalf("Save() = %v, want nil", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("failed to read the state directory: %v", err)
	}

	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".rules-") {
			t.Errorf("temporary file %q left behind, want only the target", entry.Name())
		}
	}
}

// ============================================================================
// Helpers
// ============================================================================

// newFileStoreAt builds a FileStore rooted in a temporary directory, returning
// it together with the path of its state file.
func newFileStoreAt(t *testing.T, name string) (*sentinelPropagation.FileStore, string) {
	t.Helper()

	path := filepath.Join(t.TempDir(), name)

	store, err := sentinelPropagation.NewFileStore(path)
	if err != nil {
		t.Fatalf("NewFileStore() = %v, want nil", err)
	}

	return store, path
}

func ptr[T any](v T) *T {
	return &v
}
