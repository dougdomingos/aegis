package service

import (
	"context"
	"fmt"

	"dougdomingos.com/aegis/internal/common/errors"
	"dougdomingos.com/aegis/internal/common/schemas"
	"dougdomingos.com/aegis/internal/orchestrator/domain"
)

// RuleService provides all operations needed to manage rules in the system.
type RuleService struct {

	// store provides database operations related to rules
	store domain.RuleStore

	// policies provides database operations related to policies, used to
	// scope rules into their owning policy.
	policies domain.PolicyStore

	// groups provides database operations related to groups, used to know
	// whether a policy is linked before its rules are removed.
	groups domain.GroupStore

	// notifier propagates rule changes to the distribution pipeline. It may be
	// nil, disabling notifications.
	notifier PropagationNotifier
}

// NewRuleService creates a new service instance with the provided store
// managers and propagation notifier. The notifier may be nil.
func NewRuleService(store domain.RuleStore, policies domain.PolicyStore, groups domain.GroupStore, notifier PropagationNotifier) *RuleService {
	return &RuleService{store: store, policies: policies, groups: groups, notifier: notifier}
}

// CreateRule registers a new rule within the referenced policy. The policy
// must exist, otherwise the operation fails.
func (service *RuleService) CreateRule(ctx context.Context, p schemas.CreateRuleSchema) (*schemas.RuleOutputSchema, error) {
	if err := service.ensurePolicyExists(ctx, p.PolicyID); err != nil {
		return nil, err
	}

	rulePayload := domain.Rule{
		PolicyID: p.PolicyID,
		Type:     p.Type,
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

	service.notifyPolicy(ctx, p.PolicyID)

	return mapRuleToOutputSchema(newRule), nil
}

// GetRuleByID retrieves a rule owned by the referenced policy. If no rule
// matches the requested ID within that policy, it returns nil.
func (service *RuleService) GetRuleByID(ctx context.Context, p schemas.GetRuleByIDSchema) (*schemas.RuleOutputSchema, error) {
	rule, err := service.getRuleFromPolicy(ctx, p.PolicyID, int64(p.ID))
	if err != nil {
		return nil, err
	}

	return mapRuleToOutputSchema(rule), nil
}

// ListRules returns all the rules that belong to the referenced policy and
// match the provided filters. The policy must exist, otherwise the operation
// fails.
func (service *RuleService) ListRules(ctx context.Context, p schemas.ListRulesSchema) ([]schemas.RuleOutputSchema, error) {
	if err := service.ensurePolicyExists(ctx, p.PolicyID); err != nil {
		return nil, err
	}

	filter := domain.NewRuleFilter().
		WithPolicyID(p.PolicyID).
		WithRuleType(p.Type).
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

// UpdateRule applies partial changes to a rule owned by the referenced
// policy. If no rule matches the requested ID within that policy, it returns
// an error.
func (service *RuleService) UpdateRule(ctx context.Context, p schemas.UpdateRuleSchema) (*schemas.RuleOutputSchema, error) {
	rule, err := service.getRuleFromPolicy(ctx, p.PolicyID, int64(p.ID))
	if err != nil {
		return nil, err
	}

	patched := *rule
	patched.Patch(domain.RulePatch{
		Value:    p.Value,
		Protocol: p.Protocol,
		Port:     p.Port,
	})

	if err := patched.Validate(); err != nil {
		return nil, err
	}

	if patched.IsEqual(rule) {
		return mapRuleToOutputSchema(rule), nil
	}

	updatedRule, err := service.store.Update(ctx, patched)
	if err != nil {
		return nil, err
	}

	service.notifyPolicy(ctx, p.PolicyID)

	return mapRuleToOutputSchema(updatedRule), nil
}

// RemoveRule deletes a rule owned by the referenced policy. If no rule
// matches the requested ID within that policy, it returns an error.
func (service *RuleService) RemoveRule(ctx context.Context, p schemas.RemoveRuleSchema) error {
	rule, err := service.getRuleFromPolicy(ctx, p.PolicyID, int64(p.ID))
	if err != nil {
		return err
	}

	linked, err := service.groups.IsPolicyLinked(ctx, rule.PolicyID)
	if err != nil {
		return err
	}

	if linked {
		ruleCount, err := service.store.CountByPolicy(ctx, rule.PolicyID)
		if err != nil {
			return err
		}
		if ruleCount <= 1 {
			return errors.ErrPolicyHasNoRules
		}
	}

	if err := service.store.Remove(ctx, rule.ID); err != nil {
		return fmt.Errorf("failed to remove rule %d: %v", rule.ID, err)
	}

	service.notifyPolicy(ctx, rule.PolicyID)

	return nil
}

// notifyPolicy best-effort marks the policy's machines pending with package
// intents.
func (service *RuleService) notifyPolicy(ctx context.Context, policyID int64) {
	if service.notifier == nil {
		return
	}

	service.notifier.NotifyPolicy(ctx, policyID)
}

// ensurePolicyExists checks whether a policy with the provided ID exists
// within the database.
func (service *RuleService) ensurePolicyExists(ctx context.Context, policyID int64) error {
	policy, err := service.policies.GetByID(ctx, policyID)
	if err != nil {
		return err
	}

	if policy == nil {
		return errors.ErrPolicyNotFound
	}

	return nil
}

// getRuleFromPolicy retrieves a rule by its ID, ensuring it belongs to the
// referenced policy. Non-existent rules and rules owned by other policies
// both result in ErrRuleNotFound.
func (service *RuleService) getRuleFromPolicy(ctx context.Context, policyID, ruleID int64) (*domain.Rule, error) {
	rule, err := service.store.GetByID(ctx, ruleID)
	if err != nil {
		return nil, err
	}

	if rule == nil || rule.PolicyID != policyID {
		return nil, errors.ErrRuleNotFound
	}

	return rule, nil
}

// mapRuleToOutputSchema converts the Rule entity format into the output
// schema provided by this service. Returns nil if the provided rule is nil.
func mapRuleToOutputSchema(rule *domain.Rule) *schemas.RuleOutputSchema {
	if rule == nil {
		return nil
	}

	return &schemas.RuleOutputSchema{
		ID:        int(rule.ID),
		PolicyID:  rule.PolicyID,
		Type:      rule.Type,
		Value:     rule.Value,
		Protocol:  rule.Protocol,
		Port:      rule.Port,
		CreatedAt: rule.CreatedAt,
	}
}
