//go:build e2e

// Package e2e runs end-to-end tests against the real Aegis binaries. The
// suite builds the Orchestrator and the Sentinel, boots them as separate
// processes and asserts on exit codes and emitted log records.
package e2e

import (
	"bytes"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"dougdomingos.com/aegis/internal/sentinel/identity"
)

const (
	// validLabKeyHex is a 32-byte lab key in lowercase hexadecimal shared by
	// the Orchestrator and the Sentinel in the happy-path scenario.
	validLabKeyHex = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

	// wrongLabKeyHex is a different 32-byte lab key used to invalidate the
	// Sentinel signature on purpose.
	wrongLabKeyHex = "fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210"

	// sessionSecret is the HMAC signing secret given to the Orchestrator.
	sessionSecret = "e2e-session-secret"

	// machineName is the name the Orchestrator assigns to the machine.
	machineName = "lcc1-001"

	// readinessTimeout caps how long the Orchestrator may take to accept
	// connections.
	readinessTimeout = 5 * time.Second

	// recordTimeout caps how long a Sentinel record may take to appear.
	recordTimeout = 15 * time.Second

	// sentinelTimeout caps the total runtime of a Sentinel process.
	sentinelTimeout = 30 * time.Second
)

var (
	// binOrchestrator is the path of the built Orchestrator binary.
	binOrchestrator string

	// binSentinel is the path of the built Sentinel binary.
	binSentinel string

	// binDir is the temporary directory holding the built binaries.
	binDir string
)

// TestMain builds both real binaries once and runs the suite.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "aegis-e2e-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "e2e: failed to create temporary directory: %v\n", err)
		os.Exit(1)
	}

	binDir = dir

	if err := buildBinary(&binOrchestrator, "aegis-orchestrator", "dougdomingos.com/aegis/cmd/orchestrator"); err != nil {
		fmt.Fprintf(os.Stderr, "e2e: %v\n", err)
		os.Exit(1)
	}

	if err := buildBinary(&binSentinel, "aegis-sentinel", "dougdomingos.com/aegis/cmd/sentinel"); err != nil {
		fmt.Fprintf(os.Stderr, "e2e: %v\n", err)
		os.Exit(1)
	}

	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

func TestSentinelE2E_WithValidConfiguration_AuthenticatesAndStopsGracefully(t *testing.T) {
	orchestrator := startOrchestrator(t, validLabKeyHex)
	run := startSentinel(t, orchestrator.url, orchestrator.labKeyPath)

	run.waitForRecord(t, "machine authenticated", recordTimeout)
	run.waitForRecord(t, "daemon running", recordTimeout)

	if err := run.interrupt(); err != nil {
		t.Fatalf("failed to send SIGINT: %v", err)
	}

	code := run.exitCode(t)
	if code != 0 {
		t.Fatalf("sentinel exit code = %d, want 0;\nlogs:\n%s", code, run.logs.String())
	}

	output := run.logs.String()
	for _, want := range []string{
		"configuration loaded",
		"machine identity detected",
		"machine authenticated",
		"machine=lcc1-001",
		"daemon running",
		"daemon stopping",
	} {
		if !strings.Contains(output, want) {
			t.Errorf("sentinel output is missing record %q", want)
		}
	}

	if strings.Contains(output, "forcing exit") {
		t.Errorf("sentinel output contains an unexpected forced exit record")
	}
}

func TestSentinelE2E_WithWrongLabKey_ExitsWithAuthenticationRejected(t *testing.T) {
	orchestrator := startOrchestrator(t, validLabKeyHex)

	wrongKeyPath := writeLabKeyFile(t, wrongLabKeyHex)
	run := startSentinelWithConfig(t, writeSentinelConfig(t, orchestrator.url, wrongKeyPath))

	code := run.exitCode(t)
	if code != 1 {
		t.Fatalf("sentinel exit code = %d, want 1;\nlogs:\n%s", code, run.logs.String())
	}

	if !strings.Contains(run.logs.String(), "authentication rejected by the orchestrator") {
		t.Errorf("sentinel output is missing the authentication rejected error;\nlogs:\n%s", run.logs.String())
	}
}

func TestSentinelE2E_WithUnreachableOrchestrator_ExitsWithUnreachableError(t *testing.T) {
	unreachable := freeAddress(t)
	labKeyPath := writeLabKeyFile(t, validLabKeyHex)

	run := startSentinel(t, "http://"+unreachable, labKeyPath)

	code := run.exitCode(t)
	if code != 1 {
		t.Fatalf("sentinel exit code = %d, want 1;\nlogs:\n%s", code, run.logs.String())
	}

	if !strings.Contains(run.logs.String(), "orchestrator is unreachable") {
		t.Errorf("sentinel output is missing the unreachable error;\nlogs:\n%s", run.logs.String())
	}
}

func TestSentinelE2E_WithMissingConfigFile_ExitsWithConfigurationError(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "sentinel.json")

	run := startSentinelWithConfig(t, missing)

	code := run.exitCode(t)
	if code != 1 {
		t.Fatalf("sentinel exit code = %d, want 1;\nlogs:\n%s", code, run.logs.String())
	}

	if !strings.Contains(run.logs.String(), "failed to load configuration") {
		t.Errorf("sentinel output is missing the configuration error;\nlogs:\n%s", run.logs.String())
	}
}

// buildBinary builds the provided package into the temporary directory and
// stores its path in target.
func buildBinary(target *string, name, pkg string) error {
	out := filepath.Join(binDir, name)

	cmd := exec.Command("go", "build", "-o", out, pkg)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to build %s: %w", pkg, err)
	}

	*target = out

	return nil
}

// orchestrator is a running Orchestrator process, ready to accept requests.
type orchestrator struct {
	url        string
	labKeyPath string
	logs       *syncBuffer
}

// startOrchestrator boots the real Orchestrator binary with a temporary
// database, the provided lab key and the local machine's identity in the
// machines list, waiting for it to accept connections. The process and its
// files are discarded at test cleanup.
func startOrchestrator(t *testing.T, labKeyHex string) *orchestrator {
	t.Helper()

	address := freeAddress(t)
	dir := t.TempDir()

	labKeyPath := writeLabKeyFile(t, labKeyHex)

	id, err := identity.NewDetector().Detect()
	if err != nil {
		t.Fatalf("failed to detect the local machine identity: %v", err)
	}

	machinesPath := filepath.Join(dir, "machines.csv")
	machines := fmt.Sprintf("ip,name\n%s,%s\n", id.IP, machineName)
	if err := os.WriteFile(machinesPath, []byte(machines), 0o600); err != nil {
		t.Fatalf("failed to write the machines file: %v", err)
	}

	logs := &syncBuffer{}

	cmd := exec.Command(binOrchestrator)
	cmd.Env = append(os.Environ(),
		"AEGIS_ADDR="+address,
		"AEGIS_DB="+filepath.Join(dir, "aegis.db"),
		"AEGIS_LAB_KEY="+labKeyPath,
		"AEGIS_SESSION_SECRET="+sessionSecret,
		"AEGIS_MACHINES="+machinesPath)
	cmd.Stdout = logs
	cmd.Stderr = logs

	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start the orchestrator: %v", err)
	}

	t.Cleanup(func() {
		stopProcess(t, cmd)
		if t.Failed() {
			t.Logf("orchestrator logs:\n%s", logs.String())
		}
	})

	waitForConnection(t, address)

	return &orchestrator{
		url:        "http://" + address,
		labKeyPath: labKeyPath,
		logs:       logs,
	}
}

// freeAddress returns a loopback address whose TCP port is free.
func freeAddress(t *testing.T) string {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to allocate a free port: %v", err)
	}
	defer listener.Close()

	return listener.Addr().String()
}

// waitForConnection blocks until the provided address accepts TCP
// connections.
func waitForConnection(t *testing.T, address string) {
	t.Helper()

	deadline := time.Now().Add(readinessTimeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", address, time.Second)
		if err == nil {
			conn.Close()

			return
		}

		time.Sleep(100 * time.Millisecond)
	}

	t.Fatalf("the orchestrator did not become ready within %v", readinessTimeout)
}

// stopProcess terminates the provided process with SIGTERM, forcing the kill
// after a short grace period.
func stopProcess(t *testing.T, cmd *exec.Cmd) {
	t.Helper()

	done := make(chan struct{})
	go func() {
		cmd.Wait()
		close(done)
	}()

	_ = cmd.Process.Signal(syscall.SIGTERM)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		_ = cmd.Process.Kill()
		<-done
	}
}

// sentinel is a running Sentinel process under test.
type sentinel struct {
	cmd  *exec.Cmd
	logs *syncBuffer
	done chan struct{}
	werr error
}

// startSentinel writes a configuration file pointing at the provided
// Orchestrator URL and starts the real Sentinel binary with it.
func startSentinel(t *testing.T, orchestratorURL, labKeyPath string) *sentinel {
	t.Helper()

	configPath := writeSentinelConfig(t, orchestratorURL, labKeyPath)

	return startSentinelWithConfig(t, configPath)
}

// startSentinelWithConfig starts the real Sentinel binary with the provided
// configuration file. The process is force-killed at test cleanup if it is
// still running.
func startSentinelWithConfig(t *testing.T, configPath string) *sentinel {
	t.Helper()

	logs := &syncBuffer{}

	cmd := exec.Command(binSentinel, "--config", configPath)
	cmd.Stdout = logs
	cmd.Stderr = logs

	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start the sentinel: %v", err)
	}

	done := make(chan struct{})
	run := &sentinel{cmd: cmd, logs: logs, done: done}

	go func() {
		run.werr = cmd.Wait()
		close(run.done)
	}()

	t.Cleanup(func() {
		select {
		case <-run.done:
		default:
			_ = cmd.Process.Kill()
			<-run.done
		}
	})

	return run
}

// waitForRecord blocks until the Sentinel emits a record containing the
// provided text.
func (s *sentinel) waitForRecord(t *testing.T, needle string, timeout time.Duration) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if strings.Contains(s.logs.String(), needle) {
			return
		}

		select {
		case <-s.done:
			t.Fatalf("the sentinel exited (%v) while waiting for record %q;\nlogs:\n%s", s.werr, needle, s.logs.String())
		case <-time.After(50 * time.Millisecond):
		}
	}

	t.Fatalf("the sentinel did not log %q within %v;\nlogs:\n%s", needle, timeout, s.logs.String())
}

// interrupt sends SIGINT to the running Sentinel process.
func (s *sentinel) interrupt() error {
	return s.cmd.Process.Signal(os.Interrupt)
}

// exitCode blocks until the Sentinel process exits and returns its exit code.
func (s *sentinel) exitCode(t *testing.T) int {
	t.Helper()

	select {
	case <-s.done:
		if s.cmd.ProcessState == nil {
			t.Fatalf("the sentinel has no exit status: %v", s.werr)
		}

		return s.cmd.ProcessState.ExitCode()
	case <-time.After(sentinelTimeout):
		t.Fatalf("the sentinel did not exit within %v;\nlogs:\n%s", sentinelTimeout, s.logs.String())
	}

	return 0
}

// writeSentinelConfig writes a temporary Sentinel configuration file
// pointing at the provided Orchestrator URL and returns its path.
func writeSentinelConfig(t *testing.T, orchestratorURL, labKeyPath string) string {
	t.Helper()

	document := fmt.Sprintf(`{"orchestrator_url": %q, "lab_key_path": %q}`, orchestratorURL, labKeyPath)
	path := filepath.Join(t.TempDir(), "sentinel.json")

	if err := os.WriteFile(path, []byte(document), 0o600); err != nil {
		t.Fatalf("failed to write the sentinel configuration: %v", err)
	}

	return path
}

// writeLabKeyFile writes the provided hexadecimal lab key to a temporary
// file and returns its path.
func writeLabKeyFile(t *testing.T, keyHex string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "lab.key")
	if err := os.WriteFile(path, []byte(keyHex+"\n"), 0o600); err != nil {
		t.Fatalf("failed to write the lab key file: %v", err)
	}

	return path
}

// syncBuffer is a concurrency-safe in-memory log sink shared between the
// process and its assertions.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

// Write appends the provided bytes to the buffer.
func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.buf.Write(p)
}

// String returns the accumulated contents of the buffer.
func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.buf.String()
}
