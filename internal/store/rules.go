package store

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type Rule struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Operator    string    `json:"operator"`
	Severity    string    `json:"severity"`
	Enabled     bool      `json:"enabled"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type RuleCondition struct {
	ID         string `json:"id"`
	RuleID     string `json:"rule_id"`
	CheckID    string `json:"check_id"`
	Field      string `json:"field"`
	Comparator string `json:"comparator"`
	Value      string `json:"value"`
	FailCount  int    `json:"fail_count"`
	FailWindow int    `json:"fail_window"`
	SortOrder  int    `json:"sort_order"`
}

type RuleState struct {
	RuleID        string     `json:"rule_id"`
	CurrentState  string     `json:"current_state"`
	LastChange    *time.Time `json:"last_change"`
	LastEvaluated *time.Time `json:"last_evaluated"`
}

type RuleWithConditions struct {
	Rule
	Conditions []RuleCondition `json:"conditions"`
	State      *RuleState      `json:"state"`
}

// --- CRUD ---

func (s *Store) CreateRule(r *Rule) error {
	r.ID = uuid.New().String()
	now := time.Now()
	r.CreatedAt = now
	r.UpdatedAt = now
	enabled := 0
	if r.Enabled {
		enabled = 1
	}
	if r.Operator == "" {
		r.Operator = "AND"
	}
	if r.Severity == "" {
		r.Severity = "critical"
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec(`
		INSERT INTO rules (id, name, description, operator, severity, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, r.ID, r.Name, r.Description, r.Operator, r.Severity, enabled, r.CreatedAt, r.UpdatedAt)
	if err != nil {
		return err
	}

	// Init rule_states row
	_, err = tx.Exec(`INSERT INTO rule_states (rule_id, current_state) VALUES (?, 'healthy')`, r.ID)
	if err != nil {
		return err
	}

	return tx.Commit()
}

func (s *Store) GetRule(id string) (*Rule, error) {
	r := &Rule{}
	var enabled int
	err := s.db.QueryRow(`
		SELECT id, name, description, operator, severity, enabled, created_at, updated_at
		FROM rules WHERE id = ?
	`, id).Scan(&r.ID, &r.Name, &r.Description, &r.Operator, &r.Severity, &enabled, &r.CreatedAt, &r.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	r.Enabled = enabled == 1
	return r, err
}

func (s *Store) ListRules() ([]Rule, error) {
	rows, err := s.db.Query(`
		SELECT id, name, description, operator, severity, enabled, created_at, updated_at
		FROM rules ORDER BY name ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []Rule
	for rows.Next() {
		var r Rule
		var enabled int
		if err := rows.Scan(&r.ID, &r.Name, &r.Description, &r.Operator, &r.Severity, &enabled, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		r.Enabled = enabled == 1
		rules = append(rules, r)
	}
	if rules == nil {
		rules = []Rule{}
	}
	return rules, rows.Err()
}

func (s *Store) UpdateRule(id, name, description, operator, severity string, enabled bool) error {
	en := 0
	if enabled {
		en = 1
	}
	res, err := s.db.Exec(`
		UPDATE rules SET name = ?, description = ?, operator = ?, severity = ?, enabled = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, name, description, operator, severity, en, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("rule not found")
	}
	return nil
}

func (s *Store) DeleteRule(id string) error {
	res, err := s.db.Exec(`DELETE FROM rules WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("rule not found")
	}
	return nil
}

// --- Conditions ---

func (s *Store) CreateRuleCondition(c *RuleCondition) error {
	c.ID = uuid.New().String()
	if c.Field == "" {
		c.Field = "status"
	}
	if c.Comparator == "" {
		c.Comparator = "eq"
	}
	if c.FailCount <= 0 {
		c.FailCount = 1
	}
	_, err := s.db.Exec(`
		INSERT INTO rule_conditions (id, rule_id, check_id, field, comparator, value, fail_count, fail_window, sort_order)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, c.ID, c.RuleID, c.CheckID, c.Field, c.Comparator, c.Value, c.FailCount, c.FailWindow, c.SortOrder)
	return err
}

func (s *Store) ListRuleConditions(ruleID string) ([]RuleCondition, error) {
	rows, err := s.db.Query(`
		SELECT id, rule_id, check_id, field, comparator, value, fail_count, fail_window, sort_order
		FROM rule_conditions WHERE rule_id = ? ORDER BY sort_order ASC
	`, ruleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var conds []RuleCondition
	for rows.Next() {
		var c RuleCondition
		if err := rows.Scan(&c.ID, &c.RuleID, &c.CheckID, &c.Field, &c.Comparator, &c.Value, &c.FailCount, &c.FailWindow, &c.SortOrder); err != nil {
			return nil, err
		}
		conds = append(conds, c)
	}
	if conds == nil {
		conds = []RuleCondition{}
	}
	return conds, rows.Err()
}

func (s *Store) DeleteRuleConditions(ruleID string) error {
	_, err := s.db.Exec(`DELETE FROM rule_conditions WHERE rule_id = ?`, ruleID)
	return err
}

// --- Composite queries ---

func (s *Store) GetRuleWithConditions(id string) (*RuleWithConditions, error) {
	r, err := s.GetRule(id)
	if err != nil || r == nil {
		return nil, err
	}
	conds, err := s.ListRuleConditions(id)
	if err != nil {
		return nil, err
	}
	state, err := s.GetRuleState(id)
	if err != nil {
		return nil, err
	}
	return &RuleWithConditions{Rule: *r, Conditions: conds, State: state}, nil
}

func (s *Store) ListRulesWithState() ([]RuleWithConditions, error) {
	rules, err := s.ListRules()
	if err != nil {
		return nil, err
	}

	var result []RuleWithConditions
	for _, r := range rules {
		conds, err := s.ListRuleConditions(r.ID)
		if err != nil {
			return nil, err
		}
		state, err := s.GetRuleState(r.ID)
		if err != nil {
			return nil, err
		}
		result = append(result, RuleWithConditions{Rule: r, Conditions: conds, State: state})
	}
	if result == nil {
		result = []RuleWithConditions{}
	}
	return result, nil
}

// --- Engine queries ---

func (s *Store) GetRulesByCheckID(checkID string) ([]Rule, error) {
	rows, err := s.db.Query(`
		SELECT DISTINCT r.id, r.name, r.description, r.operator, r.severity, r.enabled, r.created_at, r.updated_at
		FROM rules r
		JOIN rule_conditions rc ON rc.rule_id = r.id
		WHERE rc.check_id = ?
	`, checkID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []Rule
	for rows.Next() {
		var r Rule
		var enabled int
		if err := rows.Scan(&r.ID, &r.Name, &r.Description, &r.Operator, &r.Severity, &enabled, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		r.Enabled = enabled == 1
		rules = append(rules, r)
	}
	if rules == nil {
		rules = []Rule{}
	}
	return rules, rows.Err()
}

func (s *Store) GetRuleState(ruleID string) (*RuleState, error) {
	rs := &RuleState{}
	err := s.db.QueryRow(`
		SELECT rule_id, current_state, last_change, last_evaluated
		FROM rule_states WHERE rule_id = ?
	`, ruleID).Scan(&rs.RuleID, &rs.CurrentState, &rs.LastChange, &rs.LastEvaluated)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return rs, err
}

func (s *Store) UpdateRuleState(ruleID, newState string) error {
	_, err := s.db.Exec(`
		UPDATE rule_states SET current_state = ?, last_change = CURRENT_TIMESTAMP, last_evaluated = CURRENT_TIMESTAMP
		WHERE rule_id = ?
	`, newState, ruleID)
	return err
}

func (s *Store) TouchRuleEvaluated(ruleID string) error {
	_, err := s.db.Exec(`
		UPDATE rule_states SET last_evaluated = CURRENT_TIMESTAMP
		WHERE rule_id = ?
	`, ruleID)
	return err
}

func (s *Store) GetRuleStateSummary() (healthy, unhealthy int, err error) {
	err = s.db.QueryRow(`
		SELECT
			COALESCE(SUM(CASE WHEN rs.current_state = 'healthy' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN rs.current_state = 'unhealthy' THEN 1 ELSE 0 END), 0)
		FROM rule_states rs
		JOIN rules r ON r.id = rs.rule_id
		WHERE r.enabled = 1
	`).Scan(&healthy, &unhealthy)
	return
}
