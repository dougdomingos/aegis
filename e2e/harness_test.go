//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"os"
	"strings"
	"testing"
	"time"

	"dougdomingos.com/aegis/internal/schemas"
	mobynetwork "github.com/moby/moby/api/types/network"
	"github.com/testcontainers/testcontainers-go"
	tcnetwork "github.com/testcontainers/testcontainers-go/network"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	// networkSubnet is the fixed IPv4 subnet of the fleet network. The
	// addresses inside it are planned, so the accepted-machines registry can
	// be rendered before any container starts.
	networkSubnet = "172.30.0.0/24"

	// orchestratorIP is the planned static address of the Orchestrator.
	orchestratorIP = "172.30.0.10"

	// orchestratorPort is the container port the Orchestrator listens on.
	orchestratorPort = "8080/tcp"

	// validLabKeyHex is a 32-byte lab key in lowercase hexadecimal shared by
	// the Orchestrator and the fleet in the happy-path scenarios.
	validLabKeyHex = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

	// wrongLabKeyHex is a different 32-byte lab key used to invalidate the
	// Sentinel signature on purpose.
	wrongLabKeyHex = "fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210"

	// sessionSecret is the HMAC signing secret given to the Orchestrator.
	sessionSecret = "e2e-session-secret"

	// labKeyPath is the in-container path of the shared lab key file.
	labKeyPath = "/etc/aegis/lab.key"

	// machinesFilePath is the in-container path of the accepted-machines CSV.
	machinesFilePath = "/etc/aegis/machines.csv"

	// sentinelConfigPath is the in-container path of the Sentinel JSON config.
	sentinelConfigPath = "/etc/aegis/sentinel.json"

	// readyTimeout caps how long a fleet member may take to reach its expected
	// state.
	readyTimeout = 60 * time.Second

	// imageRepo and imageTag name the image built once per suite run.
	imageRepo = "aegis-e2e"
	imageTag  = "test"
)

// e2eImage is the reference (repo:tag) of the image built by TestMain.
var e2eImage string

// TestMain builds the single Aegis image once and runs the suite.
func TestMain(m *testing.M) {
	provider, err := testcontainers.NewDockerProvider()
	if err != nil {
		fmt.Fprintf(os.Stderr, "e2e: failed to create the docker provider: %v\n", err)
		os.Exit(1)
	}
	defer provider.Close()

	ref, err := provider.BuildImage(context.Background(), &testcontainers.ContainerRequest{
		FromDockerfile: testcontainers.FromDockerfile{
			Context:    "..",
			Dockerfile: "e2e/Dockerfile",
			Repo:       imageRepo,
			Tag:        imageTag,
			KeepImage:  true,
		},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "e2e: failed to build the aegis image: %v\n", err)
		os.Exit(1)
	}

	e2eImage = ref

	os.Exit(m.Run())
}

// sentinelIP returns the planned static IP of the sentinel container at index.
func sentinelIP(index int) string {
	return fmt.Sprintf("172.30.0.%d", 11+index)
}

// machineName returns the registry name assigned to the machine at index.
func machineName(index int) string {
	return fmt.Sprintf("lcc1-%03d", index+1)
}

// fleet is a per-test Aegis network on the fixed subnet plus its Orchestrator.
// The accepted-machines registry is rendered from the planned IPs before the
// Orchestrator starts, since SyncMachines runs only once at startup.
type fleet struct {
	t            *testing.T
	network      *testcontainers.DockerNetwork
	machineCount int
	orchestrator testcontainers.Container
}

// newFleet creates the fleet network with the fixed IPAM subnet and registers
// its cleanup.
func newFleet(t *testing.T, machineCount int) *fleet {
	t.Helper()

	nw, err := tcnetwork.New(context.Background(),
		tcnetwork.WithDriver("bridge"),
		tcnetwork.WithIPAM(&mobynetwork.IPAM{
			Driver: "default",
			Config: []mobynetwork.IPAMConfig{{Subnet: netip.MustParsePrefix(networkSubnet)}},
		}),
	)
	if err != nil {
		t.Fatalf("failed to create the fleet network: %v", err)
	}

	testcontainers.CleanupNetwork(t, nw)

	return &fleet{t: t, network: nw, machineCount: machineCount}
}

// orchestratorOptions carries per-test overrides for the Orchestrator boot.
type orchestratorOptions struct {
	// env holds extra environment variables merged over the defaults.
	env map[string]string
}

// startOrchestrator boots the real Orchestrator container on the fleet network
// with the planned static IP, the injected lab key and machines registry, and
// its API port published to the host, waiting for it to serve /machines.
func (f *fleet) startOrchestrator(t *testing.T, opts ...orchestratorOptions) testcontainers.Container {
	t.Helper()

	registry := "ip,name\n"
	for i := 0; i < f.machineCount; i++ {
		registry += fmt.Sprintf("%s,%s\n", sentinelIP(i), machineName(i))
	}

	env := map[string]string{
		"AEGIS_LAB_KEY":        labKeyPath,
		"AEGIS_SESSION_SECRET": sessionSecret,
		"AEGIS_MACHINES":       machinesFilePath,
		"AEGIS_DB":             "/home/nonroot/aegis.db",
	}
	for _, opt := range opts {
		for key, value := range opt.env {
			env[key] = value
		}
	}

	c, err := testcontainers.Run(context.Background(), e2eImage,
		testcontainers.WithEnv(env),
		testcontainers.WithFiles(
			injectedFile(labKeyPath, validLabKeyHex+"\n"),
			injectedFile(machinesFilePath, registry),
		),
		testcontainers.WithExposedPorts(orchestratorPort),
		testcontainers.WithWaitStrategyAndDeadline(readyTimeout,
			wait.ForHTTP("/machines").WithPort(orchestratorPort)),
		tcnetwork.WithNetwork(nil, f.network),
		withStaticIP(f.network.Name, orchestratorIP),
	)
	if err != nil {
		t.Fatalf("failed to start the orchestrator: %v", err)
	}

	t.Cleanup(func() {
		if err := c.Terminate(context.Background()); err != nil {
			t.Errorf("failed to terminate the orchestrator: %v", err)
		}
	})

	f.orchestrator = c

	return c
}

// machines lists the current machine registry keyed by IP via GET /machines on
// the host-mapped port of the running Orchestrator.
func (f *fleet) machines(t *testing.T) map[string]schemas.MachineOutputSchema {
	t.Helper()

	ctx := context.Background()

	endpoint, err := f.orchestrator.PortEndpoint(ctx, orchestratorPort, "http")
	if err != nil {
		t.Fatalf("failed to resolve the orchestrator endpoint: %v", err)
	}

	resp, err := http.Get(endpoint + "/machines")
	if err != nil {
		t.Fatalf("failed to list the machines: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list machines status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var list []schemas.MachineOutputSchema
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatalf("failed to decode the machines payload: %v", err)
	}

	byIP := make(map[string]schemas.MachineOutputSchema, len(list))
	for _, machine := range list {
		byIP[machine.IP] = machine
	}

	return byIP
}

// injectedFile declares an in-memory file copied into the container before it
// starts. The mode is world-readable because the distroless runtime user is
// nonroot and the containers are ephemeral.
func injectedFile(containerPath, content string) testcontainers.ContainerFile {
	return testcontainers.ContainerFile{
		Reader:            strings.NewReader(content),
		ContainerFilePath: containerPath,
		FileMode:          0o644,
	}
}

// withStaticIP pins the container's address inside the named network, so the
// IP detected by the Sentinel matches the planned entry in the registry.
func withStaticIP(networkName, ip string) testcontainers.CustomizeRequestOption {
	addr := netip.MustParseAddr(ip)

	return testcontainers.WithEndpointSettingsModifier(
		func(settings map[string]*mobynetwork.EndpointSettings) {
			if settings[networkName] == nil {
				settings[networkName] = &mobynetwork.EndpointSettings{}
			}
			settings[networkName].IPAMConfig = &mobynetwork.EndpointIPAMConfig{IPv4Address: addr}
		},
	)
}

// gracefulStopTimeout bounds how long Docker waits for a SIGTERM graceful exit
// before escalating to SIGKILL.
const gracefulStopTimeout = 10 * time.Second

// exitGraceTimeout bounds how long an exited container may take to be observed
// as stopped after its process ends.
const exitGraceTimeout = 30 * time.Second

// sentinelHandle is a running Sentinel container under test.
type sentinelHandle struct {
	container testcontainers.Container
	ip        string
	name      string
}

// sentinelOptions carries per-test overrides for the Sentinel boot.
type sentinelOptions struct {
	// heartbeatInterval is injected as the Sentinel heartbeat_interval field
	// when non-empty.
	heartbeatInterval string
}

// startSentinel boots the real Sentinel container with the planned static IP
// at index, its lab key and JSON configuration injected in-memory, pointing at
// the Orchestrator's planned address. No wait strategy runs: the process may
// exit on purpose.
func (f *fleet) startSentinel(t *testing.T, index int, labKeyHex string, opts ...sentinelOptions) *sentinelHandle {
	t.Helper()

	interval := ""
	for _, opt := range opts {
		if opt.heartbeatInterval != "" {
			interval = fmt.Sprintf(", \"heartbeat_interval\": %q", opt.heartbeatInterval)
		}
	}

	config := fmt.Sprintf(`{"orchestrator_url": "http://%s:8080", "lab_key_path": %q%s}`,
		orchestratorIP, labKeyPath, interval)

	c, err := testcontainers.Run(context.Background(), e2eImage,
		testcontainers.WithEntrypoint("/usr/local/bin/aegis-sentinel", "--config", sentinelConfigPath),
		testcontainers.WithFiles(
			injectedFile(labKeyPath, labKeyHex+"\n"),
			injectedFile(sentinelConfigPath, config),
		),
		tcnetwork.WithNetwork(nil, f.network),
		withStaticIP(f.network.Name, sentinelIP(index)),
	)
	if err != nil {
		t.Fatalf("failed to start the sentinel at %s: %v", sentinelIP(index), err)
	}

	t.Cleanup(func() {
		if err := c.Terminate(context.Background()); err != nil {
			t.Errorf("failed to terminate the sentinel at %s: %v", sentinelIP(index), err)
		}
	})

	return &sentinelHandle{container: c, ip: sentinelIP(index), name: machineName(index)}
}

// logs returns the full captured output of the Sentinel container, while it
// runs and after it exits.
func (h *sentinelHandle) logs(t *testing.T) string {
	t.Helper()

	reader, err := h.container.Logs(context.Background())
	if err != nil {
		t.Fatalf("failed to read the logs of the sentinel at %s: %v", h.ip, err)
	}
	defer reader.Close()

	out, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("failed to read the logs of the sentinel at %s: %v", h.ip, err)
	}

	return string(out)
}

// waitForLog blocks until the Sentinel emits a record containing the provided
// text, failing the test when it does not within the timeout.
func (h *sentinelHandle) waitForLog(t *testing.T, needle string, timeout time.Duration) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if strings.Contains(h.logs(t), needle) {
			return
		}

		time.Sleep(100 * time.Millisecond)
	}

	t.Fatalf("the sentinel at %s did not log %q within %v;\nlogs:\n%s",
		h.ip, needle, timeout, h.logs(t))
}

// stopAndExitCode stops the Sentinel the way Docker does — SIGTERM, escalating
// to SIGKILL after the graceful timeout — and returns its exit code.
func (h *sentinelHandle) stopAndExitCode(t *testing.T) int {
	t.Helper()

	timeout := gracefulStopTimeout
	if err := h.container.Stop(context.Background(), &timeout); err != nil {
		t.Fatalf("failed to stop the sentinel at %s: %v", h.ip, err)
	}

	return h.exitCode(t)
}

// exitCode blocks until the container is observed as stopped and returns the
// exit code reported by the runtime, independent of the process state on the
// host.
func (h *sentinelHandle) exitCode(t *testing.T) int {
	t.Helper()

	ctx := context.Background()
	deadline := time.Now().Add(exitGraceTimeout)
	for time.Now().Before(deadline) {
		state, err := h.container.State(ctx)
		if err != nil {
			t.Fatalf("failed to inspect the sentinel at %s: %v", h.ip, err)
		}

		if !state.Running {
			info, err := h.container.Inspect(ctx)
			if err != nil {
				t.Fatalf("failed to inspect the sentinel at %s: %v", h.ip, err)
			}

			return info.State.ExitCode
		}

		time.Sleep(100 * time.Millisecond)
	}

	t.Fatalf("the sentinel at %s did not stop within %v;\nlogs:\n%s",
		h.ip, exitGraceTimeout, h.logs(t))

	return 0
}

// waitForConnected blocks until every provided machine IP reports CONNECTED in
// the registry, reporting the full status map on timeout.
func (f *fleet) waitForConnected(t *testing.T, ips []string, timeout time.Duration) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		statuses := f.statuses(t)

		connected := true
		for _, ip := range ips {
			if statuses[ip] != "CONNECTED" {
				connected = false
			}
		}

		if connected {
			return
		}

		time.Sleep(500 * time.Millisecond)
	}

	t.Fatalf("the fleet did not reach CONNECTED within %v; statuses: %v", timeout, f.statuses(t))
}

// waitForDisconnected blocks until the provided machine IP reports
// DISCONNECTED in the registry, reporting the full status map on timeout.
func (f *fleet) waitForDisconnected(t *testing.T, ip string, timeout time.Duration) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if f.statuses(t)[ip] == "DISCONNECTED" {
			return
		}

		time.Sleep(500 * time.Millisecond)
	}

	t.Fatalf("the machine %s did not reach DISCONNECTED within %v; statuses: %v",
		ip, timeout, f.statuses(t))
}

// postHeartbeat sends a host-side heartbeat request with the provided bearer
// token to the running Orchestrator and returns the response status code.
func (f *fleet) postHeartbeat(t *testing.T, token string) int {
	t.Helper()

	ctx := context.Background()

	endpoint, err := f.orchestrator.PortEndpoint(ctx, orchestratorPort, "http")
	if err != nil {
		t.Fatalf("failed to resolve the orchestrator endpoint: %v", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint+"/machines/heartbeat", nil)
	if err != nil {
		t.Fatalf("failed to build the heartbeat request: %v", err)
	}

	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("heartbeat request failed: %v", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	return resp.StatusCode
}

// statuses maps every registry machine IP to its current status string.
func (f *fleet) statuses(t *testing.T) map[string]string {
	t.Helper()

	got := f.machines(t)

	statuses := make(map[string]string, len(got))
	for ip, machine := range got {
		statuses[ip] = machine.Status
	}

	return statuses
}

// secretNeedles are the secret materials that must never reach any log output.
var secretNeedles = []string{validLabKeyHex, wrongLabKeyHex, sessionSecret}

// secretHits returns the needles present in the provided output.
func secretHits(output string) []string {
	var hits []string

	for _, needle := range secretNeedles {
		if strings.Contains(output, needle) {
			hits = append(hits, needle)
		}
	}

	return hits
}

// assertNoSecrets fails the test when any secret material appears in the
// provided output. The reported message names only the hit count, so the
// assertion failure itself never leaks a secret.
func assertNoSecrets(t *testing.T, output string) {
	t.Helper()

	if hits := secretHits(output); len(hits) > 0 {
		t.Errorf("secret material leaked into the provided output: %d hit(s)", len(hits))
	}
}

// waitForExitCode blocks until the container is observed as stopped by the
// runtime on its own and asserts the exit code it reported.
func (h *sentinelHandle) waitForExitCode(t *testing.T, want int, timeout time.Duration) {
	t.Helper()

	ctx := context.Background()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		state, err := h.container.State(ctx)
		if err != nil {
			t.Fatalf("failed to inspect the sentinel at %s: %v", h.ip, err)
		}

		if !state.Running {
			info, err := h.container.Inspect(ctx)
			if err != nil {
				t.Fatalf("failed to inspect the sentinel at %s: %v", h.ip, err)
			}

			if info.State.ExitCode != want {
				t.Fatalf("the sentinel at %s exited with code %d, want %d;\nlogs:\n%s",
					h.ip, info.State.ExitCode, want, h.logs(t))
			}

			return
		}

		time.Sleep(100 * time.Millisecond)
	}

	t.Fatalf("the sentinel at %s did not exit within %v;\nlogs:\n%s", h.ip, timeout, h.logs(t))
}
