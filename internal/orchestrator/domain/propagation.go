package domain

import (
	"context"
	"time"
)

// PropagationKind classifies the audit row of a sent message.
type PropagationKind string

const (
	// PropagationPackage records the delivery of a full policy snapshot.
	PropagationPackage PropagationKind = "PACKAGE"

	// PropagationRevoke records the delivery of a rules-clearing message.
	PropagationRevoke PropagationKind = "REVOKE"
)

// PropagationStatus is the lifecycle state of one propagation row.
type PropagationStatus string

const (
	// PropagationPending marks a row created but not yet sent.
	PropagationPending PropagationStatus = "PENDING"

	// PropagationSent marks a row delivered over the machine's stream.
	PropagationSent PropagationStatus = "SENT"

	// PropagationAcked marks a row confirmed by the Sentinel.
	PropagationAcked PropagationStatus = "ACKED"

	// PropagationNacked marks a row rejected by the Sentinel.
	PropagationNacked PropagationStatus = "NACKED"

	// PropagationFailed marks a row whose delivery errored.
	PropagationFailed PropagationStatus = "FAILED"
)

// Propagation is the audit record of one message sent to a machine,
// identified per machine by a strictly increasing Sequence.
type Propagation struct {
	// ID is the unique identifier of the row, assigned at creation.
	ID int64

	// MachineID is the destination machine. It has no foreign key: the audit
	// log outlives the machine it refers to.
	MachineID int64

	// Sequence is the per-machine order assigned by the store at creation.
	Sequence int64

	// Kind classifies the message as a package or a revoke.
	Kind PropagationKind

	// PolicyID is the optional identity of the policy the message refers to.
	PolicyID *int64

	// PolicyName is the name of the policy at send time.
	PolicyName string

	// PolicyVersion is the version of the policy at send time.
	PolicyVersion int

	// PolicyType is the type of the policy at send time (e.g., WHITELIST).
	PolicyType string

	// Status is the lifecycle state of the propagation.
	Status PropagationStatus

	// SentAt is the timestamp when the message was handed to the stream.
	SentAt *time.Time

	// AckedAt is the timestamp when the Sentinel acknowledged the message.
	AckedAt *time.Time

	// NackReason carries the rejection reason of a NACK or delivery failure.
	NackReason string

	// Payload is the serialized wire message, retained for audit only.
	Payload string

	// CreatedAt is the timestamp when the row was inserted.
	CreatedAt time.Time
}

// PropagationStore declares the required operations that any storage service
// must implement to manage propagation audit rows.
type PropagationStore interface {

	// Create assigns the next per-machine sequence inside the insert
	// transaction, encodes the wire payload with it, and stores the row as
	// PENDING. The encode callback must produce the exact serialized message
	// that will be sent, so the audit reflects the delivered bytes.
	Create(ctx context.Context, propagation Propagation, encode func(sequence int64) (string, error)) (*Propagation, error)

	// MarkSent transitions the row of the given machine and sequence to SENT
	// with the provided timestamp.
	MarkSent(ctx context.Context, machineID, sequence int64, at time.Time) error

	// MarkFailed transitions the row of the given machine and sequence to
	// FAILED with the provided reason, leaving any acknowledgment timestamp
	// untouched.
	MarkFailed(ctx context.Context, machineID, sequence int64, reason string) error

	// Acknowledge applies the Sentinel's response to the row of the given
	// machine and sequence. A missing row is not an error: a late
	// acknowledgement for a superseded message is simply ignored.
	Acknowledge(ctx context.Context, machineID, sequence int64, status PropagationStatus, reason string, at time.Time) error

	// LatestByMachine retrieves the row with the highest sequence for the
	// given machine, or nil when the machine has none.
	LatestByMachine(ctx context.Context, machineID int64) (*Propagation, error)

	// ListLatest retrieves the row with the highest sequence of every machine
	// that has at least one propagation.
	ListLatest(ctx context.Context) ([]Propagation, error)

	// Epoch returns the stable identifier of this database's sequence space,
	// generating and persisting it on first call.
	Epoch(ctx context.Context) (string, error)
}
