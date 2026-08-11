package group

import (
	"context"
	"fmt"
	"strings"

	"dougdomingos.com/aegis/internal/domain"
	"dougdomingos.com/aegis/internal/errors"
	"dougdomingos.com/aegis/internal/schemas"
)

// GroupServiceInterface declares the methods provided by GroupService
// implementations.
type GroupServiceInterface interface {
	CreateGroup(ctx context.Context, p schemas.CreateGroupSchema) (*schemas.GroupOutputSchema, error)
	GetGroupByName(ctx context.Context, p schemas.GetGroupByNameSchema) (*schemas.GroupOutputSchema, error)
	ListAllGroups(ctx context.Context) ([]schemas.GroupOutputSchema, error)
	ChangeGroupName(ctx context.Context, p schemas.ChangeGroupNameSchema) (*schemas.GroupOutputSchema, error)
	RemoveGroup(ctx context.Context, p schemas.RemoveGroupSchema) error
}

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

	return mapGroupToOutputSchema(newGroup), nil
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

	return mapGroupToOutputSchema(existentGroup), nil
}

// ListAllGroups returns all the existent groups within the database.
func (service *GroupService) ListAllGroups(ctx context.Context) ([]schemas.GroupOutputSchema, error) {
	groups, err := service.store.List(ctx)
	if err != nil {
		return nil, err
	}

	output := make([]schemas.GroupOutputSchema, len(groups))
	for i, g := range groups {
		output[i] = *mapGroupToOutputSchema(&g)
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
	service.store.Update(ctx, *group)

	return mapGroupToOutputSchema(group), nil
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

// mapGroupToOutputSchema converts the Group entity format into the output
// schema provided by this service. Returns nil if the provided group is
// nil.
func mapGroupToOutputSchema(group *domain.Group) *schemas.GroupOutputSchema {
	if group == nil {
		return nil
	}

	return &schemas.GroupOutputSchema{
		ID:        group.ID,
		Name:      group.Name,
		CreatedAt: group.CreatedAt,
	}
}
