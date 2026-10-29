package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"dougdomingos.com/aegis/internal/orchestrator/api"
	"dougdomingos.com/aegis/internal/orchestrator/config"
	"dougdomingos.com/aegis/internal/orchestrator/infra"
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

func main() {
	serverAddress := envOrDefault(envServerAddress, defaultServerAddress)

	db, err := infra.InitDB(envOrDefault(envDatabasePath, defaultDatabasePath))
	if err != nil {
		log.Fatalf("failed to initialize database: %v", err)
	}
	defer db.Close()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load configuration: %v", err)
	}

	router, err := api.NewRouter(db, cfg)
	if err != nil {
		log.Fatalf("failed to initialize router: %v", err)
	}

	srv := &http.Server{
		Addr:              serverAddress,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Printf("starting server on port %s...", serverAddress)
		errCh <- srv.ListenAndServe()
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

		ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()

		if err := srv.Shutdown(ctx); err != nil {
			log.Printf("server shutdown failed: %v", err)
		}
	}
}
