package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// AdminGet returns the admin account, or ErrNotFound when none exists.
func (s *Store) AdminGet() (*Admin, error) {
	var (
		a         Admin
		enabled   int
		createdAt string
	)
	err := s.DB().QueryRow(`
		SELECT id, username, password_hash, COALESCE(totp_secret, ''), totp_enabled, created_at
		FROM admin`).Scan(&a.ID, &a.Username, &a.PasswordHash, &a.TOTPSecret, &enabled, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("admin get: %w", err)
	}
	a.TOTPEnabled = enabled != 0
	if a.CreatedAt, err = parseTime(createdAt); err != nil {
		return nil, fmt.Errorf("admin get: created_at: %w", err)
	}
	return &a, nil
}

// AdminCreate creates the single admin account; it errors if one already
// exists. The existence check and insert are one atomic statement.
func (s *Store) AdminCreate(username, hash string) error {
	res, err := s.DB().Exec(`
		INSERT INTO admin (id, username, password_hash, totp_secret, totp_enabled, created_at)
		SELECT 1, ?, ?, NULL, 0, ?
		WHERE NOT EXISTS (SELECT 1 FROM admin)`,
		username, hash, fmtTime(time.Now()))
	if err != nil {
		return fmt.Errorf("admin create: %w", err)
	}
	if n, err := res.RowsAffected(); err != nil {
		return fmt.Errorf("admin create: rows affected: %w", err)
	} else if n == 0 {
		return fmt.Errorf("admin create: admin already exists")
	}
	return nil
}

// AdminUpdatePassword replaces the admin password hash.
func (s *Store) AdminUpdatePassword(hash string) error {
	res, err := s.DB().Exec(`UPDATE admin SET password_hash = ?`, hash)
	if err != nil {
		return fmt.Errorf("admin update password: %w", err)
	}
	if n, err := res.RowsAffected(); err != nil {
		return fmt.Errorf("admin update password: rows affected: %w", err)
	} else if n == 0 {
		return ErrNotFound
	}
	return nil
}

// BackupCodesReplace atomically replaces the whole backup code set.
func (s *Store) BackupCodesReplace(hashes []string) error {
	tx, err := s.DB().Begin()
	if err != nil {
		return fmt.Errorf("backup codes replace: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM backup_codes`); err != nil {
		tx.Rollback()
		return fmt.Errorf("backup codes replace: %w", err)
	}
	for _, h := range hashes {
		if _, err := tx.Exec(
			`INSERT INTO backup_codes (code_hash, used) VALUES (?, 0)`, h); err != nil {
			tx.Rollback()
			return fmt.Errorf("backup codes replace: %w", err)
		}
	}
	return tx.Commit()
}

// BackupCodeConsume marks one backup code used atomically. It returns true
// when an unused code with that hash existed, false when already used or
// unknown.
func (s *Store) BackupCodeConsume(hash string) (bool, error) {
	res, err := s.DB().Exec(
		`UPDATE backup_codes SET used = 1 WHERE code_hash = ? AND used = 0`, hash)
	if err != nil {
		return false, fmt.Errorf("backup code consume: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("backup code consume: rows affected: %w", err)
	}
	return n == 1, nil
}

// AdminSetTOTP stores the TOTP secret and enabled flag.
func (s *Store) AdminSetTOTP(secret string, enabled bool) error {
	res, err := s.DB().Exec(
		`UPDATE admin SET totp_secret = ?, totp_enabled = ?`, secret, btoi(enabled))
	if err != nil {
		return fmt.Errorf("admin set totp: %w", err)
	}
	if n, err := res.RowsAffected(); err != nil {
		return fmt.Errorf("admin set totp: rows affected: %w", err)
	} else if n == 0 {
		return ErrNotFound
	}
	return nil
}
