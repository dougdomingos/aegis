package grpcserver_test

import (
	"context"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"dougdomingos.com/aegis/internal/common/auth"
	commonErrors "dougdomingos.com/aegis/internal/common/errors"
	"dougdomingos.com/aegis/internal/common/propagation"
	"dougdomingos.com/aegis/internal/orchestrator/api/grpcserver"
	"dougdomingos.com/aegis/internal/orchestrator/service"
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

	packagePayload := message.GetPolicy()
	if packagePayload == nil {
		t.Fatalf("payload = %T, want the policy package", message.Payload)
	}
	if packagePayload.PolicyName != "lab-whitelist" {
		t.Errorf("PolicyName = %q, want %q", packagePayload.PolicyName, "lab-whitelist")
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
	harness := newGRPCHarness(t)

	stream := harness.connect(t, harness.token(t, 42))
	if _, err := stream.Recv(); err != nil {
		t.Fatalf("Recv() = %v, want the snapshot", err)
	}

	err := stream.Send(&pb.Ack{Sequence: 4, Status: pb.AckStatus_ACK_STATUS_NACKED, Reason: "stale sequence"})
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
	if record.ack.Status != propagation.AckNacked {
		t.Errorf("Status = %q, want %q", record.ack.Status, propagation.AckNacked)
	}
	if record.ack.Reason != "stale sequence" {
		t.Errorf("Reason = %q, want %q", record.ack.Reason, "stale sequence")
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
	Registered      map[int64]service.MessageSender
	RegisterOrder   []int64
	RegisterErr     error
	SnapshotMessage propagation.Message

	nextID       uint64
	unregistered chan connRecord
	acked        chan ackRecord
}

func newMockController() *mockController {
	return &mockController{
		Registered:      make(map[int64]service.MessageSender),
		unregistered:    make(chan connRecord, 8),
		acked:           make(chan ackRecord, 8),
		SnapshotMessage: packageMessage(0),
	}
}

func (mock *mockController) Register(machineID int64, sender service.MessageSender) (uint64, error) {
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
