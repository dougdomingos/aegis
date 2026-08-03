package store_test

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"

	"dougdomingos.com/aegis/internal/store"
)

const (
	// QUERY_COUNT_MIGRATIONS returns the number of applied migrations registered
	// in the database.
	QUERY_COUNT_MIGRATIONS = "SELECT COUNT(*) FROM schema_migrations"
)

// ============================================================================
// ApplyMigrations
// ============================================================================

func TestMigrations_ApplyMigrations_Successful(t *testing.T) {
	db := arrangeTest(t)

	if err := store.ApplyMigrations(db); err != nil {
		t.Fatalf("expected migrations to be applied flawlessly, got: %v", err)
	}

	var count int
	if err := db.QueryRow(QUERY_COUNT_MIGRATIONS).Scan(&count); err != nil {
		t.Fatalf("expected schema_migrations to exist, got %v", err)
	}

	if count == 0 {
		t.Fatal("expected schema_migrations to have entries, got none")
	}
}

func TestMigrations_ApplyMigrations_MultipleTimes_EnsuresIdempotency(t *testing.T) {
	var midCount, finalCount int
	db := arrangeTest(t)

	if err := store.ApplyMigrations(db); err != nil {
		t.Fatalf("expected migrations to be applied flawlessly, got: %v", err)
	}

	if err := db.QueryRow(QUERY_COUNT_MIGRATIONS).Scan(&midCount); err != nil {
		t.Fatalf("expected schema_migrations to exist, got %v", err)
	}

	if err := store.ApplyMigrations(db); err != nil {
		t.Fatalf("expected migrations to be applied flawlessly, got: %v", err)
	}

	if err := db.QueryRow(QUERY_COUNT_MIGRATIONS).Scan(&finalCount); err != nil {
		t.Fatalf("expected schema_migrations to exist, got %v", err)
	}

	if midCount != finalCount {
		t.Fatalf("expected migration count to stay unchanged, got %d", finalCount-midCount)
	}
}

func TestRunMigrations_FileCountMatch(t *testing.T) {
	var expectedCount, appliedCount int
	db := arrangeTest(t)

	if err := store.ApplyMigrations(db); err != nil {
		t.Fatalf("expected migrations to be applied flawlessly, got: %v", err)

	}

	entries, err := store.MigrationsFS.ReadDir("migrations")
	if err != nil {
		t.Fatalf("failed to read embedded migrations directory: %v", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			expectedCount++
		}
	}

	err = db.QueryRow(QUERY_COUNT_MIGRATIONS).Scan(&appliedCount)
	if err != nil {
		t.Fatalf("failed to query schema_migrations count: %v", err)
	}

	if appliedCount != expectedCount {
		t.Errorf("migration count mismatch: applied %d, expected %d files", appliedCount, expectedCount)
	}
}

// ============================================================================
// Helpers
// ============================================================================

func arrangeTest(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite", "file::memory:?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("failted to create test db: %v", err)
	}

	t.Cleanup(func() {
		db.Close()
	})

	return db
}
