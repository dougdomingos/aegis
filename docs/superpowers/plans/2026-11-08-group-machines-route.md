# GET /groups/{name}/machines Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `GET /groups/{name}/machines`, returning the machines of one group with their latest propagation.

**Architecture:** Vertical slice, store then service then handler. The store already has `ListByGroup`. The service gains the propagation store so it can attach the latest propagation. The handler adds one route.

**Tech Stack:** Go 1.26, chi, SQLite. Tests are plain `testing` with mocks.

**Spec:** `docs/superpowers/specs/2026-11-08-group-machines-route-design.md`

## Global Constraints

- Follow `AGENTS.md`: every symbol has a doc comment starting with its name (interface methods are documented on the interface only, not on implementations); tests live in external packages (`service_test`, `handler_test`) and are named `Test<Type>_<Method>_<Scenario>_<ExpectedResult>`; arrange helpers sit at the end of the file; assertions use `t.Errorf` or `t.Fatalf`.
- Write each test file in two steps: the skeleton with signatures first, then the bodies.
- Handler tests assert the HTTP status code only.
- Do not commit. Leave changes in the working tree for developer review (`AGENTS.md` 6.1). Run `make tests` only, not the git-hook commands.
- Error mapping: blank group name gives 400 (`ErrGroupNameRequired`), unknown group gives 404 (`ErrGroupNotFound`).

## Review Focus

- A group with no machines returns `[]`, never `null` (test in Task 1 and Task 2).
- A machine in the group with no propagation row reports the PENDING representation (test in Task 1).
- A machine in another group is never returned (test in Task 1).
- Group name with surrounding whitespace only (`"  "`) returns 400, not 404 (test in Task 1).

---

### Task 1: Service method `ListGroupMachines`

**Files:**
- Modify: `internal/orchestrator/service/group_service.go` (struct fields near line 12-40, `NewGroupService` at line 47, new method after `UnlinkMachine`)
- Modify: `internal/orchestrator/service/group_service_test.go` (arrange helpers at lines 755-830 and new tests)
- Modify: `internal/orchestrator/api/router.go:80` (constructor call)

**Interfaces:**
- Consumes: `domain.PropagationStore.ListLatest(ctx) ([]domain.Propagation, error)`, `domain.MachineStore.ListByGroup(ctx, groupID int64) ([]domain.Machine, error)`, package-private `mapMachineToOutputSchema(*domain.Machine, time.Time, time.Duration) *schemas.MachineOutputSchema`, `mapPropagationToOutputSchema(*domain.Propagation) schemas.PropagationOutputSchema`, `pendingPropagationOutput() schemas.PropagationOutputSchema` (all in `service/machine_service.go`).
- Produces: `func (service *GroupService) ListGroupMachines(ctx context.Context, groupName string) ([]schemas.MachineOutputSchema, error)` and the new constructor signature `NewGroupService(store domain.GroupStore, machineStore domain.MachineStore, policyStore domain.PolicyStore, ruleStore domain.RuleStore, propagations domain.PropagationStore, notifier PropagationNotifier, stalenessGrace time.Duration, now func() time.Time) *GroupService`.

- [ ] **Step 1: Write the test skeleton** in `group_service_test.go`: five empty test functions with these names, and an `arrangeGroupListMachinesServiceTest(t)` helper at the end of the file returning `(context.Context, *MockGroupStore, *MockMachineStore, *MockPropagationStore, service.GroupService)`.
  - `TestGroupService_ListGroupMachines_WithMachinesInGroup_ReturnsOnlyThoseMachines`
  - `TestGroupService_ListGroupMachines_WithEmptyGroup_ReturnsEmptySlice`
  - `TestGroupService_ListGroupMachines_WithMachineWithoutPropagation_ReturnsPendingPropagation`
  - `TestGroupService_ListGroupMachines_WithInexistentGroup_ReturnsErrGroupNotFound`
  - `TestGroupService_ListGroupMachines_WithBlankName_ReturnsErrGroupNameRequired`

- [ ] **Step 2: Update the existing four arrange helpers** (lines 764-830) and add the new one so each passes a `NewMockPropagationStore()` to `NewGroupService` as the fifth argument. Run `go build ./... ` and confirm it fails only because the signature is not yet changed.

- [ ] **Step 3: Fill in the test bodies.** Seed a group, two machines linked to it, one machine in another group, and a `Propagation` row for one of the two linked machines (status ACKED, `SentAt` and `AckedAt` 120 ms apart). Assertions: result length is 2 and no ID from the other group appears; the machine with a row has `Propagation.Status == "ACKED"` and `*Propagation.DurationMS == 120`; the machine without a row has `Propagation.Status` equal to `pendingPropagationOutput().Status` ("PENDING"); the empty group returns a non-nil slice of length 0; the error cases use `errors.Is`.

- [ ] **Step 4: Run to confirm failure.** Run `go test ./internal/orchestrator/service -run TestGroupService_ListGroupMachines -v`. Expected: build failure on the missing method and constructor.

- [ ] **Step 5: Implement.** Add the field `propagations domain.PropagationStore` with a doc comment, extend `NewGroupService` to the signature above, update `router.go:80` to pass `propagationStore`, and implement `ListGroupMachines`: trim-check the name, load the group by name, `ListByGroup`, `ListLatest` indexed by machine ID, map each machine as `MachineService.ListMachines` does (lines 89-104 of `machine_service.go`).

- [ ] **Step 6: Run the service package.** Run `go test ./internal/orchestrator/service ./internal/orchestrator/api -v`. Expected: PASS.

### Task 2: Handler route `GET /groups/{name}/machines`

**Files:**
- Modify: `internal/orchestrator/api/handler/group_handler.go` (interface at lines 15-26, `RegisterRoutes` at lines 40-51, new method)
- Modify: `internal/orchestrator/api/handler/mock_group_service_test.go` (add `listMachinesFn` and its method)
- Modify: `internal/orchestrator/api/handler/group_handler_test.go`

**Interfaces:**
- Consumes: `GroupService.ListGroupMachines(ctx, groupName string) ([]schemas.MachineOutputSchema, error)` from Task 1.
- Produces: `func (handler *GroupHandler) ListMachines(w http.ResponseWriter, r *http.Request)` registered as `r.Get("/{name}/machines", handler.ListMachines)`.

- [ ] **Step 1: Write the test skeleton** with these names, then add `listMachinesFn` and `ListGroupMachines` to `mockGroupService`:
  - `TestGroupHandler_ListMachines_WithMachines_ReturnsStatus200`
  - `TestGroupHandler_ListMachines_WithEmptyGroup_ReturnsStatus200`
  - `TestGroupHandler_ListMachines_WithInexistentGroup_ReturnsStatus404`
  - `TestGroupHandler_ListMachines_WithBlankName_ReturnsStatus400`
  - `TestGroupHandler_ListMachines_WithServiceFailure_ReturnsStatus500`

- [ ] **Step 2: Fill in the bodies.** Each overrides `listMachinesFn` to return the scenario's result (a one-machine slice built with `machineOutput(...)` from `mock_machine_service_test.go`, a nil slice, `errors.ErrGroupNotFound`, `errors.ErrGroupNameRequired`, a generic error) and asserts the status with `assertStatusCode`. Serve through `chi` with `NewGroupHandler(mock).RegisterRoutes`, as the neighboring tests do.

- [ ] **Step 3: Run to confirm failure.** Run `go test ./internal/orchestrator/api/handler -run TestGroupHandler_ListMachines -v`. Expected: build failure (method missing from the interface).

- [ ] **Step 4: Implement.** Add `ListGroupMachines` to `GroupServiceInterface`; implement `ListMachines` like `GetByName`: read `chi.URLParam(r, "name")`, call the service, map `ErrGroupNameRequired` to 400 and `ErrGroupNotFound` to 404 with `utils.MapByError`, replace a nil result with `[]schemas.MachineOutputSchema{}`, reply 200 with `utils.EncodeToJSON`. Register the route.

- [ ] **Step 5: Verify end to end.** Add `TestNewRouter_WithGroupMachines_ReturnsLinkedMachines` to `internal/orchestrator/api/router_test.go`: build the router as the existing test does, create a group over HTTP, link the seeded machine, then `GET /groups/<name>/machines` expects 200 and one machine; an unknown group expects 404. Run `make tests`. Expected: all packages PASS.
