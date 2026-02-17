package store

import "time"

type AuditEntry struct {
	ID           int       `json:"id"`
	UserID       string    `json:"user_id"`
	Username     string    `json:"username"`
	Action       string    `json:"action"`
	ResourceType string    `json:"resource_type"`
	ResourceID   string    `json:"resource_id"`
	Detail       string    `json:"detail"`
	IPAddress    string    `json:"ip_address"`
	Status       string    `json:"status"`
	CreatedAt    time.Time `json:"created_at"`
}

func (s *Store) CreateAuditEntry(e *AuditEntry) error {
	_, err := s.db.Exec(`
		INSERT INTO audit_logs (user_id, username, action, resource_type, resource_id, detail, ip_address, status)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		e.UserID, e.Username, e.Action, e.ResourceType, e.ResourceID, e.Detail, e.IPAddress, e.Status,
	)
	return err
}

func (s *Store) PurgeOldAuditEntries(days int) (int64, error) {
	cutoff := time.Now().AddDate(0, 0, -days)
	res, err := s.db.Exec(`DELETE FROM audit_logs WHERE created_at < ?`, cutoff)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s *Store) ListAuditEntries(limit, offset int) ([]AuditEntry, int, error) {
	var total int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM audit_logs`).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := s.db.Query(`
		SELECT id, user_id, username, action, resource_type, resource_id, detail, ip_address, status, created_at
		FROM audit_logs ORDER BY created_at DESC LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var entries []AuditEntry
	for rows.Next() {
		var e AuditEntry
		if err := rows.Scan(&e.ID, &e.UserID, &e.Username, &e.Action, &e.ResourceType, &e.ResourceID, &e.Detail, &e.IPAddress, &e.Status, &e.CreatedAt); err != nil {
			return nil, 0, err
		}
		entries = append(entries, e)
	}
	if entries == nil {
		entries = []AuditEntry{}
	}
	return entries, total, rows.Err()
}
