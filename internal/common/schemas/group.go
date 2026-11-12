package schemas

import "time"

// CreateGroupSchema declares the required fields for registering a new
// group within the system.
type CreateGroupSchema struct {
	Name string `json:"name"`
}

// GetGroupByNameSchema declares the required fields for retrieving a group
// through its name.
type GetGroupByNameSchema struct {
	Name string `json:"name"`
}

// RemoveGroup declares the required fields for removing a group from the
// system.
type RemoveGroupSchema struct {
	Name string `json:"name"`
}

// ChangeGroupNameSchema declares the required fields for updating the
// name of a group.
type ChangeGroupNameSchema struct {
	TargetGroupName string `json:"targetGroupName"`
	NewName         string `json:"newName"`
}

// GroupOutputSchema declares the groups fields displayed to clients.
type GroupOutputSchema struct {
	ID         int64     `json:"id"`
	Name       string    `json:"name"`
	CreatedAt  time.Time `json:"createdAt"`
	MachineIDs []int64   `json:"machineIDs"`

	// PolicyID is the identifier of the linked policy, or nil when no policy is
	// linked.
	PolicyID *int64 `json:"policyID"`

	// PolicyName is the name of the linked policy; empty when none is linked.
	PolicyName string `json:"policyName"`
}

// LinkPolicyToGroupSchema declares the policy selected to be linked to the
// group named in the request path.
type LinkPolicyToGroupSchema struct {
	PolicyName string `json:"policy_name"`
}

// LinkMachineToGroupSchema declares the machine selected to be linked to the
// group named in the request path.
type LinkMachineToGroupSchema struct {
	MachineID int64 `json:"machine_id"`
}
