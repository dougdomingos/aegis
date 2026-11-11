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
	ID          int64
	Name        string
	IP          string
	MAC         string
	GroupID     *int64
	GroupName   string
	Status      string
	LastSeenAt  *time.Time
	CreatedAt   time.Time
	Propagation PropagationOutputSchema `json:"propagation"`
}

// PropagationOutputSchema declares the latest propagation state of a machine.
type PropagationOutputSchema struct {
	Sequence      int64      `json:"sequence"`
	Kind          string     `json:"kind"`
	Status        string     `json:"status"`
	PolicyID      *int64     `json:"policy_id"`
	PolicyName    string     `json:"policy_name"`
	PolicyVersion int        `json:"policy_version"`
	PolicyType    string     `json:"policy_type"`
	SentAt        *time.Time `json:"sent_at"`
	AckedAt       *time.Time `json:"acked_at"`
	DurationMS    *int64     `json:"duration_ms"`
	NackReason    string     `json:"nack_reason"`
}

// AuthenticateOutputSchema declares the result of a successful authentication.
type AuthenticateOutputSchema struct {
	SessionToken string
	ExpiresAt    time.Time
	Machine      MachineOutputSchema
}

// MachineImportOutputSchema declares the summary of a machine registry import.
type MachineImportOutputSchema struct {
	MachinesCreated int `json:"machines_created"`
	MachinesUpdated int `json:"machines_updated"`
	GroupsCreated   int `json:"groups_created"`
	MachinesLinked  int `json:"machines_linked"`
}
