package store

import (
	"database/sql"
	"time"
)

type Session struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	ExpiresAt time.Time `json:"expires_at"`
	IPAddress string    `json:"ip_address"`
	CreatedAt time.Time `json:"created_at"`
}

func (s *Store) CreateSession(sess *Session) error {
	_, err := s.db.Exec(`
		INSERT INTO sessions (id, user_id, expires_at, ip_address, created_at)
		VALUES (?, ?, ?, ?, ?)
	`, sess.ID, sess.UserID, sess.ExpiresAt, sess.IPAddress, sess.CreatedAt)
	return err
}

func (s *Store) GetSession(id string) (*Session, error) {
	sess := &Session{}
	err := s.db.QueryRow(`
		SELECT id, user_id, expires_at, ip_address, created_at
		FROM sessions WHERE id = ?
	`, id).Scan(&sess.ID, &sess.UserID, &sess.ExpiresAt, &sess.IPAddress, &sess.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return sess, err
}

func (s *Store) DeleteSession(id string) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE id = ?`, id)
	return err
}

func (s *Store) DeleteUserSessions(userID string) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE user_id = ?`, userID)
	return err
}

func (s *Store) DeleteUserSessionsExcept(userID, keepSessionID string) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE user_id = ? AND id != ?`, userID, keepSessionID)
	return err
}

func (s *Store) PurgeExpiredSessions() (int64, error) {
	res, err := s.db.Exec(`DELETE FROM sessions WHERE expires_at < ?`, time.Now())
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
