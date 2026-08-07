package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// ErrUserNotFound возвращается, когда пользователь не найден.
var ErrUserNotFound = errors.New("user not found")

// ErrUsernameTaken возвращается при попытке зарегистрировать уже занятый username.
var ErrUsernameTaken = errors.New("username already taken")

// UserRepository работает с таблицей users.
type UserRepository struct {
	poolDB db
}

// NewUserRepository создаёт репозиторий пользователей.
func NewUserRepository(s *Store) *UserRepository {
	return &UserRepository{poolDB: s.pool}
}

// Create вставляет нового пользователя.
// Возвращает ErrUsernameTaken при конфликте уникального индекса по username.
func (r *UserRepository) Create(ctx context.Context, u User) (User, error) {
	const query = `
INSERT INTO users (id, status, username, password_hash)
VALUES ($1, $2, $3, $4)
RETURNING id, status, username, password_hash, created_at`

	var created User
	if err := r.poolDB.QueryRow(ctx, query, u.ID, u.Status, u.Username, u.PasswordHash).Scan(
		&created.ID,
		&created.Status,
		&created.Username,
		&created.PasswordHash,
		&created.CreatedAt,
	); err != nil {
		if isUniqueViolation(err, "idx_users_username") {
			return User{}, ErrUsernameTaken
		}
		return User{}, fmt.Errorf("create user: %w", err)
	}

	return created, nil
}

// FindByUsername возвращает пользователя по username (сравнение без учёта регистра).
// Возвращает ErrUserNotFound, если пользователь не найден.
func (r *UserRepository) FindByUsername(ctx context.Context, username string) (User, error) {
	const query = `
SELECT id, status, username, password_hash, created_at
FROM users
WHERE lower(username) = lower($1)`

	var u User
	if err := r.poolDB.QueryRow(ctx, query, username).Scan(
		&u.ID,
		&u.Status,
		&u.Username,
		&u.PasswordHash,
		&u.CreatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return User{}, ErrUserNotFound
		}
		return User{}, fmt.Errorf("find user by username: %w", err)
	}

	return u, nil
}

// FindByID возвращает пользователя по идентификатору.
// Возвращает ErrUserNotFound, если пользователь не найден.
func (r *UserRepository) FindByID(ctx context.Context, userID string) (User, error) {
	const query = `
SELECT id, status, username, password_hash, created_at
FROM users
WHERE id = $1`

	var u User
	if err := r.poolDB.QueryRow(ctx, query, userID).Scan(
		&u.ID,
		&u.Status,
		&u.Username,
		&u.PasswordHash,
		&u.CreatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return User{}, ErrUserNotFound
		}
		return User{}, fmt.Errorf("find user by id: %w", err)
	}

	return u, nil
}

// SearchByUsernamePrefix возвращает active-пользователей с username по prefix (case-insensitive).
// Исключает excludeUserID и пустые username. Сортировка: username ASC.
func (r *UserRepository) SearchByUsernamePrefix(
	ctx context.Context,
	prefix, excludeUserID string,
	limit int,
) ([]User, error) {
	const query = `
SELECT id, status, username, password_hash, created_at
FROM users
WHERE status = 'active'
  AND username <> ''
  AND id <> $2
  AND starts_with(lower(username), lower($1))
ORDER BY username ASC
LIMIT $3`

	rows, err := r.poolDB.Query(ctx, query, prefix, excludeUserID, limit)
	if err != nil {
		return nil, fmt.Errorf("search users by username prefix: %w", err)
	}
	defer rows.Close()

	users := make([]User, 0, limit)
	for rows.Next() {
		var u User
		if err = rows.Scan(
			&u.ID,
			&u.Status,
			&u.Username,
			&u.PasswordHash,
			&u.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan user search row: %w", err)
		}
		users = append(users, u)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate user search rows: %w", err)
	}

	return users, nil
}
