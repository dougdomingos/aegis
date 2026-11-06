package errors

import "errors"

var (
	// ErrAuthenticationRejected is returned when the Orchestrator refuses the
	// credentials presented by the Sentinel.
	ErrAuthenticationRejected = errors.New("authentication rejected by the orchestrator")

	// ErrOrchestratorUnreachable is returned when the Orchestrator cannot be
	// contacted.
	ErrOrchestratorUnreachable = errors.New("orchestrator is unreachable")
)
