package handler_test

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	machineErrors "dougdomingos.com/aegis/internal/errors"
	"dougdomingos.com/aegis/internal/schemas"
)

// ============================================================================
// ListMachines (GET /machines)
// ============================================================================

func TestMachineHandler_ListMachines_WithMachines_ReturnsStatus200(t *testing.T) {
	mock := &mockMachineService{
		listFn: func(_ context.Context) ([]schemas.MachineOutputSchema, error) {
			return []schemas.MachineOutputSchema{
				machineOutput(1, "lcc1-011", "10.0.1.11"),
				machineOutput(2, "lcc1-012", "10.0.1.12"),
			}, nil
		},
	}

	rec := performRequest(t, setupMachineRouter(t, mock), http.MethodGet, "/machines", nil)
	assertStatusCode(t, rec, http.StatusOK)
}

func TestMachineHandler_ListMachines_WithoutMachines_ReturnsStatus200(t *testing.T) {
	mock := &mockMachineService{
		listFn: func(_ context.Context) ([]schemas.MachineOutputSchema, error) {
			return []schemas.MachineOutputSchema{}, nil
		},
	}

	rec := performRequest(t, setupMachineRouter(t, mock), http.MethodGet, "/machines", nil)
	assertStatusCode(t, rec, http.StatusOK)
}

func TestMachineHandler_ListMachines_WhenServiceFails_ReturnsStatus500(t *testing.T) {
	mock := &mockMachineService{
		listFn: func(_ context.Context) ([]schemas.MachineOutputSchema, error) {
			return nil, errors.New("failed to list machines")
		},
	}

	rec := performRequest(t, setupMachineRouter(t, mock), http.MethodGet, "/machines", nil)
	assertStatusCode(t, rec, http.StatusInternalServerError)
}

// ============================================================================
// Initiate (POST /machines/authenticate/init)
// ============================================================================

func TestMachineHandler_Initiate_WithValidIP_ReturnsStatus200(t *testing.T) {
	payload := schemas.InitiateAuthenticationSchema{IP: "10.0.1.11"}
	mock := &mockMachineService{
		initiateFn: func(_ context.Context, p schemas.InitiateAuthenticationSchema) (*schemas.InitiateOutputSchema, error) {
			return &schemas.InitiateOutputSchema{Nonce: "nonce", ExpiresAt: time.Now()}, nil
		},
	}

	rec := performRequest(t, setupMachineRouter(t, mock), http.MethodPost, "/machines/authenticate/init", payload)
	assertStatusCode(t, rec, http.StatusOK)
}

func TestMachineHandler_Initiate_WithoutIP_ReturnsStatus400(t *testing.T) {
	payload := schemas.InitiateAuthenticationSchema{}
	mock := &mockMachineService{
		initiateFn: func(_ context.Context, p schemas.InitiateAuthenticationSchema) (*schemas.InitiateOutputSchema, error) {
			return nil, machineErrors.ErrMachineIPRequired
		},
	}

	rec := performRequest(t, setupMachineRouter(t, mock), http.MethodPost, "/machines/authenticate/init", payload)
	assertStatusCode(t, rec, http.StatusBadRequest)
}

func TestMachineHandler_Initiate_WithMalformedPayload_ReturnsStatus400(t *testing.T) {
	mock := &mockMachineService{
		initiateFn: func(_ context.Context, p schemas.InitiateAuthenticationSchema) (*schemas.InitiateOutputSchema, error) {
			return nil, errors.New("malformed payload")
		},
	}

	rec := performRequest(t, setupMachineRouter(t, mock), http.MethodPost, "/machines/authenticate/init", "invalid json")
	assertStatusCode(t, rec, http.StatusBadRequest)
}

func TestMachineHandler_Initiate_WhenServiceFails_ReturnsStatus500(t *testing.T) {
	payload := schemas.InitiateAuthenticationSchema{IP: "10.0.1.11"}
	mock := &mockMachineService{
		initiateFn: func(_ context.Context, p schemas.InitiateAuthenticationSchema) (*schemas.InitiateOutputSchema, error) {
			return nil, errors.New("failed to initiate authentication")
		},
	}

	rec := performRequest(t, setupMachineRouter(t, mock), http.MethodPost, "/machines/authenticate/init", payload)
	assertStatusCode(t, rec, http.StatusInternalServerError)
}

// ============================================================================
// Authenticate (POST /machines/authenticate)
// ============================================================================

func TestMachineHandler_Authenticate_WithValidCredentials_ReturnsStatus200(t *testing.T) {
	payload := schemas.AuthenticateMachineSchema{IP: "10.0.1.11", Signature: "signature"}
	mock := &mockMachineService{
		authenticateFn: func(_ context.Context, p schemas.AuthenticateMachineSchema) (*schemas.AuthenticateOutputSchema, error) {
			return &schemas.AuthenticateOutputSchema{
				SessionToken: "token",
				Machine:      machineOutput(1, "lcc1-011", p.IP),
			}, nil
		},
	}

	rec := performRequest(t, setupMachineRouter(t, mock), http.MethodPost, "/machines/authenticate", payload)
	assertStatusCode(t, rec, http.StatusOK)
}

func TestMachineHandler_Authenticate_WithInvalidCredentials_ReturnsStatus401(t *testing.T) {
	payload := schemas.AuthenticateMachineSchema{IP: "10.0.1.99", Signature: "signature"}
	mock := &mockMachineService{
		authenticateFn: func(_ context.Context, p schemas.AuthenticateMachineSchema) (*schemas.AuthenticateOutputSchema, error) {
			return nil, machineErrors.ErrInvalidCredentials
		},
	}

	rec := performRequest(t, setupMachineRouter(t, mock), http.MethodPost, "/machines/authenticate", payload)
	assertStatusCode(t, rec, http.StatusUnauthorized)
}

func TestMachineHandler_Authenticate_WithoutIP_ReturnsStatus400(t *testing.T) {
	payload := schemas.AuthenticateMachineSchema{Signature: "signature"}
	mock := &mockMachineService{
		authenticateFn: func(_ context.Context, p schemas.AuthenticateMachineSchema) (*schemas.AuthenticateOutputSchema, error) {
			return nil, machineErrors.ErrMachineIPRequired
		},
	}

	rec := performRequest(t, setupMachineRouter(t, mock), http.MethodPost, "/machines/authenticate", payload)
	assertStatusCode(t, rec, http.StatusBadRequest)
}

func TestMachineHandler_Authenticate_WithMalformedPayload_ReturnsStatus400(t *testing.T) {
	mock := &mockMachineService{
		authenticateFn: func(_ context.Context, p schemas.AuthenticateMachineSchema) (*schemas.AuthenticateOutputSchema, error) {
			return nil, errors.New("malformed payload")
		},
	}

	rec := performRequest(t, setupMachineRouter(t, mock), http.MethodPost, "/machines/authenticate", "invalid json")
	assertStatusCode(t, rec, http.StatusBadRequest)
}

// ============================================================================
// Heartbeat (POST /machines/heartbeat)
// ============================================================================

func TestMachineHandler_Heartbeat_ValidBearerToken_ReturnsNoContent(t *testing.T) {
	mock := &mockMachineService{
		heartbeatFn: func(_ context.Context, token string) error {
			return nil
		},
	}

	rec := performBearerRequest(t, setupMachineRouter(t, mock), "session-token")
	assertStatusCode(t, rec, http.StatusNoContent)
}

func TestMachineHandler_Heartbeat_InvalidToken_ReturnsUnauthorized(t *testing.T) {
	scenarios := map[string]struct {
		header    string
		expectErr error
	}{
		"expired token":  {header: "Bearer expired-token", expectErr: machineErrors.ErrSessionTokenInvalid},
		"missing header": {header: "", expectErr: machineErrors.ErrSessionTokenInvalid},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			mock := &mockMachineService{
				heartbeatFn: func(_ context.Context, token string) error {
					return scenario.expectErr
				},
			}

			rec, req := buildTestRequest(http.MethodPost, "/machines/heartbeat", nil)
			if scenario.header != "" {
				req.Header.Set("Authorization", scenario.header)
			}

			setupMachineRouter(t, mock).ServeHTTP(rec, req)
			assertStatusCode(t, rec, http.StatusUnauthorized)
		})
	}
}
