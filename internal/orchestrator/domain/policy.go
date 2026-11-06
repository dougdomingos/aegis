package domain

import (
	"context"
	"time"
)

// PolicyType represents the enforcement mode applied to the rules that
// belong to a policy.
type PolicyType string

const (
	// WhitelistPolicyType defines that only the rules of the policy are
	// allowed (everything else is blocked).
	WhitelistPolicyType PolicyType = "WHITELIST"

	// BlacklistPolicyType defines that only the rules of the policy are
	// blocked (everything else is allowed).
	BlacklistPolicyType PolicyType = "BLACKLIST"
)

// Policy represents a set of rules that can be applied to a logical group of
// machines.
type Policy struct {

	// ID is the unique identifier of the policy, assigned at creation.
	ID int64

	// Name is a unique, human-friendly label to identify the policy.
	Name string

	// Type is the enforcement mode applied to the rules that belong to this
	// policy. Immutable after creation.
	Type PolicyType

	// Version is the number of modifications already applied to the policy.
	// Starts at 1 and is incremented on every actual change.
	Version int

	// CreatedAt is the timestamp assigned at the registration of a policy.
	CreatedAt time.Time

	// UpdatedAt is the timestamp of the last modification applied to the
	// policy.
	UpdatedAt time.Time
}

// PolicyStore declares the required operations that any storage service must
// implement to manage policy persistence.
type PolicyStore interface {

	// Create registers a new policy into the database.
	Create(ctx context.Context, name string, policyType PolicyType) (*Policy, error)

	// GetByID retrieves the policy whose ID matches the provided argument.
	GetByID(ctx context.Context, id int64) (*Policy, error)

	// GetByName retrieves the policy whose name matches the provided argument.
	GetByName(ctx context.Context, name string) (*Policy, error)

	// Exists checks the existence of a policy with the specified name.
	Exists(ctx context.Context, name string) (bool, error)

	// Update persists changes made to a policy into the database.
	Update(ctx context.Context, policy Policy) (*Policy, error)

	// List retrieves all existent policies within the database.
	List(ctx context.Context) ([]Policy, error)

	// Remove removes the policy whose ID matches the provided argument.
	Remove(ctx context.Context, id int64) error
}
