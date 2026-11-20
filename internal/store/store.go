// Package store provides the SQLite persistence layer for the panel.
package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite" // pure-Go SQLite driver
)

// Store wraps the panel SQLite database.
type Store struct {
	db *sql.DB
}

// Open creates dataDir if needed, opens dataDir/panel.db with WAL pragmas
// and applies pending migrations. Repeated calls on the same directory are
// safe and idempotent.
func Open(dataDir string) (*Store, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	dsn := filepath.Join(dataDir, "panel.db") +
		"?_journal_mode=WAL&_busy_timeout=5000&_synchronous=FULL"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// DB returns the underlying *sql.DB.
func (s *Store) DB() *sql.DB { return s.db }

// Close closes the database.
func (s *Store) Close() error { return s.db.Close() }

// migrate applies every embedded migration not yet recorded in
// schema_version, each in its own transaction.
func (s *Store) migrate() error {
	if _, err := s.db.Exec(
		"CREATE TABLE IF NOT EXISTS schema_version (version INTEGER NOT NULL)"); err != nil {
		return fmt.Errorf("create schema_version: %w", err)
	}
	names, err := migrationNames()
	if err != nil {
		return err
	}
	for _, name := range names {
		version, err := migrationVersion(name)
		if err != nil {
			return err
		}
		var applied int
		if err := s.db.QueryRow(
			"SELECT COUNT(1) FROM schema_version WHERE version = ?", version,
		).Scan(&applied); err != nil {
			return fmt.Errorf("check migration %d: %w", version, err)
		}
		if applied > 0 {
			continue
		}
		script, err := migrationsFS.ReadFile(name)
		if err != nil {
			return fmt.Errorf("read migration %q: %w", name, err)
		}
		if err := s.applyMigration(version, string(script)); err != nil {
			return fmt.Errorf("apply migration %q: %w", name, err)
		}
	}
	return nil
}

func (s *Store) applyMigration(version int, script string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	if _, err := tx.Exec(script); err != nil {
		tx.Rollback()
		return err
	}
	if _, err := tx.Exec(
		"INSERT INTO schema_version(version) VALUES (?)", version); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}
