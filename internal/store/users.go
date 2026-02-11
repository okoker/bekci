package store

import (
	"database/sql"
	"fmt"
	"time"
)

type User struct {
	ID           string    `json:"id"`
	Username     string    `json:"username"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	Role         string    `json:"role"`
	Status       string    `json:"status"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (s *Store) CreateUser(u *User) error {
	_, err := s.db.Exec(`
		INSERT INTO users (id, username, email, password_hash, role, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, u.ID, u.Username, u.Email, u.PasswordHash, u.Role, u.Status, u.CreatedAt, u.UpdatedAt)
	return err
}

func (s *Store) GetUserByID(id string) (*User, error) {
	u := &User{}
	err := s.db.QueryRow(`
		SELECT id, username, email, password_hash, role, status, created_at, updated_at
		FROM users WHERE id = ?
	`, id).Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.Role, &u.Status, &u.CreatedAt, &u.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return u, err
}

func (s *Store) GetUserByUsername(username string) (*User, error) {
	u := &User{}
	err := s.db.QueryRow(`
		SELECT id, username, email, password_hash, role, status, created_at, updated_at
		FROM users WHERE username = ?
	`, username).Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.Role, &u.Status, &u.CreatedAt, &u.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return u, err
}

func (s *Store) ListUsers() ([]User, error) {
	rows, err := s.db.Query(`
		SELECT id, username, email, role, status, created_at, updated_at
		FROM users ORDER BY created_at ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Username, &u.Email, &u.Role, &u.Status, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

// UpdateUser updates email and/or role. Does not touch password or status.
func (s *Store) UpdateUser(id, email, role string) error {
	res, err := s.db.Exec(`
		UPDATE users SET email = ?, role = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, email, role, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("user not found")
	}
	return nil
}

func (s *Store) UpdateUserPassword(id, passwordHash string) error {
	res, err := s.db.Exec(`
		UPDATE users SET password_hash = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, passwordHash, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("user not found")
	}
	return nil
}

func (s *Store) SuspendUser(id string, suspended bool) error {
	status := "active"
	if suspended {
		status = "suspended"
	}
	res, err := s.db.Exec(`
		UPDATE users SET status = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, status, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("user not found")
	}
	return nil
}

// CountActiveAdmins returns the number of active admin users.
func (s *Store) CountActiveAdmins() (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM users WHERE role = 'admin' AND status = 'active'`).Scan(&count)
	return count, err
}

func (s *Store) CountUsers() (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&count)
	return count, err
}
