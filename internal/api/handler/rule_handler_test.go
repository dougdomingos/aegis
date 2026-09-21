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
	"dougdomingos.com/aegis/internal/service"
)

// ============================================================================
// Service mock
// ============================================================================

type mockRuleService struct {
	createFn  func(ctx context.Context, p schemas.CreateRuleSchema) (*schemas.RuleOutputSchema, error)
	getByIDFn func(ctx context.Context, p schemas.GetRuleByIDSchema) (*schemas.RuleOutputSchema, error)
	listFn    func(ctx context.Context, p schemas.ListRulesSchema) ([]schemas.RuleOutputSchema, error)
	updateFn  func(ctx context.Context, p schemas.UpdateRuleSchema) (*schemas.RuleOutputSchema, error)
	removeFn  func(ctx context.Context, p schemas.RemoveRuleSchema) error
}

func (m *mockRuleService) CreateRule(ctx context.Context, p schemas.CreateRuleSchema) (*schemas.RuleOutputSchema, error) {
	return m.createFn(ctx, p)
}

func (m *mockRuleService) GetRuleByID(ctx context.Context, p schemas.GetRuleByIDSchema) (*schemas.RuleOutputSchema, error) {
	return m.getByIDFn(ctx, p)
}

func (m *mockRuleService) ListRules(ctx context.Context, p schemas.ListRulesSchema) ([]schemas.RuleOutputSchema, error) {
	return m.listFn(ctx, p)
}

func (m *mockRuleService) UpdateRule(ctx context.Context, p schemas.UpdateRuleSchema) (*schemas.RuleOutputSchema, error) {
	return m.updateFn(ctx, p)
}

func (m *mockRuleService) RemoveRule(ctx context.Context, p schemas.RemoveRuleSchema) error {
	return m.removeFn(ctx, p)
}

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

	rec, req := buildTestRequest(http.MethodPost, "/rules", payload)
	setupRuleRouter(mock).ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("expected status code 201, got %d", rec.Code)
	}
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

			rec, req := buildTestRequest(http.MethodPost, "/rules", tt.payload)
			setupRuleRouter(mock).ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Errorf("expected status code 400, got %d", rec.Code)
			}
		})
	}
}

func TestRuleHandler_Create_WithMalformedPayload_ReturnsStatus400(t *testing.T) {
	mock := &mockRuleService{
		createFn: func(_ context.Context, _ schemas.CreateRuleSchema) (*schemas.RuleOutputSchema, error) {
			return nil, nil
		},
	}

	rec, req := buildTestRequest(http.MethodPost, "/rules", "invalid json")
	setupRuleRouter(mock).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status code 400, got %d", rec.Code)
	}
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

	rec, req := buildTestRequest(http.MethodGet, reqUrl, nil)
	setupRuleRouter(mock).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status code 200, got %d", rec.Code)
	}
}

func TestRuleHandler_GetByID_WithNonExistentRule_ReturnsStatus404(t *testing.T) {
	mock := &mockRuleService{
		getByIDFn: func(_ context.Context, _ schemas.GetRuleByIDSchema) (*schemas.RuleOutputSchema, error) {
			return nil, ruleErrors.ErrRuleNotFound
		},
	}

	rec, req := buildTestRequest(http.MethodGet, "/rules/9999", nil)
	setupRuleRouter(mock).ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected status code 404, got %d", rec.Code)
	}
}

func TestRuleHandler_GetByID_WithNilResult_ReturnsStatus404(t *testing.T) {
	mock := &mockRuleService{
		getByIDFn: func(_ context.Context, _ schemas.GetRuleByIDSchema) (*schemas.RuleOutputSchema, error) {
			return nil, nil
		},
	}

	rec, req := buildTestRequest(http.MethodGet, "/rules/9999", nil)
	setupRuleRouter(mock).ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected status code 404, got %d", rec.Code)
	}
}

func TestRuleHandler_GetByID_WithInvalidID_ReturnsStatus400(t *testing.T) {
	mock := &mockRuleService{
		getByIDFn: func(_ context.Context, _ schemas.GetRuleByIDSchema) (*schemas.RuleOutputSchema, error) {
			return nil, nil
		},
	}

	rec, req := buildTestRequest(http.MethodGet, "/rules/abc", nil)
	setupRuleRouter(mock).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status code 400, got %d", rec.Code)
	}
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

	rec, req := buildTestRequest(http.MethodGet, "/rules", nil)
	setupRuleRouter(mock).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status code 200, got %d", rec.Code)
	}
}

func TestRuleHandler_List_WithoutSeededRules_ReturnsStatus200(t *testing.T) {
	mock := &mockRuleService{
		listFn: func(_ context.Context, _ schemas.ListRulesSchema) ([]schemas.RuleOutputSchema, error) {
			return []schemas.RuleOutputSchema{}, nil
		},
	}

	rec, req := buildTestRequest(http.MethodGet, "/rules", nil)
	setupRuleRouter(mock).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status code 200, got %d", rec.Code)
	}
}

func TestRuleHandler_List_WithNilResult_ReturnsStatus200(t *testing.T) {
	mock := &mockRuleService{
		listFn: func(_ context.Context, _ schemas.ListRulesSchema) ([]schemas.RuleOutputSchema, error) {
			return nil, nil
		},
	}

	rec, req := buildTestRequest(http.MethodGet, "/rules", nil)
	setupRuleRouter(mock).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status code 200, got %d", rec.Code)
	}
}

func TestRuleHandler_List_WithQueryFilters_ReturnsStatus200(t *testing.T) {
	mock := &mockRuleService{
		listFn: func(_ context.Context, _ schemas.ListRulesSchema) ([]schemas.RuleOutputSchema, error) {
			return []schemas.RuleOutputSchema{
				{ID: 1, Type: domain.DomainRuleType, Action: domain.AllowAction, Value: "test.com"},
			}, nil
		},
	}

	rec, req := buildTestRequest(http.MethodGet, "/rules?rule_type=DOMAIN&action=ALLOW&value=test", nil)
	setupRuleRouter(mock).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status code 200, got %d", rec.Code)
	}
}

func TestRuleHandler_List_WhenServiceFails_ReturnsStatus500(t *testing.T) {
	mock := &mockRuleService{
		listFn: func(_ context.Context, _ schemas.ListRulesSchema) ([]schemas.RuleOutputSchema, error) {
			return nil, ruleErrors.ErrRuleNotFound
		},
	}

	rec, req := buildTestRequest(http.MethodGet, "/rules", nil)
	setupRuleRouter(mock).ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected status code 500, got %d", rec.Code)
	}
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

	rec, req := buildTestRequest(http.MethodPatch, "/rules/1", payload)
	setupRuleRouter(mock).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status code 200, got %d", rec.Code)
	}
}

func TestRuleHandler_Update_WithNonExistentRule_ReturnsStatus404(t *testing.T) {
	mock := &mockRuleService{
		updateFn: func(_ context.Context, _ schemas.UpdateRuleSchema) (*schemas.RuleOutputSchema, error) {
			return nil, ruleErrors.ErrRuleNotFound
		},
	}

	rec, req := buildTestRequest(http.MethodPatch, "/rules/9999", schemas.UpdateRuleSchema{})
	setupRuleRouter(mock).ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected status code 404, got %d", rec.Code)
	}
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

			rec, req := buildTestRequest(http.MethodPatch, "/rules/1", schemas.UpdateRuleSchema{})
			setupRuleRouter(mock).ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Errorf("expected status code 400, got %d", rec.Code)
			}
		})
	}
}

func TestRuleHandler_Update_WithMalformedPayload_ReturnsStatus400(t *testing.T) {
	mock := &mockRuleService{
		updateFn: func(_ context.Context, _ schemas.UpdateRuleSchema) (*schemas.RuleOutputSchema, error) {
			return nil, nil
		},
	}

	rec, req := buildTestRequest(http.MethodPatch, "/rules/1", "invalid json")
	setupRuleRouter(mock).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status code 400, got %d", rec.Code)
	}
}

func TestRuleHandler_Update_WithInvalidID_ReturnsStatus400(t *testing.T) {
	mock := &mockRuleService{
		updateFn: func(_ context.Context, _ schemas.UpdateRuleSchema) (*schemas.RuleOutputSchema, error) {
			return nil, nil
		},
	}

	rec, req := buildTestRequest(http.MethodPatch, "/rules/abc", schemas.UpdateRuleSchema{})
	setupRuleRouter(mock).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status code 400, got %d", rec.Code)
	}
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

	rec, req := buildTestRequest(http.MethodDelete, "/rules/1", nil)
	setupRuleRouter(mock).ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("expected status code 204, got %d", rec.Code)
	}
}

func TestRuleHandler_Delete_WithNonExistentRule_ReturnsStatus404(t *testing.T) {
	mock := &mockRuleService{
		removeFn: func(_ context.Context, _ schemas.RemoveRuleSchema) error {
			return ruleErrors.ErrRuleNotFound
		},
	}

	rec, req := buildTestRequest(http.MethodDelete, "/rules/9999", nil)
	setupRuleRouter(mock).ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected status code 404, got %d", rec.Code)
	}
}

func TestRuleHandler_Delete_WithInvalidID_ReturnsStatus400(t *testing.T) {
	mock := &mockRuleService{
		removeFn: func(_ context.Context, _ schemas.RemoveRuleSchema) error {
			return nil
		},
	}

	rec, req := buildTestRequest(http.MethodDelete, "/rules/abc", nil)
	setupRuleRouter(mock).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status code 400, got %d", rec.Code)
	}
}

// ============================================================================
// Helpers
// ============================================================================

func setupRuleRouter(svc service.RuleServiceInterface) *chi.Mux {
	r := chi.NewRouter()
	h := handler.NewRuleHandler(svc)
	h.RegisterRuleRoutes(r)

	return r
}
