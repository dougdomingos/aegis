package group

import (
	"time"

	"github.com/google/uuid"
)

// CreateGroupSchema declares the required fields for registering a new
// group within the system.
type CreateGroupSchema struct {
	Name string
}

// GetGroupByNameSchema declares the required fields for retrieving a group
// through its name.
type GetGroupByNameSchema struct {
	Name string
}

// RemoveGroup declares the required fields for removing a group from the
// system.
type RemoveGroup struct {
	Name string
}

type ChangeGroupNameSchema struct {
	TargetGroupName string
	NewName         string
}

// GroupOutputSchema declares the groups fields displayed to clients.
type GroupOutputSchema struct {
	ID        uuid.UUID
	Name      string
	CreatedAt time.Time
}
