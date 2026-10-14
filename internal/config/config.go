// Package config loads server configuration from environment variables,
// including the shared lab key and the accepted machine sources.
package config

import (
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"time"
)

const (
	// defaultSessionTTL is the default lifetime of an issued session token.
	defaultSessionTTL = "24h"

	// envLabKey is the environment variable holding the shared lab key.
	envLabKey = "AEGIS_LAB_KEY"

	// envSessionSecret is the environment variable holding the HMAC signing
	// secret used for session tokens.
	envSessionSecret = "AEGIS_SESSION_SECRET"

	// envSessionTTL is the environment variable overriding the default session
	// token lifetime.
	envSessionTTL = "AEGIS_SESSION_TTL"

	// envMachines is the environment variable listing the accepted machine
	// sources as comma-separated ip=name pairs.
	envMachines = "AEGIS_MACHINES"

	// labKeyLength is the required length, in bytes, of the shared lab key.
	labKeyLength = 32
)

// Machine declares a single accepted machine source.
type Machine struct {
	// IP is the fixed address identifying the machine.
	IP string

	// Name is the human-friendly label following the lccX-YYY pattern.
	Name string
}

// Config holds the machine-related server configuration.
type Config struct {
	// LabKey is the shared HMAC key used to verify machine authentication
	// challenges.
	LabKey []byte

	// SessionSecret is the HMAC signing secret used for session tokens.
	SessionSecret string

	// SessionTTL is the lifetime of an issued session token.
	SessionTTL time.Duration

	// Machines is the list of accepted machine sources.
	Machines []Machine
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

	machines, err := parseMachines(os.Getenv(envMachines))
	if err != nil {
		return nil, err
	}

	if len(machines) == 0 {
		log.Printf("warning: no machines configured; all authentication attempts will fail")
	}

	return &Config{
		LabKey:        labKey,
		SessionSecret: sessionSecret,
		SessionTTL:    sessionTTL,
		Machines:      machines,
	}, nil
}

// parseLabKey decodes the shared lab key from its hexadecimal representation,
// requiring exactly 32 bytes.
func parseLabKey(raw string) ([]byte, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, fmt.Errorf("%s is required", envLabKey)
	}

	key, err := hex.DecodeString(trimmed)
	if err != nil {
		return nil, fmt.Errorf("invalid %s value %q: must be hex-encoded", envLabKey, trimmed)
	}

	if len(key) != labKeyLength {
		return nil, fmt.Errorf("invalid %s value: must decode to exactly %d bytes", envLabKey, labKeyLength)
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

// parseMachines parses the comma-separated list of ip=name machine entries.
func parseMachines(raw string) ([]Machine, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, nil
	}

	var machines []Machine

	for entry := range strings.SplitSeq(trimmed, ",") {
		parts := strings.Split(strings.TrimSpace(entry), "=")
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid %s entry %q: expected ip=name", envMachines, entry)
		}

		ip := strings.TrimSpace(parts[0])
		name := strings.TrimSpace(parts[1])

		if net.ParseIP(ip) == nil {
			return nil, fmt.Errorf("invalid %s entry %q: %q is not a valid IP address", envMachines, entry, ip)
		}

		if name == "" {
			return nil, fmt.Errorf("invalid %s entry %q: empty name", envMachines, entry)
		}

		machines = append(machines, Machine{IP: ip, Name: name})
	}

	return machines, nil
}
