// Command sentinel runs the Aegis Sentinel daemon, the per-endpoint agent
// that authenticates with the Orchestrator and enforces the policies it
// distributes.
package main

import (
	"context"
	"flag"
	"log/slog"
	"net"
	"net/url"
	"os"
	"os/signal"
	"slices"
	"syscall"
	"time"

	"dougdomingos.com/aegis/internal/sentinel/client"
	"dougdomingos.com/aegis/internal/sentinel/config"
	"dougdomingos.com/aegis/internal/sentinel/daemon"
	"dougdomingos.com/aegis/internal/sentinel/filtering"
	"dougdomingos.com/aegis/internal/sentinel/filtering/dnsserver"
	"dougdomingos.com/aegis/internal/sentinel/filtering/nftables"
	"dougdomingos.com/aegis/internal/sentinel/heartbeat"
	"dougdomingos.com/aegis/internal/sentinel/identity"
	"dougdomingos.com/aegis/internal/sentinel/logging"
	sentinelPropagation "dougdomingos.com/aegis/internal/sentinel/propagation"
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
	orchestrator, err := client.New(cfg.OrchestratorURL, cfg.PropagationAddress, cfg.LabKey, cfg.RequestTimeout, logger)
	if err != nil {
		logger.Error("failed to initialize client", "error", err)

		return 1
	}

	defer orchestrator.Close()

	stateStore, err := sentinelPropagation.NewFileStore(cfg.RulesStatePath)
	if err != nil {
		logger.Error("failed to initialize propagation state store", "error", err)

		return 1
	}

	// The default logger must be configured before the DNS server is built so
	// that its callback failures use the configured logger.
	slog.SetDefault(logger)

	var (
		filter     filtering.Filter
		essentials sentinelPropagation.Essentials
		tasks      []daemon.Task
	)

	if cfg.FilteringEnabled {
		executor := nftables.NewExecutor(cfg.NftBinaryPath)
		server := dnsserver.NewServer(cfg.DNSListenAddress, cfg.DNSUpstreamServers, executor.AddResolvedIP)
		filter = filtering.NewComposite(executor, server)

		// The Orchestrator addresses are resolved once at startup; a later
		// address change requires restarting the Sentinel.
		essentials = sentinelPropagation.Essentials{
			OrchestratorIPs: resolveOrchestratorIPs(cfg.OrchestratorURL, cfg.PropagationAddress, logger),
			UpstreamDNSIPs:  cfg.DNSUpstreamServers,
		}

		tasks = append(tasks, daemon.Task{
			Name: "dns",
			Run: func(ctx context.Context, _ *daemon.Session) error {
				return server.Start(ctx)
			},
		})
	}

	propagationRunner := sentinelPropagation.New(stateStore, orchestrator, orchestrator, filter, essentials, logger)

	heartbeats := heartbeat.New(cfg.HeartbeatInterval, orchestrator, orchestrator, logger)

	logger.Info("configuration loaded",
		"orchestrator_url", cfg.OrchestratorURL.String(),
		"propagation_address", cfg.PropagationAddress,
		"rules_state_path", cfg.RulesStatePath,
		"request_timeout", cfg.RequestTimeout,
		"heartbeat_interval", cfg.HeartbeatInterval,
		"filtering_enabled", cfg.FilteringEnabled,
		"dns_listen_address", cfg.DNSListenAddress,
		"dns_upstream_servers", cfg.DNSUpstreamServers,
		"nft_binary_path", cfg.NftBinaryPath)

	tasks = append(tasks,
		daemon.Task{Name: "heartbeat", Run: heartbeats.Run},
		daemon.Task{Name: "propagation", Run: propagationRunner.Run},
	)

	if err := daemon.New(logger, detector, orchestrator, tasks...).Run(ctx); err != nil {
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

// resolveOrchestratorIPs returns the deduplicated IP addresses of the
// Orchestrator REST and propagation hosts. Hosts that fail to resolve are
// logged and skipped.
func resolveOrchestratorIPs(restURL *url.URL, propagationAddress string, logger *slog.Logger) []string {
	hosts := []string{restURL.Hostname()}
	if host, _, err := net.SplitHostPort(propagationAddress); err == nil {
		hosts = append(hosts, host)
	} else {
		hosts = append(hosts, propagationAddress)
	}

	var ips []string

	for _, host := range hosts {
		if host == "" {
			continue
		}

		addrs, err := net.LookupHost(host)
		if err != nil {
			logger.Warn("failed to resolve orchestrator host", "host", host, "error", err)

			continue
		}

		for _, addr := range addrs {
			if !slices.Contains(ips, addr) {
				ips = append(ips, addr)
			}
		}
	}

	return ips
}
