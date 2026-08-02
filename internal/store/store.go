// Package store содержит работу с PostgreSQL для main-service.
package store

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store инкапсулирует подключение к PostgreSQL.
type Store struct {
	pool *pgxpool.Pool
}

// New создает пул подключений к PostgreSQL и проверяет доступность БД.
func New(ctx context.Context, dsn string) (*Store, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("create pgx pool: %w", err)
	}

	if err = pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	return &Store{pool: pool}, nil
}

// Close закрывает пул подключений.
func (s *Store) Close() {
	if s == nil || s.pool == nil {
		return
	}

	s.pool.Close()
}

// DB возвращает пул pgx для использования в репозиториях.
func (s *Store) DB() *pgxpool.Pool {
	return s.pool
}

// isUniqueViolation возвращает true, если ошибка — нарушение уникального ограничения PostgreSQL
// и имя индекса/ограничения содержит constraintName.
func isUniqueViolation(err error, constraintName string) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return strings.Contains(pgErr.ConstraintName, constraintName)
	}

	return false
}
