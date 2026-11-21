package stats

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"flypanel/internal/store"
)

func TestBackupCreatesAndPrunes(t *testing.T) {
	dataDir := t.TempDir()
	st, err := store.Open(dataDir)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	// 8 stale copies predating the 7-copy retention window.
	bdir := filepath.Join(dataDir, "backups")
	if err := os.MkdirAll(bdir, 0o755); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 8; i++ {
		name := fmt.Sprintf("panel-202001%02d.db", i)
		if err := os.WriteFile(filepath.Join(bdir, name), []byte("stale"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	backupOnce(context.Background(), st, dataDir)

	today := "panel-" + time.Now().UTC().Format("20060102") + ".db"
	data, err := os.ReadFile(filepath.Join(bdir, today))
	if err != nil {
		t.Fatalf("backup not created: %v", err)
	}
	if !bytes.HasPrefix(data, []byte("SQLite format 3\x00")) {
		t.Fatalf("backup %s is not a SQLite database", today)
	}

	entries, err := os.ReadDir(bdir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 7 {
		t.Fatalf("backup count = %d, want 7 (newest kept)", len(entries))
	}
	for _, name := range []string{"panel-20200101.db", "panel-20200102.db"} {
		if _, err := os.Stat(filepath.Join(bdir, name)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s not pruned (err=%v)", name, err)
		}
	}
}
