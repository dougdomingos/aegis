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

	// defaultFilteringEnabled is whether local DNS filtering is enabled when the
	// configuration does not override it.
	defaultFilteringEnabled = true

	// defaultDNSListenAddress is the host:port the embedded DNS server binds to
	// when the configuration does not override it.
	defaultDNSListenAddress = "127.0.0.1:53"

	// defaultNftBinaryPath is the nft executable resolved from PATH when the
	// configuration does not override it.
	defaultNftBinaryPath = "nft"
)

// ResolvConfPath is the resolver configuration file whose nameserver entries
// become the default DNS upstream servers. It is a variable so tests can point
// it at a fixture.
var ResolvConfPath = "/etc/resolv.conf"

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

	// FilteringEnabled reports whether local DNS filtering is enabled.
	FilteringEnabled bool

	// DNSListenAddress is the host:port the embedded DNS server binds to.
	DNSListenAddress string

	// DNSUpstreamServers are the IP addresses the embedded DNS server forwards
	// unfiltered queries to.
	DNSUpstreamServers []string

	// NftBinaryPath is the nft executable used to apply the filtering rulesets.
	NftBinaryPath string
}

// document mirrors the JSON configuration file schema.
type document struct {
	OrchestratorURL    string   `json:"orchestrator_url"`
	LabKeyPath         string   `json:"lab_key_path"`
	RequestTimeout     string   `json:"request_timeout"`
	HeartbeatInterval  string   `json:"heartbeat_interval"`
	LogLevel           string   `json:"log_level"`
	LogFormat          string   `json:"log_format"`
	PropagationAddress string   `json:"propagation_address"`
	RulesStatePath     string   `json:"rules_state_path"`
	FilteringEnabled   string   `json:"filtering_enabled"`
	DNSListenAddress   string   `json:"dns_listen_address"`
	DNSUpstreamServers []string `json:"dns_upstream_servers"`
	NftBinaryPath      string   `json:"nft_binary_path"`
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

	filteringEnabled, err := parseFilteringEnabled(doc.FilteringEnabled)
	if err != nil {
		return nil, err
	}

	dnsListenAddress, err := parseDNSListenAddress(doc.DNSListenAddress)
	if err != nil {
		return nil, err
	}

	dnsUpstreamServers, err := parseDNSUpstreamServers(doc.DNSUpstreamServers, filteringEnabled)
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
		FilteringEnabled:   filteringEnabled,
		DNSListenAddress:   dnsListenAddress,
		DNSUpstreamServers: dnsUpstreamServers,
		NftBinaryPath:      nftBinaryPath(doc.NftBinaryPath),
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

// parseFilteringEnabled parses the local filtering switch, falling back to the
// default when it is unset.
func parseFilteringEnabled(raw string) (bool, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return defaultFilteringEnabled, nil
	}

	enabled, err := strconv.ParseBool(trimmed)
	if err != nil {
		return false, fmt.Errorf("invalid filtering_enabled: %w", err)
	}

	return enabled, nil
}

// parseDNSListenAddress parses the embedded DNS listen address, falling back to
// the default when it is unset. An explicit value must be a host:port pair with
// a port within [1, 65535].
func parseDNSListenAddress(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return defaultDNSListenAddress, nil
	}

	_, port, err := net.SplitHostPort(value)
	if err != nil {
		return "", fmt.Errorf("invalid dns_listen_address %q: %w", value, err)
	}

	number, err := strconv.Atoi(port)
	if err != nil {
		return "", fmt.Errorf("invalid dns_listen_address %q: port must be a number", value)
	}

	if number < 1 || number > 65535 {
		return "", fmt.Errorf("invalid dns_listen_address %q: port must be between 1 and 65535", value)
	}

	return value, nil
}

// parseDNSUpstreamServers validates the explicit upstream servers, which must
// each be an IP address. When none are provided and filtering is enabled, the
// nameserver entries of ResolvConfPath are used instead, and it is an error for
// them to be absent as well.
func parseDNSUpstreamServers(raw []string, filteringEnabled bool) ([]string, error) {
	if len(raw) == 0 {
		if !filteringEnabled {
			return nil, nil
		}

		servers, err := readResolvNameservers(ResolvConfPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read default dns_upstream_servers: %w", err)
		}

		if len(servers) == 0 {
			return nil, fmt.Errorf("dns_upstream_servers is required: no non-loopback nameserver entries found in %s", ResolvConfPath)
		}

		return servers, nil
	}

	servers := make([]string, 0, len(raw))
	for _, entry := range raw {
		value := strings.TrimSpace(entry)
		if net.ParseIP(value) == nil {
			return nil, fmt.Errorf("invalid dns_upstream_servers entry %q: must be an IP address", entry)
		}

		servers = append(servers, value)
	}

	return servers, nil
}

// readResolvNameservers returns the IP addresses listed in the nameserver
// entries of the resolver configuration file at the provided path, skipping
// entries that are not valid IP addresses and loopback addresses, which would
// make the embedded resolver forward queries to itself.
func readResolvNameservers(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var servers []string
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || fields[0] != "nameserver" {
			continue
		}

		ip := net.ParseIP(fields[1])
		if ip == nil || ip.IsLoopback() {
			continue
		}

		servers = append(servers, fields[1])
	}

	return servers, nil
}

// nftBinaryPath resolves the nft executable path, falling back to the default
// when it is unset.
func nftBinaryPath(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return defaultNftBinaryPath
	}

	return value
}
