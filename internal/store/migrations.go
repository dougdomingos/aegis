package store

import (
	"database/sql"
	"embed"
	"fmt"
	"sort"
)

const (
	// QUERY_INIT_MIGRATION_TABLE creates the migration table in the database, if
	// it does not exist.
	QUERY_INIT_MIGRATION_TABLE = `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version TEXT PRIMARY KEY,
			applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);
	`

	// QUERY_DOES_MIGRATION_EXISTS returns a boolean value, indicating whether
	// the requested migration was processed or not.
	QUERY_DOES_MIGRATION_EXISTS = "SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = ?)"

	// QUERY_INSERT_NEW_MIGRATION creates a new entry for the provided migration.
	QUERY_INSERT_NEW_MIGRATION = "INSERT INTO schema_migrations (version) VALUES (?)"
)

// MigrationsFS integrates the migration directory with the Go application,
// allowing the SQL files to be embedded into the code seamlessly.
//
//go:embed "migrations/*.sql"
var MigrationsFS embed.FS

// ApplyMigrations executes all unapplied database migrations found in the
// embedded filesystem.
//
// It ensures the schema migration tracking table exists, loads migration files
// from the "migrations" directory, sorts them lexicographically by filename,
// and executes pending scripts sequentially within isolated database
// transactions.
func ApplyMigrations(db *sql.DB) error {
	_, err := db.Exec(QUERY_INIT_MIGRATION_TABLE)
	if err != nil {
		return fmt.Errorf("failed to create schema_migrations table: %w", err)
	}

	entries, err := MigrationsFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("failed to read migrations directory: %w", err)
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	for _, entry := range entries {
		var doesEntryExists bool
		version := entry.Name()

		if err := db.QueryRow(QUERY_DOES_MIGRATION_EXISTS, version).Scan(&doesEntryExists); err != nil {
			return fmt.Errorf("failed to verify status of migration %s: %w", version, err)
		}

		if doesEntryExists {
			continue
		}

		content, err := MigrationsFS.ReadFile("migrations/" + version)
		if err != nil {
			return fmt.Errorf("failed to read migration file %s: %w", version, err)
		}

		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("failed to init transaction for migration %s: %w", version, err)
		}

		if _, err := tx.Exec(string(content)); err != nil {
			if rbErr := tx.Rollback(); rbErr != nil {
				return fmt.Errorf("unable to execute migration %s: %w (rollback failed: %v)", version, err, rbErr)
			}
			return fmt.Errorf("unable to execute migration %s: %w", version, err)
		}

		if _, err := tx.Exec(QUERY_INSERT_NEW_MIGRATION, version); err != nil {
			if rbErr := tx.Rollback(); rbErr != nil {
				return fmt.Errorf("failed to register status of migration %s: %w (rollback failed: %v)", version, err, rbErr)
			}
			return fmt.Errorf("failed to register status of migration %s: %w", version, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("failed to confirm transaction of migration %s: %w", version, err)
		}
	}

	return nil
}
