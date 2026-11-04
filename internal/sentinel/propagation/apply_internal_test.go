package propagation

import (
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	ruleErrors "dougdomingos.com/aegis/internal/common/errors"
	"dougdomingos.com/aegis/internal/common/propagation"
	"dougdomingos.com/aegis/internal/sentinel/daemon"
)

// errSaveFailed aborts the persistence step of acceptance.
var errSaveFailed = errors.New("disk full")

// fakeStateStore is a StateStore that records every Save and can fail Load or
// Save on demand.
type fakeStateStore struct {
	mu      sync.Mutex
	stored  []State
	loadErr error
	saveErr error
}

func (fake *fakeStateStore) Load() (State, error) {
	fake.mu.Lock()
	defer fake.mu.Unlock()

	if fake.loadErr != nil {
		return State{}, fake.loadErr
	}

	return State{Rules: []propagation.Rule{}}, nil
}

func (fake *fakeStateStore) Save(state State) error {
	fake.mu.Lock()
	defer fake.mu.Unlock()

	if fake.saveErr != nil {
		return fake.saveErr
	}

	fake.stored = append(fake.stored, state)

	return nil
}

func (fake *fakeStateStore) saves() int {
	fake.mu.Lock()
	defer fake.mu.Unlock()

	return len(fake.stored)
}

func (fake *fakeStateStore) lastSaved() (State, bool) {
	fake.mu.Lock()
	defer fake.mu.Unlock()

	if len(fake.stored) == 0 {
		return State{}, false
	}

	return fake.stored[len(fake.stored)-1], true
}

// testRunner builds a Runner whose stream machinery is irrelevant to apply.
func testRunner(store *fakeStateStore) *Runner {
	return New(store, nil, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

// testSession is the authenticated session used by acceptance scenarios.
func testSession() *daemon.Session {
	return &daemon.Session{Token: "tok", MachineID: 7, MachineName: "lcc1-001", ExpiresAt: time.Now()}
}

// storedState is the applied baseline every scenario starts from.
func storedState() State {
	policyID := int64(7)

	return State{
		Epoch:         "epoch-a",
		LastSequence:  2,
		PolicyID:      &policyID,
		PolicyName:    "lab-whitelist",
		PolicyVersion: 1,
		PolicyType:    propagation.PolicyTypeWhitelist,
		Rules:         []propagation.Rule{{Type: "DOMAIN", Value: "old.example.com"}},
	}
}

// validPackage builds a well-formed package message for machine 7.
func validPackage(sequence int64) *propagation.Message {
	protocol := "TCP"
	port := 443

	return &propagation.Message{
		Epoch:         "epoch-a",
		Sequence:      sequence,
		MachineID:     7,
		Kind:          propagation.KindPackage,
		PolicyID:      7,
		PolicyName:    "lab-whitelist",
		PolicyVersion: 2,
		PolicyType:    propagation.PolicyTypeWhitelist,
		Rules: []propagation.Rule{
			{Type: "DOMAIN", Value: "example.com"},
			{Type: "IP", Value: "10.0.0.1", Protocol: &protocol, Port: &port},
		},
	}
}

func TestRunner_Apply_WithValidPackage_PersistsAndAcks(t *testing.T) {
	store := &fakeStateStore{}
	runner := testRunner(store)

	message := validPackage(3)

	next, ack := runner.apply(testSession(), storedState(), message)

	if ack.Status != propagation.AckAcked {
		t.Fatalf("ack = %+v, want ACKED", ack)
	}
	if ack.Sequence != 3 {
		t.Errorf("ack.Sequence = %d, want 3", ack.Sequence)
	}
	if next.LastSequence != 3 || next.Epoch != "epoch-a" {
		t.Errorf("state = %q/%d, want epoch-a/3", next.Epoch, next.LastSequence)
	}
	if next.PolicyID == nil || *next.PolicyID != 7 || next.PolicyVersion != 2 || next.PolicyName != "lab-whitelist" {
		t.Errorf("policy metadata = %+v, want the package identity", next)
	}
	if len(next.Rules) != 2 {
		t.Errorf("Rules = %d, want 2", len(next.Rules))
	}
	if store.saves() != 1 {
		t.Errorf("Save calls = %d, want 1", store.saves())
	}
}

func TestRunner_Apply_WithStaleSequence_NacksAndLeavesStateUnchanged(t *testing.T) {
	store := &fakeStateStore{}
	runner := testRunner(store)
	baseline := storedState()

	message := validPackage(1)

	next, ack := runner.apply(testSession(), baseline, message)

	if ack.Status != propagation.AckNacked || ack.Reason != "stale sequence" {
		t.Errorf("ack = %+v, want NACKED with reason %q", ack, "stale sequence")
	}
	if next.Epoch != baseline.Epoch || next.LastSequence != baseline.LastSequence || len(next.Rules) != len(baseline.Rules) {
		t.Errorf("state = %+v, want the unchanged baseline", next)
	}
	if store.saves() != 0 {
		t.Errorf("Save calls = %d, want 0", store.saves())
	}
}

func TestRunner_Apply_WithEqualSequence_Nacks(t *testing.T) {
	runner := testRunner(&fakeStateStore{})

	_, ack := runner.apply(testSession(), storedState(), validPackage(2))

	if ack.Status != propagation.AckNacked || ack.Reason != "stale sequence" {
		t.Errorf("ack = %+v, want NACKED with reason %q", ack, "stale sequence")
	}
}

func TestRunner_Apply_WithEpochChange_AcceptsLowerSequence(t *testing.T) {
	store := &fakeStateStore{}
	runner := testRunner(store)

	message := validPackage(1)
	message.Epoch = "epoch-b"

	next, ack := runner.apply(testSession(), storedState(), message)

	if ack.Status != propagation.AckAcked {
		t.Fatalf("ack = %+v, want ACKED after an epoch change", ack)
	}
	if next.Epoch != "epoch-b" || next.LastSequence != 1 {
		t.Errorf("state = %q/%d, want epoch-b/1", next.Epoch, next.LastSequence)
	}
	if saved, exists := store.lastSaved(); !exists || saved.Epoch != "epoch-b" {
		t.Errorf("saved state = %+v, want the new epoch", saved)
	}
}

func TestRunner_Apply_WithMachineMismatch_Nacks(t *testing.T) {
	runner := testRunner(&fakeStateStore{})

	message := validPackage(3)
	message.MachineID = 8

	_, ack := runner.apply(testSession(), storedState(), message)

	if ack.Status != propagation.AckNacked || ack.Reason != "machine mismatch" {
		t.Errorf("ack = %+v, want NACKED with reason %q", ack, "machine mismatch")
	}
}

func TestRunner_Apply_WithEmptyEpoch_Nacks(t *testing.T) {
	runner := testRunner(&fakeStateStore{})

	message := validPackage(3)
	message.Epoch = ""

	_, ack := runner.apply(testSession(), storedState(), message)

	if ack.Status != propagation.AckNacked || ack.Reason != "empty epoch" {
		t.Errorf("ack = %+v, want NACKED with reason %q", ack, "empty epoch")
	}
}

func TestRunner_Apply_WithEmptySequence_Nacks(t *testing.T) {
	runner := testRunner(&fakeStateStore{})

	_, ack := runner.apply(testSession(), storedState(), validPackage(0))

	if ack.Status != propagation.AckNacked || ack.Reason != "empty sequence" {
		t.Errorf("ack = %+v, want NACKED with reason %q", ack, "empty sequence")
	}
}

func TestRunner_Apply_WithInvalidPolicyType_Nacks(t *testing.T) {
	runner := testRunner(&fakeStateStore{})

	message := validPackage(3)
	message.PolicyType = "TRANSPARENT"

	_, ack := runner.apply(testSession(), storedState(), message)

	if ack.Status != propagation.AckNacked || ack.Reason != "invalid policy type" {
		t.Errorf("ack = %+v, want NACKED with reason %q", ack, "invalid policy type")
	}
}

func TestRunner_Apply_WithEmptyRules_Nacks(t *testing.T) {
	runner := testRunner(&fakeStateStore{})

	message := validPackage(3)
	message.Rules = nil

	_, ack := runner.apply(testSession(), storedState(), message)

	if ack.Status != propagation.AckNacked || ack.Reason != "empty policy" {
		t.Errorf("ack = %+v, want NACKED with reason %q", ack, "empty policy")
	}
}

func TestRunner_Apply_WithInvalidRule_Nacks(t *testing.T) {
	runner := testRunner(&fakeStateStore{})

	message := validPackage(3)
	message.Rules = []propagation.Rule{{Type: "DOMAIN", Value: "not a domain!"}}

	_, ack := runner.apply(testSession(), storedState(), message)

	if ack.Status != propagation.AckNacked || ack.Reason != ruleErrors.ErrMalformedRuleValue.Error() {
		t.Errorf("ack = %+v, want NACKED with reason %q", ack, ruleErrors.ErrMalformedRuleValue.Error())
	}
}

func TestRunner_Apply_WithUnknownKind_Nacks(t *testing.T) {
	runner := testRunner(&fakeStateStore{})

	message := validPackage(3)
	message.Kind = propagation.Kind("PING")

	_, ack := runner.apply(testSession(), storedState(), message)

	if ack.Status != propagation.AckNacked || ack.Reason != "unknown message kind" {
		t.Errorf("ack = %+v, want NACKED with reason %q", ack, "unknown message kind")
	}
}

func TestRunner_Apply_WhenSaveFails_NacksAndKeepsOldState(t *testing.T) {
	store := &fakeStateStore{saveErr: errSaveFailed}
	runner := testRunner(store)
	baseline := storedState()

	next, ack := runner.apply(testSession(), baseline, validPackage(3))

	if ack.Status != propagation.AckNacked || ack.Reason != "failed to persist state" {
		t.Errorf("ack = %+v, want NACKED with reason %q", ack, "failed to persist state")
	}
	if next.LastSequence != baseline.LastSequence || next.PolicyID == nil || *next.PolicyID != *baseline.PolicyID {
		t.Errorf("state = %+v, want the old state kept in memory", next)
	}
}

func TestRunner_Apply_WithRevoke_PreservesSequenceAndClearsRules(t *testing.T) {
	store := &fakeStateStore{}
	runner := testRunner(store)

	message := &propagation.Message{
		Epoch:     "epoch-a",
		Sequence:  4,
		MachineID: 7,
		Kind:      propagation.KindRevoke,
	}

	next, ack := runner.apply(testSession(), storedState(), message)

	if ack.Status != propagation.AckAcked {
		t.Fatalf("ack = %+v, want ACKED", ack)
	}
	if next.LastSequence != 4 {
		t.Errorf("LastSequence = %d, want 4 (the sequence must survive a revoke)", next.LastSequence)
	}
	if next.PolicyID != nil || next.PolicyName != "" || next.PolicyVersion != 0 || next.PolicyType != "" {
		t.Errorf("policy metadata = %+v, want cleared", next)
	}
	if len(next.Rules) != 0 {
		t.Errorf("Rules = %d, want 0", len(next.Rules))
	}
	if saved, exists := store.lastSaved(); !exists || saved.LastSequence != 4 {
		t.Errorf("saved = %+v, want the revoked state persisted", saved)
	}
}
