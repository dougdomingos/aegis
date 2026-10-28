package service_test

import (
	"context"
	"fmt"
	"time"

	"dougdomingos.com/aegis/internal/domain"
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

// Sync reconciles the stored machines with the provided accepted sources,
// upserting each configured machine and pruning any whose IP is no longer
// configured.
func (mock *MockMachineStore) Sync(ctx context.Context, machines []domain.Machine) error {
	if mock.StoreErr != nil {
		return mock.StoreErr
	}

	configured := make(map[string]bool, len(machines))

	for _, machine := range machines {
		if _, err := mock.Upsert(ctx, machine); err != nil {
			return err
		}

		configured[machine.IP] = true
	}

	for ip := range mock.machines {
		if !configured[ip] {
			delete(mock.machines, ip)
		}
	}

	return nil
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

	return nil, fmt.Errorf("machine with id %d not found", id)
}
