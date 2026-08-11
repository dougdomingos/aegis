package query_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"dougdomingos.com/aegis/internal/infra/query"
	_ "modernc.org/sqlite"
)

type TestEntity struct {
	ID   int64
	Name string
}

// ============================================================================
// QueryOne
// ============================================================================

func TestQueryExecutor_QueryOne_WhenRecordExists_ReturnsMappedItem(t *testing.T) {
	ctx, _, executor := setupTestExecutor(t)

	_, err := executor.Exec(ctx, "INSERT INTO test_entities (name) VALUES (?)", "Item A")
	if err != nil {
		t.Fatalf("failed to insert test data: %v", err)
	}

	result, err := executor.QueryOne(ctx, "SELECT id, name FROM test_entities WHERE name = ?", "Item A")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if result == nil {
		t.Fatal("expected result to be non-nil")
	}

	if result.Name != "Item A" {
		t.Errorf("expected name 'Item A', got %q", result.Name)
	}
}

func TestQueryExecutor_QueryOne_WhenNoRowsFound_ReturnsNilWithoutError(t *testing.T) {
	ctx, _, executor := setupTestExecutor(t)

	result, err := executor.QueryOne(ctx, "SELECT id, name FROM test_entities WHERE name = ?", "NonExistent")
	if err != nil {
		t.Fatalf("expected no error on missing row, got: %v", err)
	}

	if result != nil {
		t.Errorf("expected result to be nil when row is not found, got %+v", result)
	}
}

func TestQueryExecutor_QueryOne_WhenQueryFails_ReturnsError(t *testing.T) {
	ctx, db, executor := setupTestExecutor(t)
	db.Close()

	_, err := executor.QueryOne(ctx, "SELECT id, name FROM test_entities")
	if err == nil {
		t.Fatal("expected error on closed DB, got nil")
	}
}

func TestQueryExecutor_QueryOne_WhenMapperFails_ReturnsError(t *testing.T) {
	ctx, db, _ := setupTestExecutor(t)

	if _, err := db.Exec("INSERT INTO test_entities (name) VALUES (?)", "Item 1"); err != nil {
		t.Fatalf("failed to insert test data: %v", err)
	}

	mapperErr := errors.New("mapper failure")
	executor := query.NewQueryExecutor(db, func(scan query.ScanFunc) (TestEntity, error) {
		return TestEntity{}, mapperErr
	})

	_, err := executor.QueryOne(ctx, "SELECT id, name FROM test_entities WHERE name = ?", "Item A")
	if !errors.Is(err, mapperErr) {
		t.Fatalf("expected mapper error, got: %v", err)
	}
}

// ============================================================================
// QueryMany
// ============================================================================

func TestQueryExecutor_QueryMany_WhenRowsExist_ReturnsListOfItems(t *testing.T) {
	ctx, _, executor := setupTestExecutor(t)
	items := []string{"Item 1", "Item 2"}

	for _, item := range items {
		if _, err := executor.Exec(ctx, "INSERT INTO test_entities (name) VALUES (?)", item); err != nil {
			t.Fatalf("failed to insert test data: %v", err)
		}
	}

	results, err := executor.QueryMany(ctx, "SELECT id, name FROM test_entities ORDER BY id ASC")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 items, got %d", len(results))
	}

	if results[0].Name != "Item 1" || results[1].Name != "Item 2" {
		t.Errorf("unexpected mapped names in slice: %+v", results)
	}
}

func TestQueryExecutor_QueryMany_WhenNoRowsExist_ReturnsEmptyList(t *testing.T) {
	ctx, _, executor := setupTestExecutor(t)

	results, err := executor.QueryMany(ctx, "SELECT id, name FROM test_entities")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if len(results) != 0 {
		t.Errorf("expected empty slice, got %d items", len(results))
	}
}

func TestQueryExecutor_QueryMany_WhenQueryFails_ReturnsError(t *testing.T) {
	ctx, db, executor := setupTestExecutor(t)
	db.Close()

	_, err := executor.QueryMany(ctx, "SELECT id, name FROM test_entities")
	if err == nil {
		t.Fatal("expected error on closed DB, got nil")
	}
}

func TestQueryExecutor_QueryMany_WhenMapperFails_ReturnsError(t *testing.T) {
	ctx, db, _ := setupTestExecutor(t)
	items := []string{"Item 1", "Item 2"}

	for _, item := range items {
		if _, err := db.Exec("INSERT INTO test_entities (name) VALUES (?)", item); err != nil {
			t.Fatalf("failed to insert test data: %v", err)
		}
	}

	callCount := 0
	mapperErr := errors.New("mapper failure on second row")
	executor := query.NewQueryExecutor(db, func(scan query.ScanFunc) (TestEntity, error) {
		callCount++
		if callCount == 2 {
			return TestEntity{}, mapperErr
		}
		var item TestEntity
		return item, scan(&item.ID, &item.Name)
	})

	_, err := executor.QueryMany(ctx, "SELECT id, name FROM test_entities ORDER BY id ASC")
	if !errors.Is(err, mapperErr) {
		t.Fatalf("expected mapper error, got: %v", err)
	}
}

// ============================================================================
// Exec
// ============================================================================

func TestQueryExecutor_Exec_ExecutesStatementSuccessfully(t *testing.T) {
	ctx, _, executor := setupTestExecutor(t)

	result, err := executor.Exec(ctx, "INSERT INTO test_entities (name) VALUES (?)", "New Entity")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		t.Fatalf("failed to check rows affected: %v", err)
	}

	if rowsAffected != 1 {
		t.Errorf("expected 1 row affected, got %d", rowsAffected)
	}
}

func TestQueryExecutor_Exec_WhenStatementFails_ReturnsError(t *testing.T) {
	ctx, db, executor := setupTestExecutor(t)
	db.Close()

	_, err := executor.Exec(ctx, "INSERT INTO test_entities (name) VALUES (?)", "Entity")
	if err == nil {
		t.Fatal("expected error on closed DB, got nil")
	}
}

// ============================================================================
// WithTx
// ============================================================================

func TestQueryExecutor_WithTx_WhenCallbackSucceeds_CommitsTransaction(t *testing.T) {
	ctx, db, executor := setupTestExecutor(t)

	err := executor.WithTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "INSERT INTO test_entities (name) VALUES (?)", "Tx Entity")
		return err
	})

	if err != nil {
		t.Fatalf("expected transaction to succeed, got: %v", err)
	}

	var count int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM test_entities WHERE name = ?", "Tx Entity").Scan(&count); err != nil {
		t.Fatalf("failed to count rows: %v", err)
	}

	if count != 1 {
		t.Errorf("expected record to be committed, found count: %d", count)
	}
}

func TestQueryExecutor_WithTx_WhenBeginFails_ReturnsError(t *testing.T) {
	ctx, db, executor := setupTestExecutor(t)
	db.Close()

	err := executor.WithTx(ctx, func(tx *sql.Tx) error {
		return nil
	})

	if err == nil {
		t.Fatal("expected error when DB is closed, got nil")
	}
}

func TestQueryExecutor_WithTx_WhenCallbackFails_RollsBackTransaction(t *testing.T) {
	ctx, db, executor := setupTestExecutor(t)
	expectedErr := errors.New("aborted transaction test")

	err := executor.WithTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "INSERT INTO test_entities (name) VALUES (?)", "Should Rollback")
		if err != nil {
			return err
		}

		return expectedErr
	})

	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected error %v, got %v", expectedErr, err)
	}

	// Verify the record was rolled back
	var count int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM test_entities WHERE name = ?", "Should Rollback").Scan(&count); err != nil {
		t.Fatalf("failed to count rows: %v", err)
	}

	if count != 0 {
		t.Errorf("expected record to be rolled back, found count: %d", count)
	}
}

// ============================================================================
// Helper
// ============================================================================

func setupTestExecutor(t *testing.T) (context.Context, *sql.DB, *query.QueryExecutor[TestEntity]) {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}

	schema := `
	CREATE TABLE test_entities (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL
	);`

	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("failed to initialize schema: %v", err)
	}

	t.Cleanup(func() {
		_ = db.Close()
	})

	executor := query.NewQueryExecutor(db, mapTestEntity)
	return context.Background(), db, executor
}

func mapTestEntity(scan query.ScanFunc) (TestEntity, error) {
	var item TestEntity
	err := scan(&item.ID, &item.Name)
	return item, err
}
