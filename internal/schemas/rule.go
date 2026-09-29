package schemas

import (
	"time"

	"dougdomingos.com/aegis/internal/domain"
)

// CreateRuleSchema declares the avaliable fields for registering a new rule
// within the system.
type CreateRuleSchema struct {
	PolicyID int64           `json:"-"`
	Type     domain.RuleType `json:"rule_type"`
	Value    string          `json:"value"`
	Protocol *string         `json:"protocol,omitempty"`
	Port     *int            `json:"port,omitempty"`
}

// GetRuleByIDSchema declares the required fields to retrieve a rule by its ID.
type GetRuleByIDSchema struct {
	PolicyID int64 `json:"-"`
	ID       int   `json:"id"`
}

// ListRulesSchema declares the avaliable fields for listing the existent rules
// within the system.
type ListRulesSchema struct {
	PolicyID int64            `json:"id"`
	Type     *domain.RuleType `json:"rule_type,omitempty"`
	Value    *string          `json:"value,omitempty"`
}

// UpdateRuleSchema declares the avaliable fields to update a rule within the
// system.
type UpdateRuleSchema struct {
	PolicyID int64   `json:"-"`
	ID       int     `json:"id"`
	Value    *string `json:"value,omitempty"`
	Protocol *string `json:"protocol,omitempty"`
	Port     *int    `json:"port,omitempty"`
}

// RemoveRuleSchema declares the required fields to remove a rule from the
// system.
type RemoveRuleSchema struct {
	PolicyID int64 `json:"-"`
	ID       int   `json:"id"`
}

// RuleOutputSchema declares the rule fields displayed to clients.
type RuleOutputSchema struct {
	ID        int             `json:"id"`
	PolicyID  int64           `json:"policy_id"`
	Type      domain.RuleType `json:"rule_type"`
	Value     string          `json:"value"`
	Protocol  *string         `json:"protocol,omitempty"`
	Port      *int            `json:"port,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
}
