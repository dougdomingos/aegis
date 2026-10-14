package schemas

import "time"

// InitiateAuthenticationSchema declares the fields sent by a machine to start
// an authentication challenge.
type InitiateAuthenticationSchema struct {
	IP string
}

// InitiateOutputSchema declares the result of starting an authentication
// challenge.
type InitiateOutputSchema struct {
	Nonce     string
	ExpiresAt time.Time
}

// AuthenticateMachineSchema declares the fields sent by a machine to complete
// an authentication challenge.
type AuthenticateMachineSchema struct {
	IP        string
	MAC       string
	Signature string
}

// MachineOutputSchema declares the machine fields displayed to clients.
type MachineOutputSchema struct {
	ID         int64
	Name       string
	IP         string
	MAC        string
	Status     string
	LastSeenAt *time.Time
	CreatedAt  time.Time
}

// AuthenticateOutputSchema declares the result of a successful authentication.
type AuthenticateOutputSchema struct {
	SessionToken string
	ExpiresAt    time.Time
	Machine      MachineOutputSchema
}
