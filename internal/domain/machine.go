package domain

import (
	"context"
	"time"
)

// MachineStatus represents the connection state of a machine within the
// laboratory.
type MachineStatus string

const (
	// DisconnectedStatus defines that the machine is registered but has not
	// authenticated, or is no longer connected to the system.
	DisconnectedStatus MachineStatus = "DISCONNECTED"

	// ConnectedStatus defines that the machine has successfully authenticated
	// and is currently connected to the system.
	ConnectedStatus MachineStatus = "CONNECTED"
)

// Machine represents a laboratory endpoint that may authenticate against the
// system. Its identity is derived from its fixed IP address.
type Machine struct {

	// ID is the unique identifier of the machine, assigned at registration.
	ID int64

	// Name is the human-friendly label assigned to the machine through the
	// accepted-sources configuration.
	Name string

	// IP is the canonical identity of the machine within the laboratory.
	IP string

	// MAC is optional metadata about the machine's network interface.
	MAC string

	// Status is the current connection state of the machine.
	Status MachineStatus

	// LastSeenAt is the timestamp of the last successful authentication. It is
	// nil for machines that have never authenticated.
	LastSeenAt *time.Time

	// CreatedAt is the timestamp assigned at the registration of the machine.
	CreatedAt time.Time
}

// MachineStore declares the required operations that any storage service must
// implement to manage machine persistence.
type MachineStore interface {

	// GetByIP retrieves the machine whose IP matches the provided argument.
	GetByIP(ctx context.Context, ip string) (*Machine, error)

	// List retrieves all registered machines within the database.
	List(ctx context.Context) ([]Machine, error)

	// Upsert registers a machine or updates its name when its IP already exists,
	// preserving the current connection state.
	Upsert(ctx context.Context, machine Machine) (*Machine, error)

	// RemoveByIP removes the machine whose IP matches the provided argument.
	RemoveByIP(ctx context.Context, ip string) error

	// Sync reconciles the stored machines with the provided accepted sources,
	// upserting each configured machine and pruning any whose IP is no longer
	// configured, atomically.
	Sync(ctx context.Context, machines []Machine) error

	// Touch updates the connection state and last-seen timestamp of the machine
	// whose ID matches the provided argument. An empty MAC preserves the stored
	// one.
	Touch(ctx context.Context, id int64, mac string, status MachineStatus) (*Machine, error)
}
