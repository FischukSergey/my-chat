package store

import (
	"context"
	"fmt"
	"time"
)

// ReceiptRepository работает с таблицей message_receipts.
type ReceiptRepository struct {
	poolDB db
}

// NewReceiptRepository создает репозиторий receipt-статусов.
func NewReceiptRepository(s *Store) *ReceiptRepository {
	return &ReceiptRepository{poolDB: s.pool}
}

// Ensure создает строку receipt для пользователя, если ее еще нет.
func (r *ReceiptRepository) Ensure(ctx context.Context, messageID, userID string) error {
	const query = `
INSERT INTO message_receipts (message_id, user_id)
VALUES ($1, $2)
ON CONFLICT (message_id, user_id) DO NOTHING`

	if _, err := r.poolDB.Exec(ctx, query, messageID, userID); err != nil {
		return fmt.Errorf("ensure receipt: %w", err)
	}

	return nil
}

// MarkDelivered фиксирует delivered_at.
func (r *ReceiptRepository) MarkDelivered(ctx context.Context, messageID, userID string, deliveredAt time.Time) error {
	const query = `
INSERT INTO message_receipts (message_id, user_id, delivered_at)
VALUES ($1, $2, $3)
ON CONFLICT (message_id, user_id)
DO UPDATE SET delivered_at = EXCLUDED.delivered_at`

	if _, err := r.poolDB.Exec(ctx, query, messageID, userID, deliveredAt); err != nil {
		return fmt.Errorf("mark delivered: %w", err)
	}

	return nil
}

// MarkRead фиксирует read_at и при необходимости delivered_at.
func (r *ReceiptRepository) MarkRead(ctx context.Context, messageID, userID string, readAt time.Time) error {
	const query = `
INSERT INTO message_receipts (message_id, user_id, delivered_at, read_at)
VALUES ($1, $2, $3, $3)
ON CONFLICT (message_id, user_id)
DO UPDATE SET
    delivered_at = COALESCE(message_receipts.delivered_at, EXCLUDED.delivered_at),
    read_at = EXCLUDED.read_at`

	if _, err := r.poolDB.Exec(ctx, query, messageID, userID, readAt); err != nil {
		return fmt.Errorf("mark read: %w", err)
	}

	return nil
}

// CountUnread возвращает количество непрочитанных сообщений пользователя.
func (r *ReceiptRepository) CountUnread(ctx context.Context, userID string) (int, error) {
	const query = `
SELECT COUNT(1)
FROM messages m
JOIN dialogs d ON d.id = m.dialog_id
LEFT JOIN message_receipts mr ON mr.message_id = m.id AND mr.user_id = $1
WHERE
    (d.user_a_id = $1 OR d.user_b_id = $1)
    AND m.sender_id <> $1
    AND mr.read_at IS NULL`

	var unreadCount int
	if err := r.poolDB.QueryRow(ctx, query, userID).Scan(&unreadCount); err != nil {
		return 0, fmt.Errorf("count unread: %w", err)
	}

	return unreadCount, nil
}
