// Package config loads server configuration from environment variables,
// including the shared lab key and the accepted machine sources.
package config

import (
	"encoding/csv"
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

	// envLabKey is the environment variable holding the path to the file
	// containing the shared lab key.
	envLabKey = "AEGIS_LAB_KEY"

	// envSessionSecret is the environment variable holding the HMAC signing
	// secret used for session tokens.
	envSessionSecret = "AEGIS_SESSION_SECRET"

	// envSessionTTL is the environment variable overriding the default session
	// token lifetime.
	envSessionTTL = "AEGIS_SESSION_TTL"

	// envMachines is the environment variable holding the path to the CSV file
	// listing the accepted machine sources.
	envMachines = "AEGIS_MACHINES"

	// labKeyLength is the required length, in bytes, of the shared lab key.
	labKeyLength = 32

	// machinesHeader is the expected header row of the machines CSV file.
	machinesHeader = "ip,name"
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

// parseLabKey reads the shared lab key from the file at the provided path,
// requiring a hex-encoded value that decodes to exactly 32 bytes.
func parseLabKey(path string) ([]byte, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return nil, fmt.Errorf("%s is required", envLabKey)
	}

	data, err := os.ReadFile(trimmed)
	if err != nil {
		return nil, fmt.Errorf("invalid %s: failed to read key file: %w", envLabKey, err)
	}

	key, err := hex.DecodeString(strings.TrimSpace(string(data)))
	if err != nil {
		return nil, fmt.Errorf("invalid %s: key file must be hex-encoded: %w", envLabKey, err)
	}

	if len(key) != labKeyLength {
		return nil, fmt.Errorf("invalid %s: key file must decode to exactly %d bytes", envLabKey, labKeyLength)
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

// parseMachines reads the accepted machine sources from the CSV file at the
// provided path. The file must have an "ip,name" header row followed by one
// machine per line.
func parseMachines(path string) ([]Machine, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return nil, nil
	}

	file, err := os.Open(trimmed)
	if err != nil {
		return nil, fmt.Errorf("invalid %s: failed to open machines file: %w", envMachines, err)
	}
	defer file.Close()

	records, err := csv.NewReader(file).ReadAll()
	if err != nil {
		return nil, fmt.Errorf("invalid %s: failed to parse machines file: %w", envMachines, err)
	}

	if len(records) == 0 {
		return nil, fmt.Errorf("invalid %s: machines file is empty", envMachines)
	}

	header := strings.Join(records[0], ",")
	if header != machinesHeader {
		return nil, fmt.Errorf("invalid %s: header must be %q", envMachines, machinesHeader)
	}

	var machines []Machine

	for _, record := range records[1:] {
		if len(record) != 2 {
			return nil, fmt.Errorf("invalid %s: each row must have 2 fields", envMachines)
		}

		ip := strings.TrimSpace(record[0])
		name := strings.TrimSpace(record[1])

		if net.ParseIP(ip) == nil {
			return nil, fmt.Errorf("invalid %s: %q is not a valid IP address", envMachines, ip)
		}

		if name == "" {
			return nil, fmt.Errorf("invalid %s: empty name", envMachines)
		}

		machines = append(machines, Machine{IP: ip, Name: name})
	}

	return machines, nil
}
