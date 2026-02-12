package store

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type Target struct {
	ID                 string    `json:"id"`
	Name               string    `json:"name"`
	Host               string    `json:"host"`
	Description        string    `json:"description"`
	Enabled            bool      `json:"enabled"`
	PreferredCheckType string    `json:"preferred_check_type"`
	Operator           string    `json:"operator"`
	Severity           string    `json:"severity"`
	RuleID             *string   `json:"rule_id"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type TargetWithChecks struct {
	Target
	Checks []Check `json:"checks"`
}

// TargetCondition is the unified check+condition for the target API.
type TargetCondition struct {
	CheckID    string `json:"check_id,omitempty"`
	CheckType  string `json:"check_type"`
	CheckName  string `json:"check_name"`
	Config     string `json:"config"`
	IntervalS  int    `json:"interval_s"`
	Field      string `json:"field"`
	Comparator string `json:"comparator"`
	Value      string `json:"value"`
	FailCount  int    `json:"fail_count"`
	FailWindow int    `json:"fail_window"`
}

// TargetDetail is a target with its conditions and rule state.
type TargetDetail struct {
	Target
	Conditions []TargetCondition `json:"conditions"`
	State      *RuleState        `json:"state"`
}

func scanTarget(row interface{ Scan(...any) error }) (*Target, error) {
	t := &Target{}
	var enabled int
	var ruleID sql.NullString
	err := row.Scan(&t.ID, &t.Name, &t.Host, &t.Description, &enabled,
		&t.PreferredCheckType, &t.Operator, &t.Severity, &ruleID,
		&t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return nil, err
	}
	t.Enabled = enabled == 1
	if ruleID.Valid {
		t.RuleID = &ruleID.String
	}
	return t, nil
}

const targetCols = `id, name, host, description, enabled, preferred_check_type, operator, severity, rule_id, created_at, updated_at`

func (s *Store) CreateTarget(t *Target) error {
	t.ID = uuid.New().String()
	now := time.Now()
	t.CreatedAt = now
	t.UpdatedAt = now
	enabled := 0
	if t.Enabled {
		enabled = 1
	}
	if t.PreferredCheckType == "" {
		t.PreferredCheckType = "ping"
	}
	if t.Operator == "" {
		t.Operator = "AND"
	}
	if t.Severity == "" {
		t.Severity = "critical"
	}
	_, err := s.db.Exec(`
		INSERT INTO targets (id, name, host, description, enabled, preferred_check_type, operator, severity, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, t.ID, t.Name, t.Host, t.Description, enabled, t.PreferredCheckType, t.Operator, t.Severity, t.CreatedAt, t.UpdatedAt)
	return err
}

func (s *Store) GetTarget(id string) (*Target, error) {
	row := s.db.QueryRow(`SELECT `+targetCols+` FROM targets WHERE id = ?`, id)
	t, err := scanTarget(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return t, err
}

func (s *Store) ListTargets() ([]Target, error) {
	rows, err := s.db.Query(`SELECT ` + targetCols + ` FROM targets ORDER BY name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var targets []Target
	for rows.Next() {
		t, err := scanTarget(rows)
		if err != nil {
			return nil, err
		}
		targets = append(targets, *t)
	}
	if targets == nil {
		targets = []Target{}
	}
	return targets, rows.Err()
}

func (s *Store) UpdateTarget(id, name, host, description string, enabled bool, preferredCheckType, operator, severity string) error {
	en := 0
	if enabled {
		en = 1
	}
	if preferredCheckType == "" {
		preferredCheckType = "ping"
	}
	if operator == "" {
		operator = "AND"
	}
	if severity == "" {
		severity = "critical"
	}
	res, err := s.db.Exec(`
		UPDATE targets SET name = ?, host = ?, description = ?, enabled = ?,
			preferred_check_type = ?, operator = ?, severity = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, name, host, description, en, preferredCheckType, operator, severity, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("target not found")
	}
	return nil
}

func (s *Store) DeleteTarget(id string) error {
	// Delete linked rule first to prevent orphans
	var ruleID sql.NullString
	s.db.QueryRow(`SELECT rule_id FROM targets WHERE id = ?`, id).Scan(&ruleID)
	if ruleID.Valid {
		s.db.Exec(`DELETE FROM rules WHERE id = ?`, ruleID.String)
	}

	res, err := s.db.Exec(`DELETE FROM targets WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("target not found")
	}
	return nil
}

func (s *Store) GetTargetWithChecks(id string) (*TargetWithChecks, error) {
	t, err := s.GetTarget(id)
	if err != nil || t == nil {
		return nil, err
	}
	checks, err := s.ListChecksByTarget(id)
	if err != nil {
		return nil, err
	}
	return &TargetWithChecks{Target: *t, Checks: checks}, nil
}

// --- Unified target + conditions methods ---

// CreateTargetWithConditions creates a target, its checks, and auto-manages the hidden rule.
func (s *Store) CreateTargetWithConditions(t *Target, conds []TargetCondition) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Insert target
	t.ID = uuid.New().String()
	now := time.Now()
	t.CreatedAt = now
	t.UpdatedAt = now
	enabled := 0
	if t.Enabled {
		enabled = 1
	}
	if t.PreferredCheckType == "" {
		t.PreferredCheckType = "ping"
	}
	if t.Operator == "" {
		t.Operator = "AND"
	}
	if t.Severity == "" {
		t.Severity = "critical"
	}

	_, err = tx.Exec(`
		INSERT INTO targets (id, name, host, description, enabled, preferred_check_type, operator, severity, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, t.ID, t.Name, t.Host, t.Description, enabled, t.PreferredCheckType, t.Operator, t.Severity, t.CreatedAt, t.UpdatedAt)
	if err != nil {
		return err
	}

	if len(conds) == 0 {
		return tx.Commit()
	}

	// Create hidden rule
	ruleID := uuid.New().String()
	_, err = tx.Exec(`
		INSERT INTO rules (id, name, description, operator, severity, enabled, created_at, updated_at)
		VALUES (?, ?, '', ?, ?, 1, ?, ?)
	`, ruleID, t.Name+" rule", t.Operator, t.Severity, now, now)
	if err != nil {
		return err
	}
	_, err = tx.Exec(`INSERT INTO rule_states (rule_id, current_state) VALUES (?, 'healthy')`, ruleID)
	if err != nil {
		return err
	}

	// Set preferred_check_type from first condition
	preferredType := conds[0].CheckType

	// Create checks + conditions
	for i, c := range conds {
		checkID := uuid.New().String()
		conds[i].CheckID = checkID
		checkEnabled := 1
		_, err = tx.Exec(`
			INSERT INTO checks (id, target_id, type, name, config, interval_s, enabled, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, checkID, t.ID, c.CheckType, c.CheckName, c.Config, c.IntervalS, checkEnabled, now, now)
		if err != nil {
			return err
		}

		condID := uuid.New().String()
		field := c.Field
		if field == "" {
			field = "status"
		}
		comparator := c.Comparator
		if comparator == "" {
			comparator = "eq"
		}
		failCount := c.FailCount
		if failCount <= 0 {
			failCount = 1
		}
		_, err = tx.Exec(`
			INSERT INTO rule_conditions (id, rule_id, check_id, field, comparator, value, fail_count, fail_window, sort_order)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, condID, ruleID, checkID, field, comparator, c.Value, failCount, c.FailWindow, i)
		if err != nil {
			return err
		}
	}

	// Link rule to target
	_, err = tx.Exec(`UPDATE targets SET rule_id = ?, preferred_check_type = ? WHERE id = ?`, ruleID, preferredType, t.ID)
	if err != nil {
		return err
	}
	t.RuleID = &ruleID
	t.PreferredCheckType = preferredType

	return tx.Commit()
}

// UpdateTargetWithConditions updates a target and smart-diffs its checks/conditions.
func (s *Store) UpdateTargetWithConditions(id, name, host, description string, enabled bool, operator, severity string, conds []TargetCondition) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	en := 0
	if enabled {
		en = 1
	}
	if operator == "" {
		operator = "AND"
	}
	if severity == "" {
		severity = "critical"
	}

	// Get current target for rule_id
	var ruleID sql.NullString
	var currentPreferred string
	err = tx.QueryRow(`SELECT rule_id, preferred_check_type FROM targets WHERE id = ?`, id).Scan(&ruleID, &currentPreferred)
	if err != nil {
		return fmt.Errorf("target not found")
	}

	now := time.Now()
	preferredType := currentPreferred
	if len(conds) > 0 {
		preferredType = conds[0].CheckType
	}

	// Update target fields
	_, err = tx.Exec(`
		UPDATE targets SET name = ?, host = ?, description = ?, enabled = ?,
			preferred_check_type = ?, operator = ?, severity = ?, updated_at = ?
		WHERE id = ?
	`, name, host, description, en, preferredType, operator, severity, now, id)
	if err != nil {
		return err
	}

	// Handle conditions
	if len(conds) == 0 {
		// No conditions — delete rule if exists
		if ruleID.Valid {
			tx.Exec(`DELETE FROM rules WHERE id = ?`, ruleID.String)
			tx.Exec(`UPDATE targets SET rule_id = NULL WHERE id = ?`, id)
		}
		// Delete all checks for this target
		tx.Exec(`DELETE FROM checks WHERE target_id = ?`, id)
		return tx.Commit()
	}

	// Ensure rule exists
	rid := ""
	if ruleID.Valid {
		rid = ruleID.String
		// Update rule fields
		_, err = tx.Exec(`UPDATE rules SET name = ?, operator = ?, severity = ?, enabled = 1, updated_at = ? WHERE id = ?`,
			name+" rule", operator, severity, now, rid)
		if err != nil {
			return err
		}
	} else {
		// Create new rule
		rid = uuid.New().String()
		_, err = tx.Exec(`
			INSERT INTO rules (id, name, description, operator, severity, enabled, created_at, updated_at)
			VALUES (?, ?, '', ?, ?, 1, ?, ?)
		`, rid, name+" rule", operator, severity, now, now)
		if err != nil {
			return err
		}
		_, err = tx.Exec(`INSERT INTO rule_states (rule_id, current_state) VALUES (?, 'healthy')`, rid)
		if err != nil {
			return err
		}
		_, err = tx.Exec(`UPDATE targets SET rule_id = ? WHERE id = ?`, rid, id)
		if err != nil {
			return err
		}
	}

	// Smart-diff checks: build set of incoming check_ids
	incomingIDs := map[string]bool{}
	for _, c := range conds {
		if c.CheckID != "" {
			incomingIDs[c.CheckID] = true
		}
	}

	// Delete checks not in incoming set
	existingChecks, err := listCheckIDsByTargetTx(tx, id)
	if err != nil {
		return err
	}
	for _, eid := range existingChecks {
		if !incomingIDs[eid] {
			tx.Exec(`DELETE FROM checks WHERE id = ?`, eid)
		}
	}

	// Delete all rule conditions — will recreate
	tx.Exec(`DELETE FROM rule_conditions WHERE rule_id = ?`, rid)

	// Upsert checks + create conditions
	for i, c := range conds {
		checkID := c.CheckID
		if checkID != "" {
			// Update existing check
			_, err = tx.Exec(`
				UPDATE checks SET name = ?, config = ?, interval_s = ?, enabled = 1, updated_at = ? WHERE id = ?
			`, c.CheckName, c.Config, c.IntervalS, now, checkID)
			if err != nil {
				return err
			}
		} else {
			// Create new check
			checkID = uuid.New().String()
			_, err = tx.Exec(`
				INSERT INTO checks (id, target_id, type, name, config, interval_s, enabled, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?, ?, 1, ?, ?)
			`, checkID, id, c.CheckType, c.CheckName, c.Config, c.IntervalS, now, now)
			if err != nil {
				return err
			}
		}

		// Create condition
		condID := uuid.New().String()
		field := c.Field
		if field == "" {
			field = "status"
		}
		comparator := c.Comparator
		if comparator == "" {
			comparator = "eq"
		}
		failCount := c.FailCount
		if failCount <= 0 {
			failCount = 1
		}
		_, err = tx.Exec(`
			INSERT INTO rule_conditions (id, rule_id, check_id, field, comparator, value, fail_count, fail_window, sort_order)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, condID, rid, checkID, field, comparator, c.Value, failCount, c.FailWindow, i)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func listCheckIDsByTargetTx(tx *sql.Tx, targetID string) ([]string, error) {
	rows, err := tx.Query(`SELECT id FROM checks WHERE target_id = ?`, targetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		rows.Scan(&id)
		ids = append(ids, id)
	}
	return ids, nil
}

// GetTargetDetail returns a target with its conditions (check+rule_condition joined) and state.
func (s *Store) GetTargetDetail(id string) (*TargetDetail, error) {
	t, err := s.GetTarget(id)
	if err != nil || t == nil {
		return nil, err
	}

	td := &TargetDetail{Target: *t}

	// Load conditions: join checks with rule_conditions
	if t.RuleID != nil {
		rows, err := s.db.Query(`
			SELECT c.id, c.type, c.name, c.config, c.interval_s,
			       rc.field, rc.comparator, rc.value, rc.fail_count, rc.fail_window
			FROM checks c
			JOIN rule_conditions rc ON rc.check_id = c.id AND rc.rule_id = ?
			WHERE c.target_id = ?
			ORDER BY rc.sort_order ASC
		`, *t.RuleID, id)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var tc TargetCondition
				rows.Scan(&tc.CheckID, &tc.CheckType, &tc.CheckName, &tc.Config, &tc.IntervalS,
					&tc.Field, &tc.Comparator, &tc.Value, &tc.FailCount, &tc.FailWindow)
				td.Conditions = append(td.Conditions, tc)
			}
		}

		// Load state
		td.State, _ = s.GetRuleState(*t.RuleID)
	}

	// Also load checks without conditions (orphaned checks from before)
	if td.Conditions == nil {
		// Fall back to plain checks
		checks, _ := s.ListChecksByTarget(id)
		for _, c := range checks {
			td.Conditions = append(td.Conditions, TargetCondition{
				CheckID:    c.ID,
				CheckType:  c.Type,
				CheckName:  c.Name,
				Config:     c.Config,
				IntervalS:  c.IntervalS,
				Field:      "status",
				Comparator: "eq",
				Value:      "down",
				FailCount:  1,
			})
		}
	}

	if td.Conditions == nil {
		td.Conditions = []TargetCondition{}
	}
	return td, nil
}

// TargetListItem is a target summary for the list view (avoids loading full conditions).
type TargetListItem struct {
	Target
	ConditionCount int        `json:"condition_count"`
	State          *RuleState `json:"state"`
}

// ListTargetSummaries returns all targets with condition count and state, for list view.
func (s *Store) ListTargetSummaries() ([]TargetListItem, error) {
	targets, err := s.ListTargets()
	if err != nil {
		return nil, err
	}

	var result []TargetListItem
	for _, t := range targets {
		item := TargetListItem{Target: t}

		// Count checks for this target
		s.db.QueryRow(`SELECT COUNT(*) FROM checks WHERE target_id = ?`, t.ID).Scan(&item.ConditionCount)

		// Load state if rule exists
		if t.RuleID != nil {
			item.State, _ = s.GetRuleState(*t.RuleID)
		}

		result = append(result, item)
	}
	if result == nil {
		result = []TargetListItem{}
	}
	return result, nil
}
