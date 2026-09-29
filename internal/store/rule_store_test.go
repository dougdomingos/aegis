package store_test

import (
	"context"
	"strings"
	"testing"

	"dougdomingos.com/aegis/internal/domain"
	"dougdomingos.com/aegis/internal/store"
	_ "modernc.org/sqlite"
)

// ============================================================================
// Create
// ============================================================================

func TestRuleStore_Create_WithValidData_CreatesRule(t *testing.T) {
	ctx, stores := arrangeRuleStoreTest(t)

	rules := map[string]domain.Rule{
		"Only required values": domain.NewDomainRule("test.com").WithPolicy(stores.policyID).Build(),
		"All values": domain.NewIPRule("192.168.0.0/24").
			WithPolicy(stores.policyID).
			WithProtocol("TCP").
			WithPort(5432).
			Build(),
	}

	for testName, tt := range rules {
		t.Run(testName, func(t *testing.T) {
			rule, err := stores.rules.Create(ctx, tt)

			if err != nil {
				t.Fatalf("expected no error, got: %v", err)
			}

			if rule.ID == 0 {
				t.Error("expected non-zero ID for created rule")
			}
			if rule.PolicyID != stores.policyID {
				t.Errorf("expected policy ID %d, got %d", stores.policyID, rule.PolicyID)
			}
			if rule.CreatedAt.IsZero() {
				t.Error("expected created_at timestamp to be populated")
			}

			assertRuleEqual(t, &tt, rule)
		})
	}
}

func TestRuleStore_Create_WithoutPolicy_RejectsCreation(t *testing.T) {
	ctx, stores := arrangeRuleStoreTest(t)

	_, err := stores.rules.Create(ctx, domain.Rule{Type: domain.DomainRuleType, Value: "test.com"})

	if err == nil {
		t.Error("expected an error for missing policy, got nil")
	}
}

func TestRuleStore_Create_WithInexistentPolicy_RejectsCreation(t *testing.T) {
	ctx, stores := arrangeRuleStoreTest(t)

	_, err := stores.rules.Create(ctx, domain.Rule{
		PolicyID: 9999,
		Type:     domain.DomainRuleType,
		Value:    "test.com",
	})

	if err == nil {
		t.Error("expected an error for foreign key violation, got nil")
	}
}

func TestRuleStore_Create_WithoutType_RejectsCreation(t *testing.T) {
	ctx, stores := arrangeRuleStoreTest(t)

	_, err := stores.rules.Create(ctx, domain.Rule{PolicyID: stores.policyID, Value: "test.com"})

	if err == nil {
		t.Error("expected an error for non-null constraint violation, got nil")
	}
}

func TestRuleStore_Create_WithoutValue_RejectsCreation(t *testing.T) {
	ctx, stores := arrangeRuleStoreTest(t)

	_, err := stores.rules.Create(ctx, domain.Rule{PolicyID: stores.policyID, Type: domain.DomainRuleType})

	if err == nil {
		t.Error("expected an error for non-null constraint violation, got nil")
	}
}

// ============================================================================
// GetByID
// ============================================================================

func TestRuleStore_GetByID_WithSeededIPRule_ReturnsRule(t *testing.T) {
	ctx, stores := arrangeRuleStoreTest(t)

	seededRule := seedRule(
		t,
		ctx,
		stores.rules,
		domain.NewIPRule("0.0.0.0").
			WithPolicy(stores.policyID).
			WithProtocol("TCP").
			WithPort(5432).
			Build(),
	)

	rule, err := stores.rules.GetByID(ctx, seededRule.ID)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if rule == nil {
		t.Fatal("expected rule to be returned, got nil")
	}

	assertRuleEqual(t, &seededRule, rule)
}

func TestRuleStore_GetByID_WithSeededDomainRule_ReturnsRule(t *testing.T) {
	ctx, stores := arrangeRuleStoreTest(t)
	seededRule := seedRule(t, ctx, stores.rules,
		domain.NewDomainRule("test.com").WithPolicy(stores.policyID).Build())

	rule, err := stores.rules.GetByID(ctx, seededRule.ID)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if rule == nil {
		t.Fatal("expected rule to be returned, got nil")
	}

	assertRuleEqual(t, &seededRule, rule)
}

func TestRuleStore_GetByID_WithInexistentID_ReturnsNil(t *testing.T) {
	ctx, stores := arrangeRuleStoreTest(t)
	rule, err := stores.rules.GetByID(ctx, 9999)

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
	ctx, stores := arrangeRuleStoreTest(t)

	rulesToSeed := []domain.Rule{
		domain.NewDomainRule("test.com").WithPolicy(stores.policyID).Build(),
		domain.NewIPRule("10.0.0.1/24").WithPolicy(stores.policyID).Build(),
	}

	for _, rule := range rulesToSeed {
		seedRule(t, ctx, stores.rules, rule)
	}

	filter := domain.NewRuleFilter().WithPolicyID(stores.policyID).Build()
	rules, err := stores.rules.List(ctx, filter)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	rule_count, expected_count := len(rules), len(rulesToSeed)
	if rule_count != expected_count {
		t.Errorf("expected %d rules, got %d", expected_count, rule_count)
	}
}

func TestRuleStore_List_WithPolicyFilter_ReturnsOnlyPolicyRules(t *testing.T) {
	ctx, stores := arrangeRuleStoreTest(t)

	otherPolicy, err := stores.policies.Create(ctx, "Other Policy", domain.BlacklistPolicyType)
	if err != nil {
		t.Fatalf("failed to seed policy: %v", err)
	}

	seedRule(t, ctx, stores.rules, domain.NewDomainRule("mine.com").WithPolicy(stores.policyID).Build())
	seedRule(t, ctx, stores.rules, domain.NewDomainRule("theirs.com").WithPolicy(otherPolicy.ID).Build())

	filter := domain.NewRuleFilter().WithPolicyID(stores.policyID).Build()
	rules, err := stores.rules.List(ctx, filter)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(rules))
	}

	for _, rule := range rules {
		if !filter.Matches(rule) {
			t.Errorf("expected rule owned by policy %d, got %d", stores.policyID, rule.PolicyID)
		}
	}
}

func TestRuleStore_List_WithTypeFilter_ReturnsMatchingRules(t *testing.T) {
	ctx, stores := arrangeRuleStoreTest(t)

	rulesToSeed := []domain.Rule{
		domain.NewDomainRule("test.com").WithPolicy(stores.policyID).Build(),
		domain.NewIPRule("10.0.0.1/24").WithPolicy(stores.policyID).Build(),
	}

	for _, rule := range rulesToSeed {
		seedRule(t, ctx, stores.rules, rule)
	}

	testCases := map[string]domain.RuleType{
		"Domain type": domain.DomainRuleType,
		"IP type":     domain.IPRuleType,
	}

	for name, tt := range testCases {
		t.Run(name, func(t *testing.T) {
			ruleType := tt
			filter := domain.NewRuleFilter().
				WithPolicyID(stores.policyID).
				WithRuleType(&ruleType).
				Build()

			rules, err := stores.rules.List(ctx, filter)

			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}

			if len(rules) == 0 {
				t.Fatalf("expected non-empty list of rules, got %+v", rules)
			}

			for _, rule := range rules {
				if !filter.Matches(rule) {
					t.Fatalf("expected result to only have rules of type %q, found %q", *filter.Type, rule.Type)
				}
			}
		})
	}
}

func TestRuleStore_List_WithValueFilter_ReturnsMatchingRules(t *testing.T) {
	ctx, stores := arrangeRuleStoreTest(t)

	rulesToSeed := []domain.Rule{
		domain.NewDomainRule("test.com").WithPolicy(stores.policyID).Build(),
		domain.NewDomainRule("google.com").WithPolicy(stores.policyID).Build(),
		domain.NewIPRule("10.0.0.1/24").WithPolicy(stores.policyID).Build(),
		domain.NewIPRule("127.0.0.1").WithPolicy(stores.policyID).Build(),
	}

	for _, rule := range rulesToSeed {
		seedRule(t, ctx, stores.rules, rule)
	}

	testCases := map[string]string{
		"Domain rules": ".com",
		"IP rules":     "0.0.1",
	}

	for name, tt := range testCases {
		t.Run(name, func(t *testing.T) {
			value := tt
			filter := domain.NewRuleFilter().
				WithPolicyID(stores.policyID).
				WithValue(&value).
				Build()

			rules, err := stores.rules.List(ctx, filter)

			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}

			if len(rules) == 0 {
				t.Fatalf("expected non-empty list of rules, got %+v", rules)
			}

			for _, rule := range rules {
				if !filter.Matches(rule) {
					t.Fatalf("expected result to only have rules containing %q, found %q", *filter.Value, rule.Value)
				}
			}
		})
	}
}

func TestRuleStore_List_WithNoMatchingFilter_ReturnsEmpty(t *testing.T) {
	ctx, stores := arrangeRuleStoreTest(t)

	ruleType := domain.IPRuleType
	filter := domain.NewRuleFilter().
		WithPolicyID(stores.policyID).
		WithRuleType(&ruleType).
		WithValue(new(".com")).
		Build()

	rulesToSeed := []domain.Rule{
		domain.NewDomainRule("test.com").WithPolicy(stores.policyID).Build(),
		domain.NewIPRule("10.0.0.1/24").WithPolicy(stores.policyID).Build(),
	}

	for _, rule := range rulesToSeed {
		seedRule(t, ctx, stores.rules, rule)
	}

	rules, err := stores.rules.List(ctx, filter)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(rules) != 0 {
		t.Errorf("expected empty list of rules, got %+v", rules)
	}
}

func TestRuleStore_List_WithEmptyStore_ReturnsEmpty(t *testing.T) {
	ctx, stores := arrangeRuleStoreTest(t)

	filter := domain.NewRuleFilter().WithPolicyID(stores.policyID).Build()
	rules, err := stores.rules.List(ctx, filter)

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
	ctx, stores := arrangeRuleStoreTest(t)
	seededRule := seedRule(t, ctx, stores.rules,
		domain.NewIPRule("0.0.0.0").WithPolicy(stores.policyID).Build())
	updatedRule := domain.NewIPRule("192.168.0.1").
		WithPolicy(stores.policyID).
		WithProtocol("SSH").
		WithPort(22).
		Build()

	updatedRule.ID = seededRule.ID
	updateResult, err := stores.rules.Update(ctx, updatedRule)
	if err != nil {
		t.Fatalf("expected no errors, got %v", err)
	}

	assertRuleEqual(t, &updatedRule, updateResult)

	storedRule, _ := stores.rules.GetByID(ctx, seededRule.ID)

	assertRuleEqual(t, &updatedRule, storedRule)
}

func TestRuleStore_Update_WithEmptyType_ReturnsError(t *testing.T) {
	ctx, stores := arrangeRuleStoreTest(t)
	seededRule := seedRule(t, ctx, stores.rules,
		domain.NewDomainRule("test.com").WithPolicy(stores.policyID).Build())

	seededRule.Type = ""

	updateResult, err := stores.rules.Update(ctx, seededRule)
	if err == nil {
		t.Error("expected error, got nil")
	}
	if updateResult != nil {
		t.Errorf("expected update result to be nil, got %v", updateResult)
	}
}

func TestRuleStore_Update_WithoutPolicy_ReturnsError(t *testing.T) {
	ctx, stores := arrangeRuleStoreTest(t)
	seededRule := seedRule(t, ctx, stores.rules,
		domain.NewDomainRule("test.com").WithPolicy(stores.policyID).Build())

	seededRule.PolicyID = 0

	updateResult, err := stores.rules.Update(ctx, seededRule)
	if err == nil {
		t.Error("expected error, got nil")
	}
	if updateResult != nil {
		t.Errorf("expected update result to be nil, got %v", updateResult)
	}
}

func TestRuleStore_Update_WithEmptyValue_ReturnsError(t *testing.T) {
	ctx, stores := arrangeRuleStoreTest(t)
	seededRule := seedRule(t, ctx, stores.rules,
		domain.NewDomainRule("test.com").WithPolicy(stores.policyID).Build())

	seededRule.Value = ""

	updateResult, err := stores.rules.Update(ctx, seededRule)
	if err == nil {
		t.Error("expected error, got nil")
	}
	if updateResult != nil {
		t.Errorf("expected update result to be nil, got %v", updateResult)
	}
}

func TestRuleStore_Update_WithInexistentID_ReturnsError(t *testing.T) {
	ctx, stores := arrangeRuleStoreTest(t)
	seededRule := seedRule(t, ctx, stores.rules,
		domain.NewDomainRule("test.com").WithPolicy(stores.policyID).Build())

	seededRule.ID = 9999

	updateResult, err := stores.rules.Update(ctx, seededRule)
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
	ctx, stores := arrangeRuleStoreTest(t)

	seededRule := seedRule(t, ctx, stores.rules,
		domain.NewDomainRule("test.com").WithPolicy(stores.policyID).Build())

	err := stores.rules.Remove(ctx, seededRule.ID)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	rule, _ := stores.rules.GetByID(ctx, seededRule.ID)
	if rule != nil {
		t.Error("expected rule to be removed from DB")
	}
}

func TestRuleStore_Remove_WithInexistentID_RejectsRemoval(t *testing.T) {
	ctx, stores := arrangeRuleStoreTest(t)

	err := stores.rules.Remove(ctx, 9999)

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

// ruleStores bundles the stores and the seeded policy required by rule test
// scenarios.
type ruleStores struct {
	rules    *store.RuleStore
	policies *store.PolicyStore
	policyID int64
}

// arrangeRuleStoreTest provisions the schema, builds the rule and policy
// stores over the same database, and seeds a default policy to own rules.
func arrangeRuleStoreTest(t *testing.T) (context.Context, *ruleStores) {
	t.Helper()

	ctx, db := arrangeStoreDB(t)
	stores := &ruleStores{
		rules:    store.NewRuleStore(db),
		policies: store.NewPolicyStore(db),
	}

	seededPolicy, err := stores.policies.Create(ctx, "Default Policy", domain.WhitelistPolicyType)
	if err != nil {
		t.Fatalf("failed to seed policy: %v", err)
	}

	stores.policyID = seededPolicy.ID
	return ctx, stores
}

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

	if expected.PolicyID != actual.PolicyID {
		t.Errorf("policy ID does not match: expected %d, got %d", expected.PolicyID, actual.PolicyID)
	}

	if !expected.IsEqual(actual) {
		t.Errorf("rules do not match: expected %+v, got %+v", expected, actual)
	}
}
