package schemas

import (
	"time"

	"dougdomingos.com/aegis/internal/domain"
)

// CreateRuleSchema declares the avaliable fields for registering a new rule
// within the system.
type CreateRuleSchema struct {
	Type     domain.RuleType   `json:"rule_type"`
	Action   domain.RuleAction `json:"action"`
	Value    string            `json:"value"`
	Protocol *string           `json:"protocol,omitempty"`
	Port     *int              `json:"port,omitempty"`
}

// GetRuleByIDSchema declares the required fields to retrieve a rule by its ID.
type GetRuleByIDSchema struct {
	ID int `json:"id"`
}

// ListRulesSchema declares the avaliable fields for listing the existent rules
// within the system.
type ListRulesSchema struct {
	Type   *domain.RuleType   `json:"rule_type,omitempty"`
	Action *domain.RuleAction `json:"action,omitempty"`
	Value  *string            `json:"value,omitempty"`
}

// UpdateRuleSchema declares the avaliable fields to update a rule within the
// system.
type UpdateRuleSchema struct {
	ID       int                `json:"id"`
	Action   *domain.RuleAction `json:"action,omitempty"`
	Value    *string            `json:"value,omitempty"`
	Protocol *string            `json:"protocol,omitempty"`
	Port     *int               `json:"port,omitempty"`
}

// RemoveRuleSchema declares the required fields to remove a rule from the
// system.
type RemoveRuleSchema struct {
	ID int `json:"id"`
}

// RuleOutputSchema declares the rule fields displayed to clients.
type RuleOutputSchema struct {
	ID        int               `json:"id"`
	Type      domain.RuleType   `json:"rule_type"`
	Action    domain.RuleAction `json:"action"`
	Value     string            `json:"value"`
	Protocol  *string           `json:"protocol,omitempty"`
	Port      *int              `json:"port,omitempty"`
	CreatedAt time.Time         `json:"created_at"`
}
