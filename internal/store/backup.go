package store

import (
	"database/sql"
	"fmt"
	"time"
)

// BackupData is the top-level structure for a Bekci config backup.
type BackupData struct {
	Version       int               `json:"version"`
	SchemaVersion int               `json:"schema_version"`
	CreatedAt     string            `json:"created_at"`
	AppVersion    string            `json:"app_version"`
	Users         []BackupUser      `json:"users"`
	Settings      map[string]string `json:"settings"`
	Rules         []BackupRule      `json:"rules"`
	Targets       []BackupTarget    `json:"targets"`
	Checks        []BackupCheck     `json:"checks"`
	RuleConditions []RuleCondition  `json:"rule_conditions"`
	RuleStates    []RuleState       `json:"rule_states"`
}

// BackupUser mirrors User but exposes password_hash in JSON.
type BackupUser struct {
	ID           string    `json:"id"`
	Username     string    `json:"username"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"password_hash"`
	Role         string    `json:"role"`
	Status       string    `json:"status"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// BackupRule mirrors Rule for backup (severity column in DB is still named severity).
type BackupRule struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Operator    string    `json:"operator"`
	Severity    string    `json:"severity"`
	Enabled     bool      `json:"enabled"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// BackupTarget mirrors Target for backup.
type BackupTarget struct {
	ID                 string  `json:"id"`
	Name               string  `json:"name"`
	Host               string  `json:"host"`
	Description        string  `json:"description"`
	Enabled            bool    `json:"enabled"`
	PreferredCheckType string  `json:"preferred_check_type"`
	Operator           string  `json:"operator"`
	Category           string  `json:"category"`
	RuleID             *string `json:"rule_id"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// BackupCheck mirrors Check for backup.
type BackupCheck struct {
	ID        string    `json:"id"`
	TargetID  string    `json:"target_id"`
	Type      string    `json:"type"`
	Name      string    `json:"name"`
	Config    string    `json:"config"`
	IntervalS int       `json:"interval_s"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ExportBackup reads all config tables and assembles a BackupData.
func (s *Store) ExportBackup(appVersion string) (*BackupData, error) {
	data := &BackupData{
		Version:       1,
		CreatedAt:     time.Now().UTC().Format(time.RFC3339),
		AppVersion:    appVersion,
		Settings:      make(map[string]string),
	}

	// Schema version
	if err := s.db.QueryRow(`SELECT version FROM schema_version LIMIT 1`).Scan(&data.SchemaVersion); err != nil {
		return nil, fmt.Errorf("reading schema version: %w", err)
	}

	// Users (with password_hash)
	rows, err := s.db.Query(`SELECT id, username, email, password_hash, role, status, created_at, updated_at FROM users ORDER BY created_at ASC`)
	if err != nil {
		return nil, fmt.Errorf("exporting users: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var u BackupUser
		if err := rows.Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.Role, &u.Status, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, err
		}
		data.Users = append(data.Users, u)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Settings
	data.Settings, err = s.GetAllSettings()
	if err != nil {
		return nil, fmt.Errorf("exporting settings: %w", err)
	}

	// Rules
	rRows, err := s.db.Query(`SELECT id, name, description, operator, severity, enabled, created_at, updated_at FROM rules ORDER BY created_at ASC`)
	if err != nil {
		return nil, fmt.Errorf("exporting rules: %w", err)
	}
	defer rRows.Close()
	for rRows.Next() {
		var r BackupRule
		var enabled int
		if err := rRows.Scan(&r.ID, &r.Name, &r.Description, &r.Operator, &r.Severity, &enabled, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		r.Enabled = enabled == 1
		data.Rules = append(data.Rules, r)
	}
	if err := rRows.Err(); err != nil {
		return nil, err
	}

	// Targets
	tRows, err := s.db.Query(`SELECT id, name, host, description, enabled, preferred_check_type, operator, category, rule_id, created_at, updated_at FROM targets ORDER BY name ASC`)
	if err != nil {
		return nil, fmt.Errorf("exporting targets: %w", err)
	}
	defer tRows.Close()
	for tRows.Next() {
		var t BackupTarget
		var enabled int
		var ruleID sql.NullString
		if err := tRows.Scan(&t.ID, &t.Name, &t.Host, &t.Description, &enabled, &t.PreferredCheckType, &t.Operator, &t.Category, &ruleID, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		t.Enabled = enabled == 1
		if ruleID.Valid {
			t.RuleID = &ruleID.String
		}
		data.Targets = append(data.Targets, t)
	}
	if err := tRows.Err(); err != nil {
		return nil, err
	}

	// Checks
	cRows, err := s.db.Query(`SELECT id, target_id, type, name, config, interval_s, enabled, created_at, updated_at FROM checks ORDER BY created_at ASC`)
	if err != nil {
		return nil, fmt.Errorf("exporting checks: %w", err)
	}
	defer cRows.Close()
	for cRows.Next() {
		var c BackupCheck
		var enabled int
		if err := cRows.Scan(&c.ID, &c.TargetID, &c.Type, &c.Name, &c.Config, &c.IntervalS, &enabled, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		c.Enabled = enabled == 1
		data.Checks = append(data.Checks, c)
	}
	if err := cRows.Err(); err != nil {
		return nil, err
	}

	// Rule conditions
	rcRows, err := s.db.Query(`SELECT id, rule_id, check_id, field, comparator, value, fail_count, fail_window, sort_order FROM rule_conditions ORDER BY rule_id, sort_order ASC`)
	if err != nil {
		return nil, fmt.Errorf("exporting rule_conditions: %w", err)
	}
	defer rcRows.Close()
	for rcRows.Next() {
		var rc RuleCondition
		if err := rcRows.Scan(&rc.ID, &rc.RuleID, &rc.CheckID, &rc.Field, &rc.Comparator, &rc.Value, &rc.FailCount, &rc.FailWindow, &rc.SortOrder); err != nil {
			return nil, err
		}
		data.RuleConditions = append(data.RuleConditions, rc)
	}
	if err := rcRows.Err(); err != nil {
		return nil, err
	}

	// Rule states
	rsRows, err := s.db.Query(`SELECT rule_id, current_state, last_change, last_evaluated FROM rule_states ORDER BY rule_id ASC`)
	if err != nil {
		return nil, fmt.Errorf("exporting rule_states: %w", err)
	}
	defer rsRows.Close()
	for rsRows.Next() {
		var rs RuleState
		if err := rsRows.Scan(&rs.RuleID, &rs.CurrentState, &rs.LastChange, &rs.LastEvaluated); err != nil {
			return nil, err
		}
		data.RuleStates = append(data.RuleStates, rs)
	}
	if err := rsRows.Err(); err != nil {
		return nil, err
	}

	// Ensure nil slices become empty arrays in JSON
	if data.Users == nil {
		data.Users = []BackupUser{}
	}
	if data.Rules == nil {
		data.Rules = []BackupRule{}
	}
	if data.Targets == nil {
		data.Targets = []BackupTarget{}
	}
	if data.Checks == nil {
		data.Checks = []BackupCheck{}
	}
	if data.RuleConditions == nil {
		data.RuleConditions = []RuleCondition{}
	}
	if data.RuleStates == nil {
		data.RuleStates = []RuleState{}
	}

	return data, nil
}

// RestoreBackup wipes all config tables and inserts data from backup in a single transaction.
func (s *Store) RestoreBackup(data *BackupData) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Disable FK checks during wipe+insert (SQLite PRAGMA only works outside tx,
	// but delete order handles FK deps correctly)

	// Delete order: leaf to root
	deleteTables := []string{
		"rule_conditions",
		"rule_states",
		"alert_history",
		"rule_alerts",
		"alert_channels",
		"check_results",
		"checks",
		"targets",
		"rules",
		"sessions",
		"users",
		"settings",
	}
	for _, table := range deleteTables {
		if _, err := tx.Exec(fmt.Sprintf("DELETE FROM %s", table)); err != nil {
			return fmt.Errorf("clearing %s: %w", table, err)
		}
	}

	// Insert order: root to leaf

	// Users
	if len(data.Users) > 0 {
		stmt, err := tx.Prepare(`INSERT INTO users (id, username, email, password_hash, role, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`)
		if err != nil {
			return fmt.Errorf("prepare users insert: %w", err)
		}
		defer stmt.Close()
		for _, u := range data.Users {
			if _, err := stmt.Exec(u.ID, u.Username, u.Email, u.PasswordHash, u.Role, u.Status, u.CreatedAt, u.UpdatedAt); err != nil {
				return fmt.Errorf("inserting user %s: %w", u.Username, err)
			}
		}
	}

	// Settings
	if len(data.Settings) > 0 {
		stmt, err := tx.Prepare(`INSERT INTO settings (key, value) VALUES (?, ?)`)
		if err != nil {
			return fmt.Errorf("prepare settings insert: %w", err)
		}
		defer stmt.Close()
		for k, v := range data.Settings {
			if _, err := stmt.Exec(k, v); err != nil {
				return fmt.Errorf("inserting setting %s: %w", k, err)
			}
		}
	}

	// Rules
	if len(data.Rules) > 0 {
		stmt, err := tx.Prepare(`INSERT INTO rules (id, name, description, operator, severity, enabled, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`)
		if err != nil {
			return fmt.Errorf("prepare rules insert: %w", err)
		}
		defer stmt.Close()
		for _, r := range data.Rules {
			enabled := 0
			if r.Enabled {
				enabled = 1
			}
			if _, err := stmt.Exec(r.ID, r.Name, r.Description, r.Operator, r.Severity, enabled, r.CreatedAt, r.UpdatedAt); err != nil {
				return fmt.Errorf("inserting rule %s: %w", r.Name, err)
			}
		}
	}

	// Targets
	if len(data.Targets) > 0 {
		stmt, err := tx.Prepare(`INSERT INTO targets (id, name, host, description, enabled, preferred_check_type, operator, category, rule_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
		if err != nil {
			return fmt.Errorf("prepare targets insert: %w", err)
		}
		defer stmt.Close()
		for _, t := range data.Targets {
			enabled := 0
			if t.Enabled {
				enabled = 1
			}
			var ruleID any
			if t.RuleID != nil {
				ruleID = *t.RuleID
			}
			if _, err := stmt.Exec(t.ID, t.Name, t.Host, t.Description, enabled, t.PreferredCheckType, t.Operator, t.Category, ruleID, t.CreatedAt, t.UpdatedAt); err != nil {
				return fmt.Errorf("inserting target %s: %w", t.Name, err)
			}
		}
	}

	// Checks
	if len(data.Checks) > 0 {
		stmt, err := tx.Prepare(`INSERT INTO checks (id, target_id, type, name, config, interval_s, enabled, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`)
		if err != nil {
			return fmt.Errorf("prepare checks insert: %w", err)
		}
		defer stmt.Close()
		for _, c := range data.Checks {
			enabled := 0
			if c.Enabled {
				enabled = 1
			}
			if _, err := stmt.Exec(c.ID, c.TargetID, c.Type, c.Name, c.Config, c.IntervalS, enabled, c.CreatedAt, c.UpdatedAt); err != nil {
				return fmt.Errorf("inserting check %s: %w", c.Name, err)
			}
		}
	}

	// Rule conditions
	if len(data.RuleConditions) > 0 {
		stmt, err := tx.Prepare(`INSERT INTO rule_conditions (id, rule_id, check_id, field, comparator, value, fail_count, fail_window, sort_order) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`)
		if err != nil {
			return fmt.Errorf("prepare rule_conditions insert: %w", err)
		}
		defer stmt.Close()
		for _, rc := range data.RuleConditions {
			if _, err := stmt.Exec(rc.ID, rc.RuleID, rc.CheckID, rc.Field, rc.Comparator, rc.Value, rc.FailCount, rc.FailWindow, rc.SortOrder); err != nil {
				return fmt.Errorf("inserting rule_condition %s: %w", rc.ID, err)
			}
		}
	}

	// Rule states
	if len(data.RuleStates) > 0 {
		stmt, err := tx.Prepare(`INSERT INTO rule_states (rule_id, current_state, last_change, last_evaluated) VALUES (?, ?, ?, ?)`)
		if err != nil {
			return fmt.Errorf("prepare rule_states insert: %w", err)
		}
		defer stmt.Close()
		for _, rs := range data.RuleStates {
			if _, err := stmt.Exec(rs.RuleID, rs.CurrentState, rs.LastChange, rs.LastEvaluated); err != nil {
				return fmt.Errorf("inserting rule_state %s: %w", rs.RuleID, err)
			}
		}
	}

	return tx.Commit()
}
