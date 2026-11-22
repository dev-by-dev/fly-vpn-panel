package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// APIToken is a long-lived API credential. Only its hash is persisted.
type APIToken struct {
	ID        int64
	Name      string
	Scope     string
	ExpiresAt time.Time // zero = never expires
	Revoked   bool
	CreatedAt time.Time
}

// APITokenInsert records a token hash and returns the new row id.
func (s *Store) APITokenInsert(name, scope, hash string, expiresAt time.Time) (int64, error) {
	exp := ""
	if !expiresAt.IsZero() {
		exp = fmtTime(expiresAt)
	}
	res, err := s.DB().Exec(
		`INSERT INTO api_tokens (name, scope, token_hash, expires_at, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		name, scope, hash, exp, fmtTime(time.Now()))
	if err != nil {
		return 0, fmt.Errorf("api token insert: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("api token insert: last id: %w", err)
	}
	return id, nil
}

// APITokenList returns every token, newest first.
func (s *Store) APITokenList() ([]APIToken, error) {
	rows, err := s.DB().Query(
		`SELECT id, name, scope, expires_at, revoked, created_at
		 FROM api_tokens ORDER BY id DESC`)
	if err != nil {
		return nil, fmt.Errorf("api token list: %w", err)
	}
	defer rows.Close()

	var out []APIToken
	for rows.Next() {
		t, err := scanAPIToken(rows)
		if err != nil {
			return nil, fmt.Errorf("api token list: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// APITokenByHash looks a token up by its hash. Unknown hashes are (zero, false, nil).
func (s *Store) APITokenByHash(hash string) (APIToken, bool, error) {
	row := s.DB().QueryRow(
		`SELECT id, name, scope, expires_at, revoked, created_at
		 FROM api_tokens WHERE token_hash = ?`, hash)
	t, err := scanAPIToken(row)
	if errors.Is(err, sql.ErrNoRows) {
		return APIToken{}, false, nil
	}
	if err != nil {
		return APIToken{}, false, fmt.Errorf("api token by hash: %w", err)
	}
	return t, true, nil
}

// APITokenRevoke revokes one token, or ErrNotFound if the id is unknown.
func (s *Store) APITokenRevoke(id int64) error {
	res, err := s.DB().Exec(`UPDATE api_tokens SET revoked = 1 WHERE id = ? AND revoked = 0`, id)
	if err != nil {
		return fmt.Errorf("api token revoke: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("api token revoke: rows affected: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// APITokenRevokeAll revokes every token.
func (s *Store) APITokenRevokeAll() error {
	if _, err := s.DB().Exec(`UPDATE api_tokens SET revoked = 1`); err != nil {
		return fmt.Errorf("api token revoke all: %w", err)
	}
	return nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanAPIToken(r rowScanner) (APIToken, error) {
	var (
		t       APIToken
		expires string
		revoked int
		created string
	)
	if err := r.Scan(&t.ID, &t.Name, &t.Scope, &expires, &revoked, &created); err != nil {
		return APIToken{}, err
	}
	if expires != "" {
		exp, err := parseTime(expires)
		if err != nil {
			return APIToken{}, fmt.Errorf("expires_at: %w", err)
		}
		t.ExpiresAt = exp
	}
	ct, err := parseTime(created)
	if err != nil {
		return APIToken{}, fmt.Errorf("created_at: %w", err)
	}
	t.CreatedAt = ct
	t.Revoked = revoked != 0
	return t, nil
}
