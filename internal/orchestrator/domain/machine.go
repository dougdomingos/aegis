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

	// GroupID is the identifier of the group the machine is linked to, or nil
	// when the machine is not linked to any group.
	GroupID *int64

	// GroupName is the human-friendly label of the linked group, populated on
	// reads. It is empty when the machine is not linked.
	GroupName string
}

// MachineImport declares a single entry of a machine registry import: the
// machine to register and the name of the laboratory it belongs to.
type MachineImport struct {

	// Machine holds the IP and name of the machine to register.
	Machine Machine

	// Lab is the name of the group the machine must be linked to. An empty lab
	// leaves the current group association of the machine untouched.
	Lab string
}

// MachineImportResult summarizes the changes applied by a machine registry
// import.
type MachineImportResult struct {

	// MachinesCreated is the number of machines registered by the import.
	MachinesCreated int

	// MachinesUpdated is the number of already registered machines whose name
	// was refreshed by the import.
	MachinesUpdated int

	// GroupsCreated is the number of groups created to match unknown labs.
	GroupsCreated int

	// RelinkedIDs lists the IDs of the machines whose group association was
	// changed by the import.
	RelinkedIDs []int64
}

// MachineStore declares the required operations that any storage service must
// implement to manage machine persistence.
type MachineStore interface {

	// GetByIP retrieves the machine whose IP matches the provided argument.
	GetByIP(ctx context.Context, ip string) (*Machine, error)

	// GetByID retrieves the machine whose ID matches the provided argument.
	GetByID(ctx context.Context, id int64) (*Machine, error)

	// LinkToGroup associates the machine with the provided group, moving it
	// from any previously linked group.
	LinkToGroup(ctx context.Context, machineID int64, groupID int64) (*Machine, error)

	// UnlinkFromGroup clears the group association of the machine.
	UnlinkFromGroup(ctx context.Context, machineID int64) (*Machine, error)

	// ListByGroup retrieves all machines linked to the provided group.
	ListByGroup(ctx context.Context, groupID int64) ([]Machine, error)

	// List retrieves all registered machines within the database.
	List(ctx context.Context) ([]Machine, error)

	// Upsert registers a machine or updates its name when its IP already exists,
	// preserving the current connection state.
	Upsert(ctx context.Context, machine Machine) (*Machine, error)

	// RemoveByIP removes the machine whose IP matches the provided argument.
	RemoveByIP(ctx context.Context, ip string) error

	// RemoveByID removes the machine whose ID matches the provided argument.
	RemoveByID(ctx context.Context, id int64) error

	// Import atomically registers or renames each provided machine, creates a
	// group for every unknown lab, and links each machine with a non-empty lab
	// to its group. Machines absent from the entries are kept.
	Import(ctx context.Context, entries []MachineImport) (*MachineImportResult, error)

	// Touch updates the connection state and last-seen timestamp of the machine
	// whose ID matches the provided argument. An empty MAC preserves the stored
	// one.
	Touch(ctx context.Context, id int64, mac string, status MachineStatus) (*Machine, error)
}
