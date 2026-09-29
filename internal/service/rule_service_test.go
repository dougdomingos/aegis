package service_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"dougdomingos.com/aegis/internal/domain"
	ruleErrors "dougdomingos.com/aegis/internal/errors"
	"dougdomingos.com/aegis/internal/schemas"
	"dougdomingos.com/aegis/internal/service"
)

// ============================================================================
// CreateRule
// ============================================================================

func TestRuleService_CreateRule_WithValidPayload_AcceptsCreations(t *testing.T) {
	ctx, env, service := arrangeRuleServiceTest(t)

	testCases := map[string]domain.Rule{
		"Domain rule": domain.NewDomainRule("test.com").Build(),
		"IP rule":     domain.NewIPRule("127.0.0.1").Build(),
		"IP rule with all fields": domain.NewIPRule("::1").
			WithProtocol("TCP").
			WithPort(3306).
			Build(),
	}

	for testName, tt := range testCases {
		t.Run(testName, func(t *testing.T) {
			payload := schemas.CreateRuleSchema{
				PolicyID: env.policyID,
				Type:     tt.Type,
				Value:    tt.Value,
				Protocol: tt.Protocol,
				Port:     tt.Port,
			}

			result, err := service.CreateRule(ctx, payload)
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}

			if result == nil {
				t.Fatalf("expected rule to be returned, got %v", err)
			}

			if result.ID == 0 {
				t.Error("expected non-zero ID for created rule")
			}

			if result.PolicyID != env.policyID {
				t.Errorf("expected policy ID %d, got %d", env.policyID, result.PolicyID)
			}

			rule := mapOutputToRule(*result)
			if !tt.IsEqual(&rule) {
				t.Errorf("expected returned data to match payload %+v, got %+v", payload, result)
			}

			if result.CreatedAt.IsZero() {
				t.Error("expected rule to have creation timestamp")
			}
		})
	}
}

func TestRuleService_CreateRule_WithInexistentPolicy_RejectsCreation(t *testing.T) {
	ctx, _, service := arrangeRuleServiceTest(t)
	payload := schemas.CreateRuleSchema{
		PolicyID: 9999,
		Type:     domain.DomainRuleType,
		Value:    "test.com",
	}

	rule, err := service.CreateRule(ctx, payload)
	if !errors.Is(err, ruleErrors.ErrPolicyNotFound) {
		t.Errorf("expected error %q, got %q", ruleErrors.ErrPolicyNotFound, err)
	}

	if rule != nil {
		t.Errorf("expected rule to be nil, got %+v", rule)
	}
}

func TestRuleService_CreateRule_WithEmptyRuleType_RejectsCreation(t *testing.T) {
	ctx, env, service := arrangeRuleServiceTest(t)
	payload := schemas.CreateRuleSchema{
		PolicyID: env.policyID,
		Value:    "test.com",
	}

	rule, err := service.CreateRule(ctx, payload)
	if !errors.Is(err, ruleErrors.ErrRuleTypeRequired) {
		t.Errorf("expected error %q, got %q", ruleErrors.ErrRuleTypeRequired, err)
	}

	if rule != nil {
		t.Errorf("expected rule to be nil, got %+v", rule)
	}
}

func TestRuleService_CreateRule_WithEmptyValue_RejectsCreation(t *testing.T) {
	ctx, env, service := arrangeRuleServiceTest(t)
	payload := schemas.CreateRuleSchema{
		PolicyID: env.policyID,
		Type:     domain.DomainRuleType,
	}

	rule, err := service.CreateRule(ctx, payload)
	if !errors.Is(err, ruleErrors.ErrRuleValueRequired) {
		t.Errorf("expected error %q, got %q", ruleErrors.ErrRuleValueRequired, err)
	}

	if rule != nil {
		t.Errorf("expected rule to be nil, got %+v", rule)
	}
}

func TestRuleService_CreateRule_WithMalformedValue_RejectsCreation(t *testing.T) {
	ctx, env, service := arrangeRuleServiceTest(t)

	testCases := map[string]schemas.CreateRuleSchema{
		"Domain rules": {
			PolicyID: env.policyID,
			Type:     domain.DomainRuleType,
			Value:    "thisisnotadomain",
		},

		"IP rules": {
			PolicyID: env.policyID,
			Type:     domain.IPRuleType,
			Value:    "thisisnotanip",
		},
	}

	for testName, tt := range testCases {
		t.Run(testName, func(t *testing.T) {
			rule, err := service.CreateRule(ctx, tt)

			if !errors.Is(err, ruleErrors.ErrMalformedRuleValue) {
				t.Errorf("expected error %q, got %q", ruleErrors.ErrMalformedRuleValue, err)
			}

			if rule != nil {
				t.Errorf("expected rule to be nil, got %+v", rule)
			}
		})
	}

}

func TestRuleService_CreateRule_WithDomainRuleAndProtocolOrPort_RejectsCreation(t *testing.T) {
	ctx, env, service := arrangeRuleServiceTest(t)
	protocol, port := "TCP", 5432

	testCases := map[string]schemas.CreateRuleSchema{
		"Domain rule with protocol": {
			PolicyID: env.policyID,
			Type:     domain.DomainRuleType,
			Value:    "test.com",
			Protocol: &protocol,
		},
		"Domain rule with port": {
			PolicyID: env.policyID,
			Type:     domain.DomainRuleType,
			Value:    "test.com",
			Port:     &port,
		},
	}

	for testName, tt := range testCases {
		t.Run(testName, func(t *testing.T) {
			rule, err := service.CreateRule(ctx, tt)
			if !errors.Is(err, ruleErrors.ErrInvalidFieldForRuleType) {
				t.Errorf("expected error %q, got %q", ruleErrors.ErrInvalidFieldForRuleType, err)
			}

			if rule != nil {
				t.Errorf("expected rule to be nil, got %+v", rule)
			}
		})
	}
}

func TestRuleService_CreateRule_WithIPRuleAndInvalidPort_RejectsCreation(t *testing.T) {
	ctx, env, service := arrangeRuleServiceTest(t)
	testPorts := []int{-99999, -1, 65536, 99999}
	basePayload := schemas.CreateRuleSchema{
		PolicyID: env.policyID,
		Type:     domain.IPRuleType,
		Value:    "10.0.0.1",
	}

	for _, port := range testPorts {
		t.Run(fmt.Sprintf("Port %d", port), func(t *testing.T) {
			basePayload.Port = &port

			rule, err := service.CreateRule(ctx, basePayload)
			if !errors.Is(err, ruleErrors.ErrInvalidRulePortValue) {
				t.Errorf("expected error %q, got %q", ruleErrors.ErrInvalidRulePortValue, err)
			}

			if rule != nil {
				t.Errorf("expected rule to be nil, got %+v", rule)
			}
		})
	}

}

func TestRuleService_CreateRule_WhenStoreFails_RejectsCreation(t *testing.T) {
	ctx, env, service := arrangeRuleServiceTest(t)
	expectedErr := errors.New("failed to insert rule into database")
	env.rules.StoreErr = expectedErr

	payload := schemas.CreateRuleSchema{
		PolicyID: env.policyID,
		Type:     domain.DomainRuleType,
		Value:    "test.com",
	}

	if _, err := service.CreateRule(ctx, payload); !errors.Is(err, expectedErr) {
		t.Errorf("expected store error %q, got %q", expectedErr, err)
	}
}

func TestRuleService_CreateRule_WhenPolicyStoreFails_RejectsCreation(t *testing.T) {
	ctx, env, service := arrangeRuleServiceTest(t)
	expectedErr := errors.New("failed to query database for policy")
	env.policies.StoreErr = expectedErr

	payload := schemas.CreateRuleSchema{
		PolicyID: env.policyID,
		Type:     domain.DomainRuleType,
		Value:    "test.com",
	}

	if _, err := service.CreateRule(ctx, payload); !errors.Is(err, expectedErr) {
		t.Errorf("expected store error %q, got %q", expectedErr, err)
	}
}

// ============================================================================
// GetRuleByID
// ============================================================================

func TestRuleService_GetRuleByID_WithSeededRule_ReturnsRule(t *testing.T) {
	ctx, env, service := arrangeRuleServiceTest(t)
	seededRule := seedRule(t, ctx, env.rules,
		domain.NewDomainRule("test.com").WithPolicy(env.policyID).Build())
	payload := schemas.GetRuleByIDSchema{PolicyID: env.policyID, ID: int(seededRule.ID)}

	result, err := service.GetRuleByID(ctx, payload)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if result == nil {
		t.Fatal("expected rule to be returned, got nil")
	}

	if int64(result.ID) != seededRule.ID {
		t.Errorf("expected rule ID %d, got %d", seededRule.ID, result.ID)
	}

	rule := mapOutputToRule(*result)
	if !seededRule.IsEqual(&rule) {
		t.Errorf("expected result to match seed %+v, got %+v", seededRule, result)
	}

	if result.CreatedAt.IsZero() {
		t.Error("expected rule to have creation timestamp")
	}
}

func TestRuleService_GetRuleByID_WithNonExistentRule_ReturnsNotFound(t *testing.T) {
	ctx, env, service := arrangeRuleServiceTest(t)

	rule, err := service.GetRuleByID(ctx, schemas.GetRuleByIDSchema{
		PolicyID: env.policyID,
		ID:       9999,
	})

	if !errors.Is(err, ruleErrors.ErrRuleNotFound) {
		t.Errorf("expected error %q, got %q", ruleErrors.ErrRuleNotFound, err)
	}

	if rule != nil {
		t.Errorf("expected rule to be nil, got %+v", rule)
	}
}

func TestRuleService_GetRuleByID_WithRuleOfAnotherPolicy_ReturnsNotFound(t *testing.T) {
	ctx, env, service := arrangeRuleServiceTest(t)
	otherRule := seedRuleInOtherPolicy(t, ctx, env, "theirs.com")

	rule, err := service.GetRuleByID(ctx, schemas.GetRuleByIDSchema{
		PolicyID: env.policyID,
		ID:       int(otherRule.ID),
	})

	if !errors.Is(err, ruleErrors.ErrRuleNotFound) {
		t.Errorf("expected error %q, got %q", ruleErrors.ErrRuleNotFound, err)
	}

	if rule != nil {
		t.Errorf("expected rule to be nil, got %+v", rule)
	}
}

func TestRuleService_GetRuleByID_WhenStoreFails_RejectsFetch(t *testing.T) {
	ctx, env, service := arrangeRuleServiceTest(t)
	expectedErr := errors.New("failed to query database for rule")
	env.rules.StoreErr = expectedErr

	payload := schemas.GetRuleByIDSchema{PolicyID: env.policyID, ID: 1}

	if _, err := service.GetRuleByID(ctx, payload); !errors.Is(err, expectedErr) {
		t.Errorf("expected store error %q, got %q", expectedErr, err)
	}
}

// ============================================================================
// ListRules
// ============================================================================

func TestRuleService_ListRules_WithSeededRules_ReturnsAllRules(t *testing.T) {
	ctx, env, service := arrangeRuleServiceTest(t)

	rulesToSeed := []domain.Rule{
		domain.NewDomainRule("test.com").WithPolicy(env.policyID).Build(),
		domain.NewDomainRule("google.com").WithPolicy(env.policyID).Build(),
		domain.NewIPRule("10.0.0.1").WithPolicy(env.policyID).Build(),
	}

	for _, rule := range rulesToSeed {
		seedRule(t, ctx, env.rules, rule)
	}

	rules, err := service.ListRules(ctx, schemas.ListRulesSchema{PolicyID: env.policyID})

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	expectedCount, actualCount := len(rulesToSeed), len(rules)
	if len(rules) != 3 {
		t.Errorf("expected %d rules, got %d", expectedCount, actualCount)
	}
}

func TestRuleService_ListRules_WithSeededRulesAndFilter_ReturnsMatchingRules(t *testing.T) {
	ctx, env, service := arrangeRuleServiceTest(t)

	rulesToSeed := []domain.Rule{
		domain.NewDomainRule("test.com").WithPolicy(env.policyID).Build(),
		domain.NewDomainRule("google.com").WithPolicy(env.policyID).Build(),
		domain.NewIPRule("10.0.0.1").WithPolicy(env.policyID).Build(),
		domain.NewIPRule("127.0.0.1").WithPolicy(env.policyID).Build(),
	}

	for _, rule := range rulesToSeed {
		seedRule(t, ctx, env.rules, rule)
	}

	filters := map[string]*domain.RuleFilterBuilder{
		"Only domain rules":         domain.NewRuleFilter().WithRuleType(new(domain.DomainRuleType)),
		"Only IP rules":             domain.NewRuleFilter().WithRuleType(new(domain.IPRuleType)),
		"Rules that end with .com":  domain.NewRuleFilter().WithValue(new(".com")),
		"Rules that end with 0.0.1": domain.NewRuleFilter().WithValue(new("0.0.1")),
		"Domain rules with .com":    domain.NewRuleFilter().WithRuleType(new(domain.DomainRuleType)).WithValue(new(".com")),
		"IP rules with 0.0.1":       domain.NewRuleFilter().WithRuleType(new(domain.IPRuleType)).WithValue(new("0.0.1")),
	}

	for filterName, tt := range filters {
		filter := tt.WithPolicyID(env.policyID).Build()

		t.Run(filterName, func(t *testing.T) {
			results, err := service.ListRules(ctx, schemas.ListRulesSchema{
				PolicyID: env.policyID,
				Type:     filter.Type,
				Value:    filter.Value,
			})

			if err != nil {
				t.Errorf("expected no errors, got %v", err)
			}

			if len(results) == 0 {
				t.Errorf("expected result to be non-empty list, got %v", results)
			}

			for _, result := range results {
				rule := mapOutputToRule(result)
				if !filter.Matches(rule) {
					t.Errorf("expected result rules to match filter, found %+v", rule)
				}
			}
		})
	}
}

func TestRuleService_ListRules_WithRulesOfAnotherPolicy_ReturnsOnlyScopedRules(t *testing.T) {
	ctx, env, service := arrangeRuleServiceTest(t)

	seedRule(t, ctx, env.rules, domain.NewDomainRule("mine.com").WithPolicy(env.policyID).Build())
	seedRuleInOtherPolicy(t, ctx, env, "theirs.com")

	rules, err := service.ListRules(ctx, schemas.ListRulesSchema{PolicyID: env.policyID})

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(rules))
	}

	if rules[0].PolicyID != env.policyID {
		t.Errorf("expected rule owned by policy %d, got %d", env.policyID, rules[0].PolicyID)
	}
}

func TestRuleService_ListRules_WithInexistentPolicy_RejectsFetch(t *testing.T) {
	ctx, _, service := arrangeRuleServiceTest(t)

	rules, err := service.ListRules(ctx, schemas.ListRulesSchema{PolicyID: 9999})

	if !errors.Is(err, ruleErrors.ErrPolicyNotFound) {
		t.Errorf("expected error %q, got %q", ruleErrors.ErrPolicyNotFound, err)
	}

	if rules != nil {
		t.Errorf("expected rules to be nil, got %+v", rules)
	}
}

func TestRuleService_ListRules_WithNoRulesPresent_ReturnsEmptyList(t *testing.T) {
	ctx, env, service := arrangeRuleServiceTest(t)

	result, err := service.ListRules(ctx, schemas.ListRulesSchema{PolicyID: env.policyID})

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if len(result) != 0 {
		t.Errorf("expected empty list, got %d rules", len(result))
	}
}

func TestRuleService_ListRules_WhenStoreFails_RejectsFetch(t *testing.T) {
	ctx, env, service := arrangeRuleServiceTest(t)
	expectedErr := errors.New("failed to query database for rules")
	env.rules.StoreErr = expectedErr

	if _, err := service.ListRules(ctx, schemas.ListRulesSchema{PolicyID: env.policyID}); !errors.Is(err, expectedErr) {
		t.Errorf("expected store error %q, got %q", expectedErr, err)
	}
}

func TestRuleService_ListRules_WhenPolicyStoreFails_RejectsFetch(t *testing.T) {
	ctx, env, service := arrangeRuleServiceTest(t)
	expectedErr := errors.New("failed to query database for policy")
	env.policies.StoreErr = expectedErr

	if _, err := service.ListRules(ctx, schemas.ListRulesSchema{PolicyID: env.policyID}); !errors.Is(err, expectedErr) {
		t.Errorf("expected store error %q, got %q", expectedErr, err)
	}
}

// ============================================================================
// UpdateRule
/// ============================================================================

func TestRuleService_UpdateDomainRule_WithValidPayload_AcceptsUpdate(t *testing.T) {
	ctx, env, service := arrangeRuleServiceTest(t)
	seededRule := seedRule(t, ctx, env.rules,
		domain.NewDomainRule("test.com").WithPolicy(env.policyID).Build())
	newValue := "google.com"

	updatedRule, err := service.UpdateRule(
		ctx,
		schemas.UpdateRuleSchema{PolicyID: env.policyID, ID: int(seededRule.ID), Value: &newValue},
	)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if updatedRule.Value != newValue {
		t.Errorf("expected rule value to be %q, got %q", newValue, updatedRule.Value)
	}
}

func TestRuleService_UpdateIPRule_WithValidPayload_AcceptsUpdate(t *testing.T) {
	ctx, env, service := arrangeRuleServiceTest(t)
	seededRule := seedRule(t, ctx, env.rules,
		domain.NewIPRule("127.0.0.1").WithPolicy(env.policyID).Build())
	newValue := "10.0.0.1"
	protocol, port := "TCP", 5432

	updatedRule, err := service.UpdateRule(
		ctx,
		schemas.UpdateRuleSchema{
			PolicyID: env.policyID,
			ID:       int(seededRule.ID),
			Value:    &newValue,
			Protocol: &protocol,
			Port:     &port,
		},
	)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if updatedRule.Value != newValue {
		t.Errorf("expected rule value to be %q, got %q", newValue, updatedRule.Value)
	}

	if *updatedRule.Protocol != protocol {
		t.Errorf("expected rule protocol to be %q, got %q", protocol, *updatedRule.Protocol)
	}

	if *updatedRule.Port != port {
		t.Errorf("expected rule port to be %d, got %d", port, *updatedRule.Port)
	}
}

func TestRuleService_UpdateRule_WithEmptyValue_RejectsUpdate(t *testing.T) {
	ctx, env, service := arrangeRuleServiceTest(t)
	seededRule := seedRule(t, ctx, env.rules,
		domain.NewDomainRule("test.com").WithPolicy(env.policyID).Build())
	valueField := ""

	updatedRule, err := service.UpdateRule(
		ctx,
		schemas.UpdateRuleSchema{PolicyID: env.policyID, ID: int(seededRule.ID), Value: &valueField},
	)

	if !errors.Is(err, ruleErrors.ErrRuleValueRequired) {
		t.Errorf("expected error %q, got %q", ruleErrors.ErrRuleValueRequired, err)
	}

	if updatedRule != nil {
		t.Errorf("expected result to be nil, got %v", updatedRule)
	}
}

func TestRuleService_UpdateRule_WithMalformedValue_RejectsUpdate(t *testing.T) {
	ctx, env, service := arrangeRuleServiceTest(t)
	seededRule := seedRule(t, ctx, env.rules,
		domain.NewDomainRule("test.com").WithPolicy(env.policyID).Build())
	valueField := "thisisnotadomain"

	updatedRule, err := service.UpdateRule(
		ctx,
		schemas.UpdateRuleSchema{PolicyID: env.policyID, ID: int(seededRule.ID), Value: &valueField},
	)

	if !errors.Is(err, ruleErrors.ErrMalformedRuleValue) {
		t.Errorf("expected error %q, got %q", ruleErrors.ErrMalformedRuleValue, err)
	}

	if updatedRule != nil {
		t.Errorf("expected result to be nil, got %v", updatedRule)
	}
}

func TestRuleService_UpdateRule_WithMalformedValues_RejectsUpdate(t *testing.T) {
	ctx, env, service := arrangeRuleServiceTest(t)

	seededRules := map[string]*domain.Rule{
		"domain": seedRule(t, ctx, env.rules,
			domain.NewDomainRule("test.com").WithPolicy(env.policyID).Build()),
		"ip": seedRule(t, ctx, env.rules,
			domain.NewIPRule("10.0.0.1").WithPolicy(env.policyID).Build()),
	}

	testFields := map[string]string{
		"domain": "thisisnotadomain",
		"ip":     "thisisnotanip",
	}

	for ruleType, tt := range testFields {
		payload := schemas.UpdateRuleSchema{
			PolicyID: env.policyID,
			ID:       int(seededRules[ruleType].ID),
			Value:    &tt,
		}

		t.Run(fmt.Sprintf("Malformed %s value", ruleType), func(t *testing.T) {
			updatedRule, err := service.UpdateRule(ctx, payload)
			if !errors.Is(err, ruleErrors.ErrMalformedRuleValue) {
				t.Errorf("expected error %q, got %q", ruleErrors.ErrMalformedRuleValue, err)
			}

			if updatedRule != nil {
				t.Errorf("expected result to be nil, got %v", updatedRule)
			}
		})
	}
}

func TestRuleService_UpdateRule_WithIPRuleAndInvalidPort_RejectsUpdate(t *testing.T) {
	ctx, env, service := arrangeRuleServiceTest(t)
	seededRule := seedRule(t, ctx, env.rules,
		domain.NewIPRule("10.0.0.1").WithPolicy(env.policyID).Build())
	payload := schemas.UpdateRuleSchema{PolicyID: env.policyID, ID: int(seededRule.ID)}

	testPorts := []int{-99999, -1, 65536, 99999}

	for _, port := range testPorts {
		payload.Port = &port

		t.Run(fmt.Sprintf("Port %d", port), func(t *testing.T) {
			updatedRule, err := service.UpdateRule(ctx, payload)
			if !errors.Is(err, ruleErrors.ErrInvalidRulePortValue) {
				t.Errorf("expected error %q, got %q", ruleErrors.ErrInvalidRulePortValue, err)
			}

			if updatedRule != nil {
				t.Errorf("expected result to be nil, got %v", updatedRule)
			}
		})
	}
}

func TestRuleService_UpdateRule_WithRuleOfAnotherPolicy_RejectsUpdate(t *testing.T) {
	ctx, env, service := arrangeRuleServiceTest(t)
	otherRule := seedRuleInOtherPolicy(t, ctx, env, "theirs.com")
	newValue := "changed.com"

	updatedRule, err := service.UpdateRule(
		ctx,
		schemas.UpdateRuleSchema{PolicyID: env.policyID, ID: int(otherRule.ID), Value: &newValue},
	)

	if !errors.Is(err, ruleErrors.ErrRuleNotFound) {
		t.Errorf("expected error %q, got %q", ruleErrors.ErrRuleNotFound, err)
	}

	if updatedRule != nil {
		t.Errorf("expected result to be nil, got %v", updatedRule)
	}
}

func TestRuleService_UpdateRule_WhenStoreFails_RejectsFetch(t *testing.T) {
	ctx, env, service := arrangeRuleServiceTest(t)
	expectedErr := errors.New("failed to update rule in database")
	env.rules.StoreErr = expectedErr

	payload := schemas.UpdateRuleSchema{PolicyID: env.policyID, ID: 1}

	if _, err := service.UpdateRule(ctx, payload); !errors.Is(err, expectedErr) {
		t.Errorf("expected store error %q, got %q", expectedErr, err)
	}
}

// ============================================================================
// RemoveRule
// ============================================================================

func TestRuleService_RemoveRule_WithExistentRule_RemovesRule(t *testing.T) {
	ctx, env, service := arrangeRuleServiceTest(t)
	seededRule := seedRule(t, ctx, env.rules,
		domain.NewDomainRule("test.com").WithPolicy(env.policyID).Build())

	err := service.RemoveRule(ctx, schemas.RemoveRuleSchema{
		PolicyID: env.policyID,
		ID:       int(seededRule.ID),
	})

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if rule, _ := env.rules.GetByID(ctx, seededRule.ID); rule != nil {
		t.Errorf("expected rule to be deleted, got %+v", rule)
	}
}

func TestRuleService_RemoveRule_WithNonExistentRule_RejectsRemoval(t *testing.T) {
	ctx, env, service := arrangeRuleServiceTest(t)

	err := service.RemoveRule(ctx, schemas.RemoveRuleSchema{PolicyID: env.policyID, ID: 9999})

	if !errors.Is(err, ruleErrors.ErrRuleNotFound) {
		t.Errorf("expected error %q, got %q", ruleErrors.ErrRuleNotFound, err)
	}
}

func TestRuleService_RemoveRule_WithRuleOfAnotherPolicy_RejectsRemoval(t *testing.T) {
	ctx, env, service := arrangeRuleServiceTest(t)
	otherRule := seedRuleInOtherPolicy(t, ctx, env, "theirs.com")

	err := service.RemoveRule(ctx, schemas.RemoveRuleSchema{
		PolicyID: env.policyID,
		ID:       int(otherRule.ID),
	})

	if !errors.Is(err, ruleErrors.ErrRuleNotFound) {
		t.Errorf("expected error %q, got %q", ruleErrors.ErrRuleNotFound, err)
	}

	if rule, _ := env.rules.GetByID(ctx, otherRule.ID); rule == nil {
		t.Error("expected rule to be kept untouched")
	}
}

func TestRuleService_RemoveRule_WhenStoreFails_RejectsFetch(t *testing.T) {
	ctx, env, service := arrangeRuleServiceTest(t)
	expectedErr := errors.New("failed to query database for rule")
	env.rules.StoreErr = expectedErr

	if err := service.RemoveRule(ctx, schemas.RemoveRuleSchema{PolicyID: env.policyID, ID: 1}); !errors.Is(err, expectedErr) {
		t.Errorf("expected store error %q, got %q", expectedErr, err)
	}
}

// ============================================================================
// Helpers
// ============================================================================

// ruleServiceEnv bundles the mock stores and the seeded policy required by
// rule service test scenarios.
type ruleServiceEnv struct {
	rules    *MockRuleStore
	policies *MockPolicyStore
	policyID int64
}

// arrangeRuleServiceTest initializes a new service instance with mocked store
// providers and a seeded policy to own rules.
func arrangeRuleServiceTest(t *testing.T) (context.Context, *ruleServiceEnv, service.RuleService) {
	t.Helper()

	ctx := context.Background()
	env := &ruleServiceEnv{
		rules:    NewMockRuleStore(),
		policies: NewMockPolicyStore(),
	}

	seededPolicy, err := env.policies.Create(ctx, "Default Policy", domain.WhitelistPolicyType)
	if err != nil {
		t.Fatalf("failed to seed policy: %v", err)
	}

	env.policyID = seededPolicy.ID
	return ctx, env, *service.NewRuleService(env.rules, env.policies)
}

// seedRule creates one rule in the mock store and fails the test on error.
func seedRule(t *testing.T, ctx context.Context, store *MockRuleStore, rule domain.Rule) *domain.Rule {
	t.Helper()

	seededRule, err := store.Create(ctx, rule)
	if err != nil {
		t.Fatalf("failed to seed rule: %v", err)
	}

	return seededRule
}

// seedRuleInOtherPolicy creates one rule owned by a secondary policy, used to
// verify cross-policy isolation scenarios.
func seedRuleInOtherPolicy(t *testing.T, ctx context.Context, env *ruleServiceEnv, value string) *domain.Rule {
	t.Helper()

	otherPolicy, err := env.policies.Create(ctx, "Other Policy", domain.BlacklistPolicyType)
	if err != nil {
		t.Fatalf("failed to seed policy: %v", err)
	}

	return seedRule(t, ctx, env.rules, domain.NewDomainRule(value).WithPolicy(otherPolicy.ID).Build())
}

// mapOutputToRule maps the provided RuleOutputSchema to a domain.Rule
// instance.
func mapOutputToRule(output schemas.RuleOutputSchema) domain.Rule {
	return domain.Rule{
		PolicyID:  output.PolicyID,
		Type:      output.Type,
		Value:     output.Value,
		Protocol:  output.Protocol,
		Port:      output.Port,
		CreatedAt: output.CreatedAt,
	}
}
