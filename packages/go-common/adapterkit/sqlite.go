package adapterkit

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
)

// OpenSQLite opens the adapter's SQLite database at path, creating its parent directory with
// owner-only permissions. The caller's binary must register the "sqlite3" driver.
//
// The database is limited to one connection: adapters serialize their writes and rely on WAL
// plus the busy timeout for the occasional concurrent reader.
func OpenSQLite(path string) (*sql.DB, error) {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}
	if err := os.Chmod(directory, 0o700); err != nil {
		return nil, fmt.Errorf("secure database directory: %w", err)
	}
	db, err := sql.Open("sqlite3", fmt.Sprintf("file:%s?_foreign_keys=on&_busy_timeout=5000&_journal_mode=WAL", path))
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	return db, nil
}
