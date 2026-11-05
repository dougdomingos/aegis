package propagation

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"time"

	"dougdomingos.com/aegis/internal/common/propagation"
	"dougdomingos.com/aegis/internal/sentinel/daemon"
	"dougdomingos.com/aegis/internal/sentinel/identity"
	"dougdomingos.com/aegis/internal/sentinel/logging"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	// initialBackoff is the first wait between reconnection attempts.
	initialBackoff = 1 * time.Second

	// maxBackoff caps the reconnection wait, so a long outage does not delay
	// recovery beyond it.
	maxBackoff = 30 * time.Second
)

// StateStore persists the applied propagation state.
type StateStore interface {
	Load() (State, error)
	Save(state State) error
}

// Connector opens authenticated propagation streams at the Orchestrator.
type Connector interface {
	ConnectPropagation(ctx context.Context, token string) (propagation.Stream, error)
}

// SessionRenewer re-authenticates the machine when the Orchestrator rejects
// its session token.
type SessionRenewer interface {
	Authenticate(ctx context.Context, id identity.Identity) (*daemon.Session, error)
}

// Runner keeps one long-lived propagation stream alive: it applies and
// acknowledges received messages, reconnects with exponential backoff after
// failures, and renews the session when the Orchestrator reports it as
// unauthenticated. It never returns a non-nil error, so a propagation failure
// can never kill the daemon.
type Runner struct {
	store     StateStore
	connector Connector
	renewer   SessionRenewer
	logger    *slog.Logger
}

// New builds a propagation Runner. A nil logger discards every record.
func New(store StateStore, connector Connector, renewer SessionRenewer, logger *slog.Logger) *Runner {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}

	return &Runner{store: store, connector: connector, renewer: renewer, logger: logger}
}

// Run consumes the propagation stream until the context is cancelled,
// reconnecting and renewing the session as needed.
func (runner *Runner) Run(ctx context.Context, session *daemon.Session) error {
	current := *session

	state, err := runner.store.Load()
	if err != nil {
		runner.logger.Warn("propagation state corrupt", "error", err)
		state = State{Rules: []propagation.Rule{}}
	}

	backoff := initialBackoff

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		stream, err := runner.connector.ConnectPropagation(ctx, current.Token)
		if err != nil {
			if status.Code(err) == codes.Unauthenticated {
				renewed, renewErr := runner.renewer.Authenticate(ctx, identity.Identity{IP: current.IP, MAC: current.MAC})
				switch {
				case renewErr != nil:
					runner.logger.Warn("propagation session renewal failed", "error", renewErr)
				case renewed == nil:
					runner.logger.Warn("propagation session renewal returned no session")
				default:
					current = *renewed
					runner.logger.Info("propagation session renewed", "fingerprint", logging.Fingerprint(current.Token))
				}
			} else {
				runner.logger.Warn("propagation stream failed", "error", err)
			}

			// Every failed attempt backs off — including a successful
			// renewal whose fresh token the Orchestrator still rejects,
			// which must not spin the loop.
			if !sleepWithContext(ctx, backoff) {
				return nil
			}

			backoff = doubledBackoff(backoff)
			continue
		}

		backoff = initialBackoff
		runner.logger.Info("propagation connected", "machine", current.MachineName)

		runner.consume(ctx, &current, &state, stream)

		if err := stream.CloseSend(); err != nil {
			runner.logger.Debug("propagation stream close failed", "error", err)
		}
	}
}

// consume receives, applies, and acknowledges messages until the stream fails
// or the context is cancelled. A send or receive error returns control to the
// reconnection loop.
func (runner *Runner) consume(ctx context.Context, session *daemon.Session, state *State, stream propagation.Stream) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		message, err := stream.Recv()
		if err != nil {
			if !errors.Is(err, io.EOF) && ctx.Err() == nil {
				runner.logger.Warn("propagation stream failed", "error", err)
			}

			return
		}

		next, ack := runner.apply(session, *state, message)

		if ack.Status == propagation.AckAcked {
			*state = next

			if message.Kind == propagation.KindRevoke {
				runner.logger.Info("revoke applied", "sequence", ack.Sequence)
			} else {
				runner.logger.Info("package applied",
					"sequence", ack.Sequence,
					"policy", message.PolicyName,
					"version", message.PolicyVersion,
					"rules", len(message.Rules))
			}
		} else {
			runner.logger.Warn("propagation message rejected", "sequence", ack.Sequence, "reason", ack.Reason)
		}

		if err := stream.Send(ack); err != nil {
			runner.logger.Warn("propagation acknowledgement lost", "sequence", ack.Sequence, "error", err)
			return
		}
	}
}

// sleepWithContext waits for the given duration, returning false when the
// context is cancelled first.
func sleepWithContext(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

// doubledBackoff doubles the wait, capped at maxBackoff.
func doubledBackoff(duration time.Duration) time.Duration {
	doubled := duration * 2
	if doubled > maxBackoff {
		return maxBackoff
	}

	return doubled
}
