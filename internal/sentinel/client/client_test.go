package client_test

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"errors"

	sentinelErrors "dougdomingos.com/aegis/internal/common/errors"
	"dougdomingos.com/aegis/internal/common/propagation"
	"dougdomingos.com/aegis/internal/sentinel/client"
	"dougdomingos.com/aegis/internal/sentinel/hostresolve"
	"dougdomingos.com/aegis/internal/sentinel/identity"
	"dougdomingos.com/aegis/pkg/pb"

	"dougdomingos.com/aegis/internal/common/auth"
	"github.com/miekg/dns"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// labKey is the 32-byte shared key used by the client tests.
var labKey = []byte("0123456789abcdef0123456789abcdef")

// testIdentity is the machine identity used by the client tests.
var testIdentity = identity.Identity{IP: "10.0.1.11", MAC: "aa:bb:cc:dd:ee:ff"}

func TestClient_Authenticate_WithValidChallenge_ReturnsSession(t *testing.T) {
	var receivedSignature string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/machines/authenticate/init":
			fmt.Fprint(w, `{"Nonce":"abc123","ExpiresAt":"2026-10-22T12:00:00Z"}`)
		case "/machines/authenticate":
			var payload struct {
				IP        string
				MAC       string
				Signature string
			}

			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Errorf("failed to decode authenticate request: %v", err)
			}

			receivedSignature = payload.Signature
			fmt.Fprint(w, `{"SessionToken":"token-xyz","ExpiresAt":"2026-10-23T12:00:00Z","Machine":{"ID":1,"Name":"lcc1-001","IP":"10.0.1.11","MAC":"aa:bb:cc:dd:ee:ff","Status":"CONNECTED"}}`)
		default:
			t.Errorf("unexpected request path %q", r.URL.Path)
		}
	}))
	defer server.Close()

	cl := newTestClient(t, server.URL, 2*time.Second)

	session, err := cl.Authenticate(t.Context(), testIdentity)
	if err != nil {
		t.Fatalf("client.Authenticate() error = %v, want nil", err)
	}

	if session.Token != "token-xyz" {
		t.Errorf("session.Token = %q, want token-xyz", session.Token)
	}

	expectedExpiry := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	if !session.ExpiresAt.Equal(expectedExpiry) {
		t.Errorf("session.ExpiresAt = %v, want %v", session.ExpiresAt, expectedExpiry)
	}

	if session.MachineName != "lcc1-001" {
		t.Errorf("session.MachineName = %q, want lcc1-001", session.MachineName)
	}

	if session.IP != testIdentity.IP {
		t.Errorf("session.IP = %q, want %q", session.IP, testIdentity.IP)
	}

	if session.MAC != testIdentity.MAC {
		t.Errorf("session.MAC = %q, want %q", session.MAC, testIdentity.MAC)
	}

	expectedSignature := hex.EncodeToString(auth.Sign(labKey, auth.CanonicalPayload("10.0.1.11", "abc123")))
	if receivedSignature != expectedSignature {
		t.Errorf("received signature = %q, want %q", receivedSignature, expectedSignature)
	}
}

func TestClient_Authenticate_WithRejectedCredentials_ReturnsAuthenticationRejectedError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/machines/authenticate/init" {
			fmt.Fprint(w, `{"Nonce":"abc123","ExpiresAt":"2026-10-22T12:00:00Z"}`)
			return
		}

		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"error_code":401,"message":"invalid machine credentials"}`)
	}))
	defer server.Close()

	cl := newTestClient(t, server.URL, 2*time.Second)

	_, err := cl.Authenticate(t.Context(), testIdentity)
	if err == nil {
		t.Fatalf("client.Authenticate() error = nil, want error")
	}

	if !errors.Is(err, sentinelErrors.ErrAuthenticationRejected) {
		t.Errorf("client.Authenticate() error = %v, want ErrAuthenticationRejected", err)
	}

	if !strings.Contains(err.Error(), "invalid machine credentials") {
		t.Errorf("client.Authenticate() error = %v, want message invalid machine credentials", err)
	}
}

func TestClient_Authenticate_WithInitFailure_ReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"error_code":500,"message":"internal failure"}`)
	}))
	defer server.Close()

	cl := newTestClient(t, server.URL, 2*time.Second)

	if _, err := cl.Authenticate(t.Context(), testIdentity); err == nil {
		t.Errorf("client.Authenticate() error = nil, want error")
	}
}

func TestClient_Authenticate_WithMalformedResponse_ReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "not json")
	}))
	defer server.Close()

	cl := newTestClient(t, server.URL, 2*time.Second)

	if _, err := cl.Authenticate(t.Context(), testIdentity); err == nil {
		t.Errorf("client.Authenticate() error = nil, want error")
	}
}

func TestClient_Authenticate_WithUnreachableOrchestrator_ReturnsUnreachableError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request to closed server")
	}))
	server.Close()

	cl := newTestClient(t, server.URL, 2*time.Second)

	_, err := cl.Authenticate(t.Context(), testIdentity)
	if err == nil {
		t.Fatalf("client.Authenticate() error = nil, want error")
	}

	if !errors.Is(err, sentinelErrors.ErrOrchestratorUnreachable) {
		t.Errorf("client.Authenticate() error = %v, want ErrOrchestratorUnreachable", err)
	}
}

func TestClient_Authenticate_WithRequestTimeout_ReturnsUnreachableError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		fmt.Fprint(w, `{"Nonce":"abc123","ExpiresAt":"2026-10-22T12:00:00Z"}`)
	}))
	defer server.Close()

	cl := newTestClient(t, server.URL, 20*time.Millisecond)

	_, err := cl.Authenticate(t.Context(), testIdentity)
	if err == nil {
		t.Fatalf("client.Authenticate() error = nil, want error")
	}

	if !errors.Is(err, sentinelErrors.ErrOrchestratorUnreachable) {
		t.Errorf("client.Authenticate() error = %v, want ErrOrchestratorUnreachable", err)
	}
}

func TestClient_Authenticate_WithTrailingSlashBaseURL_RequestsExactPath(t *testing.T) {
	var receivedPath string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if receivedPath == "" {
			receivedPath = r.URL.Path
		}
	}))
	defer server.Close()

	cl := newTestClient(t, server.URL+"/", 2*time.Second)

	if _, err := cl.Authenticate(t.Context(), testIdentity); err == nil {
		t.Errorf("client.Authenticate() error = nil, want error")
	}

	if receivedPath != "/machines/authenticate/init" {
		t.Errorf("received path = %q, want /machines/authenticate/init", receivedPath)
	}
}

func TestClient_Heartbeat_ValidToken_ReturnsNoError(t *testing.T) {
	var receivedPath, receivedAuth string
	var receivedBody []byte

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		receivedAuth = r.Header.Get("Authorization")
		receivedBody, _ = io.ReadAll(r.Body)

		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	cl := newTestClient(t, server.URL, 2*time.Second)

	if err := cl.Heartbeat(t.Context(), "session-token"); err != nil {
		t.Fatalf("client.Heartbeat() error = %v, want nil", err)
	}

	if receivedPath != "/machines/heartbeat" {
		t.Errorf("request path = %q, want /machines/heartbeat", receivedPath)
	}

	if receivedAuth != "Bearer session-token" {
		t.Errorf("Authorization header = %q, want Bearer session-token", receivedAuth)
	}

	if len(receivedBody) != 0 {
		t.Errorf("request body = %q, want empty", receivedBody)
	}
}

func TestClient_Heartbeat_RejectedToken_ReturnsAuthenticationRejectedError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"error_code":401,"message":"invalid session token"}`)
	}))
	defer server.Close()

	cl := newTestClient(t, server.URL, 2*time.Second)

	err := cl.Heartbeat(t.Context(), "expired-token")
	if err == nil {
		t.Fatalf("client.Heartbeat() error = nil, want error")
	}

	if !errors.Is(err, sentinelErrors.ErrAuthenticationRejected) {
		t.Errorf("client.Heartbeat() error = %v, want ErrAuthenticationRejected", err)
	}
}

func TestClient_Heartbeat_ServerError_ReturnsPlainError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	cl := newTestClient(t, server.URL, 2*time.Second)

	err := cl.Heartbeat(t.Context(), "session-token")
	if err == nil {
		t.Fatalf("client.Heartbeat() error = nil, want error")
	}

	if errors.Is(err, sentinelErrors.ErrAuthenticationRejected) || errors.Is(err, sentinelErrors.ErrOrchestratorUnreachable) {
		t.Errorf("client.Heartbeat() error = %v, want plain status error", err)
	}
}

func TestClient_Heartbeat_UnreachableOrchestrator_ReturnsUnreachableError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	server.Close()

	cl := newTestClient(t, server.URL, 2*time.Second)

	if err := cl.Heartbeat(t.Context(), "session-token"); !errors.Is(err, sentinelErrors.ErrOrchestratorUnreachable) {
		t.Errorf("client.Heartbeat() error = %v, want ErrOrchestratorUnreachable", err)
	}
}

func TestClient_Heartbeat_TrailingSlashBaseURL_RequestsExactPath(t *testing.T) {
	var receivedPath string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	cl := newTestClient(t, server.URL+"/", 2*time.Second)

	if err := cl.Heartbeat(t.Context(), "session-token"); err != nil {
		t.Fatalf("client.Heartbeat() error = %v, want nil", err)
	}

	if receivedPath != "/machines/heartbeat" {
		t.Errorf("request path = %q, want /machines/heartbeat", receivedPath)
	}
}

func TestClient_New_WithInvalidPropagationAddress_ReturnsError(t *testing.T) {
	cases := map[string]struct {
		address string
	}{
		"no port":    {address: "10.0.0.5"},
		"malformed":  {address: "10.0.0.5:9090:1"},
		"empty port": {address: "10.0.0.5:"},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := client.New(mustParseURL(t, "http://127.0.0.1:1"), tc.address, labKey, time.Second, nil, nil)
			if err == nil {
				t.Errorf("expected an error for propagation address %q, got nil", tc.address)
			}
		})
	}
}

func TestClient_Authenticate_WithValidChallenge_PopulatesMachineID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/machines/authenticate/init":
			fmt.Fprint(w, `{"Nonce":"abc123","ExpiresAt":"2026-10-22T12:00:00Z"}`)
		case "/machines/authenticate":
			fmt.Fprint(w, `{"SessionToken":"token-xyz","ExpiresAt":"2026-10-23T12:00:00Z","Machine":{"ID":7,"Name":"lcc1-001","IP":"10.0.1.11","Status":"CONNECTED"}}`)
		default:
			t.Errorf("unexpected request path %q", r.URL.Path)
		}
	}))
	defer server.Close()

	cl := newTestClient(t, server.URL, 2*time.Second)

	session, err := cl.Authenticate(t.Context(), testIdentity)
	if err != nil {
		t.Fatalf("client.Authenticate() error = %v, want nil", err)
	}

	if session.MachineID != 7 {
		t.Errorf("session.MachineID = %d, want 7", session.MachineID)
	}
}

func TestClient_ConnectPropagation_WithToken_SendsAuthorizationMetadata(t *testing.T) {
	fake, address := startPropagationFake(t)

	cl := newTestClientAtPropagation(t, address, 2*time.Second)

	stream, err := cl.ConnectPropagation(t.Context(), "token-abc")
	if err != nil {
		t.Fatalf("client.ConnectPropagation() error = %v, want nil", err)
	}
	defer func() { _ = stream.CloseSend() }()

	if got := waitForString(t, fake.authorization, "authorization metadata"); got != "Bearer token-abc" {
		t.Errorf("authorization = %q, want %q", got, "Bearer token-abc")
	}
}

func TestClient_ConnectPropagation_WhenServerSendsPackage_ReturnsWireMessage(t *testing.T) {
	fake, address := startPropagationFake(t)

	protocol, port := "TCP", int32(443)
	fake.inbound <- &pb.ServerMessage{
		Epoch:     "epoch-abc",
		Sequence:  4,
		MachineId: 7,
		Payload: &pb.ServerMessage_Policy{Policy: &pb.PolicyPackage{
			PolicyId:      9,
			PolicyName:    "lab-whitelist",
			PolicyVersion: 2,
			PolicyType:    propagation.PolicyTypeWhitelist,
			Rules: []*pb.Rule{
				{Type: "IP", Value: "10.0.0.1", Protocol: &protocol, Port: &port},
				{Type: "DOMAIN", Value: "example.com"},
			},
		}},
	}

	cl := newTestClientAtPropagation(t, address, 2*time.Second)

	stream, err := cl.ConnectPropagation(t.Context(), "token-abc")
	if err != nil {
		t.Fatalf("client.ConnectPropagation() error = %v, want nil", err)
	}

	message, err := stream.Recv()
	if err != nil {
		t.Fatalf("stream.Recv() error = %v, want the package", err)
	}

	if message.Epoch != "epoch-abc" || message.Sequence != 4 || message.MachineID != 7 {
		t.Errorf("envelope = %+v, want epoch-abc/4/7", message)
	}
	if message.Kind != propagation.KindPackage {
		t.Errorf("Kind = %q, want %q", message.Kind, propagation.KindPackage)
	}
	if message.PolicyID != 9 || message.PolicyName != "lab-whitelist" || message.PolicyVersion != 2 {
		t.Errorf("policy metadata = %+v, want 9/lab-whitelist/2", message)
	}
	if message.PolicyType != propagation.PolicyTypeWhitelist {
		t.Errorf("PolicyType = %q, want %q", message.PolicyType, propagation.PolicyTypeWhitelist)
	}
	if len(message.Rules) != 2 {
		t.Fatalf("Rules = %d, want 2", len(message.Rules))
	}
	if message.Rules[0].Protocol == nil || *message.Rules[0].Protocol != "TCP" {
		t.Errorf("IP rule protocol = %v, want TCP", message.Rules[0].Protocol)
	}
	if message.Rules[0].Port == nil || *message.Rules[0].Port != 443 {
		t.Errorf("IP rule port = %v, want 443", message.Rules[0].Port)
	}
	if message.Rules[1].Protocol != nil || message.Rules[1].Port != nil {
		t.Errorf("domain rule optional fields = %v/%v, want nil/nil", message.Rules[1].Protocol, message.Rules[1].Port)
	}
}

func TestClient_ConnectPropagation_WhenServerSendsRevoke_ReturnsRevokeMessage(t *testing.T) {
	fake, address := startPropagationFake(t)

	fake.inbound <- &pb.ServerMessage{
		Epoch:     "epoch-abc",
		Sequence:  5,
		MachineId: 7,
		Payload:   &pb.ServerMessage_Revoke{Revoke: &pb.Revoke{}},
	}

	cl := newTestClientAtPropagation(t, address, 2*time.Second)

	stream, err := cl.ConnectPropagation(t.Context(), "token-abc")
	if err != nil {
		t.Fatalf("client.ConnectPropagation() error = %v, want nil", err)
	}

	message, err := stream.Recv()
	if err != nil {
		t.Fatalf("stream.Recv() error = %v, want the revoke", err)
	}

	if message.Kind != propagation.KindRevoke {
		t.Errorf("Kind = %q, want %q", message.Kind, propagation.KindRevoke)
	}
	if len(message.Rules) != 0 || message.PolicyID != 0 {
		t.Errorf("revoke message = %+v, want empty policy and rules", message)
	}
}

func TestClient_ConnectPropagation_WhenAckSent_ConvertsStatus(t *testing.T) {
	fake, address := startPropagationFake(t)

	cl := newTestClientAtPropagation(t, address, 2*time.Second)

	stream, err := cl.ConnectPropagation(t.Context(), "token-abc")
	if err != nil {
		t.Fatalf("client.ConnectPropagation() error = %v, want nil", err)
	}

	cases := []struct {
		ack        propagation.Ack
		wantStatus pb.AckStatus
	}{
		{ack: propagation.Ack{Sequence: 4, Status: propagation.AckAcked}, wantStatus: pb.AckStatus_ACK_STATUS_ACKED},
		{ack: propagation.Ack{Sequence: 5, Status: propagation.AckNacked, Reason: "stale sequence"}, wantStatus: pb.AckStatus_ACK_STATUS_NACKED},
	}

	for _, tc := range cases {
		if err := stream.Send(tc.ack); err != nil {
			t.Fatalf("stream.Send(%+v) error = %v, want nil", tc.ack, err)
		}

		received := waitForAck(t, fake.acks, "acknowledgement")
		if received.Sequence != tc.ack.Sequence {
			t.Errorf("Sequence = %d, want %d", received.Sequence, tc.ack.Sequence)
		}
		if received.Status != tc.wantStatus {
			t.Errorf("Status = %v, want %v", received.Status, tc.wantStatus)
		}
		if received.Reason != tc.ack.Reason {
			t.Errorf("Reason = %q, want %q", received.Reason, tc.ack.Reason)
		}
	}
}

func TestClient_Heartbeat_WithInjectedResolver_ResolvesHostnameThroughIt(t *testing.T) {
	// Arrange
	dnsAddress := startFakeDNS(t, "orchestrator.test.", "127.0.0.1")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	_, port, _ := net.SplitHostPort(strings.TrimPrefix(server.URL, "http://"))

	cl, err := client.New(mustParseURL(t, "http://orchestrator.test:"+port), "127.0.0.1:1", labKey,
		2*time.Second, hostresolve.Resolver(dnsAddress), nil)
	if err != nil {
		t.Fatalf("client.New() error = %v, want nil", err)
	}

	t.Cleanup(func() { _ = cl.Close() })

	// Act
	err = cl.Heartbeat(t.Context(), "session-token")

	// Assert
	if err != nil {
		t.Fatalf("client.Heartbeat() error = %v, want nil", err)
	}
}

func TestClient_ConnectPropagation_WithInjectedResolver_ResolvesHostnameThroughIt(t *testing.T) {
	// Arrange
	dnsAddress := startFakeDNS(t, "orchestrator.test.", "127.0.0.1")
	fake, address := startPropagationFake(t)
	_, port, _ := net.SplitHostPort(address)

	cl, err := client.New(mustParseURL(t, "http://127.0.0.1:1"), "orchestrator.test:"+port, labKey,
		2*time.Second, hostresolve.Resolver(dnsAddress), nil)
	if err != nil {
		t.Fatalf("client.New() error = %v, want nil", err)
	}

	t.Cleanup(func() { _ = cl.Close() })

	// Act
	stream, err := cl.ConnectPropagation(t.Context(), "token-abc")
	if err != nil {
		t.Fatalf("client.ConnectPropagation() error = %v, want nil", err)
	}
	defer func() { _ = stream.CloseSend() }()

	// Assert
	if got := waitForString(t, fake.authorization, "authorization metadata"); got != "Bearer token-abc" {
		t.Errorf("authorization = %q, want %q", got, "Bearer token-abc")
	}
}

func TestClient_Close_ClosesConnection(t *testing.T) {
	_, address := startPropagationFake(t)

	cl, err := client.New(mustParseURL(t, "http://127.0.0.1:1"), address, labKey, 2*time.Second, nil, nil)
	if err != nil {
		t.Fatalf("client.New() error = %v, want nil", err)
	}

	if err := cl.Close(); err != nil {
		t.Fatalf("client.Close() error = %v, want nil", err)
	}

	if _, err := cl.ConnectPropagation(t.Context(), "token-abc"); err == nil {
		t.Error("client.ConnectPropagation() after Close = nil, want an error")
	}
}

// ============================================================================
// Propagation gRPC test fake
// ============================================================================

// propagationFake implements pb.PropagationServer for client tests: it records
// the authorization metadata of every connection, sends the messages queued on
// inbound, and records every received acknowledgement.
type propagationFake struct {
	pb.UnimplementedPropagationServer

	authorization chan string
	inbound       chan *pb.ServerMessage
	acks          chan *pb.Ack
}

func newPropagationFake() *propagationFake {
	return &propagationFake{
		authorization: make(chan string, 8),
		inbound:       make(chan *pb.ServerMessage, 8),
		acks:          make(chan *pb.Ack, 8),
	}
}

// Connect accepts one stream, publishing its authorization metadata, draining
// queued messages, and recording received acknowledgements until it ends.
func (fake *propagationFake) Connect(stream pb.Propagation_ConnectServer) error {
	incoming, _ := metadata.FromIncomingContext(stream.Context())
	fake.authorization <- strings.Join(incoming.Get("authorization"), ",")

	for {
		select {
		case message := <-fake.inbound:
			if err := stream.Send(message); err != nil {
				return err
			}

			continue
		default:
		}

		ack, err := stream.Recv()
		if err != nil {
			return nil
		}

		fake.acks <- ack
	}
}

// startPropagationFake serves a fake propagation server on a free local port
// and returns it with its address.
func startPropagationFake(t *testing.T) (*propagationFake, string) {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to bind test listener: %v", err)
	}

	fake := newPropagationFake()
	grpcServer := grpc.NewServer(
		grpc.MaxRecvMsgSize(16<<20),
		grpc.MaxSendMsgSize(16<<20),
	)
	pb.RegisterPropagationServer(grpcServer, fake)

	go func() {
		if err := grpcServer.Serve(listener); err != nil {
			t.Logf("propagation fake stopped: %v", err)
		}
	}()

	t.Cleanup(func() {
		grpcServer.Stop()
	})

	return fake, listener.Addr().String()
}

// newTestClientAtPropagation builds a client whose propagation channel points
// at the provided gRPC address, closing it on cleanup.
func newTestClientAtPropagation(t *testing.T, address string, timeout time.Duration) *client.Client {
	t.Helper()

	cl, err := client.New(mustParseURL(t, "http://127.0.0.1:1"), address, labKey, timeout, nil, nil)
	if err != nil {
		t.Fatalf("client.New() error = %v, want nil", err)
	}

	t.Cleanup(func() { _ = cl.Close() })

	return cl
}

// waitForString receives one value from the channel or fails the test.
func waitForString(t *testing.T, channel chan string, what string) string {
	t.Helper()

	select {
	case value := <-channel:
		return value
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for %s", what)
		return ""
	}
}

// waitForAck receives one value from the channel or fails the test.
func waitForAck(t *testing.T, channel chan *pb.Ack, what string) *pb.Ack {
	t.Helper()

	select {
	case ack := <-channel:
		return ack
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for %s", what)
		return nil
	}
}

// newTestClient builds a client pointing at the provided server URL, using a
// fixed lab key and a short timeout. The propagation address points at a port
// where no server listens: gRPC connections are lazy, so it only matters for
// tests that actually open a propagation stream.
func newTestClient(t *testing.T, serverURL string, timeout time.Duration) *client.Client {
	t.Helper()

	cl, err := client.New(mustParseURL(t, serverURL), "127.0.0.1:1", labKey, timeout, nil, nil)
	if err != nil {
		t.Fatalf("client.New() error = %v, want nil", err)
	}

	t.Cleanup(func() { _ = cl.Close() })

	return cl
}

// mustParseURL parses the provided URL, failing the test on a malformed value.
func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()

	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("failed to parse test URL %q: %v", raw, err)
	}

	return parsed
}

// startFakeDNS serves a UDP DNS server on a loopback port that answers every
// A query with ip for the given fully qualified name, and returns its address.
func startFakeDNS(t *testing.T, name, ip string) string {
	t.Helper()

	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen for fake DNS: %v", err)
	}

	server := &dns.Server{PacketConn: conn, Handler: dns.HandlerFunc(func(w dns.ResponseWriter, req *dns.Msg) {
		reply := new(dns.Msg)
		reply.SetReply(req)

		if req.Question[0].Name == name && req.Question[0].Qtype == dns.TypeA {
			reply.Answer = append(reply.Answer, &dns.A{
				Hdr: dns.RR_Header{Name: name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 60},
				A:   net.ParseIP(ip),
			})
		}

		_ = w.WriteMsg(reply)
	})}

	go func() { _ = server.ActivateAndServe() }()

	t.Cleanup(func() { _ = server.Shutdown() })

	return conn.LocalAddr().String()
}
