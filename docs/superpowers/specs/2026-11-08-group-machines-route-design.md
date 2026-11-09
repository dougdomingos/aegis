# GET /groups/{name}/machines: design

Status: approved scope, not implemented. Origin: R3 in `orchestrator-api-proposal.md`.

## Intent

The admin UI's lab detail page fetches every machine with `GET /machines` and filters by group in the browser. With many labs this moves all machines to show one lab. The Orchestrator already knows which machines belong to a group, so it should answer directly. The same list feeds the Monitor panel's per-machine status grid and its online/failed/offline counts.

## Goals

- `GET /groups/{name}/machines` returns the machines of one group as `[]MachineOutputSchema`, each including its latest `propagation` object, exactly as `GET /machines` does today.
- An existing group with no machines returns `200 []`.
- An unknown group returns `404` with the existing `Error` body.
- It is the read counterpart of the existing `POST /groups/{name}/machines` and `DELETE /groups/{name}/machines/{machineID}`.

## Non-goals

- No pagination, filtering or sorting parameters.
- No new store method, no migration and no new schema type.
- No change to `Group`, `Machine` or `GET /machines`.
- No lookup by group ID (the proposal's Q5 stays answered as "name", like every other `/groups/{name}/...` route).

## Anti-goals

- The response must never differ from `GET /machines` for the same machine. A machine without any propagation reports the PENDING representation, not a missing `propagation` field.
- The route must not read every machine and filter in memory when the store can filter.

## Constraints

- Layering per `AGENTS.md`: handler, then service, then store, each depending only on interfaces. Interfaces are owned by their consumer, so the method is added to `GroupServiceInterface` in `api/handler/group_handler.go`.
- Existing store method `domain.MachineStore.ListByGroup(ctx, groupID)` is reused.
- Group name validation matches `LinkMachine`: a blank name returns `ErrGroupNameRequired` (400), a missing group returns `ErrGroupNotFound` (404).

## Decisions

1. **Where the logic lives.** The method is `GroupService.ListGroupMachines`. `GroupService` has the group and machine stores but not the propagation store, so it gains a `propagations domain.PropagationStore` dependency. This changes `NewGroupService` and its four test arrange helpers. The alternative, putting the method on `MachineService`, was rejected because that service has no group store and would need one just as much.
2. **Latest propagation.** The service calls `PropagationStore.ListLatest` once and indexes it by machine ID, then maps with the existing `mapMachineToOutputSchema`, `mapPropagationToOutputSchema` and `pendingPropagationOutput`. This is the same approach as `MachineService.ListMachines`. The duplicated indexing loop is small, so no shared helper is extracted (YAGNI; revisit if a third caller appears).
3. **Empty result.** The service returns a non-nil empty slice, and the handler also guards against nil, like `GroupHandler.List`.

## Left to the builder

- Exact ordering of the returned machines. Keep the order `ListByGroup` returns.
- Test fixture contents.
