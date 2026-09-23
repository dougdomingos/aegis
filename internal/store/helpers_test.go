package store_test

import (
	"context"
	"database/sql"
	"testing"

	"dougdomingos.com/aegis/internal/infra/migrations"
)

// arrangeStoreTest sets up an in-memory SQLite database and applies all
// migrations to provision the schema.
func arrangeStoreTest[T any](t *testing.T, newStoreFn func(db *sql.DB) *T) (context.Context, *T) {
	t.Helper()

	db, err := sql.Open("sqlite", "file::memory:?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("failed to open in-memory database: %v", err)
	}

	if err := migrations.ApplyMigrations(db); err != nil {
		t.Fatalf("failed to migrate database: %v", err)
	}

	t.Cleanup(func() {
		db.Close()
	})

	store := newStoreFn(db)
	return context.Background(), store
}
