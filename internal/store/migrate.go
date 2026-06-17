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

// MigrationReport содержит результат применения миграций.
type MigrationReport struct {
	Applied []string
}

// Migrate применяет SQL-миграции из internal/store/migrations.
func (s *Store) Migrate(ctx context.Context) (MigrationReport, error) {
	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return MigrationReport{}, fmt.Errorf("read migrations dir: %w", err)
	}

	report := MigrationReport{
		Applied: make([]string, 0, len(entries)),
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}

		path := "migrations/" + entry.Name()
		query, readErr := migrationsFS.ReadFile(path)
		if readErr != nil {
			return MigrationReport{}, fmt.Errorf("read migration %q: %w", path, readErr)
		}

		if _, execErr := s.pool.Exec(ctx, string(query)); execErr != nil {
			return MigrationReport{}, fmt.Errorf("apply migration %q: %w", path, execErr)
		}

		report.Applied = append(report.Applied, path)
	}

	return report, nil
}
