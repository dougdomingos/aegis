package main

import (
	"context"
	"errors"
	"log"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"dougdomingos.com/aegis/internal/orchestrator/api"
	"dougdomingos.com/aegis/internal/orchestrator/api/grpcserver"
	"dougdomingos.com/aegis/internal/orchestrator/config"
	"dougdomingos.com/aegis/internal/orchestrator/infra"
	"dougdomingos.com/aegis/pkg/pb"
	"google.golang.org/grpc"
)

const (
	// envServerAddress is the environment variable overriding the address the
	// API server binds to.
	envServerAddress = "AEGIS_ADDR"

	// envDatabasePath is the environment variable holding the path of the
	// SQLite database file.
	envDatabasePath = "AEGIS_DB"

	// defaultServerAddress is the address the API server binds to when the
	// environment does not override it.
	defaultServerAddress = ":8080"

	// defaultDatabasePath is the location of the SQLite database file when the
	// environment does not override it.
	defaultDatabasePath = "/tmp/aegis.db"

	// maxMessageSize is the gRPC send/receive limit shared by the propagation
	// server and the Sentinel clients.
	maxMessageSize = 16 << 20

	shutdownTimeout = 5 * time.Second
)

// envOrDefault reads the provided environment variable, falling back to the
// provided default when it is unset or blank.
func envOrDefault(name, fallback string) string {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}

	return value
}

// main starts the Orchestrator admin API and propagation servers.
//
//	@title Aegis - Orchestrator Admin API
//	@version v0.1.0
//	@servers.url http://localhost:8080
func main() {
	serverAddress := envOrDefault(envServerAddress, defaultServerAddress)
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	db, err := infra.InitDB(envOrDefault(envDatabasePath, defaultDatabasePath))
	if err != nil {
		log.Fatalf("failed to initialize database: %v", err)
	}
	defer db.Close()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load configuration: %v", err)
	}

	runtime, err := api.NewRuntime(db, cfg, logger)
	if err != nil {
		log.Fatalf("failed to initialize runtime: %v", err)
	}

	srv := &http.Server{
		Addr:              serverAddress,
		Handler:           runtime.HTTP,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	propagationCtx, cancelPropagation := context.WithCancel(context.Background())
	defer cancelPropagation()

	if err := runtime.Propagation.Start(propagationCtx); err != nil {
		log.Fatalf("failed to start propagation: %v", err)
	}

	grpcServer := grpc.NewServer(
		grpc.MaxRecvMsgSize(maxMessageSize),
		grpc.MaxSendMsgSize(maxMessageSize),
	)
	pb.RegisterPropagationServer(grpcServer, grpcserver.NewPropagationServer(runtime.Propagation, cfg.SessionSecret, logger))

	grpcListener, err := net.Listen("tcp", cfg.GRPCAddr)
	if err != nil {
		log.Fatalf("failed to listen on gRPC address %s: %v", cfg.GRPCAddr, err)
	}

	errCh := make(chan error, 2)
	go func() {
		log.Printf("starting server on port %s...", serverAddress)
		errCh <- srv.ListenAndServe()
	}()
	go func() {
		log.Printf("starting gRPC propagation server on %s...", cfg.GRPCAddr)
		errCh <- grpcServer.Serve(grpcListener)
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("application failed: %v", err)
		}
	case sig := <-sigCh:
		log.Printf("received signal %v, shutting down...", sig)

		go func() {
			<-sigCh // second signal forces immediate exit
			log.Fatal("forced shutdown by second signal")
		}()

		cancelPropagation()

		stopped := make(chan struct{})
		go func() {
			grpcServer.GracefulStop()
			close(stopped)
		}()

		select {
		case <-stopped:
		case <-time.After(shutdownTimeout):
			log.Printf("gRPC server did not drain within %v, forcing stop", shutdownTimeout)
			grpcServer.Stop()
		}

		ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()

		if err := srv.Shutdown(ctx); err != nil {
			log.Printf("server shutdown failed: %v", err)
		}
	}
}
