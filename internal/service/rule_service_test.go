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
	ctx, _, service := arrangeRuleServiceTest(t)

	testCases := map[string]domain.Rule{
		"Domain rule": domain.NewDomainRule(domain.AllowAction, "test.com").Build(),
		"IP rule":     domain.NewIPRule(domain.AllowAction, "127.0.0.1").Build(),
		"IP rule with all fields": domain.NewIPRule(domain.DenyAction, "::1").
			WithProtocol("TCP").
			WithPort(3306).
			Build(),
	}

	for testName, tt := range testCases {
		t.Run(testName, func(t *testing.T) {
			payload := schemas.CreateRuleSchema{
				Type:     tt.Type,
				Action:   tt.Action,
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

func TestRuleService_CreateRule_WithEmptyRuleType_RejectsCreation(t *testing.T) {
	ctx, _, service := arrangeRuleServiceTest(t)
	payload := schemas.CreateRuleSchema{
		Action: domain.AllowAction,
		Value:  "test.com",
	}

	rule, err := service.CreateRule(ctx, payload)
	if !errors.Is(err, ruleErrors.ErrRuleTypeRequired) {
		t.Errorf("expected error %q, got %q", ruleErrors.ErrRuleTypeRequired, err)
	}

	if rule != nil {
		t.Errorf("expected rule to be nil, got %+v", rule)
	}
}

func TestRuleService_CreateRule_WithEmptyAction_RejectsCreation(t *testing.T) {
	ctx, _, service := arrangeRuleServiceTest(t)
	payload := schemas.CreateRuleSchema{
		Type:  domain.DomainRuleType,
		Value: "test.com",
	}

	rule, err := service.CreateRule(ctx, payload)
	if !errors.Is(err, ruleErrors.ErrRuleActionRequired) {
		t.Errorf("expected error %q, got %q", ruleErrors.ErrRuleActionRequired, err)
	}

	if rule != nil {
		t.Errorf("expected rule to be nil, got %+v", rule)
	}
}

func TestRuleService_CreateRule_WithEmptyValue_RejectsCreation(t *testing.T) {
	ctx, _, service := arrangeRuleServiceTest(t)
	payload := schemas.CreateRuleSchema{
		Type:   domain.DomainRuleType,
		Action: domain.AllowAction,
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
	ctx, _, service := arrangeRuleServiceTest(t)

	testCases := map[string]schemas.CreateRuleSchema{
		"Domain rules": schemas.CreateRuleSchema{
			Type:   domain.DomainRuleType,
			Action: domain.AllowAction,
			Value:  "thisisnotadomain",
		},

		"IP rules": schemas.CreateRuleSchema{
			Type:   domain.IPRuleType,
			Action: domain.DenyAction,
			Value:  "thisisnotanip",
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
	ctx, _, service := arrangeRuleServiceTest(t)
	protocol, port := "TCP", 5432

	testCases := map[string]schemas.CreateRuleSchema{
		"Domain rule with protocol": schemas.CreateRuleSchema{
			Type:     domain.DomainRuleType,
			Action:   domain.AllowAction,
			Value:    "test.com",
			Protocol: &protocol,
		},
		"Domain rule with port": schemas.CreateRuleSchema{
			Type:   domain.DomainRuleType,
			Action: domain.DenyAction,
			Value:  "test.com",
			Port:   &port,
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
	ctx, _, service := arrangeRuleServiceTest(t)
	testPorts := []int{-99999, -1, 65536, 99999}
	basePayload := schemas.CreateRuleSchema{
		Type:   domain.IPRuleType,
		Action: domain.AllowAction,
		Value:  "10.0.0.1",
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
	ctx, store, service := arrangeRuleServiceTest(t)
	expectedErr := errors.New("failed to insert rule into database")
	store.StoreErr = expectedErr

	payload := schemas.CreateRuleSchema{
		Type:   domain.DomainRuleType,
		Action: domain.AllowAction,
		Value:  "test.com",
	}

	if _, err := service.CreateRule(ctx, payload); !errors.Is(err, expectedErr) {
		t.Errorf("expected store error %q, got %q", expectedErr, err)
	}
}

// ============================================================================
// GetRuleByID
// ============================================================================

func TestRuleService_GetRuleByID_WithSeededRule_ReturnsRule(t *testing.T) {
	ctx, store, service := arrangeRuleServiceTest(t)
	seededRule := seedRule(t, ctx, store, domain.NewDomainRule(domain.AllowAction, "test.com").Build())
	payload := schemas.GetRuleByIDSchema{ID: int(seededRule.ID)}

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

func TestRuleService_GetRuleByID_WithNonExistentRule_ReturnsNil(t *testing.T) {
	ctx, _, service := arrangeRuleServiceTest(t)

	rule, err := service.GetRuleByID(ctx, schemas.GetRuleByIDSchema{ID: 9999})

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if rule != nil {
		t.Errorf("expected rule to be nil, got %+v", rule)
	}
}

func TestRuleService_GetRuleByID_WhenStoreFails_RejectsFetch(t *testing.T) {
	ctx, store, service := arrangeRuleServiceTest(t)
	expectedErr := errors.New("failed to query database for rule")
	store.StoreErr = expectedErr

	payload := schemas.GetRuleByIDSchema{ID: 1}

	if _, err := service.GetRuleByID(ctx, payload); !errors.Is(err, expectedErr) {
		t.Errorf("expected store error %q, got %q", expectedErr, err)
	}
}

// ============================================================================
// ListRules
// ============================================================================

func TestRuleService_ListRules_WithSeededRules_ReturnsAllRules(t *testing.T) {
	ctx, store, service := arrangeRuleServiceTest(t)

	rulesToSeed := []domain.Rule{
		domain.NewDomainRule(domain.AllowAction, "test.com").Build(),
		domain.NewDomainRule(domain.DenyAction, "test.com").Build(),
		domain.NewIPRule(domain.AllowAction, "10.0.0.1").Build(),
	}

	for _, rule := range rulesToSeed {
		seedRule(t, ctx, store, rule)
	}

	rules, err := service.ListRules(ctx, schemas.ListRulesSchema{})

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	expectedCount, actualCount := len(rulesToSeed), len(rules)
	if len(rules) != 3 {
		t.Errorf("expected %d rules, got %d", expectedCount, actualCount)
	}
}

func TestRuleService_ListRules_WithSeededRulesAndFilter_ReturnsMatchingRules(t *testing.T) {
	ctx, store, service := arrangeRuleServiceTest(t)

	rulesToSeed := []domain.Rule{
		domain.NewDomainRule(domain.AllowAction, "test.com").Build(),
		domain.NewDomainRule(domain.DenyAction, "google.com").Build(),
		domain.NewIPRule(domain.AllowAction, "10.0.0.1").Build(),
		domain.NewIPRule(domain.DenyAction, "127.0.0.1").Build(),
	}

	for _, rule := range rulesToSeed {
		seedRule(t, ctx, store, rule)
	}

	filters := map[string]*domain.RuleFilterBuilder{
		"Only domain rules":         domain.NewRuleFilter().WithRuleType(new(domain.DomainRuleType)),
		"Only IP rules":             domain.NewRuleFilter().WithRuleType(new(domain.IPRuleType)),
		"Only allow rules":          domain.NewRuleFilter().WithAction(new(domain.AllowAction)),
		"Only deny rules":           domain.NewRuleFilter().WithAction(new(domain.DenyAction)),
		"Rules that end with .com":  domain.NewRuleFilter().WithValue(new(".com")),
		"Rules that end with 0.0.1": domain.NewRuleFilter().WithValue(new("0.0.1")),
		"Deny rules for domains":    domain.NewRuleFilter().WithRuleType(new(domain.DomainRuleType)).WithAction(new(domain.DenyAction)),
		"Allow rules for IPs":       domain.NewRuleFilter().WithRuleType(new(domain.IPRuleType)).WithAction(new(domain.AllowAction)),
	}

	for filterName, tt := range filters {
		filter := tt.Build()

		t.Run(filterName, func(t *testing.T) {
			results, err := service.ListRules(ctx, schemas.ListRulesSchema{
				Type:   filter.Type,
				Action: filter.Action,
				Value:  filter.Value,
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

func TestRuleService_ListRules_WithNoRulesPresent_ReturnsEmptyList(t *testing.T) {
	ctx, _, service := arrangeRuleServiceTest(t)

	result, err := service.ListRules(ctx, schemas.ListRulesSchema{})

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if len(result) != 0 {
		t.Errorf("expected empty list, got %d rules", len(result))
	}
}

func TestRuleService_ListRules_WhenStoreFails_RejectsFetch(t *testing.T) {
	ctx, store, service := arrangeRuleServiceTest(t)
	expectedErr := errors.New("failed to query database for rules")
	store.StoreErr = expectedErr

	if _, err := service.ListRules(ctx, schemas.ListRulesSchema{}); !errors.Is(err, expectedErr) {
		t.Errorf("expected store error %q, got %q", expectedErr, err)
	}
}

// ============================================================================
// UpdateRule
/// ============================================================================

func TestRuleService_UpdateDomainRule_WithValidPayload_AcceptsUpdate(t *testing.T) {
	ctx, store, service := arrangeRuleServiceTest(t)
	seededRule := seedRule(t, ctx, store, domain.NewDomainRule(domain.AllowAction, "test.com").Build())
	fields := domain.Rule{Action: domain.DenyAction, Value: "google.com"}

	updatedRule, err := service.UpdateRule(
		ctx,
		schemas.UpdateRuleSchema{ID: int(seededRule.ID), Action: &fields.Action, Value: &fields.Value},
	)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if updatedRule.Action != fields.Action {
		t.Errorf("expected rule action to be %q, got %q", fields.Action, updatedRule.Action)
	}

	if updatedRule.Value != fields.Value {
		t.Errorf("expected rule value to be %q, got %q", fields.Value, updatedRule.Value)
	}
}

func TestRuleService_UpdateIPRule_WithValidPayload_AcceptsUpdate(t *testing.T) {
	ctx, store, service := arrangeRuleServiceTest(t)
	seededRule := seedRule(t, ctx, store, domain.NewIPRule(domain.AllowAction, "127.0.0.1").Build())
	fields := domain.Rule{Action: domain.DenyAction, Value: "10.0.0.1"}
	protocol, port := "TCP", 5432

	updatedRule, err := service.UpdateRule(
		ctx,
		schemas.UpdateRuleSchema{
			ID:       int(seededRule.ID),
			Action:   &fields.Action,
			Value:    &fields.Value,
			Protocol: &protocol,
			Port:     &port,
		},
	)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if updatedRule.Action != fields.Action {
		t.Errorf("expected rule action to be %q, got %q", fields.Action, updatedRule.Action)
	}

	if updatedRule.Value != fields.Value {
		t.Errorf("expected rule value to be %q, got %q", fields.Value, updatedRule.Value)
	}

	if *updatedRule.Protocol != protocol {
		t.Errorf("expected rule protocol to be %q, got %q", *fields.Protocol, *updatedRule.Protocol)
	}

	if *updatedRule.Port != port {
		t.Errorf("expected rule port to be %d, got %d", *fields.Port, *updatedRule.Port)
	}
}

func TestRuleService_UpdateRule_WithEmptyAction_RejectsUpdate(t *testing.T) {
	ctx, store, service := arrangeRuleServiceTest(t)
	seededRule := seedRule(t, ctx, store, domain.NewDomainRule(domain.AllowAction, "test.com").Build())
	actionField := ""

	updatedRule, err := service.UpdateRule(
		ctx,
		schemas.UpdateRuleSchema{ID: int(seededRule.ID), Action: (*domain.RuleAction)(&actionField)},
	)

	if !errors.Is(err, ruleErrors.ErrRuleActionRequired) {
		t.Errorf("expected error %q, got %q", ruleErrors.ErrRuleActionRequired, err)
	}

	if updatedRule != nil {
		t.Errorf("expected result to be nil, got %v", updatedRule)
	}
}

func TestRuleService_UpdateRule_WithEmptyValue_RejectsUpdate(t *testing.T) {
	ctx, store, service := arrangeRuleServiceTest(t)
	seededRule := seedRule(t, ctx, store, domain.NewDomainRule(domain.AllowAction, "test.com").Build())
	valueField := ""

	updatedRule, err := service.UpdateRule(
		ctx,
		schemas.UpdateRuleSchema{ID: int(seededRule.ID), Value: &valueField},
	)

	if !errors.Is(err, ruleErrors.ErrRuleValueRequired) {
		t.Errorf("expected error %q, got %q", ruleErrors.ErrRuleValueRequired, err)
	}

	if updatedRule != nil {
		t.Errorf("expected result to be nil, got %v", updatedRule)
	}
}

func TestRuleService_UpdateRule_WithMalformedValue_RejectsUpdate(t *testing.T) {
	ctx, store, service := arrangeRuleServiceTest(t)
	seededRule := seedRule(t, ctx, store, domain.NewDomainRule(domain.AllowAction, "test.com").Build())
	valueField := ""

	updatedRule, err := service.UpdateRule(
		ctx,
		schemas.UpdateRuleSchema{ID: int(seededRule.ID), Value: &valueField},
	)

	if !errors.Is(err, ruleErrors.ErrRuleValueRequired) {
		t.Errorf("expected error %q, got %q", ruleErrors.ErrRuleValueRequired, err)
	}

	if updatedRule != nil {
		t.Errorf("expected result to be nil, got %v", updatedRule)
	}
}

func TestRuleService_UpdateRule_WithDomainRuleAndProtocolOrPort_RejectsUpdate(t *testing.T) {
	ctx, store, service := arrangeRuleServiceTest(t)

	seededRules := map[string]*domain.Rule{
		"domain": seedRule(t, ctx, store, domain.NewDomainRule(domain.AllowAction, "test.com").Build()),
		"ip":     seedRule(t, ctx, store, domain.NewIPRule(domain.DenyAction, "10.0.0.1").Build()),
	}

	testFields := map[string]string{
		"domain": "thisisnotadomain",
		"ip":     "thisisnotanip",
	}

	for ruleType, tt := range testFields {
		payload := schemas.UpdateRuleSchema{
			ID:    int(seededRules[ruleType].ID),
			Value: &tt,
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
	ctx, store, service := arrangeRuleServiceTest(t)
	seededRule := seedRule(t, ctx, store, domain.NewIPRule(domain.DenyAction, "10.0.0.1").Build())
	payload := schemas.UpdateRuleSchema{ID: int(seededRule.ID)}

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

func TestRuleService_UpdateRule_WhenStoreFails_RejectsFetch(t *testing.T) {
	ctx, store, service := arrangeRuleServiceTest(t)
	expectedErr := errors.New("failed to update rule in database")
	store.StoreErr = expectedErr

	action := domain.DenyAction
	payload := schemas.UpdateRuleSchema{ID: 1, Action: &action}

	if _, err := service.UpdateRule(ctx, payload); !errors.Is(err, expectedErr) {
		t.Errorf("expected store error %q, got %q", expectedErr, err)
	}
}

// ============================================================================
// RemoveRule
// ============================================================================

func TestRuleService_RemoveRule_WithExistentRule_RemovesRule(t *testing.T) {
	ctx, store, service := arrangeRuleServiceTest(t)
	seededRule := seedRule(t, ctx, store, domain.NewDomainRule(domain.AllowAction, "test.com").Build())

	err := service.RemoveRule(ctx, schemas.RemoveRuleSchema{ID: int(seededRule.ID)})

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if rule, _ := store.GetByID(ctx, seededRule.ID); rule != nil {
		t.Errorf("expected rule to be deleted, got %+v", rule)
	}
}

func TestRuleService_RemoveRule_WithNonExistentRule_RejectsRemoval(t *testing.T) {
	ctx, _, service := arrangeRuleServiceTest(t)

	err := service.RemoveRule(ctx, schemas.RemoveRuleSchema{ID: 9999})

	if !errors.Is(err, ruleErrors.ErrRuleNotFound) {
		t.Errorf("expected error %q, got %q", ruleErrors.ErrRuleNotFound, err)
	}
}

func TestRuleService_RemoveRule_WhenStoreFails_RejectsFetch(t *testing.T) {
	ctx, store, service := arrangeRuleServiceTest(t)
	expectedErr := errors.New("failed to query database for rule")
	store.StoreErr = expectedErr

	if err := service.RemoveRule(ctx, schemas.RemoveRuleSchema{ID: 1}); !errors.Is(err, expectedErr) {
		t.Errorf("expected store error %q, got %q", expectedErr, err)
	}
}

// ============================================================================
// Helpers
// ============================================================================

// arrangeTest initializes a new service instance with a mocked store provider.
func arrangeRuleServiceTest(t *testing.T) (context.Context, *MockRuleStore, service.RuleService) {
	t.Helper()

	mockStore := NewMockRuleStore()
	ctx := context.Background()
	service := service.NewRuleService(mockStore)

	return ctx, mockStore, *service
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

// mapOutputToRule maps the provided RuleOutputSchema to a domain.Rule
// instance.
func mapOutputToRule(output schemas.RuleOutputSchema) domain.Rule {
	return domain.Rule{
		Type:      output.Type,
		Action:    output.Action,
		Value:     output.Value,
		Protocol:  output.Protocol,
		Port:      output.Port,
		CreatedAt: output.CreatedAt,
	}
}
