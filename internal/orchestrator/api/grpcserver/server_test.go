package grpcserver_test

import (
	"context"
	"errors"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"dougdomingos.com/aegis/internal/common/auth"
	commonErrors "dougdomingos.com/aegis/internal/common/errors"
	"dougdomingos.com/aegis/internal/common/propagation"
	"dougdomingos.com/aegis/internal/orchestrator/api/grpcserver"
	"dougdomingos.com/aegis/pkg/pb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

const (
	sessionSecret = "test-session-secret"
	bufSize       = 16 << 20
)

// ============================================================================
// Connect
// ============================================================================

func TestPropagationServer_Connect_WithValidToken_RegistersAndDeliversSnapshot(t *testing.T) {
	harness := newGRPCHarness(t)
	harness.controller.SnapshotMessage = packageMessage(42)

	stream := harness.connect(t, harness.token(t, 42))

	message, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv() = %v, want the snapshot", err)
	}

	if message.Epoch != "epoch-abc" {
		t.Errorf("Epoch = %q, want %q", message.Epoch, "epoch-abc")
	}
	if message.MachineId != 42 {
		t.Errorf("MachineId = %d, want 42", message.MachineId)
	}
	if message.Sequence != 1 {
		t.Errorf("Sequence = %d, want 1", message.Sequence)
	}

	packagePayload := message.GetPolicy()
	if packagePayload == nil {
		t.Fatalf("payload = %T, want the policy package", message.Payload)
	}
	if packagePayload.PolicyName != "lab-whitelist" {
		t.Errorf("PolicyName = %q, want %q", packagePayload.PolicyName, "lab-whitelist")
	}
	if packagePayload.PolicyId != 7 {
		t.Errorf("PolicyId = %d, want 7", packagePayload.PolicyId)
	}
	if packagePayload.PolicyVersion != 3 {
		t.Errorf("PolicyVersion = %d, want 3", packagePayload.PolicyVersion)
	}
	if packagePayload.PolicyType != propagation.PolicyTypeWhitelist {
		t.Errorf("PolicyType = %q, want %q", packagePayload.PolicyType, propagation.PolicyTypeWhitelist)
	}

	if len(packagePayload.Rules) != 2 {
		t.Fatalf("Rules = %d, want 2", len(packagePayload.Rules))
	}

	ipRule := packagePayload.Rules[0]
	if ipRule.Protocol == nil || *ipRule.Protocol != "TCP" {
		t.Errorf("IP rule Protocol = %v, want TCP", ipRule.Protocol)
	}
	if ipRule.Port == nil || *ipRule.Port != 443 {
		t.Errorf("IP rule Port = %v, want 443", ipRule.Port)
	}

	domainRule := packagePayload.Rules[1]
	if domainRule.Protocol != nil || domainRule.Port != nil {
		t.Errorf("domain rule optional fields = %v/%v, want nil/nil", domainRule.Protocol, domainRule.Port)
	}

	if got := harness.controller.registeredMachines(); len(got) != 1 || got[0] != 42 {
		t.Errorf("registered machines = %v, want [42]", got)
	}
}

func TestPropagationServer_Connect_WithMissingToken_ReturnsUnauthenticated(t *testing.T) {
	harness := newGRPCHarness(t)

	stream := harness.connect(t, "")

	_, err := stream.Recv()
	assertCode(t, err, codes.Unauthenticated)
}

func TestPropagationServer_Connect_WithMalformedToken_ReturnsUnauthenticated(t *testing.T) {
	harness := newGRPCHarness(t)

	stream := harness.connect(t, "not-a-valid-token")

	_, err := stream.Recv()
	assertCode(t, err, codes.Unauthenticated)
}

func TestPropagationServer_Connect_WithRemovedMachine_ReturnsUnauthenticated(t *testing.T) {
	harness := newGRPCHarness(t)
	harness.controller.RegisterErr = commonErrors.ErrMachineNotFound

	stream := harness.connect(t, harness.token(t, 42))

	_, err := stream.Recv()
	assertCode(t, err, codes.Unauthenticated)
}

func TestPropagationServer_Connect_WithAck_ForwardsAck(t *testing.T) {
	testCases := map[string]struct {
		status pb.AckStatus
		want   propagation.AckStatus
	}{
		"acked":       {status: pb.AckStatus_ACK_STATUS_ACKED, want: propagation.AckAcked},
		"nacked":      {status: pb.AckStatus_ACK_STATUS_NACKED, want: propagation.AckNacked},
		"unspecified": {status: pb.AckStatus_ACK_STATUS_UNSPECIFIED, want: propagation.AckStatus("")},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			harness := newGRPCHarness(t)

			stream := harness.connect(t, harness.token(t, 42))
			if _, err := stream.Recv(); err != nil {
				t.Fatalf("Recv() = %v, want the snapshot", err)
			}

			err := stream.Send(&pb.Ack{Sequence: 4, Status: tc.status, Reason: "because"})
			if err != nil {
				t.Fatalf("Send() = %v, want nil", err)
			}

			record := waitFor(t, harness.controller.acked, "acknowledgement")
			if record.machineID != 42 {
				t.Errorf("machineID = %d, want 42", record.machineID)
			}
			if record.ack.Sequence != 4 {
				t.Errorf("Sequence = %d, want 4", record.ack.Sequence)
			}
			if record.ack.Status != tc.want {
				t.Errorf("Status = %q, want %q", record.ack.Status, tc.want)
			}
			if record.ack.Reason != "because" {
				t.Errorf("Reason = %q, want %q", record.ack.Reason, "because")
			}
		})
	}
}

func TestPropagationServer_Connect_WithRevokeSnapshot_DeliversRevokePayload(t *testing.T) {
	harness := newGRPCHarness(t)
	harness.controller.SnapshotMessage = propagation.Message{
		Epoch:     "epoch-abc",
		Sequence:  1,
		MachineID: 42,
		Kind:      propagation.KindRevoke,
	}

	stream := harness.connect(t, harness.token(t, 42))

	message, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv() = %v, want the revoke", err)
	}

	if message.GetRevoke() == nil {
		t.Fatalf("payload = %T, want the revoke payload", message.Payload)
	}
}

func TestPropagationServer_Connect_WithUnknownKind_AbandonsStreamWithoutSending(t *testing.T) {
	controller := newMockController()
	controller.SnapshotMessage = propagation.Message{
		Epoch:     "epoch-abc",
		Sequence:  1,
		MachineID: 42,
		Kind:      propagation.Kind("BOGUS"),
	}
	server := grpcserver.NewPropagationServer(controller, sessionSecret, nil)

	fake := newFakeConnectStream(signSessionToken(t, 42))

	connectDone := make(chan error, 1)
	go func() { connectDone <- server.Connect(fake) }()

	t.Cleanup(func() {
		fake.releaseSend()
		fake.endRecv()
	})

	// A rejected conversion is the only thing that cancels this handler:
	// Connect must end, proving the sender abandoned the stream, and no
	// message may have reached the wire.
	select {
	case err := <-connectDone:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Connect() = %v, want the cancellation from the rejected message", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Connect() did not return after the unknown kind was rejected")
	}

	select {
	case message := <-fake.sent:
		t.Fatalf("sent = %+v, want no message for the unknown kind", message)
	default:
	}
}

func TestPropagationServer_Connect_WhenStreamEnds_Unregisters(t *testing.T) {
	harness := newGRPCHarness(t)

	stream := harness.connect(t, harness.token(t, 42))
	if _, err := stream.Recv(); err != nil {
		t.Fatalf("Recv() = %v, want the snapshot", err)
	}

	if err := stream.CloseSend(); err != nil {
		t.Fatalf("CloseSend() = %v, want nil", err)
	}

	record := waitFor(t, harness.controller.unregistered, "unregistration")
	if record.machineID != 42 {
		t.Errorf("machineID = %d, want 42", record.machineID)
	}
	if record.connID == 0 {
		t.Error("connID = 0, want the registration connection ID")
	}
}

func TestPropagationServer_Connect_WithFiveMiBMessage_RoundTrips(t *testing.T) {
	harness := newGRPCHarness(t)

	message := packageMessage(42)
	message.Rules = []propagation.Rule{{Type: "DOMAIN", Value: strings.Repeat("a", 5<<20)}}
	harness.controller.SnapshotMessage = message

	stream := harness.connect(t, harness.token(t, 42))

	received, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv() = %v, want the 5 MiB snapshot", err)
	}

	packagePayload := received.GetPolicy()
	if packagePayload == nil {
		t.Fatal("payload = nil, want the policy package")
	}
	if len(packagePayload.Rules) != 1 || len(packagePayload.Rules[0].Value) != 5<<20 {
		t.Errorf("rules = %+v, want one rule of 5 MiB", packagePayload.Rules)
	}
}

func TestPropagationServer_Sender_WhenStreamStalls_TimesOutAndRefusesFurtherSends(t *testing.T) {
	controller := newMockController()
	server := grpcserver.NewPropagationServer(controller, sessionSecret, nil)

	fake := newFakeConnectStream(signSessionToken(t, 42))

	connectDone := make(chan error, 1)
	go func() { connectDone <- server.Connect(fake) }()

	t.Cleanup(func() {
		fake.releaseSend()
		fake.endRecv()
	})

	// The snapshot send completes, which proves the handler registered its
	// sender before delivering it.
	if snapshot := waitFor(t, fake.sent, "the snapshot"); snapshot == nil {
		t.Fatal("snapshot = nil, want the delivered package")
	}

	sender, exists := controller.senderFor(42)
	if !exists {
		t.Fatal("sender for machine 42 not registered")
	}

	// The second send stalls inside the stream: the caller's context must
	// bound it, or one wedged machine would block the propagation worker.
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	started := time.Now()
	err := sender.Send(ctx, packageMessage(42))
	if err == nil {
		t.Fatal("Send() = nil, want the deadline error for a stalled stream")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Send() = %v, want a context deadline error", err)
	}
	if elapsed := time.Since(started); elapsed > 5*time.Second {
		t.Errorf("Send() took %v, want it bounded by the 150ms context", elapsed)
	}

	// The abandoned sender must refuse further writes instead of stalling
	// again on the same stream.
	secondCtx, secondCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer secondCancel()

	started = time.Now()
	secondErr := sender.Send(secondCtx, packageMessage(42))
	if secondErr == nil {
		t.Fatal("second Send() = nil, want the wedged-stream error")
	}
	if errors.Is(secondErr, context.DeadlineExceeded) {
		t.Errorf("second Send() = %v, want an immediate wedged-stream error, not a timeout", secondErr)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Errorf("second Send() took %v, want an immediate rejection", elapsed)
	}

	// Abandoning the send cancels the handler's context, which ends Connect
	// — that teardown is what unwinds the wedged stream.
	select {
	case err := <-connectDone:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Connect() = %v, want the cancellation from the abandoned send", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Connect() did not return after the send was abandoned")
	}
}

// ============================================================================
// Harness
// ============================================================================

// connRecord captures one Unregister call.
type connRecord struct {
	machineID int64
	connID    uint64
}

// ackRecord captures one Acknowledge call.
type ackRecord struct {
	machineID int64
	ack       propagation.Ack
}

// mockController implements the consumer-owned propagationController contract:
// Register stores senders, DeliverSnapshot immediately sends the configured
// snapshot, and Unregister/Acknowledge publish what the stream handler did to
// channels the tests consume.
type mockController struct {
	mu              sync.Mutex
	Registered      map[int64]propagation.MessageSender
	RegisterOrder   []int64
	RegisterErr     error
	SnapshotMessage propagation.Message

	nextID       uint64
	unregistered chan connRecord
	acked        chan ackRecord
}

func newMockController() *mockController {
	return &mockController{
		Registered:      make(map[int64]propagation.MessageSender),
		unregistered:    make(chan connRecord, 8),
		acked:           make(chan ackRecord, 8),
		SnapshotMessage: packageMessage(0),
	}
}

func (mock *mockController) Register(_ context.Context, machineID int64, sender propagation.MessageSender) (uint64, error) {
	mock.mu.Lock()
	defer mock.mu.Unlock()

	if mock.RegisterErr != nil {
		return 0, mock.RegisterErr
	}

	mock.nextID++
	mock.Registered[machineID] = sender
	mock.RegisterOrder = append(mock.RegisterOrder, machineID)

	return mock.nextID, nil
}

func (mock *mockController) Unregister(machineID int64, connID uint64) {
	mock.unregistered <- connRecord{machineID: machineID, connID: connID}
}

func (mock *mockController) DeliverSnapshot(ctx context.Context, machineID int64) {
	mock.mu.Lock()
	sender, exists := mock.Registered[machineID]
	message := mock.SnapshotMessage
	mock.mu.Unlock()

	if !exists {
		return
	}

	if message.MachineID == 0 {
		message.MachineID = machineID
	}

	_ = sender.Send(ctx, message)
}

func (mock *mockController) Acknowledge(ctx context.Context, machineID int64, ack propagation.Ack) {
	mock.acked <- ackRecord{machineID: machineID, ack: ack}
}

func (mock *mockController) registeredMachines() []int64 {
	mock.mu.Lock()
	defer mock.mu.Unlock()

	return append([]int64{}, mock.RegisterOrder...)
}

// senderFor returns the sender registered for the machine, if any.
func (mock *mockController) senderFor(machineID int64) (propagation.MessageSender, bool) {
	mock.mu.Lock()
	defer mock.mu.Unlock()

	sender, exists := mock.Registered[machineID]
	return sender, exists
}

// grpcHarness runs the propagation server over bufconn with a mock controller
// and a real gRPC client.
type grpcHarness struct {
	controller *mockController
	client     pb.PropagationClient
}

func newGRPCHarness(t *testing.T) *grpcHarness {
	t.Helper()

	listener := bufconn.Listen(bufSize)
	controller := newMockController()

	grpcServer := grpc.NewServer(
		grpc.MaxRecvMsgSize(bufSize),
		grpc.MaxSendMsgSize(bufSize),
	)
	pb.RegisterPropagationServer(grpcServer, grpcserver.NewPropagationServer(controller, sessionSecret, nil))

	go func() {
		if err := grpcServer.Serve(listener); err != nil {
			t.Logf("bufconn server stopped: %v", err)
		}
	}()

	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return listener.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(bufSize),
			grpc.MaxCallSendMsgSize(bufSize),
		),
	)
	if err != nil {
		t.Fatalf("grpc.NewClient() = %v, want nil", err)
	}

	t.Cleanup(func() {
		conn.Close()
		grpcServer.Stop()
	})

	return &grpcHarness{controller: controller, client: pb.NewPropagationClient(conn)}
}

// token signs a session token for the machine ID with the harness secret.
func (harness *grpcHarness) token(t *testing.T, machineID int64) string {
	t.Helper()

	return signSessionToken(t, machineID)
}

// signSessionToken signs a session token for the machine ID with the test
// secret.
func signSessionToken(t *testing.T, machineID int64) string {
	t.Helper()

	token, _, err := auth.SignSessionToken(machineID, sessionSecret, time.Hour, time.Now())
	if err != nil {
		t.Fatalf("SignSessionToken() = %v, want nil", err)
	}

	return token
}

// connect opens the propagation stream, attaching the bearer token when
// non-empty.
func (harness *grpcHarness) connect(t *testing.T, token string) pb.Propagation_ConnectClient {
	t.Helper()

	ctx := context.Background()
	if token != "" {
		ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+token)
	}

	stream, err := harness.client.Connect(ctx)
	if err != nil {
		t.Fatalf("Connect() = %v, want nil", err)
	}

	return stream
}

// packageMessage builds a complete package wire message for the machine.
func packageMessage(machineID int64) propagation.Message {
	protocol := "TCP"
	port := 443

	return propagation.Message{
		Epoch:         "epoch-abc",
		Sequence:      1,
		MachineID:     machineID,
		Kind:          propagation.KindPackage,
		PolicyID:      7,
		PolicyName:    "lab-whitelist",
		PolicyVersion: 3,
		PolicyType:    propagation.PolicyTypeWhitelist,
		Rules: []propagation.Rule{
			{Type: "IP", Value: "10.0.0.1", Protocol: &protocol, Port: &port},
			{Type: "DOMAIN", Value: "example.com"},
		},
	}
}

// waitFor receives one record from the channel or fails the test.
func waitFor[T any](t *testing.T, channel <-chan T, what string) T {
	t.Helper()

	select {
	case record := <-channel:
		return record
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for %s", what)
		var zero T
		return zero
	}
}

// assertCode verifies the gRPC status code of an error.
func assertCode(t *testing.T, err error, want codes.Code) {
	t.Helper()

	if err == nil {
		t.Fatal("expected an error, got nil")
	}

	if got := status.Code(err); got != want {
		t.Errorf("status code = %v, want %v (err: %v)", got, want, err)
	}
}

// fakeConnectStream is a pb.Propagation_ConnectServer double: its context
// carries the bearer token, the first send completes so the handler can
// deliver the snapshot, every later send stalls until the test releases it,
// and Recv blocks until the stream is ended.
type fakeConnectStream struct {
	grpc.ServerStream

	ctx  context.Context
	sent chan *pb.ServerMessage
	end  chan struct{}

	stall     chan struct{}
	stallOnce sync.Once
	endOnce   sync.Once

	mu    sync.Mutex
	sends int
}

// newFakeConnectStream builds a fake stream authenticated as the machine.
func newFakeConnectStream(token string) *fakeConnectStream {
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("authorization", "Bearer "+token))

	return &fakeConnectStream{
		ctx:   ctx,
		sent:  make(chan *pb.ServerMessage, 8),
		end:   make(chan struct{}),
		stall: make(chan struct{}),
	}
}

// Context carries the incoming metadata the server authenticates with.
func (fake *fakeConnectStream) Context() context.Context {
	return fake.ctx
}

// Send records the first message and stalls every later one until released.
func (fake *fakeConnectStream) Send(message *pb.ServerMessage) error {
	fake.mu.Lock()
	fake.sends++
	first := fake.sends == 1
	fake.mu.Unlock()

	if first {
		fake.sent <- message
		return nil
	}

	<-fake.stall
	return nil
}

// Recv blocks until the stream is ended, then reports a clean end of stream.
func (fake *fakeConnectStream) Recv() (*pb.Ack, error) {
	<-fake.end
	return nil, io.EOF
}

// releaseSend unblocks every stalled send.
func (fake *fakeConnectStream) releaseSend() {
	fake.stallOnce.Do(func() { close(fake.stall) })
}

// endRecv ends the receive loop.
func (fake *fakeConnectStream) endRecv() {
	fake.endOnce.Do(func() { close(fake.end) })
}
