// Command sentinel runs the Aegis Sentinel daemon, the per-endpoint agent
// that authenticates with the Orchestrator and enforces the policies it
// distributes.
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"dougdomingos.com/aegis/internal/sentinel/client"
	"dougdomingos.com/aegis/internal/sentinel/config"
	"dougdomingos.com/aegis/internal/sentinel/daemon"
	"dougdomingos.com/aegis/internal/sentinel/heartbeat"
	"dougdomingos.com/aegis/internal/sentinel/identity"
	"dougdomingos.com/aegis/internal/sentinel/logging"
)

// secondSignalGrace is the grace window given to a second termination signal
// before the graceful shutdown starts, so that a pair of signals sent in
// quick succession still forces the exit.
const secondSignalGrace = 50 * time.Millisecond

// defaultConfigPath is the location the Sentinel reads its configuration
// from when the -config flag is not provided.
const defaultConfigPath = "/etc/aegis/sentinel.json"

func main() {
	os.Exit(run())
}

// run starts the Sentinel daemon and returns the process exit code.
func run() int {
	bootstrap := logging.New(os.Stderr, logging.Config{Level: slog.LevelInfo, Format: logging.FormatText})

	configPath := flag.String("config", defaultConfigPath, "path to the Sentinel configuration file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		bootstrap.Error("failed to load configuration", "error", err)

		return 1
	}

	logger := logging.New(os.Stderr, logging.Config{Level: cfg.LogLevel, Format: cfg.LogFormat})

	ctx, stop := notifySignals(logger)
	defer stop()

	detector := identity.NewDetector()
	orchestrator := client.New(cfg.OrchestratorURL, cfg.LabKey, cfg.RequestTimeout, logger)
	heartbeats := heartbeat.New(cfg.HeartbeatInterval, orchestrator, orchestrator, logger)

	logger.Info("configuration loaded",
		"orchestrator_url", cfg.OrchestratorURL.String(),
		"request_timeout", cfg.RequestTimeout,
		"heartbeat_interval", cfg.HeartbeatInterval)

	if err := daemon.New(logger, detector, orchestrator, daemon.Task{
		Name: "heartbeat",
		Run:  heartbeats.Run,
	}).Run(ctx); err != nil {
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
