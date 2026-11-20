package store

import (
	"database/sql"
	"errors"
	"fmt"
)

// SettingGet returns the value for key, or ErrNotFound when unset.
func (s *Store) SettingGet(key string) (string, error) {
	var value string
	err := s.DB().QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("setting get %q: %w", key, err)
	}
	return value, nil
}

// SettingSet upserts the value for key.
func (s *Store) SettingSet(key, value string) error {
	_, err := s.DB().Exec(`
		INSERT INTO settings (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	if err != nil {
		return fmt.Errorf("setting set %q: %w", key, err)
	}
	return nil
}

// SettingsAll returns all settings as a map.
func (s *Store) SettingsAll() (map[string]string, error) {
	rows, err := s.DB().Query(`SELECT key, value FROM settings ORDER BY key`)
	if err != nil {
		return nil, fmt.Errorf("settings all: %w", err)
	}
	defer rows.Close()
	m := make(map[string]string)
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, fmt.Errorf("settings all: %w", err)
		}
		m[k] = v
	}
	return m, rows.Err()
}
