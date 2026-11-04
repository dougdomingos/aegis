package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"dougdomingos.com/aegis/internal/common/propagation"
	"dougdomingos.com/aegis/internal/orchestrator/domain"
	"dougdomingos.com/aegis/internal/orchestrator/service"
)

// propagationWaitTimeout bounds every asynchronous assertion in this suite.
const propagationWaitTimeout = 5 * time.Second

func TestPropagationService_Start_WithEpochError_ReturnsErrorAndDoesNotLaunch(t *testing.T) {
	fixture := arrangePropagationServiceTest(t)
	fixture.propagations.EpochErr = errors.New("epoch unavailable")

	if err := fixture.service.Start(fixture.ctx); err == nil {
		t.Fatal("Start() = nil, want the epoch error")
	}

	machine, _ := fixture.completeMachine("10.0.0.1")
	sender := fixture.register(machine.ID)
	fixture.service.NotifyMachine(fixture.ctx, machine.ID)

	if got := fixture.propagations.CreateCallCount(); got != 0 {
		t.Errorf("CreateCallCount() = %d, want 0: the worker must not launch after a failed Start", got)
	}
	assertNoSend(t, sender.sent)
}

func TestPropagationService_NotifyPolicy_WithLinkedGroups_EnqueuesEveryMachine(t *testing.T) {
	fixture := arrangePropagationServiceTest(t)

	policy := fixture.addPolicy("shared-policy")
	group := fixture.addGroup("lab-group")
	fixture.linkPolicy(group, policy)

	first := fixture.addMachine("10.0.0.1")
	second := fixture.addMachine("10.0.0.2")
	fixture.linkMachine(first, group)
	fixture.linkMachine(second, group)
	fixture.addRule(policy.ID, "example.com")

	unlinked := fixture.addMachine("10.0.0.3")

	senders := map[int64]*fakeSender{
		first.ID:    fixture.register(first.ID),
		second.ID:   fixture.register(second.ID),
		unlinked.ID: fixture.register(unlinked.ID),
	}
	fixture.start()

	fixture.service.NotifyPolicy(fixture.ctx, policy.ID)

	for _, machineID := range []int64{first.ID, second.ID} {
		message := waitForSend(t, senders[machineID].sent)
		if message.Kind != propagation.KindPackage {
			t.Errorf("machine %d message kind = %q, want %q", machineID, message.Kind, propagation.KindPackage)
		}
	}

	assertNoSend(t, senders[unlinked.ID].sent)
	if got := fixture.propagations.CreateCallCount(); got != 2 {
		t.Errorf("CreateCallCount() = %d, want 2 (only machines of linked groups)", got)
	}
}

func TestPropagationService_NotifyGroup_WithMachines_EnqueuesEveryMachine(t *testing.T) {
	fixture := arrangePropagationServiceTest(t)

	group := fixture.addGroup("lab-group")
	policy := fixture.addPolicy("policy-a")
	fixture.linkPolicy(group, policy)

	first := fixture.addMachine("10.0.0.1")
	second := fixture.addMachine("10.0.0.2")
	fixture.linkMachine(first, group)
	fixture.linkMachine(second, group)
	fixture.addRule(policy.ID, "example.com")

	senders := map[int64]*fakeSender{
		first.ID:  fixture.register(first.ID),
		second.ID: fixture.register(second.ID),
	}
	fixture.start()

	fixture.service.NotifyGroup(fixture.ctx, group.ID)

	for _, machineID := range []int64{first.ID, second.ID} {
		message := waitForSend(t, senders[machineID].sent)
		if message.Kind != propagation.KindPackage {
			t.Errorf("machine %d message kind = %q, want %q", machineID, message.Kind, propagation.KindPackage)
		}
	}
}

func TestPropagationService_NotifyMachine_WithMachine_EnqueuesOnlyThatMachine(t *testing.T) {
	fixture := arrangePropagationServiceTest(t)

	target, _ := fixture.completeMachine("10.0.0.1")
	other, _ := fixture.completeMachine("10.0.0.2")

	targetSender := fixture.register(target.ID)
	otherSender := fixture.register(other.ID)
	fixture.start()

	fixture.service.NotifyMachine(fixture.ctx, target.ID)

	message := waitForSend(t, targetSender.sent)
	if message.MachineID != target.ID {
		t.Errorf("MachineID = %d, want %d", message.MachineID, target.ID)
	}
	if message.Kind != propagation.KindPackage {
		t.Errorf("Kind = %q, want %q", message.Kind, propagation.KindPackage)
	}

	assertNoSend(t, otherSender.sent)
	if got := fixture.propagations.CreateCallCount(); got != 1 {
		t.Errorf("CreateCallCount() = %d, want 1", got)
	}
}

func TestPropagationService_LatestIntentWins_PackageThenRevoke(t *testing.T) {
	fixture := arrangePropagationServiceTest(t)

	machine, _ := fixture.completeMachine("10.0.0.1")
	sender := fixture.register(machine.ID)
	fixture.start()

	fixture.service.NotifyMachine(fixture.ctx, machine.ID)
	fixture.service.RevokeMachines(fixture.ctx, []int64{machine.ID}, nil)

	message := waitForSend(t, sender.sent)
	if message.Kind != propagation.KindRevoke {
		t.Errorf("Kind = %q, want %q", message.Kind, propagation.KindRevoke)
	}

	assertNoSend(t, sender.sent)
	if got := fixture.propagations.CreateCallCount(); got != 1 {
		t.Errorf("CreateCallCount() = %d, want 1 (the replaced intent must not be sent)", got)
	}
}

func TestPropagationService_LatestIntentWins_RevokeThenPackage(t *testing.T) {
	fixture := arrangePropagationServiceTest(t)

	machine, policy := fixture.completeMachine("10.0.0.1")
	sender := fixture.register(machine.ID)
	fixture.start()

	fixture.service.RevokeMachines(fixture.ctx, []int64{machine.ID}, nil)
	fixture.service.NotifyMachine(fixture.ctx, machine.ID)

	message := waitForSend(t, sender.sent)
	if message.Kind != propagation.KindPackage {
		t.Errorf("Kind = %q, want %q", message.Kind, propagation.KindPackage)
	}
	if message.PolicyID != policy.ID {
		t.Errorf("PolicyID = %d, want %d", message.PolicyID, policy.ID)
	}

	assertNoSend(t, sender.sent)
	if got := fixture.propagations.CreateCallCount(); got != 1 {
		t.Errorf("CreateCallCount() = %d, want 1 (the replaced intent must not be sent)", got)
	}
}

func TestPropagationService_DeliverSnapshot_WithCompleteLinks_SendsPackage(t *testing.T) {
	fixture := arrangePropagationServiceTest(t)

	machine, policy := fixture.completeMachine("10.0.0.1")
	sender := fixture.register(machine.ID)
	fixture.start()

	fixture.service.DeliverSnapshot(fixture.ctx, machine.ID)

	message := waitForSend(t, sender.sent)

	if message.Kind != propagation.KindPackage {
		t.Errorf("Kind = %q, want %q", message.Kind, propagation.KindPackage)
	}
	if message.Epoch != fixture.propagations.EpochValue {
		t.Errorf("Epoch = %q, want %q", message.Epoch, fixture.propagations.EpochValue)
	}
	if message.Sequence != 1 {
		t.Errorf("Sequence = %d, want 1", message.Sequence)
	}
	if message.MachineID != machine.ID {
		t.Errorf("MachineID = %d, want %d", message.MachineID, machine.ID)
	}
	if message.PolicyID != policy.ID {
		t.Errorf("PolicyID = %d, want %d", message.PolicyID, policy.ID)
	}
	if message.PolicyName != "policy-10.0.0.1" {
		t.Errorf("PolicyName = %q, want %q", message.PolicyName, "policy-10.0.0.1")
	}
	if message.PolicyVersion != 1 {
		t.Errorf("PolicyVersion = %d, want 1", message.PolicyVersion)
	}
	if message.PolicyType != propagation.PolicyTypeWhitelist {
		t.Errorf("PolicyType = %q, want %q", message.PolicyType, propagation.PolicyTypeWhitelist)
	}

	if len(message.Rules) != 2 {
		t.Fatalf("Rules = %d entries, want 2", len(message.Rules))
	}

	var domainRule, ipRule *propagation.Rule
	for i := range message.Rules {
		switch message.Rules[i].Value {
		case "example-10.0.0.1.com":
			domainRule = &message.Rules[i]
		case "10.0.0.1":
			ipRule = &message.Rules[i]
		}
	}

	if domainRule == nil || domainRule.Type != "DOMAIN" || domainRule.Protocol != nil || domainRule.Port != nil {
		t.Errorf("domain rule = %+v, want DOMAIN without protocol or port", domainRule)
	}
	if ipRule == nil || ipRule.Type != "IP" || ipRule.Protocol == nil || *ipRule.Protocol != "TCP" {
		t.Errorf("IP rule = %+v, want IP with protocol TCP", ipRule)
	} else if ipRule.Port == nil || *ipRule.Port != 443 {
		t.Errorf("IP rule port = %v, want 443", ipRule.Port)
	}
}

func TestPropagationService_DeliverSnapshot_WithoutGroup_SendsRevoke(t *testing.T) {
	fixture := arrangePropagationServiceTest(t)

	machine := fixture.addMachine("10.0.0.1")
	sender := fixture.register(machine.ID)
	fixture.start()

	fixture.service.DeliverSnapshot(fixture.ctx, machine.ID)

	message := waitForSend(t, sender.sent)
	if message.Kind != propagation.KindRevoke {
		t.Errorf("Kind = %q, want %q", message.Kind, propagation.KindRevoke)
	}
}

func TestPropagationService_DeliverSnapshot_WithoutPolicy_SendsRevoke(t *testing.T) {
	fixture := arrangePropagationServiceTest(t)

	machine := fixture.addMachine("10.0.0.1")
	group := fixture.addGroup("lab-group")
	fixture.linkMachine(machine, group)
	sender := fixture.register(machine.ID)
	fixture.start()

	fixture.service.DeliverSnapshot(fixture.ctx, machine.ID)

	message := waitForSend(t, sender.sent)
	if message.Kind != propagation.KindRevoke {
		t.Errorf("Kind = %q, want %q", message.Kind, propagation.KindRevoke)
	}
}

func TestPropagationService_DeliverSnapshot_WithEmptyRules_SendsRevoke(t *testing.T) {
	fixture := arrangePropagationServiceTest(t)

	machine := fixture.addMachine("10.0.0.1")
	group := fixture.addGroup("lab-group")
	policy := fixture.addPolicy("empty-policy")
	fixture.linkMachine(machine, group)
	fixture.linkPolicy(group, policy)
	sender := fixture.register(machine.ID)
	fixture.start()

	fixture.service.DeliverSnapshot(fixture.ctx, machine.ID)

	message := waitForSend(t, sender.sent)
	if message.Kind != propagation.KindRevoke {
		t.Errorf("Kind = %q, want %q", message.Kind, propagation.KindRevoke)
	}
}

func TestPropagationService_DeliverSnapshot_WhenSendSucceeds_MarksSent(t *testing.T) {
	fixture := arrangePropagationServiceTest(t)

	machine, _ := fixture.completeMachine("10.0.0.1")
	sender := fixture.register(machine.ID)
	fixture.start()

	fixture.service.DeliverSnapshot(fixture.ctx, machine.ID)
	waitForSend(t, sender.sent)

	waitForCondition(t, func() bool {
		row, exists := fixture.propagations.RowAt(machine.ID, 1)
		return exists && row.Status == domain.PropagationSent
	}, propagationWaitTimeout)

	row, _ := fixture.propagations.RowAt(machine.ID, 1)
	if row.SentAt == nil {
		t.Error("SentAt = nil, want the orchestrator's send timestamp")
	}
	if row.Payload == "" {
		t.Error("Payload = empty, want the serialized wire message")
	}
}

func TestPropagationService_DeliverSnapshot_WithoutSender_DoesNotCreateRow(t *testing.T) {
	fixture := arrangePropagationServiceTest(t)

	offline, _ := fixture.completeMachine("10.0.0.1")
	control, _ := fixture.completeMachine("10.0.0.2")
	sender := fixture.register(control.ID)
	fixture.start()

	fixture.service.DeliverSnapshot(fixture.ctx, offline.ID)
	fixture.service.DeliverSnapshot(fixture.ctx, control.ID)
	waitForSend(t, sender.sent)

	if got := fixture.propagations.CreateCallCount(); got != 1 {
		t.Errorf("CreateCallCount() = %d, want 1 (only the connected machine)", got)
	}
	if _, exists := fixture.propagations.RowAt(offline.ID, 1); exists {
		t.Error("RowAt(offline) exists, want no audit row for an offline machine")
	}
}

func TestPropagationService_DeliverSnapshot_WhenMarkSentFails_DoesNotSend(t *testing.T) {
	fixture := arrangePropagationServiceTest(t)

	fixture.propagations.MarkSentErr = errors.New("store unavailable")
	machine, _ := fixture.completeMachine("10.0.0.1")
	sender := fixture.register(machine.ID)
	fixture.start()

	fixture.service.DeliverSnapshot(fixture.ctx, machine.ID)

	waitForCondition(t, func() bool {
		return fixture.propagations.CreateCallCount() == 1
	}, propagationWaitTimeout)

	assertNoSend(t, sender.sent)
	if row, exists := fixture.propagations.RowAt(machine.ID, 1); !exists {
		t.Fatal("RowAt() missing, want the created row even when MarkSent fails")
	} else if row.Status != domain.PropagationPending {
		t.Errorf("Status = %q, want %q (never send without a SENT row)", row.Status, domain.PropagationPending)
	}
}

func TestPropagationService_DeliverSnapshot_WhenSendFails_MarksFailed(t *testing.T) {
	fixture := arrangePropagationServiceTest(t)

	machine, _ := fixture.completeMachine("10.0.0.1")
	sender := fixture.register(machine.ID)
	sender.Err = errors.New("stream closed")
	fixture.start()

	fixture.service.DeliverSnapshot(fixture.ctx, machine.ID)

	waitForCondition(t, func() bool {
		row, exists := fixture.propagations.RowAt(machine.ID, 1)
		return exists && row.Status == domain.PropagationFailed
	}, propagationWaitTimeout)

	row, _ := fixture.propagations.RowAt(machine.ID, 1)
	if row.NackReason != "stream closed" {
		t.Errorf("NackReason = %q, want %q", row.NackReason, "stream closed")
	}
}

func TestPropagationService_Register_AfterReconnect_OldUnregisterIsNoop(t *testing.T) {
	fixture := arrangePropagationServiceTest(t)

	machine, _ := fixture.completeMachine("10.0.0.1")
	first, second := newFakeSender(), newFakeSender()

	firstID, err := fixture.service.Register(machine.ID, first)
	if err != nil {
		t.Fatalf("Register() = %v, want nil", err)
	}
	secondID, err := fixture.service.Register(machine.ID, second)
	if err != nil {
		t.Fatalf("Register() = %v, want nil", err)
	}
	if firstID == secondID {
		t.Fatalf("Register() reuse of connection ID %d, want distinct identifiers", firstID)
	}

	fixture.service.Unregister(machine.ID, firstID)
	fixture.start()

	fixture.service.DeliverSnapshot(fixture.ctx, machine.ID)

	message := waitForSend(t, second.sent)
	if message.MachineID != machine.ID {
		t.Errorf("MachineID = %d, want %d", message.MachineID, machine.ID)
	}
	assertNoSend(t, first.sent)
}

func TestPropagationService_Acknowledge_WithForeignSequence_LeavesRowsUntouched(t *testing.T) {
	fixture := arrangePropagationServiceTest(t)

	machine, _ := fixture.completeMachine("10.0.0.1")
	sender := fixture.register(machine.ID)
	fixture.start()

	fixture.service.DeliverSnapshot(fixture.ctx, machine.ID)
	waitForSend(t, sender.sent)
	fixture.service.DeliverSnapshot(fixture.ctx, machine.ID)
	waitForSend(t, sender.sent)

	fixture.service.Acknowledge(fixture.ctx, machine.ID, propagation.Ack{Sequence: 99, Status: propagation.AckAcked})
	fixture.service.Acknowledge(fixture.ctx, 777, propagation.Ack{Sequence: 1, Status: propagation.AckAcked})

	for _, sequence := range []int64{1, 2} {
		row, exists := fixture.propagations.RowAt(machine.ID, sequence)
		if !exists {
			t.Fatalf("RowAt(%d) missing, want the delivered row", sequence)
		}
		if row.Status != domain.PropagationSent {
			t.Errorf("sequence %d status = %q, want %q", sequence, row.Status, domain.PropagationSent)
		}
		if row.AckedAt != nil {
			t.Errorf("sequence %d AckedAt = %v, want nil", sequence, row.AckedAt)
		}
	}
}

func TestPropagationService_RevokeMachines_WithPolicy_RecordsPolicyMetadata(t *testing.T) {
	fixture := arrangePropagationServiceTest(t)

	machine := fixture.addMachine("10.0.0.1")
	policy := fixture.addPolicy("doomed-policy")
	sender := fixture.register(machine.ID)
	fixture.start()

	fixture.service.RevokeMachines(fixture.ctx, []int64{machine.ID}, policy)

	message := waitForSend(t, sender.sent)
	if message.Kind != propagation.KindRevoke {
		t.Errorf("Kind = %q, want %q", message.Kind, propagation.KindRevoke)
	}
	if message.PolicyID != 0 || message.PolicyName != "" || message.PolicyType != "" {
		t.Errorf("wire policy fields = %+v, want a bare revoke", message)
	}

	row, exists := fixture.propagations.RowAt(machine.ID, 1)
	if !exists {
		t.Fatal("RowAt() missing, want the revoke audit row")
	}
	if row.Kind != domain.PropagationRevoke {
		t.Errorf("row Kind = %q, want %q", row.Kind, domain.PropagationRevoke)
	}
	if row.PolicyID == nil || *row.PolicyID != policy.ID {
		t.Errorf("row PolicyID = %v, want %d", row.PolicyID, policy.ID)
	}
	if row.PolicyName != "doomed-policy" {
		t.Errorf("row PolicyName = %q, want %q", row.PolicyName, "doomed-policy")
	}
	if row.PolicyVersion != 1 {
		t.Errorf("row PolicyVersion = %d, want 1", row.PolicyVersion)
	}
	if row.PolicyType != propagation.PolicyTypeWhitelist {
		t.Errorf("row PolicyType = %q, want %q", row.PolicyType, propagation.PolicyTypeWhitelist)
	}
}

// ============================================================================
// Fixtures and helpers
// ============================================================================

// fakeSender is a service.MessageSender that records delivered messages and
// can be configured to fail.
type fakeSender struct {
	sent chan propagation.Message
	Err  error
}

// newFakeSender builds a fakeSender with a buffered message channel.
func newFakeSender() *fakeSender {
	return &fakeSender{sent: make(chan propagation.Message, 8)}
}

// Send records the message, or returns the injected error.
func (sender *fakeSender) Send(ctx context.Context, message propagation.Message) error {
	if sender.Err != nil {
		return sender.Err
	}

	sender.sent <- message
	return nil
}

// propagationFixture wires the propagation service over mock stores.
type propagationFixture struct {
	t            *testing.T
	service      *service.PropagationService
	machines     *MockMachineStore
	groups       *MockGroupStore
	policies     *MockPolicyStore
	rules        *MockRuleStore
	propagations *MockPropagationStore
	ctx          context.Context
}

// arrangePropagationServiceTest builds a propagation service over fresh mock
// stores, cancelling its context on cleanup.
func arrangePropagationServiceTest(t *testing.T) *propagationFixture {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	fixture := &propagationFixture{
		t:            t,
		machines:     NewMockMachineStore(),
		groups:       NewMockGroupStore(),
		policies:     NewMockPolicyStore(),
		rules:        NewMockRuleStore(),
		propagations: NewMockPropagationStore(),
		ctx:          ctx,
	}

	fixture.service = service.NewPropagationService(
		fixture.machines, fixture.groups, fixture.policies, fixture.rules, fixture.propagations, nil, time.Now)

	return fixture
}

// start launches the service worker, failing the test on error.
func (fixture *propagationFixture) start() {
	fixture.t.Helper()

	if err := fixture.service.Start(fixture.ctx); err != nil {
		fixture.t.Fatalf("Start() = %v, want nil", err)
	}
}

// register adds a fake sender for the machine, failing the test on error.
func (fixture *propagationFixture) register(machineID int64) *fakeSender {
	fixture.t.Helper()

	sender := newFakeSender()
	if _, err := fixture.service.Register(machineID, sender); err != nil {
		fixture.t.Fatalf("Register(%d) = %v, want nil", machineID, err)
	}

	return sender
}

// addMachine upserts a disconnected machine and returns it.
func (fixture *propagationFixture) addMachine(ip string) *domain.Machine {
	fixture.t.Helper()

	machine, err := fixture.machines.Upsert(fixture.ctx, domain.Machine{IP: ip, Name: "machine-" + ip})
	if err != nil {
		fixture.t.Fatalf("Upsert() = %v, want nil", err)
	}

	return machine
}

// addPolicy creates a whitelist policy and returns it.
func (fixture *propagationFixture) addPolicy(name string) *domain.Policy {
	fixture.t.Helper()

	policy, err := fixture.policies.Create(fixture.ctx, name, domain.WhitelistPolicyType)
	if err != nil {
		fixture.t.Fatalf("Create(%q) = %v, want nil", name, err)
	}

	return policy
}

// addGroup creates a group and returns it.
func (fixture *propagationFixture) addGroup(name string) *domain.Group {
	fixture.t.Helper()

	group, err := fixture.groups.Create(fixture.ctx, name)
	if err != nil {
		fixture.t.Fatalf("Create(%q) = %v, want nil", name, err)
	}

	return group
}

// linkPolicy associates the group with the policy.
func (fixture *propagationFixture) linkPolicy(group *domain.Group, policy *domain.Policy) {
	fixture.t.Helper()

	if _, err := fixture.groups.LinkToPolicy(fixture.ctx, group.Name, policy.ID); err != nil {
		fixture.t.Fatalf("LinkToPolicy() = %v, want nil", err)
	}
}

// linkMachine associates the machine with the group.
func (fixture *propagationFixture) linkMachine(machine *domain.Machine, group *domain.Group) {
	fixture.t.Helper()

	if _, err := fixture.machines.LinkToGroup(fixture.ctx, machine.ID, group.ID); err != nil {
		fixture.t.Fatalf("LinkToGroup() = %v, want nil", err)
	}
}

// addRule appends a domain rule to the policy.
func (fixture *propagationFixture) addRule(policyID int64, value string) {
	fixture.t.Helper()

	if _, err := fixture.rules.Create(fixture.ctx, domain.Rule{PolicyID: policyID, Type: domain.DomainRuleType, Value: value}); err != nil {
		fixture.t.Fatalf("Create() = %v, want nil", err)
	}
}

// completeMachine seeds a machine whose group links to a policy with two
// rules, returning the machine and its policy.
func (fixture *propagationFixture) completeMachine(ip string) (*domain.Machine, *domain.Policy) {
	machine := fixture.addMachine(ip)
	group := fixture.addGroup("group-" + ip)
	policy := fixture.addPolicy("policy-" + ip)
	fixture.linkPolicy(group, policy)
	fixture.linkMachine(machine, group)
	fixture.addRule(policy.ID, "example-"+ip+".com")

	protocol := "TCP"
	port := 443
	if _, err := fixture.rules.Create(fixture.ctx, domain.Rule{
		PolicyID: policy.ID,
		Type:     domain.IPRuleType,
		Value:    "10.0.0.1",
		Protocol: &protocol,
		Port:     &port,
	}); err != nil {
		fixture.t.Fatalf("Create() = %v, want nil", err)
	}

	return machine, policy
}

// waitForSend blocks until one message is delivered, failing the test on
// timeout.
func waitForSend(t *testing.T, channel chan propagation.Message) propagation.Message {
	t.Helper()

	select {
	case message := <-channel:
		return message
	case <-time.After(propagationWaitTimeout):
		t.Fatalf("timed out waiting for a propagation message")
		return propagation.Message{}
	}
}

// waitForCondition polls the provided predicate until it holds within the
// given timeout, failing the test otherwise.
func waitForCondition(t *testing.T, condition func() bool, timeout time.Duration) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}

	t.Fatalf("condition not met within %v", timeout)
}

// assertNoSend fails the test when a message is already pending on the
// channel.
func assertNoSend(t *testing.T, channel chan propagation.Message) {
	t.Helper()

	select {
	case message := <-channel:
		t.Fatalf("unexpected message %+v", message)
	default:
	}
}
