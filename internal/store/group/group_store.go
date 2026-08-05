package group

import (
	"context"
	"database/sql"
	"fmt"

	"dougdomingos.com/aegis/internal/domain"
	"dougdomingos.com/aegis/internal/store"
)

const (
	queryCreateGroup       = "INSERT INTO groups (name) VALUES (?)"
	queryGetGroupByID      = "SELECT id, name, created_at FROM groups WHERE id = ?"
	queryGetGroupByName    = "SELECT id, name, created_at FROM groups WHERE name = ?"
	queryGetAllGroups      = "SELECT id, name, created_at FROM groups"
	queryUpdateGroup       = "UPDATE groups SET name = ? WHERE id = ?"
	queryDeleteGroupByName = "DELETE FROM groups WHERE name = ?"
)

// GroupStore manages all database-related operations over groups.
type GroupStore struct {
	executor *store.QueryExecutor[domain.Group]
}

// NewGroupStore creates a new GroupStore instance.
func NewGroupStore(db *sql.DB) *GroupStore {
	return &GroupStore{
		executor: store.NewQueryExecutor(db, mapRowToGroup),
	}
}

func (store *GroupStore) Create(ctx context.Context, name string) (*domain.Group, error) {
	var createdGroup *domain.Group

	err := store.executor.WithTx(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, queryCreateGroup, name)
		if err != nil {
			return fmt.Errorf("failed to create group %q: %w", name, err)
		}

		id, err := result.LastInsertId()
		if err != nil {
			return fmt.Errorf("failed to retrieve created group id: %w", err)
		}

		row := tx.QueryRowContext(ctx, queryGetGroupByID, id)
		group, err := mapRowToGroup(row.Scan)
		if err != nil {
			return fmt.Errorf("failed to retrieve created group: %w", err)
		}

		createdGroup = &group
		return nil
	})

	if err != nil {
		return nil, err
	}

	return createdGroup, nil
}

func (store *GroupStore) GetByID(ctx context.Context, id int64) (*domain.Group, error) {
	return store.executor.QueryOne(ctx, queryGetGroupByID, id)
}

func (store *GroupStore) GetByName(ctx context.Context, name string) (*domain.Group, error) {
	return store.executor.QueryOne(ctx, queryGetGroupByName, name)
}

func (store *GroupStore) Exists(ctx context.Context, name string) (bool, error) {
	group, err := store.GetByName(ctx, name)
	if err != nil {
		return false, err
	}

	return group != nil, nil
}

func (store *GroupStore) Update(ctx context.Context, group domain.Group) (*domain.Group, error) {
	result, err := store.executor.Exec(ctx, queryUpdateGroup, group.Name, group.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to update group: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("failed to check rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return nil, fmt.Errorf("group with id %d not found", group.ID)
	}

	return &group, nil
}

func (store *GroupStore) List(ctx context.Context) ([]domain.Group, error) {
	return store.executor.QueryMany(ctx, queryGetAllGroups)
}

func (store *GroupStore) Remove(ctx context.Context, name string) error {
	result, err := store.executor.Exec(ctx, queryDeleteGroupByName, name)
	if err != nil {
		return fmt.Errorf("failed to remove group: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("group %q not found", name)
	}

	return nil
}

// mapRowToGroup maps a scan function to a domain.Group struct.
func mapRowToGroup(scan store.ScanFunc) (domain.Group, error) {
	var group domain.Group
	if err := scan(&group.ID, &group.Name, &group.CreatedAt); err != nil {
		return domain.Group{}, err
	}

	return group, nil
}
