package group

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"dougdomingos.com/aegis/internal/domain"
)

var (
	// ErrGroupNotFound is returned when the requested group is not found within
	// the database.
	ErrGroupNotFound = errors.New("requested group not found")

	// ErrNameRequired is returned when the name of a group is required but
	// not provided by the payload.
	ErrNameRequired = errors.New("field \"name\" is required")

	// ErrNameAlreadyExists is returned when the name used to create/update a
	// group is already in use by another group.
	ErrNameAlreadyExists = errors.New("provided name already exists")

	// ErrTargetGroupNameRequired is returned when the name of the targeted group
	// of a rename operation is not provided.
	ErrTargetGroupNameRequired = errors.New("must specify target group name")

	// ErrNewNameRequired is returns when the new name of a group in a rename
	// operation is not provided
	ErrNewNameRequired = errors.New("must specify new name for group")
)

// GroupService provides all operations needed to manage logical groups in the
// system.
type GroupService struct {

	// store provides database operations related to groups
	store domain.GroupStore
}

// NewGroupService creates a new service instance with the provided store
// manager.
func NewGroupService(store domain.GroupStore) *GroupService {
	return &GroupService{store: store}
}

// CreateGroup registers a new logical group into the system. The name provided to
// this new group must be unique, otherwise the operation fails.
func (service *GroupService) CreateGroup(ctx context.Context, payload CreateGroupSchema) (*GroupOutputSchema, error) {
	if strings.TrimSpace(payload.Name) == "" {
		return nil, ErrNameRequired
	}

	existentGroup, err := service.store.GetByName(ctx, payload.Name)
	if err != nil {
		return nil, err
	}

	if existentGroup != nil {
		return nil, ErrNameAlreadyExists
	}

	newGroup, err := service.store.Create(ctx, payload.Name)
	if err != nil {
		return nil, fmt.Errorf("Failed to register new group: %w", err)
	}

	return mapGroupToOutputSchema(newGroup), nil
}

// GetGroupByName retrieves a group by its name. If no group matches the
// requested name, it returns nil.
func (service *GroupService) GetGroupByName(ctx context.Context, payload GetGroupByNameSchema) (*GroupOutputSchema, error) {
	if strings.TrimSpace(payload.Name) == "" {
		return nil, ErrNameRequired
	}

	existentGroup, err := service.store.GetByName(ctx, payload.Name)
	if err != nil {
		return nil, err
	}

	return mapGroupToOutputSchema(existentGroup), nil
}

// ListAllGroups returns all the existent groups within the database.
func (service *GroupService) ListAllGroups(ctx context.Context) ([]GroupOutputSchema, error) {
	groups, err := service.store.List(ctx)
	if err != nil {
		return nil, err
	}

	output := make([]GroupOutputSchema, len(groups))
	for i, g := range groups {
		output[i] = *mapGroupToOutputSchema(&g)
	}

	return output, nil
}

func (service *GroupService) ChangeGroupName(ctx context.Context, payload ChangeGroupNameSchema) (*GroupOutputSchema, error) {
	if strings.TrimSpace(payload.TargetGroupName) == "" {
		return nil, ErrTargetGroupNameRequired
	}

	if strings.TrimSpace(payload.NewName) == "" {
		return nil, ErrNewNameRequired
	}

	doesGroupExists, err := service.store.Exists(ctx, payload.NewName)
	if err != nil {
		return nil, err
	}

	if doesGroupExists {
		return nil, ErrNameAlreadyExists
	}

	group, err := service.store.GetByName(ctx, payload.TargetGroupName)
	if err != nil {
		return nil, err
	}

	if group == nil {
		return nil, ErrGroupNotFound
	}

	group.Name = payload.NewName
	service.store.Update(ctx, *group)

	return mapGroupToOutputSchema(group), nil
}

// RemoveGroup deletes a group from the database by its name. If no group
// matches the requested name, it returns an error.
func (service *GroupService) RemoveGroup(ctx context.Context, payload RemoveGroup) error {
	if strings.TrimSpace(payload.Name) == "" {
		return ErrNameRequired
	}

	doesGroupExists, err := service.store.Exists(ctx, payload.Name)
	if err != nil {
		return err
	}

	if !doesGroupExists {
		return ErrGroupNotFound
	}

	if err := service.store.Remove(ctx, payload.Name); err != nil {
		return fmt.Errorf("failed to remove group %q: %v", payload.Name, err)
	}

	return nil
}

// mapGroupToOutputSchema converts the Group entity format into the output
// schema provided by this service. Returns nil if the provided group is
// nil.
func mapGroupToOutputSchema(group *domain.Group) *GroupOutputSchema {
	if group == nil {
		return nil
	}

	return &GroupOutputSchema{
		ID:        group.ID,
		Name:      group.Name,
		CreatedAt: group.CreatedAt,
	}
}
