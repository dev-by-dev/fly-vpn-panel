package store

import (
	"testing"
	"time"
)

func TestAPITokenLifecycle(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open(): %v", err)
	}
	defer s.Close()

	exp := time.Now().Add(30 * 24 * time.Hour).UTC().Truncate(time.Second)
	id, err := s.APITokenInsert("ci", "read", "hash-1", exp)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	if _, err := s.APITokenInsert("ops", "full", "hash-2", time.Time{}); err != nil {
		t.Fatalf("insert never: %v", err)
	}

	list, err := s.APITokenList()
	if err != nil || len(list) != 2 {
		t.Fatalf("list: %v %d", err, len(list))
	}
	if list[0].Name != "ops" || list[0].Scope != "full" {
		t.Fatalf("unexpected order: %+v", list[0])
	}
	if !list[0].ExpiresAt.IsZero() {
		t.Fatalf("never token has expiry %v", list[0].ExpiresAt)
	}
	if !list[1].ExpiresAt.Equal(exp) {
		t.Fatalf("expiry %v != %v", list[1].ExpiresAt, exp)
	}

	got, ok, err := s.APITokenByHash("hash-1")
	if err != nil || !ok {
		t.Fatalf("by hash: %v %v", err, ok)
	}
	if got.ID != id || got.Revoked {
		t.Fatalf("unexpected row: %+v", got)
	}

	if _, ok, _ := s.APITokenByHash("nope"); ok {
		t.Fatal("unknown hash reported found")
	}

	if err := s.APITokenRevoke(id); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	got, ok, _ = s.APITokenByHash("hash-1")
	if !ok || !got.Revoked {
		t.Fatalf("revoke not persisted: %+v", got)
	}

	if err := s.APITokenRevoke(id); err != ErrNotFound {
		t.Fatalf("second revoke: %v", err)
	}
	if err := s.APITokenRevokeAll(); err != nil {
		t.Fatalf("revoke all: %v", err)
	}
	got, _, _ = s.APITokenByHash("hash-2")
	if !got.Revoked {
		t.Fatal("revoke all missed row")
	}
}
