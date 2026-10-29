package heartbeat_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	sentinelErrors "dougdomingos.com/aegis/internal/common/errors"
	"dougdomingos.com/aegis/internal/sentinel/daemon"
	"dougdomingos.com/aegis/internal/sentinel/heartbeat"
	"dougdomingos.com/aegis/internal/sentinel/identity"
)

// fakePoster records every heartbeat call and replays scripted errors. Its
// counters are guarded because the test goroutine polls them while the runner
// goroutine writes them.
type fakePoster struct {
	mu       sync.Mutex
	send     func(ctx context.Context, token string) error
	tokens   []string
	attempts int
}

func (p *fakePoster) Heartbeat(ctx context.Context, token string) error {
	p.mu.Lock()
	p.attempts++
	p.tokens = append(p.tokens, token)
	p.mu.Unlock()

	if p.send != nil {
		return p.send(ctx, token)
	}

	return nil
}

// recorded returns the number of calls and a copy of the tokens seen.
func (p *fakePoster) recorded() (int, []string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	return p.attempts, append([]string(nil), p.tokens...)
}

// fakeRenewer replays scripted session renewals, with the same cross-goroutine
// polling guard as fakePoster.
type fakeRenewer struct {
	mu        sync.Mutex
	renew     func(ctx context.Context, id identity.Identity) (*daemon.Session, error)
	calls     int
	lastIdent identity.Identity
}

func (r *fakeRenewer) Authenticate(ctx context.Context, id identity.Identity) (*daemon.Session, error) {
	r.mu.Lock()
	r.calls++
	r.lastIdent = id
	r.mu.Unlock()

	if r.renew != nil {
		return r.renew(ctx, id)
	}

	return &daemon.Session{Token: "renewed-token"}, nil
}

// recorded returns the number of calls and the last identity seen.
func (r *fakeRenewer) recorded() (int, identity.Identity) {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.calls, r.lastIdent
}

// await blocks until condition is true or the deadline expires.
func await(t *testing.T, deadline time.Duration, condition func() bool) {
	t.Helper()

	end := time.Now().Add(deadline)
	for time.Now().Before(end) {
		if condition() {
			return
		}

		time.Sleep(2 * time.Millisecond)
	}

	t.Fatalf("condition not met within %v", deadline)
}

func TestRunner_Run_TickerBeat_SendsHeartbeatWithSessionToken(t *testing.T) {
	poster := &fakePoster{}
	ctx, cancel := context.WithCancel(t.Context())

	runner := heartbeat.New(5*time.Millisecond, poster, &fakeRenewer{}, nil)

	done := make(chan error, 1)
	go func() {
		done <- runner.Run(ctx, &daemon.Session{Token: "tok-1", IP: "10.0.1.11", MAC: "aa:bb"})
	}()

	await(t, time.Second, func() bool { attempts, _ := poster.recorded(); return attempts >= 1 })
	cancel()

	if err := <-done; err != nil {
		t.Errorf("Runner.Run() error = %v, want nil", err)
	}

	_, tokens := poster.recorded()

	if tokens[0] != "tok-1" {
		t.Errorf("first heartbeat token = %q, want tok-1", tokens[0])
	}
}

func TestRunner_Run_SessionRejection_RenewsSessionAndRetries(t *testing.T) {
	rejection := fmt.Errorf("%w: expired", sentinelErrors.ErrAuthenticationRejected)

	poster := &fakePoster{}
	first := true
	poster.send = func(_ context.Context, token string) error {
		if first && token == "tok-1" {
			first = false

			return rejection
		}

		return nil
	}

	renewer := &fakeRenewer{}
	ctx, cancel := context.WithCancel(t.Context())

	runner := heartbeat.New(5*time.Millisecond, poster, renewer, nil)

	done := make(chan error, 1)
	go func() {
		done <- runner.Run(ctx, &daemon.Session{Token: "tok-1", IP: "10.0.1.11", MAC: "aa:bb:cc:dd:ee:ff"})
	}()

	await(t, time.Second, func() bool {
		attempts, _ := poster.recorded()
		calls, _ := renewer.recorded()
		return attempts >= 2 && calls >= 1
	})
	cancel()

	if err := <-done; err != nil {
		t.Errorf("Runner.Run() error = %v, want nil", err)
	}

	_, tokens := poster.recorded()

	if tokens[1] != "renewed-token" {
		t.Errorf("retried heartbeat token = %q, want renewed-token", tokens[1])
	}

	_, lastIdent := renewer.recorded()

	if lastIdent.IP != "10.0.1.11" || lastIdent.MAC != "aa:bb:cc:dd:ee:ff" {
		t.Errorf("renewal identity = %+v, want the session identity", lastIdent)
	}
}

func TestRunner_Run_ReauthFailure_WaitsForNextTick(t *testing.T) {
	rejection := fmt.Errorf("%w: expired", sentinelErrors.ErrAuthenticationRejected)

	poster := &fakePoster{send: func(_ context.Context, _ string) error { return rejection }}
	renewer := &fakeRenewer{renew: func(_ context.Context, _ identity.Identity) (*daemon.Session, error) {
		return nil, errors.New("orchestrator unreachable")
	}}

	ctx, cancel := context.WithCancel(t.Context())

	runner := heartbeat.New(5*time.Millisecond, poster, renewer, nil)

	done := make(chan error, 1)
	go func() {
		done <- runner.Run(ctx, &daemon.Session{Token: "tok-1", IP: "10.0.1.11"})
	}()

	await(t, time.Second, func() bool {
		attempts, _ := poster.recorded()
		calls, _ := renewer.recorded()
		return attempts >= 2 && calls >= 2
	})
	cancel()

	if err := <-done; err != nil {
		t.Errorf("Runner.Run() error = %v, want nil after failed renewals", err)
	}
}

func TestRunner_Run_PlainFailure_ContinuesNextTick(t *testing.T) {
	poster := &fakePoster{send: func(_ context.Context, _ string) error { return errors.New("connection reset") }}
	renewer := &fakeRenewer{}

	ctx, cancel := context.WithCancel(t.Context())

	runner := heartbeat.New(5*time.Millisecond, poster, renewer, nil)

	done := make(chan error, 1)
	go func() {
		done <- runner.Run(ctx, &daemon.Session{Token: "tok-1", IP: "10.0.1.11"})
	}()

	await(t, time.Second, func() bool { attempts, _ := poster.recorded(); return attempts >= 2 })
	cancel()

	if err := <-done; err != nil {
		t.Errorf("Runner.Run() error = %v, want nil", err)
	}

	calls, _ := renewer.recorded()

	if calls != 0 {
		t.Errorf("renewal calls = %d, want 0 for a plain failure", calls)
	}
}

func TestRunner_Run_ContextCancelled_ReturnsNil(t *testing.T) {
	poster := &fakePoster{}

	ctx, cancel := context.WithCancel(t.Context())
	runner := heartbeat.New(time.Hour, poster, &fakeRenewer{}, nil)

	cancel()

	done := make(chan error, 1)
	go func() {
		done <- runner.Run(ctx, &daemon.Session{Token: "tok-1"})
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Runner.Run() error = %v, want nil", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Runner.Run() did not return after cancellation")
	}

	attempts, _ := poster.recorded()

	if attempts != 0 {
		t.Errorf("heartbeat attempts = %d, want 0 after immediate cancellation", attempts)
	}
}

func TestRunner_New_NonPositiveInterval_Panics(t *testing.T) {
	cases := map[string]struct {
		interval time.Duration
	}{
		"zero":     {interval: 0},
		"negative": {interval: -time.Second},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recovered := recover(); recovered == nil {
					t.Errorf("expected a panic for interval %v, got none", tc.interval)
				}
			}()

			heartbeat.New(tc.interval, &fakePoster{}, &fakeRenewer{}, nil)
		})
	}
}
