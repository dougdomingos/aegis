# US07 — Machine-to-Group Linking Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Persist a one-group-per-machine association and expose group-centric endpoints to link and unlink machines.

**Architecture:** Add a nullable `group_id` FK to `machines` (SQLite `ON DELETE SET NULL`), extend the machine store with link/unlink/list-by-group operations that join the group name, add link/unlink plus machine-ID output to `GroupService`, then expose `POST /groups/{name}/machines` and `DELETE /groups/{name}/machines/{machineID}`.

**Tech Stack:** Go 1.26, chi, SQLite (modernc.org/sqlite), net/http.

**Spec:** `docs/superpowers/specs/2026-10-29-us07-machine-group-link-design.md`

## Global Constraints

- Module path: `dougdomingos.com/aegis`; Go 1.26.5.
- Implement in vertical-slice order: Store → Service → API Handler. Each layer must build and pass its tests before the next.
- Existing migrations must not be modified. Add the new field through `005_add_group_id_to_machines.sql` (single schema change per file).
- Test files are external packages; test names follow `Test<Type>_<Method>_<Scenario>_<ExpectedResult>`.
- New exports need doc comments starting with the symbol name; interface implementations do not repeat interface doc comments.
- **Do not commit.** AGENTS.md forbids automatic commits; leave staged-or-unstaged changes for developer review. The git-commit steps normally added by the plan workflow are intentionally omitted.
- Run `go test` on the touched packages after each task; run `make tests` before handing off.

## Review Focus

1. Unlinking a machine from a group it is **not** linked to must return an error and must not clear a link to another group.
2. Linking a machine already in another group must **move** it (single `group_id`), not error or create a second association.
3. Removing a group must keep its machines registered with `group_id = NULL`.
4. A machine without a group must serialize `group_id` as `null` and an empty `group_name`.
5. A missing/non-numeric `machineID` path segment or malformed link body must return `400`, never panic.

---

### Task 1: Persist the Machine→Group Association

**Files:**
- Create: `internal/orchestrator/infra/migrations/sql/005_add_group_id_to_machines.sql`
- Modify: `internal/orchestrator/domain/machine.go`
- Modify: `internal/orchestrator/store/machine_store.go`
- Modify: `internal/orchestrator/service/mock_machine_store_test.go`
- Test: `internal/orchestrator/store/machine_store_test.go`

**Interfaces:**
- Consumes: existing `domain.GroupStore`, `query.QueryExecutor`, `arrangeStoreDB`.
- Produces:
  - `domain.Machine.GroupID *int64`, `domain.Machine.GroupName string`
  - `MachineStore.GetByID(ctx, id int64) (*Machine, error)`
  - `MachineStore.LinkToGroup(ctx, machineID int64, groupID int64) (*Machine, error)`
  - `MachineStore.UnlinkFromGroup(ctx, machineID int64) (*Machine, error)`
  - `MachineStore.ListByGroup(ctx, groupID int64) ([]Machine, error)`

- [ ] **Step 1: Create the migration**

`internal/orchestrator/infra/migrations/sql/005_add_group_id_to_machines.sql`:

```sql
ALTER TABLE machines ADD COLUMN group_id INTEGER REFERENCES groups(id) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS idx_machines_group_id ON machines(group_id);
```

- [ ] **Step 2: Add the domain fields and store methods**

In `domain/machine.go`, add to `Machine` after `CreatedAt`:

```go
// GroupID is the identifier of the group the machine is linked to, or nil
// when the machine is not linked to any group.
GroupID *int64

// GroupName is the human-friendly label of the linked group, populated on
// reads. It is empty when the machine is not linked.
GroupName string
```

Add to the `MachineStore` interface (after `GetByIP`):

```go
// GetByID retrieves the machine whose ID matches the provided argument.
GetByID(ctx context.Context, id int64) (*Machine, error)

// LinkToGroup associates the machine with the provided group, moving it from
// any previously linked group.
LinkToGroup(ctx context.Context, machineID int64, groupID int64) (*Machine, error)

// UnlinkFromGroup clears the group association of the machine.
UnlinkFromGroup(ctx context.Context, machineID int64) (*Machine, error)

// ListByGroup retrieves all machines linked to the provided group.
ListByGroup(ctx context.Context, groupID int64) ([]Machine, error)
```

- [ ] **Step 3: Write failing store tests**

Append to `internal/orchestrator/store/machine_store_test.go` (before the helper section) and add the helper at the end of the file:

```go
// ============================================================================
// LinkToGroup, UnlinkFromGroup, GetByID & ListByGroup
// ============================================================================

func TestMachineStore_LinkToGroup_WithValidIDs_SetsGroupAndName(t *testing.T) {
	ctx, machineStore, groupStore := arrangeMachineGroupStoreTest(t)
	group, _ := groupStore.Create(ctx, "lab1")
	machine, _ := machineStore.Upsert(ctx, domain.Machine{IP: "10.0.1.11", Name: "lcc1-011"})

	linked, err := machineStore.LinkToGroup(ctx, machine.ID, group.ID)

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if linked.GroupID == nil || *linked.GroupID != group.ID {
		t.Errorf("expected group id %d, got %v", group.ID, linked.GroupID)
	}
	if linked.GroupName != "lab1" {
		t.Errorf("expected group name %q, got %q", "lab1", linked.GroupName)
	}
}

func TestMachineStore_LinkToGroup_WithInexistentMachine_ReturnsErrMachineNotFound(t *testing.T) {
	ctx, machineStore, groupStore := arrangeMachineGroupStoreTest(t)
	group, _ := groupStore.Create(ctx, "lab1")

	_, err := machineStore.LinkToGroup(ctx, 999, group.ID)

	if !errors.Is(err, machineErrors.ErrMachineNotFound) {
		t.Errorf("expected %v, got %v", machineErrors.ErrMachineNotFound, err)
	}
}

func TestMachineStore_UnlinkFromGroup_WithLinkedMachine_ClearsGroupID(t *testing.T) {
	ctx, machineStore, groupStore := arrangeMachineGroupStoreTest(t)
	group, _ := groupStore.Create(ctx, "lab1")
	machine, _ := machineStore.Upsert(ctx, domain.Machine{IP: "10.0.1.11", Name: "lcc1-011"})
	_, _ = machineStore.LinkToGroup(ctx, machine.ID, group.ID)

	unlinked, err := machineStore.UnlinkFromGroup(ctx, machine.ID)

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if unlinked.GroupID != nil {
		t.Errorf("expected nil group id, got %v", *unlinked.GroupID)
	}
}

func TestMachineStore_GetByID_WithUnlinkedMachine_ReturnsNilGroup(t *testing.T) {
	ctx, machineStore, _ := arrangeMachineGroupStoreTest(t)
	machine, _ := machineStore.Upsert(ctx, domain.Machine{IP: "10.0.1.11", Name: "lcc1-011"})

	found, err := machineStore.GetByID(ctx, machine.ID)

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if found.GroupID != nil {
		t.Errorf("expected nil group id, got %v", *found.GroupID)
	}
}

func TestMachineStore_ListByGroup_WithMixedMachines_ReturnsOnlyGroupMembers(t *testing.T) {
	ctx, machineStore, groupStore := arrangeMachineGroupStoreTest(t)
	group, _ := groupStore.Create(ctx, "lab1")
	linked, _ := machineStore.Upsert(ctx, domain.Machine{IP: "10.0.1.11", Name: "lcc1-011"})
	_, _ = machineStore.Upsert(ctx, domain.Machine{IP: "10.0.1.12", Name: "lcc1-012"})
	_, _ = machineStore.LinkToGroup(ctx, linked.ID, group.ID)

	machines, err := machineStore.ListByGroup(ctx, group.ID)

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(machines) != 1 || machines[0].ID != linked.ID {
		t.Errorf("expected only machine %d, got %+v", linked.ID, machines)
	}
}

func TestGroupStore_Remove_WithLinkedMachines_KeepsMachinesUnlinked(t *testing.T) {
	ctx, machineStore, groupStore := arrangeMachineGroupStoreTest(t)
	group, _ := groupStore.Create(ctx, "lab1")
	machine, _ := machineStore.Upsert(ctx, domain.Machine{IP: "10.0.1.11", Name: "lcc1-011"})
	_, _ = machineStore.LinkToGroup(ctx, machine.ID, group.ID)

	if err := groupStore.Remove(ctx, "lab1"); err != nil {
		t.Fatalf("expected group to be removed, got: %v", err)
	}

	found, err := machineStore.GetByID(ctx, machine.ID)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if found == nil {
		t.Fatal("expected machine to remain registered, got nil")
	}
	if found.GroupID != nil {
		t.Errorf("expected nil group id after group removal, got %v", *found.GroupID)
	}
}

// arrangeMachineGroupStoreTest builds both stores over a migrated database.
func arrangeMachineGroupStoreTest(t *testing.T) (context.Context, *store.MachineStore, *store.GroupStore) {
	t.Helper()

	ctx, db := arrangeStoreDB(t)
	return ctx, store.NewMachineStore(db), store.NewGroupStore(db)
}
```

Add `"context"` to the test file imports.

- [ ] **Step 4: Run the tests to verify they fail**

Run: `go test ./internal/orchestrator/store/ -run 'LinkToGroup|UnlinkFromGroup|ListByGroup|GetByID|KeepsMachinesUnlinked' -v`
Expected: FAIL (undefined methods / missing columns).

- [ ] **Step 5: Implement the store queries and mapper**

In `store/machine_store.go`, replace the machine SELECT constants and add the new ones:

```go
queryGetMachineByIP = `SELECT m.id, m.name, m.ip, m.mac, m.status, m.last_seen_at, m.created_at, m.group_id, g.name
	FROM machines m LEFT JOIN groups g ON g.id = m.group_id WHERE m.ip = ?`
queryGetMachineByID = `SELECT m.id, m.name, m.ip, m.mac, m.status, m.last_seen_at, m.created_at, m.group_id, g.name
	FROM machines m LEFT JOIN groups g ON g.id = m.group_id WHERE m.id = ?`
queryGetAllMachines = `SELECT m.id, m.name, m.ip, m.mac, m.status, m.last_seen_at, m.created_at, m.group_id, g.name
	FROM machines m LEFT JOIN groups g ON g.id = m.group_id`
queryListMachinesByGroup = `SELECT m.id, m.name, m.ip, m.mac, m.status, m.last_seen_at, m.created_at, m.group_id, g.name
	FROM machines m LEFT JOIN groups g ON g.id = m.group_id WHERE m.group_id = ?`
queryLinkMachineToGroup   = "UPDATE machines SET group_id = ? WHERE id = ?"
queryUnlinkMachineFromGroup = "UPDATE machines SET group_id = NULL WHERE id = ?"
```

Add `GetByID`:

```go
func (store *MachineStore) GetByID(ctx context.Context, id int64) (*domain.Machine, error) {
	return store.executor.QueryOne(ctx, queryGetMachineByID, id)
}
```

Add link/unlink/list:

```go
func (store *MachineStore) LinkToGroup(ctx context.Context, machineID int64, groupID int64) (*domain.Machine, error) {
	result, err := store.executor.Exec(ctx, queryLinkMachineToGroup, groupID, machineID)
	if err != nil {
		return nil, fmt.Errorf("failed to link machine %d to group %d: %w", machineID, groupID, err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("failed to check rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return nil, fmt.Errorf("%w: id %d", errors.ErrMachineNotFound, machineID)
	}

	return store.GetByID(ctx, machineID)
}

func (store *MachineStore) UnlinkFromGroup(ctx context.Context, machineID int64) (*domain.Machine, error) {
	result, err := store.executor.Exec(ctx, queryUnlinkMachineFromGroup, machineID)
	if err != nil {
		return nil, fmt.Errorf("failed to unlink machine %d: %w", machineID, err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("failed to check rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return nil, fmt.Errorf("%w: id %d", errors.ErrMachineNotFound, machineID)
	}

	return store.GetByID(ctx, machineID)
}

func (store *MachineStore) ListByGroup(ctx context.Context, groupID int64) ([]domain.Machine, error) {
	return store.executor.QueryMany(ctx, queryListMachinesByGroup, groupID)
}
```

Update `mapRowToMachine` to scan the two new columns:

```go
func mapRowToMachine(scan query.ScanFunc) (domain.Machine, error) {
	var machine domain.Machine
	var groupID sql.NullInt64
	var groupName sql.NullString

	if err := scan(&machine.ID, &machine.Name, &machine.IP, &machine.MAC, &machine.Status, &machine.LastSeenAt, &machine.CreatedAt, &groupID, &groupName); err != nil {
		return domain.Machine{}, err
	}

	if groupID.Valid {
		id := groupID.Int64
		machine.GroupID = &id
	}
	machine.GroupName = groupName.String

	return machine, nil
}
```

- [ ] **Step 6: Update the service-test machine mock**

In `service/mock_machine_store_test.go`, add these methods so the mock satisfies the widened `domain.MachineStore`:

```go
// GetByID retrieves a machine by its unique ID or returns StoreErr if set.
func (mock *MockMachineStore) GetByID(ctx context.Context, id int64) (*domain.Machine, error) {
	if mock.StoreErr != nil {
		return nil, mock.StoreErr
	}
	for _, machine := range mock.machines {
		if machine.ID == id {
			return machine, nil
		}
	}
	return nil, nil
}

// LinkToGroup associates a machine with a group or returns StoreErr if set.
func (mock *MockMachineStore) LinkToGroup(ctx context.Context, machineID int64, groupID int64) (*domain.Machine, error) {
	if mock.StoreErr != nil {
		return nil, mock.StoreErr
	}
	machine, err := mock.GetByID(ctx, machineID)
	if err != nil {
		return nil, err
	}
	if machine == nil {
		return nil, fmt.Errorf("%w: id %d", machineErrors.ErrMachineNotFound, machineID)
	}
	id := groupID
	machine.GroupID = &id
	return machine, nil
}

// UnlinkFromGroup clears a machine's group association or returns StoreErr if set.
func (mock *MockMachineStore) UnlinkFromGroup(ctx context.Context, machineID int64) (*domain.Machine, error) {
	if mock.StoreErr != nil {
		return nil, mock.StoreErr
	}
	machine, err := mock.GetByID(ctx, machineID)
	if err != nil {
		return nil, err
	}
	if machine == nil {
		return nil, fmt.Errorf("%w: id %d", machineErrors.ErrMachineNotFound, machineID)
	}
	machine.GroupID = nil
	machine.GroupName = ""
	return machine, nil
}

// ListByGroup returns the machines linked to the provided group.
func (mock *MockMachineStore) ListByGroup(ctx context.Context, groupID int64) ([]domain.Machine, error) {
	if mock.StoreErr != nil {
		return nil, mock.StoreErr
	}
	list := make([]domain.Machine, 0)
	for _, machine := range mock.machines {
		if machine.GroupID != nil && *machine.GroupID == groupID {
			list = append(list, *machine)
		}
	}
	return list, nil
}
```

- [ ] **Step 7: Run the store and service tests**

Run: `go test ./internal/orchestrator/store/ ./internal/orchestrator/service/`
Expected: PASS.

---

### Task 2: Group Service Link/Unlink and Machine IDs in Group Output

**Files:**
- Modify: `internal/common/schemas/group.go`
- Modify: `internal/common/schemas/machine.go`
- Modify: `internal/common/errors/machine.go`
- Modify: `internal/orchestrator/service/machine_service.go`
- Modify: `internal/orchestrator/service/group_service.go`
- Modify: `internal/orchestrator/service/group_service_test.go`
- Modify: `internal/orchestrator/api/router.go`

**Interfaces:**
- Consumes: Task 1's `MachineStore` methods and `Machine.GroupID/GroupName`.
- Produces:
  - `schemas.LinkMachineToGroupSchema{ MachineID int64 }`
  - `schemas.MachineOutputSchema.GroupID *int64`, `.GroupName string`
  - `schemas.GroupOutputSchema.MachineIDs []int64`
  - `errors.ErrMachineNotInGroup`
  - `NewGroupService(store domain.GroupStore, machineStore domain.MachineStore, stalenessGrace time.Duration, now func() time.Time) *GroupService`
  - `(*GroupService).LinkMachine(ctx, groupName string, machineID int64) (*schemas.MachineOutputSchema, error)`
  - `(*GroupService).UnlinkMachine(ctx, groupName string, machineID int64) (*schemas.MachineOutputSchema, error)`

- [ ] **Step 1: Update schemas and errors**

`internal/common/schemas/machine.go` — extend `MachineOutputSchema` with:

```go
GroupID   *int64
GroupName string
```

Place the fields after `MAC`; output structs in this file carry no JSON tags, so
do not add any.

`internal/common/schemas/group.go` — extend `GroupOutputSchema` with `MachineIDs []int64` after `CreatedAt`, and add:

```go
// LinkMachineToGroupSchema declares the machine selected to be linked to the
// group named in the request path.
type LinkMachineToGroupSchema struct {
	MachineID int64 `json:"machine_id"`
}
```

`internal/common/errors/machine.go` — add:

```go
// ErrMachineNotInGroup is returned when unlinking a machine from a group it is
// not currently associated with.
ErrMachineNotInGroup = errors.New("machine is not linked to the requested group")
```

- [ ] **Step 2: Map the new machine fields**

In `service/machine_service.go`, extend the struct literal in `mapMachineToOutputSchema`:

```go
GroupID:   machine.GroupID,
GroupName: machine.GroupName,
```

- [ ] **Step 3: Write failing service tests**

Add to `internal/orchestrator/service/group_service_test.go` before the helpers section:

```go
// ============================================================================
// LinkMachine & UnlinkMachine
// ============================================================================

func TestGroupService_LinkMachine_WithValidGroupAndMachine_LinksMachine(t *testing.T) {
	ctx, groupStore, machineStore, svc := arrangeGroupMachineServiceTest(t)
	group, _ := groupStore.Create(ctx, "lab1")
	machine, _ := machineStore.Upsert(ctx, domain.Machine{IP: "10.0.1.11", Name: "lcc1-011"})

	res, err := svc.LinkMachine(ctx, "lab1", machine.ID)

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if res.GroupID == nil || *res.GroupID != group.ID {
		t.Errorf("expected group id %d, got %v", group.ID, res.GroupID)
	}
}

func TestGroupService_LinkMachine_WithInexistentGroup_ReturnsErrGroupNotFound(t *testing.T) {
	ctx, _, machineStore, svc := arrangeGroupMachineServiceTest(t)
	machine, _ := machineStore.Upsert(ctx, domain.Machine{IP: "10.0.1.11", Name: "lcc1-011"})

	_, err := svc.LinkMachine(ctx, "unknown", machine.ID)

	if !errors.Is(err, groupErrors.ErrGroupNotFound) {
		t.Errorf("expected %v, got %v", groupErrors.ErrGroupNotFound, err)
	}
}

func TestGroupService_LinkMachine_WithInexistentMachine_ReturnsErrMachineNotFound(t *testing.T) {
	ctx, groupStore, _, svc := arrangeGroupMachineServiceTest(t)
	_, _ = groupStore.Create(ctx, "lab1")

	_, err := svc.LinkMachine(ctx, "lab1", 999)

	if !errors.Is(err, machineErrors.ErrMachineNotFound) {
		t.Errorf("expected %v, got %v", machineErrors.ErrMachineNotFound, err)
	}
}

func TestGroupService_LinkMachine_WithMachineInAnotherGroup_MovesMachine(t *testing.T) {
	ctx, groupStore, machineStore, svc := arrangeGroupMachineServiceTest(t)
	_, _ = groupStore.Create(ctx, "lab1")
	second, _ := groupStore.Create(ctx, "lab2")
	machine, _ := machineStore.Upsert(ctx, domain.Machine{IP: "10.0.1.11", Name: "lcc1-011"})
	_, _ = svc.LinkMachine(ctx, "lab1", machine.ID)

	res, err := svc.LinkMachine(ctx, "lab2", machine.ID)

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if res.GroupID == nil || *res.GroupID != second.ID {
		t.Errorf("expected group id %d, got %v", second.ID, res.GroupID)
	}
}

func TestGroupService_UnlinkMachine_WithLinkedMachine_ClearsGroup(t *testing.T) {
	ctx, groupStore, machineStore, svc := arrangeGroupMachineServiceTest(t)
	_, _ = groupStore.Create(ctx, "lab1")
	machine, _ := machineStore.Upsert(ctx, domain.Machine{IP: "10.0.1.11", Name: "lcc1-011"})
	_, _ = svc.LinkMachine(ctx, "lab1", machine.ID)

	res, err := svc.UnlinkMachine(ctx, "lab1", machine.ID)

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if res.GroupID != nil {
		t.Errorf("expected nil group id, got %v", *res.GroupID)
	}
}

func TestGroupService_UnlinkMachine_WhenLinkedToAnotherGroup_ReturnsErrMachineNotInGroup(t *testing.T) {
	ctx, groupStore, machineStore, svc := arrangeGroupMachineServiceTest(t)
	_, _ = groupStore.Create(ctx, "lab1")
	_, _ = groupStore.Create(ctx, "lab2")
	machine, _ := machineStore.Upsert(ctx, domain.Machine{IP: "10.0.1.11", Name: "lcc1-011"})
	_, _ = svc.LinkMachine(ctx, "lab2", machine.ID)

	_, err := svc.UnlinkMachine(ctx, "lab1", machine.ID)

	if !errors.Is(err, machineErrors.ErrMachineNotInGroup) {
		t.Errorf("expected %v, got %v", machineErrors.ErrMachineNotInGroup, err)
	}
}

func TestGroupService_GetGroupByName_WithLinkedMachine_IncludesMachineIDs(t *testing.T) {
	ctx, groupStore, machineStore, svc := arrangeGroupMachineServiceTest(t)
	_, _ = groupStore.Create(ctx, "lab1")
	machine, _ := machineStore.Upsert(ctx, domain.Machine{IP: "10.0.1.11", Name: "lcc1-011"})
	_, _ = svc.LinkMachine(ctx, "lab1", machine.ID)

	res, err := svc.GetGroupByName(ctx, schemas.GetGroupByNameSchema{Name: "lab1"})

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(res.MachineIDs) != 1 || res.MachineIDs[0] != machine.ID {
		t.Errorf("expected machine ids [%d], got %v", machine.ID, res.MachineIDs)
	}
}

func TestGroupService_ListAllGroups_WithLinkedMachines_IncludesMachineIDs(t *testing.T) {
	ctx, groupStore, machineStore, svc := arrangeGroupMachineServiceTest(t)
	_, _ = groupStore.Create(ctx, "lab1")
	machine, _ := machineStore.Upsert(ctx, domain.Machine{IP: "10.0.1.11", Name: "lcc1-011"})
	_, _ = svc.LinkMachine(ctx, "lab1", machine.ID)

	res, err := svc.ListAllGroups(ctx)

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(res) != 1 || len(res[0].MachineIDs) != 1 || res[0].MachineIDs[0] != machine.ID {
		t.Errorf("expected one group linked to machine %d, got %+v", machine.ID, res)
	}
}
```

- [ ] **Step 4: Run the service tests to verify they fail**

Add `time`, `machineErrors "dougdomingos.com/aegis/internal/common/errors"`, and
`"dougdomingos.com/aegis/internal/orchestrator/domain"` to the test imports
before running (`errors` and `groupErrors` are already imported).

Run: `go test ./internal/orchestrator/service/ -run 'LinkMachine|UnlinkMachine|IncludesMachineIDs' -v`
Expected: FAIL (undefined methods / constructor arity).

- [ ] **Step 5: Extend `GroupService`**

In `service/group_service.go`:

Add imports `"time"` and keep `machineErrors`-free (both error packages are the same `common/errors`; the file already imports it as `errors`).

Add fields to `GroupService`:

```go
machineStore domain.MachineStore
stalenessGrace time.Duration
now func() time.Time
```

Replace the constructor:

```go
func NewGroupService(store domain.GroupStore, machineStore domain.MachineStore, stalenessGrace time.Duration, now func() time.Time) *GroupService {
	return &GroupService{
		store:          store,
		machineStore:   machineStore,
		stalenessGrace: stalenessGrace,
		now:            now,
	}
}
```

Add the two methods (place before `mapGroupToOutputSchema`):

```go
// LinkMachine associates the machine whose ID matches the argument with the
// group whose name matches the argument, moving it from any previous group.
func (service *GroupService) LinkMachine(ctx context.Context, groupName string, machineID int64) (*schemas.MachineOutputSchema, error) {
	if strings.TrimSpace(groupName) == "" {
		return nil, errors.ErrGroupNameRequired
	}

	group, err := service.store.GetByName(ctx, groupName)
	if err != nil {
		return nil, err
	}
	if group == nil {
		return nil, errors.ErrGroupNotFound
	}

	machine, err := service.machineStore.GetByID(ctx, machineID)
	if err != nil {
		return nil, err
	}
	if machine == nil {
		return nil, errors.ErrMachineNotFound
	}

	linked, err := service.machineStore.LinkToGroup(ctx, machineID, group.ID)
	if err != nil {
		return nil, err
	}

	return mapMachineToOutputSchema(linked, service.now(), service.stalenessGrace), nil
}

// UnlinkMachine removes the association between the machine whose ID matches
// the argument and the group whose name matches the argument.
func (service *GroupService) UnlinkMachine(ctx context.Context, groupName string, machineID int64) (*schemas.MachineOutputSchema, error) {
	if strings.TrimSpace(groupName) == "" {
		return nil, errors.ErrGroupNameRequired
	}

	group, err := service.store.GetByName(ctx, groupName)
	if err != nil {
		return nil, err
	}
	if group == nil {
		return nil, errors.ErrGroupNotFound
	}

	machine, err := service.machineStore.GetByID(ctx, machineID)
	if err != nil {
		return nil, err
	}
	if machine == nil {
		return nil, errors.ErrMachineNotFound
	}
	if machine.GroupID == nil || *machine.GroupID != group.ID {
		return nil, errors.ErrMachineNotInGroup
	}

	unlinked, err := service.machineStore.UnlinkFromGroup(ctx, machineID)
	if err != nil {
		return nil, err
	}

	return mapMachineToOutputSchema(unlinked, service.now(), service.stalenessGrace), nil
}
```

- [ ] **Step 6: Populate `MachineIDs` in group output**

Change `mapGroupToOutputSchema` to accept machine IDs:

```go
func mapGroupToOutputSchema(group *domain.Group, machineIDs []int64) *schemas.GroupOutputSchema {
	if group == nil {
		return nil
	}

	if machineIDs == nil {
		machineIDs = []int64{}
	}

	return &schemas.GroupOutputSchema{
		ID:         group.ID,
		Name:       group.Name,
		CreatedAt:  group.CreatedAt,
		MachineIDs: machineIDs,
	}
}
```

Add the helper:

```go
// machineIDs lists the IDs of the machines linked to the provided group.
func (service *GroupService) machineIDs(ctx context.Context, groupID int64) ([]int64, error) {
	machines, err := service.machineStore.ListByGroup(ctx, groupID)
	if err != nil {
		return nil, err
	}

	ids := make([]int64, 0, len(machines))
	for _, machine := range machines {
		ids = append(ids, machine.ID)
	}

	return ids, nil
}
```

Update call sites:

- `CreateGroup`: `return mapGroupToOutputSchema(newGroup, nil), nil`
- `GetGroupByName`: after the nil check, `ids, err := service.machineIDs(ctx, existentGroup.ID); if err != nil { return nil, err }; return mapGroupToOutputSchema(existentGroup, ids), nil`
- `ListAllGroups`: for each group, fetch IDs before mapping; return the first error.
- `ChangeGroupName`: after `Update`, fetch IDs and pass them to the mapper.

- [ ] **Step 7: Update the group service test helper**

At the end of `group_service_test.go`, keep the existing 3-value helper and add a 4-value helper:

```go
func arrangeGroupServiceTest(t *testing.T) (context.Context, *MockGroupStore, service.GroupService) {
	t.Helper()

	ctx, groupStore, _, svc := arrangeGroupMachineServiceTest(t)
	return ctx, groupStore, svc
}

func arrangeGroupMachineServiceTest(t *testing.T) (context.Context, *MockGroupStore, *MockMachineStore, service.GroupService) {
	t.Helper()

	groupStore := NewMockGroupStore()
	machineStore := NewMockMachineStore()
	now := func() time.Time { return time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC) }
	svc := service.NewGroupService(groupStore, machineStore, time.Minute, now)

	return context.Background(), groupStore, machineStore, *svc
}
```

- [ ] **Step 8: Update router wiring**

In `api/router.go`, replace the group handler construction:

```go
handler.NewGroupHandler(service.NewGroupService(store.NewGroupStore(db), machineStore, cfg.StalenessGrace, time.Now)),
```

- [ ] **Step 9: Run tests**

Run: `go test ./internal/orchestrator/service/ ./internal/orchestrator/api/`
Expected: PASS.

---

### Task 3: Link/Unlink HTTP Endpoints

**Files:**
- Modify: `internal/orchestrator/api/handler/group_handler.go`
- Modify: `internal/orchestrator/api/handler/mock_group_service_test.go`
- Test: `internal/orchestrator/api/handler/group_handler_test.go`

**Interfaces:**
- Consumes: Task 2's service methods and `schemas.LinkMachineToGroupSchema`.
- Produces:
  - `POST /groups/{name}/machines`
  - `DELETE /groups/{name}/machines/{machineID}`

- [ ] **Step 1: Write failing handler tests**

Add to `group_handler_test.go` before the helpers section:

```go
// ============================================================================
// LinkMachine (POST /groups/{name}/machines)
// ============================================================================

func TestGroupHandler_LinkMachine_WithValidPayload_ReturnsStatus200(t *testing.T) {
	mock := &mockGroupService{
		linkFn: func(_ context.Context, groupName string, machineID int64) (*schemas.MachineOutputSchema, error) {
			return &schemas.MachineOutputSchema{ID: machineID}, nil
		},
	}

	rec := performRequest(t, setupRouter(t, mock), http.MethodPost, "/groups/lab1/machines", schemas.LinkMachineToGroupSchema{MachineID: 1})
	assertStatusCode(t, rec, http.StatusOK)
}

func TestGroupHandler_LinkMachine_WithInexistentGroup_ReturnsStatus404(t *testing.T) {
	mock := &mockGroupService{
		linkFn: func(_ context.Context, groupName string, machineID int64) (*schemas.MachineOutputSchema, error) {
			return nil, groupErrors.ErrGroupNotFound
		},
	}

	rec := performRequest(t, setupRouter(t, mock), http.MethodPost, "/groups/unknown/machines", schemas.LinkMachineToGroupSchema{MachineID: 1})
	assertStatusCode(t, rec, http.StatusNotFound)
}

func TestGroupHandler_LinkMachine_WithInexistentMachine_ReturnsStatus404(t *testing.T) {
	mock := &mockGroupService{
		linkFn: func(_ context.Context, groupName string, machineID int64) (*schemas.MachineOutputSchema, error) {
			return nil, groupErrors.ErrMachineNotFound
		},
	}

	rec := performRequest(t, setupRouter(t, mock), http.MethodPost, "/groups/lab1/machines", schemas.LinkMachineToGroupSchema{MachineID: 999})
	assertStatusCode(t, rec, http.StatusNotFound)
}

func TestGroupHandler_LinkMachine_WithMalformedPayload_ReturnsStatus400(t *testing.T) {
	mock := &mockGroupService{}

	rec := performRequest(t, setupRouter(t, mock), http.MethodPost, "/groups/lab1/machines", "invalid json")
	assertStatusCode(t, rec, http.StatusBadRequest)
}

// ============================================================================
// UnlinkMachine (DELETE /groups/{name}/machines/{machineID})
// ============================================================================

func TestGroupHandler_UnlinkMachine_WithValidPayload_ReturnsStatus200(t *testing.T) {
	mock := &mockGroupService{
		unlinkFn: func(_ context.Context, groupName string, machineID int64) (*schemas.MachineOutputSchema, error) {
			return &schemas.MachineOutputSchema{ID: machineID}, nil
		},
	}

	rec := performRequest(t, setupRouter(t, mock), http.MethodDelete, "/groups/lab1/machines/1", nil)
	assertStatusCode(t, rec, http.StatusOK)
}

func TestGroupHandler_UnlinkMachine_WithNotLinkedMachine_ReturnsStatus404(t *testing.T) {
	mock := &mockGroupService{
		unlinkFn: func(_ context.Context, groupName string, machineID int64) (*schemas.MachineOutputSchema, error) {
			return nil, groupErrors.ErrMachineNotInGroup
		},
	}

	rec := performRequest(t, setupRouter(t, mock), http.MethodDelete, "/groups/lab1/machines/1", nil)
	assertStatusCode(t, rec, http.StatusNotFound)
}

func TestGroupHandler_UnlinkMachine_WithInvalidMachineID_ReturnsStatus400(t *testing.T) {
	mock := &mockGroupService{}

	rec := performRequest(t, setupRouter(t, mock), http.MethodDelete, "/groups/lab1/machines/not-a-number", nil)
	assertStatusCode(t, rec, http.StatusBadRequest)
}
```

- [ ] **Step 2: Run the handler tests to verify they fail**

Run: `go test ./internal/orchestrator/api/handler/ -run 'LinkMachine|UnlinkMachine' -v`
Expected: FAIL (routes/mocks missing).

- [ ] **Step 3: Extend the handler test mock**

In `mock_group_service_test.go`, add fields:

```go
linkFn   func(ctx context.Context, groupName string, machineID int64) (*schemas.MachineOutputSchema, error)
unlinkFn func(ctx context.Context, groupName string, machineID int64) (*schemas.MachineOutputSchema, error)
```

and methods:

```go
func (m *mockGroupService) LinkMachine(ctx context.Context, groupName string, machineID int64) (*schemas.MachineOutputSchema, error) {
	return m.linkFn(ctx, groupName, machineID)
}

func (m *mockGroupService) UnlinkMachine(ctx context.Context, groupName string, machineID int64) (*schemas.MachineOutputSchema, error) {
	return m.unlinkFn(ctx, groupName, machineID)
}
```

- [ ] **Step 4: Extend the handler interface and routes**

In `group_handler.go`, add to `GroupServiceInterface`:

```go
LinkMachine(ctx context.Context, groupName string, machineID int64) (*schemas.MachineOutputSchema, error)
UnlinkMachine(ctx context.Context, groupName string, machineID int64) (*schemas.MachineOutputSchema, error)
```

Inside `RegisterRoutes`'s `/groups` block, add:

```go
r.Post("/{name}/machines", handler.LinkMachine)
r.Delete("/{name}/machines/{machineID}", handler.UnlinkMachine)
```

Add the handlers and helper:

```go
func (handler *GroupHandler) LinkMachine(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")

	var payload schemas.LinkMachineToGroupSchema
	if !utils.DecodePayload(w, r, &payload) {
		return
	}

	res, err := handler.service.LinkMachine(r.Context(), name, payload.MachineID)
	if err != nil {
		utils.MapByError(w, err, map[error]int{
			errors.ErrGroupNameRequired: http.StatusBadRequest,
			errors.ErrGroupNotFound:     http.StatusNotFound,
			errors.ErrMachineNotFound:   http.StatusNotFound,
		})

		return
	}

	utils.EncodeToJSON(w, http.StatusOK, res)
}

func (handler *GroupHandler) UnlinkMachine(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	machineID, ok := parseMachineID(w, r, "machineID")
	if !ok {
		return
	}

	res, err := handler.service.UnlinkMachine(r.Context(), name, machineID)
	if err != nil {
		utils.MapByError(w, err, map[error]int{
			errors.ErrGroupNameRequired:  http.StatusBadRequest,
			errors.ErrGroupNotFound:      http.StatusNotFound,
			errors.ErrMachineNotFound:    http.StatusNotFound,
			errors.ErrMachineNotInGroup:  http.StatusNotFound,
		})

		return
	}

	utils.EncodeToJSON(w, http.StatusOK, res)
}

// parseMachineID decodes the named URL parameter into an integer. If the
// parameter is not a valid number, it writes an HTTP 400 Bad Request and
// returns false.
func parseMachineID(w http.ResponseWriter, r *http.Request, paramName string) (int64, bool) {
	rawID := chi.URLParam(r, paramName)

	id, err := strconv.ParseInt(rawID, 10, 64)
	if err != nil {
		utils.EmitError(w, "invalid machine id", http.StatusBadRequest)
		return 0, false
	}

	return id, true
}
```

Add `"strconv"` to the imports.

- [ ] **Step 5: Run the handler tests**

Run: `go test ./internal/orchestrator/api/handler/ -v`
Expected: PASS.

- [ ] **Step 6: Run the full suite and build**

Run: `make tests && make build`
Expected: all tests PASS and both binaries build.

---

## Notes for the Implementer

- `Machine.GroupName` is a read-time projection populated by the `LEFT JOIN`; it is never written directly.
- `LinkToGroup` intentionally uses a plain `UPDATE`, so relinking is an overwrite (criterion 3, one group per machine).
- Criterion 6 is enforced by the FK `ON DELETE SET NULL`; the `foreign_keys(ON)` pragma is already set in both production (`infra/database.go`) and tests (`arrangeStoreDB`).
- Keep new JSON-free field style consistent with the existing schemas (no tags are present today).
