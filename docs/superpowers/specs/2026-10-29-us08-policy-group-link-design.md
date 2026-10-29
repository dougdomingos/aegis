# US08 — Vinculação de Políticas a Grupos — Design

- **Issue:** [#18](https://github.com/dougdomingos/aegis/issues/18)
- **Status:** Approved design, pending implementation plan
- **Date:** 2026-10-29

## 1. Context & Goal

Groups currently have no associated policy. US08 lets an administrator link
an already-created policy to an existing group, enforce one-policy-per-group,
unlink, and keep policies persisted when their group is removed.

Acceptance criteria (from the issue):

1. Link a policy to a group by group name + policy name.
2. A group cannot be linked to more than one policy at a time.
3. Unlink a policy from a group.
4. Removing a group keeps previously associated policies persisted.

## 2. Scope

**In scope:**

- A nullable `policy_id` foreign key on the `groups` table (migration).
- Store operations to link/unlink a group to a policy.
- Service + handler endpoints `POST /groups/{name}/policy` and
  `DELETE /groups/{name}/policy`.
- Exposing `PolicyID *int64` and `PolicyName string` on `GroupOutputSchema`.

**Out of scope:**

- Bulk linking/unlinking.
- Rule/policy propagation consequences (later stories).
- UI.

## 3. Decisions

| Decision | Choice | Rationale |
| --- | --- | --- |
| Storage model | Nullable `policy_id` column on `groups` with FK `ON DELETE SET NULL` | A group has at most one policy (criterion 2), so a column suffices; SET NULL is harmless since policies are top-level and persist regardless. |
| Linking semantics | Allow relinking (replace existing policy) | Simpler admin UX; matches US07 relink decision. |
| Lookup | By name for both group and policy | Per issue. |
| Endpoint shape | `POST /groups/{name}/policy` body `{policy_name}`; `DELETE /groups/{name}/policy` | Group-centric, consistent with US07. |
| Output exposure | `GroupOutputSchema.PolicyID *int64` and `PolicyName string` | Mirrors US07's Machine/Group exposure. |

## 4. Migration

`006_add_policy_id_to_groups.sql`:

```sql
ALTER TABLE groups ADD COLUMN policy_id INTEGER REFERENCES policies(id) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS idx_groups_policy_id ON groups(policy_id);
```

## 5. Domain Changes

- `Group` gains `PolicyID *int64`.
- `GroupStore` gains:
  - `LinkToPolicy(ctx, groupName string, policyID int64) (*Group, error)` — sets `policy_id`.
  - `UnlinkFromPolicy(ctx, groupName string) (*Group, error)` — clears `policy_id`.
- `PolicyStore` unchanged.

## 6. Service Changes

`GroupService` owns linking (group-centric endpoint) and depends on both stores (already does).

`GroupServiceInterface` gains:
- `LinkPolicy(ctx, groupName, policyName string) (*GroupOutputSchema, error)`
- `UnlinkPolicy(ctx, groupName string) (*GroupOutputSchema, error)`

Logic:
- **Link:** resolve group by name (not found → error); resolve policy by name (not found → error); set `group.policy_id = policy.ID`; return mapped schema.
- **Unlink:** resolve group by name (not found → error); if `group.policy_id == nil` → not-linked error; otherwise clear and return.

## 7. Handler and Routes

- `GroupHandler` gains `LinkPolicy` and `UnlinkPolicy`.
- Routes:
  - `r.Post("/groups/{name}/policy", h.LinkPolicy)`
  - `r.Delete("/groups/{name}/policy", h.UnlinkPolicy)`
- Request body: `LinkPolicyToGroupSchema{ PolicyName string }`.
- Responses: `200` with `GroupOutputSchema`; `404` for missing group/policy; `400` for malformed body.

## 8. Schemas

- `internal/common/schemas`: add `LinkPolicyToGroupSchema{ PolicyName string }`.
- `GroupOutputSchema` add `PolicyID *int64` and `PolicyName string`.

## 9. Store Joins

Store populates `PolicyID` and `PolicyName` via a `LEFT JOIN policies` on `List`, `GetByName`, `GetByID`.

## 10. Testing

Follow vertical slice: store → service → handler.