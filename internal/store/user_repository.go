package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// ErrUserNotFound возвращается, когда пользователь не найден.
var ErrUserNotFound = errors.New("user not found")

// UserRepository работает с таблицей users.
type UserRepository struct {
	poolDB db
}

// NewUserRepository создаёт репозиторий пользователей.
func NewUserRepository(s *Store) *UserRepository {
	return &UserRepository{poolDB: s.pool}
}

// FindByID возвращает пользователя по идентификатору.
// Возвращает ErrUserNotFound, если пользователь не найден.
func (r *UserRepository) FindByID(ctx context.Context, userID string) (User, error) {
	const query = `
SELECT id, status, created_at
FROM users
WHERE id = $1`

	var u User
	if err := r.poolDB.QueryRow(ctx, query, userID).Scan(
		&u.ID,
		&u.Status,
		&u.CreatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return User{}, ErrUserNotFound
		}
		return User{}, fmt.Errorf("find user by id: %w", err)
	}

	return u, nil
}
