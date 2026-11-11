# Machine registry import and removal: design

Status: approved scope, implemented on `feat/issue-12-machine-import`. Origin: issue #12.

## Intent

The Orchestrator knows which machines exist, but not which lab each one belongs to. The `AEGIS_MACHINES` CSV (`ip,name`) registers machines at every boot, so an admin then links every machine to its group by hand. Re-reading the CSV at boot also prunes machines and would overwrite group links made through the API.

The registry becomes an API concern. An admin uploads a CSV that also names each machine's lab. Each lab is matched to a group, and the machines are linked to it. Machines can also be removed through the API without restarting the service.

## Goals

- The CSV gains a third column. The header is exactly `ip,name,lab`.
- `POST /machines/import` accepts the CSV as the raw request body (`text/csv`).
- Each unique non-empty `lab` is matched to a group by name. A missing group is created.
- A machine with a non-empty `lab` is linked to that group, and is moved if it was linked elsewhere.
- `DELETE /machines/{id}` removes one machine.

## Non-goals

- No boot-time CSV load. `AEGIS_MACHINES` is removed.
- Import does not prune. Machines absent from the CSV are kept; `DELETE /machines/{id}` removes them.
- Import never deletes groups, even when they are left with no machines.
- No authentication on admin routes (none exist today).
- No change of a machine's IP. A new IP is a new machine (issue #12 criterion).
- No migration. `groups.name` is already unique, and `machines.group_id` already exists.

## Anti-goals

- An import must never overwrite group links made through the API when the CSV has no opinion. An empty `lab` cell leaves the machine's current link untouched.
- A malformed CSV must never leave the database half-imported.
- Import must never reset a machine's connection state, MAC or last-seen time. Only the name is refreshed.

## Decisions (made with the developer)

1. **REST only.** The CSV reaches the database only through `POST /machines/import`. Issue #12 proposed keeping the initial CSV load; the developer chose REST only instead.
2. **Missing lab creates the group.**
3. **Header required, empty lab allowed.**
4. **Absent machines are kept** (additive import).
5. **Empty lab keeps the current link.**
6. **Emptied groups are kept.**
7. **`DELETE /machines/{id}`** added as the way to remove machines.
8. **`@Accept text/csv`** on the import route, an explicit exception to the short-swag rule.

## Behavior

### `POST /machines/import`

- Body capped at 1 MiB with `http.MaxBytesReader`. Larger bodies return `413`.
- The whole document is parsed and validated before any write. Any invalid row returns `400` (`ErrInvalidMachinesCSV`) and nothing is persisted.
- Validation: header exactly `ip,name,lab` (fields trimmed); 3 fields per row; valid IP; IP unique within the document; non-empty name. Every field is trimmed. An empty lab is allowed.
- Writes run in one SQLite transaction:
  1. For each unique non-empty lab, `INSERT OR IGNORE` the group, then resolve its ID.
  2. For each row, upsert the machine by IP (name only).
  3. For each row with a non-empty lab whose `group_id` differs, set `group_id`.
- After commit, `PropagationNotifier.NotifyMachine` runs for every machine whose group changed, so its rules are pushed again. Unchanged links are not notified.
- `200` returns `MachineImportOutputSchema`: `{machines_created, machines_updated, groups_created, machines_linked}`, where `machines_linked` counts machines whose group changed.

### `DELETE /machines/{id}`

- `204` on success, `400` for a non-numeric ID, `404` for an unknown ID.
- The machine can no longer authenticate, and its heartbeats fail.
- It does not revoke rules already applied on that machine's Sentinel. Its propagation rows stay in the database (no foreign key), matching the old prune behavior.

## Layering

Vertical slice, per `AGENTS.md`:

- **Domain:** `MachineImport`, `MachineImportResult`; `MachineStore.Import` and `MachineStore.RemoveByID` added, `MachineStore.Sync` removed.
- **Store:** `Import` runs in `executor.WithTx`. It writes the `groups` table from the machine store so the whole import stays in one transaction.
- **Service:** `MachineService.ImportMachines(ctx, io.Reader)` parses the CSV (`service/machine_import.go`), calls the store and notifies. `MachineService.RemoveMachine(ctx, id)`. `NewMachineService` gains a nil-safe `PropagationNotifier` argument.
- **Handler:** `MachineHandler.Import` and `MachineHandler.Delete`; `MachineServiceInterface` gains both methods.
- **Config:** `AEGIS_MACHINES`, `config.Machine`, `Config.Machines` and `parseMachines` are removed; `router.go` loses `seedMachines`.

## Ops

- `.env.example` and `packaging/orchestrator/Dockerfile` no longer mention `AEGIS_MACHINES`.
- Container e2e harness: the registry is POSTed after the Orchestrator is up, with an empty lab column so each test links its own groups.
- VM `deploy.sh`: waits for `/machines`, then POSTs `machines.csv`. The template has an empty lab column for the same reason.

## Left to the builder

- Error message wording and line numbers in validation errors.
- How the store counts created groups.
