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

	db.SetMaxOpenConns(1)

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrating database: %w", err)
	}

	for _, f := range []string{dbPath, dbPath + "-wal", dbPath + "-shm"} {
		if err := os.Chmod(f, 0600); err != nil && !os.IsNotExist(err) {
			slog.Warn("failed to chmod database file", "path", f, "error", err)
		}
	}
	return s, nil
}

// Close closes the database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

const schemaVersion = 22

// baselineSchema is the complete DDL for a fresh install at schema version 24.
// It is equivalent to running migration001 through migration023 on an empty database.
const baselineSchema = `
CREATE TABLE users (
	id            TEXT PRIMARY KEY,
	username      TEXT UNIQUE NOT NULL,
	email         TEXT NOT NULL DEFAULT '',
	password_hash TEXT NOT NULL,
	role          TEXT NOT NULL CHECK(role IN ('admin','operator','viewer')),
	status        TEXT NOT NULL DEFAULT 'active' CHECK(status IN ('active','suspended')),
	created_at    DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at    DATETIME DEFAULT CURRENT_TIMESTAMP,
	phone         TEXT NOT NULL DEFAULT ''
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

CREATE TABLE targets (
	id                   TEXT PRIMARY KEY,
	name                 TEXT UNIQUE NOT NULL,
	host                 TEXT NOT NULL,
	description          TEXT NOT NULL DEFAULT '',
	enabled              INTEGER NOT NULL DEFAULT 1,
	preferred_check_type TEXT NOT NULL DEFAULT 'ping',
	created_at           DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at           DATETIME DEFAULT CURRENT_TIMESTAMP,
	operator             TEXT NOT NULL DEFAULT 'AND',
	category             TEXT NOT NULL DEFAULT 'critical',
	rule_id              TEXT DEFAULT NULL,
	paused_at            DATETIME DEFAULT NULL,
	notes                TEXT DEFAULT NULL,
	contacts             TEXT DEFAULT NULL,
	project              TEXT DEFAULT NULL,
	location             TEXT DEFAULT NULL
);

CREATE INDEX idx_targets_rule_id ON targets(rule_id);

CREATE TABLE checks (
	id          TEXT PRIMARY KEY,
	target_id   TEXT NOT NULL REFERENCES targets(id) ON DELETE CASCADE,
	type        TEXT NOT NULL CHECK(type IN ('http','tcp','ping','dns','page_hash','tls_cert','snmp_v2c','snmp_v3')),
	name        TEXT NOT NULL,
	config      TEXT NOT NULL DEFAULT '{}',
	interval_s  INTEGER NOT NULL DEFAULT 300,
	enabled     INTEGER NOT NULL DEFAULT 1,
	created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_checks_target_id ON checks(target_id);

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
CREATE INDEX idx_check_results_check_id_checked_at ON check_results(check_id, checked_at DESC);

CREATE TABLE check_state (
	check_id    TEXT PRIMARY KEY REFERENCES checks(id) ON DELETE CASCADE,
	status      TEXT NOT NULL CHECK(status IN ('up','down')),
	response_ms INTEGER NOT NULL DEFAULT 0,
	message     TEXT NOT NULL DEFAULT '',
	metrics     TEXT NOT NULL DEFAULT '{}',
	checked_at  DATETIME NOT NULL
);

CREATE TABLE check_daily_rollups (
	check_id        TEXT NOT NULL REFERENCES checks(id) ON DELETE CASCADE,
	day             TEXT NOT NULL,
	total_count     INTEGER NOT NULL DEFAULT 0,
	up_count        INTEGER NOT NULL DEFAULT 0,
	down_count      INTEGER NOT NULL DEFAULT 0,
	avg_response_ms INTEGER NOT NULL DEFAULT 0,
	max_response_ms INTEGER NOT NULL DEFAULT 0,
	PRIMARY KEY (check_id, day)
);

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
	id              TEXT PRIMARY KEY,
	rule_id         TEXT NOT NULL REFERENCES rules(id) ON DELETE CASCADE,
	check_id        TEXT NOT NULL REFERENCES checks(id) ON DELETE CASCADE,
	field           TEXT NOT NULL DEFAULT 'status',
	comparator      TEXT NOT NULL DEFAULT 'eq',
	value           TEXT NOT NULL,
	fail_count      INTEGER NOT NULL DEFAULT 1,
	fail_window     INTEGER NOT NULL DEFAULT 0,
	sort_order      INTEGER NOT NULL DEFAULT 0,
	condition_group INTEGER NOT NULL DEFAULT 0,
	group_operator  TEXT NOT NULL DEFAULT 'AND'
);

CREATE INDEX idx_rule_conditions_check_id ON rule_conditions(check_id);
CREATE INDEX idx_rule_conditions_rule_id ON rule_conditions(rule_id, condition_group, sort_order);

CREATE TABLE rule_states (
	rule_id        TEXT PRIMARY KEY REFERENCES rules(id) ON DELETE CASCADE,
	current_state  TEXT NOT NULL DEFAULT 'healthy',
	last_change    DATETIME,
	last_evaluated DATETIME
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
	acknowledged_at DATETIME,
	target_id       TEXT NOT NULL DEFAULT '',
	recipient_id    TEXT NOT NULL DEFAULT ''
);

CREATE INDEX idx_ah_rule ON alert_history(rule_id, sent_at);

CREATE TABLE target_alert_recipients (
	target_id TEXT NOT NULL REFERENCES targets(id) ON DELETE CASCADE,
	user_id   TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	PRIMARY KEY (target_id, user_id)
);

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

CREATE TABLE target_pause_history (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	target_id  TEXT NOT NULL REFERENCES targets(id) ON DELETE CASCADE,
	paused_at  DATETIME NOT NULL,
	resumed_at DATETIME,
	reason     TEXT NOT NULL DEFAULT ''
);

CREATE INDEX idx_pause_history_target ON target_pause_history(target_id);

CREATE TABLE tag_options (
	id    INTEGER PRIMARY KEY AUTOINCREMENT,
	grp   TEXT NOT NULL CHECK(grp IN ('project', 'location', 'category')),
	value TEXT NOT NULL,
	UNIQUE(grp, value)
);

CREATE TABLE schema_version (version INTEGER NOT NULL);
INSERT INTO schema_version (version) VALUES (24);

INSERT INTO tag_options (grp, value) VALUES ('category', 'Key Services');
INSERT INTO tag_options (grp, value) VALUES ('category', 'Network');
INSERT INTO tag_options (grp, value) VALUES ('category', 'Other');
INSERT INTO tag_options (grp, value) VALUES ('category', 'Physical Security');
INSERT INTO tag_options (grp, value) VALUES ('category', 'Security');

INSERT INTO settings (key, value) VALUES ('session_timeout_hours', '24');
INSERT INTO settings (key, value) VALUES ('history_days', '3');
INSERT INTO settings (key, value) VALUES ('audit_retention_days', '91');
INSERT INTO settings (key, value) VALUES ('soc_public', 'false');
INSERT INTO settings (key, value) VALUES ('alert_method', 'email');
INSERT INTO settings (key, value) VALUES ('resend_api_key', '');
INSERT INTO settings (key, value) VALUES ('alert_from_email', '');
INSERT INTO settings (key, value) VALUES ('alert_cooldown_s', '1800');
INSERT INTO settings (key, value) VALUES ('alert_realert_s', '3600');
INSERT INTO settings (key, value) VALUES ('sla_network', '99.9');
INSERT INTO settings (key, value) VALUES ('sla_security', '99.9');
INSERT INTO settings (key, value) VALUES ('sla_physical_security', '99.9');
INSERT INTO settings (key, value) VALUES ('sla_key_services', '99.9');
INSERT INTO settings (key, value) VALUES ('sla_other', '99.9');
INSERT INTO settings (key, value) VALUES ('signal_api_url', '');
INSERT INTO settings (key, value) VALUES ('signal_number', '');
INSERT INTO settings (key, value) VALUES ('signal_username', '');
INSERT INTO settings (key, value) VALUES ('signal_password', '');
INSERT INTO settings (key, value) VALUES ('signal_skip_tls', 'false');
INSERT INTO settings (key, value) VALUES ('snmp_v2c_community', 'public');
INSERT INTO settings (key, value) VALUES ('snmp_v3_username', '');
INSERT INTO settings (key, value) VALUES ('snmp_v3_security_level', 'authPriv');
INSERT INTO settings (key, value) VALUES ('snmp_v3_auth_protocol', 'SHA');
INSERT INTO settings (key, value) VALUES ('snmp_v3_auth_passphrase', '');
INSERT INTO settings (key, value) VALUES ('snmp_v3_privacy_protocol', 'AES');
INSERT INTO settings (key, value) VALUES ('snmp_v3_privacy_passphrase', '');
`

func (s *Store) migrate() error {
	// Check if schema_version table exists (distinguishes fresh install from upgrade)
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='schema_version'`).Scan(&count)
	if err != nil {
		return fmt.Errorf("checking schema_version: %w", err)
	}

	if count == 0 {
		// Fresh install — run baseline schema
		slog.Info("Fresh install: creating baseline schema", "version", schemaVersion)
		if _, err := s.db.Exec(baselineSchema); err != nil {
			return fmt.Errorf("baseline schema: %w", err)
		}
		return nil
	}

	// Existing install — run any migrations beyond the baseline version
	var current int
	if err := s.db.QueryRow(`SELECT version FROM schema_version LIMIT 1`).Scan(&current); err != nil {
		return fmt.Errorf("reading schema version: %w", err)
	}

	if current < schemaVersion {
		// Bridge: apply missing pre-baseline migrations for known versions.
		// Only v21→v22 is expected (prod was deployed before migration022 existed).
		if current == 21 {
			slog.Info("Applying bridge migration 022", "from", current, "to", schemaVersion)
			if _, err := s.db.Exec(`UPDATE settings SET value = '3' WHERE key = 'history_days' AND value = '90'`); err != nil {
				return fmt.Errorf("bridge migration 022: %w", err)
			}
			if _, err := s.db.Exec(`UPDATE schema_version SET version = ?`, schemaVersion); err != nil {
				return err
			}
			current = schemaVersion
		} else {
			return fmt.Errorf("schema version %d is below baseline %d; cannot auto-upgrade (old migrations removed)", current, schemaVersion)
		}
	}

	// Future migrations go here (24, 25, ...)
	migrations := []func() error{
		s.migration023,
		s.migration024,
	}

	for i := current - schemaVersion; i < len(migrations); i++ {
		ver := schemaVersion + i + 1
		slog.Info("Running migration", "version", ver)
		if err := migrations[i](); err != nil {
			return fmt.Errorf("migration %d: %w", ver, err)
		}
		if _, err := s.db.Exec(`UPDATE schema_version SET version = ?`, ver); err != nil {
			return err
		}
	}

	return nil
}

func (s *Store) migration023() error {
	// SQLite doesn't support ALTER CHECK, so recreate the table
	_, err := s.db.Exec(`
		CREATE TABLE tag_options_new (
			id    INTEGER PRIMARY KEY AUTOINCREMENT,
			grp   TEXT NOT NULL CHECK(grp IN ('project', 'location', 'category')),
			value TEXT NOT NULL,
			UNIQUE(grp, value)
		);
		INSERT INTO tag_options_new (id, grp, value) SELECT id, grp, value FROM tag_options;
		DROP TABLE tag_options;
		ALTER TABLE tag_options_new RENAME TO tag_options;
	`)
	if err != nil {
		return fmt.Errorf("recreate tag_options: %w", err)
	}

	cats := []string{"Key Services", "Network", "Other", "Physical Security", "Security"}
	for _, c := range cats {
		if _, err := s.db.Exec(`INSERT OR IGNORE INTO tag_options (grp, value) VALUES ('category', ?)`, c); err != nil {
			return fmt.Errorf("seed category %q: %w", c, err)
		}
	}
	return nil
}

func (s *Store) migration024() error {
	_, err := s.db.Exec(`INSERT OR IGNORE INTO settings (key, value) VALUES ('signal_skip_tls', 'false')`)
	return err
}

// SchemaVersion returns the current database schema version.
func (s *Store) SchemaVersion() (int, error) {
	var v int
	if err := s.db.QueryRow(`SELECT version FROM schema_version LIMIT 1`).Scan(&v); err != nil {
		return 0, fmt.Errorf("reading schema version: %w", err)
	}
	return v, nil
}
