package store

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type Target struct {
	ID          string    `json:"id"`
	ProjectID   string    `json:"project_id"`
	Name        string    `json:"name"`
	Host        string    `json:"host"`
	Description string    `json:"description"`
	Enabled     bool      `json:"enabled"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type TargetWithChecks struct {
	Target
	Checks []Check `json:"checks"`
}

func (s *Store) CreateTarget(t *Target) error {
	t.ID = uuid.New().String()
	now := time.Now()
	t.CreatedAt = now
	t.UpdatedAt = now
	enabled := 0
	if t.Enabled {
		enabled = 1
	}
	_, err := s.db.Exec(`
		INSERT INTO targets (id, project_id, name, host, description, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, t.ID, t.ProjectID, t.Name, t.Host, t.Description, enabled, t.CreatedAt, t.UpdatedAt)
	return err
}

func (s *Store) GetTarget(id string) (*Target, error) {
	t := &Target{}
	var enabled int
	err := s.db.QueryRow(`
		SELECT id, project_id, name, host, description, enabled, created_at, updated_at
		FROM targets WHERE id = ?
	`, id).Scan(&t.ID, &t.ProjectID, &t.Name, &t.Host, &t.Description, &enabled, &t.CreatedAt, &t.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	t.Enabled = enabled == 1
	return t, err
}

func (s *Store) ListTargets(projectID string) ([]Target, error) {
	var rows *sql.Rows
	var err error
	if projectID != "" {
		rows, err = s.db.Query(`
			SELECT id, project_id, name, host, description, enabled, created_at, updated_at
			FROM targets WHERE project_id = ? ORDER BY name ASC
		`, projectID)
	} else {
		rows, err = s.db.Query(`
			SELECT id, project_id, name, host, description, enabled, created_at, updated_at
			FROM targets ORDER BY name ASC
		`)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var targets []Target
	for rows.Next() {
		var t Target
		var enabled int
		if err := rows.Scan(&t.ID, &t.ProjectID, &t.Name, &t.Host, &t.Description, &enabled, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		t.Enabled = enabled == 1
		targets = append(targets, t)
	}
	if targets == nil {
		targets = []Target{}
	}
	return targets, rows.Err()
}

func (s *Store) UpdateTarget(id, name, host, description string, enabled bool) error {
	en := 0
	if enabled {
		en = 1
	}
	res, err := s.db.Exec(`
		UPDATE targets SET name = ?, host = ?, description = ?, enabled = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, name, host, description, en, id)
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
