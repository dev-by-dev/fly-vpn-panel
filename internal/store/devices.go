package store

import (
	"database/sql"
	"errors"
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
	// Ключ устройства — HWID клиента; без него падаем на IP.
	var id int64
	var err error
	if meta.HWID != "" {
		err = s.DB().QueryRow(`SELECT id FROM devices WHERE username = ? AND hwid = ?`, username, meta.HWID).Scan(&id)
	} else {
		err = s.DB().QueryRow(`SELECT id FROM devices WHERE username = ? AND ip = ? AND hwid = ''`, username, ip).Scan(&id)
	}
	switch {
	case err == nil:
		_, err = s.DB().Exec(`
			UPDATE devices SET ip = ?, last_seen = ?,
				platform = CASE WHEN ? <> '' THEN ? ELSE platform END,
				os_version = CASE WHEN ? <> '' THEN ? ELSE os_version END,
				model = CASE WHEN ? <> '' THEN ? ELSE model END,
				user_agent = CASE WHEN ? <> '' THEN ? ELSE user_agent END
			WHERE id = ?`,
			ip, ts,
			meta.Platform, meta.Platform,
			meta.OSVersion, meta.OSVersion,
			meta.Model, meta.Model,
			meta.UserAgent, meta.UserAgent, id)
		if err != nil {
			return fmt.Errorf("device upsert: %w", err)
		}
		return nil
	case !errors.Is(err, sql.ErrNoRows):
		return fmt.Errorf("device upsert: %w", err)
	}
	_, err = s.DB().Exec(`
		INSERT INTO devices (username, ip, first_seen, last_seen, hwid, platform, os_version, model, user_agent)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
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
	if err := sc.Scan(&d.Username, &d.IP, &firstSeen, &lastSeen, &d.Country, &d.ASN, &d.Provider, &d.HWID, &d.Platform, &d.OSVersion, &d.Model, &d.UserAgent); err != nil {
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
		`SELECT username, ip, first_seen, last_seen, country, asn, provider, hwid, platform, os_version, model, user_agent FROM devices
		WHERE last_seen >= ? ORDER BY username, ip`, fmtTime(since))
}

// DevicesForUser returns the user's devices ordered by ip.
func (s *Store) DevicesForUser(username string) ([]Device, error) {
	return s.devicesQuery(
		`SELECT username, ip, first_seen, last_seen, country, asn, provider, hwid, platform, os_version, model, user_agent FROM devices
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

// DeviceGeoSet stores the resolved country and provider for one device.
func (s *Store) DeviceGeoSet(username, ip, country, asn, provider string) error {
	_, err := s.DB().Exec(
		`UPDATE devices SET country = ?, asn = ?, provider = ? WHERE username = ? AND ip = ?`,
		country, asn, provider, username, ip)
	if err != nil {
		return fmt.Errorf("device geo set: %w", err)
	}
	return nil
}
