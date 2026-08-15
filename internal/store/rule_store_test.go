package store_test

import (
	"context"
	"strings"
	"testing"

	"dougdomingos.com/aegis/internal/domain"
	"dougdomingos.com/aegis/internal/store"
	_ "modernc.org/sqlite"
)

const queryInitRuleTable = `
	CREATE TABLE IF NOT EXISTS rules (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		type TEXT NOT NULL,
		value TEXT NOT NULL,
		action TEXT NOT NULL,
		protocol TEXT,
		port INTEGER,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
`

// ============================================================================
// Create
// ============================================================================

func TestRuleStore_Create_WithValidData_CreatesRule(t *testing.T) {
	ctx, store := arrangeStoreTest(t, store.NewRuleStore, queryInitRuleTable)

	rules := map[string]domain.Rule{
		"Only required values": domain.NewDomainRule(domain.AllowAction, "test.com").Build(),
		"All values": domain.NewIPRule(domain.DenyAction, "192.168.0.0/24").
			WithProtocol("TCP").
			WithPort(5432).
			Build(),
	}

	for testName, tt := range rules {
		t.Run(testName, func(t *testing.T) {
			t.Logf("Creating rule: %+v", tt)
			rule, err := store.Create(ctx, tt)

			if err != nil {
				t.Fatalf("expected no error, got: %v", err)
			}

			if rule.ID == 0 {
				t.Error("expected non-zero ID for created rule")
			}
			if rule.CreatedAt.IsZero() {
				t.Error("expected created_at timestamp to be populated")
			}

			assertRuleEqual(t, &tt, rule)
		})
	}
}

func TestRuleStore_Create_WithoutType_RejectsCreation(t *testing.T) {
	ctx, store := arrangeStoreTest(t, store.NewRuleStore, queryInitRuleTable)

	_, err := store.Create(ctx, domain.Rule{Action: domain.AllowAction, Value: "test.com"})

	if err == nil {
		t.Error("expected an error for non-null constraint violation, got nil")
	}
}

func TestRuleStore_Create_WithoutAction_RejectsCreation(t *testing.T) {
	ctx, store := arrangeStoreTest(t, store.NewRuleStore, queryInitRuleTable)

	_, err := store.Create(ctx, domain.Rule{Type: domain.DomainRuleType, Value: "test.com"})

	if err == nil {
		t.Error("expected an error for non-null constraint violation, got nil")
	}
}

func TestRuleStore_Create_WithoutValue_RejectsCreation(t *testing.T) {
	ctx, store := arrangeStoreTest(t, store.NewRuleStore, queryInitRuleTable)

	_, err := store.Create(ctx, domain.Rule{Type: domain.DomainRuleType, Action: domain.AllowAction})

	if err == nil {
		t.Error("expected an error for non-null constraint violation, got nil")
	}
}

// ============================================================================
// GetByID
// ============================================================================

func TestRuleStore_GetByID_WithSeededIPRule_ReturnsRule(t *testing.T) {
	ctx, store := arrangeStoreTest(t, store.NewRuleStore, queryInitRuleTable)

	seededRule := seedRule(
		t,
		ctx,
		store,
		domain.NewIPRule(domain.AllowAction, "0.0.0.0").
			WithProtocol("TCP").
			WithPort(5432).
			Build(),
	)

	rule, err := store.GetByID(ctx, seededRule.ID)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if rule == nil {
		t.Fatal("expected rule to be returned, got nil")
	}

	assertRuleEqual(t, &seededRule, rule)
}

func TestRuleStore_GetByID_WithSeededDomainRule_ReturnsRule(t *testing.T) {
	ctx, store := arrangeStoreTest(t, store.NewRuleStore, queryInitRuleTable)
	seededRule := seedRule(t, ctx, store, domain.NewDomainRule(domain.AllowAction, "test.com").Build())

	rule, err := store.GetByID(ctx, seededRule.ID)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if rule == nil {
		t.Fatal("expected rule to be returned, got nil")
	}

	assertRuleEqual(t, &seededRule, rule)
}

func TestRuleStore_GetByID_WithInexistentID_ReturnsNil(t *testing.T) {
	ctx, store := arrangeStoreTest(t, store.NewRuleStore, queryInitRuleTable)
	rule, err := store.GetByID(ctx, 9999)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if rule != nil {
		t.Errorf("expected nil rule, got %+v", rule)
	}
}

// ============================================================================
// List
// ============================================================================

func TestRuleStore_List_WithSeededRules_ReturnsAll(t *testing.T) {
	ctx, store := arrangeStoreTest(t, store.NewRuleStore, queryInitRuleTable)

	rulesToSeed := []domain.Rule{
		domain.NewDomainRule(domain.AllowAction, "test.com").Build(),
		domain.NewIPRule(domain.DenyAction, "10.0.0.1/24").Build(),
	}

	for _, rule := range rulesToSeed {
		seedRule(t, ctx, store, rule)
	}

	rules, err := store.List(ctx, domain.RuleFilter{})

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	rule_count, expected_count := len(rules), len(rulesToSeed)
	if rule_count != expected_count {
		t.Errorf("expected %d groups, got %d", expected_count, rule_count)
	}
}

func TestRuleStore_List_WithTypeFilter_ReturnsMatchingRules(t *testing.T) {
	ctx, store := arrangeStoreTest(t, store.NewRuleStore, queryInitRuleTable)

	rulesToSeed := []domain.Rule{
		domain.NewDomainRule(domain.AllowAction, "test.com").Build(),
		domain.NewIPRule(domain.DenyAction, "10.0.0.1/24").Build(),
	}

	for _, rule := range rulesToSeed {
		seedRule(t, ctx, store, rule)
	}

	testCases := map[string]domain.RuleFilter{
		"Domain type": domain.NewRuleFilter().WithRuleType(domain.DomainRuleType).Build(),
		"IP type":     domain.NewRuleFilter().WithRuleType(domain.IPRuleType).Build(),
	}

	for name, tt := range testCases {
		t.Run(name, func(t *testing.T) {
			filter := domain.NewRuleFilter().
				WithRuleType(*tt.Type).
				Build()

			rules, err := store.List(ctx, filter)

			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}

			if len(rules) == 0 {
				t.Fatalf("expected non-empty list of rules, got %+v", rules)
			}

			for _, rule := range rules {
				if rule.Type != *filter.Type {
					t.Fatalf("expected result to only have rules of type %q, found %q", *filter.Type, rule.Type)
				}
			}
		})
	}
}

func TestRuleStore_List_WithActionFilter_ReturnsMatchingRules(t *testing.T) {
	ctx, store := arrangeStoreTest(t, store.NewRuleStore, queryInitRuleTable)

	rulesToSeed := []domain.Rule{
		domain.NewDomainRule(domain.AllowAction, "test.com").Build(),
		domain.NewIPRule(domain.DenyAction, "10.0.0.1/24").Build(),
	}

	for _, rule := range rulesToSeed {
		seedRule(t, ctx, store, rule)
	}

	testCases := map[string]domain.RuleFilter{
		"Allow action": domain.NewRuleFilter().WithAction(domain.AllowAction).Build(),
		"Deny action":  domain.NewRuleFilter().WithAction(domain.DenyAction).Build(),
	}

	for name, tt := range testCases {
		t.Run(name, func(t *testing.T) {
			filter := domain.NewRuleFilter().
				WithAction(*tt.Action).
				Build()

			rules, err := store.List(ctx, filter)

			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}

			if len(rules) == 0 {
				t.Fatalf("expected non-empty list of rules, got %+v", rules)
			}

			for _, rule := range rules {
				if rule.Action != *filter.Action {
					t.Fatalf("expected result to only have rules with %q action, found %q", *filter.Action, rule.Action)
				}
			}
		})
	}
}

func TestRuleStore_List_WithValueFilter_ReturnsMatchingRules(t *testing.T) {
	ctx, store := arrangeStoreTest(t, store.NewRuleStore, queryInitRuleTable)

	rulesToSeed := []domain.Rule{
		domain.NewDomainRule(domain.AllowAction, "test.com").Build(),
		domain.NewDomainRule(domain.DenyAction, "google.com").Build(),
		domain.NewIPRule(domain.DenyAction, "10.0.0.1/24").Build(),
		domain.NewIPRule(domain.AllowAction, "127.0.0.1").Build(),
	}

	for _, rule := range rulesToSeed {
		seedRule(t, ctx, store, rule)
	}

	testCases := map[string]domain.RuleFilter{
		"Domain rules": domain.NewRuleFilter().WithValue(".com").Build(),
		"IP rules":     domain.NewRuleFilter().WithValue("0.0.1").Build(),
	}

	for name, tt := range testCases {
		t.Run(name, func(t *testing.T) {
			filter := domain.NewRuleFilter().
				WithValue(*tt.Value).
				Build()

			rules, err := store.List(ctx, filter)

			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}

			if len(rules) == 0 {
				t.Fatalf("expected non-empty list of rules, got %+v", rules)
			}

			for _, rule := range rules {
				if !strings.Contains(rule.Value, *filter.Value) {
					t.Fatalf("expected result to only have rules containing %q, found %q", *filter.Value, rule.Value)
				}
			}
		})
	}
}

func TestRuleStore_List_WithNoMatchingFilter_ReturnsEmpty(t *testing.T) {
	ctx, store := arrangeStoreTest(t, store.NewRuleStore, queryInitRuleTable)

	filter := domain.NewRuleFilter().
		WithRuleType(domain.IPRuleType).
		WithValue(".com").
		Build()

	rulesToSeed := []domain.Rule{
		domain.NewDomainRule(domain.AllowAction, "test.com").Build(),
		domain.NewIPRule(domain.DenyAction, "10.0.0.1/24").Build(),
	}

	for _, rule := range rulesToSeed {
		seedRule(t, ctx, store, rule)
	}

	rules, err := store.List(ctx, filter)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(rules) != 0 {
		t.Errorf("expected empty list of rules, got %+v", rules)
	}
}

func TestRuleStore_List_WithEmptyStore_ReturnsEmpty(t *testing.T) {
	ctx, store := arrangeStoreTest(t, store.NewRuleStore, queryInitRuleTable)

	rules, err := store.List(ctx, domain.RuleFilter{})

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(rules) != 0 {
		t.Fatalf("expected empty list of rules, got %+v", rules)
	}
}

// ============================================================================
// Update
// ============================================================================

func TestRuleStore_Update_WithValidData_UpdatesRule(t *testing.T) {
	ctx, store := arrangeStoreTest(t, store.NewRuleStore, queryInitRuleTable)
	seededRule := seedRule(t, ctx, store, domain.NewIPRule(domain.AllowAction, "0.0.0.0").Build())
	updatedRule := domain.NewIPRule(domain.DenyAction, "192.168.0.1").
		WithProtocol("SSH").
		WithPort(22).
		Build()

	updatedRule.ID = seededRule.ID
	updateResult, err := store.Update(ctx, updatedRule)
	if err != nil {
		t.Fatalf("expected no errors, got %v", err)
	}

	assertRuleEqual(t, &updatedRule, updateResult)

	storedRule, _ := store.GetByID(ctx, seededRule.ID)

	assertRuleEqual(t, &updatedRule, storedRule)
}

func TestRuleStore_Update_WithEmptyType_ReturnsError(t *testing.T) {
	ctx, store := arrangeStoreTest(t, store.NewRuleStore, queryInitRuleTable)
	seededRule := seedRule(t, ctx, store, domain.NewDomainRule(domain.AllowAction, "test.com").Build())

	seededRule.Type = ""

	updateResult, err := store.Update(ctx, seededRule)
	if err == nil {
		t.Error("expected error, got nil")
	}
	if updateResult != nil {
		t.Errorf("expected update result to be nil, got %v", updateResult)
	}
}

func TestRuleStore_Update_WithEmptyAction_ReturnsError(t *testing.T) {
	ctx, store := arrangeStoreTest(t, store.NewRuleStore, queryInitRuleTable)
	seededRule := seedRule(t, ctx, store, domain.NewDomainRule(domain.AllowAction, "test.com").Build())

	seededRule.Action = ""

	updateResult, err := store.Update(ctx, seededRule)
	if err == nil {
		t.Error("expected error, got nil")
	}
	if updateResult != nil {
		t.Errorf("expected update result to be nil, got %v", updateResult)
	}
}

func TestRuleStore_Update_WithEmptyValue_ReturnsError(t *testing.T) {
	ctx, store := arrangeStoreTest(t, store.NewRuleStore, queryInitRuleTable)
	seededRule := seedRule(t, ctx, store, domain.NewDomainRule(domain.AllowAction, "test.com").Build())

	seededRule.Value = ""

	updateResult, err := store.Update(ctx, seededRule)
	if err == nil {
		t.Error("expected error, got nil")
	}
	if updateResult != nil {
		t.Errorf("expected update result to be nil, got %v", updateResult)
	}
}

func TestRuleStore_Update_WithInexistentID_ReturnsError(t *testing.T) {
	ctx, store := arrangeStoreTest(t, store.NewRuleStore, queryInitRuleTable)
	seededRule := seedRule(t, ctx, store, domain.NewDomainRule(domain.AllowAction, "test.com").Build())

	seededRule.ID = 9999

	updateResult, err := store.Update(ctx, seededRule)
	if err == nil {
		t.Error("expected error, got nil")
	}
	if updateResult != nil {
		t.Errorf("expected update result to be nil, got %v", updateResult)
	}
}

// ============================================================================
// Remove
// ============================================================================

func TestRuleStore_Remove_WithExistingID_DeletesRule(t *testing.T) {
	ctx, store := arrangeStoreTest(t, store.NewRuleStore, queryInitRuleTable)

	seededRule := seedRule(t, ctx, store, domain.NewDomainRule(domain.AllowAction, "test.com").Build())

	err := store.Remove(ctx, seededRule.ID)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	rule, _ := store.GetByID(ctx, seededRule.ID)
	if rule != nil {
		t.Error("expected rule to be removed from DB")
	}
}

func TestRuleStore_Remove_WithInexistentID_RejectsRemoval(t *testing.T) {
	ctx, store := arrangeStoreTest(t, store.NewRuleStore, queryInitRuleTable)

	err := store.Remove(ctx, 9999)

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' error, got: %v", err)
	}
}

// ============================================================================
// Helpers
// ============================================================================

// seedRule inserts the provided rule instance into the database. Used on test
// cases where a rule is expected to exist before the store operation is made.
func seedRule(t *testing.T, ctx context.Context, store *store.RuleStore, rule domain.Rule) domain.Rule {
	t.Helper()
	var seededRule *domain.Rule

	seededRule, err := store.Create(ctx, rule)

	if err != nil {
		t.Fatalf("failed to seed rule: %v", err)
	}

	return *seededRule
}

// assertRuleEqual verifies if two rule instances have the same values for
// every field, yielding an error for every mismatch.
func assertRuleEqual(t *testing.T, expected, actual *domain.Rule) {
	t.Helper()

	if expected.Type != actual.Type {
		t.Errorf("type mismatch: expected %q, got %q", expected.Type, actual.Type)
	}
	if expected.Value != actual.Value {
		t.Errorf("value mismatch: expected %q, got %q", expected.Value, actual.Value)
	}
	if expected.Action != actual.Action {
		t.Errorf("action mismatch: expected %q, got %q", expected.Action, actual.Action)
	}
	if (expected.Protocol == nil) != (actual.Protocol == nil) {
		t.Errorf("protocol nil mismatch: expected %v, got %v", expected.Protocol == nil, actual.Protocol == nil)
	} else if expected.Protocol != nil && *expected.Protocol != *actual.Protocol {
		t.Errorf("expected protocol %q, got %q", *expected.Protocol, *actual.Protocol)
	}

	if (expected.Port == nil) != (actual.Port == nil) {
		t.Errorf("port nil mismatch: expected %v, got %v", expected.Port == nil, actual.Port == nil)
	} else if expected.Port != nil && *expected.Port != *actual.Port {
		t.Errorf("expected port %d, got %d", *expected.Port, *actual.Port)
	}
}
