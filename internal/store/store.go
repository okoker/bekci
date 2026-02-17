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
		s.migration002,
		s.migration003,
		s.migration004,
		s.migration005,
		s.migration006,
		s.migration007,
		s.migration008,
		s.migration009,
		s.migration010,
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
	INSERT INTO settings (key, value) VALUES ('audit_retention_days', '91');
	`
	_, err := s.db.Exec(schema)
	return err
}

// migration002 creates the monitoring tables: projects, targets, checks, check_results.
func (s *Store) migration002() error {
	schema := `
	CREATE TABLE projects (
		id          TEXT PRIMARY KEY,
		name        TEXT UNIQUE NOT NULL,
		description TEXT NOT NULL DEFAULT '',
		created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE targets (
		id          TEXT PRIMARY KEY,
		project_id  TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
		name        TEXT NOT NULL,
		host        TEXT NOT NULL,
		description TEXT NOT NULL DEFAULT '',
		enabled     INTEGER NOT NULL DEFAULT 1,
		created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(project_id, name)
	);

	CREATE TABLE checks (
		id          TEXT PRIMARY KEY,
		target_id   TEXT NOT NULL REFERENCES targets(id) ON DELETE CASCADE,
		type        TEXT NOT NULL CHECK(type IN ('http','tcp','ping','dns','page_hash','tls_cert')),
		name        TEXT NOT NULL,
		config      TEXT NOT NULL DEFAULT '{}',
		interval_s  INTEGER NOT NULL DEFAULT 300,
		enabled     INTEGER NOT NULL DEFAULT 1,
		created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE check_results (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		check_id    TEXT NOT NULL REFERENCES checks(id) ON DELETE CASCADE,
		status      TEXT NOT NULL CHECK(status IN ('up','down')),
		response_ms INTEGER NOT NULL DEFAULT 0,
		message     TEXT NOT NULL DEFAULT '',
		metrics     TEXT NOT NULL DEFAULT '{}',
		checked_at  DATETIME NOT NULL
	);

	CREATE INDEX idx_check_results_check_id ON check_results(check_id);
	CREATE INDEX idx_check_results_checked_at ON check_results(checked_at);
	CREATE INDEX idx_targets_project_id ON targets(project_id);
	CREATE INDEX idx_checks_target_id ON checks(target_id);
	`
	_, err := s.db.Exec(schema)
	return err
}

// migration003 adds preferred_check_type to targets and soc_public setting.
func (s *Store) migration003() error {
	_, err := s.db.Exec(`
		ALTER TABLE targets ADD COLUMN preferred_check_type TEXT NOT NULL DEFAULT 'ping';
		INSERT OR IGNORE INTO settings (key, value) VALUES ('soc_public', 'false');
	`)
	return err
}

// migration005 creates rules engine + alerting tables.
func (s *Store) migration005() error {
	_, err := s.db.Exec(`
		CREATE TABLE rules (
			id          TEXT PRIMARY KEY,
			name        TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			operator    TEXT NOT NULL DEFAULT 'AND' CHECK(operator IN ('AND','OR')),
			severity    TEXT NOT NULL DEFAULT 'critical' CHECK(severity IN ('critical','warning','info')),
			enabled     INTEGER NOT NULL DEFAULT 1,
			created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP
		);

		CREATE TABLE rule_conditions (
			id          TEXT PRIMARY KEY,
			rule_id     TEXT NOT NULL REFERENCES rules(id) ON DELETE CASCADE,
			check_id    TEXT NOT NULL REFERENCES checks(id) ON DELETE CASCADE,
			field       TEXT NOT NULL DEFAULT 'status',
			comparator  TEXT NOT NULL DEFAULT 'eq',
			value       TEXT NOT NULL,
			fail_count  INTEGER NOT NULL DEFAULT 1,
			fail_window INTEGER NOT NULL DEFAULT 0,
			sort_order  INTEGER NOT NULL DEFAULT 0
		);

		CREATE TABLE rule_states (
			rule_id        TEXT PRIMARY KEY REFERENCES rules(id) ON DELETE CASCADE,
			current_state  TEXT NOT NULL DEFAULT 'healthy',
			last_change    DATETIME,
			last_evaluated DATETIME
		);

		CREATE TABLE alert_channels (
			id         TEXT PRIMARY KEY,
			name       TEXT NOT NULL,
			type       TEXT NOT NULL,
			config     TEXT NOT NULL DEFAULT '{}',
			enabled    INTEGER NOT NULL DEFAULT 1,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);

		CREATE TABLE rule_alerts (
			id         TEXT PRIMARY KEY,
			rule_id    TEXT NOT NULL REFERENCES rules(id) ON DELETE CASCADE,
			channel_id TEXT NOT NULL REFERENCES alert_channels(id) ON DELETE CASCADE,
			cooldown_s INTEGER NOT NULL DEFAULT 1800,
			enabled    INTEGER NOT NULL DEFAULT 1
		);

		CREATE TABLE alert_history (
			id              INTEGER PRIMARY KEY AUTOINCREMENT,
			rule_id         TEXT NOT NULL,
			channel_id      TEXT,
			alert_type      TEXT NOT NULL,
			message         TEXT,
			sent_at         DATETIME DEFAULT CURRENT_TIMESTAMP,
			acknowledged    INTEGER NOT NULL DEFAULT 0,
			acknowledged_by TEXT REFERENCES users(id),
			acknowledged_at DATETIME
		);

		CREATE INDEX idx_ah_rule ON alert_history(rule_id, sent_at);
	`)
	return err
}

// migration006 adds operator, severity, rule_id to targets for unified target model.
func (s *Store) migration006() error {
	_, err := s.db.Exec(`
		ALTER TABLE targets ADD COLUMN operator TEXT NOT NULL DEFAULT 'AND';
		ALTER TABLE targets ADD COLUMN severity TEXT NOT NULL DEFAULT 'critical';
		ALTER TABLE targets ADD COLUMN rule_id TEXT DEFAULT NULL;
	`)
	if err != nil {
		return err
	}

	// Auto-link existing rules that map to a single target's checks
	rows, err := s.db.Query(`
		SELECT r.id, r.operator, r.severity, t.id as target_id
		FROM rules r
		JOIN rule_conditions rc ON rc.rule_id = r.id
		JOIN checks c ON c.id = rc.check_id
		JOIN targets t ON t.id = c.target_id
		GROUP BY r.id
		HAVING COUNT(DISTINCT t.id) = 1
	`)
	if err != nil {
		return nil // non-fatal — dev data only
	}
	defer rows.Close()

	for rows.Next() {
		var ruleID, op, sev, targetID string
		if err := rows.Scan(&ruleID, &op, &sev, &targetID); err != nil {
			continue
		}
		s.db.Exec(`UPDATE targets SET rule_id = ?, operator = ?, severity = ? WHERE id = ?`,
			ruleID, op, sev, targetID)
	}

	// Delete orphaned rules that span multiple targets
	s.db.Exec(`
		DELETE FROM rules WHERE id IN (
			SELECT r.id FROM rules r
			JOIN rule_conditions rc ON rc.rule_id = r.id
			JOIN checks c ON c.id = rc.check_id
			GROUP BY r.id
			HAVING COUNT(DISTINCT c.target_id) > 1
		)
	`)

	return nil
}

// migration004 removes projects — targets become standalone.
func (s *Store) migration004() error {
	// Disable FK checks for table rebuild
	if _, err := s.db.Exec(`PRAGMA foreign_keys = OFF`); err != nil {
		return err
	}
	defer s.db.Exec(`PRAGMA foreign_keys = ON`)

	_, err := s.db.Exec(`
		CREATE TABLE targets_new (
			id                   TEXT PRIMARY KEY,
			name                 TEXT UNIQUE NOT NULL,
			host                 TEXT NOT NULL,
			description          TEXT NOT NULL DEFAULT '',
			enabled              INTEGER NOT NULL DEFAULT 1,
			preferred_check_type TEXT NOT NULL DEFAULT 'ping',
			created_at           DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at           DATETIME DEFAULT CURRENT_TIMESTAMP
		);

		INSERT INTO targets_new (id, name, host, description, enabled, preferred_check_type, created_at, updated_at)
			SELECT id, name, host, description, enabled, preferred_check_type, created_at, updated_at FROM targets;

		DROP TABLE targets;
		ALTER TABLE targets_new RENAME TO targets;
		DROP TABLE IF EXISTS projects;
	`)
	return err
}

// migration008 creates the audit_logs table.
func (s *Store) migration008() error {
	_, err := s.db.Exec(`
		CREATE TABLE audit_logs (
			id            INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id       TEXT NOT NULL,
			username      TEXT NOT NULL,
			action        TEXT NOT NULL,
			resource_type TEXT NOT NULL DEFAULT '',
			resource_id   TEXT NOT NULL DEFAULT '',
			detail        TEXT NOT NULL DEFAULT '',
			ip_address    TEXT NOT NULL DEFAULT '',
			status        TEXT NOT NULL DEFAULT 'success',
			created_at    DATETIME DEFAULT CURRENT_TIMESTAMP
		);
		CREATE INDEX idx_audit_created ON audit_logs(created_at DESC);
	`)
	return err
}

// migration007 renames severity to category on targets.
func (s *Store) migration007() error {
	_, err := s.db.Exec(`ALTER TABLE targets RENAME COLUMN severity TO category`)
	if err != nil {
		return err
	}
	// Map old severity values to default category
	_, err = s.db.Exec(`UPDATE targets SET category = 'Other' WHERE category IN ('critical', 'warning', 'info')`)
	return err
}

// migration009 seeds audit_retention_days setting.
func (s *Store) migration009() error {
	_, err := s.db.Exec(`INSERT OR IGNORE INTO settings (key, value) VALUES ('audit_retention_days', '91')`)
	return err
}

// migration010 remaps old granular categories to simplified set.
func (s *Store) migration010() error {
	_, err := s.db.Exec(`
		UPDATE targets SET category = 'Network' WHERE category IN ('ISP', 'Router/Switch');
		UPDATE targets SET category = 'Security' WHERE category IN ('FW/WAF', 'VPN', 'SIEM/Logging', 'PAM/DAM', 'Security Other');
		UPDATE targets SET category = 'Key Services' WHERE category = 'IT Server';
	`)
	return err
}
