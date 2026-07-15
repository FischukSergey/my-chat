package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrSessionNotFound возвращается, когда сессия не найдена по заданному критерию.
var ErrSessionNotFound = errors.New("auth session not found")

// AuthSessionRepository реализует операции с таблицей auth_sessions.
type AuthSessionRepository struct {
	pool *pgxpool.Pool
}

// NewAuthSessionRepository создаёт AuthSessionRepository.
func NewAuthSessionRepository(s *Store) *AuthSessionRepository {
	return &AuthSessionRepository{pool: s.pool}
}

// Create сохраняет новую сессию в БД.
func (r *AuthSessionRepository) Create(ctx context.Context, s AuthSession) error {
	const q = `
		INSERT INTO auth_sessions
		            (id, user_id, family_id, token_hash, device_id, expires_at, revoked_at, rotated_from, created_at)
		VALUES      ($1, $2, $3, $4, $5, $6, NULL, $7, $8)`

	if _, err := r.pool.Exec(ctx, q,
		s.ID, s.UserID, s.FamilyID, s.TokenHash,
		s.DeviceID, s.ExpiresAt.UTC(),
		s.RotatedFrom, s.CreatedAt.UTC(),
	); err != nil {
		return fmt.Errorf("create auth session: %w", err)
	}

	return nil
}

// FindByTokenHash ищет активную запись по SHA-256 хешу refresh-токена.
// Возвращает ErrSessionNotFound, если запись отсутствует.
func (r *AuthSessionRepository) FindByTokenHash(ctx context.Context, hash string) (AuthSession, error) {
	const q = `
		SELECT id, user_id, family_id, token_hash, device_id,
		       expires_at, revoked_at, rotated_from, created_at
		  FROM auth_sessions
		 WHERE token_hash = $1`

	row := r.pool.QueryRow(ctx, q, hash)

	s, err := scanAuthSession(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return AuthSession{}, ErrSessionNotFound
		}

		return AuthSession{}, fmt.Errorf("find auth session by token hash: %w", err)
	}

	return s, nil
}

// RevokeSession устанавливает revoked_at = now() для конкретной сессии.
// Идемпотентна: повторный вызов на уже отозванной сессии не возвращает ошибку.
func (r *AuthSessionRepository) RevokeSession(ctx context.Context, sessionID string) error {
	const q = `
		UPDATE auth_sessions
		   SET revoked_at = NOW()
		 WHERE id = $1
		   AND revoked_at IS NULL`

	if _, err := r.pool.Exec(ctx, q, sessionID); err != nil {
		return fmt.Errorf("revoke auth session: %w", err)
	}

	return nil
}

// RevokeFamily отзывает все сессии в рамках одной family (reuse detection).
func (r *AuthSessionRepository) RevokeFamily(ctx context.Context, familyID string) error {
	const q = `
		UPDATE auth_sessions
		   SET revoked_at = NOW()
		 WHERE family_id = $1
		   AND revoked_at IS NULL`

	if _, err := r.pool.Exec(ctx, q, familyID); err != nil {
		return fmt.Errorf("revoke auth session family: %w", err)
	}

	return nil
}

// RevokeAllForUser отзывает все активные сессии пользователя (logout everywhere).
func (r *AuthSessionRepository) RevokeAllForUser(ctx context.Context, userID string) error {
	const q = `
		UPDATE auth_sessions
		   SET revoked_at = NOW()
		 WHERE user_id    = $1
		   AND revoked_at IS NULL`

	if _, err := r.pool.Exec(ctx, q, userID); err != nil {
		return fmt.Errorf("revoke all auth sessions for user: %w", err)
	}

	return nil
}

// RotateSession атомарно отзывает oldSessionID и создаёт newSession в одной транзакции.
// Если oldSessionID уже revoked, транзакция всё равно создаёт новую запись.
func (r *AuthSessionRepository) RotateSession(ctx context.Context, oldSessionID string, newSession AuthSession) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin rotate transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	const revokeQ = `
		UPDATE auth_sessions
		   SET revoked_at = NOW()
		 WHERE id = $1
		   AND revoked_at IS NULL`

	if _, err = tx.Exec(ctx, revokeQ, oldSessionID); err != nil {
		return fmt.Errorf("revoke old session in rotate: %w", err)
	}

	const insertQ = `
		INSERT INTO auth_sessions
		            (id, user_id, family_id, token_hash, device_id, expires_at, revoked_at, rotated_from, created_at)
		VALUES      ($1, $2, $3, $4, $5, $6, NULL, $7, $8)`

	if _, err = tx.Exec(ctx, insertQ,
		newSession.ID, newSession.UserID, newSession.FamilyID, newSession.TokenHash,
		newSession.DeviceID, newSession.ExpiresAt.UTC(),
		newSession.RotatedFrom, newSession.CreatedAt.UTC(),
	); err != nil {
		return fmt.Errorf("insert new session in rotate: %w", err)
	}

	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit rotate transaction: %w", err)
	}

	return nil
}

func scanAuthSession(row pgx.Row) (AuthSession, error) {
	var s AuthSession
	err := row.Scan(
		&s.ID,
		&s.UserID,
		&s.FamilyID,
		&s.TokenHash,
		&s.DeviceID,
		&s.ExpiresAt,
		&s.RevokedAt,
		&s.RotatedFrom,
		&s.CreatedAt,
	)
	if err != nil {
		return AuthSession{}, err
	}

	return s, nil
}

// IsRevoked возвращает true, если сессия отозвана или истёк срок действия.
func (s AuthSession) IsRevoked() bool {
	return s.RevokedAt != nil
}

// IsExpired возвращает true, если срок действия сессии истёк.
func (s AuthSession) IsExpired() bool {
	return time.Now().UTC().After(s.ExpiresAt)
}
