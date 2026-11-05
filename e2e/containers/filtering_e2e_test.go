//go:build e2e

package containers

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"dougdomingos.com/aegis/internal/common/schemas"
	tcexec "github.com/testcontainers/testcontainers-go/exec"
)

const (
	// filteredIP is the IP rule value rendered as an nftables accept line.
	filteredIP = "203.0.113.7"

	// dnsSettleTimeout bounds how long the filtering assertions poll for the
	// enforced state to become observable after the acknowledgement.
	dnsSettleTimeout = 15 * time.Second
)

// createIPRule adds an IP rule to the policy.
func createIPRule(t *testing.T, f *fleet, policyID int64, value string) schemas.RuleOutputSchema {
	t.Helper()

	var rule schemas.RuleOutputSchema
	f.doJSON(t, http.MethodPost, fmt.Sprintf("/policies/%d/rules", policyID),
		map[string]string{"rule_type": "IP", "value": value}, &rule)

	return rule
}

// exec runs a command inside the Sentinel container and returns its exit code
// and combined output.
func (h *sentinelHandle) exec(t *testing.T, cmd ...string) (int, string) {
	t.Helper()

	code, reader, err := h.container.Exec(context.Background(), cmd, tcexec.Multiplexed())
	if err != nil {
		t.Fatalf("failed to exec %v in the sentinel at %s: %v", cmd, h.ip, err)
	}

	out, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("failed to read the output of %v in the sentinel at %s: %v", cmd, h.ip, err)
	}

	return code, string(out)
}

// dig queries the embedded resolver for an A record and returns the full dig
// output, which includes the response status.
func (h *sentinelHandle) dig(t *testing.T, domain string) string {
	t.Helper()

	_, out := h.exec(t, "dig", "+time=3", "+tries=1", "@127.0.0.1", domain, "A")

	return out
}

// waitForDig polls the embedded resolver until the output of a query for
// domain contains want, failing the test on timeout.
func (h *sentinelHandle) waitForDig(t *testing.T, domain, want string) string {
	t.Helper()

	var out string

	deadline := time.Now().Add(dnsSettleTimeout)
	for time.Now().Before(deadline) {
		out = h.dig(t, domain)
		if strings.Contains(out, want) {
			return out
		}

		time.Sleep(250 * time.Millisecond)
	}

	t.Fatalf("query for %s never contained %q within %v; last output:\n%s\nlogs:\n%s",
		domain, want, dnsSettleTimeout, out, h.logs(t))

	return out
}

// linkFilteringWhitelist builds a WHITELIST policy allowing one IP and the
// allowed.test domain, links it to the Sentinel's machine and waits until the
// package is acknowledged.
func linkFilteringWhitelist(t *testing.T, f *fleet, sentinel *sentinelHandle) schemas.PropagationOutputSchema {
	t.Helper()

	policy := f.createPolicy(t, "filter-whitelist", "WHITELIST")
	createIPRule(t, f, policy.ID, filteredIP)
	f.createRule(t, policy.ID, "allowed.test")

	group := f.createGroup(t, "lab1")
	machine := f.machineByIP(t, sentinel.ip)
	f.linkMachine(t, group.Name, machine.ID)
	f.linkPolicy(t, group.Name, policy.Name)

	return f.waitForPropagation(t, sentinel.ip, "ACKED package", func(p schemas.PropagationOutputSchema) bool {
		return p.Kind == "PACKAGE" && p.Status == "ACKED" && p.PolicyName == policy.Name
	})
}

func TestFilteringE2E_WithWhitelistPackage_AppliesNftablesAndDNS(t *testing.T) {
	f, sentinel := startPropagatedFleet(t, 1)

	acked := linkFilteringWhitelist(t, f, sentinel)

	if acked.Status != "ACKED" {
		t.Errorf("propagation status = %q, want ACKED", acked.Status)
	}

	code, table := sentinel.exec(t, "nft", "list", "table", "inet", "aegis")
	if code != 0 {
		t.Fatalf("nft list table exited %d, want 0; output:\n%s\nlogs:\n%s", code, table, sentinel.logs(t))
	}
	for _, want := range []string{
		"policy drop",
		"ip daddr " + filteredIP + " accept",
		"ip daddr " + orchestratorIP + " accept",
		"ip daddr " + dockerEmbeddedDNS + " udp dport 53 accept",
	} {
		if !strings.Contains(table, want) {
			t.Errorf("nft table missing %q; table:\n%s", want, table)
		}
	}

	allowed := sentinel.waitForDig(t, "allowed.test", "status: NOERROR")
	if !strings.Contains(allowed, orchestratorIP) {
		t.Errorf("allowed.test answer missing %s; output:\n%s", orchestratorIP, allowed)
	}

	blocked := sentinel.dig(t, "blocked.test")
	if !strings.Contains(blocked, "status: REFUSED") {
		t.Errorf("blocked.test was not refused; output:\n%s", blocked)
	}

	_, resolved := sentinel.exec(t, "nft", "list", "set", "inet", "aegis", "aegis_resolved")
	if !strings.Contains(resolved, orchestratorIP) {
		t.Errorf("resolved set missing %s after the allowed query; output:\n%s", orchestratorIP, resolved)
	}
}

func TestFilteringE2E_WithRevoke_ClearsNftablesAndDNS(t *testing.T) {
	f, sentinel := startPropagatedFleet(t, 1)

	baseline := linkFilteringWhitelist(t, f, sentinel)

	if code, out := sentinel.exec(t, "nft", "list", "table", "inet", "aegis"); code != 0 {
		t.Fatalf("nft table absent before the revoke (exit %d); output:\n%s", code, out)
	}
	if out := sentinel.dig(t, "blocked.test"); !strings.Contains(out, "status: REFUSED") {
		t.Fatalf("blocked.test not refused before the revoke; output:\n%s", out)
	}

	f.unlinkPolicy(t, "lab1")

	f.waitForPropagation(t, sentinel.ip, "ACKED revoke", func(p schemas.PropagationOutputSchema) bool {
		return p.Status == "ACKED" && p.Kind == "REVOKE" && p.Sequence > baseline.Sequence
	})

	if code, out := sentinel.exec(t, "nft", "list", "table", "inet", "aegis"); code == 0 {
		t.Errorf("nft table still present after the revoke; output:\n%s", out)
	}

	forwarded := sentinel.waitForDig(t, "blocked.test", "status: NOERROR")
	if !strings.Contains(forwarded, orchestratorIP) {
		t.Errorf("blocked.test not forwarded after the revoke; output:\n%s", forwarded)
	}
}
