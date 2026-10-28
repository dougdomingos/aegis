//go:build e2e

package e2e

import (
	"testing"
	"time"

	"dougdomingos.com/aegis/internal/auth"
)

func TestHeartbeatE2E_RunningSentinel_KeepsLastSeenAtFresh(t *testing.T) {
	f := newFleet(t, 1)
	f.startOrchestrator(t)

	sentinel := f.startSentinel(t, 0, validLabKeyHex, sentinelOptions{heartbeatInterval: "2s"})
	sentinel.waitForLog(t, "daemon running", readyTimeout)

	ip := sentinel.ip
	f.waitForConnected(t, []string{ip}, readyTimeout)

	before := f.machines(t)[ip].LastSeenAt
	if before == nil {
		t.Fatal("expected last_seen_at to be populated after authentication")
	}

	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		after := f.machines(t)[ip].LastSeenAt
		if after != nil && after.After(*before) {
			assertNoSecrets(t, sentinel.logs(t))

			return
		}

		time.Sleep(500 * time.Millisecond)
	}

	t.Fatalf("last_seen_at was not refreshed by the heartbeat within 30s;\nlogs:\n%s", sentinel.logs(t))
}

func TestHeartbeatE2E_SessionExpiry_TransparentlyRenewedAndStaysConnected(t *testing.T) {
	f := newFleet(t, 1)
	f.startOrchestrator(t, orchestratorOptions{env: map[string]string{
		"AEGIS_SESSION_TTL": "3s",
	}})

	sentinel := f.startSentinel(t, 0, validLabKeyHex, sentinelOptions{heartbeatInterval: "1s"})
	sentinel.waitForLog(t, "daemon running", readyTimeout)

	ip := sentinel.ip
	f.waitForConnected(t, []string{ip}, readyTimeout)

	sentinel.waitForLog(t, "session renewed", 30*time.Second)

	if got := f.statuses(t)[ip]; got != "CONNECTED" {
		t.Errorf("machine %s status = %q, want CONNECTED after session renewal;\nlogs:\n%s",
			ip, got, sentinel.logs(t))
	}

	running, err := sentinel.container.State(t.Context())
	if err != nil {
		t.Fatalf("failed to inspect the sentinel: %v", err)
	}

	if !running.Running {
		t.Errorf("the sentinel exited after renewal;\nlogs:\n%s", sentinel.logs(t))
	}

	assertNoSecrets(t, sentinel.logs(t))
}

func TestHeartbeatE2E_StoppedSentinel_FlipsDisconnectedAfterGrace(t *testing.T) {
	f := newFleet(t, 1)
	f.startOrchestrator(t, orchestratorOptions{env: map[string]string{
		"AEGIS_STALENESS_GRACE": "5s",
	}})

	sentinel := f.startSentinel(t, 0, validLabKeyHex, sentinelOptions{heartbeatInterval: "2s"})
	sentinel.waitForLog(t, "daemon running", readyTimeout)

	ip := sentinel.ip
	f.waitForConnected(t, []string{ip}, readyTimeout)

	if code := sentinel.stopAndExitCode(t); code != 0 {
		t.Fatalf("sentinel exit code = %d, want 0;\nlogs:\n%s", code, sentinel.logs(t))
	}

	f.waitForDisconnected(t, ip, 30*time.Second)
}

func TestHeartbeatE2E_HeartbeatWithUnusableToken_ReturnsUnauthorizedNotServerFault(t *testing.T) {
	f := newFleet(t, 1)
	f.startOrchestrator(t)

	unknownMachineToken, _, err := auth.SignSessionToken(4242, sessionSecret, time.Hour, time.Now())
	if err != nil {
		t.Fatalf("failed to sign session token: %v", err)
	}

	scenarios := map[string]struct {
		token string
	}{
		"well-formed token for a vanished machine": {token: unknownMachineToken},
		"missing header": {token: ""},
		"garbage bearer": {token: "not-a-token"},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			if status := f.postHeartbeat(t, scenario.token); status != 401 {
				t.Errorf("heartbeat status = %d, want 401", status)
			}
		})
	}
}
