package handler_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	machineErrors "dougdomingos.com/aegis/internal/common/errors"
	"dougdomingos.com/aegis/internal/common/schemas"
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
// Import (POST /machines/import)
// ============================================================================

func TestMachineHandler_Import_WithValidCSV_ReturnsStatus200(t *testing.T) {
	mock := &mockMachineService{
		importMachinesFn: func(_ context.Context, _ io.Reader) (*schemas.MachineImportOutputSchema, error) {
			return &schemas.MachineImportOutputSchema{MachinesCreated: 1, GroupsCreated: 1, MachinesLinked: 1}, nil
		},
	}

	rec := performCSVRequest(t, setupMachineRouter(t, mock), "/machines/import", "ip,name,lab\n10.0.1.11,lcc1-011,lcc1\n")
	assertStatusCode(t, rec, http.StatusOK)
}

func TestMachineHandler_Import_WithInvalidCSV_ReturnsStatus400(t *testing.T) {
	mock := &mockMachineService{
		importMachinesFn: func(_ context.Context, _ io.Reader) (*schemas.MachineImportOutputSchema, error) {
			return nil, machineErrors.ErrInvalidMachinesCSV
		},
	}

	rec := performCSVRequest(t, setupMachineRouter(t, mock), "/machines/import", "ip,name\n10.0.1.11,lcc1-011\n")
	assertStatusCode(t, rec, http.StatusBadRequest)
}

func TestMachineHandler_Import_WithOversizedBody_ReturnsStatus413(t *testing.T) {
	mock := &mockMachineService{
		importMachinesFn: func(_ context.Context, document io.Reader) (*schemas.MachineImportOutputSchema, error) {
			_, err := io.ReadAll(document)
			return nil, fmt.Errorf("%w: %w", machineErrors.ErrInvalidMachinesCSV, err)
		},
	}

	rec := performCSVRequest(t, setupMachineRouter(t, mock), "/machines/import", strings.Repeat("a", 2<<20))
	assertStatusCode(t, rec, http.StatusRequestEntityTooLarge)
}

func TestMachineHandler_Import_WhenServiceFails_ReturnsStatus500(t *testing.T) {
	mock := &mockMachineService{
		importMachinesFn: func(_ context.Context, _ io.Reader) (*schemas.MachineImportOutputSchema, error) {
			return nil, errors.New("failed to import machines")
		},
	}

	rec := performCSVRequest(t, setupMachineRouter(t, mock), "/machines/import", "ip,name,lab\n10.0.1.11,lcc1-011,lcc1\n")
	assertStatusCode(t, rec, http.StatusInternalServerError)
}

// ============================================================================
// Delete (DELETE /machines/{id})
// ============================================================================

func TestMachineHandler_Delete_WithExistingMachine_ReturnsStatus204(t *testing.T) {
	mock := &mockMachineService{
		removeMachineFn: func(_ context.Context, _ int64) error { return nil },
	}

	rec := performRequest(t, setupMachineRouter(t, mock), http.MethodDelete, "/machines/1", nil)
	assertStatusCode(t, rec, http.StatusNoContent)
}

func TestMachineHandler_Delete_WithInvalidID_ReturnsStatus400(t *testing.T) {
	mock := &mockMachineService{
		removeMachineFn: func(_ context.Context, _ int64) error { return nil },
	}

	rec := performRequest(t, setupMachineRouter(t, mock), http.MethodDelete, "/machines/abc", nil)
	assertStatusCode(t, rec, http.StatusBadRequest)
}

func TestMachineHandler_Delete_WithMissingMachine_ReturnsStatus404(t *testing.T) {
	mock := &mockMachineService{
		removeMachineFn: func(_ context.Context, _ int64) error { return machineErrors.ErrMachineNotFound },
	}

	rec := performRequest(t, setupMachineRouter(t, mock), http.MethodDelete, "/machines/999", nil)
	assertStatusCode(t, rec, http.StatusNotFound)
}

func TestMachineHandler_Delete_WhenServiceFails_ReturnsStatus500(t *testing.T) {
	mock := &mockMachineService{
		removeMachineFn: func(_ context.Context, _ int64) error { return errors.New("failed to remove machine") },
	}

	rec := performRequest(t, setupMachineRouter(t, mock), http.MethodDelete, "/machines/1", nil)
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
		wantToken string
		expectErr error
	}{
		"expired token":  {header: "Bearer expired-token", wantToken: "expired-token", expectErr: machineErrors.ErrSessionTokenInvalid},
		"missing header": {header: "", wantToken: "", expectErr: machineErrors.ErrSessionTokenInvalid},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			var receivedToken string

			mock := &mockMachineService{
				heartbeatFn: func(_ context.Context, token string) error {
					receivedToken = token

					return scenario.expectErr
				},
			}

			rec, req := buildTestRequest(http.MethodPost, "/machines/heartbeat", nil)
			if scenario.header != "" {
				req.Header.Set("Authorization", scenario.header)
			}

			setupMachineRouter(t, mock).ServeHTTP(rec, req)
			assertStatusCode(t, rec, http.StatusUnauthorized)

			if receivedToken != scenario.wantToken {
				t.Errorf("token forwarded to the service = %q, want %q", receivedToken, scenario.wantToken)
			}
		})
	}
}

// ============================================================================
// GetPropagationStatus
// ============================================================================

func TestMachineHandler_GetPropagationStatus_WithKnownMachine_ReturnsOK(t *testing.T) {
	mock := &mockMachineService{
		getPropagationStatusFn: func(_ context.Context, machineID int64) (*schemas.PropagationOutputSchema, error) {
			return &schemas.PropagationOutputSchema{
				Sequence:   4,
				Kind:       "PACKAGE",
				Status:     "ACKED",
				PolicyID:   &machineID,
				PolicyName: "lab-whitelist",
			}, nil
		},
	}

	rec := performRequest(t, setupMachineRouter(t, mock), http.MethodGet, "/machines/7/propagation", nil)
	assertStatusCode(t, rec, http.StatusOK)
}

func TestMachineHandler_GetPropagationStatus_WithPendingMachine_ReturnsOK(t *testing.T) {
	mock := &mockMachineService{
		getPropagationStatusFn: func(_ context.Context, _ int64) (*schemas.PropagationOutputSchema, error) {
			return &schemas.PropagationOutputSchema{Status: "PENDING"}, nil
		},
	}

	rec := performRequest(t, setupMachineRouter(t, mock), http.MethodGet, "/machines/7/propagation", nil)
	assertStatusCode(t, rec, http.StatusOK)
}

func TestMachineHandler_GetPropagationStatus_WithInvalidID_ReturnsBadRequest(t *testing.T) {
	mock := &mockMachineService{
		getPropagationStatusFn: func(_ context.Context, _ int64) (*schemas.PropagationOutputSchema, error) {
			t.Error("the service must not be called for an invalid machine id")
			return nil, nil
		},
	}

	rec := performRequest(t, setupMachineRouter(t, mock), http.MethodGet, "/machines/abc/propagation", nil)
	assertStatusCode(t, rec, http.StatusBadRequest)
}

func TestMachineHandler_GetPropagationStatus_WithMissingMachine_ReturnsNotFound(t *testing.T) {
	mock := &mockMachineService{
		getPropagationStatusFn: func(_ context.Context, _ int64) (*schemas.PropagationOutputSchema, error) {
			return nil, machineErrors.ErrMachineNotFound
		},
	}

	rec := performRequest(t, setupMachineRouter(t, mock), http.MethodGet, "/machines/7/propagation", nil)
	assertStatusCode(t, rec, http.StatusNotFound)
}

func TestMachineHandler_GetPropagationStatus_WhenServiceFails_ReturnsInternalServerError(t *testing.T) {
	mock := &mockMachineService{
		getPropagationStatusFn: func(_ context.Context, _ int64) (*schemas.PropagationOutputSchema, error) {
			return nil, errors.New("propagation store unavailable")
		},
	}

	rec := performRequest(t, setupMachineRouter(t, mock), http.MethodGet, "/machines/7/propagation", nil)
	assertStatusCode(t, rec, http.StatusInternalServerError)
}

// performCSVRequest sends a POST with the provided raw CSV body to the endpoint
// and returns the recorded response.
func performCSVRequest(t *testing.T, router http.Handler, endpoint, body string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, endpoint, strings.NewReader(body))
	req.Header.Set("Content-Type", "text/csv")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	return rec
}
