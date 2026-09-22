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
// CreateRule (POST /rules)
// ============================================================================

func TestRuleHandler_Create_WithValidPayload_ReturnsStatus201(t *testing.T) {
	payload := schemas.CreateRuleSchema{
		Type:   domain.DomainRuleType,
		Action: domain.AllowAction,
		Value:  "test.com",
	}
	mock := &mockRuleService{
		createFn: func(_ context.Context, p schemas.CreateRuleSchema) (*schemas.RuleOutputSchema, error) {
			return &schemas.RuleOutputSchema{
				Type:   p.Type,
				Action: p.Action,
				Value:  p.Value,
			}, nil
		},
	}

	rec := performRequest(t, setupRuleRouter(t, mock), http.MethodPost, "/rules", payload)
	assertStatusCode(t, rec, http.StatusCreated)
}

func TestRuleHandler_Create_WithInvalidPayload_ReturnsStatus400(t *testing.T) {
	testCases := map[string]struct {
		payload    schemas.CreateRuleSchema
		serviceErr error
	}{
		"Empty type": {
			payload: schemas.CreateRuleSchema{
				Action: domain.AllowAction,
				Value:  "test.com",
			},
			serviceErr: ruleErrors.ErrRuleTypeRequired,
		},
		"Empty action": {
			payload: schemas.CreateRuleSchema{
				Type:  domain.DomainRuleType,
				Value: "test.com",
			},
			serviceErr: ruleErrors.ErrRuleActionRequired,
		},
		"Empty value": {
			payload: schemas.CreateRuleSchema{
				Type:   domain.DomainRuleType,
				Action: domain.AllowAction,
			},
			serviceErr: ruleErrors.ErrRuleValueRequired,
		},
		"Malformed value": {
			payload: schemas.CreateRuleSchema{
				Type:   domain.DomainRuleType,
				Action: domain.AllowAction,
				Value:  "thisisnotadomain",
			},
			serviceErr: ruleErrors.ErrMalformedRuleValue,
		},
		"Protocol for domain rule": {
			payload: schemas.CreateRuleSchema{
				Type:     domain.DomainRuleType,
				Action:   domain.AllowAction,
				Value:    "test.com",
				Protocol: new(string),
			},
			serviceErr: ruleErrors.ErrInvalidFieldForRuleType,
		},
		"Invalid port": {
			payload: schemas.CreateRuleSchema{
				Type:   domain.IPRuleType,
				Action: domain.AllowAction,
				Value:  "10.0.0.1",
				Port:   new(int),
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

			rec := performRequest(t, setupRuleRouter(t, mock), http.MethodPost, "/rules", tt.payload)
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

	rec := performRequest(t, setupRuleRouter(t, mock), http.MethodPost, "/rules", "invalid json")
	assertStatusCode(t, rec, http.StatusBadRequest)
}

// ============================================================================
// GetRuleByID (GET /rules/{id})
// ============================================================================

func TestRuleHandler_GetByID_WithExistentRule_ReturnsStatus200(t *testing.T) {
	reqUrl := fmt.Sprintf("/rules/%d", 1)
	mock := &mockRuleService{
		getByIDFn: func(_ context.Context, p schemas.GetRuleByIDSchema) (*schemas.RuleOutputSchema, error) {
			return &schemas.RuleOutputSchema{ID: p.ID}, nil
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

	rec := performRequest(t, setupRuleRouter(t, mock), http.MethodGet, "/rules/9999", nil)
	assertStatusCode(t, rec, http.StatusNotFound)
}

func TestRuleHandler_GetByID_WithNilResult_ReturnsStatus404(t *testing.T) {
	mock := &mockRuleService{
		getByIDFn: func(_ context.Context, _ schemas.GetRuleByIDSchema) (*schemas.RuleOutputSchema, error) {
			return nil, nil
		},
	}

	rec := performRequest(t, setupRuleRouter(t, mock), http.MethodGet, "/rules/9999", nil)
	assertStatusCode(t, rec, http.StatusNotFound)
}

func TestRuleHandler_GetByID_WithInvalidID_ReturnsStatus400(t *testing.T) {
	mock := &mockRuleService{
		getByIDFn: func(_ context.Context, _ schemas.GetRuleByIDSchema) (*schemas.RuleOutputSchema, error) {
			return nil, nil
		},
	}

	rec := performRequest(t, setupRuleRouter(t, mock), http.MethodGet, "/rules/abc", nil)
	assertStatusCode(t, rec, http.StatusBadRequest)
}

// ============================================================================
// ListRules (GET /rules)
// ============================================================================

func TestRuleHandler_List_WithSeededRules_ReturnsStatus200(t *testing.T) {
	mock := &mockRuleService{
		listFn: func(_ context.Context, _ schemas.ListRulesSchema) ([]schemas.RuleOutputSchema, error) {
			return []schemas.RuleOutputSchema{
				{ID: 1, Type: domain.DomainRuleType, Action: domain.AllowAction, Value: "test.com"},
				{ID: 2, Type: domain.IPRuleType, Action: domain.DenyAction, Value: "10.0.0.1"},
			}, nil
		},
	}

	rec := performRequest(t, setupRuleRouter(t, mock), http.MethodGet, "/rules", nil)
	assertStatusCode(t, rec, http.StatusOK)
}

func TestRuleHandler_List_WithoutSeededRules_ReturnsStatus200(t *testing.T) {
	mock := &mockRuleService{
		listFn: func(_ context.Context, _ schemas.ListRulesSchema) ([]schemas.RuleOutputSchema, error) {
			return []schemas.RuleOutputSchema{}, nil
		},
	}

	rec := performRequest(t, setupRuleRouter(t, mock), http.MethodGet, "/rules", nil)
	assertStatusCode(t, rec, http.StatusOK)
}

func TestRuleHandler_List_WithNilResult_ReturnsStatus200(t *testing.T) {
	mock := &mockRuleService{
		listFn: func(_ context.Context, _ schemas.ListRulesSchema) ([]schemas.RuleOutputSchema, error) {
			return nil, nil
		},
	}

	rec := performRequest(t, setupRuleRouter(t, mock), http.MethodGet, "/rules", nil)
	assertStatusCode(t, rec, http.StatusOK)
}

func TestRuleHandler_List_WithQueryFilters_ReturnsStatus200(t *testing.T) {
	mock := &mockRuleService{
		listFn: func(_ context.Context, _ schemas.ListRulesSchema) ([]schemas.RuleOutputSchema, error) {
			return []schemas.RuleOutputSchema{
				{ID: 1, Type: domain.DomainRuleType, Action: domain.AllowAction, Value: "test.com"},
			}, nil
		},
	}

	rec := performRequest(t, setupRuleRouter(t, mock), http.MethodGet, "/rules?rule_type=DOMAIN&action=ALLOW&value=test", nil)
	assertStatusCode(t, rec, http.StatusOK)
}

func TestRuleHandler_List_WhenServiceFails_ReturnsStatus500(t *testing.T) {
	mock := &mockRuleService{
		listFn: func(_ context.Context, _ schemas.ListRulesSchema) ([]schemas.RuleOutputSchema, error) {
			return nil, ruleErrors.ErrRuleNotFound
		},
	}

	rec := performRequest(t, setupRuleRouter(t, mock), http.MethodGet, "/rules", nil)
	assertStatusCode(t, rec, http.StatusInternalServerError)
}

// ============================================================================
// UpdateRule (PATCH /rules/{id})
// ============================================================================

func TestRuleHandler_Update_WithValidPayload_ReturnsStatus200(t *testing.T) {
	payload := schemas.UpdateRuleSchema{
		Action: (*domain.RuleAction)(new(string)),
		Value:  new(string),
	}
	mock := &mockRuleService{
		updateFn: func(_ context.Context, p schemas.UpdateRuleSchema) (*schemas.RuleOutputSchema, error) {
			return &schemas.RuleOutputSchema{ID: p.ID}, nil
		},
	}

	rec := performRequest(t, setupRuleRouter(t, mock), http.MethodPatch, "/rules/1", payload)
	assertStatusCode(t, rec, http.StatusOK)
}

func TestRuleHandler_Update_WithNonExistentRule_ReturnsStatus404(t *testing.T) {
	mock := &mockRuleService{
		updateFn: func(_ context.Context, _ schemas.UpdateRuleSchema) (*schemas.RuleOutputSchema, error) {
			return nil, ruleErrors.ErrRuleNotFound
		},
	}

	rec := performRequest(t, setupRuleRouter(t, mock), http.MethodPatch, "/rules/9999", schemas.UpdateRuleSchema{})
	assertStatusCode(t, rec, http.StatusNotFound)
}

func TestRuleHandler_Update_WithInvalidPayload_ReturnsStatus400(t *testing.T) {
	testCases := map[string]error{
		"Empty action":    ruleErrors.ErrRuleActionRequired,
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

			rec := performRequest(t, setupRuleRouter(t, mock), http.MethodPatch, "/rules/1", schemas.UpdateRuleSchema{})
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

	rec := performRequest(t, setupRuleRouter(t, mock), http.MethodPatch, "/rules/1", "invalid json")
	assertStatusCode(t, rec, http.StatusBadRequest)
}

func TestRuleHandler_Update_WithInvalidID_ReturnsStatus400(t *testing.T) {
	mock := &mockRuleService{
		updateFn: func(_ context.Context, _ schemas.UpdateRuleSchema) (*schemas.RuleOutputSchema, error) {
			return nil, nil
		},
	}

	rec := performRequest(t, setupRuleRouter(t, mock), http.MethodPatch, "/rules/abc", schemas.UpdateRuleSchema{})
	assertStatusCode(t, rec, http.StatusBadRequest)
}

// ============================================================================
// RemoveRule (DELETE /rules/{id})
// ============================================================================

func TestRuleHandler_Delete_WithExistentRule_ReturnsStatus204(t *testing.T) {
	mock := &mockRuleService{
		removeFn: func(_ context.Context, _ schemas.RemoveRuleSchema) error {
			return nil
		},
	}

	rec := performRequest(t, setupRuleRouter(t, mock), http.MethodDelete, "/rules/1", nil)
	assertStatusCode(t, rec, http.StatusNoContent)
}

func TestRuleHandler_Delete_WithNonExistentRule_ReturnsStatus404(t *testing.T) {
	mock := &mockRuleService{
		removeFn: func(_ context.Context, _ schemas.RemoveRuleSchema) error {
			return ruleErrors.ErrRuleNotFound
		},
	}

	rec := performRequest(t, setupRuleRouter(t, mock), http.MethodDelete, "/rules/9999", nil)
	assertStatusCode(t, rec, http.StatusNotFound)
}

func TestRuleHandler_Delete_WithInvalidID_ReturnsStatus400(t *testing.T) {
	mock := &mockRuleService{
		removeFn: func(_ context.Context, _ schemas.RemoveRuleSchema) error {
			return nil
		},
	}

	rec := performRequest(t, setupRuleRouter(t, mock), http.MethodDelete, "/rules/abc", nil)
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
