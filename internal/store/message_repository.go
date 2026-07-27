package store

import (
	"context"
	"fmt"
	"time"
)

// MessageRepository работает с таблицей messages.
type MessageRepository struct {
	poolDB db
}

// NewMessageRepository создает репозиторий сообщений.
func NewMessageRepository(s *Store) *MessageRepository {
	return &MessageRepository{poolDB: s.pool}
}

// Create вставляет новое сообщение.
func (r *MessageRepository) Create(ctx context.Context, message Message) (Message, error) {
	const query = `
INSERT INTO messages (id, dialog_id, sender_id, body, expires_at)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, dialog_id, sender_id, body, created_at, expires_at`

	var created Message
	if err := r.poolDB.QueryRow(
		ctx,
		query,
		message.ID,
		message.DialogID,
		message.SenderID,
		message.Body,
		message.ExpiresAt,
	).Scan(
		&created.ID,
		&created.DialogID,
		&created.SenderID,
		&created.Body,
		&created.CreatedAt,
		&created.ExpiresAt,
	); err != nil {
		return Message{}, fmt.Errorf("insert message: %w", err)
	}

	return created, nil
}

// GetByID возвращает сообщение по его идентификатору.
func (r *MessageRepository) GetByID(ctx context.Context, messageID string) (Message, error) {
	const query = `
SELECT id, dialog_id, sender_id, body, created_at, expires_at
FROM messages
WHERE id = $1 AND deleted_at IS NULL`

	var message Message
	if err := r.poolDB.QueryRow(ctx, query, messageID).Scan(
		&message.ID,
		&message.DialogID,
		&message.SenderID,
		&message.Body,
		&message.CreatedAt,
		&message.ExpiresAt,
	); err != nil {
		return Message{}, fmt.Errorf("get message by id: %w", err)
	}

	return message, nil
}

// ListByDialog возвращает список активных сообщений для диалога с пагинацией.
func (r *MessageRepository) ListByDialog(ctx context.Context, dialogID string, limit int, before *time.Time) ([]Message, error) {
	const queryWithBefore = `
SELECT id, dialog_id, sender_id, body, created_at, expires_at
FROM messages
WHERE dialog_id = $1 AND created_at < $2 AND deleted_at IS NULL
ORDER BY created_at DESC
LIMIT $3`

	const queryWithoutBefore = `
SELECT id, dialog_id, sender_id, body, created_at, expires_at
FROM messages
WHERE dialog_id = $1 AND deleted_at IS NULL
ORDER BY created_at DESC
LIMIT $2`

	var (
		rows anyRows
		err  error
	)

	if before != nil {
		//nolint:sqlclosecheck // closed via defer rows.Close() below
		rows, err = r.poolDB.Query(ctx, queryWithBefore, dialogID, *before, limit)
	} else {
		//nolint:sqlclosecheck // closed via defer rows.Close() below
		rows, err = r.poolDB.Query(ctx, queryWithoutBefore, dialogID, limit)
	}
	if err != nil {
		return nil, fmt.Errorf("select messages by dialog: %w", err)
	}
	defer rows.Close()

	items := make([]Message, 0, limit)
	for rows.Next() {
		var message Message
		if err = rows.Scan(
			&message.ID,
			&message.DialogID,
			&message.SenderID,
			&message.Body,
			&message.CreatedAt,
			&message.ExpiresAt,
		); err != nil {
			return nil, fmt.Errorf("scan message row: %w", err)
		}

		items = append(items, message)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate message rows: %w", err)
	}

	return items, nil
}

// SetExpiresAt устанавливает expires_at для сообщения при первом прочтении.
// Идемпотентен: если expires_at уже задан, запрос не меняет строку (WHERE expires_at IS NULL).
func (r *MessageRepository) SetExpiresAt(ctx context.Context, messageID string, expiresAt time.Time) error {
	const query = `
UPDATE messages
SET expires_at = $2
WHERE id = $1 AND expires_at IS NULL`

	if _, err := r.poolDB.Exec(ctx, query, messageID, expiresAt); err != nil {
		return fmt.Errorf("set message expires_at: %w", err)
	}

	return nil
}

// ExpiredMessage содержит минимальные данные об истёкшем сообщении для broadcast WS-события.
type ExpiredMessage struct {
	ID       string
	DialogID string
	SenderID string
	UserAID  string // user_a_id из таблицы dialogs
	UserBID  string // user_b_id из таблицы dialogs
}

// ExpireMessages проставляет deleted_at для всех сообщений с истёкшим expires_at
// и возвращает список затронутых сообщений для broadcast WS-события message_deleted.
// Метод идемпотентен: повторный вызов с тем же now не затронет уже помеченные записи.
// batchSize ограничивает число обрабатываемых сообщений за одну итерацию.
// Возвращает оба участника диалога (UserAID, UserBID) для рассылки WS-событий.
func (r *MessageRepository) ExpireMessages(ctx context.Context, now time.Time, batchSize int) ([]ExpiredMessage, error) {
	// CTE в два шага:
	// 1. to_expire — блокируем строки в messages (без JOIN на dialogs, чтобы не лочить лишнее).
	// 2. expired — UPDATE и RETURNING.
	// Финальный SELECT джойнит dialogs, чтобы получить обоих участников диалога.
	const query = `
WITH to_expire AS (
    SELECT id, dialog_id
    FROM messages
    WHERE expires_at <= $1 AND deleted_at IS NULL
    LIMIT $2
    FOR UPDATE SKIP LOCKED
),
expired AS (
    UPDATE messages
    SET deleted_at = $1
    FROM to_expire
    WHERE messages.id = to_expire.id
    RETURNING messages.id, messages.dialog_id, messages.sender_id
)
SELECT e.id, e.dialog_id, e.sender_id, d.user_a_id, d.user_b_id
FROM expired e
JOIN dialogs d ON d.id = e.dialog_id`

	rows, err := r.poolDB.Query(ctx, query, now, batchSize)
	if err != nil {
		return nil, fmt.Errorf("expire messages: %w", err)
	}
	defer rows.Close()

	var expired []ExpiredMessage
	for rows.Next() {
		var m ExpiredMessage
		if err = rows.Scan(&m.ID, &m.DialogID, &m.SenderID, &m.UserAID, &m.UserBID); err != nil {
			return nil, fmt.Errorf("scan expired message row: %w", err)
		}
		expired = append(expired, m)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate expired message rows: %w", err)
	}

	return expired, nil
}

type anyRows interface {
	Close()
	Err() error
	Next() bool
	Scan(dest ...any) error
}
