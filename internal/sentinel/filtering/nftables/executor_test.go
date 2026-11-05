package nftables_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"dougdomingos.com/aegis/internal/common/propagation"
	"dougdomingos.com/aegis/internal/sentinel/filtering"
	"dougdomingos.com/aegis/internal/sentinel/filtering/nftables"
)

func TestExecutor_Apply_WithSnapshot_RunsRenderedRuleset(t *testing.T) {
	// Arrange
	snapshot := filtering.PolicySnapshot{PolicyType: propagation.PolicyTypeBlacklist}
	executor, recorder := arrangeExecutorTest("/usr/sbin/nft", nil)

	// Act
	err := executor.Apply(snapshot)

	// Assert
	if err != nil {
		t.Fatalf("Apply() error = %v, want nil", err)
	}

	invocation := assertSingleInvocation(t, recorder)
	if invocation.binary != "/usr/sbin/nft" {
		t.Errorf("Apply() binary = %q, want %q", invocation.binary, "/usr/sbin/nft")
	}

	wantArgs := []string{"-f"}
	if !reflect.DeepEqual(invocation.args, wantArgs) {
		t.Errorf("Apply() args = %v, want %v", invocation.args, wantArgs)
	}

	wantStdin := nftables.RenderRuleset(snapshot)
	if invocation.stdin != wantStdin {
		t.Errorf("Apply() stdin = %q, want rendered ruleset %q", invocation.stdin, wantStdin)
	}
}

func TestExecutor_Apply_WithEmptyBinaryPath_DefaultsToNft(t *testing.T) {
	// Arrange
	executor, recorder := arrangeExecutorTest("", nil)

	// Act
	err := executor.Apply(filtering.PolicySnapshot{})

	// Assert
	if err != nil {
		t.Fatalf("Apply() error = %v, want nil", err)
	}

	invocation := assertSingleInvocation(t, recorder)
	if invocation.binary != "nft" {
		t.Errorf("Apply() binary = %q, want %q", invocation.binary, "nft")
	}
}

func TestExecutor_Apply_WithRunnerError_ReturnsWrappedCommandError(t *testing.T) {
	// Arrange
	runnerErr := errors.New("runner failed")
	executor, recorder := arrangeExecutorTest("nft-test", runnerErr)

	// Act
	err := executor.Apply(filtering.PolicySnapshot{})

	// Assert
	assertSingleInvocation(t, recorder)
	assertWrappedCommandError(t, err, runnerErr, "nft-test -f")
}

func TestExecutor_Clear_WithRenderedClear_RunsTableDeletion(t *testing.T) {
	// Arrange
	executor, recorder := arrangeExecutorTest("nft-test", nil)

	// Act
	err := executor.Clear()

	// Assert
	if err != nil {
		t.Fatalf("Clear() error = %v, want nil", err)
	}

	invocation := assertSingleInvocation(t, recorder)
	wantArgs := []string{"-f"}
	if !reflect.DeepEqual(invocation.args, wantArgs) {
		t.Errorf("Clear() args = %v, want %v", invocation.args, wantArgs)
	}

	wantStdin := nftables.RenderClear()
	if invocation.stdin != wantStdin {
		t.Errorf("Clear() stdin = %q, want rendered clear ruleset %q", invocation.stdin, wantStdin)
	}
}

func TestExecutor_Clear_WithRunnerError_ReturnsWrappedCommandError(t *testing.T) {
	// Arrange
	runnerErr := errors.New("runner failed")
	executor, recorder := arrangeExecutorTest("nft-test", runnerErr)

	// Act
	err := executor.Clear()

	// Assert
	assertSingleInvocation(t, recorder)
	assertWrappedCommandError(t, err, runnerErr, "nft-test -f")
}

func TestExecutor_AddResolvedIP_WithValidAddress_UsesAddressFamilySet(t *testing.T) {
	testCases := map[string]struct {
		ip      string
		setName string
	}{
		"IPv4 address": {
			ip:      "1.2.3.4",
			setName: nftables.ResolvedSetName,
		},
		"IPv6 address": {
			ip:      "2001:db8::1",
			setName: nftables.ResolvedSetName6,
		},
		"IPv4-mapped IPv6 address": {
			ip:      "::ffff:192.0.2.1",
			setName: nftables.ResolvedSetName6,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			// Arrange
			executor, recorder := arrangeExecutorTest("nft-test", nil)

			// Act
			err := executor.AddResolvedIP(tc.ip)

			// Assert
			if err != nil {
				t.Fatalf("AddResolvedIP() error = %v, want nil", err)
			}

			invocation := assertSingleInvocation(t, recorder)
			wantArgs := []string{"add", "element", "inet", "aegis", tc.setName, "{", tc.ip, "}"}
			if !reflect.DeepEqual(invocation.args, wantArgs) {
				t.Errorf("AddResolvedIP() args = %v, want %v", invocation.args, wantArgs)
			}

			if invocation.stdin != "" {
				t.Errorf("AddResolvedIP() stdin = %q, want empty", invocation.stdin)
			}
		})
	}
}

func TestExecutor_AddResolvedIP_WithInvalidAddress_RejectsWithoutRunningCommand(t *testing.T) {
	testCases := map[string]struct {
		ip string
	}{
		"empty address":       {ip: ""},
		"malformed address":   {ip: "not-an-ip"},
		"IPv4 CIDR":           {ip: "192.0.2.1/24"},
		"IPv6 CIDR":           {ip: "2001:db8::1/128"},
		"command injection":   {ip: "1.2.3.4; delete table inet aegis"},
		"bracketed IPv6 zone": {ip: "[fe80::1%eth0]"},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			// Arrange
			executor, recorder := arrangeExecutorTest("nft-test", nil)

			// Act
			err := executor.AddResolvedIP(tc.ip)

			// Assert
			if err == nil {
				t.Fatal("AddResolvedIP() error = nil, want validation error")
			}

			if !strings.Contains(err.Error(), "invalid resolved IP") {
				t.Errorf("AddResolvedIP() error = %q, want invalid resolved IP context", err)
			}

			if len(recorder.calls) != 0 {
				t.Errorf("AddResolvedIP() command invocations = %d, want 0", len(recorder.calls))
			}
		})
	}
}

func TestExecutor_AddResolvedIP_WithRunnerError_ReturnsWrappedCommandError(t *testing.T) {
	// Arrange
	runnerErr := errors.New("runner failed")
	executor, recorder := arrangeExecutorTest("nft-test", runnerErr)

	// Act
	err := executor.AddResolvedIP("1.2.3.4")

	// Assert
	assertSingleInvocation(t, recorder)
	assertWrappedCommandError(t, err, runnerErr, "nft-test add element inet aegis aegis_resolved { 1.2.3.4 }")
}

// commandInvocation records one nft CLI invocation received by the fake runner.
type commandInvocation struct {
	binary string
	args   []string
	stdin  string
}

// commandRecorder captures command invocations and returns a configured error.
type commandRecorder struct {
	calls []commandInvocation
	err   error
}

// run records one command invocation and returns the configured runner error.
func (r *commandRecorder) run(binary string, args []string, stdin string) error {
	r.calls = append(r.calls, commandInvocation{
		binary: binary,
		args:   append([]string(nil), args...),
		stdin:  stdin,
	})

	return r.err
}

// arrangeExecutorTest builds an executor with an injected recording runner.
func arrangeExecutorTest(binaryPath string, runnerErr error) (*nftables.Executor, *commandRecorder) {
	recorder := &commandRecorder{err: runnerErr}
	executor := nftables.NewExecutor(binaryPath)
	nftables.SetRunFuncForTest(executor, recorder.run)

	return executor, recorder
}

// assertSingleInvocation requires exactly one command invocation and returns it.
func assertSingleInvocation(t *testing.T, recorder *commandRecorder) commandInvocation {
	t.Helper()

	if len(recorder.calls) != 1 {
		t.Fatalf("command invocation count = %d, want 1", len(recorder.calls))
	}

	return recorder.calls[0]
}

// assertWrappedCommandError requires the runner error and command context.
func assertWrappedCommandError(t *testing.T, err, runnerErr error, commandContext string) {
	t.Helper()

	if !errors.Is(err, runnerErr) {
		t.Errorf("error = %v, want wrapped runner error %v", err, runnerErr)
	}

	if err == nil || !strings.Contains(err.Error(), commandContext) {
		t.Errorf("error = %v, want command context %q", err, commandContext)
	}
}
