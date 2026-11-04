// Package client implements the Sentinel's client for the Orchestrator API.
package client

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"time"

	"dougdomingos.com/aegis/internal/common/auth"
	"dougdomingos.com/aegis/internal/common/errors"
	"dougdomingos.com/aegis/internal/common/propagation"
	"dougdomingos.com/aegis/internal/common/schemas"
	"dougdomingos.com/aegis/internal/sentinel/daemon"
	"dougdomingos.com/aegis/internal/sentinel/identity"
	"dougdomingos.com/aegis/pkg/pb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

const (
	// initiatePath is the path of the authentication challenge endpoint.
	initiatePath = "/machines/authenticate/init"

	// authenticatePath is the path of the authentication endpoint.
	authenticatePath = "/machines/authenticate"

	// heartbeatPath is the path of the connectivity heartbeat endpoint.
	heartbeatPath = "/machines/heartbeat"

	// maxMessageSize mirrors the Orchestrator's gRPC send/receive limit.
	maxMessageSize = 16 << 20
)

// Client performs authenticated requests against the Orchestrator API.
type Client struct {
	// baseURL is the base address of the Orchestrator API.
	baseURL *url.URL

	// httpClient sends the requests, honouring a per-request timeout.
	httpClient *http.Client

	// labKey is the shared HMAC key used to sign authentication challenges.
	labKey []byte

	// logger records the operations performed by the client.
	logger *slog.Logger

	// connection is the gRPC channel to the Orchestrator's propagation server.
	connection *grpc.ClientConn

	// propagation is the generated Propagation API stub.
	propagation pb.PropagationClient
}

// New creates a Client for the Orchestrator API at the provided base URL and
// gRPC propagation address.
func New(baseURL *url.URL, propagationAddress string, labKey []byte, timeout time.Duration, logger *slog.Logger) (*Client, error) {
	host, port, err := net.SplitHostPort(propagationAddress)
	if err != nil {
		return nil, fmt.Errorf("invalid propagation address %q: %w", propagationAddress, err)
	}

	if host == "" || port == "" {
		return nil, fmt.Errorf("invalid propagation address %q: host and port must not be empty", propagationAddress)
	}

	connection, err := grpc.NewClient(propagationAddress,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(maxMessageSize),
			grpc.MaxCallSendMsgSize(maxMessageSize),
		))
	if err != nil {
		return nil, fmt.Errorf("failed to create propagation connection: %w", err)
	}

	return &Client{
		baseURL:     baseURL,
		httpClient:  &http.Client{Timeout: timeout},
		labKey:      labKey,
		logger:      logger,
		connection:  connection,
		propagation: pb.NewPropagationClient(connection),
	}, nil
}

// Close releases the gRPC propagation connection.
func (c *Client) Close() error {
	return c.connection.Close()
}

// Authenticate performs the two-step challenge-response authentication of the
// provided identity against the Orchestrator, returning the resulting
// session.
func (c *Client) Authenticate(ctx context.Context, id identity.Identity) (*daemon.Session, error) {
	if c.logger != nil {
		c.logger.Info("authentication challenge requested", "ip", id.IP)
	}

	var initiate schemas.InitiateOutputSchema
	if err := c.post(ctx, initiatePath, schemas.InitiateAuthenticationSchema{IP: id.IP}, &initiate); err != nil {
		return nil, err
	}

	signature := hex.EncodeToString(auth.Sign(c.labKey, auth.CanonicalPayload(id.IP, initiate.Nonce)))

	if c.logger != nil {
		c.logger.Info("authentication challenge completed", "ip", id.IP)
	}

	var authenticated schemas.AuthenticateOutputSchema
	if err := c.post(ctx, authenticatePath, schemas.AuthenticateMachineSchema{
		IP:        id.IP,
		MAC:       id.MAC,
		Signature: signature,
	}, &authenticated); err != nil {
		return nil, err
	}

	return &daemon.Session{
		Token:       authenticated.SessionToken,
		ExpiresAt:   authenticated.ExpiresAt,
		MachineID:   authenticated.Machine.ID,
		MachineName: authenticated.Machine.Name,
		IP:          id.IP,
		MAC:         id.MAC,
	}, nil
}

// ConnectPropagation opens the authenticated propagation stream at the
// Orchestrator, presenting the session token as bearer metadata. The returned
// stream delivers messages with Recv and returns acknowledgements with Send.
func (c *Client) ConnectPropagation(ctx context.Context, token string) (propagation.Stream, error) {
	streamCtx := metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+token)

	stream, err := c.propagation.Connect(streamCtx)
	if err != nil {
		return nil, fmt.Errorf("failed to open propagation stream: %w", err)
	}

	return &propagationStream{stream: stream}, nil
}

// propagationStream adapts the generated bidi stream to the transport-neutral
// propagation.Stream contract.
type propagationStream struct {
	stream pb.Propagation_ConnectClient
}

// Recv receives and converts one server message.
func (s *propagationStream) Recv() (*propagation.Message, error) {
	message, err := s.stream.Recv()
	if err != nil {
		return nil, err
	}

	return fromProtoServerMessage(message)
}

// Send delivers one acknowledgement over the stream.
func (s *propagationStream) Send(ack propagation.Ack) error {
	return s.stream.Send(toProtoAck(ack))
}

// CloseSend half-closes the stream, signalling the end of acknowledgements.
func (s *propagationStream) CloseSend() error {
	return s.stream.CloseSend()
}

// post sends a JSON request to the provided path of the Orchestrator API and
// decodes the response body into out.
func (c *Client) post(ctx context.Context, path string, payload any, out any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to encode request payload: %w", err)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL.JoinPath(path).String(), bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to build request: %w", err)
	}

	request.Header.Set("Content-Type", "application/json")

	response, err := c.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("%w: %w", errors.ErrOrchestratorUnreachable, err)
	}
	defer response.Body.Close()

	raw, err := io.ReadAll(response.Body)
	if err != nil {
		return fmt.Errorf("failed to read orchestrator response: %w", err)
	}

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return c.errorFromStatus(response.StatusCode, raw)
	}

	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("failed to decode orchestrator response: %w", err)
	}

	return nil
}

// errorFromStatus builds the error matching a failed Orchestrator response
// status, extracting the message from the error payload when present.
func (c *Client) errorFromStatus(status int, body []byte) error {
	var payload errorResponse
	_ = json.Unmarshal(body, &payload)

	if status == http.StatusUnauthorized {
		return fmt.Errorf("%w: %s", errors.ErrAuthenticationRejected, payload.Message)
	}

	return fmt.Errorf("orchestrator returned status %d: %s", status, payload.Message)
}

// errorResponse mirrors the Orchestrator's error payload.
type errorResponse struct {
	ErrorCode int    `json:"error_code"`
	Message   string `json:"message"`
}

// Heartbeat reports connectivity to the Orchestrator on behalf of the session
// token, returning nil when the Orchestrator accepts the beat.
func (c *Client) Heartbeat(ctx context.Context, token string) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL.JoinPath(heartbeatPath).String(), nil)
	if err != nil {
		return fmt.Errorf("failed to build request: %w", err)
	}

	request.Header.Set("Authorization", "Bearer "+token)

	response, err := c.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("%w: %w", errors.ErrOrchestratorUnreachable, err)
	}
	defer response.Body.Close()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		raw, err := io.ReadAll(response.Body)
		if err != nil {
			return fmt.Errorf("failed to read orchestrator response: %w", err)
		}

		return c.errorFromStatus(response.StatusCode, raw)
	}

	return nil
}
