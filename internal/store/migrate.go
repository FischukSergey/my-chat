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

// migrationLockID — фиксированный идентификатор advisory-блокировки PostgreSQL,
// используемый для предотвращения параллельного применения миграций.
const migrationLockID = 1_000_000_001

// Migrate применяет SQL-миграции из internal/store/migrations.
// Защищён от параллельного запуска через PostgreSQL advisory lock.
func (s *Store) Migrate(ctx context.Context) (MigrationReport, error) {
	if _, err := s.pool.Exec(ctx, "SELECT pg_advisory_lock($1)", migrationLockID); err != nil {
		return MigrationReport{}, fmt.Errorf("acquire migration lock: %w", err)
	}
	// context.Background() intentional: must release lock even when ctx is cancelled.
	defer s.releaseMigrationLock() //nolint:contextcheck // see releaseMigrationLock

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

// releaseMigrationLock снимает advisory-блокировку. Использует context.Background(),
// чтобы снять блокировку даже если оригинальный контекст уже отменён.
func (s *Store) releaseMigrationLock() {
	_, _ = s.pool.Exec(context.Background(), "SELECT pg_advisory_unlock($1)", migrationLockID)
}
