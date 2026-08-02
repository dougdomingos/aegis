package group_test

import (
	"context"
	"fmt"
	"time"

	"dougdomingos.com/aegis/internal/domain"
	"github.com/google/uuid"
)

// MockGroupStore provides an in-memory implementation of domain.GroupStore
// for testing service operations.
//
// Each method also provides an injectable error field for testing scenarios
// where the store provider fails.
type MockGroupStore struct {
	groups   map[string]*domain.Group
	StoreErr error
}

// NewMockGroupStore initializes a new MockGroupStore instance.
func NewMockGroupStore() *MockGroupStore {
	return &MockGroupStore{
		groups: make(map[string]*domain.Group),
	}
}

// Create persists a new group or returns StoreErr if set.
func (mock *MockGroupStore) Create(ctx context.Context, name string) (*domain.Group, error) {
	if mock.StoreErr != nil {
		return nil, mock.StoreErr
	}

	if _, exists := mock.groups[name]; exists {
		return nil, fmt.Errorf("group %s already exists", name)
	}

	group := &domain.Group{
		ID:        uuid.New(),
		Name:      name,
		CreatedAt: time.Now(),
	}
	mock.groups[name] = group

	return group, nil
}

// GetByName retrieves a group by name or returns StoreErr if set.
func (mock *MockGroupStore) GetByName(ctx context.Context, name string) (*domain.Group, error) {
	if mock.StoreErr != nil {
		return nil, mock.StoreErr
	}

	group, exists := mock.groups[name]
	if !exists {
		return nil, nil
	}

	return group, nil
}

// Exists checks whether a group with the given name exists.
func (mock *MockGroupStore) Exists(ctx context.Context, name string) (bool, error) {
	if mock.StoreErr != nil {
		return false, mock.StoreErr
	}

	_, exists := mock.groups[name]

	return exists, nil
}

// Update modifies an existing group or returns StoreErr if set.
func (mock *MockGroupStore) Update(ctx context.Context, group domain.Group) (*domain.Group, error) {
	if mock.StoreErr != nil {
		return nil, mock.StoreErr
	}

	if _, exists := mock.groups[group.Name]; !exists {
		return nil, fmt.Errorf("group %s not found", group.Name)
	}

	mock.groups[group.Name] = &group
	return &group, nil
}

// List returns all stored groups or returns StoreErr if set.
func (mock *MockGroupStore) List(ctx context.Context) ([]*domain.Group, error) {
	if mock.StoreErr != nil {
		return nil, mock.StoreErr
	}

	list := make([]*domain.Group, 0, len(mock.groups))
	for _, group := range mock.groups {
		list = append(list, group)
	}

	return list, nil
}

// Remove deletes a group by name or returns StoreErr if set.
func (mock *MockGroupStore) Remove(ctx context.Context, name string) error {
	if mock.StoreErr != nil {
		return mock.StoreErr
	}

	if _, exists := mock.groups[name]; !exists {
		return fmt.Errorf("group %s not found", name)
	}

	delete(mock.groups, name)
	return nil
}
