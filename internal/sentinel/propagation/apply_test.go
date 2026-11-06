package propagation_test

import (
	"context"
	"errors"
	"testing"
	"time"

	ruleErrors "dougdomingos.com/aegis/internal/common/errors"
	"dougdomingos.com/aegis/internal/common/propagation"
	"dougdomingos.com/aegis/internal/sentinel/filtering"
	sentinelPropagation "dougdomingos.com/aegis/internal/sentinel/propagation"
)

// errSaveFailed aborts the persistence step of acceptance.
var errSaveFailed = errors.New("disk full")

// errFilterFailed aborts the enforcement step of acceptance.
var errFilterFailed = errors.New("nft failed")

func TestRunner_Apply_WithValidPackage_PersistsAndAcks(t *testing.T) {
	ack, store := runApply(t, applyBaseline(), applyPackage(3))

	if ack.Status != propagation.AckAcked {
		t.Fatalf("ack = %+v, want ACKED", ack)
	}
	if ack.Sequence != 3 {
		t.Errorf("ack.Sequence = %d, want 3", ack.Sequence)
	}
	if store.saves() != 1 {
		t.Errorf("Save calls = %d, want 1", store.saves())
	}

	saved, exists := store.lastSaved()
	if !exists {
		t.Fatal("no state saved, want the applied package persisted")
	}
	if saved.LastSequence != 3 || saved.Epoch != "epoch-a" {
		t.Errorf("state = %q/%d, want epoch-a/3", saved.Epoch, saved.LastSequence)
	}
	if saved.PolicyID == nil || *saved.PolicyID != 7 || saved.PolicyVersion != 2 || saved.PolicyName != "lab-whitelist" {
		t.Errorf("policy metadata = %+v, want the package identity", saved)
	}
	if len(saved.Rules) != 2 {
		t.Errorf("Rules = %d, want 2", len(saved.Rules))
	}
}

func TestRunner_Apply_WithStaleSequence_NacksAndLeavesStateUnchanged(t *testing.T) {
	ack, store := runApply(t, applyBaseline(), applyPackage(1))

	if ack.Status != propagation.AckNacked || ack.Reason != "stale sequence" {
		t.Errorf("ack = %+v, want NACKED with reason %q", ack, "stale sequence")
	}
	if store.saves() != 0 {
		t.Errorf("Save calls = %d, want 0 (the baseline must stay untouched)", store.saves())
	}
}

func TestRunner_Apply_WithEqualSequence_Nacks(t *testing.T) {
	ack, store := runApply(t, applyBaseline(), applyPackage(2))

	if ack.Status != propagation.AckNacked || ack.Reason != "stale sequence" {
		t.Errorf("ack = %+v, want NACKED with reason %q", ack, "stale sequence")
	}
	if store.saves() != 0 {
		t.Errorf("Save calls = %d, want 0", store.saves())
	}
}

func TestRunner_Apply_WithEpochChange_AcceptsLowerSequence(t *testing.T) {
	message := applyPackage(1)
	message.Epoch = "epoch-b"

	ack, store := runApply(t, applyBaseline(), message)

	if ack.Status != propagation.AckAcked {
		t.Fatalf("ack = %+v, want ACKED after an epoch change", ack)
	}

	saved, exists := store.lastSaved()
	if !exists || saved.Epoch != "epoch-b" || saved.LastSequence != 1 {
		t.Errorf("saved state = %+v, want epoch-b/1", saved)
	}
}

func TestRunner_Apply_WithMachineMismatch_Nacks(t *testing.T) {
	message := applyPackage(3)
	message.MachineID = 8

	ack, store := runApply(t, applyBaseline(), message)

	if ack.Status != propagation.AckNacked || ack.Reason != "machine mismatch" {
		t.Errorf("ack = %+v, want NACKED with reason %q", ack, "machine mismatch")
	}
	if store.saves() != 0 {
		t.Errorf("Save calls = %d, want 0", store.saves())
	}
}

func TestRunner_Apply_WithEmptyEpoch_Nacks(t *testing.T) {
	message := applyPackage(3)
	message.Epoch = ""

	ack, store := runApply(t, applyBaseline(), message)

	if ack.Status != propagation.AckNacked || ack.Reason != "empty epoch" {
		t.Errorf("ack = %+v, want NACKED with reason %q", ack, "empty epoch")
	}
	if store.saves() != 0 {
		t.Errorf("Save calls = %d, want 0", store.saves())
	}
}

func TestRunner_Apply_WithEmptySequence_Nacks(t *testing.T) {
	ack, store := runApply(t, applyBaseline(), applyPackage(0))

	if ack.Status != propagation.AckNacked || ack.Reason != "empty sequence" {
		t.Errorf("ack = %+v, want NACKED with reason %q", ack, "empty sequence")
	}
	if store.saves() != 0 {
		t.Errorf("Save calls = %d, want 0", store.saves())
	}
}

func TestRunner_Apply_WithInvalidPolicyType_Nacks(t *testing.T) {
	message := applyPackage(3)
	message.PolicyType = "TRANSPARENT"

	ack, store := runApply(t, applyBaseline(), message)

	if ack.Status != propagation.AckNacked || ack.Reason != "invalid policy type" {
		t.Errorf("ack = %+v, want NACKED with reason %q", ack, "invalid policy type")
	}
	if store.saves() != 0 {
		t.Errorf("Save calls = %d, want 0", store.saves())
	}
}

func TestRunner_Apply_WithEmptyRules_Nacks(t *testing.T) {
	message := applyPackage(3)
	message.Rules = nil

	ack, store := runApply(t, applyBaseline(), message)

	if ack.Status != propagation.AckNacked || ack.Reason != "empty policy" {
		t.Errorf("ack = %+v, want NACKED with reason %q", ack, "empty policy")
	}
	if store.saves() != 0 {
		t.Errorf("Save calls = %d, want 0", store.saves())
	}
}

func TestRunner_Apply_WithInvalidRule_Nacks(t *testing.T) {
	message := applyPackage(3)
	message.Rules = []propagation.Rule{{Type: "DOMAIN", Value: "not a domain!"}}

	ack, store := runApply(t, applyBaseline(), message)

	if ack.Status != propagation.AckNacked || ack.Reason != ruleErrors.ErrMalformedRuleValue.Error() {
		t.Errorf("ack = %+v, want NACKED with reason %q", ack, ruleErrors.ErrMalformedRuleValue.Error())
	}
	if store.saves() != 0 {
		t.Errorf("Save calls = %d, want 0", store.saves())
	}
}

func TestRunner_Apply_WithUnknownKind_Nacks(t *testing.T) {
	message := applyPackage(3)
	message.Kind = propagation.Kind("PING")

	ack, store := runApply(t, applyBaseline(), message)

	if ack.Status != propagation.AckNacked || ack.Reason != "unknown message kind" {
		t.Errorf("ack = %+v, want NACKED with reason %q", ack, "unknown message kind")
	}
	if store.saves() != 0 {
		t.Errorf("Save calls = %d, want 0", store.saves())
	}
}

func TestRunner_Apply_WhenSaveFails_NacksAndKeepsOldState(t *testing.T) {
	store := newRecordingStateStore()
	store.setLoadState(applyBaseline())
	store.setSaveErr(errSaveFailed)

	harness := startApplyHarness(t, store)
	defer harness.stop(t)

	// Two messages at the same sequence: the first fails to persist and is
	// NACKed, and the second is only accepted if the failure left the
	// in-memory sequence at the baseline.
	harness.stream.push(applyPackage(3))
	first := waitForAck(t, harness.stream.acks, "the first acknowledgement")
	if first.Status != propagation.AckNacked || first.Reason != "failed to persist state" {
		t.Fatalf("first ack = %+v, want NACKED with reason %q", first, "failed to persist state")
	}

	store.setSaveErr(nil)
	harness.stream.push(applyPackage(3))
	second := waitForAck(t, harness.stream.acks, "the second acknowledgement")
	if second.Status != propagation.AckAcked {
		t.Errorf("second ack = %+v, want ACKED: the failed save must keep the state at the baseline", second)
	}
}

func TestRunner_Apply_WithRevoke_PreservesSequenceAndClearsRules(t *testing.T) {
	message := &propagation.Message{
		Epoch:     "epoch-a",
		Sequence:  4,
		MachineID: 7,
		Kind:      propagation.KindRevoke,
	}

	ack, store := runApply(t, applyBaseline(), message)

	if ack.Status != propagation.AckAcked {
		t.Fatalf("ack = %+v, want ACKED", ack)
	}

	saved, exists := store.lastSaved()
	if !exists {
		t.Fatal("no state saved, want the revoked state persisted")
	}
	if saved.LastSequence != 4 {
		t.Errorf("LastSequence = %d, want 4 (the sequence must survive a revoke)", saved.LastSequence)
	}
	if saved.PolicyID != nil || saved.PolicyName != "" || saved.PolicyVersion != 0 || saved.PolicyType != "" {
		t.Errorf("policy metadata = %+v, want cleared", saved)
	}
	if len(saved.Rules) != 0 {
		t.Errorf("Rules = %d, want 0", len(saved.Rules))
	}
}

func TestRunner_Apply_WithValidPackage_AppliesFilteringBeforePersistAndAcks(t *testing.T) {
	store := newRecordingStateStore()
	store.setLoadState(applyBaseline())
	filter := &fakeFilter{store: store}

	harness := startApplyHarnessWithFilter(t, store, filter)
	defer harness.stop(t)

	harness.stream.push(applyPackage(3))
	ack := waitForAck(t, harness.stream.acks, "acknowledgement")

	if ack.Status != propagation.AckAcked {
		t.Fatalf("ack = %+v, want ACKED", ack)
	}
	if len(filter.applied) != 1 {
		t.Fatalf("Apply calls = %d, want 1", len(filter.applied))
	}
	if filter.savesAtApply[0] != 0 {
		t.Errorf("Save calls before Apply = %d, want 0 (filtering must precede persistence)", filter.savesAtApply[0])
	}
	if store.saves() != 1 {
		t.Errorf("Save calls = %d, want 1", store.saves())
	}

	snapshot := filter.applied[0]
	if snapshot.PolicyType != propagation.PolicyTypeWhitelist || len(snapshot.Rules) != 2 {
		t.Errorf("snapshot = %+v, want the whitelist package rules", snapshot)
	}
	if len(snapshot.OrchestratorIPs) != 1 || snapshot.OrchestratorIPs[0] != "10.0.0.5" {
		t.Errorf("OrchestratorIPs = %v, want the runner essentials", snapshot.OrchestratorIPs)
	}
	if len(snapshot.UpstreamDNSIPs) != 1 || snapshot.UpstreamDNSIPs[0] != "10.0.0.6" {
		t.Errorf("UpstreamDNSIPs = %v, want the runner essentials", snapshot.UpstreamDNSIPs)
	}
}

func TestRunner_Apply_WithFilterApplyFailure_NacksAndKeepsPreviousState(t *testing.T) {
	store := newRecordingStateStore()
	store.setLoadState(applyBaseline())
	filter := &fakeFilter{applyErr: errFilterFailed}

	harness := startApplyHarnessWithFilter(t, store, filter)
	defer harness.stop(t)

	harness.stream.push(applyPackage(3))
	first := waitForAck(t, harness.stream.acks, "the first acknowledgement")

	if first.Status != propagation.AckNacked || first.Reason != "failed to apply filtering: nft failed" {
		t.Fatalf("first ack = %+v, want NACKED with the filtering failure", first)
	}
	if store.saves() != 0 {
		t.Errorf("Save calls = %d, want 0", store.saves())
	}

	filter.applyErr = nil
	harness.stream.push(applyPackage(3))
	second := waitForAck(t, harness.stream.acks, "the second acknowledgement")
	if second.Status != propagation.AckAcked {
		t.Errorf("second ack = %+v, want ACKED: the failed apply must keep the state at the baseline", second)
	}
}

func TestRunner_Apply_WithNilFilter_NacksWithFilteringDisabled(t *testing.T) {
	store := newRecordingStateStore()
	store.setLoadState(applyBaseline())

	harness := startApplyHarnessWithFilter(t, store, nil)
	defer harness.stop(t)

	harness.stream.push(applyPackage(3))
	ack := waitForAck(t, harness.stream.acks, "acknowledgement")

	if ack.Status != propagation.AckNacked || ack.Reason != "filtering disabled" {
		t.Errorf("ack = %+v, want NACKED with reason %q", ack, "filtering disabled")
	}
	if store.saves() != 0 {
		t.Errorf("Save calls = %d, want 0", store.saves())
	}
}

func TestRunner_Apply_WithRevoke_ClearsFilteringAndAcks(t *testing.T) {
	store := newRecordingStateStore()
	store.setLoadState(applyBaseline())
	filter := &fakeFilter{store: store}

	harness := startApplyHarnessWithFilter(t, store, filter)
	defer harness.stop(t)

	harness.stream.push(applyRevoke(4))
	ack := waitForAck(t, harness.stream.acks, "acknowledgement")

	if ack.Status != propagation.AckAcked {
		t.Fatalf("ack = %+v, want ACKED", ack)
	}
	if filter.clears != 1 || len(filter.applied) != 0 {
		t.Errorf("Clear/Apply calls = %d/%d, want 1/0", filter.clears, len(filter.applied))
	}
	if store.saves() != 1 {
		t.Errorf("Save calls = %d, want 1", store.saves())
	}
}

func TestRunner_Apply_WithRevokeAndClearFailure_NacksAndKeepsPreviousState(t *testing.T) {
	store := newRecordingStateStore()
	store.setLoadState(applyBaseline())
	filter := &fakeFilter{clearErr: errFilterFailed}

	harness := startApplyHarnessWithFilter(t, store, filter)
	defer harness.stop(t)

	harness.stream.push(applyRevoke(4))
	first := waitForAck(t, harness.stream.acks, "the first acknowledgement")

	if first.Status != propagation.AckNacked || first.Reason != "failed to apply filtering: nft failed" {
		t.Fatalf("first ack = %+v, want NACKED with the filtering failure", first)
	}
	if store.saves() != 0 {
		t.Errorf("Save calls = %d, want 0", store.saves())
	}

	filter.clearErr = nil
	harness.stream.push(applyRevoke(4))
	second := waitForAck(t, harness.stream.acks, "the second acknowledgement")
	if second.Status != propagation.AckAcked {
		t.Errorf("second ack = %+v, want ACKED: the failed clear must keep the state at the baseline", second)
	}
}

// ============================================================================
// Fixtures and helpers
// ============================================================================

// applyBaseline is the persisted state every scenario starts from: an
// epoch-a package with sequence 2 already applied.
func applyBaseline() sentinelPropagation.State {
	policyID := int64(7)

	return sentinelPropagation.State{
		Epoch:         "epoch-a",
		LastSequence:  2,
		PolicyID:      &policyID,
		PolicyName:    "lab-whitelist",
		PolicyVersion: 1,
		PolicyType:    propagation.PolicyTypeWhitelist,
		Rules:         []propagation.Rule{{Type: "DOMAIN", Value: "old.example.com"}},
	}
}

// applyPackage builds a well-formed package for machine 7 above the baseline
// sequence.
func applyPackage(sequence int64) *propagation.Message {
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

// runApply seeds the baseline state, drives one message through the runner,
// and returns the acknowledgement with the store it persisted to.
func runApply(t *testing.T, baseline sentinelPropagation.State, message *propagation.Message) (propagation.Ack, *recordingStateStore) {
	t.Helper()

	store := newRecordingStateStore()
	store.setLoadState(baseline)

	return runApplyWithStore(t, store, message)
}

// runApplyWithStore drives one message through a runner over the provided
// store, returning the acknowledgement once it arrives.
func runApplyWithStore(t *testing.T, store *recordingStateStore, message *propagation.Message) (propagation.Ack, *recordingStateStore) {
	t.Helper()

	harness := startApplyHarness(t, store)
	defer harness.stop(t)

	harness.stream.push(message)

	return waitForAck(t, harness.stream.acks, "acknowledgement"), store
}

// applyHarness runs a runner over the provided store and a fresh stream,
// letting a test drive several messages through one session.
type applyHarness struct {
	stream *fakeStream
	cancel context.CancelFunc
	done   chan error
}

// startApplyHarness launches a runner connected to a single fake stream.
func startApplyHarness(t *testing.T, store *recordingStateStore) *applyHarness {
	t.Helper()

	return startApplyHarnessWithFilter(t, store, &fakeFilter{})
}

// startApplyHarnessWithFilter launches a runner enforcing through the provided
// filter; a nil filter disables filtering.
func startApplyHarnessWithFilter(t *testing.T, store *recordingStateStore, filter filtering.Filter) *applyHarness {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())

	stream := newFakeStream()
	connector := newFakeConnector()
	connector.enqueueStream(stream)

	runner := sentinelPropagation.New(store, connector, newFakeRenewer(nil), filter, applyEssentials(), discardLogger())

	done := make(chan error, 1)
	go func() { done <- runner.Run(ctx, runnerSession()) }()

	return &applyHarness{stream: stream, cancel: cancel, done: done}
}

// stop releases the stream and waits for the runner to return.
func (harness *applyHarness) stop(t *testing.T) {
	t.Helper()

	harness.stream.unblock()
	harness.cancel()

	select {
	case <-harness.done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run() did not return after cancellation")
	}
}

// applyEssentials are the essential addresses the runner under test carries.
func applyEssentials() sentinelPropagation.Essentials {
	return sentinelPropagation.Essentials{
		OrchestratorIPs: []string{"10.0.0.5"},
		UpstreamDNSIPs:  []string{"10.0.0.6"},
	}
}

// applyRevoke builds a revoke for machine 7 at the given sequence.
func applyRevoke(sequence int64) *propagation.Message {
	return &propagation.Message{
		Epoch:     "epoch-a",
		Sequence:  sequence,
		MachineID: 7,
		Kind:      propagation.KindRevoke,
	}
}

// fakeFilter records filtering calls and fails on demand. When store is set,
// it records how many saves had happened at each Apply. It is only touched
// from the runner goroutine while a test waits on acknowledgements.
type fakeFilter struct {
	store        *recordingStateStore
	applyErr     error
	clearErr     error
	applied      []filtering.PolicySnapshot
	savesAtApply []int
	clears       int
}

// Apply records the snapshot and returns the scripted error.
func (fake *fakeFilter) Apply(snapshot filtering.PolicySnapshot) error {
	if fake.applyErr != nil {
		return fake.applyErr
	}

	fake.applied = append(fake.applied, snapshot)
	if fake.store != nil {
		fake.savesAtApply = append(fake.savesAtApply, fake.store.saves())
	}

	return nil
}

// Clear records the call and returns the scripted error.
func (fake *fakeFilter) Clear() error {
	if fake.clearErr != nil {
		return fake.clearErr
	}

	fake.clears++

	return nil
}
