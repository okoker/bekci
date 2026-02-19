package store

import (
	"database/sql"
	"time"
)

// SetTargetRecipients replaces all recipients for a target with the given user IDs.
func (s *Store) SetTargetRecipients(targetID string, userIDs []string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM target_alert_recipients WHERE target_id = ?`, targetID); err != nil {
		return err
	}
	for _, uid := range userIDs {
		if _, err := tx.Exec(`INSERT INTO target_alert_recipients (target_id, user_id) VALUES (?, ?)`, targetID, uid); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ListTargetRecipients returns all users who are alert recipients for a target.
func (s *Store) ListTargetRecipients(targetID string) ([]User, error) {
	rows, err := s.db.Query(`
		SELECT u.id, u.username, u.email, u.phone, u.role, u.status, u.created_at, u.updated_at
		FROM users u
		JOIN target_alert_recipients tar ON tar.user_id = u.id
		WHERE tar.target_id = ?
		ORDER BY u.username ASC
	`, targetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Username, &u.Email, &u.Phone, &u.Role, &u.Status, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	if users == nil {
		users = []User{}
	}
	return users, rows.Err()
}

// ListTargetRecipientIDs returns just the user IDs who are recipients for a target.
func (s *Store) ListTargetRecipientIDs(targetID string) ([]string, error) {
	rows, err := s.db.Query(`SELECT user_id FROM target_alert_recipients WHERE target_id = ?`, targetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if ids == nil {
		ids = []string{}
	}
	return ids, rows.Err()
}

// AlertEntry represents a sent alert record.
type AlertEntry struct {
	ID          int64     `json:"id"`
	RuleID      string    `json:"rule_id"`
	TargetID    string    `json:"target_id"`
	RecipientID string    `json:"recipient_id"`
	AlertType   string    `json:"alert_type"` // "firing" or "recovery"
	Message     string    `json:"message"`
	SentAt      time.Time `json:"sent_at"`
}

// LogAlert records a sent alert in alert_history.
func (s *Store) LogAlert(targetID, ruleID, recipientID, alertType, message string) error {
	_, err := s.db.Exec(`
		INSERT INTO alert_history (rule_id, target_id, recipient_id, alert_type, message, sent_at)
		VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
	`, ruleID, targetID, recipientID, alertType, message)
	return err
}

// GetLastAlertTime returns the most recent alert sent_at for a given rule.
func (s *Store) GetLastAlertTime(ruleID string) (time.Time, error) {
	var t time.Time
	err := s.db.QueryRow(`
		SELECT sent_at FROM alert_history WHERE rule_id = ? ORDER BY sent_at DESC LIMIT 1
	`, ruleID).Scan(&t)
	if err == sql.ErrNoRows {
		return time.Time{}, nil // no previous alert
	}
	if err != nil {
		return time.Time{}, err
	}
	return t, nil
}

// AlertHistoryItem is an enriched alert entry for the history list view.
type AlertHistoryItem struct {
	ID            int64     `json:"id"`
	RuleID        string    `json:"rule_id"`
	TargetID      string    `json:"target_id"`
	TargetName    string    `json:"target_name"`
	RecipientID   string    `json:"recipient_id"`
	RecipientName string    `json:"recipient_name"`
	AlertType     string    `json:"alert_type"`
	Message       string    `json:"message"`
	SentAt        time.Time `json:"sent_at"`
}

// ListAlertHistory returns paginated alert history with target and recipient names.
func (s *Store) ListAlertHistory(limit, offset int) ([]AlertHistoryItem, int, error) {
	var total int
	s.db.QueryRow(`SELECT COUNT(*) FROM alert_history WHERE target_id != ''`).Scan(&total)

	rows, err := s.db.Query(`
		SELECT ah.id, ah.rule_id, ah.target_id,
		       COALESCE(t.name, '(deleted)') as target_name,
		       ah.recipient_id,
		       COALESCE(u.username, '(deleted)') as recipient_name,
		       ah.alert_type, COALESCE(ah.message, ''), ah.sent_at
		FROM alert_history ah
		LEFT JOIN targets t ON t.id = ah.target_id
		LEFT JOIN users u ON u.id = ah.recipient_id
		WHERE ah.target_id != ''
		ORDER BY ah.sent_at DESC
		LIMIT ? OFFSET ?
	`, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var items []AlertHistoryItem
	for rows.Next() {
		var item AlertHistoryItem
		if err := rows.Scan(&item.ID, &item.RuleID, &item.TargetID, &item.TargetName,
			&item.RecipientID, &item.RecipientName,
			&item.AlertType, &item.Message, &item.SentAt); err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	if items == nil {
		items = []AlertHistoryItem{}
	}
	return items, total, rows.Err()
}

// GetFiringRules returns rule IDs that are currently in "unhealthy" state, for re-alerting.
func (s *Store) GetFiringRules() ([]struct {
	RuleID   string
	TargetID string
}, error) {
	rows, err := s.db.Query(`
		SELECT rs.rule_id, t.id
		FROM rule_states rs
		JOIN targets t ON t.rule_id = rs.rule_id
		WHERE rs.current_state = 'unhealthy'
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []struct {
		RuleID   string
		TargetID string
	}
	for rows.Next() {
		var item struct {
			RuleID   string
			TargetID string
		}
		if err := rows.Scan(&item.RuleID, &item.TargetID); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

// PurgeOldAlertHistory deletes alert_history entries older than the given number of days.
func (s *Store) PurgeOldAlertHistory(days int) (int64, error) {
	cutoff := time.Now().AddDate(0, 0, -days)
	res, err := s.db.Exec(`DELETE FROM alert_history WHERE sent_at < ?`, cutoff)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// GetTargetIDByRuleID returns the target ID associated with a rule.
func (s *Store) GetTargetIDByRuleID(ruleID string) (string, error) {
	var targetID string
	err := s.db.QueryRow(`SELECT id FROM targets WHERE rule_id = ?`, ruleID).Scan(&targetID)
	return targetID, err
}
