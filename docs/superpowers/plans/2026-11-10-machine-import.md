# Machine Registry Import and Removal Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the boot-time `AEGIS_MACHINES` sync with `POST /machines/import` (CSV `ip,name,lab`, labs matched or created as groups), and add `DELETE /machines/{id}`.

**Architecture:** Vertical slice, store then service then handler, then wiring, config and e2e. One transactional store method does the whole import. The service parses and validates the CSV and notifies propagation for relinked machines.

**Tech Stack:** Go 1.26, chi, SQLite. Tests use plain `testing` with mocks.

**Spec:** `docs/superpowers/specs/2026-11-10-machine-import-design.md`

**Status:** All tasks implemented on `feat/issue-12-machine-import`; boxes are ticked to match.

## Global Constraints

- Follow `AGENTS.md`: every symbol has a doc comment starting with its name (interface methods are documented on the interface only); tests live in external packages and are named `Test<Type>_<Method>_<Scenario>_<ExpectedResult>`; helpers sit at the end of the file; assertions use `t.Errorf` or `t.Fatalf`; validations of one scenario are table-driven.
- Write each test file in two steps: the skeleton first, then the bodies.
- Handler tests assert the HTTP status code only.
- Swag annotations stay short; `@Accept text/csv` on the import route is the one exception.
- Do not commit. Leave changes for developer review.

## Review Focus

- An empty `lab` cell never changes an existing link (Task 1, Task 3).
- An invalid CSV persists nothing (Task 3).
- Only machines whose group changed are notified (Task 3).
- Import never resets connection state or MAC (Task 1).
- An oversized body returns 413, not 400 (Task 4).

---

### Task 1: Store `Import` replaces `Sync`

**Files:**
- Modify: `internal/orchestrator/domain/machine.go`
- Modify: `internal/orchestrator/store/machine_store.go`
- Modify: `internal/orchestrator/store/machine_store_test.go`

**Produces:** `domain.MachineImport{Machine, Lab}`, `domain.MachineImportResult{MachinesCreated, MachinesUpdated, GroupsCreated, RelinkedIDs}`, `MachineStore.Import(ctx, []domain.MachineImport) (*domain.MachineImportResult, error)`. `Sync` removed from the interface and the store, with the unused `listMachines` helper.

- [x] **Step 1:** Add the two domain types and the `Import` method to `domain.MachineStore`; remove `Sync`.
- [x] **Step 2:** Implement `Import` inside `executor.WithTx`: `ensureImportGroups` (`INSERT OR IGNORE INTO groups`, count rows affected, resolve ID by name), then `importMachine` per entry (count by IP for created vs updated, `queryUpsertMachine`, compare `group_id`, link and record the ID when it differs).
- [x] **Step 3:** Replace the `Sync` test with `Import` tests: `WithUnknownLab_CreatesGroupAndLinksMachines`, `WithExistingLab_ReusesGroup`, `WithExistingIP_UpdatesNameAndKeepsState`, `WithEmptyLab_KeepsCurrentLink`, `WithSameLab_DoesNotReportRelink`, `WithDifferentLab_MovesMachine` (old group kept), `WithAbsentMachine_KeepsMachine`. Add `seedGroup` and `seedImportMachine` helpers.
- [x] **Step 4:** `go test ./internal/orchestrator/store/` passes.

### Task 2: Store `RemoveByID`

**Files:** same as Task 1.

- [x] **Step 1:** Add `RemoveByID(ctx, id int64) error` to `domain.MachineStore`.
- [x] **Step 2:** Implement it with `DELETE FROM machines WHERE id = ?`; zero rows affected returns `ErrMachineNotFound`.
- [x] **Step 3:** Tests `WithExistingID_RemovesMachine`, `WithUnknownID_ReturnsErrMachineNotFound`; package passes.

### Task 3: Service `ImportMachines` and `RemoveMachine`

**Files:**
- Create: `internal/orchestrator/service/machine_import.go`
- Modify: `internal/orchestrator/service/machine_service.go`
- Modify: `internal/orchestrator/service/machine_service_test.go`
- Modify: `internal/orchestrator/service/mock_machine_store_test.go`
- Modify: `internal/common/errors/machine.go`, `internal/common/schemas/machine.go`

**Produces:** `ErrInvalidMachinesCSV`; `schemas.MachineImportOutputSchema` (JSON `machines_created`, `machines_updated`, `groups_created`, `machines_linked`); `NewMachineService(machineStore, propagationStore, notifier PropagationNotifier, labKey, sessionSecret, sessionTTL, stalenessGrace, now)`; `ImportMachines(ctx, io.Reader) (*schemas.MachineImportOutputSchema, error)`; `RemoveMachine(ctx, machineID int64) error`.

- [x] **Step 1:** `parseMachineImport` in `machine_import.go`: trimmed header must equal `ip,name,lab`; 3 fields per row; valid, unique IP; non-empty name; read errors wrapped as `%w: %w` so `*http.MaxBytesError` survives.
- [x] **Step 2:** `ImportMachines` parses, calls `store.Import`, calls `notifier.NotifyMachine` per `RelinkedIDs` when the notifier is not nil, maps the result. Replaces `SyncMachines`.
- [x] **Step 3:** `RemoveMachine` delegates to `store.RemoveByID`.
- [x] **Step 4:** Mock store: replace `Sync` with `Import` (labs resolved through `ImportGroupIDs`) and add `RemoveByID`.
- [x] **Step 5:** Replace the `SyncMachines` seeds in the existing tests with a `seedMachineList` helper; pass `nil` as the notifier in existing arrange helpers; add `arrangeMachineImportTest` with a recording `MockPropagationNotifier`.
- [x] **Step 6:** Import tests: `WithValidCSV_ReturnsSummary`, `WithRelinkedMachines_NotifiesEachMachine`, `WithPaddedFields_TrimsValues`, table-driven `WithInvalidCSV_ReturnsErrInvalidMachinesCSV` (empty, legacy header, wrong header, missing field, extra field, invalid IP, empty name, duplicate IP, malformed quotes; each asserts nothing persisted), `WhenStoreFails_ReturnsError`.
- [x] **Step 7:** Remove tests: `WithExistingMachine_RemovesMachine`, `WithUnknownMachine_ReturnsErrMachineNotFound`, `WhenStoreFails_ReturnsError`. Package passes.

### Task 4: Handler routes

**Files:**
- Modify: `internal/orchestrator/api/handler/machine_handler.go`
- Modify: `internal/orchestrator/api/handler/machine_handler_test.go`
- Modify: `internal/orchestrator/api/handler/mock_machine_service_test.go`

- [x] **Step 1:** Add `ImportMachines` and `RemoveMachine` to `MachineServiceInterface` and the mock.
- [x] **Step 2:** `POST /machines/import` → `Import`: wrap the body in `http.MaxBytesReader(1 MiB)`; `*http.MaxBytesError` → 413, `ErrInvalidMachinesCSV` → 400, other → 500, success → 200. Swag: `@Accept text/csv`, `@Param body body string true "CSV"`, `@Failure 400,413`.
- [x] **Step 3:** `DELETE /machines/{id}` → `Delete`: `parseMachineID` (400), `ErrMachineNotFound` → 404, success → 204.
- [x] **Step 4:** Tests (status only). Import: 200, 400, 413 (2 MiB body read by the mock), 500. Delete: 204, 400, 404, 500. Add `performCSVRequest` helper.

### Task 5: Wiring and config

**Files:**
- Modify: `internal/orchestrator/api/router.go`, `internal/orchestrator/api/router_test.go`
- Modify: `internal/orchestrator/config/config.go`, `internal/orchestrator/config/config_test.go`

- [x] **Step 1:** Delete `seedMachines`; pass `propagationService` as the notifier to `NewMachineService`.
- [x] **Step 2:** Remove `envMachines`, `machinesHeader`, `config.Machine`, `Config.Machines`, `parseMachines` and the no-machines warning; drop the matching config tests and `AEGIS_MACHINES` setup.
- [x] **Step 3:** Router tests seed through `POST /machines/import`; add `WithLabColumn_LinksImportedMachinesToGroup`.
- [x] **Step 4:** `make openapi`; check `/machines/import` (`text/csv`) and `/machines/{id}` delete in `collections/orchestrator/openapi.yml`.

### Task 6: Ops and e2e

**Files:**
- Modify: `.env.example`, `packaging/orchestrator/Dockerfile`
- Modify: `e2e/containers/harness_test.go`
- Modify: `e2e/vm/scripts/deploy.sh`, `e2e/vm/templates/machines.csv`

- [x] **Step 1:** Drop `AEGIS_MACHINES` from `.env.example` and the Dockerfile comment; point to `POST /machines/import`.
- [x] **Step 2:** Container harness: stop injecting the CSV and env var; add `fleet.importRegistry`, called from `startOrchestrator`, posting `ip,name,lab` with empty labs.
- [x] **Step 3:** VM: template gains an empty `lab` column; `deploy.sh` drops the env var and file install, waits for `/machines`, then `curl --data-binary @machines.csv` to `/machines/import`.

## Verification

- [x] `make tests` passes.
- [x] `make coverage` ≥ 75% (89.4%).
- [x] `golangci-lint run ./...` clean.
- [x] `make e2e` passes (run before Task 2 and the delete route; those changes do not touch the harness).
- [ ] VM lab (`make vm-up` and the KVM suite) not yet run against the new `deploy.sh`.
