package store

import (
	"database/sql"
	"time"
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

// --- Engine queries ---

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

