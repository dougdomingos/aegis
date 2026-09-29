package store

import (
	"context"
	"database/sql"
	"fmt"

	"dougdomingos.com/aegis/internal/domain"
	"dougdomingos.com/aegis/internal/infra/query"
)

const (
	queryCreatePolicy     = "INSERT INTO policies (name, type) VALUES (?,?)"
	queryGetPolicyByID    = "SELECT id, name, type, version, created_at, updated_at FROM policies WHERE id = ?"
	queryGetPolicyByName  = "SELECT id, name, type, version, created_at, updated_at FROM policies WHERE name = ?"
	queryGetAllPolicies   = "SELECT id, name, type, version, created_at, updated_at FROM policies"
	queryUpdatePolicyByID = "UPDATE policies SET name = ?, version = version + 1, updated_at = CURRENT_TIMESTAMP WHERE id = ?"
	queryDeletePolicyByID = "DELETE FROM policies WHERE id = ?"
)

// PolicyStore manages all database-related operations over policies.
type PolicyStore struct {
	executor *query.QueryExecutor[domain.Policy]
}

// NewPolicyStore creates a new PolicyStore instance.
func NewPolicyStore(db *sql.DB) *PolicyStore {
	return &PolicyStore{
		executor: query.NewQueryExecutor(db, mapRowToPolicy),
	}
}

func (store *PolicyStore) Create(ctx context.Context, name string, policyType domain.PolicyType) (*domain.Policy, error) {
	var createdPolicy *domain.Policy

	err := store.executor.WithTx(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, queryCreatePolicy, name, policyType)
		if err != nil {
			return fmt.Errorf("failed to create policy %q: %w", name, err)
		}

		id, err := result.LastInsertId()
		if err != nil {
			return fmt.Errorf("failed to retrieve created policy id: %w", err)
		}

		row := tx.QueryRowContext(ctx, queryGetPolicyByID, id)
		policy, err := mapRowToPolicy(row.Scan)
		if err != nil {
			return fmt.Errorf("failed to retrieve created policy: %w", err)
		}

		createdPolicy = &policy
		return nil
	})

	if err != nil {
		return nil, err
	}

	return createdPolicy, nil
}

func (store *PolicyStore) GetByID(ctx context.Context, id int64) (*domain.Policy, error) {
	return store.executor.QueryOne(ctx, queryGetPolicyByID, id)
}

func (store *PolicyStore) GetByName(ctx context.Context, name string) (*domain.Policy, error) {
	return store.executor.QueryOne(ctx, queryGetPolicyByName, name)
}

func (store *PolicyStore) Exists(ctx context.Context, name string) (bool, error) {
	policy, err := store.GetByName(ctx, name)
	if err != nil {
		return false, err
	}

	return policy != nil, nil
}

func (store *PolicyStore) Update(ctx context.Context, policy domain.Policy) (*domain.Policy, error) {
	result, err := store.executor.Exec(ctx, queryUpdatePolicyByID, policy.Name, policy.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to update policy: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("failed to check rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return nil, fmt.Errorf("policy with id %d not found", policy.ID)
	}

	return store.GetByID(ctx, policy.ID)
}

func (store *PolicyStore) List(ctx context.Context) ([]domain.Policy, error) {
	return store.executor.QueryMany(ctx, queryGetAllPolicies)
}

func (store *PolicyStore) Remove(ctx context.Context, id int64) error {
	result, err := store.executor.Exec(ctx, queryDeletePolicyByID, id)
	if err != nil {
		return fmt.Errorf("failed to remove policy: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("policy with id %d not found", id)
	}

	return nil
}

// mapRowToPolicy maps a scan function to a domain.Policy struct.
func mapRowToPolicy(scan query.ScanFunc) (domain.Policy, error) {
	var policy domain.Policy
	if err := scan(&policy.ID, &policy.Name, &policy.Type, &policy.Version, &policy.CreatedAt, &policy.UpdatedAt); err != nil {
		return domain.Policy{}, err
	}

	return policy, nil
}
