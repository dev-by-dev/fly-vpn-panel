package store

import (
	"database/sql"
	"errors"
	"fmt"
)

const userCols = `id, username, uuid, sub_token, expiry, quota_bytes, quota_period,
	quota_reset_at, device_limit, disabled, COALESCE(disabled_reason, ''), created_at`

func scanUser(sc scanner) (*User, error) {
	var (
		u            User
		disabled     int
		expiry       string
		quotaResetAt string
		createdAt    string
	)
	if err := sc.Scan(&u.ID, &u.Username, &u.UUID, &u.SubToken, &expiry, &u.QuotaBytes,
		&u.QuotaPeriod, &quotaResetAt, &u.DeviceLimit, &disabled, &u.DisabledReason, &createdAt); err != nil {
		return nil, err
	}
	u.Disabled = disabled != 0
	var err error
	if u.Expiry, err = parseTime(expiry); err != nil {
		return nil, fmt.Errorf("user %q expiry: %w", u.Username, err)
	}
	if u.QuotaResetAt, err = parseTime(quotaResetAt); err != nil {
		return nil, fmt.Errorf("user %q quota_reset_at: %w", u.Username, err)
	}
	if u.CreatedAt, err = parseTime(createdAt); err != nil {
		return nil, fmt.Errorf("user %q created_at: %w", u.Username, err)
	}
	return &u, nil
}

func (s *Store) userQuery(where string, args ...any) (*User, error) {
	row := s.DB().QueryRow(`SELECT `+userCols+` FROM users WHERE `+where, args...)
	u, err := scanUser(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("user query: %w", err)
	}
	return u, nil
}

// UserCreate inserts u and sets u.ID to the assigned row id.
func (s *Store) UserCreate(u *User) error {
	if u.QuotaPeriod == "" {
		u.QuotaPeriod = "none" // schema CHECK constraint
	}
	res, err := s.DB().Exec(`
		INSERT INTO users (username, uuid, sub_token, expiry, quota_bytes, quota_period,
			quota_reset_at, device_limit, disabled, disabled_reason, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		u.Username, u.UUID, u.SubToken, fmtTime(u.Expiry), u.QuotaBytes, u.QuotaPeriod,
		fmtTime(u.QuotaResetAt), u.DeviceLimit, btoi(u.Disabled), u.DisabledReason, fmtTime(u.CreatedAt))
	if err != nil {
		return fmt.Errorf("user create: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return fmt.Errorf("user create: last insert id: %w", err)
	}
	u.ID = id
	return nil
}

// UserGet returns the user with the given id, or ErrNotFound.
func (s *Store) UserGet(id int64) (*User, error) {
	return s.userQuery("id = ?", id)
}

// UserGetByToken returns the user with the given subscription token, or ErrNotFound.
func (s *Store) UserGetByToken(token string) (*User, error) {
	return s.userQuery("sub_token = ?", token)
}

// UserGetByUsername returns the user with the given username, or ErrNotFound.
func (s *Store) UserGetByUsername(name string) (*User, error) {
	return s.userQuery("username = ?", name)
}

// UserList returns all users ordered by username.
func (s *Store) UserList() ([]User, error) {
	rows, err := s.DB().Query(`SELECT ` + userCols + ` FROM users ORDER BY username`)
	if err != nil {
		return nil, fmt.Errorf("user list: %w", err)
	}
	defer rows.Close()
	var users []User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, fmt.Errorf("user list: %w", err)
		}
		users = append(users, *u)
	}
	return users, rows.Err()
}

// UserUpdate writes all mutable fields of u; created_at is not changed.
func (s *Store) UserUpdate(u *User) error {
	if u.QuotaPeriod == "" {
		u.QuotaPeriod = "none" // schema CHECK constraint
	}
	res, err := s.DB().Exec(`
		UPDATE users SET username = ?, uuid = ?, sub_token = ?, expiry = ?, quota_bytes = ?,
			quota_period = ?, quota_reset_at = ?, device_limit = ?, disabled = ?, disabled_reason = ?
		WHERE id = ?`,
		u.Username, u.UUID, u.SubToken, fmtTime(u.Expiry), u.QuotaBytes, u.QuotaPeriod,
		fmtTime(u.QuotaResetAt), u.DeviceLimit, btoi(u.Disabled), u.DisabledReason, u.ID)
	if err != nil {
		return fmt.Errorf("user update: %w", err)
	}
	if n, err := res.RowsAffected(); err != nil {
		return fmt.Errorf("user update: rows affected: %w", err)
	} else if n == 0 {
		return ErrNotFound
	}
	return nil
}

// UserDelete removes the user with the given id, or ErrNotFound.
func (s *Store) UserDelete(id int64) error {
	res, err := s.DB().Exec(`DELETE FROM users WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("user delete: %w", err)
	}
	if n, err := res.RowsAffected(); err != nil {
		return fmt.Errorf("user delete: rows affected: %w", err)
	} else if n == 0 {
		return ErrNotFound
	}
	return nil
}

// UserSetDisabled toggles the disabled flag. When disabling, reason is stored
// as given; when enabling, any reason is cleared.
func (s *Store) UserSetDisabled(id int64, disabled bool, reason string) error {
	if !disabled {
		reason = ""
	}
	res, err := s.DB().Exec(
		`UPDATE users SET disabled = ?, disabled_reason = ? WHERE id = ?`,
		btoi(disabled), reason, id)
	if err != nil {
		return fmt.Errorf("user set disabled: %w", err)
	}
	if n, err := res.RowsAffected(); err != nil {
		return fmt.Errorf("user set disabled: rows affected: %w", err)
	} else if n == 0 {
		return ErrNotFound
	}
	return nil
}
