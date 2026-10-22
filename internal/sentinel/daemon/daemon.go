// Package daemon implements the lifecycle of the Sentinel daemon.
package daemon

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

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

	// tasks are the long-running units of work executed by the daemon.
	tasks []Task
}

// New creates a daemon that runs the provided tasks once started.
func New(logger *slog.Logger, tasks ...Task) *Daemon {
	return &Daemon{
		logger: logger,
		tasks:  tasks,
	}
}

// Run starts the daemon tasks and blocks until the context is cancelled or a
// task fails. It returns nil when the context is cancelled, and the error of
// the failed task otherwise.
func (daemon *Daemon) Run(ctx context.Context) error {
	daemon.logger.Info("daemon starting")

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
