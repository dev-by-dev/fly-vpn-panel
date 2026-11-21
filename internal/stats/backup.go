package stats

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"time"

	"flypanel/internal/store"
)

const (
	backupEvery = 24 * time.Hour
	backupsKeep = 7
)

var backupName = regexp.MustCompile(`^panel-\d{8}\.db$`)

// RunBackup snapshots the database once immediately, then once per
// day, until ctx is cancelled. Backup failures are logged, never
// fatal.
func RunBackup(ctx context.Context, st *store.Store, dataDir string) {
	if ctx.Err() != nil {
		return
	}
	backupOnce(ctx, st, dataDir)
	ticker := time.NewTicker(backupEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			backupOnce(ctx, st, dataDir)
		}
	}
}

// backupOnce writes a VACUUM INTO snapshot for today and prunes old
// copies down to backupsKeep. A same-day restart is a no-op: VACUUM
// INTO refuses to overwrite an existing file, which is logged and
// skipped — fine at a daily cadence.
func backupOnce(ctx context.Context, st *store.Store, dataDir string) {
	if ctx.Err() != nil {
		return
	}
	dir := filepath.Join(dataDir, "backups")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Printf("stats: backup dir: %v", err)
		return
	}
	path := filepath.Join(dir, "panel-"+time.Now().UTC().Format("20060102")+".db")
	if _, err := st.DB().Exec(fmt.Sprintf("VACUUM INTO '%s'", path)); err != nil {
		log.Printf("stats: backup %s: %v", path, err)
		return
	}
	pruneBackups(dir)
}

// pruneBackups keeps the newest backupsKeep panel-YYYYMMDD.db copies;
// the names sort chronologically.
func pruneBackups(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		log.Printf("stats: backup prune: %v", err)
		return
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && backupName.MatchString(e.Name()) {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for _, name := range names[:max(0, len(names)-backupsKeep)] {
		if err := os.Remove(filepath.Join(dir, name)); err != nil {
			log.Printf("stats: backup prune %s: %v", name, err)
		}
	}
}
