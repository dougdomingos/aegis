package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"dougdomingos.com/aegis/internal/domain"
	"dougdomingos.com/aegis/internal/infra/query"
)

const (
	queryCreateRule     = "INSERT INTO rules (policy_id, type, value, protocol, port) VALUES (?,?,?,?,?)"
	queryGetRuleByID    = "SELECT id, policy_id, type, value, protocol, port, created_at FROM rules WHERE id = ?"
	queryUpdateRuleByID = "UPDATE rules SET value = ?, protocol = ?, port = ? WHERE id = ?"
	queryDeleteRuleByID = "DELETE FROM rules WHERE id = ?"
	queryColumnsRule    = "SELECT id, policy_id, type, value, protocol, port, created_at FROM rules"
)

// RuleStore manages all database-related operations over rules.
type RuleStore struct {
	executor *query.QueryExecutor[domain.Rule]
}

// NewRuleStore creates a new RuleStore instance.
func NewRuleStore(db *sql.DB) *RuleStore {
	return &RuleStore{
		executor: query.NewQueryExecutor(db, mapRowToRule),
	}
}

func (store *RuleStore) Create(ctx context.Context, rule domain.Rule) (*domain.Rule, error) {
	var createdRule *domain.Rule

	if err := validateRuleConstraints(rule); err != nil {
		return nil, err
	}

	err := store.executor.WithTx(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, queryCreateRule, rule.PolicyID, rule.Type, rule.Value, rule.Protocol, rule.Port)
		if err != nil {
			return fmt.Errorf("failed to create rule: %w", err)
		}

		id, err := result.LastInsertId()
		if err != nil {
			return fmt.Errorf("failed to retrieve created rule id: %w", err)
		}

		row := tx.QueryRowContext(ctx, queryGetRuleByID, id)
		rule, err := mapRowToRule(row.Scan)
		if err != nil {
			return fmt.Errorf("failed to retrieve created group: %w", err)
		}

		createdRule = &rule
		return nil
	})

	if err != nil {
		return nil, err
	}

	return createdRule, nil
}

func (store *RuleStore) GetByID(ctx context.Context, id int64) (*domain.Rule, error) {
	return store.executor.QueryOne(ctx, queryGetRuleByID, id)
}

func (store *RuleStore) List(ctx context.Context, filter domain.RuleFilter) ([]domain.Rule, error) {
	var args []any
	var conditions []string
	searchQuery := queryColumnsRule

	conditions = append(conditions, "policy_id = ?")
	args = append(args, filter.PolicyID)

	if filter.Type != nil {
		conditions = append(conditions, "type = ?")
		args = append(args, *filter.Type)
	}

	if filter.Value != nil {
		conditions = append(conditions, "value LIKE ?")
		args = append(args, fmt.Sprintf("%%%s%%", *filter.Value))
	}

	searchQuery += " WHERE " + strings.Join(conditions, " AND ")

	return store.executor.QueryMany(ctx, searchQuery, args...)
}

func (store *RuleStore) Update(ctx context.Context, rule domain.Rule) (*domain.Rule, error) {
	if err := validateRuleConstraints(rule); err != nil {
		return nil, err
	}

	result, err := store.executor.Exec(
		ctx,
		queryUpdateRuleByID,
		rule.Value,
		rule.Protocol,
		rule.Port,
		rule.ID)

	if err != nil {
		return nil, fmt.Errorf("failed to update rule: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("failed to check rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return nil, fmt.Errorf("rule with id %d not found", rule.ID)
	}

	return &rule, nil
}

func (store *RuleStore) Remove(ctx context.Context, id int64) error {
	result, err := store.executor.Exec(ctx, queryDeleteRuleByID, id)

	if err != nil {
		return fmt.Errorf("failed to update rule: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("rule with id %d not found", id)
	}

	return nil
}

// mapRowToRule maps a scan function to a domain.Rule struct.
func mapRowToRule(scan query.ScanFunc) (domain.Rule, error) {
	var rule domain.Rule
	if err := scan(
		&rule.ID,
		&rule.PolicyID,
		&rule.Type,
		&rule.Value,
		&rule.Protocol,
		&rule.Port,
		&rule.CreatedAt); err != nil {

		return domain.Rule{}, err
	}

	return rule, nil
}

func validateRuleConstraints(rule domain.Rule) error {
	if rule.PolicyID <= 0 {
		return fmt.Errorf("rule must belong to a policy")
	}

	if rule.Type == "" {
		return fmt.Errorf("rule type must be provided")
	}

	if rule.Value == "" {
		return fmt.Errorf("rule value cannot be empty")
	}

	return nil
}
