package errors

import "errors"

var (
	// ErrGroupNotFound is returned when the requested group is not found within
	// the database.
	ErrGroupNotFound = errors.New("requested group not found")

	// ErrGroupNameRequired is returned when the name of a group is required but
	// not provided by the payload.
	ErrGroupNameRequired = errors.New("must specify group name")

	// ErrGroupNameAlreadyExists is returned when the name used to create/update a
	// group is already in use by another group.
	ErrGroupNameAlreadyExists = errors.New("provided name already exists")

	// ErrGroupNewNameRequired is returns when the new name of a group in a rename
	// operation is not provided
	ErrGroupNewNameRequired = errors.New("must specify new name for group")
)
