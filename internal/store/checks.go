package store

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type Check struct {
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

// EnabledCheck is a flattened view for the scheduler: check + target host.
type EnabledCheck struct {
	Check
	Host string `json:"host"`
}

func (s *Store) CreateCheck(c *Check) error {
	c.ID = uuid.New().String()
	now := time.Now()
	c.CreatedAt = now
	c.UpdatedAt = now
	enabled := 0
	if c.Enabled {
		enabled = 1
	}
	_, err := s.db.Exec(`
		INSERT INTO checks (id, target_id, type, name, config, interval_s, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, c.ID, c.TargetID, c.Type, c.Name, c.Config, c.IntervalS, enabled, c.CreatedAt, c.UpdatedAt)
	return err
}

func (s *Store) GetCheck(id string) (*Check, error) {
	c := &Check{}
	var enabled int
	err := s.db.QueryRow(`
		SELECT id, target_id, type, name, config, interval_s, enabled, created_at, updated_at
		FROM checks WHERE id = ?
	`, id).Scan(&c.ID, &c.TargetID, &c.Type, &c.Name, &c.Config, &c.IntervalS, &enabled, &c.CreatedAt, &c.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	c.Enabled = enabled == 1
	return c, err
}

func (s *Store) ListChecksByTarget(targetID string) ([]Check, error) {
	rows, err := s.db.Query(`
		SELECT id, target_id, type, name, config, interval_s, enabled, created_at, updated_at
		FROM checks WHERE target_id = ? ORDER BY name ASC
	`, targetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var checks []Check
	for rows.Next() {
		var c Check
		var enabled int
		if err := rows.Scan(&c.ID, &c.TargetID, &c.Type, &c.Name, &c.Config, &c.IntervalS, &enabled, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		c.Enabled = enabled == 1
		checks = append(checks, c)
	}
	if checks == nil {
		checks = []Check{}
	}
	return checks, rows.Err()
}

func (s *Store) UpdateCheck(id, name, config string, intervalS int, enabled bool) error {
	en := 0
	if enabled {
		en = 1
	}
	res, err := s.db.Exec(`
		UPDATE checks SET name = ?, config = ?, interval_s = ?, enabled = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, name, config, intervalS, en, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("check not found")
	}
	return nil
}

func (s *Store) DeleteCheck(id string) error {
	res, err := s.db.Exec(`DELETE FROM checks WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("check not found")
	}
	return nil
}

// ListAllEnabledChecks returns all enabled checks with target host, for the scheduler.
func (s *Store) ListAllEnabledChecks() ([]EnabledCheck, error) {
	rows, err := s.db.Query(`
		SELECT c.id, c.target_id, c.type, c.name, c.config, c.interval_s, c.enabled,
		       c.created_at, c.updated_at, t.host
		FROM checks c
		JOIN targets t ON c.target_id = t.id
		WHERE c.enabled = 1 AND t.enabled = 1
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var checks []EnabledCheck
	for rows.Next() {
		var ec EnabledCheck
		var enabled int
		if err := rows.Scan(&ec.ID, &ec.TargetID, &ec.Type, &ec.Name, &ec.Config, &ec.IntervalS, &enabled,
			&ec.CreatedAt, &ec.UpdatedAt, &ec.Host); err != nil {
			return nil, err
		}
		ec.Enabled = enabled == 1
		checks = append(checks, ec)
	}
	if checks == nil {
		checks = []EnabledCheck{}
	}
	return checks, rows.Err()
}
