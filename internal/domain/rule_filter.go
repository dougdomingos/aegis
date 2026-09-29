package domain

import (
	"strings"
)

// RuleFilter defines criteria for searching and listing rules.
//
// All filters provided within an instance of RuleFilter are to be applied
// simultaneously.
type RuleFilter struct {

	// PolicyID is the identifier of the policy whose rules are being listed.
	PolicyID int64

	// Value is an optional filter matching exact or partial rule target values.
	Value *string

	// Type is an optional filter matching specific rule types (e.g., IP, DOMAIN).
	Type *RuleType
}

// Matches takes a rule and verifies whether it matches the definitions of a
// filter instance.
func (filter *RuleFilter) Matches(rule Rule) bool {
	matches := true

	if rule.PolicyID != filter.PolicyID {
		matches = false
	} else if filter.Type != nil && rule.Type != *filter.Type {
		matches = false
	} else if filter.Value != nil && !strings.Contains(rule.Value, *filter.Value) {
		matches = false
	}

	return matches
}

// RuleFilterBuilder provides a builder pattern for constructing a RuleFilter.
type RuleFilterBuilder struct {
	filter RuleFilter
}

// NewRuleFilter initializes a new empty RuleFilterBuilder instance.
func NewRuleFilter() *RuleFilterBuilder {
	return &RuleFilterBuilder{
		filter: RuleFilter{},
	}
}

// WithPolicyID sets the policy scoping condition.
func (builder *RuleFilterBuilder) WithPolicyID(policyID int64) *RuleFilterBuilder {
	builder.filter.PolicyID = policyID
	return builder
}

// WithValue sets the target value filter condition.
func (builder *RuleFilterBuilder) WithValue(value *string) *RuleFilterBuilder {
	builder.filter.Value = value
	return builder
}

// WithRuleType sets the rule type filter condition.
func (builder *RuleFilterBuilder) WithRuleType(ruleType *RuleType) *RuleFilterBuilder {
	builder.filter.Type = ruleType
	return builder
}

// Build constructs and returns the finalized RuleFilter instance.
func (builder *RuleFilterBuilder) Build() RuleFilter {
	return RuleFilter{
		PolicyID: builder.filter.PolicyID,
		Type:     builder.filter.Type,
		Value:    builder.filter.Value,
	}
}
