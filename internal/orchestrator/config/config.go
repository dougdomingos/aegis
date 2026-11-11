// Package config loads server configuration from environment variables,
// including the shared lab key and the session settings.
package config

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"dougdomingos.com/aegis/internal/common/auth"
)

const (
	// defaultSessionTTL is the default lifetime of an issued session token.
	defaultSessionTTL = "24h"

	// envLabKey is the environment variable holding the path to the file
	// containing the shared lab key.
	envLabKey = "AEGIS_LAB_KEY"

	// envSessionSecret is the environment variable holding the HMAC signing
	// secret used for session tokens.
	envSessionSecret = "AEGIS_SESSION_SECRET"

	// envSessionTTL is the environment variable overriding the default session
	// token lifetime.
	envSessionTTL = "AEGIS_SESSION_TTL"

	// envStalenessGrace is the environment variable overriding the default
	// machine staleness grace window.
	envStalenessGrace = "AEGIS_STALENESS_GRACE"

	// envGRPCAddr is the environment variable holding the address the gRPC
	// propagation server binds to.
	envGRPCAddr = "AEGIS_GRPC_ADDR"

	// defaultGRPCAddr is the address the gRPC propagation server binds to when
	// the environment does not override it.
	defaultGRPCAddr = ":9090"

	// defaultStalenessGrace is the default duration a machine remains displayed
	// as CONNECTED after its last heartbeat, covering three consecutive beats of
	// the default Sentinel heartbeat interval.
	defaultStalenessGrace = "90s"
)

// Config holds the machine-related server configuration.
type Config struct {
	// LabKey is the shared HMAC key used to verify machine authentication
	// challenges.
	LabKey []byte

	// SessionSecret is the HMAC signing secret used for session tokens.
	SessionSecret string

	// SessionTTL is the lifetime of an issued session token.
	SessionTTL time.Duration

	// StalenessGrace is the duration a machine remains displayed as CONNECTED
	// after its last heartbeat.
	StalenessGrace time.Duration

	// GRPCAddr is the address the gRPC propagation server binds to.
	GRPCAddr string
}

// Load reads the machine-related configuration from environment variables and
// returns a validated Config instance.
func Load() (*Config, error) {
	labKey, err := parseLabKey(os.Getenv(envLabKey))
	if err != nil {
		return nil, err
	}

	sessionSecret := strings.TrimSpace(os.Getenv(envSessionSecret))
	if sessionSecret == "" {
		return nil, fmt.Errorf("%s is required", envSessionSecret)
	}

	sessionTTL, err := parseSessionTTL(os.Getenv(envSessionTTL))
	if err != nil {
		return nil, err
	}

	stalenessGrace, err := parseStalenessGrace(os.Getenv(envStalenessGrace))
	if err != nil {
		return nil, err
	}

	grpcAddr, err := parseGRPCAddr(os.Getenv(envGRPCAddr))
	if err != nil {
		return nil, err
	}

	return &Config{
		LabKey:         labKey,
		SessionSecret:  sessionSecret,
		SessionTTL:     sessionTTL,
		StalenessGrace: stalenessGrace,
		GRPCAddr:       grpcAddr,
	}, nil
}

// parseLabKey reads the shared lab key from the file at the provided path,
// requiring a hex-encoded value that decodes to exactly auth.LabKeyLength
// bytes.
func parseLabKey(path string) ([]byte, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return nil, fmt.Errorf("%s is required", envLabKey)
	}

	key, err := auth.ReadLabKey(trimmed)
	if err != nil {
		return nil, fmt.Errorf("invalid %s: %w", envLabKey, err)
	}

	return key, nil
}

// parseSessionTTL parses the session TTL environment variable, falling back to
// the default when it is unset.
func parseSessionTTL(raw string) (time.Duration, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		value = defaultSessionTTL
	}

	ttl, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("invalid %s value %q: %w", envSessionTTL, value, err)
	}

	if ttl <= 0 {
		return 0, fmt.Errorf("invalid %s value %q: must be positive", envSessionTTL, value)
	}

	return ttl, nil
}

// parseStalenessGrace parses the staleness grace environment variable, falling
// back to the default when it is unset.
func parseStalenessGrace(raw string) (time.Duration, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		value = defaultStalenessGrace
	}

	grace, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("invalid %s value %q: %w", envStalenessGrace, value, err)
	}

	if grace <= 0 {
		return 0, fmt.Errorf("invalid %s value %q: must be positive", envStalenessGrace, value)
	}

	return grace, nil
}

// parseGRPCAddr parses the gRPC bind address environment variable, falling
// back to the default when it is unset. The address must be a valid host:port
// pair with a non-empty port within [1, 65535].
func parseGRPCAddr(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return defaultGRPCAddr, nil
	}

	_, port, err := net.SplitHostPort(value)
	if err != nil {
		return "", fmt.Errorf("invalid %s value %q: %w", envGRPCAddr, value, err)
	}

	if port == "" {
		return "", fmt.Errorf("invalid %s value %q: port must not be empty", envGRPCAddr, value)
	}

	number, err := strconv.Atoi(port)
	if err != nil {
		return "", fmt.Errorf("invalid %s value %q: port must be a number", envGRPCAddr, value)
	}

	if number < 1 || number > 65535 {
		return "", fmt.Errorf("invalid %s value %q: port must be between 1 and 65535", envGRPCAddr, value)
	}

	return value, nil
}
