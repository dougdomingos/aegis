package config_test

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"log/slog"

	"dougdomingos.com/aegis/internal/sentinel/config"
	"dougdomingos.com/aegis/internal/sentinel/logging"
)

// validLabKey is a 32-byte key encoded as 64 uppercase hexadecimal
// characters, to pin that uppercase hex and a trailing newline are accepted.
const validLabKey = "0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF"

// labKeyFile is the contents of a valid lab key file.
const labKeyFile = validLabKey + "\n"

func TestConfig_Load_WithValidFile_ReturnsParsedConfig(t *testing.T) {
	keyPath := writeTempFile(t, labKeyFile)
	document := `{"orchestrator_url": "http://10.0.0.5:8080", "lab_key_path": "` + keyPath + `"}`

	cfg, err := config.Load(writeTempConfig(t, document))
	if err != nil {
		t.Fatalf("config.Load() error = %v, want nil", err)
	}

	if cfg.OrchestratorURL.String() != "http://10.0.0.5:8080" {
		t.Errorf("cfg.OrchestratorURL = %q, want http://10.0.0.5:8080", cfg.OrchestratorURL.String())
	}

	if len(cfg.LabKey) != 32 {
		t.Fatalf("len(cfg.LabKey) = %d, want 32", len(cfg.LabKey))
	}

	expectedKey := decodeHex(t, validLabKey)
	if string(cfg.LabKey) != string(expectedKey) {
		t.Errorf("cfg.LabKey = %x, want %x", cfg.LabKey, expectedKey)
	}

	if cfg.RequestTimeout != 10*time.Second {
		t.Errorf("cfg.RequestTimeout = %v, want 10s", cfg.RequestTimeout)
	}

	if cfg.LogLevel != slog.LevelInfo {
		t.Errorf("cfg.LogLevel = %v, want info", cfg.LogLevel)
	}

	if cfg.LogFormat != logging.FormatText {
		t.Errorf("cfg.LogFormat = %q, want text", cfg.LogFormat)
	}
}

func TestConfig_Load_WithOptionalFields_ReturnsConfiguredValues(t *testing.T) {
	keyPath := writeTempFile(t, labKeyFile)
	document := `{
		"orchestrator_url": "http://10.0.0.5:8080",
		"lab_key_path": "` + keyPath + `",
		"request_timeout": "5s",
		"log_level": "debug",
		"log_format": "json"
	}`

	cfg, err := config.Load(writeTempConfig(t, document))
	if err != nil {
		t.Fatalf("config.Load() error = %v, want nil", err)
	}

	if cfg.RequestTimeout != 5*time.Second {
		t.Errorf("cfg.RequestTimeout = %v, want 5s", cfg.RequestTimeout)
	}

	if cfg.LogLevel != slog.LevelDebug {
		t.Errorf("cfg.LogLevel = %v, want debug", cfg.LogLevel)
	}

	if cfg.LogFormat != logging.FormatJSON {
		t.Errorf("cfg.LogFormat = %q, want json", cfg.LogFormat)
	}
}

func TestConfig_Load_WithMissingFile_ReturnsError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.json")

	if _, err := config.Load(path); err == nil {
		t.Errorf("config.Load() error = nil, want error")
	}
}

func TestConfig_Load_WithMalformedJSON_ReturnsError(t *testing.T) {
	if _, err := config.Load(writeTempConfig(t, "{")); err == nil {
		t.Errorf("config.Load() error = nil, want error")
	}
}

func TestConfig_Load_WithUnknownField_ReturnsError(t *testing.T) {
	keyPath := writeTempFile(t, labKeyFile)
	document := `{"orchestrator_url": "http://10.0.0.5:8080", "lab_key_path": "` + keyPath + `", "unknown_field": true}`

	if _, err := config.Load(writeTempConfig(t, document)); err == nil {
		t.Errorf("config.Load() error = nil, want error")
	}
}

func TestConfig_Load_WithInvalidOrchestratorURL_ReturnsError(t *testing.T) {
	cases := map[string]struct {
		url string
	}{
		"empty":              {url: ""},
		"malformed":          {url: "://bad"},
		"unsupported scheme": {url: "ftp://10.0.0.5"},
		"missing host":       {url: "http://"},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			keyPath := writeTempFile(t, labKeyFile)
			document := `{"orchestrator_url": "` + tc.url + `", "lab_key_path": "` + keyPath + `"}`

			if _, err := config.Load(writeTempConfig(t, document)); err == nil {
				t.Errorf("config.Load() error = nil, want error")
			}
		})
	}
}

func TestConfig_Load_WithInvalidLabKey_ReturnsError(t *testing.T) {
	cases := map[string]struct {
		keyPath string
	}{
		"empty path":   {keyPath: ""},
		"missing file": {keyPath: filepath.Join(t.TempDir(), "missing.key")},
		"non-hex":      {keyPath: writeTempFile(t, "not-hex")},
		"wrong size":   {keyPath: writeTempFile(t, "0123456789abcdef")},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			document := `{"orchestrator_url": "http://10.0.0.5:8080", "lab_key_path": "` + tc.keyPath + `"}`

			if _, err := config.Load(writeTempConfig(t, document)); err == nil {
				t.Errorf("config.Load() error = nil, want error")
			}
		})
	}
}

func TestConfig_Load_WithInvalidRequestTimeout_ReturnsError(t *testing.T) {
	cases := map[string]struct {
		timeout string
	}{
		"malformed": {timeout: "soon"},
		"zero":      {timeout: "0s"},
		"negative":  {timeout: "-1s"},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			keyPath := writeTempFile(t, labKeyFile)
			document := `{
				"orchestrator_url": "http://10.0.0.5:8080",
				"lab_key_path": "` + keyPath + `",
				"request_timeout": "` + tc.timeout + `"
			}`

			if _, err := config.Load(writeTempConfig(t, document)); err == nil {
				t.Errorf("config.Load() error = nil, want error")
			}
		})
	}
}

func TestConfig_Load_WithInvalidLogLevel_ReturnsError(t *testing.T) {
	keyPath := writeTempFile(t, labKeyFile)
	document := `{
		"orchestrator_url": "http://10.0.0.5:8080",
		"lab_key_path": "` + keyPath + `",
		"log_level": "verbose"
	}`

	if _, err := config.Load(writeTempConfig(t, document)); err == nil {
		t.Errorf("config.Load() error = nil, want error")
	}
}

func TestConfig_Load_WithInvalidLogFormat_ReturnsError(t *testing.T) {
	keyPath := writeTempFile(t, labKeyFile)
	document := `{
		"orchestrator_url": "http://10.0.0.5:8080",
		"lab_key_path": "` + keyPath + `",
		"log_format": "xml"
	}`

	if _, err := config.Load(writeTempConfig(t, document)); err == nil {
		t.Errorf("config.Load() error = nil, want error")
	}
}

func TestConfig_Load_HeartbeatInterval_UnsetField_DefaultsTo30Seconds(t *testing.T) {
	keyPath := writeTempFile(t, labKeyFile)
	document := `{
		"orchestrator_url": "http://10.0.0.5:8080",
		"lab_key_path": "` + keyPath + `"
	}`

	cfg, err := config.Load(writeTempConfig(t, document))
	if err != nil {
		t.Fatalf("config.Load() error = %v, want nil", err)
	}

	if cfg.HeartbeatInterval != 30*time.Second {
		t.Errorf("cfg.HeartbeatInterval = %v, want 30s", cfg.HeartbeatInterval)
	}
}

func TestConfig_Load_HeartbeatInterval_ValidValue_ReturnsParsedDuration(t *testing.T) {
	keyPath := writeTempFile(t, labKeyFile)
	document := `{
		"orchestrator_url": "http://10.0.0.5:8080",
		"lab_key_path": "` + keyPath + `",
		"heartbeat_interval": "5s"
	}`

	cfg, err := config.Load(writeTempConfig(t, document))
	if err != nil {
		t.Fatalf("config.Load() error = %v, want nil", err)
	}

	if cfg.HeartbeatInterval != 5*time.Second {
		t.Errorf("cfg.HeartbeatInterval = %v, want 5s", cfg.HeartbeatInterval)
	}
}

func TestConfig_Load_HeartbeatInterval_InvalidValues_ReturnError(t *testing.T) {
	cases := map[string]struct {
		interval string
	}{
		"malformed":                {interval: "soon"},
		"zero":                     {interval: "0s"},
		"negative":                 {interval: "-5s"},
		"exceeds staleness grace":  {interval: "2m"},
		"equal to staleness grace": {interval: "90s"},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			keyPath := writeTempFile(t, labKeyFile)
			document := `{
				"orchestrator_url": "http://10.0.0.5:8080",
				"lab_key_path": "` + keyPath + `",
				"heartbeat_interval": "` + tc.interval + `"
			}`

			_, err := config.Load(writeTempConfig(t, document))
			if err == nil {
				t.Fatalf("config.Load() error = nil, want error")
			}

			if !strings.Contains(err.Error(), "heartbeat_interval") {
				t.Errorf("config.Load() error = %v, want mention of heartbeat_interval", err)
			}
		})
	}
}

// decodeHex decodes the provided hexadecimal string, failing the test on a
// malformed value.
func decodeHex(t *testing.T, value string) []byte {
	t.Helper()

	key, err := hex.DecodeString(value)
	if err != nil {
		t.Fatalf("failed to decode hexadecimal value: %v", err)
	}

	return key
}

// writeTempFile writes the provided contents to a file in a temporary
// directory and returns its path.
func writeTempFile(t *testing.T, contents string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "lab.key")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("failed to write temporary file: %v", err)
	}

	return path
}

// writeTempConfig writes the provided JSON document to a file in a temporary
// directory and returns its path.
func writeTempConfig(t *testing.T, document string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "sentinel.json")
	if err := os.WriteFile(path, []byte(document), 0o600); err != nil {
		t.Fatalf("failed to write temporary configuration: %v", err)
	}

	return path
}
