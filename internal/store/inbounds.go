package store

import (
	"database/sql"
	"errors"
	"fmt"
)

const inboundCols = `id, tag, enabled, config_json, created_at`

func scanInbound(sc scanner) (*Inbound, error) {
	var (
		i         Inbound
		enabled   int
		createdAt string
	)
	if err := sc.Scan(&i.ID, &i.Tag, &enabled, &i.ConfigJSON, &createdAt); err != nil {
		return nil, err
	}
	i.Enabled = enabled != 0
	var err error
	if i.CreatedAt, err = parseTime(createdAt); err != nil {
		return nil, fmt.Errorf("inbound %q created_at: %w", i.Tag, err)
	}
	return &i, nil
}

// InboundCreate inserts in and sets in.ID to the assigned row id.
func (s *Store) InboundCreate(in *Inbound) error {
	res, err := s.DB().Exec(
		`INSERT INTO inbounds (tag, enabled, config_json, created_at) VALUES (?, ?, ?, ?)`,
		in.Tag, btoi(in.Enabled), in.ConfigJSON, fmtTime(in.CreatedAt))
	if err != nil {
		return fmt.Errorf("inbound create: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return fmt.Errorf("inbound create: last insert id: %w", err)
	}
	in.ID = id
	return nil
}

// InboundGet returns the inbound with the given id, or ErrNotFound.
func (s *Store) InboundGet(id int64) (*Inbound, error) {
	row := s.DB().QueryRow(`SELECT `+inboundCols+` FROM inbounds WHERE id = ?`, id)
	in, err := scanInbound(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("inbound get: %w", err)
	}
	return in, nil
}

// InboundList returns all inbounds ordered by tag.
func (s *Store) InboundList() ([]Inbound, error) {
	return s.inboundList(`SELECT ` + inboundCols + ` FROM inbounds ORDER BY tag`)
}

// InboundListEnabled returns enabled inbounds ordered by tag.
func (s *Store) InboundListEnabled() ([]Inbound, error) {
	return s.inboundList(`SELECT ` + inboundCols + ` FROM inbounds WHERE enabled = 1 ORDER BY tag`)
}

func (s *Store) inboundList(query string) ([]Inbound, error) {
	rows, err := s.DB().Query(query)
	if err != nil {
		return nil, fmt.Errorf("inbound list: %w", err)
	}
	defer rows.Close()
	var list []Inbound
	for rows.Next() {
		in, err := scanInbound(rows)
		if err != nil {
			return nil, fmt.Errorf("inbound list: %w", err)
		}
		list = append(list, *in)
	}
	return list, rows.Err()
}

// InboundUpdate writes the mutable fields of in; created_at is not changed.
func (s *Store) InboundUpdate(in *Inbound) error {
	res, err := s.DB().Exec(
		`UPDATE inbounds SET tag = ?, enabled = ?, config_json = ? WHERE id = ?`,
		in.Tag, btoi(in.Enabled), in.ConfigJSON, in.ID)
	if err != nil {
		return fmt.Errorf("inbound update: %w", err)
	}
	if n, err := res.RowsAffected(); err != nil {
		return fmt.Errorf("inbound update: rows affected: %w", err)
	} else if n == 0 {
		return ErrNotFound
	}
	return nil
}

// InboundDelete removes the inbound with the given id, or ErrNotFound.
func (s *Store) InboundDelete(id int64) error {
	res, err := s.DB().Exec(`DELETE FROM inbounds WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("inbound delete: %w", err)
	}
	if n, err := res.RowsAffected(); err != nil {
		return fmt.Errorf("inbound delete: rows affected: %w", err)
	} else if n == 0 {
		return ErrNotFound
	}
	return nil
}
