package store

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"strings"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Migrate применяет SQL-миграции из internal/store/migrations.
func (s *Store) Migrate(ctx context.Context) error {
	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}

		path := "migrations/" + entry.Name()
		query, readErr := migrationsFS.ReadFile(path)
		if readErr != nil {
			return fmt.Errorf("read migration %q: %w", path, readErr)
		}

		if _, execErr := s.pool.Exec(ctx, string(query)); execErr != nil {
			return fmt.Errorf("apply migration %q: %w", path, execErr)
		}
	}

	return nil
}
