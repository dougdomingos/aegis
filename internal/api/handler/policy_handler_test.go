package handler_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/go-chi/chi/v5"

	"dougdomingos.com/aegis/internal/api/handler"
	"dougdomingos.com/aegis/internal/domain"
	policyErrors "dougdomingos.com/aegis/internal/errors"
	"dougdomingos.com/aegis/internal/schemas"
)

// ============================================================================
// CreatePolicy (POST /policies)
// ============================================================================

func TestPolicyHandler_Create_WithValidPayload_ReturnsStatus201(t *testing.T) {
	mock := &mockPolicyService{
		createFn: func(_ context.Context, p schemas.CreatePolicySchema) (*schemas.PolicyOutputSchema, error) {
			return &schemas.PolicyOutputSchema{Name: p.Name, Type: p.Type, Version: 1}, nil
		},
	}

	rec := performRequest(t, setupPolicyRouter(t, mock), http.MethodPost, "/policies", schemas.CreatePolicySchema{Name: "Test Policy", Type: domain.WhitelistPolicyType})
	assertStatusCode(t, rec, http.StatusCreated)
}

func TestPolicyHandler_Create_WithInvalidPayload_ReturnsStatus400(t *testing.T) {
	mock := &mockPolicyService{
		createFn: func(_ context.Context, _ schemas.CreatePolicySchema) (*schemas.PolicyOutputSchema, error) {
			return nil, policyErrors.ErrPolicyNameRequired
		},
	}

	rec := performRequest(t, setupPolicyRouter(t, mock), http.MethodPost, "/policies", schemas.CreatePolicySchema{Name: ""})
	assertStatusCode(t, rec, http.StatusBadRequest)
}

func TestPolicyHandler_Create_WithDuplicateName_ReturnsStatus409(t *testing.T) {
	mock := &mockPolicyService{
		createFn: func(_ context.Context, _ schemas.CreatePolicySchema) (*schemas.PolicyOutputSchema, error) {
			return nil, policyErrors.ErrPolicyNameAlreadyExists
		},
	}

	rec := performRequest(t, setupPolicyRouter(t, mock), http.MethodPost, "/policies", schemas.CreatePolicySchema{Name: "Test Policy", Type: domain.WhitelistPolicyType})
	assertStatusCode(t, rec, http.StatusConflict)
}

func TestPolicyHandler_Create_WithEmptyType_ReturnsStatus400(t *testing.T) {
	mock := &mockPolicyService{
		createFn: func(_ context.Context, _ schemas.CreatePolicySchema) (*schemas.PolicyOutputSchema, error) {
			return nil, policyErrors.ErrPolicyTypeRequired
		},
	}

	rec := performRequest(t, setupPolicyRouter(t, mock), http.MethodPost, "/policies", schemas.CreatePolicySchema{Name: "Test Policy"})
	assertStatusCode(t, rec, http.StatusBadRequest)
}

func TestPolicyHandler_Create_WithUnsupportedType_ReturnsStatus400(t *testing.T) {
	mock := &mockPolicyService{
		createFn: func(_ context.Context, _ schemas.CreatePolicySchema) (*schemas.PolicyOutputSchema, error) {
			return nil, policyErrors.ErrInvalidPolicyType
		},
	}

	rec := performRequest(t, setupPolicyRouter(t, mock), http.MethodPost, "/policies", schemas.CreatePolicySchema{Name: "Test Policy", Type: "BOGUS"})
	assertStatusCode(t, rec, http.StatusBadRequest)
}

func TestPolicyHandler_Create_WithMalformedPayload_ReturnsStatus400(t *testing.T) {
	mock := &mockPolicyService{
		createFn: func(_ context.Context, _ schemas.CreatePolicySchema) (*schemas.PolicyOutputSchema, error) {
			return nil, nil
		},
	}

	rec := performRequest(t, setupPolicyRouter(t, mock), http.MethodPost, "/policies", "invalid json")
	assertStatusCode(t, rec, http.StatusBadRequest)
}

// ============================================================================
// GetPolicyByID (GET /policies/{id})
// ============================================================================

func TestPolicyHandler_GetByID_WithExistentPolicy_ReturnsStatus200(t *testing.T) {
	reqUrl := fmt.Sprintf("/policies/%d", 1)
	mock := &mockPolicyService{
		getByIDFn: func(_ context.Context, p schemas.GetPolicyByIDSchema) (*schemas.PolicyOutputSchema, error) {
			return &schemas.PolicyOutputSchema{ID: p.ID}, nil
		},
	}

	rec := performRequest(t, setupPolicyRouter(t, mock), http.MethodGet, reqUrl, nil)
	assertStatusCode(t, rec, http.StatusOK)
}

func TestPolicyHandler_GetByID_WithNonExistentPolicy_ReturnsStatus404(t *testing.T) {
	mock := &mockPolicyService{
		getByIDFn: func(_ context.Context, _ schemas.GetPolicyByIDSchema) (*schemas.PolicyOutputSchema, error) {
			return nil, policyErrors.ErrPolicyNotFound
		},
	}

	rec := performRequest(t, setupPolicyRouter(t, mock), http.MethodGet, "/policies/999", nil)
	assertStatusCode(t, rec, http.StatusNotFound)
}

func TestPolicyHandler_GetByID_WithNilResult_ReturnsStatus404(t *testing.T) {
	mock := &mockPolicyService{
		getByIDFn: func(_ context.Context, _ schemas.GetPolicyByIDSchema) (*schemas.PolicyOutputSchema, error) {
			return nil, nil
		},
	}

	rec := performRequest(t, setupPolicyRouter(t, mock), http.MethodGet, "/policies/999", nil)
	assertStatusCode(t, rec, http.StatusNotFound)
}

func TestPolicyHandler_GetByID_WithInvalidID_ReturnsStatus400(t *testing.T) {
	mock := &mockPolicyService{
		getByIDFn: func(_ context.Context, _ schemas.GetPolicyByIDSchema) (*schemas.PolicyOutputSchema, error) {
			return nil, nil
		},
	}

	rec := performRequest(t, setupPolicyRouter(t, mock), http.MethodGet, "/policies/abc", nil)
	assertStatusCode(t, rec, http.StatusBadRequest)
}

// ============================================================================
// ListPolicies (GET /policies)
// ============================================================================

func TestPolicyHandler_List_WithSeededPolicies_ReturnsStatus200(t *testing.T) {
	mock := &mockPolicyService{
		listFn: func(_ context.Context) ([]schemas.PolicyOutputSchema, error) {
			return []schemas.PolicyOutputSchema{
				{ID: 1, Name: "Policy A", Version: 1},
				{ID: 2, Name: "Policy B", Version: 1},
			}, nil
		},
	}

	rec := performRequest(t, setupPolicyRouter(t, mock), http.MethodGet, "/policies", nil)
	assertStatusCode(t, rec, http.StatusOK)
}

func TestPolicyHandler_List_WithNilResult_ReturnsStatus200(t *testing.T) {
	mock := &mockPolicyService{
		listFn: func(_ context.Context) ([]schemas.PolicyOutputSchema, error) {
			return nil, nil
		},
	}

	rec := performRequest(t, setupPolicyRouter(t, mock), http.MethodGet, "/policies", nil)
	assertStatusCode(t, rec, http.StatusOK)
}

func TestPolicyHandler_List_WhenServiceFails_ReturnsStatus500(t *testing.T) {
	mock := &mockPolicyService{
		listFn: func(_ context.Context) ([]schemas.PolicyOutputSchema, error) {
			return nil, policyErrors.ErrPolicyNotFound
		},
	}

	rec := performRequest(t, setupPolicyRouter(t, mock), http.MethodGet, "/policies", nil)
	assertStatusCode(t, rec, http.StatusInternalServerError)
}

// ============================================================================
// UpdatePolicy (PATCH /policies/{id})
// ============================================================================

func TestPolicyHandler_Update_WithValidPayload_ReturnsStatus200(t *testing.T) {
	mock := &mockPolicyService{
		updateFn: func(_ context.Context, p schemas.UpdatePolicySchema) (*schemas.PolicyOutputSchema, error) {
			return &schemas.PolicyOutputSchema{ID: p.ID, Name: p.Name}, nil
		},
	}

	rec := performRequest(t, setupPolicyRouter(t, mock), http.MethodPatch, "/policies/1", schemas.UpdatePolicySchema{Name: "New Name"})
	assertStatusCode(t, rec, http.StatusOK)
}

func TestPolicyHandler_Update_WithNonExistentPolicy_ReturnsStatus404(t *testing.T) {
	mock := &mockPolicyService{
		updateFn: func(_ context.Context, _ schemas.UpdatePolicySchema) (*schemas.PolicyOutputSchema, error) {
			return nil, policyErrors.ErrPolicyNotFound
		},
	}

	rec := performRequest(t, setupPolicyRouter(t, mock), http.MethodPatch, "/policies/999", schemas.UpdatePolicySchema{Name: "New Name"})
	assertStatusCode(t, rec, http.StatusNotFound)
}

func TestPolicyHandler_Update_WithEmptyName_ReturnsStatus400(t *testing.T) {
	mock := &mockPolicyService{
		updateFn: func(_ context.Context, _ schemas.UpdatePolicySchema) (*schemas.PolicyOutputSchema, error) {
			return nil, policyErrors.ErrPolicyNameRequired
		},
	}

	rec := performRequest(t, setupPolicyRouter(t, mock), http.MethodPatch, "/policies/1", schemas.UpdatePolicySchema{Name: ""})
	assertStatusCode(t, rec, http.StatusBadRequest)
}

func TestPolicyHandler_Update_WithDuplicateName_ReturnsStatus409(t *testing.T) {
	mock := &mockPolicyService{
		updateFn: func(_ context.Context, _ schemas.UpdatePolicySchema) (*schemas.PolicyOutputSchema, error) {
			return nil, policyErrors.ErrPolicyNameAlreadyExists
		},
	}

	rec := performRequest(t, setupPolicyRouter(t, mock), http.MethodPatch, "/policies/1", schemas.UpdatePolicySchema{Name: "Duplicate Name"})
	assertStatusCode(t, rec, http.StatusConflict)
}

func TestPolicyHandler_Update_WithMalformedPayload_ReturnsStatus400(t *testing.T) {
	mock := &mockPolicyService{
		updateFn: func(_ context.Context, _ schemas.UpdatePolicySchema) (*schemas.PolicyOutputSchema, error) {
			return nil, nil
		},
	}

	rec := performRequest(t, setupPolicyRouter(t, mock), http.MethodPatch, "/policies/1", "invalid json")
	assertStatusCode(t, rec, http.StatusBadRequest)
}

func TestPolicyHandler_Update_WithInvalidID_ReturnsStatus400(t *testing.T) {
	mock := &mockPolicyService{
		updateFn: func(_ context.Context, _ schemas.UpdatePolicySchema) (*schemas.PolicyOutputSchema, error) {
			return nil, nil
		},
	}

	rec := performRequest(t, setupPolicyRouter(t, mock), http.MethodPatch, "/policies/abc", schemas.UpdatePolicySchema{Name: "New Name"})
	assertStatusCode(t, rec, http.StatusBadRequest)
}

// ============================================================================
// RemovePolicy (DELETE /policies/{id})
// ============================================================================

func TestPolicyHandler_Delete_WithExistentPolicy_ReturnsStatus204(t *testing.T) {
	mock := &mockPolicyService{
		removeFn: func(_ context.Context, _ schemas.RemovePolicySchema) error {
			return nil
		},
	}

	rec := performRequest(t, setupPolicyRouter(t, mock), http.MethodDelete, "/policies/1", nil)
	assertStatusCode(t, rec, http.StatusNoContent)
}

func TestPolicyHandler_Delete_WithNonExistentPolicy_ReturnsStatus404(t *testing.T) {
	mock := &mockPolicyService{
		removeFn: func(_ context.Context, _ schemas.RemovePolicySchema) error {
			return policyErrors.ErrPolicyNotFound
		},
	}

	rec := performRequest(t, setupPolicyRouter(t, mock), http.MethodDelete, "/policies/999", nil)
	assertStatusCode(t, rec, http.StatusNotFound)
}

func TestPolicyHandler_Delete_WithInvalidID_ReturnsStatus400(t *testing.T) {
	mock := &mockPolicyService{
		removeFn: func(_ context.Context, _ schemas.RemovePolicySchema) error {
			return nil
		},
	}

	rec := performRequest(t, setupPolicyRouter(t, mock), http.MethodDelete, "/policies/abc", nil)
	assertStatusCode(t, rec, http.StatusBadRequest)
}

// ============================================================================
// Helpers
// ============================================================================

// setupPolicyRouter builds a router wired with a policy handler backed by the
// provided service mock.
func setupPolicyRouter(t *testing.T, svc handler.PolicyServiceInterface) *chi.Mux {
	t.Helper()

	r := chi.NewRouter()
	h := handler.NewPolicyHandler(svc)
	h.RegisterRoutes(r)

	return r
}
