package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	commonErrors "dougdomingos.com/aegis/internal/common/errors"
	"dougdomingos.com/aegis/internal/common/propagation"
	"dougdomingos.com/aegis/internal/orchestrator/domain"
)

// sendTimeout bounds how long the worker waits for a single stream send, so
// one unresponsive machine cannot block the whole fleet.
const sendTimeout = 10 * time.Second

// MessageSender delivers one propagation message to a connected machine over
// its stream.
type MessageSender interface {
	Send(ctx context.Context, message propagation.Message) error
}

// propagationIntent is the queued desire for one machine: a package rebuilt
// from the current state, or an explicit revoke carrying optional policy
// metadata for the audit row.
type propagationIntent struct {
	kind   propagation.Kind
	policy *domain.Policy
}

// registeredSender couples a machine's sender with the connection ID that
// registered it, so a newer stream replaces the previous one safely.
type registeredSender struct {
	id     uint64
	sender MessageSender
}

// builtMessage couples the wire message with the policy metadata recorded in
// the audit row. For a revoke, message.Policy* is empty while the audit fields
// may still carry the policy being removed.
type builtMessage struct {
	message       propagation.Message
	policyID      int64
	policyName    string
	policyVersion int
	policyType    string
}

// PropagationService owns the distribution of policy snapshots to machines:
// it resolves fan-out, serializes deliveries in a single worker, records every
// sent message in the audit store, and applies acknowledgements.
type PropagationService struct {
	machines   domain.MachineStore
	groups     domain.GroupStore
	policies   domain.PolicyStore
	rules      domain.RuleStore
	store      domain.PropagationStore
	logger     *slog.Logger
	now        func() time.Time
	epoch      string
	mu         sync.Mutex
	pending    map[int64]propagationIntent
	senders    map[int64]registeredSender
	nextConnID uint64
	wake       chan struct{}
}

var _ PropagationNotifier = (*PropagationService)(nil)

// NewPropagationService builds a PropagationService over the provided stores.
// A nil logger discards every record.
func NewPropagationService(
	machines domain.MachineStore,
	groups domain.GroupStore,
	policies domain.PolicyStore,
	rules domain.RuleStore,
	store domain.PropagationStore,
	logger *slog.Logger,
	now func() time.Time,
) *PropagationService {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}

	return &PropagationService{
		machines: machines,
		groups:   groups,
		policies: policies,
		rules:    rules,
		store:    store,
		logger:   logger,
		now:      now,
		pending:  make(map[int64]propagationIntent),
		senders:  make(map[int64]registeredSender),
		wake:     make(chan struct{}, 1),
	}
}

// Start loads the database epoch and launches the propagation worker. It
// returns an error, without launching anything, if the epoch cannot be loaded.
func (service *PropagationService) Start(ctx context.Context) error {
	epoch, err := service.store.Epoch(ctx)
	if err != nil {
		return fmt.Errorf("failed to load propagation epoch: %w", err)
	}

	service.mu.Lock()
	service.epoch = epoch
	service.mu.Unlock()

	go service.run(ctx)
	return nil
}

// Register stores the sender for the machine, returning the connection ID that
// scopes a later Unregister. Unknown machines return ErrMachineNotFound.
func (service *PropagationService) Register(machineID int64, sender MessageSender) (uint64, error) {
	machine, err := service.machines.GetByID(context.Background(), machineID)
	if err != nil {
		return 0, fmt.Errorf("failed to verify machine %d for registration: %w", machineID, err)
	}

	if machine == nil {
		return 0, commonErrors.ErrMachineNotFound
	}

	service.mu.Lock()
	defer service.mu.Unlock()

	service.nextConnID++
	service.senders[machineID] = registeredSender{id: service.nextConnID, sender: sender}

	return service.nextConnID, nil
}

// Unregister removes the machine's sender only when the connection ID still
// matches the registered one, so an old handler's deferred call cannot evict a
// newer stream.
func (service *PropagationService) Unregister(machineID int64, connID uint64) {
	service.mu.Lock()
	defer service.mu.Unlock()

	if registered, exists := service.senders[machineID]; exists && registered.id == connID {
		delete(service.senders, machineID)
	}
}

// DeliverSnapshot queues a package intent for the machine, resolved against
// the current desired state by the worker.
func (service *PropagationService) DeliverSnapshot(ctx context.Context, machineID int64) {
	service.enqueue(machineID, propagationIntent{kind: propagation.KindPackage})
}

// NotifyPolicy queues a package intent for every machine of every group
// linked to the policy. Resolution failures are logged only.
func (service *PropagationService) NotifyPolicy(ctx context.Context, policyID int64) {
	machineIDs, err := service.ResolvePolicyMachines(ctx, policyID)
	if err != nil {
		service.logger.Error("failed to resolve machines of policy for propagation", "policy_id", policyID, "error", err)
		return
	}

	for _, machineID := range machineIDs {
		service.enqueue(machineID, propagationIntent{kind: propagation.KindPackage})
	}
}

// NotifyGroup queues a package intent for every machine of the group.
func (service *PropagationService) NotifyGroup(ctx context.Context, groupID int64) {
	machineIDs, err := service.ResolveGroupMachines(ctx, groupID)
	if err != nil {
		service.logger.Error("failed to resolve machines of group for propagation", "group_id", groupID, "error", err)
		return
	}

	for _, machineID := range machineIDs {
		service.enqueue(machineID, propagationIntent{kind: propagation.KindPackage})
	}
}

// NotifyMachine queues a package intent for the single machine.
func (service *PropagationService) NotifyMachine(ctx context.Context, machineID int64) {
	service.enqueue(machineID, propagationIntent{kind: propagation.KindPackage})
}

// ResolvePolicyMachines returns every machine affected by the policy without
// queueing anything, surfacing store errors.
func (service *PropagationService) ResolvePolicyMachines(ctx context.Context, policyID int64) ([]int64, error) {
	groups, err := service.groups.ListByPolicy(ctx, policyID)
	if err != nil {
		return nil, fmt.Errorf("failed to list groups of policy %d: %w", policyID, err)
	}

	machineIDs := make([]int64, 0)
	for _, group := range groups {
		ids, err := service.machines.ListByGroup(ctx, group.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to list machines of group %d: %w", group.ID, err)
		}

		for _, machine := range ids {
			machineIDs = append(machineIDs, machine.ID)
		}
	}

	return machineIDs, nil
}

// ResolveGroupMachines returns every machine of the group without queueing
// anything, surfacing store errors.
func (service *PropagationService) ResolveGroupMachines(ctx context.Context, groupID int64) ([]int64, error) {
	machines, err := service.machines.ListByGroup(ctx, groupID)
	if err != nil {
		return nil, fmt.Errorf("failed to list machines of group %d: %w", groupID, err)
	}

	machineIDs := make([]int64, 0, len(machines))
	for _, machine := range machines {
		machineIDs = append(machineIDs, machine.ID)
	}

	return machineIDs, nil
}

// RevokeMachines queues a revoke intent for every listed machine. The optional
// policy carries the identity of a policy being removed, recorded in each
// audit row; the wire message stays a bare revoke.
func (service *PropagationService) RevokeMachines(ctx context.Context, machineIDs []int64, policy *domain.Policy) {
	var carried *domain.Policy
	if policy != nil {
		clone := *policy
		carried = &clone
	}

	for _, machineID := range machineIDs {
		service.enqueue(machineID, propagationIntent{kind: propagation.KindRevoke, policy: carried})
	}
}

// Acknowledge applies the Sentinel's response to the matching audit row.
// Unknown statuses are ignored.
func (service *PropagationService) Acknowledge(ctx context.Context, machineID int64, ack propagation.Ack) {
	var status domain.PropagationStatus

	switch ack.Status {
	case propagation.AckAcked:
		status = domain.PropagationAcked
	case propagation.AckNacked:
		status = domain.PropagationNacked
	default:
		service.logger.Warn("ignoring propagation acknowledgement with unknown status",
			"machine_id", machineID, "sequence", ack.Sequence, "status", ack.Status)
		return
	}

	if err := service.store.Acknowledge(ctx, machineID, ack.Sequence, status, ack.Reason, service.now()); err != nil {
		service.logger.Error("failed to record propagation acknowledgement",
			"machine_id", machineID, "sequence", ack.Sequence, "error", err)
	}
}

// enqueue replaces the pending intent of the machine, latest wins, and signals
// the worker without ever blocking.
func (service *PropagationService) enqueue(machineID int64, intent propagationIntent) {
	service.mu.Lock()
	service.pending[machineID] = intent
	service.mu.Unlock()

	select {
	case service.wake <- struct{}{}:
	default:
	}
}

// run is the single-threaded worker: it drains the pending intents on every
// wake and processes them one at a time, serializing sequence assignment and
// sends per machine.
func (service *PropagationService) run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-service.wake:
		}

		service.mu.Lock()
		snapshot := service.pending
		service.pending = make(map[int64]propagationIntent)
		service.mu.Unlock()

		for machineID, intent := range snapshot {
			service.dispatch(ctx, machineID, intent)
		}
	}
}

// dispatch builds, records, and sends the message for one pending intent. A
// machine without a registered sender is skipped without an audit row, and a
// message is never sent before its row is marked SENT.
func (service *PropagationService) dispatch(ctx context.Context, machineID int64, intent propagationIntent) {
	service.mu.Lock()
	registered, exists := service.senders[machineID]
	service.mu.Unlock()

	if !exists {
		return
	}

	built, err := service.buildMessage(ctx, machineID, intent)
	if err != nil {
		service.logger.Error("failed to build propagation message", "machine_id", machineID, "error", err)
		return
	}

	if built == nil {
		return
	}

	row := domain.Propagation{
		MachineID:     machineID,
		Kind:          domain.PropagationKind(built.message.Kind),
		PolicyName:    built.policyName,
		PolicyVersion: built.policyVersion,
		PolicyType:    built.policyType,
	}

	if built.policyID > 0 {
		policyID := built.policyID
		row.PolicyID = &policyID
	}

	created, err := service.store.Create(ctx, row, func(sequence int64) (string, error) {
		built.message.Sequence = sequence
		payload, err := json.Marshal(built.message)
		return string(payload), err
	})
	if err != nil {
		service.logger.Error("failed to record propagation", "machine_id", machineID, "error", err)
		return
	}

	if err := service.store.MarkSent(ctx, machineID, created.Sequence, service.now()); err != nil {
		service.logger.Error("failed to mark propagation as sent",
			"machine_id", machineID, "sequence", created.Sequence, "error", err)
		return
	}

	sendCtx, cancel := context.WithTimeout(ctx, sendTimeout)
	defer cancel()

	if err := registered.sender.Send(sendCtx, built.message); err != nil {
		service.logger.Warn("failed to send propagation message",
			"machine_id", machineID, "sequence", created.Sequence, "error", err)

		if markErr := service.store.MarkFailed(ctx, machineID, created.Sequence, err.Error()); markErr != nil {
			service.logger.Error("failed to mark propagation as failed",
				"machine_id", machineID, "sequence", created.Sequence, "error", markErr)
		}
	}
}

// buildMessage resolves the message for one intent. A package intent rebuilds
// the machine's full desired state, degrading to a revoke whenever any link
// is missing; a nil message with nil error means the machine was removed.
func (service *PropagationService) buildMessage(ctx context.Context, machineID int64, intent propagationIntent) (*builtMessage, error) {
	epoch := service.currentEpoch()

	if intent.kind == propagation.KindRevoke {
		return revokeBuilt(epoch, machineID, intent.policy), nil
	}

	machine, err := service.machines.GetByID(ctx, machineID)
	if err != nil {
		return nil, fmt.Errorf("failed to load machine %d: %w", machineID, err)
	}

	if machine == nil {
		return nil, nil
	}

	if machine.GroupID == nil {
		return revokeBuilt(epoch, machineID, nil), nil
	}

	group, err := service.groups.GetByID(ctx, *machine.GroupID)
	if err != nil {
		return nil, fmt.Errorf("failed to load group %d of machine %d: %w", *machine.GroupID, machineID, err)
	}

	if group == nil || group.PolicyID == nil {
		return revokeBuilt(epoch, machineID, nil), nil
	}

	policy, err := service.policies.GetByID(ctx, *group.PolicyID)
	if err != nil {
		return nil, fmt.Errorf("failed to load policy %d of machine %d: %w", *group.PolicyID, machineID, err)
	}

	if policy == nil {
		return revokeBuilt(epoch, machineID, nil), nil
	}

	policyRules, err := service.rules.List(ctx, domain.RuleFilter{PolicyID: policy.ID})
	if err != nil {
		return nil, fmt.Errorf("failed to load rules of policy %d: %w", policy.ID, err)
	}

	if len(policyRules) == 0 {
		return revokeBuilt(epoch, machineID, nil), nil
	}

	rules := make([]propagation.Rule, 0, len(policyRules))
	for _, rule := range policyRules {
		rules = append(rules, propagation.Rule{
			Type:     string(rule.Type),
			Value:    rule.Value,
			Protocol: rule.Protocol,
			Port:     rule.Port,
		})
	}

	return &builtMessage{
		message: propagation.Message{
			Epoch:         epoch,
			MachineID:     machineID,
			Kind:          propagation.KindPackage,
			PolicyID:      policy.ID,
			PolicyName:    policy.Name,
			PolicyVersion: policy.Version,
			PolicyType:    string(policy.Type),
			Rules:         rules,
		},
		policyID:      policy.ID,
		policyName:    policy.Name,
		policyVersion: policy.Version,
		policyType:    string(policy.Type),
	}, nil
}

// currentEpoch returns the epoch cached by Start.
func (service *PropagationService) currentEpoch() string {
	service.mu.Lock()
	defer service.mu.Unlock()

	return service.epoch
}

// revokeBuilt assembles a bare revoke message, copying the optional policy
// identity into the audit fields.
func revokeBuilt(epoch string, machineID int64, policy *domain.Policy) *builtMessage {
	built := &builtMessage{
		message: propagation.Message{
			Epoch:     epoch,
			MachineID: machineID,
			Kind:      propagation.KindRevoke,
		},
	}

	if policy != nil {
		built.policyID = policy.ID
		built.policyName = policy.Name
		built.policyVersion = policy.Version
		built.policyType = string(policy.Type)
	}

	return built
}
