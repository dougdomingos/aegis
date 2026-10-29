package store_test

import (
	"context"
	"errors"
	"testing"

	machineErrors "dougdomingos.com/aegis/internal/common/errors"
	"dougdomingos.com/aegis/internal/orchestrator/domain"
	"dougdomingos.com/aegis/internal/orchestrator/store"
	_ "modernc.org/sqlite"
)

// ============================================================================
// Upsert
// ============================================================================

func TestMachineStore_Upsert_WithNewIP_InsertsDisconnectedMachine(t *testing.T) {
	ctx, store := arrangeStoreTest(t, store.NewMachineStore)
	machine := domain.Machine{IP: "10.0.1.11", Name: "lcc1-011"}

	created, err := store.Upsert(ctx, machine)

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if created.ID == 0 {
		t.Error("expected non-zero ID for created machine")
	}

	if created.Status != domain.DisconnectedStatus {
		t.Errorf("expected status %q, got %q", domain.DisconnectedStatus, created.Status)
	}

	if created.LastSeenAt != nil {
		t.Errorf("expected nil last_seen_at, got %v", created.LastSeenAt)
	}

	if created.CreatedAt.IsZero() {
		t.Error("expected created_at timestamp to be populated")
	}
}

func TestMachineStore_Upsert_WithExistingIP_UpdatesNameOnly(t *testing.T) {
	ctx, store := arrangeStoreTest(t, store.NewMachineStore)
	machine := domain.Machine{IP: "10.0.1.11", Name: "lcc1-011"}

	created, err := store.Upsert(ctx, machine)
	if err != nil {
		t.Fatalf("failed to seed machine: %v", err)
	}

	touched, err := store.Touch(ctx, created.ID, "aa:bb:cc", domain.ConnectedStatus)
	if err != nil {
		t.Fatalf("failed to touch machine: %v", err)
	}

	updated, err := store.Upsert(ctx, domain.Machine{IP: machine.IP, Name: "lcc1-renamed"})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if updated.Name != "lcc1-renamed" {
		t.Errorf("expected name %q, got %q", "lcc1-renamed", updated.Name)
	}

	if updated.Status != domain.ConnectedStatus {
		t.Errorf("expected status to be preserved as %q, got %q", domain.ConnectedStatus, updated.Status)
	}

	if touched.LastSeenAt == nil || updated.LastSeenAt == nil {
		t.Error("expected last_seen_at to be preserved")
	}

	list, err := store.List(ctx)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if len(list) != 1 {
		t.Errorf("expected 1 machine after re-upsert, got %d", len(list))
	}
}

// ============================================================================
// GetByIP
// ============================================================================

func TestMachineStore_GetByIP_WithUnknownIP_ReturnsNil(t *testing.T) {
	ctx, store := arrangeStoreTest(t, store.NewMachineStore)

	machine, err := store.GetByIP(ctx, "10.0.1.99")

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if machine != nil {
		t.Errorf("expected nil machine, got %+v", machine)
	}
}

// ============================================================================
// List
// ============================================================================

func TestMachineStore_List_WithMultipleMachines_ReturnsAllMachines(t *testing.T) {
	ctx, store := arrangeStoreTest(t, store.NewMachineStore)

	for _, ip := range []string{"10.0.1.11", "10.0.1.12"} {
		if _, err := store.Upsert(ctx, domain.Machine{IP: ip, Name: ip}); err != nil {
			t.Fatalf("failed to seed machine %q: %v", ip, err)
		}
	}

	machines, err := store.List(ctx)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(machines) != 2 {
		t.Errorf("expected 2 machines, got %d", len(machines))
	}
}

// ============================================================================
// RemoveByIP
// ============================================================================

func TestMachineStore_RemoveByIP_WithExistingIP_RemovesMachine(t *testing.T) {
	ctx, store := arrangeStoreTest(t, store.NewMachineStore)
	machine := domain.Machine{IP: "10.0.1.11", Name: "lcc1-011"}

	if _, err := store.Upsert(ctx, machine); err != nil {
		t.Fatalf("failed to seed machine: %v", err)
	}

	if err := store.RemoveByIP(ctx, machine.IP); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	removed, err := store.GetByIP(ctx, machine.IP)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if removed != nil {
		t.Errorf("expected machine to be removed, got %+v", removed)
	}
}

// ============================================================================
// Sync
// ============================================================================

func TestMachineStore_Sync_WithSourcesAndPruning_Reconciles(t *testing.T) {
	ctx, store := arrangeStoreTest(t, store.NewMachineStore)

	if _, err := store.Upsert(ctx, domain.Machine{IP: "10.0.1.11", Name: "lcc1-011"}); err != nil {
		t.Fatalf("failed to seed machine: %v", err)
	}

	err := store.Sync(ctx, []domain.Machine{
		{IP: "10.0.1.12", Name: "lcc1-012"},
	})

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	removed, err := store.GetByIP(ctx, "10.0.1.11")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if removed != nil {
		t.Errorf("expected pruned machine to be nil, got %+v", removed)
	}

	listed, err := store.List(ctx)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if len(listed) != 1 {
		t.Fatalf("expected 1 machine, got %d", len(listed))
	}

	if listed[0].IP != "10.0.1.12" {
		t.Errorf("expected IP %q, got %q", "10.0.1.12", listed[0].IP)
	}
}

// ============================================================================
// Touch
// ============================================================================

func TestMachineStore_Touch_WithExistingMachine_UpdatesStatusMACAndLastSeen(t *testing.T) {
	ctx, store := arrangeStoreTest(t, store.NewMachineStore)
	machine := domain.Machine{IP: "10.0.1.11", Name: "lcc1-011"}

	created, err := store.Upsert(ctx, machine)
	if err != nil {
		t.Fatalf("failed to seed machine: %v", err)
	}

	touched, err := store.Touch(ctx, created.ID, "aa:bb:cc", domain.ConnectedStatus)

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if touched.MAC != "aa:bb:cc" {
		t.Errorf("expected MAC %q, got %q", "aa:bb:cc", touched.MAC)
	}

	if touched.Status != domain.ConnectedStatus {
		t.Errorf("expected status %q, got %q", domain.ConnectedStatus, touched.Status)
	}

	if touched.LastSeenAt == nil {
		t.Error("expected last_seen_at to be populated")
	}
}

func TestMachineStore_Touch_EmptyMAC_PreservesStoredMAC(t *testing.T) {
	ctx, store := arrangeStoreTest(t, store.NewMachineStore)

	created, err := store.Upsert(ctx, domain.Machine{IP: "10.0.1.11", Name: "lcc1-011"})
	if err != nil {
		t.Fatalf("failed to seed machine: %v", err)
	}

	if _, err := store.Touch(ctx, created.ID, "aa:bb:cc:dd:ee:ff", domain.ConnectedStatus); err != nil {
		t.Fatalf("failed to seed machine MAC: %v", err)
	}

	touched, err := store.Touch(ctx, created.ID, "", domain.ConnectedStatus)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if touched.MAC != "aa:bb:cc:dd:ee:ff" {
		t.Errorf("expected stored MAC to be preserved, got %q", touched.MAC)
	}

	if touched.Status != domain.ConnectedStatus {
		t.Errorf("expected status %q, got %q", domain.ConnectedStatus, touched.Status)
	}

	if touched.LastSeenAt == nil {
		t.Error("expected last_seen_at to be populated")
	}
}

func TestMachineStore_Touch_WithUnknownID_ReturnsMachineNotFoundError(t *testing.T) {
	ctx, store := arrangeStoreTest(t, store.NewMachineStore)

	_, err := store.Touch(ctx, 999, "aa:bb:cc", domain.ConnectedStatus)
	if !errors.Is(err, machineErrors.ErrMachineNotFound) {
		t.Errorf("expected ErrMachineNotFound, got: %v", err)
	}
}

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

// ============================================================================
// Helpers
// ============================================================================

// arrangeMachineGroupStoreTest builds both stores over a migrated database.
func arrangeMachineGroupStoreTest(t *testing.T) (context.Context, *store.MachineStore, *store.GroupStore) {
	t.Helper()

	ctx, db := arrangeStoreDB(t)
	return ctx, store.NewMachineStore(db), store.NewGroupStore(db)
}
