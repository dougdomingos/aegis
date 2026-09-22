package service

import (
	"context"
	"fmt"

	"dougdomingos.com/aegis/internal/domain"
	"dougdomingos.com/aegis/internal/errors"
	"dougdomingos.com/aegis/internal/schemas"
)

// RuleService provides all operations needed to manage rules in the system.
type RuleService struct {

	// store provides database operations related to rules
	store domain.RuleStore
}

// NewRuleService creates a new service instance with the provided store
// manager.
func NewRuleService(store domain.RuleStore) *RuleService {
	return &RuleService{store: store}
}

func (service *RuleService) CreateRule(ctx context.Context, p schemas.CreateRuleSchema) (*schemas.RuleOutputSchema, error) {
	rulePayload := domain.Rule{
		Type:     p.Type,
		Action:   p.Action,
		Value:    p.Value,
		Protocol: p.Protocol,
		Port:     p.Port,
	}

	if err := rulePayload.Validate(); err != nil {
		return nil, err
	}

	newRule, err := service.store.Create(ctx, rulePayload)
	if err != nil {
		return nil, fmt.Errorf("failed to register new rule: %w", err)
	}

	return mapRuleToOutputSchema(newRule), nil
}

func (service *RuleService) GetRuleByID(ctx context.Context, p schemas.GetRuleByIDSchema) (*schemas.RuleOutputSchema, error) {
	existentRule, err := service.store.GetByID(ctx, int64(p.ID))
	if err != nil {
		return nil, err
	}

	return mapRuleToOutputSchema(existentRule), nil
}

func (service *RuleService) ListRules(ctx context.Context, p schemas.ListRulesSchema) ([]schemas.RuleOutputSchema, error) {
	filter := domain.NewRuleFilter().
		WithRuleType(p.Type).
		WithAction(p.Action).
		WithValue(p.Value).
		Build()

	rules, err := service.store.List(ctx, filter)
	if err != nil {
		return nil, err
	}

	output := make([]schemas.RuleOutputSchema, len(rules))
	for i, rule := range rules {
		output[i] = *mapRuleToOutputSchema(&rule)
	}

	return output, nil
}

func (service *RuleService) UpdateRule(ctx context.Context, p schemas.UpdateRuleSchema) (*schemas.RuleOutputSchema, error) {
	ruleID := int64(p.ID)
	rule, err := service.store.GetByID(ctx, ruleID)
	if err != nil {
		return nil, err
	}

	rule.Patch(domain.RulePatch{
		Action:   p.Action,
		Value:    p.Value,
		Protocol: p.Protocol,
		Port:     p.Port,
	})

	if err := rule.Validate(); err != nil {
		return nil, err
	}

	updatedRule, err := service.store.Update(ctx, *rule)
	if err != nil {
		return nil, err
	}

	return mapRuleToOutputSchema(updatedRule), nil
}

func (service *RuleService) RemoveRule(ctx context.Context, p schemas.RemoveRuleSchema) error {
	ruleID := int64(p.ID)
	rule, err := service.store.GetByID(ctx, ruleID)
	if err != nil {
		return err
	}

	if rule == nil {
		return errors.ErrRuleNotFound
	}

	if err := service.store.Remove(ctx, ruleID); err != nil {
		return fmt.Errorf("failed to remove rule %d: %v", ruleID, err)
	}

	return nil
}

func mapRuleToOutputSchema(rule *domain.Rule) *schemas.RuleOutputSchema {
	if rule == nil {
		return nil
	}

	return &schemas.RuleOutputSchema{
		ID:        int(rule.ID),
		Type:      rule.Type,
		Action:    rule.Action,
		Value:     rule.Value,
		Protocol:  rule.Protocol,
		Port:      rule.Port,
		CreatedAt: rule.CreatedAt,
	}
}
