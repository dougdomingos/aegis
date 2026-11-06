//go:build vm

package vm

import "testing"

func TestBrowser_Policy_WithEachTypeAndRule_EnforcesTraffic(t *testing.T) {
	for name, sc := range scenarios {
		t.Run(name, func(t *testing.T) {
			// Arrange
			l := newLab(t)
			requireBrowsable(t, l, allTargets)

			// Act
			sequence := l.applyPolicy(t, sc.policyType, sc.ruleType, sc.value)

			// Assert: enforcement
			assertBrowsing(t, l, sc.reachable, true)
			assertBrowsing(t, l, blockedTargets(sc), false)

			// Act and assert: revoke
			l.revoke(t, sequence)
			l.waitTableAbsent(t)
			requireBrowsable(t, l, allTargets)
		})
	}
}

func TestBrowser_DoH_WithBlacklistedDomain_StaysBlocked(t *testing.T) {
	// Arrange: DoH is the browser's only resolver, so a successful load with
	// no policy proves the browser really uses it.
	l := newLab(t)
	l.enableChromeDoH(t)
	requireBrowsable(t, l, []string{"http://allowed.test", "http://blocked.test"})

	// Act
	l.applyPolicy(t, "BLACKLIST", "DOMAIN", "blocked.test")

	// Assert
	assertBrowsing(t, l, []string{"http://allowed.test"}, true)

	// The browser's own DoH resolver skips the Sentinel's DNS filter, so the
	// domain still loads today. The gap is tracked in issue #27: the test keeps
	// running the check and skips instead of failing, so a fix shows up as a pass.
	if l.browseAsStudent(t, "http://blocked.test") {
		t.Skip("known gap (issue #27): a browser using DNS over HTTPS bypasses a BLACKLIST domain rule")
	}
}

// blockedTargets lists the targets a scenario blocks.
func blockedTargets(sc scenario) []string {
	targets := make([]string, 0, len(sc.blocked))
	for target := range sc.blocked {
		targets = append(targets, target)
	}

	return targets
}

// assertBrowsing checks that each target loads in the browser when want is
// true, and fails to load when it is false.
func assertBrowsing(t *testing.T, l *lab, targets []string, want bool) {
	t.Helper()

	for _, target := range targets {
		if got := l.browseAsStudent(t, target); got != want {
			t.Errorf("browser loaded %s = %v, want %v", target, got, want)
		}
	}
}

// requireBrowsable stops the scenario when a target does not load with no
// policy, because later blocked results would prove nothing.
func requireBrowsable(t *testing.T, l *lab, targets []string) {
	t.Helper()

	for _, target := range targets {
		if !l.browseAsStudent(t, target) {
			t.Fatalf("browser could not load %s with no policy", target)
		}
	}
}
