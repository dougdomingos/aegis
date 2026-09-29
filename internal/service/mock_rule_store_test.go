package service_test

import (
	"context"
	"fmt"
	"time"

	"dougdomingos.com/aegis/internal/domain"
)

// MockRuleStore provides an in-memory implementation of domain.RuleStore
// for testing service operations.
//
// Each method also provides an injectable error field for testing scenarios
// where the store provider fails.
type MockRuleStore struct {
	rules    map[int64]*domain.Rule
	StoreErr error
	nextID   int64
}

// NewMockRuleStore initializes a new MockRuleStore instance.
func NewMockRuleStore() *MockRuleStore {
	return &MockRuleStore{
		rules: make(map[int64]*domain.Rule),
	}
}

// Create persists a new rule or returns StoreErr if set.
func (mock *MockRuleStore) Create(ctx context.Context, rule domain.Rule) (*domain.Rule, error) {
	if mock.StoreErr != nil {
		return nil, mock.StoreErr
	}

	mock.nextID++
	newRule := &domain.Rule{
		ID:        mock.nextID,
		PolicyID:  rule.PolicyID,
		Type:      rule.Type,
		Value:     rule.Value,
		Protocol:  rule.Protocol,
		Port:      rule.Port,
		CreatedAt: time.Now(),
	}
	mock.rules[newRule.ID] = newRule

	return newRule, nil
}

// GetByID retrieves a rule by its unique ID or returns StoreErr if set.
func (mock *MockRuleStore) GetByID(ctx context.Context, id int64) (*domain.Rule, error) {
	if mock.StoreErr != nil {
		return nil, mock.StoreErr
	}

	rule, exists := mock.rules[id]
	if !exists {
		return nil, nil
	}

	return rule, nil
}

// List returns all stored rules or returns StoreErr if set.
func (mock *MockRuleStore) List(ctx context.Context, filter domain.RuleFilter) ([]domain.Rule, error) {
	if mock.StoreErr != nil {
		return nil, mock.StoreErr
	}

	list := make([]domain.Rule, 0, len(mock.rules))
	for _, rule := range mock.rules {
		if !filter.Matches(*rule) {
			continue
		}

		list = append(list, *rule)
	}

	return list, nil
}

// Update modifies an existing rule or returns StoreErr if set.
func (mock *MockRuleStore) Update(ctx context.Context, rule domain.Rule) (*domain.Rule, error) {
	if mock.StoreErr != nil {
		return nil, mock.StoreErr
	}

	if _, exists := mock.rules[rule.ID]; !exists {
		return nil, fmt.Errorf("rule with id %d not found", rule.ID)
	}

	mock.rules[rule.ID] = &rule
	return &rule, nil
}

// Remove deletes a rule by name or returns StoreErr if set.
func (mock *MockRuleStore) Remove(ctx context.Context, id int64) error {
	if mock.StoreErr != nil {
		return mock.StoreErr
	}

	rule, err := mock.GetByID(ctx, id)
	if err != nil {
		return err
	}

	if rule == nil {
		return fmt.Errorf("rule of id %d not found", id)
	}

	delete(mock.rules, rule.ID)
	return nil
}
