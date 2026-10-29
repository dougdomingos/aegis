package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"dougdomingos.com/aegis/internal/common/errors"
	"dougdomingos.com/aegis/internal/common/schemas"
	"dougdomingos.com/aegis/internal/orchestrator/domain"
)

// GroupService provides all operations needed to manage logical groups in the
// system.
type GroupService struct {

	// store provides database operations related to groups
	store domain.GroupStore

	// machineStore provides database operations related to machines, used to
	// link and unlink machines to groups.
	machineStore domain.MachineStore

	// policyStore provides database operations related to policies, used to
	// resolve and link policies to groups.
	policyStore domain.PolicyStore

	// stalenessGrace is the duration a machine remains displayed as CONNECTED
	// after its last heartbeat.
	stalenessGrace time.Duration

	// now is the clock used to resolve the current time.
	now func() time.Time
}

// NewGroupService creates a new service instance with the provided store
// manager.
func NewGroupService(store domain.GroupStore, machineStore domain.MachineStore, policyStore domain.PolicyStore, stalenessGrace time.Duration, now func() time.Time) *GroupService {
	return &GroupService{
		store:          store,
		machineStore:   machineStore,
		policyStore:    policyStore,
		stalenessGrace: stalenessGrace,
		now:            now,
	}
}

// CreateGroup registers a new logical group into the system. The name provided to
// this new group must be unique, otherwise the operation fails.
func (service *GroupService) CreateGroup(ctx context.Context, payload schemas.CreateGroupSchema) (*schemas.GroupOutputSchema, error) {
	if strings.TrimSpace(payload.Name) == "" {
		return nil, errors.ErrGroupNameRequired
	}

	existentGroup, err := service.store.GetByName(ctx, payload.Name)
	if err != nil {
		return nil, err
	}

	if existentGroup != nil {
		return nil, errors.ErrGroupNameAlreadyExists
	}

	newGroup, err := service.store.Create(ctx, payload.Name)
	if err != nil {
		return nil, fmt.Errorf("Failed to register new group: %w", err)
	}

	return mapGroupToOutputSchema(newGroup, nil), nil
}

// GetGroupByName retrieves a group by its name. If no group matches the
// requested name, it returns nil.
func (service *GroupService) GetGroupByName(ctx context.Context, payload schemas.GetGroupByNameSchema) (*schemas.GroupOutputSchema, error) {
	if strings.TrimSpace(payload.Name) == "" {
		return nil, errors.ErrGroupNameRequired
	}

	existentGroup, err := service.store.GetByName(ctx, payload.Name)
	if err != nil {
		return nil, err
	}

	if existentGroup == nil {
		return nil, nil
	}

	ids, err := service.machineIDs(ctx, existentGroup.ID)
	if err != nil {
		return nil, err
	}

	return mapGroupToOutputSchema(existentGroup, ids), nil
}

// ListAllGroups returns all the existent groups within the database.
func (service *GroupService) ListAllGroups(ctx context.Context) ([]schemas.GroupOutputSchema, error) {
	groups, err := service.store.List(ctx)
	if err != nil {
		return nil, err
	}

	output := make([]schemas.GroupOutputSchema, len(groups))
	for i, g := range groups {
		ids, err := service.machineIDs(ctx, g.ID)
		if err != nil {
			return nil, err
		}

		output[i] = *mapGroupToOutputSchema(&g, ids)
	}

	return output, nil
}

func (service *GroupService) ChangeGroupName(ctx context.Context, payload schemas.ChangeGroupNameSchema) (*schemas.GroupOutputSchema, error) {
	if strings.TrimSpace(payload.TargetGroupName) == "" {
		return nil, errors.ErrGroupNameRequired
	}

	if strings.TrimSpace(payload.NewName) == "" {
		return nil, errors.ErrGroupNewNameRequired
	}

	doesGroupExists, err := service.store.Exists(ctx, payload.NewName)
	if err != nil {
		return nil, err
	}

	if doesGroupExists {
		return nil, errors.ErrGroupNameAlreadyExists
	}

	group, err := service.store.GetByName(ctx, payload.TargetGroupName)
	if err != nil {
		return nil, err
	}

	if group == nil {
		return nil, errors.ErrGroupNotFound
	}

	group.Name = payload.NewName
	if _, err := service.store.Update(ctx, *group); err != nil {
		return nil, err
	}

	ids, err := service.machineIDs(ctx, group.ID)
	if err != nil {
		return nil, err
	}

	return mapGroupToOutputSchema(group, ids), nil
}

// RemoveGroup deletes a group from the database by its name. If no group
// matches the requested name, it returns an error.
func (service *GroupService) RemoveGroup(ctx context.Context, payload schemas.RemoveGroupSchema) error {
	if strings.TrimSpace(payload.Name) == "" {
		return errors.ErrGroupNameRequired
	}

	doesGroupExists, err := service.store.Exists(ctx, payload.Name)
	if err != nil {
		return err
	}

	if !doesGroupExists {
		return errors.ErrGroupNotFound
	}

	if err := service.store.Remove(ctx, payload.Name); err != nil {
		return fmt.Errorf("failed to remove group %q: %v", payload.Name, err)
	}

	return nil
}

// LinkMachine associates the machine whose ID matches the argument with the
// group whose name matches the argument, moving it from any previous group.
func (service *GroupService) LinkMachine(ctx context.Context, groupName string, machineID int64) (*schemas.MachineOutputSchema, error) {
	if strings.TrimSpace(groupName) == "" {
		return nil, errors.ErrGroupNameRequired
	}

	group, err := service.store.GetByName(ctx, groupName)
	if err != nil {
		return nil, err
	}
	if group == nil {
		return nil, errors.ErrGroupNotFound
	}

	machine, err := service.machineStore.GetByID(ctx, machineID)
	if err != nil {
		return nil, err
	}
	if machine == nil {
		return nil, errors.ErrMachineNotFound
	}

	linked, err := service.machineStore.LinkToGroup(ctx, machineID, group.ID)
	if err != nil {
		return nil, err
	}

	return mapMachineToOutputSchema(linked, service.now(), service.stalenessGrace), nil
}

// UnlinkMachine removes the association between the machine whose ID matches
// the argument and the group whose name matches the argument.
func (service *GroupService) UnlinkMachine(ctx context.Context, groupName string, machineID int64) (*schemas.MachineOutputSchema, error) {
	if strings.TrimSpace(groupName) == "" {
		return nil, errors.ErrGroupNameRequired
	}

	group, err := service.store.GetByName(ctx, groupName)
	if err != nil {
		return nil, err
	}
	if group == nil {
		return nil, errors.ErrGroupNotFound
	}

	machine, err := service.machineStore.GetByID(ctx, machineID)
	if err != nil {
		return nil, err
	}
	if machine == nil {
		return nil, errors.ErrMachineNotFound
	}
	if machine.GroupID == nil || *machine.GroupID != group.ID {
		return nil, errors.ErrMachineNotInGroup
	}

	unlinked, err := service.machineStore.UnlinkFromGroup(ctx, machineID)
	if err != nil {
		return nil, err
	}

	return mapMachineToOutputSchema(unlinked, service.now(), service.stalenessGrace), nil
}

// LinkPolicy associates the policy whose name matches the argument with the
// group whose name matches the argument, replacing any previously linked
// policy.
func (service *GroupService) LinkPolicy(ctx context.Context, groupName string, policyName string) (*schemas.GroupOutputSchema, error) {
	if strings.TrimSpace(groupName) == "" {
		return nil, errors.ErrGroupNameRequired
	}

	if strings.TrimSpace(policyName) == "" {
		return nil, errors.ErrPolicyNameRequired
	}

	group, err := service.store.GetByName(ctx, groupName)
	if err != nil {
		return nil, err
	}
	if group == nil {
		return nil, errors.ErrGroupNotFound
	}

	policy, err := service.policyStore.GetByName(ctx, policyName)
	if err != nil {
		return nil, err
	}
	if policy == nil {
		return nil, errors.ErrPolicyNotFound
	}

	linked, err := service.store.LinkToPolicy(ctx, groupName, policy.ID)
	if err != nil {
		return nil, err
	}

	linked.PolicyName = policy.Name

	return mapGroupToOutputSchema(linked, nil), nil
}

// UnlinkPolicy removes the association between the group whose name matches
// the argument and its currently linked policy.
func (service *GroupService) UnlinkPolicy(ctx context.Context, groupName string) (*schemas.GroupOutputSchema, error) {
	if strings.TrimSpace(groupName) == "" {
		return nil, errors.ErrGroupNameRequired
	}

	group, err := service.store.GetByName(ctx, groupName)
	if err != nil {
		return nil, err
	}
	if group == nil {
		return nil, errors.ErrGroupNotFound
	}
	if group.PolicyID == nil {
		return nil, errors.ErrGroupNotLinkedToPolicy
	}

	unlinked, err := service.store.UnlinkFromPolicy(ctx, groupName)
	if err != nil {
		return nil, err
	}

	return mapGroupToOutputSchema(unlinked, nil), nil
}

// mapGroupToOutputSchema converts the Group entity format into the output
// schema provided by this service. Returns nil if the provided group is
// nil.
func mapGroupToOutputSchema(group *domain.Group, machineIDs []int64) *schemas.GroupOutputSchema {
	if group == nil {
		return nil
	}

	if machineIDs == nil {
		machineIDs = []int64{}
	}

	return &schemas.GroupOutputSchema{
		ID:         group.ID,
		Name:       group.Name,
		CreatedAt:  group.CreatedAt,
		MachineIDs: machineIDs,
		PolicyID:   group.PolicyID,
		PolicyName: group.PolicyName,
	}
}

// machineIDs lists the IDs of the machines linked to the provided group.
func (service *GroupService) machineIDs(ctx context.Context, groupID int64) ([]int64, error) {
	machines, err := service.machineStore.ListByGroup(ctx, groupID)
	if err != nil {
		return nil, err
	}

	ids := make([]int64, 0, len(machines))
	for _, machine := range machines {
		ids = append(ids, machine.ID)
	}

	return ids, nil
}
