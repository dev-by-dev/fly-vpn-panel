package store

import (
	"errors"
	"time"
)

// ErrNotFound is returned when a requested row does not exist.
var ErrNotFound = errors.New("not found")

// Admin is the single panel administrator account.
type Admin struct {
	ID           int64
	Username     string
	PasswordHash string
	TOTPSecret   string
	TOTPEnabled  bool
	CreatedAt    time.Time
}

// User is a VPN panel user.
type User struct {
	ID             int64
	Username       string
	UUID           string
	SubToken       string
	Expiry         time.Time
	QuotaBytes     int64
	QuotaPeriod    string
	QuotaResetAt   time.Time
	DeviceLimit    int
	Disabled       bool
	DisabledReason string
	CreatedAt      time.Time
}

// Inbound is a panel-managed inbound entry.
type Inbound struct {
	ID         int64
	Tag        string
	Enabled    bool
	ConfigJSON string
	CreatedAt  time.Time
}

// Device is a (username, IP) pair seen connecting.
type Device struct {
	Username  string
	IP        string
	FirstSeen time.Time
	LastSeen  time.Time
	// Заголовки клиента с запроса подписки; xray их не отдаёт.
	HWID      string
	Platform  string
	OSVersion string
	Model     string
	UserAgent string
}

// Bucket is an aggregated traffic sample for one time bucket.
type Bucket struct {
	TS   time.Time
	Up   int64
	Down int64
}

// scanner is satisfied by *sql.Row and *sql.Rows.
type scanner interface{ Scan(dest ...any) error }

// fmtTime renders t as the canonical RFC3339 UTC storage form.
func fmtTime(t time.Time) string { return t.UTC().Format(time.RFC3339) }

// parseTime parses an RFC3339 timestamp and normalizes it to UTC.
func parseTime(s string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, err
	}
	return t.UTC(), nil
}

// btoi converts a bool to the INTEGER storage form.
func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}
