// Package daemon implements the lifecycle of the Sentinel daemon.
package daemon

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"dougdomingos.com/aegis/internal/sentinel/identity"
	"dougdomingos.com/aegis/internal/sentinel/logging"
)

// IdentityDetector resolves the network identity of the Sentinel host.
type IdentityDetector interface {

	// Detect resolves the local network identity.
	Detect() (identity.Identity, error)
}

// Authenticator performs the authentication of the Sentinel identity against
// the Orchestrator.
type Authenticator interface {

	// Authenticate performs the challenge-response authentication of the
	// provided identity, returning the resulting session.
	Authenticate(ctx context.Context, id identity.Identity) (*Session, error)
}

// Task is a long-running unit of work executed by the daemon.
type Task struct {

	// Name identifies the task in log records.
	Name string

	// Run executes the task until the context is cancelled. A non-nil error
	// is fatal and stops the daemon.
	Run func(ctx context.Context) error
}

// Session holds the outcome of a successful authentication.
type Session struct {

	// Token is the session token issued by the Orchestrator.
	Token string

	// ExpiresAt is the moment at which the session token expires.
	ExpiresAt time.Time

	// MachineName is the name assigned to the machine by the Orchestrator.
	MachineName string
}

// Daemon owns the lifecycle of the Sentinel process.
type Daemon struct {

	// logger records the lifecycle operations.
	logger *slog.Logger

	// detector resolves the network identity of the host at startup.
	detector IdentityDetector

	// authenticator authenticates the machine identity at startup.
	authenticator Authenticator

	// tasks are the long-running units of work executed by the daemon.
	tasks []Task
}

// New creates a daemon that detects its identity, authenticates against the
// Orchestrator and finally runs the provided tasks.
func New(logger *slog.Logger, detector IdentityDetector, authenticator Authenticator, tasks ...Task) *Daemon {
	return &Daemon{
		logger:        logger,
		detector:      detector,
		authenticator: authenticator,
		tasks:         tasks,
	}
}

// Run detects the machine identity, authenticates against the Orchestrator,
// starts the daemon tasks and blocks until the context is cancelled or a task
// fails. It returns nil when the context is cancelled, and the error of the
// failed operation otherwise.
func (daemon *Daemon) Run(ctx context.Context) error {
	daemon.logger.Info("daemon starting")

	id, err := daemon.detector.Detect()
	if err != nil {
		return fmt.Errorf("failed to detect machine identity: %w", err)
	}

	daemon.logger.Info("machine identity detected", "ip", id.IP, "mac", id.MAC)

	session, err := daemon.authenticator.Authenticate(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to authenticate: %w", err)
	}

	daemon.logger.Info("machine authenticated",
		"machine", session.MachineName,
		"session_expires_at", session.ExpiresAt,
		"session_fingerprint", logging.Fingerprint(session.Token))

	errCh := make(chan error, len(daemon.tasks))
	for _, task := range daemon.tasks {
		go func(task Task) {
			if err := task.Run(ctx); err != nil {
				errCh <- fmt.Errorf("task %q failed: %w", task.Name, err)
			}
		}(task)
	}

	daemon.logger.Info("daemon running", "tasks", len(daemon.tasks))

	select {
	case <-ctx.Done():
		daemon.logger.Info("daemon stopping")

		return nil
	case err := <-errCh:
		return err
	}
}
