//go:build e2e

// Package e2e runs end-to-end tests against the real Aegis binaries. The suite
// builds one image containing both executables, boots the Orchestrator and a
// fleet of Sentinels on a fixed-subnet Docker network, and asserts on the
// machine registry, the container exit codes and the emitted log records.
package e2e

import (
	"context"
	"io"
	"strings"
	"testing"
)

func TestFleetE2E_OrchestratorStart_RegistersConfiguredMachinesAsDisconnected(t *testing.T) {
	f := newFleet(t, 2)
	f.startOrchestrator(t)

	got := f.machines(t)
	for i := 0; i < 2; i++ {
		machine, ok := got[sentinelIP(i)]
		if !ok {
			t.Fatalf("machine %s is missing from the registry: %v", sentinelIP(i), got)
		}
		if machine.Name != machineName(i) {
			t.Errorf("machine %s name = %q, want %q", sentinelIP(i), machine.Name, machineName(i))
		}
		if machine.Status != "DISCONNECTED" {
			t.Errorf("machine %s status = %q, want DISCONNECTED", sentinelIP(i), machine.Status)
		}
	}
}

func TestFleetE2E_ValidLabKeys_FleetAuthenticatesFlipsConnectedAndStopsGracefully(t *testing.T) {
	f := newFleet(t, 3)
	orchestrator := f.startOrchestrator(t)

	sentinels := make([]*sentinelHandle, 3)
	for i := range sentinels {
		sentinels[i] = f.startSentinel(t, i, validLabKeyHex)
	}

	for _, s := range sentinels {
		s.waitForLog(t, "daemon running", readyTimeout)
	}

	f.waitForConnected(t, []string{sentinelIP(0), sentinelIP(1), sentinelIP(2)}, readyTimeout)

	for i, s := range sentinels {
		output := s.logs(t)
		for _, want := range []string{
			"configuration loaded",
			"machine identity detected",
			"machine authenticated",
			"machine=" + machineName(i),
		} {
			if !strings.Contains(output, want) {
				t.Errorf("sentinel %s output is missing record %q;\nlogs:\n%s", s.ip, want, output)
			}
		}

		assertNoSecrets(t, output)

		if code := s.stopAndExitCode(t); code != 0 {
			t.Errorf("sentinel %s exit code = %d, want 0;\nlogs:\n%s", s.ip, code, s.logs(t))
		}

		stopped := s.logs(t)
		if !strings.Contains(stopped, "daemon stopping") {
			t.Errorf("sentinel %s output is missing the daemon stopping record;\nlogs:\n%s", s.ip, stopped)
		}
		if strings.Contains(stopped, "forcing exit") {
			t.Errorf("sentinel %s output contains an unexpected forced exit record;\nlogs:\n%s", s.ip, stopped)
		}
	}

	reader, err := orchestrator.Logs(context.Background())
	if err != nil {
		t.Fatalf("failed to read the orchestrator logs: %v", err)
	}
	defer reader.Close()

	out, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("failed to read the orchestrator logs: %v", err)
	}

	assertNoSecrets(t, string(out))
}
