package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// RefreshTokenInsert records a refresh token hash with its expiry.
func (s *Store) RefreshTokenInsert(hash string, exp time.Time) error {
	_, err := s.DB().Exec(
		`INSERT INTO refresh_tokens (token_hash, expires_at) VALUES (?, ?)`,
		hash, fmtTime(exp))
	if err != nil {
		return fmt.Errorf("refresh token insert: %w", err)
	}
	return nil
}

// RefreshTokenValid reports whether the token exists, is not revoked and has
// not expired. Unknown tokens are simply invalid: (false, nil).
func (s *Store) RefreshTokenValid(hash string) (bool, error) {
	var (
		revoked   int
		expiresAt string
	)
	err := s.DB().QueryRow(
		`SELECT revoked, expires_at FROM refresh_tokens WHERE token_hash = ?`, hash,
	).Scan(&revoked, &expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("refresh token valid: %w", err)
	}
	if revoked != 0 {
		return false, nil
	}
	exp, err := parseTime(expiresAt)
	if err != nil {
		return false, fmt.Errorf("refresh token valid: expires_at: %w", err)
	}
	return time.Now().UTC().Before(exp), nil
}

// RefreshTokenRevoke revokes one token, or ErrNotFound if the hash is unknown.
func (s *Store) RefreshTokenRevoke(hash string) error {
	res, err := s.DB().Exec(
		`UPDATE refresh_tokens SET revoked = 1 WHERE token_hash = ?`, hash)
	if err != nil {
		return fmt.Errorf("refresh token revoke: %w", err)
	}
	if n, err := res.RowsAffected(); err != nil {
		return fmt.Errorf("refresh token revoke: rows affected: %w", err)
	} else if n == 0 {
		return ErrNotFound
	}
	return nil
}

// RefreshTokenRevokeAll revokes every refresh token.
func (s *Store) RefreshTokenRevokeAll() error {
	_, err := s.DB().Exec(`UPDATE refresh_tokens SET revoked = 1`)
	if err != nil {
		return fmt.Errorf("refresh token revoke all: %w", err)
	}
	return nil
}
