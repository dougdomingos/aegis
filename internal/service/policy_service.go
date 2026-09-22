package service

import (
	"context"
	"fmt"
	"strings"

	"dougdomingos.com/aegis/internal/domain"
	"dougdomingos.com/aegis/internal/errors"
	"dougdomingos.com/aegis/internal/schemas"
)

// PolicyService provides all operations needed to manage policies in the
// system.
type PolicyService struct {

	// store provides database operations related to policies
	store domain.PolicyStore
}

// NewPolicyService creates a new service instance with the provided store
// manager.
func NewPolicyService(store domain.PolicyStore) *PolicyService {
	return &PolicyService{store: store}
}

// CreatePolicy registers a new policy into the system. The name provided to
// this new policy must be unique, otherwise the operation fails.
func (service *PolicyService) CreatePolicy(ctx context.Context, payload schemas.CreatePolicySchema) (*schemas.PolicyOutputSchema, error) {
	if strings.TrimSpace(payload.Name) == "" {
		return nil, errors.ErrPolicyNameRequired
	}

	existentPolicy, err := service.store.GetByName(ctx, payload.Name)
	if err != nil {
		return nil, err
	}

	if existentPolicy != nil {
		return nil, errors.ErrPolicyNameAlreadyExists
	}

	newPolicy, err := service.store.Create(ctx, payload.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to register new policy: %w", err)
	}

	return mapPolicyToOutputSchema(newPolicy), nil
}

// GetPolicyByID retrieves a policy by its ID. If no policy matches the
// requested ID, it returns nil.
func (service *PolicyService) GetPolicyByID(ctx context.Context, payload schemas.GetPolicyByIDSchema) (*schemas.PolicyOutputSchema, error) {
	existentPolicy, err := service.store.GetByID(ctx, payload.ID)
	if err != nil {
		return nil, err
	}

	return mapPolicyToOutputSchema(existentPolicy), nil
}

// ListPolicies returns all the existent policies within the database.
func (service *PolicyService) ListPolicies(ctx context.Context) ([]schemas.PolicyOutputSchema, error) {
	policies, err := service.store.List(ctx)
	if err != nil {
		return nil, err
	}

	output := make([]schemas.PolicyOutputSchema, len(policies))
	for i, p := range policies {
		output[i] = *mapPolicyToOutputSchema(&p)
	}

	return output, nil
}

// UpdatePolicy renames an existent policy. The new name must be unique; if it
// matches the current name, the operation is a no-op that does not touch the
// policy version or updated_at timestamp.
func (service *PolicyService) UpdatePolicy(ctx context.Context, payload schemas.UpdatePolicySchema) (*schemas.PolicyOutputSchema, error) {
	if strings.TrimSpace(payload.Name) == "" {
		return nil, errors.ErrPolicyNameRequired
	}

	policy, err := service.store.GetByID(ctx, payload.ID)
	if err != nil {
		return nil, err
	}

	if policy == nil {
		return nil, errors.ErrPolicyNotFound
	}

	if policy.Name == payload.Name {
		return mapPolicyToOutputSchema(policy), nil
	}

	doesNameExist, err := service.store.Exists(ctx, payload.Name)
	if err != nil {
		return nil, err
	}

	if doesNameExist {
		return nil, errors.ErrPolicyNameAlreadyExists
	}

	policy.Name = payload.Name
	updatedPolicy, err := service.store.Update(ctx, *policy)
	if err != nil {
		return nil, err
	}

	return mapPolicyToOutputSchema(updatedPolicy), nil
}

// RemovePolicy deletes a policy from the database by its ID. If no policy
// matches the requested ID, it returns an error.
func (service *PolicyService) RemovePolicy(ctx context.Context, payload schemas.RemovePolicySchema) error {
	policy, err := service.store.GetByID(ctx, payload.ID)
	if err != nil {
		return err
	}

	if policy == nil {
		return errors.ErrPolicyNotFound
	}

	if err := service.store.Remove(ctx, payload.ID); err != nil {
		return fmt.Errorf("failed to remove policy %d: %v", payload.ID, err)
	}

	return nil
}

// mapPolicyToOutputSchema converts the Policy entity format into the output
// schema provided by this service. Returns nil if the provided policy is
// nil.
func mapPolicyToOutputSchema(policy *domain.Policy) *schemas.PolicyOutputSchema {
	if policy == nil {
		return nil
	}

	return &schemas.PolicyOutputSchema{
		ID:        policy.ID,
		Name:      policy.Name,
		Version:   policy.Version,
		CreatedAt: policy.CreatedAt,
		UpdatedAt: policy.UpdatedAt,
	}
}
