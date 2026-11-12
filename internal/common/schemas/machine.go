package schemas

import "time"

// InitiateAuthenticationSchema declares the fields sent by a machine to start
// an authentication challenge.
type InitiateAuthenticationSchema struct {
	IP string `json:"ip"`
}

// InitiateOutputSchema declares the result of starting an authentication
// challenge.
type InitiateOutputSchema struct {
	Nonce     string    `json:"nonce"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// AuthenticateMachineSchema declares the fields sent by a machine to complete
// an authentication challenge.
type AuthenticateMachineSchema struct {
	IP        string `json:"ip"`
	MAC       string `json:"mac"`
	Signature string `json:"signature"`
}

// MachineOutputSchema declares the machine fields displayed to clients.
type MachineOutputSchema struct {
	ID          int64                   `json:"id"`
	Name        string                  `json:"name"`
	IP          string                  `json:"ip"`
	MAC         string                  `json:"mac"`
	GroupID     *int64                  `json:"groupID"`
	GroupName   string                  `json:"groupName"`
	Status      string                  `json:"status"`
	LastSeenAt  *time.Time              `json:"lastSeenAt"`
	CreatedAt   time.Time               `json:"createdAt"`
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
	SessionToken string              `json:"sessionToken"`
	ExpiresAt    time.Time           `json:"expiresAt"`
	Machine      MachineOutputSchema `json:"machine"`
}

// MachineImportOutputSchema declares the summary of a machine registry import.
type MachineImportOutputSchema struct {
	MachinesCreated int `json:"machines_created"`
	MachinesUpdated int `json:"machines_updated"`
	GroupsCreated   int `json:"groups_created"`
	MachinesLinked  int `json:"machines_linked"`
}
