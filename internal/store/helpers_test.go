package store_test

import (
	"context"
	"database/sql"
	"testing"
)

// arrangeStoreTest sets up an in-memory SQLite database and provisions the
// tables.
func arrangeStoreTest[T any](t *testing.T, newStoreFn func(db *sql.DB) *T, tableSchema string) (context.Context, *T) {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory database: %v", err)
	}

	if _, err := db.Exec(tableSchema); err != nil {
		t.Fatalf("failed to migrate database: %v", err)
	}

	t.Cleanup(func() {
		db.Close()
	})

	store := newStoreFn(db)
	return context.Background(), store
}
