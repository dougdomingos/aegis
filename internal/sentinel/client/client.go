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
	"net/http"
	"net/url"
	"time"

	"dougdomingos.com/aegis/internal/auth"
	"dougdomingos.com/aegis/internal/errors"
	"dougdomingos.com/aegis/internal/schemas"
	"dougdomingos.com/aegis/internal/sentinel/daemon"
	"dougdomingos.com/aegis/internal/sentinel/identity"
)

const (
	// initiatePath is the path of the authentication challenge endpoint.
	initiatePath = "/machines/authenticate/init"

	// authenticatePath is the path of the authentication endpoint.
	authenticatePath = "/machines/authenticate"

	// heartbeatPath is the path of the connectivity heartbeat endpoint.
	heartbeatPath = "/machines/heartbeat"
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
}

// New creates a Client for the Orchestrator API at the provided base URL.
func New(baseURL *url.URL, labKey []byte, timeout time.Duration, logger *slog.Logger) *Client {
	return &Client{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: timeout},
		labKey:     labKey,
		logger:     logger,
	}
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
		MachineName: authenticated.Machine.Name,
	}, nil
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
