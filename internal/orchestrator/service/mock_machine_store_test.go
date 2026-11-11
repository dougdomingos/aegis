package service_test

import (
	"context"
	"fmt"
	"time"

	machineErrors "dougdomingos.com/aegis/internal/common/errors"

	"dougdomingos.com/aegis/internal/orchestrator/domain"
)

// MockMachineStore provides an in-memory implementation of domain.MachineStore
// for testing service operations.
//
// Each method also provides an injectable error field for testing scenarios
// where the store provider fails.
type MockMachineStore struct {
	machines map[string]*domain.Machine
	StoreErr error
	nextID   int64

	// ImportGroupIDs maps lab names to the IDs of the groups resolved by Import.
	ImportGroupIDs map[string]int64
}

// NewMockMachineStore initializes a new MockMachineStore instance.
func NewMockMachineStore() *MockMachineStore {
	return &MockMachineStore{
		machines: make(map[string]*domain.Machine),
	}
}

// GetByIP retrieves a machine by its IP or returns StoreErr if set.
func (mock *MockMachineStore) GetByIP(ctx context.Context, ip string) (*domain.Machine, error) {
	if mock.StoreErr != nil {
		return nil, mock.StoreErr
	}

	machine, exists := mock.machines[ip]
	if !exists {
		return nil, nil
	}

	return machine, nil
}

// List returns all stored machines or returns StoreErr if set.
func (mock *MockMachineStore) List(ctx context.Context) ([]domain.Machine, error) {
	if mock.StoreErr != nil {
		return nil, mock.StoreErr
	}

	list := make([]domain.Machine, 0, len(mock.machines))
	for _, machine := range mock.machines {
		list = append(list, *machine)
	}

	return list, nil
}

// Upsert registers a machine or updates its name when its IP already exists,
// preserving the current connection state.
func (mock *MockMachineStore) Upsert(ctx context.Context, machine domain.Machine) (*domain.Machine, error) {
	if mock.StoreErr != nil {
		return nil, mock.StoreErr
	}

	if existing, exists := mock.machines[machine.IP]; exists {
		existing.Name = machine.Name
		return existing, nil
	}

	mock.nextID++
	stored := machine
	stored.ID = mock.nextID
	stored.Status = domain.DisconnectedStatus
	stored.CreatedAt = time.Now()
	mock.machines[stored.IP] = &stored

	return &stored, nil
}

// RemoveByIP deletes a machine by its IP or returns StoreErr if set.
func (mock *MockMachineStore) RemoveByIP(ctx context.Context, ip string) error {
	if mock.StoreErr != nil {
		return mock.StoreErr
	}

	delete(mock.machines, ip)
	return nil
}

// RemoveByID deletes a machine by its ID or returns StoreErr if set. An unknown
// ID returns ErrMachineNotFound, mirroring the real store.
func (mock *MockMachineStore) RemoveByID(ctx context.Context, id int64) error {
	if mock.StoreErr != nil {
		return mock.StoreErr
	}

	for ip, machine := range mock.machines {
		if machine.ID == id {
			delete(mock.machines, ip)
			return nil
		}
	}

	return fmt.Errorf("%w: id %d", machineErrors.ErrMachineNotFound, id)
}

// Import registers or renames each entry's machine and links it to the group
// resolved by ImportGroupIDs, or returns StoreErr if set. Labs without a
// configured group ID are counted as created groups and assigned new IDs.
func (mock *MockMachineStore) Import(ctx context.Context, entries []domain.MachineImport) (*domain.MachineImportResult, error) {
	if mock.StoreErr != nil {
		return nil, mock.StoreErr
	}

	if mock.ImportGroupIDs == nil {
		mock.ImportGroupIDs = make(map[string]int64)
	}

	result := &domain.MachineImportResult{}

	for _, entry := range entries {
		if _, exists := mock.machines[entry.Machine.IP]; exists {
			result.MachinesUpdated++
		} else {
			result.MachinesCreated++
		}

		machine, err := mock.Upsert(ctx, entry.Machine)
		if err != nil {
			return nil, err
		}

		if entry.Lab == "" {
			continue
		}

		groupID, exists := mock.ImportGroupIDs[entry.Lab]
		if !exists {
			groupID = int64(len(mock.ImportGroupIDs) + 1000)
			mock.ImportGroupIDs[entry.Lab] = groupID
			result.GroupsCreated++
		}

		if machine.GroupID != nil && *machine.GroupID == groupID {
			continue
		}

		machine.GroupID = &groupID
		machine.GroupName = entry.Lab
		result.RelinkedIDs = append(result.RelinkedIDs, machine.ID)
	}

	return result, nil
}

// Touch updates the connection state and last-seen timestamp of a machine.
func (mock *MockMachineStore) Touch(ctx context.Context, id int64, mac string, status domain.MachineStatus) (*domain.Machine, error) {
	if mock.StoreErr != nil {
		return nil, mock.StoreErr
	}

	for _, machine := range mock.machines {
		if machine.ID == id {
			if mac != "" {
				machine.MAC = mac
			}

			machine.Status = status
			now := time.Now()
			machine.LastSeenAt = &now
			return machine, nil
		}
	}

	return nil, fmt.Errorf("%w: id %d", machineErrors.ErrMachineNotFound, id)
}

// GetByID retrieves a machine by its unique ID or returns StoreErr if set.
// A cancelled context fails the read, mirroring the real store.
func (mock *MockMachineStore) GetByID(ctx context.Context, id int64) (*domain.Machine, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

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
