package errors

import "errors"

var (
	// ErrPolicyNotFound is returned when the requested policy is not found
	// within the database.
	ErrPolicyNotFound = errors.New("requested policy not found")

	// ErrPolicyNameRequired is returned when the name of a policy is required
	// but not provided by the payload.
	ErrPolicyNameRequired = errors.New("must specify policy name")

	// ErrPolicyNameAlreadyExists is returned when the name used to create/update
	// a policy is already in use by another policy.
	ErrPolicyNameAlreadyExists = errors.New("provided name already exists")
)
