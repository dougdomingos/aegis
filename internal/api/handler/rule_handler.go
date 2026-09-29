package handler

import (
	"context"
	"net/http"
	"strconv"

	"dougdomingos.com/aegis/internal/api/utils"
	"dougdomingos.com/aegis/internal/domain"
	"dougdomingos.com/aegis/internal/errors"
	"dougdomingos.com/aegis/internal/schemas"
	"github.com/go-chi/chi/v5"
)

// RuleServiceInterface declares the methods provided by RuleService
// implementations.
type RuleServiceInterface interface {
	CreateRule(ctx context.Context, p schemas.CreateRuleSchema) (*schemas.RuleOutputSchema, error)
	GetRuleByID(ctx context.Context, p schemas.GetRuleByIDSchema) (*schemas.RuleOutputSchema, error)
	ListRules(ctx context.Context, p schemas.ListRulesSchema) ([]schemas.RuleOutputSchema, error)
	UpdateRule(ctx context.Context, p schemas.UpdateRuleSchema) (*schemas.RuleOutputSchema, error)
	RemoveRule(ctx context.Context, p schemas.RemoveRuleSchema) error
}

// RuleHandler implements the methods that map HTTP requests into operations
// within the application. Rules are exposed as a sub-resource of policies.
type RuleHandler struct {
	service RuleServiceInterface
}

// NewRuleHandler creates a new RuleHandler instance.
func NewRuleHandler(s RuleServiceInterface) *RuleHandler {
	return &RuleHandler{service: s}
}

// RegisterRoutes registers the rule management routes into the provided
// router, scoped under the policy that owns the rules.
func (handler *RuleHandler) RegisterRoutes(r chi.Router) {
	r.Route("/policies/{policyID}/rules", func(r chi.Router) {
		r.Post("/", handler.Create)
		r.Get("/", handler.List)
		r.Get("/{ruleID}", handler.GetByID)
		r.Patch("/{ruleID}", handler.Update)
		r.Delete("/{ruleID}", handler.Delete)
	})
}

// Create registers a new rule within the policy referenced in the URL.
func (handler *RuleHandler) Create(w http.ResponseWriter, r *http.Request) {
	policyID, ok := parsePolicyID(w, r, "policyID")
	if !ok {
		return
	}

	var payload schemas.CreateRuleSchema
	if !utils.DecodePayload(w, r, &payload) {
		return
	}

	payload.PolicyID = policyID

	res, err := handler.service.CreateRule(r.Context(), payload)
	if err != nil {
		utils.MapByError(w, err, map[error]int{
			errors.ErrRulePolicyRequired:      http.StatusBadRequest,
			errors.ErrRuleTypeRequired:        http.StatusBadRequest,
			errors.ErrRuleValueRequired:       http.StatusBadRequest,
			errors.ErrMalformedRuleValue:      http.StatusBadRequest,
			errors.ErrInvalidFieldForRuleType: http.StatusBadRequest,
			errors.ErrInvalidRulePortValue:    http.StatusBadRequest,
			errors.ErrPolicyNotFound:          http.StatusNotFound,
		})

		return
	}

	utils.EncodeToJSON(w, http.StatusCreated, res)
}

// List returns all the rules that belong to the policy referenced in the URL
// and match the provided query filters. If no filter is provided, it returns
// every rule of that policy.
func (handler *RuleHandler) List(w http.ResponseWriter, r *http.Request) {
	policyID, ok := parsePolicyID(w, r, "policyID")
	if !ok {
		return
	}

	var payload schemas.ListRulesSchema
	payload.PolicyID = policyID

	if value := r.URL.Query().Get("rule_type"); value != "" {
		ruleType := domain.RuleType(value)
		payload.Type = &ruleType
	}

	if value := r.URL.Query().Get("value"); value != "" {
		payload.Value = &value
	}

	rules, err := handler.service.ListRules(r.Context(), payload)
	if err != nil {
		utils.MapByError(w, err, map[error]int{
			errors.ErrPolicyNotFound: http.StatusNotFound,
		})
		return
	}

	if rules == nil {
		rules = []schemas.RuleOutputSchema{}
	}

	utils.EncodeToJSON(w, http.StatusOK, rules)
}

// GetByID retrieves a rule by its ID within the policy referenced in the URL.
func (handler *RuleHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	policyID, ruleID, ok := parseScopedRuleID(w, r)
	if !ok {
		return
	}

	res, err := handler.service.GetRuleByID(r.Context(), schemas.GetRuleByIDSchema{
		PolicyID: policyID,
		ID:       ruleID,
	})
	if err != nil {
		utils.MapByError(w, err, map[error]int{
			errors.ErrRuleNotFound: http.StatusNotFound,
		})
		return
	}

	if res == nil {
		utils.EmitError(w, errors.ErrRuleNotFound.Error(), http.StatusNotFound)
		return
	}

	utils.EncodeToJSON(w, http.StatusOK, res)
}

// Update applies the provided changes to an existent rule within the policy
// referenced in the URL.
func (handler *RuleHandler) Update(w http.ResponseWriter, r *http.Request) {
	policyID, ruleID, ok := parseScopedRuleID(w, r)
	if !ok {
		return
	}

	var payload schemas.UpdateRuleSchema
	if !utils.DecodePayload(w, r, &payload) {
		return
	}

	payload.PolicyID = policyID
	payload.ID = ruleID

	res, err := handler.service.UpdateRule(r.Context(), payload)
	if err != nil {
		utils.MapByError(w, err, map[error]int{
			errors.ErrRuleNotFound:            http.StatusNotFound,
			errors.ErrRuleTypeRequired:        http.StatusBadRequest,
			errors.ErrRuleValueRequired:       http.StatusBadRequest,
			errors.ErrMalformedRuleValue:      http.StatusBadRequest,
			errors.ErrInvalidFieldForRuleType: http.StatusBadRequest,
			errors.ErrInvalidRulePortValue:    http.StatusBadRequest,
		})

		return
	}

	utils.EncodeToJSON(w, http.StatusOK, res)
}

// Delete removes a rule by its ID within the policy referenced in the URL.
func (handler *RuleHandler) Delete(w http.ResponseWriter, r *http.Request) {
	policyID, ruleID, ok := parseScopedRuleID(w, r)
	if !ok {
		return
	}

	err := handler.service.RemoveRule(r.Context(), schemas.RemoveRuleSchema{
		PolicyID: policyID,
		ID:       ruleID,
	})
	if err != nil {
		utils.MapByError(w, err, map[error]int{
			errors.ErrRuleNotFound: http.StatusNotFound,
		})

		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// parseScopedRuleID decodes both the policyID and ruleID URL parameters. If
// any of them is not a valid number, it writes an HTTP 400 Bad Request and
// returns false.
func parseScopedRuleID(w http.ResponseWriter, r *http.Request) (int64, int, bool) {
	policyID, ok := parsePolicyID(w, r, "policyID")
	if !ok {
		return 0, 0, false
	}

	ruleID, ok := parseRuleID(w, r, "ruleID")
	if !ok {
		return 0, 0, false
	}

	return policyID, ruleID, true
}

// parseRuleID decodes the named URL parameter into an integer. If the
// parameter is not a valid number, it writes an HTTP 400 Bad Request and
// returns false.
func parseRuleID(w http.ResponseWriter, r *http.Request, paramName string) (int, bool) {
	rawID := chi.URLParam(r, paramName)

	id, err := strconv.Atoi(rawID)
	if err != nil {
		utils.EmitError(w, "invalid rule id", http.StatusBadRequest)
		return 0, false
	}

	return id, true
}
