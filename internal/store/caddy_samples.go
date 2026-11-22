package store

import (
	"database/sql"
	"fmt"
	"time"
)

// CaddySample is one scrape of Caddy's Prometheus metrics.
type CaddySample struct {
	TS              time.Time
	Requests        float64
	Errors          float64
	InFlight        float64
	DurationSum     float64
	DurationCount   float64
	RSS             float64
	UpstreamHealthy *bool // nil when the metric was absent
}

// CaddySampleInsert records one scrape.
func (s *Store) CaddySampleInsert(c CaddySample) error {
	var up any
	if c.UpstreamHealthy != nil {
		up = btoi(*c.UpstreamHealthy)
	}
	_, err := s.DB().Exec(
		`INSERT INTO caddy_samples
		 (ts, requests, errors, in_flight, duration_sum, duration_count, rss, upstream_healthy)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		fmtTime(c.TS), c.Requests, c.Errors, c.InFlight,
		c.DurationSum, c.DurationCount, c.RSS, up)
	if err != nil {
		return fmt.Errorf("caddy sample insert: %w", err)
	}
	return nil
}

// CaddySamplesSince returns samples with ts >= since, oldest first.
func (s *Store) CaddySamplesSince(since time.Time) ([]CaddySample, error) {
	rows, err := s.DB().Query(
		`SELECT ts, requests, errors, in_flight, duration_sum, duration_count, rss, upstream_healthy
		 FROM caddy_samples WHERE ts >= ? ORDER BY ts`,
		fmtTime(since))
	if err != nil {
		return nil, fmt.Errorf("caddy samples since: %w", err)
	}
	defer rows.Close()

	var out []CaddySample
	for rows.Next() {
		var c CaddySample
		var ts string
		var up sql.NullInt64
		if err := rows.Scan(&ts, &c.Requests, &c.Errors, &c.InFlight,
			&c.DurationSum, &c.DurationCount, &c.RSS, &up); err != nil {
			return nil, fmt.Errorf("caddy samples since: %w", err)
		}
		c.TS, _ = parseTime(ts)
		if up.Valid {
			v := up.Int64 != 0
			c.UpstreamHealthy = &v
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// CaddySamplesPrune deletes samples older than before.
func (s *Store) CaddySamplesPrune(before time.Time) error {
	if _, err := s.DB().Exec(`DELETE FROM caddy_samples WHERE ts < ?`, fmtTime(before)); err != nil {
		return fmt.Errorf("caddy samples prune: %w", err)
	}
	return nil
}
