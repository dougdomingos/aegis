package config_test

import (
	"bytes"
	"encoding/hex"
	"log"
	"os"
	"strings"
	"testing"
	"time"

	"dougdomingos.com/aegis/internal/config"
)

// validLabKey is a 32-byte key encoded as 64 hexadecimal characters.
const validLabKey = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestConfig_Load_WithValidEnv_ReturnsParsedConfig(t *testing.T) {
	t.Setenv("AEGIS_LAB_KEY", validLabKey)
	t.Setenv("AEGIS_SESSION_SECRET", "sign-secret")
	t.Setenv("AEGIS_MACHINES", "10.0.1.11=lcc1-011,10.0.1.12=lcc1-012")

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
	t.Setenv("AEGIS_LAB_KEY", validLabKey)
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

func TestConfig_Load_WithoutLabKey_ReturnsError(t *testing.T) {
	t.Setenv("AEGIS_SESSION_SECRET", "sign-secret")

	_, err := config.Load()

	if err == nil {
		t.Error("expected an error for missing lab key, got nil")
	}
}

func TestConfig_Load_WithNonHexLabKey_ReturnsError(t *testing.T) {
	t.Setenv("AEGIS_LAB_KEY", "not-hex")
	t.Setenv("AEGIS_SESSION_SECRET", "sign-secret")

	_, err := config.Load()

	if err == nil {
		t.Error("expected an error for a non-hex lab key, got nil")
	}
}

func TestConfig_Load_WithWrongSizeLabKey_ReturnsError(t *testing.T) {
	t.Setenv("AEGIS_LAB_KEY", "0123456789abcdef")
	t.Setenv("AEGIS_SESSION_SECRET", "sign-secret")

	_, err := config.Load()

	if err == nil {
		t.Error("expected an error for a lab key that is not 32 bytes, got nil")
	}
}

func TestConfig_Load_WithoutSessionSecret_ReturnsError(t *testing.T) {
	t.Setenv("AEGIS_LAB_KEY", validLabKey)

	_, err := config.Load()

	if err == nil {
		t.Error("expected an error for missing session secret, got nil")
	}
}

func TestConfig_Load_WithInvalidTTL_ReturnsError(t *testing.T) {
	t.Setenv("AEGIS_LAB_KEY", validLabKey)
	t.Setenv("AEGIS_SESSION_SECRET", "sign-secret")
	t.Setenv("AEGIS_SESSION_TTL", "soon")

	_, err := config.Load()

	if err == nil {
		t.Error("expected an error for invalid TTL, got nil")
	}
}

func TestConfig_Load_WithMalformedMachineEntry_ReturnsError(t *testing.T) {
	t.Setenv("AEGIS_LAB_KEY", validLabKey)
	t.Setenv("AEGIS_SESSION_SECRET", "sign-secret")
	t.Setenv("AEGIS_MACHINES", "10.0.1.11")

	_, err := config.Load()

	if err == nil {
		t.Error("expected an error for malformed machine entry, got nil")
	}
}

func TestConfig_Load_WithInvalidMachineIP_ReturnsError(t *testing.T) {
	t.Setenv("AEGIS_LAB_KEY", validLabKey)
	t.Setenv("AEGIS_SESSION_SECRET", "sign-secret")
	t.Setenv("AEGIS_MACHINES", "not-an-ip=lcc1-011")

	_, err := config.Load()

	if err == nil {
		t.Error("expected an error for invalid machine IP, got nil")
	}
}

func TestConfig_Load_WithoutMachines_LogsWarning(t *testing.T) {
	t.Setenv("AEGIS_LAB_KEY", validLabKey)
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
