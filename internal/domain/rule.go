package domain

import (
	"context"
	"time"
)

// RuleType represents the rule classification type.
type RuleType string

// RuleAction represents the action to enforce when a rule matches.
type RuleAction string

const (
	// DomainRuleType identifies domain-based filtering rules.
	DomainRuleType RuleType = "DOMAIN"

	// IPRuleType identifies IP address or CIDR-based filtering rules.
	IPRuleType RuleType = "IP"

	// AllowAction permits matching network traffic.
	AllowAction RuleAction = "ALLOW"

	// DenyAction blocks matching network traffic.
	DenyAction RuleAction = "DENY"
)

// Rule represents a network rule to be enforced by the system.
type Rule struct {

	// ID is the unique identifier of the rule, assigned at creation.
	ID int64

	// Type is the immutable rule classification (e.g., IP, DOMAIN).
	Type RuleType

	// Value is the targeted resource (e.g., IP address, CIDR, domain name). Its
	// format must match the specified RuleType of its instance.
	Value string

	// Action is the enforcement result applied when matched (e.g., ALLOW, DENY).
	Action RuleAction

	// (Optional) Protocol is the targeted network protocol (e.g., TCP, UDP). Only
	// applicable for IP-based rules; defaults to nil if not provided.
	Protocol *string

	// (Optional) Port is the targeted network port or range. Only applicable for
	// IP-based rules; defaults to nil if not provided.
	Port *int

	// CreatedAt is the timestamp when the rule was first created.
	CreatedAt time.Time
}

// IsEqual checks whether the provided rule instance has the same values as the
// caller.
func (r *Rule) IsEqual(rule *Rule) bool {
	if r.Type != rule.Type || r.Value != rule.Value || r.Action != rule.Action {
		return false
	} else if (r.Protocol == nil) != (rule.Protocol == nil) || (r.Port == nil) != (rule.Port == nil) {
		return false
	} else if r.Protocol != nil && *r.Protocol != *rule.Protocol {
		return false
	} else if r.Port != nil && *r.Port != *rule.Port {
		return false
	}

	return true
}

// RuleStore declares the required operations that any storage service must
// implement to manage rule persistence.
type RuleStore interface {

	// Create registers a new rule into the database.
	Create(ctx context.Context, rule Rule) (*Rule, error)

	// GetByID retrieves the rule whose ID matches the provided argument.
	GetByID(ctx context.Context, id int64) (*Rule, error)

	// List retrieves all existent groups within the database. If filter is
	// provided, it returns only the rules that match the specified conditions.
	List(ctx context.Context, filter RuleFilter) ([]Rule, error)

	// Update persists changes made to a rule into the database.
	Update(ctx context.Context, rule Rule) (*Rule, error)

	// Remove removes the rule whose ID matches the provided argument.
	Remove(ctx context.Context, id int64) error
}

// IPRuleBuilder provides a builder pattern for constructing IP-based rules.
type IPRuleBuilder struct {
	rule Rule
}

// NewIPRule initializes a new IPRuleBuilder instance with action and target value.
func NewIPRule(action RuleAction, value string) *IPRuleBuilder {
	return &IPRuleBuilder{
		rule: Rule{
			Type:   IPRuleType,
			Action: action,
			Value:  value,
		},
	}
}

// WithProtocol sets the targeted network protocol for an IP rule.
func (b *IPRuleBuilder) WithProtocol(p string) *IPRuleBuilder {
	b.rule.Protocol = &p
	return b
}

// WithPort sets the targeted network port for an IP rule.
func (b *IPRuleBuilder) WithPort(p int) *IPRuleBuilder {
	b.rule.Port = &p
	return b
}

// Build constructs and returns the finalized IP Rule instance.
func (b *IPRuleBuilder) Build() Rule {
	return b.rule
}

// DomainRuleBuilder provides a builder pattern for constructing domain-based rules.
type DomainRuleBuilder struct {
	rule Rule
}

// NewDomainRule initializes a new DomainRuleBuilder instance with action and target value.
func NewDomainRule(action RuleAction, value string) *DomainRuleBuilder {
	return &DomainRuleBuilder{
		rule: Rule{
			Type:   DomainRuleType,
			Action: action,
			Value:  value,
		},
	}
}

// Build constructs and returns the finalized Domain Rule instance.
func (b *DomainRuleBuilder) Build() Rule {
	return b.rule
}
