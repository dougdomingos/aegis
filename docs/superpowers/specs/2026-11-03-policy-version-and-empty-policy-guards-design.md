# Policy Version Bump and Empty-Policy Guards — Design

- **Issue:** None yet (prerequisite for [#19](https://github.com/dougdomingos/aegis/issues/19))
- **Status:** Draft, pending review
- **Date:** 2026-11-03
- **Plan:** [`docs/superpowers/plans/2026-11-03-policy-version-and-empty-policy-guards.md`](../plans/2026-11-03-policy-version-and-empty-policy-guards.md)

## 1. Context & Goal

Three behaviors of the current code must change before US09 (#19) can build on
them:

1. **Rule changes do not bump the policy version.** `domain.Policy.Version` is
   documented as "incremented on every actual change", and a policy rename
   increments it (`queryUpdatePolicyByID`). Rule create, update and remove
   leave it untouched. A rule change is an actual change to the policy, so the
   version is currently wrong.
2. **A policy with no rules can be linked to a group.** US08 never checked for
   rules. US09 would then propagate an empty package, which serves no purpose.
3. **The last rule of a linked policy can be removed.** The linked group would
   be left with a policy that has nothing to enforce.

This design fixes all three. It is a separate branch because it changes shipped
behavior (US02, US03 and US08) and should be reviewed on its own.

## 2. Scope

**In scope:**

- Rule create, update and remove increment the owning policy's `version` and
  `updated_at`, in the same transaction as the rule change.
- A rule update that changes nothing does not touch the rule, the version or
  `updated_at`.
- Linking a policy with no rules to a group is rejected with `409`.
- Removing the last rule of a policy that is linked to a group is rejected with
  `409`.

**Out of scope:**

- Propagating changes to machines (US09, #19).
- Revoking machines when a policy is unlinked or removed (US09).
- Changing a policy's type (types stay immutable).
- Retroactive cleanup of existing empty linked policies in any existing
  database.

## 3. Decisions

| Decision | Choice | Rationale |
| --- | --- | --- |
| Where the bump happens | Inside the rule store, in the same transaction as the rule change | A rule change and its version bump must commit together. A failed rule write must not leave the version incremented, and vice versa. |
| Bump statement | `UPDATE policies SET version = version + 1, updated_at = CURRENT_TIMESTAMP WHERE id = ?` | Matches the existing policy update, so both paths bump the version identically. |
| Remove | The bump uses `WHERE id = (SELECT policy_id FROM rules WHERE id = ?)` before the delete, in the same transaction | `Remove` receives only the rule ID, and the rule must still exist when the policy is resolved. If the rule does not exist the subquery is `NULL`, nothing changes, and the removal fails with the existing error. |
| No-op update | The service skips the store call when the patched rule equals the stored one (`Rule.IsEqual`) | Mirrors the policy rename no-op, which keeps the version unchanged. |
| Empty-link guard | `GroupService.LinkPolicy` rejects a policy whose `CountByPolicy` is `0` with `ErrPolicyHasNoRules` | The check belongs in the service, next to the existing policy lookup. |
| Last-rule guard | `RuleService.RemoveRule` rejects removing a rule when `CountByPolicy` is `1` and the policy is linked, with `ErrLinkedPolicyLastRule` (a distinct error, so the 409 message states the real reason instead of the link-guard wording) | Unlinked policies may be emptied freely. Only linked policies need rules for propagation. |
| Linked check | New `GroupStore.IsPolicyLinked(ctx, policyID) (bool, error)` | `RuleService` needs a read-only answer and must not depend on the group's full entity. |
| Status code | `409 Conflict` | Same convention as the existing duplicate-name conflicts. The request is well formed; its effect conflicts with the current state. |

## 4. Store Changes

### 4.1 `RuleStore`

- `Create`: the existing transaction also runs the policy version bump.
- `Update`: moves from a bare `Exec` into a transaction, and bumps the owning
  policy's version in that transaction. A missing rule rolls back the bump.
- `Remove`: runs the subquery-based bump, then the delete, in one transaction.
- `CountByPolicy(ctx, policyID int64) (int64, error)`: counts the rules of a
  policy.

### 4.2 `GroupStore`

- `IsPolicyLinked(ctx, policyID int64) (bool, error)`: reports whether any group
  has `policy_id` equal to the policy.

No migration is needed. The `version` and `updated_at` columns already exist.

## 5. Service Changes

- `RuleService` gains a `domain.GroupStore` dependency (for `IsPolicyLinked`).
- `GroupService` gains a `domain.RuleStore` dependency (for `CountByPolicy`).
- `errors.ErrPolicyHasNoRules` is added to `internal/common/errors/policy.go`.
- `errors.ErrLinkedPolicyLastRule` (same file) is returned by the last-rule
  guard, so its message describes the removal being refused.
- `RuleService.UpdateRule` returns the stored rule unchanged when the patch
  produces an equal rule.
- `RuleService.RemoveRule` checks the guard before calling the store.
- `GroupService.LinkPolicy` checks the guard after resolving the policy and
  before linking.

Relinking a group to the policy it already has is still checked: a policy with
no rules cannot be linked even when it is already the group's policy. A group
that is already linked to a policy whose last rule disappears through another
path (for example, the policy is removed) is not handled here; that is US09.

## 6. Handler Changes

- `GroupHandler.LinkPolicy` maps `ErrPolicyHasNoRules` to `409`.
- `RuleHandler.RemoveRule` maps `ErrLinkedPolicyLastRule` to `409`.
- `router.go` passes the new dependencies to `NewGroupService` and
  `NewRuleService`.

## 7. Existing Data

Policies that already have no rules and are already linked to a group stay as
they are. The guards apply to new links and new removals only. The database
is pre-release, so no migration cleanup is planned; if a development database
contains such a link, it can be unlinked and relinked after rules are added.

## 8. Testing

Follows the vertical slice: store → service → handler.

- Store: `arrangeStoreTest`. Each rule mutation bumps the version and
  `updated_at`; a failed mutation leaves the version unchanged; `CountByPolicy`
  and `IsPolicyLinked` return the expected values.
- Service: mocks for the stores. A no-op update does not call the store; the
  linking guard rejects an empty policy; the removal guard rejects the last rule
  of a linked policy and allows it for an unlinked one.
- Handler: service mocks; status codes only.

Coverage is held to the project's 75% gate.

## 9. Documentation Updates

- The US08 spec gets a follow-up note that linking now requires rules.
- `AGENTS.md` is unchanged.

## 10. Open Questions

1. The issue number for the branch name. The repository's convention is
   `feat/issue-N-slug`; no issue exists for this prerequisite yet.
2. Should existing development databases be cleaned up, or is the guidance in
   §7 enough?
