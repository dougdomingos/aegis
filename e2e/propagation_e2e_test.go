//go:build e2e

package e2e

import (
	"encoding/json"
	"testing"

	sentinelPropagation "dougdomingos.com/aegis/internal/sentinel/propagation"

	"dougdomingos.com/aegis/internal/common/schemas"
)

// startPropagatedFleet boots one Orchestrator plus one Sentinel and waits for
// the machine to report CONNECTED in the registry.
func startPropagatedFleet(t *testing.T, machineCount int) (*fleet, *sentinelHandle) {
	t.Helper()

	f := newFleet(t, machineCount)
	f.startOrchestrator(t)

	sentinel := f.startSentinel(t, 0, validLabKeyHex)
	sentinel.waitForLog(t, "daemon running", readyTimeout)
	f.waitForConnected(t, []string{sentinel.ip}, readyTimeout)

	return f, sentinel
}

// linkPolicyToMachine creates a two-rule WHITELIST policy and a group, links
// the Sentinel's machine to the group, the group to the policy, and waits
// until the machine acknowledged the resulting package. It returns the policy
// and the acknowledged propagation row (the sequence baseline for later
// assertions).
func linkPolicyToMachine(t *testing.T, f *fleet, sentinel *sentinelHandle, policyName string) (schemas.PolicyOutputSchema, schemas.PropagationOutputSchema) {
	t.Helper()

	policy := f.createPolicy(t, policyName, "WHITELIST")
	f.createRule(t, policy.ID, "example.com")
	f.createRule(t, policy.ID, "test.local")

	group := f.createGroup(t, "lab1")
	machine := f.machineByIP(t, sentinel.ip)
	f.linkMachine(t, group.Name, machine.ID)
	f.linkPolicy(t, group.Name, policy.Name)

	acked := f.waitForPropagation(t, sentinel.ip, "ACKED package", func(p schemas.PropagationOutputSchema) bool {
		return p.Kind == "PACKAGE" && p.Status == "ACKED" && p.PolicyName == policyName
	})

	return policy, acked
}

// readAppliedState decodes the Sentinel's persisted propagation state.
func readAppliedState(t *testing.T, sentinel *sentinelHandle) sentinelPropagation.State {
	t.Helper()

	var state sentinelPropagation.State
	if err := json.Unmarshal([]byte(sentinel.readContainerFile(t, rulesStatePath)), &state); err != nil {
		t.Fatalf("failed to decode the sentinel state file: %v", err)
	}

	return state
}

func TestPropagationE2E_LinkedPolicy_AppliesPackageAndReportsAcked(t *testing.T) {
	f, sentinel := startPropagatedFleet(t, 1)
	policy, propagation := linkPolicyToMachine(t, f, sentinel, "lab-whitelist")

	if propagation.Sequence == 0 {
		t.Error("Sequence = 0, want a positive assigned sequence")
	}
	if propagation.DurationMS == nil {
		t.Error("DurationMS = nil, want the measured acknowledgement duration")
	}
	if propagation.PolicyName != "lab-whitelist" {
		t.Errorf("PolicyName = %q, want %q", propagation.PolicyName, "lab-whitelist")
	}
	if current := f.getPolicy(t, policy.ID); propagation.PolicyVersion != current.Version {
		t.Errorf("PolicyVersion = %d, want the policy's current version %d",
			propagation.PolicyVersion, current.Version)
	}

	sentinel.waitForLog(t, "package applied", readyTimeout)
	assertNoSecrets(t, sentinel.logs(t))
}

func TestPropagationE2E_RuleChange_PushesNewSequence(t *testing.T) {
	f, sentinel := startPropagatedFleet(t, 1)
	policy, baseline := linkPolicyToMachine(t, f, sentinel, "lab-whitelist")

	rules := f.listRules(t, policy.ID)
	if len(rules) == 0 {
		t.Fatal("expected the seeded rules to be listed")
	}

	f.updateRule(t, policy.ID, rules[0].ID, "changed.example.com")

	updated := f.waitForPropagation(t, sentinel.ip, "a newer ACKED sequence", func(p schemas.PropagationOutputSchema) bool {
		return p.Status == "ACKED" && p.Sequence > baseline.Sequence
	})

	if updated.Kind != "PACKAGE" {
		t.Errorf("Kind = %q, want PACKAGE", updated.Kind)
	}

	state := readAppliedState(t, sentinel)
	if state.LastSequence != updated.Sequence {
		t.Errorf("state LastSequence = %d, want %d", state.LastSequence, updated.Sequence)
	}
	if len(state.Rules) != 2 || (state.Rules[0].Value != "changed.example.com" && state.Rules[1].Value != "changed.example.com") {
		t.Errorf("state rules = %+v, want the changed value among two rules", state.Rules)
	}
}

func TestPropagationE2E_PolicyRename_PushesNewNameAndVersion(t *testing.T) {
	f, sentinel := startPropagatedFleet(t, 1)
	policy, baseline := linkPolicyToMachine(t, f, sentinel, "lab-whitelist")

	f.renamePolicy(t, policy.ID, "renamed-whitelist")

	renamed := f.waitForPropagation(t, sentinel.ip, "ACKED package with the new name", func(p schemas.PropagationOutputSchema) bool {
		return p.Status == "ACKED" && p.PolicyName == "renamed-whitelist" && p.Sequence > baseline.Sequence
	})

	if renamed.PolicyVersion <= policy.Version {
		t.Errorf("PolicyVersion = %d, want greater than %d after the rename", renamed.PolicyVersion, policy.Version)
	}
}

func TestPropagationE2E_UnlinkPolicy_PushesRevoke(t *testing.T) {
	f, sentinel := startPropagatedFleet(t, 1)
	_, baseline := linkPolicyToMachine(t, f, sentinel, "lab-whitelist")

	f.unlinkPolicy(t, "lab1")

	revoked := f.waitForPropagation(t, sentinel.ip, "ACKED revoke after unlink", func(p schemas.PropagationOutputSchema) bool {
		return p.Status == "ACKED" && p.Kind == "REVOKE" && p.Sequence > baseline.Sequence
	})

	if revoked.PolicyName != "" {
		t.Errorf("PolicyName = %q, want the revoke to report no policy", revoked.PolicyName)
	}

	state := readAppliedState(t, sentinel)
	if len(state.Rules) != 0 || state.PolicyID != nil {
		t.Errorf("applied state = %+v, want cleared rules and no policy", state)
	}
}

func TestPropagationE2E_DeleteLinkedPolicy_PushesRevoke(t *testing.T) {
	f, sentinel := startPropagatedFleet(t, 1)
	_, baseline := linkPolicyToMachine(t, f, sentinel, "first-policy")

	// Replace the link with a second policy, then delete it while linked: the
	// machines of the linking group must receive a revoke.
	second := f.createPolicy(t, "second-policy", "WHITELIST")
	f.createRule(t, second.ID, "second.example.com")
	f.linkPolicy(t, "lab1", "second-policy")

	relinked := f.waitForPropagation(t, sentinel.ip, "ACKED package of the second policy", func(p schemas.PropagationOutputSchema) bool {
		return p.Status == "ACKED" && p.Kind == "PACKAGE" && p.PolicyName == "second-policy" && p.Sequence > baseline.Sequence
	})

	f.deletePolicy(t, second.ID)

	revoked := f.waitForPropagation(t, sentinel.ip, "ACKED revoke after deleting the linked policy", func(p schemas.PropagationOutputSchema) bool {
		return p.Status == "ACKED" && p.Kind == "REVOKE" && p.Sequence > relinked.Sequence
	})

	if revoked.PolicyID == nil || *revoked.PolicyID != second.ID {
		t.Errorf("revoked PolicyID = %v, want the deleted policy %d in the audit row", revoked.PolicyID, second.ID)
	}

	state := readAppliedState(t, sentinel)
	if len(state.Rules) != 0 || state.PolicyID != nil {
		t.Errorf("applied state = %+v, want cleared rules and no policy", state)
	}
}

func TestPropagationE2E_DeleteGroupWithMachines_PushesRevoke(t *testing.T) {
	f, sentinel := startPropagatedFleet(t, 1)
	_, baseline := linkPolicyToMachine(t, f, sentinel, "lab-whitelist")

	f.deleteGroup(t, "lab1")

	revoked := f.waitForPropagation(t, sentinel.ip, "ACKED revoke after deleting the group", func(p schemas.PropagationOutputSchema) bool {
		return p.Status == "ACKED" && p.Kind == "REVOKE" && p.Sequence > baseline.Sequence
	})

	if revoked.PolicyName != "" {
		t.Errorf("PolicyName = %q, want the group-delete revoke to carry no policy", revoked.PolicyName)
	}

	state := readAppliedState(t, sentinel)
	if len(state.Rules) != 0 {
		t.Errorf("applied rules = %+v, want cleared", state.Rules)
	}
}

func TestPropagationE2E_MachineWithoutPolicy_ReportsPending(t *testing.T) {
	f, sentinel := startPropagatedFleet(t, 2)
	linkPolicyToMachine(t, f, sentinel, "lab-whitelist")

	// The second machine never connects, so no propagation row exists for it:
	// the API must report the synthesized PENDING representation.
	other := f.machineByIP(t, sentinelIP(1))

	if other.Propagation.Status != "PENDING" {
		t.Errorf("idle machine status = %q, want PENDING", other.Propagation.Status)
	}
	if other.Propagation.Sequence != 0 {
		t.Errorf("idle machine sequence = %d, want 0", other.Propagation.Sequence)
	}
	if other.Propagation.PolicyID != nil {
		t.Errorf("idle machine policy_id = %v, want null", other.Propagation.PolicyID)
	}
	if other.Propagation.SentAt != nil || other.Propagation.AckedAt != nil {
		t.Error("idle machine carries delivery timestamps, want none")
	}

	// And the single-machine endpoint reports PENDING too.
	idle := f.propagationOf(t, other.ID)
	if idle.Status != "PENDING" || idle.Kind != "" || idle.Sequence != 0 {
		t.Errorf("GET /machines/%d/propagation = %+v, want the PENDING representation", other.ID, idle)
	}
}

func TestPropagationE2E_SentinelRestart_PreservesSequence(t *testing.T) {
	f, sentinel := startPropagatedFleet(t, 1)
	policy, _ := linkPolicyToMachine(t, f, sentinel, "lab-whitelist")

	state := readAppliedState(t, sentinel)
	if state.LastSequence == 0 {
		t.Fatal("state last_sequence = 0, want a positive baseline before the restart")
	}

	if exitCode := sentinel.stopAndExitCode(t); exitCode != 0 {
		t.Errorf("sentinel exit code = %d, want 0 on graceful stop", exitCode)
	}

	sentinel.restart(t)
	sentinel.waitForLog(t, "configuration loaded", readyTimeout)

	rules := f.listRules(t, policy.ID)
	f.updateRule(t, policy.ID, rules[0].ID, "after-restart.example.com")

	acked := f.waitForPropagation(t, sentinel.ip, "an ACKED sequence above the baseline", func(p schemas.PropagationOutputSchema) bool {
		return p.Status == "ACKED" && p.Sequence > state.LastSequence
	})

	restarted := readAppliedState(t, sentinel)
	if restarted.LastSequence <= state.LastSequence {
		t.Errorf("state last_sequence = %d, want greater than %d after the restart",
			restarted.LastSequence, state.LastSequence)
	}
	if acked.Sequence < state.LastSequence {
		t.Errorf("acked sequence = %d, want above the pre-restart baseline %d", acked.Sequence, state.LastSequence)
	}
}
