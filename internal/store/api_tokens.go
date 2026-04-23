package store

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// APIToken represents a bearer token used by machine consumers to reach
// /api/v1/* endpoints. The plaintext value is only ever returned by
// CreateAPIToken — after that only id/name/prefix/timestamps surface.
type APIToken struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Prefix     string     `json:"prefix"`
	CreatedBy  string     `json:"created_by"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
}

// tokenPrefix is the human-visible marker on every Bekci API token.
const tokenPrefix = "bk_"

// hashToken returns the hex sha256 of the token plaintext — the form
// stored in api_tokens.token_hash.
func hashToken(plaintext string) string {
	sum := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(sum[:])
}

// generateToken produces a fresh plaintext token of the form
// "bk_<64 hex chars>" (256 bits of entropy from crypto/rand).
func generateToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate token entropy: %w", err)
	}
	return tokenPrefix + hex.EncodeToString(buf), nil
}

// CreateAPIToken inserts a new token for `name` owned by user `createdBy`
// and returns the row plus the plaintext — the plaintext is only ever
// returned here and cannot be retrieved later.
func (s *Store) CreateAPIToken(name, createdBy string) (*APIToken, string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, "", fmt.Errorf("token name required")
	}
	plaintext, err := generateToken()
	if err != nil {
		return nil, "", err
	}
	tok := &APIToken{
		ID:        uuid.New().String(),
		Name:      name,
		Prefix:    plaintext[:min(len(plaintext), 11)], // "bk_" + 8 hex chars
		CreatedBy: createdBy,
		CreatedAt: time.Now().UTC(),
	}
	_, err = s.db.Exec(
		`INSERT INTO api_tokens (id, name, token_hash, prefix, created_by, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		tok.ID, tok.Name, hashToken(plaintext), tok.Prefix, tok.CreatedBy, tok.CreatedAt,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return nil, "", fmt.Errorf("a token named %q already exists", name)
		}
		return nil, "", fmt.Errorf("insert api_token: %w", err)
	}
	return tok, plaintext, nil
}

// ListAPITokens returns every token (active + revoked) sorted newest
// first. Token hash and plaintext never leave the store.
func (s *Store) ListAPITokens() ([]APIToken, error) {
	rows, err := s.db.Query(`
		SELECT id, name, prefix, created_by, created_at, last_used_at, revoked_at
		FROM api_tokens
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []APIToken{}
	for rows.Next() {
		var t APIToken
		var lastUsed, revoked *time.Time
		if err := rows.Scan(&t.ID, &t.Name, &t.Prefix, &t.CreatedBy, &t.CreatedAt, &lastUsed, &revoked); err != nil {
			return nil, err
		}
		t.LastUsedAt = lastUsed
		t.RevokedAt = revoked
		out = append(out, t)
	}
	return out, rows.Err()
}

// RevokeAPIToken marks a token as revoked (soft delete). Idempotent —
// re-revoking is a no-op.
func (s *Store) RevokeAPIToken(id string) error {
	res, err := s.db.Exec(
		`UPDATE api_tokens SET revoked_at = ? WHERE id = ? AND revoked_at IS NULL`,
		time.Now().UTC(), id,
	)
	if err != nil {
		return fmt.Errorf("revoke api_token: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		// Distinguish "not found" from "already revoked".
		var count int
		_ = s.db.QueryRow(`SELECT COUNT(*) FROM api_tokens WHERE id = ?`, id).Scan(&count)
		if count == 0 {
			return fmt.Errorf("token not found")
		}
	}
	return nil
}

// AuthenticateAPIToken validates a plaintext token presented in a Bearer
// header. Returns the matching row if active, nil otherwise. Touches
// last_used_at on success.
func (s *Store) AuthenticateAPIToken(plaintext string) (*APIToken, error) {
	if !strings.HasPrefix(plaintext, tokenPrefix) {
		return nil, nil
	}
	h := hashToken(plaintext)
	var t APIToken
	var lastUsed, revoked *time.Time
	err := s.db.QueryRow(`
		SELECT id, name, prefix, created_by, created_at, last_used_at, revoked_at
		FROM api_tokens WHERE token_hash = ?
	`, h).Scan(&t.ID, &t.Name, &t.Prefix, &t.CreatedBy, &t.CreatedAt, &lastUsed, &revoked)
	if err != nil {
		return nil, nil // not found or scan error — treat as invalid
	}
	if revoked != nil {
		return nil, nil
	}
	t.LastUsedAt = lastUsed
	t.RevokedAt = revoked
	// Update last_used_at asynchronously-in-spirit; if it fails we still auth.
	_, _ = s.db.Exec(`UPDATE api_tokens SET last_used_at = ? WHERE id = ?`, time.Now().UTC(), t.ID)
	return &t, nil
}
