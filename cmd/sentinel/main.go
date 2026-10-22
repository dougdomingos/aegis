// Command sentinel runs the Aegis Sentinel daemon, the per-endpoint agent
// that authenticates with the Orchestrator and enforces the policies it
// distributes.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"dougdomingos.com/aegis/internal/sentinel/daemon"
	"dougdomingos.com/aegis/internal/sentinel/logging"
)

// secondSignalGrace is the grace window given to a second termination signal
// before the graceful shutdown starts, so that a pair of signals sent in
// quick succession still forces the exit.
const secondSignalGrace = 50 * time.Millisecond

func main() {
	os.Exit(run())
}

// run starts the Sentinel daemon and returns the process exit code.
func run() int {
	logger := logging.New(os.Stderr, logging.Config{Level: slog.LevelInfo, Format: logging.FormatText})

	ctx, stop := notifySignals(logger)
	defer stop()

	if err := daemon.New(logger).Run(ctx); err != nil {
		logger.Error("daemon stopped with a fatal error", "error", err)

		return 1
	}

	return 0
}

// notifySignals returns a context that is cancelled when a termination signal
// is received, along with a function that releases the signal handler. A
// second received signal terminates the process immediately.
func notifySignals(logger *slog.Logger) (context.Context, func()) {
	ctx, cancel := context.WithCancel(context.Background())

	signals := make(chan os.Signal, 2)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)

	go func() {
		received := <-signals
		logger.Info("received signal, shutting down", "signal", received.String())

		go func() {
			forced := <-signals
			logger.Error("received second signal, forcing exit", "signal", forced.String())
			os.Exit(1)
		}()

		// A second signal may already be on its way: wait briefly for it
		// before starting the graceful shutdown, since the shutdown itself
		// only takes as long as the tasks need.
		select {
		case forced := <-signals:
			logger.Error("received second signal, forcing exit", "signal", forced.String())
			os.Exit(1)
		case <-time.After(secondSignalGrace):
		}

		cancel()
	}()

	stop := func() {
		signal.Stop(signals)
		cancel()
	}

	return ctx, stop
}
