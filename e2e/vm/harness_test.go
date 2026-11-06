//go:build vm

// Package vm verifies local filtering against real traffic in KVM VMs.
package vm

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"dougdomingos.com/aegis/internal/common/schemas"
)

const (
	// orchestratorURL is the Orchestrator API as reachable from the host.
	orchestratorURL = "http://192.168.150.10:8080"

	// sentinelIP is the address of the Sentinel VM.
	sentinelIP = "192.168.150.20"

	// sentinelDomain is the libvirt name of the Sentinel VM.
	sentinelDomain = "aegis-sentinel"

	// sshKeyPath is the throwaway key vm-up generates, relative to this package.
	sshKeyPath = ".state/id_ed25519"

	// groupName is the group holding the Sentinel during a scenario.
	groupName = "vm-lab"

	// readyTimeout caps waits for an acknowledgement or a state change.
	readyTimeout = 60 * time.Second

	// rebootTimeout caps the wait for a rebooted VM to recover.
	rebootTimeout = 3 * time.Minute

	// targetMarker is the text of the page both HTTP targets serve.
	targetMarker = "aegis-target"

	// pollInterval is the pause between polls of a condition.
	pollInterval = 500 * time.Millisecond
)

// TestMain fails fast with a clear message when the lab is not running.
func TestMain(m *testing.M) {
	client := http.Client{Timeout: 3 * time.Second}

	response, err := client.Get(orchestratorURL + "/machines")
	if err != nil {
		fmt.Fprintf(os.Stderr, "vm: the lab is not reachable (%v); run \"make vm-up\" first\n", err)
		os.Exit(1)
	}
	response.Body.Close()

	os.Exit(m.Run())
}

// lab drives the running VMs through the Orchestrator API and SSH.
type lab struct{}

// newLab returns a lab once the Sentinel is reported as CONNECTED.
func newLab(t *testing.T) *lab {
	t.Helper()

	l := &lab{}
	l.waitConnected(t)

	return l
}

// curlAsStudent fetches target as the unprivileged user and returns curl's
// exit code.
func (l *lab) curlAsStudent(t *testing.T, target string) int {
	t.Helper()

	code, _ := l.sudo(t, "sudo -u student curl -s -o /dev/null --max-time 5 "+target)

	return code
}

// sudo runs a command as root on the Sentinel, returning its exit code and
// combined output. A broken SSH session fails the test.
func (l *lab) sudo(t *testing.T, command string) (int, string) {
	t.Helper()

	code, out, err := sshRun("sudo sh -c '" + command + "'")
	if err != nil {
		t.Fatalf("ssh to the sentinel failed running %q: %v; output: %s", command, err, out)
	}

	return code, out
}

// applyPolicy creates a one-rule policy, links it to the Sentinel through a
// new group, waits for the acknowledgement and registers the cleanup.
func (l *lab) applyPolicy(t *testing.T, policyType, ruleType, value string) int64 {
	t.Helper()

	var policy schemas.PolicyOutputSchema
	doJSON(t, http.MethodPost, "/policies",
		map[string]string{"name": "vm-" + strings.ToLower(policyType+"-"+ruleType), "type": policyType}, &policy)
	t.Cleanup(func() { deleteIgnoringStatus(fmt.Sprintf("/policies/%d", policy.ID)) })

	doJSON(t, http.MethodPost, fmt.Sprintf("/policies/%d/rules", policy.ID),
		map[string]string{"rule_type": ruleType, "value": value}, nil)

	doJSON(t, http.MethodPost, "/groups", map[string]string{"name": groupName}, nil)

	// Cleanups run last-in first-out: unlink and remove the group before the
	// policy, then wait for the Sentinel to drop its table.
	t.Cleanup(func() {
		deleteIgnoringStatus("/groups/" + groupName + "/policy")
		deleteIgnoringStatus("/groups/" + groupName)
		l.waitTableAbsent(t)
	})

	machine := machineByIP(t, sentinelIP)
	doJSON(t, http.MethodPost, "/groups/"+groupName+"/machines", map[string]int64{"machine_id": machine.ID}, nil)
	doJSON(t, http.MethodPost, "/groups/"+groupName+"/policy", map[string]string{"policy_name": policy.Name}, nil)

	acked := waitForPropagation(t, "ACKED package", func(p schemas.PropagationOutputSchema) bool {
		return p.Kind == "PACKAGE" && p.Status == "ACKED" && p.PolicyName == policy.Name
	})

	return acked.Sequence
}

// revoke unlinks the policy and waits for the revoke acknowledgement.
func (l *lab) revoke(t *testing.T, afterSequence int64) {
	t.Helper()

	doJSON(t, http.MethodDelete, "/groups/"+groupName+"/policy", nil, nil)
	waitForPropagation(t, "ACKED revoke", func(p schemas.PropagationOutputSchema) bool {
		return p.Kind == "REVOKE" && p.Status == "ACKED" && p.Sequence > afterSequence
	})
}

// reboot restarts the Sentinel VM through libvirt and waits until it
// acknowledges a package newer than afterSequence, proving it authenticated
// and reapplied its rules with no manual step. It returns the new sequence.
func (l *lab) reboot(t *testing.T, afterSequence int64) int64 {
	t.Helper()

	out, err := exec.Command("virsh", "-c", "qemu:///system", "reboot", sentinelDomain).CombinedOutput()
	if err != nil {
		t.Fatalf("virsh reboot failed: %v; output: %s", err, out)
	}

	deadline := time.Now().Add(rebootTimeout)
	for {
		propagation := machineByIP(t, sentinelIP).Propagation
		if propagation.Kind == "PACKAGE" && propagation.Status == "ACKED" && propagation.Sequence > afterSequence {
			return propagation.Sequence
		}

		if time.Now().After(deadline) {
			t.Fatalf("the sentinel did not reapply its rules within %v; latest: %+v", rebootTimeout, propagation)
		}

		time.Sleep(pollInterval)
	}
}

// browseAsStudent loads url in headless Chrome as the unprivileged user and
// reports whether the target page's body arrived. A failed load dumps Chrome's
// error page instead, which lacks the marker.
func (l *lab) browseAsStudent(t *testing.T, url string) bool {
	t.Helper()

	// The sandbox is off because Ubuntu restricts the user namespaces it needs;
	// the profile is throwaway so no state leaks between loads.
	_, out := l.sudo(t, "sudo -u student -H timeout 30 google-chrome --headless=new --no-sandbox "+
		"--disable-gpu --disable-background-networking --user-data-dir=/tmp/chrome-profile-$$ "+
		"--dump-dom "+url+" 2>/dev/null; rm -rf /tmp/chrome-profile-$$")

	return strings.Contains(out, targetMarker)
}

// enableChromeDoH makes Chrome resolve names only through the lab's
// DNS-over-HTTPS server and removes the setting when the test ends.
func (l *lab) enableChromeDoH(t *testing.T) {
	t.Helper()

	const policyDir = "/etc/opt/chrome/policies/managed"

	if code, out := l.sudo(t, "mkdir -p "+policyDir+" && cp /etc/aegis/chrome-doh-policy.json "+policyDir+"/"); code != 0 {
		t.Fatalf("failed to install the Chrome DoH policy: exit %d; output: %s", code, out)
	}

	t.Cleanup(func() { l.sudo(t, "rm -f "+policyDir+"/chrome-doh-policy.json") })
}

// waitConnected waits until the Orchestrator reports the Sentinel CONNECTED.
func (l *lab) waitConnected(t *testing.T) {
	t.Helper()

	deadline := time.Now().Add(readyTimeout)
	for {
		machine := machineByIP(t, sentinelIP)
		if machine.Status == "CONNECTED" {
			return
		}

		if time.Now().After(deadline) {
			t.Fatalf("the sentinel is not CONNECTED after %v; status %q", readyTimeout, machine.Status)
		}

		time.Sleep(pollInterval)
	}
}

// waitTableAbsent waits until the Sentinel no longer has the Aegis table.
func (l *lab) waitTableAbsent(t *testing.T) {
	t.Helper()

	deadline := time.Now().Add(readyTimeout)
	for {
		code, _ := l.sudo(t, "nft list table inet aegis")
		if code != 0 {
			return
		}

		if time.Now().After(deadline) {
			t.Fatalf("the aegis table is still present after %v", readyTimeout)
		}

		time.Sleep(pollInterval)
	}
}

// sshRun runs a command on the Sentinel as the aegis user. The error is set
// only when SSH itself failed (exit 255), not when the command did.
func sshRun(command string) (int, string, error) {
	cmd := exec.Command("ssh", "-i", sshKeyPath,
		"-o", "StrictHostKeyChecking=no", "-o", "UserKnownHostsFile=/dev/null",
		"-o", "LogLevel=ERROR", "-o", "ConnectTimeout=5", "aegis@"+sentinelIP, command)

	out, err := cmd.CombinedOutput()

	var exitErr *exec.ExitError
	switch {
	case err == nil:
		return 0, string(out), nil
	case errors.As(err, &exitErr) && exitErr.ExitCode() != 255:
		return exitErr.ExitCode(), string(out), nil
	default:
		return -1, string(out), err
	}
}

// doJSON performs one API request, failing the test on a non-2xx response,
// and decodes the body into out when provided.
func doJSON(t *testing.T, method, path string, body, out any) {
	t.Helper()

	var payload []byte
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("failed to encode the %s %s payload: %v", method, path, err)
		}

		payload = encoded
	}

	request, err := http.NewRequest(method, orchestratorURL+path, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("failed to build the %s %s request: %v", method, path, err)
	}

	request.Header.Set("Content-Type", "application/json")

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("the %s %s request failed: %v", method, path, err)
	}
	defer response.Body.Close()

	raw, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("failed to read the %s %s response: %v", method, path, err)
	}

	if response.StatusCode < 200 || response.StatusCode > 299 {
		t.Fatalf("%s %s status = %d, want 2xx; body: %s", method, path, response.StatusCode, raw)
	}

	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			t.Fatalf("failed to decode the %s %s response: %v; body: %s", method, path, err, raw)
		}
	}
}

// deleteIgnoringStatus sends a best-effort cleanup DELETE: the resource may
// already be gone when a scenario removed it or failed before creating it.
func deleteIgnoringStatus(path string) {
	request, err := http.NewRequest(http.MethodDelete, orchestratorURL+path, nil)
	if err != nil {
		return
	}

	if response, err := http.DefaultClient.Do(request); err == nil {
		response.Body.Close()
	}
}

// machineByIP returns the registry entry of the machine at ip.
func machineByIP(t *testing.T, ip string) schemas.MachineOutputSchema {
	t.Helper()

	var machines []schemas.MachineOutputSchema
	doJSON(t, http.MethodGet, "/machines", nil, &machines)

	for _, machine := range machines {
		if machine.IP == ip {
			return machine
		}
	}

	t.Fatalf("machine %s not found in the registry", ip)

	return schemas.MachineOutputSchema{}
}

// waitForPropagation polls the Sentinel's latest propagation until condition
// holds, failing the test on timeout.
func waitForPropagation(t *testing.T, want string, condition func(schemas.PropagationOutputSchema) bool) schemas.PropagationOutputSchema {
	t.Helper()

	deadline := time.Now().Add(readyTimeout)
	for {
		propagation := machineByIP(t, sentinelIP).Propagation
		if condition(propagation) {
			return propagation
		}

		if time.Now().After(deadline) {
			t.Fatalf("propagation never reached %s within %v; latest: %+v", want, readyTimeout, propagation)
		}

		time.Sleep(pollInterval)
	}
}
