package store

import (
	"testing"
)

var allTables = []string{
	"admin", "backup_codes", "users", "inbounds",
	"settings", "traffic_samples", "devices", "refresh_tokens",
}

func schemaVersion(t *testing.T, s *Store) int {
	t.Helper()
	var v int
	if err := s.DB().QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_version").Scan(&v); err != nil {
		t.Fatalf("read schema_version: %v", err)
	}
	return v
}

func TestOpenMigrates(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open(): %v", err)
	}
	defer s.Close()

	if got := schemaVersion(t, s); got != 2 {
		t.Errorf("schema version = %d, want 2", got)
	}
	for _, table := range allTables {
		var name string
		err := s.DB().QueryRow(
			"SELECT name FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&name)
		if err != nil {
			t.Errorf("table %q missing: %v", table, err)
			continue
		}
		if name != table {
			t.Errorf("table name = %q, want %q", name, table)
		}
	}
}

func TestOpenIdempotent(t *testing.T) {
	dir := t.TempDir()
	s1, err := Open(dir)
	if err != nil {
		t.Fatalf("first Open(): %v", err)
	}
	s1.Close()

	s2, err := Open(dir)
	if err != nil {
		t.Fatalf("second Open(): %v", err)
	}
	defer s2.Close()

	if got := schemaVersion(t, s2); got != 2 {
		t.Errorf("schema version after reopen = %d, want 2", got)
	}
}

func TestWALPragmas(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open(): %v", err)
	}
	defer s.Close()

	var journal string
	if err := s.DB().QueryRow("PRAGMA journal_mode").Scan(&journal); err != nil {
		t.Fatalf("journal_mode: %v", err)
	}
	if journal != "wal" {
		t.Errorf("journal_mode = %q, want wal", journal)
	}
	var busy int
	if err := s.DB().QueryRow("PRAGMA busy_timeout").Scan(&busy); err != nil {
		t.Fatalf("busy_timeout: %v", err)
	}
	if busy != 5000 {
		t.Errorf("busy_timeout = %d, want 5000", busy)
	}
	var sync int
	if err := s.DB().QueryRow("PRAGMA synchronous").Scan(&sync); err != nil {
		t.Fatalf("synchronous: %v", err)
	}
	if sync != 2 {
		t.Errorf("synchronous = %d, want 2 (FULL)", sync)
	}
}
