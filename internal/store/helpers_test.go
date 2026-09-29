package store_test

import (
	"context"
	"database/sql"
	"testing"

	"dougdomingos.com/aegis/internal/infra/migrations"
)

// arrangeStoreDB sets up an in-memory SQLite database and applies all
// migrations to provision the schema.
func arrangeStoreDB(t *testing.T) (context.Context, *sql.DB) {
	t.Helper()

	db, err := sql.Open("sqlite", "file::memory:?mode=memory&cache=shared&_pragma=foreign_keys(ON)")
	if err != nil {
		t.Fatalf("failed to open in-memory database: %v", err)
	}

	if err := migrations.ApplyMigrations(db); err != nil {
		t.Fatalf("failed to migrate database: %v", err)
	}

	t.Cleanup(func() {
		db.Close()
	})

	return context.Background(), db
}

// arrangeStoreTest provisions the schema and builds a store instance over it.
func arrangeStoreTest[T any](t *testing.T, newStoreFn func(db *sql.DB) *T) (context.Context, *T) {
	t.Helper()

	ctx, db := arrangeStoreDB(t)
	return ctx, newStoreFn(db)
}
