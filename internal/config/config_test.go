package config_test

import (
	"bytes"
	"encoding/hex"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"dougdomingos.com/aegis/internal/config"
)

// validLabKey is a 32-byte key encoded as 64 hexadecimal characters.
const validLabKey = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

// labKeyFile is the contents of a valid lab key file.
const labKeyFile = validLabKey + "\n"

// machinesFile is the contents of a valid machines CSV file.
const machinesFile = "ip,name\n10.0.1.11,lcc1-011\n10.0.1.12,lcc1-012\n"

func TestConfig_Load_WithValidEnv_ReturnsParsedConfig(t *testing.T) {
	t.Setenv("AEGIS_LAB_KEY", writeTempFile(t, labKeyFile))
	t.Setenv("AEGIS_SESSION_SECRET", "sign-secret")
	t.Setenv("AEGIS_MACHINES", writeTempFile(t, machinesFile))

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

	if len(cfg.Machines) != 2 {
		t.Fatalf("expected 2 machines, got %d", len(cfg.Machines))
	}

	if cfg.Machines[0].IP != "10.0.1.11" || cfg.Machines[0].Name != "lcc1-011" {
		t.Errorf("unexpected first machine: %+v", cfg.Machines[0])
	}

	if cfg.Machines[1].IP != "10.0.1.12" || cfg.Machines[1].Name != "lcc1-012" {
		t.Errorf("unexpected second machine: %+v", cfg.Machines[1])
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
	t.Setenv("AEGIS_MACHINES", writeTempFile(t, machinesFile))

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
	t.Setenv("AEGIS_MACHINES", writeTempFile(t, machinesFile))
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

func TestConfig_Load_WithMissingMachinesFile_ReturnsError(t *testing.T) {
	t.Setenv("AEGIS_LAB_KEY", writeTempFile(t, labKeyFile))
	t.Setenv("AEGIS_SESSION_SECRET", "sign-secret")
	t.Setenv("AEGIS_MACHINES", filepath.Join(t.TempDir(), "does-not-exist.csv"))

	_, err := config.Load()

	if err == nil {
		t.Error("expected an error for a missing machines file, got nil")
	}
}

func TestConfig_Load_WithWrongHeaderMachinesFile_ReturnsError(t *testing.T) {
	t.Setenv("AEGIS_LAB_KEY", writeTempFile(t, labKeyFile))
	t.Setenv("AEGIS_SESSION_SECRET", "sign-secret")
	t.Setenv("AEGIS_MACHINES", writeTempFile(t, "name,ip\nlcc1-011,10.0.1.11\n"))

	_, err := config.Load()

	if err == nil {
		t.Error("expected an error for a machines file with the wrong header, got nil")
	}
}

func TestConfig_Load_WithInvalidMachineIP_ReturnsError(t *testing.T) {
	t.Setenv("AEGIS_LAB_KEY", writeTempFile(t, labKeyFile))
	t.Setenv("AEGIS_SESSION_SECRET", "sign-secret")
	t.Setenv("AEGIS_MACHINES", writeTempFile(t, "ip,name\nnot-an-ip,lcc1-011\n"))

	_, err := config.Load()

	if err == nil {
		t.Error("expected an error for invalid machine IP, got nil")
	}
}

func TestConfig_Load_WithEmptyMachineName_ReturnsError(t *testing.T) {
	t.Setenv("AEGIS_LAB_KEY", writeTempFile(t, labKeyFile))
	t.Setenv("AEGIS_SESSION_SECRET", "sign-secret")
	t.Setenv("AEGIS_MACHINES", writeTempFile(t, "ip,name\n10.0.1.11,\n"))

	_, err := config.Load()

	if err == nil {
		t.Error("expected an error for an empty machine name, got nil")
	}
}

func TestConfig_Load_WithoutMachinesFile_LogsWarning(t *testing.T) {
	t.Setenv("AEGIS_LAB_KEY", writeTempFile(t, labKeyFile))
	t.Setenv("AEGIS_SESSION_SECRET", "sign-secret")

	var buf bytes.Buffer
	log.SetOutput(&buf)
	log.SetFlags(0)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	_, err := config.Load()

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if !strings.Contains(buf.String(), "no machines configured") {
		t.Errorf("expected warning about no machines, got %q", buf.String())
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
