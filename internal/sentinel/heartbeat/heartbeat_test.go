package heartbeat_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	sentinelErrors "dougdomingos.com/aegis/internal/errors"
	"dougdomingos.com/aegis/internal/sentinel/daemon"
	"dougdomingos.com/aegis/internal/sentinel/heartbeat"
	"dougdomingos.com/aegis/internal/sentinel/identity"
)

// fakePoster records every heartbeat call and replays scripted errors.
type fakePoster struct {
	send     func(ctx context.Context, token string) error
	tokens   []string
	attempts int
}

func (p *fakePoster) Heartbeat(ctx context.Context, token string) error {
	p.attempts++
	p.tokens = append(p.tokens, token)

	if p.send != nil {
		return p.send(ctx, token)
	}

	return nil
}

// fakeRenewer replays scripted session renewals.
type fakeRenewer struct {
	renew     func(ctx context.Context, id identity.Identity) (*daemon.Session, error)
	calls     int
	lastIdent identity.Identity
}

func (r *fakeRenewer) Authenticate(ctx context.Context, id identity.Identity) (*daemon.Session, error) {
	r.calls++
	r.lastIdent = id

	if r.renew != nil {
		return r.renew(ctx, id)
	}

	return &daemon.Session{Token: "renewed-token"}, nil
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

	await(t, time.Second, func() bool { return poster.attempts >= 1 })
	cancel()

	if err := <-done; err != nil {
		t.Errorf("Runner.Run() error = %v, want nil", err)
	}

	if poster.tokens[0] != "tok-1" {
		t.Errorf("first heartbeat token = %q, want tok-1", poster.tokens[0])
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

	await(t, time.Second, func() bool { return poster.attempts >= 2 && renewer.calls >= 1 })
	cancel()

	if err := <-done; err != nil {
		t.Errorf("Runner.Run() error = %v, want nil", err)
	}

	if poster.tokens[1] != "renewed-token" {
		t.Errorf("retried heartbeat token = %q, want renewed-token", poster.tokens[1])
	}

	if renewer.lastIdent.IP != "10.0.1.11" || renewer.lastIdent.MAC != "aa:bb:cc:dd:ee:ff" {
		t.Errorf("renewal identity = %+v, want the session identity", renewer.lastIdent)
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

	await(t, time.Second, func() bool { return poster.attempts >= 2 && renewer.calls >= 2 })
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

	await(t, time.Second, func() bool { return poster.attempts >= 2 })
	cancel()

	if err := <-done; err != nil {
		t.Errorf("Runner.Run() error = %v, want nil", err)
	}

	if renewer.calls != 0 {
		t.Errorf("renewal calls = %d, want 0 for a plain failure", renewer.calls)
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

	if poster.attempts != 0 {
		t.Errorf("heartbeat attempts = %d, want 0 after immediate cancellation", poster.attempts)
	}
}
