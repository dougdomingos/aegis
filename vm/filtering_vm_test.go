//go:build vm

package vm

import (
	"net/http"
	"testing"
)

const (
	// allowedIP and blockedIP are the addresses of the two HTTP targets.
	allowedIP = "192.168.150.31"
	blockedIP = "192.168.150.32"

	// curlDNSRefused is curl's exit code when a name does not resolve.
	curlDNSRefused = 6

	// curlTimedOut is curl's exit code when packets are silently dropped.
	curlTimedOut = 28
)

// allTargets lists every target that must be reachable with no policy.
var allTargets = []string{"http://allowed.test", "http://blocked.test", "http://" + allowedIP, "http://" + blockedIP}

func TestFiltering_Policy_WithEachTypeAndRule_EnforcesTraffic(t *testing.T) {
	scenarios := map[string]struct {
		policyType string
		ruleType   string
		value      string
		reachable  []string
		blocked    map[string]int
	}{
		"whitelist domain": {
			policyType: "WHITELIST", ruleType: "DOMAIN", value: "allowed.test",
			reachable: []string{"http://allowed.test"},
			blocked:   map[string]int{"http://blocked.test": curlDNSRefused, "http://" + blockedIP: curlTimedOut},
		},
		"whitelist ip": {
			policyType: "WHITELIST", ruleType: "IP", value: allowedIP,
			reachable: []string{"http://" + allowedIP},
			blocked:   map[string]int{"http://" + blockedIP: curlTimedOut},
		},
		"blacklist domain": {
			policyType: "BLACKLIST", ruleType: "DOMAIN", value: "blocked.test",
			reachable: []string{"http://allowed.test"},
			blocked:   map[string]int{"http://blocked.test": curlDNSRefused},
		},
		"blacklist ip": {
			policyType: "BLACKLIST", ruleType: "IP", value: blockedIP,
			reachable: []string{"http://" + allowedIP},
			blocked:   map[string]int{"http://" + blockedIP: curlTimedOut},
		},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			// Arrange
			l := newLab(t)
			requireReachable(t, l, allTargets)

			// Act and assert: enforcement
			sequence := l.applyPolicy(t, scenario.policyType, scenario.ruleType, scenario.value)
			assertTraffic(t, l, scenario.reachable, scenario.blocked)

			// Act and assert: recovery after a reboot
			sequence = l.reboot(t, sequence)
			assertTraffic(t, l, scenario.reachable, scenario.blocked)

			// Act and assert: revoke
			l.revoke(t, sequence)
			l.waitTableAbsent(t)
			requireReachable(t, l, allTargets)
		})
	}
}

func TestFiltering_Assertions_WithFilterRemoved_DetectBrokenFiltering(t *testing.T) {
	// Arrange
	l := newLab(t)
	l.applyPolicy(t, "WHITELIST", "IP", allowedIP)
	if code := l.curlAsStudent(t, "http://"+blockedIP); code != curlTimedOut {
		t.Fatalf("curl to %s exited %d before sabotage, want %d", blockedIP, code, curlTimedOut)
	}

	// Act
	if code, out := l.sudo(t, "nft delete table inet aegis"); code != 0 {
		t.Fatalf("failed to delete the aegis table: exit %d; output: %s", code, out)
	}

	// Assert: the same check now sees traffic pass, so it can fail.
	if code := l.curlAsStudent(t, "http://"+blockedIP); code != 0 {
		t.Errorf("curl to %s exited %d with no filter, want 0: blocked results may not come from Aegis", blockedIP, code)
	}

	// Force the cleanup path through a real revoke of the removed table.
	doJSON(t, http.MethodDelete, "/groups/"+groupName+"/policy", nil, nil)
}

// assertTraffic checks that each reachable target responds and each blocked
// target fails with its expected curl exit code.
func assertTraffic(t *testing.T, l *lab, reachable []string, blocked map[string]int) {
	t.Helper()

	for _, target := range reachable {
		if code := l.curlAsStudent(t, target); code != 0 {
			t.Errorf("curl to %s exited %d, want 0", target, code)
		}
	}

	for target, want := range blocked {
		if code := l.curlAsStudent(t, target); code != want {
			t.Errorf("curl to %s exited %d, want %d", target, code, want)
		}
	}
}

// requireReachable stops the scenario when a target is unreachable, because
// later blocked results would prove nothing.
func requireReachable(t *testing.T, l *lab, targets []string) {
	t.Helper()

	for _, target := range targets {
		if code := l.curlAsStudent(t, target); code != 0 {
			t.Fatalf("curl to %s exited %d with no policy, want 0", target, code)
		}
	}
}
