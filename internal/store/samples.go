package store

import (
	"fmt"
	"time"
)

// SampleInsert records one traffic delta sample for username at ts.
func (s *Store) SampleInsert(username string, ts time.Time, up, down int64) error {
	_, err := s.DB().Exec(
		`INSERT INTO traffic_samples (username, ts, up_delta, down_delta) VALUES (?, ?, ?, ?)`,
		username, fmtTime(ts), up, down)
	if err != nil {
		return fmt.Errorf("sample insert: %w", err)
	}
	return nil
}

// SamplesSince returns the summed up and down deltas for username with
// ts >= since. Missing rows return 0, 0, nil.
func (s *Store) SamplesSince(username string, since time.Time) (up, down int64, err error) {
	err = s.DB().QueryRow(
		`SELECT COALESCE(SUM(up_delta), 0), COALESCE(SUM(down_delta), 0)
		FROM traffic_samples WHERE username = ? AND ts >= ?`,
		username, fmtTime(since)).Scan(&up, &down)
	if err != nil {
		return 0, 0, fmt.Errorf("samples since: %w", err)
	}
	return up, down, nil
}

// TrafficSeries returns summed traffic per hour or day bucket for username
// over [from, to], ordered by bucket start. RFC3339 UTC strings sort
// lexicographically, so string comparison of ts is correct.
func (s *Store) TrafficSeries(username string, from, to time.Time, bucket string) ([]Bucket, error) {
	var tsFmt string
	switch bucket {
	case "hour":
		tsFmt = "%Y-%m-%dT%H:00:00Z"
	case "day":
		tsFmt = "%Y-%m-%dT00:00:00Z"
	default:
		return nil, fmt.Errorf("traffic series: invalid bucket %q (want hour or day)", bucket)
	}
	rows, err := s.DB().Query(`
		SELECT strftime(?, ts) AS bucket, SUM(up_delta), SUM(down_delta)
		FROM traffic_samples
		WHERE username = ? AND ts >= ? AND ts <= ?
		GROUP BY bucket
		ORDER BY bucket`,
		tsFmt, username, fmtTime(from), fmtTime(to))
	if err != nil {
		return nil, fmt.Errorf("traffic series: %w", err)
	}
	defer rows.Close()
	var buckets []Bucket
	for rows.Next() {
		var b Bucket
		var ts string
		if err := rows.Scan(&ts, &b.Up, &b.Down); err != nil {
			return nil, fmt.Errorf("traffic series: %w", err)
		}
		t, err := parseTime(ts)
		if err != nil {
			return nil, fmt.Errorf("traffic series: bucket ts: %w", err)
		}
		b.TS = t
		buckets = append(buckets, b)
	}
	return buckets, rows.Err()
}

// SamplesSeries returns per-tick samples summed across all users with
// ts >= from, ordered by ts. One tick writes one row per active user,
// so grouping by exact ts collapses a tick into a single bucket.
func (s *Store) SamplesSeries(from time.Time) ([]Bucket, error) {
	rows, err := s.DB().Query(`
		SELECT ts, SUM(up_delta), SUM(down_delta)
		FROM traffic_samples
		WHERE ts >= ?
		GROUP BY ts
		ORDER BY ts`,
		fmtTime(from))
	if err != nil {
		return nil, fmt.Errorf("samples series: %w", err)
	}
	defer rows.Close()
	var buckets []Bucket
	for rows.Next() {
		var b Bucket
		var ts string
		if err := rows.Scan(&ts, &b.Up, &b.Down); err != nil {
			return nil, fmt.Errorf("samples series: %w", err)
		}
		t, err := parseTime(ts)
		if err != nil {
			return nil, fmt.Errorf("samples series: ts: %w", err)
		}
		b.TS = t
		buckets = append(buckets, b)
	}
	return buckets, rows.Err()
}

// PruneSamples deletes samples with ts strictly before the given time.
func (s *Store) PruneSamples(before time.Time) error {
	_, err := s.DB().Exec(`DELETE FROM traffic_samples WHERE ts < ?`, fmtTime(before))
	if err != nil {
		return fmt.Errorf("prune samples: %w", err)
	}
	return nil
}
