package store

import "fmt"

type TagOption struct {
	ID    int    `json:"id"`
	Group string `json:"group"`
	Value string `json:"value"`
}

// ListTagOptions returns all tag options for a group ("project" or "location").
func (s *Store) ListTagOptions(group string) ([]TagOption, error) {
	rows, err := s.db.Query(`SELECT id, grp, value FROM tag_options WHERE grp = ? ORDER BY value ASC`, group)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tags []TagOption
	for rows.Next() {
		var t TagOption
		if err := rows.Scan(&t.ID, &t.Group, &t.Value); err != nil {
			return nil, err
		}
		tags = append(tags, t)
	}
	if tags == nil {
		tags = []TagOption{}
	}
	return tags, rows.Err()
}

// CreateTagOption adds a new tag value to a group.
func (s *Store) CreateTagOption(group, value string) (*TagOption, error) {
	res, err := s.db.Exec(`INSERT INTO tag_options (grp, value) VALUES (?, ?)`, group, value)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return &TagOption{ID: int(id), Group: group, Value: value}, nil
}

// DeleteTagOption removes a tag option and clears it from all targets that reference it.
func (s *Store) DeleteTagOption(id int) error {
	var grp, value string
	err := s.db.QueryRow(`SELECT grp, value FROM tag_options WHERE id = ?`, id).Scan(&grp, &value)
	if err != nil {
		return fmt.Errorf("tag option not found")
	}

	// Clear from targets — grp is "project" or "location" which matches column name
	col := grp
	if _, err := s.db.Exec(`UPDATE targets SET `+col+` = NULL WHERE `+col+` = ?`, value); err != nil {
		return err
	}

	_, err = s.db.Exec(`DELETE FROM tag_options WHERE id = ?`, id)
	return err
}
