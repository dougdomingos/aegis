package schemas

import "time"

// CreatePolicySchema declares the required fields for registering a new
// policy within the system.
type CreatePolicySchema struct {
	Name string `json:"name"`
}

// GetPolicyByIDSchema declares the required fields for retrieving a policy
// through its ID.
type GetPolicyByIDSchema struct {
	ID int64 `json:"id"`
}

// UpdatePolicySchema declares the required fields for updating the name of
// a policy.
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
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Version   int       `json:"version"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
