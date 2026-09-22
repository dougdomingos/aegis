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
// within the application.
type RuleHandler struct {
	service RuleServiceInterface
}

// NewRuleHandler creates a new RuleHandler instance.
func NewRuleHandler(s RuleServiceInterface) *RuleHandler {
	return &RuleHandler{service: s}
}

// RegisterRuleRoutes registers the rule management routes into the provided
// router.
func (handler *RuleHandler) RegisterRuleRoutes(r chi.Router) {
	r.Route("/rules", func(r chi.Router) {
		r.Post("/", handler.Create)
		r.Get("/", handler.List)
		r.Get("/{id}", handler.GetByID)
		r.Patch("/{id}", handler.Update)
		r.Delete("/{id}", handler.Delete)
	})
}

// Create registers a new rule within the system.
func (handler *RuleHandler) Create(w http.ResponseWriter, r *http.Request) {
	var payload schemas.CreateRuleSchema
	if !utils.DecodePayload(w, r, &payload) {
		return
	}

	res, err := handler.service.CreateRule(r.Context(), payload)
	if err != nil {
		utils.MapByError(w, err, map[error]int{
			errors.ErrRuleTypeRequired:        http.StatusBadRequest,
			errors.ErrRuleActionRequired:      http.StatusBadRequest,
			errors.ErrRuleValueRequired:       http.StatusBadRequest,
			errors.ErrMalformedRuleValue:      http.StatusBadRequest,
			errors.ErrInvalidFieldForRuleType: http.StatusBadRequest,
			errors.ErrInvalidRulePortValue:    http.StatusBadRequest,
		})

		return
	}

	utils.EncodeToJSON(w, http.StatusCreated, res)
}

// List returns all the rules that match the provided query filters. If no
// filter is provided, it returns every rule registered within the system.
func (handler *RuleHandler) List(w http.ResponseWriter, r *http.Request) {
	var payload schemas.ListRulesSchema

	if value := r.URL.Query().Get("rule_type"); value != "" {
		ruleType := domain.RuleType(value)
		payload.Type = &ruleType
	}

	if value := r.URL.Query().Get("action"); value != "" {
		action := domain.RuleAction(value)
		payload.Action = &action
	}

	if value := r.URL.Query().Get("value"); value != "" {
		payload.Value = &value
	}

	rules, err := handler.service.ListRules(r.Context(), payload)
	if err != nil {
		utils.MapByError(w, err, nil)
		return
	}

	if rules == nil {
		rules = []schemas.RuleOutputSchema{}
	}

	utils.EncodeToJSON(w, http.StatusOK, rules)
}

// GetByID retrieves a rule by its ID.
func (handler *RuleHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	id, ok := parseRuleID(w, r, "id")
	if !ok {
		return
	}

	res, err := handler.service.GetRuleByID(r.Context(), schemas.GetRuleByIDSchema{ID: id})
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

// Update applies the provided changes to an existent rule.
func (handler *RuleHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := parseRuleID(w, r, "id")
	if !ok {
		return
	}

	var payload schemas.UpdateRuleSchema
	if !utils.DecodePayload(w, r, &payload) {
		return
	}

	payload.ID = id

	res, err := handler.service.UpdateRule(r.Context(), payload)
	if err != nil {
		utils.MapByError(w, err, map[error]int{
			errors.ErrRuleNotFound:            http.StatusNotFound,
			errors.ErrRuleTypeRequired:        http.StatusBadRequest,
			errors.ErrRuleActionRequired:      http.StatusBadRequest,
			errors.ErrRuleValueRequired:       http.StatusBadRequest,
			errors.ErrMalformedRuleValue:      http.StatusBadRequest,
			errors.ErrInvalidFieldForRuleType: http.StatusBadRequest,
			errors.ErrInvalidRulePortValue:    http.StatusBadRequest,
		})

		return
	}

	utils.EncodeToJSON(w, http.StatusOK, res)
}

// Delete removes a rule by its ID.
func (handler *RuleHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := parseRuleID(w, r, "id")
	if !ok {
		return
	}

	err := handler.service.RemoveRule(r.Context(), schemas.RemoveRuleSchema{ID: id})
	if err != nil {
		utils.MapByError(w, err, map[error]int{
			errors.ErrRuleNotFound: http.StatusNotFound,
		})

		return
	}

	w.WriteHeader(http.StatusNoContent)
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
