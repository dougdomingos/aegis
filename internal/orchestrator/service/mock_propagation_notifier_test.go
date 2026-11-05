package service_test

import (
	"context"

	"dougdomingos.com/aegis/internal/orchestrator/domain"
	"dougdomingos.com/aegis/internal/orchestrator/service"
)

// MockPropagationNotifier provides an in-memory implementation of
// service.PropagationNotifier that records every call, in order, into an
// optional shared log. Mock stores append their own events (e.g., "remove")
// to the same log, so delete-ordering scenarios can assert the full
// resolve → remove → revoke sequence.
//
// Each method also provides an injectable error field for testing scenarios
// where the notifier provider fails, and the resolve methods return the
// configured machine IDs.
type MockPropagationNotifier struct {
	Log *[]string

	GroupMachineIDs          []int64
	PolicyMachineIDs         []int64
	ResolveGroupMachinesErr  error
	ResolvePolicyMachinesErr error

	RevokedMachineIDs []int64
	RevokedPolicy     *domain.Policy
}

var _ service.PropagationNotifier = (*MockPropagationNotifier)(nil)

// NotifyPolicy records the notification.
func (mock *MockPropagationNotifier) NotifyPolicy(ctx context.Context, policyID int64) {
	mock.record("notify_policy")
}

// NotifyGroup records the notification.
func (mock *MockPropagationNotifier) NotifyGroup(ctx context.Context, groupID int64) {
	mock.record("notify_group")
}

// NotifyMachine records the notification.
func (mock *MockPropagationNotifier) NotifyMachine(ctx context.Context, machineID int64) {
	mock.record("notify_machine")
}

// ResolvePolicyMachines records the resolution and returns the configured
// machine IDs or the injected error.
func (mock *MockPropagationNotifier) ResolvePolicyMachines(ctx context.Context, policyID int64) ([]int64, error) {
	mock.record("resolve")
	if mock.ResolvePolicyMachinesErr != nil {
		return nil, mock.ResolvePolicyMachinesErr
	}

	return mock.PolicyMachineIDs, nil
}

// ResolveGroupMachines records the resolution and returns the configured
// machine IDs or the injected error.
func (mock *MockPropagationNotifier) ResolveGroupMachines(ctx context.Context, groupID int64) ([]int64, error) {
	mock.record("resolve")
	if mock.ResolveGroupMachinesErr != nil {
		return nil, mock.ResolveGroupMachinesErr
	}

	return mock.GroupMachineIDs, nil
}

// RevokeMachines records the revocation along with its arguments.
func (mock *MockPropagationNotifier) RevokeMachines(ctx context.Context, machineIDs []int64, policy *domain.Policy) {
	mock.record("revoke")
	mock.RevokedMachineIDs = machineIDs
	mock.RevokedPolicy = policy
}

func (mock *MockPropagationNotifier) record(call string) {
	if mock.Log == nil {
		return
	}

	*mock.Log = append(*mock.Log, call)
}

// logEquals reports whether the shared log holds exactly the provided calls,
// in order.
func logEquals(log *[]string, calls ...string) bool {
	if len(*log) != len(calls) {
		return false
	}

	for i, call := range calls {
		if (*log)[i] != call {
			return false
		}
	}

	return true
}
