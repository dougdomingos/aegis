// Package heartbeat implements the Sentinel's periodic connectivity reports
// to the Orchestrator.
package heartbeat

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	sentinelErrors "dougdomingos.com/aegis/internal/errors"
	"dougdomingos.com/aegis/internal/sentinel/daemon"
	"dougdomingos.com/aegis/internal/sentinel/identity"
	"dougdomingos.com/aegis/internal/sentinel/logging"
)

// Poster reports connectivity to the Orchestrator.
type Poster interface {

	// Heartbeat sends a connectivity report for the provided session token.
	Heartbeat(ctx context.Context, token string) error
}

// SessionRenewer re-authenticates the machine identity against the
// Orchestrator.
type SessionRenewer interface {

	// Authenticate performs the challenge-response authentication of the
	// provided identity, returning the resulting session.
	Authenticate(ctx context.Context, id identity.Identity) (*daemon.Session, error)
}

// Runner sends periodic connectivity heartbeats to the Orchestrator,
// transparently renewing the session when the current token is rejected.
type Runner struct {

	// interval is the period between consecutive heartbeats.
	interval time.Duration

	// poster sends the connectivity reports.
	poster Poster

	// renewer re-authenticates the machine when its session is rejected.
	renewer SessionRenewer

	// logger records the heartbeat operations.
	logger *slog.Logger

	// token is the session token used by the next heartbeat. The daemon
	// runs the task in a single goroutine, so plain access is safe.
	token string
}

// New creates a heartbeat Runner with the provided interval, poster, renewer
// and logger.
func New(interval time.Duration, poster Poster, renewer SessionRenewer, logger *slog.Logger) *Runner {
	return &Runner{
		interval: interval,
		poster:   poster,
		renewer:  renewer,
		logger:   logger,
	}
}

// Run ticks until the context is cancelled, sending heartbeats with the
// current session token and renewing the session when a beat is rejected. It
// always returns nil: a heartbeat failure never stops the daemon.
func (runner *Runner) Run(ctx context.Context, session *daemon.Session) error {
	runner.token = session.Token

	ticker := time.NewTicker(runner.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			runner.beat(ctx, session)
		}
	}
}

// beat sends one heartbeat, renewing the session first when the Orchestrator
// rejects the current token. Every outcome is reported through the logger.
func (runner *Runner) beat(ctx context.Context, session *daemon.Session) {
	err := runner.poster.Heartbeat(ctx, runner.token)
	if err == nil {
		runner.logInfo("heartbeat sent")

		return
	}

	if !errors.Is(err, sentinelErrors.ErrAuthenticationRejected) {
		runner.logWarn("heartbeat failed", "error", err)

		return
	}

	renewed, err := runner.renewer.Authenticate(ctx, identity.Identity{IP: session.IP, MAC: session.MAC})
	if err != nil {
		runner.logWarn("heartbeat failed", "error", fmt.Errorf("failed to renew session: %w", err))

		return
	}

	runner.token = renewed.Token
	runner.logInfo("session renewed", "fingerprint", logging.Fingerprint(runner.token))

	if err := runner.poster.Heartbeat(ctx, runner.token); err != nil {
		runner.logWarn("heartbeat failed", "error", err)

		return
	}

	runner.logInfo("heartbeat sent")
}

// logInfo records an informational message when a logger is configured.
func (runner *Runner) logInfo(msg string, args ...any) {
	if runner.logger != nil {
		runner.logger.Info(msg, args...)
	}
}

// logWarn records a warning message when a logger is configured.
func (runner *Runner) logWarn(msg string, args ...any) {
	if runner.logger != nil {
		runner.logger.Warn(msg, args...)
	}
}
