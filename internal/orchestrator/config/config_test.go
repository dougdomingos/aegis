package config_test

import (
	"bytes"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"dougdomingos.com/aegis/internal/orchestrator/config"
)

// validLabKey is a 32-byte key encoded as 64 hexadecimal characters.
const validLabKey = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

// labKeyFile is the contents of a valid lab key file.
const labKeyFile = validLabKey + "\n"

func TestConfig_Load_WithValidEnv_ReturnsParsedConfig(t *testing.T) {
	t.Setenv("AEGIS_LAB_KEY", writeTempFile(t, labKeyFile))
	t.Setenv("AEGIS_SESSION_SECRET", "sign-secret")

	cfg, err := config.Load()

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	wantKey, _ := hex.DecodeString(validLabKey)
	if !bytes.Equal(cfg.LabKey, wantKey) {
		t.Errorf("expected lab key %x, got %x", wantKey, cfg.LabKey)
	}

	if cfg.SessionSecret != "sign-secret" {
		t.Errorf("expected session secret %q, got %q", "sign-secret", cfg.SessionSecret)
	}

	if cfg.SessionTTL != 24*time.Hour {
		t.Errorf("expected default TTL %v, got %v", 24*time.Hour, cfg.SessionTTL)
	}
}

func TestConfig_Load_WithCustomTTL_ReturnsConfiguredTTL(t *testing.T) {
	t.Setenv("AEGIS_LAB_KEY", writeTempFile(t, labKeyFile))
	t.Setenv("AEGIS_SESSION_SECRET", "sign-secret")
	t.Setenv("AEGIS_SESSION_TTL", "1h")

	cfg, err := config.Load()

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if cfg.SessionTTL != time.Hour {
		t.Errorf("expected TTL %v, got %v", time.Hour, cfg.SessionTTL)
	}
}

func TestConfig_Load_WithUnsetGrace_DefaultsToNinetySeconds(t *testing.T) {
	t.Setenv("AEGIS_LAB_KEY", writeTempFile(t, labKeyFile))
	t.Setenv("AEGIS_SESSION_SECRET", "sign-secret")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if cfg.StalenessGrace != 90*time.Second {
		t.Errorf("expected default grace %v, got %v", 90*time.Second, cfg.StalenessGrace)
	}
}

func TestConfig_Load_WithCustomGrace_ReturnsConfiguredGrace(t *testing.T) {
	t.Setenv("AEGIS_LAB_KEY", writeTempFile(t, labKeyFile))
	t.Setenv("AEGIS_SESSION_SECRET", "sign-secret")
	t.Setenv("AEGIS_STALENESS_GRACE", "10s")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if cfg.StalenessGrace != 10*time.Second {
		t.Errorf("expected grace %v, got %v", 10*time.Second, cfg.StalenessGrace)
	}
}

func TestConfig_Load_WithInvalidGrace_ReturnsError(t *testing.T) {
	cases := map[string]struct {
		grace string
	}{
		"malformed": {grace: "soon"},
		"zero":      {grace: "0s"},
		"negative":  {grace: "-5s"},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Setenv("AEGIS_LAB_KEY", writeTempFile(t, labKeyFile))
			t.Setenv("AEGIS_SESSION_SECRET", "sign-secret")
			t.Setenv("AEGIS_STALENESS_GRACE", tc.grace)

			if _, err := config.Load(); err == nil {
				t.Errorf("expected an error for grace %q, got nil", tc.grace)
			} else if !strings.Contains(err.Error(), "AEGIS_STALENESS_GRACE") {
				t.Errorf("expected error to mention AEGIS_STALENESS_GRACE, got: %v", err)
			}
		})
	}
}

func TestConfig_Load_WithoutLabKey_ReturnsError(t *testing.T) {
	t.Setenv("AEGIS_SESSION_SECRET", "sign-secret")

	_, err := config.Load()

	if err == nil {
		t.Error("expected an error for missing lab key, got nil")
	}
}

func TestConfig_Load_WithMissingLabKeyFile_ReturnsError(t *testing.T) {
	t.Setenv("AEGIS_LAB_KEY", filepath.Join(t.TempDir(), "does-not-exist.key"))
	t.Setenv("AEGIS_SESSION_SECRET", "sign-secret")

	_, err := config.Load()

	if err == nil {
		t.Error("expected an error for a missing lab key file, got nil")
	}
}

func TestConfig_Load_WithNonHexLabKeyFile_ReturnsError(t *testing.T) {
	t.Setenv("AEGIS_LAB_KEY", writeTempFile(t, "not-hex\n"))
	t.Setenv("AEGIS_SESSION_SECRET", "sign-secret")

	_, err := config.Load()

	if err == nil {
		t.Error("expected an error for a non-hex lab key file, got nil")
	}
}

func TestConfig_Load_WithWrongSizeLabKeyFile_ReturnsError(t *testing.T) {
	t.Setenv("AEGIS_LAB_KEY", writeTempFile(t, "0123456789abcdef\n"))
	t.Setenv("AEGIS_SESSION_SECRET", "sign-secret")

	_, err := config.Load()

	if err == nil {
		t.Error("expected an error for a lab key file that is not 32 bytes, got nil")
	}
}

func TestConfig_Load_WithoutSessionSecret_ReturnsError(t *testing.T) {
	t.Setenv("AEGIS_LAB_KEY", writeTempFile(t, labKeyFile))

	_, err := config.Load()

	if err == nil {
		t.Error("expected an error for missing session secret, got nil")
	}
}

func TestConfig_Load_WithInvalidTTL_ReturnsError(t *testing.T) {
	t.Setenv("AEGIS_LAB_KEY", writeTempFile(t, labKeyFile))
	t.Setenv("AEGIS_SESSION_SECRET", "sign-secret")
	t.Setenv("AEGIS_SESSION_TTL", "soon")

	_, err := config.Load()

	if err == nil {
		t.Error("expected an error for invalid TTL, got nil")
	}
}

func TestConfig_Load_WithoutGRPCAddr_DefaultsToPort9090(t *testing.T) {
	t.Setenv("AEGIS_LAB_KEY", writeTempFile(t, labKeyFile))
	t.Setenv("AEGIS_SESSION_SECRET", "sign-secret")

	cfg, err := config.Load()

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if cfg.GRPCAddr != ":9090" {
		t.Errorf("expected default gRPC address %q, got %q", ":9090", cfg.GRPCAddr)
	}
}

func TestConfig_Load_WithGRPCAddr_ReturnsConfiguredAddress(t *testing.T) {
	t.Setenv("AEGIS_LAB_KEY", writeTempFile(t, labKeyFile))
	t.Setenv("AEGIS_SESSION_SECRET", "sign-secret")
	t.Setenv("AEGIS_GRPC_ADDR", "10.0.0.5:9091")

	cfg, err := config.Load()

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if cfg.GRPCAddr != "10.0.0.5:9091" {
		t.Errorf("expected gRPC address %q, got %q", "10.0.0.5:9091", cfg.GRPCAddr)
	}
}

func TestConfig_Load_WithInvalidGRPCAddr_ReturnsError(t *testing.T) {
	cases := map[string]struct {
		addr string
	}{
		"no colon":     {addr: "9090"},
		"empty host":   {addr: ":abc"},
		"port too big": {addr: ":70000"},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Setenv("AEGIS_LAB_KEY", writeTempFile(t, labKeyFile))
			t.Setenv("AEGIS_SESSION_SECRET", "sign-secret")
			t.Setenv("AEGIS_GRPC_ADDR", tc.addr)

			if _, err := config.Load(); err == nil {
				t.Errorf("expected an error for gRPC address %q, got nil", tc.addr)
			}
		})
	}
}

// writeTempFile writes content to a temporary file and returns its path.
func writeTempFile(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	return path
}
