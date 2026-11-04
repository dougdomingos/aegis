package service

import (
	"context"
	"fmt"
	"strings"

	"dougdomingos.com/aegis/internal/common/errors"
	"dougdomingos.com/aegis/internal/common/schemas"
	"dougdomingos.com/aegis/internal/orchestrator/domain"
)

// PolicyService provides all operations needed to manage policies in the
// system.
type PolicyService struct {

	// store provides database operations related to policies
	store domain.PolicyStore

	// notifier propagates policy changes to the distribution pipeline. It may
	// be nil, disabling notifications.
	notifier PropagationNotifier
}

// NewPolicyService creates a new service instance with the provided store
// manager and propagation notifier. The notifier may be nil.
func NewPolicyService(store domain.PolicyStore, notifier PropagationNotifier) *PolicyService {
	return &PolicyService{store: store, notifier: notifier}
}

// CreatePolicy registers a new policy into the system. The name provided to
// this new policy must be unique, otherwise the operation fails.
func (service *PolicyService) CreatePolicy(ctx context.Context, payload schemas.CreatePolicySchema) (*schemas.PolicyOutputSchema, error) {
	if strings.TrimSpace(payload.Name) == "" {
		return nil, errors.ErrPolicyNameRequired
	}

	if err := validatePolicyType(payload.Type); err != nil {
		return nil, err
	}

	existentPolicy, err := service.store.GetByName(ctx, payload.Name)
	if err != nil {
		return nil, err
	}

	if existentPolicy != nil {
		return nil, errors.ErrPolicyNameAlreadyExists
	}

	newPolicy, err := service.store.Create(ctx, payload.Name, payload.Type)
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
// policy version or updated_at timestamp. The policy type is immutable and is
// therefore absent from the update contract, so it cannot be changed here.
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

	if service.notifier != nil {
		service.notifier.NotifyPolicy(ctx, policy.ID)
	}

	return mapPolicyToOutputSchema(updatedPolicy), nil
}

// RemovePolicy deletes a policy from the database by its ID. If no policy
// matches the requested ID, it returns an error. Machines of groups linked to
// the policy are resolved before the deletion, so a resolution failure aborts
// it; the revoke is enqueued only after the deletion succeeds.
func (service *PolicyService) RemovePolicy(ctx context.Context, payload schemas.RemovePolicySchema) error {
	policy, err := service.store.GetByID(ctx, payload.ID)
	if err != nil {
		return err
	}

	if policy == nil {
		return errors.ErrPolicyNotFound
	}

	var machineIDs []int64
	if service.notifier != nil {
		machineIDs, err = service.notifier.ResolvePolicyMachines(ctx, policy.ID)
		if err != nil {
			return fmt.Errorf("failed to resolve machines of policy %d: %w", policy.ID, err)
		}
	}

	if err := service.store.Remove(ctx, payload.ID); err != nil {
		return fmt.Errorf("failed to remove policy %d: %v", payload.ID, err)
	}

	if service.notifier != nil {
		service.notifier.RevokeMachines(ctx, machineIDs, policy)
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
		Type:      policy.Type,
		Version:   policy.Version,
		CreatedAt: policy.CreatedAt,
		UpdatedAt: policy.UpdatedAt,
	}
}

// validatePolicyType checks whether the provided policy type is informed and
// supported by the system.
func validatePolicyType(policyType domain.PolicyType) error {
	if strings.TrimSpace(string(policyType)) == "" {
		return errors.ErrPolicyTypeRequired
	}

	if policyType != domain.WhitelistPolicyType && policyType != domain.BlacklistPolicyType {
		return errors.ErrInvalidPolicyType
	}

	return nil
}
