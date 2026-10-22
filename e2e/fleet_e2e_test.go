//go:build e2e

// Package e2e runs end-to-end tests against the real Aegis binaries. The suite
// builds one image containing both executables, boots the Orchestrator and a
// fleet of Sentinels on a fixed-subnet Docker network, and asserts on the
// machine registry, the container exit codes and the emitted log records.
package e2e

import "testing"

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
