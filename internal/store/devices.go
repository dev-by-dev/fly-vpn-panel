package store

import (
	"fmt"
	"time"
)

// DeviceMeta — заголовки клиента с запроса подписки. Пустые поля
// не затирают уже сохранённые значения.
type DeviceMeta struct {
	HWID      string
	Platform  string
	OSVersion string
	Model     string
	UserAgent string
}

// DeviceUpsert records a (username, ip) sighting. New pairs get
// first_seen = last_seen = now; existing pairs only get last_seen updated,
// so first_seen is immutable.
func (s *Store) DeviceUpsert(username, ip string, meta DeviceMeta, now time.Time) error {
	ts := fmtTime(now)
	_, err := s.DB().Exec(`
		INSERT INTO devices (username, ip, first_seen, last_seen, hwid, platform, os_version, model, user_agent)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(username, ip) DO UPDATE SET
			last_seen = excluded.last_seen,
			hwid = CASE WHEN excluded.hwid <> '' THEN excluded.hwid ELSE devices.hwid END,
			platform = CASE WHEN excluded.platform <> '' THEN excluded.platform ELSE devices.platform END,
			os_version = CASE WHEN excluded.os_version <> '' THEN excluded.os_version ELSE devices.os_version END,
			model = CASE WHEN excluded.model <> '' THEN excluded.model ELSE devices.model END,
			user_agent = CASE WHEN excluded.user_agent <> '' THEN excluded.user_agent ELSE devices.user_agent END`,
		username, ip, ts, ts, meta.HWID, meta.Platform, meta.OSVersion, meta.Model, meta.UserAgent)
	if err != nil {
		return fmt.Errorf("device upsert: %w", err)
	}
	return nil
}

func scanDevice(sc scanner) (*Device, error) {
	var (
		d         Device
		firstSeen string
		lastSeen  string
	)
	if err := sc.Scan(&d.Username, &d.IP, &firstSeen, &lastSeen, &d.HWID, &d.Platform, &d.OSVersion, &d.Model, &d.UserAgent); err != nil {
		return nil, err
	}
	var err error
	if d.FirstSeen, err = parseTime(firstSeen); err != nil {
		return nil, fmt.Errorf("device %q/%q first_seen: %w", d.Username, d.IP, err)
	}
	if d.LastSeen, err = parseTime(lastSeen); err != nil {
		return nil, fmt.Errorf("device %q/%q last_seen: %w", d.Username, d.IP, err)
	}
	return &d, nil
}

func (s *Store) devicesQuery(query string, args ...any) ([]Device, error) {
	rows, err := s.DB().Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("devices query: %w", err)
	}
	defer rows.Close()
	var devices []Device
	for rows.Next() {
		d, err := scanDevice(rows)
		if err != nil {
			return nil, fmt.Errorf("devices query: %w", err)
		}
		devices = append(devices, *d)
	}
	return devices, rows.Err()
}

// DevicesOnline returns devices with last_seen >= since, ordered by
// username then ip.
func (s *Store) DevicesOnline(since time.Time) ([]Device, error) {
	return s.devicesQuery(
		`SELECT username, ip, first_seen, last_seen, hwid, platform, os_version, model, user_agent FROM devices
		WHERE last_seen >= ? ORDER BY username, ip`, fmtTime(since))
}

// DevicesForUser returns the user's devices ordered by ip.
func (s *Store) DevicesForUser(username string) ([]Device, error) {
	return s.devicesQuery(
		`SELECT username, ip, first_seen, last_seen, hwid, platform, os_version, model, user_agent FROM devices
		WHERE username = ? ORDER BY ip`, username)
}

// PruneDevices deletes devices with last_seen strictly before the given time.
func (s *Store) PruneDevices(before time.Time) error {
	_, err := s.DB().Exec(`DELETE FROM devices WHERE last_seen < ?`, fmtTime(before))
	if err != nil {
		return fmt.Errorf("prune devices: %w", err)
	}
	return nil
}
