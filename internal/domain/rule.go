package domain

import (
	"context"
	"net"
	"regexp"
	"strings"
	"time"

	"dougdomingos.com/aegis/internal/errors"
)

// RuleType represents the rule classification type.
type RuleType string

const (
	// DomainRuleType identifies domain-based filtering rules.
	DomainRuleType RuleType = "DOMAIN"

	// IPRuleType identifies IP address or CIDR-based filtering rules.
	IPRuleType RuleType = "IP"
)

// domainRegex
var domainRegex = regexp.MustCompile(`^([a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,}$`)

// Rule represents a network rule to be enforced by the system.
type Rule struct {

	// ID is the unique identifier of the rule, assigned at creation.
	ID int64

	// PolicyID is the identifier of the policy that owns this rule. A rule
	// only exists within a policy.
	PolicyID int64

	// Type is the immutable rule classification (e.g., IP, DOMAIN).
	Type RuleType

	// Value is the targeted resource (e.g., IP address, CIDR, domain name). Its
	// format must match the specified RuleType of its instance.
	Value string

	// (Optional) Protocol is the targeted network protocol (e.g., TCP, UDP). Only
	// applicable for IP-based rules; defaults to nil if not provided.
	Protocol *string

	// (Optional) Port is the targeted network port or range. Only applicable for
	// IP-based rules; defaults to nil if not provided.
	Port *int

	// CreatedAt is the timestamp when the rule was first created.
	CreatedAt time.Time
}

// RulePatch contains the fields that can be updated in a Rule. Only non-nil
// fields will be applied.
type RulePatch struct {
	Value    *string
	Protocol *string
	Port     *int
}

// Patch applies a partial update to the Rule, modifying only the non-nil
// fields of the provided RulePatch.
func (r *Rule) Patch(patch RulePatch) {
	if patch.Value != nil {
		r.Value = *patch.Value
	}

	if patch.Protocol != nil {
		r.Protocol = patch.Protocol
	}

	if patch.Port != nil {
		r.Port = patch.Port
	}
}

// IsEqual checks whether the provided rule instance has the same values as the
// caller.
func (r *Rule) IsEqual(rule *Rule) bool {
	if r.Type != rule.Type || r.Value != rule.Value {
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

// Validate checks whether the rule instance holds a valid configuration.
func (r *Rule) Validate() error {
	if r.PolicyID <= 0 {
		return errors.ErrRulePolicyRequired
	}

	if strings.TrimSpace(string(r.Type)) == "" {
		return errors.ErrRuleTypeRequired
	}

	if strings.TrimSpace(r.Value) == "" {
		return errors.ErrRuleValueRequired
	}

	switch r.Type {
	case DomainRuleType:
		if r.Protocol != nil || r.Port != nil {
			return errors.ErrInvalidFieldForRuleType
		}

		if !domainRegex.MatchString(r.Value) {
			return errors.ErrMalformedRuleValue
		}

	case IPRuleType:
		if net.ParseIP(r.Value) == nil {
			return errors.ErrMalformedRuleValue
		}

		if r.Port != nil {
			if *r.Port < 0 || *r.Port > 65535 {
				return errors.ErrInvalidRulePortValue
			}
		}
	}

	return nil
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
