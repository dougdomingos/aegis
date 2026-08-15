package domain

import (
	"fmt"
	"strings"
)

// RuleFilter defines criteria for searching and listing rules.
//
// All filters provided within an instance of RuleFilter are to be applied
// simultaneously.
type RuleFilter struct {

	// Value is an optional filter matching exact or partial rule target values.
	Value *string

	// Type is an optional filter matching specific rule types (e.g., IP, DOMAIN).
	Type *RuleType

	// Action is an optional filter matching specific rule actions
	// (e.g., ALLOW, DENY).
	Action *RuleAction
}

// Matches takes a rule and verifies whether it matches the definitions of a
// filter instance.
func (filter *RuleFilter) Matches(rule Rule) bool {
	matches := true

	fmt.Printf("Comparing Filter %+v with rule %v...\n", filter, rule)

	if filter.Type != nil && rule.Type != *filter.Type {
		matches = false
	} else if filter.Action != nil && rule.Action != *filter.Action {
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

// WithValue sets the target value filter condition.
func (builder *RuleFilterBuilder) WithValue(value string) *RuleFilterBuilder {
	builder.filter.Value = &value
	return builder
}

// WithRuleType sets the rule type filter condition.
func (builder *RuleFilterBuilder) WithRuleType(ruleType RuleType) *RuleFilterBuilder {
	builder.filter.Type = &ruleType
	return builder
}

// WithAction sets the rule action filter condition.
func (builder *RuleFilterBuilder) WithAction(action RuleAction) *RuleFilterBuilder {
	builder.filter.Action = &action
	return builder
}

// Build constructs and returns the finalized RuleFilter instance.
func (builder *RuleFilterBuilder) Build() RuleFilter {
	return RuleFilter{
		Type:   builder.filter.Type,
		Action: builder.filter.Action,
		Value:  builder.filter.Value,
	}
}
