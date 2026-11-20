package store

import (
	"embed"
	"fmt"
	"io/fs"
	"sort"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// migrationNames returns embedded SQL migrations in ascending order.
func migrationNames() ([]string, error) {
	names, err := fs.Glob(migrationsFS, "migrations/*.sql")
	if err != nil {
		return nil, fmt.Errorf("glob migrations: %w", err)
	}
	sort.Strings(names)
	return names, nil
}

// migrationVersion extracts the numeric version from a name like
// "migrations/0001_init.sql".
func migrationVersion(name string) (int, error) {
	var v int
	if _, err := fmt.Sscanf(name, "migrations/%d_", &v); err != nil {
		return 0, fmt.Errorf("bad migration name %q: %w", name, err)
	}
	return v, nil
}
