# Policy Version Bump and Empty-Policy Guards Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rule mutations bump the owning policy's version, and policies without rules cannot be linked to groups nor have their last rule removed while linked.

**Architecture:** The rule store performs the version bump inside the same transaction as each rule change. The services enforce the two guards using a new rule count and a new linked-policy check, both read-only.

**Tech Stack:** Go 1.26.5, chi, SQLite (`modernc.org/sqlite`), existing store test helpers.

**Spec:** [`docs/superpowers/specs/2026-11-03-policy-version-and-empty-policy-guards-design.md`](../specs/2026-11-03-policy-version-and-empty-policy-guards-design.md)

## Global Constraints

- Go version: `1.26.5`.
- Layering: API handler → service → store; dependencies are interfaces owned by the consumer.
- Every symbol carries a doc comment starting with its name; interface method implementations carry none.
- Migrations: none needed in this branch.
- Tests: external packages (`*_test`), Arrange-Act-Assert, names `Test<Type>_<Method>_<Scenario>_<ExpectedResult>`, `t.Errorf`/`t.Fatalf`, helpers at the end of the file, table-driven tests via `t.Run` for multiple validations of one scenario.
- Store tests use `arrangeStoreTest`; no schema is replicated.
- Handler tests assert only the HTTP status.
- Lint and format: `golangci-lint` v1.64.x and `goimports`; do not run hook-applied commands locally.
- Coverage gate: 75% (`make coverage`).
- Branch: `feat/policy-version-and-empty-policy-guards`, created from `develop`.

---

### Task 1: Rule mutations bump the policy version

**Files:**
- Modify: `internal/orchestrator/store/rule_store.go` (`Create`, `Update`, `Remove`)
- Test: `internal/orchestrator/store/rule_store_test.go`

**Interfaces:**
- Consumes: existing `domain.RuleStore` and `query.QueryExecutor.WithTx`.
- Produces: no interface change. The `version` and `updated_at` of the owning policy change within each rule mutation's transaction.

**Steps:**

- [ ] **Step 1: Write the failing tests** (names only, assertions as described):
  - `TestRuleStore_Create_WithValidData_BumpsPolicyVersion`: policy version goes from `1` to `2`, and `updated_at` changes.
  - `TestRuleStore_Create_WithInexistentPolicy_LeavesVersionUnchanged`: no policy exists, so nothing is bumped and the error is returned.
  - `TestRuleStore_Update_WithValidData_BumpsPolicyVersion`: version increments by exactly one.
  - `TestRuleStore_Update_WithInexistentID_LeavesVersionUnchanged`: the bump rolls back with the failed update.
  - `TestRuleStore_Remove_WithExistingID_BumpsPolicyVersion`: version increments by exactly one.
  - `TestRuleStore_Remove_WithInexistentID_LeavesVersionUnchanged`: no policy changes.
- [ ] **Step 2: Run them to see them fail.** Run: `go test ./internal/orchestrator/store/ -run 'TestRuleStore_(Create|Update|Remove)_.*Version' -v`. Expected: FAIL (versions do not change).
- [ ] **Step 3: Implement.** Add `queryBumpPolicyVersion = "UPDATE policies SET version = version + 1, updated_at = CURRENT_TIMESTAMP WHERE id = ?"` and a `queryBumpPolicyVersionByRule = "UPDATE policies SET version = version + 1, updated_at = CURRENT_TIMESTAMP WHERE id = (SELECT policy_id FROM rules WHERE id = ?)"`.
  - `Create`: after the insert, run the bump with `rule.PolicyID` inside the existing transaction.
  - `Update`: wrap the existing `Exec` in `WithTx`, run the bump by `rule.PolicyID` in the same transaction, and keep the rows-affected check so a missing rule rolls back.
  - `Remove`: wrap in `WithTx`, run `queryBumpPolicyVersionByRule` before the delete, then the existing delete and rows-affected check.
- [ ] **Step 4: Run the tests.** Run: `go test ./internal/orchestrator/store/ -v`. Expected: PASS, with existing store tests unchanged.

---

### Task 2: Store queries for the guards

**Files:**
- Modify: `internal/orchestrator/domain/rule.go` (`RuleStore.CountByPolicy`), `internal/orchestrator/domain/group.go` (`GroupStore.IsPolicyLinked`)
- Modify: `internal/orchestrator/store/rule_store.go`, `internal/orchestrator/store/group_store.go`
- Test: `internal/orchestrator/store/rule_store_test.go`, `internal/orchestrator/store/group_store_test.go`

**Interfaces:**
- Produces:
  - `RuleStore.CountByPolicy(ctx context.Context, policyID int64) (int64, error)`
  - `GroupStore.IsPolicyLinked(ctx context.Context, policyID int64) (bool, error)`

**Steps:**

- [ ] **Step 1: Write the failing tests:**
  - `TestRuleStore_CountByPolicy_WithSeededRules_ReturnsCount`
  - `TestRuleStore_CountByPolicy_WithoutRules_ReturnsZero`
  - `TestGroupStore_IsPolicyLinked_WithLinkedPolicy_ReturnsTrue`
  - `TestGroupStore_IsPolicyLinked_WithUnlinkedPolicy_ReturnsFalse`
- [ ] **Step 2: Run them to see them fail.** Run: `go test ./internal/orchestrator/store/ -run 'CountByPolicy|IsPolicyLinked' -v`. Expected: FAIL (methods missing).
- [ ] **Step 3: Implement.** `SELECT COUNT(*) FROM rules WHERE policy_id = ?` for the count. `SELECT EXISTS(SELECT 1 FROM groups WHERE policy_id = ?)` for the linked check. Declare both methods in the domain interfaces with doc comments, and the store methods without.
- [ ] **Step 4: Run the tests.** Expected: PASS.

---

### Task 3: No-op rule updates are skipped

**Files:**
- Modify: `internal/orchestrator/service/rule_service.go` (`UpdateRule`)
- Test: `internal/orchestrator/service/rule_service_test.go`

**Interfaces:**
- Consumes: `domain.Rule.IsEqual`, `domain.Rule.Patch`.

**Steps:**

- [ ] **Step 1: Write the failing test** `TestRuleService_UpdateRule_WithUnchangedValues_DoesNotCallStoreUpdate`. The mock store records update calls; the test asserts none were made and that the returned rule equals the stored one.
- [ ] **Step 2: Run it to see it fail.** Expected: FAIL (update is called).
- [ ] **Step 3: Implement.** Copy the rule before `Patch`; after `Validate`, if the patched rule `IsEqual` the copy, return the mapped copy without calling the store.
- [ ] **Step 4: Run the tests.** Run: `go test ./internal/orchestrator/service/ -run UpdateRule -v`. Expected: PASS.

---

### Task 4: Empty-policy guards in the services

**Files:**
- Modify: `internal/common/errors/policy.go` (`ErrPolicyHasNoRules`)
- Modify: `internal/orchestrator/service/group_service.go` (`LinkPolicy`; constructor takes `domain.RuleStore`)
- Modify: `internal/orchestrator/service/rule_service.go` (`RemoveRule`; constructor takes `domain.GroupStore`)
- Test: `internal/orchestrator/service/group_service_test.go`, `internal/orchestrator/service/rule_service_test.go`
- Modify: mocks as needed in `mock_*_test.go`

**Interfaces:**
- Produces:
  - `errors.ErrPolicyHasNoRules`
  - `NewGroupService(store domain.GroupStore, machineStore domain.MachineStore, policyStore domain.PolicyStore, ruleStore domain.RuleStore, stalenessGrace time.Duration, now func() time.Time) *GroupService`
  - `NewRuleService(store domain.RuleStore, policyStore domain.PolicyStore, groupStore domain.GroupStore) *RuleService`

**Steps:**

- [ ] **Step 1: Write the failing tests:**
  - `TestGroupService_LinkPolicy_WithPolicyWithoutRules_ReturnsErrPolicyHasNoRules`: the link store method is not called.
  - `TestGroupService_LinkPolicy_WithPolicyWithRules_LinksPolicy`
  - `TestRuleService_RemoveRule_LastRuleOfLinkedPolicy_ReturnsErrPolicyHasNoRules`: the remove store method is not called.
  - `TestRuleService_RemoveRule_LastRuleOfUnlinkedPolicy_RemovesRule`
  - `TestRuleService_RemoveRule_RuleAmongSeveralInLinkedPolicy_RemovesRule`
- [ ] **Step 2: Run them to see them fail.** Expected: FAIL (constructors and guards missing).
- [ ] **Step 3: Implement.**
  - `LinkPolicy`: after resolving the policy, `CountByPolicy`; `0` returns `ErrPolicyHasNoRules`.
  - `RemoveRule`: after resolving the rule, `IsPolicyLinked(rule.PolicyID)`; if linked, `CountByPolicy`; if `<= 1`, return `ErrPolicyHasNoRules`. Otherwise remove.
  - Update the constructors and the `router.go` call sites.
- [ ] **Step 4: Run the tests.** Run: `go test ./internal/orchestrator/service/ -v`. Expected: PASS.

---

### Task 5: Handler status mapping and wiring

**Files:**
- Modify: `internal/orchestrator/api/handler/group_handler.go`, `internal/orchestrator/api/handler/rule_handler.go` (map `ErrPolicyHasNoRules` to `409`)
- Modify: `internal/orchestrator/api/router.go` (pass the new stores)
- Test: `internal/orchestrator/api/handler/group_handler_test.go`, `internal/orchestrator/api/handler/rule_handler_test.go`

**Steps:**

- [ ] **Step 1: Write the failing tests** asserting only the status:
  - `TestGroupHandler_LinkPolicy_WithPolicyWithoutRules_Returns409`
  - `TestRuleHandler_RemoveRule_LastRuleOfLinkedPolicy_Returns409`
- [ ] **Step 2: Run them to see them fail.** Expected: FAIL.
- [ ] **Step 3: Implement** the `errMap` entries in both handlers (the same pattern used for existing conflict errors) and the `router.go` wiring.
- [ ] **Step 4: Run the tests.** Run: `go test ./internal/orchestrator/api/... -v`. Expected: PASS.

---

### Task 6: Gate and documentation

**Files:**
- Modify: `docs/superpowers/specs/2026-10-29-us08-policy-group-link-design.md` (follow-up note: linking requires rules)

**Steps:**

- [ ] **Step 1: Add the follow-up note** to the US08 spec's scope section.
- [ ] **Step 2: Run the full unit gate.** Run: `make tests`. Expected: PASS.
- [ ] **Step 3: Run the coverage gate.** Run: `make coverage`. Expected: total coverage ≥ 75%.
- [ ] **Step 4: Do not commit code changes automatically.** Commit only when the human partner requests it.

---

## Review Focus

1. **Remove of a rule that does not exist** must not bump any policy version. Test: `TestRuleStore_Remove_WithInexistentID_LeavesVersionUnchanged` (Task 1).
2. **Update of a rule that does not exist** must roll back the bump. Test: `TestRuleStore_Update_WithInexistentID_LeavesVersionUnchanged` (Task 1).
3. **Relinking a group to its current policy when that policy has no rules** is rejected, not treated as a no-op. Test: add `TestGroupService_LinkPolicy_CurrentPolicyWithoutRules_ReturnsErrPolicyHasNoRules` (Task 4).
4. **Removing the last rule of an unlinked policy** remains allowed. Test: `TestRuleService_RemoveRule_LastRuleOfUnlinkedPolicy_RemovesRule` (Task 4).
5. **Updating a rule with identical values** must not bump the version. Test: `TestRuleService_UpdateRule_WithUnchangedValues_DoesNotCallStoreUpdate` (Task 3).
