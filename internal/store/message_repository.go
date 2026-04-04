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
INSERT INTO messages (id, dialog_id, sender_id, body)
VALUES ($1, $2, $3, $4)
RETURNING id, dialog_id, sender_id, body, created_at`

	var created Message
	if err := r.poolDB.QueryRow(
		ctx,
		query,
		message.ID,
		message.DialogID,
		message.SenderID,
		message.Body,
	).Scan(
		&created.ID,
		&created.DialogID,
		&created.SenderID,
		&created.Body,
		&created.CreatedAt,
	); err != nil {
		return Message{}, fmt.Errorf("insert message: %w", err)
	}

	return created, nil
}

// GetByID возвращает сообщение по его идентификатору.
func (r *MessageRepository) GetByID(ctx context.Context, messageID string) (Message, error) {
	const query = `
SELECT id, dialog_id, sender_id, body, created_at
FROM messages
WHERE id = $1`

	var message Message
	if err := r.poolDB.QueryRow(ctx, query, messageID).Scan(
		&message.ID,
		&message.DialogID,
		&message.SenderID,
		&message.Body,
		&message.CreatedAt,
	); err != nil {
		return Message{}, fmt.Errorf("get message by id: %w", err)
	}

	return message, nil
}

// ListByDialog возвращает список сообщений для диалога с пагинацией.
func (r *MessageRepository) ListByDialog(ctx context.Context, dialogID string, limit int, before *time.Time) ([]Message, error) {
	const queryWithBefore = `
SELECT id, dialog_id, sender_id, body, created_at
FROM messages
WHERE dialog_id = $1 AND created_at < $2
ORDER BY created_at DESC
LIMIT $3`

	const queryWithoutBefore = `
SELECT id, dialog_id, sender_id, body, created_at
FROM messages
WHERE dialog_id = $1
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

type anyRows interface {
	Close()
	Err() error
	Next() bool
	Scan(dest ...any) error
}
