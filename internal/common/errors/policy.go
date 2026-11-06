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

	// ErrPolicyTypeRequired is returned when the type of a policy is required
	// but not provided by the payload.
	ErrPolicyTypeRequired = errors.New("must specify policy type")

	// ErrInvalidPolicyType is returned when the provided policy type is not a
	// supported one (WHITELIST or BLACKLIST).
	ErrInvalidPolicyType = errors.New("policy type must be WHITELIST or BLACKLIST")

	// ErrPolicyHasNoRules is returned when a policy without rules is linked
	// to a group.
	ErrPolicyHasNoRules = errors.New("policy must have at least one rule to be linked")

	// ErrLinkedPolicyLastRule is returned when the last rule of a policy that
	// is linked to a group would be removed, leaving the group without
	// enforceable rules.
	ErrLinkedPolicyLastRule = errors.New("cannot remove the last rule of a policy linked to a group")
)
