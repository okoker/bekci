package store

import (
	"fmt"
	"strings"
)

type TagOption struct {
	ID    int    `json:"id"`
	Group string `json:"group"`
	Value string `json:"value"`
}

// CategoryInUseError is returned when attempting to delete a category that has targets assigned.
type CategoryInUseError struct {
	Targets []string
}

func (e *CategoryInUseError) Error() string {
	return fmt.Sprintf("category has %d assigned targets", len(e.Targets))
}

// CategoryToSLAKey derives the settings key for a category name.
// "Physical Security" → "sla_physical_security"
func CategoryToSLAKey(name string) string {
	return "sla_" + strings.ToLower(strings.ReplaceAll(name, " ", "_"))
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

// CreateTagOption adds a new tag value to a group. Values for the free-form
// 'tag' group are uppercased before insert so "P1", "p1", and " P1 " all
// collapse to the same canonical entry.
func (s *Store) CreateTagOption(group, value string) (*TagOption, error) {
	if group == "tag" {
		value = strings.ToUpper(strings.TrimSpace(value))
	}
	res, err := s.db.Exec(`INSERT INTO tag_options (grp, value) VALUES (?, ?)`, group, value)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return &TagOption{ID: int(id), Group: group, Value: value}, nil
}

// DeleteTagOption removes a tag option. For project/location, clears from targets.
// For category, blocks deletion if targets are assigned (returns CategoryInUseError)
// and cleans up the associated SLA setting.
func (s *Store) DeleteTagOption(id int) error {
	var grp, value string
	err := s.db.QueryRow(`SELECT grp, value FROM tag_options WHERE id = ?`, id).Scan(&grp, &value)
	if err != nil {
		return fmt.Errorf("tag option not found")
	}

	if grp == "category" {
		if value == "Other" {
			return fmt.Errorf("cannot delete the 'Other' category")
		}
		rows, err := s.db.Query(`SELECT name FROM targets WHERE category = ?`, value)
		if err != nil {
			return fmt.Errorf("checking targets: %w", err)
		}
		defer rows.Close()
		var names []string
		for rows.Next() {
			var n string
			if err := rows.Scan(&n); err != nil {
				return err
			}
			names = append(names, n)
		}
		if len(names) > 0 {
			return &CategoryInUseError{Targets: names}
		}
		// Clean up SLA setting
		slaKey := CategoryToSLAKey(value)
		if _, err := s.db.Exec(`DELETE FROM settings WHERE key = ?`, slaKey); err != nil {
			return fmt.Errorf("cleanup sla setting: %w", err)
		}
	} else if grp == "project" || grp == "location" {
		if _, err := s.db.Exec(`UPDATE targets SET `+grp+` = NULL WHERE `+grp+` = ?`, value); err != nil {
			return err
		}
	} else if grp == "tag" {
		// target_tags rows are cleaned by ON DELETE CASCADE on the tag_options FK.
	} else {
		return fmt.Errorf("invalid tag group: %s", grp)
	}

	_, err = s.db.Exec(`DELETE FROM tag_options WHERE id = ?`, id)
	return err
}

// RenameTagOption renames a tag option. For categories, cascades the rename
// to targets.category and the associated SLA settings key.
func (s *Store) RenameTagOption(id int, newValue string) error {
	var grp, oldValue string
	err := s.db.QueryRow(`SELECT grp, value FROM tag_options WHERE id = ?`, id).Scan(&grp, &oldValue)
	if err != nil {
		return fmt.Errorf("tag option not found")
	}

	if grp == "category" && oldValue == "Other" {
		return fmt.Errorf("cannot rename the 'Other' category")
	}

	if grp == "tag" {
		newValue = strings.ToUpper(strings.TrimSpace(newValue))
	}

	if _, err := s.db.Exec(`UPDATE tag_options SET value = ? WHERE id = ?`, newValue, id); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return fmt.Errorf("a tag with this name already exists")
		}
		return err
	}

	if grp == "category" {
		if _, err := s.db.Exec(`UPDATE targets SET category = ? WHERE category = ?`, newValue, oldValue); err != nil {
			return fmt.Errorf("cascade rename targets: %w", err)
		}
		oldKey := CategoryToSLAKey(oldValue)
		newKey := CategoryToSLAKey(newValue)
		if _, err := s.db.Exec(`UPDATE settings SET key = ? WHERE key = ?`, newKey, oldKey); err != nil {
			return fmt.Errorf("rename sla setting: %w", err)
		}
	}

	return nil
}

// ValidateCategory checks if a category name exists in tag_options.
func (s *Store) ValidateCategory(name string) (bool, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM tag_options WHERE grp = 'category' AND value = ?`, name).Scan(&count)
	return count > 0, err
}
