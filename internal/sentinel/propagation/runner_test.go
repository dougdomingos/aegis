package propagation_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"dougdomingos.com/aegis/internal/common/propagation"
	"dougdomingos.com/aegis/internal/sentinel/daemon"
	"dougdomingos.com/aegis/internal/sentinel/identity"
	sentinelPropagation "dougdomingos.com/aegis/internal/sentinel/propagation"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ============================================================================
// Runner
// ============================================================================

func TestRunner_Run_WithPackageMessage_AppliesAndAcks(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store := newRecordingStateStore()
	stream := newFakeStream()
	stream.push(packageMessage(1))

	connector := newFakeConnector()
	connector.enqueueStream(stream)

	runner := sentinelPropagation.New(store, connector, newFakeRenewer(nil), &fakeFilter{}, sentinelPropagation.Essentials{}, discardLogger())

	session := runnerSession()
	done := make(chan error, 1)
	go func() { done <- runner.Run(ctx, session) }()

	ack := waitForAck(t, stream.acks, "acknowledgement")
	if ack.Status != propagation.AckAcked || ack.Sequence != 1 {
		t.Errorf("ack = %+v, want ACKED sequence 1", ack)
	}

	waitForCondition(t, func() bool { return store.saves() == 1 })
	saved, exists := store.lastSaved()
	if !exists {
		t.Fatal("no state saved, want the applied package persisted")
	}
	if saved.LastSequence != 1 || saved.PolicyID == nil || *saved.PolicyID != 7 || len(saved.Rules) != 1 {
		t.Errorf("saved = %+v, want the applied package state", saved)
	}
}

func TestRunner_Run_WhenStreamFails_Reconnects(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store := newRecordingStateStore()
	broken := newFakeStream()
	broken.failAfter(errors.New("connection reset"))

	recovered := newFakeStream()
	recovered.push(packageMessage(1))

	connector := newFakeConnector()
	connector.enqueueStream(broken)
	connector.enqueueStream(recovered)

	runner := sentinelPropagation.New(store, connector, newFakeRenewer(nil), &fakeFilter{}, sentinelPropagation.Essentials{}, discardLogger())

	done := make(chan error, 1)
	go func() { done <- runner.Run(ctx, runnerSession()) }()

	ack := waitForAck(t, recovered.acks, "acknowledgement over the recovered stream")
	if ack.Status != propagation.AckAcked {
		t.Errorf("ack = %+v, want ACKED", ack)
	}

	if connector.calls() < 2 {
		t.Errorf("connect calls = %d, want at least 2 (a reconnect must have happened)", connector.calls())
	}
	waitForCondition(t, func() bool { return store.saves() == 1 })
}

func TestRunner_Run_WhenUnauthenticated_RenewsSessionAndRetries(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store := newRecordingStateStore()
	renewed := &daemon.Session{Token: "renewed-token", MachineID: 7, MachineName: "lcc1-001", IP: "10.0.1.11", MAC: "aa:bb:cc:dd:ee:ff"}

	recovered := newFakeStream()
	recovered.push(packageMessage(1))

	connector := newFakeConnector()
	connector.enqueueError(status.Error(codes.Unauthenticated, "invalid session token"))
	connector.enqueueStream(recovered)

	renewer := newFakeRenewer(renewed)

	runner := sentinelPropagation.New(store, connector, renewer, &fakeFilter{}, sentinelPropagation.Essentials{}, discardLogger())

	done := make(chan error, 1)
	go func() { done <- runner.Run(ctx, runnerSession()) }()

	ack := waitForAck(t, recovered.acks, "acknowledgement after renewal")
	if ack.Status != propagation.AckAcked {
		t.Errorf("ack = %+v, want ACKED", ack)
	}

	if renewer.calls() != 1 {
		t.Errorf("renewal calls = %d, want 1", renewer.calls())
	}

	tokens := connector.recordedTokens()
	if len(tokens) < 2 || tokens[1] != "renewed-token" {
		t.Errorf("connect tokens = %v, want the renewed token on the second attempt", tokens)
	}
}

func TestRunner_Run_WhenRenewedTokenStillRejected_BacksOffInsteadOfSpinning(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store := newRecordingStateStore()
	connector := newFakeConnector()
	connector.alwaysErr = status.Error(codes.Unauthenticated, "invalid session token")

	renewer := newFakeRenewer(&daemon.Session{
		Token:       "renewed-token",
		MachineID:   7,
		MachineName: "lcc1-001",
		IP:          "10.0.1.11",
		MAC:         "aa:bb:cc:dd:ee:ff",
	})

	runner := sentinelPropagation.New(store, connector, renewer, &fakeFilter{}, sentinelPropagation.Essentials{}, discardLogger())

	done := make(chan error, 1)
	go func() { done <- runner.Run(ctx, runnerSession()) }()

	// The first cycle fails, renews, and must sleep the initial backoff
	// before retrying the rejected token: a tight loop would renew
	// hundreds of times within this window.
	time.Sleep(300 * time.Millisecond)

	if calls := renewer.calls(); calls > 2 {
		t.Errorf("renewal calls = %d, want at most 2 within the initial 1s backoff", calls)
	}

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run() = %v, want nil after cancellation", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run() did not return after cancellation")
	}
}

func TestRunner_Run_WhenContextCancelled_ReturnsNil(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	store := newRecordingStateStore()
	stream := newFakeStream()

	connector := newFakeConnector()
	connector.enqueueStream(stream)

	runner := sentinelPropagation.New(store, connector, newFakeRenewer(nil), &fakeFilter{}, sentinelPropagation.Essentials{}, discardLogger())

	done := make(chan error, 1)
	go func() { done <- runner.Run(ctx, runnerSession()) }()

	waitForCondition(t, func() bool { return connector.calls() >= 1 })

	cancel()
	stream.unblock()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run() = %v, want nil after cancellation", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run() did not return after cancellation")
	}
}

func TestRunner_Run_WhenStateFileCorrupt_StartsFromZeroState(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store := newRecordingStateStore()
	store.setLoadErr(errors.New("corrupt json"))

	stream := newFakeStream()
	stream.push(packageMessage(1))

	connector := newFakeConnector()
	connector.enqueueStream(stream)

	runner := sentinelPropagation.New(store, connector, newFakeRenewer(nil), &fakeFilter{}, sentinelPropagation.Essentials{}, discardLogger())

	done := make(chan error, 1)
	go func() { done <- runner.Run(ctx, runnerSession()) }()

	ack := waitForAck(t, stream.acks, "acknowledgement despite the corrupt state file")
	if ack.Status != propagation.AckAcked {
		t.Errorf("ack = %+v, want ACKED from the zero state", ack)
	}

	waitForCondition(t, func() bool { return store.saves() == 1 })
}

// ============================================================================
// Fixtures
// ============================================================================

// recordingStateStore is a sentinelPropagation.StateStore double that records
// saves and can fail Load or Save, or serve a seeded load state.
type recordingStateStore struct {
	mu      sync.Mutex
	stored  []sentinelPropagation.State
	loaded  sentinelPropagation.State
	loadErr error
	saveErr error
}

func newRecordingStateStore() *recordingStateStore {
	return &recordingStateStore{loaded: sentinelPropagation.State{Rules: []propagation.Rule{}}}
}

// Load returns the seeded state, or the injected error.
func (fake *recordingStateStore) Load() (sentinelPropagation.State, error) {
	fake.mu.Lock()
	defer fake.mu.Unlock()

	if fake.loadErr != nil {
		return sentinelPropagation.State{}, fake.loadErr
	}

	return fake.loaded, nil
}

// Save records the state, or returns the injected error.
func (fake *recordingStateStore) Save(state sentinelPropagation.State) error {
	fake.mu.Lock()
	defer fake.mu.Unlock()

	if fake.saveErr != nil {
		return fake.saveErr
	}

	fake.stored = append(fake.stored, state)

	return nil
}

func (fake *recordingStateStore) setLoadErr(err error) {
	fake.mu.Lock()
	defer fake.mu.Unlock()

	fake.loadErr = err
}

// setLoadState seeds the state Load reports to the runner.
func (fake *recordingStateStore) setLoadState(state sentinelPropagation.State) {
	fake.mu.Lock()
	defer fake.mu.Unlock()

	fake.loaded = state
}

// setSaveErr makes every Save return the injected error.
func (fake *recordingStateStore) setSaveErr(err error) {
	fake.mu.Lock()
	defer fake.mu.Unlock()

	fake.saveErr = err
}

func (fake *recordingStateStore) saves() int {
	fake.mu.Lock()
	defer fake.mu.Unlock()

	return len(fake.stored)
}

func (fake *recordingStateStore) lastSaved() (sentinelPropagation.State, bool) {
	fake.mu.Lock()
	defer fake.mu.Unlock()

	if len(fake.stored) == 0 {
		return sentinelPropagation.State{}, false
	}

	return fake.stored[len(fake.stored)-1], true
}

// fakeStream is a propagation.Stream double: it delivers queued messages,
// then fails or blocks until the test unblocks it, and records ACKs.
type fakeStream struct {
	mu       sync.Mutex
	queued   []*propagation.Message
	consumed int
	recvErr  error
	blocked  chan struct{}
	wake     chan struct{}
	closed   bool

	acks chan propagation.Ack
}

func newFakeStream() *fakeStream {
	return &fakeStream{
		blocked: make(chan struct{}),
		wake:    make(chan struct{}, 1),
		acks:    make(chan propagation.Ack, 8),
	}
}

// push queues a message and wakes a Recv that is already waiting.
func (fake *fakeStream) push(message *propagation.Message) {
	fake.mu.Lock()
	fake.queued = append(fake.queued, message)
	fake.mu.Unlock()

	select {
	case fake.wake <- struct{}{}:
	default:
	}
}

// failAfter makes every Recv after the queued messages return the error.
func (fake *fakeStream) failAfter(err error) {
	fake.mu.Lock()
	defer fake.mu.Unlock()

	fake.recvErr = err
}

// unblock releases a Recv that is waiting for messages.
func (fake *fakeStream) unblock() {
	fake.mu.Lock()
	defer fake.mu.Unlock()

	if !fake.closed {
		fake.closed = true
		close(fake.blocked)
	}
}

// Recv delivers queued messages, then the injected failure, or — once the
// stream is unblocked — io.EOF.
func (fake *fakeStream) Recv() (*propagation.Message, error) {
	for {
		fake.mu.Lock()
		if fake.consumed < len(fake.queued) {
			message := fake.queued[fake.consumed]
			fake.consumed++
			fake.mu.Unlock()

			return message, nil
		}

		err := fake.recvErr
		blocked := fake.blocked
		fake.mu.Unlock()

		if err != nil {
			return nil, err
		}

		select {
		case <-fake.wake:
		case <-blocked:
			return nil, io.EOF
		}
	}
}

// Send records one acknowledgement.
func (fake *fakeStream) Send(ack propagation.Ack) error {
	fake.acks <- ack

	return nil
}

// CloseSend is a no-op for the fake.
func (fake *fakeStream) CloseSend() error {
	return nil
}

// connectAttempt is one queued ConnectPropagation outcome.
type connectAttempt struct {
	stream *fakeStream
	err    error
}

// fakeConnector is a sentinelPropagation.Connector double serving queued
// outcomes and recording the token of every attempt. When alwaysErr is set,
// every attempt fails with it instead.
type fakeConnector struct {
	mu        sync.Mutex
	attempts  []connectAttempt
	seen      int
	tokens    []string
	alwaysErr error
}

func newFakeConnector() *fakeConnector {
	return &fakeConnector{}
}

func (fake *fakeConnector) enqueueStream(stream *fakeStream) {
	fake.mu.Lock()
	defer fake.mu.Unlock()

	fake.attempts = append(fake.attempts, connectAttempt{stream: stream})
}

func (fake *fakeConnector) enqueueError(err error) {
	fake.mu.Lock()
	defer fake.mu.Unlock()

	fake.attempts = append(fake.attempts, connectAttempt{err: err})
}

// ConnectPropagation consumes the next queued outcome, or the injected
// always-failing error.
func (fake *fakeConnector) ConnectPropagation(ctx context.Context, token string) (propagation.Stream, error) {
	fake.mu.Lock()
	defer fake.mu.Unlock()

	fake.tokens = append(fake.tokens, token)

	if fake.alwaysErr != nil {
		return nil, fake.alwaysErr
	}

	if fake.seen >= len(fake.attempts) {
		return nil, errors.New("no further streams queued for the propagation fake")
	}

	attempt := fake.attempts[fake.seen]
	fake.seen++

	if attempt.err != nil {
		return nil, attempt.err
	}

	return attempt.stream, nil
}

func (fake *fakeConnector) calls() int {
	fake.mu.Lock()
	defer fake.mu.Unlock()

	return fake.seen
}

func (fake *fakeConnector) recordedTokens() []string {
	fake.mu.Lock()
	defer fake.mu.Unlock()

	return append([]string{}, fake.tokens...)
}

// fakeRenewer is a sentinelPropagation.SessionRenewer double returning the
// configured session.
type fakeRenewer struct {
	mu       sync.Mutex
	session  *daemon.Session
	renewals int
}

func newFakeRenewer(session *daemon.Session) *fakeRenewer {
	return &fakeRenewer{session: session}
}

// Authenticate returns the configured renewed session.
func (fake *fakeRenewer) Authenticate(ctx context.Context, id identity.Identity) (*daemon.Session, error) {
	fake.mu.Lock()
	defer fake.mu.Unlock()

	fake.renewals++

	if fake.session == nil {
		return nil, errors.New("renewer not configured")
	}

	return fake.session, nil
}

func (fake *fakeRenewer) calls() int {
	fake.mu.Lock()
	defer fake.mu.Unlock()

	return fake.renewals
}

// ============================================================================
// Helpers
// ============================================================================

// discardLogger builds a logger that writes nowhere.
func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// runnerSession is the authenticated session the runner starts from.
func runnerSession() *daemon.Session {
	return &daemon.Session{
		Token:       "original-token",
		MachineID:   7,
		MachineName: "lcc1-001",
		IP:          "10.0.1.11",
		MAC:         "aa:bb:cc:dd:ee:ff",
	}
}

// packageMessage builds a valid package for machine 7 in the given sequence.
func packageMessage(sequence int64) *propagation.Message {
	return &propagation.Message{
		Epoch:         "epoch-a",
		Sequence:      sequence,
		MachineID:     7,
		Kind:          propagation.KindPackage,
		PolicyID:      7,
		PolicyName:    "lab-whitelist",
		PolicyVersion: 1,
		PolicyType:    propagation.PolicyTypeWhitelist,
		Rules:         []propagation.Rule{{Type: "DOMAIN", Value: "example.com"}},
	}
}

// waitForAck receives one acknowledgement or fails the test.
func waitForAck(t *testing.T, channel chan propagation.Ack, what string) propagation.Ack {
	t.Helper()

	select {
	case ack := <-channel:
		return ack
	case <-time.After(10 * time.Second):
		t.Fatalf("timed out waiting for %s", what)
		return propagation.Ack{}
	}
}

// waitForCondition polls the predicate until it holds within the timeout.
func waitForCondition(t *testing.T, condition func() bool) {
	t.Helper()

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}

	t.Fatalf("condition not met within 10s")
}
