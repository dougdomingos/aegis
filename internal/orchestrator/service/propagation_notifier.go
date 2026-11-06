package service

import (
	"context"

	"dougdomingos.com/aegis/internal/orchestrator/domain"
)

// PropagationNotifier decouples the domain services from rule distribution:
// mutation notifications are best effort and never fail the operation that
// triggered them, while the resolve methods are read-only and may return an
// error, aborting the destructive operation that asked for them.
type PropagationNotifier interface {

	// NotifyPolicy marks every machine of every group linked to the policy as
	// pending with a package intent.
	NotifyPolicy(ctx context.Context, policyID int64)

	// NotifyGroup marks every machine of the group as pending with a package
	// intent.
	NotifyGroup(ctx context.Context, groupID int64)

	// NotifyMachine marks the machine as pending with a package intent.
	NotifyMachine(ctx context.Context, machineID int64)

	// ResolvePolicyMachines returns the IDs of every machine that would be
	// affected by a change to the policy, without enqueuing anything.
	ResolvePolicyMachines(ctx context.Context, policyID int64) ([]int64, error)

	// ResolveGroupMachines returns the IDs of every machine of the group,
	// without enqueuing anything.
	ResolveGroupMachines(ctx context.Context, groupID int64) ([]int64, error)

	// RevokeMachines marks every listed machine as pending with a revoke
	// intent. The optional policy carries the identity of a policy being
	// removed, recorded in each audit row.
	RevokeMachines(ctx context.Context, machineIDs []int64, policy *domain.Policy)
}
