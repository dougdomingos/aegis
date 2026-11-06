# US07 — Vinculação de Máquinas a Grupos — Design

- **Issue:** [#17](https://github.com/dougdomingos/aegis/issues/17)
- **Status:** Approved design, pending implementation plan
- **Date:** 2026-10-29

## 1. Context & Goal

Aegis groups machines into logical laboratories (groups) so an administrator
can manage sets of machines at once. Today groups and machines exist
independently: there is no association between a machine and a group.

US07 lets an administrator link an already-registered (possibly disconnected)
machine to an existing group, enforce one-group-per-machine, unlink, and keep
machines authenticated when their group is removed.

Acceptance criteria (from the issue):

1. Link a machine to a group by group name + machine ID, even if disconnected.
2. No duplicate machines within the same group.
3. A machine cannot belong to multiple groups simultaneously.
4. Unlink a machine from its group.
5. Removing a group leaves previously associated machines authenticated.

## 2. Scope

**In scope:**

- A nullable `group_id` foreign key on the `machines` table (migration).
- Store operations to link/unlink a machine to a group, and to clear
  `group_id` on group removal.
- Service + handler endpoints `POST /groups/{name}/machines` and
  `DELETE /groups/{name}/machines/{machine_id}`.
- Exposing `group_id`/`group_name` on `MachineOutputSchema` and machine IDs on
  `GroupOutputSchema`.

**Out of scope:**

- Bulk linking/unlinking.
- Rule/policy propagation consequences of grouping (later stories).
- UI.

## 3. Decisions

| Decision | Choice | Rationale |
| --- | --- | --- |
| Storage model | Nullable `group_id` column on `machines` with FK `ON DELETE SET NULL` | A machine belongs to at most one group (criterion 3), so a column is enough; SET NULL satisfies criterion 6 automatically. |
| Linking semantics | Allow moving a machine between groups (relink) instead of erroring | Per partner preference; simpler admin UX. |
| Lookup | By machine ID only | Per issue. |
| Endpoint shape | `POST /groups/{name}/machines` body `{machine_id}`; `DELETE /groups/{name}/machines/{machine_id}` | Matches issue wording and existing REST style. |
| Output exposure | `MachineOutputSchema.GroupID *int64` and `GroupName string`; `GroupOutputSchema.MachineIDs []int64` | Aids management; nullable pointer distinguishes "no group". |

## 4. Migration

`005_add_group_id_to_machines.sql`:

```sql
ALTER TABLE machines ADD COLUMN group_id INTEGER REFERENCES groups(id) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS idx_machines_group_id ON machines(group_id);
```

SQLite supports `ALTER TABLE ... ADD COLUMN` with FK references; FK enforcement
relies on `PRAGMA foreign_keys = ON` (already set in infra).

## 5. Domain Changes

- `Machine` gains `GroupID *int64`.
- `MachineStore` gains:
  - `LinkToGroup(ctx context.Context, machineID, groupID int64) (*Machine, error)` —
    sets `group_id`, returning the updated machine; fails if the machine or
    group does not exist.
  - `UnlinkFromGroup(ctx context.Context, machineID int64) (*Machine, error)` —
    clears `group_id`, returning the updated machine.
  - `ListByGroup(ctx context.Context, groupID int64) ([]Machine, error)` — for
    group output.
- `GroupStore.Remove` is unchanged; the FK `ON DELETE SET NULL` clears links.

## 6. Service Changes

`GroupService` owns linking (the endpoint is group-centric) and depends on both
stores.

`GroupServiceInterface` gains:

- `LinkMachine(ctx context.Context, groupName string, machineID int64) (*MachineOutputSchema, error)`
- `UnlinkMachine(ctx context.Context, groupName string, machineID int64) (*MachineOutputSchema, error)`

Logic:

- **Link:** resolve group by name (not found → error); resolve machine by ID
  (not found → error); set `machine.group_id = group.ID` (allows moving between
  groups); return the updated machine mapped to a schema.
- **Unlink:** resolve group by name (not found → error); resolve machine by ID
  (not found → error); if `machine.group_id != group.ID` → not-linked error;
  otherwise clear `group_id` and return the updated machine.

## 7. Handler and Routes

- `GroupHandler` gains `LinkMachine` and `UnlinkMachine` methods.
- Routes:
  - `r.Post("/groups/{name}/machines", h.LinkMachine)`
  - `r.Delete("/groups/{name}/machines/{machineID}", h.UnlinkMachine)`
- Request body: `LinkMachineToGroupSchema{ MachineID int64 }`.
- Responses: `200` with `MachineOutputSchema`; `404` for missing group/machine;
  `400` for a malformed body.

## 8. Schemas

- `internal/common/schemas`: add `LinkMachineToGroupSchema{ MachineID int64 }`.
- `MachineOutputSchema` add `GroupID *int64` and `GroupName string`.
- `GroupOutputSchema` add `MachineIDs []int64`.

## 9. Store Joins

Store populates `GroupID` and `GroupName` via a `LEFT JOIN groups` on
`List`, `GetByIP`, and the new `ListByGroup`; `ListByGroup` additionally filters
`WHERE machines.group_id = ?`.

## 10. Testing

Follow the vertical slice: store → service → handler, reusing existing arrange
helpers. Add arrange helpers for the linked state.
