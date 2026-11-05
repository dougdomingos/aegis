// Package propagation declares the transport-neutral message types shared by
// the Orchestrator and the Sentinel for rule distribution.
package propagation

import "context"

// Kind distinguishes the two propagation messages a machine can receive.
type Kind string

const (
	// KindPackage carries a full snapshot of the machine's desired rules.
	KindPackage Kind = "PACKAGE"

	// KindRevoke instructs the machine to clear its applied rules.
	KindRevoke Kind = "REVOKE"
)

// Rule is a single filtering rule as carried on the wire.
type Rule struct {
	Type     string  `json:"type"`
	Value    string  `json:"value"`
	Protocol *string `json:"protocol,omitempty"`
	Port     *int    `json:"port,omitempty"`
}

// Message is a propagation delivery: either a full policy package or a
// revoke, ordered per machine by Sequence.
type Message struct {
	Epoch         string `json:"epoch"`
	Sequence      int64  `json:"sequence"`
	MachineID     int64  `json:"machine_id"`
	Kind          Kind   `json:"kind"`
	PolicyID      int64  `json:"policy_id,omitempty"`
	PolicyName    string `json:"policy_name,omitempty"`
	PolicyVersion int    `json:"policy_version,omitempty"`
	PolicyType    string `json:"policy_type,omitempty"`
	Rules         []Rule `json:"rules,omitempty"`
}

// AckStatus is the terminal outcome a Sentinel reports for one Message.
type AckStatus string

const (
	// AckAcked means the message was validated and persisted.
	AckAcked AckStatus = "ACKED"

	// AckNacked means the message was rejected; Reason carries the cause.
	AckNacked AckStatus = "NACKED"
)

// Policy type wire values shared by the Orchestrator and the Sentinel.
const (
	// PolicyTypeWhitelist marks a policy that blocks everything but its rules.
	PolicyTypeWhitelist = "WHITELIST"

	// PolicyTypeBlacklist marks a policy that blocks only its rules.
	PolicyTypeBlacklist = "BLACKLIST"
)

// Ack is a Sentinel's response to one Message, referencing its Sequence.
type Ack struct {
	Sequence int64     `json:"sequence"`
	Status   AckStatus `json:"status"`
	Reason   string    `json:"reason,omitempty"`
}

// Stream is the ordered, bidirectional channel a Sentinel uses to receive
// messages and send acknowledgements.
type Stream interface {
	Recv() (*Message, error)
	Send(ack Ack) error
	CloseSend() error
}

// MessageSender delivers one propagation message to a connected machine over
// its stream. It is implemented by the API adapters that own the transport
// and consumed by the service that schedules deliveries.
type MessageSender interface {
	Send(ctx context.Context, message Message) error
}
