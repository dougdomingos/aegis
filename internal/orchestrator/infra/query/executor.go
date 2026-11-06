package query

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// ScanFunc abstracts the Scan method from sql.Row and sql.Rows, which allows
// the mapping function to be used on both contexts.
type ScanFunc func(dest ...any) error

// RowMapper maps a result row to a value of type T.
type RowMapper[T any] func(scan ScanFunc) (T, error)

// QueryExecutor manages database operations and maps query results to type T.
type QueryExecutor[T any] struct {
	db     *sql.DB
	mapper RowMapper[T]
}

// NewQueryExecutor creates a new QueryExecutor for type T.
func NewQueryExecutor[T any](db *sql.DB, mapper RowMapper[T]) *QueryExecutor[T] {
	return &QueryExecutor[T]{db: db, mapper: mapper}
}

// QueryOne executes a query expected to return at most one row.
func (queryExec *QueryExecutor[T]) QueryOne(ctx context.Context, query string, args ...any) (*T, error) {
	row := queryExec.db.QueryRowContext(ctx, query, args...)

	result, err := queryExec.mapper(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("failed to map row: %w", err)
	}

	return &result, nil
}

// QueryMany executes a query and returns a slice of mapped results.
func (queryExec *QueryExecutor[T]) QueryMany(ctx context.Context, query string, args ...any) ([]T, error) {
	rows, err := queryExec.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to execute query: %w", err)
	}
	defer rows.Close()

	var results []T
	for rows.Next() {
		result, err := queryExec.mapper(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("failed to map row: %w", err)
		}

		results = append(results, result)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return results, nil
}

// Exec executes a query without returning any data rows.
func (queryExec *QueryExecutor[T]) Exec(ctx context.Context, query string, args ...any) (sql.Result, error) {
	result, err := queryExec.db.ExecContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to execute statement: %w", err)
	}

	return result, nil
}

// WithTx executes fn within a database transaction, committing on success or rolling back on error.
func (queryExec *QueryExecutor[T]) WithTx(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := queryExec.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}
