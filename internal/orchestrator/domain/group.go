package domain

import (
	"context"
	"time"
)

// Group represents a logical set of machines that belong to the same physical
// laboratory.
type Group struct {

	// ID is the unique identifier of the group, assigned at creation.
	ID int64

	// Name is a unique, human-friendly label to identify the group.
	Name string

	// CreatedAt is the timestamp assigned at the registration of a group.
	CreatedAt time.Time

	// PolicyID is the identifier of the policy linked to the group, or nil
	// when the group is not linked to any policy.
	PolicyID *int64

	// PolicyName is the human-friendly label of the linked policy, populated
	// on reads. It is empty when the group is not linked.
	PolicyName string
}

// GroupStore declares the required operations that any storage service must
// implement to manage group persistence.
type GroupStore interface {

	// Create registers a new group into the database.
	Create(ctx context.Context, name string) (*Group, error)

	// GetByID retrieves the group whose ID matches the provided argument.
	GetByID(ctx context.Context, id int64) (*Group, error)

	// GetByName retrieves the group whose name matches the provided argument.
	GetByName(ctx context.Context, name string) (*Group, error)

	// Exists checks the existence of a group with the specified name.
	Exists(ctx context.Context, name string) (bool, error)

	// Update persists changes made to a group into the database.
	Update(ctx context.Context, group Group) (*Group, error)

	// List retrieves all existent groups within the database.
	List(ctx context.Context) ([]Group, error)

	// Remove removes the group whose name matches the provided argument.
	Remove(ctx context.Context, name string) error

	// LinkToPolicy associates the group with the provided policy, replacing any
	// previously linked policy.
	LinkToPolicy(ctx context.Context, groupName string, policyID int64) (*Group, error)

	// UnlinkFromPolicy clears the policy association of the group.
	UnlinkFromPolicy(ctx context.Context, groupName string) (*Group, error)
}
