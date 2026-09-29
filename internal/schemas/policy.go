package schemas

import (
	"time"

	"dougdomingos.com/aegis/internal/domain"
)

// CreatePolicySchema declares the required fields for registering a new
// policy within the system.
type CreatePolicySchema struct {
	Name string            `json:"name"`
	Type domain.PolicyType `json:"type"`
}

// GetPolicyByIDSchema declares the required fields for retrieving a policy
// through its ID.
type GetPolicyByIDSchema struct {
	ID int64 `json:"id"`
}

// UpdatePolicySchema declares the required fields for updating the name of
// a policy. The policy type is immutable after creation and is therefore not
// part of this contract; any value sent for it is ignored by the decoder.
type UpdatePolicySchema struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// RemovePolicySchema declares the required fields for removing a policy from
// the system.
type RemovePolicySchema struct {
	ID int64 `json:"id"`
}

// PolicyOutputSchema declares the policy fields displayed to clients.
type PolicyOutputSchema struct {
	ID        int64             `json:"id"`
	Name      string            `json:"name"`
	Type      domain.PolicyType `json:"type"`
	Version   int               `json:"version"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
}
