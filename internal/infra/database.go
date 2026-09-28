package infra

import (
	"database/sql"
	"fmt"
	"sync"
	"time"

	"dougdomingos.com/aegis/internal/infra/migrations"
	_ "modernc.org/sqlite"
)

var (
	// once is used to ensure that the database initialization is executed
	// only once after every startup.
	once sync.Once

	// db is the singleton database instance, used throughout the whole project.
	db *sql.DB
)

// InitDB initializes the database instance shared by all stores.
func InitDB(path string) (*sql.DB, error) {
	var err error

	once.Do(func() {
		dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(ON)", path)
		db, err = sql.Open("sqlite", dsn)
		if err != nil {
			return
		}

		db.SetMaxOpenConns(1)
		db.SetMaxIdleConns(1)
		db.SetConnMaxLifetime(1 * time.Hour)

		err = db.Ping()

		err = migrations.ApplyMigrations(db)
	})

	if err != nil {
		return nil, err
	}

	return db, nil
}
