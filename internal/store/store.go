package store

import (
	"database/sql"
	"fmt"
	"log/slog"
	"os"

	_ "github.com/mattn/go-sqlite3"
)

type Store struct {
	db *sql.DB
}

// New creates a new store, runs migrations, and seeds defaults.
func New(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=ON")
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrating database: %w", err)
	}

	os.Chmod(dbPath, 0600)
	return s, nil
}

// Close closes the database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate() error {
	// Create schema_version table
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS schema_version (version INTEGER NOT NULL)`)
	if err != nil {
		return fmt.Errorf("creating schema_version table: %w", err)
	}

	var current int
	err = s.db.QueryRow(`SELECT version FROM schema_version LIMIT 1`).Scan(&current)
	if err == sql.ErrNoRows {
		_, err = s.db.Exec(`INSERT INTO schema_version (version) VALUES (0)`)
		if err != nil {
			return err
		}
		current = 0
	} else if err != nil {
		return err
	}

	migrations := []func() error{
		s.migration001,
	}

	for i := current; i < len(migrations); i++ {
		slog.Info("Running migration", "version", i+1)
		if err := migrations[i](); err != nil {
			return fmt.Errorf("migration %d: %w", i+1, err)
		}
		if _, err := s.db.Exec(`UPDATE schema_version SET version = ?`, i+1); err != nil {
			return err
		}
	}

	return nil
}

// migration001 creates the v2 auth tables.
func (s *Store) migration001() error {
	schema := `
	CREATE TABLE users (
		id            TEXT PRIMARY KEY,
		username      TEXT UNIQUE NOT NULL,
		email         TEXT NOT NULL DEFAULT '',
		password_hash TEXT NOT NULL,
		role          TEXT NOT NULL CHECK(role IN ('admin','operator','viewer')),
		status        TEXT NOT NULL DEFAULT 'active' CHECK(status IN ('active','suspended')),
		created_at    DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at    DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE sessions (
		id         TEXT PRIMARY KEY,
		user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		expires_at DATETIME NOT NULL,
		ip_address TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX idx_sessions_user_id ON sessions(user_id);
	CREATE INDEX idx_sessions_expires_at ON sessions(expires_at);

	CREATE TABLE settings (
		key   TEXT PRIMARY KEY,
		value TEXT NOT NULL
	);

	INSERT INTO settings (key, value) VALUES ('session_timeout_hours', '24');
	INSERT INTO settings (key, value) VALUES ('history_days', '90');
	INSERT INTO settings (key, value) VALUES ('default_check_interval', '300');
	`
	_, err := s.db.Exec(schema)
	return err
}
