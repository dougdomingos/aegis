# US08 — Policy-to-Group Linking Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Persist a one-policy-per-group association and expose group-centric endpoints to link and unlink policies.

**Architecture:** Add a nullable `policy_id` FK to `groups` (SQLite `ON DELETE SET NULL`), extend the group store with link/unlink operations that join the policy name, add link/unlink plus `PolicyID/PolicyName` to `GroupOutputSchema`, then expose `POST /groups/{name}/policy` and `DELETE /groups/{name}/policy`.

**Tech Stack:** Go 1.26, chi, SQLite (modernc.org/sqlite), net/http.

**Spec:** `docs/superpowers/specs/2026-10-29-us08-policy-group-link-design.md`

## Global Constraints

- Module path: `dougdomingos.com/aegis`; Go 1.26.5.
- Implement in vertical-slice order: Store → Service → API Handler. Each layer must build and pass its tests before the next.
- Existing migrations must not be modified. Add the new field through `006_add_policy_id_to_groups.sql`.
- Test files are external packages; test names follow `Test<Type>_<Method>_<Scenario>_<ExpectedResult>`.
- New exports need doc comments starting with the symbol name; interface implementations do not repeat interface doc comments.
- **Do not commit.** AGENTS.md forbids automatic commits; leave changes for developer review. Git-commit steps are omitted.
- Run `go test` on touched packages after each task; run `make tests` before handing off.

## Review Focus

1. Unlinking a group with no policy must return an error and not clear anything else.
2. Relinking a group to a new policy must replace the old one (single `policy_id`).
3. Removing a group must leave its policy persisted.
4. A group without a policy must serialize `policy_id` as `null` and an empty `policy_name`.
5. A missing/non-existent policy name must return `404`, never panic.

---

### Task 1: Persist the Group→Policy Association

**Files:**
- Create: `internal/orchestrator/infra/migrations/sql/006_add_policy_id_to_groups.sql`
- Modify: `internal/orchestrator/domain/group.go`
- Modify: `internal/orchestrator/store/group_store.go`
- Modify: `internal/orchestrator/service/mock_group_store_test.go`
- Test: `internal/orchestrator/store/group_store_test.go`

**Interfaces:**
- Consumes: existing `domain.PolicyStore`, `query.QueryExecutor`, `arrangeStoreDB`.
- Produces:
  - `domain.Group.PolicyID *int64`, `domain.Group.PolicyName string`
  - `GroupStore.LinkToPolicy(ctx, groupName string, policyID int64) (*Group, error)`
  - `GroupStore.UnlinkFromPolicy(ctx, groupName string) (*Group, error)`

- [ ] **Step 1: Create the migration**

`internal/orchestrator/infra/migrations/sql/006_add_policy_id_to_groups.sql`:
```sql
ALTER TABLE groups ADD COLUMN policy_id INTEGER REFERENCES policies(id) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS idx_groups_policy_id ON groups(policy_id);
```

- [ ] **Step 2: Add domain fields and store methods**

In `domain/group.go`, add to `Group` after `CreatedAt`:
```go
PolicyID   *int64
PolicyName string
```
Add to `GroupStore` interface:
```go
LinkToPolicy(ctx context.Context, groupName string, policyID int64) (*Group, error)
UnlinkFromPolicy(ctx context.Context, groupName string) (*Group, error)
```

- [ ] **Step 3: Write failing store tests** then implement; follow US07 pattern (arrange helpers, LEFT JOIN policies). Run `go test ./internal/orchestrator/store/`.

### Task 2: Group Service Link/Unlink and Policy Exposure

**Files:**
- Modify: `internal/common/schemas/group.go`
- Modify: `internal/orchestrator/service/group_service.go`
- Modify: `internal/orchestrator/service/group_service_test.go`
- Modify: `internal/common/errors/group.go` (if needed, add `ErrGroupNotLinkedToPolicy`)

**Interfaces:**
- Consumes: Task 1's store methods.
- Produces:
  - `schemas.LinkPolicyToGroupSchema{ PolicyName string }`
  - `schemas.GroupOutputSchema.PolicyID *int64`, `.PolicyName string`
  - `GroupServiceInterface.LinkPolicy(ctx, groupName, policyName string) (*schemas.GroupOutputSchema, error)`
  - `GroupServiceInterface.UnlinkPolicy(ctx, groupName string) (*schemas.GroupOutputSchema, error)`

- [ ] **Step 1: Update schemas** — add `PolicyID *int64`, `PolicyName string` to `GroupOutputSchema`; add `LinkPolicyToGroupSchema{ PolicyName string `json:"policy_name"` }`.
- [ ] **Step 2: Write failing service tests** for LinkPolicy/UnlinkPolicy (valid, missing group, missing policy, relink, unlink when not linked).
- [ ] **Step 3: Implement** `LinkPolicy` (resolve group & policy by name; set policy_id; return mapped schema) and `UnlinkPolicy` (resolve group; if policy_id nil → not-linked error; clear; return).
- [ ] **Step 4: Populate PolicyID/PolicyName** in `mapGroupToOutputSchema` via LEFT JOIN (store already joins).
- [ ] **Step 5: Run `go test ./internal/orchestrator/service/`.**

### Task 3: Link/Unlink HTTP Endpoints

**Files:**
- Modify: `internal/orchestrator/api/handler/group_handler.go`
- Modify: `internal/orchestrator/api/handler/mock_group_service_test.go`
- Test: `internal/orchestrator/api/handler/group_handler_test.go`

**Interfaces:**
- Consumes: Task 2's service methods and `schemas.LinkPolicyToGroupSchema`.
- Produces:
  - `POST /groups/{name}/policy`
  - `DELETE /groups/{name}/policy`

- [ ] **Step 1: Write failing handler tests** (200, 404 missing group/policy, 400 malformed body, 400 invalid path not applicable since no id).
- [ ] **Step 2: Extend mock** with `linkPolicyFn`/`unlinkPolicyFn`.
- [ ] **Step 3: Add to `GroupServiceInterface`** and `RegisterRoutes`: `r.Post("/{name}/policy", h.LinkPolicy)`, `r.Delete("/{name}/policy", h.UnlinkPolicy)`.
- [ ] **Step 4: Implement handlers** `LinkPolicy` (decode body, call service, map errors) and `UnlinkPolicy`.
- [ ] **Step 5: Run `go test ./internal/orchestrator/api/handler/` and `make tests`.**
