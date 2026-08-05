package store

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
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
//
// Параллельные вызовы сериализуются через pg_advisory_lock на одном соединении
// пула (lock / SQL / unlock в одной session). Уже применённые файлы
// пропускаются по таблице schema_migrations — иначе повторный DDL
// (DROP/CREATE INDEX) дедлочит с параллельными integration-тестами.
func (s *Store) Migrate(ctx context.Context) (MigrationReport, error) {
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return MigrationReport{}, fmt.Errorf("acquire connection for migrate: %w", err)
	}
	// context.Background() внутри releaseMigrateConn — намеренно: unlock после cancel ctx.
	defer releaseMigrateConn(conn) //nolint:contextcheck // unlock must not depend on cancelled ctx

	if _, err = conn.Exec(ctx, "SELECT pg_advisory_lock($1)", migrationLockID); err != nil {
		return MigrationReport{}, fmt.Errorf("acquire migration lock: %w", err)
	}

	if _, err = conn.Exec(ctx, `
CREATE TABLE IF NOT EXISTS schema_migrations (
    filename   TEXT PRIMARY KEY,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
)`); err != nil {
		return MigrationReport{}, fmt.Errorf("ensure schema_migrations: %w", err)
	}

	// Уже существующая БД без учёта миграций: помечаем файлы как applied,
	// кроме ещё не внедрённых (012+). Иначе повторный прогон 001–011 гоняет DDL.
	if err = bootstrapSchemaMigrations(ctx, conn); err != nil {
		return MigrationReport{}, err
	}

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

		var already bool
		if err = conn.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE filename = $1)`,
			entry.Name(),
		).Scan(&already); err != nil {
			return MigrationReport{}, fmt.Errorf("check migration %q: %w", entry.Name(), err)
		}
		if already {
			continue
		}

		path := "migrations/" + entry.Name()
		query, readErr := migrationsFS.ReadFile(path)
		if readErr != nil {
			return MigrationReport{}, fmt.Errorf("read migration %q: %w", path, readErr)
		}

		if _, execErr := conn.Exec(ctx, string(query)); execErr != nil {
			return MigrationReport{}, fmt.Errorf("apply migration %q: %w", path, execErr)
		}

		if _, execErr := conn.Exec(ctx,
			`INSERT INTO schema_migrations (filename) VALUES ($1)`,
			entry.Name(),
		); execErr != nil {
			return MigrationReport{}, fmt.Errorf("record migration %q: %w", entry.Name(), execErr)
		}

		report.Applied = append(report.Applied, path)
	}

	return report, nil
}

// releaseMigrateConn снимает advisory-lock и возвращает соединение в пул.
// context.Background() — lock нужно снять даже при отмене исходного ctx.
func releaseMigrateConn(conn *pgxpool.Conn) {
	_, _ = conn.Exec(context.Background(), "SELECT pg_advisory_unlock($1)", migrationLockID)
	conn.Release()
}

// bootstrapCutoff — миграции с именем < cutoff считаются уже применёнными
// на «живой» БД без schema_migrations (prod / старые test DB).
// 012 — первая миграция, которая обязана реально выполниться после введения учёта.
const bootstrapCutoff = "012_auth_sessions_device_id_no_fk.sql"

func bootstrapSchemaMigrations(ctx context.Context, conn *pgxpool.Conn) error {
	var count int
	if err := conn.QueryRow(ctx, `SELECT COUNT(*) FROM schema_migrations`).Scan(&count); err != nil {
		return fmt.Errorf("count schema_migrations: %w", err)
	}
	if count > 0 {
		return nil
	}

	var usersExist bool
	if err := conn.QueryRow(ctx, `
SELECT EXISTS (
    SELECT 1 FROM information_schema.tables
    WHERE table_schema = 'public' AND table_name = 'users'
)`).Scan(&usersExist); err != nil {
		return fmt.Errorf("check users table: %w", err)
	}
	if !usersExist {
		return nil // чистая БД — применим все миграции с нуля
	}

	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("read migrations for bootstrap: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		if entry.Name() >= bootstrapCutoff {
			continue
		}
		if _, err = conn.Exec(ctx,
			`INSERT INTO schema_migrations (filename) VALUES ($1) ON CONFLICT DO NOTHING`,
			entry.Name(),
		); err != nil {
			return fmt.Errorf("bootstrap migration %q: %w", entry.Name(), err)
		}
	}

	return nil
}
