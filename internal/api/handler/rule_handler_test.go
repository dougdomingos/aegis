package handler_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/go-chi/chi/v5"

	"dougdomingos.com/aegis/internal/api/handler"
	"dougdomingos.com/aegis/internal/domain"
	ruleErrors "dougdomingos.com/aegis/internal/errors"
	"dougdomingos.com/aegis/internal/schemas"
)

// ============================================================================
// CreateRule (POST /policies/{policyID}/rules)
// ============================================================================

func TestRuleHandler_Create_WithValidPayload_ReturnsStatus201(t *testing.T) {
	payload := schemas.CreateRuleSchema{
		Type:  domain.DomainRuleType,
		Value: "test.com",
	}
	mock := &mockRuleService{
		createFn: func(_ context.Context, p schemas.CreateRuleSchema) (*schemas.RuleOutputSchema, error) {
			return &schemas.RuleOutputSchema{
				PolicyID: p.PolicyID,
				Type:     p.Type,
				Value:    p.Value,
			}, nil
		},
	}

	rec := performRequest(t, setupRuleRouter(t, mock), http.MethodPost, "/policies/1/rules", payload)
	assertStatusCode(t, rec, http.StatusCreated)
}

func TestRuleHandler_Create_WithInexistentPolicy_ReturnsStatus404(t *testing.T) {
	mock := &mockRuleService{
		createFn: func(_ context.Context, _ schemas.CreateRuleSchema) (*schemas.RuleOutputSchema, error) {
			return nil, ruleErrors.ErrPolicyNotFound
		},
	}

	rec := performRequest(t, setupRuleRouter(t, mock), http.MethodPost, "/policies/9999/rules", nil)
	assertStatusCode(t, rec, http.StatusNotFound)
}

func TestRuleHandler_Create_WithInvalidPolicyID_ReturnsStatus400(t *testing.T) {
	mock := &mockRuleService{
		createFn: func(_ context.Context, _ schemas.CreateRuleSchema) (*schemas.RuleOutputSchema, error) {
			return nil, nil
		},
	}

	rec := performRequest(t, setupRuleRouter(t, mock), http.MethodPost, "/policies/abc/rules", nil)
	assertStatusCode(t, rec, http.StatusBadRequest)
}

func TestRuleHandler_Create_WithInvalidPayload_ReturnsStatus400(t *testing.T) {
	testCases := map[string]struct {
		payload    schemas.CreateRuleSchema
		serviceErr error
	}{
		"Empty type": {
			payload: schemas.CreateRuleSchema{
				Value: "test.com",
			},
			serviceErr: ruleErrors.ErrRuleTypeRequired,
		},
		"Empty value": {
			payload: schemas.CreateRuleSchema{
				Type: domain.DomainRuleType,
			},
			serviceErr: ruleErrors.ErrRuleValueRequired,
		},
		"Malformed value": {
			payload: schemas.CreateRuleSchema{
				Type:  domain.DomainRuleType,
				Value: "thisisnotadomain",
			},
			serviceErr: ruleErrors.ErrMalformedRuleValue,
		},
		"Protocol for domain rule": {
			payload: schemas.CreateRuleSchema{
				Type:     domain.DomainRuleType,
				Value:    "test.com",
				Protocol: new(string),
			},
			serviceErr: ruleErrors.ErrInvalidFieldForRuleType,
		},
		"Invalid port": {
			payload: schemas.CreateRuleSchema{
				Type:  domain.IPRuleType,
				Value: "10.0.0.1",
				Port:  new(int),
			},
			serviceErr: ruleErrors.ErrInvalidRulePortValue,
		},
	}

	for testName, tt := range testCases {
		t.Run(testName, func(t *testing.T) {
			mock := &mockRuleService{
				createFn: func(_ context.Context, _ schemas.CreateRuleSchema) (*schemas.RuleOutputSchema, error) {
					return nil, tt.serviceErr
				},
			}

			rec := performRequest(t, setupRuleRouter(t, mock), http.MethodPost, "/policies/1/rules", tt.payload)
			assertStatusCode(t, rec, http.StatusBadRequest)
		})
	}
}

func TestRuleHandler_Create_WithMalformedPayload_ReturnsStatus400(t *testing.T) {
	mock := &mockRuleService{
		createFn: func(_ context.Context, _ schemas.CreateRuleSchema) (*schemas.RuleOutputSchema, error) {
			return nil, nil
		},
	}

	rec := performRequest(t, setupRuleRouter(t, mock), http.MethodPost, "/policies/1/rules", "invalid json")
	assertStatusCode(t, rec, http.StatusBadRequest)
}

// ============================================================================
// GetRuleByID (GET /policies/{policyID}/rules/{ruleID})
// ============================================================================

func TestRuleHandler_GetByID_WithExistentRule_ReturnsStatus200(t *testing.T) {
	reqUrl := fmt.Sprintf("/policies/%d/rules/%d", 1, 1)
	mock := &mockRuleService{
		getByIDFn: func(_ context.Context, p schemas.GetRuleByIDSchema) (*schemas.RuleOutputSchema, error) {
			return &schemas.RuleOutputSchema{ID: p.ID, PolicyID: p.PolicyID}, nil
		},
	}

	rec := performRequest(t, setupRuleRouter(t, mock), http.MethodGet, reqUrl, nil)
	assertStatusCode(t, rec, http.StatusOK)
}

func TestRuleHandler_GetByID_WithNonExistentRule_ReturnsStatus404(t *testing.T) {
	mock := &mockRuleService{
		getByIDFn: func(_ context.Context, _ schemas.GetRuleByIDSchema) (*schemas.RuleOutputSchema, error) {
			return nil, ruleErrors.ErrRuleNotFound
		},
	}

	rec := performRequest(t, setupRuleRouter(t, mock), http.MethodGet, "/policies/1/rules/9999", nil)
	assertStatusCode(t, rec, http.StatusNotFound)
}

func TestRuleHandler_GetByID_WithNilResult_ReturnsStatus404(t *testing.T) {
	mock := &mockRuleService{
		getByIDFn: func(_ context.Context, _ schemas.GetRuleByIDSchema) (*schemas.RuleOutputSchema, error) {
			return nil, nil
		},
	}

	rec := performRequest(t, setupRuleRouter(t, mock), http.MethodGet, "/policies/1/rules/9999", nil)
	assertStatusCode(t, rec, http.StatusNotFound)
}

func TestRuleHandler_GetByID_WithInvalidRuleID_ReturnsStatus400(t *testing.T) {
	mock := &mockRuleService{
		getByIDFn: func(_ context.Context, _ schemas.GetRuleByIDSchema) (*schemas.RuleOutputSchema, error) {
			return nil, nil
		},
	}

	rec := performRequest(t, setupRuleRouter(t, mock), http.MethodGet, "/policies/1/rules/abc", nil)
	assertStatusCode(t, rec, http.StatusBadRequest)
}

func TestRuleHandler_GetByID_WithInvalidPolicyID_ReturnsStatus400(t *testing.T) {
	mock := &mockRuleService{
		getByIDFn: func(_ context.Context, _ schemas.GetRuleByIDSchema) (*schemas.RuleOutputSchema, error) {
			return nil, nil
		},
	}

	rec := performRequest(t, setupRuleRouter(t, mock), http.MethodGet, "/policies/abc/rules/1", nil)
	assertStatusCode(t, rec, http.StatusBadRequest)
}

// ============================================================================
// ListRules (GET /policies/{policyID}/rules)
// ============================================================================

func TestRuleHandler_List_WithSeededRules_ReturnsStatus200(t *testing.T) {
	mock := &mockRuleService{
		listFn: func(_ context.Context, _ schemas.ListRulesSchema) ([]schemas.RuleOutputSchema, error) {
			return []schemas.RuleOutputSchema{
				{ID: 1, PolicyID: 1, Type: domain.DomainRuleType, Value: "test.com"},
				{ID: 2, PolicyID: 1, Type: domain.IPRuleType, Value: "10.0.0.1"},
			}, nil
		},
	}

	rec := performRequest(t, setupRuleRouter(t, mock), http.MethodGet, "/policies/1/rules", nil)
	assertStatusCode(t, rec, http.StatusOK)
}

func TestRuleHandler_List_WithoutSeededRules_ReturnsStatus200(t *testing.T) {
	mock := &mockRuleService{
		listFn: func(_ context.Context, _ schemas.ListRulesSchema) ([]schemas.RuleOutputSchema, error) {
			return []schemas.RuleOutputSchema{}, nil
		},
	}

	rec := performRequest(t, setupRuleRouter(t, mock), http.MethodGet, "/policies/1/rules", nil)
	assertStatusCode(t, rec, http.StatusOK)
}

func TestRuleHandler_List_WithNilResult_ReturnsStatus200(t *testing.T) {
	mock := &mockRuleService{
		listFn: func(_ context.Context, _ schemas.ListRulesSchema) ([]schemas.RuleOutputSchema, error) {
			return nil, nil
		},
	}

	rec := performRequest(t, setupRuleRouter(t, mock), http.MethodGet, "/policies/1/rules", nil)
	assertStatusCode(t, rec, http.StatusOK)
}

func TestRuleHandler_List_WithQueryFilters_ReturnsStatus200(t *testing.T) {
	mock := &mockRuleService{
		listFn: func(_ context.Context, _ schemas.ListRulesSchema) ([]schemas.RuleOutputSchema, error) {
			return []schemas.RuleOutputSchema{
				{ID: 1, PolicyID: 1, Type: domain.DomainRuleType, Value: "test.com"},
			}, nil
		},
	}

	rec := performRequest(t, setupRuleRouter(t, mock), http.MethodGet, "/policies/1/rules?rule_type=DOMAIN&value=test", nil)
	assertStatusCode(t, rec, http.StatusOK)
}

func TestRuleHandler_List_WithInexistentPolicy_ReturnsStatus404(t *testing.T) {
	mock := &mockRuleService{
		listFn: func(_ context.Context, _ schemas.ListRulesSchema) ([]schemas.RuleOutputSchema, error) {
			return nil, ruleErrors.ErrPolicyNotFound
		},
	}

	rec := performRequest(t, setupRuleRouter(t, mock), http.MethodGet, "/policies/9999/rules", nil)
	assertStatusCode(t, rec, http.StatusNotFound)
}

func TestRuleHandler_List_WhenServiceFails_ReturnsStatus500(t *testing.T) {
	mock := &mockRuleService{
		listFn: func(_ context.Context, _ schemas.ListRulesSchema) ([]schemas.RuleOutputSchema, error) {
			return nil, ruleErrors.ErrRuleNotFound
		},
	}

	rec := performRequest(t, setupRuleRouter(t, mock), http.MethodGet, "/policies/1/rules", nil)
	assertStatusCode(t, rec, http.StatusInternalServerError)
}

// ============================================================================
// UpdateRule (PATCH /policies/{policyID}/rules/{ruleID})
// ============================================================================

func TestRuleHandler_Update_WithValidPayload_ReturnsStatus200(t *testing.T) {
	payload := schemas.UpdateRuleSchema{
		Value: new(string),
	}
	mock := &mockRuleService{
		updateFn: func(_ context.Context, p schemas.UpdateRuleSchema) (*schemas.RuleOutputSchema, error) {
			return &schemas.RuleOutputSchema{ID: p.ID, PolicyID: p.PolicyID}, nil
		},
	}

	rec := performRequest(t, setupRuleRouter(t, mock), http.MethodPatch, "/policies/1/rules/1", payload)
	assertStatusCode(t, rec, http.StatusOK)
}

func TestRuleHandler_Update_WithNonExistentRule_ReturnsStatus404(t *testing.T) {
	mock := &mockRuleService{
		updateFn: func(_ context.Context, _ schemas.UpdateRuleSchema) (*schemas.RuleOutputSchema, error) {
			return nil, ruleErrors.ErrRuleNotFound
		},
	}

	rec := performRequest(t, setupRuleRouter(t, mock), http.MethodPatch, "/policies/1/rules/9999", schemas.UpdateRuleSchema{})
	assertStatusCode(t, rec, http.StatusNotFound)
}

func TestRuleHandler_Update_WithInvalidPayload_ReturnsStatus400(t *testing.T) {
	testCases := map[string]error{
		"Empty type":      ruleErrors.ErrRuleTypeRequired,
		"Empty value":     ruleErrors.ErrRuleValueRequired,
		"Malformed value": ruleErrors.ErrMalformedRuleValue,
		"Invalid port":    ruleErrors.ErrInvalidRulePortValue,
	}

	for testName, tt := range testCases {
		t.Run(testName, func(t *testing.T) {
			mock := &mockRuleService{
				updateFn: func(_ context.Context, _ schemas.UpdateRuleSchema) (*schemas.RuleOutputSchema, error) {
					return nil, tt
				},
			}

			rec := performRequest(t, setupRuleRouter(t, mock), http.MethodPatch, "/policies/1/rules/1", schemas.UpdateRuleSchema{})
			assertStatusCode(t, rec, http.StatusBadRequest)
		})
	}
}

func TestRuleHandler_Update_WithMalformedPayload_ReturnsStatus400(t *testing.T) {
	mock := &mockRuleService{
		updateFn: func(_ context.Context, _ schemas.UpdateRuleSchema) (*schemas.RuleOutputSchema, error) {
			return nil, nil
		},
	}

	rec := performRequest(t, setupRuleRouter(t, mock), http.MethodPatch, "/policies/1/rules/1", "invalid json")
	assertStatusCode(t, rec, http.StatusBadRequest)
}

func TestRuleHandler_Update_WithInvalidRuleID_ReturnsStatus400(t *testing.T) {
	mock := &mockRuleService{
		updateFn: func(_ context.Context, _ schemas.UpdateRuleSchema) (*schemas.RuleOutputSchema, error) {
			return nil, nil
		},
	}

	rec := performRequest(t, setupRuleRouter(t, mock), http.MethodPatch, "/policies/1/rules/abc", schemas.UpdateRuleSchema{})
	assertStatusCode(t, rec, http.StatusBadRequest)
}

// ============================================================================
// RemoveRule (DELETE /policies/{policyID}/rules/{ruleID})
// ============================================================================

func TestRuleHandler_Delete_WithExistentRule_ReturnsStatus204(t *testing.T) {
	mock := &mockRuleService{
		removeFn: func(_ context.Context, _ schemas.RemoveRuleSchema) error {
			return nil
		},
	}

	rec := performRequest(t, setupRuleRouter(t, mock), http.MethodDelete, "/policies/1/rules/1", nil)
	assertStatusCode(t, rec, http.StatusNoContent)
}

func TestRuleHandler_Delete_WithNonExistentRule_ReturnsStatus404(t *testing.T) {
	mock := &mockRuleService{
		removeFn: func(_ context.Context, _ schemas.RemoveRuleSchema) error {
			return ruleErrors.ErrRuleNotFound
		},
	}

	rec := performRequest(t, setupRuleRouter(t, mock), http.MethodDelete, "/policies/1/rules/9999", nil)
	assertStatusCode(t, rec, http.StatusNotFound)
}

func TestRuleHandler_Delete_WithInvalidRuleID_ReturnsStatus400(t *testing.T) {
	mock := &mockRuleService{
		removeFn: func(_ context.Context, _ schemas.RemoveRuleSchema) error {
			return nil
		},
	}

	rec := performRequest(t, setupRuleRouter(t, mock), http.MethodDelete, "/policies/1/rules/abc", nil)
	assertStatusCode(t, rec, http.StatusBadRequest)
}

// ============================================================================
// Helpers
// ============================================================================

// setupRuleRouter builds a router wired with a rule handler backed by the
// provided service mock.
func setupRuleRouter(t *testing.T, svc handler.RuleServiceInterface) *chi.Mux {
	t.Helper()

	r := chi.NewRouter()
	h := handler.NewRuleHandler(svc)
	h.RegisterRoutes(r)

	return r
}
