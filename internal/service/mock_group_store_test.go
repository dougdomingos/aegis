package service_test

import (
	"context"
	"fmt"
	"time"

	"dougdomingos.com/aegis/internal/domain"
)

// MockGroupStore provides an in-memory implementation of domain.GroupStore
// for testing service operations.
//
// Each method also provides an injectable error field for testing scenarios
// where the store provider fails.
type MockGroupStore struct {
	groups   map[int64]*domain.Group
	StoreErr error
	nextID   int64
}

// NewMockGroupStore initializes a new MockGroupStore instance.
func NewMockGroupStore() *MockGroupStore {
	return &MockGroupStore{
		groups: make(map[int64]*domain.Group),
	}
}

// Create persists a new group or returns StoreErr if set.
func (mock *MockGroupStore) Create(ctx context.Context, name string) (*domain.Group, error) {
	if mock.StoreErr != nil {
		return nil, mock.StoreErr
	}

	for _, existing := range mock.groups {
		if existing.Name == name {
			return nil, fmt.Errorf("group %s already exists", name)
		}
	}

	mock.nextID++
	group := &domain.Group{
		ID:        mock.nextID,
		Name:      name,
		CreatedAt: time.Now(),
	}
	mock.groups[group.ID] = group

	return group, nil
}

// GetByID retrieves a group by its unique ID or returns StoreErr if set.
func (mock *MockGroupStore) GetByID(ctx context.Context, id int64) (*domain.Group, error) {
	if mock.StoreErr != nil {
		return nil, mock.StoreErr
	}

	group, exists := mock.groups[id]
	if !exists {
		return nil, nil
	}

	return group, nil
}

// GetByName retrieves a group by name or returns StoreErr if set.
func (mock *MockGroupStore) GetByName(ctx context.Context, name string) (*domain.Group, error) {
	if mock.StoreErr != nil {
		return nil, mock.StoreErr
	}

	for _, group := range mock.groups {
		if group.Name == name {
			return group, nil
		}
	}

	return nil, nil
}

// Exists checks whether a group with the given name exists.
func (mock *MockGroupStore) Exists(ctx context.Context, name string) (bool, error) {
	if mock.StoreErr != nil {
		return false, mock.StoreErr
	}

	group, err := mock.GetByName(ctx, name)
	if err != nil {
		return false, err
	}

	return group != nil, nil
}

// Update modifies an existing group or returns StoreErr if set.
func (mock *MockGroupStore) Update(ctx context.Context, group domain.Group) (*domain.Group, error) {
	if mock.StoreErr != nil {
		return nil, mock.StoreErr
	}

	if _, exists := mock.groups[group.ID]; !exists {
		return nil, fmt.Errorf("group with id %d not found", group.ID)
	}

	mock.groups[group.ID] = &group
	return &group, nil
}

// List returns all stored groups or returns StoreErr if set.
func (mock *MockGroupStore) List(ctx context.Context) ([]domain.Group, error) {
	if mock.StoreErr != nil {
		return nil, mock.StoreErr
	}

	list := make([]domain.Group, 0, len(mock.groups))
	for _, group := range mock.groups {
		list = append(list, *group)
	}

	return list, nil
}

// Remove deletes a group by name or returns StoreErr if set.
func (mock *MockGroupStore) Remove(ctx context.Context, name string) error {
	if mock.StoreErr != nil {
		return mock.StoreErr
	}

	group, err := mock.GetByName(ctx, name)
	if err != nil {
		return err
	}

	if group == nil {
		return fmt.Errorf("group %s not found", name)
	}

	delete(mock.groups, group.ID)
	return nil
}
