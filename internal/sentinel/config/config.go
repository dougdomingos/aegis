// Package config loads and validates the Sentinel daemon configuration.
package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"dougdomingos.com/aegis/internal/common/auth"
	"dougdomingos.com/aegis/internal/sentinel/logging"
)

const (
	// defaultRequestTimeout is the default request timeout used when the
	// configuration does not override it.
	defaultRequestTimeout = 10 * time.Second

	// defaultLogLevel is the default log level name used when the
	// configuration does not override it.
	defaultLogLevel = "info"

	// defaultLogFormat is the default log format name used when the
	// configuration does not override it.
	defaultLogFormat = "text"

	// defaultHeartbeatInterval is the default period between connectivity
	// heartbeats when the configuration does not override it.
	defaultHeartbeatInterval = 30 * time.Second

	// defaultPropagationPort is the port combined with the Orchestrator host
	// when propagation_address is unset.
	defaultPropagationPort = "9090"

	// defaultRulesStatePath is the durable state file of the propagation
	// runner when the configuration does not override it.
	defaultRulesStatePath = "/var/lib/aegis/rules.json"

	// maxHeartbeatInterval mirrors the Orchestrator's machine staleness grace
	// (90s): an interval equal to or longer than it would leave a machine
	// displayed as DISCONNECTED between heartbeats that always succeed.
	maxHeartbeatInterval = 90 * time.Second
)

// Config holds the Sentinel daemon configuration.
type Config struct {
	// OrchestratorURL is the base address of the Orchestrator API.
	OrchestratorURL *url.URL

	// LabKey is the shared HMAC key used to authenticate against the
	// Orchestrator.
	LabKey []byte

	// RequestTimeout bounds each request made to the Orchestrator.
	RequestTimeout time.Duration

	// HeartbeatInterval is the period between connectivity heartbeats sent
	// to the Orchestrator.
	HeartbeatInterval time.Duration

	// LogLevel is the minimum severity of the emitted log records.
	LogLevel slog.Level

	// LogFormat is the output format of the emitted log records.
	LogFormat logging.Format

	// PropagationAddress is the host:port of the Orchestrator's gRPC
	// propagation server.
	PropagationAddress string

	// RulesStatePath is the durable state file where the Sentinel stores the
	// last applied propagation.
	RulesStatePath string
}

// document mirrors the JSON configuration file schema.
type document struct {
	OrchestratorURL    string `json:"orchestrator_url"`
	LabKeyPath         string `json:"lab_key_path"`
	RequestTimeout     string `json:"request_timeout"`
	HeartbeatInterval  string `json:"heartbeat_interval"`
	LogLevel           string `json:"log_level"`
	LogFormat          string `json:"log_format"`
	PropagationAddress string `json:"propagation_address"`
	RulesStatePath     string `json:"rules_state_path"`
}

// Load reads and validates the Sentinel daemon configuration from the file
// at the provided path, returning the first validation error it meets.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read configuration file: %w", err)
	}

	var doc document
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&doc); err != nil {
		return nil, fmt.Errorf("failed to parse configuration file: %w", err)
	}

	orchestratorURL, err := parseOrchestratorURL(doc.OrchestratorURL)
	if err != nil {
		return nil, err
	}

	labKey, err := parseLabKey(doc.LabKeyPath)
	if err != nil {
		return nil, err
	}

	requestTimeout, err := parseRequestTimeout(doc.RequestTimeout)
	if err != nil {
		return nil, err
	}

	heartbeatInterval, err := parseHeartbeatInterval(doc.HeartbeatInterval)
	if err != nil {
		return nil, err
	}

	logLevel, err := parseLogLevel(doc.LogLevel)
	if err != nil {
		return nil, err
	}

	logFormat, err := parseLogFormat(doc.LogFormat)
	if err != nil {
		return nil, err
	}

	propagationAddress, err := parsePropagationAddress(doc.PropagationAddress, orchestratorURL)
	if err != nil {
		return nil, err
	}

	rulesStatePath, err := parseRulesStatePath(doc.RulesStatePath)
	if err != nil {
		return nil, err
	}

	return &Config{
		OrchestratorURL:    orchestratorURL,
		LabKey:             labKey,
		RequestTimeout:     requestTimeout,
		HeartbeatInterval:  heartbeatInterval,
		LogLevel:           logLevel,
		LogFormat:          logFormat,
		PropagationAddress: propagationAddress,
		RulesStatePath:     rulesStatePath,
	}, nil
}

// parseOrchestratorURL parses the Orchestrator base address, requiring an
// http or https scheme and a non-empty host.
func parseOrchestratorURL(raw string) (*url.URL, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, fmt.Errorf("orchestrator_url is required")
	}

	parsed, err := url.Parse(trimmed)
	if err != nil {
		return nil, fmt.Errorf("invalid orchestrator_url: %w", err)
	}

	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("orchestrator_url must use the http or https scheme")
	}

	if parsed.Host == "" {
		return nil, fmt.Errorf("orchestrator_url must include a host")
	}

	return parsed, nil
}

// parseLabKey reads the shared lab key from the file at the provided path.
func parseLabKey(path string) ([]byte, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return nil, fmt.Errorf("lab_key_path is required")
	}

	key, err := auth.ReadLabKey(trimmed)
	if err != nil {
		return nil, fmt.Errorf("invalid lab_key_path: %w", err)
	}

	return key, nil
}

// parseRequestTimeout parses the request timeout, falling back to the default
// when it is unset.
func parseRequestTimeout(raw string) (time.Duration, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return defaultRequestTimeout, nil
	}

	timeout, err := time.ParseDuration(trimmed)
	if err != nil {
		return 0, fmt.Errorf("invalid request_timeout: %w", err)
	}

	if timeout <= 0 {
		return 0, fmt.Errorf("invalid request_timeout: must be positive")
	}

	return timeout, nil
}

// parseLogLevel parses the log level name, falling back to the default when
// it is unset.
func parseLogLevel(raw string) (slog.Level, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		trimmed = defaultLogLevel
	}

	level, err := logging.ParseLevel(trimmed)
	if err != nil {
		return 0, fmt.Errorf("invalid log_level: %w", err)
	}

	return level, nil
}

// parseLogFormat parses the log format name, falling back to the default when
// it is unset.
func parseLogFormat(raw string) (logging.Format, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		trimmed = defaultLogFormat
	}

	format, err := logging.ParseFormat(trimmed)
	if err != nil {
		return "", fmt.Errorf("invalid log_format: %w", err)
	}

	return format, nil
}

// parseHeartbeatInterval parses the heartbeat interval, falling back to the
// default when it is unset.
func parseHeartbeatInterval(raw string) (time.Duration, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return defaultHeartbeatInterval, nil
	}

	interval, err := time.ParseDuration(trimmed)
	if err != nil {
		return 0, fmt.Errorf("invalid heartbeat_interval: %w", err)
	}

	if interval <= 0 {
		return 0, fmt.Errorf("invalid heartbeat_interval: must be positive")
	}

	if interval >= maxHeartbeatInterval {
		return 0, fmt.Errorf("invalid heartbeat_interval: must be shorter than %v, the Orchestrator staleness grace", maxHeartbeatInterval)
	}

	return interval, nil
}

// parsePropagationAddress resolves the gRPC propagation address, defaulting to
// the Orchestrator host with the default propagation port. An explicit value
// must be a host:port pair with a non-empty host and a port within
// [1, 65535].
func parsePropagationAddress(raw string, orchestratorURL *url.URL) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return net.JoinHostPort(orchestratorURL.Hostname(), defaultPropagationPort), nil
	}

	host, port, err := net.SplitHostPort(value)
	if err != nil {
		return "", fmt.Errorf("invalid propagation_address %q: %w", value, err)
	}

	if host == "" {
		return "", fmt.Errorf("invalid propagation_address %q: host must not be empty", value)
	}

	number, err := strconv.Atoi(port)
	if err != nil {
		return "", fmt.Errorf("invalid propagation_address %q: port must be a number", value)
	}

	if number < 1 || number > 65535 {
		return "", fmt.Errorf("invalid propagation_address %q: port must be between 1 and 65535", value)
	}

	return value, nil
}

// parseRulesStatePath resolves the durable propagation state path, falling back
// to the default when unset.
func parseRulesStatePath(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return defaultRulesStatePath, nil
	}

	return value, nil
}
