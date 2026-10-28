package errors

import "errors"

var (
	// ErrMachineIPRequired is returned when the IP of a machine is required but
	// not provided by the payload.
	ErrMachineIPRequired = errors.New("must specify machine ip")

	// ErrInvalidCredentials is returned when a machine cannot be authenticated,
	// either because its IP is not a configured source or because the shared
	// token is incorrect.
	ErrInvalidCredentials = errors.New("invalid machine credentials")

	// ErrSessionTokenInvalid is returned when a session token presented to the
	// system is malformed, tampered with, or expired.
	ErrSessionTokenInvalid = errors.New("invalid session token")
)
